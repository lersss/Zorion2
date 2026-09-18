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

	"zorion/internal/auth"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// newAuthHandlersHarness — sqlmock-БД + AuthHandlers + реальный travel.Manager.
func newAuthHandlersHarness(t *testing.T) (*AuthHandlers, sqlmock.Sqlmock, *travel.Manager) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager(nil)
	return NewAuthHandlers(
		repository.NewUserRepository(db),
		repository.NewWorldRepository(db),
		tm,
	), mock, tm
}

// authUserCols — колонки users для sqlmock.
var authUserCols = []string{
	"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at",
}

func TestGetMeWithActiveFlight(t *testing.T) {
	h, mock, tm := newAuthHandlersHarness(t)

	userID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now()))

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
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "alice", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now()))

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
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now()))

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
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "x.png", nil, nil, nil, "player", now(), now()))

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

// ==================== /me: РАЗДЕЛ «КОРАБЛЬ» (спека 91a §7.3) ====================

// Стартовая комплектация 91a: ship_model (имя модели), ship_catalog (весь
// каталог, включая engine_1), ship_speed_factor из установленного двигателя.
func TestGetMeShipSection(t *testing.T) {
	// Каталог оборудования и модели — дефолты (PITFALLS.md:185).
	ship.LoadDefaults()
	ship.LoadModelDefaults()

	h, mock, _ := newAuthHandlersHarness(t)

	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, "player", now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// ship_model: {id, name, slots} из ship_models.
	model, ok := resp["ship_model"].(map[string]interface{})
	require.True(t, ok, "ship_model должен быть объектом")
	assert.Equal(t, "starter", model["id"])
	assert.Equal(t, "Стартовый разведчик", model["name"])
	slots, ok := model["slots"].(map[string]interface{})
	require.True(t, ok, "slots модели должны быть объектом")
	assert.Equal(t, float64(1), slots["radar"])
	assert.Equal(t, float64(1), slots["scanner"])
	assert.Equal(t, float64(1), slots["engine"])

	// ship_catalog: весь каталог (id/type/name/params), включая engine_1.
	catalog, ok := resp["ship_catalog"].([]interface{})
	require.True(t, ok, "ship_catalog должен быть массивом")
	require.Len(t, catalog, 3, "radar_1 + scanner_1 + engine_1")
	engineItem, ok := catalog[0].(map[string]interface{})
	require.True(t, ok, "сортировка по id: engine_1 первый")
	assert.Equal(t, "engine_1", engineItem["id"])
	assert.Equal(t, "engine", engineItem["type"])
	assert.Equal(t, "Двигатель-1", engineItem["name"])
	params, ok := engineItem["params"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(0.3), params["speed_factor"])

	// ship_speed_factor: из установленного двигателя (0.3, константа 66a).
	assert.Equal(t, float64(0.3), resp["ship_speed_factor"])

	// Существующие поля не меняются (И3).
	assert.Equal(t, "starter", resp["ship_model_id"])
	assert.Equal(t, float64(800), resp["radar_radius"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Без двигателя ship_speed_factor не отдаётся — ячейка «не установлен» (спека 91a §6.1).
func TestGetMeShipSectionNoEngine(t *testing.T) {
	ship.LoadDefaults()
	ship.LoadModelDefaults()

	h, mock, _ := newAuthHandlersHarness(t)

	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, "player", now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	_, ok := resp["ship_speed_factor"]
	assert.False(t, ok, "без двигателя ship_speed_factor не отдаётся")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== REGISTER: СТАРТОВЫЙ МИР (баг 77a) ====================

// authUserColsRegister — колонки users для sqlmock (13 колонок userSelect).
var authUserColsRegister = []string{
	"id", "username", "password_hash", "email", "agent_id", "current_world_id",
	"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at",
}

// expectRegisterPrecheck — pre-check занятости имени (пусто).
func expectRegisterPrecheck(mock sqlmock.Sqlmock, username string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE username = \$1`).
		WithArgs(username).
		WillReturnRows(sqlmock.NewRows(authUserColsRegister))
}

// expectRegisterInsert — INSERT нового пользователя (current_world_id — как задан).
func expectRegisterInsert(mock sqlmock.Sqlmock, username string, currentWorldID interface{}) {
	mock.ExpectExec(`INSERT INTO users \(id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_model_id, equipment, role, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12\)`).
		WithArgs(sqlmock.AnyArg(), username, sqlmock.AnyArg(), nil, nil, currentWorldID, "crescent.png", "starter", sqlmock.AnyArg(), "player", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// registerRequest — POST /register с телом {username, password}.
func registerRequest(username string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/register",
		strings.NewReader(`{"username":"`+username+`","password":"secret123"}`))
}

// Мир с людьми найден — current_world_id записан в INSERT (201).
func TestRegisterAssignsSpawnWorld(t *testing.T) {
	_ = auth.InitJWTSecret("handler-test-secret-that-is-long-enough-32b!")
	h, mock, _ := newAuthHandlersHarness(t)

	expectRegisterPrecheck(mock, "newbie")
	mock.ExpectQuery(`SELECT w.id FROM worlds w WHERE EXISTS.*race_id = 'humans'.*LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("w-humans"))
	expectRegisterInsert(mock, "newbie", "w-humans")

	rec := execJSON(h.Register, registerRequest("newbie"))
	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Миров нет — current_world_id = NULL, регистрация не падает (201).
func TestRegisterNoWorlds(t *testing.T) {
	_ = auth.InitJWTSecret("handler-test-secret-that-is-long-enough-32b!")
	h, mock, _ := newAuthHandlersHarness(t)

	expectRegisterPrecheck(mock, "newbie")
	mock.ExpectQuery(`SELECT w.id FROM worlds w WHERE EXISTS.*race_id = 'humans'.*LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT id FROM worlds ORDER BY \(coord_x \* coord_x \+ coord_y \* coord_y\) LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	expectRegisterInsert(mock, "newbie", nil)

	rec := execJSON(h.Register, registerRequest("newbie"))
	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Миров с людьми нет, но миры есть — фолбэк на ближайший к центру (201).
func TestRegisterFallbackClosestWorld(t *testing.T) {
	_ = auth.InitJWTSecret("handler-test-secret-that-is-long-enough-32b!")
	h, mock, _ := newAuthHandlersHarness(t)

	expectRegisterPrecheck(mock, "newbie")
	mock.ExpectQuery(`SELECT w.id FROM worlds w WHERE EXISTS.*race_id = 'humans'.*LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT id FROM worlds ORDER BY \(coord_x \* coord_x \+ coord_y \* coord_y\) LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("w-closest"))
	expectRegisterInsert(mock, "newbie", "w-closest")

	rec := execJSON(h.Register, registerRequest("newbie"))
	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}