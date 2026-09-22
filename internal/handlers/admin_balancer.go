// internal/handlers/admin_balancer.go
// UI-инструмент балансировки компонент изменения населения (спека 99.2.17
// §6): кривые R(X) компонент (жара/холод/гравитация/радиация) — сегментные
// кривые из in-memory store (internal/economy/settlement/balancer_store.go).
// Эндпоинты:
//
//	GET  /admin/balancer/curve?component=heat    — узлы + изгибы;
//	PUT  /admin/balancer/curve                    — сохранить (валидация 422);
//	POST /admin/balancer/curve/reset              — сброс на дефолт §3;
//	POST /admin/balancer/curve/sample             — серверная оцифровка
//	      (клиент НЕ дублирует evaluateCurve — один факт, одно место);
//	GET  /admin/balancer/etalons?component=heat   — эталонные маркеры §4.
//
// Авторизация — auth.AdminAuth (как соседние admin-хендлеры, main.go).
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"zorion/internal/economy/settlement"
)

// balancerCurveResponse — кривая компоненты (GET/PUT/reset). Recovery —
// скаляр эффект-компоненты (`hunger`, §7.1): у hunger присутствует всегда, у
// не-эффект-компонент отсутствует (omitempty).
type balancerCurveResponse struct {
	Component string                   `json:"component"`
	Nodes     []settlement.SegmentNode `json:"nodes"`
	Bends     []float64                `json:"bends"`
	Recovery  *float64                 `json:"recovery,omitempty"`
}

// balancerCurveResponseFor — ответ с кривой и (для эффект-компонент) скаляром
// `recovery` из store (§7.1).
func balancerCurveResponseFor(component string, nodes []settlement.SegmentNode, bends []float64) balancerCurveResponse {
	resp := balancerCurveResponse{Component: component, Nodes: nodes, Bends: bends}
	if v, ok := settlement.ComponentScalar(component); ok {
		resp.Recovery = &v
	}
	return resp
}

// balancerEtalon — эталонный маркер-крестик (§4, фиксированные данные).
type balancerEtalon struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Label string  `json:"label"`
}

// balancerEtalons — фиксированные маркеры §4 (константы, не редактируются).
var balancerEtalons = map[string][]balancerEtalon{
	"heat": {
		{X: 200, Y: 0.00370, Label: "20% за минуты"},
		{X: 300, Y: 4.0e-3, Label: "мгновенная гибель, минуты"},
		{X: 500, Y: 0.4507, Label: "95% за секунды"},
		{X: 1000, Y: 0.6019, Label: "99% за секунды"},
		{X: 4000, Y: 0.6019, Label: "смерть за 5 секунд"},
	},
	"cold": {
		{X: -200, Y: 0.0117, Label: "−200 °C → минуты"},
	},
	"gravity": {
		{X: 0.1, Y: 6.6e-8, Label: "годы (атрофия)"},
		{X: 0.3, Y: 2.19e-8, Label: "~годы"},
		{X: 2, Y: 4.8e-5, Label: "дни–недели"},
		{X: 3, Y: 1.2e-4, Label: "дни"},
		{X: 5, Y: 4.8e-4, Label: "часы–дни"},
		{X: 10, Y: 1.92e-3, Label: "часы"},
	},
	"radiation": {
		{X: 40, Y: 6.7e-8, Label: "годы–десятилетия"},
		{X: 60, Y: 8.0e-6, Label: "недели–месяцы"},
		{X: 80, Y: 1.3e-4, Label: "«за часы» → дни"},
		{X: 100, Y: 9.5e-4, Label: "«за минуты» → часы"},
	},
}

// balancerComponentOK — проверка компоненты из допустимых (heat/cold/gravity/
// radiation/hunger, иначе 422).
func balancerComponentOK(w http.ResponseWriter, component string) bool {
	if settlement.ValidComponent(component) {
		return true
	}
	writeJSONError(w, fmt.Sprintf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component),
		http.StatusUnprocessableEntity)
	return false
}

