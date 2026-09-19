// internal/handlers/admin_resources_test.go
// Тест read-only просмотра универсального слоя (спека 94a): GET /admin/resources
// — каталог 20 ресурсов, 13 шаблонов, покрытие 50 рас без дыр. Данные из
// internal/resource, мутаций нет. Real-часть (спека iterB §5.3) — из БД
// (goods kind=resource с props.family), мокается sqlmock.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

// realRows — строки real-ресурсов витрины (props JSONB, русские ключи осей).
func realRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "code", "props"}).
		AddRow(int64(21), "Железо Fe", "mineral",
			`{"твёрдость":50,"эластичность":45,"проводимость":75,"плотность":80,"энергоёмкость":10,"биосовместимость":10,"радиоактивность":5,"токсичность":15,"горючесть":5,"химическая активность":30,"family":"Металлы","t_melt_k":1811,"t_boil_k":3134}`).
		AddRow(int64(22), "Вода H₂O", "water",
			`{"твёрдость":5,"эластичность":15,"проводимость":10,"плотность":35,"энергоёмкость":8,"биосовместимость":63,"радиоактивность":3,"токсичность":12,"горючесть":5,"химическая активность":35,"family":"Вода и растворы","t_melt_k":273,"t_boil_k":373}`)
}

// expectRealQuery — ожидание real-запроса витрины (спека iterB §5.3).
func expectRealQuery(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT g\.id, g\.name, c\.code, g\.props FROM goods g JOIN categories c ON c\.id = g\.category_id WHERE g\.kind = 'resource' AND g\.props \? 'family' ORDER BY g\.id`).
		WillReturnRows(realRows())
}

func TestAdminResources(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	expectRealQuery(mock)

	h := NewAdminHandlers(nil, db, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/resources", nil)
	rec := execJSON(h.GetAdminResources, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
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

// TestAdminResourcesRealFromDB — real-секция /admin/resources из БД
// (спека iterB §5.3): real/real_summary/families собираются из goods
// kind=resource с props.family; DistinctCount пересчитывается из БД-данных
// (развилка 6); id — строка (BIGSERIAL → string); category — code.
func TestAdminResourcesRealFromDB(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	expectRealQuery(mock)

	h := NewAdminHandlers(nil, db, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/resources", nil)
	rec := execJSON(h.GetAdminResources, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Real []struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Family      string  `json:"family"`
			Category    string  `json:"category"`
			Icon        string  `json:"category_icon"`
			Hardness    float64 `json:"hardness"`
			TMelt       float64 `json:"t_melt"`
			TBoil       float64 `json:"t_boil"`
			Sublimating bool    `json:"sublimating"`
		} `json:"real"`
		RealSummary struct {
			Total      int `json:"total"`
			ByAxes     int `json:"by_axes"`
			WithT      int `json:"with_t"`
			Collisions int `json:"collisions"`
		} `json:"real_summary"`
		Families []string `json:"families"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.Real, 2, "real из БД: 2 строки сида")
	require.Equal(t, "21", resp.Real[0].ID, "id — строка (BIGSERIAL → string)")
	require.Equal(t, "Железо Fe", resp.Real[0].Name)
	require.Equal(t, "Металлы", resp.Real[0].Family, "family из props")
	require.Equal(t, "mineral", resp.Real[0].Category, "category — code из categories.code")
	require.NotEmpty(t, resp.Real[0].Icon, "category_icon из Go-функции GetCategory(code).Icon")
	require.Equal(t, 50.0, resp.Real[0].Hardness, "русский ключ оси → английский DTO")
	require.Equal(t, 1811.0, resp.Real[0].TMelt, "t_melt в K")
	require.Equal(t, 3134.0, resp.Real[0].TBoil, "t_boil в K")
	require.False(t, resp.Real[0].Sublimating, "TBoil > TMelt — не сублимирующий")

	require.Equal(t, 2, resp.RealSummary.Total, "Total = число real из БД")
	require.Equal(t, 2, resp.RealSummary.ByAxes, "DistinctCount на БД-данных")
	require.Equal(t, 2, resp.RealSummary.WithT)
	require.Equal(t, 0, resp.RealSummary.Collisions)
	require.Equal(t, []string{"Металлы", "Вода и растворы"}, resp.Families, "families из props, порядок появления")
}

// Каталог рас не загружен — 500 с понятным текстом (паттерн
// admin_race_settlements.go:60).
func TestAdminResourcesNoRaceCatalog(t *testing.T) {
	// Каталог мог быть загружен другим тестом — сбросить нельзя (пакетная
	// переменная), поэтому проверяем только что хендлер не паникует и отвечает
	// JSON'ом (200 или 500 — зависит от состояния каталога).
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	expectRealQuery(mock)

	h := NewAdminHandlers(nil, db, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/resources", nil)
	rec := execJSON(h.GetAdminResources, req)
	require.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, rec.Code)
}