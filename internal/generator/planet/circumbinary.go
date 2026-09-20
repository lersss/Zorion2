// internal/generator/planet/circumbinary.go
package planet

import (
	"encoding/json"

	"github.com/google/uuid"
	"zorion/internal/names"
	"zorion/internal/resource"
)

// P-ветка циркумбинарных планет (35b §4.1). Отклонение от 99.2.4 §5.3
// (close уходит с общего пути на явную ветку), одобрено создателем
// 2026-09-15 (вариант A, решения №1 и №7 задания).
//
// Орбита: единственная P-орбита на систему, r_P = 3×companion_sep_au
// (парный пол, правило стабильности P-типа 3×). Планеты идут через тот же
// физический каскад (99.2.20) с L_total = L₁+L₂ (инсоляция от суммы
// светимостей — сохраняется, 99.2.18) и r_P без масштаба √L. Полосы
// circumbinaryBandForTemp заменены единым каскадом (полоса по T — как у
// S-планет). Справочные температуры P-планет из 99.2.18 §5.1 — исторические
// (старая таблица светимостей и старая модель T); новые значения даёт каскад.

// generateCircumbinaryPlanet — P-планета тесной двойной (35b §4.1).
// Требует CompanionSepAU (новые миры); старые close-миры без sep идут
// общим путём как S-тип (фолбэк §2.4 — см. generateWorldWithCountIntoBuffer).
func (g *Generator) generateCircumbinaryPlanet(w WorldInfo) *PlanetData {
	a := *w.Mods.CompanionSepAU
	rP := 3 * a
	lTotal := luminosityBySpectral(w.SpectralClass) + luminosityBySpectral(w.Mods.Companion)

	sp := stellarParamsFromWorld(w, g.rng)
	sp.Luminosity = lTotal
	// Период P-планеты — по суммарной массе пары (Кеплер III).
	if w.Mods.CompanionMass != nil && *w.Mods.CompanionMass > 0 {
		sp.StellarMass += *w.Mods.CompanionMass
	} else {
		sp.StellarMass += fallbackStellarMass(w.Mods.Companion)
	}

	// Подкрутка расы-дома (99.2.22 §3.3): P-планеты — ручки 2–4, 7 (тот же
	// каскад); ручка 1 (сдвиг орбиты) — НЕТ: r_P = 3·companion_sep_au
	// фиксирован разделением пары (99.2.18), сдвиг сломал бы семантику
	// циркумбинарной орбиты. Ролл — в фиксированной позиции (до каскада).
	tune := g.raceTunePlanet(sp, rP, false)
	if tune != nil && tune.ageGyr > 0 {
		sp.AgeGyr = tune.ageGyr
	}

	// Гиганты P-типа разрешены: шанс от спектра главной, орбита — та же 3a.
	if g.rng.Float64() < g.gasGiantChanceShifted(w.SpectralClass) {
		return g.generateCircumbinaryGiant(w, rP, sp, tune)
	}
	return g.generateCircumbinaryRocky(w, rP, sp, tune)
}

