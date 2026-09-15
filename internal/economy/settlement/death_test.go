package settlement

import (
	"math"
	"testing"
	"time"
)

// --- DeathTime: момент пересечения порога NDead из живой чек-точки (R-модель) ---

func TestDeathTimeR(t *testing.T) {
	computedAt := time.Now().Add(-10 * 24 * time.Hour)
	createdAt := computedAt.Add(-365 * 24 * time.Hour) // возраст ~1 год на чек-точке
	input := hotInput() // 365 K, жара: r ≈ 7.3·10⁻⁵/сек
	r := ChangeComponents(input)
	want := time.Unix(computedAt.Unix()+int64(
		math.Log(1000/NDead)/math.Abs(math.Log(1-r))), 0)
	got, ok := DeathTime(1000, r, computedAt, createdAt)
	if !ok {
		t.Fatalf("DeathTime(живая чек-точка) = ok=false, хочу true")
	}
	if !got.Equal(want) {
		t.Errorf("DeathTime = %v, хочу %v", got, want)
	}
}

func TestDeathTimeGuardIsComputedAt(t *testing.T) {
	// Гвард r ≥ 1 (экстремальные входы) → t_death = computed_at (мгновенно).
	computedAt := time.Now().Add(-5 * time.Hour)
	got, ok := DeathTime(1000, 1.5, computedAt, computedAt)
	if !ok {
		t.Fatalf("DeathTime(r ≥ 1) = ok=false, хочу true")
	}
	if !got.Equal(computedAt) {
		t.Errorf("DeathTime при r ≥ 1 = %v, хочу computed_at %v", got, computedAt)
	}
}

func TestDeathTimeDeadCheckpointNoEntry(t *testing.T) {
	// Чек-точка уже мёртвая (population_exact <= NDead) — дата невычислима,
	// запись не создаётся (бэкфилл отменён, решение создателя 2026-09-14).
	r := ChangeComponents(hotInput())
	now := time.Now()
	if _, ok := DeathTime(NDead, r, now, now); ok {
		t.Errorf("DeathTime при population_exact == NDead: ok=true, хочу false")
	}
	if _, ok := DeathTime(50, r, now, now); ok {
		t.Errorf("DeathTime при population_exact < NDead: ok=true, хочу false")
	}
}

func TestDeathTimeNoDecayNoEntry(t *testing.T) {
	// Нет изменения (r = 0) — равновесие: не вымирают, записи нет
	// (99.2.16: потолка нет, инверсия прежней «смерти по потолку»).
	createdAt := time.Now().Add(-50 * 365 * 24 * time.Hour)
	now := time.Now()
	if _, ok := DeathTime(1000, 0, now, createdAt); ok {
		t.Errorf("DeathTime без изменения: ok=true, хочу false")
	}
}

func TestDeathTimeEquilibriumNoEntry(t *testing.T) {
	// Равновесие r=0 живёт вечно (99.2.16 §6.2 п.5): записи «Вымерло» нет
	// при любом возрасте — потолка нет (инверсия TestDeathTimeZeroRAtLifespan).
	computedAt := time.Now().Add(-1 * time.Hour)
	createdAt := computedAt.Add(-130 * 365 * 24 * time.Hour) // возраст 130 лет
	if _, ok := DeathTime(1_000_000, 0, computedAt, createdAt); ok {
		t.Errorf("DeathTime(r=0, возраст 130) = ok=true, хочу false (равновесие не вымирает)")
	}
	// Равновесие по настройкам: k=1 в комфорте 30 °C → r = 0.
	t.Cleanup(ResetPopulationSettings)
	if err := SetBirthRateCoefficient(1); err != nil {
		t.Fatalf("SetBirthRateCoefficient(1): %v", err)
	}
	input := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	if r := ChangeComponents(input); r != 0 {
		t.Fatalf("k=1, 30 °C: r = %v, хочу 0", r)
	}
	if _, ok := DeathTime(1_000_000, ChangeComponents(input), computedAt, createdAt); ok {
		t.Errorf("DeathTime(k=1, 30 °C) = ok=true, хочу false")
	}
}

