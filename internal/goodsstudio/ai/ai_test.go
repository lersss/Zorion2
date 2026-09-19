package ai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
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

// --- Применение ответа (спека iterC §5.4): ApplyProposals per-slot ---

// mkState — состояние: категория 1 (kind=good) + товар goodID с recipe.
func mkState(recipe []model.Slot) *model.State {
	return &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "1", Name: "Корабли", Kind: model.KindGood}},
		Goods: []model.Good{
			{ID: "g1", Name: "Корабль", Category: "1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: recipe},
		},
	}
}

// newItem — ProposalItem kind=new с категорией 1 (выбор попапа).
func newItem(slot int, name string) ProposalItem {
	return ProposalItem{Slot: slot, Name: name, CategoryID: "1", Kind: "new"}
}

// TestApplyExactK — ровно K пунктов: по одному на каждый пустой слот.
func TestApplyExactK(t *testing.T) {
	st := mkState([]model.Slot{{}, {}})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Сталь"), newItem(1, "Топливо")})
	require.Equal(t, 2, applied)
	require.Len(t, st.Goods[0].Recipe, 2)
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Equal(t, "g3", st.Goods[0].Recipe[1].GoodID)
	require.Len(t, st.Goods, 3)
	require.Equal(t, model.StatusDraft, st.Goods[1].Status)
	require.Equal(t, model.SourceAI, st.Goods[1].Source)
	require.Equal(t, "1", st.Goods[1].Category) // категория из пункта (выбор попапа)
	require.Empty(t, report)
}

// TestApplyCategoryFromItem — категория из пункта (выбор попапа) ставится
// товару (доработка 2026-09-19: категорию проставляет нейросеть, не родитель;
// в C финальная категория — выбор попапа).
func TestApplyCategoryFromItem(t *testing.T) {
	st := mkState([]model.Slot{{}})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Сталь")})
	require.Equal(t, 1, applied)
	require.Equal(t, "1", st.Goods[1].Category)
	require.Empty(t, report)
}

// TestApplySmallResponse — меньше K: вставка того, что есть; остальные
// слоты пустые (информация «меньше запрошенного» — на фазе BuildProposals).
func TestApplySmallResponse(t *testing.T) {
	st := mkState([]model.Slot{{}, {}, {}})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Сталь")})
	require.Equal(t, 1, applied)
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Empty(t, st.Goods[0].Recipe[1].GoodID)
	require.Empty(t, st.Goods[0].Recipe[2].GoodID)
	require.Empty(t, report)
}

// TestApplyBanDrop — совпадение с баном → дроп + отчёт, слот пуст (§7.4 п.4).
func TestApplyBanDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Запрещёнка", Status: model.StatusBanned, Kind: model.KindGood})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Запрещёнка", Kind: "link"}})
	require.Equal(t, 0, applied)
	require.Empty(t, st.Goods[0].Recipe[0].GoodID)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "забаненное")
	require.Contains(t, report[0], "Запрещёнка")
}

// TestApplyExcludedDrop — совпадение с исключённым → дроп + отчёт.
func TestApplyExcludedDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Исключёнка", Status: model.StatusExcluded, Kind: model.KindGood})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Исключёнка", Kind: "link"}})
	require.Equal(t, 0, applied)
	require.Empty(t, st.Goods[0].Recipe[0].GoodID)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "исключённое")
}

// TestApplyLinkExisting — совпадение с существующим товаром → ссылка,
// новый товар не создаётся (§6.3).
func TestApplyLinkExisting(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Сталь", Status: model.StatusApproved, Kind: model.KindGood})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "  сталь ", Kind: "link"}})
	require.Equal(t, 1, applied)
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Len(t, st.Goods, 2) // новый не создан
	require.Empty(t, report)
}

// TestApplyLinkResource — совпадение с ресурсом → ссылка на ресурс.
// Слот с галкой «заполнять ресурсом» (99a Пакет 4, п.8): ресурс принимается.
func TestApplyLinkResource(t *testing.T) {
	st := mkState([]model.Slot{{AllowResource: true}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusApproved, Kind: model.KindResource})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Железо Fe", Kind: "link"}})
	require.Equal(t, 1, applied)
	require.Equal(t, "res:zhelezo", st.Goods[0].Recipe[0].GoodID)
	require.Empty(t, report)
}

