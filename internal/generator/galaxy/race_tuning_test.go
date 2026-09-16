// internal/generator/galaxy/race_tuning_test.go — подкрутка весов
// спектральных классов под расу-дома (99.2.22 §3.3 ручка 5, §3.4):
// home-классы ×1.6, умеренно-горячим O/B ×1.3; слой 1: eff = 1 + (config−1)·s;
// композиция с профилем региона (веса × profile × race, все > 0).
package galaxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/races"
	"zorion/internal/regionprofile"
)

// Аммиачники (home K/M): s = 1 → K/M ×1.6, остальные 1.0.
func TestRaceSpectralMultAmmonia(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	weights := DefaultWeights().Spectral
	boosted := applyRaceSpectralMult(weights, "ammonia", 1)
	assert.InDelta(t, weights["K"]*1.6, boosted["K"], 0.001, "home K ×1.6")
	assert.InDelta(t, weights["M"]*1.6, boosted["M"], 0.001, "home M ×1.6")
	assert.InDelta(t, weights["G"], boosted["G"], 0.001, "не-home класс = 1.0")
	assert.InDelta(t, weights["O"], boosted["O"], 0.001, "не-home класс = 1.0")
}

// Серные гнёзда (умеренно-горячие, opt_hi 480 ≤ 600): home K/M ×1.6 + O/B ×1.3.
func TestRaceSpectralMultSulfurNests(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	weights := DefaultWeights().Spectral
	boosted := applyRaceSpectralMult(weights, "sulfur_nests", 1)
	assert.InDelta(t, weights["K"]*1.6, boosted["K"], 0.001)
	assert.InDelta(t, weights["O"]*1.3, boosted["O"], 0.001, "умеренно-горячие — O ×1.3")
	assert.InDelta(t, weights["B"]*1.3, boosted["B"], 0.001, "умеренно-горячие — B ×1.3")
}

// Сверхкритические (окно через 600): O/B нейтральны (дефолт итерации 3).
func TestRaceSpectralMultSupercritical(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	weights := DefaultWeights().Spectral
	boosted := applyRaceSpectralMult(weights, "supercritical", 1)
	assert.InDelta(t, weights["O"], boosted["O"], 0.001, "окно через 600 — O нейтрален")
	assert.InDelta(t, weights["B"], boosted["B"], 0.001, "окно через 600 — B нейтрален")
	assert.InDelta(t, weights["F"]*1.6, boosted["F"], 0.001, "home F ×1.6")
}

// Слой 1 (непрерывный): eff = 1 + (config−1)·s. s = 0 → веса как есть;
// s = 0.5 → ×1.3; s = 1 → ×1.6.
func TestRaceSpectralMultSoftness(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	weights := DefaultWeights().Spectral

	neutral := applyRaceSpectralMult(weights, "ammonia", 0)
	assert.Equal(t, weights, neutral, "s = 0 — веса как есть (слой 1 нейтрален)")

	half := applyRaceSpectralMult(weights, "ammonia", 0.5)
	assert.InDelta(t, weights["K"]*1.3, half["K"], 0.001, "eff = 1 + (1.6−1)×0.5")

	full := applyRaceSpectralMult(weights, "ammonia", 1)
	assert.InDelta(t, weights["K"]*1.6, full["K"], 0.001, "s = 1 — полный сдвиг")
}

// Без расы / раса не найдена — веса как есть (нейтрально).
func TestRaceSpectralMultNoRace(t *testing.T) {
	weights := DefaultWeights().Spectral
	assert.Equal(t, weights, applyRaceSpectralMult(weights, "", 0.5), "без расы — как есть")
	assert.Equal(t, weights, applyRaceSpectralMult(weights, "nonexistent", 0.5), "раса не найдена — как есть")
}

// Композиция с профилем региона (59a): веса × profile_mult × race_mult,
// все > 0 (инвариант «все типы возможны», спека §5).
func TestRaceSpectralMultWithProfile(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	profile := &regionprofile.Profile{
		Star: regionprofile.StarMods{
			SpectralMult: map[string]float64{"M": 1.5, "K": 1.2},
		},
	}
	weights := DefaultWeights().Spectral
	withProfile := regionprofile.ApplyMultMap(weights, profile.Star.SpectralMult, regionprofile.Strong)
	combined := applyRaceSpectralMult(withProfile, "ammonia", 1)

	// K: профиль ×1.2, раса ×1.6 → ×1.92; M: профиль ×1.5, раса ×1.6 → ×2.4.
	assert.InDelta(t, weights["K"]*1.2*1.6, combined["K"], 0.001, "K: profile × race")
	assert.InDelta(t, weights["M"]*1.5*1.6, combined["M"], 0.001, "M: profile × race")
	assert.InDelta(t, weights["G"], combined["G"], 0.001, "G: без сдвигов")
	for cls, v := range combined {
		assert.Greater(t, v, 0.0, "вес %s > 0 (все типы возможны)", cls)
	}
}

// Интеграция: generateWorld в кластере расы применяет race_mult к весам
// (спектральный класс мира выбирается из сдвинутых весов).
func TestGenerateWorldRaceSpectral(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	g := NewGenerator(&Config{Seed: 5})
	g.raceID = "ammonia"
	g.raceSoftness = 1
	world := g.generateWorld(struct{ X, Y float64 }{X: 0, Y: 0}, nil, 0, "ammonia")
	require.NotNil(t, world)
	assert.NotEmpty(t, world.SpectralClass, "мир получает спектральный класс")
}
