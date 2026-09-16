// internal/generator/settlement/race_preset.go
//
// Пресет расового генератора поселений (65a): шанс заселения доминанты,
// крутилка подселения соседа на выбросе, стратегия населения. Заменил
// ключи старого пресета поселений (config/settlement_preset.json, скрыт
// 65a): «шанс_заселения_соседней_расы» переехал сюда как neighbor_chance.
package settlement

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RacePreset — параметры расового генератора поселений (config/race_settlement.json).
type RacePreset struct {
	// NeighborChance — шанс заселения соседней расы на выбросе (0–1,
	// спека 99.2.21 §7.3, идея 56a «Крутилка»). Дефолт 0.3.
	NeighborChance float64 `json:"neighbor_chance"`
	// Chance — шанс заселения доминантной расы кластера (0–1, 65a).
	// Дефолт 1.0.
	Chance float64 `json:"chance"`
	// Population — стратегия населения поселений рас (fixed/random, 65a).
	// Дефолт — random 100k–1B (как DefaultModel, скрыт 65a).
	Population Population `json:"population"`
}

var (
	racePresetMu sync.RWMutex
	raceCurrent  = DefaultRacePreset()
)

// LoadRacePreset — загружает пресет расового генератора из файла и делает
// его текущим. При ошибке текущий пресет не трогается.
func LoadRacePreset(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("race settlement preset: %w", err)
	}
	var p RacePreset
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("race settlement preset %s: %w", absPath, err)
	}
	racePresetMu.Lock()
	raceCurrent = &p
	racePresetMu.Unlock()
	return nil
}

// CurrentRacePreset — текущий пресет расового генератора (дефолтный,
// если не загружен).
func CurrentRacePreset() *RacePreset {
	racePresetMu.RLock()
	defer racePresetMu.RUnlock()
	return raceCurrent
}

// DefaultRacePreset — значения по умолчанию: крутилка 0.3, шанс доминанты
// 1.0, население random 100k–1B (как старый пресет поселений, скрыт 65a).
func DefaultRacePreset() *RacePreset {
	return &RacePreset{
		NeighborChance: 0.3,
		Chance:         1.0,
		Population: Population{
			Kind: "random",
			Min:  100_000,
			Max:  1_000_000_000,
		},
	}
}