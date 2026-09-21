package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

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

	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
		AddRow("s1", "p1", 5_000_000, float64(5_000_000), 60, now, now, now, nil).
		AddRow("s2", "p1", 8_000_000, float64(8_000_000), 70, now, now, now, nil)

	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	// attachBranches (спека 2026-09-22-поселение-ветка-буферы-переработка
	// §4.2): у поселений веток нет — пустая выборка; идёт после пересчёта
	// населения и до чтения лога.
	expectEmptyBranches(mock)

	// attachSettlements читает лог поселений (18b §«Лог поселения») — пусто.
	mock.ExpectQuery(`
		SELECT id, settlement_id, type, occurred_at, cause, created_at
		FROM (
			SELECT id, settlement_id, type, occurred_at, cause, created_at,
			       ROW_NUMBER() OVER (PARTITION BY settlement_id ORDER BY occurred_at DESC) AS rn
			FROM settlement_log
			WHERE settlement_id = ANY($1)
		) sub
		WHERE rn <= 3
		ORDER BY occurred_at DESC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "type", "occurred_at", "cause", "created_at"}))

	// attachFactionsAndBuildings — фракций/строений у планет нет (спека
	// 2026-09-21-фабрики-релиз-2-столицы-фракций §6).
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)

	// Открытие карточки системы триггерит пересчёт населения (18a_population_death.md);
	// computed_at = now, т.е. Δt < MinPersistInterval — «простой визит»: пересчёт
	// только в памяти, записей в БД нет. Планета p1 — комфортная (288 K, R=0, λ=0):
	// население не меняется (0 K дала бы жёсткий ноль холода и обнулила тест —
	// см. флак из-за кванта time.Now). Тест про группировку, не про физику.

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
		// P-планета: циркумбинарная, вокруг барицентра пары.
		AddRow("pP", "w1", "Циркумбинарная", 1,
			`{"life":false,"habitable":false,"orbit_center":"barycenter","orbit_radius_au":0.9,"circumbinary":true}`,
			now, now).
		// S-планета: вокруг главной.
		AddRow("pS", "w1", "С-тип", 2,
			`{"life":false,"habitable":false,"orbit_center":"main","orbit_radius_au":0.68}`,
			now, now)

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		ORDER BY orbit_index ASC
	`).WithArgs("w1").WillReturnRows(planetRows)

	// attachSettlements: поселений нет — settlements-запрос возвращает пусто,
	// settlement_log при пустом ids не запрашивается (GetSettlementLogBySettlementIDs).
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}))

	// attachFactionsAndBuildings — фракций/строений у планет нет (§6).
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)

	planets, err := NewPlanetRepository(db).GetPlanetsByWorldID("w1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets, 2)

	// P-планета: поля доехали из data.
	assert.Equal(t, "barycenter", planets[0].OrbitCenter, "P: orbit_center=barycenter")
	assert.InDelta(t, 0.9, planets[0].OrbitRadiusAU, 1e-9, "P: orbit_radius_au=3a")
	assert.True(t, planets[0].Circumbinary, "P: circumbinary=true")

	// S-планета.
	assert.Equal(t, "main", planets[1].OrbitCenter, "S: orbit_center=main")
	assert.InDelta(t, 0.68, planets[1].OrbitRadiusAU, 1e-9, "S: фактический радиус")
	assert.False(t, planets[1].Circumbinary)

	// Сериализация как в API-ответе (/api/worlds/{id}/planets отдаёт
	// []models.Planet целиком): ключи в snake_case присутствуют.
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
	rows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
		AddRow("s1", "p1", 100, float64(100), 50, now, now, now, nil).
		AddRow("s2", "p2", 200, float64(200), 60, now, now, now, "humans").
		AddRow("s3", "p1", 300, float64(300), 70, now, now, now, nil)

	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)

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

	// NULL race_id (легаси/люди) + раса из каталога (sulfur_nests → «Серные гнёзда»).
	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
		AddRow("s1", "p1", 5_000_000, float64(5_000_000), 60, now, now, now, nil).
		AddRow("s2", "p1", 8_000_000, float64(8_000_000), 70, now, now, now, "sulfur_nests")

	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	expectEmptyBranches(mock)

	mock.ExpectQuery(`
		SELECT id, settlement_id, type, occurred_at, cause, created_at
		FROM (
			SELECT id, settlement_id, type, occurred_at, cause, created_at,
			       ROW_NUMBER() OVER (PARTITION BY settlement_id ORDER BY occurred_at DESC) AS rn
			FROM settlement_log
			WHERE settlement_id = ANY($1)
		) sub
		WHERE rn <= 3
		ORDER BY occurred_at DESC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "type", "occurred_at", "cause", "created_at"}))

	// attachFactionsAndBuildings — фракций/строений у планеты нет (§6).
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)

	planets, err := NewPlanetRepository(db).GetPlanetsByWorldID("w1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets, 1)
	require.Len(t, planets[0].Settlements, 2)
	assert.Equal(t, "Люди", planets[0].Settlements[0].RaceName, "NULL race_id → «Люди»")
	assert.Equal(t, "Серные гнёзда", planets[0].Settlements[1].RaceName, "имя из каталога рас")

	// JSON-вывод как в API-ответе: race_name присутствует, race_id — только
	// у расового поселения (omitempty).
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