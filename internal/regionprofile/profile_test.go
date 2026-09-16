// internal/regionprofile/profile_test.go
package regionprofile

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// fp — указатель на float64 (для полей-указателей VariableMult/PlanetCountMult).
func fp(v float64) *float64 { return &v }

// ==================== ИНТЕНСИВНОСТЬ (спека §7) ====================

func TestIntensityScale(t *testing.T) {
	assert.InDelta(t, 0.3, intensityScale(Weak), 0.001)
	assert.InDelta(t, 0.6, intensityScale(Medium), 0.001)
	assert.InDelta(t, 1.0, intensityScale(Strong), 0.001)
}

func TestMultEffInterpolation(t *testing.T) {
	// eff = 1 + (config − 1) × scale: слабая ×0.3, средняя ×0.6, сильная ×1.0.
	assert.InDelta(t, 1.0, multEff(1.0, Strong), 0.001)
	assert.InDelta(t, 2.0, multEff(2.0, Strong), 0.001)
	assert.InDelta(t, 1.3, multEff(2.0, Weak), 0.001)
	assert.InDelta(t, 1.6, multEff(2.0, Medium), 0.001)
	assert.InDelta(t, 0.3, multEff(0.3, Strong), 0.001) // 1 + (0.3−1)×1
	assert.InDelta(t, 0.91, multEff(0.7, Weak), 0.001)  // 1 + (−0.3)×0.3
}

func TestAddEff(t *testing.T) {
	assert.InDelta(t, 0.3, addEff(0.3, Strong), 0.001)
	assert.InDelta(t, 0.09, addEff(0.3, Weak), 0.001)
	assert.InDelta(t, 0.18, addEff(0.3, Medium), 0.001)
}

// ==================== ЭФФЕКТИВНЫЕ СДВИГИ ====================

func TestProfileEffectiveMods(t *testing.T) {
	p := &Profile{
		Star: StarMods{
			SpectralMult:     map[string]float64{"O": 2.5, "M": 0.6},
			SystemTypeMult:   map[string]float64{"black_hole": 3.0},
			MassBias:         0.2,
			MetallicityShift: 0.1,
			VariableMult:     fp(1.5),
		},
		Planet: PlanetMods{
			PlanetCountMult: fp(0.8),
			GasGiantShift:   -0.1,
			SurfaceBias:     map[string]float64{"кратеры": 1.3},
			ResourceBias:    map[string]float64{"rare": 2.0},
		},
	}
	// Сильная — конфиг как есть.
	assert.InDelta(t, 2.5, p.SpectralMult(Strong)["O"], 0.001)
	assert.InDelta(t, 0.6, p.SpectralMult(Strong)["M"], 0.001)
	assert.InDelta(t, 0.2, p.MassBias(Strong), 0.001)
	assert.InDelta(t, 0.1, p.MetallicityShift(Strong), 0.001)
	assert.InDelta(t, 1.5, p.VariableMult(Strong), 0.001)
	assert.InDelta(t, 0.8, p.PlanetCountMult(Strong), 0.001)
	assert.InDelta(t, -0.1, p.GasGiantShift(Strong), 0.001)
	// Слабая — масштаб 0.3.
	assert.InDelta(t, 1.45, p.SpectralMult(Weak)["O"], 0.001) // 1 + (2.5−1)×0.3
	assert.InDelta(t, 0.88, p.SpectralMult(Weak)["M"], 0.001) // 1 + (0.6−1)×0.3
	assert.InDelta(t, 0.06, p.MassBias(Weak), 0.001)
	assert.InDelta(t, 0.03, p.MetallicityShift(Weak), 0.001)
	assert.InDelta(t, 1.15, p.VariableMult(Weak), 0.001)
	assert.InDelta(t, 0.94, p.PlanetCountMult(Weak), 0.001) // 1 + (0.8−1)×0.3
	assert.InDelta(t, -0.03, p.GasGiantShift(Weak), 0.001)
}

func TestApplyMultMap(t *testing.T) {
	base := map[string]float64{"O": 0.5, "B": 2.0, "M": 32.0}
	mult := map[string]float64{"O": 2.5, "M": 0.6}
	out := ApplyMultMap(base, mult, Strong)
	assert.InDelta(t, 1.25, out["O"], 0.001)
	assert.InDelta(t, 2.0, out["B"], 0.001) // неупомянутый — 1.0
	assert.InDelta(t, 19.2, out["M"], 0.001)

	// nil mult — base как есть (тот же map, без копии).
	assert.Equal(t, base, ApplyMultMap(base, nil, Strong))

	// Ключ mult вне base — no-op (форма вне шаблона полосы, O4).
	out2 := ApplyMultMap(base, map[string]float64{"несуществующая_форма": 3.0}, Strong)
	assert.Equal(t, base, out2)
}

// ==================== ВАЛИДАЦИЯ (спека §11.1, §9) ====================

