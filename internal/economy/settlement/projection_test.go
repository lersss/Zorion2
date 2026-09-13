package settlement

import "testing"

func TestTotalLambdaZeroInComfort(t *testing.T) {
	input := PlanetInput{
		TemperatureK:      275, // внутри HumanTemperatureProfile
		GravityG:          1.0, // внутри HumanGravityProfile
		CoreRadioactivity: 5,   // ниже HumanRadioactivityProfile.Threshold
	}
	if got := TotalLambda(input, DefaultScale); got != 0 {
		t.Errorf("TotalLambda в комфорте по всем факторам = %v, хочу 0", got)
	}
}

func TestTotalLambdaSumsFactors(t *testing.T) {
	comfortable := PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 5}
	onlyHot := PlanetInput{TemperatureK: 900, GravityG: 1.0, CoreRadioactivity: 5}
	onlyRadioactive := PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 90}
	both := PlanetInput{TemperatureK: 900, GravityG: 1.0, CoreRadioactivity: 90}

	lambdaComfortable := TotalLambda(comfortable, DefaultScale)
	lambdaHot := TotalLambda(onlyHot, DefaultScale)
	lambdaRadioactive := TotalLambda(onlyRadioactive, DefaultScale)
	lambdaBoth := TotalLambda(both, DefaultScale)

	if lambdaComfortable != 0 {
		t.Fatalf("контрольная точка сломана: lambdaComfortable = %v", lambdaComfortable)
	}
	want := lambdaHot + lambdaRadioactive
	if want != lambdaBoth {
		t.Errorf("факторы не складываются: жара+радиация раздельно = %v, вместе = %v", want, lambdaBoth)
	}
	if !(lambdaBoth > lambdaHot && lambdaBoth > lambdaRadioactive) {
		t.Errorf("комбинация должна убивать быстрее одного фактора: both=%v hot=%v rad=%v", lambdaBoth, lambdaHot, lambdaRadioactive)
	}
}

func TestProjectionZeroLambdaIsConstant(t *testing.T) {
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
	lambda := 0.01
	proj := Projection(p0, lambda)

	prev := p0
	for _, cp := range StandardCheckpoints {
		got := proj[cp.Label]
		if got > prev {
			t.Errorf("население выросло на контрольной точке %q: было %v, стало %v", cp.Label, prev, got)
		}
		prev = got
	}
}
