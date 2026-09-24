// internal/handlers/admin_structures_test.go
//
// Тесты админ-инструмента «Построить структуру на планете» (спека
// 2026-09-24-постройка-структур-на-планете §5–§11): права, валидации
// (403/409/422), создание поселения/строения, класс по якорю, население,
// build-options и поиск владельца.
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
	"zorion/internal/generator"
)

const (
	ownerUUID     = "11111111-1111-1111-1111-111111111111"
	structurePath = "/admin/planets/p1/structures"
)

// structureReq — POST-запрос создания структуры.
func structureReq(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, structurePath, strings.NewReader(body))
}

// expectStructurePlanet — общие ожидания до транзакции: планета+мир, регионы.
func expectStructurePlanet(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT p\.world_id, w\.coord_x, w\.coord_y FROM planets p JOIN worlds w ON w\.id = p\.world_id WHERE p\.id = \$1`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"world_id", "coord_x", "coord_y"}).AddRow("w1", 0.0, 0.0))
	mock.ExpectQuery(`FROM regions`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "center_x", "center_y", "radius", "color", "world_count", "profile", "profile_intensity", "race_id"}))
}

// expectStructureAnchor — резолв класса по якорю: базовый тип 100 → корень 148
// рекурсивным обходом parent_id вверх до корня (спека §4).
func expectStructureAnchor(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte(`100`)))
	mock.ExpectQuery(`WITH RECURSIVE up AS`).WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(148)))
}

// ==================== ПРАВА (T1) ====================

// T1: player без прав → 403 (через auth.AdminAuth, как на роуте).
func TestCreateStructurePlayerForbidden(t *testing.T) {
	requireHandlerJWT(t)
	tok, err := auth.GenerateToken("player1", string(auth.RolePlayer))
	require.NoError(t, err)

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}

	handler := auth.AdminAuth(h.CreateStructure)
	req := structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"` + ownerUUID + `"}`)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// ==================== ВАЛИДАЦИИ (T2/T3/T6/T15) ====================

// T3: owner_type вне домена → 422 (до обращения к БД).
func TestCreateStructureBadOwnerType(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}

	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"npc","owner_id":"`+ownerUUID+`"}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T3: owner_id не UUID → 422 (до обращения к БД).
func TestCreateStructureOwnerIDNotUUID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}

	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"nope"}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T15: population = 0 → 422 (население должно быть > 0).
func TestCreateStructurePopulationZero(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}

	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`","population":0}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T2: скрытый тип → 422.
func TestCreateStructureHiddenType(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectStructurePlanet(mock)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name, hidden FROM producer_types WHERE id = \$1`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "hidden"}).AddRow("Аутпост", true))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`"}`))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T6: якорь класса не найден → 422 «класс не определён».
func TestCreateStructureNoAnchor(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectStructurePlanet(mock)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name, hidden FROM producer_types WHERE id = \$1`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "hidden"}).AddRow("Аутпост", false))
	// Якоря нет.
	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`"}`))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T3: владельца нет в таблице → 422.
func TestCreateStructureOwnerNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectStructurePlanet(mock)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name, hidden FROM producer_types WHERE id = \$1`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "hidden"}).AddRow("Аутпост", false))
	expectStructureAnchor(mock)
	mock.ExpectQuery(`WITH RECURSIVE up AS`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(148)))
	mock.ExpectQuery(`SELECT name FROM factions WHERE id = \$1`).WithArgs(ownerUUID).
		WillReturnRows(sqlmock.NewRows([]string{"name"}))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`"}`))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ГЕЙТЫ (T7) ====================

// T7: пакман ест миры → 409, ничего не создано.
func TestCreateStructurePacmanGate(t *testing.T) {
	startJob(t, generator.JobPacman)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`"}`))

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T7: universeMutationMu занят → 409.
func TestCreateStructureMutexGate(t *testing.T) {
	universeMutationMu.Lock()
	defer universeMutationMu.Unlock()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`"}`))

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== СОЗДАНИЕ (T4/T5/T8) ====================

