// internal/generator/planet/exotic.go
package planet

import (
	"encoding/json"
	"math"

	"github.com/google/uuid"
	"zorion/internal/names"
)

// generateExoticPlanet — планета у экзотического объекта (99.2.4 §5.3).
// Собственный выход «тип → физика → ярлык архетипа» (§6.1): общий путь O–Y
// и рулетка GenerateArchetype НЕ вызываются.
//
// Запреты ветки: океанические не генерируются (минует физику, ревизия п.13),
// газовые гиганты не генерируются (диски фотоиспарились/кик), GenerateArchetype
// не вызывается. Параметры: вода 0, жизнь 0, settleable=false, композиция
// каменистая/кратерная (нормализована до 100, неотрицательна) — аудит-гейты
// §6.2 держатся самой веткой (T 25–150 K внутри [20, 2500]).
func (g *Generator) generateExoticPlanet(w WorldInfo, orbitIndex int) *PlanetData {
	var temp float64
	switch w.StarType {
	case "black_hole":
		temp = 30 + g.rng.Float64()*50 // 30–80 K, тёмная: фоновый нагрев (§5.3)
	case "neutron":
		temp = 30 + g.rng.Float64()*120 // 30–150 K, остаточный/ветровой нагрев (§5.3)
	case "white_dwarf":
		temp = g.wdPlanetTemp() // 25–37 K, кламп ≥25 (баланс-проверка В4)
	default:
		return nil // протозвезда — диск вместо планет
	}
	return g.buildExoticPlanet(w, orbitIndex, temp)
}

// wdPlanetTemp — равновесная температура планеты белого карлика (99.2.4 §5.3):
// L ∈ [10⁻⁴, 10⁻²] L☉, орбиты 5–8 (r 5.7–28 а.е.) → формула даёт 5.3–36.9 K,
// итог клампится снизу к 25 K (остаточное тепло ядра + фоновый нагрев).
// Результат всегда 25–37 K — внутри [20, 2500] с запасом (гейт §6.2).
func (g *Generator) wdPlanetTemp() float64 {
	l := math.Pow(10, -4+g.rng.Float64()*2) // [1e-4, 1e-2] L☉
	orbit := 5 + g.rng.Intn(4)              // орбиты 5..8
	t := computeEquilibriumTemp(l, orbitRadiusByIndex(orbit))
	if t < 25 {
		t = 25
	}
	return t
}

// buildExoticPlanet — собирает JSON планеты остатка (мёртвое каменистое тело).
func (g *Generator) buildExoticPlanet(w WorldInfo, orbitIndex int, temp float64) *PlanetData {
	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Мёртвая-" + uuidShort()
	}

	// Каменистое тело: масса 0.05–0.45 M⊕, плотность по каменистой композиции.
	mass := 0.05 + g.rng.Float64()*0.4
	density := 0.9 + g.rng.Float64()*0.4
	size := computeRadius(mass, density)

	surfaceComp := Composition{
		SurfaceRocks:   50 + g.rng.Float64()*20,
		SurfaceCraters: 30 + g.rng.Float64()*20,
	}.Normalize().NonZero()

	subterrainComp := Composition{
		SubterrainEmptyRock:        60 + g.rng.Float64()*20,
		SubterrainSedimentaryRocks: 20 + g.rng.Float64()*10,
	}.Normalize().NonZero()

	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         TypeDead, // «мёртвая» — консистентность-правила её не трогают
		OrbitIndex:   orbitIndex,
		Atmosphere:   "нет",
		Hydrosphere:  "сухая",
		Temperature:  temp,
		WaterPercent: 0,
		Mass:         mass,
		Density:      density,
		Moons:        0,
		Life:         false,
		Surface:      surfaceComp,
		Core:         Core{},
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":                   size,
		"mass":                   mass,
		"density":                density,
		"gravity":                computeGravity(mass, size),
		"atmosphere":             "нет",
		"hydrosphere":            "сухая",
		"biosphere":              "стерильная",
		"temperature":            temp,
		"water_percent":          0.0,
		"life":                   false,
		"political_system":       "нет",
		"moons":                  0,
		"development_level":      0.0,
		"archetype":              "холодный", // ярлык архетипа экзотики (§6.1)
		"system_age":             determineSystemAge("", g.rng),
		"surface_composition":    composeToJSON(surfaceComp),
		"subterrain_composition": composeToJSON(subterrainComp),
		"surface_dominant":       SurfaceRocks,
		"type":                   TypeDead,
		"settleable":             false,
		"star_type":              w.StarType,
		"exotic_system":          true, // читают аудит (§6.2) и UI (§8)
		"description":            GenerateDescription(descCtx),
	}

	dataJSON, _ := json.Marshal(data)

	return &PlanetData{
		ID:         planetID,
		WorldID:    w.ID,
		Name:       name,
		OrbitIndex: orbitIndex,
		Data:       dataJSON,
	}
}