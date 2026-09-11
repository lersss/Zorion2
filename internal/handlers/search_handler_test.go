package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSearchName(t *testing.T) {
	assert.Equal(t, "планета x", normalizeSearchName("  ПланетА X  "))
	assert.Equal(t, "a   b", normalizeSearchName("a   b"))
	assert.Equal(t, "", normalizeSearchName("   "))
}

func TestParseSearchLimit(t *testing.T) {
	v, err := parseSearchLimit("")
	assert.NoError(t, err)
	assert.Equal(t, defaultSearchLimit, v)

	v, err = parseSearchLimit("5")
	assert.NoError(t, err)
	assert.Equal(t, 5, v)

	v, err = parseSearchLimit("100")
	assert.NoError(t, err)
	assert.Equal(t, maxSearchLimit, v)

	for _, bad := range []string{"0", "-3", "abc", "1.5", "  "} {
		_, err = parseSearchLimit(bad)
		assert.Error(t, err, "raw=%q should be rejected", bad)
	}
}

func TestMergeSearchResults(t *testing.T) {
	worlds := []searchResult{
		{Kind: searchKindWorld, ID: "w1"},
		{Kind: searchKindWorld, ID: "w2"},
	}
	planets := []searchResult{
		{Kind: searchKindPlanet, ID: "p1", PlanetID: "p1"},
	}
	sats := []searchResult{
		{Kind: searchKindSatellite, ID: "s1", PlanetID: "p1"},
	}

	// Порядок всегда «звезда → планета → спутник», независимо от порядка групп.
	merged := mergeSearchResults(20, planets, worlds, sats)
	require.Len(t, merged, 4)
	assert.Equal(t, searchKindWorld, merged[0].Kind)
	assert.Equal(t, "w1", merged[0].ID)
	assert.Equal(t, searchKindWorld, merged[1].Kind)
	assert.Equal(t, searchKindPlanet, merged[2].Kind)
	assert.Equal(t, searchKindSatellite, merged[3].Kind)

	// Дубликаты (kind+id) схлопываются.
	merged = mergeSearchResults(20, planets, planets, worlds, sats, sats)
	assert.Len(t, merged, 4)

	// Усечение по лимиту.
	merged = mergeSearchResults(3, planets, worlds, sats)
	require.Len(t, merged, 3)
	assert.Equal(t, searchKindWorld, merged[0].Kind)
	assert.Equal(t, searchKindWorld, merged[1].Kind)
	assert.Equal(t, searchKindPlanet, merged[2].Kind)

	// limit = 0 → пустой результат.
	assert.Empty(t, mergeSearchResults(0, worlds, planets, sats))

	// Ни одной группы → пустой результат.
	assert.Empty(t, mergeSearchResults(5))
}

func TestSearchByNameSkipsEmptyQueries(t *testing.T) {
	// searchWorldsByName/searchPlanetsByName/searchSatellitesByName при limit <= 0
	// возвращают nil без обращения к БД (Node не создаётся).
	worlds, err := searchWorldsByName(nil, nil, "x", 0)
	assert.NoError(t, err)
	assert.Len(t, worlds, 0)

	planets, err := searchPlanetsByName(nil, nil, "x", 0)
	assert.NoError(t, err)
	assert.Len(t, planets, 0)

	sats, err := searchSatellitesByName(nil, nil, "x", 0)
	assert.NoError(t, err)
	assert.Len(t, sats, 0)
}