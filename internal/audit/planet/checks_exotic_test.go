// internal/audit/planet/checks_exotic_test.go
// Тесты спец-правил экзотики (99.2.4 §6.2): life_in_exotic_system и
// settleable_under_exotic — машинная гарантия инварианта ветки §5.3.
// Без ложных срабатываний на обычных звёздах (O–Y, без флага exotic_system).
package planet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/audit"
)

func TestAllRulesCount42(t *testing.T) {
	// Счётчик правил: 40 базовых + 2 спец-правила экзотики + 2 правила
	// физического каскада (99.2.20 §6.2: atmosphere_sum_not_100,
	// liquid_water_mismatch) + 3 правила биомов (99.2.28 §14:
	// biosphere_without_conditions, empty_biomes, biome_not_in_catalog) +
	// 1 правило истории формирования (спека 2026-09-22-облако-этап-2 §6.3:
	// formation_history_mismatch) = 48.
	require.Len(t, AllRules(), 48, "число правил аудита планет")
}

// exoticDeadPlanet — планета остатка по ветке §5.3: вода 0, жизнь 0,
// T внутри [20, 2500], settleable=false, exotic_system=true,
// композиция каменистая/кратерная (без океанов/биосферы).
func exoticDeadPlanet() map[string]interface{} {
	data := baseData()
	data["type"] = "мёртвая"
	data["temperature"] = 40.0
	data["water_percent"] = 0.0
	data["life"] = false
	data["settleable"] = false
	data["exotic_system"] = true
	data["star_type"] = "black_hole"
	data["surface_composition"] = map[string]interface{}{
		"горы":    60.0,
		"кратеры": 40.0,
	}
	return data
}

func TestLifeInExoticSystemFires(t *testing.T) {
	data := exoticDeadPlanet()
	data["life"] = true // нарушение ветки: жизнь у ЧД
	issues := runCheck(checkLifeInExoticSystem, data)
	require.Len(t, issues, 1)
	assert.Equal(t, "life_in_exotic_system", issues[0].Code)
	assert.Equal(t, audit.SeverityHigh, issues[0].Severity)
}

func TestLifeInExoticSystemCleanPlanetNoFire(t *testing.T) {
	assert.Empty(t, runCheck(checkLifeInExoticSystem, exoticDeadPlanet()),
		"мёртвая планета остатка не даёт жизни")
}

func TestLifeInExoticSystemNoFireOnNormalStar(t *testing.T) {
	// Обычная звезда без флага exotic_system — правило молчит даже при жизни.
	data := baseData()
	assert.Empty(t, runCheck(checkLifeInExoticSystem, data),
		"на O–Y (без exotic_system) ложных срабатываний нет (99.2.4 §6.2)")
}

func TestSettleableUnderExoticFires(t *testing.T) {
	// Пресет пригодности (T 200–350 K, вода ≥ 10) в экзотической системе.
	data := exoticDeadPlanet()
	data["temperature"] = 300.0
	data["water_percent"] = 40.0
	issues := runCheck(checkSettleableUnderExotic, data)
	require.Len(t, issues, 1)
	assert.Equal(t, "settleable_under_exotic", issues[0].Code)
}

func TestSettleableUnderExoticColdNoFire(t *testing.T) {
	// T вне пресета пригодности — не срабатывает (это норма ветки).
	assert.Empty(t, runCheck(checkSettleableUnderExotic, exoticDeadPlanet()))
}

func TestSettleableUnderExoticNoFireOnNormalStar(t *testing.T) {
	data := baseData()
	data["temperature"] = 300.0
	data["water_percent"] = 40.0
	assert.Empty(t, runCheck(checkSettleableUnderExotic, data),
		"на обычной звезде (без exotic_system) — молчит, даже пригодная планета")
}

func TestExoticBranchPlanetPassesAudit(t *testing.T) {
	// Планета, сгенерированная веткой §5.3 (как в exotic_test.go), проходит
	// ВСЕ 45 правил без единой проблемы.
	data := exoticDeadPlanet()
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	views := ParseAll([]Row{{ID: "p1", WorldID: "w1", Name: "Мёртвая", Data: raw}})
	require.Len(t, views, 1)
	res := audit.Run("planet", views, AllRules())
	assert.Zero(t, res.TotalIssues, "планета экзотической ветки не должна давать issues аудита")
}