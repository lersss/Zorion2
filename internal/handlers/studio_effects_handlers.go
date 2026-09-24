// internal/handlers/studio_effects_handlers.go
// HTTP-хендлеры /studio/api/effects* (спека 2026-09-22-эффекты-снабжения-
// задержка-голод §7.4): CRUD типа эффекта студии. Тип несёт только имя, impact
// (открытый набор) и params.curve — ссылку на компоненту «Балансировки»
// (recovery в типе не хранится — скаляр Балансировки). Ошибки — ErrCatalog
// (400/404/409) тем же writeCatalogErr, что остальная студия.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"zorion/internal/repository"
)

// EffectTypeView — тип эффекта в ответах студии.
type EffectTypeView struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	NameNorm string `json:"name_norm"`
	Impact   string `json:"impact"`
	Curve    string `json:"curve"`
	// Code — метка переноса (effect_types.code, спека 2026-09-24 §3.1/§3.3):
	// справочная, только чтение; NULL в БД → поле отсутствует (omitempty).
	Code string `json:"code,omitempty"`
}

// effectTypeView — EffectTypeRow → EffectTypeView.
func effectTypeView(e repository.EffectTypeRow) EffectTypeView {
	return EffectTypeView{ID: e.ID, Name: e.Name, NameNorm: e.NameNorm, Impact: e.Impact, Curve: e.Curve, Code: e.Code.String}
}

// Effects — GET (каталог типов) / POST (создание) /studio/api/effects.
func (h *StudioHandlers) Effects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.effects.EffectTypes()
		if err != nil {
			studioErr(w, "ошибка чтения типов эффектов: "+err.Error(), http.StatusInternalServerError)
			return
		}
		views := make([]EffectTypeView, 0, len(list))
		for _, e := range list {
			views = append(views, effectTypeView(e))
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{"effects": views})
	case http.MethodPost:
		var body struct {
			Name   string `json:"name"`
			Impact string `json:"impact"`
			Curve  string `json:"curve"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		e, err := h.effects.CreateEffectType(body.Name, body.Impact, body.Curve)
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusCreated, effectTypeView(e))
	default:
		studioErr(w, "только GET/POST", http.StatusMethodNotAllowed)
	}
}

// EffectByID — PUT/DELETE /studio/api/effects/{id} и GET
// /studio/api/effects/{id}/counts (счётчики привязок типа, §7.4).
func (h *StudioHandlers) EffectByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/studio/api/effects/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	id, err := parseID(parts[0])
	if err != nil {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	if len(parts) == 2 && parts[1] == "counts" {
		if r.Method != http.MethodGet {
			studioErr(w, "только GET", http.StatusMethodNotAllowed)
			return
		}
		active, producers, err := h.effects.EffectBindingsCount(id)
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int{"active_effects": active, "producers": producers})
		return
	}
	if len(parts) != 1 {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name   *string `json:"name"`
			Impact *string `json:"impact"`
			Curve  *string `json:"curve"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := h.effects.UpdateEffectType(id, body.Name, body.Impact, body.Curve); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"id": id})
	case http.MethodDelete:
		if err := h.effects.DeleteEffectType(id); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"deleted": id})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}
