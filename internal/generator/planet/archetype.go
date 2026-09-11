// internal/generator/planet/archetype.go
package planet

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"path/filepath"
)

// Archetype — результат выбора архетипа для конкретной планеты.
//
// Масса первична. Размер вычисляется из массы и плотности
// (см. Properties, densityForPlanet).
type Archetype struct {
	ID             string
	Name           string
	Climate        string
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

// ClimateConfig — конфиг климата из JSON.
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

// GenerateArchetype — выбирает климат по весу спектрального класса
// и формирует архетип с базовыми весами композиции.
func GenerateArchetype(spectralClass string, rng *rand.Rand) *Archetype {
	if archetypeCache == nil || len(archetypeCache.Climates) == 0 {
		return fallbackArchetype()
	}

	var selectedClimate *ClimateConfig
	totalWeight := 0.0
	for i := range archetypeCache.Climates {
		if w, ok := archetypeCache.Climates[i].Weight[spectralClass]; ok {
			totalWeight += w
		}
	}

	if totalWeight == 0 {
		idx := rng.Intn(len(archetypeCache.Climates))
		selectedClimate = &archetypeCache.Climates[idx]
	} else {
		r := rng.Float64() * totalWeight
		for i := range archetypeCache.Climates {
			c := &archetypeCache.Climates[i]
			if w, ok := c.Weight[spectralClass]; ok {
				r -= w
				if r <= 0 {
					selectedClimate = c
					break
				}
			}
		}
		if selectedClimate == nil {
			selectedClimate = &archetypeCache.Climates[0]
		}
	}

	return buildArchetype(selectedClimate, rng)
}

// buildArchetype — формирует архетип из конфига климата.
func buildArchetype(c *ClimateConfig, rng *rand.Rand) *Archetype {
	hydro := pickOrFallback(c.AllowedHydrospheres, rng, "сухая")
	atmo := pickOrFallback(c.AllowedAtmospheres, rng, "разреженная")
	bio := pickOrFallback(c.AllowedBiospheres, rng, "стерильная")

	baseSurface := copyWeights(c.BaseSurface)
	baseSubterrain := copyWeights(c.BaseSubterrain)

	massMin := c.MassMin
	if massMin <= 0 {
		massMin = 0.1
	}
	massMax := c.MassMax
	if massMax <= massMin {
		massMax = massMin + 1.0
	}

	return &Archetype{
		ID:             c.ID,
		Name:           c.Name,
		Climate:        c.ID,
		BaseSurface:    baseSurface,
		BaseSubterrain: baseSubterrain,
		Hydrosphere:    hydro,
		Atmosphere:     atmo,
		Biosphere:      bio,
		TemperatureMin: c.TemperatureMin,
		TemperatureMax: c.TemperatureMax,
		WaterChance:    c.WaterChance,
		LifeChance:     c.LifeChance,
		MassMin:        massMin,
		MassMax:        massMax,
	}
}

// archetypeTemperate — архетип «Умеренный» из конфига (или fallback)
// с гарантированно землеподобными параметрами для прототипа поселения:
// океаны, азотно-кислородная атмосфера, растительная биосфера, жизнь 100%.
func (g *Generator) archetypeTemperate() *Archetype {
	a := fallbackArchetype()
	if archetypeCache != nil {
		for i := range archetypeCache.Climates {
			c := &archetypeCache.Climates[i]
			if c.ID != "temperate" {
				continue
			}
			massMin := c.MassMin
			if massMin <= 0 {
				massMin = 0.1
			}
			massMax := c.MassMax
			if massMax <= massMin {
				massMax = massMin + 1.0
			}
			a = &Archetype{
				ID:             c.ID,
				Name:           c.Name,
				Climate:        c.ID,
				BaseSurface:    copyWeights(c.BaseSurface),
				BaseSubterrain: copyWeights(c.BaseSubterrain),
				TemperatureMin: c.TemperatureMin,
				TemperatureMax: c.TemperatureMax,
				WaterChance:    c.WaterChance,
				LifeChance:     c.LifeChance,
				MassMin:        massMin,
				MassMax:        massMax,
			}
			break
		}
	}
	// Гарантия землеподобия: вода, жизнь, обитаемость и умеренная температура.
	a.Hydrosphere = "океаны"
	a.Atmosphere = "азотно-кислородная"
	a.Biosphere = "растительная"
	a.WaterChance = 1.0
	a.LifeChance = 1.0
	a.TemperatureMin = 270
	a.TemperatureMax = 310
	return a
}

// fallbackArchetype — используется, если JSON не загрузился.
func fallbackArchetype() *Archetype {
	return &Archetype{
		ID:      "fallback",
		Name:    "Землеподобная (fallback)",
		Climate: "temperate",
		BaseSurface: map[string]float64{
			SurfaceRocks:   0.3,
			SurfaceSands:   0.15,
			SurfaceOceans:  0.2,
			SurfaceLakes:   0.1,
			SurfaceForests: 0.15,
			SurfaceCraters: 0.1,
		},
		BaseSubterrain: map[string]float64{
			SubterrainEmptyRock:        0.3,
			SubterrainMagmaticRocks:    0.15,
			SubterrainSedimentaryRocks: 0.15,
			SubterrainOreVeins:         0.15,
			SubterrainGroundwater:      0.15,
			SubterrainCrystalVeins:     0.1,
		},
		Hydrosphere:    "океаны",
		Atmosphere:     "азотно-кислородная",
		Biosphere:      "растительная",
		TemperatureMin: 200,
		TemperatureMax: 350,
		WaterChance:    0.7,
		LifeChance:     0.4,
		MassMin:        0.3,
		MassMax:        2.0,
	}
}

// pickOrFallback — случайный элемент из списка, либо fallback если пусто.
func pickOrFallback(items []string, rng *rand.Rand, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return items[rng.Intn(len(items))]
}