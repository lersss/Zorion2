package settlement

import (
	"math"
	"testing"
)

func TestTemperatureSeverityInsideComfort(t *testing.T) {
	profile := HumanTemperatureProfile
	for _, temp := range []float64{profile.ComfortMin, 275, profile.ComfortMax} {
		if got := TwoSidedSeverity(profile, temp); got != 0 {
			t.Errorf("TwoSidedSeverity(%v) = %v, хочу 0 внутри комфорта", temp, got)
		}
	}
}

func TestTemperatureSeverityAsymmetricColdSofter(t *testing.T) {
	// Асимметрия жары/холода (99.2.12, H3): при одинаковом отклонении 50 K
	// холод (150 K) мягче жары (400 K): sev = (50/350)² = 0.0204 < (50/200)² = 0.0625.
	profile := HumanTemperatureProfile
	below := TwoSidedSeverity(profile, profile.ComfortMin-50)
	above := TwoSidedSeverity(profile, profile.ComfortMax+50)
	if !(below < above) {
		t.Errorf("холод должен быть мягче жары при равном отклонении: холод=%v, жара=%v", below, above)
	}
}

func TestTemperatureSeverityMonotonic(t *testing.T) {
	profile := HumanTemperatureProfile
	near := TwoSidedSeverity(profile, profile.ComfortMax+10)
	far := TwoSidedSeverity(profile, profile.ComfortMax+100)
	if !(near < far) {
		t.Errorf("тяжесть не растёт с отклонением: near=%v far=%v", near, far)
	}
}

func TestTemperatureSeverityQuadraticUnclamped(t *testing.T) {
	// Квадратичная форма без капа (99.2.12, H2): на насыщении sev = 1,
	// дальше растёт: 600 K → dev 250 → (250/200)² = 1.5625.
	profile := HumanTemperatureProfile
	atSaturation := TwoSidedSeverity(profile, profile.ComfortMax+profile.SaturateHot) // 550 K: dev 200
	if atSaturation != 1 {
		t.Errorf("severity на насыщении = %v, хочу 1", atSaturation)
	}
	beyond := TwoSidedSeverity(profile, profile.ComfortMax+250) // 600 K: dev 250
	if beyond != 1.5625 {
		t.Errorf("severity за насыщением должна расти без капа: %v, хочу 1.5625", beyond)
	}
}

func TestGravitySeverityInsideComfort(t *testing.T) {
	profile := HumanGravityProfile
	for _, g := range []float64{profile.ComfortMin, 1.0, profile.ComfortMax} {
		if got := TwoSidedSeverity(profile, g); got != 0 {
			t.Errorf("TwoSidedSeverity(%v) = %v, хочу 0 внутри комфорта", g, got)
		}
	}
}

func TestGravitySeveritySymmetric(t *testing.T) {
	profile := HumanGravityProfile
	below := TwoSidedSeverity(profile, profile.ComfortMin-0.3)
	above := TwoSidedSeverity(profile, profile.ComfortMax+0.3)
	if below != above {
		t.Errorf("тяжесть несимметрична: недогрузка=%v, перегрузка=%v при одинаковом отклонении", below, above)
	}
}

func TestGravitySeverityZeroGIsNotBelowComfortMin(t *testing.T) {
	// Невесомость (0g) — валидное, но не бесконечно плохое значение: тяжесть
	// не должна вылезать за пределы [0, 1] даже когда отклонение ограничено
	// снизу нулевой гравитацией.
	profile := HumanGravityProfile
	got := TwoSidedSeverity(profile, 0)
	if got < 0 || got > 1 {
		t.Errorf("TwoSidedSeverity(0) = %v, хочу в [0, 1]", got)
	}
}

func TestGravitySeveritySaturates(t *testing.T) {
	profile := HumanGravityProfile
	atSaturation := TwoSidedSeverity(profile, profile.ComfortMax+profile.SaturateHot)
	beyond := TwoSidedSeverity(profile, profile.ComfortMax+profile.SaturateHot*10)
	if atSaturation != 1 || beyond != 1 {
		t.Errorf("нет насыщения на 1: на границе=%v, далеко за ней=%v", atSaturation, beyond)
	}
}

func TestRadioactivitySeverityAtOrBelowThresholdIsZero(t *testing.T) {
	profile := HumanRadioactivityProfile
	for _, value := range []float64{0, profile.Threshold / 2, profile.Threshold} {
		if got := OneSidedSeverity(profile, value); got != 0 {
			t.Errorf("OneSidedSeverity(%v) = %v, хочу 0 на пороге и ниже", value, got)
		}
	}
}

func TestRadioactivitySeverityMonotonic(t *testing.T) {
	profile := HumanRadioactivityProfile
	near := OneSidedSeverity(profile, profile.Threshold+5)
	far := OneSidedSeverity(profile, profile.Threshold+50)
	if !(near < far) {
		t.Errorf("тяжесть не растёт с превышением порога: near=%v far=%v", near, far)
	}
}

func TestRadioactivitySeveritySaturates(t *testing.T) {
	profile := HumanRadioactivityProfile
	atSaturation := OneSidedSeverity(profile, profile.Threshold+profile.SaturateAt)
	beyond := OneSidedSeverity(profile, profile.Threshold+profile.SaturateAt*10)
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

func TestHardZeroHot(t *testing.T) {
	// Жёсткий ноль жары (99.2.12, H4): T ≥ 700 K → λ = +Inf, население = 0.
	input := comfortableInput()
	input.TemperatureK = 700
	if got := TotalLambda(input, DefaultScale); !math.IsInf(got, 1) {
		t.Errorf("TotalLambda(700 K) = %v, хочу +Inf", got)
	}
	input.TemperatureK = 699
	got := TotalLambda(input, DefaultScale)
	if math.IsInf(got, 1) || got <= 0 {
		t.Errorf("TotalLambda(699 K) = %v, хочу конечную и > 0", got)
	}
}

func TestHardZeroColdNotSet(t *testing.T) {
	// Холодный жёсткий ноль не задан (99.2.12, H4): все холодные планеты
	// принципиально обитаемы, λ конечна.
	input := comfortableInput()
	for _, temp := range []float64{50, 0} {
		input.TemperatureK = temp
		if got := TotalLambda(input, DefaultScale); math.IsInf(got, 1) {
			t.Errorf("TotalLambda(%v K) = +Inf, хочу конечную", temp)
		}
	}
}

func TestPopulationInfLambda(t *testing.T) {
	// Guard жёсткого нуля (99.2.12, §Изменения в коде): +Inf убивает сразу
	// при Δt > 0, но не двигает чек-точку при Δt = 0.
	if got := Population(1000, math.Inf(1), 24); got != 0 {
		t.Errorf("Population(1000, +Inf, 24) = %v, хочу 0", got)
	}
	if got := Population(1000, math.Inf(1), 0); got != 1000 {
		t.Errorf("Population(1000, +Inf, 0) = %v, хочу 1000 (чек-точка не движется)", got)
	}
}