// T4/T8: поселение — INSERT settlements с типом/владельцем/населением,
// ответ {created{type_name=запрошенная}, settlements[]}.
func TestCreateStructureSettlement(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectStructurePlanet(mock)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name, hidden FROM producer_types WHERE id = \$1`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "hidden"}).AddRow("Аутпост", false))
	expectStructureAnchor(mock)
	mock.ExpectQuery(`WITH RECURSIVE up AS`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(148)))
	mock.ExpectQuery(`SELECT name FROM factions WHERE id = \$1`).WithArgs(ownerUUID).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Люди"))
	mock.ExpectExec(`INSERT INTO settlements`).
		WithArgs(sqlmock.AnyArg(), "p1", 2000, float64(2000), 100, nil, int64(152), "faction", ownerUUID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Свежие блоки карточки: GetPlanetByID (чтение полным путём).
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at\s+FROM planets\s+WHERE id = \$1`).WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Планета", 0, `{}`, time.Now(), time.Now()))
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(settlementSelectCols()).
			AddRow("new1", "p1", 2000, float64(2000), 100, time.Now(), time.Now(), time.Now(), nil, int64(152), "Аутпост", nil, nil, "faction", ownerUUID))
	mock.ExpectQuery(`SELECT 'player', id::text, username FROM users WHERE id::text = ANY\(\$1\)[\s\S]*SELECT 'faction'[\s\S]*SELECT 'agent'`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"type", "id", "name"}).AddRow("faction", ownerUUID, "Люди"))
	expectOwnerPassEmptyRegexp(mock)
	// У поселения есть тип (152) → настройки типа и числа скорости пары.
	mock.ExpectQuery(`SELECT id, params->'eat', params->'effects' FROM producer_types WHERE id = ANY\(\$1\)`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "eat", "effects"}))
	mock.ExpectQuery(`SELECT producer_type_id, recipe_id, rate FROM producer_recipes WHERE producer_type_id = ANY\(\$1\)`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate"}))
	expectModesSettlementLog(mock)
	expectEmptyFactionsBuildings(mock)

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`","population":2000}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body struct {
		Created struct {
			Kind      string `json:"kind"`
			ID        string `json:"id"`
			TypeName  string `json:"type_name"`
			OwnerName string `json:"owner_name"`
		} `json:"created"`
		Settlements []map[string]interface{} `json:"settlements"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "settlement", body.Created.Kind)
	assert.Equal(t, "Аутпост", body.Created.TypeName, "created.type_name — ЗАПРОШЕННАЯ ступень")
	assert.Equal(t, "Люди", body.Created.OwnerName)
	assert.NotEmpty(t, body.Created.ID)
	require.Len(t, body.Settlements, 1)
}

// T5: строение — INSERT buildings с building_type='producer' и
// producer_type_id; класс резолвится от корня (не «Поселение»).
func TestCreateStructureBuilding(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectStructurePlanet(mock)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name, hidden FROM producer_types WHERE id = \$1`).WithArgs(int64(200)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "hidden"}).AddRow("Фабрика", false))
	expectStructureAnchor(mock)
	// Корень типа 200 — 160 (не класс «Поселение» 148) → строение.
	mock.ExpectQuery(`WITH RECURSIVE up AS`).WithArgs(int64(200)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(160)))
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1`).WithArgs(ownerUUID).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("Игрок"))
	mock.ExpectExec(`INSERT INTO buildings`).
		WithArgs(sqlmock.AnyArg(), "p1", "player", ownerUUID, int64(200)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at\s+FROM planets\s+WHERE id = \$1`).WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Планета", 0, `{}`, time.Now(), time.Now()))
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(settlementSelectCols()))
	mock.ExpectQuery(`FROM factions WHERE homeworld_id = ANY\(\$1\) ORDER BY name ASC`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "color", "description", "homeworld_id"}))
	mock.ExpectQuery(`FROM buildings b[\s\S]*WHERE b\.planet_id = ANY\(\$1\) ORDER BY b\.building_type ASC, b\.id ASC`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "building_type", "owner_type", "owner_id", "producer_type_id", "name"}).
			AddRow("new2", "p1", "producer", "player", ownerUUID, int64(200), "Фабрика"))
	mock.ExpectQuery(`SELECT 'player', id::text, username FROM users WHERE id::text = ANY\(\$1\)[\s\S]*SELECT 'faction'[\s\S]*SELECT 'agent'`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"type", "id", "name"}).AddRow("player", ownerUUID, "Игрок"))

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":200,"owner_type":"player","owner_id":"`+ownerUUID+`"}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body struct {
		Created struct {
			Kind     string `json:"kind"`
			TypeName string `json:"type_name"`
		} `json:"created"`
		Buildings []map[string]interface{} `json:"buildings"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "building", body.Created.Kind)
	assert.Equal(t, "Фабрика", body.Created.TypeName)
	require.Len(t, body.Buildings, 1)
}

