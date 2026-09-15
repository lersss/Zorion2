// internal/generator/planet/circumbinary.go
package planet

import (
	"encoding/json"
	"math"

	"github.com/google/uuid"
	"zorion/internal/generator/settlement"
	"zorion/internal/names"
	"zorion/internal/resource"
)

// P-ветка циркумбинарных планет (35b §4.1). Отклонение от 99.2.4 §5.3
// (close уходит с общего пути на явную ветку), одобрено создателем
// 2026-09-15 (вариант A, решения №1 и №7 задания).
//
// Орбита: единственная P-орбита на систему, r_P = 3×companion_sep_au
// (парный пол, правило стабильности P-типа 3×). Температура честно от
// L_total = L₁+L₂ (сумма светимостей обоих компонентов), без архетип-
// клампов (§99.2.11 не применяются); абсолютные границы [20, 2500] K
// (аудит-гейты 99.2.4 §6.2) применяются всегда.

// circumbinaryLifeChance — шанс жизни P-планеты в пригодной полосе
// (200 < T < 400, вода > 10). Значение спека 35b §4.1 не фиксирует
// («→ шанс»); взято по образцу умеренного архетипа (life_chance 0.7,
// planet_archetypes.json).
const circumbinaryLifeChance = 0.7

// ==================== ПОЛОСЫ КОМПОЗИЦИИ ====================

// circumbinaryBand — полоса композиции P-планеты по честной T (35b §4.1:
// «горячая/умеренная/холодная полоса стандартного генератора композиции»).
// Источник базовых весов — конфиг архетипов (жаркий/умеренный/холодный);
// при незагруженном конфиге (тесты) — встроенные дефолты тех же значений.
type circumbinaryBand struct {
	id             string
	baseSurface    map[string]float64
	baseSubterrain map[string]float64
	atmospheres    []string
	massMin        float64
	massMax        float64
	biosphere      string // "растительная" в умеренной полосе (жизнь возможна), иначе "микробная"
}

// circumbinaryBandForTemp — полоса по честной T (границы — по диапазонам
// архетипов: жаркий ≥ 300, умеренный 200–300, холодный < 200).
func circumbinaryBandForTemp(temp float64) circumbinaryBand {
	switch {
	case temp >= 300:
		return circumbinaryBandByID("жаркий")
	case temp >= 200:
		return circumbinaryBandByID("умеренный")
	default:
		return circumbinaryBandByID("холодный")
	}
}

