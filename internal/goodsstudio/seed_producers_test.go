// internal/goodsstudio/seed_producers_test.go
// Тесты сидера каталога производителей (спека 2026-09-20-фабрики §10.1 п.2):
// маркер producer_catalog_seed — пропуск; полный сид — 8 типов производителей
// + 4 предмета + 4 связи + маркер, всё в одной транзакции.
package goodsstudio

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// TestSeedProducersMarkerSkips — маркер есть → сид пропускается (повторный
// старт не возвращает удалённое, С1-паттерн): никаких INSERT не выполняется.
func TestSeedProducersMarkerSkips(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM generation_config WHERE key = \$1\)`).
		WithArgs(ProducerSeedMarkerKey).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	require.NoError(t, SeedProducers(db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSeedProducersFull — нет маркера → полный сид в одной транзакции:
// категории (для добывающей платформы), 8 типов, 4 предмета, 4 связи, маркер.
func TestSeedProducersFull(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM generation_config WHERE key = \$1\)`).
		WithArgs(ProducerSeedMarkerKey).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	mock.ExpectBegin()

	// Категории по name_norm (посеяны goodsstudio.Seed до этого сида).
	mock.ExpectQuery(`SELECT id, name_norm FROM categories`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name_norm"}).
			AddRow(int64(7), "минералы"))

	// 8 типов производителей (approved).
	for range seedProducers {
		mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, output, input, params, status\)\s+VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, 'approved'\) RETURNING id`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	}

	// 4 предмета (approved).
	for range seedItems {
		mock.ExpectQuery(`INSERT INTO items \(name, name_norm, slot_type, status, unlocks, params\)\s+VALUES \(\$1, \$2, \$3, 'approved', \$4, \$5\) RETURNING id`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	}

	// 4 связи лабораторий с предметами.
	for range seedProducerItems {
		mock.ExpectExec(`INSERT INTO producer_items \(producer_type_id, item_id\) VALUES \(\$1, \$2\)`).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	// Маркер — в той же транзакции.
	mock.ExpectExec(`INSERT INTO generation_config \(key, payload\) VALUES \(\$1, \$2\)`).
		WithArgs(ProducerSeedMarkerKey, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	require.NoError(t, SeedProducers(db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSeedProducersContent — состав сида: 8 типов (поселение, фабрика,
// автофабрика, добывающая платформа, энергостанция, 3 лаборатории),
// 4 предмета (чертёж, сертификат, модуль, кирка), 4 связи; автофабрика —
// корзина роботов (энергия + детали + комплектующие, НЕ еда).
func TestSeedProducersContent(t *testing.T) {
	require.Len(t, seedProducers, 8, "поселение, фабрика, автофабрика, платформа, станция, 3 лаборатории")
	require.Len(t, seedItems, 4, "чертёж, сертификат анализа, модуль корабля, кирка")
	require.Len(t, seedProducerItems, 4, "3 лаборатории → предметы + сертификат")

	var autoFactory *seedProducer
	for i := range seedProducers {
		if seedProducers[i].Name == "Автофабрика" {
			autoFactory = &seedProducers[i]
		}
	}
	require.NotNil(t, autoFactory, "автофабрика в сиде")
	require.Equal(t, "goods", autoFactory.Kind)
	require.Contains(t, autoFactory.Input, "детали")
	require.Contains(t, autoFactory.Input, "комплектующие")
	require.Contains(t, autoFactory.Input, "energy")
	require.NotContains(t, autoFactory.Input, "еда")

	// 3 лаборатории — kind=items.
	labs := 0
	for _, p := range seedProducers {
		if p.Kind == "items" {
			labs++
		}
	}
	require.Equal(t, 3, labs, "3 лаборатории как типы предметов-производителей")
}