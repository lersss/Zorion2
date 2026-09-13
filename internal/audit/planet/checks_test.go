// internal/audit/planet/checks_test.go
package planet

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/audit"
)

// ==================== ХЕЛПЕРЫ ====================

// baseData — корректная «эталонная» землеподобная планета.
// Должна проходить ВСЕ правила без единой проблемы — это проверяет
// TestAllRulesCleanPlanet. Любой тест правила стартует отсюда и ломает
// ровно одно поле, чтобы ожидаемая проблема была предсказуемой.
func baseData() map[string]interface{} {
	return map[string]interface{}{
		"id":       "p1",
		"name":     "Тестовая",
		"world_id": "w1",

		"size":          1.0,
		"mass":          1.0,
		"density":       1.0,
		"temperature":   300.0,
		"water_percent": 40.0,

		"type":             "землеподобная",
		"surface_dominant": "скалы",
		"climate":          "умеренный",
		"atmosphere":       "азотно-кислородная",
		"hydrosphere":      "гидросфера",
		"biosphere":        "углеродная",

		"is_gas_giant": false,
		"radioactive":  false,
		"life":         true,

		"surface_composition": map[string]interface{}{
			"скалы":  60.0,
			"океаны": 40.0,
		},
		"subterrain_composition": map[string]interface{}{
			"пустые_породы":   60.0,
			"магматические": 40.0,
		},
		"core": map[string]interface{}{
			"type":          "силикатное",
			"mass_percent":  30.0,
			"activity":      30.0,
			"radioactivity": 10.0,
			"age":           5.0,
			"is_active":     true,
			"is_metallic":   false,
		},
		"satellites": []interface{}{},
	}
}

// view собирает View из карты данных (как из строки БД).
func view(t *testing.T, data map[string]interface{}) *View {
	t.Helper()
	return Parse(data)
}

// runCheck прогоняет одно правило над планетой.
func runCheck(check func(*View) []audit.Issue, data map[string]interface{}) []audit.Issue {
	return check(Parse(data))
}

