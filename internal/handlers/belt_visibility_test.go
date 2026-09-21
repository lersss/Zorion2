// internal/handlers/belt_visibility_test.go
// Тесты этапа 2 поясов малых тел (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §9): показ/знание
// (V1–V6) и полёт к поясу (F1–F16). Отдельный файл — visibility_handlers_test.go
// занят параллельным потоком (контракты B1), не редактируется.
package handlers

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// ==================== ХЕЛПЕРЫ ====================

// beltRow — строка system_belts для sqlmock (порядок колонок GetBeltsByWorldID).
func beltRow(id, worldID, kind, name string, orbitIndex interface{}, radiusAU, widthAU, mass, bodySizeKm float64, composition string, visible bool, data string) []driver.Value {
	return []driver.Value{
		id, worldID, kind, name, orbitIndex, radiusAU, widthAU, mass, bodySizeKm,
		composition, visible, data, now(), now(),
	}
}

// expectBelts — ожидание GetBeltsByWorldID (SELECT ... FROM system_belts).
func expectBelts(mock sqlmock.Sqlmock, worldID string, rows ...[]driver.Value) {
	r := sqlmock.NewRows([]string{
		"id", "world_id", "kind", "name", "orbit_index", "radius_au", "width_au",
		"mass", "body_size_km", "composition", "visible", "data", "created_at", "updated_at",
	})
	for _, row := range rows {
		r.AddRow(row...)
	}
	mock.ExpectQuery(`SELECT id, world_id, kind, name, orbit_index, radius_au, width_au, mass,\s+body_size_km, composition, visible, data, created_at, updated_at\s+FROM system_belts\s+WHERE world_id = \$1\s+ORDER BY radius_au ASC`).
		WithArgs(worldID).
		WillReturnRows(r)
}

// beltHarness — AdminHandlers с visibility (игрок в w1, радар radar_1 → 800).
// Собственный харнесс (не visAdminHandlers из занятого файла).
func beltHarness(t *testing.T) (*AdminHandlers, sqlmock.Sqlmock) {
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

// expectBeltWorldInfo — ожидание SELECT мира в planet_handler (w2).
func expectBeltWorldInfo(mock sqlmock.Sqlmock, worldID string, mods string) {
	expectBeltWorldInfoAt(mock, worldID, mods, 100, 0)
}

// expectBeltWorldInfoAt — то же с явными координатами (для проверки радиуса).
func expectBeltWorldInfoAt(mock sqlmock.Sqlmock, worldID, mods string, x, y float64) {
	mock.ExpectQuery(`SELECT name, COALESCE\(spectral_class,''\), star_type, system_type, stellar_mods, stellar_mass, age, temperature, coord_x, coord_y FROM worlds WHERE id = \$1`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "spectral_class", "star_type", "system_type", "stellar_mods",
			"stellar_mass", "age", "temperature", "coord_x", "coord_y",
		}).AddRow("Мир2", "K", "star", "single", mods, nil, nil, 4000, x, y))
}

// expectBeltPlanets — ожидание GetPlanetsByWorldID (полный путь модалки).
func expectBeltPlanets(mock sqlmock.Sqlmock, worldID string, rows ...[]driver.Value) {
	r := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"})
	for _, row := range rows {
		r.AddRow(row...)
	}
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(worldID).
		WillReturnRows(r)
}

// expectBeltPlanetAttachments — пустые поселения/фракции/строения/залежи.
func expectBeltPlanetAttachments(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id FROM settlements WHERE planet_id = ANY\(\$1\) ORDER BY created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "planet_id", "population", "population_exact", "stability",
			"computed_at", "created_at", "updated_at", "race_id",
		}))
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)
}

// beltModalRequest — GET /api/worlds/{id}/planets с ролью.
func beltModalRequest(userID, worldID, role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/worlds/"+worldID+"/planets", nil)
	req = withUserID(req, userID)
	req = withRole(req, role)
	return req
}

// beltModalResponse — разбор ответа модалки (belts как сырой JSON).
type beltModalResponse struct {
	Planets     []models.Planet          `json:"planets"`
	Belts       []map[string]interface{} `json:"belts"`
	CompanionID string                   `json:"companion_id"`
	MyPosition  *models.CurrentPosition  `json:"my_position"`
}

