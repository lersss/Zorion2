// internal/goodsstudio/seed_test.go
// Тесты сидера каталога (спека переноса-студии-товаров-iterA §5/§11):
// маркер в generation_config — пропуск; полный сид — 6 ресурсных + 13
// товарных категорий + 131 ресурс (слой 20 + витрина 111) + маркер,
// всё в одной транзакции.
package goodsstudio

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
	"zorion/internal/resource"
)

// TestSeedMarkerSkips — маркер есть → сид пропускается (повторный старт
// не возвращает удалённое, С1): никаких INSERT не выполняется.
func TestSeedMarkerSkips(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM generation_config WHERE key = \$1\)`).
		WithArgs(SeedMarkerKey).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	require.NoError(t, Seed(db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSeedFull — нет маркера → полный сид в одной транзакции:
// 6 ресурсных категорий (RETURNING id), 13 товарных, 131 ресурс, маркер.
func TestSeedFull(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM generation_config WHERE key = \$1\)`).
		WithArgs(SeedMarkerKey).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	mock.ExpectBegin()

	// 6 ресурсных категорий (is_system, code) — RETURNING id.
	for i := 1; i <= len(resource.AllCategories); i++ {
		mock.ExpectQuery(`INSERT INTO categories \(name, name_norm, kind, code, is_system\) VALUES \(\$1, \$2, 'resource', \$3, true\) RETURNING id`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(i)))
	}

	// 13 товарных категорий.
	for range model.DefaultCategories {
		mock.ExpectExec(`INSERT INTO categories \(name, name_norm, kind\) VALUES \(\$1, \$2, 'good'\)`).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	// 131 ресурс (20 + 111), approved/palette, props.
	resourceCount := len(resource.LayerCatalog()) + len(resource.RealCatalog())
	require.Equal(t, 131, resourceCount, "слой 20 + витрина 111")
	for i := 0; i < resourceCount; i++ {
		mock.ExpectExec(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source, props\)\s+VALUES \(\$1, \$2, \$3, 'resource', 'approved', 'palette', \$4\)`).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	// Маркер — в той же транзакции.
	mock.ExpectExec(`INSERT INTO generation_config \(key, payload\) VALUES \(\$1, \$2\)`).
		WithArgs(SeedMarkerKey, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	require.NoError(t, Seed(db))
	require.NoError(t, mock.ExpectationsWereMet())
}