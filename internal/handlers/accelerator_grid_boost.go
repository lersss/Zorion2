package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/routegame"
	"zorion/internal/ship"
)

// accelerator_grid_boost.go — ручка POST /api/accelerator/boost на модели v9
// «Планшет» (спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §14):
// сервер пересобирает ТО ЖЕ поле, что видел клиент (seed сегмента + серверный
// secret/layout из player_route_puzzle), применяет помеху вскрытых
// unstable-секторов (§14.4 R2 / §14.7 `ping_destabilize`), оценивает путь
// кривой модели (bonus может быть отрицательным — перелёт удлиняется, И-6
// отменена) и атомарно перебазирует сегмент. Старая (v1) модель
// (field.go/evaluate.go) тут больше не участвует.

// acceleratorBoostRequest — тело POST /api/accelerator/boost: fingerprint
// текущего сегмента + путь клетками доски v9 (индексы 0..N·N−1, 4-связная
// цепочка старт → все маяки → финиш).
type acceleratorBoostRequest struct {
	Fingerprint string `json:"fingerprint"`
	Path        []int  `json:"path"`
}

// acceleratorBoostResponse — результат применения ускорения (§4.1 шаг 9):
// бонус модели (может быть отрицательным), новый остаток сегмента и
// аддитивный разбор по факторам (§14.13.5). Поля старой шкалы (`quality`)
// убраны — с кривой v9 смысла не несут.
type acceleratorBoostResponse struct {
	Bonus      float64                `json:"bonus"`
	RemainingS int                    `json:"remaining_s"`
	Breakdown  []routegame.GridFactor `json:"breakdown"`
}

// acceleratorUnstableSectors — индексы вскрытых `unstable`-секторов из revealed
// (элементы {"sector":N,"content":"unstable"}). Повтор вскрытия даёт дубликаты —
// помеха идемпотентна, поэтому дедупликация не нужна.
func acceleratorUnstableSectors(revealed []byte) []int {
	unstable := routegame.GridContentUnstable.String()
	var out []int
	for _, r := range acceleratorRevealedList(revealed) {
		if r.Content == unstable {
			out = append(out, r.Sector)
		}
	}
	return out
}

// acceleratorRevealedContents — revealed из БД в карту sector → content для
// разбора по факторам (§14.13.5). Неизвестный content пропускается; повтор
// сектора перезаписывает значение (set-семантика).
func acceleratorRevealedContents(raw []byte) map[int]routegame.GridSectorContent {
	out := make(map[int]routegame.GridSectorContent)
	for _, r := range acceleratorRevealedList(raw) {
		if c, ok := routegame.GridSectorContentFromString(r.Content); ok {
			out[r.Sector] = c
		}
	}
	return out
}

// clearRoutePuzzle — явная очистка задачи сегмента при прибытии (§14.1:
// состояние живёт при сегменте player_flights). Nil-безопасно: без репозитория —
// no-op (тесты/конфигурации без доски).
func (h *TravelHandlers) clearRoutePuzzle(userID string) {
	if h.routePuzzleRepo == nil {
		return
	}
	if err := h.routePuzzleRepo.Delete(context.Background(), userID, acceleratorPuzzleKind); err != nil {
		log.Printf("⚠️ accelerator: clear route puzzle (user %s): %v", userID, err)
	}
}

// AcceleratorBoost — POST /api/accelerator/boost (§4.1): гейты как у offer/scan
// (нет полёта; нет/невалиден модуль; игра не зарегистрирована; already_active;
// устаревший fingerprint; порог остатка отправки), пересборка того же поля v9,
// помеха разведки, оценка пути и атомарная замена сегмента через BoostFlight.
// Откат тратится только за принятый полный маршрут (решение О8).
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
	if !h.allowAccelRequest(userID, now) {
		writeAcceleratorRefusal(w, http.StatusTooManyRequests, "rate_limited", 0)
		return
	}

	// Шаг 1: нет полёта.
	flight := h.travelManager.GetFlight(userID)
	if flight == nil {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_flight", 0)
		return
	}
	if h.accelRepo == nil || h.worldRepo == nil || h.routePuzzleRepo == nil {
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

	// Гейт модуля/активности — тот же источник, что у offer/scan (§3.4/§4.1),
	// ещё без порога остатка: порог ОТПРАВКИ проверяется после мира-цели (§4.1
	// шаги 4–5). Откат проверяет ApplyBoost под локом (шаг 8) — checkCooldown=false.
	remaining := flightRemaining(flight, now)
	_, cfg, found, valid := ship.AcceleratorModule(user.Equipment)
	if reason := acceleratorTravelReason(found, valid, ship.AcceleratorGameRegistered(cfg.Game), models.BoostActive(state.LastBoostAt, flight.StartTime), remaining, 0, cooldownLeft, false); reason != "" {
		writeAcceleratorRefusal(w, http.StatusConflict, reason, cooldownLeft)
		return
	}

	// Шаг 4: мир-цель съеден пакманом — откат не тратится.
	target, err := h.worldRepo.GetByID(flight.ToWorld)
	if err != nil || target == nil {
		writeJSONError(w, "Мир назначения не найден", http.StatusNotFound)
		return
	}

	// Шаг 5: порог ОТПРАВКИ (не показа) — откат не тратится.
	if reason := acceleratorTravelReason(found, valid, ship.AcceleratorGameRegistered(cfg.Game), false, remaining, cfg.MinRemainingBoostS, cooldownLeft, false); reason != "" {
		writeAcceleratorRefusal(w, http.StatusConflict, reason, cooldownLeft)
		return
	}

	from, err := h.worldRepo.GetByID(flight.FromWorld)
	if err != nil || from == nil {
		writeJSONError(w, "Мир отправления не найден", http.StatusNotFound)
		return
	}

	// Шаг 7: пересборка ТОГО ЖЕ поля, что в offer/scan (тот же seed + серверный
	// secret/layout) — клиент и сервер оценивают один план (§6.1). Смена
	// сегмента автоматически пересоздаёт задачу (segment_hash) — старая не влияет.
	_, st, field, err := h.acceleratorGridSegment(userID, flight, from, target)
	if err != nil {
		if errors.Is(err, errGridNoField) {
			writeAcceleratorRefusal(w, http.StatusConflict, "no_field", cooldownLeft)
			return
		}
		log.Printf("⚠️ accelerator boost: puzzle (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось собрать доску", http.StatusInternalServerError)
		return
	}

	// Помеха разведки: вскрытые unstable-секторы делают клетки и подход дорогими
	// (§14.4 R2 / §14.7 ping_destabilize) — бонус ниже, вплоть до отрицательного.
	field = routegame.DestabilizeGridField(field, acceleratorUnstableSectors(st.Revealed))

	bonus, pathValid, reason := routegame.EvaluateGridPath(field, req.Path)
	if !pathValid {
		writeJSONStatus(w, http.StatusBadRequest, map[string]interface{}{
			"available": false,
			"reason":    reason,
		})
		return
	}
	// Разбор по факторам (§14.13): называется, что случилось на курсе; на bonus
	// и применение не влияет. Поле аддитивное — старый клиент читает только
	// bonus/remaining_s.
	breakdown := routegame.ExplainGridPath(field, req.Path, acceleratorRevealedContents(st.Revealed))
	// bonus может быть отрицательным (низ шкалы ≈ −0.20): перелёт удлиняется.
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
		Bonus:      bonus,
		RemainingS: int(newRem.Seconds()),
		Breakdown:  breakdown,
	})
}
