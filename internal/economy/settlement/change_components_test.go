// Тесты компонент изменения населения (99.2.13: холод, гравитация, радиация
// в R-модели; допуск «порядок»/±50% — контрольные точки критерий тестера).
package settlement

import (
	"math"
	"testing"
)

func TestColdRCurve(t *testing.T) {
	// R_холод по контрольным точкам (99.2.13, §Контрольные точки; ±50%).
	cases := []struct {
		tempK float64
		want  float64
	}{
		{288, 0},
		{223, 2.44e-5},
		{173, 3.50e-5},
		{150, 3.92e-5}, // сшивка: σ(150) = 0.5, R_ум(150) = R_кр(150)
		{73, 0.0117},
		{23, 0.0349},
	}
	for _, tc := range cases {
		got := ColdChangeRate(tc.tempK)
		if math.Abs(got-tc.want) > 0.5*tc.want+1e-12 {
			t.Errorf("R_холод(%v K) = %v, хочу ≈ %v (±50%%)", tc.tempK, got, tc.want)
		}
	}
}

func TestColdRMonotonic(t *testing.T) {
	// R_холод монотонно растёт при охлаждении (сегментная кривая из balancer
	// store, 99.2.17: дефолты = формула в пределах допуска §9); нет NaN.
	var prev float64
	for i, tempK := range []float64{288, 273, 223, 173, 150, 123, 100, 73, 50, 23} {
		r := ColdChangeRate(tempK)
		if math.IsNaN(r) {
			t.Fatalf("R_холод(%v K) = NaN", tempK)
		}
		if i > 0 && !(r > prev) {
			t.Errorf("R_холод не монотонна: %v K → %v (было %v)", tempK, r, prev)
		}
		prev = r
	}
}

// TestHotPathNoAllocs — хот-пат (мини-R из Recompute на каждый просмотр
// поселения, 36a Пункт 1) не должен копировать кривые на каждый вызов:
// раньше каждая из 4 компонент звала GetCurve → RLock + 2 копии слайсов
// (узлы + изгибы) = 8 аллокаций/проход. Теперь — ссылка на неизменную
// кривую store, аллокаций нет.
func TestHotPathNoAllocs(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		HeatTemperatureChangeRate(473.15)
		ColdChangeRate(73)
		GravityChangeRate(2)
		RadiationChangeRate(80)
	})
	if allocs >= 1 {
		t.Fatalf("хот-пат аллоцирует %.2f объектов/проход, хочу 0 (копии кривых устранены)", allocs)
	}
}

// TestMiniRWatchdogs — сторожевые тесты формул (спека 99.2.17 §9): быстрое
// красное, если кто-то вернёт аналитическую формулу вместо кривой (или
// сломает конверсию единиц / имя компоненты). Допуск ±20% (кривая держит
// формулу в 5%, TestSegmentApproximation; запас на округления).
func TestMiniRWatchdogs(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"HeatTemperatureChangeRate(473.15 K = 200 °C)", HeatTemperatureChangeRate(473.15), 1.15e-3},
		{"ColdChangeRate(73 K)", ColdChangeRate(73), 0.0117},
		{"GravityChangeRate(2 g)", GravityChangeRate(2), 4.8e-5},
		{"RadiationChangeRate(80 rad)", RadiationChangeRate(80), 1.3e-4},
	}
	for _, tc := range cases {
		if math.Abs(tc.got-tc.want) > 0.2*tc.want {
			t.Errorf("%s = %v, хочу ≈ %v (±20%%)", tc.name, tc.got, tc.want)
		}
	}
}

func TestGravityRCurve(t *testing.T) {
	// R_гравитация по контрольным точкам (99.2.13, §Контрольные точки; ±50%).
	cases := []struct {
		g    float64
		want float64
	}{
		{0.8, 0},
		{1.0, 0},
		{1.2, 0},
		{0.3, 2.19e-8},
		{0.1, 6.6e-8},
		{2, 4.8e-5},
		{3, 1.2e-4},
		{5, 4.8e-4},
		{10, 1.92e-3},
	}
	for _, tc := range cases {
		got := GravityChangeRate(tc.g)
		if math.Abs(got-tc.want) > 0.5*tc.want+1e-12 {
			t.Errorf("R_гравитация(%v g) = %v, хочу ≈ %v (±50%%)", tc.g, got, tc.want)
		}
	}
}

