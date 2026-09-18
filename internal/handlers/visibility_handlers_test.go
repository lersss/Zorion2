// internal/handlers/visibility_handlers_test.go
// Применение серверной видимости (спека 77a §11): гибрид «звёздное поле» в
// /api/worlds/filter, 403 в модалке системы, фильтр NPC-агентов, скрытие
// координат в поиске агента, валидация цели /travel, позиции чужих игроков.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/auth"
	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/npc"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// withRole кладёт роль в контекст, как это делает AuthMiddleware.
func withRole(r *http.Request, role string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), auth.RoleKey, role))
}

// visWorlds — снапшот карты для тестов видимости: w1(0,0), w2(100,0), w3(1000,0).
func visWorlds() *mapcache.Snapshot {
	return mapcache.NewSnapshot([]mapcache.World{
		{ID: "w1", Name: "Мир1", X: 0, Y: 0, Spectral: "G", Temp: 5772},
		{ID: "w2", Name: "Мир2", X: 100, Y: 0, Spectral: "K", Temp: 4000},
		{ID: "w3", Name: "Мир3", X: 1000, Y: 0, Spectral: "M", Temp: 3000},
	})
}

// visAdminHandlers — AdminHandlers с visibility (игрок в w1, радар radar_1 → 800).
func visAdminHandlers(t *testing.T) (*AdminHandlers, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	mc := mapcache.NewManager()
	mc.Replace(visWorlds())

	userRepo := repository.NewUserRepository(db)
	knowledge := repository.NewKnowledgeRepository(db)
	v := NewVisibility(userRepo, tm, mc, knowledge)

	h := NewAdminHandlers(repository.NewWorldRepository(db), db, mc)
	h.SetVisibility(v)
	h.SetTravelManager(tm)
	return h, mock
}

