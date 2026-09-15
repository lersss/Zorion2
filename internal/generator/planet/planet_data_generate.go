// internal/generator/planet/planet_data_generate.go
package planet

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/google/uuid"
	"zorion/internal/generator/settlement"
	"zorion/internal/names"
)

// determinePlanetCount — сколько планет у звезды данного класса.
// Mean-модель (99.2.4 §5.2): n = floor(mean) + Бернулли(frac), потолок 8.
func (g *Generator) determinePlanetCount(spectralClass string) int {
	return meanPlanetCount(g.rng, g.means.meanForClass(spectralClass), g.means.Max)
}

// planetCountFor — число планет для мира (99.2.4 §5.2): mean-модель по типу.
// Обычные звёзды — mean по классу; двойные широкие ×0.9 (S-тип), тесные
// mean 0.2 (P-тип); кратные ×0.9; остатки/прочая экзотика — свои mean
// (максимум генератора 1, «чаще 0»); протозвезда — 0 (диск вместо планет).
func (g *Generator) planetCountFor(w WorldInfo) int {
	m := g.means
	switch w.StarType {
	case "star", "":
		if w.Mods != nil && w.Mods.IsSupergiantExotic() {
			// Прочая экзотика (сверхгиганты O–B–A, фаза I): mean 0.1, максимум 1.
			return capRemnant(meanPlanetCount(g.rng, m.Exotic, m.Max))
		}
		mean := m.meanForClass(w.SpectralClass)
		switch w.SystemType {
		case "binary":
			if w.Mods != nil && w.Mods.BinaryType == "close" {
				// P-тип редок (Kepler-16/47): mean 0.2.
				return meanPlanetCount(g.rng, m.BinaryCloseMean, m.Max)
			}
			// S-тип: планеты у главного компонента, ×0.9.
			return meanPlanetCount(g.rng, mean*m.BinaryWideFactor, m.Max)
		case "multiple":
			// wide-семантика: S-тип у главного компонента, ×0.9.
			return meanPlanetCount(g.rng, mean*m.MultipleFactor, m.Max)
		default:
			return meanPlanetCount(g.rng, mean, m.Max)
		}
	case "black_hole":
		return capRemnant(meanPlanetCount(g.rng, m.BlackHole, m.Max))
	case "neutron":
		return capRemnant(meanPlanetCount(g.rng, m.Neutron, m.Max))
	case "white_dwarf":
		return capRemnant(meanPlanetCount(g.rng, m.WhiteDwarf, m.Max))
	case "protostar":
		return 0 // диск вместо планет (§5.3)
	}
	return 0
}

// capRemnant — «максимум генератора 1» у остатков (99.2.4 §5.2, решение §4к.1).
func capRemnant(n int) int {
	if n > 1 {
		return 1
	}
	return n
}

// ==================== СРЕДНЕЕ ЧИСЛО ПЛАНЕТ (99.2.4 §5.2, конфиг 99.2.3 §4.3) ====================

// PlanetMeans — среднее число планет по типу звезды. Механика целого счёта:
// n = floor(mean) + Бернулли(frac), потолок 8 (Kepler-90). При mean < 1 —
// n ∈ {0, 1} («чаще 0»), верхний предел остатков — 1 (решение §4к.1).
//
// Дефолты — физический реализм (ревизия астронома §4е, решения §4ж/§4и.3):
// M ~2.5, K ~2, G/F ~1.75, A ~1, O/B ~0.3, L/T/Y ~1; экзотика: ЧД 0.1,
// НЗ 0.05, WD 0.3, протозвезда 0 (диск), прочая экзотика 0.1; двойные
// широкие ×0.9, тесные mean 0.2, кратные ×0.9.
type PlanetMeans struct {
	O float64 `json:"O"`
	B float64 `json:"B"`
	A float64 `json:"A"`
	F float64 `json:"F"`
	G float64 `json:"G"`
	K float64 `json:"K"`
	M float64 `json:"M"`
	L float64 `json:"L"`
	T float64 `json:"T"`
	Y float64 `json:"Y"`

	// Экзотика (максимум генератора 1, «чаще 0»).
	BlackHole  float64 `json:"black_hole"`
	Neutron    float64 `json:"neutron"`
	WhiteDwarf float64 `json:"white_dwarf"`
	Protostar  float64 `json:"protostar"`
	Exotic     float64 `json:"exotic"` // прочая экзотика (сверхгиганты O–B–A, фаза I)

	// Двойные/кратные: S-тип у главного компонента, P-тип редок.
	BinaryWideFactor float64 `json:"binary_wide_factor"` // ×0.9
	BinaryCloseMean  float64 `json:"binary_close_mean"`  // mean 0.2
	MultipleFactor   float64 `json:"multiple_factor"`    // ×0.9

	// Max — потолок числа планет (Kepler-90 = 8, решение §4е).
	Max int `json:"max"`
}

// DefaultPlanetMeans — физические дефолты (99.2.4 §5.2).
func DefaultPlanetMeans() PlanetMeans {
	return PlanetMeans{
		O: 0.3, B: 0.3, A: 1, F: 1.75, G: 1.75, K: 2, M: 2.5, L: 1, T: 1, Y: 1,
		BlackHole: 0.1, Neutron: 0.05, WhiteDwarf: 0.3, Protostar: 0, Exotic: 0.1,
		BinaryWideFactor: 0.9, BinaryCloseMean: 0.2, MultipleFactor: 0.9,
		Max: 8,
	}
}

