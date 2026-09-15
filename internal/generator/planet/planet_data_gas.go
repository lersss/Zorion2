// internal/generator/planet/planet_data_gas.go
package planet

import (
	"encoding/json"

	"github.com/google/uuid"
	"zorion/internal/names"
	"zorion/internal/resource"
)

// generateGasGiant — газовый гигант. У него нет композиции поверхности
// (только атмосфера), но есть спутники — каждый полноценная локация.
//
// Ядро есть (металлическое, по массе), но при расчёте температуры
// поверхности оно игнорируется (SkipInternal = true): газовый гигант
// греется в основном за счёт сжатия и внутренних процессов.
func (g *Generator) generateGasGiant(
	worldID string,
	worldName string,
	orbitIndex int,
	spectralClass string,
	systemAge float64,
) *PlanetData {
	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Газовый гигант-" + uuidShort()
	}

	// Масса: усечённое логнормальное (медиана 1 MJ = 317.8 M⊕, σ = 0.9 декады,
	// диапазон 15.9–4131, пересэмплинг вместо клампа — gas_giant_physics.go).
	mass := g.gasGiantMass()

	// Размер — кривая с насыщением (НЕ (M/ρ)^(1/3)); плотность и гравитация —
	// производные: ρ = M/R³, g = M/R².
	size := GasGiantRadius(mass)
	density := mass / (size * size * size)

	atmospheres := []string{"водородно-гелиевая", "водородная", "гелиевая"}
	atmosphere := atmospheres[g.rng.Intn(len(atmospheres))]

	// --- ФИЗИЧЕСКАЯ ТЕМПЕРАТУРА ---
	// Равновесная от звезды + внутренний нагрев от сжатия (не от ядра).
	luminosity := luminosityBySpectral(spectralClass)
	orbitRadius := orbitRadiusByIndex(orbitIndex)
	tEq := computeEquilibriumTemp(luminosity, orbitRadius)

	greenhouse := computeGreenhouse(atmosphere)
	temp := tEq*greenhouse + 30

	if temp > TempAbsoluteMax {
		temp = TempAbsoluteMax
	}
	if temp < TempAbsoluteMin {
		temp = TempAbsoluteMin
	}

	// --- ЯДРО ---
	emptySubterrain := Composition{}
	core := GenerateCore(mass, "жаркий", emptySubterrain, systemAge, g.rng)

	// --- СПУТНИКИ ---
	satelliteCount := 3 + g.rng.Intn(8)
	satellites := g.generateSatellites(satelliteCount, worldName, size, temp, spectralClass)

	satellitesJSON := make([]map[string]interface{}, 0, len(satellites))
	for _, sat := range satellites {
		satellitesJSON = append(satellitesJSON, satelliteToMap(sat))
	}

	// --- РЕСУРСЫ: 1 из атмосферы (газ или топливо) ---
	planetID := uuid.New().String()

	resources := resource.GenerateGasGiantResource(planetID, g.rng)
	resourceSummary := resource.Summary(resources)

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         TypeGasGiant,
		OrbitIndex:   orbitIndex,
		Atmosphere:   atmosphere,
		Hydrosphere:  "сухая",
		Temperature:  temp,
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
		"temperature":       temp,
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
		"orbit_center":      "main",
		"orbit_radius_au":   orbitRadiusByIndex(orbitIndex),
		"description":       GenerateDescription(descCtx),
	}
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

// satelliteToMap — превращает Satellite в map для JSON-сериализации.
func satelliteToMap(s *Satellite) map[string]interface{} {
	return map[string]interface{}{
		"id":                     s.ID,
		"name":                   s.Name,
		"orbit_index":            s.OrbitIndex,
		"size":                   s.Size,
		"mass":                   s.Mass,
		"gravity":                computeGravity(s.Mass, s.Size),
		"temperature":            s.Temperature,
		"water_percent":          s.WaterPercent,
		"habitable":              s.Habitable,
		"life":                   s.Life,
		"atmosphere":             s.Atmosphere,
		"biosphere":              s.Biosphere,
		"surface_composition":    composeToJSON(s.SurfaceComposition),
		"subterrain_composition": composeToJSON(s.SubterrainComposition),
		"surface_dominant":       s.SurfaceComposition.DominantForm(),
		"description":            s.Description,
	}
}
