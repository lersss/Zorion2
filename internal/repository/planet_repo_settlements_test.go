package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/races"
)

// settlementsQuery — запрос поселений планет с типом (нормы eat + привязки
// effects, спека 2026-09-22-эффекты-снабжения §4.2).
const settlementsQuery = `
	SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at,
	                 s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name,
	                 pt.params->'eat', pt.params->'effects'
	          FROM settlements s
	          LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
	          WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC`

const settlementLogQuery = `
		SELECT id, settlement_id, type, occurred_at, cause, created_at
		FROM (
			SELECT id, settlement_id, type, occurred_at, cause, created_at,
			       ROW_NUMBER() OVER (PARTITION BY settlement_id ORDER BY occurred_at DESC) AS rn
			FROM settlement_log
			WHERE settlement_id = ANY($1)
		) sub
		WHERE rn <= 3
		ORDER BY occurred_at DESC`

// settlementCols — колонки выборки поселений.
func settlementCols() []string {
	return []string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat", "effects"}
}

// expectEmptySettlementLog — лог поселений пуст.
func expectEmptySettlementLog(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(settlementLogQuery).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "type", "occurred_at", "cause", "created_at"}))
}

// GetPlanetsByWorldID подтягивает поселения и считает население планеты
// как сумму населения её поселений. Планета без поселений — население 0.
func TestGetPlanetsByWorldIDWithSettlements(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	planetRows := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
		AddRow("p1", "w1", "Обитаемая", 1, `{"life":true,"habitable":true,"temperature":288,"gravity":1.0}`, now, now).
		AddRow("p2", "w1", "Пустырь", 2, `{"life":false,"habitable":false}`, now, now)

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		ORDER BY orbit_index ASC
	`).WithArgs("w1").WillReturnRows(planetRows)

	settlementRows := sqlmock.NewRows(settlementCols()).
		AddRow("s1", "p1", 5_000_000, float64(5_000_000), 60, now, now, now, nil, int64(ownerTestTypeID), nil, nil, nil).
		AddRow("s2", "p1", 8_000_000, float64(8_000_000), 70, now, now, now, nil, int64(ownerTestTypeID), nil, nil, nil)
	mock.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	// Owner-проход: у поселений веток нет и computed_at = now (Δt < порога) —
	// путь «в памяти», записей в БД нет.
	expectOwnerPassNoBranches(mock, nil)
	expectEmptySettlementLog(mock)

	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)

	planets, err := NewPlanetRepository(db).GetPlanetsByWorldID("w1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets, 2)
	assert.Equal(t, int64(13_000_000), planets[0].Population, "население = сумма поселений")
	assert.Len(t, planets[0].Settlements, 2)
	assert.Equal(t, 5_000_000, planets[0].Settlements[0].Population)
	assert.Equal(t, int64(0), planets[1].Population, "без поселений население 0")
	assert.Empty(t, planets[1].Settlements)
}

// TestGetPlanetsByWorldIDOrbitContext — регрессия блокера 35b: API-слой
// (GetPlanetsByWorldID → models.Planet, отдаётся /api/worlds/{id}/planets
// целиком) доносит орбитальный контекст P- и S-планет из planets.data
// (whitelist populatePlanetFromJSON, 35b §2.2).
func TestGetPlanetsByWorldIDOrbitContext(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	planetRows := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
		AddRow("pP", "w1", "Циркумбинарная", 1,
			`{"life":false,"habitable":false,"orbit_center":"barycenter","orbit_radius_au":0.9,"circumbinary":true}`,
			now, now).
		AddRow("pS", "w1", "С-тип", 2,
			`{"life":false,"habitable":false,"orbit_center":"main","orbit_radius_au":0.68}`,
			now, now)

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		ORDER BY orbit_index ASC
	`).WithArgs("w1").WillReturnRows(planetRows)

	mock.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(settlementCols()))
	// поселений нет — owner-проход не запускается, лог не запрашивается.
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)

	planets, err := NewPlanetRepository(db).GetPlanetsByWorldID("w1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets, 2)
	assert.Equal(t, "barycenter", planets[0].OrbitCenter, "P: orbit_center=barycenter")
	assert.InDelta(t, 0.9, planets[0].OrbitRadiusAU, 1e-9, "P: orbit_radius_au=3a")
	assert.True(t, planets[0].Circumbinary, "P: circumbinary=true")
	assert.Equal(t, "main", planets[1].OrbitCenter, "S: orbit_center=main")
	assert.InDelta(t, 0.68, planets[1].OrbitRadiusAU, 1e-9, "S: фактический радиус")
	assert.False(t, planets[1].Circumbinary)

	raw, err := json.Marshal(planets[0])
	require.NoError(t, err)
	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &obj))
	assert.Equal(t, "barycenter", obj["orbit_center"])
	assert.InDelta(t, 0.9, obj["orbit_radius_au"].(float64), 1e-9)
	assert.Equal(t, true, obj["circumbinary"])
}

// GetSettlementsByPlanetIDs группирует поселения по планетам.
func TestGetSettlementsByPlanetIDs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	rows := sqlmock.NewRows(settlementCols()).
		AddRow("s1", "p1", 100, float64(100), 50, now, now, now, nil, nil, nil, nil, nil).
		AddRow("s2", "p2", 200, float64(200), 60, now, now, now, "humans", nil, nil, nil, nil).
		AddRow("s3", "p1", 300, float64(300), 70, now, now, now, nil, nil, nil, nil, nil)

	mock.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)

	byPlanet, err := NewEconomyRepository(db).GetSettlementsByPlanetIDs([]string{"p1", "p2"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, byPlanet, 2)
	assert.Len(t, byPlanet["p1"], 2)
	assert.Len(t, byPlanet["p2"], 1)
	assert.Equal(t, "", byPlanet["p1"][0].RaceID, "NULL race_id → пусто (легаси/люди)")
	assert.Equal(t, "humans", byPlanet["p2"][0].RaceID, "race_id сканируется из БД")
}

