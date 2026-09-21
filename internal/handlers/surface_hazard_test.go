// internal/handlers/surface_hazard_test.go
// Тесты профиля опасности прогулки (спека 2026-09-21 §8.2/§8.3):
// реперные планеты, непрерывность осей (Б3), отсутствие событий при
// severity = 0 (И10), доля событий ≤ 51 %.
package handlers

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// near — сравнение с относительной/абсолютной погрешностью.
func near(t *testing.T, want, got, tol float64, msg ...string) {
	t.Helper()
	if len(msg) > 0 {
		assert.InDelta(t, want, got, tol, msg[0])
		return
	}
	assert.InDelta(t, want, got, tol)
}

// Мягкая планета (288 K, 1 атм): фон 0, severity 0, событий нет (мягкая не наказывает).
func TestHazardSoftPlanetIsHarmless(t *testing.T) {
	h := ComputeSurfaceHazard(288, 1.0, 0, 0, "литосфера")
	assert.Zero(t, h.Temperature)
	assert.Zero(t, h.Pressure)
	assert.Zero(t, h.Radiation)
	assert.Zero(t, h.Toxic)
	assert.Zero(t, h.Severity, "severity = 0 при нулевом фоне")
	assert.Zero(t, h.Events, "событий нет при severity = 0 (И10)")
	assert.Zero(t, h.Total, "мягкая планета не наказывает")
}

// Реперные планеты §8.2: фон, severity, события, total (числа спеки).
func TestHazardReferencePlanets(t *testing.T) {
	cases := []struct {
		name         string
		t, p, radio  float64
		category     string
		wantBg       float64
		wantSeverity float64
		wantEvents   float64
		wantTotal    float64
	}{
		{name: "марсоподобная (210 K, 0.01 атм, литосфера)", t: 210, p: 0.01, category: "литосфера",
			wantBg: 0.067, wantSeverity: 0.13, wantEvents: 0.028, wantTotal: 0.095},
		{name: "холодная умеренная (150 K, 1 атм, крио)", t: 150, p: 1.0, category: "крио",
			wantBg: 0.102, wantSeverity: 0.20, wantEvents: 0.057, wantTotal: 0.159},
		{name: "ледяная экстремальная (60 K, 0.05 атм, крио)", t: 60, p: 0.05, category: "крио",
			wantBg: 0.350, wantSeverity: 0.70, wantEvents: 0.196, wantTotal: 0.545},
		{name: "венерианская (700 K, 90 атм, вулканизм)", t: 700, p: 90, category: "вулканизм",
			wantBg: 1.342, wantSeverity: 2.0, wantEvents: 1.050, wantTotal: 2.392},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := ComputeSurfaceHazard(tc.t, tc.p, tc.radio, 0, tc.category)
			bg := h.Temperature + h.Pressure + h.Radiation + h.Toxic
			near(t, tc.wantBg, bg, 0.002, "фон Σr")
			near(t, tc.wantSeverity, h.Severity, 0.01, "severity")
			near(t, tc.wantEvents, h.Events, 0.002, "события HP/с")
			near(t, tc.wantTotal, h.Total, 0.003, "total HP/с")
			// Время до смерти (с событиями): 100 / total.
			t.Logf("%s: время до 0 HP ≈ %.0f с", tc.name, SurfaceHPMax/h.Total)
		})
	}
}

// Венерианский репер: только фон ≈75 с, с событиями ≈42 с (принято создателем).
func TestHazardVenusianTimes(t *testing.T) {
	h := ComputeSurfaceHazard(700, 90, 0, 0, "вулканизм")
	bg := h.Temperature + h.Pressure
	near(t, 75, SurfaceHPMax/bg, 1, "только фон ≈ 75 с")
	near(t, 42, SurfaceHPMax/h.Total, 1, "с событиями ≈ 42 с")
}

// Радиация непрерывна (Б3): 0 → 0, нет скачка на пороге 50; r = 0.10·(x/100)².
func TestHazardRadiationContinuous(t *testing.T) {
	assert.Zero(t, hazardRadiation(0))
	near(t, 0.025, hazardRadiation(50), 1e-9)
	near(t, 0.10, hazardRadiation(100), 1e-9)
	lo := hazardRadiation(49.9)
	hi := hazardRadiation(50.1)
	assert.Less(t, math.Abs(hi-lo), 0.001, "нет разрыва на пороге 50 (Б3)")
}

// Токсичность непрерывна (Б3): 0 → 0; при ratio ≥ 1 — кламп min(1, ratio²).
func TestHazardToxicContinuous(t *testing.T) {
	assert.Zero(t, hazardToxic(0))
	near(t, 0.0125, hazardToxic(0.5), 1e-9)
	near(t, 0.05, hazardToxic(1.0), 1e-9)
	near(t, 0.05, hazardToxic(3.0), 1e-9, "кламп min(1, ratio²)")
}

// Доля событий ≤ 51 % и убывает на сверхгорячих (И10/Б1).
func TestHazardEventShareBound(t *testing.T) {
	for _, tc := range []struct {
		t, p     float64
		category string
	}{
		{210, 0.01, "литосфера"},
		{150, 1.0, "крио"},
		{60, 0.05, "крио"},
		{700, 90, "вулканизм"},
	} {
		h := ComputeSurfaceHazard(tc.t, tc.p, 0, 0, tc.category)
		if h.Total == 0 {
			continue
		}
		share := h.Events / h.Total
		assert.LessOrEqual(t, share, 0.51, "доля событий ≤ 51 %")
	}
}

// surfaceHPAt: hp(t) = max(0, 100 − total·Δt); нулевой фон → 100 навсегда.
func TestSurfaceHPAt(t *testing.T) {
	now := time.Now()
	assert.Equal(t, SurfaceHPMax, surfaceHPAt(now, 0, now))
	// total 0.5 HP/с, 10 с → 95.
	hp := surfaceHPAt(now.Add(-10*time.Second), 0.5, now)
	near(t, 95, hp, 0.001)
	// total 5 HP/с, 30 с → 0 (не отрицательно).
	assert.Zero(t, surfaceHPAt(now.Add(-30*time.Second), 5, now))
}
