// internal/generator/planet/properties.go
package planet

import (
	"math"
	"math/rand"
)

// ==================== РАДИУС ====================

// computeRadius — радиус из массы и плотности.
// R = (M / ρ)^(1/3), в земных единицах.
func computeRadius(mass, density float64) float64 {
	if density <= 0 {
		density = 1.0
	}
	if mass <= 0 {
		mass = 0.1
	}
	return math.Pow(mass/density, 1.0/3.0)
}

// ==================== ВОДА, ЖИЗНЬ, СПУТНИКИ ====================

// Пороги физических состояний воды для гейта гидросфер:
//
//	heatThreshold — граница «жары» (совпадает с порогом аудита
//	                oceans_in_heat): выше жидкой воды не бывает;
//	freezingPoint — точка замерзания воды.
const (
	heatThreshold = 400.0
	freezingPoint = 273.0
)

func generateWater(archetype *Archetype, temp float64, rng *rand.Rand) float64 {
	switch archetype.Hydrosphere {
	case "океаны":
		// Жидкие океаны — только до порога «жары» (тот же, что у аудита
		// oceans_in_heat). Выше — воды нет: пара, а не океан.
		if temp <= heatThreshold {
			return 70 + rng.Float64()*25
		}
		return 0
	case "озёра":
		if temp <= heatThreshold {
			return 20 + rng.Float64()*40
		}
		return 0
	case "ледяной покров":
		// Лёд — только ниже точки замерзания.
		if temp < freezingPoint {
			return 5 + rng.Float64()*20
		}
		return 0
	case "подлёдная":
		// Подлёдный океан — только под замёрзшей поверхностью.
		if temp < freezingPoint {
			return 50 + rng.Float64()*40
		}
		return 0
	case "кислотная":
		return 0
	case "сухая":
		return 0
	}

	if temp > 250 && temp < 400 {
		if rng.Float64() < archetype.WaterChance {
			return 30 + rng.Float64()*60
		}
	} else if temp > 150 && temp <= 250 {
		if rng.Float64() < archetype.WaterChance*0.7 {
			return 20 + rng.Float64()*40
		}
	} else {
		if rng.Float64() < archetype.WaterChance*0.3 {
			return 10 + rng.Float64()*20
		}
	}
	return 0
}

func determineMoons(size float64, archetypeID string, rng *rand.Rand) int {
	base := 0
	switch archetypeID {
	case "жаркий", "экстремальный":
		base = int(size)
	case "холодный":
		base = int(size * 1.5)
	default:
		base = int(size * 1.2)
	}
	if base < 0 {
		base = 0
	}
	jitter := rng.Intn(3) - 1
	moons := base + jitter
	if moons < 0 {
		moons = 0
	}
	return moons
}