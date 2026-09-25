// internal/repository/settlement_owner_basis_test.go
//
// Тесты стартового базиса нагрузки owner-прохода (идея 2026-09-25 «счётчик
// нехватки с рождения поселения», решение создателя 2026-09-25):
// новорождённое поселение (чек-точка населения ещё не двигалась —
// computed_at == created_at) при привязке без сохранённого базиса считает
// нагрузку с created_at; уже пересчитывавшееся поселение (эффект появился
// позже) — с now (без бэкдейта); сброс по смене стадии — всегда с now, даже
// если поселение впервые проходит проход. Путь «в памяти» здесь не строится:
// под 2 ч/30 ч интервал всегда персистентный.
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// expectOwnerNoBranchPersistentPass — персистентный owner-проход по поселению
// s1 без веток и без сохранённого базиса нагрузки (новая строка active_effects):
// head пачки + транзакция (advisory-лок, settlements FOR UPDATE, ветки FOR
// UPDATE) + запись нагрузки (wantLoad) и населения + очистка сирот + commit.
// computedAt — чек-точка населения из БД; createdAt — рождение поселения.
func expectOwnerNoBranchPersistentPass(mock sqlmock.Sqlmock, computedAt, createdAt time.Time, wantLoad float64) {
	expectOwnerPassNoBranches(mock, nil)

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, createdAt, "", int64(ownerTestTypeID)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}))
	// Реестр нужд: ячейка позиции-эффекта «пища» (good 378).
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellsDeleteEmptyOutsideSQL).WithArgs("settlement", "s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").
		WillReturnRows(storageCellRows(int64(1), "settlement", "s1", int64(378), 0.0, 1.0))
	mock.ExpectExec(settlementStorageSizeUpdateSQL).WithArgs("s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "пища", sqlmock.AnyArg(), amountNear{wantLoad}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
	mock.ExpectCommit()
}

// TestOwnerPassNewbornLoadFromBirth — новорождённое поселение без базиса и без
// источников: при чтении через 2 ч нагрузка = 2 сило-часа (счёт с created_at),
// w = 1, порог 24 не достигнут. Красный до фикса: load_at = now → нагрузка 0.
func TestOwnerPassNewbornLoadFromBirth(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	createdAt := now.Add(-2 * time.Hour)
	o := ownerInput(createdAt) // ComputedAt == CreatedAt == createdAt (не двигалась)

	expectOwnerNoBranchPersistentPass(mock, createdAt, createdAt, 2.0)

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 2.0, res.Effects[0].Load, 1e-6, "2 ч с рождения при w=1 → load = 2 сило-часа")
	require.InDelta(t, 1.0, res.Effects[0].W, 1e-9, "позиция без источника → w=1")
	require.False(t, res.Effects[0].Enabled, "порог 24 не достигнут")
}

// TestOwnerPassNewbornEnabledAfterThreshold — то же новорождённое поселение, но
// через 30 ч: нагрузка 30 ≥ порога 24 → состояние «включён».
func TestOwnerPassNewbornEnabledAfterThreshold(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	createdAt := now.Add(-30 * time.Hour)
	o := ownerInput(createdAt)

	expectOwnerNoBranchPersistentPass(mock, createdAt, createdAt, 30.0)

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 30.0, res.Effects[0].Load, 1e-6)
	require.True(t, res.Effects[0].Enabled, "load 30 ≥ порог 24 → включён")
}

// TestOwnerPassNewbornMemoryPathAlive — тот же счёт с рождения на пути «в
// памяти» (Δt < MinPersistInterval): нагрузка оживает сразу в ответе, без
// первой записи в БД. Ни одной транзакции не выставляется — запись уронила бы
// тест (незарегистрированный запрос → ошибка драйверу).
func TestOwnerPassNewbornMemoryPathAlive(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	createdAt := now.Add(-20 * time.Minute)
	o := ownerInput(createdAt)

	expectOwnerPassNoBranches(mock, nil)
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(storageCellRows())

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 20.0/60.0, res.Effects[0].Load, 1e-6, "20 мин с рождения при w=1 → load = 1/3 сило-часа")
	require.InDelta(t, 1.0, res.Effects[0].W, 1e-9)
}