// TestApplyResourceWithoutFlagDrop — ресурс в слот БЕЗ галки «заполнять
// ресурсом» → дроп + отчёт (99a Пакет 4, п.8).
func TestApplyResourceWithoutFlagDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusApproved, Kind: model.KindResource})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Железо Fe", Kind: "link"}})
	require.Equal(t, 0, applied)
	require.Empty(t, st.Goods[0].Recipe[0].GoodID) // слот пуст
	require.Len(t, report, 1)
	require.Contains(t, report[0], "не разрешён")
	require.Contains(t, report[0], "заполнять ресурсом")
}

// TestApplyResourceWithFlagAccepted — ресурс в слот С галкой → принимается
// (99a Пакет 4, п.8), отчёт пуст.
func TestApplyResourceWithFlagAccepted(t *testing.T) {
	st := mkState([]model.Slot{{AllowResource: true}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusApproved, Kind: model.KindResource})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Железо Fe", Kind: "link"}})
	require.Equal(t, 1, applied)
	require.Equal(t, "res:zhelezo", st.Goods[0].Recipe[0].GoodID)
	require.Empty(t, report)
}

// TestApplyResourceFlagPerSlot — галка действует по-слотово: ресурс в слот
// с галкой принимается, в слот без галки — дроп (99a Пакет 4, п.8).
func TestApplyResourceFlagPerSlot(t *testing.T) {
	st := mkState([]model.Slot{{AllowResource: true}, {}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusApproved, Kind: model.KindResource})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{
		{Slot: 0, Name: "Железо Fe", Kind: "link"},
		{Slot: 1, Name: "Железо Fe", Kind: "link"},
	})
	require.Equal(t, 1, applied)
	require.Equal(t, "res:zhelezo", st.Goods[0].Recipe[0].GoodID) // слот 0 (галка) — принят
	require.Empty(t, st.Goods[0].Recipe[1].GoodID)                // слот 1 (без галки) — дроп
	require.Len(t, report, 1)
	require.Contains(t, report[0], "не разрешён")
}

// TestApplyCycleDrop — цикл из ИИ-ответа: дроп позиции, слот пуст (§7.4 п.5).
func TestApplyCycleDrop(t *testing.T) {
	// g1 (Корабль) уже содержит g2 (Сталь); ИИ предлагает g2 в слот g1,
	// а g2 уже содержит g1 → цикл.
	st := &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "1", Name: "Корабли", Kind: model.KindGood}},
		Goods: []model.Good{
			{ID: "g1", Name: "Корабль", Category: "1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: []model.Slot{{}}},
			{ID: "g2", Name: "Сталь", Category: "1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: []model.Slot{{GoodID: "g1"}}},
		},
	}
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Сталь", Kind: "link"}})
	require.Equal(t, 0, applied)
	require.Empty(t, st.Goods[0].Recipe[0].GoodID) // слот пуст
	require.Len(t, report, 1)
	require.Contains(t, report[0], "цикл")
}

// TestApplyReason — reason из пункта сохраняется в слоте (тултип, §7.3).
func TestApplyReason(t *testing.T) {
	st := mkState([]model.Slot{{}})
	applied, _ := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Сталь", CategoryID: "1", Kind: "new", Reason: "несущий каркас"}})
	require.Equal(t, 1, applied)
	require.Equal(t, "несущий каркас", st.Goods[0].Recipe[0].Reason)
}

// TestApplyNoEmptySlots — нет пустых слотов → отчёт, ничего не меняется.
func TestApplyNoEmptySlots(t *testing.T) {
	st := mkState([]model.Slot{{GoodID: "g2"}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Сталь", Kind: model.KindGood})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Топливо")})
	require.Equal(t, 0, applied)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "нет пустых слотов")
	require.Len(t, st.Goods, 2)
}

// --- ApplyProposals: подмножество и дропы apply-фазы (спека iterC §5.4/§11) ---

