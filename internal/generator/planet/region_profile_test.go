// internal/generator/planet/region_profile_test.go
package planet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/regionprofile"
	"zorion/internal/resource"
)

// fp — указатель на float64 (для поля-указателя PlanetCountMult).
func fp(v float64) *float64 { return &v }

// ==================== ЧИСЛО ПЛАНЕТ (59a §8 P3, инвариант §11.4) ====================

func TestProfilePlanetCountMult(t *testing.T) {
	g := NewGenerator(nil, 1)
	g.means = DefaultPlanetMeans()

	// Без профиля — дефолт.
	avgBase := planetCountAverage(g, "G", 20_000)
	assert.InDelta(t, 1.75, avgBase, 0.1)

	// С профилем ×1.3 — mean × mult перед потолком 8.
	g.profile = &regionprofile.Profile{Planet: regionprofile.PlanetMods{PlanetCountMult: fp(1.3)}}
	g.profileIntensity = regionprofile.Strong
	avgBoosted := planetCountAverage(g, "G", 20_000)
	assert.InDelta(t, 1.75*1.3, avgBoosted, 0.15)
	assert.LessOrEqual(t, avgBoosted, 8.0, "потолок 8 не нарушается")

	// Слабый профиль — слабый сдвиг.
	g.profileIntensity = regionprofile.Weak
	avgWeak := planetCountAverage(g, "G", 20_000)
	assert.InDelta(t, 1.75*(1+(1.3-1)*0.3), avgWeak, 0.15)
}

// TestProfilePlanetCountPerWorld — регрессия stale-профиля (ревью гейта 2):
// продакшн-поток generateWorldIntoBuffer: два мира в разных регионах с
// разными planet_count_mult — каждый мир считает планеты по СВОЕМУ региону,
// а не по предыдущему (баг: planetCountFor вычислялся до выставления
// g.profile, счёт брал профиль предыдущего мира).
func TestProfilePlanetCountPerWorld(t *testing.T) {
	g := NewGenerator(nil, 1)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(1000)

	w1 := WorldInfo{ID: "w1", Name: "W1", SpectralClass: "G", StarType: "star",
		Profile: &regionprofile.Profile{Planet: regionprofile.PlanetMods{PlanetCountMult: fp(1.3)}},
		ProfileIntensity: regionprofile.Strong}
	w2 := WorldInfo{ID: "w2", Name: "W2", SpectralClass: "G", StarType: "star",
		Profile: &regionprofile.Profile{Planet: regionprofile.PlanetMods{PlanetCountMult: fp(0.7)}},
		ProfileIntensity: regionprofile.Strong}

	const n = 5000
	for i := 0; i < n; i++ {
		g.generateWorldIntoBuffer(w1, buf)
		g.generateWorldIntoBuffer(w2, buf)
	}

	counts := map[string]int{}
	for _, row := range buf.planetRows {
		fields := row.([]interface{})
		counts[fields[1].(string)]++
	}
	// w1: mean 1.75 × 1.3 = 2.275; w2: mean 1.75 × 0.7 = 1.225.
	assert.InDelta(t, 1.75*1.3, float64(counts["w1"])/n, 0.15, "мир 1 считает по своему региону")
	assert.InDelta(t, 1.75*0.7, float64(counts["w2"])/n, 0.15, "мир 2 считает по своему региону, а не по предыдущему")
}

func planetCountAverage(g *Generator, cls string, n int) float64 {
	total := 0
	for i := 0; i < n; i++ {
		total += g.determinePlanetCount(cls)
	}
	return float64(total) / float64(n)
}

// ==================== ГАЗОВЫЕ ГИГАНТЫ (59a §8 P4, инвариант §11.5) ====================

func TestProfileGasGiantShiftClamped(t *testing.T) {
	g := NewGenerator(nil, 1)

	// L/T/Y: 0.1 + 0.2 = 0.3 (в диапазоне).
	g.profile = &regionprofile.Profile{Planet: regionprofile.PlanetMods{GasGiantShift: 0.2}}
	g.profileIntensity = regionprofile.Strong
	assert.InDelta(t, 0.3, g.gasGiantChanceShifted("L"), 0.001)

	// O/B/A: 0.8 + 0.2 = 1.0 → кламп 0.95.
	assert.InDelta(t, 0.95, g.gasGiantChanceShifted("O"), 0.001)

	// 0.1 − 0.2 = −0.1 → кламп 0.05.
	g.profile = &regionprofile.Profile{Planet: regionprofile.PlanetMods{GasGiantShift: -0.2}}
	assert.InDelta(t, 0.05, g.gasGiantChanceShifted("L"), 0.001)

	// Без профиля — дефолт.
	g.profile = nil
	assert.InDelta(t, 0.1, g.gasGiantChanceShifted("L"), 0.001)
}

