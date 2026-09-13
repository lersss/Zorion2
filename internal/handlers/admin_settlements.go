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

// SettlementFields — реестр полей planet.data для формы правил модели
// генерации поселений (типы, экстремумы, допустимые значения).
func (h *AdminHandlers) SettlementFields(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settlement.FieldRegistry())
}

// GenerateSettlements — массовая генерация поселений по модели генерации.
//
// Тело запроса (JSON) — объект Model:
// {"mode":"complex","chance":0.5,"population":{"kind":"random","min":100000,
//  "max":1000000000},"rules":[{"field":"temperature","min":200,"max":350}]}.
// Пустое тело — модель по умолчанию (DefaultModel).
func (h *AdminHandlers) GenerateSettlements(w http.ResponseWriter, r *http.Request) {
	model := settlement.DefaultModel()
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(model); err != nil && err != io.EOF {
			http.Error(w, "Bad JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := model.Validate(); err != nil {
		http.Error(w, "Bad model: "+err.Error(), http.StatusBadRequest)
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

		// B18: старые поселения/заводы/товары удаляются до генерации, иначе
		// повторный запуск дублирует данные (аналогично clearPlanets в B11).
		// Заводы и товары по логике от поселений не зависят — каскада нет,
		// чистим все три таблицы явно.
		oldCount, err := h.clearSettlementsLayer()
		if err != nil {
			log.Printf("❌ GenerateSettlements: delete old records: %v", err)
			statusManager.Fail(generator.JobGenerateSettlements, err.Error())
			return
		}
		log.Printf("🗑️ GenerateSettlements: удалено старых записей: %d", oldCount)

		gen := settlement.NewGenerator(h.db, 0)
		count, err := gen.GenerateSettlements(ctx, model, func(processed int) {
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

// ClearSettlements — удаляет ВСЕ поселения, оставляя планеты на месте.
//
// Заводы и партии товаров не трогаются: с поселениями они по логике не
// связаны (FK заводов на planets, не на поселения) и создаются отдельно
// (решение игрока 2026-09-13).
func (h *AdminHandlers) ClearSettlements(w http.ResponseWriter, r *http.Request) {
	if statusManager.IsRunning(generator.JobGenerateSettlements) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}

	var before int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM settlements`).Scan(&before); err != nil {
		log.Printf("❌ ClearSettlements: count error: %v", err)
		http.Error(w, "Failed to count settlements", http.StatusInternalServerError)
		return
	}
	if _, err := h.db.Exec(`DELETE FROM settlements`); err != nil {
		log.Printf("❌ ClearSettlements: delete error: %v", err)
		http.Error(w, "Failed to clear settlements", http.StatusInternalServerError)
		return
	}

	log.Printf("🗑️ ClearSettlements: удалено поселений: %d", before)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"deleted": before})
}

// clearSettlementsLayer — удаляет поселения, заводы и партии товаров
// (слой экономики, создаваемый GenerateSettlements). Возвращает суммарное
// число удалённых записей. Заводы и товары не зависят от поселений
// (FK заводов/товаров на planets), поэтому все три таблицы чистим явно.
func (h *AdminHandlers) clearSettlementsLayer() (int, error) {
	tables := []string{"settlements", "factories", "goods_batches"}
	deleted := 0
	for _, table := range tables {
		var n int
		if err := h.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			return 0, err
		}
		if _, err := h.db.Exec(`DELETE FROM ` + table); err != nil {
			return 0, err
		}
		deleted += n
	}
	return deleted, nil
}