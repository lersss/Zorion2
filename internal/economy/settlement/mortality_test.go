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

func TestTemperatureSeverityMonotonic(t *testing.T) {
	// Холодная сторона полюса B монотонна: чем дальше от комфорта, тем тяжелее.
	profile := HumanTemperatureProfile
	near := TwoSidedSeverity(profile, profile.ComfortMin-10)
	far := TwoSidedSeverity(profile, profile.ComfortMin-100)
	if !(near < far) {
		t.Errorf("тяжесть не растёт с отклонением: near=%v far=%v", near, far)
	}
}

func TestTemperatureSeverityQuadraticUnclamped(t *testing.T) {
	// Полюсная форма B сохранилась для холода (99.2.12, концепция 2.0:
	// жара переведена на двухфазную модель, холод временно на полюсе B):
	// sev = (dev/Sat)²/(1−(dev/H)²), полюс при dev = HardZero = 100.
	profile := HumanTemperatureProfile
	cases := []struct {
		name string
		temp float64
		want float64
	}{
		{"150 K: dev 50", profile.ComfortMin - 50, 0.0272}, // 0.0204/0.75
		{"123 K: dev 77", profile.ComfortMin - 77, 0.1189}, // 0.0484/0.4071
		{"101 K: dev 99", profile.ComfortMin - 99, 4.02},   // 0.0800/0.0199
	}
	for _, tc := range cases {
		got := TwoSidedSeverity(profile, tc.temp)
		if math.Abs(got-tc.want) > 0.01*tc.want {
			t.Errorf("TwoSidedSeverity(%v) = %v, хочу %v (±1%%)", tc.temp, got, tc.want)
		}
	}
	// Полюс холода: dev = HardZero → guard → +Inf (знаменатель 1−(dev/H)² = 0).
	if got := TwoSidedSeverity(profile, profile.ComfortMin-profile.HardZeroCold); !math.IsInf(got, 1) {
		t.Errorf("TwoSidedSeverity(100 K) = %v, хочу +Inf (полюс холода)", got)
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

func TestSeverityRateZero(t *testing.T) {
	if got := SeverityRate(0, DefaultScale); got != 0 {
		t.Errorf("SeverityRate(0, ...) = %v, хочу 0", got)
	}
}

func TestSeverityRateScalesWithSeverity(t *testing.T) {
	half := SeverityRate(0.5, DefaultScale)
	full := SeverityRate(1.0, DefaultScale)
	if math.Abs(full-2*half) > 1e-9 {
		t.Errorf("SeverityRate не линейна по тяжести: half=%v full=%v", half, full)
	}
}

func TestPopulationNoLambdaIsConstant(t *testing.T) {
	p0 := 1000.0
	got := Population(p0, 0, 0, 24*365*3600)
	if got != p0 {
		t.Errorf("Population без угрозы изменилось: %v != %v", got, p0)
	}
}

// Урок отброшенного пробника (docs/gamedesign/ideas/13b...md, п.6): большое
// население не должно давать иммунитет к среде — доля убыли одинакова
// независимо от размера поселения.
func TestPopulationNoImmunityFromSize(t *testing.T) {
	lambda := SeverityRate(1.0, DefaultScale)
	deltaSeconds := 10 * 3600.0 // оба остаются ≥ 1 (порог «p < 1» не мешает)

	small := Population(100, 0, lambda, deltaSeconds)
	large := Population(1_000_000, 0, lambda, deltaSeconds)

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

	got := Population(p0, 0, lambda, halfLife*3600)
	want := p0 / 2
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("Population на половинном времени жизни = %v, хочу %v", got, want)
	}
}

func TestPopulationNeverNegativeOrExceedsStart(t *testing.T) {
	p0 := 500.0
	got := Population(p0, 0, 0.5, 1000*3600)
	if got < 0 || got > p0 {
		t.Errorf("Population вышло за пределы [0, p0]: %v", got)
	}
}

func TestPopulationZeroStart(t *testing.T) {
	if got := Population(0, 0, 1, 10*3600); got != 0 {
		t.Errorf("Population(0, ...) = %v, хочу 0", got)
	}
}

func TestPopulationInfLambda(t *testing.T) {
	// Guard жёсткого нуля холода (99.2.12, R-модель): +Inf убивает сразу
	// при Δt > 0, но не двигает чек-точку при Δt = 0.
	if got := Population(1000, 0, math.Inf(1), 24*3600); got != 0 {
		t.Errorf("Population(1000, +Inf, 24 ч) = %v, хочу 0", got)
	}
	if got := Population(1000, 0, math.Inf(1), 0); got != 1000 {
		t.Errorf("Population(1000, +Inf, 0) = %v, хочу 1000 (чек-точка не движется)", got)
	}
}

// ==================== ЖАРА: R-МОДЕЛЬ (99.2.12) ====================

func TestRCurve(t *testing.T) {
	// R(T) по двухрежимной сшивке в контрольных точках (99.2.12, §Критерии
	// готовности; допуск ±50% — точки критерий тестера, не узлы).
	cases := []struct {
		tempC float64
		want  float64
	}{
		{30, 6.34e-10},
		{70, 6.6e-9},
		{100, 1.16e-4},
		{200, 1.15e-3},
		{500, 0.016},
		{2000, 0.47},
		{4000, 0.98},
	}
	for _, tc := range cases {
		got := RecursiveChangeRate(tc.tempC + 273.15)
		if math.Abs(got-tc.want) > 0.5*tc.want {
			t.Errorf("R(%v °C) = %v, хочу ≈ %v (±50%%)", tc.tempC, got, tc.want)
		}
	}
	// Комфорт и рампа: 288 K → 0; 303 K → ≈ R_ест; 295 K → ≈ R_ест·7/15.
	if got := RecursiveChangeRate(288); got != 0 {
		t.Errorf("R(288 K) = %v, хочу 0", got)
	}
	if got := RecursiveChangeRate(303); math.Abs(got-NaturalChangeRate) > 0.02*NaturalChangeRate {
		t.Errorf("R(303 K) = %v, хочу ≈ R_ест (±2%%)", got)
	}
	ramp := RecursiveChangeRate(295) // 21.85 °C → R_ест·6.85/15; ориентир «7/15» ≈ 22 °C
	want := NaturalChangeRate * (295 - 273.15 - 15) / 15
	if math.Abs(ramp-want) > 1e-12 {
		t.Errorf("R(295 K) = %v, хочу %v", ramp, want)
	}
	if math.Abs(ramp-NaturalChangeRate*7/15) > 0.05*NaturalChangeRate*7/15 {
		t.Errorf("R(295 K) = %v, хочу ≈ R_ест·7/15 (±5%%)", ramp)
	}
	// Модульность: естественная и температурная компоненты раздельны —
	// R_total(30 °C) = 6.34·10⁻¹⁰ (только естественная), R_темп(30 °C) = 0.
	if got := NaturalComponent(30 + 273.15); got != NaturalChangeRate {
		t.Errorf("NaturalComponent(30 °C) = %v, хочу %v", got, NaturalChangeRate)
	}
	if got := HeatTemperatureChangeRate(30 + 273.15); got != 0 {
		t.Errorf("R_темп(30 °C) = %v, хочу 0 (температурная компонента без естественной)", got)
	}
	if got := HeatTemperatureChangeRate(30.1 + 273.15); got <= 0 {
		t.Errorf("R_темп(30.1 °C) = %v, хочу > 0", got)
	}
}

func TestRMonotonic(t *testing.T) {
	// R монотонно растёт на [15, 4000] °C и всегда < 1 (99.2.12, §Решение).
	var prev float64
	for i, tempC := range []float64{15, 30, 50, 70, 90, 100, 200, 300, 500, 1000, 2000, 4000} {
		r := RecursiveChangeRate(tempC + 273.15)
		if r >= 1 {
			t.Errorf("R(%v °C) = %v, хочу < 1", tempC, r)
		}
		if i > 0 && !(r > prev) {
			t.Errorf("R не монотонна: %v °C → %v (было %v)", tempC, r, prev)
		}
		prev = r
	}
}

func TestRSpliceContinuity(t *testing.T) {
	// Стык зон (99.2.12, §Стык зон): σ(90) = 0.5 — чисто температурная
	// компонента R_темп(90) = среднее зон; шаг 89→91 °C конечен и мал
	// относительно уровня тепловой зоны (нет разрыва — сигмоида C∞).
	sigma90 := 1 / (1 + math.Exp(-rSigmaK*(90-rSigmaT)))
	if math.Abs(sigma90-0.5) > 1e-12 {
		t.Errorf("σ(90) = %v, хочу 0.5", sigma90)
	}
	// Естественная компонента отделена: R_total(90) = NaturalChangeRate + R_темп(90).
	if got := NaturalComponent(90 + 273.15); got != NaturalChangeRate {
		t.Errorf("NaturalComponent(90 °C) = %v, хочу %v", got, NaturalChangeRate)
	}
	blend := 0.5 * (heatTempC*60*60 +
		(1 - math.Exp(-rThermalA*math.Pow(0.6, rThermalN))))
	if r90 := HeatTemperatureChangeRate(90 + 273.15); math.Abs(r90-blend) > 1e-12 {
		t.Errorf("R_темп(90) = %v, хочу среднее зон %v (σ=0.5)", r90, blend)
	}
	step := math.Abs(RecursiveChangeRate(91+273.15) - RecursiveChangeRate(89+273.15))
	if step > 0.5*RecursiveChangeRate(100+273.15) {
		t.Errorf("разрыв на стыке 89→91 °C: шаг %v > 50%% R(100)", step)
	}
}

func TestRPopulation(t *testing.T) {
	// Дискретная рекурсия: p = p0·(1−r)^Δt_сек (99.2.12, §Решение).
	want := 1000 * math.Pow(0.99885, 3600) // (1−0.00115)^3600
	if got := Population(1000, 0.00115, 0, 3600); math.Abs(got-want) > 1e-6 {
		t.Errorf("Population(1000, 0.00115, 0, 3600) = %v, хочу %v", got, want)
	}
	if got := Population(1000, 0.00115, 0, 0); got != 1000 {
		t.Errorf("Population(1000, r, 0) = %v, хочу 1000 (чек-точка не движется)", got)
	}
	if got := Population(1000, 0.5, 0, 1); got != 500 {
		t.Errorf("Population(1000, 0.5, 0, 1) = %v, хочу 500", got)
	}
}

func TestRDeathThreshold(t *testing.T) {
	// Порог «p < 1 → поселение мёртвое» (99.2.12, §Решение).
	if got := Population(1, 0.00115, 0, 3600); got != 0 {
		t.Errorf("Population(1, 0.00115, 0, 3600) = %v, хочу 0 (p < 1)", got)
	}
	if got := Population(0.5, 0.00115, 0, 10); got != 0 {
		t.Errorf("Population(0.5, ...) = %v, хочу 0", got)
	}
}

func TestChangeComponentsModularSum(t *testing.T) {
	// Модульность (99.2.12, концепция создателя): изменение = сумма
	// компонент. λ-компоненты аддитивны (холод + гравитация + радиация
	// складываются в OtherChangeRate); рекурсивная компонента — сумма
	// естественной (NaturalComponent) и чисто температурной
	// (HeatTemperatureChangeRate).
	base := PlanetInput{TemperatureK: 150, GravityG: 4.533, CoreRadioactivity: 90}
	onlyCold := PlanetInput{TemperatureK: 150, GravityG: 1.0, CoreRadioactivity: 5}
	onlyGravity := PlanetInput{TemperatureK: 275, GravityG: 4.533, CoreRadioactivity: 5}
	onlyRad := PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 90}

	want := OtherChangeRate(onlyCold, DefaultScale) +
		OtherChangeRate(onlyGravity, DefaultScale) +
		OtherChangeRate(onlyRad, DefaultScale)
	if got := OtherChangeRate(base, DefaultScale); math.Abs(got-want) > 1e-12 {
		t.Errorf("λ-компоненты не складываются: вместе=%v, раздельно=%v", got, want)
	}

	// Рекурсивная компонента = естественная + температурная (жара, 300 °C).
	hot := PlanetInput{TemperatureK: 573.15, GravityG: 1.0, CoreRadioactivity: 5}
	r, lambdaPerHour := ChangeComponents(hot, DefaultScale)
	if lambdaPerHour != 0 {
		t.Errorf("ChangeComponents(300 °C): λ = %v, хочу 0 (жара в r)", lambdaPerHour)
	}
	wantR := NaturalComponent(573.15) + HeatTemperatureChangeRate(573.15)
	if math.Abs(r-wantR) > 1e-12 {
		t.Errorf("рекурсивная компонента не сумма: r=%v, natural+heat=%v", r, wantR)
	}
	if r != NaturalChangeRate+HeatTemperatureChangeRate(573.15) {
		t.Errorf("r(300 °C) = %v, хочу NaturalChangeRate + R_темп (естественная отделена)", r)
	}

	// Рекурсивная компонента: будущий рост — r < 0 → (1−r) > 1.
	if got := Population(1000, -0.01, 0, 3600); !(got > 1000) {
		t.Errorf("Population с r<0 (рост) = %v, хочу > 1000", got)
	}
}