// TestApplySubset — приняты пункты 0 и 2 → заполнены слоты 0 и 2;
// пропущенный (1) не тронут (per-slot, не «первые пустые по порядку»).
func TestApplySubset(t *testing.T) {
	st := mkState([]model.Slot{{}, {}, {}})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Сталь"), newItem(2, "Топливо")})
	require.Equal(t, 2, applied)
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Empty(t, st.Goods[0].Recipe[1].GoodID) // пропущенный — пуст
	require.Equal(t, "g3", st.Goods[0].Recipe[2].GoodID)
	require.Len(t, st.Goods, 3)
	require.Empty(t, report)
}

// TestApplyLinkTargetGoneC1 — link-цель удалена/переименована между фазами →
// дроп + отчёт «ссылка исчезла», НЕ создаётся новый товар (С1).
func TestApplyLinkTargetGoneC1(t *testing.T) {
	st := mkState([]model.Slot{{}})
	// g2 (Сталь) удалена между фазами — в снимке её нет
	applied, report := ApplyProposals(st, "g1", []ProposalItem{{Slot: 0, Name: "Сталь", Kind: "link"}})
	require.Equal(t, 0, applied)
	require.Empty(t, st.Goods[0].Recipe[0].GoodID)
	require.Len(t, st.Goods, 1) // новый товар НЕ создан
	require.Len(t, report, 1)
	require.Contains(t, report[0], "ссылка исчезла")
}

// TestApplyCategoryGoneC2 — категория удалена между фазами → дроп + отчёт
// «категория удалена» (С2).
func TestApplyCategoryGoneC2(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Categories = nil // категория удалена между фазами
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Сталь")})
	require.Equal(t, 0, applied)
	require.Empty(t, st.Goods[0].Recipe[0].GoodID)
	require.Len(t, st.Goods, 1)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "категория удалена")
}

// TestApplyCategoryWrongKindC2 — категория стала ресурсной (kind=resource) →
// дроп (С2: новые товары — только в товарные категории).
func TestApplyCategoryWrongKindC2(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Categories = []model.Category{{ID: "1", Name: "Минералы", Kind: model.KindResource}}
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Сталь")})
	require.Equal(t, 0, applied)
	require.Empty(t, st.Goods[0].Recipe[0].GoodID)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "категория удалена")
}

// TestApplySlotGoneC3 — слот удалён (DeleteSlot — сдвиг pos) или вручную
// заполнен между фазами → дроп + отчёт «слот занят/удалён» (С3).
func TestApplySlotGoneC3(t *testing.T) {
	st := mkState([]model.Slot{{GoodID: "g2"}, {}}) // слот 0 вручную заполнен между фазами
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Сталь", Kind: model.KindGood})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Топливо")})
	require.Equal(t, 0, applied)
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID) // не тронут
	require.Len(t, st.Goods, 2)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "слот 0 занят/удалён")
}

// TestApplySlotOutOfRangeC3 — слот вне диапазона (сдвиг pos после DeleteSlot).
func TestApplySlotOutOfRangeC3(t *testing.T) {
	st := mkState([]model.Slot{{}})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(5, "Топливо")})
	require.Equal(t, 0, applied)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "слот 5 занят/удалён")
}

// TestApplyNewNameFoundLinks — kind=new, но имя нашлось → ссылка на
// существующий (безопасно, категория игнорируется, С1).
func TestApplyNewNameFoundLinks(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Сталь", Status: model.StatusApproved, Kind: model.KindGood})
	applied, report := ApplyProposals(st, "g1", []ProposalItem{newItem(0, "Сталь")})
	require.Equal(t, 1, applied)
	require.Equal(t, "g2", st.Goods[0].Recipe[0].GoodID)
	require.Len(t, st.Goods, 2) // новый не создан
	require.Empty(t, report)
}

// --- BuildProposals (спека iterC §5.4/§11) ---

// TestBuildProposalsLink — совпадение по нормализованному имени → kind=link
// (link_id/link_name), категория не резолвится.
func TestBuildProposalsLink(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Сталь", Status: model.StatusApproved, Kind: model.KindGood})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "  сталь ", Category: "мусор", Reason: "каркас"}})
	require.Len(t, proposals, 1)
	require.Equal(t, "link", proposals[0].Kind)
	require.Equal(t, "g2", proposals[0].LinkID)
	require.Equal(t, "Сталь", proposals[0].LinkName)
	require.Equal(t, 0, proposals[0].Slot)
	require.Empty(t, report)
}

