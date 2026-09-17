package settlement

import (
	"encoding/json"
	"math"
	"testing"
)

// rTotal — полная рекурсивная компонента для тестов жары/естественной
// (комфортные гравитация и радиация: вклад = 0).
func rTotal(tempK float64) float64 {
	return ChangeComponents(PlanetInput{TemperatureK: tempK, GravityG: 1, CoreRadioactivity: 5})
}

func TestRCurve(t *testing.T) {
	// R(T) по двухрежимной сшивке в контрольных точках (99.2.12, §Критерии
	// готовности; допуск ±50% — точки критерий тестера, не узлы). С
	// рождаемостью (99.2.16, дефолт k=2) в комфорте 30 °C нетто −NCR (рост),
	// поэтому ожидание со знаком; точки 70..4000 °C в допусках остаются
	// (вклад −NCR·1 = −0.63e-9 съедается −k·NCR, жара доминирует).
	cases := []struct {
		tempC float64
		want  float64
	}{
		{30, -6.34e-10},
		{70, 6.6e-9},
		{100, 1.16e-4},
		{200, 1.15e-3},
		{500, 0.016},
		{2000, 0.47},
		{4000, 0.98},
	}
	for _, tc := range cases {
		got := rTotal(tc.tempC + 273.15)
		if math.Abs(got-tc.want) > 0.5*math.Abs(tc.want) {
			t.Errorf("R(%v °C) = %v, хочу ≈ %v (±50%%)", tc.tempC, got, tc.want)
		}
	}
	// Комфорт и рампа: 288 K → 0; 303 K → ≈ −R_ест (рост при k=2);
	// 295 K → ≈ −R_ест·7/15.
	if got := rTotal(288); got != 0 {
		t.Errorf("R(288 K) = %v, хочу 0", got)
	}
	if got := rTotal(303); math.Abs(got+NaturalChangeRate()) > 0.02*NaturalChangeRate() {
		t.Errorf("R(303 K) = %v, хочу ≈ −R_ест (±2%%)", got)
	}
	ramp := rTotal(295) // 21.85 °C → −R_ест·6.85/15; ориентир «7/15» ≈ 22 °C
	want := -NaturalChangeRate() * (295 - 273.15 - 15) / 15
	if math.Abs(ramp-want) > 1e-12 {
		t.Errorf("R(295 K) = %v, хочу %v", ramp, want)
	}
	if math.Abs(ramp+NaturalChangeRate()*7/15) > 0.05*NaturalChangeRate()*7/15 {
		t.Errorf("R(295 K) = %v, хочу ≈ −R_ест·7/15 (±5%%)", ramp)
	}
	// Модульность: естественная и температурная компоненты раздельны —
	// R_жара(30 °C) = 0, NaturalComponent(30 °C) = R_ест (99.2.16:
	// рождаемость — отдельный член BirthComponent).
	if got := NaturalComponent(30 + 273.15); got != NaturalChangeRate() {
		t.Errorf("NaturalComponent(30 °C) = %v, хочу %v", got, NaturalChangeRate())
	}
	if got := HeatTemperatureChangeRate(30 + 273.15); got != 0 {
		t.Errorf("R_жара(30 °C) = %v, хочу 0 (температурная компонента без естественной)", got)
	}
	if got := HeatTemperatureChangeRate(30.1 + 273.15); got <= 0 {
		t.Errorf("R_жара(30.1 °C) = %v, хочу > 0", got)
	}
}

func TestRMonotonic(t *testing.T) {
	// R монотонно растёт на [30, 4000] °C и всегда < 1 (99.2.12, §Решение).
	// На [15, 30) °C с рождаемостью (k=2) r < 0 (рост) — монотонность
	// проверяется по абсолютной величине убыли (99.2.16 §6.1).
	var prev float64
	for i, tempC := range []float64{30, 50, 70, 90, 100, 200, 300, 500, 1000, 2000, 4000} {
		r := rTotal(tempC + 273.15)
		if r >= 1 {
			t.Errorf("R(%v °C) = %v, хочу < 1", tempC, r)
		}
		if i > 0 && !(r > prev) {
			t.Errorf("R не монотонна: %v °C → %v (было %v)", tempC, r, prev)
		}
		prev = r
	}
	// [15, 30) °C: рост, |r| монотонно растёт (рампа смертности × (1−k)).
	var prevAbs float64
	for i, tempC := range []float64{20, 25, 29} {
		r := rTotal(tempC + 273.15)
		if !(r < 0) {
			t.Errorf("R(%v °C) = %v, хочу < 0 (рост при k=2)", tempC, r)
		}
		if i > 0 && !(math.Abs(r) > prevAbs) {
			t.Errorf("|R| не монотонна на рампе: %v °C → %v (было %v)", tempC, math.Abs(r), prevAbs)
		}
		prevAbs = math.Abs(r)
	}
}

