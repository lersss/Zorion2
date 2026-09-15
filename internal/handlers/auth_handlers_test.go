// internal/handlers/auth_handlers_test.go
// Тесты /me (идея 42a): поле "flight" — активный полёт из travel.Manager.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
}

func TestGetMeWithActiveFlight(t *testing.T) {
	h, mock, tm := newAuthHandlersHarness(t)

	userID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", "player", now(), now()))

	// Полёт с часовой длительностью — не завершится во время теста.
	tm.StartFlight(userID, "w-from", "w-to", time.Hour, nil)

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
	startMS, ok := flight["start_time"].(float64)
	require.True(t, ok, "start_time должен быть UnixMilli числом")
	assert.InDelta(t, float64(time.Now().UnixMilli()), startMS, 5000)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMeWithoutFlight(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "22222222-2222-2222-2222-222222222222"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "alice", "hash", nil, nil, nil, "ship_strela.svg", "player", now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp["flight"], "без полёта flight должен быть null")
	require.NoError(t, mock.ExpectationsWereMet())
}