// meanForClass — среднее по спектральному классу обычной звезды.
func (m PlanetMeans) meanForClass(cls string) float64 {
	switch cls {
	case "O":
		return m.O
	case "B":
		return m.B
	case "A":
		return m.A
	case "F":
		return m.F
	case "G":
		return m.G
	case "K":
		return m.K
	case "M":
		return m.M
	case "L":
		return m.L
	case "T":
		return m.T
	case "Y":
		return m.Y
	}
	return 0
}

// Validate — 0 ≤ mean ≤ 8 (99.2.3 §4.3): mean > 8 молча обрежет распределение
// (E[n] ≠ mean), поэтому значение отклоняется валидацией, а не клампится.
func (m PlanetMeans) Validate() error {
	for name, v := range m.all() {
		if v < 0 || v > 8 {
			return fmt.Errorf("%s: mean %.2f вне [0, 8]", name, v)
		}
	}
	return nil
}

// all — все редактируемые значения таблицы средних (для валидации).
func (m PlanetMeans) all() map[string]float64 {
	return map[string]float64{
		"O": m.O, "B": m.B, "A": m.A, "F": m.F, "G": m.G,
		"K": m.K, "M": m.M, "L": m.L, "T": m.T, "Y": m.Y,
		"black_hole":         m.BlackHole,
		"neutron":            m.Neutron,
		"white_dwarf":        m.WhiteDwarf,
		"protostar":          m.Protostar,
		"exotic":             m.Exotic,
		"binary_wide_factor": m.BinaryWideFactor,
		"binary_close_mean":  m.BinaryCloseMean,
		"multiple_factor":    m.MultipleFactor,
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
	return g.generateStandardPlanet(
		worldID, worldName, orbitIndex, spectralClass, systemAge,
		GenerateArchetype(spectralClass, g.rng),
	)
}

// generateStandardPlanet — планета по явному архетипу (стандартный путь).
func (g *Generator) generateStandardPlanet(
	worldID, worldName string,
	orbitIndex int,
	spectralClass string,
	systemAge float64,
	archetype *Archetype,
) *PlanetData {
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
		Settleable:    props.Settleable,
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
		Surface:      props.SurfaceComposition,
		Core:         props.Core,
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":              props.Size,
		"mass":              props.Mass,
		"density":           props.Density,
		"gravity":           computeGravity(props.Mass, props.Size),
		"atmosphere":        props.Atmosphere,
		"hydrosphere":       archetype.Hydrosphere,
		"biosphere":         archetype.Biosphere,
		"temperature":       props.Temperature,
		"water_percent":     props.WaterPercent,
		"life":              props.Life,
		"political_system":  props.Political,
		"moons":             props.Moons,
		"development_level": props.Development,
		"archetype":         archetype.ArchetypeID,
		"system_age":        systemAge,

		// Орбитальный контекст S-планеты (35b §2.2): вокруг главной.
		"orbit_center":    "main",
		"orbit_radius_au": orbitRadiusByIndex(orbitIndex),

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

// GeneratePrototypePlanet — землеподобная планета для прототипа поселения:
// умеренный архетип, жизнь и вода. Внутренняя орбита (1).
// Население задаётся отдельной вставкой поселения в admin_universe.go.
func (g *Generator) GeneratePrototypePlanet(worldID, worldName, spectralClass string) *PlanetData {
	return g.generateStandardPlanet(
		worldID, worldName, 1, spectralClass,
		determineSystemAge(spectralClass, g.rng),
		g.archetypeTemperate(),
	)
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

	core := GenerateCore(mass, "умеренный", subterrainComp, systemAge, g.rng)

	political := "нет"
	if settlement.Suitable(temp, waterPercent, atmosphere, life, false, false) {
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
		Surface:      surfaceComp,
		Core:         core,
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":                   size,
		"mass":                   mass,
		"density":                density,
		"gravity":                computeGravity(mass, size),
		"atmosphere":             atmosphere,
		"hydrosphere":            "океаны",
		"biosphere":              "растительная",
		"temperature":            temp,
		"water_percent":          waterPercent,
		"life":                   life,
		"political_system":       political,
		"moons":                  moons,
		"development_level":      0.0,
		"archetype":              "умеренный",
		"system_age":             systemAge,
		"orbit_center":           "main",
		"orbit_radius_au":        orbitRadiusByIndex(orbitIndex),
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

	core := GenerateCore(mass, "экстремальный", subterrainComp, systemAge, g.rng)

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
		Surface:      surfaceComp,
		Core:         core,
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":                   size,
		"mass":                   mass,
		"density":                density,
		"gravity":                computeGravity(mass, size),
		"atmosphere":             atmosphere,
		"hydrosphere":            "сухая",
		"biosphere":              "стерильная",
		"temperature":            temp,
		"water_percent":          waterPercent,
		"life":                   life,
		"political_system":       "нет",
		"moons":                  moons,
		"development_level":      0.0,
		"archetype":              "экстремальный",
		"system_age":             systemAge,
		"radioactive":            true,
		"orbit_center":           "main",
		"orbit_radius_au":        orbitRadiusByIndex(orbitIndex),
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