// expectPlayerUser — ожидание GetByID игрока (в w1, радар radar_1).
func expectPlayerUser(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id",
			"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at",
		}).AddRow(id, "player", "hash", nil, nil, "w1", "ship_strela.svg", nil, "starter",
			`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, "player", now(), now()))
}

// expectKnownWorlds — ожидание KnownWorldIDs (пусто по умолчанию).
func expectKnownWorlds(mock sqlmock.Sqlmock, worlds ...string) {
	rows := sqlmock.NewRows([]string{"world_id"})
	for _, w := range worlds {
		rows.AddRow(w)
	}
	mock.ExpectQuery(`SELECT DISTINCT p.world_id FROM player_planet_knowledge k JOIN planets p ON p.id = k.planet_id WHERE k.user_id = \$1`).
		WithArgs("u1").
		WillReturnRows(rows)
}

// ==================== /api/worlds/filter: ОТКРЫТАЯ ЗВЕЗДА (И11 новый) ====================

func TestFilterWorldsAllStarsFull(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Вид звезды — открытая информация (спека 77a §5.5/И11): сервер не
	// запрашивает ни пользователя, ни знание — кластеры отдаются как админу.
	req := httptest.NewRequest(http.MethodGet,
		"/api/worlds/filter?x_min=-10&x_max=1100&y_min=-10&y_max=10&cell=50", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.FilterWorldsHandler, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var clusters []worldCluster
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &clusters))
	require.Len(t, clusters, 3)

	byX := map[float64]worldCluster{}
	for _, c := range clusters {
		byX[c.X] = c
	}
	require.Equal(t, "Мир1", byX[0].SampleName, "в радиусе — полные данные")
	require.Equal(t, "Мир2", byX[100].SampleName, "в радиусе — полные данные")
	require.Equal(t, 1, byX[0].Count)

	// w3 (1000,0) — за радаром: полный вид звезды (имя/спектр/координаты —
	// открытая информация, «видно в телескоп»); точка-огонёк отменена.
	c3 := byX[1000]
	require.Equal(t, 1, c3.Count)
	require.Equal(t, "Мир3", c3.SampleName, "за-радарная звезда — полный вид (И11)")
	require.Equal(t, "M", c3.SampleSpectral)
	require.Equal(t, 1000.0, c3.X, "координаты открыты")
}

func TestFilterWorldsAdminSeesAll(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// admin/skycomposer — без фильтра (И7): ни GetByID, ни KnownWorldIDs не вызываются.
	req := httptest.NewRequest(http.MethodGet,
		"/api/worlds/filter?x_min=-10&x_max=1100&y_min=-10&y_max=10&cell=50", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RoleAdmin))

	rec := execJSON(h.FilterWorldsHandler, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var clusters []worldCluster
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &clusters))
	require.Len(t, clusters, 3)
	require.Equal(t, "Мир3", clusters[2].SampleName, "админ видит всё")
}

// ==================== /api/worlds/{id}/planets: 403 ВНЕ РАДИУСА ====================

func TestGetPlanetsByWorldOutsideRadius(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Сначала планеты системы (пусто — поселений нет), затем мир.
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs("w3").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}))

	// Мир w3 (1000,0) — за радаром (800) и не «зажжён» → 403.
	mock.ExpectQuery(`SELECT name, COALESCE\(spectral_class,''\), star_type, system_type, stellar_mods, stellar_mass, age, temperature, coord_x, coord_y FROM worlds WHERE id = \$1`).
		WithArgs("w3").
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "spectral_class", "star_type", "system_type", "stellar_mods",
			"stellar_mass", "age", "temperature", "coord_x", "coord_y",
		}).AddRow("Мир3", "M", "star", "single", nil, nil, nil, 3000, 1000, 0))

	expectPlayerUser(mock, userID)
	expectKnownWorlds(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/worlds/w3/planets", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.GetPlanetsByWorld, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "вне зоны видимости")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetPlanetsByWorldInsideRadius(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Планеты системы: p1 (без поселений).
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs("w2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w2", "Планета1", 0, `{"type":"землеподобная","surface_dominant":"вода"}`, now(), now()))

	// Поселения планеты (пусто).
	mock.ExpectQuery(`SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id FROM settlements WHERE planet_id = ANY\(\$1\) ORDER BY created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "planet_id", "population", "population_exact", "stability",
			"computed_at", "created_at", "updated_at", "race_id",
		}))

	// Мир w2 (100,0) — в радиусе 800: планеты отдаются, детали скрыты без знания.
	mock.ExpectQuery(`SELECT name, COALESCE\(spectral_class,''\), star_type, system_type, stellar_mods, stellar_mass, age, temperature, coord_x, coord_y FROM worlds WHERE id = \$1`).
		WithArgs("w2").
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "spectral_class", "star_type", "system_type", "stellar_mods",
			"stellar_mass", "age", "temperature", "coord_x", "coord_y",
		}).AddRow("Мир2", "K", "star", "single", nil, nil, nil, 4000, 100, 0))

	expectPlayerUser(mock, userID)
	expectKnownWorlds(mock)

	// Сканер установлен → ленивый прогон ScanSystem.
	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\), COALESCE\(p.data->'surface_composition', '\{\}'::jsonb\), \(SELECT COUNT\(\*\) FROM settlements s WHERE s.planet_id = p.id\) FROM planets p WHERE p.world_id = \$1`).
		WithArgs("w2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}).
			AddRow("p1", "вода", `{"вода":100}`, 0))
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs(userID, "p1", sqlmock.AnyArg(), "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Чтение знания после скана.
	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs(userID, "p1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}).
			AddRow(userID, "p1", `{"surface_dominant":"вода","surface_composition":{"вода":100},"settlements_count":0}`, now(), "scanner"))

	req := httptest.NewRequest(http.MethodGet, "/api/worlds/w2/planets", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.GetPlanetsByWorld, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Planets []models.Planet `json:"planets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	p := resp.Planets[0]
	require.NotNil(t, p.Knowledge, "знание сканера приложено")
	require.Equal(t, "вода", p.Knowledge.SurfaceDominant)
	require.Empty(t, p.SurfaceDominant, "детали планеты скрыты из тела (знание — в Knowledge)")
	require.Empty(t, p.Atmosphere, "атмосфера всегда скрыта для player")
	require.Nil(t, p.Settlements, "детали поселений всегда скрыты")
}

