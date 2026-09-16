// internal/generator/planet/core.go
package planet

import "math/rand"

// Core — ядро планеты. Первичная сущность, определяющая температуру
// поверхности (через HeatContribution), наличие магнитного поля
// (через IsActive) и геологическую активность.
type Core struct {
	Type          string  // "металлическое" | "силикатное" | "ледяное" | "экзотическое"
	MassPercent   float64 // доля от массы планеты (5–60%)
	Activity      float64 // 0–100: жидкое ядро, конвекция
	Radioactivity float64 // 0–100: U, Th, K
	Age           float64 // млрд лет
	// HeatFluxWm2 — внутренний тепловой поток F_int (Вт/м², 99.2.20 §3.4):
	// радиогенный + аккреционный + Кельвина–Гельмгольца (гиганты).
	// Пишется в core.heat_flux_w_m2; игровой radioactivity (0–100) остаётся.
	HeatFluxWm2 float64
}

// ==================== ТИПЫ ЯДРА ====================

const (
	CoreMetallic = "металлическое"
	CoreSilicate = "силикатное"
	CoreIce      = "ледяное"
	CoreExotic   = "экзотическое"
)

// ==================== ПОРОГИ МАССЫ ====================

// Пороги подобраны под реальные диапазоны:
//   - Земля: mass=1, металлическое ядро, 32% массы
//   - Венера: mass=0.8, металлическое ядро, 31%
//   - Марс: mass=0.1, силикатное ядро, 25%
//   - Луна: mass=0.012, силикатное/ледяное, 1–5%
//   - Суперземля: mass=5, металлическое ядро
//   - Газовый гигант: mass>30, металлическое ядро, но малое % от общей массы
const (
	massGasGiant   = 5.0 // выше — почти всегда металлическое
	massEarthlike  = 0.5 // выше — металлическое с шансом 60%
	massSmall      = 0.5 // ниже — силикатное/ледяное
)

// ==================== ГЕНЕРАЦИЯ ====================

// GenerateCore — создаёт ядро для планеты.
func GenerateCore(
	mass float64,
	archetypeID string,
	subterrain Composition,
	systemAge float64,
	rng *rand.Rand,
) Core {
	coreType := determineCoreType(mass, rng)
	massPercent := determineCoreMassPercent(mass, rng)
	activity := determineCoreActivity(archetypeID, subterrain, rng)
	radioactivity := determineCoreRadioactivity(subterrain, rng)

	return Core{
		Type:          coreType,
		MassPercent:   massPercent,
		Activity:      activity,
		Radioactivity: radioactivity,
		Age:           systemAge,
	}
}

// determineCoreType — тип ядра по массе.
//
// Целевое распределение:
//   - металлическое ~55%
//   - силикатное ~30%
//   - ледяное ~12%
//   - экзотическое ~3%
func determineCoreType(mass float64, rng *rand.Rand) string {
	switch {
	case mass > massGasGiant:
		// Суперземли и газовые гиганты — почти всегда металлическое ядро
		if rng.Float64() < 0.95 {
			return CoreMetallic
		}
		return CoreSilicate

	case mass > massEarthlike:
		// Землеподобные: металлическое или силикатное
		if rng.Float64() < 0.6 {
			return CoreMetallic
		}
		return CoreSilicate

	default:
		// Малые планеты: силикатное / ледяное / редко экзотика
		r := rng.Float64()
		switch {
		case r < 0.55:
			return CoreSilicate
		case r < 0.9:
			return CoreIce
		default:
			return CoreExotic
		}
	}
}

// determineCoreMassPercent — доля ядра от массы планеты.
//
// Земля: 32%, Венера: 31%, Марс: 25%, Луна: 1–5%.
func determineCoreMassPercent(mass float64, rng *rand.Rand) float64 {
	switch {
	case mass > massGasGiant:
		return 20 + rng.Float64()*20 // 20–40% (газовые гиганты)
	case mass > massEarthlike:
		return 25 + rng.Float64()*15 // 25–40% (Земля 32%)
	default:
		return 5 + rng.Float64()*20 // 5–25% (малые планеты)
	}
}

// determineCoreActivity — активность ядра.
//
// Зависит от:
//   - архетипа (экстремальный/жаркий → активнее);
//   - доли магматических камер в недрах;
//   - доли магматических пород.
func determineCoreActivity(
	archetypeID string,
	subterrain Composition,
	rng *rand.Rand,
) float64 {
	var base float64
	switch archetypeID {
	case "экстремальный":
		base = 70 + rng.Float64()*30
	case "жаркий":
		base = 40 + rng.Float64()*30
	case "умеренный":
		base = 30 + rng.Float64()*30
	case "изменчивый":
		base = 20 + rng.Float64()*30
	case "холодный":
		base = 5 + rng.Float64()*20
	default:
		base = 20 + rng.Float64()*30
	}

	base += subterrain.ShareOf(SubterrainMagmaChambers) * 1.5
	base += subterrain.ShareOf(SubterrainMagmaticRocks) * 0.5

	if base > 100 {
		base = 100
	}
	if base < 0 {
		base = 0
	}
	return base
}

// determineCoreRadioactivity — радиоактивность ядра.
//
// Прямо связана с долей радиоактивных зон в недрах.
func determineCoreRadioactivity(subterrain Composition, rng *rand.Rand) float64 {
	share := subterrain.ShareOf(SubterrainRadioactiveZones)
	base := share * 6.0
	base += rng.Float64() * 10

	if base > 100 {
		base = 100
	}
	if base < 0 {
		base = 0
	}
	return base
}

// ==================== ВКЛАД В ТЕМПЕРАТУРУ ====================

// HeatContribution — сколько кельвинов добавляет ядро к температуре поверхности.
//
// Формула:
//
//	base_heat = Activity × 1.5 + Radioactivity × 2.5   (0–400 K)
//	age_factor = max(0.15, 1.0 - Age/15)               (старение)
//	mass_factor = MassPercent / 30                     (масштаб ядра)
//	contribution = base_heat × age_factor × mass_factor
func (c Core) HeatContribution() float64 {
	baseHeat := c.Activity*1.5 + c.Radioactivity*2.5

	ageFactor := 1.0 - c.Age/15.0
	if ageFactor < 0.15 {
		ageFactor = 0.15
	}

	massFactor := c.MassPercent / 30.0
	if massFactor < 0.1 {
		massFactor = 0.1
	}

	result := baseHeat * ageFactor * massFactor
	if result < 0 {
		result = 0
	}
	return result
}

// ==================== ФЛАГИ ====================

// IsMetallic — металлическое ли ядро.
func (c Core) IsMetallic() bool {
	return c.Type == CoreMetallic
}

// IsActive — активное ли ядро (для магнитного поля).
// Порог 40 — эмпирический: Земля ~70, Марс ~20.
func (c Core) IsActive() bool {
	return c.Activity > 40
}

// IsRadioactive — сильно ли радиоактивное ядро.
func (c Core) IsRadioactive() bool {
	return c.Radioactivity > 50
}