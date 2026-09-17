package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

// §8.1 «все накормлены»: все 50 био-рас покрыты по всем consumption-осям
// с весом ≥ 10 (инвариант 3).
func TestCheckAllFed(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	gaps := CheckAllFed(LayerCatalog(), LayerTemplates(), races.Catalog())
	assert.Empty(t, gaps, "дыры покрытия: %+v", gaps)
}

// §8.1: дыра обнаруживается, если ресурс не попадает в окно (например, T вне окна).
func TestCheckAllFedDetectsGap(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	broken := []*Resource{
		{
			ID: "broken", Name: "сломанный", Category: CategoryWater, Closes: []string{AxisWater},
			Hardness: 10, Elasticity: 15, Conductivity: 30, Density: 35, EnergyDensity: 8,
			Biocompatibility: 63, Radioactivity: 3, Toxicity: 12, Flammability: 5, ChemicalActivity: 35,
			TMelt: 100, TBoil: 150, // T вне окна ВОД [250,280]/[350,400]
		},
	}
	gaps := CheckAllFed(broken, LayerTemplates(), races.Catalog())
	require.NotEmpty(t, gaps, "сломанный ресурс не закрывает ВОД")
	found := false
	for _, g := range gaps {
		if g.Axis == AxisWater {
			found = true
		}
	}
	assert.True(t, found, "дыра по оси ВОД")
}

// §8.2 посредственность: все «↑»-оси товаров у каталога < 70; максимумы —
// из спеки §5.3 (инвариант 4).
func TestCheckMediocrity(t *testing.T) {
	maxima := CheckMediocrity(LayerCatalog())
	expected := map[string]float64{
		AxisBiocompatibility: 65, // №9 органика-ресурс
		AxisEnergyDensity:    61, // №3 метан-ресурс
		AxisFlammability:     63, // №3 метан-ресурс
		AxisHardness:         63, // №8 кремний-ресурс
		AxisElasticity:       55, // №9 органика-ресурс
		AxisConductivity:     40, // №6 солевой расплав
		AxisRadioactivity:    65, // №13 радиоактивный материал
	}
	for axis, want := range expected {
		assert.InDelta(t, want, maxima[axis], 1e-9, "максимум по оси %s", axis)
		assert.Less(t, maxima[axis], 70.0, "ось %s: посредственность (< 70)", axis)
	}
}

// §8.4 диапазоны: оси [0,100], T в [10,6000], зазор ≥ 5 K (кроме сублимирующих),
// сумма consumption = 100 (инварианты 1, 7).
func TestCheckRanges(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	assert.Empty(t, CheckRanges(LayerCatalog(), races.Catalog()))
}

// §8.4: нарушение зазора T обнаруживается.
func TestCheckRangesDetectsViolation(t *testing.T) {
	bad := []*Resource{
		{
			ID: "bad", Name: "плохой", Category: CategoryWater, Closes: []string{AxisWater},
			TMelt: 300, TBoil: 302, // зазор 2 < 5
		},
	}
	violations := CheckRanges(bad, nil)
	require.NotEmpty(t, violations, "зазор T_boil−T_melt < 5 обнаружен")
}

// §8.5 корреляции температур: пища в пригодной фазе хотя бы в части окна
// обитания (инвариант 5).
func TestCheckTemperatureCorrelation(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	failed := CheckTemperatureCorrelation(LayerTemplates(), races.Catalog())
	assert.Empty(t, failed, "расы без пересечения T-окна пищи с окном обитания: %v", failed)
}

// §8.5 примеры из спеки: аммиачники (жидкая фаза в нижней части окна),
// ледяные пастухи (твёрдая фаза CO₂-льда).
func TestTemperatureCorrelationExamples(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	ammonia := races.ByID("ammonia")
	require.NotNil(t, ammonia)
	assert.True(t, raceFoodIntersectsHabitat(ammonia, LayerTemplates()),
		"аммиачники: пересечение [196,240] ≠ ∅")

	herders := races.ByID("ice_herders")
	require.NotNil(t, herders)
	assert.True(t, raceFoodIntersectsHabitat(herders, LayerTemplates()),
		"ледяные пастухи: твёрдая фаза при T < 189")
}

// Инвариант 9: живость мостовых — точечный профиль (включая T) попадает
// в окна адресатов; мёртвых записей нет.
func TestCheckBridgeLiveness(t *testing.T) {
	violations := CheckBridgeLiveness(LayerCatalog(), LayerTemplates())
	assert.Empty(t, violations, "мёртвые записи: %v", violations)
}

// Инвариант 9: мёртвый мостовой обнаруживается (T вне окна адресата).
func TestCheckBridgeLivenessDetectsDead(t *testing.T) {
	dead := &Resource{
		ID: "dead_bridge", Name: "мёртвый мост", Category: CategoryWater,
		Bridge: true,
		Closes: []string{AxisWater, AxisOrganic},
		Biocompatibility: 63, Toxicity: 12,
		TMelt: 100, TBoil: 150, // вне ВОД [250,280]/[350,400] и ОРГ [250,350]/[400,600]
	}
	violations := CheckBridgeLiveness([]*Resource{dead}, LayerTemplates())
	require.NotEmpty(t, violations, "мёртвый мостовой обнаружен")
}