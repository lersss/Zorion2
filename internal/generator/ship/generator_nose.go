// internal/generator/ship/generator_nose.go
// Шаблон «Нос» (спека §3.1): зона x 125–200, база 24–40 с центром y=100,
// длина 55–70, bbox max x ∈ [185,197], база (min x) ≤ 130 — попадает в
// гарантированную область корпуса. Обязательный акцент — кабина (стекло).
// Два варианта силуэта: клин (прямое сужение) и ступенчатый (полновысотная
// секция у базы + сужение) — «10 различимых форм» (спека §3.3).
package ship

import (
	"math"
	"math/rand"
)

// genNose — нос: база (стык с корпусом, x = baseX ≤ 130) к острию
// (max x ∈ [185,197]). Наклонные кромки — из семейства углов 20/25/30°
// (стиль §3.2 п.7), где возможен; иначе — параллельный нос.
func genNose(cfg *Config, rng *rand.Rand) (Geometry, map[string]interface{}) {
	st := cfg.Style
	cc := cfg.Categories["nose"]

	base := math.Round(randInRange(rng, *cc.Base))
	length := math.Round(randInRange(rng, *cc.Length))
	// baseX: база на корпусе (≤ 130) в зоне [125,200], tip = baseX + length ∈ [185,197].
	baseX := math.Round(randInRange(rng, Range{
		Min: math.Max(125, cc.TipMaxX.Min-length),
		Max: math.Min(130, cc.TipMaxX.Max-length),
	}))
	tipX := baseX + length

	// Угол сужения из семейства {20,25,30}° (круче — если влезает с tipHalf ≥ 2).
	tipHalf := 2.0
	for i := len(st.Angles) - 1; i >= 0; i-- {
		th := st.Angles[i] * math.Pi / 180
		if half := base/2 - math.Tan(th)*length; half >= 2 {
			tipHalf = math.Round(half)
			break
		}
	}

	// Вариант силуэта: 0 — клин (прямое сужение), 1 — ступенчатый (секция
	// полной высоты у базы, затем сужение). Разные очертания — разные формы.
	baseSeg := Segment{A: Pt{X: baseX, Y: 100 - base/2}, B: Pt{X: baseX, Y: 100 + base/2}}
	var poly Polygon
	kinkX := baseX
	stepped := rng.Float64() < 0.5
	if stepped {
		kinkX = math.Round(baseX + randInRange(rng, Range{Min: 0.45, Max: 0.7})*length)
		poly = Polygon{
			{X: baseX, Y: 100 - base/2},
			{X: kinkX, Y: 100 - base/2},
			{X: tipX, Y: 100 - tipHalf},
			{X: tipX, Y: 100 + tipHalf},
			{X: kinkX, Y: 100 + base/2},
			{X: baseX, Y: 100 + base/2},
		}
	} else {
		poly = Polygon{
			{X: baseX, Y: 100 - base/2},
			{X: tipX, Y: 100 - tipHalf},
			{X: tipX, Y: 100 + tipHalf},
			{X: baseX, Y: 100 + base/2},
		}
	}
	// База — левая кромка (стык): её углы не фасуются (стиль §3.2 п.5).
	poly = chamferSharp(poly, st.CornerRadius, 100*math.Pi/180, st.SegmentsMax, baseSeg)
	poly = poly.Round()

	// Кабина (стекло) — обязательный акцент носа (спека §3.3): убирается
	// от острия так, чтобы влезать с отступом ≥ 3 px (проверяет checkPart).
	glass := cfg.Accents.Glass
	fill := poly.Area()
	cabinW := math.Round(randInRange(rng, Range{Min: 8, Max: 12}))
	cabinH := clamp(0.9*math.Min(st.MaxAccentArea, 0.06*fill)/cabinW, 3, 5)

	var cabin Accent
	if stepped {
		// В полновысотной секции кабина влезает с запасом: центр ближе к базе.
		cx := math.Round(clamp(randInRange(rng, Range{Min: baseX + 14, Max: kinkX - 8}),
			baseX+14, kinkX-8))
		cabin = Accent{
			Kind:  "rect",
			Color: glass,
			Rect: Rect{
				X: math.Round(cx - cabinW/2),
				Y: math.Round(100 - cabinH/2),
				W: cabinW,
				H: cabinH,
			},
		}
	} else {
		// Расстояние от острия: половина высоты носа в точке cx должна быть
		// ≥ cabinH/2 + offset + запас.
		d := 12.0
		if need := cabinH/2 + st.AccentOffset + 1; need > 0 {
			if denom := base/2 - tipHalf; denom > 0 {
				d = math.Max(d, (need-tipHalf)*length/denom)
			}
		}
		cx := math.Round(tipX - d)
		cabin = Accent{
			Kind:  "rect",
			Color: glass,
			Rect: Rect{
				X: math.Round(cx - cabinW/2),
				Y: math.Round(100 - cabinH/2),
				W: cabinW,
				H: cabinH,
			},
		}
	}

	v := 0.0
	if stepped {
		v = 1
	}
	params := map[string]interface{}{
		"b": base, "l": length, "bx": baseX, "th": tipHalf, "v": v,
		"cw": cabin.Rect.W, "ch": cabin.Rect.H, "cx": cabin.Rect.X, "cy": cabin.Rect.Y,
	}
	return Geometry{Components: []Component{{Poly: poly, Base: &baseSeg}}, Accents: []Accent{cabin}}, params
}