func TestValidateRejectsNonPositiveMultipliers(t *testing.T) {
	// Множитель 0 — тип может исчезнуть (нарушение мягкости).
	p := &Profile{ID: "x", Star: StarMods{SpectralMult: map[string]float64{"O": 0}}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Star: StarMods{SystemTypeMult: map[string]float64{"single": -1}}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Planet: PlanetMods{SurfaceBias: map[string]float64{"кратеры": 0}}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Planet: PlanetMods{ResourceBias: map[string]float64{"rare": -0.5}}}
	require.Error(t, p.Validate())
	// Мультипликативные ручки строго > 0 (спека §7): 0 тоже запрещён.
	p = &Profile{ID: "x", Star: StarMods{VariableMult: fp(0)}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Planet: PlanetMods{PlanetCountMult: fp(0)}}
	require.Error(t, p.Validate())
	// Пустой id — ошибка.
	require.Error(t, (&Profile{}).Validate())
}

func TestValidateRejectsAdditiveOutOfRange(t *testing.T) {
	// Аддитивные ручки: mass_bias/metallicity_shift ±0.3 (спека §7),
	// gas_giant_shift ±0.2 (спека §8).
	p := &Profile{ID: "x", Star: StarMods{MassBias: 0.4}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Star: StarMods{MassBias: -0.31}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Star: StarMods{MetallicityShift: 0.31}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Star: StarMods{MetallicityShift: -0.4}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Planet: PlanetMods{GasGiantShift: 0.21}}
	require.Error(t, p.Validate())
	p = &Profile{ID: "x", Planet: PlanetMods{GasGiantShift: -0.3}}
	require.Error(t, p.Validate())
	// Границы диапазонов допустимы.
	p = &Profile{ID: "x", Star: StarMods{MassBias: 0.3, MetallicityShift: -0.3}, Planet: PlanetMods{GasGiantShift: 0.2}}
	require.NoError(t, p.Validate())
}

func TestValidateFormsRejectsUnknownNames(t *testing.T) {
	valid := []string{"кратеры", "леса"}
	p := &Profile{ID: "x", Planet: PlanetMods{SurfaceBias: map[string]float64{"кратеры": 1.3}}}
	require.NoError(t, p.ValidateForms(valid, nil))
	p = &Profile{ID: "x", Planet: PlanetMods{SurfaceBias: map[string]float64{"неизвестная_форма": 1.3}}}
	require.Error(t, p.ValidateForms(valid, nil))
}

// ==================== КАТАЛОГ ====================

func TestLoadProfilesFromConfigDir(t *testing.T) {
	require.NoError(t, LoadProfiles("../../config/region_profiles", nil, nil))
	require.NotEmpty(t, Profiles())
	// 12 стартовых классов (спека §9).
	require.Len(t, Profiles(), 12)
	ids := map[string]bool{}
	for _, p := range Profiles() {
		require.False(t, ids[p.ID], "дубликат id %s", p.ID)
		ids[p.ID] = true
	}
	for _, id := range []string{"quasar", "toxic", "young", "metal_rich", "dead", "pirate",
		"predator", "fertile", "abandoned", "old", "dense", "unstable"} {
		assert.True(t, ids[id], "класс %s отсутствует", id)
	}
}

func TestPickClassWeighted(t *testing.T) {
	require.NoError(t, LoadProfiles("../../config/region_profiles", nil, nil))
	rng := rand.New(rand.NewSource(1))
	counts := map[string]int{}
	const n = 100000
	for i := 0; i < n; i++ {
		p := PickClass(rng)
		require.NotNil(t, p)
		counts[p.ID]++
	}
	// quasar (вес 0.5) реже, чем toxic (вес 1.0).
	assert.Less(t, counts["quasar"], counts["toxic"])
	// Все классы возможны.
	for _, p := range Profiles() {
		assert.Greater(t, counts[p.ID], 0, "класс %s не выбран ни разу", p.ID)
	}
}

func TestByID(t *testing.T) {
	require.NoError(t, LoadProfiles("../../config/region_profiles", nil, nil))
	assert.NotNil(t, ByID("quasar"))
	assert.Nil(t, ByID("нет_такого"))
}

// ==================== БЛИЖАЙШИЙ РЕГИОН (спека §10) ====================

func TestNearestRegionIndex(t *testing.T) {
	regions := []*models.Region{
		{CenterX: 0, CenterY: 0},
		{CenterX: 100, CenterY: 0},
		{CenterX: 0, CenterY: 100},
	}
	assert.Equal(t, 0, NearestRegionIndex(1, 1, regions))
	assert.Equal(t, 1, NearestRegionIndex(90, 1, regions))
	assert.Equal(t, 2, NearestRegionIndex(1, 90, regions))
	assert.Equal(t, -1, NearestRegionIndex(0, 0, nil))
}