// TestOwnerPassLateBindingNoBackdate — поселение уже пересчитывалось
// (computed_at > created_at), привязка появилась позже без сохранённого
// базиса: счёт с now, назад к рождению НЕ отматывается. При бэкдейте к
// created_at нагрузка была бы ~100 (ровно возраст поселения).
func TestOwnerPassLateBindingNoBackdate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-2 * time.Hour)
	createdAt := now.Add(-100 * time.Hour)
	o := ownerInput(computedAt)
	o.CreatedAt = createdAt

	expectOwnerNoBranchPersistentPass(mock, computedAt, createdAt, 0.0)

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 0.0, res.Effects[0].Load, 1e-6, "эффект появился сейчас → счёт с now, без бэкдейта")
	require.Less(t, res.Effects[0].Load, 50.0, "нагрузка не отмотана к рождению (было бы ~100)")
}

// TestActiveEffectUpsertInsertCarriesComputedLoad — guard без живой БД: ветка
// INSERT `activeEffectUpsertSQL` обязана писать вычисленную нагрузку ($5), а не
// литерал 0 — иначе первая персистентная запись обнуляет нагрузку, накопленную
// с рождения (регресс @tester, идея 2026-09-25). sqlmock семантику INSERT не
// эмулирует, поэтому текст SQL проверяется явно; поведенческий кейс —
// интеграционный TestSupplyEffectsIntegrationFirstPersistKeepsLoad (на живой БД).
func TestActiveEffectUpsertInsertCarriesComputedLoad(t *testing.T) {
	require.Contains(t, activeEffectUpsertSQL, "$3, $5, $4)",
		"INSERT active_effects несёт вычисленную нагрузку $5 (не литерал 0)")
	require.NotContains(t, activeEffectUpsertSQL, "$3, 0, $4)",
		"литерал 0 в INSERT обнулял бы нагрузку первой персистентной записи")
}

// TestOwnerPassNewbornStageTransitionFromNow — крайний случай: смена стадии в
// САМОМ ПЕРВОМ проходе новорождённого поселения. Базис обнулён намеренно →
// счёт с now (нагрузка 0), а не с created_at (иначе было бы 2). Явный признак
// сброса-по-стадии отличает этот случай от «новорождённое без перехода».
func TestOwnerPassNewbornStageTransitionFromNow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	createdAt := now.Add(-2 * time.Hour)
	o := ownerInput(createdAt) // тип 148, население 1e9 > enter 151

	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(goodsNamesSQL).WillReturnRows(goodsNameRows())
	mock.ExpectQuery(recipeComponentOccurrencesSQL).WillReturnRows(recipeOccurrenceRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(emptyBranchRows())
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(activeEffectRows())
	expectStageBatch(mock)

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), createdAt, createdAt, "", int64(148)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(emptyBranchRows())

	// Переход 148 → 151: запись стадии, набор рецептов (пуст), обнуление ячеек.
	mock.ExpectExec(settlementTypeWriteSQL).WithArgs(int64(151), "s1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(settlementRecipesSelectSQL).WithArgs(int64(151)).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "count"}))
	mock.ExpectExec(storageCellsClearSQL).WithArgs("settlement", "s1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectsClearSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 1))

	// Перечитать ветки после перехода (пусто).
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(emptyBranchRows())

	// Реестр нужд: ячейка позиции-эффекта «пища» (good 378).
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellsDeleteEmptyOutsideSQL).WithArgs("settlement", "s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").
		WillReturnRows(storageCellRows(int64(1), "settlement", "s1", int64(378), 0.0, 1.0))
	mock.ExpectExec(settlementStorageSizeUpdateSQL).WithArgs("s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))

	// Сброс по стадии → load_at = now → нагрузка 0.
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "пища", sqlmock.AnyArg(), amountNear{0.0}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 0.0, res.Effects[0].Load, 1e-6,
		"сброс по стадии → счёт с текущего момента, не с рождения (иначе было бы 2)")
}
