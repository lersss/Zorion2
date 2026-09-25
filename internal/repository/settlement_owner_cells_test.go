// internal/repository/settlement_owner_cells_test.go
//
// Тесты механики ячеек внутреннего хранилища сквозь runOwnerPass (спека
// 2026-09-25-внутреннее-хранилище-и-рождение-заказов §5.7/§7.1, ЧК2а):
// F2 — пропорциональное деление вход-ячейки между потребителями; F6 — базис
// позиции берётся один раз; F4 — свежий выход текущего прохода не виден
// потребителю того же прохода; T18 — комбинированный товар без двойного счёта.
package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

// f64ptr — указатель на float64 (число скорости пары).
func f64ptr(v float64) *float64 { return &v }

// cellsPassFixture — базовый вход runOwnerPass: поселение s1 (1e9, тип
// ownerTestTypeID) с привязкой позиции-ТОВАРА «пища» → «голод»; словарь товаров
// и карты пачки. Числа скорости/набор рецептов тесты задают сами.
func cellsPassFixture(now time.Time) (OwnerSettlement, goodsCatalog, ownerBatchData) {
	o := OwnerSettlement{
		ID:                "s1",
		PlanetID:          "p1",
		Population:        1_000_000_000,
		PopulationExact:   1_000_000_000,
		ComputedAt:        now.Add(-time.Hour),
		CreatedAt:         now.Add(-time.Hour),
		SettlementTypeID:  ownerTestTypeID,
		Planet:            settlement.PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5},
		EatByPosition:     map[string]float64{"пища": settlement.DefaultEatK},
		EffectsByPosition: map[string]string{"пища": "голод"},
	}
	goods := goodsCatalog{
		byPosition: map[string]int64{"пища": 378},
		nameByID:   map[int64]string{378: "Пища", 359: "Мясо", 379: "Еда"},
	}
	data := ownerBatchData{
		rates:       map[int64]map[int64]*float64{ownerTestTypeID: {}},
		recipes:     map[int64]map[int64]bool{ownerTestTypeID: {}},
		occurrences: map[int64]int{359: 1, 378: 1},
	}
	return o, goods, data
}

// cellsEffectCatalog — каталог типов эффектов с «голодом».
func cellsEffectCatalog() map[string]effectTypeMeta {
	return map[string]effectTypeMeta{
		"голод": {ID: 1, Name: "Голод", Impact: settlement.ImpactPopulationRate, Curve: "hunger"},
	}
}

// writesByBranch — данные записи по id ветки.
func writesByBranch(run ownerRun) map[string]ownerBranchWrite {
	out := make(map[string]ownerBranchWrite, len(run.writes))
	for _, w := range run.writes {
		out[w.rec.branch.ID] = w
	}
	return out
}

// T11/F2: две ветки одного входного товара делят базис ∝ потребности (не
// поровну, не по порядку); детерминизм при равных потребностях.
func TestRunOwnerPassF2TwoBranchesShareInput(t *testing.T) {
	now := time.Now()
	o, goods, data := cellsPassFixture(now)
	// r1 = 2·r2 → потребности 2:1.
	data.rates[ownerTestTypeID] = map[int64]*float64{69: f64ptr(660), 70: f64ptr(330)}
	data.recipes[ownerTestTypeID] = map[int64]bool{69: true, 70: true}
	branches := []*branchRecord{
		{branch: models.SettlementBranch{ID: "b1", RecipeID: 69, ProcessedAt: o.ComputedAt}, outputGoodID: 378, outputPosition: "пища",
			components: []settlement.BranchComponent{{GoodID: 359, Quantity: 1}}},
		{branch: models.SettlementBranch{ID: "b2", RecipeID: 70, ProcessedAt: o.ComputedAt}, outputGoodID: 379, outputPosition: "еда",
			components: []settlement.BranchComponent{{GoodID: 359, Quantity: 1}}},
	}
	cells := []models.StorageCell{{GoodID: 359, Amount: 1000}, {GoodID: 378, Amount: 0}, {GoodID: 379, Amount: 0}}
	_, size := storagePlan(o, branches, goods, data)

	run, err := runOwnerPass(o, branches, nil, cellsEffectCatalog(), goods, data, nil, cells, size, now, false)
	require.NoError(t, err)

	w := writesByBranch(run)
	require.InDelta(t, 1000*2.0/3.0, w["b1"].input[359], 1e-6, "доля b1 ∝ потребности (2/3)")
	require.InDelta(t, 1000*1.0/3.0, w["b2"].input[359], 1e-6, "доля b2 ∝ потребности (1/3)")
	require.NotEqual(t, w["b1"].input[359], w["b2"].input[359], "деление ∝ потребности, не поровну")

	// Детерминизм: повторный проход с тем же входом — те же доли.
	run2, err := runOwnerPass(o, branches, nil, cellsEffectCatalog(), goods, data, nil, cells, size, now, false)
	require.NoError(t, err)
	w2 := writesByBranch(run2)
	require.Equal(t, w["b1"].input[359], w2["b1"].input[359])
	require.Equal(t, w["b2"].input[359], w2["b2"].input[359])
}

