// internal/repository/settlement_stage_test.go
//
// Тесты механики стадий поселения (спека 2026-09-23-стадии-поселения-и-
// скорость-производства §4–§5, §15.2 T-С4…С7): путь «в памяти» стадию не
// меняет; персистентный переход записывает стадию, добирает недостающие ветки
// (существующие сохраняются), очищает склады и сбрасывает нагрузку (БД и
// память); очистка сирот — по типу эффекта; гвард галактической сводки.
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// --- SQL CreateBranchTx (нормализованные литералы для QueryMatcherEqual) ---
const (
	testCreateBranchSQL       = `INSERT INTO settlement_branches (id, settlement_id, recipe_id) VALUES ($1, $2, $3)`
	testBranchComponentSQL    = `SELECT component_id FROM recipe_components WHERE recipe_id = $1 AND component_id IS NOT NULL ORDER BY pos`
	testBranchSeedInputSQL    = `INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount) VALUES ($1, 'input', $2, 0) ON CONFLICT DO NOTHING`
	testBranchOutputGoodSQL   = `SELECT good_id FROM recipes WHERE id = $1`
	testBranchSeedOutputSQL   = `INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount) VALUES ($1, 'output', $2, 0) ON CONFLICT DO NOTHING`
	testStageLadderRootTypeID = int64(100)
)

// stageLadderRows — ладдера: 148 (пол, enter 0) + 151 (enter 100, exit 50).
func stageLadderRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "stage"}).
		AddRow(int64(148), []byte(`{"enter":0,"exit":0}`)).
		AddRow(int64(151), []byte(`{"enter":100,"exit":50}`))
}

// producerTypeStageRows — настройки типов 148/151 (eat/effects).
func producerTypeStageRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "eat", "effects"}).
		AddRow(int64(148), []byte(`{"продовольствие":600}`), []byte(`{"продовольствие":"голод"}`)).
		AddRow(int64(151), []byte(`{"продовольствие":600}`), []byte(`{"продовольствие":"голод"}`))
}

// expectStageBatch — запросы пачки с настроенной ладдерой (148 → 151) и числами
// скорости нового типа.
func expectStageBatch(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(defaultSettlementTypeSelectSQL).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte("148")))
	mock.ExpectQuery(settlementRootTypeIDSQL).WithArgs(int64(148)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(testStageLadderRootTypeID))
	mock.ExpectQuery(settlementStageLadderSQL).WithArgs(testStageLadderRootTypeID).WillReturnRows(stageLadderRows())
	mock.ExpectQuery(producerTypesSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(producerTypeStageRows())
	mock.ExpectQuery(producerRatesSelectSQL).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate"}).
			AddRow(int64(151), int64(69), ownerTestRate))
}

// T-С4: путь «в памяти» (Δt < MinPersistInterval) стадию НЕ оценивает — ветки,
// буферы и active_effects не трогаются; ни одной записи не выставляется.
func TestStageNotEvaluatedOnMemoryPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	o := ownerInput(now) // computedAt = now → «в памяти»; население 1e9 выше порога 151

	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(categoryNamesSQL).WillReturnRows(categoryNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(emptyBranchRows())
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(activeEffectRows())
	expectStageBatch(mock)
	// Ни одного ExpectBegin/ExpectExec: стадия на пути «в памяти» не двигается.

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, out["s1"].Branches)
}

// T-С5: переход стадии — settlement_type_id записан; недостающая ветка набора
// новой стадии создана (рецепт с компонентом); рецепт без компонентов
// пропущен; существующая ветка на месте; буферы очищены.
func TestStageTransitionCreatesMissingBranches(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-2 * time.Hour)
	o := ownerInput(computedAt) // тип 148, население 1e9

	// Пачка: одна ветка b1 (рецепт 69), хранимый базис нагрузки (сбросится).
	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(categoryNamesSQL).WillReturnRows(categoryNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBranchRows(computedAt, "продовольствие"))
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBufferRows())
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(activeEffectRows(int64(1), "продовольствие", 5.0, computedAt, "population_rate", "hunger", "s1"))
	expectStageBatch(mock)

	// Персистентный путь.
	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, "", int64(148)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "продовольствие"))
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBufferRows())
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectExec(branchTopUpInputSQL).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}))

	// Переход на 151: запись стадии, набор рецептов новой стадии.
	mock.ExpectExec(settlementTypeWriteSQL).WithArgs(int64(151), "s1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(settlementRecipesSelectSQL).WithArgs(int64(151)).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "count"}).
			AddRow(int64(69), int64(1)). // ветка уже есть
			AddRow(int64(71), int64(1)). // добор
			AddRow(int64(72), int64(0))) // без компонентов → пропуск
	mock.ExpectExec(testCreateBranchSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(testBranchComponentSQL).WithArgs(int64(71)).
		WillReturnRows(sqlmock.NewRows([]string{"component_id"}).AddRow(int64(360)))
	mock.ExpectExec(testBranchSeedInputSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(testBranchOutputGoodSQL).WithArgs(int64(71)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}).AddRow(int64(380)))
	mock.ExpectExec(testBranchSeedOutputSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchBuffersClearSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectsClearSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 1))

	// Перечитать ветки: существующая b1 (69) + доборная b2 (71, свежая).
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}).
			AddRow("b1", "s1", int64(69), computedAt, int64(378), "Пища", int64(1), "продовольствие").
			AddRow("b2", "s1", int64(71), now, int64(380), "Вода", int64(1), "продовольствие"))
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
			AddRow("b1", "output", int64(378), "Пища", 0.0).
			AddRow("b1", "input", int64(359), "Мясо", 0.0).
			AddRow("b2", "output", int64(380), "Вода", 0.0).
			AddRow("b2", "input", int64(360), "Лёд", 0.0))
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).
			AddRow(int64(69), int64(359), 1).
			AddRow(int64(71), int64(360), 1))

	// Запись прохода: обе ветки.
	mock.ExpectExec(branchWriteInputSQL).WithArgs(sqlmock.AnyArg(), "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteOutputSQL).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteInputSQL).WithArgs(sqlmock.AnyArg(), "b2", int64(360)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteOutputSQL).WithArgs("b2", int64(380), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "продовольствие", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Branches, 2, "существующая ветка на месте + доборная создана (ручные не удаляются)")
	ids := map[string]bool{}
	for _, b := range res.Branches {
		ids[b.ID] = true
	}
	require.True(t, ids["b1"], "существующая ветка сохранена")
	require.True(t, ids["b2"], "недостающая ветка набора новой стадии создана")
}

