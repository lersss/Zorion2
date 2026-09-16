// internal/handlers/auth_handlers_test.go
// Тесты /me (идея 42a): поле "flight" — активный полёт из travel.Manager.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
	"zorion/internal/travel"
)

// newAuthHandlersHarness — sqlmock-БД + AuthHandlers + реальный travel.Manager.
func newAuthHandlersHarness(t *testing.T) (*AuthHandlers, sqlmock.Sqlmock, *travel.Manager) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager()
	return NewAuthHandlers(
		repository.NewUserRepository(db),
		repository.NewWorldRepository(db),
		tm,
	), mock, tm
}

// authUserCols — колонки users для sqlmock.
var authUserCols = []string{
	"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "ship_color", "role", "created_at", "updated_at",
}

func TestGetMeWithActiveFlight(t *testing.T) {
	h, mock, tm := newAuthHandlersHarness(t)

	userID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, "player", now(), now()))

	// Полёт с часовой длительностью — не завершится во время теста.
	tm.StartFlight(userID, "w-from", "w-to", 10.5, 20.5, time.Hour, nil)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	flight, ok := resp["flight"].(map[string]interface{})
	require.True(t, ok, "/me должен вернуть flight объектом")
	assert.Equal(t, "w-from", flight["from"])
	assert.Equal(t, "w-to", flight["to"])
	assert.Equal(t, float64(3600), flight["duration"])
	assert.Equal(t, 10.5, flight["start_x"])
	assert.Equal(t, 20.5, flight["start_y"])
	startMS, ok := flight["start_time"].(float64)
	require.True(t, ok, "start_time должен быть UnixMilli числом")
	assert.InDelta(t, float64(time.Now().UnixMilli()), startMS, 5000)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMeWithoutFlight(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "22222222-2222-2222-2222-222222222222"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "alice", "hash", nil, nil, nil, "ship_strela.svg", nil, "player", now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp["flight"], "без полёта flight должен быть null")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== /me: МАППИНГ ship_icon (спека 61b §4) ====================

// Legacy SVG-имя маппится в PNG-имя; ship_options — ровно 21 запись.
func TestGetMeMapsLegacyShipIcon(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "33333333-3333-3333-3333-333333333333"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, "player", now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "boomerang.png", resp["ship_icon"], "ship_strela.svg должен смаппиться в boomerang.png")
	assert.Nil(t, resp["ship_color"], "NULL цвет = «Оригинал»")
	opts, ok := resp["ship_options"].([]interface{})
	require.True(t, ok, "ship_options должен быть массивом")
	assert.Len(t, opts, 21, "ship_options — ровно 21 спрайт")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Неизвестное имя ship_icon → дефолт crescent.png.
func TestGetMeMapsUnknownShipIconToDefault(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "44444444-4444-4444-4444-444444444444"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "x.png", nil, "player", now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "crescent.png", resp["ship_icon"], "неизвестное имя → дефолт")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== PUT /me/ship-icon (спека 61b §4) ====================

// Имя из реестра принимается.
func TestUpdateShipIconAcceptsRegistryName(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "55555555-5555-5555-5555-555555555555"
	mock.ExpectExec(`UPDATE users SET ship_icon = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("shark.png", userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPut, "/me/ship-icon", strings.NewReader(`{"ship_icon":"shark.png"}`))
	rec := execJSON(h.UpdateShipIcon, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "shark.png", resp["ship_icon"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Legacy-имя и мусор → 400 (выбор только из нового набора).
func TestUpdateShipIconRejectsLegacyAndUnknown(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "66666666-6666-6666-6666-666666666666"
	for _, icon := range []string{"ship_strela.svg", "x.png"} {
		req := httptest.NewRequest(http.MethodPut, "/me/ship-icon", strings.NewReader(`{"ship_icon":"`+icon+`"}`))
		rec := execJSON(h.UpdateShipIcon, withUserID(req, userID))
		assert.Equal(t, http.StatusBadRequest, rec.Code, "иконка %s должна быть отклонена", icon)
	}
	require.NoError(t, mock.ExpectationsWereMet(), "невалидные иконки не должны трогать БД")
}

// ==================== PUT /me/ship-color (спека 61b §7) ====================

// Цвет из палитры принимается.
func TestUpdateShipColorAcceptsPaletteColor(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "77777777-7777-7777-7777-777777777777"
	mock.ExpectExec(`UPDATE users SET ship_color = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("#ef4444", userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPut, "/me/ship-color", strings.NewReader(`{"ship_color":"#ef4444"}`))
	rec := execJSON(h.UpdateShipColor, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "#ef4444", resp["ship_color"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// NULL («Оригинал») принимается.
func TestUpdateShipColorAcceptsNull(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "88888888-8888-8888-8888-888888888888"
	mock.ExpectExec(`UPDATE users SET ship_color = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(nil, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPut, "/me/ship-color", strings.NewReader(`{"ship_color":null}`))
	rec := execJSON(h.UpdateShipColor, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp["ship_color"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Мусор (вне палитры) → 400.
func TestUpdateShipColorRejectsGarbage(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "99999999-9999-9999-9999-999999999999"
	for _, body := range []string{`{"ship_color":"#000000"}`, `{"ship_color":"red"}`, `{"ship_color":""}`} {
		req := httptest.NewRequest(http.MethodPut, "/me/ship-color", strings.NewReader(body))
		rec := execJSON(h.UpdateShipColor, withUserID(req, userID))
		assert.Equal(t, http.StatusBadRequest, rec.Code, "цвет %s должен быть отклонён", body)
	}
	require.NoError(t, mock.ExpectationsWereMet(), "невалидные цвета не должны трогать БД")
}