// Тесты на генерацию поселений рас (admin_race_settlements.go).
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

// GenerateRaceSettlements отклоняет битое JSON-тело.
func TestGenerateRaceSettlementsBadBody(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/generate-race-settlements",
		strings.NewReader("{не-json"))
	rec := httptest.NewRecorder()

	h.GenerateRaceSettlements(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// GenerateRaceSettlements отклоняет шанс вне [0, 1].
func TestGenerateRaceSettlementsBadChance(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/generate-race-settlements",
		strings.NewReader(`{"neighbor_chance":1.5}`))
	rec := httptest.NewRecorder()

	h.GenerateRaceSettlements(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// Каталог рас не загружен — 500 до запросов в БД.
func TestGenerateRaceSettlementsNoCatalog(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/generate-race-settlements", nil)
	rec := httptest.NewRecorder()

	h.GenerateRaceSettlements(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "без каталога БД не трогается")
}

// Планет нет — 200 с total 0 (каталог загружен).
func TestGenerateRaceSettlementsNoPlanets(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT(*) FROM planets`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/generate-race-settlements", nil)
	rec := httptest.NewRecorder()

	h.GenerateRaceSettlements(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"total":0`)
	require.NoError(t, mock.ExpectationsWereMet())
}