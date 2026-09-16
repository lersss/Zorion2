// internal/generator/galaxy/races.go — раздача рас по территориям
// (спека 99.2.21 §7, идея 56a «Механика заселения»).
//
// Кластеров (регионов) больше, чем рас (50): регионы группируются по
// пространственной близости центров в 50 территорий, каждая территория
// получает одну расу (доминанту, regions.race_id). Все 50 рас получают
// территорию (при числе регионов ≥ 50).
package galaxy

import (
	"math"

	"zorion/internal/models"
	"zorion/internal/races"
)

// assignRacesToRegions — раздача рас по территориям. Вызывается после
// buildRegions, до генерации миров (детерминизм по seed: группировка
// детерминирована центрами, сопоставление рас — через g.rng).
// Каталог не загружен — no-op (регионы остаются без расы).
func (g *Generator) assignRacesToRegions(regions []*models.Region) {
	catalog := races.Catalog()
	if len(catalog) == 0 || len(regions) == 0 {
		return
	}

	territories := groupRegionsIntoTerritories(regions, len(catalog))

	// Случайное сопоставление рас территориям (Fisher-Yates через g.rng —
	// детерминизм по seed генерации).
	ids := make([]string, len(catalog))
	for i, r := range catalog {
		ids[i] = r.ID
	}
	for i := len(ids) - 1; i > 0; i-- {
		j := g.rng.Intn(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}

	for ti, group := range territories {
		raceID := ids[ti%len(ids)]
		for _, ri := range group {
			regions[ri].RaceID = raceID
		}
	}
}

// groupRegionsIntoTerritories — агломеративная кластеризация центров
// регионов до target групп: на каждом шаге объединяются две группы с
// минимальным расстоянием между центроидами. Детерминированно (без
// случайности; при равенстве расстояний побеждает первая пара в порядке
// обхода). При числе регионов ≤ target каждый регион — своя территория.
//
// Выбор агломеративной вместо K-means: K-means требует случайной
// инициализации центроидов (k-means++/ролл) и может давать пустые кластеры;
// агломеративная гарантирует ровно min(N, target) непустых групп и
// детерминирована входными центрами (которые детерминированы seed).
func groupRegionsIntoTerritories(regions []*models.Region, target int) [][]int {
	n := len(regions)
	if n <= target {
		out := make([][]int, n)
		for i := range out {
			out[i] = []int{i}
		}
		return out
	}

	groups := make([][]int, n)
	for i := range groups {
		groups[i] = []int{i}
	}

	for len(groups) > target {
		bestI, bestJ, bestD := -1, -1, math.Inf(1)
		for i := 0; i < len(groups); i++ {
			for j := i + 1; j < len(groups); j++ {
				d := centroidDistSq(groups[i], groups[j], regions)
				if d < bestD {
					bestD, bestI, bestJ = d, i, j
				}
			}
		}
		groups[bestI] = append(groups[bestI], groups[bestJ]...)
		groups = append(groups[:bestJ], groups[bestJ+1:]...)
	}
	return groups
}

// centroidDistSq — квадрат расстояния между центроидами двух групп.
func centroidDistSq(a, b []int, regions []*models.Region) float64 {
	ax, ay := centroid(a, regions)
	bx, by := centroid(b, regions)
	dx, dy := ax-bx, ay-by
	return dx*dx + dy*dy
}

// centroid — центроид группы регионов.
func centroid(group []int, regions []*models.Region) (float64, float64) {
	var sx, sy float64
	for _, i := range group {
		sx += regions[i].CenterX
		sy += regions[i].CenterY
	}
	n := float64(len(group))
	return sx / n, sy / n
}