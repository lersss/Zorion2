// Тесты на предпросмотр смерти населения от среды (admin_mortality.go).
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
	"zorion/internal/races"
)

func TestMortalityPreviewMissingPlanetID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/admin/mortality-preview", nil)
	rec := httptest.NewRecorder()

	h.MortalityPreview(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMortalityPreviewPlanetNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("missing").WillReturnError(sql.ErrNoRows)

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/admin/mortality-preview?planet_id=missing", nil)
	rec := httptest.NewRecorder()

	h.MortalityPreview(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMortalityPreviewComfortablePlanetHasZeroLambda(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("p1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Уютная", 1, `{"temperature":288,"gravity":1.0}`, now, now),
	)
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, now, now, now, nil),
	)
	// Ветки поселения (спека 2026-09-22-поселение-ветка-буферы-переработка
	// §4.2): у поселения s1 веток нет — пустая выборка (attachBranches идёт
	// ПОСЛЕ пересчёта населения и до чтения лога).
	mock.ExpectQuery(`SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at, r.good_id, og.name, r.complexity FROM settlement_branches b JOIN recipes r ON r.id = b.recipe_id JOIN goods og ON og.id = r.good_id WHERE b.settlement_id = ANY($1) ORDER BY b.created_at ASC, b.id ASC`).
		WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity"}))
	// attachSettlements читает лог поселения (18b §«Лог поселения») — пусто.
	mock.ExpectQuery(`
		SELECT id, settlement_id, type, occurred_at, cause, created_at
		FROM (
			SELECT id, settlement_id, type, occurred_at, cause, created_at,
			       ROW_NUMBER() OVER (PARTITION BY settlement_id ORDER BY occurred_at DESC) AS rn
			FROM settlement_log
			WHERE settlement_id = ANY($1)
		) sub
		WHERE rn <= 3
		ORDER BY occurred_at DESC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "type", "occurred_at", "cause", "created_at"}))

	// Фракции/строения планеты (спека 2026-09-21-фабрики-релиз-2) — пусто.
	expectEmptyFactionsBuildingsExact(mock)

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/admin/mortality-preview?planet_id=p1", nil)
	rec := httptest.NewRecorder()

	h.MortalityPreview(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		P0            float64            `json:"p0"`
		LambdaPerHour float64            `json:"lambda_per_hour"`
		Projection    map[string]float64 `json:"projection"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(1_000_000), resp.P0)
	require.Equal(t, float64(0), resp.LambdaPerHour, "комфортная планета не должна убивать")
	require.Equal(t, float64(1_000_000), resp.Projection["1 год"], "без угрозы население не меняется")
}

func TestMortalityPreviewP0Override(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("p1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Без поселений", 1, `{"temperature":288,"gravity":1.0}`, now, now),
	)
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}),
	)

	// Фракции/строения планеты (спека 2026-09-21-фабрики-релиз-2) — пусто.
	expectEmptyFactionsBuildingsExact(mock)

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/admin/mortality-preview?planet_id=p1&p0=500", nil)
	rec := httptest.NewRecorder()

	h.MortalityPreview(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		P0 float64 `json:"p0"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(500), resp.P0, "p0 из query должен переопределять население планеты")
}

// Тест 14 (§14): предпросмотр с расой — /admin/mortality-preview?race_id=ammonia
// на планете 215 K → r_per_sec < 0 (рост, аммиачник в своём доме), t_death
// скрыт; без race_id — человеческая модель (убыль на 215 K).
func TestMortalityPreviewRaceID(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	require.NoError(t, settlement.LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")))

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("p1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Аммиачный дом", 1, `{"temperature":215,"gravity":1.0}`, now, now),
	)
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}),
	)

	// Фракции/строения планеты (спека 2026-09-21-фабрики-релиз-2) — пусто.
	expectEmptyFactionsBuildingsExact(mock)

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/admin/mortality-preview?planet_id=p1&p0=100000000&race_id=ammonia", nil)
	rec := httptest.NewRecorder()

	h.MortalityPreview(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		RPerSec     float64            `json:"r_per_sec"`
		TDeathHours *float64           `json:"t_death_hours"`
		Projection  map[string]float64 `json:"projection"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Less(t, resp.RPerSec, 0.0, "аммиачник на 215 K растёт (r < 0)")
	require.Nil(t, resp.TDeathHours, "t_death скрыт при росте")
	require.Greater(t, resp.Projection["1 год"], float64(100_000_000), "проекция на год — рост")
}

func TestMortalityPreviewUninhabitable(t *testing.T) {
	// R-модель (99.2.12/99.2.13): витринный порог «t_смерти(p0) < 1 ч» по
	// полному r (ChangeComponents) — для p0 = 10⁶ порог r > 1 − exp(−ln(p0)/3600)
	// ≈ 3.8·10⁻³: +310 °C (r ≈ 0.0042, t ≈ 55 мин) — uninhabitable true,
	// lambda_per_hour = 0 (λ-механизм убран, всё в r_per_sec), проекция 0;
	// +250 °C (r ≈ 0.0022, t ≈ 1.7 ч) — false; холод 50 K (r ≈ 0.0206,
	// t ≈ 11 мин) — true, без +Inf (жёсткие нули убраны, 99.2.13).
	cases := []struct {
		name           string
		temp           float64
		uninhabitable  bool
		lambda         float64 // ожидаемый lambda_per_hour (всегда 0)
		r              float64 // ожидаемый r_per_sec
		projectionZero bool   // все точки projection == 0 (фактически вымерло к 1 ч)
	}{
		{"жара 583.15 K (+310 °C)", 583.15, true, 0, 0.00416, true},
		{"жара 523.15 K (+250 °C)", 523.15, false, 0, 0.00223, false},
		{"холод 50 K", 50, true, 0, 0.0206, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer db.Close()

			now := time.Now()
			mock.ExpectQuery(`
				SELECT id, world_id, name, orbit_index, data, created_at, updated_at
				FROM planets
				WHERE id = $1
			`).WithArgs("p1").WillReturnRows(
				sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
					AddRow("p1", "w1", "Необитаемая", 1, fmt.Sprintf(`{"temperature":%v,"gravity":1.0}`, tc.temp), now, now),
			)
			mock.ExpectQuery(`
				SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
				FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
			`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
				sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}),
			)

			// Фракции/строения планеты (спека 2026-09-21-фабрики-релиз-2) — пусто.
			expectEmptyFactionsBuildingsExact(mock)

			h := &AdminHandlers{db: db}
			req := httptest.NewRequest(http.MethodGet, "/admin/mortality-preview?planet_id=p1&p0=1000000", nil)
			rec := httptest.NewRecorder()

			h.MortalityPreview(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			require.NoError(t, mock.ExpectationsWereMet())

			var resp struct {
				LambdaPerHour float64            `json:"lambda_per_hour"`
				Uninhabitable bool               `json:"uninhabitable"`
				RPerSec       float64            `json:"r_per_sec"`
				Projection    map[string]float64 `json:"projection"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, tc.uninhabitable, resp.Uninhabitable)
			require.InDelta(t, tc.lambda, resp.LambdaPerHour, 0.001, "lambda_per_hour")
			require.InDelta(t, tc.r, resp.RPerSec, 0.001, "r_per_sec")
			if tc.projectionZero {
				for _, cp := range settlement.StandardCheckpoints {
					require.Equal(t, float64(0), resp.Projection[cp.Label], "все точки projection должны быть 0")
				}
			}
		})
	}
}
