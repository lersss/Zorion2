package handlers

import (
	"math"
	"testing"
)

// TestSizeHintStepped — степенная лестница размера тела в небе (спека
// 2026-09-22 §4.4): h = size^0.6 / 4.26, кламп [0.05, 1.0] (4.26 = 11.2^0.6 —
// нормализатор, гигант → 1.0). Держит главный дефект старой формулы
// clamp(size/5, …): все гиганты ≥5.9 давали ровно 1.0 (неразличимы).
func TestSizeHintStepped(t *testing.T) {
	cases := []struct{ size, want float64 }{
		{0.3, 0.11}, {1.0, 0.23}, {3.0, 0.45}, {5.9, 0.68}, {11.2, 1.0},
		{0, 0.05}, {-2, 0.05}, // неположительный размер → пол 0.05 (не NaN)
	}
	for _, c := range cases {
		got := sizeHint(c.size)
		if math.IsNaN(got) {
			t.Errorf("sizeHint(%v) = NaN", c.size)
			continue
		}
		if math.Abs(got-c.want) > 0.005 {
			t.Errorf("sizeHint(%v) = %v, want ~%v", c.size, got, c.want)
		}
	}
	// Гиганты различимы: 5.9 строго меньше 11.2 (старая формула давала 1.0/1.0).
	if !(sizeHint(5.9) < sizeHint(11.2)) {
		t.Errorf("гиганты неразличимы: 5.9=%v 11.2=%v", sizeHint(5.9), sizeHint(11.2))
	}
}
