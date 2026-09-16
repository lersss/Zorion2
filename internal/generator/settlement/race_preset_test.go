package settlement

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Дефолтный пресет расового генератора (65a): крутилка 0.3, шанс доминанты
// 1.0, население random 100k–1B (как старый пресет поселений).
func TestDefaultRacePreset(t *testing.T) {
	p := DefaultRacePreset()
	assert.Equal(t, 0.3, p.NeighborChance)
	assert.Equal(t, 1.0, p.Chance)
	assert.Equal(t, "random", p.Population.Kind)
	assert.Equal(t, 100_000, p.Population.Min)
	assert.Equal(t, 1_000_000_000, p.Population.Max)
}

// LoadRacePreset — читает конфиг расового генератора (65a): ключи
// neighbor_chance/chance/population из config/race_settlement.json.
func TestLoadRacePreset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "race_settlement.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"neighbor_chance": 0.5,
		"chance": 0.7,
		"population": {"kind": "fixed", "fixed": 50000}
	}`), 0o644))

	require.NoError(t, LoadRacePreset(path))
	p := CurrentRacePreset()
	assert.Equal(t, 0.5, p.NeighborChance)
	assert.Equal(t, 0.7, p.Chance)
	assert.Equal(t, "fixed", p.Population.Kind)
	assert.Equal(t, 50000, p.Population.Fixed)
}

// RaceGenConfig.Validate — валидация как в Model.Validate (65a): шансы 0–1,
// население по стратегии.
func TestRaceGenConfigValidate(t *testing.T) {
	require.NoError(t, RaceGenConfig{Chance: 1.0, NeighborChance: 0.3, Population: Population{Kind: "random", Min: 100_000, Max: 1_000_000_000}}.Validate())

	require.Error(t, RaceGenConfig{Chance: 1.5}.Validate(), "chance > 1 — ошибка")
	require.Error(t, RaceGenConfig{Chance: 1.0, NeighborChance: -0.1}.Validate(), "neighbor_chance < 0 — ошибка")
	require.Error(t, RaceGenConfig{Chance: 1.0, Population: Population{Kind: "fixed", Fixed: 0}}.Validate(), "fixed ≤ 0 — ошибка")
	require.Error(t, RaceGenConfig{Chance: 1.0, Population: Population{Kind: "random", Min: 200, Max: 100}}.Validate(), "min > max — ошибка")
}