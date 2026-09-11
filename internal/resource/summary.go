// internal/resource/summary.go
package resource

import "zorion/internal/models"

// Summary — сводит сгенерированные ресурсы планеты к карте
// «категория (английский код) → богатство 0..1».
//
// Значение растёт с количеством ресурсов категории на планете
// (0.55 для одной, до 1.0). Всегда > 0.3 — порога фильтра карты.
func Summary(resources []*models.PlanetResource) map[string]float64 {
	counts := map[string]int{}
	for _, r := range resources {
		counts[r.Category]++
	}

	out := make(map[string]float64, len(counts))
	for cat, n := range counts {
		v := 0.4 + 0.15*float64(n)
		if v > 1 {
			v = 1
		}
		out[cat] = v
	}
	return out
}
