// internal/generator/planet/planet_data_minineptune.go
package planet

import (
	"encoding/json"

	"github.com/google/uuid"
	"zorion/internal/names"
	"zorion/internal/resource"
)

// generateMiniNeptune — мини-нептун (спека
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны §7.1/§8): класс
// (M_crit, 16] M⊕, оболочка из газового резервуара системы (M_gas), радиус по
// кривой мини-нептуна, пустые биомы/недра, life/settleable — false.
//
// mass — готовая M_body из пред-слоя перелива; coreMass — твёрдое ядро (срез
// твёрдого бюджета M_диск, идёт в сумму applySumClamp, §7.2).
// tune — подкрутка расы-дома (99.2.22 §3.3): сдвиг орбиты + возраст.
func (g *Generator) generateMiniNeptune(
	worldID, worldName string,
	orbitIndex int,
	sp StellarParams,
	tune *raceTune,
	mass, coreMass float64,
) *PlanetData {
	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Мини-нептун-" + uuidShort()
	}

	orbitRadius := orbitRadiusScaled(orbitIndex, sp.Luminosity)
	if tune != nil && tune.orbitMult > 0 {
		orbitRadius *= tune.orbitMult
	}
	res := g.runCascadeMiniNeptune(cascadeInput{
		Luminosity:    sp.Luminosity,
		StellarMass:   sp.StellarMass,
		AgeGyr:        sp.AgeGyr,
		Metallicity:   sp.Metallicity,
		TEff:          sp.TEff,
		OrbitRadiusAU: orbitRadius,
		OrbitIndex:    orbitIndex,
		MassOverride:  mass,
	})

	// --- СПУТНИКИ: 0–4 (§8.3) ---
	moons := g.minineptuneMoons()
	satellites := g.generateSatellites(moons, worldName, res.Size, res.TFinal, sp.SpectralClass)
	satellitesJSON := make([]map[string]interface{}, 0, len(satellites))
	for _, sat := range satellites {
		satellitesJSON = append(satellitesJSON, satelliteToMap(sat))
	}

	// --- РЕСУРСЫ: H/He-оболочка (тот же источник, что у гиганта) ---
	planetID := uuid.New().String()
	resources := resource.GenerateGasGiantResource(planetID, g.rng)
	resourceSummary := resource.Summary(resources)

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         TypeMiniNeptune,
		OrbitIndex:   orbitIndex,
		Atmosphere:   res.AtmosphereLabel,
		Hydrosphere:  "сухая",
		Temperature:  res.TFinal,
		WaterPercent: 0.0,
		Mass:         res.Mass,
		Density:      res.Density,
		Moons:        moons,
		Life:         false,
		Settleable:   res.Settleable,
		Surface:      nil,
		Core:         res.Core,
		IsGasGiant:   false,
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
		"moons":             moons,
		"development_level": 0.0,
		"archetype":         "жаркий",
		"system_age":        sp.AgeGyr,
		"is_gas_giant":      false,
		"is_mini_neptune":   true,
		"resources":         resourceSummary,
		"satellites":        satellitesJSON,
		"surface_dominant":  "мини-нептун",
		"type":              TypeMiniNeptune,
		"core":              coreToJSON(res.Core),
		"orbit_center":      "main",
		"orbit_radius_au":   orbitRadius,

		// Поверхностей/недр нет (как у гиганта): пустые массивы — норма класса.
		"biomes":     []map[string]interface{}{},
		"subterrain": []map[string]interface{}{},

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
		// Mass — твёрдое ядро (срез M_диск): оболочка из M_gas в твёрдую
		// сумму не входит (§7.2, applySumClamp).
		Mass: coreMass,
	}
}
