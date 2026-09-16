// internal/handlers/admin_regenerate_planets.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/lib/pq"
	"zorion/internal/generator"
	"zorion/internal/generator/planet"
	"zorion/internal/regionprofile"
)

// RegeneratePlanets — POST /admin/regenerate-planets (99.2.3 §5).
//
// Семантика: НЕ создаёт новых миров. Удаляет планеты выбранных миров и
// генерирует заново по равномерному счёту из полей формы (0–8): для обычных
// звёзд — общий путь generatePlanet, для экзотики — generateExoticPlanet
// (99.2.4 §5.3). Средние из «Звёзд» НЕ применяются — это ручной пересчёт,
// инструмент-«лекарство», не второй генератор.
//
// Живой прогон пишет в planets — без параллельных с генерацией вселенной
// (AGENTS.md §23: проверка статуса перед стартом, TryStart).
func (h *AdminHandlers) RegeneratePlanets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MinPlanets    int  `json:"min_planets"`
		MaxPlanets    int  `json:"max_planets"`
		IncludeNormal bool `json:"include_normal"` // обычные (одиночные) звёзды
		IncludeBinary bool `json:"include_binary"` // двойные/кратные
		IncludeExotic bool `json:"include_exotic"` // экзотика (остатки, протозвёзды)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.MinPlanets < 0 || req.MaxPlanets > 8 || req.MinPlanets > req.MaxPlanets {
		http.Error(w, "min_planets и max_planets: 0 ≤ min ≤ max ≤ 8", http.StatusBadRequest)
		return
	}
	if !req.IncludeNormal && !req.IncludeBinary && !req.IncludeExotic {
		http.Error(w, "Нужно выбрать хотя бы одну категорию миров", http.StatusBadRequest)
		return
	}

	// Взаимная блокировка с генерацией вселенной/планет (AGENTS.md §23,
	// 99.2.3 §5: «без параллельных с генерацией вселенной»). Проверка до
	// выборки миров — fail fast, без лишней нагрузки на БД.
	if statusManager.IsRunning(generator.JobGenerateUniverse) ||
		statusManager.IsRunning(generator.JobGeneratePlanets) {
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	worlds, err := h.worldRepo.GetAll()
	if err != nil {
		http.Error(w, "Failed to fetch worlds: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Раса-дома региона (99.2.22 §2.1): перегенерация — штатный инструмент
	// применения подкрутки к старым вселенным (спека §8 «Бэкфилл — нет;
	// перегенерация POST /admin/regenerate-planets»). Привязка — та же
	// NearestRegionIndex, что в GeneratePlanets (консистентно с profile).
	regions, err := h.loadRegionsWithProfiles()
	if err != nil {
		http.Error(w, "Failed to load regions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	selected := make([]planet.WorldInfo, 0, len(worlds))
	for _, w := range worlds {
		cat := worldCategory(w.StarType, w.SystemType)
		if !req.IncludeNormal && cat == "normal" {
			continue
		}
		if !req.IncludeBinary && cat == "binary" {
			continue
		}
		if !req.IncludeExotic && cat == "exotic" {
			continue
		}
		wi := planet.WorldInfo{
			ID:            w.ID,
			Name:          w.Name,
			SpectralClass: w.SpectralClass,
			Temperature:   w.Temperature,
			StarType:      w.StarType,
			SystemType:    w.SystemType,
			Mods:          w.StellarMods,
			Age:           w.Age,
			StellarMass:   w.StellarMass,
		}
		if idx := regionprofile.NearestRegionIndex(w.CoordX, w.CoordY, regions); idx >= 0 {
			wi.RaceID = regions[idx].RaceID
		}
		selected = append(selected, wi)
	}
	if len(selected) == 0 {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"regenerated","worlds":0,"planets":0}`))
		return
	}

	_, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobRegeneratePlanets, len(selected), cancel) {
		cancel()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ RegeneratePlanets panic: %v", rec)
				statusManager.Fail(generator.JobRegeneratePlanets, "panic: "+recoverErr(rec))
			}
		}()

		// Удаляем планеты выбранных миров (поселения — каскадно).
		oldCount, err := h.clearPlanetsOf(selected)
		if err != nil {
			log.Printf("❌ RegeneratePlanets: delete old planets: %v", err)
			statusManager.Fail(generator.JobRegeneratePlanets, err.Error())
			return
		}
		log.Printf("🗑️ RegeneratePlanets: удалено старых планет: %d", oldCount)

		planetGen := planet.NewGenerator(h.db, 0)
		// Мягкость подкрутки под расу-дома (99.2.22 §4.3): слой 2 вероятностный —
		// физические ручки. Множитель числа планет не применяется (пересчёт —
		// равномерный счёт, не mean-модель).
		if loaded, err := h.loadGenerationConfig(); err == nil {
			planetGen.SetRaceTuning(loaded.RaceTuningSoftness, 1.0)
		}
		totalPlanets, err := planetGen.RegeneratePlanetsForWorlds(selected, req.MinPlanets, req.MaxPlanets, 500, func(processed int) {
			statusManager.Progress(generator.JobRegeneratePlanets, processed)
		})
		if err != nil {
			log.Printf("❌ RegeneratePlanets: %v", err)
			statusManager.Fail(generator.JobRegeneratePlanets, err.Error())
			return
		}

		log.Printf("✅ RegeneratePlanets: %d миров, %d планет", len(selected), totalPlanets)
		statusManager.SetReport(generator.JobRegeneratePlanets,
			fmt.Sprintf("пересчитано миров: %d, планет: %d", len(selected), totalPlanets))
		statusManager.Done(generator.JobRegeneratePlanets)
		h.recomputePlanetStats()
		h.mapCache.LoadAsync(h.db)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// worldCategory — категория мира для фильтра пересчёта (99.2.3 §5).
func worldCategory(starType, systemType string) string {
	switch starType {
	case "white_dwarf", "neutron", "black_hole", "protostar":
		return "exotic"
	}
	if systemType == "binary" || systemType == "multiple" {
		return "binary"
	}
	return "normal"
}

// clearPlanetsOf — удаляет планеты выбранных миров и возвращает число удалённых.
// Дочерние записи (поселения, ресурсы) удаляются каскадно (ON DELETE CASCADE).
// ВАЖНО: world_id = ANY($1) требует pq.Array — []string lib/pq не конвертирует
// ("unsupported type []string", баг #2, прогон @tester); паттерн — как
// pqStringArray в economy_repository.go.
func (h *AdminHandlers) clearPlanetsOf(worlds []planet.WorldInfo) (int, error) {
	ids := pq.Array(worldIDs(worlds))
	var oldCount int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM planets WHERE world_id = ANY($1)`, ids).Scan(&oldCount); err != nil {
		return 0, err
	}
	if _, err := h.db.Exec(`DELETE FROM planets WHERE world_id = ANY($1)`, ids); err != nil {
		return 0, err
	}
	return oldCount, nil
}

// worldIDs — id миров из WorldInfo.
func worldIDs(worlds []planet.WorldInfo) []string {
	ids := make([]string, 0, len(worlds))
	for _, w := range worlds {
		ids = append(ids, w.ID)
	}
	return ids
}