// HandleBalancerCurve — диспетчер по методу для /admin/balancer/curve
// (GET — чтение, PUT — сохранение).
func (h *AdminHandlers) HandleBalancerCurve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetBalancerCurve(w, r)
	case http.MethodPut:
		h.PutBalancerCurve(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// GetBalancerCurve — GET /admin/balancer/curve?component=heat.
func (h *AdminHandlers) GetBalancerCurve(w http.ResponseWriter, r *http.Request) {
	component := r.URL.Query().Get("component")
	if !balancerComponentOK(w, component) {
		return
	}
	nodes, bends, ok := settlement.GetCurve(component)
	if !ok {
		writeJSONError(w, "кривая компоненты недоступна", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, balancerCurveResponseFor(component, nodes, bends))
}

// PutBalancerCurve — PUT /admin/balancer/curve: {component, nodes, bends}.
// Валидация §6 (3..16 узлов, изгибов = узлов − 1, X строго возрастают,
// y ∈ [0, 0.999), |k| ≤ 10, X в диапазоне компоненты) — все ошибки → 422
// с текстом причины, ничего не сохраняется. Идемпотентность: повторный PUT
// тех же данных — тот же результат. Ответ — сохранённая кривая.
func (h *AdminHandlers) PutBalancerCurve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Component string                   `json:"component"`
		Nodes     []settlement.SegmentNode `json:"nodes"`
		Bends     []float64                `json:"bends"`
		Recovery  *float64                 `json:"recovery"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if !balancerComponentOK(w, req.Component) {
		return
	}
	// Кривая и скаляр пишутся ОДНОЙ операцией store (атомарно, §7.1): при
	// ошибке валидации не меняется ни то, ни другое. `recovery` принимается
	// только эффект-компонентой (hunger), иначе 422.
	if err := settlement.SetCurveWithScalar(req.Component, settlement.ComponentCurve{
		Nodes: req.Nodes,
		Bends: req.Bends,
	}, req.Recovery); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	nodes, bends, _ := settlement.GetCurve(req.Component)
	writeJSONStatus(w, http.StatusOK, balancerCurveResponseFor(req.Component, nodes, bends))
}

// HandleBalancerCurveReset — POST /admin/balancer/curve/reset:
// {"component": "heat"}. Ответ — дефолтная кривая §3.
func (h *AdminHandlers) HandleBalancerCurveReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Component string `json:"component"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if !balancerComponentOK(w, req.Component) {
		return
	}
	if err := settlement.ResetCurveWithScalar(req.Component); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	nodes, bends, _ := settlement.GetCurve(req.Component)
	writeJSONStatus(w, http.StatusOK, balancerCurveResponseFor(req.Component, nodes, bends))
}

// HandleBalancerCurveSample — POST /admin/balancer/curve/sample:
// {"component": "heat", "xs": [...]}. Серверная оцифровка кривой для
// отрисовки — единый источник математики (клиент НЕ дублирует
// evaluateCurve/bendTransform). Валидация: 2..500 точек X; точки вне
// диапазона компоненты вычисляются горизонтальной экстраполяцией (§2) —
// это и есть цель sample. Query-параметр race_id (99.2.23 §4.3): задан —
// оцифровка active-кривой расы (расширение вкладки «Балансировка»).
func (h *AdminHandlers) HandleBalancerCurveSample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Component string    `json:"component"`
		Xs        []float64 `json:"xs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if !balancerComponentOK(w, req.Component) {
		return
	}
	if len(req.Xs) < 2 || len(req.Xs) > 500 {
		writeJSONError(w, fmt.Sprintf("число точек X должно быть в диапазоне 2..500, получили %d", len(req.Xs)),
			http.StatusUnprocessableEntity)
		return
	}
	var ys []float64
	var ok bool
	if raceID := r.URL.Query().Get("race_id"); raceID != "" {
		// Расовый режим принимает только расовые компоненты (§7.2): hunger —
		// глобальная, иначе SampleRaceCurve вернул бы nil → 500.
		if !balancerRaceComponentOK(w, req.Component) {
			return
		}
		if !raceBalancerRaceOK(w, raceID) {
			return
		}
		ys, ok = settlement.SampleRaceCurve(raceID, req.Component, req.Xs)
	} else {
		ys, ok = settlement.SampleCurve(req.Component, req.Xs)
	}
	if !ok {
		writeJSONError(w, "кривая компоненты недоступна", http.StatusInternalServerError)
		return
	}
	points := make([]settlement.SegmentNode, len(req.Xs))
	for i, x := range req.Xs {
		points[i] = settlement.SegmentNode{X: x, Y: ys[i]}
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"component": req.Component,
		"points":    points,
	})
}

// HandleBalancerEtalons — GET /admin/balancer/etalons?component=heat:
// фиксированные эталонные маркеры §4 (не редактируются).
func (h *AdminHandlers) HandleBalancerEtalons(w http.ResponseWriter, r *http.Request) {
	component := r.URL.Query().Get("component")
	if !balancerComponentOK(w, component) {
		return
	}
	// У hunger эталонов нет — пустой список (норма, §7.1), а не null.
	etalons := balancerEtalons[component]
	if etalons == nil {
		etalons = []balancerEtalon{}
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"component": component,
		"etalons":   etalons,
	})
}

// ==================== Пресеты (итерация 7, спека §6) ====================

// HandleBalancerPresets — диспетчер GET/POST/DELETE для /admin/balancer/presets.
func (h *AdminHandlers) HandleBalancerPresets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetBalancerPresets(w, r)
	case http.MethodPost:
		h.PostBalancerPresets(w, r)
	case http.MethodDelete:
		h.DeleteBalancerPresets(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// GetBalancerPresets — GET /admin/balancer/presets?component=heat:
// компактный список (имена + даты) + активный пресет (§6).
func (h *AdminHandlers) GetBalancerPresets(w http.ResponseWriter, r *http.Request) {
	component := r.URL.Query().Get("component")
	if !balancerComponentOK(w, component) {
		return
	}
	presets, active, ok := settlement.ListPresets(component)
	if !ok {
		writeJSONError(w, "кривая компоненты недоступна", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"component": component,
		"active":    active,
		"presets":   presets,
	})
}

// PostBalancerPresets — POST /admin/balancer/presets: {component, name}.
// «Сохранить как пресет»: кривая берётся из store (текущая), пишется в
// файл и сразу активна («сохранил = применил»). Перезапись имени
// разрешена. Ответ — сохранённый пресет {component, name, updated_at}.
func (h *AdminHandlers) PostBalancerPresets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Component string `json:"component"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if !balancerComponentOK(w, req.Component) {
		return
	}
	p, err := settlement.SavePreset(req.Component, req.Name)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"component":  p.Component,
		"name":       p.Name,
		"updated_at": p.UpdatedAt,
	})
}

