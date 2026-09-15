// internal/economy/settlement/balancer_curve_test.go
// Тесты математики сегментной кривой (спека 99.2.17 §2/§9):
// bendTransform (прямая/выпуклая/вогнутая/пределы), непрерывность на узлах,
// монотонность и границы внутри сегмента, горизонтальная экстраполяция.
package settlement

import (
	"math"
	"testing"
)

// TestBendLinear — k = 0: t_k = t; k = 1e-12 — тоже ≈ t (устойчивость порога 1e-10).
func TestBendLinear(t *testing.T) {
	for _, tt := range []float64{0, 0.25, 0.5, 0.75, 1} {
		if got := bendTransform(tt, 0); math.Abs(got-tt) > 1e-10 {
			t.Errorf("bendTransform(%v, 0) = %v, хочу %v", tt, got, tt)
		}
		if got := bendTransform(tt, 1e-12); math.Abs(got-tt) > 1e-10 {
			t.Errorf("bendTransform(%v, 1e-12) = %v, хочу %v (устойчивость)", tt, got, tt)
		}
	}
}

// TestBendConvex — k > 0: t_k < t внутри (0,1) — кривая «прилипает» к началу
// сегмента, подъём ускоряется к концу (выпуклая). Пример спеки §2/§9:
// k=2, t=0.5 → t_k = (e−1)/(e²−1) ≈ 0.269.
func TestBendConvex(t *testing.T) {
	if got := bendTransform(0.5, 2); math.Abs(got-0.269) > 1e-3 {
		t.Errorf("bendTransform(0.5, 2) = %v, хочу ≈0.269", got)
	}
	for _, tt := range []float64{0.25, 0.5, 0.75} {
		if got := bendTransform(tt, 2); !(got < tt) {
			t.Errorf("k=2: t_k(%v) = %v, хочу < t (выпуклая, ниже диагонали)", tt, got)
		}
	}
}

// TestBendConcave — k < 0: t_k > t внутри (0,1) — кривая «прилипает» к концу
// сегмента, рост замедляется (вогнутая). Пример: k=−2, t=0.5 → ≈0.731.
func TestBendConcave(t *testing.T) {
	if got := bendTransform(0.5, -2); math.Abs(got-0.731) > 1e-3 {
		t.Errorf("bendTransform(0.5, -2) = %v, хочу ≈0.731", got)
	}
	for _, tt := range []float64{0.25, 0.5, 0.75} {
		if got := bendTransform(tt, -2); !(got > tt) {
			t.Errorf("k=−2: t_k(%v) = %v, хочу > t (вогнутая, выше диагонали)", tt, got)
		}
	}
}

// TestBendLimits — |k| ≤ 10; t_k(0.5) при k=±10 в ожидаемых пределах
// (§2 «Ограничение изгиба»): k=10 → ≈0.0067, k=−10 → ≈0.9933.
func TestBendLimits(t *testing.T) {
	got10 := bendTransform(0.5, 10)
	if math.Abs(got10-0.0067) > 1e-3 {
		t.Errorf("bendTransform(0.5, 10) = %v, хочу ≈0.0067", got10)
	}
	gotM10 := bendTransform(0.5, -10)
	if math.Abs(gotM10-0.9933) > 1e-3 {
		t.Errorf("bendTransform(0.5, -10) = %v, хочу ≈0.9933", gotM10)
	}
	// Диапазон |k| ≤ 10 — валидация store (TestValidation422); здесь —
	// форма при крайних k не вырождается в ступеньку за пределами.
	if !(got10 > 0) || !(gotM10 < 1) {
		t.Errorf("крайние k вырождают форму: %v / %v", got10, gotM10)
	}
}

// curveFixture — простая кривая для тестов сегментов: 3 узла, 2 сегмента.
func curveFixture(bend0, bend1 float64) ([]SegmentNode, []float64) {
	nodes := []SegmentNode{{X: 0, Y: 0.1}, {X: 1, Y: 0.5}, {X: 2, Y: 0.9}}
	return nodes, []float64{bend0, bend1}
}

