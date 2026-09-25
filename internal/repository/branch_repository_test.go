// internal/repository/branch_repository_test.go
//
// Тесты owner-прохода поселения (спека 2026-09-25-внутреннее-хранилище-и-
// рождение-заказов §4.3/§5.7, ЧК2а): персистентный путь — одна транзакция на
// поселение (`pg_advisory_xact_lock(hashtext(settlement_id))` → settlements
// FOR UPDATE → ветки FOR UPDATE (порядок id) → залежи FOR UPDATE → сверка
// набора ячеек (реестр нужд) → запись ячеек/размера/залежей/processed_at +
// UPSERT active_effects + население), путь «в памяти» (Δt < MinPersistInterval)
// — без локов и записи. Запросы утверждаются дословно.
package repository

import (
	"database/sql/driver"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

// T5/F3: реестр нужд = эффекты ∪ компоненты ∪ выходы веток; веса — GoodWeights
// (вхождения во входы рецептов, для товара с эффектом — max(occ,1), выход без
// вхождений/эффекта — 0); размер — StorageSizeDefault без ручки типа.
func TestStoragePlanNeedsAndWeights(t *testing.T) {
	o := OwnerSettlement{
		SettlementTypeID:  ownerTestTypeID,
		EffectsByPosition: map[string]string{"пища": "голод"},
	}
	goods := goodsCatalog{
		byPosition: map[string]int64{"пища": 378},
		nameByID:   map[int64]string{378: "Пища", 359: "Мясо", 360: "Лёд", 380: "Вода"},
	}
	branches := []*branchRecord{
		{branch: models.SettlementBranch{ID: "b1", RecipeID: 69}, outputGoodID: 378,
			components: []settlement.BranchComponent{{GoodID: 359, Quantity: 1}}},
		{branch: models.SettlementBranch{ID: "b2", RecipeID: 71}, outputGoodID: 380,
			components: []settlement.BranchComponent{{GoodID: 360, Quantity: 1}}},
	}
	data := ownerBatchData{occurrences: map[int64]int{359: 2, 360: 1}}

	weights, size := storagePlan(o, branches, goods, data)

	require.Len(t, weights, 4, "нужды: 378 (эффект+выход), 359, 360, 380 (выход)")
	require.InDelta(t, 2, weights[359], 1e-9, "вес = вхождения во входы рецептов")
	require.InDelta(t, 1, weights[360], 1e-9)
	require.InDelta(t, 1, weights[378], 1e-9, "товар с эффектом — max(occ,1)")
	require.InDelta(t, 0, weights[380], 1e-9, "выход без вхождений и эффекта — вес 0")
	require.InDelta(t, settlement.StorageSizeDefault, size, 1e-9, "нет ручки params.storage.size → дефолт")
}

// amountNear — argument-matcher для абсолютной записи amount (float).
type amountNear struct{ want float64 }

func (m amountNear) Match(v driver.Value) bool {
	f, ok := v.(float64)
	return ok && math.Abs(f-m.want) < 1e-6
}

// ownerBranchRows — ветка b1 (рецепт 69, выход 378 «Пища», сложность 1) с
// позицией выхода `position` (name_norm ТОВАРА-выхода, §7.1).
func ownerBranchRows(processedAt time.Time, position string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}).
		AddRow("b1", "s1", int64(69), processedAt, int64(378), "Пища", int64(1), position)
}

func ownerComponentRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).
		AddRow(int64(69), int64(359), 1)
}

// ownerInput — вход owner-прохода: поселение s1 (1e9, тип ownerTestTypeID) с
// привязкой позиции-ТОВАРА «пища» → тип «голод», норма 600 ед/сутки/млрд.
func ownerInput(computedAt time.Time) OwnerSettlement {
	return OwnerSettlement{
		ID:                "s1",
		PlanetID:          "p1",
		Population:        1_000_000_000,
		PopulationExact:   1_000_000_000,
		ComputedAt:        computedAt,
		CreatedAt:         computedAt,
		SettlementTypeID:  ownerTestTypeID,
		Planet:            settlement.PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5},
		EatByPosition:     map[string]float64{"пища": settlement.DefaultEatK},
		EffectsByPosition: map[string]string{"пища": "голод"},
	}
}

