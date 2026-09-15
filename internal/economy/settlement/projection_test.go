package settlement

import (
	"math"
	"testing"
)

func TestProjectionZeroIsConstant(t *testing.T) {
	p0 := 1000.0
	proj := Projection(p0, 0)
	for _, cp := range StandardCheckpoints {
		if proj[cp.Label] != p0 {
			t.Errorf("Projection[%q] = %v без угрозы, хочу %v", cp.Label, proj[cp.Label], p0)
		}
	}
}

func TestProjectionDecreasesOverCheckpoints(t *testing.T) {
	p0 := 1_000_000.0
	r := 0.00115
	proj := Projection(p0, r)

	prev := p0
	for _, cp := range StandardCheckpoints {
		got := proj[cp.Label]
		if got > prev {
			t.Errorf("население выросло на контрольной точке %q: было %v, стало %v", cp.Label, prev, got)
		}
		prev = got
	}
}

func TestProjectionGuardZero(t *testing.T) {
	// Гвард робастности (99.2.13): r ≥ 1 (экстремальные входы) → все точки 0.
	proj := Projection(1000, 1.5)
	for _, cp := range StandardCheckpoints {
		if proj[cp.Label] != 0 {
			t.Errorf("Projection[%q] при r ≥ 1 = %v, хочу 0", cp.Label, proj[cp.Label])
		}
	}
}

// Контрольная точка: комфорт по всем факторам (303.15 K, 1 g, rad ≤ 20) —
// только естественная пара (99.2.16): рождаемость минус смертность, нетто
// −NCR (рост при дефолте k=2) — прежнее «+NCR» пересмотрено.
func TestComfortTotalIsNettoNatural(t *testing.T) {
	input := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 20}
	if got := ChangeComponents(input); math.Abs(got+NaturalChangeRate()) > 1e-12 {
		t.Errorf("комфорт по всем факторам: r = %v, хочу −R_ест %v", got, NaturalChangeRate())
	}
}