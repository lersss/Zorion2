// internal/handlers/npc_positions.go
// Позиции NPC-агентов для карты (спека 20a.1 §7, §8): GET /api/npc/positions
// (игровой JWT). Отдаёт in-memory snapshot менеджера — O(1), lock-free
// (спека §2.2.B); клиент не знает кортежей полёта, позиции уже
// интерполированы на момент последнего тика.
package handlers

import (
	"net/http"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/npc"
)

// Positions — GET /api/npc/positions: только агенты в полёте (90a) с x, y,
// status, target. Для role=player агенты вне радиуса радара скрыты
// (спека 77a §5.4/§11.2); admin/skycomposer — без фильтра радиуса (И7),
// фильтр «только в полёте» применяется ко всем ролям (90a).
func (h *AdminNPCHandlers) Positions(w http.ResponseWriter, r *http.Request) {
	positions := h.manager.Positions()
	if positions == nil {
		positions = []npc.InterpolatedPosition{}
	}
	// Только корабли в полёте (90a): idle-агенты не отображаются (для всех
	// ролей, вариант a).
	flying := make([]npc.InterpolatedPosition, 0, len(positions))
	for _, p := range positions {
		if p.Status == models.NPCAgentStatusFlying {
			flying = append(flying, p)
		}
	}
	positions = flying
	if h.visibility != nil && roleFromContext(r) == string(models.RolePlayer) {
		positions = h.filterPositionsByVisibility(r, positions)
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"positions": positions})
}

// filterPositionsByVisibility — оставляет агентов в круге радара игрока.
// Позиция игрока неизвестна — пустой список (безопасное направление:
// ничего не «светим» сверх радиуса).
func (h *AdminNPCHandlers) filterPositionsByVisibility(r *http.Request, positions []npc.InterpolatedPosition) []npc.InterpolatedPosition {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	user, err := h.visibility.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return []npc.InterpolatedPosition{}
	}
	centerX, centerY, ok := h.visibility.PlayerPosition(user)
	if !ok {
		return []npc.InterpolatedPosition{}
	}
	radius := h.visibility.RadarRadius(user)
	out := make([]npc.InterpolatedPosition, 0, len(positions))
	for _, p := range positions {
		if IsVisible(p.X, p.Y, centerX, centerY, radius) {
			out = append(out, p)
		}
	}
	return out
}