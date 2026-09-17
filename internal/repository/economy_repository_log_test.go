package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
)

// «Событие» с обвалом населения: запись «Вымерло» создаётся той же
// транзакцией синка, что продвигает чек-точку (INSERT ... ON CONFLICT
// DO NOTHING — анти-дубль, 18a §«Анти-дубль и синк»).
func TestRecomputeSettlementPopulationDeathWritesLog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-10 * 24 * 365 * time.Hour) // 10 лет — заведомо обвал
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 1000, float64(1000), 60, since, since, since, ""))
	mock.ExpectExec(`UPDATE settlements SET population`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO settlement_log`).
		WithArgs("s1", sqlmock.AnyArg(), "heat").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	input := settlement.PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(loadSettlement("s1", 1000, since), input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Zero(t, got.Population, "обвал: население обнулено целиком")
	require.Equal(t, "", got.RaceID, "легаси-поселение без расы — пустой RaceID (75a)")
}

// Мёртвая чек-точка (population_exact <= NDead на старте): обвал есть, но
// запись НЕ создаётся — дата невычислима, бэкфилл отменён (18a §«Бэкфилл»).
func TestRecomputeSettlementPopulationDeadCheckpointNoLog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-10 * 24 * 365 * time.Hour)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 50, float64(50), 60, since, since, since, ""))
	mock.ExpectExec(`UPDATE settlements SET population`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	input := settlement.PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(loadSettlement("s1", 50, since), input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(), "INSERT в settlement_log не должен происходить для мёртвой чек-точки")
	require.Zero(t, got.Population)
}

// GetSettlementLogBySettlementIDs — последние 3 записи на поселение,
// сортировка по дате убывающая.
func TestGetSettlementLogBySettlementIDs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	occ := time.Now().Add(-24 * time.Hour)
	cause := "heat"

	mock.ExpectQuery(`FROM settlement_log`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "type", "occurred_at", "cause", "created_at"}).
			AddRow("l1", "s1", "extinct", occ, cause, occ))

	logs, err := NewEconomyRepository(db).GetSettlementLogBySettlementIDs([]string{"s1"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, logs["s1"], 1)
	require.Equal(t, "extinct", logs["s1"][0].Type)
	require.Equal(t, "heat", *logs["s1"][0].Cause)
	require.Equal(t, occ, logs["s1"][0].OccurredAt)
}

// Пустой список id — не запрос, а пустой результат (соглашение репозиториев).
func TestGetSettlementLogBySettlementIDsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logs, err := NewEconomyRepository(db).GetSettlementLogBySettlementIDs(nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, logs)
}