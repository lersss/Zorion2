// Тесты на предпросмотр смерти населения от среды (admin_mortality.go).
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
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
			AddRow("p1", "w1", "Уютная", 1, `{"temperature":275,"gravity":1.0}`, now, now),
	)
	mock.ExpectQuery(`
		SELECT id, planet_id, population, stability, created_at, updated_at
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "planet_id", "population", "stability", "created_at", "updated_at"}).
			AddRow("s1", "p1", 1_000_000, 60, now, now),
	)

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
			AddRow("p1", "w1", "Без поселений", 1, `{"temperature":275,"gravity":1.0}`, now, now),
	)
	mock.ExpectQuery(`
		SELECT id, planet_id, population, stability, created_at, updated_at
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "planet_id", "population", "stability", "created_at", "updated_at"}),
	)

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