// «Зажжённая» знанием система ВНЕ радиуса (w3, 1000,0): модалка открывается
// с ИМЕЮЩИМСЯ знанием (даже устаревшим, fresh:false), но сканер НЕ запускается
// (нет upsert) — иначе знание известных систем никогда не стареет из любой
// точки (спека 77a §6.1, подрыв И8/И9).
func TestGetPlanetsByWorldIgnitedOutsideRadiusNoScan(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Планеты системы: p1.
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs("w3").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w3", "Планета1", 0, `{"type":"землеподобная"}`, now(), now()))

	// Поселения планеты (пусто).
	mock.ExpectQuery(`SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id FROM settlements WHERE planet_id = ANY\(\$1\) ORDER BY created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "planet_id", "population", "population_exact", "stability",
			"computed_at", "created_at", "updated_at", "race_id",
		}))

	// Мир w3 (1000,0) — за радаром, но «зажжён» знанием.
	mock.ExpectQuery(`SELECT name, COALESCE\(spectral_class,''\), star_type, system_type, stellar_mods, stellar_mass, age, temperature, coord_x, coord_y FROM worlds WHERE id = \$1`).
		WithArgs("w3").
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "spectral_class", "star_type", "system_type", "stellar_mods",
			"stellar_mass", "age", "temperature", "coord_x", "coord_y",
		}).AddRow("Мир3", "M", "star", "single", nil, nil, nil, 3000, 1000, 0))

	expectPlayerUser(mock, userID)
	expectKnownWorlds(mock, "w3")

	// Чтение ИМЕЮЩЕГОСЯ знания: устаревшее (8 дней назад → fresh:false).
	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs(userID, "p1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}).
			AddRow(userID, "p1", `{"surface_dominant":"скалы","settlements_count":0}`, now().Add(-8*24*time.Hour), "scanner"))

	req := httptest.NewRequest(http.MethodGet, "/api/worlds/w3/planets", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.GetPlanetsByWorld, req)
	require.Equal(t, http.StatusOK, rec.Code)
	// ExpectationsWereMet: если ScanSystem вызовется — будет неожиданный
	// запрос (upsert) и тест упадёт. Здесь его НЕТ.
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Planets []models.Planet `json:"planets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	require.NotNil(t, resp.Planets[0].Knowledge, "имеющееся знание показано")
	require.False(t, resp.Planets[0].Knowledge.Fresh, "устаревшее знание помечено fresh:false (И8)")
}

// ==================== /api/npc/positions: ФИЛЬТР ПО РАДИУСУ ====================

func TestNPCPositionsFilteredByRadius(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	mc := mapcache.NewManager()
	mc.Replace(visWorlds())
	userRepo := repository.NewUserRepository(db)
	v := NewVisibility(userRepo, tm, mc, repository.NewKnowledgeRepository(db))

	npcManager := npc.NewManager(repository.NewNPCRepository(db), npc.NewMapCacheSource(mc), npc.DefaultSettings())
	h := NewAdminNPCHandlers(repository.NewNPCRepository(db), repository.NewWorldRepository(db), npcManager)
	h.SetVisibility(v)

	// Агент a1 в (0,0) — в радиусе; a2 в (1000,0) — за радаром. Оба в полёте
	// (90a): радиус-фильтр проверяется на летящих.
	npcManager.SetPositions([]npc.InterpolatedPosition{
		{ID: "a1", Name: "Агент1", X: 0, Y: 0, Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1"},
		{ID: "a2", Name: "Агент2", X: 1000, Y: 0, Status: models.NPCAgentStatusFlying, CurrentWorldID: "w3"},
	})

	expectPlayerUser(mock, "u1")

	req := httptest.NewRequest(http.MethodGet, "/api/npc/positions", nil)
	req = withUserID(req, "u1")
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.Positions, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Positions []npc.InterpolatedPosition `json:"positions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Positions, 1, "агент за радаром скрыт")
	require.Equal(t, "a1", resp.Positions[0].ID)
}

// ==================== /api/npc/search: КООРДИНАТЫ СКРЫТЫ ВНЕ РАДИУСА ====================

func TestNPCSearchHidesCoordsOutsideRadius(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	mc := mapcache.NewManager()
	mc.Replace(visWorlds())
	userRepo := repository.NewUserRepository(db)
	v := NewVisibility(userRepo, tm, mc, repository.NewKnowledgeRepository(db))

	npcManager := npc.NewManager(repository.NewNPCRepository(db), npc.NewMapCacheSource(mc), npc.DefaultSettings())
	h := NewAdminNPCHandlers(repository.NewNPCRepository(db), repository.NewWorldRepository(db), npcManager)
	h.SetVisibility(v)

	// Поиск по имени находит агента a2 (в 1000,0 — за радаром).
	mock.ExpectQuery(`SELECT id, name, status, current_world_id, target_world_id FROM npc_agents WHERE name ILIKE \$1 LIMIT \$2`).
		WithArgs("%Агент2%", 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "target_world_id",
		}).AddRow("a2", "Агент2", "idle", "w3", nil))

	npcManager.SetPositions([]npc.InterpolatedPosition{
		{ID: "a2", Name: "Агент2", X: 1000, Y: 0, Status: "idle", CurrentWorldID: "w3"},
	})

	expectPlayerUser(mock, "u1")

	req := httptest.NewRequest(http.MethodGet, "/api/npc/search?q=Агент2", nil)
	req = withUserID(req, "u1")
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.SearchAgent, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Results []map[string]interface{} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 1)
	r := resp.Results[0]
	require.Equal(t, "Агент2", r["name"], "имя — справочное, находится")
	require.Nil(t, r["x"], "координаты скрыты вне радиуса")
	require.Nil(t, r["y"])
	require.Equal(t, true, r["outside_visibility"], "пометка «вне зоны видимости»")
}

// ==================== /travel: ПОЛЁТ К ЛЮБОЙ ЗВЕЗДЕ (И6 новый) ====================

func TestStartTravelFarStarAllowed(t *testing.T) {
	h, tm, mock := newTravelHarness(t)
	const userID = "u1"
	const fromWorld = "w1"
	const target = "w3"

	// Цель w3 (1000,0) — за радаром (800) и не известна: полёт разрешён
	// (И6 новый, решение создателя 2026-09-17: «полёт к любой звезде»;
	// топливо/дальность — будущий задел). Валидация — только существование цели.
	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 1000, 0)

	req := travelRequest(userID, target)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.StartTravel, req)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NotNil(t, tm.GetFlight(userID), "полёт к неизвестной звезде запущен")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== /api/entities/search: ОТКРЫТАЯ ЗВЕЗДА, ЗАКРЫТАЯ СИСТЕМА (спека 77a §10) ====================

func TestSearchEntitiesFarStarFullPlanetHidden(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Поиск находит звезду w3 (1000,0 — за радаром) и планету p1 из w3.
	// Все три запроса (звёзды/планеты/спутники) идут с одним q.
	mock.ExpectQuery(`SELECT id, name, COALESCE\(spectral_class,''\), coord_x, coord_y FROM worlds WHERE LOWER\(name\) = \$1 ORDER BY name LIMIT \$2`).
		WithArgs("мир3", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "spectral", "coord_x", "coord_y"}).
			AddRow("w3", "Мир3", "M", 1000, 0))
	mock.ExpectQuery(`SELECT p.id, p.name, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y, COALESCE\(p.data->>'type', ''\) FROM planets p JOIN worlds w ON w.id = p.world_id WHERE LOWER\(p.name\) = \$1 ORDER BY p.name LIMIT \$2`).
		WithArgs("мир3", 19).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "world_id", "world_name", "spectral", "coord_x", "coord_y", "type"}).
			AddRow("p1", "Планета3", "w3", "Мир3", "M", 1000, 0, "землеподобная"))
	mock.ExpectQuery(`SELECT sat->>'id', sat->>'name', p.id, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y FROM planets p JOIN worlds w ON w.id = p.world_id CROSS JOIN LATERAL jsonb_array_elements\(p.data->'satellites'\) AS sat WHERE LOWER\(sat->>'name'\) = \$1 ORDER BY sat->>'name' LIMIT \$2`).
		WithArgs("мир3", 18).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "planet_id", "world_id", "world_name", "spectral", "coord_x", "coord_y"}))

	expectPlayerUser(mock, userID)
	expectKnownWorlds(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/entities/search?q=мир3", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.SearchEntitiesHandler, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Results []searchResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 2, "звезда + планета")

	// Звезда за радаром — полный вид (И11 новый): имя/спектр/координаты открыты.
	star := resp.Results[0]
	require.Equal(t, searchKindWorld, star.Kind)
	require.Equal(t, "Мир3", star.Name)
	require.Equal(t, "M", star.Spectral)
	require.Equal(t, 1000.0, star.CoordX)

	// Планета неизвестной системы — без имени звезды/координат (И11 в части
	// содержимого остаётся): цепочка search → /worlds/{id} закрыта.
	planet := resp.Results[1]
	require.Equal(t, searchKindPlanet, planet.Kind)
	require.Equal(t, "Планета3", planet.Name, "планета найдена (справочник имён)")
	require.Empty(t, planet.WorldName, "имя за-радарной звезды скрыто (И11)")
	require.Empty(t, planet.WorldID, "world_id скрыт — цепочка search → /worlds/{id} закрыта (И11)")
	require.Equal(t, 0.0, planet.CoordX, "координаты скрыты")
}

func TestSearchEntitiesKnownStarReturned(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Звезда w2 (100,0 — в радиусе 800) — полный результат.
	mock.ExpectQuery(`SELECT id, name, COALESCE\(spectral_class,''\), coord_x, coord_y FROM worlds WHERE LOWER\(name\) = \$1 ORDER BY name LIMIT \$2`).
		WithArgs("мир2", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "spectral", "coord_x", "coord_y"}).
			AddRow("w2", "Мир2", "K", 100, 0))
	mock.ExpectQuery(`SELECT p.id, p.name, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y, COALESCE\(p.data->>'type', ''\) FROM planets p JOIN worlds w ON w.id = p.world_id WHERE LOWER\(p.name\) = \$1 ORDER BY p.name LIMIT \$2`).
		WithArgs("мир2", 19).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "world_id", "world_name", "spectral", "coord_x", "coord_y", "type"}))
	mock.ExpectQuery(`SELECT sat->>'id', sat->>'name', p.id, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y FROM planets p JOIN worlds w ON w.id = p.world_id CROSS JOIN LATERAL jsonb_array_elements\(p.data->'satellites'\) AS sat WHERE LOWER\(sat->>'name'\) = \$1 ORDER BY sat->>'name' LIMIT \$2`).
		WithArgs("мир2", 19).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "planet_id", "world_id", "world_name", "spectral", "coord_x", "coord_y"}))

	expectPlayerUser(mock, userID)
	expectKnownWorlds(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/entities/search?q=мир2", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.SearchEntitiesHandler, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Results []searchResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 1)
	require.Equal(t, "Мир2", resp.Results[0].Name, "звезда в радиусе — полный результат")
	require.Equal(t, 100.0, resp.Results[0].CoordX)
}

// ==================== /worlds и /worlds/{id}: LEGACY-РОУТЫ (И11) ====================

// visWorldHandlers — WorldHandlers с visibility (игрок в w1, радар radar_1 → 800).
func visWorldHandlers(t *testing.T) (*WorldHandlers, sqlmock.Sqlmock, *travel.Manager) {
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

	h := NewWorldHandlers(
		repository.NewWorldRepository(db),
		repository.NewLocationRepository(db),
		repository.NewAssignmentRepository(db),
	)
	h.SetVisibility(v)
	return h, mock, tm
}

// expectWorldRow — строка мира для sqlmock (worldColumns).
func expectWorldRow(mock sqlmock.Sqlmock, id string, x, y float64) {
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
			"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
			"created_at", "updated_at",
		}).AddRow(id, "Мир"+id, x, y, "G", 5772, "star", "single", nil, nil, nil, now(), now()))
}

// expectEmptyLocationsAssignments — пустые locations/assignments для 200-ответа.
func expectEmptyLocationsAssignments(mock sqlmock.Sqlmock, worldID string) {
	mock.ExpectQuery(`SELECT id, world_id, name, is_inhabited, state, created_at, updated_at FROM locations WHERE world_id = \$1 ORDER BY name`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "is_inhabited", "state", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT id, world_id, author_type, author_id, title, description, type, reward, expires_at, status, effects, created_at, updated_at FROM assignments WHERE world_id = \$1 AND status = 'open'`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "world_id", "author_type", "author_id", "title", "description",
			"type", "reward", "expires_at", "status", "effects", "created_at", "updated_at",
		}))
}

