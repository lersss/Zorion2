// internal/generator/ship/validate.go
// Проверка инвариантов генерации (спека §3.1–§3.2, §9): деталь и её акценты
// в своей зоне (И6), стык с корпусом гарантирован попаданием базы в область
// [65,130]×[79,121] (И6/И9), единство стиля (И9). checkPart вызывается
// генератором при каждой попытке (невалидная форма перегенерируется) и
// тестами на реальном выводе.
package ship

import (
	"fmt"
	"math"
)

// checkPart — все инварианты одной детали. Ошибка — какая инварианта
// нарушена (для диагностики перегенерации и тестов).
func checkPart(cfg *Config, cat string, g Geometry) error {
	cc := cfg.Categories[cat]
	for i, comp := range g.Components {
		if len(comp.Poly) < cfg.Style.SegmentsMin || len(comp.Poly) > cfg.Style.SegmentsMax {
			return fmt.Errorf("%s: компонент %d: сегментов %d (нужно %d–%d)",
				cat, i, len(comp.Poly), cfg.Style.SegmentsMin, cfg.Style.SegmentsMax)
		}
		if err := checkComponentZone(cat, i, comp.Poly, cc.Zone); err != nil {
			return err
		}
		// Заполненность ≥ 55% bbox (стиль §3.2 п.4): по компоненту — для крыльев
		// (две зеркальные плоскости далеко друг от друга, общий bbox дал бы
		// «скелет», хотя каждая плоскость цельная) — и по части в целом для
		// однокомпонентных деталей. См. коммент в partFillRatio.
		bx0, by0, bx1, by1 := comp.Poly.BBox()
		bboxArea := (bx1 - bx0) * (by1 - by0)
		if bboxArea > 0 && comp.Poly.Area()/bboxArea < cfg.Style.FillRatio {
			return fmt.Errorf("%s: компонент %d: заполненность %.2f < %.2f",
				cat, i, comp.Poly.Area()/bboxArea, cfg.Style.FillRatio)
		}
	}
	if len(g.Components) == 1 {
		if r := partFillRatio(g); r < cfg.Style.FillRatio {
			return fmt.Errorf("%s: заполненность части %.2f < %.2f", cat, r, cfg.Style.FillRatio)
		}
	}

	switch cat {
	case "hull":
		if err := checkHullReach(cfg, g); err != nil {
			return err
		}
	case "nose":
		if err := checkBase(cat, g, cc, true, "min x"); err != nil {
			return err
		}
	case "engine":
		if err := checkBase(cat, g, cc, false, "max x"); err != nil {
			return err
		}
	case "wings", "tail":
		if err := checkVerticalBase(cat, g, cc); err != nil {
			return err
		}
		// Размах (bbox x) и max x — габариты форм §3.1.
		polys := make([]Polygon, len(g.Components))
		for i, c := range g.Components {
			polys[i] = c.Poly
		}
		x0, _, x1, _ := unionBBox(polys)
		if span := x1 - x0; cc.Span != nil && (span < cc.Span.Min-0.5 || span > cc.Span.Max+0.5) {
			return fmt.Errorf("%s: размах %.0f вне [%.0f..%.0f]", cat, span, cc.Span.Min, cc.Span.Max)
		}
		if cat == "wings" && cc.MaxXPart != nil && x1 > *cc.MaxXPart+0.5 {
			return fmt.Errorf("wings: max x %.0f > %v (крылья не залезают на нос)", x1, *cc.MaxXPart)
		}
	}
	return checkAccents(cfg, cat, g)
}

// checkComponentZone — bbox компонента внутри зоны категории (спека §3.1).
// Крылья: компонент 0 — верхняя плоскость (зона y 0–80), компонент 1 —
// зеркальная нижняя (зона y 120–200, зеркало по y=100).
func checkComponentZone(cat string, idx int, poly Polygon, zone Zone) error {
	x0, y0, x1, y1 := poly.BBox()
	z := zone
	if cat == "wings" && idx == 1 {
		z = Zone{X: zone.X, Y: [2]float64{120, 200}}
	}
	if x0 < z.X[0]-0.5 || x1 > z.X[1]+0.5 || y0 < z.Y[0]-0.5 || y1 > z.Y[1]+0.5 {
		return fmt.Errorf("%s: bbox [%.0f..%.0f]×[%.0f..%.0f] вне зоны [%.0f..%.0f]×[%.0f..%.0f]",
			cat, x0, x1, y0, y1, z.X[0], z.X[1], z.Y[0], z.Y[1])
	}
	return nil
}

