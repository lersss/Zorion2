// internal/handlers/surface_handlers_test.go
// Тесты высадки/возврата (спека 2026-09-21 §6): валидации land по порядку
// (идемпотентность до «на орбите»), жребий биома (§5.1), пакет прогулки (§7.1),
// серверное HP (§8.7), leave → орбита/нормализация (§4.3), normalizeMyPosition
// (ветка surface).
package handlers

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genplanet "zorion/internal/generator/planet"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/travel"
)

// ==================== ХЕЛПЕРЫ ====================

func newSurfaceHarness(t *testing.T) (*SurfaceHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	intraMgr := travel.NewIntrasystemManager(nil)
	return NewSurfaceHandlers(
		repository.NewUserRepository(db),
		repository.NewWorldRepository(db),
		repository.NewPlanetRepository(db),
		intraMgr,
	), mock
}

const surfaceUserQueryRe = `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination FROM users WHERE id = \$1`

// expectSurfaceUser — GetByIDWithPosition (worldID/posRaw могут быть nil),
// роль player (для админских тестов — expectSurfaceUserRole).
func expectSurfaceUser(mock sqlmock.Sqlmock, id string, worldID, posRaw interface{}) {
	expectSurfaceUserRole(mock, id, worldID, posRaw, "player")
}

// expectSurfaceUserRole — как expectSurfaceUser, но с заданной ролью
// (идея 2026-09-21: выбор биома — только admin/skycomposer).
func expectSurfaceUserRole(mock sqlmock.Sqlmock, id string, worldID, posRaw interface{}, role string) {
	mock.ExpectQuery(surfaceUserQueryRe).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(intraUserCols).
			AddRow(id, "player", "hash", nil, nil, worldID, "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, role, now(), now(), posRaw, nil))
}

// expectSurfacePlanetsLight — планеты системы (лёгкий запрос).
func expectSurfacePlanetsLight(mock sqlmock.Sqlmock, worldID string, rows ...[]driver.Value) {
	r := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"})
	for _, row := range rows {
		r.AddRow(row...)
	}
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(worldID).
		WillReturnRows(r)
}

// expectSurfaceUpdate — точечный UPDATE позиции.
func expectSurfaceUpdate(mock sqlmock.Sqlmock, userID string) {
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// biomeInPos — matcher позиции в UPDATE: JSON содержит выбранный биом
// («позиция с этим биомом», DoD идеи 2026-09-21).
type biomeInPos string

func (b biomeInPos) Match(v driver.Value) bool {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		return false
	}
	return strings.Contains(s, `"biome":"`+string(b)+`"`)
}

// expectSurfaceUpdateBiome — UPDATE позиции с проверкой биома.
func expectSurfaceUpdateBiome(mock sqlmock.Sqlmock, userID, biome string) {
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(biomeInPos(biome), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// testBiomeByCategory — первый биом каталога заданной категории (id в UTF-8
// не хардкодим — берём из справочника).
func testBiomeByCategory(t *testing.T, category string) genplanet.BiomeDef {
	t.Helper()
	for _, b := range genplanet.GetBiomeCatalog().Biomes {
		if b.Category == category {
			return b
		}
	}
	t.Fatalf("нет биома категории %q в каталоге", category)
	return genplanet.BiomeDef{}
}

// surfacePlanetData — JSON data планеты с одним биомом и физикой.
func surfacePlanetData(biomeID string, share float64, temperature, pressure, radioactivity float64, life bool) string {
	data := map[string]interface{}{
		"temperature": temperature,
		"gravity":     1.0,
		"life":        life,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": pressure,
			"composition":  map[string]interface{}{},
		},
		"core":   map[string]interface{}{"radioactivity": radioactivity},
		"biomes": []map[string]interface{}{{"form": biomeID, "share": share}},
	}
	b, _ := json.Marshal(data)
	return string(b)
}

// surfacePlanetRow — строка планеты для expectSurfacePlanetsLight.
func surfacePlanetRow(id, worldID, name, data string) []driver.Value {
	return []driver.Value{id, worldID, name, 0, data, now(), now()}
}

// surfacePlanetDataMulti — JSON data планеты с несколькими биомами
// (админский выбор биома: планета должна нести нужный биом с share > 0).
func surfacePlanetDataMulti(biomes map[string]float64, temperature, pressure, radioactivity float64, life bool) string {
	arr := []map[string]interface{}{}
	for form, share := range biomes {
		arr = append(arr, map[string]interface{}{"form": form, "share": share})
	}
	data := map[string]interface{}{
		"temperature": temperature,
		"gravity":     1.0,
		"life":        life,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": pressure,
			"composition":  map[string]interface{}{},
		},
		"core":   map[string]interface{}{"radioactivity": radioactivity},
		"biomes": arr,
	}
	b, _ := json.Marshal(data)
	return string(b)
}

func surfacePosJSON(planetID, biome string, hp float64, landedAt time.Time) string {
	b, _ := json.Marshal(models.SurfacePosition(planetID, biome, hp, landedAt))
	return string(b)
}

const orbitPlanetPos = `{"status":"orbit","object_type":"planet","object_id":"pl-1","level":"orbit"}`

func surfaceLandRequest(userID, planetID string) *http.Request {
	return surfaceLandBiomeRequest(userID, planetID, "")
}

// surfaceLandBiomeRequest — тело land с (необязательным) админским выбором биома.
func surfaceLandBiomeRequest(userID, planetID, biome string) *http.Request {
	body, _ := json.Marshal(SurfaceLandRequest{PlanetID: planetID, Biome: biome})
	req := httptest.NewRequest(http.MethodPost, "/api/surface/land", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

func surfaceLeaveRequest(userID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/surface/leave", nil)
	return withUserID(req, userID)
}

// ==================== LAND ====================

// Успешная высадка с орбиты планеты: 200 + пакет, позиция surface в UPDATE.
func TestSurfaceLandSuccess(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	biome := testBiomeByCategory(t, "литосфера")
	data := surfacePlanetData(biome.ID, 100, 288, 1.0, 0, true)

	expectSurfaceUser(mock, uid, "w1", orbitPlanetPos)
	expectIntraWorld(mock, "w1")
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "Nemurzan II", data))
	expectSurfaceUpdate(mock, uid)

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var pkg SurfacePackage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pkg))
	assert.Equal(t, "pl-1", pkg.PlanetID)
	assert.Equal(t, biome.ID, pkg.Biome)
	assert.Equal(t, biome.Name, pkg.BiomeName)
	near(t, 100, pkg.BiomeShare, 0.001)
	assert.Equal(t, surfaceSeed("pl-1", biome.ID), pkg.Seed)
	assert.True(t, pkg.Life)
	near(t, SurfaceHPMax, pkg.HP, 0.001)
	assert.Zero(t, pkg.Hazard.Total, "мягкая планета — урона нет")
	assert.NotEmpty(t, pkg.Sky.Star.Color)
	assert.Len(t, pkg.Sky.Bodies, 1, "тело системы — сама планета")
	assert.Equal(t, "player", pkg.Role, "роль игрока в пакете (§7.1, идея 2026-09-21)")
}

