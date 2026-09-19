package ai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/cmd/goods-studio/model"
)

// --- Разбор ответа (спека 99a.1 §7.3/§7.4) ---

// TestParseValidJSON — валидный JSON разбирается.
func TestParseValidJSON(t *testing.T) {
	comps, err := ParseFillResponse(`{"components":[{"name":"Сталь","category":"c3","reason":"основа"}]}`)
	require.NoError(t, err)
	require.Len(t, comps, 1)
	require.Equal(t, "Сталь", comps[0].Name)
	require.Equal(t, "c3", comps[0].Category)
	require.Equal(t, "основа", comps[0].Reason)
}

// TestParseCodeFence — обёртка в ```json ... ``` снимается.
func TestParseCodeFence(t *testing.T) {
	comps, err := ParseFillResponse("```json\n{\"components\":[{\"name\":\"Сталь\"}]}\n```")
	require.NoError(t, err)
	require.Len(t, comps, 1)
	require.Equal(t, "Сталь", comps[0].Name)
}

// TestParseGarbage — мусор → ошибка (слоты остаются пустыми, §7.4 п.1).
func TestParseGarbage(t *testing.T) {
	_, err := ParseFillResponse("это не json вообще")
	require.Error(t, err)
	_, err = ParseFillResponse(`{"components": "не массив"}`)
	require.Error(t, err)
}

// TestParseEmptyNames — пустые имена отбрасываются.
func TestParseEmptyNames(t *testing.T) {
	comps, err := ParseFillResponse(`{"components":[{"name":"  "},{"name":"Сталь"}]}`)
	require.NoError(t, err)
	require.Len(t, comps, 1)
	require.Equal(t, "Сталь", comps[0].Name)
}

// --- Промпт (99a Пакет 4, п.8: какие пустые слоты допускают ресурсы) ---

// TestPromptAllowResourceSlots — промпт перечисляет слоты с галкой
// «заполнять ресурсом» (1-базовые позиции среди пустых) и запрещает ресурсы
// для остальных.
func TestPromptAllowResourceSlots(t *testing.T) {
	g := &model.Good{Name: "Корабль", Category: "c1"}
	p := BuildPrompt(g, "Корабли", 2, nil, nil, []string{"Железо Fe"}, []string{"c1: Корабли"}, 3, []int{2, 3})
	require.Contains(t, p, "допускающие ресурсы")
	require.Contains(t, p, "2, 3")
	require.Contains(t, p, "только товары, НЕ ресурсы")
}

// TestPromptNoAllowResourceSlots — без галок промпт запрещает ресурсы
// для всех пустых слотов.
func TestPromptNoAllowResourceSlots(t *testing.T) {
	g := &model.Good{Name: "Корабль", Category: "c1"}
	p := BuildPrompt(g, "Корабли", 2, nil, nil, []string{"Железо Fe"}, []string{"c1: Корабли"}, 2, nil)
	require.Contains(t, p, "Ни один пустой слот не допускает ресурсы")
	require.Contains(t, p, "только товары, не ресурсы")
}

// --- Применение ответа (спека 99a.1 §7.4) ---

// mkState — состояние: категория c1 + товар goodID с recipe.
func mkState(recipe []model.Slot) *model.State {
	return &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "c1", Name: "Корабли"}},
		Goods: []model.Good{
			{ID: "g1", Name: "Корабль", Category: "c1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: recipe},
		},
	}
}

// TestApplyExactK — ровно K элементов: по одному на каждый пустой слот.
func TestApplyExactK(t *testing.T) {
	st := mkState([]model.Slot{{}, {}})
	report := ApplyFill(st, "g1", []Component{{Name: "Сталь"}, {Name: "Топливо"}})
	require.Len(t, st.Goods[0].Recipe, 2)
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Equal(t, "g3", st.Goods[0].Recipe[1].GoodID)
	require.Len(t, st.Goods, 3)
	require.Equal(t, model.StatusDraft, st.Goods[1].Status)
	require.Equal(t, model.SourceAI, st.Goods[1].Source)
	require.Equal(t, "", st.Goods[1].Category) // ИИ не указал категорию — без категории (доработка 2026-09-19)
	require.Empty(t, report)
}

