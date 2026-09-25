// internal/handlers/property_handlers.go
// Собственность игрока (спека 2026-09-26-собственность-игрока-в-дашборде §5):
// GET /me/property — список единиц собственности игрока (поселения и строения с
// владельцем-игроком) с адресом и режимом знания. Владелец — из JWT (owner_id
// из запроса не принимается, И-1); чтение пакетное — в
// property_repository.go (§4.3). Живых чисел планеты в списке нет (решение В2).
package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"zorion/internal/auth"
	"zorion/internal/repository"
)

// PropertyHandlers — read-ручка собственности игрока.
type PropertyHandlers struct {
	propertyRepo *repository.PropertyRepository
	userRepo     *repository.UserRepository
	planetRepo   *repository.PlanetRepository
}

func NewPropertyHandlers(
	propertyRepo *repository.PropertyRepository,
	userRepo *repository.UserRepository,
	planetRepo *repository.PlanetRepository,
) *PropertyHandlers {
	return &PropertyHandlers{
		propertyRepo: propertyRepo,
		userRepo:     userRepo,
		planetRepo:   planetRepo,
	}
}

// GetMyProperty — GET /me/property: список собственности игрока (§5.1). Планета
// присутствия (приоритетный режим knowledge.mode = presence, §4.3) резолвится по
// позиции игрока тем же правилом, что попап системы (presencePlanetID; спутник —
// по родителю). Пусто → 200 {"items": []} (И-4).
func (h *PropertyHandlers) GetMyProperty(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	// Присутствие: сбой чтения позиции не роняет список — строки отдаются в
	// режиме по знанию (presence лишь приоритетнее, а не обязателен).
	presenceID := ""
	user, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil {
		log.Printf("/me/property: GetByIDWithPosition(%s): %v", userID, err)
	} else if user != nil && user.CurrentWorldID != nil {
		presenceID = presencePlanetID(pos, *user.CurrentWorldID, h.planetRepo)
	}

	items, err := h.propertyRepo.GetPlayerProperty(userID, presenceID)
	if err != nil {
		writeJSONError(w, "Не удалось получить собственность", http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []repository.PropertyItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
}
