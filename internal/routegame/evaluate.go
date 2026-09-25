// internal/routegame/evaluate.go
// Оценка пути мини-игры «Прокладка маршрута» (ЧК3, подэтап S1a; спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md §6.3/§6.6): серверная
// валидация полилинии, эффективная длина L_eff, эталон L_ref и качество q.
// Только чистые функции — без БД, HTTP, RNG и времени.
package routegame

import "math"

// maxPathPoints — потолок числа точек полилинии (анти-абьюз, §6.6) — ГИПОТЕЗА.
const maxPathPoints = 64

// refOvershoot — эталон «чуть больше оптимума»: L_ref = refOvershoot × оптимум
// (§6.3), чтобы идеальный игрок был «сверх эталона» — ГИПОТЕЗА.
const refOvershoot = 1.05

// bruteForceLimit — до стольких обязательных маяков эталон ищется полным
// перебором порядка обхода; больше — детерминированной эвристикой (§6.3).
const bruteForceLimit = 8

// coordinateEpsilon — допуск проверки координат на [0,1] (погрешность float).
const coordinateEpsilon = 1e-9

// Причины невалидности пути (§6.6). Стабильные коды для хендлера.
const (
	reasonTooFewPoints      = "too_few_points"
	reasonTooManyPoints     = "too_many_points"
	reasonPointOutOfBounds  = "point_out_of_bounds"
	reasonStartNotCaptured  = "start_not_captured"
	reasonFinishNotCaptured = "finish_not_captured"
	reasonBeaconNotCaptured = "beacon_not_captured"
)

// EvaluatePath — серверная оценка полилинии (§6.3/§6.6). Валидный путь:
// 2..maxPathPoints точек, все в [0,1], начинается в радиусе СТАРТА, заканчивается
// в радиусе ФИНИША, захватывает все beacon (расстояние точки до полилинии ≤ r).
// quality = clamp01(L_ref / L_eff); невалидный путь → quality=0, reason≠"".
func EvaluatePath(field Field, path []Point) (quality float64, valid bool, reason string) {
	if len(path) < 2 {
		return 0, false, reasonTooFewPoints
	}
	if len(path) > maxPathPoints {
		return 0, false, reasonTooManyPoints
	}
	for _, p := range path {
		if !inUnitSquare(p) {
			return 0, false, reasonPointOutOfBounds
		}
	}
	if dist(path[0], field.Start) > endpointCaptureRadius {
		return 0, false, reasonStartNotCaptured
	}
	if dist(path[len(path)-1], field.Finish) > endpointCaptureRadius {
		return 0, false, reasonFinishNotCaptured
	}
	for _, n := range field.Nodes {
		if n.Type != beaconType {
			continue
		}
		if pointToPolylineDist(Point{X: n.X, Y: n.Y}, path) > n.R {
			return 0, false, reasonBeaconNotCaptured
		}
	}
	lEff := pathEffLen(path, field.Zones)
	if lEff <= 0 {
		return 0, true, "" // вырожденный путь нулевой длины — выигрыша нет
	}
	return clamp01(referenceLength(field) / lEff), true, ""
}

// inUnitSquare — точка лежит в [0,1]² с допуском coordinateEpsilon.
func inUnitSquare(p Point) bool {
	return p.X >= -coordinateEpsilon && p.X <= 1+coordinateEpsilon &&
		p.Y >= -coordinateEpsilon && p.Y <= 1+coordinateEpsilon
}

// pathEffLen — эффективная длина L_eff (§6.3): сумма длин сегментов; сегмент,
// пересекающий зону (расстояние центра зоны до сегмента ≤ R), умножается на её
// коэффициент. Пересечение нескольких зон — произведение коэффициентов.
func pathEffLen(path []Point, zones []FieldZone) float64 {
	total := 0.0
	for i := 0; i+1 < len(path); i++ {
		seg := dist(path[i], path[i+1])
		mult := 1.0
		for _, z := range zones {
			if pointSegDist(Point{X: z.X, Y: z.Y}, path[i], path[i+1]) <= z.R {
				mult *= z.Coefficient
			}
		}
		total += seg * mult
	}
	return total
}

// referenceLength — эталон L_ref (§6.3): оптимальный порядок обхода всех маяков
// со теми же штрафами зон, × refOvershoot. Детерминированно.
func referenceLength(field Field) float64 {
	beacons := make([]Point, 0, len(field.Nodes))
	for _, n := range field.Nodes {
		if n.Type == beaconType {
			beacons = append(beacons, Point{X: n.X, Y: n.Y})
		}
	}
	var opt float64
	if len(beacons) <= bruteForceLimit {
		opt = bestRouteCostBrute(field.Start, field.Finish, beacons, field.Zones)
	} else {
		opt = bestRouteCostGreedy(field.Start, field.Finish, beacons, field.Zones)
	}
	return refOvershoot * opt
}

// bestRouteCostBrute — полный перебор порядка обхода маяков (для малого k).
func bestRouteCostBrute(start, finish Point, beacons []Point, zones []FieldZone) float64 {
	used := make([]bool, len(beacons))
	route := make([]Point, 0, len(beacons)+2)
	route = append(route, start)
	best := math.Inf(1)
	var walk func()
	walk = func() {
		if len(route) == len(beacons)+1 {
			full := make([]Point, 0, len(route)+1)
			full = append(full, route...)
			full = append(full, finish)
			if c := pathEffLen(full, zones); c < best {
				best = c
			}
			return
		}
		for i := range beacons {
			if used[i] {
				continue
			}
			used[i] = true
			route = append(route, beacons[i])
			walk()
			route = route[:len(route)-1]
			used[i] = false
		}
	}
	walk()
	return best
}

// bestRouteCostGreedy — детерминированная эвристика «ближайший сосед» (для
// большого k; в v1 не достигается — маяков ≤ beaconMax).
func bestRouteCostGreedy(start, finish Point, beacons []Point, zones []FieldZone) float64 {
	used := make([]bool, len(beacons))
	route := make([]Point, 0, len(beacons)+2)
	route = append(route, start)
	cur := start
	for range beacons {
		bestI, bestD := -1, math.Inf(1)
		for i := range beacons {
			if used[i] {
				continue
			}
			if d := dist(cur, beacons[i]); d < bestD {
				bestD, bestI = d, i
			}
		}
		used[bestI] = true
		route = append(route, beacons[bestI])
		cur = beacons[bestI]
	}
	route = append(route, finish)
	return pathEffLen(route, zones)
}

// pointToPolylineDist — минимальное расстояние от точки до полилинии.
func pointToPolylineDist(p Point, path []Point) float64 {
	best := math.Inf(1)
	for i := 0; i+1 < len(path); i++ {
		if d := pointSegDist(p, path[i], path[i+1]); d < best {
			best = d
		}
	}
	return best
}

// pointSegDist — расстояние от точки до отрезка [a,b].
func pointSegDist(p, a, b Point) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		return dist(p, a)
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / len2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return dist(p, Point{X: a.X + t*dx, Y: a.Y + t*dy})
}

// dist — расстояние между точками.
func dist(a, b Point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

// clamp01 — зажимает значение в [0,1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
