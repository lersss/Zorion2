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

	// Чек-поинт «свежий», но не «сейчас»: пересчёт населения идёт от реального
	// времени (p0·(1−r)^Δt), поэтому с computed_at = now между настройкой мока и
	// вызовом проходят миллисекунды и значение уезжает на единицы (флак под
	// нагрузкой: 999 998 вместо 1 000 000). Сдвиг вперёд даёт Δt ≤ 0 — гвард
	// модели возвращает p0 без убыли, и проверка остаётся точной.
	freshCheckpoint := now.Add(time.Minute)
	settlementRows := sqlmock.NewRows(settlementCols()).
		AddRow("s1", "p1", 1_000_000, float64(1_000_000), 60, freshCheckpoint, now, now, nil, int64(ownerTestTypeID), nil, nil, nil)
	mock.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(settlementRows)

	// Owner-проход: поселение есть, веток нет, Δt ≤ 0 — путь «в памяти», без записи.
	expectOwnerPassNoBranches(mock, nil)

	expectEmptySettlementLog(mock)

	// attachFactionsAndBuildings — фракций/строений у планеты нет (спека
	// 2026-09-21-фабрики-релиз-2-столицы-фракций §6).
	expectEmptyFactionsBuildings(mock)

	// Открытие карточки планеты триггерит пересчёт населения от среды
	// (18a_population_death.md); Δt < MinPersistInterval — «простой визит»:
	// пересчёт только в памяти, записей в БД нет.
	// Температура 400 K (+127 °C) — жара с R ≈ 2.7·10⁻⁴/сек (R-модель,
	// 99.2.12, обнуления нет): убыль непрерывна и идёт от реального Δt, поэтому
	// чек-поинт сдвинут вперёд (см. выше) — иначе значение зависит от загрузки
	// машины (был флак: 999 998 вместо 1 000 000 в прогоне всех пакетов).

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
