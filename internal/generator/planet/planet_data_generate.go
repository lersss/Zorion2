// internal/generator/planet/planet_data_generate.go
package planet

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/google/uuid"
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

// ==================== ПАРАМЕТРЫ ЗВЕЗДЫ (99.2.20 §3.1) ====================

// StellarParams — параметры звезды для каскада (слой 1).
type StellarParams struct {
	SpectralClass string
	Luminosity    float64 // L☉
	StellarMass   float64 // M☉
	AgeGyr        float64 // млрд лет
	Metallicity   float64 // [Fe/H]
	TEff          float64 // K
}

// stellarParamsFromWorld — параметры звезды из WorldInfo с фолбэками:
// масса — worlds.stellar_mass (фолбэк серединой диапазона класса),
// возраст — worlds.age (фолбэк determineSystemAge), металличность —
// StellarMods.Metallicity (фолбэк 0 — солнечная), T_eff — worlds.temperature.
func stellarParamsFromWorld(w WorldInfo, rng *rand.Rand) StellarParams {
	sp := StellarParams{
		SpectralClass: w.SpectralClass,
		Luminosity:    luminosityBySpectral(w.SpectralClass),
		TEff:          float64(w.Temperature),
	}
	if w.StellarMass != nil && *w.StellarMass > 0 {
		sp.StellarMass = *w.StellarMass
	} else {
		sp.StellarMass = fallbackStellarMass(w.SpectralClass)
	}
	if w.Age != nil && *w.Age > 0 {
		sp.AgeGyr = *w.Age
	} else {
		sp.AgeGyr = determineSystemAge(w.SpectralClass, rng)
	}
	if w.Mods != nil && w.Mods.Metallicity != nil {
		sp.Metallicity = *w.Mods.Metallicity
	}
	return sp
}

// stellarParamsFromClass — параметры звезды по классу (старые пути без
// WorldInfo: GeneratePlanetsForWorld, прототип): все фолбэки.
func stellarParamsFromClass(spectralClass string, temperature int, rng *rand.Rand) StellarParams {
	return StellarParams{
		SpectralClass: spectralClass,
		Luminosity:    luminosityBySpectral(spectralClass),
		StellarMass:   fallbackStellarMass(spectralClass),
		AgeGyr:        determineSystemAge(spectralClass, rng),
		Metallicity:   0,
		TEff:          float64(temperature),
	}
}

// ==================== ОБЫЧНАЯ ПЛАНЕТА ====================

// generatePlanet — планета обычной звезды (99.2.20): газовый гигант на
// дальней орбите (существующий триггер) или физический каскад. Подветки
// океанических/радиоактивных растворены в каскаде (типы возникают из
// физики: гидросфера «океаны» + вода > 60; core.radioactivity > 50).
func (g *Generator) generatePlanet(worldID, worldName string, orbitIndex int, sp StellarParams) *PlanetData {
	// --- ГАЗОВЫЙ ГИГАНТ ---
	if orbitIndex >= 3 {
		if g.rng.Float64() < gasGiantChance(sp.SpectralClass) {
			return g.generateGasGiant(worldID, worldName, orbitIndex, sp)
		}
	}

	// --- СТАНДАРТНАЯ ГЕНЕРАЦИЯ ЧЕРЕЗ ФИЗИЧЕСКИЙ КАСКАД ---
	return g.generateStandardPlanet(worldID, worldName, orbitIndex, sp, false)
}

