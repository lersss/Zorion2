package ai

import (
	"fmt"

	"zorion/cmd/goods-studio/graph"
	"zorion/cmd/goods-studio/model"
)

// ApplyFill применяет ответ ИИ к пустым слотам товара (спека 99a.1 §7.4):
// по одному товару на пустой слот; локальный фильтр бана/исключённого —
// инвариант (список в промпте — best-effort); совпадение с существующим —
// ссылка; иначе новый товар draft/source=ai; проверка цикла на каждую
// вставку (цикл → дроп позиции, слот остаётся пустым). Возвращает отчёт.
// Мутирует st.
func ApplyFill(st *model.State, goodID string, comps []Component) []string {
	var report []string
	gi := indexOfGood(st.Goods, goodID)
	if gi < 0 {
		return append(report, "товар не найден")
	}
	empty := emptySlotIndices(st.Goods[gi].Recipe)
	if len(empty) == 0 {
		return append(report, "нет пустых слотов")
	}
	k := len(empty)
	if len(comps) > k {
		report = append(report, fmt.Sprintf("ИИ вернул больше запрошенного: %d лишних — пропущены", len(comps)-k))
		comps = comps[:k]
	} else if len(comps) < k {
		report = append(report, fmt.Sprintf("ИИ вернул меньше запрошенного: %d из %d — остальные слоты пустые", len(comps), k))
	}

	byName := graph.NameIndex(st.Goods)

	type insert struct {
		slot   int
		compID string
		reason string
	}
	var inserts []insert

	for i, comp := range comps {
		name := graph.NormalizeName(comp.Name)
		if name == "" {
			continue
		}
		// бан/исключённое — дроп + отчёт (локальный фильтр, §7.4 п.4)
		if idx, ok := byName[name]; ok {
			existing := &st.Goods[idx]
			if existing.Status == model.StatusBanned || existing.Status == model.StatusExcluded {
				report = append(report, fmt.Sprintf("ИИ предложил %s: %s — пропущено", statusWord(existing.Status), existing.Name))
				continue
			}
			// ресурс в слот БЕЗ галки «заполнять ресурсом» — дроп + отчёт
			// (99a Пакет 4, п.8, решение создателя 2026-09-19: ресурсы в
			// рецепт только если прямо указано; «прямо указано» = галка на
			// слоте). С галкой — ресурс принимается (ссылка, ниже).
			if existing.Kind == model.KindResource && !st.Goods[gi].Recipe[empty[i]].AllowResource {
				report = append(report, fmt.Sprintf("ресурс %s не разрешён для этого слота — включи галку \"заполнять ресурсом\"", existing.Name))
				continue
			}
		}
		// существующий товар/ресурс — ссылка (новый товар не создаётся, §6.3)
		compID := ""
		if idx, ok := byName[name]; ok {
			compID = st.Goods[idx].ID
		} else {
			// категорию проставляет нейросеть (доработка 2026-09-19): строго
			// из реестра категорий студии; фолбека на родителя нет. Не найдена —
			// пустая категория + предупреждение (новые категории не создаём,
			// решение №5 — их прописывает создатель).
			cat, warn := resolveCategory(comp.Category, st.Categories)
			if warn != "" {
				report = append(report, warn)
			}
			newGood := model.Good{
				ID:        model.NextGoodID(st.Goods),
				Name:      comp.Name,
				Category:  cat,
				Status:    model.StatusDraft,
				Kind:      model.KindGood,
				Source:    model.SourceAI,
				Recipe:    []model.Slot{},
				CreatedAt: model.NowISO(),
			}
			st.Goods = append(st.Goods, newGood)
			compID = newGood.ID
			byName[name] = len(st.Goods) - 1
		}
		// цикл — дроп позиции, слот остаётся пустым (§7.4 п.5)
		byID := graph.ByID(st.Goods) // пересборка: append мог переаллоцировать слайс
		if graph.WouldCreateCycle(compID, goodID, byID) {
			report = append(report, fmt.Sprintf("ИИ предложил цикл: %s — пропущено", comp.Name))
			continue
		}
		inserts = append(inserts, insert{slot: empty[i], compID: compID, reason: comp.Reason})
	}

	// вставка в пустые слоты по порядку (частичная вставка принята, §7.4 п.2)
	for _, ins := range inserts {
		st.Goods[gi].Recipe[ins.slot].GoodID = ins.compID
		st.Goods[gi].Recipe[ins.slot].Reason = ins.reason
	}
	return report
}

func indexOfGood(goods []model.Good, id string) int {
	for i := range goods {
		if goods[i].ID == id {
			return i
		}
	}
	return -1
}

func emptySlotIndices(recipe []model.Slot) []int {
	var out []int
	for i := range recipe {
		if recipe[i].GoodID == "" {
			out = append(out, i)
		}
	}
	return out
}

// resolveCategory — категория из ответа ИИ (спека 99a.1 §7.3, доработка
// 2026-09-19): строго из реестра категорий студии. Порядок: точный ID →
// нормализованное совпадение имени → пустая категория + предупреждение
// (категории прописывает создатель, решение №5 — новые не создаём).
func resolveCategory(compCat string, cats []model.Category) (string, string) {
	if compCat == "" {
		return "", "" // ИИ не указал категорию — товар без категории, без шума
	}
	for _, c := range cats {
		if c.ID == compCat {
			return c.ID, ""
		}
	}
	norm := graph.NormalizeName(compCat)
	for _, c := range cats {
		if graph.NormalizeName(c.Name) == norm {
			return c.ID, ""
		}
	}
	return "", fmt.Sprintf("категория %q не существует — товар без категории, добавь категорию или поправь", compCat)
}

func statusWord(s model.Status) string {
	switch s {
	case model.StatusBanned:
		return "забаненное"
	case model.StatusExcluded:
		return "исключённое"
	}
	return string(s)
}