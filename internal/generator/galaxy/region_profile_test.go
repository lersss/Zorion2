// internal/generator/galaxy/region_profile_test.go
package galaxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/regionprofile"
)

// fp — указатель на float64 (для полей-указателей VariableMult/PlanetCountMult).
func fp(v float64) *float64 { return &v }

// ==================== СПЕКТРАЛЬНЫЙ СДВИГ (59a §7) ====================

func TestProfileSpectralShiftSoft(t *testing.T) {
	g := NewGenerator(&Config{Seed: 1})
	g.profile = &regionprofile.Profile{Star: regionprofile.StarMods{
		SpectralMult: map[string]float64{"O": 2.5, "B": 1.5, "A": 1.2, "K": 0.8, "M": 0.6},
	}}
	g.profileIntensity = regionprofile.Strong

	counts := map[string]int{}
	const n = 100_000
	for i := 0; i < n; i++ {
		counts[g.spectralClass()]++
	}
	// Инвариант §11.1: все классы возможны (мягкость).
	for _, cls := range spectralOrder {
		assert.Greater(t, counts[cls], 0, "класс %s должен оставаться возможным", cls)
	}
	// Сдвиг: O/B выше дефолта, M ниже.
	def := DefaultWeights().Spectral
	assert.Greater(t, float64(counts["O"])/n, def["O"]/100.0, "O должен быть выше дефолта")
	assert.Greater(t, float64(counts["B"])/n, def["B"]/100.0, "B должен быть выше дефолта")
	assert.Less(t, float64(counts["M"])/n, def["M"]/100.0, "M должен быть ниже дефолта")
}

func TestProfileSpectralShiftWeakIsMild(t *testing.T) {
	// Слабая интенсивность — «почти нормальный» регион (02_worlds §2.6.1).
	g := NewGenerator(&Config{Seed: 2})
	g.profile = &regionprofile.Profile{Star: regionprofile.StarMods{
		SpectralMult: map[string]float64{"O": 2.5, "M": 0.6},
	}}
	g.profileIntensity = regionprofile.Weak

	counts := map[string]int{}
	const n = 100_000
	for i := 0; i < n; i++ {
		counts[g.spectralClass()]++
	}
	def := DefaultWeights().Spectral
	// Сдвиг слабее, чем при сильной: O-доля ближе к дефолту.
	strong := NewGenerator(&Config{Seed: 2})
	strong.profile = &regionprofile.Profile{Star: regionprofile.StarMods{
		SpectralMult: map[string]float64{"O": 2.5, "M": 0.6},
	}}
	strong.profileIntensity = regionprofile.Strong
	strongCounts := map[string]int{}
	for i := 0; i < n; i++ {
		strongCounts[strong.spectralClass()]++
	}
	weakO := float64(counts["O"]) / n
	strongO := float64(strongCounts["O"]) / n
	assert.Less(t, weakO, strongO, "слабая интенсивность сдвигает меньше сильной")
	assert.Greater(t, weakO, def["O"]/100.0, "слабый сдвиг всё же есть")
}

// ==================== ТИПЫ СИСТЕМ (59a §7) ====================

func TestProfileAllSystemTypesPossible(t *testing.T) {
	g := NewGenerator(&Config{Seed: 3})
	g.profile = &regionprofile.Profile{Star: regionprofile.StarMods{
		SystemTypeMult: map[string]float64{"single": 0.3, "black_hole": 3.0, "neutron": 2.0},
	}}
	g.profileIntensity = regionprofile.Strong

	counts := map[string]int{}
	const n = 100_000
	for i := 0; i < n; i++ {
		counts[g.randomSystemType()]++
	}
	// Инвариант §11.1: все типы возможны.
	for _, st := range systemTypeOrder {
		assert.Greater(t, counts[st], 0, "тип %s должен оставаться возможным", st)
	}
	// ЧД чаще дефолта.
	def := DefaultWeights().SystemTypes
	assert.Greater(t, float64(counts["black_hole"])/n, def["black_hole"]/100.0)
}

// ==================== МАССА (59a §7, инвариант §11.2) ====================

func TestProfileMassBiasStaysInRange(t *testing.T) {
	g := NewGenerator(&Config{Seed: 4})
	g.profile = &regionprofile.Profile{Star: regionprofile.StarMods{MassBias: 0.3}}
	g.profileIntensity = regionprofile.Strong

	r := DefaultStellarMassRanges()["G"]
	for i := 0; i < 10_000; i++ {
		m, ok := g.randomMass("G")
		require.True(t, ok)
		assert.GreaterOrEqual(t, m, r.Min, "масса не ниже минимума класса")
		assert.LessOrEqual(t, m, r.Max, "масса не выше максимума класса")
	}
	// Сдвиг вверх: средняя масса выше середины диапазона.
	g2 := NewGenerator(&Config{Seed: 4})
	g2.profile = &regionprofile.Profile{Star: regionprofile.StarMods{MassBias: 0.3}}
	g2.profileIntensity = regionprofile.Strong
	sum := 0.0
	for i := 0; i < 10_000; i++ {
		m, _ := g2.randomMass("G")
		sum += m
	}
	mid := (r.Min + r.Max) / 2
	assert.Greater(t, sum/10_000, mid, "mass_bias +0.3 должен сдвигать массу вверх")
}