// circumbinaryBandByID — полоса из конфига архетипов (или дефолт).
func circumbinaryBandByID(id string) circumbinaryBand {
	if archetypeCache != nil {
		for i := range archetypeCache.Climates {
			c := &archetypeCache.Climates[i]
			if c.ID != id {
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
			bio := "микробная"
			if id == "умеренный" {
				bio = "растительная"
			}
			return circumbinaryBand{
				id:             id,
				baseSurface:    copyWeights(c.BaseSurface),
				baseSubterrain: copyWeights(c.BaseSubterrain),
				atmospheres:    append([]string(nil), c.AllowedAtmospheres...),
				massMin:        massMin,
				massMax:        massMax,
				biosphere:      bio,
			}
		}
	}
	return fallbackCircumbinaryBand(id)
}

// fallbackCircumbinaryBand — дефолтные полосы (значения совпадают с
// config/planet_archetypes.json; используются, пока конфиг не загружен).
func fallbackCircumbinaryBand(id string) circumbinaryBand {
	switch id {
	case "жаркий":
		return circumbinaryBand{
			id: "жаркий",
			baseSurface: map[string]float64{
				SurfaceRocks: 0.2, SurfaceSands: 0.4, SurfaceGlassFields: 0.1,
				SurfaceLavaFields: 0.08, SurfaceVolcanicFields: 0.07,
				SurfaceJungles: 0.05, SurfaceCraters: 0.1,
			},
			baseSubterrain: map[string]float64{
				SubterrainEmptyRock: 0.25, SubterrainMagmaticRocks: 0.2,
				SubterrainMetamorphicRocks: 0.1, SubterrainOreVeins: 0.15,
				SubterrainMagmaChambers: 0.08, SubterrainRadioactiveZones: 0.12,
				SubterrainCrystalVeins: 0.1,
			},
			atmospheres: []string{"плотная", "парниковая", "ядовитая", "облачная"},
			massMin:     0.1, massMax: 5.0, biosphere: "микробная",
		}
	case "умеренный":
		return circumbinaryBand{
			id: "умеренный",
			baseSurface: map[string]float64{
				SurfaceRocks: 0.13, SurfaceSands: 0.05, SurfaceLakes: 0.10,
				SurfaceOceans: 0.13, SurfaceMeadows: 0.16, SurfaceForests: 0.20,
				SurfaceJungles: 0.12, SurfaceSwamps: 0.07, SurfaceCraters: 0.04,
			},
			baseSubterrain: map[string]float64{
				SubterrainEmptyRock: 0.25, SubterrainSedimentaryRocks: 0.2,
				SubterrainMagmaticRocks: 0.1, SubterrainOreVeins: 0.1,
				SubterrainGroundwater: 0.15, SubterrainCoalSeams: 0.1,
				SubterrainOilPockets: 0.05, SubterrainCrystalVeins: 0.05,
			},
			atmospheres: []string{"азотно-кислородная", "углекислая", "туманная"},
			massMin:     0.2, massMax: 3.0, biosphere: "растительная",
		}
	default: // холодный
		return circumbinaryBand{
			id: "холодный",
			baseSurface: map[string]float64{
				SurfaceRocks: 0.25, SurfaceGlaciers: 0.35, SurfaceFrozenGases: 0.15,
				SurfaceCraters: 0.15, SurfaceLakes: 0.1,
			},
			baseSubterrain: map[string]float64{
				SubterrainEmptyRock: 0.3, SubterrainMagmaticRocks: 0.15,
				SubterrainMetamorphicRocks: 0.15, SubterrainGroundIce: 0.2,
				SubterrainCrystalVeins: 0.1, SubterrainOreVeins: 0.1,
			},
			atmospheres: []string{"разреженная", "метановая", "азотная"},
			massMin:     0.1, massMax: 3.0, biosphere: "микробная",
		}
	}
}

// ==================== ГЕНЕРАЦИЯ ====================

// generateCircumbinaryPlanet — P-планета тесной двойной (35b §4.1).
// Требует CompanionSepAU (новые миры); старые close-миры без sep идут
// общим путём как S-тип (фолбэк §2.4 — см. generateWorldWithCountIntoBuffer).
func (g *Generator) generateCircumbinaryPlanet(w WorldInfo, systemAge float64) *PlanetData {
	a := *w.Mods.CompanionSepAU
	rP := 3 * a
	lTotal := luminosityBySpectral(w.SpectralClass) + luminosityBySpectral(w.Mods.Companion)
	tEq := solarConstant * math.Pow(lTotal, 0.25) / math.Sqrt(rP)
	temp := clamp(tEq, TempAbsoluteMin, TempAbsoluteMax)

	// Гиганты P-типа разрешены: шанс от спектра главной, орбита — та же 3a.
	if g.rng.Float64() < gasGiantChance(w.SpectralClass) {
		return g.generateCircumbinaryGiant(w, rP, temp, systemAge)
	}
	return g.generateCircumbinaryRocky(w, rP, temp, systemAge)
}

// generateCircumbinaryRocky — каменистая P-планета: композиция/вода/жизнь
// по температурным полосам от честной T (без рулетки GenerateArchetype).
func (g *Generator) generateCircumbinaryRocky(w WorldInfo, rP, temp, systemAge float64) *PlanetData {
	band := circumbinaryBandForTemp(temp)

	// Масса по полосе.
	mass := band.massMin + g.rng.Float64()*(band.massMax-band.massMin)

	preliminarySurface := GenerateSurfaceComposition(band.baseSurface, 0, 0, g.rng)
	density := densityForPlanet(mass, preliminarySurface, g.rng)
	size := computeRadius(mass, density)
	preliminarySubterrain := GenerateSubterrainComposition(band.baseSubterrain, preliminarySurface, 0, 0, g.rng)
	core := GenerateCore(mass, band.id, preliminarySubterrain, systemAge, g.rng)

	atmosphere := band.atmospheres[g.rng.Intn(len(band.atmospheres))]

	// Вода по полосе от честной T: 250 < T < 400 → полный шанс;
	// 150 < T ≤ 250 → ×0.7; иначе ×0.3 (generateWater, свойства от честной T).
	waterPercent := generateWater(&Archetype{Hydrosphere: "", WaterChance: 1.0}, temp, g.rng)

	surfaceComp := GenerateSurfaceComposition(band.baseSurface, temp, waterPercent, g.rng)
	subterrainComp := GenerateSubterrainComposition(band.baseSubterrain, surfaceComp, temp, waterPercent, g.rng)

	// ~3–4% планет — «примитивные» тела (как в общем пути).
	if g.rng.Float64() < primitivePlanetProbability {
		surfaceComp = simplifyComposition(surfaceComp, g.rng)
		subterrainComp = simplifyComposition(subterrainComp, g.rng)
	}

	// Жизнь по полосе: вода > 10 и 200 < T < 400 → шанс (generateLife).
	life := generateLife(
		&Archetype{Biosphere: band.biosphere, LifeChance: circumbinaryLifeChance},
		waterPercent, temp, g.rng,
	)
	// Пригодность — по общим правилам (35b §4.1): settlement.Suitable.
	// Порядок аргументов — сигнатурный (water, temp), как в
	// descriptions_tags.go inhabited(); properties.go передаёт (temp, water)
	// — пре-существующая несогласованность, вне скоупа 35b.
	settleable := settlement.Suitable(waterPercent, temp, atmosphere, life, false, false)
	moons := determineMoons(size, band.id, g.rng)

	political := "нет"
	if settleable {
		systems := []string{
			"демократия", "диктатура", "теократия",
			"корпоратократия", "анархия", "ИИ-управление",
		}
		political = systems[g.rng.Intn(len(systems))]
	}
	devLevel := 0.0
	if settleable {
		devLevel = 0.1 + g.rng.Float64()*0.9
	}

	// Гидросфера — честная производная от воды полосы.
	hydrosphere := "сухая"
	if waterPercent > 50 {
		hydrosphere = "океаны"
	} else if waterPercent > 20 {
		hydrosphere = "озёра"
	}

	dominant := surfaceComp.DominantForm()
	if dominant == "" {
		dominant = SurfaceRocks
	}
	gdType := ClassifyGameDesignType(PlanetClassificationInput{
		IsGasGiant:    false,
		IsRadioactive: false,
		Surface:       surfaceComp,
		Temperature:   temp,
		WaterPercent:  waterPercent,
		Settleable:    settleable,
		Life:          life,
	})

	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Планета-" + uuidShort()
	}
	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         gdType,
		OrbitIndex:   1,
		Atmosphere:   atmosphere,
		Hydrosphere:  hydrosphere,
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
		"size":              size,
		"mass":              mass,
		"density":           density,
		"gravity":           computeGravity(mass, size),
		"atmosphere":        atmosphere,
		"hydrosphere":       hydrosphere,
		"biosphere":         band.biosphere,
		"temperature":       temp,
		"water_percent":     waterPercent,
		"life":              life,
		"political_system":  political,
		"moons":             moons,
		"development_level": devLevel,
		"archetype":         band.id,
		"system_age":        systemAge,

		"surface_composition":    composeToJSON(surfaceComp),
		"subterrain_composition": composeToJSON(subterrainComp),
		"surface_dominant":       dominant,
		"type":                   gdType,
		"core":                   coreToJSON(core),

		// Орбитальный контекст P-планеты (35b §2.2): вокруг барицентра пары.
		"orbit_center":    "barycenter",
		"orbit_radius_au": rP,
		"circumbinary":    true,

		"description": GenerateDescription(descCtx),
	}

	resources := attachResources(
		data, planetID, dominant,
		map[string]float64(subterrainComp),
		w.SpectralClass, g.rng,
	)

	dataJSON, _ := json.Marshal(data)

	return &PlanetData{
		ID:         planetID,
		WorldID:    w.ID,
		Name:       name,
		OrbitIndex: 1,
		Data:       dataJSON,
		Resources:  resources,
	}
}

