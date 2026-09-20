// internal/generator/planet/composition_modifiers.go
package planet

import "math/rand"

// ==================== РАНДОМ ====================

// applyRandomJitter — умножает каждый вес на случайный множитель [0.8, 1.2].
func applyRandomJitter(c map[string]float64, rng *rand.Rand) {
	for k, v := range c {
		factor := 0.8 + rng.Float64()*0.4
		c[k] = v * factor
	}
}

// ==================== ПОВЕРХНОСТЬ: ТЕМПЕРАТУРА ====================

// applySurfaceTempModifiers — корректирует веса поверхности по температуре.
//
// Логика:
//   - при экстремальных T (жар/холод) биосферные формы удаляются совсем;
//   - в «пограничной зоне» (220–250 K) сильно подавляются (×0.05),
//     но не удаляются — остаётся шанс на локальную аномалию.
func applySurfaceTempModifiers(c map[string]float64, temperature float64) {
	switch {
	case temperature > 700:
		// Экстремально жарко
		multiplyIfExists(c, SurfaceLavaFields, 1.8)
		multiplyIfExists(c, SurfaceVolcanicFields, 1.3)
		multiplyIfExists(c, SurfaceGlassFields, 1.4)
		multiplyIfExists(c, SurfaceMetalFields, 1.2)
		delete(c, SurfaceOceans)
		delete(c, SurfaceLakes)
		delete(c, SurfaceGlaciers)
		delete(c, SurfaceFrozenGases)
		delete(c, SurfaceForests)
		delete(c, SurfaceJungles)
		delete(c, SurfaceMeadows)
		delete(c, SurfaceSwamps)
		delete(c, SurfaceCoralReefs)

	case temperature > 500:
		// Очень жарко
		multiplyIfExists(c, SurfaceLavaFields, 1.5)
		multiplyIfExists(c, SurfaceVolcanicFields, 1.2)
		multiplyIfExists(c, SurfaceGlassFields, 1.2)
		delete(c, SurfaceGlaciers)
		delete(c, SurfaceFrozenGases)
		delete(c, SurfaceForests)
		delete(c, SurfaceJungles)
		delete(c, SurfaceMeadows)
		delete(c, SurfaceSwamps)
		delete(c, SurfaceCoralReefs)
		multiplyIfExists(c, SurfaceOceans, 0.3)
		multiplyIfExists(c, SurfaceLakes, 0.3)

	case temperature < 220:
		// Очень холодно — биосфера удаляется
		delete(c, SurfaceJungles)
		delete(c, SurfaceForests)
		delete(c, SurfaceMeadows)
		delete(c, SurfaceSwamps)
		delete(c, SurfaceCoralReefs)
		delete(c, SurfaceOceans)
		multiplyIfExists(c, SurfaceLakes, 0.2)
		multiplyIfExists(c, SurfaceGlaciers, 1.8)
		multiplyIfExists(c, SurfaceFrozenGases, 1.5)

	case temperature < 250:
		// Холодно — биосфера сильно подавляется, но не удаляется.
		// Остаётся шанс на локальный «оазис» — редкую аномалию.
		delete(c, SurfaceJungles)
		delete(c, SurfaceCoralReefs)
		multiplyIfExists(c, SurfaceForests, 0.05)
		multiplyIfExists(c, SurfaceMeadows, 0.05)
		multiplyIfExists(c, SurfaceSwamps, 0.05)
		multiplyIfExists(c, SurfaceGlaciers, 1.4)
		multiplyIfExists(c, SurfaceFrozenGases, 1.3)

	case temperature > 300 && temperature < 380:
		// Умеренная зона — биосфера вверх
		multiplyIfExists(c, SurfaceForests, 1.2)
		multiplyIfExists(c, SurfaceMeadows, 1.2)
		multiplyIfExists(c, SurfaceLakes, 1.1)
	}

	// Абсолютные физические запреты (независимо от архетипа)
	if temperature < 500 {
		delete(c, SurfaceLavaFields)
	}
	if temperature > 320 {
		delete(c, SurfaceGlaciers)
		delete(c, SurfaceFrozenGases)
	}
}

// ==================== ПОВЕРХНОСТЬ: ВОДА ====================

// applySurfaceWaterModifiers — корректирует веса поверхности по проценту воды.
func applySurfaceWaterModifiers(c map[string]float64, waterPercent float64) {
	switch {
	case waterPercent > 70:
		multiplyIfExists(c, SurfaceOceans, 2.0)
		multiplyIfExists(c, SurfaceLakes, 1.3)
		multiplyIfExists(c, SurfaceCoralReefs, 1.5)
		multiplyIfExists(c, SurfaceSands, 0.5)
		multiplyIfExists(c, SurfaceRocks, 0.6)

	case waterPercent > 40:
		multiplyIfExists(c, SurfaceOceans, 1.4)
		multiplyIfExists(c, SurfaceLakes, 1.2)

	case waterPercent < 5:
		// Практически нет воды — водные и биосферные формы невозможны
		delete(c, SurfaceOceans)
		delete(c, SurfaceLakes)
		delete(c, SurfaceCoralReefs)
		delete(c, SurfaceSwamps)
		delete(c, SurfaceForests)
		delete(c, SurfaceJungles)
		delete(c, SurfaceMeadows)
		multiplyIfExists(c, SurfaceSands, 1.4)
		multiplyIfExists(c, SurfaceRocks, 1.2)
		multiplyIfExists(c, SurfaceCraters, 1.2)

	case waterPercent < 20:
		// Мало воды — самые требовательные формы убираем
		delete(c, SurfaceJungles)
		multiplyIfExists(c, SurfaceOceans, 0.3)
		multiplyIfExists(c, SurfaceLakes, 0.5)
		multiplyIfExists(c, SurfaceSwamps, 0.3)
		multiplyIfExists(c, SurfaceCoralReefs, 0.2)
		multiplyIfExists(c, SurfaceForests, 0.4)
		multiplyIfExists(c, SurfaceMeadows, 0.5)
		multiplyIfExists(c, SurfaceSands, 1.2)
	}
}

