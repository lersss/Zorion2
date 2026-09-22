// internal/handlers/cargo_handlers_test.go
//
// Трюм игрока (спека 2026-09-22-трюм-грузоподъёмность-корабля §9): форма
// GET /api/cargo (limits.mass used/total + items), JWT-гейт, валидация и
// «груз за борт» (единственная player-facing запись, §7.1).
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/cargo"
	"zorion/internal/ship"
)

// newCargoHandlersHarness — sqlmock + хендлер трюма.
func newCargoHandlersHarness(t *testing.T) (*CargoHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewCargoHandlers(cargo.NewService(db)), mock
}

const cargoUser = "11111111-1111-1111-1111-111111111111"

// GET /api/cargo — форма ответа §9.1: limits.mass {used,total} + items; mass
// считает сервер; used ≤ total.
func TestGetCargoHandler(t *testing.T) {
	ship.LoadDefaults()
	ship.LoadModelDefaults()
	h, mock := newCargoHandlersHarness(t)

	mock.ExpectQuery(`SELECT pc\.good_id, g\.name, g\.kind, pc\.quantity, g\.weight`).
		WithArgs(cargoUser).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "name", "kind", "quantity", "weight"}).
			AddRow(int64(21), "Железо Fe", "resource", 12.0, 1.0))
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1`).
		WithArgs(cargoUser).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))

	req := httptest.NewRequest(http.MethodGet, "/api/cargo", nil)
	rec := execJSON(h.GetCargo, withUserID(req, cargoUser))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Limits struct {
			Mass struct {
				Used  float64 `json:"used"`
				Total float64 `json:"total"`
			} `json:"mass"`
		} `json:"limits"`
		Items []struct {
			GoodID   int64   `json:"good_id"`
			Name     string  `json:"name"`
			Quantity float64 `json:"quantity"`
			Mass     float64 `json:"mass"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 12.0, resp.Limits.Mass.Used)
	assert.Equal(t, 100.0, resp.Limits.Mass.Total)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, int64(21), resp.Items[0].GoodID)
	assert.Equal(t, 12.0, resp.Items[0].Mass)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Без JWT — 401, БД не трогается.
func TestGetCargoUnauthorized(t *testing.T) {
	h, mock := newCargoHandlersHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/cargo", nil)
	rec := execJSON(h.GetCargo, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Некорректное тело — 400 без обращения к БД.
func TestJettisonBadBody(t *testing.T) {
	h, mock := newCargoHandlersHarness(t)
	req := httptest.NewRequest(http.MethodPost, "/api/cargo/jettison", strings.NewReader("{not json"))
	rec := execJSON(h.Jettison, withUserID(req, cargoUser))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// quantity ≤ 0 без all — 400 (нечего сбрасывать).
func TestJettisonNonPositiveQuantity(t *testing.T) {
	h, mock := newCargoHandlersHarness(t)
	req := httptest.NewRequest(http.MethodPost, "/api/cargo/jettison", strings.NewReader(`{"good_id":21,"quantity":0}`))
	rec := execJSON(h.Jettison, withUserID(req, cargoUser))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// good_id не задан и all не выставлен — 400.
func TestJettisonMissingGoodID(t *testing.T) {
	h, mock := newCargoHandlersHarness(t)
	req := httptest.NewRequest(http.MethodPost, "/api/cargo/jettison", strings.NewReader(`{"quantity":5}`))
	rec := execJSON(h.Jettison, withUserID(req, cargoUser))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// «Выбросить всё»: удаление всего трюма + ответ — обновлённый (пустой) трюм.
func TestJettisonAllHandler(t *testing.T) {
	ship.LoadDefaults()
	ship.LoadModelDefaults()
	h, mock := newCargoHandlersHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`DELETE FROM player_cargo WHERE user_id = \$1 RETURNING quantity`).
		WithArgs(cargoUser).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(3.0))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT pc\.good_id, g\.name, g\.kind, pc\.quantity, g\.weight`).
		WithArgs(cargoUser).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "name", "kind", "quantity", "weight"}))
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1`).
		WithArgs(cargoUser).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))

	req := httptest.NewRequest(http.MethodPost, "/api/cargo/jettison", strings.NewReader(`{"all":true}`))
	rec := execJSON(h.Jettison, withUserID(req, cargoUser))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "[]", string(resp["items"]), "пустой трюм — items: []")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Неизвестный товар — 404.
func TestJettisonUnknownGoodHandler(t *testing.T) {
	h, mock := newCargoHandlersHarness(t)

	mock.ExpectQuery(`SELECT 1 FROM goods WHERE id = \$1`).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"one"}))

	req := httptest.NewRequest(http.MethodPost, "/api/cargo/jettison", strings.NewReader(`{"good_id":999,"quantity":1}`))
	rec := execJSON(h.Jettison, withUserID(req, cargoUser))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Без JWT — 401.
func TestJettisonUnauthorized(t *testing.T) {
	h, mock := newCargoHandlersHarness(t)
	req := httptest.NewRequest(http.MethodPost, "/api/cargo/jettison", strings.NewReader(`{"all":true}`))
	rec := execJSON(h.Jettison, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Неверный метод — 405 (гейт по методу; трюм не отдаём и не трогаем БД).
func TestCargoMethodNotAllowed(t *testing.T) {
	h, mock := newCargoHandlersHarness(t)

	recGet := execJSON(h.GetCargo, httptest.NewRequest(http.MethodPost, "/api/cargo", nil))
	require.Equal(t, http.StatusMethodNotAllowed, recGet.Code)

	recJettison := execJSON(h.Jettison, httptest.NewRequest(http.MethodGet, "/api/cargo/jettison", nil))
	require.Equal(t, http.StatusMethodNotAllowed, recJettison.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}
