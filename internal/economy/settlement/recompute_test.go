package settlement

import (
	"testing"
	"time"
)

func comfortableInput() PlanetInput {
	return PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 5}
}

func hotInput() PlanetInput {
	// 500 K — горячая сторона с конечной λ (жёсткий ноль на T ≥ 700 K,
	// 99.2.12, H4): тесты «убывает, но не мгновенно» остаются честными.
	return PlanetInput{TemperatureK: 500, GravityG: 1.0, CoreRadioactivity: 5}
}

func TestRecomputeNoTimePassedIsUnchanged(t *testing.T) {
	now := time.Now()
	got := Recompute(hotInput(), DefaultScale, 1000, now, now)
	if got != 1000 {
		t.Errorf("Recompute без прошедшего времени = %v, хочу 1000", got)
	}
}

func TestRecomputeClockWentBackwardsIsUnchanged(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	// since = now, now = earlier: отрицательный Δt не должен ничего менять.
	got := Recompute(hotInput(), DefaultScale, 1000, now, earlier)
	if got != 1000 {
		t.Errorf("Recompute с отрицательным Δt = %v, хочу 1000 (без изменений)", got)
	}
}

func TestRecomputeComfortableIsUnchanged(t *testing.T) {
	since := time.Now().Add(-24 * 365 * time.Hour) // год назад
	now := time.Now()
	got := Recompute(comfortableInput(), DefaultScale, 1_000_000, since, now)
	if got != 1_000_000 {
		t.Errorf("Recompute в комфорте изменил население: %v", got)
	}
}

func TestRecomputeHotPlanetDecreases(t *testing.T) {
	since := time.Now().Add(-24 * time.Hour)
	now := time.Now()
	got := Recompute(hotInput(), DefaultScale, 1_000_000, since, now)
	if !(got < 1_000_000 && got > 0) {
		t.Errorf("Recompute на горячей планете = %v, хочу строго между 0 и 1_000_000", got)
	}
}

func TestRecomputeBelowNDeadCollapsesToZero(t *testing.T) {
	since := time.Now().Add(-24 * 365 * 10 * time.Hour) // 10 лет — заведомо ниже порога
	now := time.Now()
	got := Recompute(hotInput(), DefaultScale, 1000, since, now)
	if got != 0 {
		t.Errorf("Recompute ниже NDead = %v, хочу 0 (разовый обвал)", got)
	}
}