func TestREtalonTimes(t *testing.T) {
	// «Полное вымирание» Земли (p0 = 8·10⁹): t = ln(p0)/|ln(1−R)| (99.2.12,
	// §Контрольные точки; допуск «порядок» ×≤5).
	p0 := 8e9
	cases := []struct {
		tempC float64
		want  float64 // сек
	}{
		{30, 1000 * 365.25 * 24 * 3600}, // ~1000 лет
		{70, 100 * 365.25 * 24 * 3600},  // ~100 лет
		{100, 2 * 24 * 3600},            // ~2 дня
		{200, 5 * 3600},                 // ~5 ч
		{500, 20 * 60},                  // ~20 мин
		{2000, 30},                      // ~30 сек
		{4000, 5},                       // ~5 сек
	}
	for _, tc := range cases {
		r := RecursiveChangeRate(tc.tempC + 273.15)
		got := math.Log(p0) / math.Abs(math.Log(1-r))
		if math.Abs(got-tc.want) > 4*tc.want {
			t.Errorf("t_вымир(+%v °C) = %v сек, хочу ≈ %v сек (порядок)", tc.tempC, got, tc.want)
		}
	}
}

func TestDeathMomentR(t *testing.T) {
	// Момент смерти (99.2.12, §«Момент смерти»): t = ln(p0)/|ln(1−R)| сек;
	// дата = created_at + t. После t население < 1 (мёртвое).
	p0, r := 1000.0, 0.00115
	want := math.Log(p0) / math.Abs(math.Log(1-r))
	got := DeathMomentSeconds(p0, r)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("DeathMomentSeconds = %v, хочу %v", got, want)
	}
	if p := Population(p0, r, 0, got+1); p != 0 {
		t.Errorf("Population после t_смерти+1 = %v, хочу 0 (мёртвое)", p)
	}
	if got := DeathMomentSeconds(1, r); got != 0 { // p0 = 1 — уже мёртвое
		t.Errorf("DeathMomentSeconds(1, ...) = %v, хочу 0", got)
	}
	if got := DeathMomentSeconds(1000, 0); !math.IsInf(got, 1) {
		t.Errorf("DeathMomentSeconds(1000, 0) = %v, хочу +Inf (не вымирают)", got)
	}
}

