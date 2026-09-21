package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// TestShipHintPair — чистый канонизатор подсказки авто-носа (спека §3.3):
// |v|≤1 → A=0, знак и зеркало закреплены в одном месте (§9 п.3a).
func TestShipHintPair(t *testing.T) {
	cases := []struct {
		name      string
		angle     float64
		mirror    bool
		ambiguous bool
		wantA     float64
		wantF     bool
	}{
		{"v>1 без зеркала", 12.5, false, false, -12.5, false},
		{"v>1 с зеркалом", 12.5, true, false, 12.5, true},
		{"v<=1 без зеркала", 0.5, false, false, 0, false},
		{"v<=1 с зеркалом", 0.5, true, false, 0, true},
		{"ambiguous снимает зеркало", 12.5, true, true, -12.5, false},
	}
	for _, c := range cases {
		a, f := ShipHintPair(c.angle, c.mirror, c.ambiguous)
		if a != c.wantA || f != c.wantF {
			t.Errorf("%s: (%v,%v), want (%v,%v)", c.name, a, f, c.wantA, c.wantF)
		}
	}
}

// TestShipOrientWritersWrap — писатели пары держат (−180,180]: setangle −180 →
// 180, rotate(+200) → −160 (спека §3.1, §9 п.12).
func TestShipOrientWritersWrap(t *testing.T) {
	dir := t.TempDir()
	meta := `[{"file":"s01.png","race":"humans"}]`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}
	if !SetShipOrient(dir, "s01.png", -180, false) {
		t.Fatal("SetShipOrient: кандидат не найден")
	}
	if a, _, _ := ShipOrientOf(dir, "s01.png"); a != 180 {
		t.Errorf("setangle -180 → %v, want 180", a)
	}
	if !SetShipOrient(dir, "s01.png", 0, false) {
		t.Fatal("SetShipOrient: кандидат не найден")
	}
	if !UpdateShipAngle(dir, "s01.png", 200, false) {
		t.Fatal("UpdateShipAngle: кандидат не найден")
	}
	if a, _, _ := ShipOrientOf(dir, "s01.png"); a != -160 {
		t.Errorf("rotate(+200) → %v, want -160", a)
	}
}