// ==================== V1–V6: ПОКАЗ И ЗНАНИЕ ====================

// V1: player в радиусе получает belts с базовыми полями.
func TestBeltsInModalResponse(t *testing.T) {
	h, mock := beltHarness(t)
	const userID = "u1"

	expectBeltPlanets(mock, "w2", []driver.Value{"p1", "w2", "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, "w2", "")
	expectPlayerUserWithPosition(mock, userID, "w2", nil)
	expectKnownWorlds(mock)
	// Пояса грузятся ДО ScanSystem (порядок хендлера).
	expectBelts(mock, "w2",
		beltRow("b1", "w2", "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{"rock":0.7,"iron":0.2,"ice":0.1}`, true, `{}`))
	// Сканер в радиусе → ScanSystem (планет нет — пусто).
	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\), COALESCE\(p.data->'surface_composition', '\{\}'::jsonb\), \(SELECT COUNT\(\*\) FROM settlements s WHERE s.planet_id = p.id\) FROM planets p WHERE p.world_id = \$1`).
		WithArgs("w2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}))

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, "w2", string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp beltModalResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Belts, 1)
	b := resp.Belts[0]
	assert.Equal(t, "b1", b["id"])
	assert.Equal(t, "asteroid", b["kind"])
	assert.Equal(t, "Пояс астероидов", b["name"])
	assert.InDelta(t, 3.0, b["radius_au"].(float64), 0.001)
	assert.InDelta(t, 0.6, b["width_au"].(float64), 0.001)
	assert.InDelta(t, 120.0, b["body_size_km"].(float64), 0.001)
	assert.InDelta(t, 0.05, b["mass"].(float64), 0.001)
	// Система в радиусе → состав раскрыт.
	require.NotNil(t, b["composition"], "состав при знании (система в радиусе)")
	// Служебные поля не отдаются.
	assert.Nil(t, b["visible"], "visible не отдаётся игроку")
	assert.Nil(t, b["data"], "data не отдаётся игроку")
	assert.Nil(t, b["created_at"], "created_at не отдаётся игроку")
}

// V2: состав есть в радиусе; для известной системы ВНЕ радиуса — нет.
func TestBeltCompositionInRadarOnly(t *testing.T) {
	h, mock := beltHarness(t)
	const userID = "u1"

	// Система w3 (1000,0) — вне радара (800), но «зажжена» знанием.
	expectBeltPlanets(mock, "w3", []driver.Value{"p1", "w3", "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfoAt(mock, "w3", "", 1000, 0)
	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectKnownWorlds(mock, "w3")
	// Вне радиуса — ScanSystem НЕ вызывается.
	expectBelts(mock, "w3",
		beltRow("b1", "w3", "kuiper", "Пояс Койпера", nil, 35.0, 10.0, 0.2, 50.0, `{"rock":0.5,"ice":0.5}`, true, `{}`))

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, "w3", string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp beltModalResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Belts, 1)
	assert.Nil(t, resp.Belts[0]["composition"], "вне радиуса состав не раскрыт")
	assert.Nil(t, resp.Belts[0]["orbit_index"], "у Койпера orbit_index null")
}

// V3: присутствие в поясе → состав виден без сканера (система вне радиуса).
func TestBeltCompositionAtPresence(t *testing.T) {
	h, mock := beltHarness(t)
	const userID = "u1"

	expectBeltPlanets(mock, "w3", []driver.Value{"p1", "w3", "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfoAt(mock, "w3", "", 1000, 0)
	// Игрок в w3, позиция — орбита пояса b1.
	pos := `{"status":"orbit","object_type":"belt","object_id":"b1","level":"orbit"}`
	expectPlayerUserWithPosition(mock, userID, "w3", pos)
	expectKnownWorlds(mock, "w3")
	expectBelts(mock, "w3",
		beltRow("b1", "w3", "kuiper", "Пояс Койпера", nil, 35.0, 10.0, 0.2, 50.0, `{"rock":0.5,"ice":0.5}`, true, `{}`))

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, "w3", string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp beltModalResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Belts, 1)
	require.NotNil(t, resp.Belts[0]["composition"], "присутствие раскрывает состав")
	require.NotNil(t, resp.MyPosition)
	assert.Equal(t, "belt", resp.MyPosition.ObjectType, "позиция-в-поясе сохранена (не фолбэк)")
	assert.Equal(t, "b1", resp.MyPosition.ObjectID)
}

// V4: visible=false не отдаётся player, отдаётся admin.
func TestBeltInvisibleHiddenForPlayer(t *testing.T) {
	h, mock := beltHarness(t)
	const userID = "u1"

	expectBeltPlanets(mock, "w2", []driver.Value{"p1", "w2", "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, "w2", "")
	expectPlayerUserWithPosition(mock, userID, "w2", nil)
	expectKnownWorlds(mock)
	expectBelts(mock, "w2",
		beltRow("b1", "w2", "asteroid", "Видимый", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`),
		beltRow("b2", "w2", "debris", "Скрытый", nil, 5.0, 1.0, 0.01, 10.0, `{}`, false, `{}`))
	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\), COALESCE\(p.data->'surface_composition', '\{\}'::jsonb\), \(SELECT COUNT\(\*\) FROM settlements s WHERE s.planet_id = p.id\) FROM planets p WHERE p.world_id = \$1`).
		WithArgs("w2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}))

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, "w2", string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp beltModalResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Belts, 1, "visible=false не отдаётся player")
	assert.Equal(t, "b1", resp.Belts[0]["id"])
}

// V5: admin видит все пояса (включая data/visible).
func TestBeltDetailsAdminUnfiltered(t *testing.T) {
	h, mock := beltHarness(t)
	const userID = "u1"

	expectBeltPlanets(mock, "w2", []driver.Value{"p1", "w2", "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, "w2", "")
	// admin — GetByIDWithPosition всё равно вызывается (С-3), но без фильтра.
	expectPlayerUserWithPosition(mock, userID, "w2", nil)
	expectBelts(mock, "w2",
		beltRow("b1", "w2", "asteroid", "Видимый", 2, 3.0, 0.6, 0.05, 120.0, `{"rock":1}`, true, `{"resources":[]}`),
		beltRow("b2", "w2", "debris", "Скрытый", nil, 5.0, 1.0, 0.01, 10.0, `{}`, false, `{}`))

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, "w2", string(models.RoleAdmin)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp beltModalResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Belts, 2, "admin видит все пояса (И7)")
	b := resp.Belts[0]
	require.NotNil(t, b["data"], "admin видит data")
	require.NotNil(t, b["visible"], "admin видит visible")
	require.NotNil(t, b["composition"], "admin видит состав")
}

// V6: аддитивность — planets/companion_id/my_position не изменились.
func TestBeltsResponseAdditive(t *testing.T) {
	h, mock := beltHarness(t)
	const userID = "u1"

	expectBeltPlanets(mock, "w2", []driver.Value{"p1", "w2", "Планета1", 0, `{"type":"землеподобная","orbit_radius_au":1.0}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, "w2", `{"binary_type":"wide","companion":"K","companion_sep_au":1000}`)
	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectPlayerUserWithPosition(mock, userID, "w2", pos)
	expectKnownWorlds(mock)
	expectBelts(mock, "w2")
	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\), COALESCE\(p.data->'surface_composition', '\{\}'::jsonb\), \(SELECT COUNT\(\*\) FROM settlements s WHERE s.planet_id = p.id\) FROM planets p WHERE p.world_id = \$1`).
		WithArgs("w2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}).
			AddRow("p1", "вода", `{"вода":100}`, 0))
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs(userID, "p1", sqlmock.AnyArg(), "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs(userID, "p1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}).
			AddRow(userID, "p1", `{"surface_dominant":"вода","settlements_count":0}`, now(), "scanner"))

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, "w2", string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp beltModalResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1, "planets не изменились")
	assert.Equal(t, "companion:w2", resp.CompanionID, "companion_id не изменился")
	require.NotNil(t, resp.MyPosition, "my_position не изменился")
	assert.Equal(t, "planet", resp.MyPosition.ObjectType)
	require.NotNil(t, resp.Belts, "belts аддитивно (пустой массив, не null)")
	assert.Len(t, resp.Belts, 0)
}

// H4: admin-путь без visibility (visibility == nil) отдаёт belts: [] (не null)
// при отсутствии поясов — контракт §4.2/§7.6 (секция «Поясов нет»).
func TestBeltsAdminNoVisibilityEmptyArray(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Харнесс БЕЗ SetVisibility: h.visibility == nil (админ-путь без видимости).
	h := NewAdminHandlers(repository.NewWorldRepository(db), db, mapcache.NewManager())

	expectBeltPlanets(mock, "w2", []driver.Value{"p1", "w2", "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, "w2", "")
	expectBelts(mock, "w2") // поясов нет → nil-срез из репозитория

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest("u1", "w2", string(models.RoleAdmin)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	// Сырой JSON: belts — пустой массив, не null.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Equal(t, "[]", strings.TrimSpace(string(raw["belts"])),
		"belts должен быть [] (не null) на admin-пути без visibility")
}

// ==================== F1–F16: ПОЛЁТ К ПОЯСУ ====================

// expectIntraBelts — ожидание GetBeltsByWorldID в StartIntraFlight.
func expectIntraBelts(mock sqlmock.Sqlmock, worldID string, rows ...[]driver.Value) {
	expectBelts(mock, worldID, rows...)
}

// expectIntraPlanetsWithBelts — планеты + пояса системы для StartIntraFlight
// (порядок хендлера: планеты → пояса). Заменяет expectIntraPlanets (тот даёт
// пустые пояса) в тестах с непустыми поясами.
func expectIntraPlanetsWithBelts(mock sqlmock.Sqlmock, worldID string, planetRows [][]driver.Value, beltRows ...[]driver.Value) {
	r := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"})
	for _, row := range planetRows {
		r.AddRow(row...)
	}
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(worldID).
		WillReturnRows(r)
	expectBelts(mock, worldID, beltRows...)
}

// F1: старт к поясу своей системы → 202, to_type='belt'.
func TestBeltFlightStarts(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1")
	expectIntraPlanetsWithBelts(mock, "w1",
		[][]driver.Value{planetRow("p1", "w1", "Планета1", 0, 1.0)},
		beltRow("b1", "w1", "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`))
	expectIntraStartAtomic(mock, userID, "star", "w1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "belt", "b1"))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "belt", resp.ToType)
	assert.Equal(t, "b1", resp.ToID)
	// dist = |0 − 3| = 3 а.е. → 3 сек (минимум).
	assert.Equal(t, 3, resp.Duration)

	flight := intraMgr.GetIntraFlight(userID)
	require.NotNil(t, flight)
	assert.Equal(t, "belt", flight.ToType)
}

