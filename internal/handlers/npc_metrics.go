// internal/handlers/npc_metrics.go
// Метрики поведения NPC (спека 26a.1 §8): GET /admin/npc/metrics — живые
// COUNT по индексам (всего/idle/flying/очередь на тик) + in-memory счётчики
// планировщика (последний тик, полные батчи, последняя пачка генерации).
// In-memory часть сбрасывается при рестарте — для инструмента замеров ок
// (спека §2, Р4-B; ограничение 1 §11).
package handlers

import (
	"log"
	"net/http"
	"time"

	"zorion/internal/models"
)

// Metrics — GET /admin/npc/metrics (спека 26a.1 §8.2). overdue_arrivals —
// живой COUNT при каждом опросе (вариант «а» создателя): flying И
// arrive_at <= now — главный индикатор лагов планировщика (§8.1).
func (h *AdminNPCHandlers) Metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}

	total, err := h.npcRepo.CountTotal()
	if err != nil {
		log.Printf("Metrics: CountTotal: %v", err)
		writeJSONError(w, "Не удалось получить метрики", http.StatusInternalServerError)
		return
	}
	idle, err := h.npcRepo.CountByStatus(models.NPCAgentStatusIdle)
	if err != nil {
		log.Printf("Metrics: CountByStatus(idle): %v", err)
		writeJSONError(w, "Не удалось получить метрики", http.StatusInternalServerError)
		return
	}
	flying, err := h.npcRepo.CountByStatus(models.NPCAgentStatusFlying)
	if err != nil {
		log.Printf("Metrics: CountByStatus(flying): %v", err)
		writeJSONError(w, "Не удалось получить метрики", http.StatusInternalServerError)
		return
	}
	overdue, err := h.npcRepo.CountOverdue(time.Now())
	if err != nil {
		log.Printf("Metrics: CountOverdue: %v", err)
		writeJSONError(w, "Не удалось получить метрики", http.StatusInternalServerError)
		return
	}

	mm := h.manager.Metrics()
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"agents_total":      total,
		"agents_idle":       idle,
		"agents_flying":     flying,
		"overdue_arrivals":  overdue,
		"scheduler":         mm.Scheduler,
		"last_bulk":         mm.LastBulk, // null, если пачки не было (§8.2)
	})
}