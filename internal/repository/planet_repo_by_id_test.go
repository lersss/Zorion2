package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlanetByID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	planetRows := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
		AddRow("p1", "w1", "Жаркая", 1, `{"temperature":400,"gravity":1.2}`, now, now)

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("p1").WillReturnRows(planetRows)

	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
		AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, now, now, now, nil)
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	// attachSettlements читает лог поселения (18b §«Лог поселения») — пусто.
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

	// attachFactionsAndBuildings — фракций/строений у планеты нет (спека
	// 2026-09-21-фабрики-релиз-2-столицы-фракций §6).
	expectEmptyFactionsBuildings(mock)

	// Открытие карточки планеты триггерит пересчёт населения от среды
	// (18a_population_death.md); computed_at = now, т.е. Δt < MinPersistInterval —
	// «простой визит»: пересчёт только в памяти, записей в БД нет.
	// Температура 400 K (+127 °C) — жара с R ≈ 2.7·10⁻⁴/сек (R-модель,
	// 99.2.12, обнуления нет): за микросекунды Δt убыль копеечная, население
	// не зависит от скорости прогона.

	planet, err := NewPlanetRepository(db).GetPlanetByID("p1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.NotNil(t, planet)
	assert.Equal(t, "p1", planet.ID)
	assert.InDelta(t, 400, planet.Temperature, 0.0001)
	assert.InDelta(t, 1.2, planet.Gravity, 0.0001)
	assert.Equal(t, int64(1_000_000), planet.Population)
}

func TestGetPlanetByIDNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("missing").WillReturnError(sql.ErrNoRows)

	planet, err := NewPlanetRepository(db).GetPlanetByID("missing")
	require.NoError(t, err)
	assert.Nil(t, planet)
	require.NoError(t, mock.ExpectationsWereMet())
}
