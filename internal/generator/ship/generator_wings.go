// internal/generator/ship/generator_wings.go
// Шаблон «Крылья» (спека §3.1): верхняя плоскость в зоне x 55–145, y 0–80
// (база достигает y ≥ 79 — лежит на корпусе, центр базы x ∈ [65,130]) +
// зеркальная нижняя (y ≤ 121). Акценты — парные навигационные огни:
// красный (левый борт = верхняя плоскость), зелёный (правый = нижняя).
package ship

import (
	"math"
	"math/rand"
)

// genWings — пара зеркальных плоскостей-крыльев. Хорда у борта 6–38
// (эффективно ≥ 11: с размахом ≥ 20 и заполненностью ≥ 55% простая трапеция
// короче не бывает), вылет 15–50, размах (x) 20–65, max x ≤ 150.
func genWings(cfg *Config, rng *rand.Rand) (Geometry, map[string]interface{}) {
	st := cfg.Style
	cc := cfg.Categories["wings"]

	chord := math.Round(randInRange(rng, Range{Min: math.Max(cc.Chord.Min, 11), Max: cc.Chord.Max}))
	reach := math.Round(randInRange(rng, *cc.Reach))
	by := math.Round(randInRange(rng, Range{Min: 79, Max: 80}))
	// Центр базы x ∈ [65,130] (база на корпусе при любой форме корпуса).
	bx := math.Round(randInRange(rng, Range{Min: math.Max(55, 65-chord/2), Max: 130 - chord/2}))
	tchord := math.Round(randInRange(rng, Range{Min: math.Max(2, chord*0.5), Max: chord}))

	// Размах (bbox x): bbox = max(chord, sweep+tchord) — требуем bbox = span,
	// поэтому span ≥ chord. span ∈ [20,65]; заполненность (chord+tchord)/(2·span)
	// ≥ 55% → span ≤ (chord+tchord)/1.1; max x = bx + span ≤ 145 (зона).
	spanHi := math.Min(65, math.Min(145-bx, (chord+tchord)/1.1))
	spanLo := math.Max(20, math.Max(chord, tchord))
	span := math.Round(randInRange(rng, Range{Min: spanLo, Max: spanHi}))
	sweep := span - tchord
	tx := bx + sweep
	ty := by - reach

	upper := Polygon{
		{X: bx, Y: by},
		{X: bx + chord, Y: by},
		{X: tx + tchord, Y: ty},
		{X: tx, Y: ty},
	}
	baseSeg := Segment{A: Pt{X: bx, Y: by}, B: Pt{X: bx + chord, Y: by}}
	upper = chamferSharp(upper, st.CornerRadius, 100*math.Pi/180, st.SegmentsMax, baseSeg)
	upper = upper.Round()

	lower := mirrorY(upper)
	lowerBase := Segment{
		A: Pt{X: bx, Y: 200 - by},
		B: Pt{X: bx + chord, Y: 200 - by},
	}

	// Парные огни на законцовках: красный — верхняя плоскость, зелёный —
	// нижняя (спека §3.3, как в legacy-спрайтах). Размер — от площади
	// заливки детали (пара плоскостей), лимит §9.
	fill := upper.Area() + lower.Area()
	r := clamp(math.Sqrt(0.04*fill/math.Pi), 1.8, 2.6)
	red := Accent{
		Kind:   "circle",
		Color:  cfg.Accents.FireRed,
		Center: Pt{X: math.Round(tx + tchord/2), Y: math.Round(ty + r + 3.5)},
		Radius: math.Round(r*10) / 10,
	}
	green := Accent{
		Kind:   "circle",
		Color:  cfg.Accents.FireGreen,
		Center: Pt{X: red.Center.X, Y: math.Round(200 - red.Center.Y)},
		Radius: red.Radius,
	}

	params := map[string]interface{}{
		"c": chord, "r": reach, "by": by, "bx": bx, "tc": tchord, "sp": span,
		"lr": red.Radius,
	}
	return Geometry{
		Components: []Component{
			{Poly: upper, Base: &baseSeg},
			{Poly: lower, Base: &lowerBase},
		},
		Accents: []Accent{red, green},
	}, params
}