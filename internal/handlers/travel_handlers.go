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
	TravelID  string  `json:"travel_id"`
	Duration  int     `json:"duration"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	StartX    float64 `json:"start_x"`    // стартовая точка сегмента (61a)
	StartY    float64 `json:"start_y"`
	StartTime int64   `json:"start_time"` // UnixMilli из фактического полёта
}

// travelCancelResponse — ответ при возврате в мир отправления во время
// полёта (61a): полёт отменяется, корабль остаётся в мире отправления.
type travelCancelResponse struct {
	Status    string `json:"status"`
	WorldID   string `json:"world_id"`
	WorldName string `json:"world_name,omitempty"`
}

// calcTravelDuration вычисляет длительность полёта по расстоянию между мирами:
// dist * 0.3 секунд, минимум 3 секунды (решение создателя 2026-09-16;
// потолок 20 секунд от 2026-09-14 убран).
func calcTravelDuration(dist float64) time.Duration {
	speedFactor := 0.3
	duration := time.Duration(dist*speedFactor) * time.Second
	if duration < 3*time.Second {
		duration = 3 * time.Second
	}
	return duration
}

// redirectStartPoint — стартовая точка нового полёта при редиректе (61a):
// точка P на отрезке from→to по прогрессу старого полёта (зажат 0..1).
// elapsed < 0 или duration <= 0 → P = from; elapsed >= duration → P = to.
func redirectStartPoint(fromX, fromY, toX, toY float64, elapsed, duration time.Duration) (float64, float64) {
	progress := 0.0
	if duration > 0 {
		progress = float64(elapsed) / float64(duration)
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return fromX + (toX-fromX)*progress, fromY + (toY-fromY)*progress
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

	// Возврат в мир отправления во время полёта (61a): выбор fromWorldID
	// (= user.CurrentWorldID, меняется только по прибытии) при активном
	// полёте = отмена полёта, а не «Already in this world». Имя мира —
	// из targetWorld: в этой ветке req.WorldID == fromWorldID, т.е. это
	// тот же мир. Без полёта — прежний 400 остаётся ниже.
	if flight := h.travelManager.GetFlight(userID); flight != nil && req.WorldID == fromWorldID {
		h.travelManager.CancelFlight(userID)
		writeJSONStatus(w, http.StatusAccepted, travelCancelResponse{
			Status:    "cancelled",
			WorldID:   fromWorldID,
			WorldName: targetWorld.Name,
		})
		return
	}

	if fromWorldID == req.WorldID {
		http.Error(w, "Already in this world", http.StatusBadRequest)
		return
	}

	// Идемпотентность: повторный запрос той же цели во время полёта не
	// перезапускает полёт — возвращаем текущий без сброса прогресса.
	if flight := h.travelManager.GetFlight(userID); flight != nil && flight.ToWorld == req.WorldID {
		resp := TravelResponse{
			TravelID:  userID + "-" + time.Now().Format("20060102150405"),
			Duration:  int(flight.Duration.Seconds()),
			From:      flight.FromWorld,
			To:        flight.ToWorld,
			StartX:    flight.StartX,
			StartY:    flight.StartY,
			StartTime: flight.StartTime.UnixMilli(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
		return
	}

	fromWorld, err := h.worldRepo.GetByID(fromWorldID)
	if err != nil || fromWorld == nil {
		http.Error(w, "Current world not found", http.StatusInternalServerError)
		return
	}

	// Стартовая точка сегмента: при обычном старте — координаты FromWorld;
	// при редиректе (активный полёт, новая цель) — текущая точка P маршрута
	// (61a, решение создателя 2026-09-16: корабль не отскакивает к стартовой
	// звезде, а продолжает из текущей точки).
	startX, startY := fromWorld.CoordX, fromWorld.CoordY
	if flight := h.travelManager.GetFlight(userID); flight != nil {
		// Координаты цели старого полёта. Если мир удалён при перегенерации
		// вселенной — фолбэк на текущую позицию (flight.StartX/StartY).
		toX, toY := flight.StartX, flight.StartY
		if oldTo, err := h.worldRepo.GetByID(flight.ToWorld); err == nil && oldTo != nil {
			toX, toY = oldTo.CoordX, oldTo.CoordY
		}
		// Точка P считается от фактической стартовой точки предыдущего
		// сегмента (flight.StartX/StartY), а не от мира отправления A —
		// иначе при каскадных редиректах корабль «перескакивал» на линию
		// A→новая цель (баг создателя 2026-09-16).
		startX, startY = redirectStartPoint(
			flight.StartX, flight.StartY, toX, toY,
			time.Since(flight.StartTime), flight.Duration,
		)
	}

	dx := startX - targetWorld.CoordX
	dy := startY - targetWorld.CoordY
	dist := math.Sqrt(dx*dx + dy*dy)

	duration := calcTravelDuration(dist)

	onArrival := func(uid, worldID string) {
		if err := h.userRepo.UpdateCurrentWorld(uid, worldID); err != nil {
			log.Printf("Failed to update current world for user %s: %v", uid, err)
		}
	}
	h.travelManager.StartFlight(userID, fromWorldID, req.WorldID, startX, startY, duration, onArrival)

	newFlight := h.travelManager.GetFlight(userID)
	resp := TravelResponse{
		TravelID:  userID + "-" + time.Now().Format("20060102150405"),
		Duration:  int(newFlight.Duration.Seconds()),
		From:      fromWorldID,
		To:        req.WorldID,
		StartX:    newFlight.StartX,
		StartY:    newFlight.StartY,
		StartTime: newFlight.StartTime.UnixMilli(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}