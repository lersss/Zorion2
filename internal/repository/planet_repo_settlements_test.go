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

	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat"}).
		AddRow("s1", "p1", 5_000_000, float64(5_000_000), 60, now, now, now, nil, nil, nil, nil).
		AddRow("s2", "p1", 8_000_000, float64(8_000_000), 70, now, now, now, nil, nil, nil, nil)

	mock.ExpectQuery(`
		SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at, s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name, pt.params->'eat'
		FROM settlements s
		LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
		WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC
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
		SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at, s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name, pt.params->'eat'
		FROM settlements s
		LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
		WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat"}))

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
	rows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat"}).
		AddRow("s1", "p1", 100, float64(100), 50, now, now, now, nil, nil, nil, nil).
		AddRow("s2", "p2", 200, float64(200), 60, now, now, now, "humans", nil, nil, nil).
		AddRow("s3", "p1", 300, float64(300), 70, now, now, now, nil, nil, nil, nil)

	mock.ExpectQuery(`
		SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at, s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name, pt.params->'eat'
		FROM settlements s
		LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
		WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC
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
	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat"}).
		AddRow("s1", "p1", 5_000_000, float64(5_000_000), 60, now, now, now, nil, nil, nil, nil).
		AddRow("s2", "p1", 8_000_000, float64(8_000_000), 70, now, now, now, "sulfur_nests", nil, nil, nil)

	mock.ExpectQuery(`
		SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at, s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name, pt.params->'eat'
		FROM settlements s
		LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
		WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC
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

// T6/T8 (итерация 4, находка ревью 2026-09-22): норма типа (`params.eat`)
// доходит от чтения поселения через пересчёт населения («событие») до синка
// веток — а не подменяется константой DefaultEatK. Поселение проходит
// Δt ≥ MinPersistInterval; ветка — путь «в памяти», компонентов нет
// (производство 0), выход только убывает на еду по норме своего товара.
func TestAttachSettlementsCarriesTypeEatNorm(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	computedAt := now.Add(-24 * time.Hour) // Δt ≥ MinPersistInterval → путь «событие»

	// Поселение с типом и нормой «пища» = 1e-8 (≠ DefaultEatK = 2.5e-8).
	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat"}).
		AddRow("s1", "p1", 1_000_000_000, float64(1_000_000_000), 85, computedAt, computedAt, computedAt, nil, int64(148), "Обычное поселение", []byte(`{"пища": 1e-8}`))
	mock.ExpectQuery(`
		SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at, s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name, pt.params->'eat'
		FROM settlements s
		LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
		WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	// Пересчёт населения — путь «событие» (транзакция).
	mock.ExpectBegin()
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE id = $1 FOR UPDATE`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 1_000_000_000, float64(1_000_000_000), 85, computedAt, computedAt, computedAt, ""))
	mock.ExpectExec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Ветка «Пища» — путь «в памяти» (Δt < MinPersistInterval), компонентов нет.
	mock.ExpectQuery(`
		SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at, r.good_id, og.name, r.complexity
		FROM settlement_branches b
		JOIN recipes r ON r.id = b.recipe_id
		JOIN goods og ON og.id = r.good_id
		WHERE b.settlement_id = ANY($1)
		ORDER BY b.created_at ASC, b.id ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity"}).
			AddRow("b1", "s1", int64(69), now.Add(-time.Minute), int64(378), "Пища", int64(1)))
	mock.ExpectQuery(`
		SELECT rc.recipe_id, rc.component_id, rc.quantity
		FROM recipe_components rc
		WHERE rc.recipe_id = ANY($1) AND rc.component_id IS NOT NULL
		ORDER BY rc.recipe_id, rc.pos
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}))
	mock.ExpectQuery(`
		SELECT bb.branch_id, bb.direction, bb.good_id, g.name, bb.amount
		FROM settlement_branch_buffers bb
		JOIN goods g ON g.id = bb.good_id
		WHERE bb.branch_id = ANY($1)
		ORDER BY bb.branch_id, bb.direction, bb.good_id
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
			AddRow("b1", "output", int64(378), "Пища", 1000.0))

	// Лог поселения — пусто.
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

	planets := []models.Planet{{ID: "p1", Temperature: 288, Gravity: 1.0}}
	require.NoError(t, NewPlanetRepository(db).attachSettlements(planets))
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets[0].Settlements, 1)
	branches := planets[0].Settlements[0].Branches
	require.Len(t, branches, 1)
	// eaten = 1e-8 · 1e9 · (1/60 ч) = 0.1667 (не 0.4167 = DefaultEatK):
	// выход 1000 − 0.1667 = 999.8333. Допуск 1e-3 — под нагрузкой Δt ветки
	// включает миллисекунды между now теста и пересчётом (различие норм 0.25).
	require.InDelta(t, 1000.0-10.0/60.0, branches[0].Output[0].Amount, 1e-3,
		"норма params.eat обязана дойти до синка веток (не подменяться DefaultEatK)")
	require.InDelta(t, 1e-8*1e9/3600, branches[0].EatenRate, 1e-12,
		"скорость поедания — по норме типа")
}