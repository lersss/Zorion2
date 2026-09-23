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
	"zorion/internal/models"
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
	h := NewAuthHandlers(
		repository.NewUserRepository(db),
		repository.NewWorldRepository(db),
		tm,
	)
	// planetRepo — пересчёт HP на поверхности в /me (спека 2026-09-21 §8.7).
	h.SetPlanetRepo(repository.NewPlanetRepository(db))
	return h, mock, tm
}

// authUserCols — колонки users для sqlmock (GetByIDWithPosition:
// + current_position, + pending_destination — спека 99.2.30 §6.3).
var authUserCols = []string{
	"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "current_position", "pending_destination", "race_id",
}

func TestGetMeWithActiveFlight(t *testing.T) {
	h, mock, tm := newAuthHandlersHarness(t)

	userID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now(), nil, nil, nil))

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

// /me со status=surface отдаёт позицию поверхности, а hp пересчитывается от
// landed_at (спека 2026-09-21 §7.6 п.6/§8.7): сохранённое значение — не истина.
func TestGetMeSurfacePosition(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "33333333-3333-3333-3333-333333333333"
	biome := testBiomeByCategory(t, "вулканизм")
	// Давно прошедшая высадка на жёсткой планете → серверный hp ≈ 0.
	pos := `{"status":"surface","level":"surface","object_type":"planet","object_id":"pl-1","biome":"` + biome.ID + `","hp":80,"landed_at":"2020-01-01T00:00:00Z"}`
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, "w1", "ship_strela.svg", nil, nil, nil, "player", now(), now(), pos, nil, nil))
	// Имя мира (GetMe) + планета системы для пересчёта HP.
	expectIntraWorld(mock, "w1")
	expectSurfacePlanetsLight(mock, "w1",
		surfacePlanetRow("pl-1", "w1", "Venus", surfacePlanetData(biome.ID, 100, 700, 90, 0, false)))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	cur, ok := resp["current_position"].(map[string]interface{})
	require.True(t, ok, "/me должен отдать current_position объектом")
	assert.Equal(t, "surface", cur["status"])
	assert.Equal(t, "pl-1", cur["object_id"])
	hp, ok := cur["hp"].(float64)
	require.True(t, ok, "/me должен отдать пересчитанный hp")
	assert.Less(t, hp, 1.0, "hp пересчитан от landed_at (§8.7), а не сохранённые 80")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMeWithoutFlight(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "22222222-2222-2222-2222-222222222222"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "alice", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now(), nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp["flight"], "без полёта flight должен быть null")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== /me: РОЛЬ ДЛЯ UI-ГЕЙТА (идея 2026-09-23) ====================

