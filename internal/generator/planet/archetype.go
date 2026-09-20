// internal/generator/planet/archetype.go
package planet

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Archetype — полоса композиции планеты (99.2.20 §5).
//
// Роли полей переопределены каскадом: base_surface/base_subterrain — базовые
// веса композиции полосы; allowed_hydrospheres/atmospheres/biospheres —
// списки выбора (гейтятся флагом); water_chance/life_chance — косметические
// вероятности (гейтятся физикой); temperature_min/max, mass_min/max,
// weight — НЕ используются (физика решает; рулетка отменена).
type Archetype struct {
	ID             string
	Name           string
	ArchetypeID    string
	BaseSurface    map[string]float64
	BaseSubterrain map[string]float64
	Hydrosphere    string
	Atmosphere     string
	Biosphere      string
	TemperatureMin float64
	TemperatureMax float64
	WaterChance    float64
	LifeChance     float64
	MassMin        float64
	MassMax        float64
}

// ClimateConfig — конфиг одного архетипа из JSON.
type ClimateConfig struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Weight              map[string]float64 `json:"weight"`
	BaseSurface         map[string]float64 `json:"base_surface"`
	BaseSubterrain      map[string]float64 `json:"base_subterrain"`
	AllowedHydrospheres []string           `json:"allowed_hydrospheres"`
	AllowedAtmospheres  []string           `json:"allowed_atmospheres"`
	AllowedBiospheres   []string           `json:"allowed_biospheres"`
	TemperatureMin      float64            `json:"temperature_min"`
	TemperatureMax      float64            `json:"temperature_max"`
	WaterChance         float64            `json:"water_chance"`
	LifeChance          float64            `json:"life_chance"`
	MassMin             float64            `json:"mass_min"`
	MassMax             float64            `json:"mass_max"`
}

type ArchetypeConfig struct {
	Climates []ClimateConfig `json:"climates"`
}

var (
	archetypeMu   sync.RWMutex
	archetypeCache *ArchetypeConfig
	// archetypePath — путь рабочего файла (config/planet_archetypes.json),
	// задаётся LoadArchetypes; SaveArchetypes пишет сюда атомарно.
	archetypePath string
)

// LoadArchetypes — загружает архетипы из JSON.
func LoadArchetypes(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		log.Printf("⚠️ Ошибка получения абсолютного пути: %v", err)
		absPath = path
	}
	log.Printf("🔍 Загрузка архетипов из: %s", absPath)

	data, err := os.ReadFile(absPath)
	if err != nil {
		cwd, _ := os.Getwd()
		log.Printf("⚠️ Текущая директория: %s", cwd)
		return err
	}
	var cfg ArchetypeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("⚠️ Ошибка парсинга JSON: %v", err)
		return err
	}
	archetypeMu.Lock()
	archetypeCache = &cfg
	archetypePath = absPath
	archetypeMu.Unlock()
	return nil
}

// GetArchetypes — текущий конфиг полос климатов (копия: читатели не мутируют
// store — паттерн GetCurve 99.2.17). nil, если не загружен.
func GetArchetypes() *ArchetypeConfig {
	archetypeMu.RLock()
	defer archetypeMu.RUnlock()
	if archetypeCache == nil {
		return nil
	}
	cfg := *archetypeCache
	cfg.Climates = append([]ClimateConfig(nil), archetypeCache.Climates...)
	for i := range cfg.Climates {
		c := &cfg.Climates[i]
		c.Weight = copyFloatMap(c.Weight)
		c.BaseSurface = copyFloatMap(c.BaseSurface)
		c.BaseSubterrain = copyFloatMap(c.BaseSubterrain)
		c.AllowedHydrospheres = append([]string(nil), c.AllowedHydrospheres...)
		c.AllowedAtmospheres = append([]string(nil), c.AllowedAtmospheres...)
		c.AllowedBiospheres = append([]string(nil), c.AllowedBiospheres...)
	}
	return &cfg
}

// RebuildArchetypes — атомарная замена store (hot-reload для админки).
func RebuildArchetypes(cfg *ArchetypeConfig) error {
	if cfg == nil || len(cfg.Climates) == 0 {
		return fmt.Errorf("полосы климатов: пустой конфиг")
	}
	archetypeMu.Lock()
	archetypeCache = cfg
	archetypeMu.Unlock()
	return nil
}

// SaveArchetypes — атомарная запись полос климатов в файл (tmp + rename,
// паттерн race_balancer). Путь — из LoadArchetypes; не загружен → ошибка.
func SaveArchetypes(cfg *ArchetypeConfig) error {
	archetypeMu.RLock()
	path := archetypePath
	archetypeMu.RUnlock()
	if path == "" {
		return fmt.Errorf("полосы климатов: путь файла не задан (LoadArchetypes не вызывался)")
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("полосы климатов: marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("полосы климатов: каталог %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("полосы климатов: запись tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("полосы климатов: rename: %w", err)
	}
	return nil
}

// copyFloatMap — копия map[string]float64 (для GetArchetypes).
func copyFloatMap(src map[string]float64) map[string]float64 {
	if src == nil {
		return nil
	}
	dst := make(map[string]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}