func TestGravityRBothSides(t *testing.T) {
	// Двусторонность: обе ветки вне комфорта, комфорт — 0; невесомость
	// медленнее перегрузки.
	if got := GravityChangeRate(1.0); got != 0 {
		t.Errorf("R_гравитация(1.0) = %v, хочу 0 (комфорт)", got)
	}
	if got := GravityChangeRate(0.5); got <= 0 {
		t.Errorf("R_гравитация(0.5) = %v, хочу > 0 (невесомость)", got)
	}
	if got := GravityChangeRate(1.5); got <= 0 {
		t.Errorf("R_гравитация(1.5) = %v, хочу > 0 (перегрузка)", got)
	}
	if !(GravityChangeRate(0.5) < GravityChangeRate(5)) {
		t.Errorf("невесомость должна быть медленнее перегрузки")
	}
}

func TestRadiationRCurve(t *testing.T) {
	// R_радиация по контрольным точкам (99.2.13; порог 20 — фон; ±50%).
	cases := []struct {
		rad  float64
		want float64
	}{
		{20, 0},
		{5, 0},
		{40, 6.7e-8},
		{60, 8.0e-6},
		{80, 1.3e-4},
		{100, 9.5e-4},
	}
	for _, tc := range cases {
		got := RadiationChangeRate(tc.rad)
		if math.Abs(got-tc.want) > 0.5*tc.want+1e-12 {
			t.Errorf("R_радиация(%v) = %v, хочу ≈ %v (±50%%)", tc.rad, got, tc.want)
		}
	}
}

func TestNoHardZeroOther(t *testing.T) {
	// Порог-переключатель убран (99.2.13): экстремальные входы дают конечную
	// R (не +Inf): 10 g, 73 K, rad 100.
	hot := ChangeComponents(PlanetInput{TemperatureK: 275, GravityG: 10, CoreRadioactivity: 5})
	if !(hot > 0) || math.IsInf(hot, 1) {
		t.Errorf("R_total(10 g) = %v, хочу конечную > 0", hot)
	}
	cold := ChangeComponents(PlanetInput{TemperatureK: 73, GravityG: 1, CoreRadioactivity: 5})
	if !(cold > 0) || math.IsInf(cold, 1) {
		t.Errorf("R_total(73 K) = %v, хочу конечную > 0", cold)
	}
	rad := ChangeComponents(PlanetInput{TemperatureK: 275, GravityG: 1, CoreRadioactivity: 100})
	if !(rad > 0) || math.IsInf(rad, 1) {
		t.Errorf("R_total(rad 100) = %v, хочу конечную > 0", rad)
	}
}

func TestTotalRSum(t *testing.T) {
	// Модульность (99.2.13 + 99.2.16): R_total = R_ест + R_рожд + R_жара +
	// R_холод + R_гравитация + R_радиация — декомпозиция: каждый фактор
	// добавляет свою компоненту. Рождаемость (дефолт k=2) при 30 °C даёт
	// нетто (1−2)·NCR = −NCR (рост).
	base := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5} // 30 °C: только естественная пара
	if got := ChangeComponents(base); math.Abs(got+NaturalChangeRate()) > 1e-12 {
		t.Errorf("комфорт по всем факторам: r = %v, хочу −R_ест %v (k=2)", got, NaturalChangeRate())
	}

	withCold := base
	withCold.TemperatureK = 173 // R_холод ≈ 3.50e-5; естественная пара = 0 (T < 15 °C)
	if got := ChangeComponents(withCold); math.Abs(got-3.50e-5) > 0.5*3.50e-5 {
		t.Errorf("+холод: r = %v, хочу R_холод", got)
	}

	withGravity := base
	withGravity.GravityG = 3 // R_гравитация ≈ 1.2e-4
	if got := ChangeComponents(withGravity); math.Abs(got+NaturalChangeRate()-1.2e-4) > 0.5*1.2e-4 {
		t.Errorf("+гравитация: r = %v, хочу −R_ест + R_гравитация", got)
	}

	withRad := base
	withRad.CoreRadioactivity = 80 // R_радиация ≈ 1.3e-4
	if got := ChangeComponents(withRad); math.Abs(got+NaturalChangeRate()-1.3e-4) > 0.5*1.3e-4 {
		t.Errorf("+радиация: r = %v, хочу −R_ест + R_радиация", got)
	}

	all := PlanetInput{TemperatureK: 473.15, GravityG: 3, CoreRadioactivity: 80} // жара ≈ 1.15e-3
	wantAll := ChangeComponents(all)
	if !(wantAll > 1.2e-4+1.3e-4) {
		t.Errorf("сумма факторов должна быть больше каждого по отдельности: %v", wantAll)
	}
}

