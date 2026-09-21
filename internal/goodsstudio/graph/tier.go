// Package graph — граф рецептов (спека 99a.1 §6): тиры, проверка циклов,
// нормализация имён и дубликаты.
package graph

import "zorion/internal/goodsstudio/model"

// Tier — тир товара в графе (спека 99a.1 §6.1). Производный, не хранится:
//
//	тир(ресурс) = 0
//	тир(товар)  = 1 + max(тиры заполненных слотов)
//	тир(товар с пустым рецептом) = 0 (лист — цепочка не достроена)
func Tier(g *model.Good, byID map[string]*model.Good) int {
	if g.Kind == model.KindResource {
		return 0
	}
	hasFilled := false
	max := 0
	for _, slot := range g.Recipe {
		if slot.GoodID == "" {
			continue
		}
		hasFilled = true
		comp := byID[slot.GoodID]
		if comp == nil {
			continue
		}
		if t := Tier(comp, byID); t > max {
			max = t
		}
	}
	if !hasFilled {
		return 0
	}
	return 1 + max
}

// EffectiveTier — эффективный тир (99a.3 §4.2 + спека
// 2026-09-21-рецепт-сущность §2.3/§5): сложность рецепта
// (Good.Complexity), если задана, иначе вычисленный; ресурс = 0 (рецепта у
// ресурса нет — ручной сложности не существует). Сложность не влияет на
// структуру графа (Tier остаётся вычисленным для промпта ИИ, валидаторов,
// глубины — §4.4/§5).
func EffectiveTier(g *model.Good, byID map[string]*model.Good) int {
	if g.Kind == model.KindResource {
		return 0
	}
	if g.Complexity != nil {
		return *g.Complexity
	}
	return Tier(g, byID)
}

// ByID — карта id → указатель на товар (для чтения графа).
func ByID(goods []model.Good) map[string]*model.Good {
	m := make(map[string]*model.Good, len(goods))
	for i := range goods {
		m[goods[i].ID] = &goods[i]
	}
	return m
}