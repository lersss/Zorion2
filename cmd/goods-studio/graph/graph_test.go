package graph

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/cmd/goods-studio/model"
)

// helper: строит состояние из товаров (id → Good).
func mkGoods(goods ...model.Good) []model.Good { return goods }

func good(id, name string, recipe ...model.Slot) model.Good {
	return model.Good{ID: id, Name: name, Kind: model.KindGood, Recipe: recipe}
}

func res(id, name string) model.Good {
	return model.Good{ID: id, Name: name, Kind: model.KindResource}
}

// --- Тиры (спека 99a.1 §6.1) ---

// TestTierChain — цепочка ресурс → товар1 → товар2: тиры 0, 1, 2.
func TestTierChain(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "Сталь", model.Slot{GoodID: "r1"}),
		good("g2", "Каркас", model.Slot{GoodID: "g1"}),
	)
	byID := ByID(goods)
	require.Equal(t, 0, Tier(&goods[0], byID))
	require.Equal(t, 1, Tier(&goods[1], byID))
	require.Equal(t, 2, Tier(&goods[2], byID))
}

// TestTierMixed — смешанные тиры: max(тиры слотов) + 1.
func TestTierMixed(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "Сталь", model.Slot{GoodID: "r1"}),
		good("g2", "Механизм", model.Slot{GoodID: "g1"}, model.Slot{GoodID: "r1"}),
	)
	byID := ByID(goods)
	require.Equal(t, 2, Tier(&goods[2], byID)) // 1 + max(1, 0)
}

// TestTierEmptyRecipe — пустой рецепт = лист = 0 (цепочка не достроена).
func TestTierEmptyRecipe(t *testing.T) {
	goods := mkGoods(
		good("g1", "Верхушка"),
		good("g2", "С пустыми слотами", model.Slot{}),
	)
	byID := ByID(goods)
	require.Equal(t, 0, Tier(&goods[0], byID))
	require.Equal(t, 0, Tier(&goods[1], byID))
}

// TestTierResourceZero — ресурс всегда тир 0 (база шкалы, решение создателя).
func TestTierResourceZero(t *testing.T) {
	goods := mkGoods(res("r1", "Железо"))
	require.Equal(t, 0, Tier(&goods[0], ByID(goods)))
}

// --- Циклы (спека 99a.1 §6.2) ---

// TestCycleDirect — прямой X→X отклоняется.
func TestCycleDirect(t *testing.T) {
	goods := mkGoods(good("g1", "X"))
	byID := ByID(goods)
	require.True(t, WouldCreateCycle("g1", "g1", byID))
}

// TestCycleTransitive — транзитивный X→Y→Z + попытка Z→X — отклонение.
func TestCycleTransitive(t *testing.T) {
	goods := mkGoods(
		good("g1", "X"),
		good("g2", "Y", model.Slot{GoodID: "g1"}),
		good("g3", "Z", model.Slot{GoodID: "g2"}),
	)
	byID := ByID(goods)
	require.False(t, WouldCreateCycle("g1", "g3", byID)) // X в слот Z — ок
	require.True(t, WouldCreateCycle("g3", "g1", byID))  // Z в слот X — цикл
	require.True(t, WouldCreateCycle("g2", "g1", byID))  // Y в слот X — цикл
}

// TestCycleNoFalsePositive — независимые ветки не дают ложного цикла.
func TestCycleNoFalsePositive(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "A", model.Slot{GoodID: "r1"}),
		good("g2", "B", model.Slot{GoodID: "r1"}),
	)
	byID := ByID(goods)
	require.False(t, WouldCreateCycle("g1", "g2", byID))
	require.False(t, WouldCreateCycle("g2", "g1", byID))
}

// TestHasCycle — страховка: транзитивный цикл в состоянии ловится.
func TestHasCycle(t *testing.T) {
	acyclic := mkGoods(
		res("r1", "Железо"),
		good("g1", "A", model.Slot{GoodID: "r1"}),
		good("g2", "B", model.Slot{GoodID: "g1"}),
	)
	require.False(t, HasCycle(acyclic))

	cyclic := mkGoods(
		good("g1", "A", model.Slot{GoodID: "g2"}),
		good("g2", "B", model.Slot{GoodID: "g1"}),
	)
	require.True(t, HasCycle(cyclic))
}

// --- Дубликаты имён (спека 99a.1 §6.3) ---

// TestNormalizeName — lowercase, trim, схлопывание пробелов.
func TestNormalizeName(t *testing.T) {
	require.Equal(t, "штурмовой корабль", NormalizeName("  Штурмовой   Корабль "))
	require.Equal(t, "железо fe", NormalizeName("Железо Fe"))
}

// TestFindByName — совпадение после нормализации.
func TestFindByName(t *testing.T) {
	goods := mkGoods(good("g1", "Штурмовой корабль"))
	require.NotNil(t, FindByName("  штурмовой   корабль ", goods))
	require.Nil(t, FindByName("Другой", goods))
}

// TestNameIndex — карта нормализованных имён.
func TestNameIndex(t *testing.T) {
	goods := mkGoods(good("g1", "Сталь"), res("r1", "Железо"))
	idx := NameIndex(goods)
	require.Equal(t, 0, idx["сталь"])
	require.Equal(t, 1, idx["железо"])
	_, ok := idx["нет"]
	require.False(t, ok)
}