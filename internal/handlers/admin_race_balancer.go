// internal/handlers/admin_race_balancer.go
// Расовые R-кривые — админка (спека 99.2.23 §4.3): расширение вкладки
// «Балансировка» (99.2.17) селектором расы. Эндпоинты (префикс
// /admin/race-balancer/*, не ломает 99.2.17):
//
//	GET  /admin/race-balancer/curve?race_id=&component=   — active-кривая + мета
//	PUT  /admin/race-balancer/curve?race_id=&component=   — сохранить active-кривую
//	PUT  /admin/race-balancer/reproduction?race_id=       — сохранить active.reproduction
//	POST /admin/race-balancer/generate?race_id=           — перегенерировать factory из карточки
//	POST /admin/race-balancer/reset-factory?race_id=      — active = factory
//	GET  /admin/race-balancer/factory?race_id=&component= — factory-кривая (оверлей)
//	GET  /admin/race-balancer/status                      — список рас (селектор + пометки)
//
// Авторизация — auth.AdminAuth (как соседние admin-хендлеры, main.go).
package handlers

import (
	"encoding/json"
	"net/http"

	"zorion/internal/economy/settlement"
	"zorion/internal/races"
)

// raceCurveResponse — active-кривая расы + мета (спека §4.3 GET curve):
// reproduction, card_hash, factory_hash, «устарело ли», active==factory,
// ручки живого расчёта «рост в оптимуме» (k и R_ест — из настроек).
type raceCurveResponse struct {
	RaceID               string                   `json:"race_id"`
	Component            string                   `json:"component"`
	Nodes                []settlement.SegmentNode `json:"nodes"`
	Bends                []float64                `json:"bends"`
	Reproduction         float64                  `json:"reproduction"`
	CardHash             string                   `json:"card_hash"`
	FactoryHash          string                   `json:"factory_hash"`
	Stale                bool                     `json:"stale"`
	ActiveEqualsFactory  bool                     `json:"active_equals_factory"`
	BirthRateCoefficient float64                  `json:"birth_rate_coefficient"`
	NaturalRatePerSec    float64                  `json:"natural_rate_per_sec"`
}