func TestRSpliceContinuity(t *testing.T) {
	// Стык зон жары (99.2.12, §Стык зон). С 99.2.17 компоненты жары —
	// сегментные кривые из balancer store (дефолты подогнаны под формулу,
	// TestSegmentApproximation): непрерывность обеспечивается математикой
	// сегментов (на узлах t_k = 0/1), скачок на стыке 84–90 °C отсутствует.
	// Естественная компонента отделена: R_total(90) = −NCR + R_жара(90).
	if got := NaturalComponent(90 + 273.15); got != NaturalChangeRate() {
		t.Errorf("NaturalComponent(90 °C) = %v, хочу %v", got, NaturalChangeRate())
	}
	// R_жара(90 °C) = узел кривой (дефолт = формула 3.91e-5, допуск §9).
	nodes, bends, ok := GetCurve("heat")
	if !ok {
		t.Fatal("GetCurve(heat): !ok")
	}
	if r90 := evaluateCurve(nodes, bends, 90); math.Abs(r90-3.91e-5) > 0.05*3.91e-5 {
		t.Errorf("R_жара(90) = %v, хочу ≈ 3.91e-5 (±5%%)", r90)
	}
	// Шаг 89→91 °C конечен и мал относительно уровня тепловой зоны (кривая
	// непрерывна — разрыва нет).
	step := math.Abs(rTotal(91+273.15) - rTotal(89+273.15))
	if step > 0.5*rTotal(100+273.15) {
		t.Errorf("разрыв на стыке 89→91 °C: шаг %v > 50%% R(100)", step)
	}
}

func TestRPopulation(t *testing.T) {
	// Дискретная рекурсия: p = p0·(1−r)^Δt_сек (99.2.12, §Решение).
	want := 1000 * math.Pow(0.99885, 3600) // (1−0.00115)^3600
	if got := Population(1000, 0.00115, 3600); math.Abs(got-want) > 1e-6 {
		t.Errorf("Population(1000, 0.00115, 3600) = %v, хочу %v", got, want)
	}
	if got := Population(1000, 0.00115, 0); got != 1000 {
		t.Errorf("Population(1000, r, 0) = %v, хочу 1000 (чек-точка не движется)", got)
	}
	if got := Population(1000, 0.5, 1); got != 500 {
		t.Errorf("Population(1000, 0.5, 1) = %v, хочу 500", got)
	}
}

func TestRDeathThreshold(t *testing.T) {
	// Порог «p < 1 → поселение мёртвое» (99.2.12, §Решение).
	if got := Population(1, 0.00115, 3600); got != 0 {
		t.Errorf("Population(1, 0.00115, 3600) = %v, хочу 0 (p < 1)", got)
	}
	if got := Population(0.5, 0.00115, 10); got != 0 {
		t.Errorf("Population(0.5, ...) = %v, хочу 0", got)
	}
}

func TestRGuardCap(t *testing.T) {
	// Гвард робастности (99.2.13): суммарный r ≥ 1 → мгновенная гибель p = 0,
	// без NaN (pow(1−r, Δt) с основанием ≤ 0 не вызывается); рост r < 0
	// разрешён. С 99.2.17 экстремальная гравитация больше НЕ даёт r ≥ 1:
	// сегментные кривые экстраполируются горизонтально (константа крайнего
	// узла, §2 «Режем по корню») — R_гравитация(1000 g) = 1.93e-3, без
	// переполнения степенной.
	if got := Population(1e6, 1.5, 60); got != 0 {
		t.Errorf("Population с r ≥ 1 = %v, хочу 0 (мгновенная гибель, без NaN)", got)
	}
	if got := Population(1000, -0.1, 10); !(got > 1000) {
		t.Errorf("Population с r<0 (рост) = %v, хочу > 1000 (рост не обрезается)", got)
	}
	// Экстремальная гравитация: конечная R (не +Inf, не NaN), r < 1 — гвард
	// не срабатывает (горизонтальная экстраполяция), Population считает.
	input := PlanetInput{TemperatureK: 275, GravityG: 1000, CoreRadioactivity: 5}
	r := ChangeComponents(input)
	if math.IsInf(r, 1) || math.IsNaN(r) || r >= 1 {
		t.Fatalf("экстремальная g: r = %v, хочу конечную < 1 (горизонтальная экстраполяция)", r)
	}
	if got := Population(1e6, r, 60); got <= 0 {
		t.Errorf("Population с экстремальной g = %v, хочу > 0 (убыль конечная)", got)
	}
}