// TestApplyCategoryFromAI — категория из ответа ИИ (валидный ID) ставится
// товару (доработка 2026-09-19: категорию проставляет нейросеть, не родитель).
func TestApplyCategoryFromAI(t *testing.T) {
	st := mkState([]model.Slot{{}})
	report := ApplyFill(st, "g1", []Component{{Name: "Сталь", Category: "c1"}})
	require.Equal(t, "c1", st.Goods[1].Category)
	require.Empty(t, report)
}

// TestApplyCategoryByName — категория по имени (не ID) сопоставляется с
// категорией студии нормализованным совпадением.
func TestApplyCategoryByName(t *testing.T) {
	st := mkState([]model.Slot{{}})
	report := ApplyFill(st, "g1", []Component{{Name: "Сталь", Category: "  Корабли "}})
	require.Equal(t, "c1", st.Goods[1].Category)
	require.Empty(t, report)
}

// TestApplyCategoryUnknown — несуществующая категория: товар без категории
// ("") + предупреждение в отчёте (новые категории не создаём — решение №5).
func TestApplyCategoryUnknown(t *testing.T) {
	st := mkState([]model.Slot{{}})
	report := ApplyFill(st, "g1", []Component{{Name: "Сталь", Category: "Несуществующая"}})
	require.Equal(t, "", st.Goods[1].Category)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "Несуществующая")
	require.Contains(t, report[0], "не существует")
}

// TestApplyCategoryEmpty — ИИ не указал категорию: товар без категории,
// без предупреждения.
func TestApplyCategoryEmpty(t *testing.T) {
	st := mkState([]model.Slot{{}})
	report := ApplyFill(st, "g1", []Component{{Name: "Сталь"}})
	require.Equal(t, "", st.Goods[1].Category)
	require.Empty(t, report)
}

// TestApplySmallResponse — меньше K: вставка того, что есть; остальные
// слоты пустые + отчёт (§7.4 п.2).
func TestApplySmallResponse(t *testing.T) {
	st := mkState([]model.Slot{{}, {}, {}})
	report := ApplyFill(st, "g1", []Component{{Name: "Сталь"}})
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Empty(t, st.Goods[0].Recipe[1].GoodID)
	require.Empty(t, st.Goods[0].Recipe[2].GoodID)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "меньше запрошенного")
}

// TestApplyBigResponse — больше K: первые K по порядку, лишние дроп + отчёт
// (§7.4 п.3).
func TestApplyBigResponse(t *testing.T) {
	st := mkState([]model.Slot{{}, {}})
	report := ApplyFill(st, "g1", []Component{{Name: "A"}, {Name: "B"}, {Name: "C"}})
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID) // A
	require.Equal(t, "g3", st.Goods[0].Recipe[1].GoodID) // B
	require.Len(t, st.Goods, 3)                          // C не создан
	require.Len(t, report, 1)
	require.Contains(t, report[0], "больше запрошенного")
	require.Contains(t, report[0], "1 лишних")
}

// TestApplyBanDrop — совпадение с баном → дроп + отчёт, слот пуст (§7.4 п.4).
func TestApplyBanDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Запрещёнка", Status: model.StatusBanned, Kind: model.KindGood})
	report := ApplyFill(st, "g1", []Component{{Name: "Запрещёнка"}})
	require.Empty(t, st.Goods[0].Recipe[0].GoodID)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "забаненное")
	require.Contains(t, report[0], "Запрещёнка")
}

// TestApplyExcludedDrop — совпадение с исключённым → дроп + отчёт.
func TestApplyExcludedDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Исключёнка", Status: model.StatusExcluded, Kind: model.KindGood})
	report := ApplyFill(st, "g1", []Component{{Name: "Исключёнка"}})
	require.Empty(t, st.Goods[0].Recipe[0].GoodID)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "исключённое")
}

// TestApplyLinkExisting — совпадение с существующим товаром → ссылка,
// новый товар не создаётся (§6.3).
func TestApplyLinkExisting(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Сталь", Status: model.StatusApproved, Kind: model.KindGood})
	report := ApplyFill(st, "g1", []Component{{Name: "  сталь "}})
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Len(t, st.Goods, 2) // новый не создан
	require.Empty(t, report)
}

