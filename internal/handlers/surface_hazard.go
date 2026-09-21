// internal/handlers/surface_hazard.go
//
// Профиль опасности планеты (спека 2026-09-21 §8.2/§8.3): непрерывные оси
// среды (жара/холод, давление/вакуум, радиация, токсичность) + дискретные
// телеграфированные события. Числа — предположение дизайнера, приняты
// создателем 2026-09-21. Все оси непрерывны и дают 0 на границе вилки
// скафандра (правило PITFALLS «жёсткий ноль как порог-переключатель»);
// события существуют только при severity > 0 (§8.3, И10).
package handlers

import (
	"math"
	"time"
)

// Базовый скафандр (спека §8.1): максимум HP и вилки комфорта.
const (
	SurfaceHPMax           = 100.0
	SurfaceTempComfortMinK = 263.0 // −10 °C
	SurfaceTempComfortMaxK = 313.0 // +40 °C
	SurfacePressureMinAtm  = 0.5
	SurfacePressureMaxAtm  = 3.0
)

// Константы событий/северити (спека §8.2/§8.3).
const (
	surfaceSeverityScale  = 0.5  // severity = min(2, Σr / 0.5)
	surfaceSeverityCap    = 2.0  // потолок severity
	surfaceEventAvgDamage = 17.5 // d̄ (HP), 10–25
)

// SurfaceHazard — профиль опасности (пакет прогулки §7.1). Все значения в HP/с;
// total = фон + события.
type SurfaceHazard struct {
	Temperature float64 `json:"temperature"`
	Pressure    float64 `json:"pressure"`
	Radiation   float64 `json:"radiation"`
	Toxic       float64 `json:"toxic"`
	Events      float64 `json:"events"`
	Total       float64 `json:"total"`
	Severity    float64 `json:"severity"`
}

// hazardTemperature — ось жара/холод (§8.2): r = 0.02·(ΔT/50)².
func hazardTemperature(t float64) float64 {
	dT := math.Max(0, math.Max(SurfaceTempComfortMinK-t, t-SurfaceTempComfortMaxK))
	if dT == 0 {
		return 0
	}
	return 0.02 * math.Pow(dT/50, 2)
}

// hazardPressure — ось давления (§8.2): высокое — 0.08·(log₁₀(P/3))^1.5,
// вакуум — 0.02·(log₁₀(0.5/P))^1.5. P ≤ 0 (нет данных) → 0 (не падать).
func hazardPressure(p float64) float64 {
	if p <= 0 {
		return 0
	}
	if p > SurfacePressureMaxAtm {
		return 0.08 * math.Pow(math.Log10(p/SurfacePressureMaxAtm), 1.5)
	}
	if p < SurfacePressureMinAtm {
		return 0.02 * math.Pow(math.Log10(SurfacePressureMinAtm/p), 1.5)
	}
	return 0
}

// hazardRadiation — ось радиации (§8.2, Б3): непрерывно от radioactivity 0–100,
// r = 0.10·(radioactivity/100)². Метка radioactive на урон не влияет.
func hazardRadiation(radioactivity float64) float64 {
	if radioactivity <= 0 {
		return 0
	}
	r := radioactivity / 100
	return 0.10 * r * r
}

// hazardToxic — ось токсичности (§8.2, Б3): r = 0.05·min(1, tox_ratio²).
func hazardToxic(toxRatio float64) float64 {
	if toxRatio <= 0 {
		return 0
	}
	return 0.05 * math.Min(1, toxRatio*toxRatio)
}

// eventFrequency — f_кат: частота событий при severity = 1 (§8.3). Биосфера — 0
// (жизнь не бьёт, «не бой» §7.3); неизвестная категория — 0.
func eventFrequency(category string) float64 {
	switch category {
	case "литосфера":
		return 0.012
	case "вода":
		return 0.014
	case "крио":
		return 0.016
	case "вулканизм":
		return 0.030
	case "экзотика":
		return 0.018
	default:
		return 0
	}
}

// ComputeSurfaceHazard — профиль опасности из физики планеты и категории биома
// (§8.2 фон + §8.3 события). При severity = 0 событий нет и фон = 0.
func ComputeSurfaceHazard(temperature, pressureAtm, radioactivity, toxRatio float64, category string) SurfaceHazard {
	h := SurfaceHazard{
		Temperature: hazardTemperature(temperature),
		Pressure:    hazardPressure(pressureAtm),
		Radiation:   hazardRadiation(radioactivity),
		Toxic:       hazardToxic(toxRatio),
	}
	bg := h.Temperature + h.Pressure + h.Radiation + h.Toxic
	h.Severity = math.Min(surfaceSeverityCap, bg/surfaceSeverityScale)
	h.Events = surfaceEventAvgDamage * eventFrequency(category) * h.Severity
	h.Total = bg + h.Events
	return h
}

// surfaceHPAt — серверное здоровье от времени высадки (§8.7):
// hp(t) = max(0, 100 − total·(t − landed_at)). landedAt.IsZero() → 100.
func surfaceHPAt(landedAt time.Time, hazardTotal float64, now time.Time) float64 {
	if landedAt.IsZero() {
		return SurfaceHPMax
	}
	dt := now.Sub(landedAt).Seconds()
	if dt < 0 {
		dt = 0
	}
	hp := SurfaceHPMax - hazardTotal*dt
	if hp < 0 {
		return 0
	}
	return hp
}

// dominantHazardCause — причина смерти для экрана (§6.3): доминирующая ось.
// Пусто, если все оси нулевые (мягкая планета — смерть не от среды).
func dominantHazardCause(h SurfaceHazard, temperature float64) string {
	best := h.Temperature
	cause := "жара"
	if temperature < SurfaceTempComfortMinK {
		cause = "холод"
	}
	consider := func(v float64, name string) {
		if v > best {
			best = v
			cause = name
		}
	}
	consider(h.Pressure, "давление")
	consider(h.Radiation, "радиация")
	consider(h.Toxic, "токсичная атмосфера")
	consider(h.Events, "выброс")
	if best <= 0 {
		return ""
	}
	return cause
}
