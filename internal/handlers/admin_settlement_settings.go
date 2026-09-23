// internal/handlers/admin_settlement_settings.go
// Настройки рождаемости/СПЖ из админки (спека 99.2.16 §3.6): GET/PATCH
// /admin/settlement-settings. Хранятся в памяти пакета settlement
// (internal/economy/settlement/settings.go: дефолты СПЖ 50 лет, k=2; после
// рестарта — снова дефолты). Прецедент — internal/handlers/npc_settings.go.
package handlers

import (
	"encoding/json"
	"net/http"

	"zorion/internal/economy/settlement"
	"zorion/internal/repository"
)

// HandleSettlementSettings — диспетчер по методу для /admin/settlement-settings.
func (h *AdminHandlers) HandleSettlementSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetSettlementSettings(w, r)
	case http.MethodPatch:
		h.PatchSettlementSettings(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// settlementSettingsResponse — текущие значения + производные для UI
// (единое место расчёта на сервере, фронт не дублирует формулу, §3.6):
// natural_change_rate_per_sec = 1/СПЖ_сек,
// netto_per_year_percent = (1−k)/СПЖ_лет × 100 (минус = рост).
func settlementSettingsResponse() map[string]interface{} {
	return map[string]interface{}{
		"life_expectancy_years":      settlement.LifeExpectancyYears(),
		"birth_rate_coefficient":     settlement.BirthRateCoefficient(),
		"natural_change_rate_per_sec": settlement.NaturalChangeRate(),
		"netto_per_year_percent":     settlement.NettoPerYearPercent(),
	}
}

func (h *AdminHandlers) GetSettlementSettings(w http.ResponseWriter, r *http.Request) {
	writeJSONStatus(w, http.StatusOK, settlementSettingsResponse())
}

// PatchSettlementSettings — PATCH /admin/settlement-settings:
// {life_expectancy_years?, birth_rate_coefficient?,
//  settlement_arithmetic_visible_to_player?}. Значения СПЖ/k валидируются
// сеттерами (СПЖ 1..500, k 0..10; без клампа — ошибка 400 с текстом), текущие
// значения при ошибке не меняются. Видимость арифметики игроку (спека
// 2026-09-23-стадии-поселения §8.3/§10) — persistent-ключ generation_config;
// выключение действует немедленно (сервер не сериализует блок).
func (h *AdminHandlers) PatchSettlementSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LifeExpectancyYears  *float64 `json:"life_expectancy_years"`
		BirthRateCoefficient *float64 `json:"birth_rate_coefficient"`
		ArithmeticVisible    *bool    `json:"settlement_arithmetic_visible_to_player"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.LifeExpectancyYears == nil && req.BirthRateCoefficient == nil && req.ArithmeticVisible == nil {
		writeJSONError(w, "Нет полей для обновления", http.StatusBadRequest)
		return
	}
	if req.LifeExpectancyYears != nil {
		if err := settlement.SetLifeExpectancyYears(*req.LifeExpectancyYears); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.BirthRateCoefficient != nil {
		if err := settlement.SetBirthRateCoefficient(*req.BirthRateCoefficient); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.ArithmeticVisible != nil {
		if err := repository.SetSettlementArithmeticVisibleToPlayer(h.db, *req.ArithmeticVisible); err != nil {
			writeJSONError(w, "Не удалось сохранить настройку: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	resp := settlementSettingsResponse()
	if req.ArithmeticVisible != nil {
		resp["settlement_arithmetic_visible_to_player"] = *req.ArithmeticVisible
	}
	writeJSONStatus(w, http.StatusOK, resp)
}
