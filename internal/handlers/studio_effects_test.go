// internal/handlers/studio_effects_test.go
// Тесты хендлеров «Эффекты» студии (спека 2026-09-22-эффекты-снабжения-
// задержка-голод §7.4): список/создание типа, удаление типа с действующими
// эффектами → 409 (T16), счётчики привязок (T16).
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

	"zorion/internal/goodsstudio/ai"
)

// TestStudioEffectsList — GET /studio/api/effects: каталог типов.
func TestStudioEffectsList(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at, code FROM effect_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at", "code"}).
			AddRow(int64(1), "Голод", "голод", "population_rate", "hunger", time.Now(), "e_0001"))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/effects", nil)
	rec := httptest.NewRecorder()
	h.Effects(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Effects []EffectTypeView `json:"effects"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Effects, 1)
	require.Equal(t, "hunger", body.Effects[0].Curve)
	require.Equal(t, "e_0001", body.Effects[0].Code, "представление эффекта несёт метку переноса")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioCreateEffect — POST /studio/api/effects {name, impact, curve} → 201.
func TestStudioCreateEffect(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM effect_types WHERE name_norm = \$1\)`).
		WithArgs("жажда").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO effect_types`).
		WithArgs("Жажда", "жажда", "population_rate", `{"curve":"thirst"}`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at"}).
			AddRow(int64(2), "Жажда", "жажда", "population_rate", "thirst", time.Now()))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/effects",
		strings.NewReader(`{"name":"Жажда","impact":"population_rate","curve":"thirst"}`))
	rec := httptest.NewRecorder()
	h.Effects(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var v EffectTypeView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v))
	require.Equal(t, "thirst", v.Curve)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDeleteEffectRestrict — DELETE типа с действующими эффектами → 409.
func TestStudioDeleteEffectRestrict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM effect_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM active_effects WHERE effect_type_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/effects/1", nil)
	rec := httptest.NewRecorder()
	h.EffectByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioEffectCounts — GET /studio/api/effects/{id}/counts: счётчик привязок.
func TestStudioEffectCounts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Счётчики — в одной транзакции под advisory-локом каталога (замечание
	// @reviewer): иначе счётчики могут разъехаться между запросами.
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT name_norm FROM effect_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("голод"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM active_effects WHERE effect_type_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM producer_types pt WHERE EXISTS`).
		WithArgs("голод").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/effects/1/counts", nil)
	rec := httptest.NewRecorder()
	h.EffectByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]int
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 4, body["active_effects"])
	require.Equal(t, 1, body["producers"])
	require.NoError(t, mock.ExpectationsWereMet())
}