// T15: население не передано → дефолт ступени (stage.enter ≥ 1).
func TestCreateStructurePopulationDefaultFromStage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectStructurePlanet(mock)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name, hidden FROM producer_types WHERE id = \$1`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "hidden"}).AddRow("Посёлок", false))
	expectStructureAnchor(mock)
	mock.ExpectQuery(`WITH RECURSIVE up AS`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(148)))
	mock.ExpectQuery(`SELECT name FROM factions WHERE id = \$1`).WithArgs(ownerUUID).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Люди"))
	mock.ExpectQuery(`SELECT params->'stage'->>'enter' FROM producer_types WHERE id = \$1`).WithArgs(int64(152)).
		WillReturnRows(sqlmock.NewRows([]string{"enter"}).AddRow("5000"))
	mock.ExpectExec(`INSERT INTO settlements`).
		WithArgs(sqlmock.AnyArg(), "p1", 5000, float64(5000), 100, nil, int64(152), "faction", ownerUUID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at\s+FROM planets\s+WHERE id = \$1`).WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Планета", 0, `{}`, time.Now(), time.Now()))
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`).
		WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(settlementSelectCols()))
	expectEmptyFactionsBuildings(mock)

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.CreateStructure(rec, structureReq(`{"producer_type_id":152,"owner_type":"faction","owner_id":"`+ownerUUID+`"}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== BUILD-OPTIONS (T10) ====================

