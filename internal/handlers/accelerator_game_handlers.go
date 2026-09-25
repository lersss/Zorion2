package handlers

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/routegame"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// accelerator_game_handlers.go — ручки мини-игры «Прокладка маршрута»
// (спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §3.4/§4.1, ЧК3):
// GET /api/accelerator/offer и POST /api/accelerator/boost.
//
// Поле/паспорт/оценка — производные (§6.1/§5.3): поле считается из seed
// hash(from,to) и дальности СЕГМЕНТА (старт сегмента → цель), не из «живого»
// остатка/прогресса, поэтому offer и boost строят одно и то же поле. Расчёт
// P/bonus/newRem — в хендлере (оркестрация С-1), менеджер лишь атомарно
// заменяет сегмент под своим локом.

// acceleratorOfferResponse — контракт offer (§3.4): без прогноза прибытия и
// числа секунд результата (решение 14); remaining_s — верхний уровень (в
// паспорт не входит).
type acceleratorOfferResponse struct {
	Fingerprint        string             `json:"fingerprint"`
	Game               string             `json:"game"`
	Passport           routegame.Passport `json:"passport"`
	Field              routegame.Field    `json:"field"`
	RemainingS         int                `json:"remaining_s"`
	MinRemainingBoostS int                `json:"min_remaining_boost_s"`
	CooldownRemainingS *int64             `json:"cooldown_remaining_s"`
}

// acceleratorBoostRequest — тело POST /api/accelerator/boost (§3.4):
// fingerprint текущего сегмента + полилиния пути в поле [0,1]².
type acceleratorBoostRequest struct {
	Fingerprint string            `json:"fingerprint"`
	Path        []routegame.Point `json:"path"`
}

// acceleratorBoostResponse — результат применения ускорения (§4.1 шаг 9).
type acceleratorBoostResponse struct {
	Quality    float64 `json:"quality"`
	Bonus      float64 `json:"bonus"`
	RemainingS int     `json:"remaining_s"`
}

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

