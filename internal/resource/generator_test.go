// internal/resource/generator_test.go
package resource

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== SUMMARY ====================

func TestSummary_Empty(t *testing.T) {
	s := Summary(nil)
	assert.Empty(t, s, "без ресурсов summary должен быть пустым")
}

func TestSummary_OneCategory(t *testing.T) {
	res := []*models.PlanetResource{
		{Category: CategoryMineral},
	}
	s := Summary(res)
	require.Contains(t, s, CategoryMineral)
	assert.Greater(t, s[CategoryMineral], 0.3, "значение должно превышать порог фильтра 0.3")
	assert.LessOrEqual(t, s[CategoryMineral], 1.0)
}

func TestSummary_GrowsWithCount(t *testing.T) {
	one := Summary([]*models.PlanetResource{{Category: CategoryGas}})
	many := Summary([]*models.PlanetResource{
		{Category: CategoryGas},
		{Category: CategoryGas},
		{Category: CategoryGas},
	})
	assert.Greater(t, many[CategoryGas], one[CategoryGas],
		"больше ресурсов категории → выше богатство")
}

func TestSummary_CappedAtOne(t *testing.T) {
	many := make([]*models.PlanetResource, 50)
	for i := range many {
		many[i] = &models.PlanetResource{Category: CategoryRare}
	}
	s := Summary(many)
	assert.Equal(t, 1.0, s[CategoryRare], "богатство не превышает 1.0")
}

func TestSummary_SeparatesCategories(t *testing.T) {
	res := []*models.PlanetResource{
		{Category: CategoryMineral},
		{Category: CategoryOrganic},
		{Category: CategoryWater},
	}
	s := Summary(res)
	assert.Len(t, s, 3)
	assert.Contains(t, s, CategoryMineral)
	assert.Contains(t, s, CategoryOrganic)
	assert.Contains(t, s, CategoryWater)
}

// ==================== ГАЗОВЫЙ ГИГАНТ ====================

func TestGenerateGasGiantResource_ExactlyOne(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 50; i++ {
		res := GenerateGasGiantResource("planet-1", rng)
		require.Len(t, res, 1, "гигант получает ровно один ресурс")
		assert.Equal(t, "planet-1", res[0].PlanetID)
		assert.Contains(t, []string{CategoryGas, CategoryFuel}, res[0].Category,
			"ресурс гиганта — газ или топливо")
		assert.True(t, res[0].IsKnown, "ресурс атмосферы известен")
	}
}

func TestGenerateGasGiantResource_ValidName(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 100; i++ {
		res := GenerateGasGiantResource("planet-1", rng)
		name := res[0].Name
		assert.NotEmpty(t, name, "имя ресурса не должно быть пустым")
		for _, ch := range name {
			assert.True(t, ch >= 'А' && ch <= 'я' || ch == '-',
				"имя %q содержит недопустимый символ %q", name, ch)
		}
	}
}

// ==================== GENERATE RESOURCES ====================

func TestGenerateResources_AllCategoriesValid(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		res := GenerateResources(
			"planet-1",
			"скалы",
			map[string]float64{"рудные_жилы": 60, "пустая_порода": 40},
			"G",
			rng,
		)
		require.NotEmpty(t, res, "планета со скалами и рудами не должна быть пустой")
		for _, r := range res {
			assert.Contains(t, AllCategories, r.Category, "категория %q не из списка", r.Category)
			assert.True(t, r.PlanetID == "planet-1")
			assert.GreaterOrEqual(t, r.Quantity, 100)
			assert.LessOrEqual(t, r.Quantity, 500)
		}
	}
}

func TestGenerateResources_UniqueNames(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	res := GenerateResources(
		"planet-1",
		"океаны",
		map[string]float64{"нефтяные_карманы": 50, "подземные_воды": 50},
		"K",
		rng,
	)
	seen := map[string]bool{}
	for _, r := range res {
		assert.False(t, seen[r.Name], "дубликат имени %s", r.Name)
		seen[r.Name] = true
	}
}

func TestGenerateResources_PropertiesInRange(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	res := GenerateResources(
		"planet-1",
		"леса",
		map[string]float64{"угольные_пласты": 100},
		"M",
		rng,
	)
	require.NotEmpty(t, res)
	for _, r := range res {
		for name, v := range map[string]float64{
			"hardness":          r.Hardness,
			"elasticity":        r.Elasticity,
			"conductivity":      r.Conductivity,
			"heat_resistance":   r.HeatResistance,
			"chemical_activity": r.ChemicalActivity,
			"density":           r.Density,
			"biocompatibility":  r.Biocompatibility,
			"energy_density":    r.EnergyDensity,
			"volatility":        r.Volatility,
		} {
			assert.InDelta(t, v, clamp(v, 0, 100), 20, "%s вне диапазона", name)
		}
	}
}

func TestGenerateResources_KnownSplit(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	res := GenerateResources(
		"planet-1",
		"скалы",
		map[string]float64{"рудные_жилы": 100},
		"G",
		rng,
	)
	require.GreaterOrEqual(t, len(res), 2)
	known := 0
	for _, r := range res {
		if r.IsKnown {
			known++
		}
	}
	assert.True(t, known >= 1, "хотя бы один ресурс известен")
	assert.True(t, known < len(res), "есть и неизвестные ресурсы")
}

func TestDetermineTotal_Clamps(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	total := determineTotal("океаны", map[string]float64{}, "O", rng)
	assert.GreaterOrEqual(t, total, 1)
	assert.LessOrEqual(t, total, 20)

	total = determineTotal("океаны", map[string]float64{
		"рудные_жилы": 10, "газовые_карманы": 10, "угольные_пласты": 10,
		"подземные_воды": 10, "соляные_купола": 10, "пещерные_системы": 10,
		"кристаллические_жилы": 10,
	}, "O", rng)
	assert.LessOrEqual(t, total, 20, "общее количество ограничено 20")
}
