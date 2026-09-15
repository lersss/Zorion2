// internal/handlers/admin_settlement_settings_test.go
// Тесты ручек настроек населения (спека 99.2.16 §3.6): GET дефолтов с
// производными, PATCH успех, невалид → 400 с текстом, пустое тело → 400,
// не-PATCH → 405.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
)

func TestAdminSettlementSettingsGet(t *testing.T) {
	t.Cleanup(settlement.ResetPopulationSettings)
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/settlement-settings", nil)
	rec := execJSON(h.GetSettlementSettings, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		LifeExpectancyYears     float64 `json:"life_expectancy_years"`
		BirthRateCoefficient    float64 `json:"birth_rate_coefficient"`
		NaturalChangeRatePerSec float64 `json:"natural_change_rate_per_sec"`
		NettoPerYearPercent     float64 `json:"netto_per_year_percent"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 50.0, resp.LifeExpectancyYears, "дефолт СПЖ, спека §3.2")
	require.Equal(t, 2.0, resp.BirthRateCoefficient, "дефолт k, спека §3.2")
	require.InDelta(t, settlement.NaturalChangeRate(), resp.NaturalChangeRatePerSec, 1e-20)
	require.InDelta(t, -2.0, resp.NettoPerYearPercent, 1e-9, "(1−2)/50×100 = −2.0")
}

func TestAdminSettlementSettingsPatch(t *testing.T) {
	t.Cleanup(settlement.ResetPopulationSettings)
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodPatch, "/admin/settlement-settings",
		strings.NewReader(`{"life_expectancy_years":80,"birth_rate_coefficient":3}`))
	rec := execJSON(h.PatchSettlementSettings, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		LifeExpectancyYears  float64 `json:"life_expectancy_years"`
		BirthRateCoefficient float64 `json:"birth_rate_coefficient"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 80.0, resp.LifeExpectancyYears)
	require.Equal(t, 3.0, resp.BirthRateCoefficient)
	require.Equal(t, 80.0, settlement.LifeExpectancyYears(), "применяется сразу")
	require.Equal(t, 3.0, settlement.BirthRateCoefficient())
}

func TestAdminSettlementSettingsPatchInvalid(t *testing.T) {
	t.Cleanup(settlement.ResetPopulationSettings)
	h := &AdminHandlers{}

	// СПЖ вне 1..500 — 400 с текстом (§3.3, без клампа).
	req := httptest.NewRequest(http.MethodPatch, "/admin/settlement-settings",
		strings.NewReader(`{"life_expectancy_years":501}`))
	rec := execJSON(h.PatchSettlementSettings, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "1..500")

	// k вне 0..10 — 400 с текстом.
	req = httptest.NewRequest(http.MethodPatch, "/admin/settlement-settings",
		strings.NewReader(`{"birth_rate_coefficient":11}`))
	rec = execJSON(h.PatchSettlementSettings, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "0..10")

	// Значения не меняются при ошибке.
	require.Equal(t, 50.0, settlement.LifeExpectancyYears())
	require.Equal(t, 2.0, settlement.BirthRateCoefficient())

	// Пустое тело — 400 (прецедент npc_settings).
	req = httptest.NewRequest(http.MethodPatch, "/admin/settlement-settings",
		strings.NewReader(`{}`))
	rec = execJSON(h.PatchSettlementSettings, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminSettlementSettingsMethodNotAllowed(t *testing.T) {
	t.Cleanup(settlement.ResetPopulationSettings)
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodPost, "/admin/settlement-settings", nil)
	rec := execJSON(h.HandleSettlementSettings, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}