// expectPersistentOwnerTx — общие ожидания персистентного пути после выборки
// веток/состава/залежей: сверка набора ячеек (компонент 359 + выход 378),
// чтение ячеек, запись размера, чек-поинт, эффект, население, очистка сирот.
func expectPersistentOwnerTx(mock sqlmock.Sqlmock, computedAt time.Time, branchRows *sqlmock.Rows, componentRows *sqlmock.Rows, cellRows *sqlmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, "", int64(ownerTestTypeID)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").WillReturnRows(branchRows)
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(componentRows)
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}))
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(359), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellsDeleteEmptyOutsideSQL).WithArgs("settlement", "s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(cellRows)
}

// TestSyncSettlementsPersistentPath — Δt ≥ MinPersistInterval → tx-путь:
// advisory-лок, settlements FOR UPDATE, ветки FOR UPDATE, залежи FOR UPDATE,
// сверка ячеек, запись ячеек (компонент −27.8, выход +27.8), UPSERT активного
// эффекта (load = 1 сило-час за час без источника покрытия) и население.
func TestSyncSettlementsPersistentPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-time.Hour)
	// Позиция ветки «металлы» — источника позиции-товара «пища» нет →
	// coverage = 0, w = 1 весь час (лог position_no_source) → load = 1; строка
	// нагрузки уже существует (базис load_at = computedAt).
	expectOwnerPassWithBranches(mock, ownerBranchRows(computedAt, "металлы"), ownerComponentRows(),
		activeEffectRows(int64(1), "пища", 0.0, computedAt, "population_rate", "hunger", "s1"))

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, "", int64(ownerTestTypeID)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "металлы"))
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}))
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(359), amountNear{1}).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(378), amountNear{1}).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellsDeleteEmptyOutsideSQL).WithArgs("settlement", "s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").
		WillReturnRows(storageCellRows(
			int64(1), "settlement", "s1", int64(359), 1e15, 1.0,
			int64(2), "settlement", "s1", int64(378), 0.0, 1.0))
	mock.ExpectExec(storageCellIncrementSQL).WithArgs("settlement", "s1", sqlmock.AnyArg(), int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellIncrementSQL).WithArgs("settlement", "s1", sqlmock.AnyArg(), int64(378)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementStorageSizeUpdateSQL).WithArgs("s1", amountNear{settlement.StorageSizeDefault}).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "пища", sqlmock.AnyArg(), amountNear{1.0}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(computedAt)})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Branches, 1)
	require.InDelta(t, 27.8, res.Branches[0].Produced, 1e-6, "произведено за час (rate × население)")
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
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(storageCellRows())
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
	expectOwnerPassWithBranches(mock, ownerBranchRows(processedAt, "пища"), ownerComponentRows(), nil)
	// Залежи пути «в памяти» — только чтение.
	mock.ExpectQuery(depositMemorySelectSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"planet_id", "id", "good_id", "amount"}).AddRow("p1", "dep1", int64(359), 5000.0))
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(storageCellRows())

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
	expectOwnerPassWithBranches(mock, ownerBranchRows(computedAt, "пища"), ownerComponentRows(), nil)

	expectPersistentOwnerTx(mock, computedAt, ownerBranchRows(computedAt, "пища"), ownerComponentRows(),
		storageCellRows(
			int64(1), "settlement", "s1", int64(359), 1e15, 1.0,
			int64(2), "settlement", "s1", int64(378), 0.0, 1.0))
	mock.ExpectExec(storageCellIncrementSQL).WithArgs("settlement", "s1", sqlmock.AnyArg(), int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellIncrementSQL).WithArgs("settlement", "s1", sqlmock.AnyArg(), int64(378)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementStorageSizeUpdateSQL).WithArgs("s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(sqlmock.AnyArg(), "s1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
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
		AddRow("b1", "s1", int64(69), computedAt, int64(378), "Пища", int64(1), "пища").
		AddRow("b2", "s1", int64(70), computedAt, int64(379), "Еда", int64(1), "пища")
	components := sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"})
	expectOwnerPassWithBranches(mock, rows, components, nil)
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(storageCellRows())

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(computedAt)})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, out["s1"].Effects, 1, "две ветки одной позиции → один эффект (T13)")
}

