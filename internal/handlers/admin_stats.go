// internal/handlers/admin_stats.go
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// PlanetStats — агрегированная статистика по планетам.
type PlanetStats struct {
	TotalWorlds       int                       `json:"total_worlds"`
	WorldsWithPlanets int                       `json:"worlds_with_planets"`
	TotalPlanets      int                       `json:"total_planets"`
	PlanetsByType     map[string]int            `json:"planets_by_type"`
	PlanetsBySpectral map[string]map[string]int `json:"planets_by_spectral"`
	GameDesignTypes   map[string]int            `json:"game_design_types"`

	// Поверхность
	SurfaceFormCounts map[string]int     `json:"surface_form_counts"`
	SurfaceFormShares map[string]float64 `json:"surface_form_shares"`
	SurfaceFormAvg    map[string]float64 `json:"surface_form_avg"`

	// Недра
	SubterrainCounts map[string]int     `json:"subterrain_counts"`
	SubterrainShares map[string]float64 `json:"subterrain_shares"`
	SubterrainAvg    map[string]float64 `json:"subterrain_avg"`

	// Ядра
	CoreTypeCounts        map[string]int `json:"core_type_counts"`
	ActiveCoreCount       int            `json:"active_core_count"`
	MetallicCoreCount     int            `json:"metallic_core_count"`
	RadioactiveCoreCount  int            `json:"radioactive_core_count"`
	AvgCoreMassPercent    float64        `json:"avg_core_mass_percent"`
	AvgCoreActivity       float64        `json:"avg_core_activity"`
	AvgCoreRadioactivity  float64        `json:"avg_core_radioactivity"`
	AvgSystemAge          float64        `json:"avg_system_age"`

	HydrosphereCount map[string]int `json:"hydrosphereCount"`
	AtmosphereCount  map[string]int `json:"atmosphereCount"`
	BiosphereCount   map[string]int `json:"biosphereCount"`

	HabitableCount int     `json:"habitable_count"`
	LifeCount      int     `json:"life_count"`
	AvgSize        float64 `json:"avg_size"`
	AvgMass        float64 `json:"avg_mass"`
	AvgTemp        float64 `json:"avg_temp"`
	AvgWater       float64 `json:"avg_water"`
	AvgPopulation  int64   `json:"avg_population"`

	Anomalies []Anomaly `json:"anomalies"`
}

// Anomaly — отклонение от ожидаемого распределения.
type Anomaly struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Expected    float64 `json:"expected"`
	Severity    string  `json:"severity"`
}

// GetPlanetStatsHandler — HTTP-обработчик статистики.
// Отдаёт закэшированный результат, считает только при пустом кэше.
// `?refresh=1` сбрасывает кэш и пересчитывает заново.
func (h *AdminHandlers) GetPlanetStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		h.invalidatePlanetStats()
	}
	stats, err := h.getPlanetStats()
	if err != nil {
		log.Printf("❌ Failed to calculate planet stats: %v", err)
		http.Error(w, "Failed to calculate stats", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// getPlanetStats — возвращает закэшированную статистику.
// При пустом кэше считает один раз под полным локом (single-flight)
// и кладёт результат в кэш.
func (h *AdminHandlers) getPlanetStats() (*PlanetStats, error) {
	h.planetStatsMu.RLock()
	cached := h.planetStats
	h.planetStatsMu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	h.planetStatsMu.Lock()
	defer h.planetStatsMu.Unlock()
	if h.planetStats != nil {
		return h.planetStats, nil
	}

	stats, err := h.calculatePlanetStats()
	if err != nil {
		return nil, err
	}
	h.planetStats = stats
	return stats, nil
}

// setPlanetStats — кладёт готовую статистику в кэш (после генерации).
func (h *AdminHandlers) setPlanetStats(stats *PlanetStats) {
	h.planetStatsMu.Lock()
	h.planetStats = stats
	h.planetStatsMu.Unlock()
}

// invalidatePlanetStats — сбрасывает кэш после изменения вселенной.
func (h *AdminHandlers) invalidatePlanetStats() {
	h.planetStatsMu.Lock()
	h.planetStats = nil
	h.planetStatsMu.Unlock()
}

// recomputePlanetStats — пересчитывает статистику и кладёт в кэш.
// Вызывается после успешной генерации. При ошибке кэш сбрасывается.
func (h *AdminHandlers) recomputePlanetStats() {
	stats, err := h.calculatePlanetStats()
	if err != nil {
		log.Printf("❌ Failed to recompute planet stats: %v", err)
		h.invalidatePlanetStats()
		return
	}
	h.setPlanetStats(stats)
}

// calculatePlanetStats — основной расчёт статистики.
func (h *AdminHandlers) calculatePlanetStats() (*PlanetStats, error) {
	stats := newPlanetStats()

	worlds, err := h.loadWorlds()
	if err != nil {
		return nil, err
	}
	stats.TotalWorlds = len(worlds)

	planets, err := h.loadPlanets()
	if err != nil {
		return nil, err
	}
	stats.TotalPlanets = len(planets)

	aggregator := newStatsAggregator(worlds)
	aggregator.process(planets, stats)
	aggregator.applyAverages(stats)
	applyFormAverages(stats)

	detectWorldAnomalies(stats)
	detectTypeAnomalies(stats)

	return stats, nil
}

// newPlanetStats — инициализация со всеми map-полями.
func newPlanetStats() *PlanetStats {
	return &PlanetStats{
		PlanetsByType:     make(map[string]int),
		PlanetsBySpectral: make(map[string]map[string]int),
		GameDesignTypes:   make(map[string]int),
		SurfaceFormCounts: make(map[string]int),
		SurfaceFormShares: make(map[string]float64),
		SurfaceFormAvg:    make(map[string]float64),
		SubterrainCounts:  make(map[string]int),
		SubterrainShares:  make(map[string]float64),
		SubterrainAvg:     make(map[string]float64),
		CoreTypeCounts:    make(map[string]int),
		HydrosphereCount:  make(map[string]int),
		AtmosphereCount:   make(map[string]int),
		BiosphereCount:    make(map[string]int),
		Anomalies:         []Anomaly{},
	}
}

// applyFormAverages — считает средний процент каждой формы по всем планетам.
func applyFormAverages(stats *PlanetStats) {
	if stats.TotalPlanets == 0 {
		return
	}
	total := float64(stats.TotalPlanets)

	for form, share := range stats.SurfaceFormShares {
		stats.SurfaceFormAvg[form] = share / total
	}
	for subType, share := range stats.SubterrainShares {
		stats.SubterrainAvg[subType] = share / total
	}
}