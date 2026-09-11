// internal/generator/planet/planet_data_generate.go
package planet

import (
	"encoding/json"
	"math/rand"

	"github.com/google/uuid"
	"zorion/internal/names"
)

// determinePlanetCount — сколько планет у звезды данного класса.
func (g *Generator) determinePlanetCount(spectralClass string) int {
	switch spectralClass {
	case "O", "B", "A":
		return g.rng.Intn(9)
	case "F", "G":
		return 2 + g.rng.Intn(7)
	case "K", "M":
		return g.rng.Intn(7)
	default:
		return g.rng.Intn(5)
	}
}

// determineSystemAge — возраст звёздной системы в млрд лет.
func determineSystemAge(spectralClass string, rng *rand.Rand) float64 {
	switch spectralClass {
	case "O", "B":
		return 0.1 + rng.Float64()*0.9
	case "A":
		return 0.3 + rng.Float64()*1.7
	case "F":
		return 1.0 + rng.Float64()*2.0
	case "G":
		return 2.0 + rng.Float64()*6.0
	case "K":
		return 4.0 + rng.Float64()*7.0
	case "M":
		return 6.0 + rng.Float64()*7.0
	case "L", "T", "Y":
		return 5.0 + rng.Float64()*8.0
	default:
		return 2.0 + rng.Float64()*8.0
	}
}

// ==================== ГАЗОВЫЕ ГИГАНТЫ ====================

// gasGiantChance — шанс газового гиганта на дальней орбите
// в зависимости от спектрального класса звезды.
func gasGiantChance(spectralClass string) float64 {
	switch spectralClass {
	case "O", "B", "A":
		return 0.8
	case "F", "G":
		return 0.5
	case "K", "M":
		return 0.3
	case "L", "T", "Y":
		return 0.1
	default:
		return 0.3
	}
}

// ==================== ОБЫЧНАЯ ПЛАНЕТА ====================

func (g *Generator) generatePlanet(
	worldID string,
	worldName string,
	orbitIndex int,
	spectralClass string,
	systemAge float64,
) *PlanetData {
	// --- ГАЗОВЫЙ ГИГАНТ ---
	if orbitIndex >= 3 {
		if g.rng.Float64() < gasGiantChance(spectralClass) {
			return g.generateGasGiant(worldID, worldName, orbitIndex, spectralClass, systemAge)
		}
	}

	// --- ОКЕАНИЧЕСКАЯ ПЛАНЕТА (2.5%) ---
	if g.rng.Float64() < 0.025 {
		return g.generateOceanicPlanet(worldID, orbitIndex, spectralClass, systemAge)
	}

	// --- РАДИОАКТИВНАЯ ПЛАНЕТА (1.5%, для горячих 3%) ---
	hotStars := map[string]bool{"O": true, "B": true, "A": true}
	chance := 0.015
	if hotStars[spectralClass] {
		chance = 0.03
	}
	if g.rng.Float64() < chance {
		return g.generateRadioactivePlanet(worldID, orbitIndex, spectralClass, systemAge)
	}

	// --- СТАНДАРТНАЯ ГЕНЕРАЦИЯ ЧЕРЕЗ АРХЕТИП ---
	archetype := GenerateArchetype(spectralClass, g.rng)
	props := GenerateProperties(archetype, orbitIndex, spectralClass, systemAge, g.rng)

	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Планета-" + uuidShort()
	}

	dominant := props.SurfaceComposition.DominantForm()
	if dominant == "" {
		dominant = SurfaceRocks
	}

	// Геймдизайнерский тип по композиции
	gdType := ClassifyGameDesignType(PlanetClassificationInput{
		IsGasGiant:    false,
		IsRadioactive: false,
		Surface:       props.SurfaceComposition,
		Temperature:   props.Temperature,
		WaterPercent:  props.WaterPercent,
		Habitable:     props.Habitable,
		Life:          props.Life,
	})

	// UUID генерируется ЗАРАНЕЕ — нужен для детерминированного выбора описания.
	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         gdType,
		OrbitIndex:   orbitIndex,
		Atmosphere:   props.Atmosphere,
		Hydrosphere:  archetype.Hydrosphere,
		Temperature:  props.Temperature,
		WaterPercent: props.WaterPercent,
		Mass:         props.Mass,
		Density:      props.Density,
		Moons:        props.Moons,
		Life:         props.Life,
		Habitable:    props.Habitable,
		Population:   props.Population,
		Surface:      props.SurfaceComposition,
		Core:         props.Core,
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":              props.Size,
		"mass":              props.Mass,
		"density":           props.Density,
		"atmosphere":        props.Atmosphere,
		"hydrosphere":       archetype.Hydrosphere,
		"biosphere":         archetype.Biosphere,
		"temperature":       props.Temperature,
		"water_percent":     props.WaterPercent,
		"habitable":         props.Habitable,
		"life":              props.Life,
		"population":        props.Population,
		"political_system":  props.Political,
		"conflict_level":    props.ConflictLevel,
		"moons":             props.Moons,
		"development_level": props.Development,
		"climate":           archetype.Climate,
		"system_age":        systemAge,

		"surface_composition":    composeToJSON(props.SurfaceComposition),
		"subterrain_composition": composeToJSON(props.SubterrainComposition),
		"surface_dominant":       dominant,
		"type":                   gdType,
		"core":                   coreToJSON(props.Core),

		"description": GenerateDescription(descCtx),
	}

	// Ресурсы генерируются здесь же: summary попадает в JSON планеты
	// (data["resources"]), сами ресурсы живут только в памяти.
	resources := attachResources(
		data, planetID, dominant,
		map[string]float64(props.SubterrainComposition),
		spectralClass, g.rng,
	)

	dataJSON, _ := json.Marshal(data)

	return &PlanetData{
		ID:         planetID,
		WorldID:    worldID,
		Name:       name,
		OrbitIndex: orbitIndex,
		Data:       dataJSON,
		Resources:  resources,
	}
}

