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
		AddRow("p1", "w1", "Жаркая", 1, `{"temperature":900,"gravity":1.2}`, now, now)

	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("p1").WillReturnRows(planetRows)

	settlementRows := sqlmock.NewRows([]string{"id", "planet_id", "population", "stability", "created_at", "updated_at"}).
		AddRow("s1", "p1", 1_000_000, 60, now, now)
	mock.ExpectQuery(`
		SELECT id, planet_id, population, stability, created_at, updated_at
		FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	planet, err := NewPlanetRepository(db).GetPlanetByID("p1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.NotNil(t, planet)
	assert.Equal(t, "p1", planet.ID)
	assert.InDelta(t, 900, planet.Temperature, 0.0001)
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