// Тест 16 (§14, поправки В1+В2): гвард переполнения роста — расчётный кламп
// MaxPopulation = 1e300 (не +Inf, json.Marshal не падает); при малом Δt —
// обычный рост ×54.9 без клампа.
func TestPopulationOverflowClamp(t *testing.T) {
	// Population(10⁹, −1.27·10⁻⁷, Δt ≈ 5.3·10⁹ с) (≈167 лет) → 1e300, не +Inf.
	got := Population(1e9, -1.27e-7, 5.3e9)
	if got != MaxPopulation {
		t.Errorf("Population(1e9, −1.27e-7, 5.3e9) = %v, хочу кламп %v", got, MaxPopulation)
	}
	if math.IsInf(got, 1) || math.IsNaN(got) {
		t.Fatalf("результат = %v, хочу конечный (не +Inf/NaN)", got)
	}
	if _, err := json.Marshal(got); err != nil {
		t.Errorf("json.Marshal результата падает: %v", err)
	}

	// При малом Δt (1 год) — обычный рост ×54.9 без клампа.
	year := 365.0 * 24 * 3600
	got = Population(1e9, -1.27e-7, year)
	want := 1e9 * 54.9
	if math.Abs(got-want) > 0.01*want {
		t.Errorf("Population(1e9, −1.27e-7, 1 год) = %v, хочу ≈ %v (×54.9/год)", got, want)
	}
}

func TestREtalonTimes(t *testing.T) {
	// «Полное вымирание» Земли (p0 = 8·10⁹): t = ln(p0)/|ln(1−R)| (99.2.12,
	// §Контрольные точки; допуск «порядок» ×≤5). Строка +30 °C убрана
	// (99.2.16): в комфорте при k=2 — рост, а не вымирание (см.
	// TestComfortGrowth в recompute_test.go).
	p0 := 8e9
	cases := []struct {
		tempC float64
		want  float64 // сек
	}{
		{70, 100 * 365.25 * 24 * 3600}, // ~100 лет
		{100, 2 * 24 * 3600},           // ~2 дня
		{200, 5 * 3600},                // ~5 ч
		{500, 20 * 60},                 // ~20 мин
		{2000, 30},                     // ~30 сек
		{4000, 5},                      // ~5 сек
	}
	for _, tc := range cases {
		r := rTotal(tc.tempC + 273.15)
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
	if p := Population(p0, r, got+1); p != 0 {
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
	// Витринный порог «t_смерти(p0) < 1 ч» (99.2.12, §uninhabitable): для
	// p0 = 10⁶ порог r > 1 − exp(−ln(p0)/3600) ≈ 3.8·10⁻³. Жёстких нулей
	// больше нет (99.2.13) — холод/гравитация дают конечные R.
	input := comfortableInput()
	input.TemperatureK = 250 + 273.15 // жара ≈ 0.0022 → t ≈ 1.7 ч → false
	if Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(+250 °C, 1e6) = true, хочу false")
	}
	input.TemperatureK = 310 + 273.15 // жара ≈ 0.0042 → t ≈ 55 мин → true
	if !Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(+310 °C, 1e6) = false, хочу true")
	}
	input.TemperatureK = 450 // +177 °C, жара ≈ 7.9e-4 → t ≈ 4.9 ч → false
	if Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(450 K, 1e6) = true, хочу false")
	}
	// Холод (50 K): R_холод ≈ 0.0206 → t ≈ 11 мин → true (без +Inf, 99.2.13).
	input.TemperatureK = 50
	if !Uninhabitable(input, 1e6) {
		t.Errorf("Uninhabitable(50 K, 1e6) = false, хочу true (R_холод ≈ 0.02)")
	}
}

func TestPopulationNoLambdaIsConstant(t *testing.T) {
	p0 := 1000.0
	got := Population(p0, 0, 24*365*3600)
	if got != p0 {
		t.Errorf("Population без угрозы изменилось: %v != %v", got, p0)
	}
}

// Урок отброшенного пробника (docs/gamedesign/ideas/13b...md, п.6): большое
// население не должно давать иммунитет к среде — доля убыли одинакова
// независимо от размера поселения.
func TestPopulationNoImmunityFromSize(t *testing.T) {
	// Оба размера остаются ≥ 1 (порог «p < 1» не мешает): r = 0.1, Δt = 10 сек.
	deltaSeconds := 10.0
	r := 0.1

	small := Population(100, r, deltaSeconds)
	large := Population(1_000_000, r, deltaSeconds)

	fractionSmall := small / 100
	fractionLarge := large / 1_000_000
	if math.Abs(fractionSmall-fractionLarge) > 1e-9 {
		t.Errorf("доля выживших зависит от размера: маленькое=%v большое=%v", fractionSmall, fractionLarge)
	}
}

func TestPopulationHalfLife(t *testing.T) {
	p0 := 1000.0
	r := 0.1
	halfLife := math.Ln2 / math.Abs(math.Log(1-r)) // p0·(1−r)^t = p0/2

	got := Population(p0, r, halfLife)
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
	if got := Population(0, 0.5, 10); got != 0 {
		t.Errorf("Population(0, ...) = %v, хочу 0", got)
	}
}
