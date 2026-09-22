// internal/repository/branch_repository_test.go
//
// Тесты owner-прохода поселения (спека 2026-09-22-эффекты-снабжения-задержка-
// голод §4.1/§4.5): персистентный путь — одна транзакция на поселение
// (`pg_advisory_xact_lock(hashtext(settlement_id))` → settlements FOR UPDATE →
// ветки FOR UPDATE (порядок id) → залежи FOR UPDATE → запись буферов/залежей/
// processed_at + UPSERT active_effects + население), путь «в памяти» (Δt <
// MinPersistInterval) — без локов и записи. Запросы утверждаются дословно.
package repository

import (
	"database/sql/driver"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
)

// amountNear — argument-matcher для абсолютной записи amount (float).
type amountNear struct{ want float64 }

func (m amountNear) Match(v driver.Value) bool {
	f, ok := v.(float64)
	return ok && math.Abs(f-m.want) < 1e-6
}

// ownerBranchRows — ветка b1 (рецепт 69, выход 378 «Пища», сложность 1) с
// категорией выхода `category` (позиция корзины, §4.2).
func ownerBranchRows(processedAt time.Time, category string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}).
		AddRow("b1", "s1", int64(69), processedAt, int64(378), "Пища", int64(1), category)
}

func ownerComponentRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).
		AddRow(int64(69), int64(359), 1)
}

func ownerBufferRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
		AddRow("b1", "output", int64(378), "Пища", 0.0).
		AddRow("b1", "input", int64(359), "Мясо", 1e15)
}

// ownerInput — вход owner-прохода: поселение s1 (1e9) с привязкой позиции
// «продовольствие» → тип «голод», норма 2.5e-8.
func ownerInput(computedAt time.Time) OwnerSettlement {
	return OwnerSettlement{
		ID:                "s1",
		PlanetID:          "p1",
		Population:        1_000_000_000,
		PopulationExact:   1_000_000_000,
		ComputedAt:        computedAt,
		CreatedAt:         computedAt,
		Planet:            settlement.PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5},
		EatByPosition:     map[string]float64{"продовольствие": 2.5e-8},
		EffectsByPosition: map[string]string{"продовольствие": "голод"},
	}
}

// TestSyncSettlementsPersistentPath — Δt ≥ MinPersistInterval → tx-путь:
// advisory-лок, settlements FOR UPDATE, ветки FOR UPDATE, залежи FOR UPDATE,
// запись буфера `O0_b + batches_b − drawn_b`, UPSERT активного эффекта
// (load = 1 сило-час за час без источника покрытия) и население.
func TestSyncSettlementsPersistentPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-time.Hour)
	// Категория ветки «металлы» — источника позиции «продовольствие» нет →
	// coverage = 0, w = 1 весь час (лог position_no_source) → load = 1; строка
	// нагрузки уже существует (базис load_at = computedAt).
	expectOwnerPassWithBranches(mock, ownerBranchRows(computedAt, "металлы"), ownerComponentRows(), ownerBufferRows(),
		activeEffectRows(int64(1), "продовольствие", 0.0, computedAt, "population_rate", "hunger", "s1"))

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, ""))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "металлы"))
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBufferRows())
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectExec(branchTopUpInputSQL).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}))
	mock.ExpectExec(branchWriteInputSQL).WithArgs(amountNear{1e15 - 27.8}, "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteOutputSQL).WithArgs("b1", int64(378), amountNear{27.8}).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "продовольствие", sqlmock.AnyArg(), amountNear{1.0}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(computedAt)})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Branches, 1)
	require.InDelta(t, 27.8, res.Branches[0].Output[0].Amount, 1e-6, "выход = O0_b + batches_b − drawn_b (drawn=0)")
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 1.0, res.Effects[0].Load, 1e-9, "час без источника покрытия → load = 1")
	require.False(t, res.Effects[0].Enabled, "порог 24 не достигнут")
}

// TestSyncSettlementsMemoryPath — Δt < MinPersistInterval → без локов и записи;
// базис не двигается.
func TestSyncSettlementsMemoryPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	expectOwnerPassNoBranches(mock, nil)
	// Ни одной ExpectBegin/Exec: любой запрос записи уронит тест.

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(now.Add(-time.Minute))})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, out["s1"].Branches)
}

// TestSyncSettlementsMemoryPathWithBranches — ветки есть, но Δt у них мал →
// пересчёт только в памяти: залежи читаются без блокировки, записи нет.
func TestSyncSettlementsMemoryPathWithBranches(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	processedAt := now.Add(-time.Minute)
	expectOwnerPassWithBranches(mock, ownerBranchRows(processedAt, "продовольствие"), ownerComponentRows(), ownerBufferRows(), nil)
	// Залежи пути «в памяти» — только чтение.
	mock.ExpectQuery(depositMemorySelectSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"planet_id", "id", "good_id", "amount"}).AddRow("p1", "dep1", int64(359), 5000.0))

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(now.Add(-time.Minute))})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, out["s1"].Branches, 1)
}

// T24 (инвариант базиса): после персистентного прохода ветка штампуется тем же
// now, что load_at и computed_at; порядок и состав персистов не меняют результат.
func TestSyncSettlementsBasisInvariant(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-2 * time.Hour)
	expectOwnerPassWithBranches(mock, ownerBranchRows(computedAt, "продовольствие"), ownerComponentRows(), ownerBufferRows(), nil)

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, ""))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "продовольствие"))
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBufferRows())
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectExec(branchTopUpInputSQL).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}))
	mock.ExpectExec(branchWriteInputSQL).WithArgs(sqlmock.AnyArg(), "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteOutputSQL).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(sqlmock.AnyArg(), "s1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(computedAt)})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.WithinDuration(t, now, out["s1"].Branches[0].ProcessedAt, time.Second,
		"processed_at == now (единый now прохода, §4.5)")
	require.WithinDuration(t, now, out["s1"].ComputedAt, time.Second, "computed_at == now")
}

// T13: две ветки одной позиции → один активный эффект (одна строка UPSERT).
func TestSyncSettlementsTwoBranchesOneEffect(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-time.Minute) // путь «в памяти» — БД не пишется
	rows := sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}).
		AddRow("b1", "s1", int64(69), computedAt, int64(378), "Пища", int64(1), "продовольствие").
		AddRow("b2", "s1", int64(70), computedAt, int64(379), "Еда", int64(1), "продовольствие")
	components := sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"})
	buffers := sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
		AddRow("b1", "output", int64(378), "Пища", 0.0).
		AddRow("b1", "input", int64(359), "Мясо", 0.0).
		AddRow("b2", "output", int64(379), "Еда", 0.0).
		AddRow("b2", "input", int64(359), "Мясо", 0.0)
	expectOwnerPassWithBranches(mock, rows, components, buffers, nil)

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(computedAt)})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, out["s1"].Effects, 1, "две ветки одной позиции → один эффект (T13)")
}
