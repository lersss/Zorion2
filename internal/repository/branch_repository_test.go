// internal/repository/branch_repository_test.go
//
// Тесты БД-пути ленивого синка веток поселения (спека 2026-09-22-поселение-
// ветка-буферы-переработка §4.2 + спека итерации 3 §12): персистентный путь
// (Δt ≥ MinPersistInterval) берёт блокировку `FOR UPDATE OF b` только на строке
// ветки, делает top-up строк входа (T14), читает залежи планеты под блокировкой
// (`FOR UPDATE ORDER BY id`) и пишет буферы/залежи/processed_at; путь «в памяти»
// (Δt < порога) и Δt ≤ 0 — только чтение (в т.ч. залежей без блокировки), без
// записи. Запросы утверждаются дословно (QueryMatcherEqual).
package repository

import (
	"database/sql/driver"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// SQL персистентного пути — дословно (stripQuery нормализует пробелы).
const (
	sqlBranchesBySettlements = `SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at, r.good_id, og.name, r.complexity FROM settlement_branches b JOIN recipes r ON r.id = b.recipe_id JOIN goods og ON og.id = r.good_id WHERE b.settlement_id = ANY($1) ORDER BY b.created_at ASC, b.id ASC`
	sqlBranchComponents      = `SELECT rc.recipe_id, rc.component_id, rc.quantity FROM recipe_components rc WHERE rc.recipe_id = ANY($1) AND rc.component_id IS NOT NULL ORDER BY rc.recipe_id, rc.pos`
	sqlBranchBuffers         = `SELECT bb.branch_id, bb.direction, bb.good_id, g.name, bb.amount FROM settlement_branch_buffers bb JOIN goods g ON g.id = bb.good_id WHERE bb.branch_id = ANY($1) ORDER BY bb.branch_id, bb.direction, bb.good_id`
	sqlBranchByIDLock        = `SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at, r.good_id, og.name, r.complexity FROM settlement_branches b JOIN recipes r ON r.id = b.recipe_id JOIN goods og ON og.id = r.good_id WHERE b.id = $1 FOR UPDATE OF b`
	sqlTopUpInput            = `INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount) VALUES ($1, 'input', $2, 0) ON CONFLICT (branch_id, direction, good_id) DO NOTHING`
	sqlWriteInput            = `UPDATE settlement_branch_buffers SET amount = $1, updated_at = NOW() WHERE branch_id = $2 AND direction = 'input' AND good_id = $3`
	sqlWriteOutput           = `INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount) VALUES ($1, 'output', $2, $3) ON CONFLICT (branch_id, direction, good_id) DO UPDATE SET amount = EXCLUDED.amount, updated_at = NOW()`
	sqlWriteCheckpoint       = `UPDATE settlement_branches SET processed_at = $1, updated_at = NOW() WHERE id = $2`
	sqlDepositsForUpdate     = `SELECT id, good_id, amount FROM deposits WHERE planet_id = $1 AND good_id = ANY($2) AND amount > 0 ORDER BY id FOR UPDATE`
	sqlDepositsMemory        = `SELECT planet_id, id, good_id, amount FROM deposits WHERE planet_id = ANY($1) AND good_id = ANY($2) AND amount > 0 ORDER BY planet_id, id`
	sqlWriteDeposit          = `UPDATE deposits SET amount = $1, updated_at = NOW() WHERE id = $2`
)

// settlementPlanets — карта «поселение → планета» для attachBranches (§4.2).
func settlementPlanets() map[string]string { return map[string]string{"s1": "p1"} }

func branchRows(processedAt time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity"}).
		AddRow("b1", "s1", int64(69), processedAt, int64(378), "Пища", int64(1))
}

func branchComponentRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).
		AddRow(int64(69), int64(359), 1)
}

func branchBufferRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
		AddRow("b1", "output", int64(378), "Пища", 0.0).
		AddRow("b1", "input", int64(359), "Мясо", 100.0)
}

// branchBufferRowsEmptyInput — вход пуст (проходной), залежь — единственный
// источник добычи (T1).
func branchBufferRowsEmptyInput() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
		AddRow("b1", "output", int64(378), "Пища", 0.0).
		AddRow("b1", "input", int64(359), "Мясо", 0.0)
}

func branchDepositRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "good_id", "amount"}).
		AddRow("dep1", int64(359), 5000.0)
}

func branchMemoryDepositRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"planet_id", "id", "good_id", "amount"}).
		AddRow("p1", "dep1", int64(359), 5000.0)
}

// expectBranchLoads — три запроса фазы загрузки (ветки, компоненты, буферы).
func expectBranchLoads(mock sqlmock.Sqlmock, processedAt time.Time) {
	mock.ExpectQuery(sqlBranchesBySettlements).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows(processedAt))
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchComponentRows())
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRows())
}

// amountNear — argument-matcher для абсолютной записи amount (float).
type amountNear struct{ want float64 }

func (m amountNear) Match(v driver.Value) bool {
	f, ok := v.(float64)
	return ok && math.Abs(f-m.want) < 1e-6
}

