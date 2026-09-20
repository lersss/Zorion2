// internal/handlers/admin_race_settlements.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"zorion/internal/generator"
	"zorion/internal/generator/settlement"
	"zorion/internal/races"
)

// GenerateRaceSettlements — массовая генерация поселений рас (спека 99.2.21
// §7, идея 56a): доминанта кластера + подселение соседней расы на выбросах.
// Отдельный проход от человеческого GenerateSettlements (скрыт 65a):
// удаляет только поселения рас (race_id IS NOT NULL), человеческие не трогает.
//
// Тело (JSON): {"neighbor_chance": 0.3, "chance": 1.0, "population": {...}} —
// крутилка подселения соседа (0–1), шанс заселения доминанты (0–1, 65a),
// стратегия населения (fixed/random, 65a). Пустое тело — дефолты из пресета
// расового генератора (config/race_settlement.json).
func (h *AdminHandlers) GenerateRaceSettlements(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NeighborChance *float64             `json:"neighbor_chance"`
		Chance         *float64             `json:"chance"`
		Population     *settlement.Population `json:"population"`
	}
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "Bad JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	preset := settlement.CurrentRacePreset()
	cfg := settlement.RaceGenConfig{
		NeighborChance: preset.NeighborChance,
		Chance:         preset.Chance,
		Population:     preset.Population,
	}
	if req.NeighborChance != nil {
		cfg.NeighborChance = *req.NeighborChance
	}
	if req.Chance != nil {
		cfg.Chance = *req.Chance
	}
	if req.Population != nil {
		cfg.Population = *req.Population
	}
	if err := cfg.Validate(); err != nil {
		http.Error(w, "Bad config: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(races.Catalog()) == 0 {
		http.Error(w, "Каталог рас не загружен (config/races.json)", http.StatusInternalServerError)
		return
	}

	var total int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM planets`).Scan(&total); err != nil {
		log.Printf("❌ GenerateRaceSettlements: count error: %v", err)
		http.Error(w, "Failed to count planets", http.StatusInternalServerError)
		return
	}
	if total == 0 {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"race_settlements_generated","total":0}`))
		return
	}

	// Пакман ест миры (спека 2026-09-20 §2.2): поселения рас пишут в
	// settlements — генерация поверх пакмана не стартует.
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}

	// Контекст от фоновой задачи, НЕ от запроса (как GenerateSettlements).
	ctx, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobGenerateRaceSettlements, total, cancel) {
		cancel()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ GenerateRaceSettlements panic: %v", rec)
				statusManager.Fail(generator.JobGenerateRaceSettlements, "panic: "+recoverErr(rec))
			}
		}()
		select {
		case <-ctx.Done():
			statusManager.Cancel(generator.JobGenerateRaceSettlements)
			return
		default:
		}
		log.Printf("👽 GenerateRaceSettlements: start (neighbor_chance=%.2f, chance=%.2f)", cfg.NeighborChance, cfg.Chance)

		// Слой рас: удаляем только поселения рас, человеческие не трогаем.
		oldCount, err := h.clearRaceSettlementsLayer()
		if err != nil {
			log.Printf("❌ GenerateRaceSettlements: delete old records: %v", err)
			statusManager.Fail(generator.JobGenerateRaceSettlements, err.Error())
			return
		}
		log.Printf("🗑️ GenerateRaceSettlements: удалено старых поселений рас: %d", oldCount)

		gen := settlement.NewGenerator(h.db, 0)
		count, unsettled, err := gen.GenerateRaceSettlements(ctx, cfg, func(processed int) {
			statusManager.Progress(generator.JobGenerateRaceSettlements, processed)
		})
		if err != nil {
			log.Printf("❌ GenerateRaceSettlements: %v", err)
			statusManager.Fail(generator.JobGenerateRaceSettlements, err.Error())
			return
		}
		report := formatUnsettledRaces(unsettled)
		if report != "" {
			statusManager.SetReport(generator.JobGenerateRaceSettlements, report)
		}
		log.Printf("✅ GenerateRaceSettlements: %d поселений рас; %s", count, report)
		statusManager.Done(generator.JobGenerateRaceSettlements)
		h.recomputePlanetStats()
		h.mapCache.LoadAsync(h.db)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// clearRaceSettlementsLayer — удаляет поселения рас (race_id IS NOT NULL).
// Человеческие поселения (race_id NULL) не трогаются.
func (h *AdminHandlers) clearRaceSettlementsLayer() (int, error) {
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM settlements WHERE race_id IS NOT NULL`).Scan(&n); err != nil {
		return 0, err
	}
	if _, err := h.db.Exec(`DELETE FROM settlements WHERE race_id IS NOT NULL`); err != nil {
		return 0, err
	}
	return n, nil
}

// formatUnsettledRaces — список незаселившихся рас для отчёта генерации
// (копилка для разбора причин, идея 56a).
func formatUnsettledRaces(unsettled []string) string {
	if len(unsettled) == 0 {
		return ""
	}
	names := make([]string, 0, len(unsettled))
	for _, id := range unsettled {
		if r := races.ByID(id); r != nil {
			names = append(names, id+" ("+r.Name+")")
		} else {
			names = append(names, id)
		}
	}
	return fmt.Sprintf("Не заселились (%d из %d): %s", len(unsettled), len(races.Catalog()), strings.Join(names, ", "))
}