func TestUninhabitableR(t *testing.T) {
	// Витринный порог «t_смерти(p0) < 1 ч» (99.2.12, §uninhabitable):
	// для p0 = 10⁶ порог R > 1 − exp(−ln(p0)/3600) ≈ 3.8·10⁻³.
	input := comfortableInput()
	input.TemperatureK = 250 + 273.15 // R ≈ 0.0022 → t ≈ 1.7 ч → false
	if Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(+250 °C, 1e6) = true, хочу false")
	}
	input.TemperatureK = 310 + 273.15 // R ≈ 0.0042 → t ≈ 55 мин → true
	if !Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(+310 °C, 1e6) = false, хочу true")
	}
	input.TemperatureK = 450 // +177 °C, R ≈ 7.9e-4 → t ≈ 4.9 ч → false
	if Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(450 K, 1e6) = true, хочу false")
	}
	// Холод — по-прежнему +Inf за 100 K (временно).
	input.TemperatureK = 50
	if !Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(50 K, 1e6) = false, хочу true (холодный полюс)")
	}
}

func TestHardZeroCold(t *testing.T) {
	// Жёсткий ноль холода (99.2.12, H4, перекалибровка 14b): T ≤ 100 K →
	// λ = +Inf, население = 0 (эталон «−200 °C → минуты», симметрия жаре).
	input := comfortableInput()
	input.TemperatureK = 100
	if got := OtherChangeRate(input, DefaultScale); !math.IsInf(got, 1) {
		t.Errorf("OtherChangeRate(100 K) = %v, хочу +Inf", got)
	}
	input.TemperatureK = 101
	got := OtherChangeRate(input, DefaultScale)
	if math.IsInf(got, 1) || got <= 0 {
		t.Errorf("OtherChangeRate(101 K) = %v, хочу конечную и > 0", got)
	}
	if math.Abs(got-0.402) > 0.001 {
		t.Errorf("OtherChangeRate(101 K) = %v, хочу ≈ 0.402 (полюс B: sev 4.02 × 0.1)", got)
	}
}

func TestHardZeroColdAlsoKills(t *testing.T) {
	// Все холодные точки за жёстким нулём (99.2.12, §Решение): включая
	// −200 °C = 73 K (эталон «минуты») — население 0.
	input := comfortableInput()
	for _, temp := range []float64{73, 50, 0} {
		input.TemperatureK = temp
		if got := OtherChangeRate(input, DefaultScale); !math.IsInf(got, 1) {
			t.Errorf("OtherChangeRate(%v K) = %v, хочу +Inf", temp, got)
		}
	}
}