// TestBuildProposalsNewValidCategory — не найдено → kind=new, категория
// валидна (по имени, нормализованное совпадение).
func TestBuildProposalsNewValidCategory(t *testing.T) {
	st := mkState([]model.Slot{{}})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "Сталь", Category: "  Корабли "}})
	require.Len(t, proposals, 1)
	require.Equal(t, "new", proposals[0].Kind)
	require.True(t, proposals[0].CategoryValid)
	require.Equal(t, int64(1), proposals[0].CategoryID)
	require.Empty(t, report)
}

// TestBuildProposalsNewInvalidCategory — невалидная категория → kind=new,
// category_valid=false (пометка в попапе, не дроп).
func TestBuildProposalsNewInvalidCategory(t *testing.T) {
	st := mkState([]model.Slot{{}})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "Сталь", Category: "Несуществующая"}})
	require.Len(t, proposals, 1)
	require.Equal(t, "new", proposals[0].Kind)
	require.False(t, proposals[0].CategoryValid)
	require.Equal(t, int64(0), proposals[0].CategoryID)
	require.Equal(t, "Несуществующая", proposals[0].Category)
	require.Empty(t, report)
}

// TestBuildProposalsNewEmptyCategory — ИИ не указал категорию →
// category_valid=false, без шума.
func TestBuildProposalsNewEmptyCategory(t *testing.T) {
	st := mkState([]model.Slot{{}})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "Сталь"}})
	require.Len(t, proposals, 1)
	require.False(t, proposals[0].CategoryValid)
	require.Empty(t, report)
}

// TestBuildProposalsBigResponse — больше K: первые K по порядку, лишние
// дроп + отчёт «больше запрошенного» (М7).
func TestBuildProposalsBigResponse(t *testing.T) {
	st := mkState([]model.Slot{{}, {}})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "A"}, {Name: "B"}, {Name: "C"}})
	require.Len(t, proposals, 2) // C не предложен
	require.Len(t, report, 1)
	require.Contains(t, report[0], "больше запрошенного")
	require.Contains(t, report[0], "1 лишних")
}

// TestBuildProposalsSmallResponse — меньше K: информационно, слоты пустые.
func TestBuildProposalsSmallResponse(t *testing.T) {
	st := mkState([]model.Slot{{}, {}, {}})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "Сталь"}})
	require.Len(t, proposals, 1)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "меньше запрошенного")
}

// TestBuildProposalsBanDrop — совпадение с баном → дроп на разборе + отчёт.
func TestBuildProposalsBanDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "g2", Name: "Запрещёнка", Status: model.StatusBanned, Kind: model.KindGood})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "Запрещёнка"}})
	require.Empty(t, proposals)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "забаненное")
}

// TestBuildProposalsResourceWithoutFlagDrop — ресурс в слот без галки →
// дроп на разборе + отчёт.
func TestBuildProposalsResourceWithoutFlagDrop(t *testing.T) {
	st := mkState([]model.Slot{{}})
	st.Goods = append(st.Goods, model.Good{ID: "res:zhelezo", Name: "Железо Fe", Status: model.StatusApproved, Kind: model.KindResource})
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "Железо Fe"}})
	require.Empty(t, proposals)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "не разрешён")
}

// TestBuildProposalsCycleDrop — цикл для link → дроп на разборе + отчёт.
func TestBuildProposalsCycleDrop(t *testing.T) {
	st := &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "1", Name: "Корабли", Kind: model.KindGood}},
		Goods: []model.Good{
			{ID: "g1", Name: "Корабль", Category: "1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: []model.Slot{{}}},
			{ID: "g2", Name: "Сталь", Category: "1", Status: model.StatusDraft, Kind: model.KindGood, Recipe: []model.Slot{{GoodID: "g1"}}},
		},
	}
	proposals, report := BuildProposals(st, "g1", []Component{{Name: "Сталь"}})
	require.Empty(t, proposals)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "цикл")
}