// T11/F2: ветка и население делят базис вход-ячейки ∝ потребности; ветка берёт
// свою долю, не весь базис.
func TestRunOwnerPassF2BranchAndPopulationShareInput(t *testing.T) {
	now := time.Now()
	o, goods, data := cellsPassFixture(now)
	data.rates[ownerTestTypeID] = map[int64]*float64{70: f64ptr(660)}
	data.recipes[ownerTestTypeID] = map[int64]bool{70: true}
	// b2 потребляет 378 — тот же товар, что и позиция населения «пища».
	branches := []*branchRecord{
		{branch: models.SettlementBranch{ID: "b2", RecipeID: 70, ProcessedAt: o.ComputedAt}, outputGoodID: 379, outputPosition: "еда",
			components: []settlement.BranchComponent{{GoodID: 378, Quantity: 1}}},
	}
	cells := []models.StorageCell{{GoodID: 378, Amount: 1000}, {GoodID: 379, Amount: 0}}
	_, size := storagePlan(o, branches, goods, data)

	run, err := runOwnerPass(o, branches, nil, cellsEffectCatalog(), goods, data, nil, cells, size, now, false)
	require.NoError(t, err)

	nb := settlement.PerSecond(660, 1e9)
	np := settlement.PerSecond(settlement.DefaultEatK, 1e9)
	wantB2 := 1000 * nb / (nb + np)
	w := writesByBranch(run)
	require.InDelta(t, wantB2, w["b2"].input[378], 1e-6, "ветка берёт долю ∝ потребности, не весь базис")
	require.Less(t, w["b2"].input[378], 1000.0, "население тоже претендует на ячейку")
	require.GreaterOrEqual(t, run.cellFinals[378], 0.0, "ячейка не уходит в минус")
}

// T16/F6: базис позиции берётся ОДИН раз, а не суммой по веткам (несколько
// веток с общим выходом).
func TestRunOwnerPassF6BasisOnce(t *testing.T) {
	now := time.Now()
	o, goods, data := cellsPassFixture(now)
	o.EffectsByPosition = nil // население не потребляет — изолируем производство
	o.EatByPosition = nil
	data.rates[ownerTestTypeID] = map[int64]*float64{69: f64ptr(660), 70: f64ptr(660)}
	data.recipes[ownerTestTypeID] = map[int64]bool{69: true, 70: true}
	branches := []*branchRecord{
		{branch: models.SettlementBranch{ID: "b1", RecipeID: 69, ProcessedAt: o.ComputedAt}, outputGoodID: 378, outputPosition: "пища",
			components: []settlement.BranchComponent{{GoodID: 359, Quantity: 1}}},
		{branch: models.SettlementBranch{ID: "b2", RecipeID: 70, ProcessedAt: o.ComputedAt}, outputGoodID: 378, outputPosition: "пища",
			components: []settlement.BranchComponent{{GoodID: 359, Quantity: 1}}},
	}
	cells := []models.StorageCell{{GoodID: 359, Amount: 1e9}, {GoodID: 378, Amount: 100}}
	_, size := storagePlan(o, branches, goods, data)

	run, err := runOwnerPass(o, branches, nil, cellsEffectCatalog(), goods, data, nil, cells, size, now, false)
	require.NoError(t, err)

	produced := settlement.PerSecond(660, 1e9) * 3600 // 27.5 на ветку
	require.InDelta(t, 100+2*produced, run.cellFinals[378], 1e-6,
		"базис 100 учтён один раз (не 200): 100 + 2·produced")
}