func TestBirthComponent(t *testing.T) {
	// Рампа рождаемости = рампа смертности (99.2.16 §2.2): 30 °C → −k·NCR;
	// 295 K → −k·NCR·6.85/15; 288.15 K (15 °C) → 0; 288 K → 0.
	k := BirthRateCoefficient()
	if got := BirthComponent(303.15); math.Abs(got+k*NaturalChangeRate()) > 1e-12 {
		t.Errorf("BirthComponent(30 °C) = %v, хочу −k·NCR %v", got, -k*NaturalChangeRate())
	}
	want295 := -k * NaturalChangeRate() * (295 - 273.15 - 15) / 15
	if got := BirthComponent(295); math.Abs(got-want295) > 1e-12 {
		t.Errorf("BirthComponent(295 K) = %v, хочу %v", got, want295)
	}
	if got := BirthComponent(288.15); got != 0 {
		t.Errorf("BirthComponent(15 °C) = %v, хочу 0", got)
	}
	if got := BirthComponent(288); got != 0 {
		t.Errorf("BirthComponent(288 K) = %v, хочу 0", got)
	}
}

func TestNettoRatesByK(t *testing.T) {
	// Нетто естественной пары при 30 °C (комфорт по прочим) (99.2.16 §2.3):
	// k=0 → +NCR (чистая убыль); k=1 → 0 (равновесие); k=2 → −NCR (рост).
	t.Cleanup(ResetPopulationSettings)
	input := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	for _, tc := range []struct {
		k    float64
		want float64
	}{
		{0, NaturalChangeRate()},
		{1, 0},
		{2, -NaturalChangeRate()},
	} {
		if err := SetBirthRateCoefficient(tc.k); err != nil {
			t.Fatalf("SetBirthRateCoefficient(%v): %v", tc.k, err)
		}
		if got := ChangeComponents(input); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("k=%v: r = %v, хочу %v", tc.k, got, tc.want)
		}
	}
}

func TestTotalRSumWithBirth(t *testing.T) {
	// Модульность с рождаемостью (99.2.16 §6.2 п.11): r = Natural + Birth +
	// heat + cold + grav + rad — рождаемость отдельный член, не вшита в
	// смертность (декомпозиция 99.2.13 сохранена).
	input := PlanetInput{TemperatureK: 365, GravityG: 1.0, CoreRadioactivity: 5} // 91.85 °C: жара ≈ 7.3e-5
	got := ChangeComponents(input)
	want := NaturalComponent(input.TemperatureK) + BirthComponent(input.TemperatureK) +
		HeatTemperatureChangeRate(input.TemperatureK) + ColdChangeRate(input.TemperatureK) +
		GravityChangeRate(input.GravityG) + RadiationChangeRate(input.CoreRadioactivity)
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("r = %v, хочу сумму компонент %v", got, want)
	}
	// Рождаемость — отрицательный член: в комфорте (30 °C, k=2) нетто < 0.
	if got := ChangeComponents(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}); got >= 0 {
		t.Errorf("комфорт 30 °C при k=2: r = %v, хочу < 0 (рост)", got)
	}
}

func TestNoBirthBelow15C(t *testing.T) {
	// Ниже 15 °C рождаемость не включается (99.2.16 §2.2): там холод,
	// естественной пары нет ни по смертности, ни по рождаемости.
	if got := BirthComponent(288.15); got != 0 {
		t.Errorf("BirthComponent(288.15 K) = %v, хочу 0", got)
	}
	if got := ChangeComponents(PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5}); got != 0 {
		t.Errorf("ChangeComponents(288 K, k=2) = %v, хочу 0", got)
	}
}

func TestREtalonCold(t *testing.T) {
	// «−200 °C → минуты» (99.2.13): t_вымир(1e9) при 73 K ≈ 30 мин (порядок).
	p0 := 1e9
	r := ColdChangeRate(73)
	got := math.Log(p0) / math.Abs(math.Log(1-r))
	want := 30 * 60.0 // ~30 мин
	if math.Abs(got-want) > 4*want {
		t.Errorf("t_вымир(73 K) = %v сек, хочу ≈ %v сек (порядок)", got, want)
	}
}