// Медленная natural-убыль без среды (k=0, комфорт 303.15 K, pop 10⁹): дата
// ≈ +800 лет через Unix-секунды. Регресс переполнения time.Duration
// (перенесён из удалённого TestDeathTimeNaturalCappedAtLifespan, 99.2.16):
// раньше computedAt.Add(...) падал/врал за ~292 года — теперь int64-секунд,
// запись создаётся с честной датой.
func TestDeathTimeSlowDecayCreatesEntry(t *testing.T) {
	t.Cleanup(ResetPopulationSettings)
	if err := SetBirthRateCoefficient(0); err != nil {
		t.Fatalf("SetBirthRateCoefficient(0): %v", err)
	}
	computedAt := time.Now().Add(-1 * time.Hour)
	input := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	r := ChangeComponents(input)
	if r <= 0 {
		t.Fatalf("k=0, комфорт: r = %v, хочу > 0 (чистая убыль)", r)
	}
	got, ok := DeathTime(1_000_000_000, r, computedAt, computedAt)
	if !ok {
		t.Fatalf("DeathTime(медленная убыль) = ok=false, хочу true")
	}
	tSec := math.Log(1_000_000_000/NDead) / math.Abs(math.Log(1-r))
	want := time.Unix(computedAt.Unix()+int64(tSec), 0)
	if math.Abs(got.Sub(want).Seconds()) > 1 {
		t.Errorf("DeathTime = %v, хочу %v (±1 сек)", got, want)
	}
	// Годы — через Unix-секунды (не time.Duration: 800 лет переполнили бы int64 нс).
	years := float64(got.Unix()-computedAt.Unix()) / (365.25 * 24 * 3600)
	if years < 700 || years > 900 {
		t.Errorf("дата смерти ≈ %v лет от чек-точки, хочу ≈ 800 (медленная убыль)", years)
	}
}

// Умеренная убыль (99.2.16 §6.2 п.7): ok=true, дата ровно
// computedAt + ln(pop/NDead)/|ln(1−r)| — без среза потолком сверху
// (на смену TestDeathTimeFormulaCappedAtLifespan: потолка нет).
func TestDeathTimeFastFormulaNoCeiling(t *testing.T) {
	computedAt := time.Now().Add(-1 * time.Hour)
	r := 1e-5
	tSec := math.Log(1_000_000/NDead) / math.Abs(math.Log(1-r))
	want := time.Unix(computedAt.Unix()+int64(tSec), 0)
	got, ok := DeathTime(1_000_000, r, computedAt, computedAt)
	if !ok {
		t.Fatalf("DeathTime(умеренная убыль) = ok=false, хочу true")
	}
	if !got.Equal(want) {
		t.Errorf("DeathTime = %v, хочу %v (формула без потолка)", got, want)
	}
}

// --- DeathCause: доминирующая рекурсивная компонента (99.2.13) ---

func TestDeathCauseHeat(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5}); got != "heat" {
		t.Errorf("DeathCause(400 K) = %q, хочу heat", got)
	}
}

func TestDeathCauseCold(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 150, GravityG: 1.0, CoreRadioactivity: 5}); got != "cold" {
		t.Errorf("DeathCause(150 K) = %q, хочу cold", got)
	}
}

func TestDeathCauseGravityHigh(t *testing.T) {
	// Комфортная температура (303.15 K — только R_ест, cold/heat = 0), чтобы
	// доминировала перегрузка.
	if got := DeathCause(PlanetInput{TemperatureK: 303.15, GravityG: 4.533, CoreRadioactivity: 5}); got != "gravity_high" {
		t.Errorf("DeathCause(g=4.533) = %q, хочу gravity_high", got)
	}
}

func TestDeathCauseGravityLow(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 303.15, GravityG: 0.4, CoreRadioactivity: 5}); got != "gravity_low" {
		t.Errorf("DeathCause(g=0.4) = %q, хочу gravity_low", got)
	}
}

func TestDeathCauseRadiation(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 90}); got != "radiation" {
		t.Errorf("DeathCause(rad=90) = %q, хочу radiation", got)
	}
}

func TestDeathCauseHeatDominatesModerateGravity(t *testing.T) {
	// Жара (400 K, R ≈ 2.7·10⁻⁴) доминирует над умеренной перегрузкой
	// (g=2, R ≈ 4.8·10⁻⁵).
	if got := DeathCause(PlanetInput{TemperatureK: 400, GravityG: 2, CoreRadioactivity: 5}); got != "heat" {
		t.Errorf("DeathCause(жара + умеренная перегрузка) = %q, хочу heat", got)
	}
}

func TestDeathCauseComfortAllZeroIsNatural(t *testing.T) {
	// Полный комфорт (T 293.15 K = +20 °C, g 1.0, rad ≤ 20): вклады среды = 0,
	// вымирание от естественной убыли (NaturalComponent) → причина "natural",
	// а не "cold" (дезинформация «Сильный холод» при идеальном климате).
	if got := DeathCause(PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5}); got != "natural" {
		t.Errorf("DeathCause(полный комфорт) = %q, хочу natural", got)
	}
	if got := DeathCause(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}); got != "natural" {
		t.Errorf("DeathCause(30 °C, комфорт по g/rad) = %q, хочу natural", got)
	}
}