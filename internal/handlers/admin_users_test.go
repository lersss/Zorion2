// internal/handlers/admin_users_test.go
// Тесты раздела «Пользователи» (спека 99.2.14 §6, §7): список, профиль,
// создание, смена роли, сброс пароля, удаление + перевод инвариантов И1/И2
// в 403 и гейт SkycomposerAuth на уровне роута.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/auth"
	"zorion/internal/repository"
)

// now — стабильная дата для строк sqlmock.
func now() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}

// ==================== ХЕЛПЕРЫ ====================

// newAdminUsersHarness — sqlmock-БД + хендлеры раздела.
func newAdminUsersHarness(t *testing.T) (*AdminUsersHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewAdminUsersHandlers(
		repository.NewUserRepository(db),
		repository.NewWorldRepository(db),
	), mock
}

// withUserID кладёт id в контекст, как это делает AuthMiddleware.
func withUserID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), auth.UserIDKey, id))
}

// execJSON отправляет запрос и возвращает recorder.
func execJSON(handler http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler(rec, r)
	return rec
}

// ==================== СПИСОК §6.1 ====================

func TestAdminUsersListSuccess(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE \(username ILIKE.*role = \$2`).
		WithArgs("bo", "admin").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`FROM users WHERE \(username ILIKE.*ORDER BY created_at DESC, id LIMIT \$3 OFFSET \$4`).
		WithArgs("bo", "admin", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}).AddRow("11111111-1111-1111-1111-111111111111", "bob", "hash", nil, nil, nil, "ship_strela.svg", "admin", now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/admin/users?query=bo&role=admin&page=1&limit=20", nil)
	rec := execJSON(h.HandleCollection, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Total int `json:"total"`
		Page  int `json:"page"`
		Limit int `json:"limit"`
		Users []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"users"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.Limit)
	require.Len(t, resp.Users, 1)
	assert.Equal(t, "bob", resp.Users[0].Username)
	assert.Equal(t, "admin", resp.Users[0].Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminUsersListInvalidRole(t *testing.T) {
	h, _ := newAdminUsersHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?role=god", nil)
	rec := execJSON(h.HandleCollection, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Неверный фильтр роли")
}

func TestAdminUsersListLimitCappedAt100(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`FROM users ORDER BY created_at DESC, id LIMIT \$1 OFFSET \$2`).
		WithArgs(100, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}))

	req := httptest.NewRequest(http.MethodGet, "/admin/users?limit=500", nil)
	rec := execJSON(h.HandleCollection, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Limit int `json:"limit"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 100, resp.Limit)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== СОЗДАНИЕ §6.3 ====================

func TestAdminUsersCreateSuccess(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at FROM users WHERE username = \$1`).
		WithArgs("newbie").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}))
	mock.ExpectExec(`INSERT INTO users`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"username":"newbie","password":"secret","email":"n@x.io","role":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(body))
	rec := execJSON(h.HandleCollection, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "newbie", resp["username"])
	assert.Equal(t, "admin", resp["role"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminUsersCreateTaken(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at FROM users WHERE username = \$1`).
		WithArgs("newbie").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}).AddRow("u9", "newbie", "hash", nil, nil, nil, "ship_strela.svg", "player", now(), now()))

	body := `{"username":"newbie","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(body))
	rec := execJSON(h.HandleCollection, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminUsersCreateMissingFields(t *testing.T) {
	h, _ := newAdminUsersHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(`{"username":"x"}`))
	rec := execJSON(h.HandleCollection, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(`{"username":"x","password":"p","role":"god"}`))
	rec = execJSON(h.HandleCollection, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== ПРОФИЛЬ §6.2 ====================

func TestAdminUsersGetProfileWithWorldName(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	wid := "w1"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}).AddRow("11111111-1111-1111-1111-111111111111", "bob", "hash", "b@x.io", nil, wid, "ship_strela.svg", "admin", now(), now()))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(wid).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "created_at", "updated_at",
		}).AddRow(wid, "Земля", 0, 0, "G", 288, now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/admin/users/11111111-1111-1111-1111-111111111111", nil)
	rec := execJSON(h.HandleUser, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "bob", resp["username"])
	assert.Equal(t, "admin", resp["role"])
	assert.Equal(t, "Земля", resp["current_world_name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminUsersGetProfileNotFound(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("33333333-3333-3333-3333-333333333333").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}))

	req := httptest.NewRequest(http.MethodGet, "/admin/users/33333333-3333-3333-3333-333333333333", nil)
	rec := execJSON(h.HandleUser, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== СМЕНА РОЛИ §6.4 ====================

func TestAdminUsersUpdateRoleSuccess(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("player"))
	mock.ExpectExec(`UPDATE users SET role = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("admin", "11111111-1111-1111-1111-111111111111").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// Перечитывание профиля для ответа.
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}).AddRow("11111111-1111-1111-1111-111111111111", "bob", "hash", nil, nil, nil, "ship_strela.svg", "admin", now(), now()))

	req := httptest.NewRequest(http.MethodPatch, "/admin/users/11111111-1111-1111-1111-111111111111/role", strings.NewReader(`{"role":"admin"}`))
	rec := execJSON(h.HandleUser, withUserID(req, "caller"))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "admin", resp["role"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// И2: смена роли самому себе → 403.
func TestAdminUsersUpdateRoleSelfForbidden(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectRollback()

	req := httptest.NewRequest(http.MethodPatch, "/admin/users/11111111-1111-1111-1111-111111111111/role", strings.NewReader(`{"role":"admin"}`))
	rec := execJSON(h.HandleUser, withUserID(req, "11111111-1111-1111-1111-111111111111"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// И1: снятие последнего skycomposer → 403.
func TestAdminUsersUpdateRoleLastSkycomposerForbidden(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("22222222-2222-2222-2222-222222222222").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE role = \$1`).
		WithArgs("skycomposer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	req := httptest.NewRequest(http.MethodPatch, "/admin/users/22222222-2222-2222-2222-222222222222/role", strings.NewReader(`{"role":"player"}`))
	rec := execJSON(h.HandleUser, withUserID(req, "caller"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminUsersUpdateRoleNotFound(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("33333333-3333-3333-3333-333333333333").
		WillReturnRows(sqlmock.NewRows([]string{"role"}))
	mock.ExpectRollback()

	req := httptest.NewRequest(http.MethodPatch, "/admin/users/33333333-3333-3333-3333-333333333333/role", strings.NewReader(`{"role":"admin"}`))
	rec := execJSON(h.HandleUser, withUserID(req, "caller"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== СБРОС ПАРОЛЯ §6.5 ====================

func TestAdminUsersResetPasswordSuccess(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectExec(`UPDATE users SET password_hash = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "11111111-1111-1111-1111-111111111111").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/admin/users/11111111-1111-1111-1111-111111111111/reset-password", strings.NewReader(`{"password":"newpass"}`))
	rec := execJSON(h.HandleUser, withUserID(req, "caller"))

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// И2 (§9.3): сброс пароля самому себе через раздел — нельзя → 403.
func TestAdminUsersResetPasswordSelfForbidden(t *testing.T) {
	h, _ := newAdminUsersHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/11111111-1111-1111-1111-111111111111/reset-password", strings.NewReader(`{"password":"newpass"}`))
	rec := execJSON(h.HandleUser, withUserID(req, "11111111-1111-1111-1111-111111111111"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAdminUsersResetPasswordNotFound(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectExec(`UPDATE users SET password_hash = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "33333333-3333-3333-3333-333333333333").
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodPost, "/admin/users/33333333-3333-3333-3333-333333333333/reset-password", strings.NewReader(`{"password":"newpass"}`))
	rec := execJSON(h.HandleUser, withUserID(req, "caller"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== УДАЛЕНИЕ §6.6 ====================

func TestAdminUsersDeleteSuccess(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("player"))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/11111111-1111-1111-1111-111111111111", nil)
	rec := execJSON(h.HandleUser, withUserID(req, "caller"))

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminUsersDeleteSelfForbidden(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectRollback()

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/11111111-1111-1111-1111-111111111111", nil)
	rec := execJSON(h.HandleUser, withUserID(req, "11111111-1111-1111-1111-111111111111"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ГЕЙТ РОУТА (И3) ====================

var jwtOnce sync.Once

func requireHandlerJWT(t *testing.T) {
	t.Helper()
	jwtOnce.Do(func() {
		require.NoError(t, auth.InitJWTSecret("handler-test-secret-that-is-long-enough-32b!"))
	})
}

// ADMIN не проходит в раздел «Пользователи» (И3: только Skycomposer).
func TestAdminUsersRouteDeniesAdmin(t *testing.T) {
	requireHandlerJWT(t)

	tok, err := auth.GenerateToken("admin1", string(auth.RoleAdmin))
	require.NoError(t, err)

	handler := auth.SkycomposerAuth(NewAdminUsersHandlers(nil, nil).HandleCollection)
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "Недостаточно прав")
}

// Skycomposer проходит гейт и получает данные списка.
func TestAdminUsersRouteAllowsSkycomposer(t *testing.T) {
	requireHandlerJWT(t)

	tok, err := auth.GenerateToken("sky1", string(auth.RoleSkycomposer))
	require.NoError(t, err)

	h, mock := newAdminUsersHarness(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`FROM users ORDER BY created_at DESC, id LIMIT \$1 OFFSET \$2`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "role", "created_at", "updated_at",
		}))

	handler := auth.SkycomposerAuth(h.HandleCollection)
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Регрессия (отчёт @tester 2026-09-14): невалидный UUID в {id} падал в БД
// (pq: invalid input syntax for type uuid) → 500. Теперь 400 «Невалидный ID»
// на всех четырёх ручках раздела, до обращения к БД.
func TestAdminUsersInvalidIDReturns400(t *testing.T) {
	h, mock := newAdminUsersHarness(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"profile", http.MethodGet, "/admin/users/not-a-uuid", ""},
		{"role", http.MethodPatch, "/admin/users/not-a-uuid/role", `{"role":"admin"}`},
		{"delete", http.MethodDelete, "/admin/users/not-a-uuid", ""},
		{"reset-password", http.MethodPost, "/admin/users/not-a-uuid/reset-password", `{"password":"x"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			rec := execJSON(h.HandleUser, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "Невалидный ID", resp["error"])
		})
	}
	require.NoError(t, mock.ExpectationsWereMet(), "невалидный id не должен трогать БД")
}