// generateCircumbinaryRocky — каменистая P-планета через физический каскад
// (99.2.20): L_total, r_P = 3a, полоса по T — как у S-планет.
// tune — подкрутка расы-дома (99.2.22): ручки 2–4, 7; nil — без подкрутки.
func (g *Generator) generateCircumbinaryRocky(w WorldInfo, rP float64, sp StellarParams, tune *raceTune) *PlanetData {
	in := cascadeInput{
		Luminosity:    sp.Luminosity,
		StellarMass:   sp.StellarMass,
		AgeGyr:        sp.AgeGyr,
		Metallicity:   sp.Metallicity,
		TEff:          sp.TEff,
		OrbitRadiusAU: rP,
		OrbitIndex:    1,
	}
	if tune != nil {
		in.FVolOverride = tune.fVol
		in.SurfaceOverride = tune.surface
		in.CompositionOverride = tune.composition
		in.CompositionRegime = tune.compositionRegime
	}
	res := g.runCascade(in)

	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Планета-" + uuidShort()
	}

	dominant := res.Surface.DominantForm()
	if dominant == "" {
		dominant = SurfaceRocks
	}

	radioactive := res.Core.IsRadioactive()
	gdType := ClassifyGameDesignType(PlanetClassificationInput{
		IsGasGiant:    false,
		IsRadioactive: radioactive,
		Surface:       res.Surface,
		Temperature:   res.TFinal,
		WaterPercent:  res.WaterPercent,
		Settleable:    res.Settleable,
		Life:          res.Life,
	})

	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         gdType,
		OrbitIndex:   1,
		Atmosphere:   res.AtmosphereLabel,
		Hydrosphere:  res.Hydrosphere,
		Temperature:  res.TFinal,
		WaterPercent: res.WaterPercent,
		Mass:         res.Mass,
		Density:      res.Density,
		Moons:        res.Moons,
		Life:         res.Life,
		Settleable:   res.Settleable,
		Surface:      res.Surface,
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
		"hydrosphere":       res.Hydrosphere,
		"biosphere":         res.Biosphere,
		"temperature":       res.TFinal,
		"water_percent":     res.WaterPercent,
		"life":              res.Life,
		"liquid_water_possible": res.LiquidWater,
		"political_system":  res.Political,
		"moons":             res.Moons,
		"development_level": res.Development,
		"archetype":         res.ArchetypeBand,
		"system_age":        sp.AgeGyr,

		"surface_composition":    composeToJSON(res.Surface),
		"subterrain_composition": composeToJSON(res.Subterrain),
		"surface_dominant":       dominant,
		"type":                   gdType,
		"radioactive":            radioactive,
		"core":                   coreToJSON(res.Core),

		// Биомы и зоны недр объектами (99.2.28 §9): финальная поверхность/недра.
		"biomes":     biomesToJSON(res.Biomes),
		"subterrain": zonesToJSON(res.SubterrainZones),

		// Орбитальный контекст P-планеты (35b §2.2): вокруг барицентра пары.
		"orbit_center":    "barycenter",
		"orbit_radius_au": rP,
		"circumbinary":    true,

		// Новые поля каскада (99.2.20 §4.1).
		"orbital_period":  res.OrbitalPeriod,
		"eccentricity":    res.Eccentricity,
		"escape_velocity": res.EscapeVelocity,
		"tidal_lock":      res.TidalLock,

		"description": GenerateDescription(descCtx),
	}

	resources := attachResources(
		data, planetID, dominant,
		map[string]float64(res.Subterrain),
		w.SpectralClass, g.rng, g.resourceBias(),
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
// распределению гигантов, температура от T⁴ + F_KH (99.2.20 §3.6).
// tune — подкрутка расы-дома (99.2.22): возраст (ручка 4); nil — без.
func (g *Generator) generateCircumbinaryGiant(w WorldInfo, rP float64, sp StellarParams, tune *raceTune) *PlanetData {
	res := g.runCascadeGiant(cascadeInput{
		Luminosity:    sp.Luminosity,
		StellarMass:   sp.StellarMass,
		AgeGyr:        sp.AgeGyr,
		Metallicity:   sp.Metallicity,
		TEff:          sp.TEff,
		OrbitRadiusAU: rP,
		OrbitIndex:    1,
	})

	satelliteCount := 3 + g.rng.Intn(8)
	satellites := g.generateSatellites(satelliteCount, w.Name, res.Size, res.TFinal, w.SpectralClass)

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
		Atmosphere:   res.AtmosphereLabel,
		Hydrosphere:  "сухая",
		Temperature:  res.TFinal,
		WaterPercent: 0.0,
		Mass:         res.Mass,
		Density:      res.Density,
		Moons:        satelliteCount,
		Life:         false,
		Settleable:   res.Settleable,
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

		// Орбитальный контекст P-планеты (35b §2.2).
		"orbit_center":    "barycenter",
		"orbit_radius_au": rP,
		"circumbinary":    true,

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
		WorldID:    w.ID,
		Name:       name,
		OrbitIndex: 1,
		Data:       dataJSON,
		Resources:  resources,
	}
}