// /me отдаёт роль из токена (контекста), а не из БД: панель гейтит по этой
// роли, а админские ручки проверяют роль в токене — при смене роли после
// выдачи токена рассинхрон давал 403-спам в админке.
func TestGetMeRoleFromContext(t *testing.T) {
	cases := []struct {
		name    string
		dbRole  string
		ctxRole string
		want    string
	}{
		{"db_admin_token_player", "admin", "player", "player"},
		{"db_player_token_admin", "player", "admin", "admin"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, mock, _ := newAuthHandlersHarness(t)
			userID := "12121212-1212-1212-1212-121212121212"
			mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
				WithArgs(userID).
				WillReturnRows(sqlmock.NewRows(authUserCols).
					AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, c.dbRole, now(), now(), nil, nil, nil))

			req := withRole(httptest.NewRequest(http.MethodGet, "/me", nil), c.ctxRole)
			rec := execJSON(h.GetMe, withUserID(req, userID))

			require.Equal(t, http.StatusOK, rec.Code)
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, c.want, resp["role"], "роль — из токена, а не из БД")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// /me без роли в контексте (легаси-тесты/вызовы) → фолбэк на роль из БД.
func TestGetMeRoleFallsBackToDB(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "13131313-1313-1313-1313-131313131313"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "admin", now(), now(), nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "admin", resp["role"], "нет роли в контексте → фолбэк на БД")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== /me: МАППИНГ ship_icon (спека 61b §4) ====================

// Legacy SVG-имя маппится в PNG-имя; ship_options — весь реестр спрайтов.
func TestGetMeMapsLegacyShipIcon(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "33333333-3333-3333-3333-333333333333"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now(), nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "race_humans_starship.png", resp["ship_icon"], "легаси ship_strela.svg → людской корабль")
	assert.Nil(t, resp["ship_color"], "NULL цвет = «Оригинал»")
	opts, ok := resp["ship_options"].([]interface{})
	require.True(t, ok, "ship_options должен быть массивом")
	assert.Len(t, opts, len(models.ShipOptions), "ship_options — расовый транспорт реестра")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Неизвестное имя ship_icon → дефолт crescent.png.
func TestGetMeMapsUnknownShipIconToDefault(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "44444444-4444-4444-4444-444444444444"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "x.png", nil, nil, nil, "player", now(), now(), nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "race_humans_starship.png", resp["ship_icon"], "неизвестное имя → людской корабль")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== PUT /me/ship-icon (спека 61b §4) ====================

// Имя из реестра принимается (и принадлежит расе игрока — спека §6.5).
func TestUpdateShipIconAcceptsRegistryName(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "55555555-5555-5555-5555-555555555555"
	// Валидация расы (спека 2026-09-23 §6.5): игрок-человек, файл людского пула.
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserColsRegister).
			AddRow(userID, "bob", "hash", nil, nil, nil, "race_humans_starship.png", nil, nil, nil, "player", now(), now(), "humans"))
	mock.ExpectExec(`UPDATE users SET ship_icon = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("race_humans_cruiser.png", userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPut, "/me/ship-icon", strings.NewReader(`{"ship_icon":"race_humans_cruiser.png"}`))
	rec := execJSON(h.UpdateShipIcon, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "race_humans_cruiser.png", resp["ship_icon"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Файл чужой расы → 400 (спека 2026-09-23 §6.5: игрок-человек не летает
// кораблём аммиачных), БД не трогается.
func TestUpdateShipIconRejectsOtherRace(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "55555555-5555-5555-5555-555555555555"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserColsRegister).
			AddRow(userID, "bob", "hash", nil, nil, nil, "race_humans_starship.png", nil, nil, nil, "player", now(), now(), "humans"))

	req := httptest.NewRequest(http.MethodPut, "/me/ship-icon", strings.NewReader(`{"ship_icon":"race_ammonia_02.png"}`))
	rec := execJSON(h.UpdateShipIcon, withUserID(req, userID))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "чужой расовый корабль не должен трогать БД")
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
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, "player", now(), now(), nil, nil, nil))

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

	// ship_catalog: весь каталог (id/type/name/params), включая engine_1 и
	// грузовой модуль cargo_1 (спека трюма §8.3).
	catalog, ok := resp["ship_catalog"].([]interface{})
	require.True(t, ok, "ship_catalog должен быть массивом")
	require.Len(t, catalog, 4, "cargo_1 + engine_1 + radar_1 + scanner_1")
	cargoItem, ok := catalog[0].(map[string]interface{})
	require.True(t, ok, "сортировка по id: cargo_1 первый")
	assert.Equal(t, "cargo_1", cargoItem["id"])
	assert.Equal(t, "cargo", cargoItem["type"])
	assert.Equal(t, "Грузовой модуль-1", cargoItem["name"])
	cargoParams, ok := cargoItem["params"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(80), cargoParams["capacity"])
	engineItem, ok := catalog[1].(map[string]interface{})
	require.True(t, ok, "сортировка по id: engine_1 второй")
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
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, "player", now(), now(), nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	_, ok := resp["ship_speed_factor"]
	assert.False(t, ok, "без двигателя ship_speed_factor не отдаётся")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== /me: ВНУТРИСИСТЕМНАЯ ПОЗИЦИЯ (спека 99.2.27 §4.3) ====================

// current_position отдаётся как есть (JSONB); NULL-позиция → null.
func TestGetMeCurrentPosition(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	pos := `{"status":"in_flight","from_type":"star","from_id":"w1","to_type":"planet","to_id":"p1","start_time":1726800000000,"arrive_at":1726800014000}`
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, "w1", "ship_strela.svg", nil, nil, nil, "player", now(), now(), pos, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	cp, ok := resp["current_position"].(map[string]interface{})
	require.True(t, ok, "current_position должен быть объектом")
	assert.Equal(t, "in_flight", cp["status"])
	assert.Equal(t, "planet", cp["to_type"])
	assert.Equal(t, "p1", cp["to_id"])
	assert.Equal(t, float64(1726800000000), cp["start_time"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// NULL-позиция → current_position: null (не отсутствует).
func TestGetMeCurrentPositionNull(t *testing.T) {
	h, mock, _ := newAuthHandlersHarness(t)

	userID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, "w1", "ship_strela.svg", nil, nil, nil, "player", now(), now(), nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp["current_position"], "NULL-позиция → null")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== REGISTER: СТАРТОВЫЙ МИР (баг 77a) ====================

// authUserColsRegister — колонки users для sqlmock (13 колонок userSelect).
var authUserColsRegister = []string{
	"id", "username", "password_hash", "email", "agent_id", "current_world_id",
	"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "race_id",
}

// expectRegisterPrecheck — pre-check занятости имени (пусто).
func expectRegisterPrecheck(mock sqlmock.Sqlmock, username string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE username = \$1`).
		WithArgs(username).
		WillReturnRows(sqlmock.NewRows(authUserColsRegister))
}

// expectRegisterInsert — INSERT нового пользователя + счёт игрока в ОДНОЙ
// транзакции (спека 2026-09-22-деньги-и-эскроу §3.4); current_world_id — как задан.
func expectRegisterInsert(mock sqlmock.Sqlmock, username string, currentWorldID interface{}) {
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO users \(id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_model_id, equipment, role, created_at, updated_at, race_id\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12, \$13\)`).
		WithArgs(sqlmock.AnyArg(), username, sqlmock.AnyArg(), nil, nil, currentWorldID, sqlmock.AnyArg(), "starter", sqlmock.AnyArg(), "player", sqlmock.AnyArg(), sqlmock.AnyArg(), "humans").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Счёт игрока: seed PlayerBalanceSeed, идемпотентно (ON CONFLICT DO NOTHING).
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("player", sqlmock.AnyArg(), int64(models.PlayerBalanceSeed)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
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

// ==================== /me: ЛЕНИВАЯ СТРАХОВКА СЧЁТА (спека §3.4) ====================

// /me идемпотентно гарантирует счёт игрока (страховка старых и
// админ-созданных учёток) — INSERT ... ON CONFLICT DO NOTHING.
func TestGetMeEnsuresPlayerAccount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	h := NewAuthHandlers(
		repository.NewUserRepository(db),
		repository.NewWorldRepository(db),
		travel.NewManager(nil),
	)
	h.SetAccountRepo(repository.NewAccountRepository(db))

	userID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(authUserCols).
			AddRow(userID, "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now(), nil, nil, nil))
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("player", userID, int64(models.PlayerBalanceSeed)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := execJSON(h.GetMe, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
