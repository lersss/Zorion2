package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

const resolveTypeQuery = `SELECT payload FROM generation_config WHERE key = \$1`

// ключ есть, payload — JSON-число → id типа.
func TestResolveDefaultSettlementTypeIDFromConfig(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(resolveTypeQuery).
		WithArgs(models.DefaultSettlementTypeIDKey).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte(`148`)))

	id, err := ResolveDefaultSettlementTypeID(db)
	require.NoError(t, err)
	require.Equal(t, int64(148), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ключа нет → 0 без ошибки (с WARN в лог): связь остаётся NULL.
func TestResolveDefaultSettlementTypeIDMissingKey(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(resolveTypeQuery).
		WithArgs(models.DefaultSettlementTypeIDKey).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}))

	id, err := ResolveDefaultSettlementTypeID(db)
	require.NoError(t, err)
	require.Equal(t, int64(0), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

// payload не число → 0 без ошибки (с WARN в лог).
func TestResolveDefaultSettlementTypeIDNotANumber(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(resolveTypeQuery).
		WithArgs(models.DefaultSettlementTypeIDKey).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte(`"городок"`)))

	id, err := ResolveDefaultSettlementTypeID(db)
	require.NoError(t, err)
	require.Equal(t, int64(0), id)
	require.NoError(t, mock.ExpectationsWereMet())
}
