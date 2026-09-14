package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at"}).
		AddRow("s1", "p1", 5_000_000, float64(5_000_000), 60, now, now, now).
		AddRow("s2", "p1", 8_000_000, float64(8_000_000), 70, now, now, now)

	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	// attachSettlements читает лог поселений (18a §«Лог поселения») — пусто.
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

// GetSettlementsByPlanetIDs группирует поселения по планетам.
func TestGetSettlementsByPlanetIDs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at"}).
		AddRow("s1", "p1", 100, float64(100), 50, now, now, now).
		AddRow("s2", "p2", 200, float64(200), 60, now, now, now).
		AddRow("s3", "p1", 300, float64(300), 70, now, now, now)

	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)

	byPlanet, err := NewEconomyRepository(db).GetSettlementsByPlanetIDs([]string{"p1", "p2"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, byPlanet, 2)
	assert.Len(t, byPlanet["p1"], 2)
	assert.Len(t, byPlanet["p2"], 1)
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