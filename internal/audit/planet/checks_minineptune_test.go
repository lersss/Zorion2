// internal/audit/planet/checks_minineptune_test.go
//
// Аудит мини-нептуна (спека 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны
// §11.2/§14.3, тест O17): размер обязан лежать на кривой
// R = 2.61·(M/8.6)^0.577 (minineptune_radius_mismatch — зеркало
// giant_radius_mismatch), пустые биомы/недры — норма класса, H/He-оболочка
// при высокой T — норма класса.
package planet

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"zorion/internal/audit"
	genplanet "zorion/internal/generator/planet"
)

// miniNeptuneData — мини-нептун с честными ρ = M/R³ и g = M/R².
func miniNeptuneData(mass, size float64) map[string]interface{} {
	d := baseData()
	d["type"] = "мини-нептун"
	d["is_gas_giant"] = false
	d["is_mini_neptune"] = true
	d["mass"] = mass
	d["size"] = size
	d["density"] = mass / (size * size * size)
	d["gravity"] = mass / (size * size)
	d["atmosphere"] = "водородно-гелиевая"
	d["hydrosphere"] = "сухая"
	d["biosphere"] = "стерильная"
	d["life"] = false
	d["surface_dominant"] = "мини-нептун"
	d["surface_composition"] = map[string]interface{}{}
	d["subterrain_composition"] = map[string]interface{}{}
	d["biomes"] = []interface{}{}
	d["subterrain"] = []interface{}{}
	d["moons"] = 2
	return d
}

// O17 — размер мини-нептуна обязан лежать на кривой §8.2.
func TestMinineptuneAuditCurve(t *testing.T) {
	// Честный мини-нептун 12 M⊕ — размер по кривой → без issue.
	d := miniNeptuneData(12, genplanet.MiniNeptuneRadius(12))
	assert.Empty(t, runCheck(checkMassSizeDensity, d))

	// Битый размер → minineptune_radius_mismatch (зеркало giant_radius_mismatch).
	d = miniNeptuneData(12, 2.0)
	issues := runCheck(checkMassSizeDensity, d)
	assertSingle(t, issues, "minineptune_radius_mismatch", audit.SeverityMedium)
	assert.InDelta(t, genplanet.MiniNeptuneRadius(12), issues[0].Details["expected"], 1e-9)
}

// Пустые биомы/недры — норма класса (иначе ложный High на каждом мини-нептуне).
func TestMinineptuneAuditNoBiomesExclusion(t *testing.T) {
	d := miniNeptuneData(12, genplanet.MiniNeptuneRadius(12))
	assert.Empty(t, runCheck(checkEmptyBiomes, d), "пустые биомы/недры — норма класса")
}

// H/He-оболочка при высокой T — норма класса (как у гиганта).
func TestMinineptuneAuditHydrogenInHeatExclusion(t *testing.T) {
	d := miniNeptuneData(12, genplanet.MiniNeptuneRadius(12))
	d["temperature"] = 900.0
	assert.Empty(t, runCheck(checkHydrogenInHeat, d))
}