// TestSyncBranchesPersistentPath — Δt ≥ MinPersistInterval → tx-путь: `FOR UPDATE
// OF b` на ветке, top-up строки входа, `FOR UPDATE` залежей планеты, запись
// входа/выхода/залежей и processed_at.
func TestSyncBranchesPersistentPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	expectBranchLoads(mock, now.Add(-time.Hour))

	mock.ExpectBegin()
	mock.ExpectQuery(sqlBranchByIDLock).WithArgs("b1").WillReturnRows(branchRows(now.Add(-time.Hour)))
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchComponentRows())
	// Top-up: строка уже есть (RowsAffected 0) — «не перетирается».
	mock.ExpectExec(sqlTopUpInput).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRows())
	// Залежь планеты под блокировкой (порядок локов: ветка → залежи).
	mock.ExpectQuery(sqlDepositsForUpdate).WithArgs("p1", sqlmock.AnyArg()).WillReturnRows(branchDepositRows())
	mock.ExpectExec(sqlWriteInput).WithArgs(sqlmock.AnyArg(), "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteOutput).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	// Входу (100) хватает на нужду за час — залежь записывается без убыли.
	mock.ExpectExec(sqlWriteDeposit).WithArgs(amountNear{5000}, "dep1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteCheckpoint).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, out["s1"], 1)
	b := out["s1"][0]
	require.Greater(t, b.Output[0].Amount, 0.0, "выход должен прирасти")
	require.Less(t, b.Input[0].Amount, 100.0, "вход должен убыть")
	require.WithinDuration(t, now, b.ProcessedAt, time.Second, "чек-точка продвинулась")
}

// TestSyncBranchesPersistentExtractsDeposit — T1: пустой вход, большая залежь,
// Δt ≥ порога → залежь убыла на batches×quantity, выход вырос, вход не вырос
// (проходной).
func TestSyncBranchesPersistentExtractsDeposit(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(sqlBranchesBySettlements).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows(now.Add(-time.Hour)))
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchComponentRows())
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRowsEmptyInput())

	mock.ExpectBegin()
	mock.ExpectQuery(sqlBranchByIDLock).WithArgs("b1").WillReturnRows(branchRows(now.Add(-time.Hour)))
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchComponentRows())
	mock.ExpectExec(sqlTopUpInput).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRowsEmptyInput())
	mock.ExpectQuery(sqlDepositsForUpdate).WithArgs("p1", sqlmock.AnyArg()).WillReturnRows(branchDepositRows())
	mock.ExpectExec(sqlWriteInput).WithArgs(amountNear{0}, "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteOutput).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	// desired за час при P=1e9 → ≈27.8 батча; залежь убыла на эту величину.
	mock.ExpectExec(sqlWriteDeposit).WithArgs(amountNear{5000 - 27.8}, "dep1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteCheckpoint).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	b := out["s1"][0]
	require.Equal(t, 0.0, b.Input[0].Amount, "вход проходной")
	require.Greater(t, b.Output[0].Amount, 0.0)
}

// TestSyncBranchesMemoryPath — T8: Δt < MinPersistInterval → пересчёт только в
// памяти: залежи читаются без блокировки, ни транзакции, ни записей (deposits
// не меняется).
func TestSyncBranchesMemoryPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	expectBranchLoads(mock, now.Add(-time.Minute))
	// Залежь читается, но не блокируется и не пишется.
	mock.ExpectQuery(sqlDepositsMemory).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(branchMemoryDepositRows())

	out, err := NewBranchRepository(db).SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, out["s1"], 1)
	// Добор посчитан в памяти (выход вырос), но BEGIN/Exec не было — запись не шла.
	require.Greater(t, out["s1"][0].Output[0].Amount, 0.0, "добор в памяти виден в карточке")
}

// TestSyncBranchesZeroDeltaNoWrite — идемпотентность: чек-точка в будущем
// (Δt ≤ 0) → ни одного UPDATE/INSERT, буферы и processed_at не меняются.
func TestSyncBranchesZeroDeltaNoWrite(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	future := now.Add(time.Hour)
	expectBranchLoads(mock, future)
	// Δt ≤ 0 < порога → путь «в памяти»: залежи читаются без записи.
	mock.ExpectQuery(sqlDepositsMemory).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(branchMemoryDepositRows())
	// Ни Begin, ни Exec не ожидаются: любой запрос записи уронит тест.

	out, err := NewBranchRepository(db).SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, out["s1"], 1)
	require.Equal(t, future.Unix(), out["s1"][0].ProcessedAt.Unix(), "processed_at не двигается при Δt ≤ 0")
	require.Equal(t, 100.0, out["s1"][0].Input[0].Amount, "вход не меняется")
}

