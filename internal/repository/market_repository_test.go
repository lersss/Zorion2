// internal/repository/market_repository_test.go
//
// Справочник предложений витрины (спека 2026-09-24-магазин-модулей-локальный-
// рынок §5): чтение всех строк / по id, живость поселения (гейт покупки §7.2
// п.2; на чтении витрины не раскрывается).
package repository

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// marketOfferRows — строки market_offers (порядок колонок ListOffers).
func marketOfferRows(rows ...[]driver.Value) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"id", "kind", "item_id", "price"})
	for _, row := range rows {
		r.AddRow(row...)
	}
	return r
}

// ListOffers: все 4 базовых модуля, порядок по id.
func TestMarketListOffers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	repo := NewMarketRepository(db)

	mock.ExpectQuery(`SELECT id, kind, item_id, price FROM market_offers ORDER BY id ASC`).
		WillReturnRows(marketOfferRows(
			[]driver.Value{int64(1), "module", "cargo_1", int64(3000)},
			[]driver.Value{int64(2), "module", "engine_1", int64(3000)},
			[]driver.Value{int64(3), "module", "radar_1", int64(3000)},
			[]driver.Value{int64(4), "module", "scanner_1", int64(3000)},
		))

	offers, err := repo.ListOffers()
	require.NoError(t, err)
	require.Len(t, offers, 4)
	require.Equal(t, int64(1), offers[0].ID)
	require.Equal(t, "module", offers[0].Kind)
	require.Equal(t, "cargo_1", offers[0].ItemID)
	require.Equal(t, int64(3000), offers[0].Price)
	require.NoError(t, mock.ExpectationsWereMet())
}

// GetOfferByID: найденное предложение и отсутствующее (nil, nil).
func TestMarketGetOfferByID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	repo := NewMarketRepository(db)

	mock.ExpectQuery(`SELECT id, kind, item_id, price FROM market_offers WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(marketOfferRows([]driver.Value{int64(1), "module", "radar_1", int64(3000)}))

	offer, err := repo.GetOfferByID(1)
	require.NoError(t, err)
	require.NotNil(t, offer)
	require.Equal(t, "radar_1", offer.ItemID)
	require.Equal(t, int64(3000), offer.Price)

	mock.ExpectQuery(`SELECT id, kind, item_id, price FROM market_offers WHERE id = \$1`).
		WithArgs(int64(999)).
		WillReturnError(sql.ErrNoRows)

	offer, err = repo.GetOfferByID(999)
	require.NoError(t, err)
	require.Nil(t, offer)
	require.NoError(t, mock.ExpectationsWereMet())
}

// HasLiveSettlement: живое поселение есть / нет (ErrNoRows → false).
func TestMarketHasLiveSettlement(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	repo := NewMarketRepository(db)

	mock.ExpectQuery(`SELECT 1 FROM settlements WHERE planet_id = \$1 AND population_exact > 0 LIMIT 1`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	live, err := repo.HasLiveSettlement("p1")
	require.NoError(t, err)
	require.True(t, live)

	mock.ExpectQuery(`SELECT 1 FROM settlements WHERE planet_id = \$1 AND population_exact > 0 LIMIT 1`).
		WithArgs("p2").
		WillReturnError(sql.ErrNoRows)
	live, err = repo.HasLiveSettlement("p2")
	require.NoError(t, err)
	require.False(t, live)
	require.NoError(t, mock.ExpectationsWereMet())
}