// ==================== ВЕСА ПОЛОСЫ (59a §8, ограничение O4) ====================

func TestProfileSurfaceBiasShiftsDominant(t *testing.T) {
	dominantShare := func(profile *regionprofile.Profile, intensity regionprofile.Intensity) map[string]int {
		g := NewGenerator(nil, 42)
		g.profile = profile
		g.profileIntensity = intensity
		counts := map[string]int{}
		const n = 3000
		for i := 0; i < n; i++ {
			res := g.runCascade(cascadeInput{
				Luminosity: 1, StellarMass: 1, AgeGyr: 5, Metallicity: 0, TEff: 5778,
				OrbitRadiusAU: 1, OrbitIndex: 1,
			})
			counts[res.Surface.DominantForm()]++
		}
		return counts
	}

	base := dominantShare(nil, 0)
	biased := dominantShare(&regionprofile.Profile{Planet: regionprofile.PlanetMods{
		SurfaceBias: map[string]float64{"луга_степи": 3.0},
	}}, regionprofile.Strong)

	baseMeadow := float64(base["луга_степи"]) / 3000
	biasedMeadow := float64(biased["луга_степи"]) / 3000
	assert.Greater(t, biasedMeadow, baseMeadow*1.5,
		"surface_bias должен поднять долю доминирующих лугов (base=%.3f, biased=%.3f)", baseMeadow, biasedMeadow)
}

func TestProfileSubterrainBiasShiftsComposition(t *testing.T) {
	groundwaterShare := func(profile *regionprofile.Profile, intensity regionprofile.Intensity) float64 {
		g := NewGenerator(nil, 7)
		g.profile = profile
		g.profileIntensity = intensity
		share := 0.0
		const n = 2000
		for i := 0; i < n; i++ {
			res := g.runCascade(cascadeInput{
				Luminosity: 1, StellarMass: 1, AgeGyr: 5, Metallicity: 0, TEff: 5778,
				OrbitRadiusAU: 1, OrbitIndex: 1,
			})
			share += res.Subterrain[SubterrainGroundwater]
		}
		return share / n
	}

	base := groundwaterShare(nil, 0)
	biased := groundwaterShare(&regionprofile.Profile{Planet: regionprofile.PlanetMods{
		SubterrainBias: map[string]float64{"подземные_воды": 3.0},
	}}, regionprofile.Strong)
	assert.Greater(t, biased, base*1.5,
		"subterrain_bias должен поднять долю подземных вод (base=%.3f, biased=%.3f)", base, biased)
}

// ==================== РЕСУРСЫ (59a §8 P2) ====================

func TestProfileResourceBiasWeightsCategories(t *testing.T) {
	g := NewGenerator(nil, 3)
	g.profile = &regionprofile.Profile{Planet: regionprofile.PlanetMods{
		ResourceBias: map[string]float64{"rare": 10.0},
	}}
	g.profileIntensity = regionprofile.Strong

	// "горы" → mineral+rare; "рудные_жилы" → mineral+rare. С bias rare 10
	// редкие должны доминировать.
	res := resource.GenerateResources("p1", "горы", map[string]float64{"рудные_жилы": 100},
		"G", g.rng, g.resourceBias())
	require.NotEmpty(t, res)
	rare := 0
	for _, r := range res {
		if r.Category == "rare" {
			rare++
		}
	}
	assert.Greater(t, float64(rare)/float64(len(res)), 0.5, "bias rare 10 должен доминировать")

	// Без профиля — равномерно (mineral и rare примерно поровну).
	g.profile = nil
	res2 := resource.GenerateResources("p1", "горы", map[string]float64{"рудные_жилы": 100},
		"G", g.rng, g.resourceBias())
	require.NotEmpty(t, res2)
	rare2 := 0
	for _, r := range res2 {
		if r.Category == "rare" {
			rare2++
		}
	}
	assert.InDelta(t, 0.5, float64(rare2)/float64(len(res2)), 0.3)
}