// acceleratorSegmentView — паспорт и поле сегмента одними входами: seed
// hash(from,to) + дальность сегмента + паспорт (пояса — только видимые).
// Используется и offer, и boost — сервер оценивает путь по тому же полю,
// что видел клиент.
func acceleratorSegmentView(flight *travel.TravelInfo, from, to *models.World, belts []models.Belt) (routegame.Passport, routegame.Field) {
	dist := acceleratorSegmentDist(flight, to)
	passport := routegame.BuildPassport(dist, *from, *to, visibleBelts(belts))
	field := routegame.GenerateField(routegame.HashSeed(flight.FromWorld, flight.ToWorld), dist, passport)
	return passport, field
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
// cooldown_remaining_s} без field/passport; доступно → 200 с паспортом и полем.
func (h *TravelHandlers) AcceleratorOffer(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	now := time.Now()
	flight := h.travelManager.GetFlight(userID)
	if flight == nil {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_flight", 0)
		return
	}
	if h.accelRepo == nil || h.worldRepo == nil {
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
	reason := acceleratorTravelReason(found, valid, ship.AcceleratorGameRegistered(cfg.Game), active, flight, cfg, cooldownLeft, now)
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

	passport, field := acceleratorSegmentView(flight, from, target, h.acceleratorBelts(flight.ToWorld))
	writeJSON(w, acceleratorOfferResponse{
		Fingerprint:        acceleratorFingerprint(flight),
		Game:               cfg.Game,
		Passport:           passport,
		Field:              field,
		RemainingS:         int(flightRemaining(flight, now).Seconds()),
		MinRemainingBoostS: cfg.MinRemainingBoostS,
		CooldownRemainingS: cooldownPtr(cooldownLeft),
	})
}

// AcceleratorBoost — POST /api/accelerator/boost (§4.1, шаги 1–9): сервер
// пересобирает то же поле, проверяет путь, считает bonus/newRem и атомарно
// перебазирует сегмент через Manager.BoostFlight. Откат тратится только за
// отправленный полный маршрут (решение О8).
func (h *TravelHandlers) AcceleratorBoost(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	var req acceleratorBoostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}
	now := time.Now()

	// Шаг 1: нет полёта.
	flight := h.travelManager.GetFlight(userID)
	if flight == nil {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_flight", 0)
		return
	}
	if h.accelRepo == nil || h.worldRepo == nil {
		writeJSONError(w, "Ускоритель недоступен", http.StatusInternalServerError)
		return
	}

	// Шаг 2: маршрут изменился (разворот 61a/перебазирование) — откат не тратится.
	if req.Fingerprint != acceleratorFingerprint(flight) {
		writeAcceleratorRefusal(w, http.StatusConflict, "changed", 0)
		return
	}

	state, err := h.accelRepo.GetState(userID)
	if err != nil {
		log.Printf("⚠️ accelerator boost: state (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось прочитать состояние ускорителя", http.StatusInternalServerError)
		return
	}
	cooldownLeft := models.CooldownRemaining(state.LastBoostAt, state.LastCooldownMin, now)
	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}

	// Гейт модуля — тот же приоритет, что у available (§3.4).
	_, cfg, found, valid := ship.AcceleratorModule(user.Equipment)
	if !found {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_module", cooldownLeft)
		return
	}
	if !valid || !ship.AcceleratorGameRegistered(cfg.Game) {
		writeAcceleratorRefusal(w, http.StatusConflict, "unknown_game", cooldownLeft)
		return
	}

	// Шаг 3: ускорение уже действует на сегменте — откат не тратится.
	if models.BoostActive(state.LastBoostAt, flight.StartTime) {
		writeAcceleratorRefusal(w, http.StatusConflict, "already_active", cooldownLeft)
		return
	}

	// Шаг 4: мир-цель съеден пакманом — откат не тратится.
	target, err := h.worldRepo.GetByID(flight.ToWorld)
	if err != nil || target == nil {
		writeJSONError(w, "Мир назначения не найден", http.StatusNotFound)
		return
	}

	// Шаг 5: порог ОТПРАВКИ (не показа) — откат не тратится.
	remaining := flightRemaining(flight, now)
	if remaining < time.Duration(cfg.MinRemainingBoostS)*time.Second {
		writeAcceleratorRefusal(w, http.StatusConflict, "too_short", cooldownLeft)
		return
	}

	from, err := h.worldRepo.GetByID(flight.FromWorld)
	if err != nil || from == nil {
		writeJSONError(w, "Мир отправления не найден", http.StatusNotFound)
		return
	}

	// Шаг 7: пересборка ТОГО ЖЕ поля, что в offer (dist — от старта сегмента,
	// не от P) — клиент и сервер оценивают один план (§6.1).
	_, field := acceleratorSegmentView(flight, from, target, h.acceleratorBelts(flight.ToWorld))
	q, pathValid, reason := routegame.EvaluatePath(field, req.Path)
	if !pathValid {
		writeJSONStatus(w, http.StatusBadRequest, map[string]interface{}{
			"available": false,
			"reason":    reason,
		})
		return
	}
	bonus, _ := ship.AcceleratorBonus(q, cfg.BonusMax)
	newRem := time.Duration(float64(remaining) / (1 + bonus))

	// Шаг 6: P — текущая точка на линии старта сегмента к цели (примитив 61a).
	px, py := redirectStartPoint(flight.StartX, flight.StartY, target.CoordX, target.CoordY,
		now.Sub(flight.StartTime), flight.Duration)

	// Шаг 8: атомарная замена сегмента под локом менеджера (источник истины
	// по active/откату — ApplyBoost). Ошибка/отказ — откат не списан.
	applied, boostReason, err := h.travelManager.BoostFlight(userID, flight.StartTime, px, py, newRem, cfg.CooldownMin, h.ArrivalHandler)
	if err != nil {
		log.Printf("⚠️ accelerator boost: apply (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось применить ускорение", http.StatusInternalServerError)
		return
	}
	if !applied {
		if boostReason == "" {
			boostReason = "changed" // полёт сменился/истёк между проверкой и локом
		}
		writeAcceleratorRefusal(w, http.StatusConflict, boostReason, cooldownLeft)
		return
	}

	// Шаг 9: выигрыш зафиксирован (решение 4) — новый остаток сегмента.
	writeJSON(w, acceleratorBoostResponse{
		Quality:    q,
		Bonus:      bonus,
		RemainingS: int(newRem.Seconds()),
	})
}
