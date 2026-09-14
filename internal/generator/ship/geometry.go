// internal/generator/ship/geometry.go
// Геометрические примитивы генератора форм кораблей: полигоны основной
// массы, фаски вместо острых углов (стиль §3.2: скругления 6/3 px —
// прямые фаски, без нулевых «инопланетных» углов), площадь/bbox/принадлежность.
package ship

import (
	"math"
	"strconv"
	"strings"
)

// Pt — точка холста 200×200 (нос вправо, ось y вниз, корпус центрирован
// на y=100, спека §3.1).
type Pt struct {
	X, Y float64
}

// Polygon — замкнутый контур (последняя точка соединяется с первой).
type Polygon []Pt

// Segment — отрезок (прямая базовая кромка придатка, спека §3.2 п.5).
type Segment struct{ A, B Pt }

// Rect — прямоугольник (акцентный элемент с rx=accentRadius).
type Rect struct{ X, Y, W, H float64 }

// BBox — ограничивающий прямоугольник полигона.
func (p Polygon) BBox() (minX, minY, maxX, maxY float64) {
	if len(p) == 0 {
		return 0, 0, 0, 0
	}
	minX, minY = p[0].X, p[0].Y
	maxX, maxY = p[0].X, p[0].Y
	for _, pt := range p[1:] {
		if pt.X < minX {
			minX = pt.X
		}
		if pt.X > maxX {
			maxX = pt.X
		}
		if pt.Y < minY {
			minY = pt.Y
		}
		if pt.Y > maxY {
			maxY = pt.Y
		}
	}
	return minX, minY, maxX, maxY
}

// Area — площадь полигона (шнуровка; абсолютное значение).
func (p Polygon) Area() float64 {
	if len(p) < 3 {
		return 0
	}
	var sum float64
	for i := 0; i < len(p); i++ {
		j := (i + 1) % len(p)
		sum += p[i].X*p[j].Y - p[j].X*p[i].Y
	}
	return math.Abs(sum) / 2
}

// Round — округление всех точек до целых (детерминизм SVG, аккуратные числа).
func (p Polygon) Round() Polygon {
	out := make(Polygon, len(p))
	for i, pt := range p {
		out[i] = Pt{X: math.Round(pt.X), Y: math.Round(pt.Y)}
	}
	return out
}

// interiorAngle — внутренний угол полигона в точке i (радианы).
func (p Polygon) interiorAngle(i int) float64 {
	n := len(p)
	prev := p[(i+n-1)%n]
	cur := p[i]
	next := p[(i+1)%n]
	v1 := Pt{X: prev.X - cur.X, Y: prev.Y - cur.Y}
	v2 := Pt{X: next.X - cur.X, Y: next.Y - cur.Y}
	dot := v1.X*v2.X + v1.Y*v2.Y
	l1 := math.Hypot(v1.X, v1.Y)
	l2 := math.Hypot(v2.X, v2.Y)
	if l1 == 0 || l2 == 0 {
		return math.Pi
	}
	cos := dot / (l1 * l2)
	if cos > 1 {
		cos = 1
	}
	if cos < -1 {
		cos = -1
	}
	return math.Acos(cos)
}

// chamferAt — фаска в точке i: точка заменяется двумя точками на рёбрах на
// расстоянии d от исходной вершины (если рёбра не короче фаски).
func (p Polygon) chamferAt(d float64, i int) Polygon {
	n := len(p)
	pt := p[i]
	prev := p[(i+n-1)%n]
	next := p[(i+1)%n]
	l1 := math.Hypot(prev.X-pt.X, prev.Y-pt.Y)
	l2 := math.Hypot(next.X-pt.X, next.Y-pt.Y)
	out := make(Polygon, 0, n+1)
	for k := 0; k < n; k++ {
		if k == i {
			if l1 > d && l2 > d {
				p1 := Pt{X: pt.X + (prev.X-pt.X)*d/l1, Y: pt.Y + (prev.Y-pt.Y)*d/l1}
				p2 := Pt{X: pt.X + (next.X-pt.X)*d/l2, Y: pt.Y + (next.Y-pt.Y)*d/l2}
				out = append(out, p1, p2)
			} else {
				out = append(out, pt)
			}
			continue
		}
		out = append(out, p[k])
	}
	return out
}