// buildSurfaceSky отдаёт planet_id только у планет (идея 2026-09-22 §8.4):
// спутникам картинки нет — id пуст, клиент рисует фолбэк-диск.
func TestBuildSurfaceSkyPlanetID(t *testing.T) {
	planets := []models.Planet{{
		ID:         "pl-1",
		Name:       "Nemurzan II",
		Size:       1.2,
		Satellites: []models.PlanetSatellite{{Name: "Луна", Size: 0.3}},
	}}
	sky := buildSurfaceSky(nil, planets)
	require.Len(t, sky.Bodies, 2)
	assert.Equal(t, "planet", sky.Bodies[0].Kind)
	assert.Equal(t, "pl-1", sky.Bodies[0].PlanetID, "планета несёт id для /api/planet-image")
	assert.Equal(t, "satellite", sky.Bodies[1].Kind)
	assert.Empty(t, sky.Bodies[1].PlanetID, "у спутника картинки нет — id не даём")
}

// Админ выбирает биом: позиция сохраняется с выбранным биомом, пакет отдаёт его
// и роль admin (идея 2026-09-21 §3).
func TestSurfaceLandAdminBiome(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	dominant := testBiomeByCategory(t, "литосфера")
	pick := testBiomeByCategory(t, "крио")
	// Выбираем НЕ доминирующий биом — жребий его почти наверняка не дал бы.
	data := surfacePlanetDataMulti(map[string]float64{dominant.ID: 90, pick.ID: 10}, 288, 1.0, 0, false)

	expectSurfaceUserRole(mock, uid, "w1", orbitPlanetPos, "admin")
	expectIntraWorld(mock, "w1")
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "X", data))
	expectSurfaceUpdateBiome(mock, uid, pick.ID)

	rec := execJSON(h.Land, surfaceLandBiomeRequest(uid, "pl-1", pick.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var pkg SurfacePackage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pkg))
	assert.Equal(t, pick.ID, pkg.Biome, "высадка в выбранный биом")
	assert.Equal(t, pick.Name, pkg.BiomeName)
	assert.Equal(t, "admin", pkg.Role)
}