// Пустой запрос не трогает БД и возвращает пустую карту.
func TestGetSettlementsByPlanetIDsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	byPlanet, err := NewEconomyRepository(db).GetSettlementsByPlanetIDs(nil)
	require.NoError(t, err)
	assert.Empty(t, byPlanet)
	require.NoError(t, mock.ExpectationsWereMet())
}

// race_name в JSON-выводе поселения: NULL race_id → «Люди» (легаси/люди,
// спека 99.2.21 §2.3), непустой → имя из каталога рас.
func TestGetPlanetsByWorldIDRaceName(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	planetRows := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
		AddRow("p1", "w1", "Двойная", 1, `{"life":true,"habitable":true,"temperature":288,"gravity":1.0}`, now, now)

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		ORDER BY orbit_index ASC
	`).WithArgs("w1").WillReturnRows(planetRows)

	settlementRows := sqlmock.NewRows(settlementCols()).
		AddRow("s1", "p1", 5_000_000, float64(5_000_000), 60, now, now, now, nil, int64(ownerTestTypeID), nil, nil, nil).
		AddRow("s2", "p1", 8_000_000, float64(8_000_000), 70, now, now, now, "sulfur_nests", int64(ownerTestTypeID), nil, nil, nil)
	mock.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	expectOwnerPassNoBranches(mock, nil)
	expectEmptySettlementLog(mock)
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)

	planets, err := NewPlanetRepository(db).GetPlanetsByWorldID("w1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets, 1)
	require.Len(t, planets[0].Settlements, 2)
	assert.Equal(t, "Люди", planets[0].Settlements[0].RaceName, "NULL race_id → «Люди»")
	assert.Equal(t, "Серные гнёзда", planets[0].Settlements[1].RaceName, "имя из каталога рас")

	raw, err := json.Marshal(planets[0].Settlements[0])
	require.NoError(t, err)
	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &obj))
	assert.Equal(t, "Люди", obj["race_name"])
	assert.NotContains(t, obj, "race_id", "NULL race_id не сериализуется (omitempty)")

	raw2, err := json.Marshal(planets[0].Settlements[1])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw2, &obj))
	assert.Equal(t, "Серные гнёзда", obj["race_name"])
	assert.Equal(t, "sulfur_nests", obj["race_id"])
}

// TestAttachSettlementsOwnerPass — owner-проход: население и ветки считаются
// ДО населения (порядок «производство → потребность → население», §4.1), а
// хвоста `eaten` в ProcessBranch больше нет — выход = O0_b + batches_b − drawn_b
// (при полном покрытии drawn = 0, выход растёт на всё производство).
func TestAttachSettlementsOwnerPass(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-24 * time.Hour) // Δt ≥ MinPersistInterval → персистентный путь

	settlementRows := sqlmock.NewRows(settlementCols()).
		AddRow("s1", "p1", 1_000_000_000, float64(1_000_000_000), 85, computedAt, computedAt, computedAt, nil, int64(148), "Обычное поселение",
			[]byte(`{"продовольствие": 600}`), []byte(`{"продовольствие": "голод"}`))
	mock.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	expectOwnerPassWithBranches(mock, ownerBranchRows(computedAt, "продовольствие"), ownerComponentRows(), ownerBufferRows(), nil)

	mock.ExpectBegin()
	mock.ExpectExec(advisoryOwnerLockSQL).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(settlementLockSQL).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"population", "population_exact", "computed_at", "created_at", "race_id", "settlement_type_id"}).
			AddRow(1_000_000_000, float64(1_000_000_000), computedAt, computedAt, "", int64(148)))
	mock.ExpectQuery(branchSelectBySettlementForUpdateSQL).WithArgs("s1").
		WillReturnRows(ownerBranchRows(computedAt, "продовольствие"))
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerBufferRows())
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(ownerComponentRows())
	mock.ExpectExec(branchTopUpInputSQL).WithArgs("b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(depositExtractionSelectSQL).WithArgs("p1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "amount"}))
	// Точные объёмы зависят от миллисекунд между `now` теста и внутренним
	// `time.Now()` attachSettlements — суммы проверяются по знаку/порядку ниже.
	mock.ExpectExec(branchWriteInputSQL).WithArgs(sqlmock.AnyArg(), "b1", int64(359)).WillReturnResult(sqlmock.NewResult(0, 1))
	// Выход = 0 + batches(24 ч · 27.8/ч) − 0 ≈ 667.2 — ветка не «ест» сама.
	mock.ExpectExec(branchWriteOutputSQL).WithArgs("b1", int64(378), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(branchWriteCheckpointSQL).WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(activeEffectUpsertSQL).WithArgs(int64(1), "s1", "продовольствие", sqlmock.AnyArg(), amountNear{0.0}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(settlementPopulationWriteSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOrphanCleanup(mock, "s1", []int64{1})
	mock.ExpectCommit()

	expectEmptySettlementLog(mock)

	planets := []models.Planet{{ID: "p1", Temperature: 288, Gravity: 1.0}}
	require.NoError(t, NewPlanetRepository(db).attachSettlements(planets))
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets[0].Settlements, 1)
	branches := planets[0].Settlements[0].Branches
	require.Len(t, branches, 1)
	require.InDelta(t, 667.2, branches[0].Output[0].Amount, 1.0,
		"выход = O0_b + batches_b − drawn_b; хвоста eaten в ProcessBranch нет")
	require.Zero(t, branches[0].Eaten, "списания нет — позиция покрыта производством")
}
