// internal/generator/planet/planet_data_gas.go
package planet

import (
	"encoding/json"

	"github.com/google/uuid"
	"zorion/internal/names"
	"zorion/internal/resource"
)

// generateGasGiant — газовый гигант (99.2.20 §3.6 «Гиганты»). У него нет
// композиции поверхности (только атмосфера), но есть спутники — каждый
// полноценная локация.
//
// Кривая M→R, распределение масс, ρ = M/R³, g = M/R² — без изменений
// (99.2.15, эталон создателя). Температура — T⁴ + Кельвина–Гельмгольца
// (F_KH): чинит известную особенность «гигант 13 MJ → 20 K» (Юпитер ≈ 118 K).
// Старая формула tEq × greenhouse + 30 отброшена.
func (g *Generator) generateGasGiant(
	worldID string,
	worldName string,
	orbitIndex int,
	sp StellarParams,
) *PlanetData {
	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Газовый гигант-" + uuidShort()
	}

	orbitRadius := orbitRadiusScaled(orbitIndex, sp.Luminosity)
	res := g.runCascadeGiant(cascadeInput{
		Luminosity:    sp.Luminosity,
		StellarMass:   sp.StellarMass,
		AgeGyr:        sp.AgeGyr,
		Metallicity:   sp.Metallicity,
		TEff:          sp.TEff,
		OrbitRadiusAU: orbitRadius,
		OrbitIndex:    orbitIndex,
	})

	// --- СПУТНИКИ ---
	satelliteCount := 3 + g.rng.Intn(8)
	satellites := g.generateSatellites(satelliteCount, worldName, res.Size, res.TFinal, sp.SpectralClass)

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
		Atmosphere:   res.AtmosphereLabel,
		Hydrosphere:  "сухая",
		Temperature:  res.TFinal,
		WaterPercent: 0.0,
		Mass:         res.Mass,
		Density:      res.Density,
		Moons:        satelliteCount,
		Life:         false,
		Surface:      nil,
		Core:         res.Core,
		IsGasGiant:   true,
	}

	data := map[string]interface{}{
		"size":              res.Size,
		"mass":              res.Mass,
		"density":           res.Density,
		"gravity":           res.Gravity,
		"atmosphere":        res.AtmosphereLabel,
		"atmosphere_data":   atmosphereDataToJSON(res.AtmosphereData),
		"hydrosphere":       "сухая",
		"biosphere":         "стерильная",
		"temperature":       res.TFinal,
		"water_percent":     0.0,
		"life":              false,
		"political_system":  "нет",
		"moons":             satelliteCount,
		"development_level": 0.0,
		"archetype":         "жаркий",
		"system_age":        sp.AgeGyr,
		"is_gas_giant":      true,
		"resources":         resourceSummary,
		"satellites":        satellitesJSON,
		"surface_dominant":  "газовый_гигант",
		"type":              TypeGasGiant,
		"core":              coreToJSON(res.Core),
		"orbit_center":      "main",
		"orbit_radius_au":   orbitRadius,

		// Новые поля каскада (99.2.20 §4.1).
		"orbital_period":  res.OrbitalPeriod,
		"eccentricity":    res.Eccentricity,
		"escape_velocity": res.EscapeVelocity,
		"tidal_lock":      res.TidalLock,

		"description": GenerateDescription(descCtx),
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