// TestSegmentContinuity — на узлах x = x_i значение = y_i; на стыках сегментов
// нет скачка (значение непрерывно, наклон может изламываться — допускается).
func TestSegmentContinuity(t *testing.T) {
	for _, comp := range []string{"heat", "cold", "gravity", "radiation"} {
		nodes, bends, ok := GetCurve(comp)
		if !ok {
			t.Fatalf("компонента %q недоступна", comp)
		}
		for i, n := range nodes {
			if got := evaluateCurve(nodes, bends, n.X); math.Abs(got-n.Y) > 1e-12 {
				t.Errorf("%s: узел %d (x=%v): кривая = %v, хочу %v", comp, i, n.X, got, n.Y)
			}
		}
	}
	// Произвольные bends — непрерывность тоже (свойство t_k на t=0/1).
	nodes, bends := curveFixture(3, -3)
	for i, n := range nodes {
		if got := evaluateCurve(nodes, bends, n.X); math.Abs(got-n.Y) > 1e-12 {
			t.Errorf("fixture: узел %d (x=%v): кривая = %v, хочу %v", i, n.X, got, n.Y)
		}
	}
}

// TestSegmentMonotonicBounds — y(x) никогда не выходит за [y_i, y_{i+1}]
// внутри сегмента (дискретная сетка t ∈ [0,1] с шагом 0.01) — ключевое
// свойство для гварда R < 1 (§2).
func TestSegmentMonotonicBounds(t *testing.T) {
	nodes, bends := curveFixture(5, -5)
	for i := 0; i < len(nodes)-1; i++ {
		lo, hi := nodes[i].Y, nodes[i+1].Y
		if lo > hi {
			lo, hi = hi, lo
		}
		for tt := 0.0; tt <= 1; tt += 0.01 {
			x := nodes[i].X + tt*(nodes[i+1].X-nodes[i].X)
			got := evaluateCurve(nodes, bends, x)
			if got < lo-1e-12 || got > hi+1e-12 {
				t.Errorf("сегмент %d, t=%v: y=%v вне [%v, %v]", i, tt, got, lo, hi)
			}
		}
	}
	// То же на дефолтных кривых (все сегменты).
	for _, comp := range []string{"heat", "cold", "gravity", "radiation"} {
		cn, cb, ok := GetCurve(comp)
		if !ok {
			t.Fatalf("компонента %q недоступна", comp)
		}
		for i := 0; i < len(cn)-1; i++ {
			lo, hi := cn[i].Y, cn[i+1].Y
			if lo > hi {
				lo, hi = hi, lo
			}
			for tt := 0.0; tt <= 1; tt += 0.01 {
				x := cn[i].X + tt*(cn[i+1].X-cn[i].X)
				got := evaluateCurve(cn, cb, x)
				if got < lo-1e-12 || got > hi+1e-12 {
					t.Errorf("%s: сегмент %d, t=%v: y=%v вне [%v, %v]", comp, i, tt, got, lo, hi)
				}
			}
		}
	}
}

// TestExtrapolation — x < x₀ → y₀; x > xₙ → yₙ (горизонтальная, §2).
func TestExtrapolation(t *testing.T) {
	for _, comp := range []string{"heat", "cold", "gravity", "radiation"} {
		nodes, bends, ok := GetCurve(comp)
		if !ok {
			t.Fatalf("компонента %q недоступна", comp)
		}
		first, last := nodes[0], nodes[len(nodes)-1]
		if got := evaluateCurve(nodes, bends, first.X-1000); got != first.Y {
			t.Errorf("%s: слева от x₀: %v, хочу %v (константа)", comp, got, first.Y)
		}
		if got := evaluateCurve(nodes, bends, last.X+1000); got != last.Y {
			t.Errorf("%s: справа от xₙ: %v, хочу %v (константа)", comp, got, last.Y)
		}
	}
	nodes, bends := curveFixture(2, 2)
	if got := evaluateCurve(nodes, bends, -5); got != nodes[0].Y {
		t.Errorf("fixture: слева: %v, хочу %v", got, nodes[0].Y)
	}
	if got := evaluateCurve(nodes, bends, 7); got != nodes[2].Y {
		t.Errorf("fixture: справа: %v, хочу %v", got, nodes[2].Y)
	}
}