// За-радарная звезда (w3, 1000,0) — 403 для player (И11).
func TestGetWorldOutsideRadius(t *testing.T) {
	h, mock, _ := visWorldHandlers(t)
	const userID = "u1"

	expectWorldRow(mock, "w3", 1000, 0)
	expectPlayerUser(mock, userID)
	expectKnownWorlds(mock)

	req := httptest.NewRequest(http.MethodGet, "/worlds/w3", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.GetWorld, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "вне зоны видимости")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Звезда в радиусе (w2, 100,0) — 200 с полными данными.
func TestGetWorldInsideRadius(t *testing.T) {
	h, mock, _ := visWorldHandlers(t)
	const userID = "u1"

	expectWorldRow(mock, "w2", 100, 0)
	expectPlayerUser(mock, userID)
	expectEmptyLocationsAssignments(mock, "w2")

	req := httptest.NewRequest(http.MethodGet, "/worlds/w2", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.GetWorld, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		World *models.World `json:"world"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.World)
	require.Equal(t, "Мирw2", resp.World.Name)
}

// Цель активного полёта (w3 за радаром) — 200: знание цели принадлежит игроку,
// восстановление полёта после рефреша (42a) не ломается.
func TestGetWorldFlightTargetAllowed(t *testing.T) {
	h, mock, tm := visWorldHandlers(t)
	const userID = "u1"

	tm.StartFlight(userID, "w1", "w3", 0, 0, time.Hour, nil)

	expectWorldRow(mock, "w3", 1000, 0)
	expectPlayerUser(mock, userID)
	expectEmptyLocationsAssignments(mock, "w3")

	req := httptest.NewRequest(http.MethodGet, "/worlds/w3", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.GetWorld, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// /worlds для player — только миры в радиусе/известные (иначе полный каталог,
// обход гибрида «звёздное поле», И11).
func TestGetAllWorldsFilteredForPlayer(t *testing.T) {
	h, mock, _ := visWorldHandlers(t)
	const userID = "u1"

	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
			"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
			"created_at", "updated_at",
		}).
			AddRow("w1", "Мир1", 0, 0, "G", 5772, "star", "single", nil, nil, nil, now(), now()).
			AddRow("w2", "Мир2", 100, 0, "K", 4000, "star", "single", nil, nil, nil, now(), now()).
			AddRow("w3", "Мир3", 1000, 0, "M", 3000, "star", "single", nil, nil, nil, now(), now()))

	expectPlayerUser(mock, userID)
	// w3 (1000,0) за радаром и не известна → KnownWorldIDs (пусто) → отфильтрована.
	expectKnownWorlds(mock)

	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.GetAllWorlds, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var worlds []*models.World
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &worlds))
	require.Len(t, worlds, 2, "за-радарная звезда отфильтрована")
	require.Equal(t, "Мир1", worlds[0].Name)
	require.Equal(t, "Мир2", worlds[1].Name)
}

// /worlds для admin — без фильтра (И7).
func TestGetAllWorldsAdminSeesAll(t *testing.T) {
	h, mock, _ := visWorldHandlers(t)
	const userID = "u1"

	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
			"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
			"created_at", "updated_at",
		}).AddRow("w3", "Мир3", 1000, 0, "M", 3000, "star", "single", nil, nil, nil, now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RoleAdmin))

	rec := execJSON(h.GetAllWorlds, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var worlds []*models.World
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &worlds))
	require.Len(t, worlds, 1, "админ видит всё")
}

// ==================== /api/players/positions: ЧУЖИЕ ИГРОКИ В РАДИУСЕ ====================

func TestPlayersPositionsFilteredByRadius(t *testing.T) {
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

	const userID = "u1"
	expectPlayerUser(mock, userID)

	// Другие игроки (оба в полёте, 90a): p2 летит w1→w2 (старт 0,0 — в
	// радиусе), p3 летит w3→w1 (старт 1000,0 — за радаром).
	tm.StartFlight("p2", "w1", "w2", 0, 0, time.Hour, nil)
	tm.StartFlight("p3", "w3", "w1", 1000, 0, time.Hour, nil)

	mock.ExpectQuery(`SELECT id, username, ship_icon, ship_color, current_world_id, role FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "ship_icon", "ship_color", "current_world_id", "role"}).
			AddRow(userID, "player", "ship_strela.svg", nil, "w1", "player").
			AddRow("p2", "alice", "shark.png", nil, "w2", "player").
			AddRow("p3", "bob", "crescent.png", nil, "w3", "player"))

	req := httptest.NewRequest(http.MethodGet, "/api/players/positions", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.PlayersPositions, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1, "только игрок в радиусе")
	require.Equal(t, "alice", resp.Players[0]["username"])
	require.Equal(t, "в полёте", resp.Players[0]["status"])
	require.InDelta(t, 0.0, resp.Players[0]["x"].(float64), 1.0, "старт сегмента w1 (0,0)")
}

