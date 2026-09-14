// Тесты на вывод населения во вкладке «Миры» (admin_worlds.go):
// галактический итог с трендом, население в строке мира, сортировка по
// населению в обе стороны.
package handlers

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
)

// galaxyReportRows — 3 поселения на 2 мирах (температуры: 290 K — лёгкая
// убыль, 320 K — жара, 280 K — холод): итог 3500, тренд убыли, w1 = 1500,
// w2 = 2000.
func galaxyReportRows(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"w_id", "p_data", "population_exact", "computed_at", "created_at"}).
		AddRow("w1", `{"temperature":290,"gravity":1.0}`, 1000.0, now, now).
		AddRow("w1", `{"temperature":320,"gravity":1.0}`, 500.0, now, now).
		AddRow("w2", `{"temperature":280,"gravity":1.0}`, 2000.0, now, now)
}

const galaxyReportSQL = `
	SELECT w.id, p.data, s.population_exact, s.computed_at, s.created_at
	FROM settlements s
	JOIN planets p ON p.id = s.planet_id
	JOIN worlds w ON w.id = p.world_id`

func worldRow(id, name string, now time.Time) []driver.Value {
	return []driver.Value{id, name, 0.0, 0.0, "G", 5778, now, now}
}

// Default: без sort — пагинация по имени, итог по галактике + тренд, у мира
// население из отчёта (живой пересчёт на now).
func TestGetAllWorldsWithGalaxyPopulation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()

	// Отчёт по галактике.
	mock.ExpectQuery(galaxyReportSQL).WillReturnRows(galaxyReportRows(now))

	// Пагинация миров по имени (search пустой).
	mock.ExpectQuery(`SELECT COUNT(*) FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at FROM worlds ORDER BY name LIMIT $1 OFFSET $2`).
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "created_at", "updated_at"}).
			AddRow(worldRow("w1", "Альфа", now)...).
			AddRow(worldRow("w2", "Бета", now)...))

	h := &AdminHandlers{db: db, worldRepo: repository.NewWorldRepository(db)}
	req := httptest.NewRequest(http.MethodGet, "/admin/worlds?page=1&limit=50", nil)
	rec := httptest.NewRecorder()

	h.GetAllWorlds(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Data             []map[string]interface{} `json:"data"`
		GalaxyPopulation int64                    `json:"galaxy_population"`
		GalaxyTrend      string                   `json:"galaxy_trend"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Equal(t, int64(3500), resp.GalaxyPopulation)
	require.Equal(t, "decline", resp.GalaxyTrend)

	byID := map[string]float64{}
	trendByID := map[string]string{}
	for _, w := range resp.Data {
		byID[w["id"].(string)] = w["population"].(float64)
		trendByID[w["id"].(string)] = w["population_trend"].(string)
	}
	require.Equal(t, float64(1500), byID["w1"])
	require.Equal(t, float64(2000), byID["w2"])
	// Убыль по обоим мирам (все температуры дают r ≥ 0).
	require.Equal(t, "decline", trendByID["w1"])
	require.Equal(t, "decline", trendByID["w2"])
}

// sort=population&order=desc: миры сортируются по населению по убыванию
// (сортировка совпадает с показанными живыми значениями).
func TestGetAllWorldsSortByPopulationDesc(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()

	mock.ExpectQuery(galaxyReportSQL).WillReturnRows(galaxyReportRows(now))

	// Без пагинации грузятся все миры (GetAll), сортировка в Go.
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at FROM worlds ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "created_at", "updated_at"}).
			AddRow(worldRow("w1", "Альфа", now)...).
			AddRow(worldRow("w2", "Бета", now)...))

	h := &AdminHandlers{db: db, worldRepo: repository.NewWorldRepository(db)}
	req := httptest.NewRequest(http.MethodGet, "/admin/worlds?page=1&limit=50&sort=population&order=desc", nil)
	rec := httptest.NewRecorder()

	h.GetAllWorlds(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Data []struct {
			ID         string  `json:"id"`
			Population float64 `json:"population"`
		} `json:"data"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.Data, 2)
	require.Equal(t, 2, resp.Total)
	require.Equal(t, "w2", resp.Data[0].ID) // 2000 → первая
	require.Equal(t, "w1", resp.Data[1].ID) // 1500 → вторая
}

// sort=population&order=asc: сортировка по возрастанию.
func TestGetAllWorldsSortByPopulationAsc(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()

	mock.ExpectQuery(galaxyReportSQL).WillReturnRows(galaxyReportRows(now))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at FROM worlds ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "created_at", "updated_at"}).
			AddRow(worldRow("w1", "Альфа", now)...).
			AddRow(worldRow("w2", "Бета", now)...))

	h := &AdminHandlers{db: db, worldRepo: repository.NewWorldRepository(db)}
	req := httptest.NewRequest(http.MethodGet, "/admin/worlds?page=1&limit=50&sort=population&order=asc", nil)
	rec := httptest.NewRecorder()

	h.GetAllWorlds(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "w1", resp.Data[0].ID)
	require.Equal(t, "w2", resp.Data[1].ID)
}

// Пустая галактика (поселений нет): итог 0, тренд «stable», у миров 0.
func TestGetAllWorldsEmptyGalaxy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()

	mock.ExpectQuery(galaxyReportSQL).WillReturnRows(sqlmock.NewRows([]string{"w_id", "p_data", "population_exact", "computed_at", "created_at"}))
	mock.ExpectQuery(`SELECT COUNT(*) FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at FROM worlds ORDER BY name LIMIT $1 OFFSET $2`).
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "created_at", "updated_at"}).
			AddRow(worldRow("w1", "Альфа", now)...))

	h := &AdminHandlers{db: db, worldRepo: repository.NewWorldRepository(db)}
	req := httptest.NewRequest(http.MethodGet, "/admin/worlds?page=1&limit=50", nil)
	rec := httptest.NewRecorder()

	h.GetAllWorlds(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		GalaxyPopulation int64  `json:"galaxy_population"`
		GalaxyTrend      string `json:"galaxy_trend"`
		Data             []struct {
			ID              string  `json:"id"`
			Population      float64 `json:"population"`
			PopulationTrend string  `json:"population_trend"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(0), resp.GalaxyPopulation)
	require.Equal(t, "stable", resp.GalaxyTrend)
	require.Len(t, resp.Data, 1)
	require.Equal(t, float64(0), resp.Data[0].Population)
	// Мир без поселений — «stable» (нет населения, нет и тренда убыли).
	require.Equal(t, "stable", resp.Data[0].PopulationTrend)
}