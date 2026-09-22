package repository

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
	"zorion/internal/races"
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
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE id = $1 FOR UPDATE`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, since, since, since, "microcrack"))
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
	require.Equal(t, "microcrack", got.RaceID, "путь «событие» не должен терять расу (75a)")
}

// T6/T8/T19 (итерация 4): путь «событие» не должен терять тип поселения и
// нормы еды (params.eat) — иначе синк веток ест по DefaultEatK и настройка
// студии игнорируется (спека §3.3/§3.5, находка ревью 2026-09-22).
func TestRecomputeSettlementPopulationEventKeepsTypeAndEat(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour) // Δt ≥ MinPersistInterval → путь «событие»
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE id = $1 FOR UPDATE`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, since, since, since, ""))
	mock.ExpectExec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`).
		WithArgs(1_000_000, float64(1_000_000), now, "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	s := loadSettlement("s1", 1_000_000, since)
	s.SettlementTypeID = 148
	s.TypeName = "Обычное поселение"
	s.EatByPosition = map[string]float64{"продовольствие": 1e-8}
	s.EffectsByPosition = map[string]string{"продовольствие": "голод"}

	input := settlement.PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(s, input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Equal(t, int64(148), got.SettlementTypeID,
		"путь «событие» не должен терять тип поселения")
	require.Equal(t, "Обычное поселение", got.TypeName)
	require.Equal(t, 1e-8, got.EatByPosition["продовольствие"],
		"путь «событие» не должен терять нормы params.eat (иначе спрос — по DefaultEatK)")
	require.Equal(t, "голод", got.EffectsByPosition["продовольствие"],
		"путь «событие» не должен терять привязки params.effects (§4.2)")
}

// «Событие» на жаркой планете: население убывает, чек-точка продвигается.
func TestRecomputeSettlementPopulationEventHotDecreases(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-24 * time.Hour)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, since, since, since, "microcrack"))
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
	require.Equal(t, "microcrack", got.RaceID, "путь «событие» не должен терять расу (75a)")
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

// Тест 16 (§14, поправка В2): гвард записи int4 — newExact = 2.15·10⁹
// (термо-рои от p0 = 10⁹ через ~70 дней) клампится в MaxInt4Population
// (2^31−1) ПЕРЕД приведением к int; UPDATE settlements не падает
// («integer out of range» невозможен); newExact = 1e15 → тоже 2 147 483 647.
func TestRecomputeSettlementPopulationInt4Clamp(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	require.NoError(t, settlement.LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")))

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-70 * 24 * time.Hour) // ~70 дней ≥ MinPersistInterval
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE id = $1 FOR UPDATE`).
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}).
			AddRow("s1", "p1", 1_000_000_000, float64(1_000_000_000), 60, since, since, since, "thermo_swarms"))
	mock.ExpectExec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`).
		WithArgs(settlement.MaxInt4Population, sqlmock.AnyArg(), now, "s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Термо-рои в оптимуме (600 K): r = −200·R_ест ≈ −1.27·10⁻⁷ → за 70 дней
	// newExact ≈ 2.15·10⁹ > 2^31−1 → кламп записи.
	s := loadSettlement("s1", 1_000_000_000, since)
	s.RaceID = "thermo_swarms"
	input := settlement.PlanetInput{TemperatureK: 600, GravityG: 1.0, CoreRadioactivity: 10}
	got, err := NewEconomyRepository(db).RecomputeSettlementPopulation(s, input, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, settlement.MaxInt4Population, got.Population,
		"newExact > 2^31−1 клампится в MaxInt4Population перед int-конверсией")
	require.Equal(t, "thermo_swarms", got.RaceID, "путь «событие» не должен терять расу (75a)")
}