// T16/F4: свежий выход текущего прохода НЕ виден потребителю того же прохода;
// становится входом при следующем пересчёте.
func TestRunOwnerPassF4FreshOutputNotVisible(t *testing.T) {
	now := time.Now()
	o, goods, data := cellsPassFixture(now)
	o.EffectsByPosition = nil
	o.EatByPosition = nil
	data.rates[ownerTestTypeID] = map[int64]*float64{69: f64ptr(660), 70: f64ptr(660)}
	data.recipes[ownerTestTypeID] = map[int64]bool{69: true, 70: true}
	// b1 производит 378; b2 потребляет 378.
	branches := []*branchRecord{
		{branch: models.SettlementBranch{ID: "b1", RecipeID: 69, ProcessedAt: o.ComputedAt}, outputGoodID: 378, outputPosition: "пища",
			components: []settlement.BranchComponent{{GoodID: 359, Quantity: 1}}},
		{branch: models.SettlementBranch{ID: "b2", RecipeID: 70, ProcessedAt: o.ComputedAt}, outputGoodID: 379, outputPosition: "еда",
			components: []settlement.BranchComponent{{GoodID: 378, Quantity: 1}}},
	}
	cells := []models.StorageCell{{GoodID: 359, Amount: 1e9}, {GoodID: 378, Amount: 0}, {GoodID: 379, Amount: 0}}
	_, size := storagePlan(o, branches, goods, data)

	run, err := runOwnerPass(o, branches, nil, cellsEffectCatalog(), goods, data, nil, cells, size, now, false)
	require.NoError(t, err)

	produced := settlement.PerSecond(660, 1e9) * 3600
	w := writesByBranch(run)
	require.InDelta(t, 0.0, w["b2"].input[378], 1e-9, "свежий выход b1 не виден b2 в том же проходе (F4)")
	require.InDelta(t, 0.0, w["b2"].produced, 1e-9, "b2 инертна — вход пуст")
	require.InDelta(t, produced, run.cellFinals[378], 1e-6, "выход b1 лёг в ячейку")

	// Следующий пересчёт: ячейка 378 = produced → b2 видит вход.
	cells2 := []models.StorageCell{{GoodID: 359, Amount: 1e9}, {GoodID: 378, Amount: produced}, {GoodID: 379, Amount: 0}}
	run2, err := runOwnerPass(o, branches, nil, cellsEffectCatalog(), goods, data, nil, cells2, size, now.Add(time.Hour), false)
	require.NoError(t, err)
	w2 := writesByBranch(run2)
	require.Greater(t, w2["b2"].input[378], 0.0, "на следующем пересчёте выход b1 виден b2")
}

// T18: комбинированный товар (выход/потребность населения И вход рецепта) —
// двойного счёта нет, ячейка не уходит в минус.
func TestRunOwnerPassT18CombinedGoodNoDoubleCount(t *testing.T) {
	now := time.Now()
	o, goods, data := cellsPassFixture(now) // «пища» (378) — позиция эффекта
	data.rates[ownerTestTypeID] = map[int64]*float64{69: f64ptr(6.6), 70: f64ptr(6600)}
	data.recipes[ownerTestTypeID] = map[int64]bool{69: true, 70: true}
	// 378 — выход b1, компонент b2 и позиция населения.
	branches := []*branchRecord{
		{branch: models.SettlementBranch{ID: "b1", RecipeID: 69, ProcessedAt: o.ComputedAt}, outputGoodID: 378, outputPosition: "пища",
			components: []settlement.BranchComponent{{GoodID: 359, Quantity: 1}}},
		{branch: models.SettlementBranch{ID: "b2", RecipeID: 70, ProcessedAt: o.ComputedAt}, outputGoodID: 379, outputPosition: "еда",
			components: []settlement.BranchComponent{{GoodID: 378, Quantity: 1}}},
	}
	cells := []models.StorageCell{{GoodID: 359, Amount: 1e9}, {GoodID: 378, Amount: 100}, {GoodID: 379, Amount: 0}}
	_, size := storagePlan(o, branches, goods, data)

	run, err := runOwnerPass(o, branches, nil, cellsEffectCatalog(), goods, data, nil, cells, size, now, false)
	require.NoError(t, err)

	producedB1 := settlement.PerSecond(6.6, 1e9) * 3600
	require.GreaterOrEqual(t, run.cellFinals[378], 0.0, "ячейка не уходит в минус (кламп)")
	require.LessOrEqual(t, run.cellFinals[378], 100+producedB1,
		"базис 100 учтён один раз (нет двойного счёта)")
	require.Less(t, run.cellFinals[378], 100+producedB1,
		"потребители (b2 и население) списали часть ячейки")
}
