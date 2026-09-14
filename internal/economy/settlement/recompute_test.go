package settlement

import (
	"math"
	"testing"
	"time"
)

func comfortableInput() PlanetInput {
	return PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 5}
}

func hotInput() PlanetInput {
	// 365 K (+92 °C) — жара с R ≈ 7.3·10⁻⁵/сек (R-модель, 99.2.12):
	// за сутки убывает, но остаётся ≥ NDead («убывает, не мгновенно»).
	return PlanetInput{TemperatureK: 365, GravityG: 1.0, CoreRadioactivity: 5}
}

func TestRecomputeNoTimePassedIsUnchanged(t *testing.T) {
	now := time.Now()
	got := Recompute(hotInput(), DefaultScale, 1000, now, now, now)
	if got != 1000 {
		t.Errorf("Recompute без прошедшего времени = %v, хочу 1000", got)
	}
}

func TestRecomputeClockWentBackwardsIsUnchanged(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	// since = now, now = earlier: отрицательный Δt не должен ничего менять.
	got := Recompute(hotInput(), DefaultScale, 1000, now, earlier, now)
	if got != 1000 {
		t.Errorf("Recompute с отрицательным Δt = %v, хочу 1000 (без изменений)", got)
	}
}

func TestRecomputeComfortableIsUnchanged(t *testing.T) {
	since := time.Now().Add(-24 * 365 * time.Hour) // год назад
	now := time.Now()
	got := Recompute(comfortableInput(), DefaultScale, 1_000_000, since, now, since)
	if got != 1_000_000 {
		t.Errorf("Recompute в комфорте изменил население: %v", got)
	}
}

func TestRecomputeHotPlanetDecreases(t *testing.T) {
	since := time.Now().Add(-24 * time.Hour)
	now := time.Now()
	got := Recompute(hotInput(), DefaultScale, 1_000_000, since, now, since)
	if !(got < 1_000_000 && got > 0) {
		t.Errorf("Recompute на горячей планете = %v, хочу строго между 0 и 1_000_000", got)
	}
}

func TestRecomputeBelowNDeadCollapsesToZero(t *testing.T) {
	since := time.Now().Add(-24 * 365 * 10 * time.Hour) // 10 лет — заведомо ниже порога
	now := time.Now()
	got := Recompute(hotInput(), DefaultScale, 1000, since, now, since)
	if got != 0 {
		t.Errorf("Recompute ниже NDead = %v, хочу 0 (разовый обвал)", got)
	}
}

// --- Естественная смертность (класс B): СПЖ 50 лет + возрастной потолок 120 ---

func TestNaturalLife(t *testing.T) {
	// 30 °C (303.15 K): только естественная компонента R_ест = 1/СПЖ 50 лет
	// (при 275 K комфорт, R=0 — естественная смертность включается рампой
	// с 15 °C). p(50 лет) ≈ p0·(1−R_ест)^(50 лет) ≈ 0.37·p0.
	natural30 := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	createdAt := time.Now().Add(-50 * 365.25 * 24 * time.Hour)
	now := time.Now()
	delta := now.Sub(createdAt).Seconds()
	got := Recompute(natural30, DefaultScale, 1_000_000, createdAt, now, createdAt)
	want := 1_000_000 * math.Pow(1-NaturalChangeRate, delta)
	if math.Abs(got-want) > 1 {
		t.Errorf("Recompute(50 лет) = %v, хочу ≈ %v (≈0.37·p0, СПЖ 50 лет)", got, want)
	}
	if math.Abs(got/1_000_000-0.37) > 0.01 {
		t.Errorf("p(50 лет)/p0 = %v, хочу ≈ 0.37", got/1_000_000)
	}
	// Полное вымирание при 30 °C — потолок 120 лет для ЛЮБОГО p0
	// (раньше было «~1000 лет», пересмотрено на «полное ≤ 120», 99.2.12).
	for _, p0 := range []float64{1e3, 1e9} {
		old := time.Now().Add(-120 * 365.25 * 24 * time.Hour)
		if got := Recompute(natural30, DefaultScale, p0, old, time.Now(), old); got != 0 {
			t.Errorf("полное вымирание при 30 °C (p0=%v) = %v, хочу 0 (потолок 120 лет)", p0, got)
		}
	}
}

func TestNaturalCap(t *testing.T) {
	// Возрастной потолок: age ≥ 120 лет → население 0; age < 120 → живёт.
	old := time.Now().Add(-130 * 365.25 * 24 * time.Hour)
	if got := Recompute(comfortableInput(), DefaultScale, 1e6, old, time.Now(), old); got != 0 {
		t.Errorf("возраст 130 лет = %v, хочу 0 (потолок 120 лет)", got)
	}
	age110 := time.Now().Add(-110 * 365.25 * 24 * time.Hour)
	if got := Recompute(comfortableInput(), DefaultScale, 1e6, age110, time.Now(), age110); got <= 0 {
		t.Errorf("возраст 110 лет = %v, хочу > 0", got)
	}
}

func TestNaturalInvariance(t *testing.T) {
	// Инвариант пересчёта: два последовательных = один (потолок и экспонента
	// — функции абсолютного возраста/времени, а не числа пересчётов).
	createdAt := time.Now().Add(-100 * 365.25 * 24 * time.Hour)
	t1 := createdAt.Add(50 * 365.25 * 24 * time.Hour)
	t2 := createdAt.Add(100 * 365.25 * 24 * time.Hour)

	one := Recompute(comfortableInput(), DefaultScale, 1e6, createdAt, t2, createdAt)
	step1 := Recompute(comfortableInput(), DefaultScale, 1e6, createdAt, t1, createdAt)
	two := Recompute(comfortableInput(), DefaultScale, step1, t1, t2, createdAt)
	if math.Abs(one-two) > 1e-6 {
		t.Errorf("инвариант нарушен: один пересчёт=%v, два=%v", one, two)
	}
}