// DeleteBalancerPresets — DELETE /admin/balancer/presets: {component, name}.
// Удаляет запись из файла; текущая кривая в store НЕ меняется. default →
// 422; пресет не найден → 404.
func (h *AdminHandlers) DeleteBalancerPresets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Component string `json:"component"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if !balancerComponentOK(w, req.Component) {
		return
	}
	if err := settlement.DeletePreset(req.Component, req.Name); err != nil {
		if errors.Is(err, settlement.ErrPresetNotFound) {
			writeJSONError(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// HandleBalancerPresetsApply — POST /admin/balancer/presets/apply:
// {component, name}. Пресет из файла → SetCurve (hot-swap) + active + файл.
// Не найден → 404; невалиден (файл правили руками) → 422. Ответ — кривая.
func (h *AdminHandlers) HandleBalancerPresetsApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Component string `json:"component"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if !balancerComponentOK(w, req.Component) {
		return
	}
	nodes, bends, err := settlement.ApplyPreset(req.Component, req.Name)
	if err != nil {
		if errors.Is(err, settlement.ErrPresetNotFound) {
			writeJSONError(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSONStatus(w, http.StatusOK, balancerCurveResponseFor(req.Component, nodes, bends))
}

// HandleBalancerPresetsResetDefault — POST /admin/balancer/presets/reset-default:
// {component}. «Вернуть заводской»: кодовые дефолты в store + пресет default
// перезаписан + active = default + файл. Ответ — дефолтная кривая.
func (h *AdminHandlers) HandleBalancerPresetsResetDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Component string `json:"component"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusUnprocessableEntity)
		return
	}
	if !balancerComponentOK(w, req.Component) {
		return
	}
	nodes, bends, err := settlement.ResetDefaultPreset(req.Component)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSONStatus(w, http.StatusOK, balancerCurveResponseFor(req.Component, nodes, bends))
}
