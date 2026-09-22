// internal/handlers/admin_settlement_effects.go
//
// Админ-инструмент «задать нагрузку эффекта вручную» (спека
// 2026-09-22-эффекты-снабжения-задержка-голод §6, F9 = A): песочница —
// перезапись `load` действующего эффекта поселения с новым базисом `load_at`.
// Плюс диспетчер пути /admin/settlements/ по суффиксу (…/branches —
// существующий AddBranch, …/effects — новая ручка). JWT admin/skycomposer,
// гейты мутаций вселенной (Пакман/universeMutationMu).
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"zorion/internal/generator"
	"zorion/internal/repository"
)

// settlementEffectsID — id поселения из /admin/settlements/{id}/effects.
func settlementEffectsID(path string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/admin/settlements/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "effects" {
		return "", false
	}
	return parts[0], true
}

// HandleSettlementRoute — диспетчер по суффиксу под /admin/settlements/:
// …/effects → SetSettlementEffectLoad; всё прочее (в т.ч. …/branches) →
// AddBranch — прежнее поведение сохранено. Хвостовой слэш у эффект-пути
// (…/{id}/effects/) — осмысленный отказ, а не молчаливый уход в ветки
// (AddBranch вернул бы сбивающее «settlement id required» про ветки на
// опечатку в пути эффекта; мелочь ревью этапа 3, §6).
func (h *AdminHandlers) HandleSettlementRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/effects/") {
		http.Error(w, "лишний слэш в конце пути эффекта", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/effects") {
		h.SetSettlementEffectLoad(w, r)
		return
	}
	h.AddBranch(w, r)
}

// SetSettlementEffectLoad — POST /admin/settlements/{id}/effects (§6 F9).
// Тело {effect_type_id, load}. Отрицательная нагрузка / нет effect_type_id →
// 422; действующего эффекта нет → 404. Гейты мутаций вселенной — как у
// соседних ручек: проверка + действие атомарны.
func (h *AdminHandlers) SetSettlementEffectLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	settlementID, ok := settlementEffectsID(r.URL.Path)
	if !ok {
		http.Error(w, "settlement id required", http.StatusBadRequest)
		return
	}
	var body struct {
		EffectTypeID int64    `json:"effect_type_id"`
		Load         *float64 `json:"load"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if body.EffectTypeID <= 0 {
		http.Error(w, "effect_type_id обязателен", http.StatusUnprocessableEntity)
		return
	}
	if body.Load == nil || *body.Load < 0 {
		http.Error(w, "load должен быть ≥ 0", http.StatusUnprocessableEntity)
		return
	}
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}
	if !universeMutationMu.TryLock() {
		http.Error(w, "Universe mutation is running, wait for it", http.StatusConflict)
		return
	}
	defer universeMutationMu.Unlock()

	if err := repository.NewEffectRepository(h.db).SetSettlementEffectLoad(
		settlementID, body.EffectTypeID, *body.Load); err != nil {
		writeCatalogErr(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"settlement_id":  settlementID,
		"effect_type_id": body.EffectTypeID,
		"load":           *body.Load,
	})
}
