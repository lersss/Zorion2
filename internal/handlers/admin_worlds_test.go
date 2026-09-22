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
// w2 = 2000. race_id — NULL (человеческая модель, 99.2.23 §2.2), effects —
// NULL (привязок нет, R = 0).
func galaxyReportRows(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"w_id", "p_data", "id", "population_exact", "computed_at", "created_at", "race_id", "effects"}).
		AddRow("w1", `{"temperature":290,"gravity":1.0}`, "s1", 1000.0, now, now, nil, nil).
		AddRow("w1", `{"temperature":320,"gravity":1.0}`, "s2", 500.0, now, now, nil, nil).
		AddRow("w2", `{"temperature":280,"gravity":1.0}`, "s3", 2000.0, now, now, nil, nil)
}

const galaxyReportSQL = `
	SELECT w.id, p.data, s.id, s.population_exact, s.computed_at, s.created_at, s.race_id,
	       pt.params->'effects'
	FROM settlements s
	JOIN planets p ON p.id = s.planet_id
	JOIN worlds w ON w.id = p.world_id
	LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id`

// expectGalaxyActiveEffects — хранимая нагрузка поселений отчёта (пусто).
func expectGalaxyActiveEffects(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`
		SELECT ae.effect_type_id, COALESCE(ae.source_position, ''), ae.load, ae.load_at,
		       et.impact, COALESCE(et.params->>'curve', ''), ae.owner_id
		FROM active_effects ae
		JOIN effect_types et ON et.id = ae.effect_type_id
		WHERE ae.owner_type = 'settlement' AND ae.owner_id = ANY($1)
	`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"effect_type_id", "source_position", "load", "load_at", "impact", "curve", "owner_id"}))
}

func worldRow(id, name string, now time.Time) []driver.Value {
	return []driver.Value{id, name, 0.0, 0.0, "G", 5778, "star", "single", nil, nil, nil, now, now}
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
	expectGalaxyActiveEffects(mock)

	// Пагинация миров по имени (search пустой).
	mock.ExpectQuery(`SELECT COUNT(*) FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE(spectral_class,''), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds ORDER BY name LIMIT $1 OFFSET $2`).
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "star_type", "system_type", "stellar_mods", "stellar_mass", "age", "created_at", "updated_at"}).
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
	expectGalaxyActiveEffects(mock)

	// Без пагинации грузятся все миры (GetAll), сортировка в Go.
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE(spectral_class,''), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "star_type", "system_type", "stellar_mods", "stellar_mass", "age", "created_at", "updated_at"}).
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
	expectGalaxyActiveEffects(mock)
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE(spectral_class,''), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "star_type", "system_type", "stellar_mods", "stellar_mass", "age", "created_at", "updated_at"}).
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

	mock.ExpectQuery(galaxyReportSQL).WillReturnRows(sqlmock.NewRows([]string{"w_id", "p_data", "id", "population_exact", "computed_at", "created_at", "race_id", "effects"}))
	mock.ExpectQuery(`SELECT COUNT(*) FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE(spectral_class,''), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds ORDER BY name LIMIT $1 OFFSET $2`).
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "star_type", "system_type", "stellar_mods", "stellar_mass", "age", "created_at", "updated_at"}).
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