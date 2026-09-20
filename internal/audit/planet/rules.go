// internal/audit/planet/rules.go
package planet

import "zorion/internal/audit"

// ==================== СПИСОК ПРАВИЛ ====================

// AllRules — все правила аудита планет.
//
// Каждое правило — функция, которая принимает *View и возвращает []audit.Issue.
// Порядок правил не важен: движок применяет все.
func AllRules() []audit.Rule[View] {
	return []audit.Rule[View]{
		// === Физика: базовые диапазоны ===
		{Check: checkTemperatureRange},
		{Check: checkMassSizeDensity},
		{Check: checkSurfaceSum},
		{Check: checkSubterrainSum},
		{Check: checkNegativeShares},

		// === Композиция vs температура ===
		{Check: checkJunglesInCold},
		{Check: checkForestsInCold},
		{Check: checkMeadowsInCold},
		{Check: checkSwampsInCold},
		{Check: checkCoralReefsInCold},
		{Check: checkGlaciersInHeat},
		{Check: checkLavaInCold},
		{Check: checkFrozenGasesInHeat},
		{Check: checkOceansInHeat},

		// === Композиция vs вода ===
		{Check: checkOceansWithoutWater},
		{Check: checkBiosphereWithoutWater},

		// === Атмосфера vs температура ===
		{Check: checkGreenhouseInCold},
		{Check: checkOxygenInHeat},
		{Check: checkMethaneInHeat},
		{Check: checkHydrogenInHeat},

		// === Ядро ===
		{Check: checkCoreRanges},
		{Check: checkCoreAge},
		{Check: checkCoreTypeMismatch},

		// === Спутники ===
		{Check: checkSatelliteTemperature},
		{Check: checkSatelliteMass},

		// === Жизнь и обитаемость ===
		{Check: checkLifeWithoutWater},
		{Check: checkLifeWithoutTemperature},

		// === Экзотические системы (99.2.4 §6.2) ===
		{Check: checkLifeInExoticSystem},
		{Check: checkSettleableUnderExotic},

		// === Type ↔ composition ===
		{Check: checkEarthlikeConsistency},
		{Check: checkOceanicConsistency},
		{Check: checkIceConsistency},
		{Check: checkGasGiantConsistency},
		{Check: checkVolcanicConsistency},
		{Check: checkDesertConsistency},
		{Check: checkRadioactiveConsistency},

		// === Химия ===
		{Check: checkRadioactiveWithoutZones},

		// === Мусор в данных ===
		{Check: checkNaNValues},
		{Check: checkZeroMass},
		{Check: checkZeroSize},
		{Check: checkZeroDensity},
		{Check: checkEmptyName},

		// === Физический каскад (99.2.20 §6.2): атмосфера-объект, флаг воды ===
		{Check: checkAtmosphereSumNot100},
		{Check: checkLiquidWaterMismatch},

		// === Биомы (99.2.28 §14): биосфера без условий, пустые биомы/недры,
		// биом вне справочника ===
		{Check: checkBiosphereWithoutConditions},
		{Check: checkEmptyBiomes},
		{Check: checkBiomeNotInCatalog},
	}
}

// ==================== ВЫСОКОУРОВНЕВЫЙ API ====================

// AuditRows — запускает аудит планет по строкам из БД.
// Удобный вход для хендлера: дал строки — получил результат.
func AuditRows(rows []Row) *audit.AuditResult {
	views := ParseAll(rows)
	return audit.Run("planet", views, AllRules())
}