// ==================== ОКЕАНИЧЕСКАЯ ПЛАНЕТА ====================

func (g *Generator) generateOceanicPlanet(
	worldID string,
	orbitIndex int,
	spectralClass string,
	systemAge float64,
) *PlanetData {
	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Океаническая-" + uuidShort()
	}

	atmospheres := []string{"азотно-кислородная", "плотная"}
	atmosphere := atmospheres[g.rng.Intn(len(atmospheres))]

	mass := 0.5 + g.rng.Float64()*2.5

	temp := 273 + g.rng.Float64()*100
	waterPercent := 70 + g.rng.Float64()*29
	life := g.rng.Float64() < 0.7
	habitable := life

	surfaceComp := Composition{
		SurfaceOceans:     60 + g.rng.Float64()*15,
		SurfaceLakes:      5 + g.rng.Float64()*10,
		SurfaceRocks:      5 + g.rng.Float64()*10,
		SurfaceSands:      5 + g.rng.Float64()*10,
		SurfaceCoralReefs: 3 + g.rng.Float64()*7,
	}.Normalize().NonZero()

	subterrainComp := Composition{
		SubterrainSedimentaryRocks: 30,
		SubterrainOilPockets:       15,
		SubterrainSaltDomes:        10,
		SubterrainGroundwater:      20,
		SubterrainOreVeins:         15,
		SubterrainEmptyRock:        10,
	}.Normalize().NonZero()

	// ~3–4% планет — «примитивные» тела: 1–2 типа поверхности и недр.
	if g.rng.Float64() < primitivePlanetProbability {
		surfaceComp = simplifyComposition(surfaceComp, g.rng)
		subterrainComp = simplifyComposition(subterrainComp, g.rng)
	}

	density := densityForPlanet(mass, surfaceComp, g.rng)
	size := computeRadius(mass, density)
	moons := int(size / 5)

	core := GenerateCore(mass, "temperate", subterrainComp, systemAge, g.rng)

	var population int64 = 0
	political := "нет"
	if life {
		basePop := int64(1000000 + g.rng.Float64()*999000000)
		dev := 0.1 + g.rng.Float64()*0.9
		population = int64(float64(basePop) * dev)
		systems := []string{
			"демократия", "диктатура", "теократия",
			"корпоратократия", "анархия", "ИИ-управление",
		}
		political = systems[g.rng.Intn(len(systems))]
	}

	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         TypeOceanic,
		OrbitIndex:   orbitIndex,
		Atmosphere:   atmosphere,
		Hydrosphere:  "океаны",
		Temperature:  temp,
		WaterPercent: waterPercent,
		Mass:         mass,
		Density:      density,
		Moons:        moons,
		Life:         life,
		Habitable:    habitable,
		Population:   population,
		Surface:      surfaceComp,
		Core:         core,
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":                   size,
		"mass":                   mass,
		"density":                density,
		"atmosphere":             atmosphere,
		"hydrosphere":            "океаны",
		"biosphere":              "растительная",
		"temperature":            temp,
		"water_percent":          waterPercent,
		"habitable":              habitable,
		"life":                   life,
		"population":             population,
		"political_system":       political,
		"conflict_level":         0.0,
		"moons":                  moons,
		"development_level":      0.0,
		"climate":                "temperate",
		"system_age":             systemAge,
		"surface_composition":    composeToJSON(surfaceComp),
		"subterrain_composition": composeToJSON(subterrainComp),
		"surface_dominant":       SurfaceOceans,
		"type":                   TypeOceanic,
		"core":                   coreToJSON(core),
		"description":            GenerateDescription(descCtx),
	}

	resources := attachResources(
		data, planetID, SurfaceOceans,
		map[string]float64(subterrainComp),
		spectralClass, g.rng,
	)

	dataJSON, _ := json.Marshal(data)

	return &PlanetData{
		ID:         planetID,
		WorldID:    worldID,
		Name:       name,
		OrbitIndex: orbitIndex,
		Data:       dataJSON,
		Resources:  resources,
	}
}

