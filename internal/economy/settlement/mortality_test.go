package settlement

import (
	"math"
	"testing"
)

func TestTemperatureSeverityInsideComfort(t *testing.T) {
	profile := HumanProfile
	for _, temp := range []float64{profile.ComfortMinK, 275, profile.ComfortMaxK} {
		if got := TemperatureSeverity(profile, temp); got != 0 {
			t.Errorf("TemperatureSeverity(%v) = %v, хочу 0 внутри комфорта", temp, got)
		}
	}
}

func TestTemperatureSeveritySymmetric(t *testing.T) {
	profile := HumanProfile
	below := TemperatureSeverity(profile, profile.ComfortMinK-50)
	above := TemperatureSeverity(profile, profile.ComfortMaxK+50)
	if below != above {
		t.Errorf("тяжесть несимметрична: холод=%v, жара=%v при одинаковом отклонении", below, above)
	}
}

func TestTemperatureSeverityMonotonic(t *testing.T) {
	profile := HumanProfile
	near := TemperatureSeverity(profile, profile.ComfortMaxK+10)
	far := TemperatureSeverity(profile, profile.ComfortMaxK+100)
	if !(near < far) {
		t.Errorf("тяжесть не растёт с отклонением: near=%v far=%v", near, far)
	}
}

func TestTemperatureSeveritySaturates(t *testing.T) {
	profile := HumanProfile
	atSaturation := TemperatureSeverity(profile, profile.ComfortMaxK+profile.SaturateAtK)
	beyond := TemperatureSeverity(profile, profile.ComfortMaxK+profile.SaturateAtK*10)
	if atSaturation != 1 || beyond != 1 {
		t.Errorf("нет насыщения на 1: на границе=%v, далеко за ней=%v", atSaturation, beyond)
	}
}

func TestLambdaZeroSeverityIsZero(t *testing.T) {
	if got := Lambda(0, DefaultScale); got != 0 {
		t.Errorf("Lambda(0, ...) = %v, хочу 0", got)
	}
}

func TestLambdaScalesWithSeverity(t *testing.T) {
	half := Lambda(0.5, DefaultScale)
	full := Lambda(1.0, DefaultScale)
	if math.Abs(full-2*half) > 1e-9 {
		t.Errorf("Lambda не линейна по тяжести: half=%v full=%v", half, full)
	}
}

func TestPopulationNoLambdaIsConstant(t *testing.T) {
	p0 := 1000.0
	got := Population(p0, 0, 24*365)
	if got != p0 {
		t.Errorf("Population без угрозы изменилось: %v != %v", got, p0)
	}
}

// Урок отброшенного пробника (docs/gamedesign/ideas/13b...md, п.6): большое
// население не должно давать иммунитет к среде — доля убыли одинакова
// независимо от размера поселения.
func TestPopulationNoImmunityFromSize(t *testing.T) {
	lambda := Lambda(1.0, DefaultScale)
	deltaHours := 100.0

	small := Population(100, lambda, deltaHours)
	large := Population(1_000_000, lambda, deltaHours)

	fractionSmall := small / 100
	fractionLarge := large / 1_000_000
	if math.Abs(fractionSmall-fractionLarge) > 1e-9 {
		t.Errorf("доля выживших зависит от размера: маленькое=%v большое=%v", fractionSmall, fractionLarge)
	}
}

func TestPopulationHalfLife(t *testing.T) {
	p0 := 1000.0
	lambda := 0.1
	halfLife := math.Ln2 / lambda

	got := Population(p0, lambda, halfLife)
	want := p0 / 2
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("Population на половинном времени жизни = %v, хочу %v", got, want)
	}
}

func TestPopulationNeverNegativeOrExceedsStart(t *testing.T) {
	p0 := 500.0
	got := Population(p0, 0.5, 1000)
	if got < 0 || got > p0 {
		t.Errorf("Population вышло за пределы [0, p0]: %v", got)
	}
}

func TestPopulationZeroStart(t *testing.T) {
	if got := Population(0, 1, 10); got != 0 {
		t.Errorf("Population(0, ...) = %v, хочу 0", got)
	}
}
