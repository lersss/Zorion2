// internal/resource/distance.go — проверка различимости каталога реальных
// веществ (идея 2026-09-18 §4б): порог «отличаются хотя бы по одной оси
// на 10» (решение создателя §1.3). Живой код оценки ёмкости: числа
// 92/110/40 из §4а считаются здесь, а не в документе.
package resource

import "math"

// MaxAxisDiff — максимум расхождения по 10 осям свойств (0–100).
func MaxAxisDiff(a, b *RealResource) float64 {
	diffs := []float64{
		math.Abs(a.Hardness - b.Hardness),
		math.Abs(a.Elasticity - b.Elasticity),
		math.Abs(a.Conductivity - b.Conductivity),
		math.Abs(a.Density - b.Density),
		math.Abs(a.EnergyDensity - b.EnergyDensity),
		math.Abs(a.Biocompatibility - b.Biocompatibility),
		math.Abs(a.Radioactivity - b.Radioactivity),
		math.Abs(a.Toxicity - b.Toxicity),
		math.Abs(a.Flammability - b.Flammability),
		math.Abs(a.ChemicalActivity - b.ChemicalActivity),
	}
	max := 0.0
	for _, d := range diffs {
		if d > max {
			max = d
		}
	}
	return max
}

// Colliding — коллизия профилей: ни одна ось не различается на ≥ 10
// (максимум расхождения < 10 по всем осям).
func Colliding(a, b *RealResource) bool {
	return MaxAxisDiff(a, b) < 10
}

// ResolvedByT — T-разрешение: T_melt или T_boil различаются ≥ 10 K.
// Кламп T учтён в данных (He/He-3 оба 10 K) — такие пары не разрешаются.
func ResolvedByT(a, b *RealResource) bool {
	return math.Abs(a.TMelt-b.TMelt) >= 10 || math.Abs(a.TBoil-b.TBoil) >= 10
}

// DistinctCount — различимость каталога: byAxes — число различимых профилей
// по осям (компоненты связности графа коллизий: каждая компонента = один
// различимый профиль), withT — с учётом T-разрешения, collisions — список
// коллизирующих пар (имена, по осям).
func DistinctCount(resources []*RealResource) (byAxes, withT int, collisions []struct{ A, B string }) {
	n := len(resources)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if Colliding(resources[i], resources[j]) {
				collisions = append(collisions, struct{ A, B string }{resources[i].Name, resources[j].Name})
			}
		}
	}
	byAxes = components(n, func(i, j int) bool { return Colliding(resources[i], resources[j]) })
	withT = components(n, func(i, j int) bool {
		return Colliding(resources[i], resources[j]) && !ResolvedByT(resources[i], resources[j])
	})
	return byAxes, withT, collisions
}

// components — число компонент связности графа на n вершинах (union-find),
// ребро между i и j — по предикату edge.
func components(n int, edge func(i, j int) bool) int {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if edge(i, j) {
				ri, rj := find(i), find(j)
				if ri != rj {
					parent[ri] = rj
				}
			}
		}
	}
	roots := make(map[int]bool)
	for i := 0; i < n; i++ {
		roots[find(i)] = true
	}
	return len(roots)
}