// ==================== РАДИОАКТИВНАЯ ПЛАНЕТА ====================

func (g *Generator) generateRadioactivePlanet(
	worldID string,
	orbitIndex int,
	spectralClass string,
	systemAge float64,
) *PlanetData {
	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Радиоактивная-" + uuidShort()
	}

	// Доминирующая форма поверхности: металлические или стеклянные поля.
	dominantSurface := SurfaceMetalFields
	if g.rng.Intn(2) == 0 {
		dominantSurface = SurfaceGlassFields
	}

	atmospheres := []string{"плотная", "ядовитая"}
	atmosphere := atmospheres[g.rng.Intn(len(atmospheres))]

	mass := 0.5 + g.rng.Float64()*9.5

	luminosity := luminosityBySpectral(spectralClass)
	orbitRadius := orbitRadiusByIndex(orbitIndex)
	baseTemp := computeEquilibriumTemp(luminosity, orbitRadius)

	temp := baseTemp + 200 + g.rng.Float64()*200
	if temp > 1200 {
		temp = 1200
	}

	waterPercent := 0.0
	if g.rng.Float64() < 0.1 {
		waterPercent = g.rng.Float64() * 20
	}
	life := g.rng.Float64() < 0.05

	surfaceComp := Composition{
		dominantSurface:       50 + g.rng.Float64()*20,
		SurfaceRocks:          15 + g.rng.Float64()*10,
		SurfaceCraters:        10 + g.rng.Float64()*10,
		SurfaceVolcanicFields: 5 + g.rng.Float64()*10,
	}.Normalize().NonZero()

	subterrainComp := Composition{
		SubterrainRadioactiveZones: 30,
		SubterrainMetalCores:       20,
		SubterrainRareEarthVeins:   20,
		SubterrainMagmaticRocks:    15,
		SubterrainOreVeins:         15,
	}.Normalize().NonZero()

	// ~3–4% планет — «примитивные» тела: 1–2 типа поверхности и недр.
	if g.rng.Float64() < primitivePlanetProbability {
		surfaceComp = simplifyComposition(surfaceComp, g.rng)
		subterrainComp = simplifyComposition(subterrainComp, g.rng)
	}

	density := 1.2 + g.rng.Float64()*0.6
	size := computeRadius(mass, density)
	moons := int(size / 8)

	core := GenerateCore(mass, "extreme", subterrainComp, systemAge, g.rng)

	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         TypeRadioactive,
		OrbitIndex:   orbitIndex,
		Atmosphere:   atmosphere,
		Hydrosphere:  "сухая",
		Temperature:  temp,
		WaterPercent: waterPercent,
		Mass:         mass,
		Density:      density,
		Moons:        moons,
		Life:         life,
		Habitable:    false,
		Population:   0,
		Surface:      surfaceComp,
		Core:         core,
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":                   size,
		"mass":                   mass,
		"density":                density,
		"atmosphere":             atmosphere,
		"hydrosphere":            "сухая",
		"biosphere":              "стерильная",
		"temperature":            temp,
		"water_percent":          waterPercent,
		"habitable":              false,
		"life":                   life,
		"population":             0,
		"political_system":       "нет",
		"conflict_level":         0.0,
		"moons":                  moons,
		"development_level":      0.0,
		"climate":                "extreme",
		"system_age":             systemAge,
		"radioactive":            true,
		"surface_composition":    composeToJSON(surfaceComp),
		"subterrain_composition": composeToJSON(subterrainComp),
		"surface_dominant":       dominantSurface,
		"type":                   TypeRadioactive,
		"core":                   coreToJSON(core),
		"description":            GenerateDescription(descCtx),
	}

	resources := attachResources(
		data, planetID, dominantSurface,
		map[string]float64(subterrainComp),
		spectralClass, g.rng,
	)

	dataJSON, _ := json.Marshal(data)

	return &PlanetData{
		ID:         planetID,
		WorldID:    worldID,
		Name:       name,
		OrbitIndex: orbitIndex,
		Data:       dataJSON,
		Resources:  resources,
	}
}

// ==================== УТИЛИТЫ ====================

// coreToJSON — сериализует ядро для JSON-поля.
func coreToJSON(c Core) map[string]interface{} {
	return map[string]interface{}{
		"type":          c.Type,
		"mass_percent":  c.MassPercent,
		"activity":      c.Activity,
		"radioactivity": c.Radioactivity,
		"age":           c.Age,
		"is_active":     c.IsActive(),
		"is_metallic":   c.IsMetallic(),
	}
}
