// internal/goodsstudio/seed_producers_test.go
// Тесты сидера каталога производителей (спека 2026-09-20-фабрики §10.1 п.2 +
// 2026-09-21-студия-дерево-построек-канвас §1.4): маркер producer_catalog_seed —
// пропуск; полный сид — 10 типов производителей (включая «Лабораторию»-родителя
// и «Фабрику продовольствия»-подтип) + 4 предмета + 4 связи + маркер, всё в
// одной транзакции.
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
// категории (минералы — не используется, продовольствие — для фабрики
// продовольствия), 10 типов, 4 предмета, 4 связи, маркер.
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
			AddRow(int64(7), "минералы").
			AddRow(int64(8), "продовольствие"))

	// 10 типов производителей (approved; ON CONFLICT DO NOTHING — миграция
	// 000051 могла создать «Лабораторию» на свежей БД).
	for range seedProducers {
		mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race, output, input, params, status\)\s+VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, 'approved'\)\s+ON CONFLICT \(name_norm\) DO NOTHING RETURNING id`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	}

	// Слоты родителя (спека скрытых §1.3, путь 2): категории по kind —
	// Фабрика/Автофабрика × good (продовольствие), Платформа × resource
	// (минералы); 3 слота ON CONFLICT DO NOTHING.
	mock.ExpectQuery(`SELECT id, kind FROM categories`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kind"}).
			AddRow(int64(7), "resource").
			AddRow(int64(8), "good"))
	for i := 0; i < 3; i++ {
		mock.ExpectExec(`INSERT INTO producer_slots \(parent_id, category_id\) VALUES \(\$1, \$2\)\s+ON CONFLICT DO NOTHING`).
			WillReturnResult(sqlmock.NewResult(0, 1))
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

// TestSeedProducersContent — состав сида: 10 типов (поселение, фабрика,
// автофабрика, добывающая платформа, энергостанция, лаборатория-родитель,
// 3 лаборатории-подтипа, фабрика продовольствия), 4 предмета (чертёж,
// сертификат, модуль, кирка), 4 связи; автофабрика — корзина роботов
// (энергия + механика + электроника, НЕ еда); дерево построек: лаборатории —
// подтипы «Лаборатории», фабрика продовольствия — подтип «Фабрики» с
// категорией, платформа — без категории.
func TestSeedProducersContent(t *testing.T) {
	require.Len(t, seedProducers, 10, "поселение, фабрика, автофабрика, платформа, станция, лаборатория, 3 лаборатории-подтипа, фабрика продовольствия")
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
	require.Contains(t, autoFactory.Input, "механика")
	require.Contains(t, autoFactory.Input, "электроника")
	require.Contains(t, autoFactory.Input, "energy")
	require.NotContains(t, autoFactory.Input, "еда")

	// Лаборатория-родитель + 3 лаборатории-подтипа — kind=items.
	labs := 0
	for _, p := range seedProducers {
		if p.Kind == "items" {
			labs++
		}
	}
	require.Equal(t, 4, labs, "лаборатория-родитель + 3 лаборатории как типы предметов-производителей")

	// Дерево построек: подтипы ссылаются на родителей по имени.
	byName := map[string]seedProducer{}
	for _, p := range seedProducers {
		byName[p.Name] = p
	}
	require.Equal(t, "Лаборатория", byName["Лаборатория космических технологий"].Parent)
	require.Equal(t, "Лаборатория", byName["Лаборатория экипировки"].Parent)
	require.Equal(t, "Исследовательская лаборатория", byName["Исследовательская лаборатория"].Name)
	require.Equal(t, "Лаборатория", byName["Исследовательская лаборатория"].Parent)
	require.Equal(t, "Фабрика", byName["Фабрика продовольствия"].Parent)
	require.Equal(t, "продовольствие", byName["Фабрика продовольствия"].Category)
	require.Equal(t, "", byName["Добывающая платформа"].Category, "платформа — чистый тип уровня 3, без категории")
	require.Equal(t, "", byName["Лаборатория"].Parent, "лаборатория — тип-родитель")
}