// generateStandardPlanet — планета по физическому каскаду (стандартный путь).
// forceLife — форсировать жизнь в каскаде (прототип поселения §6.9:
// жизненный проход даёт азотно-кислородную атмосферу, консистентную
// с контрактом инструмента).
func (g *Generator) generateStandardPlanet(
	worldID, worldName string,
	orbitIndex int,
	sp StellarParams,
	forceLife bool,
) *PlanetData {
	orbitRadius := orbitRadiusScaled(orbitIndex, sp.Luminosity)
	res := g.runCascade(cascadeInput{
		Luminosity:    sp.Luminosity,
		StellarMass:   sp.StellarMass,
		AgeGyr:        sp.AgeGyr,
		Metallicity:   sp.Metallicity,
		TEff:          sp.TEff,
		OrbitRadiusAU: orbitRadius,
		OrbitIndex:    orbitIndex,
		ForceLife:     forceLife,
	})

	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Планета-" + uuidShort()
	}

	dominant := res.Surface.DominantForm()
	if dominant == "" {
		dominant = SurfaceRocks
	}

	radioactive := res.Core.IsRadioactive()

	// Геймдизайнерский тип по композиции (существующий механизм; типы
	// «океаническая»/«радиоактивная» возникают из физики каскада).
	gdType := ClassifyGameDesignType(PlanetClassificationInput{
		IsGasGiant:    false,
		IsRadioactive: radioactive,
		Surface:       res.Surface,
		Temperature:   res.TFinal,
		WaterPercent:  res.WaterPercent,
		Settleable:    res.Settleable,
		Life:          res.Life,
	})

	// UUID генерируется ЗАРАНЕЕ — нужен для детерминированного выбора описания.
	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         gdType,
		OrbitIndex:   orbitIndex,
		Atmosphere:   res.AtmosphereLabel,
		Hydrosphere:  res.Hydrosphere,
		Temperature:  res.TFinal,
		WaterPercent: res.WaterPercent,
		Mass:         res.Mass,
		Density:      res.Density,
		Moons:        res.Moons,
		Life:         res.Life,
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

		// Орбитальный контекст S-планеты (35b §2.2): вокруг главной.
		"orbit_center":    "main",
		"orbit_radius_au": orbitRadius,

		// Новые поля каскада (99.2.20 §4.1).
		"orbital_period":  res.OrbitalPeriod,
		"eccentricity":    res.Eccentricity,
		"escape_velocity": res.EscapeVelocity,
		"tidal_lock":      res.TidalLock,

		"surface_composition":    composeToJSON(res.Surface),
		"subterrain_composition": composeToJSON(res.Subterrain),
		"surface_dominant":       dominant,
		"type":                   gdType,
		"radioactive":            radioactive,
		"core":                   coreToJSON(res.Core),

		"description": GenerateDescription(descCtx),
	}

	// Ресурсы генерируются здесь же: summary попадает в JSON планеты
	// (data["resources"]), сами ресурсы живут только в памяти.
	resources := attachResources(
		data, planetID, dominant,
		map[string]float64(res.Subterrain),
		sp.SpectralClass, g.rng,
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

// GeneratePrototypePlanet — землеподобная планета для прототипа поселения.
// Контракт инструмента (99.2.20 §6.9): полоса «умеренный», T = 288 K,
// флаг true, вода 80%, жизнь true — явные оверрайды после каскада
// (прототипу разрешено форсировать физику; без форсирования орбита 1
// G-звезды дала бы T_final ≈ 365 K — вне пресета [200, 350]).
// forceLife=true в каскаде: жизненный проход даёт азотно-кислородную
// атмосферу (atmosphere_data консистентен с life=true).
// Население задаётся отдельной вставкой поселения в admin_universe.go.
func (g *Generator) GeneratePrototypePlanet(worldID, worldName, spectralClass string) *PlanetData {
	sp := stellarParamsFromClass(spectralClass, 0, g.rng)
	pd := g.generateStandardPlanet(worldID, worldName, 1, sp, true)

	var data map[string]interface{}
	if err := json.Unmarshal(pd.Data, &data); err != nil {
		return pd
	}
	data["archetype"] = "умеренный"
	data["temperature"] = 288.0
	data["liquid_water_possible"] = true
	data["water_percent"] = 80.0
	data["life"] = true
	data["political_system"] = "демократия"
	data["development_level"] = 0.5
	dataJSON, _ := json.Marshal(data)
	pd.Data = dataJSON
	return pd
}

// ==================== УТИЛИТЫ ====================

// coreToJSON — сериализует ядро для JSON-поля (включая внутренний поток
// heat_flux_w_m2, 99.2.20 §4.2).
func coreToJSON(c Core) map[string]interface{} {
	return map[string]interface{}{
		"type":           c.Type,
		"mass_percent":   c.MassPercent,
		"activity":       c.Activity,
		"radioactivity":  c.Radioactivity,
		"age":            c.Age,
		"is_active":      c.IsActive(),
		"is_metallic":    c.IsMetallic(),
		"heat_flux_w_m2": c.HeatFluxWm2,
	}
}

// atmosphereDataToJSON — сериализует атмосферу-объект (99.2.20 §4.1).
func atmosphereDataToJSON(a AtmosphereData) map[string]interface{} {
	return map[string]interface{}{
		"composition":            a.Composition,
		"pressure_atm":           a.PressureAtm,
		"mass_earth_atm":         a.MassEarthAtm,
		"tau_ir":                 a.TauIR,
		"scale_height_km":        a.ScaleHeightKm,
		"mean_molecular_weight":  a.MeanMolecularWeight,
	}
}