// ==================== ПОВЕРХНОСТЬ: БИОСФЕРНЫЕ НИШИ ====================

// applyBiosphereModifiers — делает конкретную биосферную форму доминирующей
// в её архетипной нише (иначе «леса» выигрывают всегда из-за веса):
//
//	джунгли — тёплые влажные миры (вода > 60%, T > 300);
//	болота   — обильная вода при умеренном тепле (вода > 55%, T 270–330);
//	луга     — умеренная вода (20–45%), мягкая температура (T 250–330);
//	леса     — остаются дефолтным биомом (умеренная ниша без усиления).
func applyBiosphereModifiers(c map[string]float64, temperature, waterPercent float64) {
	switch {
	case waterPercent > 60 && temperature > 300:
		// Джунгли
		multiplyIfExists(c, SurfaceJungles, 3.0)
		multiplyIfExists(c, SurfaceForests, 0.7)
		multiplyIfExists(c, SurfaceMeadows, 0.6)
		multiplyIfExists(c, SurfaceSwamps, 0.6)

	case waterPercent > 55 && temperature >= 270 && temperature <= 330:
		// Болота
		multiplyIfExists(c, SurfaceSwamps, 3.2)
		multiplyIfExists(c, SurfaceForests, 0.7)
		multiplyIfExists(c, SurfaceOceans, 0.55)

	case waterPercent >= 20 && waterPercent <= 45 && temperature >= 250 && temperature <= 330:
		// Луга
		multiplyIfExists(c, SurfaceMeadows, 2.4)
		multiplyIfExists(c, SurfaceForests, 0.7)
	}
}

// ==================== НЕДРА: ТЕМПЕРАТУРА ====================

// applySubterrainTempModifiers — корректировки недр по температуре.
func applySubterrainTempModifiers(c map[string]float64, temperature float64) {
	switch {
	case temperature > 700:
		multiplyIfExists(c, SubterrainMagmaChambers, 2.0)
		multiplyIfExists(c, SubterrainMagmaticRocks, 1.3)
		multiplyIfExists(c, SubterrainMetalCores, 1.3)
		multiplyIfExists(c, SubterrainRadioactiveZones, 1.2)
		delete(c, SubterrainGroundIce)
		delete(c, SubterrainCoalSeams)
		delete(c, SubterrainOilPockets)
		delete(c, SubterrainGasPockets)

	case temperature > 400:
		multiplyIfExists(c, SubterrainMagmaChambers, 1.4)
		multiplyIfExists(c, SubterrainMagmaticRocks, 1.2)
		multiplyIfExists(c, SubterrainGroundIce, 0.2)

	case temperature < 150:
		multiplyIfExists(c, SubterrainGroundIce, 1.8)
		multiplyIfExists(c, SubterrainMagmaChambers, 0.3)
		multiplyIfExists(c, SubterrainCoalSeams, 0.5)

	case temperature < 220:
		multiplyIfExists(c, SubterrainGroundIce, 1.3)
	}
}

// ==================== НЕДРА: ВОДА ====================

// applySubterrainWaterModifiers — корректировки недр по воде.
func applySubterrainWaterModifiers(c map[string]float64, waterPercent float64) {
	switch {
	case waterPercent > 60:
		multiplyIfExists(c, SubterrainGroundwater, 1.6)
		multiplyIfExists(c, SubterrainSedimentaryRocks, 1.3)
		multiplyIfExists(c, SubterrainSaltDomes, 1.2)

	case waterPercent < 5:
		multiplyIfExists(c, SubterrainGroundwater, 0.3)
		multiplyIfExists(c, SubterrainGroundIce, 0.3)
		multiplyIfExists(c, SubterrainOilPockets, 0.5)
		multiplyIfExists(c, SubterrainCoalSeams, 0.5)
	}
}

// ==================== НЕДРА: СВЯЗЬ С ПОВЕРХНОСТЬЮ ====================

// applySurfaceToSubterrainLinks — мягкое влияние поверхности на недра.
// Читает draft поверхности (слой 8, 99.2.28 §6.3). Костыль «лава → магмакамеры»
// удалён (99.2.28 §7): вулканизм поверхности и недр — один корень V.
func applySurfaceToSubterrainLinks(c map[string]float64, surface Composition) {
	if len(surface) == 0 {
		return
	}

	if surface.ShareOf(SurfaceOceans) > 30 {
		multiplyIfExists(c, SubterrainOilPockets, 1.4)
		multiplyIfExists(c, SubterrainSaltDomes, 1.3)
		multiplyIfExists(c, SubterrainSedimentaryRocks, 1.2)
	}

	if surface.ShareOf(SurfaceForests)+surface.ShareOf(SurfaceSwamps) > 20 {
		multiplyIfExists(c, SubterrainCoalSeams, 1.5)
		multiplyIfExists(c, SubterrainOilPockets, 1.2)
	}

	if surface.ShareOf(SurfaceGlaciers) > 30 {
		multiplyIfExists(c, SubterrainGroundIce, 1.5)
		multiplyIfExists(c, SubterrainCrystalVeins, 1.2)
	}

	if surface.ShareOf(SurfaceCraters) > 15 {
		multiplyIfExists(c, SubterrainOreVeins, 1.3)
		multiplyIfExists(c, SubterrainRareEarthVeins, 1.3)
	}
}