// checkHullReach — корпус покрывает гарантированную область [65,130]×[79,121]:
// min x ≤ 65, max x ≥ 130, высота 42–60 с центром y=100 (спека §3.1).
func checkHullReach(cfg *Config, g Geometry) error {
	poly := g.Components[0].Poly
	x0, y0, x1, y1 := poly.BBox()
	region := guaranteedHullRegion
	if x0 > region.X[0]+0.5 || x1 < region.X[1]-0.5 {
		return fmt.Errorf("hull: bbox x [%.0f..%.0f] не покрывает [%.0f..%.0f]",
			x0, x1, region.X[0], region.X[1])
	}
	if y0 > region.Y[0]+0.5 || y1 < region.Y[1]-0.5 {
		return fmt.Errorf("hull: bbox y [%.0f..%.0f] не покрывает [%.0f..%.0f]",
			y0, y1, region.Y[0], region.Y[1])
	}
	// Прямоугольник области целиком внутри полигона корпуса (углы + центр).
	for _, c := range []Pt{
		{X: region.X[0], Y: region.Y[0]},
		{X: region.X[1], Y: region.Y[0]},
		{X: region.X[0], Y: region.Y[1]},
		{X: region.X[1], Y: region.Y[1]},
		{X: (region.X[0] + region.X[1]) / 2, Y: (region.Y[0] + region.Y[1]) / 2},
	} {
		if !PointInPolygon(c, poly) {
			return fmt.Errorf("hull: точка (%.0f,%.0f) гарантированной области вне корпуса", c.X, c.Y)
		}
	}
	return nil
}

// checkBase — база придатка в гарантированной области корпуса (И6/И9).
// Нос: bbox min x ≤ tipMinX (130), база y-протяжённость 24–40, центр y=100.
// Двигатели: bbox max x ≥ baseMaxX (65), база 22–40, центр y=100.
func checkBase(cat string, g Geometry, cc Category, nose bool, axis string) error {
	comp := g.Components[0]
	if comp.Base == nil {
		return fmt.Errorf("%s: нет базовой кромки", cat)
	}
	x0, _, x1, _ := comp.Poly.BBox()
	if nose && cc.TipMinX != nil && x0 > *cc.TipMinX+0.5 {
		return fmt.Errorf("nose: bbox min x %.0f > %v (база не достаёт до корпуса)", x0, *cc.TipMinX)
	}
	if !nose && cc.BaseMaxX != nil && x1 < *cc.BaseMaxX-0.5 {
		return fmt.Errorf("engine: bbox max x %.0f < %v (база не на корпусе)", x1, *cc.BaseMaxX)
	}
	midY := (comp.Base.A.Y + comp.Base.B.Y) / 2
	if math.Abs(midY-100) > 1 {
		return fmt.Errorf("%s: база не центрирована на y=100 (центр %.0f)", cat, midY)
	}
	return baseInGuaranteedRegion(cat, *comp.Base)
}

// checkVerticalBase — крылья/хвост: база достигает y ≥ reachY (79) / y ≤ 121
// (зеркало) и центр базы по x в гарантированной области [65,130] (спека §3.1).
func checkVerticalBase(cat string, g Geometry, cc Category) error {
	if cc.ReachY == nil {
		return fmt.Errorf("%s: нет reachY в конфиге", cat)
	}
	thr := *cc.ReachY
	for i, comp := range g.Components {
		if comp.Base == nil {
			return fmt.Errorf("%s: нет базовой кромки у компонента %d", cat, i)
		}
		base := *comp.Base
		// Верхняя плоскость/киль: база снизу, достигает y ≥ 79.
		// Нижняя плоскость (зеркало): база сверху, достигает y ≤ 121.
		var ok bool
		if i == 0 {
			ok = base.A.Y >= thr-0.5 && base.B.Y >= thr-0.5
		} else {
			ok = base.A.Y <= 200-thr+0.5 && base.B.Y <= 200-thr+0.5
		}
		if !ok {
			return fmt.Errorf("%s: база компонента %d (y %.0f..%.0f) не достигает y ≥ %v / y ≤ %v",
				cat, i, base.A.Y, base.B.Y, thr, 200-thr)
		}
		mx := (base.A.X + base.B.X) / 2
		if mx < guaranteedHullRegion.X[0]-0.5 || mx > guaranteedHullRegion.X[1]+0.5 {
			return fmt.Errorf("%s: центр базы x %.0f вне [65,130]", cat, mx)
		}
		if cat == "tail" && i == 0 && cc.MaxX != nil {
			x0, _, x1, _ := comp.Poly.BBox()
			if x1 < *cc.MaxX-0.5 {
				return fmt.Errorf("tail: bbox max x %.0f < %v (киль не над кормой)", x1, *cc.MaxX)
			}
			_ = x0
		}
	}
	return baseInGuaranteedRegion(cat, *g.Components[0].Base)
}