// T-С6: при переходе ComputeNeeds получает load = 0, load_at = now в этом же
// проходе (несмотря на уже загруженный storedLoad) — в БД строка пересоздаётся
// с load = 0. Без сброса тест красный (старый load = 5 протёк бы).
func TestStageTransitionResetsLoadInMemory(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-2 * time.Hour)
	o := ownerInput(computedAt)

	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(categoryNamesSQL).WillReturnRows(categoryNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBranchRows(computedAt, "продовольствие"))
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBufferRows())
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(activeEffectRows(int64(1), "продовольствие", 5.0, computedAt, "population_rate", "hunger", "s1"))
	expectStageBatch(mock)

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, "", int64(148)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "продовольствие"))
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBufferRows())
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectExec(branchTopUpInputSQL).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	// Залежь покрывает спрос → w = 0 → load не растёт (с базисом 5 было бы 4.5).
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}).AddRow("dep1", int64(359), 1_000_000.0))

	// Переход: у 151 нет своих рецептов — только запись стадии/очистки.
	mock.ExpectExec(settlementTypeWriteSQL).WithArgs(int64(151), "s1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(settlementRecipesSelectSQL).WithArgs(int64(151)).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "count"}))
	mock.ExpectExec(branchBuffersClearSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectsClearSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 1))

	// Перечитать ветки (буферы обнулены).
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "продовольствие"))
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
			AddRow("b1", "output", int64(378), "Пища", 0.0).
			AddRow("b1", "input", int64(359), "Мясо", 0.0))
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())

	mock.ExpectExec(branchWriteInputSQL).WithArgs(sqlmock.AnyArg(), "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteOutputSQL).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(depositWriteAmountSQL).WithArgs(sqlmock.AnyArg(), "dep1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	// Строка нагрузки пересоздаётся с load = 0 (сброс в памяти, §5.1 п.5).
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "продовольствие", sqlmock.AnyArg(), amountNear{0.0}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, out["s1"].Effects, 1)
	require.InDelta(t, 0.0, out["s1"].Effects[0].Load, 1e-9, "старый load не протёк — базис сброшен в памяти")
}

// T-С7: очистка сирот строится по ТИПУ ЭФФЕКТА; SQL защищён COALESCE и не
// использует source_position.
func TestOrphanCleanupByEffectTypeSQL(t *testing.T) {
	require.Contains(t, orphanEffectsDeleteSQL, "effect_type_id <> ALL")
	require.Contains(t, orphanEffectsDeleteSQL, "COALESCE($2::bigint[], '{}'::bigint[])")
	require.NotContains(t, orphanEffectsDeleteSQL, "source_position")
}

// T-С7: пустой набор привязок → в очистку уходит пустой массив (удаляются все
// строки владельца).
func TestOrphanCleanupEmptyBindings(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-time.Hour)
	o := ownerInput(computedAt)
	o.EffectsByPosition = nil // «не потребляет» → все строки владельца — сироты
	o.EatByPosition = nil

	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(categoryNamesSQL).WillReturnRows(categoryNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(emptyBranchRows())
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(activeEffectRows(int64(1), "продовольствие", 5.0, computedAt, "population_rate", "hunger", "s1"))
	expectBatchStageQueries(mock) // ладдера пуста — перехода нет

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, "", int64(148)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{})
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, out["s1"].Effects)
}

// T-С7 (S5): гвард галактической сводки отбрасывает хранимую строку по ТИПУ
// ЭФФЕКТА, а не по source_position: снятие одной позиции из группы из двух не
// теряет живую строку, пока тип привязан другой позицией.
func TestGalaxyEffectsGuardByEffectType(t *testing.T) {
	catalog := map[string]effectTypeMeta{
		"голод": {ID: 1, Name: "Голод", Impact: "population_rate", Curve: "hunger"},
		"жара":  {ID: 2, Name: "Жара", Impact: "population_rate", Curve: "heat"},
	}
	now := time.Now()
	stored := []storedEffect{{
		effectTypeID: 1, sourcePosition: "a", load: 3,
		loadAt: now.Add(-time.Hour), impact: "population_rate", curve: "hunger",
	}}

	// source_position "a" ушёл, но тип 1 привязан позицией "b" → строка жива.
	out, _ := galaxyEffects(galaxySettlementRow{id: "s1", effects: map[string]string{"b": "голод"}}, stored, catalog, now)
	require.Len(t, out, 1, "живая строка не отбрасывается по source_position (S5)")

	// Тип 1 больше не привязан → сирота отброшена.
	out, _ = galaxyEffects(galaxySettlementRow{id: "s1", effects: map[string]string{"b": "жара"}}, stored, catalog, now)
	require.Empty(t, out, "сирота по типу эффекта отброшена")

	// Привязок нет вовсе.
	out, _ = galaxyEffects(galaxySettlementRow{id: "s1"}, stored, catalog, now)
	require.Empty(t, out)
}