// raceBalancerRaceOK — race_id: непустой, из каталога, не humans (спец-случай
// §2.3: записи "humans" в расовом store не создаются). Иначе 422.
func raceBalancerRaceOK(w http.ResponseWriter, raceID string) bool {
	if raceID == "" {
		writeJSONError(w, "race_id обязателен", http.StatusUnprocessableEntity)
		return false
	}
	if raceID == "humans" {
		writeJSONError(w, "humans — спец-случай (99.2.23 §2.3): расовый store не используется", http.StatusUnprocessableEntity)
		return false
	}
	if races.ByID(raceID) == nil {
		writeJSONError(w, "раса "+raceID+" не найдена в каталоге", http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// raceCurveMeta — сборка ответа GET curve / PUT curve из записи store.
func raceCurveMeta(raceID, component string, rec *settlement.RaceRecord) raceCurveResponse {
	race := races.ByID(raceID)
	stale := race != nil && rec.CardHash != settlement.CardHash(race)
	return raceCurveResponse{
		RaceID:               raceID,
		Component:            component,
		Nodes:                rec.Active.Curves[component].Nodes,
		Bends:                rec.Active.Curves[component].Bends,
		Reproduction:         rec.Active.Reproduction,
		CardHash:             rec.CardHash,
		FactoryHash:          settlement.FactoryHash(&rec.Factory),
		Stale:                stale,
		ActiveEqualsFactory:  settlement.RaceCurvesEqual(&rec.Active, &rec.Factory),
		BirthRateCoefficient: settlement.BirthRateCoefficient(),
		NaturalRatePerSec:    settlement.NaturalChangeRate(),
	}
}

// HandleRaceBalancerCurve — диспетчер GET/PUT для /admin/race-balancer/curve.
func (h *AdminHandlers) HandleRaceBalancerCurve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetRaceBalancerCurve(w, r)
	case http.MethodPut:
		h.PutRaceBalancerCurve(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// GetRaceBalancerCurve — GET /admin/race-balancer/curve?race_id=&component=:
// active-кривая + мета (reproduction, card_hash, factory_hash, stale,
// active==factory, ручки живого расчёта).
func (h *AdminHandlers) GetRaceBalancerCurve(w http.ResponseWriter, r *http.Request) {
	raceID := r.URL.Query().Get("race_id")
	component := r.URL.Query().Get("component")
	if !raceBalancerRaceOK(w, raceID) {
		return
	}
	if !balancerComponentOK(w, component) {
		return
	}
	rec, ok := settlement.GetRaceRecordMeta(raceID)
	if !ok {
		writeJSONError(w, "раса не найдена в расовом store", http.StatusNotFound)
		return
	}
	if rec.Active.Curves[component] == nil {
		writeJSONError(w, "кривая компоненты отсутствует", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, raceCurveMeta(raceID, component, rec))
}

// PutRaceBalancerCurve — PUT /admin/race-balancer/curve?race_id=&component=:
// {nodes, bends}. Валидация как PUT 99.2.17 §6, но без диапазона X компоненты
// (у расовых кривых свои диапазоны — сдвиг из карточки); ошибки → 422.
// Ответ — сохранённая кривая + мета (как GET).
func (h *AdminHandlers) PutRaceBalancerCurve(w http.ResponseWriter, r *http.Request) {
	raceID := r.URL.Query().Get("race_id")
	component := r.URL.Query().Get("component")
	if !raceBalancerRaceOK(w, raceID) {
		return
	}
	if !balancerComponentOK(w, component) {
		return
	}
	var req struct {
		Nodes []settlement.SegmentNode `json:"nodes"`
		Bends []float64                `json:"bends"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if err := settlement.SetRaceCurve(raceID, component, settlement.ComponentCurve{
		Nodes: req.Nodes,
		Bends: req.Bends,
	}); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	rec, _ := settlement.GetRaceRecordMeta(raceID)
	writeJSONStatus(w, http.StatusOK, raceCurveMeta(raceID, component, rec))
}

// HandleRaceBalancerReproduction — PUT /admin/race-balancer/reproduction?race_id=:
// {reproduction}. Валидация: множитель > 0 (иначе 422). Ответ — мета.
func (h *AdminHandlers) HandleRaceBalancerReproduction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	raceID := r.URL.Query().Get("race_id")
	if !raceBalancerRaceOK(w, raceID) {
		return
	}
	var req struct {
		Reproduction float64 `json:"reproduction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if err := settlement.SetRaceReproduction(raceID, req.Reproduction); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	rec, _ := settlement.GetRaceRecordMeta(raceID)
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"race_id":       raceID,
		"reproduction":  rec.Active.Reproduction,
		"card_hash":     rec.CardHash,
		"stale":         rec.CardHash != settlement.CardHash(races.ByID(raceID)),
		"factory_hash":  settlement.FactoryHash(&rec.Factory),
		"active_equals_factory": settlement.RaceCurvesEqual(&rec.Active, &rec.Factory),
	})
}

// HandleRaceBalancerGenerate — POST /admin/race-balancer/generate?race_id=:
// перегенерировать factory из текущей карточки (§3.3); active НЕ трогается
// (приоритет ручной правки, §2.4.4); card_hash обновляется. Ответ — мета.
func (h *AdminHandlers) HandleRaceBalancerGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	raceID := r.URL.Query().Get("race_id")
	if !raceBalancerRaceOK(w, raceID) {
		return
	}
	if err := settlement.RegenerateRaceFactory(raceID); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	rec, _ := settlement.GetRaceRecordMeta(raceID)
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"race_id":             raceID,
		"card_hash":           rec.CardHash,
		"stale":               false,
		"factory_hash":        settlement.FactoryHash(&rec.Factory),
		"active_equals_factory": settlement.RaceCurvesEqual(&rec.Active, &rec.Factory),
	})
}

// HandleRaceBalancerResetFactory — POST /admin/race-balancer/reset-factory?race_id=:
// active = factory (deep copy). Ответ — мета.
func (h *AdminHandlers) HandleRaceBalancerResetFactory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	raceID := r.URL.Query().Get("race_id")
	if !raceBalancerRaceOK(w, raceID) {
		return
	}
	if err := settlement.ResetRaceToFactory(raceID); err != nil {
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}
	rec, _ := settlement.GetRaceRecordMeta(raceID)
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"race_id":             raceID,
		"card_hash":           rec.CardHash,
		"stale":               rec.CardHash != settlement.CardHash(races.ByID(raceID)),
		"factory_hash":        settlement.FactoryHash(&rec.Factory),
		"active_equals_factory": true,
	})
}

// HandleRaceBalancerFactory — GET /admin/race-balancer/factory?race_id=&component=:
// factory-кривая (оверлей заводской пунктиром в UI).
func (h *AdminHandlers) HandleRaceBalancerFactory(w http.ResponseWriter, r *http.Request) {
	raceID := r.URL.Query().Get("race_id")
	component := r.URL.Query().Get("component")
	if !raceBalancerRaceOK(w, raceID) {
		return
	}
	if !balancerComponentOK(w, component) {
		return
	}
	rec, ok := settlement.GetRaceRecordMeta(raceID)
	if !ok {
		writeJSONError(w, "раса не найдена в расовом store", http.StatusNotFound)
		return
	}
	if rec.Factory.Curves[component] == nil {
		writeJSONError(w, "кривая компоненты отсутствует", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"race_id":   raceID,
		"component": component,
		"nodes":     rec.Factory.Curves[component].Nodes,
		"bends":     rec.Factory.Curves[component].Bends,
	})
}

// HandleRaceBalancerStatus — GET /admin/race-balancer/status: список рас
// каталога (кроме humans) — селектор расы в UI + пометки: has_record,
// card_hash_ok («заводские устарели»), active_equals_factory.
func (h *AdminHandlers) HandleRaceBalancerStatus(w http.ResponseWriter, r *http.Request) {
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"races": settlement.RaceStatusList(),
	})
}