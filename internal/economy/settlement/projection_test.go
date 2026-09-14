package settlement

import (
	"math"
	"testing"
)

func TestOtherChangeRateZeroInComfort(t *testing.T) {
	input := PlanetInput{
		TemperatureK:      275, // внутри HumanTemperatureProfile
		GravityG:          1.0, // внутри HumanGravityProfile
		CoreRadioactivity: 5,   // ниже HumanRadioactivityProfile.Threshold
	}
	if got := OtherChangeRate(input, DefaultScale); got != 0 {
		t.Errorf("OtherChangeRate в комфорте по всем факторам = %v, хочу 0", got)
	}
}

func TestOtherChangeRateSumsFactors(t *testing.T) {
	// Факторы складываются (99.2.12, 18a «Механизм»), а не берётся худший.
	// Жара в λ НЕ входит — её рекурсивная компонента HeatChangeRate
	// (99.2.12, R-модель). «Холодный» фактор — 150 K (λ 0.0027, полюс холода),
	// радиоактивность 90 → sev кап 1 → λ 0.1.
	comfortable := PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 5}
	onlyHot := PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5}
	onlyCold := PlanetInput{TemperatureK: 150, GravityG: 1.0, CoreRadioactivity: 5}
	onlyRadioactive := PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 90}
	both := PlanetInput{TemperatureK: 150, GravityG: 1.0, CoreRadioactivity: 90}

	lambdaComfortable := OtherChangeRate(comfortable, DefaultScale)
	lambdaHot := OtherChangeRate(onlyHot, DefaultScale)
	lambdaCold := OtherChangeRate(onlyCold, DefaultScale)
	lambdaRadioactive := OtherChangeRate(onlyRadioactive, DefaultScale)
	lambdaBoth := OtherChangeRate(both, DefaultScale)

	if lambdaComfortable != 0 {
		t.Fatalf("контрольная точка сломана: lambdaComfortable = %v", lambdaComfortable)
	}
	if lambdaHot != 0 {
		t.Errorf("жара не должна давать λ: %v (её рекурсивная компонента)", lambdaHot)
	}
	if RecursiveChangeRate(400) <= 0 {
		t.Errorf("RecursiveChangeRate(400 K) = %v, хочу > 0 (жара жива в r)", RecursiveChangeRate(400))
	}
	// Калибровочные якоря (99.2.12): 150 K → λ 0.0027 (полюс холода),
	// радиоактивность 90 → sev кап 1 → λ 0.1.
	if math.Abs(lambdaCold-0.0027) > 0.0001 || math.Abs(lambdaRadioactive-0.1) > 1e-9 {
		t.Errorf("калибровка λ сломана: cold=%v (хочу 0.0027), rad=%v (хочу 0.1)", lambdaCold, lambdaRadioactive)
	}
	want := lambdaCold + lambdaRadioactive
	if want != lambdaBoth {
		t.Errorf("факторы не складываются: холод+радиация раздельно = %v, вместе = %v", want, lambdaBoth)
	}
	if !(lambdaBoth > lambdaCold && lambdaBoth > lambdaRadioactive) {
		t.Errorf("комбинация должна убивать быстрее одного фактора: both=%v cold=%v rad=%v", lambdaBoth, lambdaCold, lambdaRadioactive)
	}
}

func TestProjectionZeroLambdaIsConstant(t *testing.T) {
	p0 := 1000.0
	proj := Projection(p0, 0, 0)
	for _, cp := range StandardCheckpoints {
		if proj[cp.Label] != p0 {
			t.Errorf("Projection[%q] = %v без угрозы, хочу %v", cp.Label, proj[cp.Label], p0)
		}
	}
}

func TestProjectionDecreasesOverCheckpoints(t *testing.T) {
	p0 := 1_000_000.0
	lambda := 0.01
	proj := Projection(p0, 0, lambda)

	prev := p0
	for _, cp := range StandardCheckpoints {
		got := proj[cp.Label]
		if got > prev {
			t.Errorf("население выросло на контрольной точке %q: было %v, стало %v", cp.Label, prev, got)
		}
		prev = got
	}
}

func TestOtherChangeRateHardZeroDominatesSum(t *testing.T) {
	// Жёсткий ноль доминирует над сложением компонент (99.2.12, R-модель):
	// холод T = 50 K → +Inf (полюс холода, временно), а не конечная сумма.
	// Жара обнуления не имеет — её рекурсивная компонента (см. TestRCurve).
	input := PlanetInput{
		TemperatureK:      50,
		GravityG:          1.0,
		CoreRadioactivity: 5,
	}
	if got := OtherChangeRate(input, DefaultScale); !math.IsInf(got, 1) {
		t.Errorf("OtherChangeRate(50 K) = %v, хочу +Inf", got)
	}
}

func TestProjectionHardZeroIsZero(t *testing.T) {
	// Projection(p0, 0, +Inf) = 0 на всех чекпоинтах: +Inf (холод) → 0
	// (99.2.12, R-модель).
	proj := Projection(1000, 0, math.Inf(1))
	for _, cp := range StandardCheckpoints {
		if proj[cp.Label] != 0 {
			t.Errorf("Projection[%q] при +Inf = %v, хочу 0", cp.Label, proj[cp.Label])
		}
	}
}