// baseInGuaranteedRegion — базовая кромка пересекает гарантированную область
// корпуса [65,130]×[79,121] (спека §3.1, «база придатка попадает в область»).
func baseInGuaranteedRegion(cat string, base Segment) error {
	reg := guaranteedHullRegion
	// База горизонтальна (крылья/хвост) или вертикальна (нос/двигатели).
	if math.Abs(base.A.Y-base.B.Y) < math.Abs(base.A.X-base.B.X) {
		// Горизонтальная: y в [79,121], x-протяжённость пересекает [65,130].
		if base.A.Y < reg.Y[0]-0.5 || base.A.Y > reg.Y[1]+0.5 {
			return fmt.Errorf("%s: база y %.0f вне [79,121]", cat, base.A.Y)
		}
		if math.Max(base.A.X, base.B.X) < reg.X[0]-0.5 || math.Min(base.A.X, base.B.X) > reg.X[1]+0.5 {
			return fmt.Errorf("%s: база x [%.0f..%.0f] не пересекает [65,130]", cat, base.A.X, base.B.X)
		}
		return nil
	}
	// Вертикальная: x в [65,130], y-протяжённость пересекает [79,121].
	if base.A.X < reg.X[0]-0.5 || base.A.X > reg.X[1]+0.5 {
		return fmt.Errorf("%s: база x %.0f вне [65,130]", cat, base.A.X)
	}
	if math.Max(base.A.Y, base.B.Y) < reg.Y[0]-0.5 || math.Min(base.A.Y, base.B.Y) > reg.Y[1]+0.5 {
		return fmt.Errorf("%s: база y [%.0f..%.0f] не пересекает [79,121]", cat, base.A.Y, base.B.Y)
	}
	return nil
}

// partFillArea — суммарная площадь заливки основной массы детали.
func partFillArea(g Geometry) float64 {
	var sum float64
	for _, c := range g.Components {
		sum += c.Poly.Area()
	}
	return sum
}

// partFillRatio — заполненность части: суммарная площадь / площадь bbox.
// Для крыльев (две зеркальные плоскости) считается по компонентам в
// checkPart — общий bbox двух далеко разнесённых плоскостей занижал бы
// честную заполненность до «скелета» (спека §3.2 п.4 про запрет решётчатых
// форм, а не про разнесение зеркальных пар).
func partFillRatio(g Geometry) float64 {
	polys := make([]Polygon, len(g.Components))
	for i, c := range g.Components {
		polys[i] = c.Poly
	}
	x0, y0, x1, y1 := unionBBox(polys)
	bbox := (x1 - x0) * (y1 - y0)
	if bbox <= 0 {
		return 0
	}
	return partFillArea(g) / bbox
}

// checkAccents — акценты внутри силуэта, отступ ≥ accentOffset, лимиты
// площади: один ≤ min(maxAccentArea, 6% заливки), суммарно ≤ min(maxAccentTotal,
// 10% заливки) (спека §3.2 п.6, §9).
func checkAccents(cfg *Config, cat string, g Geometry) error {
	fill := partFillArea(g)
	// Лимит «6%/10% заливки» — от площади заливки детали, не от холста
	// (спека §9: минимальный корпус 60×42 остаётся «однотонным с акцентами»);
	// сверху — жёсткие пиксельные потолки 90/240 px².
	singleLimit := math.Min(cfg.Style.MaxAccentArea, 0.06*fill)
	totalLimit := math.Min(cfg.Style.MaxAccentTotal, 0.10*fill)
	var total float64
	for i, a := range g.Accents {
		area := accentArea(a)
		if area > singleLimit+0.5 {
			return fmt.Errorf("%s: акцент %d: площадь %.1f > лимита %.1f (6%% заливки / %v px²)",
				cat, i, area, singleLimit, cfg.Style.MaxAccentArea)
		}
		total += area
		if err := checkAccentPlacement(cfg, g, a); err != nil {
			return fmt.Errorf("%s: акцент %d: %w", cat, i, err)
		}
	}
	if total > totalLimit+0.5 {
		return fmt.Errorf("%s: суммарная площадь акцентов %.1f > лимита %.1f (10%% заливки / %v px²)",
			cat, total, totalLimit, cfg.Style.MaxAccentTotal)
	}
	return nil
}