// ==================== МЕТАЛЛИЧНОСТЬ (59a §7, инвариант §11.3) ====================

func TestProfileMetallicityShiftClamped(t *testing.T) {
	g := NewGenerator(&Config{Seed: 5})
	g.profile = &regionprofile.Profile{Star: regionprofile.StarMods{MetallicityShift: 0.3}}
	g.profileIntensity = regionprofile.Strong

	for i := 0; i < 10_000; i++ {
		met := g.rollMetallicity()
		assert.GreaterOrEqual(t, met, -0.8, "металличность не ниже −0.8")
		assert.LessOrEqual(t, met, 0.5, "металличность не выше +0.5")
	}
}

// ==================== ПЕРЕМЕННОСТЬ (59a §7) ====================

func TestProfileVariableMultIncreasesVariables(t *testing.T) {
	uvCetiShare := func(profile *regionprofile.Profile, intensity regionprofile.Intensity) float64 {
		g := NewGenerator(&Config{Seed: 6})
		g.profile = profile
		g.profileIntensity = intensity
		vars := 0
		const n = 50_000
		for i := 0; i < n; i++ {
			w := &models.World{}
			g.rollStarMods(w, "single", "M")
			if w.StellarMods != nil && w.StellarMods.VariableType == "uv_ceti" {
				vars++
			}
		}
		return float64(vars) / n
	}
	base := uvCetiShare(nil, 0)
	boosted := uvCetiShare(&regionprofile.Profile{Star: regionprofile.StarMods{VariableMult: fp(2.0)}}, regionprofile.Strong)
	assert.Greater(t, boosted, base*1.5, "variable_mult 2.0 должен заметно поднять долю переменных")
}

func TestClampProb(t *testing.T) {
	assert.InDelta(t, 1.0, clampProb(1.5), 0.001)
	assert.InDelta(t, 0.5, clampProb(0.5), 0.001)
}

// ==================== РОЛЛ ПРОФИЛЯ В buildRegions (59a §10) ====================

func TestBuildRegionsRollsProfiles(t *testing.T) {
	require.NoError(t, regionprofile.LoadProfiles("../../../config/region_profiles", nil, nil))
	g := NewGenerator(&Config{Seed: 42, ClusterRadius: 100})
	centers := make([]struct{ X, Y float64 }, 200)
	for i := range centers {
		centers[i] = struct{ X, Y float64 }{float64(i), float64(i)}
	}
	regions := g.buildRegions(centers)
	require.Len(t, regions, 200)

	withProfile := 0
	for _, r := range regions {
		if r.Profile != "" {
			withProfile++
			assert.Contains(t, []int{0, 1, 2}, r.ProfileIntensity, "интенсивность 0/1/2")
			assert.NotNil(t, regionprofile.ByID(r.Profile), "класс %s есть в каталоге", r.Profile)
		}
	}
	// ~75% регионов с профилем (допуск на случайность).
	assert.InDelta(t, 0.75, float64(withProfile)/float64(len(regions)), 0.15)
}

// ==================== ИНТЕГРАЦИЯ: ГАЛАКТИКА С ПРОФИЛЯМИ ====================

func TestGenerateGalaxyWithProfilesAllTypesPossible(t *testing.T) {
	require.NoError(t, regionprofile.LoadProfiles("../../../config/region_profiles", nil, nil))
	g := NewGenerator(&Config{
		Seed: 42, WorldCount: 4000, MapSize: 40000, MinDist: 40, WorldSpread: 0,
		ClusterCount: 10, ClusterRadius: 300, ClusterSpacing: 800,
	})
	result := g.GenerateGalaxyWithRegions()
	require.NotEmpty(t, result.Regions)

	withProfile := 0
	for _, r := range result.Regions {
		if r.Profile != "" {
			withProfile++
		}
	}
	assert.Greater(t, withProfile, 0, "часть регионов должна получить профиль")

	spec := map[string]int{}
	sys := map[string]int{}
	for _, w := range result.Worlds {
		if w.SpectralClass != "" {
			spec[w.SpectralClass]++
		}
		// Экзотика пишет тип объекта в StarType (SystemType остаётся "single").
		if w.StarType == "star" {
			sys[w.SystemType]++
		} else {
			sys[w.StarType]++
		}
	}
	for _, cls := range spectralOrder {
		assert.Greater(t, spec[cls], 0, "спектральный класс %s должен оставаться возможным", cls)
	}
	// Типы, реально появляющиеся как значения: single/binary/multiple
	// (SystemType) + black_hole/neutron/white_dwarf/protostar (StarType).
	// «exotic»-сверхгиганты пишут star|single (фаза I в модах) — их покрывает
	// проверка спектральных классов O/B/A.
	for _, st := range []string{"single", "binary", "multiple", "black_hole", "neutron", "white_dwarf", "protostar"} {
		assert.Greater(t, sys[st], 0, "тип %s должен оставаться возможным", st)
	}
}