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
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

type TravelHandlers struct {
	worldRepo     *repository.WorldRepository
	userRepo      *repository.UserRepository
	travelManager *travel.Manager
	// Внутрисистемные полёты (спека 99.2.27 §4.2, С1): старт межзвёздного
	// отменяет активный внутрисистемный полёт и NULL-ит позицию.
	intraManager *travel.IntrasystemManager
	intraRepo    *repository.PlayerIntrasystemFlightRepository
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

// SetIntrasystem — подключает внутрисистемные полёты (С1-дельта /travel).
// Сеттер (не параметр конструктора): менеджер создаётся позже в main.go.
func (h *TravelHandlers) SetIntrasystem(intraManager *travel.IntrasystemManager, intraRepo *repository.PlayerIntrasystemFlightRepository) {
	h.intraManager = intraManager
	h.intraRepo = intraRepo
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

// calcTravelDuration вычисляет длительность полёта по расстоянию между мирами:
// dist * speedFactor секунд, минимум 3 секунды (решение создателя 2026-09-16;
// потолок 20 секунд от 2026-09-14 убран). speedFactor — скорость из
// установленного двигателя игрока (спека 91a §7.1: ship.EngineSpeed, 0.3 —
// значение 66a не меняется, меняется источник).
func calcTravelDuration(dist float64, speedFactor float64) time.Duration {
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

	// Валидация двигателя (спека 91a §6.1): role=player без установленного
	// двигателя не летает («не может летать без двигателя» — создатель);
	// админ/skycomposer — исключение (летают всегда, как видимость 77a).
	// NPC-агенты не затрагиваются — у них своя настройка npcSpeedFactor.
	if user.Role == models.RolePlayer && !ship.HasEngine(user.Equipment) {
		writeJSONError(w, "Двигатель не установлен — полёт невозможен", http.StatusBadRequest)
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

	// Полёт к любой звезде разрешён (спека 77a §7.3, решение создателя
	// 2026-09-17): валидация цели — только существование мира (404 выше);
	// «слепой прыжок» отменён, знание координат для полёта не требуется (И6).
	// Топливо/дальность — будущее ограничение (задел, не реализуется).

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

	// «Already in this world» — только без полёта (66a): при активном полёте
	// выбор мира отправления обрабатывается редиректом выше (разворот из
	// текущей точки P), а не 400.
	if flight := h.travelManager.GetFlight(userID); flight == nil && fromWorldID == req.WorldID {
		http.Error(w, "Already in this world", http.StatusBadRequest)
		return
	}

	dx := startX - targetWorld.CoordX
	dy := startY - targetWorld.CoordY
	dist := math.Sqrt(dx*dx + dy*dy)

	// Скорость полёта — из установленного двигателя (спека 91a §7.1):
	// значение 0.3 (66a) не меняется, меняется источник (замысел 77a §1.3).
	duration := calcTravelDuration(dist, ship.EngineSpeed(user.Equipment))

	// Прибытие межзвёздного полёта (спека 99.2.27 §3.6.3/ИП-2): current_world_id
	// + current_position = «орбита звезды» одним UPDATE (С-1) — позиция никогда
	// не остаётся битой между двумя апдейтами.
	onArrival := func(uid, worldID string) {
		// Дефенсив (пакман, спека 2026-09-20 §7.2): цель съедена между
		// запросом и прибытием — обнуляем current_world_id/current_position
		// вместо FK-violation (users.current_world_id → worlds NO ACTION).
		w, err := h.worldRepo.GetByID(worldID)
		if err != nil || w == nil {
			if err := h.userRepo.ClearCurrentWorld(uid); err != nil {
				log.Printf("Failed to clear current world for user %s: %v", uid, err)
			}
			return
		}
		if err := h.userRepo.UpdateCurrentWorldAndPosition(uid, worldID, models.StarOrbitPosition(worldID)); err != nil {
			log.Printf("Failed to update current world for user %s: %v", uid, err)
		}
	}

	// С1 (спека 99.2.27 §4.2): старт межзвёздного полёта отменяет активный
	// внутрисистемный полёт и NULL-ит позицию (игрок покидает систему).
	// Атомарность: отмена intra + позиция NULL — одной транзакцией
	// (CancelAtomic), не остаётся окна, где intra отменён, а позиция ещё нет.
	// Порядок: СНАЧАЛА транзакция (строка + позиция NULL), ПОТОМ in-memory
	// отмена — при краше в окне между ними позиция уже NULL (не in_flight
	// без строки полёта).
	if h.intraRepo != nil {
		if err := h.intraRepo.CancelAtomic(userID); err != nil {
			log.Printf("⚠️ travel: cancel intrasystem (user %s): %v", userID, err)
		}
	}
	if h.intraManager != nil {
		h.intraManager.CancelIntraFlight(userID)
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