// internal/handlers/admin_settlements.go
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"zorion/internal/generator"
	"zorion/internal/generator/settlement"
)

// GenerateSettlements — массовая генерация поселений по пресету пригодности.
//
// Тело запроса (JSON) опционально переопределяет параметры пресета:
// {"минимальная_вода": 5, "шанс_заселения": 0.5}. Пустое тело — пресет
// из файла config/settlement_preset.json.
func (h *AdminHandlers) GenerateSettlements(w http.ResponseWriter, r *http.Request) {
	var overrides map[string]interface{}
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&overrides); err != nil && err != io.EOF {
			http.Error(w, "Bad JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Свежий пресет из файла (ручные правки подхватываются) + переопределения.
	if err := settlement.LoadPreset("config/settlement_preset.json"); err != nil {
		log.Printf("⚠️ settlement preset: %v, использую дефолты", err)
	}
	preset, err := settlement.ApplyOverrides(overrides)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var total int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM planets`).Scan(&total); err != nil {
		log.Printf("❌ GenerateSettlements: count error: %v", err)
		http.Error(w, "Failed to count planets", http.StatusInternalServerError)
		return
	}
	if total == 0 {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"settlements_generated","total":0}`))
		return
	}

	// Контекст от фоновой задачи, НЕ от запроса: контекст запроса
	// отменяется, когда handler возвращает ответ, и генерация мгновенно
	// «остановилась бы».
	ctx, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobGenerateSettlements, total, cancel) {
		cancel()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ GenerateSettlements panic: %v", rec)
				statusManager.Fail(generator.JobGenerateSettlements, "panic: "+recoverErr(rec))
			}
		}()
		select {
		case <-ctx.Done():
			statusManager.Cancel(generator.JobGenerateSettlements)
			return
		default:
		}
		log.Printf("🏘️ GenerateSettlements: start")
		gen := settlement.NewGenerator(h.db, 0)
		count, err := gen.GenerateSettlements(ctx, preset, func(processed int) {
			statusManager.Progress(generator.JobGenerateSettlements, processed)
		})
		if err != nil {
			log.Printf("❌ GenerateSettlements: %v", err)
			statusManager.Fail(generator.JobGenerateSettlements, err.Error())
			return
		}
		log.Printf("✅ GenerateSettlements: %d поселений", count)
		statusManager.Done(generator.JobGenerateSettlements)
		h.recomputePlanetStats()
		h.mapCache.LoadAsync(h.db)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}


