// internal/generator/planet/archetype.go
package planet

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
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

var archetypeCache *ArchetypeConfig

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
	archetypeCache = &cfg
	return nil
}