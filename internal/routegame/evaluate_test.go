package routegame

import (
	"math"
	"testing"
)

// evalField — поле с одним маяком вне прямой СТАРТ→ФИНИШ.
func evalField() Field {
	return Field{
		Seed:   1,
		Start:  Point{X: 0.1, Y: 0.5},
		Finish: Point{X: 0.9, Y: 0.5},
		Nodes:  []FieldNode{{Type: beaconType, X: 0.5, Y: 0.2, R: captureRadius}},
		Zones:  []FieldZone{},
	}
}

// referencePath — путь СТАРТ → все маяки (в порядке поля) → ФИНИШ.
func referencePath(f Field) []Point {
	pts := []Point{f.Start}
	for _, n := range f.Nodes {
		if n.Type == beaconType {
			pts = append(pts, Point{X: n.X, Y: n.Y})
		}
	}
	return append(pts, f.Finish)
}

func TestEvaluatePath_ReferencePathQualityHigh(t *testing.T) {
	field := evalField()
	q, valid, reason := EvaluatePath(field, referencePath(field))
	if !valid {
		t.Fatalf("reference path invalid: %s", reason)
	}
	if q < 0.95 {
		t.Errorf("reference path q = %v, want ≥ 0.95", q)
	}
	if q > 1 {
		t.Errorf("q = %v, want ≤ 1", q)
	}
}

func TestEvaluatePath_MissingBeaconInvalid(t *testing.T) {
	field := evalField()
	q, valid, reason := EvaluatePath(field, []Point{field.Start, field.Finish})
	if valid {
		t.Fatalf("path skipping beacon considered valid")
	}
	if q != 0 {
		t.Errorf("invalid path q = %v, want 0", q)
	}
	if reason != reasonBeaconNotCaptured {
		t.Errorf("reason = %q, want %q", reason, reasonBeaconNotCaptured)
	}
}

func TestEvaluatePath_OutOfBoundsInvalid(t *testing.T) {
	field := evalField()
	q, valid, reason := EvaluatePath(field, []Point{field.Start, {X: 1.5, Y: 0.5}})
	if valid {
		t.Fatalf("out-of-bounds path considered valid")
	}
	if q != 0 {
		t.Errorf("q = %v, want 0", q)
	}
	if reason != reasonPointOutOfBounds {
		t.Errorf("reason = %q, want %q", reason, reasonPointOutOfBounds)
	}
}

func TestEvaluatePath_EmptyAndSinglePointInvalid(t *testing.T) {
	field := evalField()
	cases := map[string][]Point{
		"nil":   nil,
		"empty": {},
		"one":   {field.Start},
	}
	for name, path := range cases {
		q, valid, reason := EvaluatePath(field, path)
		if valid {
			t.Errorf("%s: considered valid", name)
		}
		if q != 0 {
			t.Errorf("%s: q = %v, want 0", name, q)
		}
		if reason != reasonTooFewPoints {
			t.Errorf("%s: reason = %q, want %q", name, reason, reasonTooFewPoints)
		}
	}
}

func TestEvaluatePath_TooManyPointsInvalid(t *testing.T) {
	field := evalField()
	path := make([]Point, 0, maxPathPoints+1)
	path = append(path, field.Start)
	for i := 0; i < maxPathPoints; i++ {
		path = append(path, Point{X: 0.5, Y: 0.5})
	}
	path = append(path, field.Finish)
	if len(path) <= maxPathPoints {
		t.Fatalf("test path too short: %d", len(path))
	}
	q, valid, reason := EvaluatePath(field, path)
	if valid {
		t.Fatalf("over-cap path considered valid")
	}
	if q != 0 || reason != reasonTooManyPoints {
		t.Errorf("q=%v reason=%q, want 0/%q", q, reason, reasonTooManyPoints)
	}
}

