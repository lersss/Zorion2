// internal/handlers/admin_stats_aggregator.go
package handlers

import "zorion/internal/generator/planet"

// statsAggregator — агрегирует статистику по планетам.
type statsAggregator struct {
	worldsByID map[string]worldInfo
	worldsSeen map[string]bool

	totalSize       float64
	totalMass       float64
	totalTemp       float64
	totalWater      float64
	totalPopulation int64

	sizeCount  int
	massCount  int
	tempCount  int
	waterCount int
	popCount   int

	// Население по планете (SUM settlements.population), заполняется перед process.
	popByPlanet map[string]int64

	// Ядра
	totalCoreMassPercent   float64
	totalCoreActivity      float64
	totalCoreRadioactivity float64
	totalSystemAge         float64
	coreCount              int
}

// newStatsAggregator — создаёт агрегатор с индексом миров.
func newStatsAggregator(worlds []worldInfo) *statsAggregator {
	byID := make(map[string]worldInfo, len(worlds))
	for _, w := range worlds {
		byID[w.ID] = w
	}
	return &statsAggregator{
		worldsByID: byID,
		worldsSeen: make(map[string]bool),
	}
}

// process — проходит по всем планетам, обновляя stats.
func (a *statsAggregator) process(planets []planetRecord, stats *PlanetStats) {
	for _, p := range planets {
		a.worldsSeen[p.WorldID] = true
		a.processOne(p, stats)
	}
	stats.WorldsWithPlanets = len(a.worldsSeen)
}

// processOne — обработка одной планеты.
func (a *statsAggregator) processOne(p planetRecord, stats *PlanetStats) {
	dominant := getString(p.Data, "surface_dominant")
	if dominant == "" {
		dominant = getString(p.Data, "type")
	}
	if dominant == "" {
		dominant = planet.SurfaceRocks
	}
	stats.PlanetsByType[dominant]++

	// По спектральному классу — только обычные звёзды (star); экзотика
	// группируется по star_type отдельным блоком, в класс «G»/«по умолчанию»
	// не сливается (99.2.4 §7).
	if w, ok := a.worldsByID[p.WorldID]; ok {
		if w.StarType == "" || w.StarType == "star" {
			if w.SpectralClass != "" {
				if _, ok := stats.PlanetsBySpectral[w.SpectralClass]; !ok {
					stats.PlanetsBySpectral[w.SpectralClass] = make(map[string]int)
				}
				stats.PlanetsBySpectral[w.SpectralClass][dominant]++
			}
		} else {
			if _, ok := stats.PlanetsByStarType[w.StarType]; !ok {
				stats.PlanetsByStarType[w.StarType] = make(map[string]int)
			}
			stats.PlanetsByStarType[w.StarType][dominant]++
		}
	}

	// Гидросфера / атмосфера / биосфера
	incrementIfPresent(stats.HydrosphereCount, getString(p.Data, "hydrosphere"))
	incrementIfPresent(stats.AtmosphereCount, getString(p.Data, "atmosphere"))
	incrementIfPresent(stats.BiosphereCount, getString(p.Data, "biosphere"))

	// Композиция поверхности
	surface := extractComposition(p.Data, "surface_composition")
	for form, share := range surface {
		stats.SurfaceFormCounts[form]++
		stats.SurfaceFormShares[form] += share
	}

	// Композиция недр
	subterrain := extractComposition(p.Data, "subterrain_composition")
	for subType, share := range subterrain {
		stats.SubterrainCounts[subType]++
		stats.SubterrainShares[subType] += share
	}

	// Ядро
	a.accumulateCore(p.Data, stats)

	// Геймдизайнерский тип. Пригодность (обитаемость) — производная:
	// планета с поселением считается обитаемой.
	_, hasSettlement := a.popByPlanet[p.ID]
	in := buildClassificationInput(p.Data, surface, hasSettlement)
	gdType := planet.ClassifyGameDesignType(in)
	stats.GameDesignTypes[gdType]++

	// Счётчики
	if getBool(p.Data, "life") {
		stats.LifeCount++
	}
	if hasSettlement {
		stats.HabitableCount++
	}

	// Средние
	a.accumulateAverages(p.ID, p.Data)
}

// accumulateCore — обрабатывает поле core из JSON планеты.
func (a *statsAggregator) accumulateCore(data map[string]interface{}, stats *PlanetStats) {
	coreRaw, ok := data["core"].(map[string]interface{})
	if !ok {
		return
	}

	coreType := getString(coreRaw, "type")
	if coreType != "" {
		stats.CoreTypeCounts[coreType]++
	}

	if getBool(coreRaw, "is_active") {
		stats.ActiveCoreCount++
	}
	if getBool(coreRaw, "is_metallic") {
		stats.MetallicCoreCount++
	}

	radioactivity := getFloat(coreRaw, "radioactivity")
	if radioactivity > 50 {
		stats.RadioactiveCoreCount++
	}

	// Средние по ядру
	a.totalCoreMassPercent += getFloat(coreRaw, "mass_percent")
	a.totalCoreActivity += getFloat(coreRaw, "activity")
	a.totalCoreRadioactivity += radioactivity

	// Возраст системы — из планеты, а не из ядра (но совпадает)
	a.totalSystemAge += getFloat(data, "system_age")
	a.coreCount++
}

// accumulateAverages — накапливает суммы и счётчики для средних.
func (a *statsAggregator) accumulateAverages(planetID string, data map[string]interface{}) {
	if size, ok := data["size"].(float64); ok {
		a.totalSize += size
		a.sizeCount++
	}
	if mass, ok := data["mass"].(float64); ok {
		a.totalMass += mass
		a.massCount++
	}
	if temp, ok := data["temperature"].(float64); ok {
		a.totalTemp += temp
		a.tempCount++
	}
	if water, ok := data["water_percent"].(float64); ok {
		a.totalWater += water
		a.waterCount++
	}
	if pop, ok := a.popByPlanet[planetID]; ok {
		a.totalPopulation += pop
		a.popCount++
	}
}

// applyAverages — записывает средние значения в stats.
func (a *statsAggregator) applyAverages(stats *PlanetStats) {
	if a.sizeCount > 0 {
		stats.AvgSize = a.totalSize / float64(a.sizeCount)
	}
	if a.massCount > 0 {
		stats.AvgMass = a.totalMass / float64(a.massCount)
	}
	if a.tempCount > 0 {
		stats.AvgTemp = a.totalTemp / float64(a.tempCount)
	}
	if a.waterCount > 0 {
		stats.AvgWater = a.totalWater / float64(a.waterCount)
	}
	if a.popCount > 0 {
		stats.AvgPopulation = a.totalPopulation / int64(a.popCount)
	}
	if a.coreCount > 0 {
		n := float64(a.coreCount)
		stats.AvgCoreMassPercent = a.totalCoreMassPercent / n
		stats.AvgCoreActivity = a.totalCoreActivity / n
		stats.AvgCoreRadioactivity = a.totalCoreRadioactivity / n
		stats.AvgSystemAge = a.totalSystemAge / n
	}
}