// Не-админ с полем biome → явный 400 (не молчаливое игнорирование), гейт
// срабатывает ДО запросов системы/планет — только выборка пользователя.
func TestSurfaceLandBiomeNonAdmin(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	biome := testBiomeByCategory(t, "литосфера")

	expectSurfaceUser(mock, uid, "w1", orbitPlanetPos)

	rec := execJSON(h.Land, surfaceLandBiomeRequest(uid, "pl-1", biome.ID))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Выбор биома доступен только администратору")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Админ, но биома нет у планеты → 400 «Нет такого биома на планете».
func TestSurfaceLandAdminUnknownBiome(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	planetBiome := testBiomeByCategory(t, "литосфера")
	foreign := testBiomeByCategory(t, "крио")
	data := surfacePlanetData(planetBiome.ID, 100, 288, 1.0, 0, false)

	expectSurfaceUserRole(mock, uid, "w1", orbitPlanetPos, "admin")
	expectIntraWorld(mock, "w1")
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "X", data))

	rec := execJSON(h.Land, surfaceLandBiomeRequest(uid, "pl-1", foreign.ID))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Нет такого биома на планете")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Идемпотентность: на поверхности с биомом A повторный land с биомом B →
// сохранённый A присланный B игнорируется, без UPDATE (§6.1 шаг 2).
func TestSurfaceLandIdempotentIgnoresBiome(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	saved := testBiomeByCategory(t, "крио")
	other := testBiomeByCategory(t, "литосфера")
	data := surfacePlanetDataMulti(map[string]float64{saved.ID: 60, other.ID: 40}, 60, 0.05, 0, false)
	pos := surfacePosJSON("pl-1", saved.ID, 87, time.Now().Add(-60*time.Second))

	expectSurfaceUserRole(mock, uid, "w1", pos, "admin")
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "Ice", data))
	expectIntraWorld(mock, "w1") // buildWalkPackage: мир для light

	rec := execJSON(h.Land, surfaceLandBiomeRequest(uid, "pl-1", other.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var pkg SurfacePackage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pkg))
	assert.Equal(t, saved.ID, pkg.Biome, "рефреш не перебрасывает биом")
	assert.Equal(t, "admin", pkg.Role)
}

// Идемпотентность: повторный land при position surface — тот же биом, без UPDATE.
func TestSurfaceLandIdempotent(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	biome := testBiomeByCategory(t, "крио")
	data := surfacePlanetData(biome.ID, 100, 60, 0.05, 0, false)
	pos := surfacePosJSON("pl-1", biome.ID, 87, time.Now().Add(-60*time.Second))

	expectSurfaceUser(mock, uid, "w1", pos)
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "Ice", data))
	expectIntraWorld(mock, "w1") // buildWalkPackage: мир для light

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var pkg SurfacePackage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pkg))
	assert.Equal(t, biome.ID, pkg.Biome, "рефреш не перебрасывает биом")
	assert.Greater(t, pkg.Hazard.Total, 0.0, "жёсткая планета — фон есть")
	assert.Less(t, pkg.HP, SurfaceHPMax, "hp пересчитан от landed_at")
}

