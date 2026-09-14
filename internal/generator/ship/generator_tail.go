// internal/generator/ship/generator_tail.go
// Шаблон «Хвост (киль)» (спека §3.1): зона x 25–110, y 0–85; ширина базы
// 18–40, высота 15–55, размах (x) 20–60, max x ≥ 65 (сидит над кормой),
// база достигает y ≥ 79 — лежит на корпусе. Без акцентов (спека §3.3).
package ship

import (
	"math"
	"math/rand"
)

// genTail — кормовой киль над палубой кормы: база (нижняя кромка) лежит на
// корпусе (y ∈ [79,85], центр базы x ∈ [65,130]), вершина — со сдвигом
// (стреловидность назад/вперёд) и сужением. Размах и заполненность
// проверяет checkPart (пере-ролл при промахе).
func genTail(cfg *Config, rng *rand.Rand) (Geometry, map[string]interface{}) {
	st := cfg.Style
	cc := cfg.Categories["tail"]

	bw := math.Round(randInRange(rng, *cc.Base))
	height := math.Round(randInRange(rng, *cc.Height))
	baseY := math.Round(randInRange(rng, Range{Min: 79, Max: 85}))
	tw := math.Round(randInRange(rng, Range{Min: math.Max(2, bw*0.4), Max: bw}))
	tipOff := math.Round(randInRange(rng, Range{Min: -0.5 * bw, Max: 0.5 * bw}))

	// bx: база в зоне [25,110], центр базы в [65,130], bbox ⊆ зона.
	extentRight := math.Max(bw, tipOff+tw) - math.Min(0, tipOff)
	lo := math.Max(25-math.Min(0, tipOff), 65-bw/2)
	hi := math.Min(130-bw/2, 110-extentRight)
	bx := math.Round(randInRange(rng, Range{Min: lo, Max: hi}))

	topY := baseY - height
	poly := Polygon{
		{X: bx, Y: baseY},
		{X: bx + bw, Y: baseY},
		{X: bx + tipOff + tw, Y: topY},
		{X: bx + tipOff, Y: topY},
	}
	// База — нижняя кромка (стык с палубой кормы): её углы не фасуются.
	baseSeg := Segment{A: Pt{X: bx, Y: baseY}, B: Pt{X: bx + bw, Y: baseY}}
	poly = chamferSharp(poly, st.CornerRadius, 100*math.Pi/180, st.SegmentsMax, baseSeg)
	poly = poly.Round()

	params := map[string]interface{}{
		"bw": bw, "ht": height, "by": baseY, "tw": tw, "to": tipOff, "bx": bx,
	}
	return Geometry{Components: []Component{{Poly: poly, Base: &baseSeg}}}, params
}