// T-Р3 (число по паре): OwnerSettlement несёт тип, тип пути «событие» читается
// из settlementLockSQL; промах пары (тип, рецепт) → числа нет → ветка инертна,
// но жива: произведено 0, ячейка не тронута, processed_at продвигается
// (спека 2026-09-23 §3.2/§3.5).
func TestSyncSettlementsRateMissInert(t *testing.T) {
	require.Contains(t, settlementLockSQL, "settlement_type_id",
		"путь «событие» читает тип поселения из settlementLockSQL (§3.3 п.2)")

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-2 * time.Hour)
	expectOwnerPassWithBranches(mock, ownerBranchRows(computedAt, "пища"), ownerComponentRows(), nil)

	o := ownerInput(computedAt)
	o.SettlementTypeID = 999 // пары (999, 69) нет → числа нет → инертна

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, "", int64(999)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "пища"))
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}))
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(359), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellUpsertSQL).WithArgs("settlement", "s1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(storageCellsDeleteEmptyOutsideSQL).WithArgs("settlement", "s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").
		WillReturnRows(storageCellRows(
			int64(1), "settlement", "s1", int64(359), 1e15, 1.0,
			int64(2), "settlement", "s1", int64(378), 0.0, 1.0))
	// Дельты нулевые — инкрементов ячеек нет.
	mock.ExpectExec(settlementStorageSizeUpdateSQL).WithArgs("s1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "пища", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
	mock.ExpectCommit()

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Branches, 1, "ветка жива — не удаляется молча (§3.5)")
	require.Zero(t, res.Branches[0].Produced, "числа нет → ветка инертна")
	require.WithinDuration(t, now, res.Branches[0].ProcessedAt, time.Second, "processed_at продвигается — не застой")
}

// T20 (спека 2026-09-24-потребление-по-товарам §9.4): позиция-ТОВАР резолвится
// (не отсекается position_unknown), словарь товаров читается ОДИН раз на пачку
// (несколько владельцев — один запрос goodsNamesSQL), source_position = ключ товара.
func TestSyncSettlementsGoodsPositionResolvesOncePerBatch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	// goodsNamesSQL ожидается ровно один раз на пачку из двух владельцев.
	expectOwnerPassNoBranches(mock, nil)
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(storageCellRows())
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s2").WillReturnRows(storageCellRows())

	o1 := ownerInput(now.Add(-time.Minute))
	o2 := ownerInput(now.Add(-time.Minute))
	o2.ID = "s2"

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o1, o2})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, out["s1"].Effects, 1, "товарный ключ «пища» резолвится (не position_unknown)")
	require.Len(t, out["s2"].Effects, 1, "второй владелец пачки тоже резолвится")
	require.Equal(t, "пища", *out["s1"].Effects[0].SourcePosition, "source_position = ключ товара")
}

// T20/§6.1 (негатив): ключ, совпавший с именем КАТЕГОРИИ, но не товара, —
// позиция-неизвестна: no-op (привязки нет, категория не подставляется).
func TestSyncSettlementsCategoryKeyIsNotAPosition(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	expectOwnerPassNoBranches(mock, nil)
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(storageCellRows())

	o := ownerInput(now.Add(-time.Minute))
	o.EffectsByPosition = map[string]string{"продовольствие": "голод"}
	o.EatByPosition = map[string]float64{"продовольствие": settlement.DefaultEatK}

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, out["s1"].Effects, "категория — не позиция (position_unknown, no-op)")
}
