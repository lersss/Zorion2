package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Каталог универсального слоя: ровно 20 предустановленных ресурсов
// (спека 94a §5: 13 ядерных + 7 мостовых, инвариант 2).
func TestLayerCatalogSize(t *testing.T) {
	require.Len(t, LayerCatalog(), 20, "каталог: 13 ядерных + 7 мостовых")
}

// Каждый ресурс: id, имя-заглушка, категория из 6, непустой Closes.
func TestLayerCatalogShape(t *testing.T) {
	for _, res := range LayerCatalog() {
		require.NotEmpty(t, res.ID, "id ресурса")
		require.NotEmpty(t, res.Name, "имя-заглушка ресурса %s", res.ID)
		require.NotEmpty(t, res.Category, "категория ресурса %s", res.ID)
		require.Contains(t, AllCategories, res.Category, "категория ресурса %s", res.ID)
		require.NotEmpty(t, res.Closes, "ресурс %s закрывает ≥ 1 ось", res.ID)
	}
}

// Ядерные (13) — по одному на хемотип, мостовые (7) — мульти-потребностные
// (§5.1/§5.2). №16/№17 — мостовые с одной осью (ГРД/ОРГ), поэтому признак
// Bridge явный, не выводится из Closes.
func TestLayerCatalogNuclearVsBridge(t *testing.T) {
	nuclear, bridge := 0, 0
	for _, res := range LayerCatalog() {
		if res.Bridge {
			bridge++
		} else {
			nuclear++
		}
	}
	assert.Equal(t, 13, nuclear, "ядерных ресурсов")
	assert.Equal(t, 7, bridge, "мостовых ресурсов")
}

// №4 CO₂-лёд — сублимирующий (T_boil ≤ T_melt, §4 уточнение 1); остальные — нет.
func TestLayerCatalogSublimating(t *testing.T) {
	co2 := LayerByID("co2_ice")
	require.NotNil(t, co2, "CO₂-лёд есть в каталоге")
	assert.True(t, co2.Sublimating(), "CO₂-лёд сублимирует (T_boil ≤ T_melt)")
	assert.LessOrEqual(t, co2.TBoil, co2.TMelt)
	for _, res := range LayerCatalog() {
		if res.ID == "co2_ice" {
			continue
		}
		assert.False(t, res.Sublimating(), "ресурс %s не сублимирующий", res.ID)
	}
}

// №7 сверхкритический флюид — признак сверхкритического (крит. точка ~304 K).
func TestLayerCatalogSupercritical(t *testing.T) {
	sc := LayerByID("supercritical_fluid")
	require.NotNil(t, sc, "сверхкритический флюид есть в каталоге")
	assert.True(t, sc.Supercritical, "сверхкритический флюид помечен")
	assert.InDelta(t, 304, (sc.TMelt+sc.TBoil)/2, 20, "T вокруг критической точки CO₂ ~304 K")
}

// Точечные профили §5.3: спот-проверка ключевых ресурсов.
func TestLayerCatalogSpotProfiles(t *testing.T) {
	water := LayerByID("water")
	require.NotNil(t, water)
	assert.Equal(t, 268.0, water.TMelt)
	assert.Equal(t, 372.0, water.TBoil)
	assert.Equal(t, 63.0, water.Biocompatibility)
	assert.Equal(t, 12.0, water.Toxicity)

	co2 := LayerByID("co2_ice")
	require.NotNil(t, co2)
	assert.Equal(t, 196.0, co2.TMelt)
	assert.Equal(t, 189.0, co2.TBoil)
	assert.Equal(t, 40.0, co2.ChemicalActivity)
}