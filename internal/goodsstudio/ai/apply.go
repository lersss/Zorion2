package ai

import (
	"zorion/internal/goodsstudio/model"
)

// indexOfGood — индекс товара по id.
func indexOfGood(goods []model.Good, id string) int {
	for i := range goods {
		if goods[i].ID == id {
			return i
		}
	}
	return -1
}

// emptySlotIndices — индексы пустых слотов рецепта (0-based).
func emptySlotIndices(recipe []model.Slot) []int {
	var out []int
	for i := range recipe {
		if recipe[i].GoodID == "" {
			out = append(out, i)
		}
	}
	return out
}

// statusWord — слово статуса для отчёта (бан/исключённое).
func statusWord(s model.Status) string {
	switch s {
	case model.StatusBanned:
		return "забаненное"
	case model.StatusExcluded:
		return "исключённое"
	}
	return string(s)
}