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
	"zorion/internal/repository"
	"zorion/internal/routegame"
	"zorion/internal/ship"
)

// accelerator_grid_scan.go — ручка разведки сектора POST /api/accelerator/scan
// (§14.4). Вынесено из accelerator_grid_handlers.go без изменения поведения.

// AcceleratorScan — POST /api/accelerator/scan (§14.4): вскрыть сектор серверным
// secret (RevealGridSector) и атомарно списать импульс (Repository.Reveal).
// Гейты — как у offer/boost (нет полёта, модуль/игра, active, fingerprint), но
// БЕЗ порогов остатка и отката: разведка их не тратит. Импульсов нет → 409.
func (h *TravelHandlers) AcceleratorScan(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	var req acceleratorScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректный запрос", http.StatusBadRequest)
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
	if req.Fingerprint != acceleratorFingerprint(flight) {
		writeAcceleratorRefusal(w, http.StatusConflict, "changed", 0)
		return
	}
	state, err := h.accelRepo.GetState(userID)
	if err != nil {
		log.Printf("⚠️ accelerator scan: state (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось прочитать состояние ускорителя", http.StatusInternalServerError)
		return
	}
	cooldownLeft := models.CooldownRemaining(state.LastBoostAt, state.LastCooldownMin, now)
	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	_, cfg, found, valid := ship.AcceleratorModule(user.Equipment)
	// Разведка не тратит ни остаток, ни откат — единый гейт берётся без порогов
	// (minRemainingS=0, checkCooldown=false) при общем порядке причин §3.4/§4.1.
	if reason := acceleratorTravelReason(found, valid, ship.AcceleratorGameRegistered(cfg.Game), models.BoostActive(state.LastBoostAt, flight.StartTime), 0, 0, cooldownLeft, false); reason != "" {
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
	_, st, _, err := h.acceleratorGridSegment(userID, flight, from, target)
	if err != nil {
		if errors.Is(err, errGridNoField) {
			writeAcceleratorRefusal(w, http.StatusConflict, "no_field", cooldownLeft)
			return
		}
		log.Printf("⚠️ accelerator scan: puzzle (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось получить доску", http.StatusInternalServerError)
		return
	}
	var layout routegame.GridLayout
	if err := json.Unmarshal(st.Layout, &layout); err != nil {
		log.Printf("⚠️ accelerator scan: layout (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось прочитать доску", http.StatusInternalServerError)
		return
	}
	if req.Sector < 0 || req.Sector >= len(layout.Sectors) {
		writeAcceleratorRefusal(w, http.StatusBadRequest, "bad_sector", cooldownLeft)
		return
	}
	content, err := routegame.RevealGridSector(st.Secret, layout, req.Sector)
	if err != nil {
		log.Printf("⚠️ accelerator scan: reveal (user %s, sector %d): %v", userID, req.Sector, err)
		writeJSONError(w, "Не удалось вскрыть сектор", http.StatusInternalServerError)
		return
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		writeJSONError(w, "Не удалось вскрыть сектор", http.StatusInternalServerError)
		return
	}
	revealed, pingsLeft, err := h.routePuzzleRepo.Reveal(context.Background(), userID, acceleratorPuzzleKind, req.Sector, string(contentJSON))
	if errors.Is(err, repository.ErrNoPingsLeft) {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_pings", cooldownLeft)
		return
	}
	if err != nil {
		log.Printf("⚠️ accelerator scan: reveal db (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось списать импульс", http.StatusInternalServerError)
		return
	}
	writeJSON(w, acceleratorScanResponse{
		Fingerprint: acceleratorFingerprint(flight),
		Sector:      req.Sector,
		Content:     content,
		PingsLeft:   pingsLeft,
		Revealed:    acceleratorRevealedList(revealed),
	})
}