// checkAccentPlacement — акцент строго внутри силуэта детали с отступом
// от контура ≥ accentOffset (стиль §3.2 п.6).
func checkAccentPlacement(cfg *Config, g Geometry, a Accent) error {
	switch a.Kind {
	case "circle":
		for _, c := range g.Components {
			if !PointInPolygon(a.Center, c.Poly) {
				continue
			}
			// Отступ от границы круга до контура ≥ offset: dist(центр) ≥ r + offset.
			if distToContour(a.Center, c.Poly) < a.Radius+cfg.Style.AccentOffset-0.5 {
				return fmt.Errorf("круг вне отступа (r=%.1f)", a.Radius)
			}
			return nil
		}
		return fmt.Errorf("круг вне силуэта")
	case "rect":
		r := a.Rect
		corners := []Pt{
			{X: r.X, Y: r.Y},
			{X: r.X + r.W, Y: r.Y},
			{X: r.X, Y: r.Y + r.H},
			{X: r.X + r.W, Y: r.Y + r.H},
		}
		for _, c := range g.Components {
			in := true
			for _, p := range corners {
				if !PointInPolygon(p, c.Poly) {
					in = false
					break
				}
			}
			if !in {
				continue
			}
			// Отступ: минимальное расстояние между рёбрами прямоугольника и
			// контуром ≥ offset.
			if rectToContour(r, c.Poly) < cfg.Style.AccentOffset-0.5 {
				return fmt.Errorf("прямоугольник вне отступа")
			}
			return nil
		}
		return fmt.Errorf("прямоугольник вне силуэта")
	}
	return fmt.Errorf("неизвестный вид акцента %q", a.Kind)
}

// accentArea — площадь акцента (заливка, без обводки).
func accentArea(a Accent) float64 {
	switch a.Kind {
	case "circle":
		return math.Pi * a.Radius * a.Radius
	case "rect":
		return a.Rect.W * a.Rect.H
	}
	return 0
}

// rectToContour — минимальное расстояние между рёбрами прямоугольника и
// контуром полигона.
func rectToContour(r Rect, poly Polygon) float64 {
	edges := [][2]Pt{
		{{X: r.X, Y: r.Y}, {X: r.X + r.W, Y: r.Y}},
		{{X: r.X + r.W, Y: r.Y}, {X: r.X + r.W, Y: r.Y + r.H}},
		{{X: r.X + r.W, Y: r.Y + r.H}, {X: r.X, Y: r.Y + r.H}},
		{{X: r.X, Y: r.Y + r.H}, {X: r.X, Y: r.Y}},
	}
	min := math.Inf(1)
	for _, e := range edges {
		for i := 0; i < len(poly); i++ {
			j := (i + 1) % len(poly)
			d := distSegSeg(e[0], e[1], poly[i], poly[j])
			if d < min {
				min = d
			}
		}
	}
	return min
}

// distSegSeg — расстояние между двумя отрезками (0, если пересекаются).
func distSegSeg(p1, p2, p3, p4 Pt) float64 {
	if segsIntersect(p1, p2, p3, p4) {
		return 0
	}
	return math.Min(math.Min(distToSegment(p1, p3, p4), distToSegment(p2, p3, p4)),
		math.Min(distToSegment(p3, p1, p2), distToSegment(p4, p1, p2)))
}

// segsIntersect — пересечение отрезков (включая касание).
func segsIntersect(p1, p2, p3, p4 Pt) bool {
	d1 := cross(p3, p4, p1)
	d2 := cross(p3, p4, p2)
	d3 := cross(p1, p2, p3)
	d4 := cross(p1, p2, p4)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	// Касания (коллинеарные точки на отрезке) — тоже пересечение.
	return (d1 == 0 && onSegment(p3, p4, p1)) ||
		(d2 == 0 && onSegment(p3, p4, p2)) ||
		(d3 == 0 && onSegment(p1, p2, p3)) ||
		(d4 == 0 && onSegment(p1, p2, p4))
}

func cross(a, b, c Pt) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

func onSegment(a, b, p Pt) bool {
	return math.Min(a.X, b.X) <= p.X+0.5 && p.X <= math.Max(a.X, b.X)+0.5 &&
		math.Min(a.Y, b.Y) <= p.Y+0.5 && p.Y <= math.Max(a.Y, b.Y)+0.5
}