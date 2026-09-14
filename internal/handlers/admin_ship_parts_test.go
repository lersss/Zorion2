// internal/handlers/admin_ship_parts_test.go
// Тесты вкладки «Корабли» (спека 99.2.15 §10): список с данными предпросмотра
// (гейт И9), генерация пачки, удаление, перегенерация категории. БД — sqlmock;
// каталог в памяти — настоящий (перезагрузка после генерации).
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator/ship"
	"zorion/internal/repository"
)

// loadShipTestConfig — конфиг генератора из репозитория (как generator_test).
func loadShipTestConfig(t *testing.T) {
	t.Helper()
	_, err := ship.LoadConfig("../../config/ship_visual.json")
	require.NoError(t, err)
}

// newAdminShipPartsHarness — sqlmock-БД + настоящий каталог в памяти.
func newAdminShipPartsHarness(t *testing.T) (*AdminShipPartsHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	catalog := repository.NewShipCatalog(testPalette, testLayerOrder)
	return NewAdminShipPartsHandlers(repository.NewShipRepository(db), catalog), mock
}

// ==================== GET /admin/ship-parts ====================

func TestAdminShipPartsList(t *testing.T) {
	h, mock := newAdminShipPartsHarness(t)
	created := now()

	mock.ExpectQuery(`SELECT .* FROM ship_parts ORDER BY category, id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category", "name", "svg", "params", "created_at"}).
			AddRow("hull_min", "hull", "Брус", "<path/>", `{"w":65,"h":42}`, created).
			AddRow("hull_max", "hull", "Щит", "<path/>", `{"w":110,"h":60}`, created).
			AddRow("hull_mid", "hull", "Стрела", "<path/>", `{"w":85,"h":51}`, created).
			AddRow("nose_long", "nose", "Клин", "<path/>", `{"l":70,"b":40}`, created).
			AddRow("wings_big", "wings", "Размах", "<path/>", `{"r":50,"c":38}`, created).
			AddRow("engine_big", "engine", "Факел", "<path/>", `{"l":60,"b":40}`, created).
			AddRow("tail_big", "tail", "Киль", "<path/>", `{"ht":55,"bw":40}`, created))

	req := httptest.NewRequest(http.MethodGet, "/admin/ship-parts", nil)
	rec := execJSON(h.List, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Parts []struct {
			ID        string      `json:"id"`
			Params    interface{} `json:"params"`
			CreatedAt time.Time   `json:"created_at"`
		} `json:"parts"`
		Palette    []string          `json:"palette"`
		LayerOrder []string          `json:"layerOrder"`
		Hulls      map[string]string `json:"hulls"`
		Neutral    map[string]string `json:"neutral_parts"`
		Worst      []struct {
			Key   string            `json:"key"`
			Parts map[string]string `json:"parts"`
		} `json:"worst_cases"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Parts, 7)
	require.Equal(t, "hull_min", resp.Parts[0].ID)
	require.NotNil(t, resp.Parts[0].Params, "админка получает параметры генерации")
	require.Equal(t, created, resp.Parts[0].CreatedAt)
	require.Equal(t, testPalette, resp.Palette)
	require.Equal(t, testLayerOrder, resp.LayerOrder)

	// Стрип: три корпуса (нейтральный/min/max по площади) + нейтральные придатки.
	require.Equal(t, "hull_min", resp.Hulls["min"])
	require.Equal(t, "hull_max", resp.Hulls["max"])
	require.Equal(t, "hull_mid", resp.Hulls["neutral"])
	require.Equal(t, "nose_long", resp.Neutral["nose"])

	// Worst-case сэмплер: 4 сборки у границ габаритов (§3.1), все на min-корпусе.
	require.Len(t, resp.Worst, 4)
	byKey := map[string]map[string]string{}
	for _, w := range resp.Worst {
		byKey[w.Key] = w.Parts
	}
	require.Equal(t, "hull_min", byKey["nose_max_hull_min"]["hull"])
	require.Equal(t, "nose_long", byKey["nose_max_hull_min"]["nose"])
	require.Equal(t, "wings_big", byKey["wings_max_hull_min"]["wings"])
	require.Equal(t, "engine_big", byKey["engine_max_hull_min"]["engine"])
	require.Equal(t, "tail_big", byKey["tail_max_hull_min"]["tail"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminShipPartsListEmpty(t *testing.T) {
	h, mock := newAdminShipPartsHarness(t)
	mock.ExpectQuery(`SELECT .* FROM ship_parts ORDER BY category, id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category", "name", "svg", "params", "created_at"}))

	req := httptest.NewRequest(http.MethodGet, "/admin/ship-parts", nil)
	rec := execJSON(h.List, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Parts      []json.RawMessage `json:"parts"`
		Hulls      map[string]string `json:"hulls"`
		WorstCases []json.RawMessage `json:"worst_cases"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Empty(t, resp.Parts)
	require.Empty(t, resp.Hulls, "без корпусов — нет данных стрипа")
	require.Nil(t, resp.WorstCases, "без корпусов — нет worst-case сборок")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== POST /admin/ship-parts/generate ====================

func TestAdminShipPartsGenerate(t *testing.T) {
	h, mock := newAdminShipPartsHarness(t)
	loadShipTestConfig(t)

	mock.ExpectExec(`INSERT INTO ship_parts`).WillReturnResult(sqlmock.NewResult(0, 1))
	// Перезагрузка каталога в память после генерации.
	mock.ExpectQuery(`SELECT .* FROM ship_parts ORDER BY category, id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category", "name", "svg", "params", "created_at"}))

	req := httptest.NewRequest(http.MethodPost, "/admin/ship-parts/generate",
		strings.NewReader(`{"category":"nose","count":1,"seed":42}`))
	rec := execJSON(h.Generate, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp struct {
		Parts []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
			Name     string `json:"name"`
			SVG      string `json:"svg"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Parts, 1)
	require.Equal(t, "nose", resp.Parts[0].Category)
	require.NotEmpty(t, resp.Parts[0].ID)
	require.NotEmpty(t, resp.Parts[0].SVG)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminShipPartsGenerateInvalidCategory(t *testing.T) {
	h, _ := newAdminShipPartsHarness(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/ship-parts/generate",
		strings.NewReader(`{"category":"laser","count":1}`))
	rec := execJSON(h.Generate, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== DELETE /admin/ship-parts/{id} ====================

func TestAdminShipPartsDelete(t *testing.T) {
	h, mock := newAdminShipPartsHarness(t)

	mock.ExpectExec(`DELETE FROM ship_parts`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM ship_parts ORDER BY category, id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category", "name", "svg", "params", "created_at"}))

	req := httptest.NewRequest(http.MethodDelete, "/admin/ship-parts/nose_x", nil)
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminShipPartsDeleteNotFound(t *testing.T) {
	h, mock := newAdminShipPartsHarness(t)

	mock.ExpectExec(`DELETE FROM ship_parts`).WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodDelete, "/admin/ship-parts/nose_x", nil)
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== POST /admin/ship-parts/regenerate-category ====================

func TestAdminShipPartsRegenerateCategory(t *testing.T) {
	h, mock := newAdminShipPartsHarness(t)
	loadShipTestConfig(t)

	// 10 новых форм + перезагрузка каталога.
	for i := 0; i < 10; i++ {
		mock.ExpectExec(`INSERT INTO ship_parts`).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	// Старые детали категории: одна вне новой десятки — удаляется.
	mock.ExpectQuery(`SELECT .* FROM ship_parts WHERE category = \$1`).
		WithArgs("nose").
		WillReturnRows(sqlmock.NewRows([]string{"id", "category", "name", "svg", "params", "created_at"}).
			AddRow("nose_old", "nose", "Старый", "<path/>", `{}`, now()))
	mock.ExpectExec(`DELETE FROM ship_parts`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM ship_parts ORDER BY category, id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category", "name", "svg", "params", "created_at"}))

	req := httptest.NewRequest(http.MethodPost, "/admin/ship-parts/regenerate-category",
		strings.NewReader(`{"category":"nose"}`))
	rec := execJSON(h.RegenerateCategory, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Parts []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Parts, 10)
	for _, p := range resp.Parts {
		require.Equal(t, "nose", p.Category)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}