// F2: dist = |r(from) − belt.radius_au|; длительность = CalcIntraDuration.
func TestBeltFlightDistance(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	// from — орбита планеты p1 (r=1), цель — пояс b1 (r=50) → dist=49 → 14.7 сек.
	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanetsWithBelts(mock, "w1",
		[][]driver.Value{planetRow("p1", "w1", "Планета1", 0, 1.0)},
		beltRow("b1", "w1", "kuiper", "Пояс Койпера", nil, 50.0, 10.0, 0.2, 50.0, `{}`, true, `{}`))
	expectIntraStartAtomic(mock, userID, "planet", "p1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "belt", "b1"))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "planet", resp.FromType)
	assert.Equal(t, "p1", resp.FromID)
	// dist = |1 − 50| = 49 → 49×0.3 = 14.7 сек.
	assert.Equal(t, 14, resp.Duration, "dist=49 → 14.7 сек (усечение до int)")
}

// F3: пояс чужой системы → 400 «Объект не найден в вашей системе».
func TestBeltFlightForeignRejected(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1")
	expectIntraPlanetsWithBelts(mock, "w1",
		[][]driver.Value{planetRow("p1", "w1", "Планета1", 0, 1.0)})
	// Поясов в системе w1 нет — цель b-other не найдена.

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "belt", "b-other"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Объект не найден в вашей системе")
	require.NoError(t, mock.ExpectationsWereMet())
}

// F4: несуществующий/visible=false пояс для player → 400.
func TestBeltFlightMissingRejected(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1")
	expectIntraPlanetsWithBelts(mock, "w1",
		[][]driver.Value{planetRow("p1", "w1", "Планета1", 0, 1.0)},
		// Пояс есть, но visible=false — player его не видит → невалидная цель.
		beltRow("b1", "w1", "debris", "Скрытый", nil, 5.0, 1.0, 0.01, 10.0, `{}`, false, `{}`))

	rec := execJSON(h.StartIntraFlight, withRole(intraRequest(userID, "belt", "b1"), string(models.RolePlayer)))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Объект не найден в вашей системе")
	require.NoError(t, mock.ExpectationsWereMet())
}

// F5: «Вы уже в поясе» → 400.
func TestBeltFlightAlreadyThere(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	pos := `{"status":"orbit","object_type":"belt","object_id":"b1","level":"orbit"}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanetsWithBelts(mock, "w1",
		[][]driver.Value{planetRow("p1", "w1", "Планета1", 0, 1.0)},
		beltRow("b1", "w1", "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`))

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "belt", "b1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы уже в поясе")
	require.NoError(t, mock.ExpectationsWereMet())
}

