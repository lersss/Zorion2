package ai

import (
	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
)

// BuildFillPrompt — промпт «заполнить комплектующие» на снимке каталога
// (спека переноса-студии-товаров-iterC §5.4; перенос buildFillPrompt из
// cmd/goods-studio/handlers/state.go:763): tier — вычисленный (graph.Tier,
// оверрайд не влияет на генерацию), filled — имена заполненных слотов,
// banned — имена banned/excluded, resNames — kind=resource, catNames —
// "id: имя" (id — числовые строки БД), k пустых слотов, allowResSlots —
// 1-базовые позиции пустых слотов с галкой «заполнять ресурсом».
func BuildFillPrompt(st *model.State, goodID string) string {
	gi := indexOfGood(st.Goods, goodID)
	if gi < 0 {
		return ""
	}
	g := &st.Goods[gi]
	byID := graph.ByID(st.Goods)
	tier := graph.Tier(g, byID)
	catName := ""
	for _, c := range st.Categories {
		if c.ID == g.Category {
			catName = c.Name
			break
		}
	}
	var filled, banned, resNames, catNames []string
	for _, slot := range g.Recipe {
		if slot.GoodID == "" {
			continue
		}
		if comp := byID[slot.GoodID]; comp != nil {
			filled = append(filled, comp.Name)
		}
	}
	for i := range st.Goods {
		gg := &st.Goods[i]
		if gg.Status == model.StatusBanned || gg.Status == model.StatusExcluded {
			banned = append(banned, gg.Name)
		}
		if gg.Kind == model.KindResource {
			resNames = append(resNames, gg.Name)
		}
	}
	for _, c := range st.Categories {
		catNames = append(catNames, c.ID+": "+c.Name)
	}
	k := len(emptySlotIndices(g.Recipe))
	// позиции пустых слотов с галкой «заполнять ресурсом» (1-базовые, по
	// порядку пустых слотов) — для промпта (99a Пакет 4, п.8)
	var allowResSlots []int
	for pos, slotIdx := range emptySlotIndices(g.Recipe) {
		if g.Recipe[slotIdx].AllowResource {
			allowResSlots = append(allowResSlots, pos+1)
		}
	}
	return BuildPrompt(g, catName, tier, filled, banned, resNames, catNames, k, allowResSlots)
}