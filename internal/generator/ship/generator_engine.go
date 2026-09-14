// internal/generator/ship/generator_engine.go
// Шаблон «Двигатели» (спека §3.1): зона x 0–70, y 75–125; база 22–40 с
// центром y=100 у x=65 (bbox max x ≥ 65 — база на корпусе), длина 30–60,
// левый край ≥ 3 px (запас под обводку). Акценты — тёмное сопло + свечение
// (обязательно, спека §3.3).
package ship

import (
	"math"
	"math/rand"
)

// genEngine — гондола двигателя: от базы (стык с кормой корпуса, x=65) к
// соплу (x = 65 - length) с лёгким сужением. Сопло — тёмный прямоугольник
// у выходного торца, свечение — круг у сопла (явный fill, не currentColor).
func genEngine(cfg *Config, rng *rand.Rand) (Geometry, map[string]interface{}) {
	st := cfg.Style
	cc := cfg.Categories["engine"]

	base := math.Round(randInRange(rng, *cc.Base))
	length := math.Round(randInRange(rng, *cc.Length))
	left := 65 - length
	// Сужение к соплу — лёгкое: exhaustHalf ∈ [base/2-5, base/2-1.5]
	// (заполненность ≥ 55% держится при любом варианте).
	eh := math.Round(randInRange(rng, Range{
		Min: math.Max(2, base/2-5),
		Max: base/2 - 1.5,
	}))

	poly := Polygon{
		{X: 65, Y: 100 - base/2},
		{X: 65, Y: 100 + base/2},
		{X: left, Y: 100 + eh},
		{X: left, Y: 100 - eh},
	}
	// База — правая кромка (стык с кормой): её углы не фасуются.
	baseSeg := Segment{A: Pt{X: 65, Y: 100 - base/2}, B: Pt{X: 65, Y: 100 + base/2}}
	poly = chamferSharp(poly, st.CornerRadius, 100*math.Pi/180, st.SegmentsMax, baseSeg)
	poly = poly.Round()

	// Акценты: сопло (тёмное) + свечение (обязательно, спека §3.3/§9).
	// Гондола сужается к соплу: отступ от торца выбирается так, чтобы в точке
	// размещения половина высоты гондолы ≥ (высота акцента)/2 + offset + запас.
	fill := poly.Area()
	nozzleArea := 0.5 * math.Min(st.MaxAccentArea, 0.06*fill)
	nozzleLen := math.Round(randInRange(rng, Range{Min: 4, Max: 6}))
	nh := clamp(nozzleArea/(2*nozzleLen), 1.5, 3.5)
	nozzleD := st.AccentOffset + 0.5
	if base/2 > eh {
		nozzleD = math.Max(nozzleD, (nh+st.AccentOffset+1-eh)*length/(base/2-eh))
	}
	nozzle := Accent{
		Kind:  "rect",
		Color: cfg.Accents.Nozzle,
		Rect: Rect{
			X: math.Round(left + nozzleD),
			Y: math.Round(100 - nh),
			W: nozzleLen,
			H: math.Round(2 * nh),
		},
	}
	glowR := clamp(math.Sqrt(0.03*fill/math.Pi), 1.5, 2.5)
	glowD := glowR + st.AccentOffset + 0.5
	if base/2 > eh {
		glowD = math.Max(glowD, (glowR+st.AccentOffset+1-eh)*length/(base/2-eh))
	}
	glow := Accent{
		Kind:   "circle",
		Color:  cfg.Accents.Glow,
		Center: Pt{X: math.Round(left + glowD), Y: 100},
		Radius: math.Round(glowR*10) / 10,
	}

	params := map[string]interface{}{
		"b": base, "l": length, "eh": eh,
		"nl": nozzle.Rect.W, "nh": nozzle.Rect.H,
		"gr": glow.Radius,
	}
	return Geometry{
		Components: []Component{{Poly: poly, Base: &baseSeg}},
		Accents:    []Accent{nozzle, glow},
	}, params
}