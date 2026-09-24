// internal/handlers/planet_visibility_handler_modes_test.go
// Режимы показа и флаг канала покупки через /api/worlds/{id}/planets (спека
// 2026-09-23-орбита-планеты-присутствие-и-снимок §5.1/§5.2, тесты T9/T11/T12):
// presence/snapshot/scan/none, can_buy_report по присутствию на другой планете
// (спутник — по родителю), чтение снимка не делает UPDATE поселений (И-С3).
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

	"zorion/internal/models"
)

// modesResponse — разбор ответа модалки (планеты + флаг покупки).
type modesResponse struct {
	Planets []models.Planet `json:"planets"`
}

// expectModesPlanets — планеты системы (одна строка p1).
func expectModesPlanets(mock sqlmock.Sqlmock, worldID string) {
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", worldID, "Планета1", 0, `{"type":"землеподобная","surface_dominant":"горы"}`, now(), now()))
}

// expectModesSettlements — поселения планеты (одно, с чек-точками).
// computed_at — свежий (time.Now()), чтобы owner-проход пошёл «в памяти»
// (без транзакции/записи): ожидания expectOwnerPassEmptyRegexp.
func expectModesSettlements(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(settlementSelectCols()).
			AddRow("s1", "p1", 876000000, 876000000.4, 69, time.Now(), now(), now(), "spark", nil, nil, nil, nil))
}

// expectModesWorld — мир системы (w2, в радиусе радара).
func expectModesWorld(mock sqlmock.Sqlmock, worldID string) {
	mock.ExpectQuery(`SELECT name, COALESCE\(spectral_class,''\), star_type, system_type, stellar_mods, stellar_mass, age, temperature, coord_x, coord_y FROM worlds WHERE id = \$1`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "spectral_class", "star_type", "system_type", "stellar_mods",
			"stellar_mass", "age", "temperature", "coord_x", "coord_y",
		}).AddRow("Мир2", "K", "star", "single", nil, nil, nil, 4000, 100, 0))
}

// expectModesBelts — пустая выборка поясов мира (аддитивная секция).
func expectModesBelts(mock sqlmock.Sqlmock, worldID string) {
	mock.ExpectQuery(`SELECT id, world_id, kind, name, orbit_index, radius_au, width_au, mass,\s+body_size_km, composition, visible, data, iron_remaining, ice_remaining, created_at, updated_at\s+FROM system_belts\s+WHERE world_id = \$1\s+ORDER BY radius_au ASC`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "world_id", "kind", "name", "orbit_index", "radius_au", "width_au",
			"mass", "body_size_km", "composition", "visible", "data", "iron_remaining",
			"ice_remaining", "created_at", "updated_at",
		}))
}

// expectModesScan — ленивый прогон сканера (ScanSystem + merge-запись).
func expectModesScan(mock sqlmock.Sqlmock, userID, worldID string) {
	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\).*FROM planets p WHERE p.world_id = \$1`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}).
			AddRow("p1", "горы", `{"горы":62}`, 1))
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs(userID, "p1", sqlmock.AnyArg(), "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectOwnerPassEmptyRegexp — owner-проход без веток для regexp-матчера
// (visAdminHandlers): общий expectOwnerPassEmpty рассчитан на QueryMatcherEqual
// (неэкранированные скобки в SQL под regexp стали бы группами и не совпали) —
// здесь шаблоны без скобочных участков.
func expectOwnerPassEmptyRegexp(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve"}))
	mock.ExpectQuery(`SELECT name_norm FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}))
	mock.ExpectQuery(`SELECT b\.id.*FROM settlement_branches b`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}))
	mock.ExpectQuery(`SELECT ae\.effect_type_id.*FROM active_effects ae`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"effect_type_id", "source_position", "load", "load_at", "impact", "curve", "owner_id"}))
	// Ладдера стадий: нет ключа базового типа → пусто; типа у поселения нет →
	// запросы настроек/чисел не идут.
	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}))
}

