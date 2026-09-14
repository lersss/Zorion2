package settlement

import (
	"math"
	"testing"
	"time"
)

// --- DeathTime: момент пересечения порога NDead из живой чек-точки (R-модель) ---

func TestDeathTimeR(t *testing.T) {
	computedAt := time.Now().Add(-10 * 24 * time.Hour)
	input := hotInput() // 373 K, жара: R ≈ 1.16·10⁻⁴/сек
	r := RecursiveChangeRate(input.TemperatureK)
	lambda := OtherChangeRate(input, DefaultScale)
	want := computedAt.Add(time.Duration(
		math.Log(1000/NDead)/math.Abs(math.Log(1-r)) * float64(time.Second)))
	got, ok := DeathTime(1000, r, lambda, computedAt)
	if !ok {
		t.Fatalf("DeathTime(живая чек-точка) = ok=false, хочу true")
	}
	if !got.Equal(want) {
		t.Errorf("DeathTime = %v, хочу %v", got, want)
	}
}

func TestDeathTimeHardZeroIsComputedAt(t *testing.T) {
	computedAt := time.Now().Add(-5 * time.Hour)
	// 100 K — холодный жёсткий ноль → λ = +Inf → t_death = computed_at.
	lambda := OtherChangeRate(PlanetInput{TemperatureK: 100, GravityG: 1.0, CoreRadioactivity: 5}, DefaultScale)
	if !math.IsInf(lambda, 1) {
		t.Fatalf("ожидали λ = +Inf, получили %v", lambda)
	}
	got, ok := DeathTime(1000, 0, lambda, computedAt)
	if !ok {
		t.Fatalf("DeathTime(+Inf) = ok=false, хочу true")
	}
	if !got.Equal(computedAt) {
		t.Errorf("DeathTime при λ=+Inf = %v, хочу computed_at %v", got, computedAt)
	}
}

func TestDeathTimeDeadCheckpointNoEntry(t *testing.T) {
	// Чек-точка уже мёртвая (population_exact <= NDead) — дата невычислима,
	// запись не создаётся (бэкфилл отменён, решение создателя 2026-09-14).
	r := RecursiveChangeRate(hotInput().TemperatureK)
	if _, ok := DeathTime(NDead, r, 0, time.Now()); ok {
		t.Errorf("DeathTime при population_exact == NDead: ok=true, хочу false")
	}
	if _, ok := DeathTime(50, r, 0, time.Now()); ok {
		t.Errorf("DeathTime при population_exact < NDead: ok=true, хочу false")
	}
}

func TestDeathTimeNoDecayNoEntry(t *testing.T) {
	// Нет убыли (R = 0 и λ = 0) — обвала нет, запись не возникает.
	if _, ok := DeathTime(1000, 0, 0, time.Now()); ok {
		t.Errorf("DeathTime без убыли: ok=true, хочу false")
	}
}

// --- DeathCause: доминирующий фактор по вкладу в суммарную λ ---

func TestDeathCauseHot(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5}, DefaultScale); got != "heat" {
		t.Errorf("DeathCause(400 K) = %q, хочу heat", got)
	}
}

func TestDeathCauseCold(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 150, GravityG: 1.0, CoreRadioactivity: 5}, DefaultScale); got != "cold" {
		t.Errorf("DeathCause(150 K) = %q, хочу cold", got)
	}
}

func TestDeathCauseGravityHigh(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 275, GravityG: 4.533, CoreRadioactivity: 5}, DefaultScale); got != "gravity_high" {
		t.Errorf("DeathCause(g=4.533) = %q, хочу gravity_high", got)
	}
}

func TestDeathCauseGravityLow(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 275, GravityG: 0.4, CoreRadioactivity: 5}, DefaultScale); got != "gravity_low" {
		t.Errorf("DeathCause(g=0.4) = %q, хочу gravity_low", got)
	}
}

func TestDeathCauseRadiation(t *testing.T) {
	if got := DeathCause(PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 90}, DefaultScale); got != "radiation" {
		t.Errorf("DeathCause(rad=90) = %q, хочу radiation", got)
	}
}

func TestDeathCauseTieTemperatureWins(t *testing.T) {
	// Жара 400 K (sev 0.833) и гравитация g=4.533 (sev 0.833) дают равные вклады;
	// при равенстве — температура → гравитация → радиоактивность (2026-09-14).
	if got := DeathCause(PlanetInput{TemperatureK: 400, GravityG: 4.533, CoreRadioactivity: 5}, DefaultScale); got != "heat" {
		t.Errorf("DeathCause(равный вклад жары и гравитации) = %q, хочу heat", got)
	}
}

func TestDeathCauseHardZeroTemperature(t *testing.T) {
	// Жёсткий ноль (λ=+Inf): причина — фактор, давший ноль (температура).
	if got := DeathCause(PlanetInput{TemperatureK: 500, GravityG: 1.0, CoreRadioactivity: 5}, DefaultScale); got != "heat" {
		t.Errorf("DeathCause(500 K) = %q, хочу heat", got)
	}
	if got := DeathCause(PlanetInput{TemperatureK: 100, GravityG: 1.0, CoreRadioactivity: 5}, DefaultScale); got != "cold" {
		t.Errorf("DeathCause(100 K) = %q, хочу cold", got)
	}
}
