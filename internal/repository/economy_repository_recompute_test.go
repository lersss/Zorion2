package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
)

func TestRecomputeSettlementPopulationComfortableUnchanged(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at
		FROM settlements WHERE id = $1 FOR UPDATE`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at"}).
			AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, since, since, since))
	mock.ExpectExec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`).
		WithArgs(1_000_000, float64(1_000_000), now, "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	input := settlement.PlanetInput{TemperatureK: 275, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation("s1", input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, 1_000_000, got.Population)
	require.Equal(t, float64(1_000_000), got.PopulationExact)
}

func TestRecomputeSettlementPopulationHotDecreases(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at"}).
			AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, since, since, since))
	mock.ExpectExec(`UPDATE settlements SET population`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	input := settlement.PlanetInput{TemperatureK: 900, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation("s1", input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Less(t, got.Population, 1_000_000, "на жаркой планете население должно уменьшиться")
	require.Greater(t, got.Population, 0)
}
