package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

func loadSettlement(id string, population int, computedAt time.Time) *models.Settlement {
	return &models.Settlement{
		ID: id, PlanetID: "p1", Population: population,
		PopulationExact: float64(population), Stability: 60,
		ComputedAt: computedAt, CreatedAt: computedAt, UpdatedAt: computedAt,
	}
}

// «Событие» (Δt >= MinPersistInterval): чек-точка продвигается в БД.
// На комфортной планете λ = 0, население не меняется, лямбда уходит в ответ.
func TestRecomputeSettlementPopulationEventComfortableUnchanged(t *testing.T) {
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

	input := settlement.PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(loadSettlement("s1", 1_000_000, since), input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, 1_000_000, got.Population)
	require.Equal(t, float64(1_000_000), got.PopulationExact)
	require.Equal(t, float64(0), got.LambdaPerHour, "комфортная планета не должна убивать")
	require.Equal(t, float64(100), got.NDead)
}

// «Событие» на жаркой планете: население убывает, чек-точка продвигается.
func TestRecomputeSettlementPopulationEventHotDecreases(t *testing.T) {
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

	// 365 K (+92 °C) — жара с R ≈ 7.3·10⁻⁵/сек (R-модель, 99.2.12):
	// население «убыло, но живо» (за сутки ≥ NDead).
	input := settlement.PlanetInput{TemperatureK: 365, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(loadSettlement("s1", 1_000_000, since), input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Less(t, got.Population, 1_000_000, "на жаркой планете население должно уменьшиться")
	require.Greater(t, got.Population, 0)
	require.Greater(t, got.RPerSec, float64(0), "жара — в R_per_sec, не в lambda_per_hour")
}

// «Простой визит» (Δt < MinPersistInterval): население пересчитывается только
// в памяти — никаких запросов к БД после уже загруженной чек-точки.
func TestRecomputeSettlementPopulationVisitNoWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-1 * time.Minute)
	now := time.Now()

	input := settlement.PlanetInput{TemperatureK: 365, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(loadSettlement("s1", 1_000_000, since), input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(), "визит не должен трогать БД")

	require.Less(t, got.Population, 1_000_000, "на жаркой планете население должно уменьшиться")
	require.Greater(t, got.Population, 0)
	require.Greater(t, got.RPerSec, float64(0), "жара — в R_per_sec, не в lambda_per_hour")
	require.Equal(t, now.Unix(), got.ComputedAt.Unix(), "в ответе — чек-точка на момент пересчёта")
	require.Equal(t, float64(100), got.NDead)
}

// «Простой визит» на комфортной планете: население не меняется, ёмкость
// строки не нужна — путь без транзакции вообще не открывается.
func TestRecomputeSettlementPopulationVisitComfortableUnchanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-1 * time.Minute)
	now := time.Now()

	input := settlement.PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(loadSettlement("s1", 1_000_000, since), input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Equal(t, 1_000_000, got.Population)
	require.Equal(t, float64(0), got.LambdaPerHour)
}