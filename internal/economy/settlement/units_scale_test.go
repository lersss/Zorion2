// internal/economy/settlement/units_scale_test.go
// Переключатель масштаба отображения/ввода единицы «ед/сутки/млрд» (задача
// «переключатель масштаба единицы», спека 2026-09-23-стадии-поселения §2.1):
// хранимая единица одна, масштаб влияет только на показ/ввод. Множители: на
// человека ×1e-9, на 1000 ×1e-6, на 10⁶ ×1e-3, на млрд ×1.
package settlement

import (
	"math"
	"testing"
)

// Прямой и обратный перевод по всем четырём масштабам; неизвестный масштаб —
// хранимая единица (×1), без паники.
func TestUnitScaleConversion(t *testing.T) {
	cases := []struct {
		scale UnitScale
		mul   float64
	}{
		{ScalePerBillion, 1},
		{ScalePerMillion, 1e-3},
		{ScalePerThousand, 1e-6},
		{ScalePerPerson, 1e-9},
	}
	for _, c := range cases {
		if got := UnitScaleMultiplier(c.scale); math.Abs(got-c.mul) > 1e-18 {
			t.Fatalf("%q: множитель %v, ждали %v", c.scale, got, c.mul)
		}
		stored := 600.0
		if got := StoredToDisplay(stored, c.scale); math.Abs(got-stored*c.mul) > 1e-18 {
			t.Fatalf("%q: StoredToDisplay(600) = %v, ждали %v", c.scale, got, stored*c.mul)
		}
		back := DisplayToStored(StoredToDisplay(stored, c.scale), c.scale)
		if math.Abs(back-stored) > 1e-9 {
			t.Fatalf("%q: обратный перевод не вернул 600: %v", c.scale, back)
		}
	}
	// «на человека»: ввод 0.6 → хранимое 6·10⁸.
	if got := DisplayToStored(0.6, ScalePerPerson); math.Abs(got-6e8) > 1e-3 {
		t.Fatalf("ввод 0.6 «на человека» → %v, ждали 6e8", got)
	}
	if got := UnitScaleMultiplier(UnitScale("nope")); got != 1 {
		t.Fatalf("неизвестный масштаб → ×1, получили %v", got)
	}
}
