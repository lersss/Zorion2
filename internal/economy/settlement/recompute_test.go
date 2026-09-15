package settlement

import (
	"math"
	"testing"
	"time"
)

func comfortableInput() PlanetInput {
	// 288 K (14.85 °C) — комфорт-ноль: холод 0 (T ≥ 288), рампа естественной
	// не включена (T < 15 °C), жара 0 — R_total = 0 (99.2.13).
	return PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5}
}

func hotInput() PlanetInput {
	// 365 K (+92 °C) — жара с R ≈ 7.3·10⁻⁵/сек (R-модель, 99.2.12):
	// за сутки убывает, но остаётся ≥ NDead («убывает, не мгновенно»).
	return PlanetInput{TemperatureK: 365, GravityG: 1.0, CoreRadioactivity: 5}
}

func TestRecomputeNoTimePassedIsUnchanged(t *testing.T) {
	now := time.Now()
	got := Recompute(hotInput(), 1000, now, now, now)
	if got != 1000 {
		t.Errorf("Recompute без прошедшего времени = %v, хочу 1000", got)
	}
}

func TestRecomputeClockWentBackwardsIsUnchanged(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	// since = now, now = earlier: отрицательный Δt не должен ничего менять.
	got := Recompute(hotInput(), 1000, now, earlier, now)
	if got != 1000 {
		t.Errorf("Recompute с отрицательным Δt = %v, хочу 1000 (без изменений)", got)
	}
}

func TestRecomputeComfortableIsUnchanged(t *testing.T) {
	since := time.Now().Add(-24 * 365 * time.Hour) // год назад
	now := time.Now()
	got := Recompute(comfortableInput(), 1_000_000, since, now, since)
	if got != 1_000_000 {
		t.Errorf("Recompute в комфорте изменил население: %v", got)
	}
}

func TestRecomputeHotPlanetDecreases(t *testing.T) {
	since := time.Now().Add(-24 * time.Hour)
	now := time.Now()
	got := Recompute(hotInput(), 1_000_000, since, now, since)
	if !(got < 1_000_000 && got > 0) {
		t.Errorf("Recompute на горячей планете = %v, хочу строго между 0 и 1_000_000", got)
	}
}

func TestRecomputeBelowNDeadCollapsesToZero(t *testing.T) {
	since := time.Now().Add(-24 * 365 * 10 * time.Hour) // 10 лет — заведомо ниже порога
	now := time.Now()
	got := Recompute(hotInput(), 1000, since, now, since)
	if got != 0 {
		t.Errorf("Recompute ниже NDead = %v, хочу 0 (разовый обвал)", got)
	}
}

// --- Естественная пара (класс B): СПЖ + рождаемость, потолка нет ---

func TestNaturalLife(t *testing.T) {
	// 30 °C (303.15 K): комфорт, естественная пара при дефолте k=2 даёт нетто
	// (1−2)·NCR = −NCR → рост (99.2.16): p(50 лет) ≈ p0·(1+NCR)^(50 лет)
	// ≈ 2.7·p0. Прежнее «0.37·p0» (чистая убыль) воспроизводится при k=0.
	natural30 := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	createdAt := time.Now().Add(-50 * 365.25 * 24 * time.Hour)
	now := time.Now()
	delta := now.Sub(createdAt).Seconds()

	got := Recompute(natural30, 1_000_000, createdAt, now, createdAt)
	wantGrowth := 1_000_000 * math.Pow(1+NaturalChangeRate(), delta)
	if math.Abs(got-wantGrowth) > 1 {
		t.Errorf("Recompute(50 лет, k=2) = %v, хочу ≈ %v (рост)", got, wantGrowth)
	}
	if math.Abs(got/1_000_000-2.7) > 0.03 {
		t.Errorf("p(50 лет)/p0 = %v, хочу ≈ 2.7 (рост +2%%/год)", got/1_000_000)
	}

	t.Cleanup(ResetPopulationSettings)
	if err := SetBirthRateCoefficient(0); err != nil {
		t.Fatalf("SetBirthRateCoefficient(0): %v", err)
	}
	gotDecay := Recompute(natural30, 1_000_000, createdAt, now, createdAt)
	wantDecay := 1_000_000 * math.Pow(1-NaturalChangeRate(), delta)
	if math.Abs(gotDecay-wantDecay) > 1 {
		t.Errorf("Recompute(50 лет, k=0) = %v, хочу ≈ %v (убыль)", gotDecay, wantDecay)
	}
	if math.Abs(gotDecay/1_000_000-0.37) > 0.01 {
		t.Errorf("p(50 лет)/p0 = %v, хочу ≈ 0.37 (убыль, k=0)", gotDecay/1_000_000)
	}
}

func TestComfortGrowth(t *testing.T) {
	// Рост в комфорте (99.2.16 §6.2 п.3): Recompute при k=2, 303.15 K,
	// Δt = 1 год → p ≈ p0·(1+NCR)^(сек_год) (+2%/год).
	input := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	p0 := 1_000_000.0
	since := time.Now().Add(-365.25 * 24 * time.Hour)
	now := time.Now()
	got := Recompute(input, p0, since, now, since)
	want := p0 * math.Pow(1+NaturalChangeRate(), now.Sub(since).Seconds())
	if math.Abs(got-want) > 1 {
		t.Errorf("Recompute(1 год, k=2) = %v, хочу ≈ %v", got, want)
	}
	if !(got > p0) {
		t.Errorf("Recompute(1 год, k=2) = %v, хочу > p0 (рост)", got)
	}
}

func TestAgeDoesNotMatter(t *testing.T) {
	// Возрастной потолок удалён (99.2.16 §6.2 п.4, на смену TestNaturalCap):
	// возраст 130 лет не обнуляет население ни при равновесии (288 K, r=0),
	// ни при росте (303.15 K, k=2).
	old := time.Now().Add(-130 * 365.25 * 24 * time.Hour)
	now := time.Now()
	if got := Recompute(comfortableInput(), 1e6, old, now, old); got != 1e6 {
		t.Errorf("возраст 130 лет, r=0: Recompute = %v, хочу p0", got)
	}
	input := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	if got := Recompute(input, 1e6, old, now, old); !(got > 1e6) {
		t.Errorf("возраст 130 лет, k=2: Recompute = %v, хочу рост (возраст не обнуляет)", got)
	}
}

func TestNaturalInvariance(t *testing.T) {
	// Инвариант пересчёта: два последовательных = один (потолок и экспонента
	// — функции абсолютного возраста/времени, а не числа пересчётов).
	createdAt := time.Now().Add(-100 * 365.25 * 24 * time.Hour)
	t1 := createdAt.Add(50 * 365.25 * 24 * time.Hour)
	t2 := createdAt.Add(100 * 365.25 * 24 * time.Hour)

	one := Recompute(comfortableInput(), 1e6, createdAt, t2, createdAt)
	step1 := Recompute(comfortableInput(), 1e6, createdAt, t1, createdAt)
	two := Recompute(comfortableInput(), step1, t1, t2, createdAt)
	if math.Abs(one-two) > 1e-6 {
		t.Errorf("инвариант нарушен: один пересчёт=%v, два=%v", one, two)
	}
}