// Только корабли в полёте (90a): игрок без активного полёта не отдаётся.
func TestPlayersPositionsOnlyFlying(t *testing.T) {
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

	const userID = "u1"
	expectPlayerUser(mock, userID)

	// p2 стоит в w2 (без полёта) — скрыт (90a); p3 летит w1→w2 — отдан.
	tm.StartFlight("p3", "w1", "w2", 0, 0, time.Hour, nil)

	mock.ExpectQuery(`SELECT id, username, ship_icon, ship_color, current_world_id, role FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "ship_icon", "ship_color", "current_world_id", "role"}).
			AddRow(userID, "player", "ship_strela.svg", nil, "w1", "player").
			AddRow("p2", "alice", "shark.png", nil, "w2", "player").
			AddRow("p3", "bob", "crescent.png", nil, "w3", "player"))

	req := httptest.NewRequest(http.MethodGet, "/api/players/positions", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.PlayersPositions, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1, "игрок без полёта скрыт (90a)")
	require.Equal(t, "bob", resp.Players[0]["username"])
	require.Equal(t, "в полёте", resp.Players[0]["status"])
}

// Админ тоже видит только летящих игроков (90a, вариант a).
func TestPlayersPositionsAdminOnlyFlying(t *testing.T) {
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

	const userID = "u1"

	// p2 стоит в w2 (без полёта) — скрыт (90a); p3 летит w3→w1 — отдан.
	tm.StartFlight("p3", "w3", "w1", 1000, 0, time.Hour, nil)

	mock.ExpectQuery(`SELECT id, username, ship_icon, ship_color, current_world_id, role FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "ship_icon", "ship_color", "current_world_id", "role"}).
			AddRow(userID, "player", "ship_strela.svg", nil, "w1", "player").
			AddRow("p2", "alice", "shark.png", nil, "w2", "player").
			AddRow("p3", "bob", "crescent.png", nil, "w3", "player"))

	req := httptest.NewRequest(http.MethodGet, "/api/players/positions", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RoleAdmin))

	rec := execJSON(h.PlayersPositions, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1, "админ видит только летящих (90a)")
	require.Equal(t, "bob", resp.Players[0]["username"])
	require.Equal(t, "в полёте", resp.Players[0]["status"])
}