// surface на другой планете → идемпотентность не срабатывает → 400.
func TestSurfaceLandSurfaceOnOtherPlanet(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	biome := testBiomeByCategory(t, "литосфера")
	pos := surfacePosJSON("pl-2", biome.ID, 90, time.Now())

	expectSurfaceUser(mock, uid, "w1", pos)
	expectIntraWorld(mock, "w1")

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы только с орбиты планеты")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Не с орбиты планеты (позиция NULL) → 400.
func TestSurfaceLandNotOnOrbit(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"

	expectSurfaceUser(mock, uid, "w1", nil)
	expectIntraWorld(mock, "w1")

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы только с орбиты планеты")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Активный внутрисистемный полёт (in_flight) → 400.
func TestSurfaceLandInFlight(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	pos := `{"status":"in_flight","from_type":"star","from_id":"w1","to_type":"planet","to_id":"pl-1","start_time":1,"arrive_at":2}`

	expectSurfaceUser(mock, uid, "w1", pos)
	expectIntraWorld(mock, "w1")

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы в полёте")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Нет current_world_id → 400 «Вы не в системе».
func TestSurfaceLandNoWorld(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"

	expectSurfaceUser(mock, uid, nil, nil)

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы не в системе")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Биомы пусты → 400 «Нет данных о поверхности» (газовый гигант/старый мир).
func TestSurfaceLandNoBiomes(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	data := `{"temperature":288,"gravity":1.0,"biomes":[]}`

	expectSurfaceUser(mock, uid, "w1", orbitPlanetPos)
	expectIntraWorld(mock, "w1")
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "Giant", data))

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Нет данных о поверхности")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Биомы есть, но все form неизвестны каталогу → valid пуст → 400 (В5).
func TestSurfaceLandUnknownBiome(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	data := surfacePlanetData("__нет_такого_биома__", 100, 288, 1.0, 0, false)

	expectSurfaceUser(mock, uid, "w1", orbitPlanetPos)
	expectIntraWorld(mock, "w1")
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "X", data))

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Нет данных о поверхности")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Высадка на жёсткой планете: hp ровно 100 (landed_at округлён вверх — усечение
// вниз сразу отнимало бы HP на момент высадки).
func TestSurfaceLandHPStartsAt100(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	biome := testBiomeByCategory(t, "вулканизм")
	data := surfacePlanetData(biome.ID, 100, 700, 90, 0, false)

	expectSurfaceUser(mock, uid, "w1", orbitPlanetPos)
	expectIntraWorld(mock, "w1")
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "Venus", data))
	expectSurfaceUpdate(mock, uid)

	rec := execJSON(h.Land, surfaceLandRequest(uid, "pl-1"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var pkg SurfacePackage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pkg))
	assert.Greater(t, pkg.Hazard.Total, 0.0, "жёсткая планета — фон есть")
	assert.Equal(t, SurfaceHPMax, pkg.HP, "на момент высадки hp ровно 100")
}

// ==================== LEAVE ====================

// Возврат с поверхности: позиция → орбита планеты, hp посчитан, причина — ось.
func TestSurfaceLeaveToOrbit(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	biome := testBiomeByCategory(t, "вулканизм")
	data := surfacePlanetData(biome.ID, 100, 700, 90, 0, false)
	pos := surfacePosJSON("pl-1", biome.ID, 100, time.Now().Add(-10*time.Second))

	expectSurfaceUser(mock, uid, "w1", pos)
	expectSurfacePlanetsLight(mock, "w1", surfacePlanetRow("pl-1", "w1", "Venus", data))
	expectSurfaceUpdate(mock, uid)

	rec := execJSON(h.Leave, surfaceLeaveRequest(uid))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp SurfaceLeaveResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Position)
	assert.Equal(t, "orbit", resp.Position.Status)
	assert.Equal(t, "planet", resp.Position.ObjectType)
	assert.Equal(t, "pl-1", resp.Position.ObjectID)
	assert.Less(t, resp.HP, SurfaceHPMax)
	assert.Greater(t, resp.HP, 0.0)
	assert.Equal(t, "жара", resp.Cause, "доминирующая ось — жара")
}

// Не на поверхности → 200 no-op, без UPDATE (идемпотентность важнее строгости).
func TestSurfaceLeaveNoOp(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"

	expectSurfaceUser(mock, uid, "w1", orbitPlanetPos)

	rec := execJSON(h.Leave, surfaceLeaveRequest(uid))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp SurfaceLeaveResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Position)
	assert.Equal(t, "orbit", resp.Position.Status)
}

// Битая планета (удалена) → фолбэк «орбита звезды» (ИП-4').
func TestSurfaceLeaveBrokenPlanetFallback(t *testing.T) {
	h, mock := newSurfaceHarness(t)
	const uid = "11111111-1111-1111-1111-111111111111"
	pos := surfacePosJSON("GONE", "биом", 50, time.Now().Add(-5*time.Second))

	expectSurfaceUser(mock, uid, "w1", pos)
	expectSurfacePlanetsLight(mock, "w1") // планеты нет
	expectSurfaceUpdate(mock, uid)

	rec := execJSON(h.Leave, surfaceLeaveRequest(uid))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp SurfaceLeaveResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Position)
	assert.Equal(t, "star", resp.Position.ObjectType)
	assert.Equal(t, "w1", resp.Position.ObjectID)
}

