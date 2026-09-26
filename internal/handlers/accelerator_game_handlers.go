package handlers

import (
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// accelerator_game_handlers.go — ручки мини-игры «Прокладка маршрута»
// (спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута): GET
// /api/accelerator/offer (доска v9, см. accelerator_grid_handlers.go),
// POST /api/accelerator/scan (разведка, там же) и POST /api/accelerator/boost
// (оценка пути и бонус v9, см. accelerator_grid_boost.go).
//
// Поле/паспорт/оценка — производные (§6.1/§5.3): поле считается из seed
// сегмента и серверного secret задачи (player_route_puzzle), не из «живого»
// остатка/прогресса. Старая (v1) модель (field.go/evaluate.go) в проде больше
// не участвует (список потребителей — в отчёте подэтапа B).

// acceleratorFingerprint — стабильный отпечаток сегмента (§3.4/§5.3):
// from_world + ":" + to_world + ":" + start_time (мс). Меняется при
// развороте/перебазировании → старый submit отвергается. Формат — контракт:
// клиент возвращает его без изменений.
func acceleratorFingerprint(flight *travel.TravelInfo) string {
	return flight.FromWorld + ":" + flight.ToWorld + ":" + strconv.FormatInt(flight.StartTime.UnixMilli(), 10)
}

// acceleratorSegmentDist — дальность сегмента от СТАРТА ТЕКУЩЕГО СЕГМЕНТА
// (flight.StartX/StartY) до цели: поле не должно зависеть от прогресса, иначе
// клиент и сервер увидят разные поля (§6.1).
func acceleratorSegmentDist(flight *travel.TravelInfo, target *models.World) float64 {
	dx := target.CoordX - flight.StartX
	dy := target.CoordY - flight.StartY
	return math.Sqrt(dx*dx + dy*dy)
}

// acceleratorBelts — пояса системы назначения (для паспорта); ошибка/нет
// репозитория → пусто (паспорт без поясов, детерминированно).
func (h *TravelHandlers) acceleratorBelts(worldID string) []models.Belt {
	if h.planetRepo == nil {
		return nil
	}
	belts, err := h.planetRepo.GetBeltsByWorldID(worldID)
	if err != nil {
		log.Printf("⚠️ accelerator: belts (world %s): %v", worldID, err)
		return nil
	}
	return belts
}

// cooldownPtr — остаток отката в секундах или nil (нет идущего отката).
func cooldownPtr(d time.Duration) *int64 {
	if d <= 0 {
		return nil
	}
	v := cooldownSeconds(d)
	return &v
}

// writeAcceleratorRefusal — единый вид отказа (409): {available:false, reason,
// cooldown_remaining_s} — без field/passport (§3.4).
func writeAcceleratorRefusal(w http.ResponseWriter, status int, reason string, cooldownLeft time.Duration) {
	writeJSONStatus(w, status, map[string]interface{}{
		"available":            false,
		"reason":               reason,
		"cooldown_remaining_s": cooldownPtr(cooldownLeft),
	})
}

// AcceleratorOffer — GET /api/accelerator/offer (§3.4): предложение мини-игры
// только в полёте. Недоступно → 409 {available:false, reason,
// cooldown_remaining_s} без board/passport; доступно → 200 с паспортом и доской v9.
func (h *TravelHandlers) AcceleratorOffer(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	now := time.Now()
	if !h.allowAccelRequest(userID, now) {
		writeAcceleratorRefusal(w, http.StatusTooManyRequests, "rate_limited", 0)
		return
	}
	flight := h.travelManager.GetFlight(userID)
	if flight == nil {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_flight", 0)
		return
	}
	if h.accelRepo == nil || h.worldRepo == nil || h.routePuzzleRepo == nil {
		writeJSONError(w, "Ускоритель недоступен", http.StatusInternalServerError)
		return
	}

	state, err := h.accelRepo.GetState(userID)
	if err != nil {
		log.Printf("⚠️ accelerator offer: state (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось прочитать состояние ускорителя", http.StatusInternalServerError)
		return
	}
	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}

	_, cfg, found, valid := ship.AcceleratorModule(user.Equipment)
	cooldownLeft := models.CooldownRemaining(state.LastBoostAt, state.LastCooldownMin, now)
	active := models.BoostActive(state.LastBoostAt, flight.StartTime)
	reason := acceleratorTravelReason(found, valid, ship.AcceleratorGameRegistered(cfg.Game), active, flightRemaining(flight, now), cfg.MinRemainingOfferS, cooldownLeft, true)
	if reason != "" {
		writeAcceleratorRefusal(w, http.StatusConflict, reason, cooldownLeft)
		return
	}

	target, err := h.worldRepo.GetByID(flight.ToWorld)
	if err != nil || target == nil {
		writeJSONError(w, "Мир назначения не найден", http.StatusNotFound)
		return
	}
	from, err := h.worldRepo.GetByID(flight.FromWorld)
	if err != nil || from == nil {
		writeJSONError(w, "Мир отправления не найден", http.StatusNotFound)
		return
	}

	// Доска v9 «Планшет» (§14): публичный слой поля текущего сегмента. Строка
	// player_route_puzzle создаётся/пересоздаётся при смене сегмента.
	passport, st, field, err := h.acceleratorGridSegment(userID, flight, from, target)
	if err != nil {
		if errors.Is(err, errGridNoField) {
			writeAcceleratorRefusal(w, http.StatusConflict, "no_field", cooldownLeft)
			return
		}
		log.Printf("⚠️ accelerator offer: puzzle (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось собрать доску", http.StatusInternalServerError)
		return
	}
	writeJSON(w, acceleratorOfferResponse{
		Fingerprint:        acceleratorFingerprint(flight),
		Game:               cfg.Game,
		Passport:           passport,
		Board:              acceleratorGridBoard(field),
		PingsLeft:          st.PingsLeft,
		Revealed:           acceleratorRevealedList(st.Revealed),
		RemainingS:         int(flightRemaining(flight, now).Seconds()),
		MinRemainingBoostS: cfg.MinRemainingBoostS,
		CooldownRemainingS: cooldownPtr(cooldownLeft),
	})
}