// chamferSharp — фасует острые углы (внутренний угол < thresholdRad) с
// бюджетом сегментов: по одному самому острому за итерацию, пока контур
// не достигнет maxSegments (стиль §3.2 п.1–3: никаких игл; контур основной
// массы 4–8 сегментов). Точки базовой кромки (base) не фасуются — у
// придатка ровно 1 прямая база (стиль §3.2 п.5). Детерминизм: перебор
// индексов в порядке возрастания, одинаковые углы не переставляются.
func chamferSharp(p Polygon, d, thresholdRad float64, maxSegments int, base Segment) Polygon {
	isBasePt := func(pt Pt) bool {
		return (pt.X == base.A.X && pt.Y == base.A.Y) ||
			(pt.X == base.B.X && pt.Y == base.B.Y)
	}
	for len(p) < maxSegments {
		best, bestAngle := -1, thresholdRad
		for i, pt := range p {
			if isBasePt(pt) {
				continue
			}
			if a := p.interiorAngle(i); a < bestAngle {
				best, bestAngle = i, a
			}
		}
		if best < 0 {
			break
		}
		next := p.chamferAt(d, best)
		if len(next) == len(p) {
			// Рёбра короче фаски — дальше зафасить нечем (защита от цикла).
			break
		}
		p = next
	}
	return p
}

// PointInPolygon — принадлежность точки полигону (ray casting).
func PointInPolygon(pt Pt, poly Polygon) bool {
	if len(poly) < 3 {
		return false
	}
	inside := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		xi, yi := poly[i].X, poly[i].Y
		xj, yj := poly[j].X, poly[j].Y
		if ((yi > pt.Y) != (yj > pt.Y)) &&
			(pt.X < (xj-xi)*(pt.Y-yi)/(yj-yi)+xi) {
			inside = !inside
		}
	}
	return inside
}

// distToSegment — расстояние от точки до отрезка.
func distToSegment(pt Pt, a, b Pt) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(pt.X-a.X, pt.Y-a.Y)
	}
	t := ((pt.X-a.X)*dx + (pt.Y-a.Y)*dy) / l2
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return math.Hypot(pt.X-(a.X+t*dx), pt.Y-(a.Y+t*dy))
}

// distToContour — минимальное расстояние от точки до контура полигона
// (для проверки отступа акцента ≥ accentOffset, стиль §3.2 п.6).
func distToContour(pt Pt, poly Polygon) float64 {
	if len(poly) < 2 {
		return 0
	}
	min := math.Inf(1)
	for i := 0; i < len(poly); i++ {
		j := (i + 1) % len(poly)
		d := distToSegment(pt, poly[i], poly[j])
		if d < min {
			min = d
		}
	}
	return min
}

// pathD — строка пути SVG из полигона: "M x y L ... Z". Координаты — целые.
func pathD(p Polygon) string {
	if len(p) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("M" + num(p[0].X) + " " + num(p[0].Y))
	for _, pt := range p[1:] {
		sb.WriteString("L" + num(pt.X) + " " + num(pt.Y))
	}
	sb.WriteString("Z")
	return sb.String()
}

// num — целое число как строка (без хвостов ".0").
func num(f float64) string {
	return strconv.FormatFloat(f, 'f', 0, 64)
}

// unionBBox — объединённый bbox нескольких полигонов (для глобального
// инварианта собранного корабля, спека §9: ширина/высота ∈ [0.8, 3.5]).
func unionBBox(polys []Polygon) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, p := range polys {
		x0, y0, x1, y1 := p.BBox()
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}
	return minX, minY, maxX, maxY
}