// T10: типы с target/live/stage/class_name; корень «Поселение» исключён
// (t.id <> $1); порядок серверный; factions с color/type; default_owner/
// default_race — ключи всегда; population_fallback.
func TestBuildOptions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT p\.world_id, w\.coord_x, w\.coord_y FROM planets p JOIN worlds w ON w\.id = p\.world_id WHERE p\.id = \$1`).WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"world_id", "coord_x", "coord_y"}).AddRow("w1", 0.0, 0.0))
	expectStructureAnchor(mock)
	// COALESCE вокруг предиката live обязателен: у части типов params = NULL,
	// и без него выражение вернуло бы NULL — скан в bool падает (живая находка).
	mock.ExpectQuery(`WITH RECURSIVE up AS[\s\S]*COALESCE\(\(t\.params \? 'eat'\) OR \(t\.params \? 'effects'\), false\)[\s\S]*t\.hidden = false AND t\.id <> \$1`).WithArgs(int64(148)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "root_id", "class_name", "enter", "live"}).
			AddRow(int64(152), "Аутпост", int64(148), "Поселение", "0", true).
			AddRow(int64(153), "Посёлок", int64(148), "Поселение", "1000", true).
			AddRow(int64(160), "Фабрика", int64(160), "Фабрика", nil, false))
	mock.ExpectQuery(`SELECT id, name, type, color FROM factions ORDER BY name ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "color"}).
			AddRow("f1", "Люди", "Корпорация", "#ff6b6b"))
	mock.ExpectQuery(`FROM regions`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "center_x", "center_y", "radius", "color", "world_count", "profile", "profile_intensity", "race_id"}).
			AddRow("r1", "Регион", 0.0, 0.0, 100.0, "#fff", 1, nil, 0, "humans"))
	mock.ExpectQuery(`SELECT id, name FROM factions WHERE race_id = \$1 LIMIT 1`).WithArgs("humans").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("f1", "Люди"))

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.BuildOptions(rec, httptest.NewRequest(http.MethodGet, "/admin/planets/p1/build-options", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body struct {
		Types []struct {
			ID        int64   `json:"id"`
			ClassID   int64   `json:"class_id"`
			ClassName string  `json:"class_name"`
			Target    string  `json:"target"`
			Stage     *struct {
				Enter float64 `json:"enter"`
			} `json:"stage"`
			Live bool `json:"live"`
		} `json:"types"`
		Factions []struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Color string `json:"color"`
		} `json:"factions"`
		DefaultOwner *struct {
			OwnerType string `json:"owner_type"`
			OwnerID   string `json:"owner_id"`
		} `json:"default_owner"`
		DefaultRace *struct {
			RaceID   string `json:"race_id"`
			RaceName string `json:"race_name"`
		} `json:"default_race"`
		PopulationFallback int `json:"population_fallback"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	require.Len(t, body.Types, 3)
	assert.Equal(t, "settlement", body.Types[0].Target)
	assert.Equal(t, "Поселение", body.Types[0].ClassName)
	require.NotNil(t, body.Types[0].Stage)
	assert.Equal(t, float64(0), body.Types[0].Stage.Enter)
	assert.True(t, body.Types[0].Live)
	assert.Equal(t, "building", body.Types[2].Target)
	assert.False(t, body.Types[2].Live)
	assert.Nil(t, body.Types[2].Stage, "тип вне ладдеры — stage null")

	require.Len(t, body.Factions, 1)
	assert.Equal(t, "Корпорация", body.Factions[0].Type)
	assert.Equal(t, "#ff6b6b", body.Factions[0].Color)

	require.NotNil(t, body.DefaultOwner, "ключ default_owner отдаётся всегда")
	assert.Equal(t, "faction", body.DefaultOwner.OwnerType)
	assert.Equal(t, "f1", body.DefaultOwner.OwnerID)
	require.NotNil(t, body.DefaultRace)
	assert.Equal(t, "humans", body.DefaultRace.RaceID)
	assert.Equal(t, 1000, body.PopulationFallback)
}

// ==================== OWNER-CANDIDATES (T19) ====================

// T19: q короче 2 → 200, пустой список, запрос в БД не идёт.
func TestOwnerCandidatesShortQueryNoDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.OwnerCandidates(rec, httptest.NewRequest(http.MethodGet, "/admin/owner-candidates?type=player&q=a", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "короткий q — запроса в БД нет")

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Items)
}

// T19: q ≥ 2 → ≤50 совпадений игроков.
func TestOwnerCandidatesPlayers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username FROM users WHERE username ILIKE \$1 ESCAPE '\\' ORDER BY username LIMIT \$2`).
		WithArgs("%ab%", 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow("u1", "Абрис"))

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.OwnerCandidates(rec, httptest.NewRequest(http.MethodGet, "/admin/owner-candidates?type=player&q=ab", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body struct {
		Items []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Subtitle string `json:"subtitle"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Items, 1)
	assert.Equal(t, "u1", body.Items[0].ID)
	assert.Equal(t, "игрок", body.Items[0].Subtitle)
}

// T19: type вне домена → 422.
func TestOwnerCandidatesBadType(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.OwnerCandidates(rec, httptest.NewRequest(http.MethodGet, "/admin/owner-candidates?type=faction&q=ab", nil))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== РЕЗОЛВ КЛАССА И СОРТИРОВКА (регресс) ====================

// T6: якорь стоит на глубине дерева — корень резолвится рекурсивным обходом
// parent_id вверх до корня (спека §4), а не одним шагом к непосредственному
// родителю.
func TestResolveSettlementRootIDRecursive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte(`90`)))
	mock.ExpectQuery(`WITH RECURSIVE up AS`).WithArgs(int64(90)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(148)))

	root, err := resolveSettlementRootID(db)
	require.NoError(t, err)
	require.Equal(t, int64(148), root)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T10: нечисловой/NULL enter не роняет сортировку — внутри «Поселения» по
// возрастанию enter, нечисловые/NULL в конце; группы — по корню.
func TestLoadBuildOptionTypesSafeEnterSort(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`WITH RECURSIVE up AS`).WithArgs(int64(148)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "root_id", "class_name", "enter", "live"}).
			AddRow(int64(153), "Посёлок", int64(148), "Поселение", "1000", true).
			AddRow(int64(152), "Аутпост", int64(148), "Поселение", "0", true).
			AddRow(int64(154), "Кривой", int64(148), "Поселение", "не число", true).
			AddRow(int64(160), "Фабрика", int64(160), "Фабрика", nil, false))

	types, err := loadBuildOptionTypes(db, 148)
	require.NoError(t, err)
	require.Len(t, types, 4)
	assert.Equal(t, []int64{152, 153, 154, 160},
		[]int64{types[0].ID, types[1].ID, types[2].ID, types[3].ID})
	assert.Nil(t, types[2].Stage, "нечисловой enter — stage null, не 500")
	assert.Nil(t, types[3].Stage, "NULL enter — stage null")
}
