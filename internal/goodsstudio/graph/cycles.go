package graph

import "zorion/internal/goodsstudio/model"

// WouldCreateCycle — true, если добавление составляющей `from` в слот
// товара `to` создаст цикл: `to` достижим из `from` по составляющим (DFS)
// или from == to (спека 99a.1 §6.2). Рёбра только вверх — циклы исключены
// построением; проверка сильнее сравнения тиров и работает при пустых
// рецептах (тир листа = 0).
func WouldCreateCycle(from, to string, byID map[string]*model.Good) bool {
	if from == to {
		return true
	}
	seen := make(map[string]bool)
	var dfs func(id string) bool
	dfs = func(id string) bool {
		if id == to {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		g := byID[id]
		if g == nil {
			return false
		}
		for _, slot := range g.Recipe {
			if slot.GoodID == "" {
				continue
			}
			if dfs(slot.GoodID) {
				return true
			}
		}
		return false
	}
	return dfs(from)
}

// HasCycle — true, если в графе есть цикл (страховка, спека 99a.1 §8 п.4).
// Циклы исключены построением (§6.2) — проверка-страховка, не «чинить».
func HasCycle(goods []model.Good) bool {
	byID := ByID(goods)
	for i := range goods {
		if reachableFromSelf(goods[i].ID, byID) {
			return true
		}
	}
	return false
}

// reachableFromSelf — достижим ли id из самого себя через составляющие
// (транзитивный цикл; прямой X→X ловится первым же слотом).
func reachableFromSelf(id string, byID map[string]*model.Good) bool {
	seen := make(map[string]bool)
	var dfs func(cur string) bool
	dfs = func(cur string) bool {
		g := byID[cur]
		if g == nil {
			return false
		}
		for _, slot := range g.Recipe {
			if slot.GoodID == "" {
				continue
			}
			if slot.GoodID == id {
				return true
			}
			if seen[slot.GoodID] {
				continue
			}
			seen[slot.GoodID] = true
			if dfs(slot.GoodID) {
				return true
			}
		}
		return false
	}
	return dfs(id)
}