// internal/audit/planet/checks_giant_test.go
//
// Аудит газовых гигантов (99.2.15 §7.4): честный гигант проходит по кривой
// M→R, битый размер (не на кривой) флагуется как giant_radius_mismatch,
// обычные планеты проверяются как раньше ((M/ρ)^(1/3)).
package planet

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"zorion/internal/audit"
	genplanet "zorion/internal/generator/planet"
)

// giantData — газовый гигант с честными ρ = M/R³ и g = M/R².
func giantData(mass, size float64) map[string]interface{} {
	d := baseData()
	d["type"] = "газовый гигант"
	d["is_gas_giant"] = true
	d["mass"] = mass
	d["size"] = size
	d["density"] = mass / (size * size * size)
	d["gravity"] = mass / (size * size)
	d["atmosphere"] = "водородно-гелиевая"
	d["hydrosphere"] = "сухая"
	d["biosphere"] = "стерильная"
	d["life"] = false
	d["surface_dominant"] = "газовый_гигант"
	d["moons"] = 8
	return d
}

// Честный гигант 13 MJ (4131 M⊕, R=11.2 по кривой) — без issue по физике.
func TestCheckMassSizeDensityHonestGiant(t *testing.T) {
	d := giantData(genplanet.GasGiantMassMax, genplanet.GasGiantRadiusMax)
	assert.Empty(t, runCheck(checkMassSizeDensity, d))

	// Лёгкий гигант на кривой (15.9 → 5.9) — тоже без issue.
	d = giantData(genplanet.GasGiantMassMin, genplanet.GasGiantRadiusMin)
	assert.Empty(t, runCheck(checkMassSizeDensity, d))
}

// Битый гигант: масса 13 MJ, но размер 5 R⊕ (не на кривой). Плотность
// самосогласована (ρ = M/R³) — старую проверку (M/ρ)^(1/3) это не поймало
// бы; ловит именно инвариант кривой gasGiantRadius.
func TestCheckMassSizeDensityBrokenGiant(t *testing.T) {
	d := giantData(genplanet.GasGiantMassMax, 5.0)
	issues := runCheck(checkMassSizeDensity, d)
	assertSingle(t, issues, "giant_radius_mismatch", audit.SeverityMedium)
	assert.InDelta(t, genplanet.GasGiantRadiusMax, issues[0].Details["expected"], 1e-9)
}

// Обычная планета (не гигант) — как раньше: (M/ρ)^(1/3) с допуском 0.05.
func TestCheckMassSizeDensityNonGiantUnchanged(t *testing.T) {
	d := baseData()
	d["size"] = 2.0 // (M/ρ)^(1/3) = 1.0, расхождение 1.0
	issues := runCheck(checkMassSizeDensity, d)
	assertSingle(t, issues, "mass_size_density_mismatch", audit.SeverityMedium)

	// Гигант с самосогласованной физикой, но флагом false — не гигант для
	// правила: у такого «гиганта» ρ = 2.94 и R = 11.2 при M = 4131 —
	// (4131/2.94)^(1/3) = 11.2, расхождения нет.
	d = baseData()
	d["mass"] = genplanet.GasGiantMassMax
	d["size"] = genplanet.GasGiantRadiusMax
	d["density"] = genplanet.GasGiantMassMax / (genplanet.GasGiantRadiusMax * genplanet.GasGiantRadiusMax * genplanet.GasGiantRadiusMax)
	assert.Empty(t, runCheck(checkMassSizeDensity, d))
}