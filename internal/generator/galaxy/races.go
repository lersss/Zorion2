// internal/generator/galaxy/races.go — раздача рас по территориям
// (спека 99.2.21 §7, идея 56a «Механика заселения»; роботы — 99.2.24 §5).
//
// Кластеров (регионов) больше, чем рас (60): регионы группируются по
// пространственной близости центров в 60 территорий, каждая территория
// получает одну расу (доминанту, regions.race_id). Все 60 рас получают
// территорию (при числе регионов ≥ 60). Био-расы сопоставляются территориям
// случайно; роботорасы (territory "conditions") — со стремлением к
// рассеиванию по галактике (мягкое, 99.2.24 §5 п.2).
package galaxy

import (
	"log"
	"math"
	"sort"

	"zorion/internal/models"
	"zorion/internal/races"
)

// assignRacesToRegions — раздача рас по территориям. Вызывается после
// buildRegions, до генерации миров (детерминизм по seed: группировка
// детерминирована центрами, сопоставление рас — через g.rng).
// Каталог не загружен — no-op (регионы остаются без расы).
//
// Био-расы (territory "adjacency"/пусто) сопоставляются территориям случайно
// (Fisher-Yates через g.rng), как раньше (99.2.21 §7.1). Роботорасы
// (territory "conditions", 99.2.24 §3.2) — со стремлением к рассеиванию по
// галактике (мягкое, §5 п.2): каждый робот выбирает свободную территорию,
// максимизирующую минимальное расстояние до уже занятых роботами территорий.
// Best-effort: если свободных территорий нет (малая галактика) — робот не
// получает территории (разносится сколько возможно); в отчёт генерации
// добавляется пометка о неразнесённых роботорасах.
func (g *Generator) assignRacesToRegions(regions []*models.Region) {
	catalog := races.Catalog()
	if len(catalog) == 0 || len(regions) == 0 {
		return
	}

	territories := groupRegionsIntoTerritories(regions, len(catalog))
	T := len(territories)

	// Разделяем каталог на роботов (territory == "conditions") и био-расы.
	var robots, bio []*races.Race
	for _, r := range catalog {
		if r.Territory == "conditions" {
			robots = append(robots, r)
		} else {
			bio = append(bio, r)
		}
	}

	// Био-расы — случайное сопоставление территориям (Fisher-Yates через
	// g.rng — детерминизм по seed), как раньше.
	bioIDs := make([]string, len(bio))
	for i, r := range bio {
		bioIDs[i] = r.ID
	}
	for i := len(bioIDs) - 1; i > 0; i-- {
		j := g.rng.Intn(i + 1)
		bioIDs[i], bioIDs[j] = bioIDs[j], bioIDs[i]
	}

	assigned := make([]bool, T)
	robotTerritories := []int{} // индексы территорий, занятых роботами

	// Роботы: стремление к рассеиванию (мягкое, 99.2.24 §5 п.2). Каждый
	// робот выбирает свободную территорию, максимизирующую минимальное
	// расстояние до уже занятых роботами территорий. Best-effort: если
	// свободных территорий нет — робот не получает территории.
	for _, r := range robots {
		best := -1
		bestScore := -1.0
		for ti := 0; ti < T; ti++ {
			if assigned[ti] {
				continue
			}
			score := minDistToRobotTerritories(ti, robotTerritories, territories, regions)
			if score > bestScore {
				bestScore = score
				best = ti
			}
		}
		if best < 0 {
			break
		}
		assigned[best] = true
		robotTerritories = append(robotTerritories, best)
		for _, ri := range territories[best] {
			regions[ri].RaceID = r.ID
		}
	}

	// Био-расы — оставшиеся территории (в перемешанном порядке).
	bioIdx := 0
	for ti := 0; ti < T && bioIdx < len(bioIDs); ti++ {
		if assigned[ti] {
			continue
		}
		for _, ri := range territories[ti] {
			regions[ri].RaceID = bioIDs[bioIdx]
		}
		assigned[ti] = true
		bioIdx++
	}

	// Отчёт о неразнесённых роботорасах (99.2.24 §5 п.2): сколько удалось
	// разнести из 10, если не все.
	if spread, total := robotSpreadingReport(territories, robotTerritories, regions); total > 0 && spread < total {
		log.Printf("⚠️ Роботорасы: разнесено %d из %d (малая галактика / мало территорий)", spread, total)
	}
}

// minDistToRobotTerritories — минимальное расстояние от центроида территории
// ti до центроидов уже занятых роботами территорий. Пустой список — +Inf
// (первый робот: любая территория).
func minDistToRobotTerritories(ti int, robotTerritories []int, territories [][]int, regions []*models.Region) float64 {
	if len(robotTerritories) == 0 {
		return math.Inf(1)
	}
	best := math.Inf(1)
	for _, rt := range robotTerritories {
		d := centroidDist(territories[ti], territories[rt], regions)
		if d < best {
			best = d
		}
	}
	return best
}

// robotSpreadingReport — сколько роботорас разнесено (не соседствуют с другой
// роботорасой). Соседство = центроиды территорий ближе медианного расстояния
// до ближайшего соседа по всем территориям (порог из данных галактики).
func robotSpreadingReport(territories [][]int, robotTerritories []int, regions []*models.Region) (spread, total int) {
	total = len(robotTerritories)
	if total == 0 {
		return 0, 0
	}
	nn := make([]float64, len(territories))
	for i := range territories {
		best := math.Inf(1)
		for j := range territories {
			if i == j {
				continue
			}
			d := centroidDist(territories[i], territories[j], regions)
			if d < best {
				best = d
			}
		}
		nn[i] = best
	}
	sort.Float64s(nn)
	threshold := nn[len(nn)/2]

	spread = 0
	for _, ti := range robotTerritories {
		adjacent := false
		for _, tj := range robotTerritories {
			if ti == tj {
				continue
			}
			if centroidDist(territories[ti], territories[tj], regions) < threshold {
				adjacent = true
				break
			}
		}
		if !adjacent {
			spread++
		}
	}
	return spread, total
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

// centroidDist — расстояние между центроидами двух групп.
func centroidDist(a, b []int, regions []*models.Region) float64 {
	return math.Sqrt(centroidDistSq(a, b, regions))
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