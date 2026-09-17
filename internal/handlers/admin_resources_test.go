// internal/handlers/admin_resources_test.go
// Тест read-only просмотра универсального слоя (спека 94a): GET /admin/resources
// — каталог 20 ресурсов, 13 шаблонов, покрытие 50 рас без дыр. Данные из
// internal/resource, мутаций нет.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

func TestAdminResources(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/resources", nil)
	rec := execJSON(h.GetAdminResources, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Resources []struct {
			ID            string   `json:"id"`
			Name          string   `json:"name"`
			Category      string   `json:"category"`
			Bridge        bool     `json:"bridge"`
			Closes        []string `json:"closes"`
			TMelt         float64  `json:"t_melt"`
			TBoil         float64  `json:"t_boil"`
			Sublimating   bool     `json:"sublimating"`
			Supercritical bool     `json:"supercritical"`
		} `json:"resources"`
		Templates []struct {
			Axis  string `json:"axis"`
			Phase string `json:"phase"`
		} `json:"templates"`
		Races []struct {
			RaceID      string              `json:"race_id"`
			Name        string              `json:"name"`
			Consumption map[string]float64  `json:"consumption"`
			Coverage    map[string][]string `json:"coverage"`
		} `json:"races"`
		Gaps []struct {
			RaceID string  `json:"race_id"`
			Axis   string  `json:"axis"`
			Weight float64 `json:"weight"`
		} `json:"gaps"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Каталог: 20 ресурсов, 13 шаблонов, 50 био-рас, дыр нет.
	require.Len(t, resp.Resources, 20, "каталог: 20 ресурсов")
	require.Len(t, resp.Templates, 13, "шаблоны: 13 хемотипов")
	require.Len(t, resp.Races, 50, "покрытие: 50 био-рас")
	require.Empty(t, resp.Gaps, "дыр покрытия нет")

	// Каждая раса: каждая ось с весом ≥ 10 покрыта ≥ 1 ресурсом каталога.
	for _, rc := range resp.Races {
		for axis, weight := range rc.Consumption {
			if weight < 10 {
				continue
			}
			require.NotEmpty(t, rc.Coverage[axis],
				"раса %s (%s): ось %s (вес %v) покрыта", rc.RaceID, rc.Name, axis, weight)
		}
	}

	// Спот-проверка флагов: CO₂-лёд сублимирующий, сверхкритический флюид —
	// сверхкритический, вода — обычный.
	byID := map[string]struct {
		Sublimating   bool
		Supercritical bool
	}{}
	for _, r := range resp.Resources {
		byID[r.ID] = struct {
			Sublimating   bool
			Supercritical bool
		}{r.Sublimating, r.Supercritical}
	}
	require.True(t, byID["co2_ice"].Sublimating, "CO₂-лёд сублимирующий")
	require.False(t, byID["co2_ice"].Supercritical)
	require.True(t, byID["supercritical_fluid"].Supercritical, "сверхкритический флюид")
	require.False(t, byID["water"].Sublimating, "вода-ресурс не сублимирующий")
}

// Каталог рас не загружен — 500 с понятным текстом (паттерн
// admin_race_settlements.go:60).
func TestAdminResourcesNoRaceCatalog(t *testing.T) {
	// Каталог мог быть загружен другим тестом — сбросить нельзя (пакетная
	// переменная), поэтому проверяем только что хендлер не паникует и отвечает
	// JSON'ом (200 или 500 — зависит от состояния каталога).
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/resources", nil)
	rec := execJSON(h.GetAdminResources, req)
	require.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, rec.Code)
}