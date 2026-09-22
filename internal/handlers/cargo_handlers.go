// internal/handlers/cargo_handlers.go
//
// Трюм игрока (спека 2026-09-22-трюм-грузоподъёмность-корабля §9): чтение
// GET /api/cargo и единственная player-facing запись — «груз за борт»
// POST /api/cargo/jettison (§7.1). Ручек «положить/взять» для игрока нет
// (§9.2): пополнение — только сервис из игровой логики.
package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"zorion/internal/auth"
	"zorion/internal/cargo"
)

// CargoHandlers — трюм игрока.
type CargoHandlers struct {
	svc *cargo.Service
}

func NewCargoHandlers(svc *cargo.Service) *CargoHandlers {
	return &CargoHandlers{svc: svc}
}

// cargoJettisonRequest — тело POST /api/cargo/jettison (§9.2):
// {good_id, quantity} | {good_id, all:true} | {all:true} (весь трюм).
type cargoJettisonRequest struct {
	GoodID   int64   `json:"good_id"`
	Quantity float64 `json:"quantity"`
	All      bool    `json:"all"`
}

// GetCargo — GET /api/cargo: лимиты по осям + строки груза (§9.1). Свой трюм
// (owner — из JWT); чужие не отдаются.
func (h *CargoHandlers) GetCargo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	view, err := h.svc.View(userID)
	if err != nil {
		log.Printf("/api/cargo: view(%s): %v", userID, err)
		writeJSONError(w, "Не удалось получить трюм", http.StatusInternalServerError)
		return
	}
	writeJSON(w, view)
}

// Jettison — POST /api/cargo/jettison: «груз за борт» (§7.1). Только
// уменьшает: списывается не больше, чем есть; ответ — обновлённый трюм
// (та же форма, что GET).
func (h *CargoHandlers) Jettison(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	var req cargoJettisonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}

	if req.All && req.GoodID == 0 {
		if _, err := h.svc.JettisonAll(userID); err != nil {
			log.Printf("/api/cargo/jettison: all(%s): %v", userID, err)
			writeJSONError(w, "Не удалось сбросить груз", http.StatusInternalServerError)
			return
		}
	} else {
		if req.GoodID <= 0 {
			writeJSONError(w, "good_id обязателен", http.StatusBadRequest)
			return
		}
		if !req.All && req.Quantity <= 0 {
			writeJSONError(w, "quantity должна быть > 0 или all=true", http.StatusBadRequest)
			return
		}
		if _, err := h.svc.JettisonCargo(userID, req.GoodID, req.Quantity, req.All); err != nil {
			if errors.Is(err, cargo.ErrGoodNotFound) {
				writeJSONError(w, "Товар не найден", http.StatusNotFound)
				return
			}
			log.Printf("/api/cargo/jettison: good=%d(%s): %v", req.GoodID, userID, err)
			writeJSONError(w, "Не удалось сбросить груз", http.StatusInternalServerError)
			return
		}
	}

	view, err := h.svc.View(userID)
	if err != nil {
		log.Printf("/api/cargo/jettison: view(%s): %v", userID, err)
		writeJSONError(w, "Не удалось получить трюм", http.StatusInternalServerError)
		return
	}
	writeJSON(w, view)
}
