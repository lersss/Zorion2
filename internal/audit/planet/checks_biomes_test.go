// internal/audit/planet/checks_biomes_test.go
//
// Тесты правил биомов (99.2.28 §14): biosphere_without_conditions,
// empty_biomes (ключ есть и пуст; отсутствие ключа = старый мир — не
// срабатывает), biome_not_in_catalog.
package planet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/audit"
)

// ==================== biosphere_without_conditions ====================

func TestBiosphereWithoutConditions(t *testing.T) {
	// Земная биосферная форма (тег «биосферный» + no_toxic) при отсутствии
	// life/флага воды/нетоксичной атмосферы → High.
	data := baseData()
	data["life"] = false
	data["biomes"] = []interface{}{
		map[string]interface{}{"form": "леса", "share": 40.0},
		map[string]interface{}{"form": "горы", "share": 60.0},
	}
	data["surface_composition"] = map[string]interface{}{"леса": 40.0, "горы": 60.0}
	data["liquid_water_possible"] = true
	data["atmosphere_data"] = map[string]interface{}{
		"composition": map[string]interface{}{"N2": 78.0, "O2": 21.0, "CO2": 1.0},
	}
	issues := runCheck(checkBiosphereWithoutConditions, data)
	require.Len(t, issues, 1, "леса без life → High")
	assert.Equal(t, "biosphere_without_conditions", issues[0].Code)
	assert.Equal(t, audit.SeverityHigh, issues[0].Severity)
}

func TestBiosphereWithConditionsClean(t *testing.T) {
	// Земная биосферная форма при life + флаге + нетоксичной атмосфере — чисто.
	data := baseData()
	data["life"] = true
	data["biomes"] = []interface{}{
		map[string]interface{}{"form": "леса", "share": 40.0},
		map[string]interface{}{"form": "горы", "share": 60.0},
	}
	data["liquid_water_possible"] = true
	data["surface_composition"] = map[string]interface{}{"леса": 40.0, "горы": 60.0}
	data["atmosphere_data"] = map[string]interface{}{
		"composition": map[string]interface{}{"N2": 78.0, "O2": 21.0, "CO2": 1.0},
	}
	assert.Empty(t, runCheck(checkBiosphereWithoutConditions, data))
}

func TestBiosphereExtremophilesNotChecked(t *testing.T) {
	// Экстремофилы (кислотные дебри — toxic) — жизнь без нетоксичной, не проверяются.
	data := baseData()
	data["life"] = true
	data["biomes"] = []interface{}{
		map[string]interface{}{"form": "кислотные_дебри", "share": 40.0},
		map[string]interface{}{"form": "горы", "share": 60.0},
	}
	data["liquid_water_possible"] = false
	data["surface_composition"] = map[string]interface{}{"кислотные_дебри": 40.0, "горы": 60.0}
	data["atmosphere_data"] = map[string]interface{}{
		"composition": map[string]interface{}{"N2": 90.0, "CH4": 3.0, "O2": 7.0},
	}
	assert.Empty(t, runCheck(checkBiosphereWithoutConditions, data))
}

func TestBiosphereOldWorldWithoutBiomesKey(t *testing.T) {
	// Старый мир без ключа biomes — поверхность из старых данных, правило
	// биомов не применяется (замечание ревью: не шуметь на старых мирах).
	data := baseData()
	data["life"] = false
	data["surface_composition"] = map[string]interface{}{"леса": 40.0, "горы": 60.0}
	data["liquid_water_possible"] = true
	data["atmosphere_data"] = map[string]interface{}{
		"composition": map[string]interface{}{"N2": 78.0, "O2": 21.0, "CO2": 1.0},
	}
	assert.Empty(t, runCheck(checkBiosphereWithoutConditions, data),
		"старый мир без biomes — без проблем")
}

// ==================== empty_biomes ====================

func TestEmptyBiomesKeyPresentAndEmpty(t *testing.T) {
	// Ключ biomes присутствует и пуст у не-гиганта → High.
	data := baseData()
	data["biomes"] = []interface{}{}
	data["subterrain"] = []interface{}{}
	issues := runCheck(checkEmptyBiomes, data)
	require.Len(t, issues, 2, "пустые biomes и subterrain → 2 проблемы")
	assert.Equal(t, "empty_biomes", issues[0].Code)
}

func TestEmptyBiomesMissingKeyOldWorld(t *testing.T) {
	// Отсутствие ключа = старый мир — НЕ проверяется (находка @critic).
	data := baseData()
	assert.Empty(t, runCheck(checkEmptyBiomes, data))
}

func TestEmptyBiomesGasGiant(t *testing.T) {
	// Газовый гигант — пустые биомы не ошибка (99.2.28 §9.5).
	data := baseData()
	data["is_gas_giant"] = true
	data["biomes"] = []interface{}{}
	data["subterrain"] = []interface{}{}
	assert.Empty(t, runCheck(checkEmptyBiomes, data))
}

func TestEmptyBiomesNonEmpty(t *testing.T) {
	data := baseData()
	data["biomes"] = []interface{}{
		map[string]interface{}{"form": "горы", "share": 60.0},
		map[string]interface{}{"form": "океаны", "share": 40.0},
	}
	data["subterrain"] = []interface{}{
		map[string]interface{}{"type": "пустая_порода", "share": 100.0},
	}
	assert.Empty(t, runCheck(checkEmptyBiomes, data))
}

// ==================== biome_not_in_catalog ====================

func TestBiomeNotInCatalog(t *testing.T) {
	// Биом вне справочника (старый мир с удалённым биомом) → Medium.
	data := baseData()
	data["surface_composition"] = map[string]interface{}{"удалённый_биом": 40.0, "горы": 60.0}
	issues := runCheck(checkBiomeNotInCatalog, data)
	require.Len(t, issues, 1)
	assert.Equal(t, "biome_not_in_catalog", issues[0].Code)
	assert.Equal(t, audit.SeverityMedium, issues[0].Severity)
}

func TestBiomeNotInCatalogClean(t *testing.T) {
	data := baseData()
	data["surface_composition"] = map[string]interface{}{"горы": 60.0, "океаны": 40.0}
	assert.Empty(t, runCheck(checkBiomeNotInCatalog, data))
}
