// internal/handlers/npc_search.go
// Поиск агента на карте (спека 26a.1 §6): GET /api/npc/search?q=&limit= —
// игровая ручка (JWT). Валидный UUID в q → точное совпадение по id (PK,
// быстрый); иначе — name ILIKE '%q%'. Позиция — из in-memory PositionCache
// (x/y = null, если агента нет в снапшоте — клиент центрирует только при
// наличии, §6.2). Механика полёта/наблюдения не трогается.
package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"zorion/internal/models"
)

const (
	npcSearchDefaultLimit = 20
	npcSearchMaxLimit     = 100
	npcSearchMaxQLen      = 100
)

// SearchAgent — GET /api/npc/search (спека 26a.1 §6.1).
func (h *AdminNPCHandlers) SearchAgent(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSONError(w, "q обязателен", http.StatusBadRequest)
		return
	}
	if len(q) > npcSearchMaxQLen {
		writeJSONError(w, "q слишком длинный (макс. 100 символов)", http.StatusBadRequest)
		return
	}

	limit := npcSearchDefaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			writeJSONError(w, "limit должен быть целым ≥ 1", http.StatusBadRequest)
			return
		}
		if n > npcSearchMaxLimit {
			writeJSONError(w, "limit не может превышать 100", http.StatusBadRequest)
			return
		}
		limit = n
	}

	// Валидный UUID → точное совпадение по id; иначе — ILIKE по имени.
	var agents []models.NPCAgent
	var err error
	if _, uerr := uuid.Parse(q); uerr == nil {
		var a *models.NPCAgent
		a, err = h.npcRepo.GetByID(q)
		if a != nil {
			agents = []models.NPCAgent{*a}
		}
	} else {
		agents, err = h.npcRepo.SearchByName(q, limit)
	}
	if err != nil {
		log.Printf("SearchAgent: %v", err)
		writeJSONError(w, "Не удалось выполнить поиск", http.StatusInternalServerError)
		return
	}

	// Позиции — из снапшота PositionCache (спека §6.1: линейный поиск по id
	// в слайсе — ≤ limit × 100к сравнений на запрос, разово, приемлемо).
	positions := h.manager.Positions()
	results := make([]map[string]interface{}, 0, len(agents))
	for _, a := range agents {
		res := map[string]interface{}{
			"id":       a.ID,
			"name":     a.Name,
			"status":   a.Status,
			"world_id": a.CurrentWorldID, // idle: текущий мир
		}
		if a.Status == models.NPCAgentStatusFlying && a.TargetWorldID != nil {
			res["world_id"] = *a.TargetWorldID // летящий: цель полёта
		}
		found := false
		for _, p := range positions {
			if p.ID == a.ID {
				res["x"] = p.X
				res["y"] = p.Y
				found = true
				break
			}
		}
		if !found {
			res["x"] = nil // агента нет в snapshot — позиция неизвестна
			res["y"] = nil
		}
		results = append(results, res)
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"results": results})
}