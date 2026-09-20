package validate

import (
	"fmt"
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
	v, w := 1.0, 2.0
	st := mkState(
		good("g1", "Сталь", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	st.Goods[0].Volume = &v
	st.Goods[0].Weight = &w
	require.Empty(t, Validate(st))
}

// TestMissingVolumeWeight — approved-товар без веса/объёма → warning
// (спека 2026-09-20-фабрики §3.1, решение 3b.6.4: NULL-каталог запрещён);
// ресурсы пропускаются (сырьё не имеет объёма/веса как товар).
func TestMissingVolumeWeight(t *testing.T) {
	v, w := 1.0, 2.0
	st := mkState(
		good("g1", "Сталь", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe"),
	)
	require.Contains(t, codes(Validate(st)), "missing_volume_weight")

	st.Goods[0].Volume = &v
	st.Goods[0].Weight = &w
	require.NotContains(t, codes(Validate(st)), "missing_volume_weight")

	// draft без веса/объёма — не флагается (NULL у draft разрешён).
	st2 := mkState(good("g2", "Черновик", model.StatusDraft))
	require.NotContains(t, codes(Validate(st2)), "missing_volume_weight")
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

// --- Ресурсы по kind (критика №1, спека переноса-студии-товаров iterA §8.3) ---

// TestApprovedResourcesNoIncompleteChain — 131 approved-ресурс без слотов
// не даёт «неполных цепочек» (иначе сид флагался бы каждый ресурс).
func TestApprovedResourcesNoIncompleteChain(t *testing.T) {
	goods := make([]model.Good, 0, 131)
	for i := 0; i < 131; i++ {
		goods = append(goods, model.Good{
			ID: fmt.Sprintf("r%d", i), Name: fmt.Sprintf("Ресурс %d", i),
			Category: "mineral", Status: model.StatusApproved, Kind: model.KindResource,
		})
	}
	w := Validate(mkState(goods...))
	require.NotContains(t, codes(w), "incomplete_chain")
	require.NotContains(t, codes(w), "missing_resource")
}

// TestNonApprovedRefResourceByKind — ссылка approved-товара на draft-ресурс
// не флагается (ресурс — валидный лист независимо от статуса, §8.3 п.3).
func TestNonApprovedRefResourceByKind(t *testing.T) {
	st := mkState(
		good("g1", "Согласованный", model.StatusApproved, model.Slot{GoodID: "r1"}),
		model.Good{ID: "r1", Name: "Железо Fe", Category: "mineral", Status: model.StatusDraft, Kind: model.KindResource},
	)
	w := Validate(st)
	require.NotContains(t, codes(w), "non_approved_ref")
}