// expectModesSettlementLog — пустой лог поселений (owner-проход).
func expectModesSettlementLog(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .*FROM settlement_log.*`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "type", "occurred_at", "cause", "created_at"}))
}

// expectModesKnowledge — чтение знания игрока о планете.
func expectModesKnowledge(mock sqlmock.Sqlmock, userID, planetID, data string, scannedAt time.Time) {
	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs(userID, planetID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}).
			AddRow(userID, planetID, data, scannedAt, "presence"))
}

// modesRequest — GET /api/worlds/{id}/planets с ролью player.
func modesRequest(userID, worldID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/worlds/"+worldID+"/planets", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))
	return req
}

// ==================== T9: ЧТЕНИЕ СНИМКА НЕ ПИШЕТ В БД (И-С3) ====================

// Режим snapshot: поселения читаются, но owner-проход (UPDATE settlements /
// веток / active_effects) НЕ выполняется — ожидания записи не выставляются,
// лишний запрос уронил бы тест.
func TestSnapshotReadDoesNotUpdateSettlements(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"
	at := time.Now().Add(-time.Hour)

	expectModesPlanets(mock, "w2")
	expectModesSettlements(mock)
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)
	expectModesWorld(mock, "w2")
	// Игрок в w1 (не в w2) — присутствия на планете карточки нет.
	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectKnownWorlds(mock, "w2")
	expectModesBelts(mock, "w2")
	// Сканер установлен и система в радиусе → ленивый скан.
	expectModesScan(mock, userID, "w2")
	expectModesKnowledge(mock, userID, "p1",
		`{"surface_dominant":"горы","settlements_count":1,"snapshot":{"at":"`+at.UTC().Format(time.RFC3339)+`","source":"presence","surface_dominant":"горы","surface_composition":{"горы":62},"settlements":[{"id":"s1","race_id":"spark","race_name":"Искры","population":876000000,"stability":69,"branches":[]}],"factions":[],"buildings":[]}}`,
		at)

	rec := execJSON(h.GetPlanetsByWorld, modesRequest(userID, "w2"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet(), "чтение снимка не пишет в БД (И-С3)")

	var resp modesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	p := resp.Planets[0]
	require.NotNil(t, p.Knowledge)
	assert.Equal(t, knowledgeModeSnapshot, p.Knowledge.Mode)
	require.NotNil(t, p.Knowledge.SnapshotAt)
	assert.True(t, p.Knowledge.SnapshotFresh)
	require.Len(t, p.Settlements, 1, "картина снимка разложена в Settlements")
	assert.Equal(t, 876000000, p.Settlements[0].Population)
	assert.Equal(t, int64(876000000), p.Population)
}

// ==================== T11: ПРИСУТСТВИЕ НА ДРУГОЙ ПЛАНЕТЕ ====================

// Игрок на орбите планеты p1 системы w2: p1 — presence (полные данные),
// can_buy_report=false (карточка — та же планета присутствия).
func TestPresenceModeOnOwnPlanet(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	expectModesPlanets(mock, "w2")
	expectModesSettlements(mock)
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)
	expectModesWorld(mock, "w2")
	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectPlayerUserWithPosition(mock, userID, "w2", pos)
	expectKnownWorlds(mock)
	expectModesBelts(mock, "w2")
	// Сканер в радиусе → ScanSystem (до presence-блока).
	expectModesScan(mock, userID, "w2")
	// Живой путь присутствия: owner-проход поселений (каталог/категории/ветки/
	// нагрузка) + лог.
	expectOwnerPassEmptyRegexp(mock)
	expectModesSettlementLog(mock)
	expectModesKnowledge(mock, userID, "p1", `{"surface_dominant":"горы","settlements_count":1}`, time.Now())

	rec := execJSON(h.GetPlanetsByWorld, modesRequest(userID, "w2"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp modesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	p := resp.Planets[0]
	require.NotNil(t, p.Knowledge)
	assert.Equal(t, knowledgeModePresence, p.Knowledge.Mode)
	assert.False(t, p.CanBuyReport, "карточка — планета присутствия: покупка не предлагается")
	require.Len(t, p.Settlements, 1, "полные данные присутствия")
	// Население на живом пути пересчитывает owner-проход (И-С3: живой путь
	// сохраняет ленивый синк) — точное число зависит от момента чтения, поэтому
	// проверяем, что раса и население поселения показаны (не занулены).
	assert.Equal(t, "spark", p.Settlements[0].RaceID, "раса поселения видна при присутствии")
	assert.Positive(t, p.Settlements[0].Population, "население присутствия показано")
}

// ==================== T12: can_buy_report ====================

// Игрок на орбите p1 системы w2, карточка системы w3 (планета p9): присутствие
// на ДРУГОЙ планете → can_buy_report=true.
func TestCanBuyReportOnOtherPlanet(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Карточка — система w3 (1000,0), «зажжена» знанием.
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs("w3").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p9", "w3", "Планета9", 0, `{"type":"землеподобная"}`, now(), now()))
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(settlementSelectCols()))
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)
	mock.ExpectQuery(`SELECT name, COALESCE\(spectral_class,''\), star_type, system_type, stellar_mods, stellar_mass, age, temperature, coord_x, coord_y FROM worlds WHERE id = \$1`).
		WithArgs("w3").
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "spectral_class", "star_type", "system_type", "stellar_mods",
			"stellar_mass", "age", "temperature", "coord_x", "coord_y",
		}).AddRow("Мир3", "M", "star", "single", nil, nil, nil, 3000, 1000, 0))
	// Игрок в w2, на орбите планеты p1 (другой системы).
	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectPlayerUserWithPosition(mock, userID, "w2", pos)
	expectKnownWorlds(mock, "w3")
	expectModesBelts(mock, "w3")
	expectModesKnowledge(mock, userID, "p9", `{"surface_dominant":"скалы","settlements_count":0}`, time.Now())

	rec := execJSON(h.GetPlanetsByWorld, modesRequest(userID, "w3"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp modesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	assert.True(t, resp.Planets[0].CanBuyReport, "присутствие на другой планете → покупка доступна")
	assert.Equal(t, knowledgeModeScan, resp.Planets[0].Knowledge.Mode)
}

// Присутствие на спутнике s1 родителя p1: карточка родителя p1 → false
// (та же планета присутствия).
func TestCanBuyReportSatelliteParent(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	// Карточка — система w2, планета p1 (родитель спутника).
	expectModesPlanets(mock, "w2")
	expectModesSettlements(mock)
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)
	expectModesWorld(mock, "w2")
	// Позиция — орбита спутника s1; FindPlanetBySatellite → родитель p1.
	pos := `{"status":"orbit","object_type":"satellite","object_id":"s1","level":"orbit"}`
	expectPlayerUserWithPosition(mock, userID, "w2", pos)
	expectKnownWorlds(mock)
	expectModesBelts(mock, "w2")
	// Сканер в радиусе → ScanSystem (до presence-блока).
	expectModesScan(mock, userID, "w2")
	// presencePlanetID → FindPlanetBySatellite (родитель спутника).
	expectFindPlanetBySatellite(mock, "w2", "s1", "p1")
	// Живой путь присутствия (родитель): owner-проход + лог.
	expectOwnerPassEmptyRegexp(mock)
	expectModesSettlementLog(mock)
	expectModesKnowledge(mock, userID, "p1", `{"surface_dominant":"горы","settlements_count":1}`, time.Now())

	rec := execJSON(h.GetPlanetsByWorld, modesRequest(userID, "w2"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp modesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	p := resp.Planets[0]
	assert.Equal(t, knowledgeModePresence, p.Knowledge.Mode, "спутник → присутствие на родителе")
	assert.False(t, p.CanBuyReport, "карточка родителя — та же планета присутствия")
}

// admin/skycomposer: can_buy_report=false (поле не для них), режимы не
// применяются (И7).
func TestCanBuyReportFalseForAdmin(t *testing.T) {
	h, mock := visAdminHandlers(t)
	const userID = "u1"

	expectModesPlanets(mock, "w2")
	expectModesSettlements(mock)
	// admin идёт полным путём GetPlanetsByWorldID: owner-проход (живой) + лог,
	// затем фракции/строения/залежи.
	expectOwnerPassEmptyRegexp(mock)
	expectModesSettlementLog(mock)
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)
	expectModesWorld(mock, "w2")
	// visibility подключён → user и пояса грузятся и для admin (И7: без фильтра).
	expectPlayerUserWithPosition(mock, userID, "w2", nil)
	expectModesBelts(mock, "w2")

	req := httptest.NewRequest(http.MethodGet, "/api/worlds/w2/planets", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RoleAdmin))

	rec := execJSON(h.GetPlanetsByWorld, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp modesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	assert.False(t, resp.Planets[0].CanBuyReport, "админу флаг покупки не нужен")
	assert.Nil(t, resp.Planets[0].Knowledge, "админ идёт мимо фильтра (И7)")
}