// F6: активный полёт к тому же поясу → 202 (идемпотентность).
func TestBeltFlightIdempotent(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "belt", "b1", time.Hour, nil)

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1")
	expectIntraPlanetsWithBelts(mock, "w1",
		[][]driver.Value{planetRow("p1", "w1", "Планета1", 0, 1.0)},
		beltRow("b1", "w1", "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`))

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "belt", "b1"))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "b1", resp.ToID)
	assert.Equal(t, 3600, resp.Duration, "текущий полёт, без перезапуска")
}

// F7: без двигателя → 400 (91a).
func TestBeltFlightNoEngine(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUserNoEngine(mock, userID)

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "belt", "b1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Двигатель не установлен")
	require.NoError(t, mock.ExpectationsWereMet())
}

// F8: onArrival → {orbit, belt, id, level:orbit}.
func TestBeltArrivalPosition(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	// Валидация цели прибытия: пояс жив (GetBeltsByWorldID).
	expectBelts(mock, "w1",
		beltRow("b1", "w1", "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`))
	// Атомарно: позиция orbit + удаление строки.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(jsonContains{[]string{`"status":"orbit"`, `"object_type":"belt"`, `"object_id":"b1"`, `"level":"orbit"`}}, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1 AND start_time = \$2 AND arrive_at = \$3`).
		WithArgs(userID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "belt", "b1", 30*time.Millisecond,
		NewIntraArrivalHandler(h.intraRepo, h.planetRepo, h.knowledgeRepo))

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond)
}

// F9: пояс удалён → фолбэк «орбита звезды» (ИП-4).
func TestBeltArrivalBrokenTarget(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	// Пояс удалён — GetBeltsByWorldID пусто → arrivalTargetValid false.
	expectBelts(mock, "w1")
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(jsonContains{[]string{`"status":"orbit"`, `"object_type":"star"`, `"object_id":"w1"`}}, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1 AND start_time = \$2 AND arrive_at = \$3`).
		WithArgs(userID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "belt", "b1", 30*time.Millisecond,
		NewIntraArrivalHandler(h.intraRepo, h.planetRepo, h.knowledgeRepo))

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond)
}

// F10: валидный belt сохраняется; битый → «орбита звезды».
func TestNormalizeMyPositionBelt(t *testing.T) {
	belts := []models.Belt{{ID: "b1", WorldID: "w1", Kind: "asteroid", RadiusAU: 3.0}}
	validStar := func(id string) bool { return id == "w1" }

	pos := models.OrbitPosition("belt", "b1")
	out := normalizeMyPosition(pos, "w1", nil, belts, validStar)
	require.NotNil(t, out)
	assert.Equal(t, "orbit", out.Status)
	assert.Equal(t, "belt", out.ObjectType, "валидный belt сохраняется")
	assert.Equal(t, "b1", out.ObjectID)

	broken := models.OrbitPosition("belt", "GONE")
	out2 := normalizeMyPosition(broken, "w1", nil, belts, validStar)
	assert.Equal(t, "star", out2.ObjectType, "битый belt → орбита звезды")
	assert.Equal(t, "w1", out2.ObjectID)
}

// F11: CHECK-миграция допускает belt; прежние значения сохранены.
// Не-тавтологично: читает артефакт migrations/000065_belt_flight.sql и
// проверяет, что CHECK from_type/to_type поимённо включает 'belt' вместе с
// прежними 'star'/'planet'/'satellite'. Живая БД не требуется.
func TestMigrationCheckAllowsBelt(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "000065_belt_flight.sql")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "миграция 000065_belt_flight.sql должна существовать")
	sql := string(raw)

	// Оба CHECK (from_type и to_type) расширены значением 'belt'.
	require.Contains(t, sql, "player_intrasystem_flights_from_type_check",
		"миграция пересоздаёт CHECK from_type")
	require.Contains(t, sql, "player_intrasystem_flights_to_type_check",
		"миграция пересоздаёт CHECK to_type")

	// Списки типов поимённо: прежние значения + belt (анти-ловушка §5.4 —
	// расширение не должно терять star/planet/satellite).
	for _, typ := range []string{"'star'", "'planet'", "'satellite'", "'belt'"} {
		require.Contains(t, sql, typ, "CHECK должен включать "+typ)
	}
	// from_type и to_type — оба с belt (два вхождения в ADD CONSTRAINT).
	require.GreaterOrEqual(t, strings.Count(sql, "'belt'"), 2,
		"belt должен быть и в from_type, и в to_type")
	// companion не входит в CHECK (транслируется в star на внутрисистемном слое).
	require.NotContains(t, sql, "'companion'",
		"companion не входит в CHECK (транслируется в star)")
}

// F13: статус «в поясе X» + object_type/object_id; битый → «в системе X».
func TestPlayersPositionsBeltStatus(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w2", "player",
			`{"status":"orbit","object_type":"belt","object_id":"b1","level":"orbit"}`},
	)
	mock.ExpectQuery(`SELECT id, name FROM system_belts WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("b1", "Пояс астероидов"))

	rec := execJSON(h.PlayersPositions, playersPositionsRequest(userID, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp struct {
		Players []map[string]interface{} `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Players, 1)
	p := resp.Players[0]
	assert.Equal(t, "в поясе Пояс астероидов", p["status"])
	assert.Equal(t, "belt", p["object_type"])
	assert.Equal(t, "b1", p["object_id"])
}

// F13b: битый пояс → фолбэк «в системе X».
func TestPlayersPositionsBeltBrokenFallback(t *testing.T) {
	h, mock := newPlayersPositionsHarness(t)
	const userID = "u1"

	expectPlayerUser(mock, userID)
	expectPositionsUsers(mock,
		[]driver.Value{userID, "player", "ship_strela.svg", nil, "w1", "player", nil},
		[]driver.Value{"p2", "alice", "shark.png", nil, "w2", "player",
			`{"status":"orbit","object_type":"belt","object_id":"GONE","level":"orbit"}`},
	)
	mock.ExpectQuery(`SELECT id, name FROM system_belts WHERE id = ANY\(\$1\)`).
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
	assert.Equal(t, "в системе Мир2", p["status"], "битый пояс → фолбэк")
	assert.Equal(t, "star", p["object_type"])
}

// F14: /travel destination belt → pending_destination; автостарт → intra belt;
// битая цель → фолбэк без 400.
func TestCompositeDestinationBelt(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w2","object_type":"belt","object_id":"b1"}`)
	// autostartIntra: мир → пояс жив → двигатель → current_world_id == w2 →
	// планеты + пояса (радиус) → StartAtomic (to_type='belt', без трансляции).
	expectWorld(mock, target, 10, 0)
	expectBelts(mock, target,
		beltRow("b1", target, "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`))
	expectUser(mock, userID, target)
	expectPlanetsLight(mock, target, travelPlanetRow("p1", target, `{"orbit_radius_au":1.0}`))
	expectBelts(mock, target,
		beltRow("b1", target, "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO player_intrasystem_flights \(user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs(userID, target, "star", target, "belt", "b1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(jsonContains{[]string{`"status":"in_flight"`, `"to_type":"belt"`, `"to_id":"b1"`}}, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())

	f := intraManager.GetIntraFlight(userID)
	require.NotNil(t, f, "автостарт запустил внутрисистемный полёт к поясу")
	assert.Equal(t, "belt", f.ToType, "пояс — без трансляции (в отличие от companion)")
	assert.Equal(t, "b1", f.ToID)
}

// F14b: битая цель-belt → фолбэк без 400 (намерение очищается).
func TestCompositeDestinationBeltBroken(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w2","object_type":"belt","object_id":"GONE"}`)
	expectWorld(mock, target, 10, 0)
	expectBelts(mock, target) // пояс удалён
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID), "битая цель — полёт не стартует")
}

// F15: destination belt чужого/несуществующего мира → 400.
func TestBeltDestinationForeignRejected(t *testing.T) {
	h, _, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectWorld(mock, target, 10, 0)
	expectUser(mock, userID, fromWorld)
	// Планеты системы w2 (валидация belt идёт после планетного списка).
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(target).
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", target, "Планета1", 0, `{"type":"землеподобная"}`, now(), now()))
	// Поясов в системе w2 нет — цель b-other не найдена.
	expectBelts(mock, target)

	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
		`{"world_id":"w2","destination":{"object_type":"belt","object_id":"b-other"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := execJSON(h.StartTravel, withUserID(req, userID))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Объект не найден в системе назначения")
	require.NoError(t, mock.ExpectationsWereMet())
}

// F16: star/planet/satellite-полёты не изменились (регресс).
func TestIntraFlightLegacyUnchanged(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))
	// Пояса грузятся всегда (новый запрос, пусто), но не влияют на планетный полёт.
	expectIntraStartAtomic(mock, userID, "star", "w1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "planet", resp.ToType)
	assert.Equal(t, "p1", resp.ToID)
	assert.Equal(t, 3, resp.Duration)
}

// ==================== РЕПОЗИТОРИЙ ====================

// GetBeltsByWorldID: разбор строки (orbit_index NULL, composition/data JSONB).
func TestGetBeltsByWorldID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	expectBelts(mock, "w1",
		beltRow("b1", "w1", "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{"rock":0.7,"iron":0.2,"ice":0.1}`, true, `{"resources":[]}`),
		beltRow("b2", "w1", "kuiper", "Пояс Койпера", nil, 35.0, 10.0, 0.2, 50.0, `{"rock":0.5,"ice":0.5}`, true, `{}`))

	repo := repository.NewPlanetRepository(db)
	belts, err := repo.GetBeltsByWorldID("w1")
	require.NoError(t, err)
	require.Len(t, belts, 2)
	require.NoError(t, mock.ExpectationsWereMet())

	assert.Equal(t, "b1", belts[0].ID)
	assert.Equal(t, "asteroid", belts[0].Kind)
	require.NotNil(t, belts[0].OrbitIndex)
	assert.Equal(t, 2, *belts[0].OrbitIndex)
	assert.InDelta(t, 0.7, belts[0].Composition["rock"], 0.001)
	assert.True(t, belts[0].Visible)
	assert.NotNil(t, belts[0].Data)

	assert.Nil(t, belts[1].OrbitIndex, "у Койпера orbit_index null")
	assert.Equal(t, "kuiper", belts[1].Kind)
}

// GetBeltsByWorldID: пусто → nil-срез без ошибки.
func TestGetBeltsByWorldIDEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	expectBelts(mock, "w1")

	repo := repository.NewPlanetRepository(db)
	belts, err := repo.GetBeltsByWorldID("w1")
	require.NoError(t, err)
	assert.Len(t, belts, 0)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== stripBeltDetails (чистая функция) ====================

func TestStripBeltDetails(t *testing.T) {
	idx := 2
	b := models.Belt{
		ID: "b1", WorldID: "w1", Kind: "asteroid", Name: "Пояс",
		OrbitIndex: &idx, RadiusAU: 3.0, WidthAU: 0.6, Mass: 0.05, BodySizeKm: 120.0,
		Composition: map[string]float64{"rock": 1.0},
		Visible:     true,
		Data:        map[string]interface{}{"resources": []interface{}{}},
	}

	// Без знания — состав отсутствует, служебные не отдаются.
	v := stripBeltDetails(b, false)
	assert.Equal(t, "b1", v.ID)
	assert.Equal(t, "asteroid", v.Kind)
	assert.InDelta(t, 3.0, v.RadiusAU, 0.001)
	assert.InDelta(t, 0.05, v.Mass, 0.001)
	assert.Nil(t, v.Composition, "без знания состав не отдаётся")

	// Со знанием — состав есть.
	v2 := stripBeltDetails(b, true)
	require.NotNil(t, v2.Composition)
	assert.InDelta(t, 1.0, v2.Composition["rock"], 0.001)

	// BeltView не содержит visible/data (проверка через JSON).
	raw, err := json.Marshal(v2)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"visible"`)
	assert.NotContains(t, string(raw), `"data"`)
	assert.NotContains(t, string(raw), `"created_at"`)
}

// ==================== applyBeltVisibility (чистая функция) ====================

func TestApplyBeltVisibility(t *testing.T) {
	belts := []models.Belt{
		{ID: "b1", Kind: "asteroid", Visible: true, Composition: map[string]float64{"rock": 1}},
		{ID: "b2", Kind: "debris", Visible: false, Composition: map[string]float64{"ice": 1}},
	}

	// В радиусе — состав раскрыт, visible=false отфильтрован.
	out := applyBeltVisibility(belts, true, nil)
	require.Len(t, out, 1)
	assert.Equal(t, "b1", out[0].ID)
	require.NotNil(t, out[0].Composition)

	// Вне радиуса, без присутствия — состав не раскрыт.
	out2 := applyBeltVisibility(belts, false, nil)
	require.Len(t, out2, 1)
	assert.Nil(t, out2[0].Composition)

	// Присутствие в поясе b1 — состав раскрыт без радиуса.
	pos := models.OrbitPosition("belt", "b1")
	out3 := applyBeltVisibility(belts, false, pos)
	require.Len(t, out3, 1)
	require.NotNil(t, out3[0].Composition)
}

// ==================== objectRadiusAU / targetInSystem (belt) ====================

func TestObjectRadiusAUBelt(t *testing.T) {
	world := &models.World{ID: "w1"}
	belts := []models.Belt{{ID: "b1", RadiusAU: 3.0}}

	r, ok := objectRadiusAU(world, nil, belts, "belt", "b1")
	require.True(t, ok)
	assert.InDelta(t, 3.0, r, 0.001)

	_, ok = objectRadiusAU(world, nil, belts, "belt", "GONE")
	assert.False(t, ok)
}

func TestTargetInSystemBelt(t *testing.T) {
	world := &models.World{ID: "w1"}
	belts := []models.Belt{{ID: "b1"}}

	assert.True(t, targetInSystem(world, nil, belts, "belt", "b1"))
	assert.False(t, targetInSystem(world, nil, belts, "belt", "GONE"))
}

// ==================== destinationInSystem (belt) ====================

func TestDestinationInSystemBelt(t *testing.T) {
	world := &models.World{ID: "w2"}
	belts := []models.Belt{{ID: "b1"}}

	assert.True(t, destinationInSystem(world, nil, belts, "belt", "b1"))
	assert.False(t, destinationInSystem(world, nil, belts, "belt", "GONE"))
}

// ==================== beltPresence / visibleBelts ====================

func TestBeltPresence(t *testing.T) {
	pos := models.OrbitPosition("belt", "b1")
	assert.True(t, beltPresence(pos, "b1"))
	assert.False(t, beltPresence(pos, "b2"))
	assert.False(t, beltPresence(nil, "b1"))
	assert.False(t, beltPresence(models.OrbitPosition("planet", "b1"), "b1"))
}

func TestVisibleBelts(t *testing.T) {
	belts := []models.Belt{
		{ID: "b1", Visible: true},
		{ID: "b2", Visible: false},
	}
	out := visibleBelts(belts)
	require.Len(t, out, 1)
	assert.Equal(t, "b1", out[0].ID)
}

// ==================== F12: Restore (отложено из-за локейта) ====================
// TestBeltFlightRestore требует правки cmd/server/main.go (блок Restore) —
// занят параллельным потоком (контракты B1). Отложено, см. отчёт.

var _ = sql.ErrNoRows
