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
	now := time.Now()
	input := hotInput() // 365 K, жара: r ≈ 7.3·10⁻⁵/сек
	r := ChangeComponents(input)
	want := computedAt.Add(time.Duration(
		math.Log(1000/NDead)/math.Abs(math.Log(1-r)) * float64(time.Second)))
	got, ok := DeathTime(1000, r, computedAt, now, createdAt)
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
	got, ok := DeathTime(1000, 1.5, computedAt, time.Now(), computedAt)
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
	if _, ok := DeathTime(NDead, r, now, now, now); ok {
		t.Errorf("DeathTime при population_exact == NDead: ok=true, хочу false")
	}
	if _, ok := DeathTime(50, r, now, now, now); ok {
		t.Errorf("DeathTime при population_exact < NDead: ok=true, хочу false")
	}
}

func TestDeathTimeNoDecayYoungNoEntry(t *testing.T) {
	// Нет изменения (r = 0) и возраст < 120 лет — не вымирают, записи нет.
	createdAt := time.Now().Add(-50 * 365 * 24 * time.Hour)
	now := time.Now()
	if _, ok := DeathTime(1000, 0, now, now, createdAt); ok {
		t.Errorf("DeathTime без изменения и без потолка: ok=true, хочу false")
	}
}

// natural-смерть (полный комфорт, r = NaturalChangeRate): формула даёт ~460 лет,
// но потолок 120 лет раньше → дата = created_at + 120 лет. Регресс переполнения
// time.Duration: формула natural (1.45·10¹⁰ сек) переполнила бы int64 нс и
// ушла в 1613 г (тестер) — сравнение с потолком до конвертации.
func TestDeathTimeNaturalCappedAtLifespan(t *testing.T) {
	createdAt := time.Now().Add(-121 * 365 * 24 * time.Hour) // возраст 121 год
	now := time.Now()
	computedAt := now.Add(-1 * time.Hour)
	input := PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5} // полный комфорт
	r := ChangeComponents(input)
	if r <= 0 {
		t.Fatalf("ожидали r = NaturalChangeRate > 0, получили %v", r)
	}
	got, ok := DeathTime(1_000_000, r, computedAt, now, createdAt)
	want := createdAt.Add(time.Duration(MaxLifespanSeconds) * time.Second)
	if !ok {
		t.Fatalf("DeathTime(natural, возраст 121 год) = ok=false, хочу true")
	}
	if !got.Equal(want) {
		t.Errorf("DeathTime = %v, хочу потолок %v", got, want)
	}
	if got.Year() < 2000 {
		t.Errorf("DeathTime = %v — похоже на переполнение time.Duration (1613 г)", got)
	}
}

// r = 0 (288 K: NaturalComponent = 0, рампа с 288.15 K) и возраст ≥ 120 лет —
// смерть только по потолку (Recompute: age ≥ 120 → население = 0 для любого
// p0) → запись есть, дата = created_at + 120 лет.
func TestDeathTimeZeroRAtLifespan(t *testing.T) {
	createdAt := time.Now().Add(-130 * 365 * 24 * time.Hour) // возраст 130 лет
	now := time.Now()
	computedAt := now.Add(-1 * time.Hour) // чек-точка ещё живая (возраст ~129 лет)
	got, ok := DeathTime(1_000_000, 0, computedAt, now, createdAt)
	want := createdAt.Add(time.Duration(MaxLifespanSeconds) * time.Second)
	if !ok {
		t.Fatalf("DeathTime(r=0, возраст ≥ 120) = ok=false, хочу true (смерть по потолку)")
	}
	if !got.Equal(want) {
		t.Errorf("DeathTime = %v, хочу потолок %v", got, want)
	}
}

// Смешанный: жара + natural, смерть по формуле (быстрая), но не позже потолка:
// t = min(формула, ceiling) = формула.
func TestDeathTimeFormulaCappedAtLifespan(t *testing.T) {
	createdAt := time.Now().Add(-121 * 365 * 24 * time.Hour) // возраст ≥ 120 (потолок достигнут)
	now := time.Now()
	computedAt := createdAt // чек-точка = создание (возраст 0 на чек-точке)
	r := ChangeComponents(PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5})
	want := computedAt.Add(time.Duration(
		math.Log(1_000_000/NDead)/math.Abs(math.Log(1-r)) * float64(time.Second)))
	got, ok := DeathTime(1_000_000, r, computedAt, now, createdAt)
	ceiling := createdAt.Add(time.Duration(MaxLifespanSeconds) * time.Second)
	if !ok {
		t.Fatalf("DeathTime(жара, возраст ≥ 120) = ok=false, хочу true")
	}
	if !got.Equal(want) {
		t.Errorf("DeathTime = %v, хочу формулу %v", got, want)
	}
	if got.After(ceiling) {
		t.Errorf("DeathTime = %v позже потолка %v", got, ceiling)
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