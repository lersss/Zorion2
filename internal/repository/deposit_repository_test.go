// internal/repository/deposit_repository_test.go
//
// Тесты залежей поверхности (спека 2026-09-22-поселение-добыча-сырья-биома-
// ленивый-буфер): чтение/группировка (T6), счётчик залежей ресурса (T14),
// вставка; инвариант «залежи не утекают» — light-пути их не несут (§5.1).
package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// depositRows — стандартные колонки выборки залежей.
func depositRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "planet_id", "good_id", "name", "stratum", "wealth", "amount"})
}

// GetDepositsByPlanetIDs группирует залежи по planet_id (один запрос на систему).
func TestGetDepositsByPlanetIDs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(depositSelectByPlanetsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		depositRows().
			AddRow("d1", "p1", int64(358), "Растения", "surface", 0.5, 1000.0).
			AddRow("d2", "p1", int64(359), "Мясо", "surface", 0.7, 900.0).
			AddRow("d3", "p2", int64(1), "вода-ресурс", "surface", 0.3, 1100.0))

	byPlanet, err := NewDepositRepository(db).GetDepositsByPlanetIDs([]string{"p1", "p2"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, byPlanet["p1"], 2, "две залежи у p1")
	require.Len(t, byPlanet["p2"], 1)
	assert.Equal(t, "Растения", byPlanet["p1"][0].GoodName)
	assert.Equal(t, "surface", byPlanet["p1"][0].Stratum)
	assert.InDelta(t, 0.5, byPlanet["p1"][0].Wealth, 1e-9)
	assert.InDelta(t, 1000.0, byPlanet["p1"][0].Amount, 1e-9)
}

// Пустой вход — пустая карта без запроса.
func TestGetDepositsByPlanetIDsEmpty(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	out, err := NewDepositRepository(db).GetDepositsByPlanetIDs(nil)
	require.NoError(t, err)
	require.Empty(t, out)
}

// CountDepositsByGood — число залежей ресурса во всех мирах (T14).
func TestCountDepositsByGood(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM deposits WHERE good_id = \$1`).
		WithArgs(int64(359)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(17))

	n, err := NewDepositRepository(db).CountDepositsByGood(359)
	require.NoError(t, err)
	assert.Equal(t, 17, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

// InsertDeposit — вставка залежи админ-ручкой (§6) в транзакции.
func TestInsertDeposit(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO deposits \(id, planet_id, good_id, stratum, wealth, amount, created_at, updated_at\)`).
		WithArgs("d1", "p1", int64(358), "surface", 0.5, 1000.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = NewDepositRepository(db).InsertDeposit(context.Background(), tx, models.SurfaceDeposit{
		ID: "d1", PlanetID: "p1", GoodID: 358, Stratum: "surface", Wealth: 0.5, Amount: 1000,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// T6: карточка системы (GetPlanetsByWorldID) несёт залежи построчно.
func TestGetPlanetsByWorldIDAttachesDeposits(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		ORDER BY orbit_index ASC
	`).WithArgs("w1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Планета", 1, `{"surface_composition":{"камни":100}}`, now, now))
	mock.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(settlementCols()))
	expectEmptyFactionsBuildings(mock)
	mock.ExpectQuery(depositSelectByPlanetsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(
		depositRows().
			AddRow("d1", "p1", int64(359), "Мясо", "surface", 0.5, 1000.0).
			AddRow("d2", "p1", int64(1), "вода-ресурс", "surface", 0.8, 700.0))

	planets, err := NewPlanetRepository(db).GetPlanetsByWorldID("w1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets, 1)
	require.Len(t, planets[0].Deposits, 2, "карточка несёт залежи построчно")
	assert.Equal(t, int64(359), planets[0].Deposits[0].GoodID)
	assert.Equal(t, "Мясо", planets[0].Deposits[0].GoodName)
}

// T6: light-пути залежей НЕ несут (инвариант §5.1) — иначе запрос deposits
// получил бы неожиданный вызов и sqlmock вернул ошибку.
func TestLightPathsNoDeposits(t *testing.T) {
	now := time.Now()

	// GetPlanetsLightByWorldID — только выборка планет.
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		ORDER BY orbit_index ASC
	`).WithArgs("w1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Планета", 1, `{"surface_composition":{"камни":100}}`, now, now))
	light, err := NewPlanetRepository(db).GetPlanetsLightByWorldID("w1")
	require.NoError(t, err)
	require.Len(t, light, 1)
	require.Empty(t, light[0].Deposits, "light-путь залежи не подтягивает")
	require.NoError(t, mock.ExpectationsWereMet())

	// GetPlanetByID — планета + поселения + фракции/строения; залежей нет.
	db2, mock2, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db2.Close()
	mock2.ExpectQuery(`
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`).WithArgs("p1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Планета", 1, `{"surface_composition":{"камни":100}}`, now, now))
	mock2.ExpectQuery(settlementsQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(settlementCols()))
	expectEmptyFactionsBuildings(mock2)
	p, err := NewPlanetRepository(db2).GetPlanetByID("p1")
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Empty(t, p.Deposits, "GetPlanetByID залежи не подтягивает")
	require.NoError(t, mock2.ExpectationsWereMet())
}