func TestEvaluatePath_StartNotCaptured(t *testing.T) {
	field := evalField()
	_, valid, reason := EvaluatePath(field, []Point{{X: 0.5, Y: 0.5}, field.Finish})
	if valid || reason != reasonStartNotCaptured {
		t.Errorf("valid=%v reason=%q, want false/%q", valid, reason, reasonStartNotCaptured)
	}
}

func TestEvaluatePath_FinishNotCaptured(t *testing.T) {
	field := evalField()
	q, valid, reason := EvaluatePath(field, []Point{field.Start, {X: 0.5, Y: 0.5}})
	if valid || q != 0 || reason != reasonFinishNotCaptured {
		t.Errorf("valid=%v q=%v reason=%q, want false/0/%q", valid, q, reason, reasonFinishNotCaptured)
	}
}

func TestEvaluatePath_LongerValidPathLowerQuality(t *testing.T) {
	field := evalField()
	detour := []Point{field.Start, {X: 0.5, Y: 0.8}, {X: 0.5, Y: 0.2}, field.Finish}
	q, valid, reason := EvaluatePath(field, detour)
	if !valid {
		t.Fatalf("detour invalid: %s", reason)
	}
	if q >= 1 {
		t.Errorf("detour q = %v, want < 1", q)
	}
	if q <= 0 {
		t.Errorf("detour q = %v, want > 0", q)
	}
}

func TestPathEffLen_ZonePenalty(t *testing.T) {
	start := Point{X: 0.1, Y: 0.5}
	finish := Point{X: 0.9, Y: 0.5}
	direct := []Point{start, finish}
	without := pathEffLen(direct, nil)

	onPath := []FieldZone{{Type: hazardType, X: 0.5, Y: 0.5, R: 0.2, Coefficient: 1.8}}
	withZone := pathEffLen(direct, onPath)
	if !(withZone > without) {
		t.Fatalf("path through zone not more expensive: %v vs %v", withZone, without)
	}
	if math.Abs(withZone-without*1.8) > 1e-9 {
		t.Errorf("zone multiplier not applied: got %v want %v", withZone, without*1.8)
	}

	// Зона вдали от отрезка штрафа не даёт.
	far := []FieldZone{{Type: hazardType, X: 0.5, Y: 0.95, R: 0.1, Coefficient: 1.8}}
	if got := pathEffLen(direct, far); math.Abs(got-without) > 1e-9 {
		t.Errorf("far zone changed cost: got %v want %v", got, without)
	}
}

func TestEvaluatePath_QualityInRange(t *testing.T) {
	field := evalField()
	paths := [][]Point{
		referencePath(field),
		{field.Start, {X: 0.5, Y: 0.8}, {X: 0.5, Y: 0.2}, field.Finish},
	}
	for i, path := range paths {
		q, valid, reason := EvaluatePath(field, path)
		if !valid {
			t.Fatalf("path %d invalid: %s", i, reason)
		}
		if q < 0 || q > 1 {
			t.Errorf("path %d q = %v out of [0,1]", i, q)
		}
	}
}

func TestEvaluatePath_GeneratedFieldReferenceValid(t *testing.T) {
	field := GenerateField(2024, 25000, restlessPassport())
	path := referencePath(field)
	q, valid, reason := EvaluatePath(field, path)
	if !valid {
		t.Fatalf("reference path over generated field invalid: %s", reason)
	}
	if q <= 0 || q > 1 {
		t.Errorf("q = %v out of (0,1]", q)
	}
}

func TestEvaluatePath_Deterministic(t *testing.T) {
	field := evalField()
	path := referencePath(field)
	q1, v1, r1 := EvaluatePath(field, path)
	q2, v2, r2 := EvaluatePath(field, path)
	if q1 != q2 || v1 != v2 || r1 != r2 {
		t.Errorf("evaluation not deterministic: (%v,%v,%q) vs (%v,%v,%q)", q1, v1, r1, q2, v2, r2)
	}
}