// ==================== ЖРЕБИЙ И НОРМАЛИЗАЦИЯ ====================

// Жребий ∝ share (70/20/10): статистическая проверка.
func TestPickWalkBiomeDistribution(t *testing.T) {
	b1 := testBiomeByCategory(t, "литосфера")
	b2 := testBiomeByCategory(t, "крио")
	b3 := testBiomeByCategory(t, "вулканизм")
	valid := []models.Biome{
		{Form: b1.ID, Share: 70},
		{Form: b2.ID, Share: 20},
		{Form: b3.ID, Share: 10},
	}
	counts := map[string]int{}
	rng := rand.New(rand.NewSource(42))
	const n = 60000
	for i := 0; i < n; i++ {
		counts[pickWalkBiomeAt(valid, rng.Float64()*100)]++
	}
	near(t, 0.70, float64(counts[b1.ID])/n, 0.03, "доля ~70%")
	near(t, 0.20, float64(counts[b2.ID])/n, 0.03, "доля ~20%")
	near(t, 0.10, float64(counts[b3.ID])/n, 0.03, "доля ~10%")
}

// Валидация жребия: share ≤ 0 и неизвестные каталогу form исключаются (§5.1).
func TestWalkBiomeValidFilters(t *testing.T) {
	b1 := testBiomeByCategory(t, "литосфера")
	valid := walkBiomeValid(&models.Planet{Biomes: []models.Biome{
		{Form: b1.ID, Share: 50},
		{Form: b1.ID, Share: 0},      // share ≤ 0 — исключён
		{Form: "__нет__", Share: 50}, // неизвестный каталогу — исключён
	}})
	require.Len(t, valid, 1)
	assert.Equal(t, b1.ID, valid[0].Form)
}

// NormalizeSurfaceBiome: валидный → как есть; битый → доминирующий; пусто → "".
func TestNormalizeSurfaceBiome(t *testing.T) {
	b1 := testBiomeByCategory(t, "литосфера")
	b2 := testBiomeByCategory(t, "крио")
	p := &models.Planet{Biomes: []models.Biome{{Form: b2.ID, Share: 60}, {Form: b1.ID, Share: 40}}}

	assert.Equal(t, b1.ID, NormalizeSurfaceBiome(p, b1.ID), "валидный биом — как есть")
	assert.Equal(t, b2.ID, NormalizeSurfaceBiome(p, "битый"), "битый → доминирующий (max share)")
	assert.Equal(t, "", NormalizeSurfaceBiome(&models.Planet{}, b1.ID), "нет биомов → пусто")
	assert.Equal(t, "", NormalizeSurfaceBiome(nil, b1.ID), "nil-планета → пусто")
}

// normalizeMyPosition (ветка surface): валидная → как есть (hp пересчитан);
// битая планета → орбита звезды.
func TestNormalizeMyPositionSurface(t *testing.T) {
	b1 := testBiomeByCategory(t, "крио")
	planets := []models.Planet{{ID: "pl-1", WorldID: "w1",
		Temperature: 60, AtmosphereData: map[string]interface{}{"pressure_atm": 0.05},
		Biomes: []models.Biome{{Form: b1.ID, Share: 100}}}}
	validStar := func(id string) bool { return id == "w1" }

	pos := models.SurfacePosition("pl-1", b1.ID, 100, time.Now().Add(-10*time.Second))
	out := normalizeMyPosition(pos, "w1", planets, nil, validStar)
	require.NotNil(t, out)
	assert.Equal(t, "surface", out.Status)
	require.NotNil(t, out.HP)
	assert.Less(t, *out.HP, 100.0, "hp пересчитан от landed_at (§8.7)")

	// Битая планета (нет в системе) → орбита звезды.
	pos2 := models.SurfacePosition("GONE", b1.ID, 100, time.Now())
	out2 := normalizeMyPosition(pos2, "w1", planets, nil, validStar)
	assert.Equal(t, "star", out2.ObjectType)
	assert.Equal(t, "w1", out2.ObjectID)

	// Битый биом при живой планете → доминирующий.
	pos3 := models.SurfacePosition("pl-1", "битый", 100, time.Now())
	out3 := normalizeMyPosition(pos3, "w1", planets, nil, validStar)
	assert.Equal(t, b1.ID, out3.Biome)
}
