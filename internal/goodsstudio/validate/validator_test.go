package validate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
)

// mkState — состояние с категорией c1 и товарами.
func mkState(goods ...model.Good) *model.State {
	return &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "c1", Name: "Корабли"}},
		Goods:         goods,
	}
}

func good(id, name string, recipe ...model.Slot) model.Good {
	return model.Good{ID: id, Name: name, Category: "c1", Kind: model.KindGood, Recipe: recipe}
}

func res(id, name string) model.Good {
	return model.Good{ID: id, Name: name, Category: "mineral", Kind: model.KindResource}
}

func codes(w []Warning) []string {
	out := make([]string, len(w))
	for i := range w {
		out[i] = w[i].Code
	}
	return out
}

// TestMissingResource — лист-составляющая, не покрытая каталогами, —
// кандидат в недостающие ресурсы.
func TestMissingResource(t *testing.T) {
	st := mkState(
		good("g1", "Корабль", model.Slot{GoodID: "g2"}),
		good("g2", "Недостающий ресурс"), // лист, входящая степень 1
	)
	w := Validate(st)
	require.Contains(t, codes(w), "missing_resource")
}

// TestMissingResourceTop — товар-верхушка с пустым рецептом (входящая
// степень 0) — лист-намерение, в кандидаты не входит.
func TestMissingResourceTop(t *testing.T) {
	st := mkState(
		good("g1", "Корабль"), // верхушка, никто не ссылается
	)
	w := Validate(st)
	require.NotContains(t, codes(w), "missing_resource")
}

// TestMissingResourceCovered — лист, покрытый каталогом (ресурс), —
// не кандидат.
func TestMissingResourceCovered(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	w := Validate(st)
	require.NotContains(t, codes(w), "missing_resource")
}

// TestCleanState — валидное состояние без предупреждений (товар привязан
// к фабрике — unbound_recipe не срабатывает).
func TestCleanState(t *testing.T) {
	v, w := 1.0, 1.0
	st := mkState(
		good("1", "Сталь", model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	st.Goods[0].Volume = &v
	st.Goods[0].Weight = &w
	st.Bindings = []model.RecipeBinding{{RecipeID: 10, ProducerTypeID: 2, GoodID: 1}}
	require.Empty(t, Validate(st))
}

// TestUnboundRecipe — товар (kind=good), чей рецепт не привязан ни к одной
// фабрике, — warning unbound_recipe (спека 2026-09-21-рецепт-сущность §5).
func TestUnboundRecipe(t *testing.T) {
	v, w := 1.0, 1.0
	st := mkState(
		good("1", "Сталь", model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	st.Goods[0].Volume = &v
	st.Goods[0].Weight = &w
	require.Contains(t, codes(Validate(st)), "unbound_recipe")
	// привязан — warning уходит
	st.Bindings = []model.RecipeBinding{{RecipeID: 10, ProducerTypeID: 2, GoodID: 1}}
	require.NotContains(t, codes(Validate(st)), "unbound_recipe")
	// ресурс без привязки не флагается (рецепта у ресурса нет)
	require.NotContains(t, codes(Validate(mkState(res("r1", "Железо Fe")))), "unbound_recipe")
}

// TestMissingVolumeWeight — Р2 (2026-09-21): значение веса/объёма есть всегда
// (NOT NULL DEFAULT 1) — warning инертен. Товар с 1/1 не флагается; ресурс —
// не флагается (сырьё не имеет объёма/веса как товар).
func TestMissingVolumeWeight(t *testing.T) {
	v, w := 1.0, 1.0
	st := mkState(
		good("g1", "Сталь", model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	st.Goods[0].Volume = &v
	st.Goods[0].Weight = &w
	require.NotContains(t, codes(Validate(st)), "missing_volume_weight")

	// ресурс без веса/объёма — тоже не флагается (kind=resource пропускается).
	require.NotContains(t, codes(Validate(mkState(res("r1", "Железо Fe")))), "missing_volume_weight")
}

// TestSafetyCycle — страховка: цикл в состоянии ловится.
func TestSafetyCycle(t *testing.T) {
	st := mkState(
		good("g1", "A", model.Slot{GoodID: "g2"}),
		good("g2", "B", model.Slot{GoodID: "g1"}),
	)
	w := Validate(st)
	require.Contains(t, codes(w), "cycle")
}

// TestSafetyDuplicateName — страховка: дубликат имени ловится.
func TestSafetyDuplicateName(t *testing.T) {
	st := mkState(
		good("g1", "Сталь"),
		good("g2", "  сталь "),
	)
	w := Validate(st)
	require.Contains(t, codes(w), "duplicate_name")
}
