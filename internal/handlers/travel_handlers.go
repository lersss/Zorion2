package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"zorion/internal/auth"
	"zorion/internal/repository"
	"zorion/internal/travel"
)

type TravelHandlers struct {
	worldRepo     *repository.WorldRepository
	userRepo      *repository.UserRepository
	travelManager *travel.Manager
}

func NewTravelHandlers(
	worldRepo *repository.WorldRepository,
	userRepo *repository.UserRepository,
	travelManager *travel.Manager,
) *TravelHandlers {
	return &TravelHandlers{
		worldRepo:     worldRepo,
		userRepo:      userRepo,
		travelManager: travelManager,
	}
}

type TravelRequest struct {
	WorldID string `json:"world_id"`
}

type TravelResponse struct {
	TravelID string `json:"travel_id"`
	Duration int    `json:"duration"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// calcTravelDuration вычисляет длительность полёта по расстоянию между мирами:
// dist * 0.3 секунд, минимум 3 секунды, потолок 20 секунд (решение создателя 2026-09-14).
func calcTravelDuration(dist float64) time.Duration {
	speedFactor := 0.3
	duration := time.Duration(dist*speedFactor) * time.Second
	if duration < 3*time.Second {
		duration = 3 * time.Second
	}
	if duration > 20*time.Second {
		duration = 20 * time.Second
	}
	return duration
}

func (h *TravelHandlers) StartTravel(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req TravelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.WorldID == "" {
		http.Error(w, "world_id required", http.StatusBadRequest)
		return
	}

	targetWorld, err := h.worldRepo.GetByID(req.WorldID)
	if err != nil || targetWorld == nil {
		http.Error(w, "World not found", http.StatusNotFound)
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	fromWorldID := ""
	if user.CurrentWorldID != nil {
		fromWorldID = *user.CurrentWorldID
		// Битый current_world_id (мир удалён при перегенерации вселенной) —
		// сбрасываем и берём первый доступный мир.
		fromWorld, err := h.worldRepo.GetByID(fromWorldID)
		if err != nil || fromWorld == nil {
			fromWorldID = ""
		}
	}
	if fromWorldID == "" {
		worlds, err := h.worldRepo.GetAll()
		if err != nil || len(worlds) == 0 {
			http.Error(w, "No worlds available", http.StatusInternalServerError)
			return
		}
		fromWorldID = worlds[0].ID
		// Возвращаем пользователю валидный текущий мир.
		_ = h.userRepo.UpdateCurrentWorld(userID, fromWorldID)
	}

	if fromWorldID == req.WorldID {
		http.Error(w, "Already in this world", http.StatusBadRequest)
		return
	}

	if h.travelManager.IsInFlight(userID) {
		http.Error(w, "Already in flight", http.StatusBadRequest)
		return
	}

	fromWorld, err := h.worldRepo.GetByID(fromWorldID)
	if err != nil || fromWorld == nil {
		http.Error(w, "Current world not found", http.StatusInternalServerError)
		return
	}
	dx := fromWorld.CoordX - targetWorld.CoordX
	dy := fromWorld.CoordY - targetWorld.CoordY
	dist := math.Sqrt(dx*dx + dy*dy)

	duration := calcTravelDuration(dist)

	onArrival := func(uid, worldID string) {
		if err := h.userRepo.UpdateCurrentWorld(uid, worldID); err != nil {
			log.Printf("Failed to update current world for user %s: %v", uid, err)
		}
	}
	h.travelManager.StartFlight(userID, fromWorldID, req.WorldID, duration, onArrival)

	resp := TravelResponse{
		TravelID: userID + "-" + time.Now().Format("20060102150405"),
		Duration: int(duration.Seconds()),
		From:     fromWorldID,
		To:       req.WorldID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}