// TestSyncBranchesTopUpNewComponent — T14: новый заполненный компонент рецепта
// без строки входа → top-up создаёт строку `input=0` (RowsAffected 1);
// существующая строка второго компонента не перетирается (DO NOTHING, 0).
func TestSyncBranchesTopUpNewComponent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(sqlBranchesBySettlements).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows(now.Add(-time.Hour)))
	// Два заполненных компонента: 359 (строка есть), 358 (новая строка).
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).
			AddRow(int64(69), int64(359), 1).
			AddRow(int64(69), int64(358), 1))
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRows())

	mock.ExpectBegin()
	mock.ExpectQuery(sqlBranchByIDLock).WithArgs("b1").WillReturnRows(branchRows(now.Add(-time.Hour)))
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).
			AddRow(int64(69), int64(359), 1).
			AddRow(int64(69), int64(358), 1))
	mock.ExpectExec(sqlTopUpInput).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(sqlTopUpInput).WithArgs("b1", int64(358)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRows())
	// Залежей у планеты нет — пустая выборка (запись залежей не идёт).
	mock.ExpectQuery(sqlDepositsForUpdate).WithArgs("p1", sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "good_id", "amount"}))
	mock.ExpectExec(sqlWriteInput).WithArgs(sqlmock.AnyArg(), "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteInput).WithArgs(sqlmock.AnyArg(), "b1", int64(358)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteOutput).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteCheckpoint).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, err = NewBranchRepository(db).SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSyncBranchesNoDoubleAccountingMemoryThenPersist — T9: путь «в памяти» не
// двигает чек-точку и не пишет; следующий персистентный синк считает Δt от
// СОХРАНЁННОЙ чек-точки и забирает объём один раз (суммарное списание — одному
// проходу).
func TestSyncBranchesNoDoubleAccountingMemoryThenPersist(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	checkpoint := now.Add(-time.Hour)
	repo := NewBranchRepository(db)

	// 1) «Простой визит»: Δt = 1 минута < порога — чтение залежей без записи,
	//    чек-точка в результате не сдвинулась.
	expectBranchLoads(mock, now.Add(-time.Minute))
	mock.ExpectQuery(sqlDepositsMemory).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(branchMemoryDepositRows())
	out, err := repo.SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err)
	// В память чек-точка вынесена (карточка свежая), но в БД НЕ записана —
	// ExpectationsWereMet докажет отсутствие Begin/Exec.
	require.Equal(t, now.Unix(), out["s1"][0].ProcessedAt.Unix(), "память показывает свежий расчёт")
	require.NoError(t, mock.ExpectationsWereMet())

	// 2) Персистентный синк от той же сохранённой чек-точки (now − 1 час):
	//    залежь убывает ровно на один проход, а не на два.
	mock.ExpectQuery(sqlBranchesBySettlements).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows(checkpoint))
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchComponentRows())
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRowsEmptyInput())

	mock.ExpectBegin()
	mock.ExpectQuery(sqlBranchByIDLock).WithArgs("b1").WillReturnRows(branchRows(checkpoint))
	mock.ExpectQuery(sqlBranchComponents).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchComponentRows())
	mock.ExpectExec(sqlTopUpInput).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(sqlBranchBuffers).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchBufferRowsEmptyInput())
	mock.ExpectQuery(sqlDepositsForUpdate).WithArgs("p1", sqlmock.AnyArg()).WillReturnRows(branchDepositRows())
	mock.ExpectExec(sqlWriteInput).WithArgs(amountNear{0}, "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteOutput).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteDeposit).WithArgs(amountNear{5000 - 27.8}, "dep1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlWriteCheckpoint).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, err = repo.SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSyncBranchesBranchDeletedNoOp — ветку удалили между loadBranches и
// FOR UPDATE: ErrBranchNotFound не роняет чтение (карточка GET planets не 500),
// ветка просто не попадает в результат.
func TestSyncBranchesBranchDeletedNoOp(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	expectBranchLoads(mock, now.Add(-time.Hour))

	mock.ExpectBegin()
	mock.ExpectQuery(sqlBranchByIDLock).WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity"}))
	mock.ExpectRollback()

	out, err := NewBranchRepository(db).SyncBranches([]string{"s1"}, map[string]float64{"s1": 1e9}, settlementPlanets(), now)
	require.NoError(t, err, "удалённая ветка — no-op, не 500")
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, out["s1"], "удалённой ветки в результате нет")
}

// T13 (каскад goods → recipes → settlement_branches → buffers; goods → deposits)
// — sqlmock не выражает FK-каскады: проверяется только живым прогоном (@tester,
// живая БД). Новых предупреждений студии итерация 3 не вводит.
func TestBranchCascadeOnlyLive(t *testing.T) {
	t.Skip("каскад FK выражается только живой БД — sqlmock его не исполняет (передать @tester)")
}

// T11 (порядок локов/дедлок: два параллельных синка веток с пересекающимися
// залежами) — требует живой Postgres с двумя транзакциями; в юнит-прогоне
// недоступно. Рантайм-проверка под `-race` — за @tester.
func TestSyncBranchesLockOrderOnlyLive(t *testing.T) {
	t.Skip("дедлок/порядок локов ветка→залежи выражается только живой БД (передать @tester)")
}

// T15 (студия + дедлок: DeleteGood ресурса в обеих ролях параллельно синку) —
// явный риск §4.4 спеки итерации 3; рантайм-проверка живого сервера за @tester.
func TestSyncBranchesDeleteGoodDeadlockOnlyLive(t *testing.T) {
	t.Skip("DeleteGood ∥ синк веток — только живая БД под -race (передать @tester)")
}
