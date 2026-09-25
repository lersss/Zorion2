package handlers

import (
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

// accelerator_handlers.go — блок accelerator в контрактах /travel и /me
// (спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §3.4, ЧК1).
//
// Гейт «доступно ускорение» и признак «ускорение действует» — производные от
// уже хранимых полей (§3.3): active = flight.StartTime.UnixMilli() ==
// last_boost_at.UnixMilli(); новых колонок/флагов нет.
//
// Ручки предложения/применения мини-игры (GET /api/accelerator/offer,
// POST /api/accelerator/boost) и UI перенесены в ЧК3 (решение менеджера по гейту
// ЧК1, 2026-09-25): их тела требуют «паспорта перелёта» (ЧК2) и поля/оценки
// качества q (ЧК3). Заглушек в прод-пути нет.
//
// Доступность мини-игры — по реестру реализованных игр (§3.4): пока игр нет
// (ship.AcceleratorGameRegistered), любая валидная запись каталога даёт
// available=false, reason=unknown_game — честное состояние «игры ещё нет».

// AcceleratorResetSelf — POST /admin/accelerator/reset-self (идея ускорителя
// §13): админский сброс СОБСТВЕННОГО таймера вызывающего — и отката, и
// признака «ускорение уже действует» — для наигрыша мини-игры без ожидания
// отката. Роли (admin + skycomposer) проверяет auth.AdminAuth в main.go.
func (h *TravelHandlers) AcceleratorResetSelf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	if h.accelRepo == nil {
		writeJSONError(w, "Ускоритель недоступен", http.StatusInternalServerError)
		return
	}
	if err := h.accelRepo.Reset(userID); err != nil {
		log.Printf("⚠️ accelerator reset-self (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось сбросить таймер ускорителя", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// acceleratorTravelReason — единый приоритет причин недоступности (§3.4/§4.1):
// no_module → unknown_game → already_active → too_short → cooldown. Пусто —
// доступно. gameRegistered — сервер умеет проводить игру из params.game; в ЧК1
// игр не зарегистрировано, поэтому валидный модуль даёт unknown_game.
func acceleratorTravelReason(found, valid, gameRegistered, active bool, flight *travel.TravelInfo, cfg ship.AcceleratorConfig, cooldownLeft time.Duration, now time.Time) string {
	switch {
	case !found:
		return "no_module"
	case !valid || !gameRegistered:
		return "unknown_game"
	case active:
		return "already_active"
	case flight == nil || flightRemaining(flight, now) < time.Duration(cfg.MinRemainingOfferS)*time.Second:
		return "too_short"
	case cooldownLeft > 0:
		return "cooldown"
	}
	return ""
}

// acceleratorTravelBlock — блок accelerator в ответе /travel (§3.4). Возвращает
// nil, если репозиторий/пользователь недоступны (аддитивность, И3). Блок
// отдаётся на ОБОИХ путях /travel (оба 202, М-6).
func acceleratorTravelBlock(repo *repository.PlayerAcceleratorRepository, user *models.User, flight *travel.TravelInfo, now time.Time) interface{} {
	if repo == nil || user == nil {
		return nil
	}
	state, err := repo.GetState(user.ID)
	if err != nil {
		log.Printf("⚠️ accelerator: state (user %s): %v", user.ID, err)
		return nil
	}

	id, cfg, found, valid := ship.AcceleratorModule(user.Equipment)
	cooldownLeft := models.CooldownRemaining(state.LastBoostAt, state.LastCooldownMin, now)
	active := flight != nil && models.BoostActive(state.LastBoostAt, flight.StartTime)
	reason := acceleratorTravelReason(found, valid, ship.AcceleratorGameRegistered(cfg.Game), active, flight, cfg, cooldownLeft, now)

	out := map[string]interface{}{
		"available": reason == "",
		"active":    active,
	}
	if reason != "" {
		out["reason"] = reason
	}
	if found {
		out["module_id"] = id
		out["game"] = cfg.Game
	}
	if cooldownLeft > 0 {
		out["cooldown_remaining_s"] = cooldownSeconds(cooldownLeft)
	} else {
		out["cooldown_remaining_s"] = nil
	}
	return out
}

// acceleratorMeBlock — аддитивный блок accelerator в /me (М-4): индикатор
// готовности/действующего ускорения нужен и ВНЕ полёта. cooldown_min —
// ПРИМЕНЁННЫЙ откат (player_accelerator.last_cooldown_min), не параметр текущего
// модуля (§3.3/М-5): после смены модуля таймер не «прыгает». nil, если
// репозиторий/пользователь недоступны.
func acceleratorMeBlock(repo *repository.PlayerAcceleratorRepository, user *models.User, flight *travel.TravelInfo, now time.Time) interface{} {
	if repo == nil || user == nil {
		return nil
	}
	state, err := repo.GetState(user.ID)
	if err != nil {
		log.Printf("⚠️ accelerator: state (user %s): %v", user.ID, err)
		return nil
	}

	id, _, found, _ := ship.AcceleratorModule(user.Equipment)
	cooldownLeft := models.CooldownRemaining(state.LastBoostAt, state.LastCooldownMin, now)
	active := flight != nil && models.BoostActive(state.LastBoostAt, flight.StartTime)

	out := map[string]interface{}{
		"active": active,
		"ready":  cooldownLeft <= 0,
	}
	if found {
		out["module_id"] = id
	}
	if state.LastBoostAt != nil {
		out["last_boost_at"] = state.LastBoostAt.UnixMilli()
	} else {
		out["last_boost_at"] = nil
	}
	if state.LastCooldownMin != nil {
		out["cooldown_min"] = *state.LastCooldownMin
	} else {
		out["cooldown_min"] = nil
	}
	if cooldownLeft > 0 {
		out["cooldown_remaining_s"] = cooldownSeconds(cooldownLeft)
	} else {
		out["cooldown_remaining_s"] = nil
	}
	return out
}

// flightRemaining — остаток текущего сегмента; истёк → 0.
func flightRemaining(f *travel.TravelInfo, now time.Time) time.Duration {
	end := f.StartTime.Add(f.Duration)
	if !end.After(now) {
		return 0
	}
	return end.Sub(now)
}

// cooldownSeconds — остаток отката в секундах для таймера клиента (вверх, чтобы
// во время идущего отката не показать 0).
func cooldownSeconds(d time.Duration) int64 {
	return int64(math.Ceil(d.Seconds()))
}