// codes вытаскивает коды проблем.
func codes(issues []audit.Issue) []string {
	out := make([]string, 0, len(issues))
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

// assertSingle — ровно одна проблема с ожидаемым кодом и severity.
func assertSingle(t *testing.T, issues []audit.Issue, code, severity string) {
	t.Helper()
	require.Len(t, issues, 1, "ожидалась ровно одна проблема %s", code)
	assert.Equal(t, code, issues[0].Code)
	assert.Equal(t, severity, issues[0].Severity)
	assert.NotEmpty(t, issues[0].Description)
}

// ==================== ПАРСИНГ ====================

func TestParseBase(t *testing.T) {
	v := view(t, baseData())

	assert.Equal(t, "p1", v.ID)
	assert.Equal(t, "Тестовая", v.Name)
	assert.Equal(t, "w1", v.WorldID)

	assert.Equal(t, 1.0, v.Size)
	assert.Equal(t, 1.0, v.Mass)
	assert.Equal(t, 1.0, v.Density)
	assert.Equal(t, 300.0, v.Temperature)
	assert.Equal(t, 40.0, v.WaterPercent)

	assert.Equal(t, "землеподобная", v.Type)
	assert.Equal(t, "скалы", v.SurfaceDominant)
	assert.Equal(t, "умеренный", v.Climate)
	assert.Equal(t, "азотно-кислородная", v.Atmosphere)

	assert.False(t, v.IsGasGiant)
	assert.False(t, v.IsRadioactive)
	assert.True(t, v.Life)

	assert.Equal(t, map[string]float64{"скалы": 60, "океаны": 40}, v.Surface)
	assert.Equal(t, map[string]float64{"пустые_породы": 60, "магматические": 40}, v.Subterrain)

	require.NotNil(t, v.Core)
	assert.Equal(t, "силикатное", v.Core.Type)
	assert.Equal(t, 30.0, v.Core.MassPercent)
	assert.Equal(t, 10.0, v.Core.Radioactivity)
	assert.Equal(t, 5.0, v.Core.Age)
	assert.True(t, v.Core.IsActive)
	assert.False(t, v.Core.IsMetallic)

	assert.Empty(t, v.Satellites)
	assert.NotNil(t, v.Raw)
}

func TestParseCoreMissing(t *testing.T) {
	d := baseData()
	delete(d, "core")
	assert.Nil(t, view(t, d).Core)
}

func TestParseSatellites(t *testing.T) {
	d := baseData()
	d["satellites"] = []interface{}{
		map[string]interface{}{
			"id": "s1", "name": "Луна-1", "mass": 0.5, "size": 0.4,
			"temperature": 250.0, "water_percent": 10.0,
			"atmosphere": "разреженная", "habitable": false, "life": false,
		},
		"не карта", // мусорный элемент должен быть пропущен
		map[string]interface{}{
			"id": "s2", "name": "Луна-2", "mass": 0.1, "size": 0.2,
			"temperature": 120.0, "habitable": true, "life": true,
		},
	}

	v := view(t, d)
	require.Len(t, v.Satellites, 2)

	assert.Equal(t, "s1", v.Satellites[0].ID)
	assert.Equal(t, "Луна-1", v.Satellites[0].Name)
	assert.Equal(t, 0.5, v.Satellites[0].Mass)
	assert.Equal(t, 250.0, v.Satellites[0].Temperature)
	assert.Equal(t, "разреженная", v.Satellites[0].Atmosphere)

	assert.Equal(t, "s2", v.Satellites[1].ID)
	assert.True(t, v.Satellites[1].Habitable)
	assert.True(t, v.Satellites[1].Life)
}

func TestParseAll(t *testing.T) {
	valid, err := json.Marshal(baseData())
	require.NoError(t, err)

	rows := []Row{
		{ID: "valid", WorldID: "w1", Name: "Название", Data: valid},
		{ID: "broken", WorldID: "w2", Name: "Битый", Data: []byte("{not json")},
		{ID: "empty", WorldID: "w3", Name: "Пустой", Data: []byte("{}")},
	}

	views := ParseAll(rows)

	// Битый JSON пропускается — остаются 2 планеты.
	require.Len(t, views, 2)

	assert.Equal(t, "p1", views[0].ID)
	assert.Equal(t, "w1", views[0].WorldID)

	// Пустой JSON: ID/Name/WorldID берутся из строки БД.
	assert.Equal(t, "empty", views[1].ID)
	assert.Equal(t, "Пустой", views[1].Name)
	assert.Equal(t, "w3", views[1].WorldID)
}

// ==================== ЭТАЛОН: ЧИСТАЯ ПЛАНЕТА ====================

// TestAllRulesCleanPlanet — корректная сгенерированная планета не должна
// давать ни одной проблемы аудита. Это регрессия на согласованность
// генератора и правил (геймдизайн).
func TestAllRulesCleanPlanet(t *testing.T) {
	res := audit.Run("planet", []*View{Parse(baseData())}, AllRules())

	assert.Equal(t, 0, res.TotalIssues, "codes: %v", res.IssuesByCode)
	assert.Equal(t, 0, res.EntitiesWithIssue)
	assert.Empty(t, res.SampleIssues)
}

// ==================== ФИЗИКА: БАЗОВЫЕ ДИАПАЗОНЫ ====================

func TestTemperatureRange(t *testing.T) {
	d := baseData()
	d["temperature"] = 10.0
	assertSingle(t, runCheck(checkTemperatureRange, d), "temp_too_low", audit.SeverityHigh)

	d = baseData()
	d["temperature"] = 3000.0
	assertSingle(t, runCheck(checkTemperatureRange, d), "temp_too_high", audit.SeverityHigh)

	d = baseData()
	assert.Empty(t, runCheck(checkTemperatureRange, d))
}

func TestMassSizeDensity(t *testing.T) {
	d := baseData()
	d["size"] = 2.0 // (M/ρ)^(1/3) = 1.0, расхождение 1.0
	issues := runCheck(checkMassSizeDensity, d)
	assertSingle(t, issues, "mass_size_density_mismatch", audit.SeverityMedium)
	assert.Equal(t, 1.0, issues[0].Details["expected"])

	d = baseData()
	assert.Empty(t, runCheck(checkMassSizeDensity, d))
}

func TestSurfaceSum(t *testing.T) {
	d := baseData()
	d["surface_composition"] = map[string]interface{}{"скалы": 30.0, "океаны": 55.0}
	issues := runCheck(checkSurfaceSum, d)
	assertSingle(t, issues, "surface_sum_not_100", audit.SeverityHigh)
	assert.Equal(t, 85.0, issues[0].Details["sum"])

	d = baseData()
	assert.Empty(t, runCheck(checkSurfaceSum, d))
}

func TestSubterrainSum(t *testing.T) {
	d := baseData()
	d["subterrain_composition"] = map[string]interface{}{"пустые_породы": 30.0}
	issues := runCheck(checkSubterrainSum, d)
	assertSingle(t, issues, "subterrain_sum_not_100", audit.SeverityHigh)
	assert.Equal(t, 30.0, issues[0].Details["sum"])

	d = baseData()
	assert.Empty(t, runCheck(checkSubterrainSum, d))
}

func TestNegativeShares(t *testing.T) {
	d := baseData()
	d["surface_composition"] = map[string]interface{}{"скалы": 105.0, "странность": -5.0}
	issues := runCheck(checkNegativeShares, d)
	assertSingle(t, issues, "negative_surface_share", audit.SeverityHigh)
	assert.Equal(t, "странность", issues[0].Details["form"])

	d = baseData()
	d["subterrain_composition"] = map[string]interface{}{"пустые_породы": 110.0, "дыра": -10.0}
	issues = runCheck(checkNegativeShares, d)
	assertSingle(t, issues, "negative_subterrain_share", audit.SeverityHigh)
	assert.Equal(t, "дыра", issues[0].Details["type"])

	assert.Empty(t, runCheck(checkNegativeShares, baseData()))
}

// ==================== КОМПОЗИЦИЯ vs ТЕМПЕРАТУРА ====================

func TestJunglesInCold(t *testing.T) {
	d := baseData()
	d["temperature"] = 150.0
	d["surface_composition"] = map[string]interface{}{"джунгли": 40.0, "скалы": 60.0}
	assertSingle(t, runCheck(checkJunglesInCold, d), "jungles_in_cold", audit.SeverityLow)

	d = baseData()
	d["temperature"] = 290.0
	d["surface_composition"] = map[string]interface{}{"джунгли": 40.0, "скалы": 60.0}
	assert.Empty(t, runCheck(checkJunglesInCold, d))
}

func TestLavaInCold(t *testing.T) {
	d := baseData()
	d["temperature"] = 300.0
	d["surface_composition"] = map[string]interface{}{"лавовые_поля": 30.0, "скалы": 70.0}
	assertSingle(t, runCheck(checkLavaInCold, d), "lava_in_cold", audit.SeverityHigh)

	d = baseData()
	d["temperature"] = 600.0
	d["surface_composition"] = map[string]interface{}{"лавовые_поля": 30.0, "скалы": 70.0}
	assert.Empty(t, runCheck(checkLavaInCold, d))
}

func TestGlaciersInHeat(t *testing.T) {
	d := baseData()
	d["temperature"] = 350.0
	d["surface_composition"] = map[string]interface{}{"ледники": 30.0, "скалы": 70.0}
	assertSingle(t, runCheck(checkGlaciersInHeat, d), "glaciers_in_heat", audit.SeverityMedium)

	d = baseData()
	d["temperature"] = 250.0
	d["surface_composition"] = map[string]interface{}{"ледники": 30.0, "скалы": 70.0}
	assert.Empty(t, runCheck(checkGlaciersInHeat, d))
}

func TestFrozenGasesInHeat(t *testing.T) {
	d := baseData()
	d["temperature"] = 300.0
	d["surface_composition"] = map[string]interface{}{"мёрзлые_газы": 20.0, "скалы": 80.0}
	assertSingle(t, runCheck(checkFrozenGasesInHeat, d), "frozen_gases_in_heat", audit.SeverityMedium)

	d = baseData()
	d["temperature"] = 200.0
	d["surface_composition"] = map[string]interface{}{"мёрзлые_газы": 20.0, "скалы": 80.0}
	assert.Empty(t, runCheck(checkFrozenGasesInHeat, d))
}

func TestOceansInHeat(t *testing.T) {
	d := baseData()
	d["temperature"] = 500.0
	d["surface_composition"] = map[string]interface{}{"океаны": 40.0, "скалы": 60.0}
	assertSingle(t, runCheck(checkOceansInHeat, d), "oceans_in_heat", audit.SeverityHigh)

	d = baseData()
	assert.Empty(t, runCheck(checkOceansInHeat, d))
}

// ==================== КОМПОЗИЦИЯ vs ВОДА ====================

func TestOceansWithoutWater(t *testing.T) {
	d := baseData()
	d["water_percent"] = 0.0
	assertSingle(t, runCheck(checkOceansWithoutWater, d), "oceans_without_water", audit.SeverityHigh)

	d = baseData()
	d["water_percent"] = 10.0
	assert.Empty(t, runCheck(checkOceansWithoutWater, d))
}

func TestBiosphereWithoutWater(t *testing.T) {
	d := baseData()
	d["water_percent"] = 3.0
	d["surface_composition"] = map[string]interface{}{"леса": 30.0, "джунгли": 20.0, "скалы": 50.0}
	issues := runCheck(checkBiosphereWithoutWater, d)
	assert.ElementsMatch(t, []string{"biosphere_without_water", "biosphere_without_water"}, codes(issues))
	for _, i := range issues {
		assert.Equal(t, audit.SeverityLow, i.Severity)
	}

	d = baseData()
	d["water_percent"] = 15.0
	d["surface_composition"] = map[string]interface{}{"леса": 30.0, "джунгли": 20.0, "скалы": 50.0}
	assert.Empty(t, runCheck(checkBiosphereWithoutWater, d))
}

// ==================== АТМОСФЕРА vs ТЕМПЕРАТУРА ====================

func TestGreenhouseInCold(t *testing.T) {
	d := baseData()
	d["atmosphere"] = "парниковая"
	d["temperature"] = 100.0
	assertSingle(t, runCheck(checkGreenhouseInCold, d), "greenhouse_in_cold", audit.SeverityHigh)

	d = baseData()
	d["atmosphere"] = "парниковая"
	d["temperature"] = 300.0
	assert.Empty(t, runCheck(checkGreenhouseInCold, d))
}

func TestOxygenInHeat(t *testing.T) {
	d := baseData()
	d["temperature"] = 800.0
	assertSingle(t, runCheck(checkOxygenInHeat, d), "oxygen_in_heat", audit.SeverityMedium)

	d = baseData()
	assert.Empty(t, runCheck(checkOxygenInHeat, d))
}

func TestMethaneInHeat(t *testing.T) {
	d := baseData()
	d["atmosphere"] = "метановая"
	d["temperature"] = 400.0
	assertSingle(t, runCheck(checkMethaneInHeat, d), "methane_in_heat", audit.SeverityMedium)

	d = baseData()
	d["atmosphere"] = "метановая"
	d["temperature"] = 200.0
	assert.Empty(t, runCheck(checkMethaneInHeat, d))
}

func TestHydrogenInHeat(t *testing.T) {
	d := baseData()
	d["atmosphere"] = "водородно-гелиевая"
	d["temperature"] = 900.0
	assertSingle(t, runCheck(checkHydrogenInHeat, d), "hydrogen_in_heat", audit.SeverityMedium)

	// Газовый гигант: H-He при любой температуре — норма.
	d = baseData()
	d["atmosphere"] = "водородно-гелиевая"
	d["temperature"] = 900.0
	d["type"] = "газовый гигант"
	d["is_gas_giant"] = true
	assert.Empty(t, runCheck(checkHydrogenInHeat, d))
}

// ==================== ЯДРО ====================

func TestCoreRanges(t *testing.T) {
	d := baseData()
	d["core"].(map[string]interface{})["activity"] = 150.0
	d["core"].(map[string]interface{})["radioactivity"] = -5.0
	d["core"].(map[string]interface{})["mass_percent"] = 120.0

	issues := runCheck(checkCoreRanges, d)
	assert.ElementsMatch(t, []string{
		"core_activity_out_of_range",
		"core_radioactivity_out_of_range",
		"core_mass_percent_out_of_range",
	}, codes(issues))
	for _, i := range issues {
		assert.Equal(t, audit.SeverityHigh, i.Severity)
	}

	assert.Empty(t, runCheck(checkCoreRanges, baseData()))
}

func TestCoreAge(t *testing.T) {
	d := baseData()
	d["core"].(map[string]interface{})["age"] = 20.0
	assertSingle(t, runCheck(checkCoreAge, d), "core_age_too_high", audit.SeverityHigh)

	d = baseData()
	d["core"].(map[string]interface{})["age"] = 13.0
	assert.Empty(t, runCheck(checkCoreAge, d))
}

func TestCoreTypeMismatch(t *testing.T) {
	d := baseData()
	d["core"].(map[string]interface{})["is_metallic"] = true // type=силикатное
	assertSingle(t, runCheck(checkCoreTypeMismatch, d), "core_metallic_flag_mismatch", audit.SeverityMedium)

	d = baseData()
	d["core"].(map[string]interface{})["type"] = "металлическое"
	d["core"].(map[string]interface{})["is_metallic"] = false
	assertSingle(t, runCheck(checkCoreTypeMismatch, d), "core_metallic_flag_mismatch", audit.SeverityMedium)

	d = baseData()
	d["core"].(map[string]interface{})["type"] = "металлическое"
	d["core"].(map[string]interface{})["is_metallic"] = true
	assert.Empty(t, runCheck(checkCoreTypeMismatch, d))
}

// ==================== СПУТНИКИ ====================

func TestSatelliteTemperature(t *testing.T) {
	d := baseData()
	d["type"] = "газовый гигант"
	d["is_gas_giant"] = true
	d["temperature"] = 200.0
	d["satellites"] = []interface{}{
		map[string]interface{}{"id": "s1", "name": "Ио", "temperature": 400.0},
	}
	assertSingle(t, runCheck(checkSatelliteTemperature, d), "satellite_hotter_than_giant", audit.SeverityMedium)

	// Не газовый гигант — правило не применяется.
	d = baseData()
	d["satellites"] = []interface{}{
		map[string]interface{}{"id": "s1", "name": "Ио", "temperature": 400.0},
	}
	assert.Empty(t, runCheck(checkSatelliteTemperature, d))
}

func TestSatelliteMass(t *testing.T) {
	d := baseData()
	d["type"] = "газовый гигант"
	d["is_gas_giant"] = true
	d["satellites"] = []interface{}{
		map[string]interface{}{"id": "s1", "name": "Гигант", "mass": 6.0},
	}
	assertSingle(t, runCheck(checkSatelliteMass, d), "satellite_mass_too_high", audit.SeverityLow)

	d = baseData()
	d["type"] = "газовый гигант"
	d["is_gas_giant"] = true
	d["satellites"] = []interface{}{
		map[string]interface{}{"id": "s1", "name": "Норм", "mass": 2.0},
	}
	assert.Empty(t, runCheck(checkSatelliteMass, d))
}

// ==================== ЖИЗНЬ И ОБИТАЕМОСТЬ ====================

func TestLifeWithoutWater(t *testing.T) {
	d := baseData()
	d["water_percent"] = 2.0
	assertSingle(t, runCheck(checkLifeWithoutWater, d), "life_without_water", audit.SeverityHigh)

	d = baseData()
	d["water_percent"] = 10.0
	assert.Empty(t, runCheck(checkLifeWithoutWater, d))
}

func TestLifeWithoutTemperature(t *testing.T) {
	d := baseData()
	d["temperature"] = 100.0
	assertSingle(t, runCheck(checkLifeWithoutTemperature, d), "life_without_temperature", audit.SeverityHigh)

	d = baseData()
	assert.Empty(t, runCheck(checkLifeWithoutTemperature, d))
}

// ==================== TYPE ↔ КОМПОЗИЦИЯ ====================

func TestEarthlikeConsistency(t *testing.T) {
	d := baseData()
	d["surface_composition"] = map[string]interface{}{"пески_пустыни": 100.0}
	issues := runCheck(checkEarthlikeConsistency, d)
	assert.ElementsMatch(t, []string{"earthlike_without_rocks", "earthlike_without_water"}, codes(issues))

	assert.Empty(t, runCheck(checkEarthlikeConsistency, baseData()))
}

func TestOceanicConsistency(t *testing.T) {
	d := baseData()
	d["type"] = "океаническая"
	d["surface_composition"] = map[string]interface{}{"скалы": 100.0}
	d["water_percent"] = 50.0
	issues := runCheck(checkOceanicConsistency, d)
	assert.ElementsMatch(t, []string{"oceanic_without_oceans", "oceanic_low_water"}, codes(issues))

	d = baseData()
	d["type"] = "океаническая"
	d["surface_composition"] = map[string]interface{}{"океаны": 80.0, "скалы": 20.0}
	d["water_percent"] = 80.0
	assert.Empty(t, runCheck(checkOceanicConsistency, d))
}

func TestIceConsistency(t *testing.T) {
	d := baseData()
	d["type"] = "ледяная"
	d["temperature"] = 300.0
	d["surface_composition"] = map[string]interface{}{"скалы": 100.0}
	assertSingle(t, runCheck(checkIceConsistency, d), "ice_not_cold", audit.SeverityHigh)

	// Либо холодно, либо есть ледники — достаточно одного.
	d = baseData()
	d["type"] = "ледяная"
	d["temperature"] = 200.0
	d["surface_composition"] = map[string]interface{}{"скалы": 100.0}
	assert.Empty(t, runCheck(checkIceConsistency, d))

	d = baseData()
	d["type"] = "ледяная"
	d["temperature"] = 300.0
	d["surface_composition"] = map[string]interface{}{"ледники": 100.0}
	assert.Empty(t, runCheck(checkIceConsistency, d))
}

func TestGasGiantConsistency(t *testing.T) {
	d := baseData()
	d["type"] = "газовый гигант" // is_gas_giant остаётся false
	assertSingle(t, runCheck(checkGasGiantConsistency, d), "gas_giant_flag_mismatch", audit.SeverityHigh)

	d = baseData()
	d["type"] = "газовый гигант"
	d["is_gas_giant"] = true
	assert.Empty(t, runCheck(checkGasGiantConsistency, d))
}

func TestVolcanicConsistency(t *testing.T) {
	d := baseData()
	d["type"] = "вулканическая"
	d["surface_composition"] = map[string]interface{}{"скалы": 100.0}
	assertSingle(t, runCheck(checkVolcanicConsistency, d), "volcanic_without_lava", audit.SeverityMedium)

	d = baseData()
	d["type"] = "вулканическая"
	d["surface_composition"] = map[string]interface{}{"лавовые_поля": 70.0, "скалы": 30.0}
	assert.Empty(t, runCheck(checkVolcanicConsistency, d))
}

func TestDesertConsistency(t *testing.T) {
	d := baseData()
	d["type"] = "пустынная"
	d["surface_composition"] = map[string]interface{}{"скалы": 100.0}
	assertSingle(t, runCheck(checkDesertConsistency, d), "desert_without_sands", audit.SeverityMedium)

	d = baseData()
	d["type"] = "пустынная"
	d["surface_composition"] = map[string]interface{}{"пески_пустыни": 100.0}
	assert.Empty(t, runCheck(checkDesertConsistency, d))
}

func TestRadioactiveConsistency(t *testing.T) {
	d := baseData()
	d["type"] = "радиоактивная" // radioactive остаётся false
	assertSingle(t, runCheck(checkRadioactiveConsistency, d), "radioactive_flag_mismatch", audit.SeverityMedium)

	d = baseData()
	d["type"] = "радиоактивная"
	d["radioactive"] = true
	assert.Empty(t, runCheck(checkRadioactiveConsistency, d))
}

func TestRadioactiveWithoutZones(t *testing.T) {
	d := baseData()
	d["radioactive"] = true
	d["subterrain_composition"] = map[string]interface{}{"радиоактивные_зоны": 3.0}
	assertSingle(t, runCheck(checkRadioactiveWithoutZones, d), "radioactive_without_zones", audit.SeverityMedium)

	d = baseData()
	d["radioactive"] = true
	d["subterrain_composition"] = map[string]interface{}{"радиоактивные_зоны": 8.0}
	assert.Empty(t, runCheck(checkRadioactiveWithoutZones, d))
}

// ==================== NaN / НУЛИ / ПУСТОТА ====================

func TestNaNValues(t *testing.T) {
	d := baseData()
	d["mass"] = math.NaN()
	issues := runCheck(checkNaNValues, d)
	assertSingle(t, issues, "nan_or_inf_value", audit.SeverityHigh)
	assert.Equal(t, "mass", issues[0].Details["field"])

	d = baseData()
	d["temperature"] = math.Inf(1)
	assertSingle(t, runCheck(checkNaNValues, d), "nan_or_inf_value", audit.SeverityHigh)

	assert.Empty(t, runCheck(checkNaNValues, baseData()))
}

func TestZeroMass(t *testing.T) {
	d := baseData()
	d["mass"] = 0.0
	assertSingle(t, runCheck(checkZeroMass, d), "zero_mass", audit.SeverityHigh)

	d = baseData()
	d["mass"] = -1.0
	assertSingle(t, runCheck(checkZeroMass, d), "zero_mass", audit.SeverityHigh)

	assert.Empty(t, runCheck(checkZeroMass, baseData()))
}

func TestZeroSize(t *testing.T) {
	d := baseData()
	d["size"] = 0.0
	assertSingle(t, runCheck(checkZeroSize, d), "zero_size", audit.SeverityHigh)

	assert.Empty(t, runCheck(checkZeroSize, baseData()))
}

func TestZeroDensity(t *testing.T) {
	d := baseData()
	d["density"] = 0.0
	assertSingle(t, runCheck(checkZeroDensity, d), "zero_density", audit.SeverityHigh)

	// Газовый гигант без плотности — допустимо.
	d = baseData()
	d["density"] = 0.0
	d["is_gas_giant"] = true
	assert.Empty(t, runCheck(checkZeroDensity, d))

	assert.Empty(t, runCheck(checkZeroDensity, baseData()))
}

func TestEmptyName(t *testing.T) {
	d := baseData()
	d["name"] = ""
	assertSingle(t, runCheck(checkEmptyName, d), "empty_name", audit.SeverityMedium)

	d = baseData()
	assert.Empty(t, runCheck(checkEmptyName, d))
}
