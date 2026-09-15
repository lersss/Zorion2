// Тесты настроек рождаемости/СПЖ (спека 99.2.16 §3, §6.2): валидация
// диапазонов (без клампа), round-trip, helper нетто-темпа для UI.
package settlement

import (
	"math"
	"testing"
)

func TestSettingsValidation(t *testing.T) {
	t.Cleanup(ResetPopulationSettings)
	// СПЖ: 1 и 500 — ок (границы включены); 0.5 и 501 — ошибка (§3.3).
	for _, y := range []float64{1, 500} {
		if err := SetLifeExpectancyYears(y); err != nil {
			t.Errorf("SetLifeExpectancyYears(%v) = %v, хочу nil", y, err)
		}
	}
	for _, y := range []float64{0.5, 501} {
		if err := SetLifeExpectancyYears(y); err == nil {
			t.Errorf("SetLifeExpectancyYears(%v) = nil, хочу ошибку", y)
		}
	}
	// k: 0 и 10 — ок; −1 и 11 — ошибка.
	for _, k := range []float64{0, 10} {
		if err := SetBirthRateCoefficient(k); err != nil {
			t.Errorf("SetBirthRateCoefficient(%v) = %v, хочу nil", k, err)
		}
	}
	for _, k := range []float64{-1, 11} {
		if err := SetBirthRateCoefficient(k); err == nil {
			t.Errorf("SetBirthRateCoefficient(%v) = nil, хочу ошибку", k)
		}
	}
	// Значения не меняются при ошибке (без клампа — ошибка админа видима).
	if err := SetLifeExpectancyYears(80); err != nil {
		t.Fatalf("SetLifeExpectancyYears(80): %v", err)
	}
	_ = SetLifeExpectancyYears(501) // ошибка
	if got := LifeExpectancyYears(); got != 80 {
		t.Errorf("СПЖ после ошибочной записи = %v, хочу 80 (не меняется)", got)
	}
	if err := SetBirthRateCoefficient(3); err != nil {
		t.Fatalf("SetBirthRateCoefficient(3): %v", err)
	}
	_ = SetBirthRateCoefficient(-1) // ошибка
	if got := BirthRateCoefficient(); got != 3 {
		t.Errorf("k после ошибочной записи = %v, хочу 3 (не меняется)", got)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	t.Cleanup(ResetPopulationSettings)
	// СПЖ 80 лет → NaturalChangeRate() = 1/(80·365.25·24·3600);
	// k=3 → ChangeComponents(30 °C) = (1−3)·NCR(80 лет) (§6.2 п.10).
	if err := SetLifeExpectancyYears(80); err != nil {
		t.Fatalf("SetLifeExpectancyYears(80): %v", err)
	}
	wantNCR := 1.0 / (80 * 365.25 * 24 * 3600)
	if got := NaturalChangeRate(); math.Abs(got-wantNCR) > 1e-12 {
		t.Errorf("NaturalChangeRate() при СПЖ 80 = %v, хочу %v", got, wantNCR)
	}
	if err := SetBirthRateCoefficient(3); err != nil {
		t.Fatalf("SetBirthRateCoefficient(3): %v", err)
	}
	input := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	want := (1 - 3) * wantNCR
	if got := ChangeComponents(input); math.Abs(got-want) > 1e-12 {
		t.Errorf("ChangeComponents(30 °C, k=3, СПЖ 80) = %v, хочу %v", got, want)
	}
}

func TestNettoPerYear(t *testing.T) {
	// Helper для UI (99.2.16 §6.2 п.12): (1−k)/СПЖ_лет × 100 = −2.0 при
	// дефолтах (СПЖ 50, k=2); знак минус = рост.
	if got := NettoPerYearPercent(); math.Abs(got-(-2.0)) > 1e-9 {
		t.Errorf("NettoPerYearPercent() = %v, хочу −2.0", got)
	}
	t.Cleanup(ResetPopulationSettings)
	if err := SetLifeExpectancyYears(100); err != nil {
		t.Fatalf("SetLifeExpectancyYears(100): %v", err)
	}
	if err := SetBirthRateCoefficient(1); err != nil {
		t.Fatalf("SetBirthRateCoefficient(1): %v", err)
	}
	if got := NettoPerYearPercent(); math.Abs(got) > 1e-9 {
		t.Errorf("NettoPerYearPercent() при k=1 = %v, хочу 0 (равновесие)", got)
	}
}