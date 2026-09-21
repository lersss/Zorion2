// internal/handlers/admin_deposits_test.go
//
// Тесты админ-ручки «добавить залежь вручную» (спека 2026-09-22-поселение-
// добыча-сырья-биома-ленивый-буфер §6/T7): гейты 409, 404, 422, контракт
// ответа {planet_id, world_id, deposits[]}.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
)

func addDepositReq(path, body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
}

// T7: успешное добавление по good_id → 200 и блок deposits планеты.
func TestAddDepositHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT world_id FROM planets WHERE id = \$1`).
		WithArgs("p1").WillReturnRows(sqlmock.NewRows([]string{"world_id"}).AddRow("w1"))
	mock.ExpectQuery(`SELECT id, name FROM goods WHERE id = \$1 AND kind = 'resource'`).
		WithArgs(int64(359)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(359, "Мясо"))
	mock.ExpectExec(`INSERT INTO deposits`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`FROM deposits d`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "planet_id", "good_id", "name", "stratum", "wealth", "amount"}).
			AddRow("d1", "p1", int64(359), "Мясо", "surface", 0.5, 1000.0))
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", `{"good_id":359}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body struct {
		PlanetID string `json:"planet_id"`
		WorldID  string `json:"world_id"`
		Deposits []struct {
			GoodID   int64   `json:"good_id"`
			GoodName string  `json:"good_name"`
			Stratum  string  `json:"stratum"`
			Wealth   float64 `json:"wealth"`
			Amount   float64 `json:"amount"`
		} `json:"deposits"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "p1", body.PlanetID)
	assert.Equal(t, "w1", body.WorldID)
	require.Len(t, body.Deposits, 1)
	assert.Equal(t, int64(359), body.Deposits[0].GoodID)
	assert.Equal(t, "Мясо", body.Deposits[0].GoodName)
	assert.Equal(t, "surface", body.Deposits[0].Stratum)
}

// T7: планету не нашли → 404, залежь не создаётся.
func TestAddDepositPlanetNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT world_id FROM planets WHERE id = \$1`).WithArgs("pX").
		WillReturnRows(sqlmock.NewRows([]string{"world_id"}))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/pX/deposits", `{"good_id":1}`))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T7: ресурса нет / kind != 'resource' → 422 (резолв по good_name).
func TestAddDepositNotResource(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT world_id FROM planets WHERE id = \$1`).
		WithArgs("p1").WillReturnRows(sqlmock.NewRows([]string{"world_id"}).AddRow("w1"))
	mock.ExpectQuery(`SELECT id, name FROM goods WHERE name_norm = \$1 AND kind = 'resource'`).
		WithArgs("растения").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", `{"good_name":"Растения"}`))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T7: stratum итерации 1 — только 'surface'; иное → 422 без обращений к БД.
func TestAddDepositBadStratum(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", `{"good_id":1,"stratum":"subsurface"}`))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

// T7: wealth вне [0, 1] → 422 без обращений к БД (не 500 из CHECK БД).
func TestAddDepositWealthOutOfRange(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	for _, body := range []string{`{"good_id":1,"wealth":-0.1}`, `{"good_id":1,"wealth":1.5}`} {
		rec := httptest.NewRecorder()
		h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", body))
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "тело %s", body)
	}
}

// T7: amount < 0 → 422 без обращений к БД.
func TestAddDepositAmountNegative(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", `{"good_id":1,"amount":-5}`))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

// §6: сбой INSERT откатывает транзакцию — частичного состояния нет.
func TestAddDepositInsertFailureRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT world_id FROM planets WHERE id = \$1`).
		WithArgs("p1").WillReturnRows(sqlmock.NewRows([]string{"world_id"}).AddRow("w1"))
	mock.ExpectQuery(`SELECT id, name FROM goods WHERE id = \$1 AND kind = 'resource'`).
		WithArgs(int64(359)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(359, "Мясо"))
	mock.ExpectExec(`INSERT INTO deposits`).WillReturnError(fmt.Errorf("disk full"))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", `{"good_id":359}`))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T7: пакман ест миры → 409, ничего не создано.
func TestAddDepositPacmanGate(t *testing.T) {
	startJob(t, generator.JobPacman)

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", `{"good_id":1}`))

	require.Equal(t, http.StatusConflict, rec.Code)
}

// T7: universeMutationMu занят → 409, ничего не создано.
func TestAddDepositMutexGate(t *testing.T) {
	universeMutationMu.Lock()
	defer universeMutationMu.Unlock()

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddDeposit(rec, addDepositReq("/admin/planets/p1/deposits", `{"good_id":1}`))

	require.Equal(t, http.StatusConflict, rec.Code)
}
