// internal/handlers/admin_stats_aggregator_test.go
// Тесты группировки статистики (99.2.4 §7): планеты обычных звёзд — в
// PlanetsBySpectral, экзотики — в PlanetsByStarType (без «вранья»).
package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatsAggregatorExoticGrouping(t *testing.T) {
	worlds := []worldInfo{
		{ID: "w1", SpectralClass: "G", StarType: "star", SystemType: "single", Temperature: 5600},
		{ID: "w2", SpectralClass: "", StarType: "black_hole", SystemType: "single", Temperature: 0},
	}
	a := newStatsAggregator(worlds)
	stats := newPlanetStats()

	planets := []planetRecord{
		{ID: "p1", WorldID: "w1", Data: map[string]interface{}{"surface_dominant": "горы", "temperature": 300.0, "water_percent": 40.0, "life": true}},
		{ID: "p2", WorldID: "w2", Data: map[string]interface{}{"surface_dominant": "кратеры", "temperature": 40.0, "water_percent": 0.0, "life": false}},
	}
	a.process(planets, stats)

	// Обычная звезда G → PlanetsBySpectral.
	require.Equal(t, 1, stats.PlanetsBySpectral["G"]["горы"])
	// Экзотика (ЧД) → PlanetsByStarType, в PlanetsBySpectral не сливается.
	require.Equal(t, 1, stats.PlanetsByStarType["black_hole"]["кратеры"])
	require.NotContains(t, stats.PlanetsBySpectral, "black_hole")
	require.NotContains(t, stats.PlanetsBySpectral, "")
}

func TestSystemTypeDistribution(t *testing.T) {
	worlds := []worldInfo{
		{ID: "w1", SystemType: "single"},
		{ID: "w2", SystemType: "binary"},
		{ID: "w3", SystemType: ""}, // старые строки без типа → single
	}
	stats := newPlanetStats()
	for _, w := range worlds {
		st := w.SystemType
		if st == "" {
			st = "single"
		}
		stats.SystemTypeDistribution[st]++
	}
	require.Equal(t, 2, stats.SystemTypeDistribution["single"])
	require.Equal(t, 1, stats.SystemTypeDistribution["binary"])
}