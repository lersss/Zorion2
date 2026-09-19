package validate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/cmd/goods-studio/model"
)

// mkState — состояние с категорией c1 и товарами.
func mkState(goods ...model.Good) *model.State {
	return &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "c1", Name: "Корабли"}},
		Goods:         goods,
	}
}

func good(id, name string, status model.Status, recipe ...model.Slot) model.Good {
	return model.Good{ID: id, Name: name, Category: "c1", Status: status, Kind: model.KindGood, Recipe: recipe}
}

func res(id, name string) model.Good {
	return model.Good{ID: id, Name: name, Category: "mineral", Status: model.StatusResource, Kind: model.KindResource}
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
		good("g1", "Корабль", model.StatusApproved, model.Slot{GoodID: "g2"}),
		good("g2", "Недостающий ресурс", model.StatusDraft), // лист, входящая степень 1
	)
	w := Validate(st)
	require.Contains(t, codes(w), "missing_resource")
}

// TestMissingResourceTop — товар-верхушка с пустым рецептом (входящая
// степень 0) — лист-намерение, в кандидаты не входит.
func TestMissingResourceTop(t *testing.T) {
	st := mkState(
		good("g1", "Корабль", model.StatusDraft), // верхушка, никто не ссылается
	)
	w := Validate(st)
	require.NotContains(t, codes(w), "missing_resource")
}

// TestMissingResourceCovered — лист, покрытый каталогом (ресурс), —
// не кандидат.
func TestMissingResourceCovered(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	w := Validate(st)
	require.NotContains(t, codes(w), "missing_resource")
}

// TestIncompleteChain — согласованный товар с пустым рецептом (тир 0).
func TestIncompleteChain(t *testing.T) {
	st := mkState(
		good("g1", "Согласован без рецепта", model.StatusApproved),
	)
	w := Validate(st)
	require.Contains(t, codes(w), "incomplete_chain")
}

// TestNonApprovedRef — рецепт согласованного ссылается на draft/excluded/
// banned составляющего.
func TestNonApprovedRef(t *testing.T) {
	st := mkState(
		good("g1", "Согласованный", model.StatusApproved, model.Slot{GoodID: "g2"}),
		good("g2", "Черновик", model.StatusDraft),
		good("g3", "Исключённый", model.StatusExcluded),
		good("g4", "Забаненный", model.StatusBanned),
		good("g5", "Согласованный2", model.StatusApproved, model.Slot{GoodID: "g3"}, model.Slot{GoodID: "g4"}),
	)
	w := Validate(st)
	require.Contains(t, codes(w), "non_approved_ref")
}

// TestNonApprovedRefResource — ссылка на ресурс (status=resource) —
// не предупреждение (ресурс — валидная составляющая).
func TestNonApprovedRefResource(t *testing.T) {
	st := mkState(
		good("g1", "Согласованный", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	w := Validate(st)
	require.NotContains(t, codes(w), "non_approved_ref")
}

// TestCleanState — валидное состояние без предупреждений.
func TestCleanState(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	w := Validate(st)
	require.Empty(t, w)
}

// TestSafetyCycle — страховка: цикл в состоянии ловится.
func TestSafetyCycle(t *testing.T) {
	st := mkState(
		good("g1", "A", model.StatusDraft, model.Slot{GoodID: "g2"}),
		good("g2", "B", model.StatusDraft, model.Slot{GoodID: "g1"}),
	)
	w := Validate(st)
	require.Contains(t, codes(w), "cycle")
}

// TestSafetyDuplicateName — страховка: дубликат имени ловится.
func TestSafetyDuplicateName(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", model.StatusDraft),
		good("g2", "  сталь ", model.StatusDraft),
	)
	w := Validate(st)
	require.Contains(t, codes(w), "duplicate_name")
}