// Только корабли в полёте (90a): idle-агент скрыт, flying отдан.
func TestNPCPositionsOnlyFlying(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	mc := mapcache.NewManager()
	mc.Replace(visWorlds())
	userRepo := repository.NewUserRepository(db)
	v := NewVisibility(userRepo, tm, mc, repository.NewKnowledgeRepository(db))

	npcManager := npc.NewManager(repository.NewNPCRepository(db), npc.NewMapCacheSource(mc), npc.DefaultSettings())
	h := NewAdminNPCHandlers(repository.NewNPCRepository(db), repository.NewWorldRepository(db), npcManager)
	h.SetVisibility(v)

	// a1 idle в (0,0) — скрыт (90a); a2 flying в (0,0) — отдан.
	npcManager.SetPositions([]npc.InterpolatedPosition{
		{ID: "a1", Name: "Агент1", X: 0, Y: 0, Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
		{ID: "a2", Name: "Агент2", X: 0, Y: 0, Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1"},
	})

	expectPlayerUser(mock, "u1")

	req := httptest.NewRequest(http.MethodGet, "/api/npc/positions", nil)
	req = withUserID(req, "u1")
	req = withRole(req, string(models.RolePlayer))

	rec := execJSON(h.Positions, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Positions []npc.InterpolatedPosition `json:"positions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Positions, 1, "idle-агент скрыт (90a)")
	require.Equal(t, "a2", resp.Positions[0].ID)
}

// Админ тоже видит только летящих агентов (90a, вариант a).
func TestNPCPositionsAdminOnlyFlying(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	mc := mapcache.NewManager()
	mc.Replace(visWorlds())
	userRepo := repository.NewUserRepository(db)
	v := NewVisibility(userRepo, tm, mc, repository.NewKnowledgeRepository(db))

	npcManager := npc.NewManager(repository.NewNPCRepository(db), npc.NewMapCacheSource(mc), npc.DefaultSettings())
	h := NewAdminNPCHandlers(repository.NewNPCRepository(db), repository.NewWorldRepository(db), npcManager)
	h.SetVisibility(v)

	npcManager.SetPositions([]npc.InterpolatedPosition{
		{ID: "a1", Name: "Агент1", X: 0, Y: 0, Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
		{ID: "a2", Name: "Агент2", X: 0, Y: 0, Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/npc/positions", nil)
	req = withUserID(req, "u1")
	req = withRole(req, string(models.RoleAdmin))

	rec := execJSON(h.Positions, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Positions []npc.InterpolatedPosition `json:"positions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Positions, 1, "админ видит только летящих агентов (90a)")
	require.Equal(t, "a2", resp.Positions[0].ID)
}