// generateCircumbinaryGiant — газовый гигант P-типа (35b §4.1): масса по
// распределению гигантов, температура от L_total (равновесная × парник + 30).
func (g *Generator) generateCircumbinaryGiant(w WorldInfo, rP, temp, systemAge float64) *PlanetData {
	mass := g.gasGiantMass()
	size := GasGiantRadius(mass)
	density := mass / (size * size * size)

	atmospheres := []string{"водородно-гелиевая", "водородная", "гелиевая"}
	atmosphere := atmospheres[g.rng.Intn(len(atmospheres))]

	giantTemp := temp*computeGreenhouse(atmosphere) + 30
	giantTemp = clamp(giantTemp, TempAbsoluteMin, TempAbsoluteMax)

	emptySubterrain := Composition{}
	core := GenerateCore(mass, "жаркий", emptySubterrain, systemAge, g.rng)

	satelliteCount := 3 + g.rng.Intn(8)
	satellites := g.generateSatellites(satelliteCount, w.Name, size, giantTemp, w.SpectralClass)

	satellitesJSON := make([]map[string]interface{}, 0, len(satellites))
	for _, sat := range satellites {
		satellitesJSON = append(satellitesJSON, satelliteToMap(sat))
	}

	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Газовый гигант-" + uuidShort()
	}
	planetID := uuid.New().String()

	resources := resource.GenerateGasGiantResource(planetID, g.rng)
	resourceSummary := resource.Summary(resources)

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         TypeGasGiant,
		OrbitIndex:   1,
		Atmosphere:   atmosphere,
		Hydrosphere:  "сухая",
		Temperature:  giantTemp,
		WaterPercent: 0.0,
		Mass:         mass,
		Density:      density,
		Moons:        satelliteCount,
		Life:         false,
		Surface:      nil,
		Core:         core,
		IsGasGiant:   true,
	}

	data := map[string]interface{}{
		"size":              size,
		"mass":              mass,
		"density":           density,
		"gravity":           computeGravity(mass, size),
		"atmosphere":        atmosphere,
		"hydrosphere":       "сухая",
		"biosphere":         "стерильная",
		"temperature":       giantTemp,
		"water_percent":     0.0,
		"life":              false,
		"political_system":  "нет",
		"moons":             satelliteCount,
		"development_level": 0.0,
		"archetype":         "жаркий",
		"system_age":        systemAge,
		"is_gas_giant":      true,
		"resources":         resourceSummary,
		"satellites":        satellitesJSON,
		"surface_dominant":  "газовый_гигант",
		"type":              TypeGasGiant,
		"core":              coreToJSON(core),

		// Орбитальный контекст P-планеты (35b §2.2).
		"orbit_center":    "barycenter",
		"orbit_radius_au": rP,
		"circumbinary":    true,

		"description": GenerateDescription(descCtx),
	}
	dataJSON, _ := json.Marshal(data)

	return &PlanetData{
		ID:         planetID,
		WorldID:    w.ID,
		Name:       name,
		OrbitIndex: 1,
		Data:       dataJSON,
		Resources:  resources,
	}
}