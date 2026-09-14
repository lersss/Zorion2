// internal/handlers/npc_positions.go
// Позиции NPC-агентов для карты (спека 20a.1 §7, §8): GET /api/npc/positions
// (игровой JWT). Отдаёт in-memory snapshot менеджера — O(1), lock-free
// (спека §2.2.B); клиент не знает кортежей полёта, позиции уже
// интерполированы на момент последнего тика.
package handlers

import (
	"net/http"

	"zorion/internal/npc"
)

// Positions — GET /api/npc/positions: все агенты с x, y, status, target.
func (h *AdminNPCHandlers) Positions(w http.ResponseWriter, r *http.Request) {
	positions := h.manager.Positions()
	if positions == nil {
		positions = []npc.InterpolatedPosition{}
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"positions": positions})
}