// TestApplyLinkResource — совпадение с ресурсом → ссылка на ресурс.
// Слот с галкой «заполнять ресурсом» (99a Пакет 4, п.8): ресурс принимается.
func TestApplyLinkResource(t *testing.T) {
	st := mkState([]model.Slot{{AllowResource: true}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusResource, Kind: model.KindResource})
	report := ApplyFill(st, "g1", []Component{{Name: "Железо Fe"}})
	require.Equal(t, "res:zhelezo", st.Goods[0].Recipe[0].GoodID)
	require.Empty(t, report)
}

// TestApplyResourceWithoutFlagDrop — ресурс в слот БЕЗ галки «заполнять
// ресурсом» → дроп + отчёт (99a Пакет 4, п.8: ресурсы в рецепт только если
// прямо указано; «прямо указано» = галка на слоте).
func TestApplyResourceWithoutFlagDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusResource, Kind: model.KindResource})
	report := ApplyFill(st, "g1", []Component{{Name: "Железо Fe"}})
	require.Empty(t, st.Goods[0].Recipe[0].GoodID) // слот пуст
	require.Len(t, report, 1)
	require.Contains(t, report[0], "не разрешён")
	require.Contains(t, report[0], "заполнять ресурсом")
}

// TestApplyResourceWithFlagAccepted — ресурс в слот С галкой → принимается
// (99a Пакет 4, п.8), отчёт пуст.
func TestApplyResourceWithFlagAccepted(t *testing.T) {
	st := mkState([]model.Slot{{AllowResource: true}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusResource, Kind: model.KindResource})
	report := ApplyFill(st, "g1", []Component{{Name: "Железо Fe"}})
	require.Equal(t, "res:zhelezo", st.Goods[0].Recipe[0].GoodID)
	require.Empty(t, report)
}

// TestApplyResourceFlagPerSlot — галка действует по-слотово: ресурс в слот
// с галкой принимается, в слот без галки — дроп (99a Пакет 4, п.8).
func TestApplyResourceFlagPerSlot(t *testing.T) {
	st := mkState([]model.Slot{{AllowResource: true}, {}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusResource, Kind: model.KindResource})
	report := ApplyFill(st, "g1", []Component{{Name: "Железо Fe"}, {Name: "Железо Fe"}})
	require.Equal(t, "res:zhelezo", st.Goods[0].Recipe[0].GoodID) // слот 0 (галка) — принят
	require.Empty(t, st.Goods[0].Recipe[1].GoodID)                // слот 1 (без галки) — дроп
	require.Len(t, report, 1)
	require.Contains(t, report[0], "не разрешён")
}

// TestApplyCycleDrop — цикл из ИИ-ответа: дроп позиции, слот пуст (§7.4 п.5).
func TestApplyCycleDrop(t *testing.T) {
	// g1 (Корабль) уже содержит g2 (Сталь); ИИ предлагает g1 в слот g2 —
	// это цикл (g1 достижим из g2? нет — g2 не содержит g1).
	// Строим обратный случай: ИИ предлагает g2 в слот g1, а g2 уже
	// содержит g1 → цикл.
	st := &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "c1", Name: "Корабли"}},
		Goods: []model.Good{
			{ID: "g1", Name: "Корабль", Category: "c1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: []model.Slot{{}}},
			{ID: "g2", Name: "Сталь", Category: "c1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: []model.Slot{{GoodID: "g1"}}},
		},
	}
	report := ApplyFill(st, "g1", []Component{{Name: "Сталь"}})
	require.Empty(t, st.Goods[0].Recipe[0].GoodID) // слот пуст
	require.Len(t, report, 1)
	require.Contains(t, report[0], "цикл")
}

// TestApplyReason — reason из ответа ИИ сохраняется в слоте (тултип, §7.3).
func TestApplyReason(t *testing.T) {
	st := mkState([]model.Slot{{}})
	ApplyFill(st, "g1", []Component{{Name: "Сталь", Reason: "несущий каркас"}})
	require.Equal(t, "несущий каркас", st.Goods[0].Recipe[0].Reason)
}

// TestApplyNoEmptySlots — нет пустых слотов → отчёт, ничего не меняется.
func TestApplyNoEmptySlots(t *testing.T) {
	st := mkState([]model.Slot{{GoodID: "g2"}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Сталь", Kind: model.KindGood})
	report := ApplyFill(st, "g1", []Component{{Name: "Топливо"}})
	require.Len(t, report, 1)
	require.Contains(t, report[0], "нет пустых слотов")
	require.Len(t, st.Goods, 2)
}