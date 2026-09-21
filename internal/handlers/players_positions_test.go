// internal/handlers/players_positions_test.go
// Тесты /api/players/positions по правилу 99.2.27 §4.5 (изменение 90a,
// решение создателя С2): стоящие на орбите видны в радиусе, статусы с именем
// объекта от сервера (С3), битая цель → фолбэк «в системе X» (ИП-4, М-2).
package handlers

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// newPlayersPositionsHarness — AdminHandlers с visibility (игрок в w1, радар 800).
func newPlayersPositionsHarness(t *testing.T) (*AdminHandlers, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	mc := mapcache.NewManager()
	mc.Replace(visWorlds())
	userRepo := repository.NewUserRepository(db)
	v := NewVisibility(userRepo, tm, mc, repository.NewKnowledgeRepository(db))

	h := NewAdminHandlers(repository.NewWorldRepository(db), db, mc)
	h.SetVisibility(v)
	h.SetTravelManager(tm)
	return h, mock
}

// playersPositionsRequest — GET /api/players/positions с ролью.
func playersPositionsRequest(userID, role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/players/positions", nil)
	req = withUserID(req, userID)
	req = withRole(req, role)
	return req
}

// expectPositionsUsers — ожидание PlayerPositions (с current_position).
func expectPositionsUsers(mock sqlmock.Sqlmock, rows ...[]driver.Value) {
	r := sqlmock.NewRows([]string{"id", "username", "ship_icon", "ship_color", "current_world_id", "role", "current_position"})
	for _, row := range rows {
		r.AddRow(row...)
	}
	mock.ExpectQuery(`SELECT id, username, ship_icon, ship_color, current_world_id, role, current_position FROM users`).
		WillReturnRows(r)
}

// ==================== СТОЯЩИЕ ВИДНЫ (С2) ====================

// Стоящий на орбите планеты в радиусе — виден, статус «у планеты X» с именем.
func TestPlayersPositionsStandingVisible(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w2", "player",
			`{"status":"orbit","object_type":"planet","object_id":"pl-1","level":"orbit"}`},
	)
	// Имя планеты — батчем.
	mock.ExpectQuery(`SELECT id, name FROM planets WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("pl-1", "Nemurzan II"))

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1, "стоящий в радиусе виден (С2)")
	p := resp.Players[0]
	require.Equal(t, "alice", p["username"])
	require.Equal(t, "у планеты Nemurzan II", p["status"])
	require.Equal(t, "planet", p["object_type"])
	require.Equal(t, "pl-1", p["object_id"])
	require.InDelta(t, 100.0, p["x"].(float64), 0.01, "координаты = звезда системы (С3)")
}

// Стоящий вне радиуса — скрыт (И1).
func TestPlayersPositionsStandingOutsideRadiusHidden(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w3", "player", // w3 (1000,0) — за радаром
			`{"status":"orbit","object_type":"planet","object_id":"pl-1","level":"orbit"}`},
	)
	// Имя планеты запрашивается, но радиус-фильтр скрывает игрока.
	mock.ExpectQuery(`SELECT id, name FROM planets WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("pl-1", "Nemurzan II"))

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 0, "стоящий за радаром скрыт")
}

// Внутрисистемный полёт (status=in_flight) — «в полёте (система X)», координаты звезды.
func TestPlayersPositionsIntraInFlight(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w2", "player",
			`{"status":"in_flight","from_type":"star","from_id":"w2","to_type":"planet","to_id":"pl-1","start_time":1,"arrive_at":2}`},
	)

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1)
	p := resp.Players[0]
	require.Equal(t, "в полёте (система Мир2)", p["status"])
	require.Nil(t, p["object_type"], "летящий — без object_type (маршрут не показывается)")
	require.InDelta(t, 100.0, p["x"].(float64), 0.01, "координаты = звезда системы")
}

// ==================== КОМПАНЬОНЫ (С3) ====================

// Позиция у компаньона — статус «у компаньона X» (X — спектральный класс).
func TestPlayersPositionsCompanion(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w2", "player",
			`{"status":"orbit","object_type":"star","object_id":"companion:w2","level":"orbit"}`},
	)
	// stellar_mods системы — для подписи компаньона.
	mock.ExpectQuery(`SELECT id, stellar_mods FROM worlds WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "stellar_mods"}).
			AddRow("w2", `{"binary_type":"wide","companion":"K","companion_sep_au":1000}`))

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1)
	p := resp.Players[0]
	require.Equal(t, "у компаньона K", p["status"])
	require.Equal(t, "star", p["object_type"])
	require.Equal(t, "companion:w2", p["object_id"])
}

// ==================== БИТАЯ ЦЕЛЬ (ИП-4, М-2) ====================

// Планета удалена перегенерацией — фолбэк «в системе X», статус не падает.
func TestPlayersPositionsBrokenTargetFallback(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w2", "player",
			`{"status":"orbit","object_type":"planet","object_id":"GONE","level":"orbit"}`},
	)
	// Имя планеты не найдено (планета удалена) → фолбэк «в системе X».
	mock.ExpectQuery(`SELECT id, name FROM planets WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1)
	p := resp.Players[0]
	require.Equal(t, "в системе Мир2", p["status"], "битая цель → фолбэк «в системе X»")
	require.Equal(t, "star", p["object_type"])
	require.Equal(t, "w2", p["object_id"])
}

// ==================== ПОВЕРХНОСТЬ СКРЫТА (спека 2026-09-21 §7.6 п.7) ====================

// Игрок со status=surface не отдаётся другим (решение создателя: скрывать).
func TestPlayersPositionsSurfaceHidden(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w2", "player",
			`{"status":"surface","level":"surface","object_type":"planet","object_id":"pl-1","biome":"x","hp":80,"landed_at":"2026-09-21T12:00:00Z"}`},
	)

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 0, "поверхность скрыта от других (guard status=surface)")
}

// ==================== АДМИН (И7) ====================
// Админ видит стоящих без фильтра радиуса.
func TestPlayersPositionsAdminSeesStanding(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	// Админ — без playerContextFrom (GetByID не вызывается).
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w3", "player", // w3 за радаром — админ видит
			`{"status":"orbit","object_type":"planet","object_id":"pl-1","level":"orbit"}`},
	)
	mock.ExpectQuery(`SELECT id, name FROM planets WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("pl-1", "Nemurzan II"))

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RoleAdmin)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1, "админ видит стоящего без радиуса (И7)")
	require.Equal(t, "у планеты Nemurzan II", resp.Players[0]["status"])
}