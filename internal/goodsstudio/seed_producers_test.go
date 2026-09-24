// internal/goodsstudio/seed_producers_test.go
// Тесты сидера каталога производителей (спека 2026-09-20-фабрики §10.1 п.2 +
// 2026-09-21-студия-дерево-построек-канвас §1.4): маркер producer_catalog_seed —
// пропуск; полный сид — 17 типов производителей (включая «Лабораторию»-родителя,
// «Фабрику продовольствия»-подтип и 7 подтипов-ступеней класса «Колония», канон
// 2026-09-24) + 4 предмета + 4 связи + маркер, всё в одной транзакции.
package goodsstudio

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
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
// продовольствия), 17 типов, 4 предмета, 4 связи, маркер.
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

	// 17 типов производителей (статуса нет — запись живая, hidden из дефолта;
	// ON CONFLICT DO NOTHING — миграция 000051 могла создать «Лабораторию»).
	for range seedProducers {
		mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race, output, input, params\)\s+VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10\)\s+ON CONFLICT \(name_norm\) DO NOTHING RETURNING id`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	}

	// Базовый тип поселения — id в generation_config (решение создателя
	// 2026-09-23: связь по id, не по имени; путь свежей БД — миграция 000075
	// ключ не ставит).
	mock.ExpectExec(`INSERT INTO generation_config \(key, payload\) VALUES \(\$1, to_jsonb\(\$2::bigint\)\)\s+ON CONFLICT \(key\) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW\(\)`).
		WithArgs(models.DefaultSettlementTypeIDKey, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

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

	// 4 предмета (статуса нет).
	for range seedItems {
		mock.ExpectQuery(`INSERT INTO items \(name, name_norm, slot_type, unlocks, params\)\s+VALUES \(\$1, \$2, \$3, \$4, \$5\) RETURNING id`).
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

// TestSeedProducersContent — состав сида: 17 типов (класс «Колония», 7
// подтипов-ступеней (Форпост → Поселение → … → Экуменополис), фабрика,
// автофабрика, добывающая платформа, энергостанция, лаборатория-родитель,
// 3 лаборатории-подтипа, фабрика продовольствия), 4 предмета (чертёж,
// сертификат, модуль, кирка), 4 связи;
// автофабрика — корзина роботов (энергия + механика + электроника, НЕ еда);
// дерево построек: лаборатории — подтипы «Лаборатории», фабрика продовольствия
// — подтип «Фабрики» с категорией, платформа — без категории.
func TestSeedProducersContent(t *testing.T) {
	require.Len(t, seedProducers, 17, "класс «Колония», 7 ступеней, фабрика, автофабрика, платформа, станция, лаборатория, 3 лаборатории-подтипа, фабрика продовольствия")
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

	// Базовая ступень (спека стадий §4.3, итерации 4 §5.2): подтип класса
	// «Колония» без категории, residual у родителя снят, нормы еды —
	// структура params.eat, порог — params.stage (пол, enter = 0).
	require.Equal(t, "{}", byName["Колония"].Output, "residual у класса «Колония» снят (решение п.38)")
	sub := byName["Форпост"]
	require.Equal(t, "Колония", sub.Parent, "базовая ступень — подтип класса «Колония»")
	require.Equal(t, "goods", sub.Kind)
	require.Equal(t, "", sub.Category, "подтип типа без слотов — без категории (п.37)")
	require.Equal(t, "{}", sub.Output)
	require.Equal(t, "{}", sub.Input)
	require.Contains(t, sub.Params, `"eat"`, "нормы еды — структура params.eat (п.45)")
	require.Contains(t, sub.Params, `"пища"`)
	require.Contains(t, sub.Params, `"очищенная вода"`)
}

// TestSeedSettlementStageLadder — сид даёт ровно 7 подтипов-ступеней класса
// «Колония» (parent «Колония») с корректными params.stage (спека стадий
// §4.3): Форпост — пол (enter 0, exit не задан), далее enter/exit по таблице,
// exit = 75 % от enter (зазор гистерезиса exit < enter). Красный на сиде без
// ступеней.
func TestSeedSettlementStageLadder(t *testing.T) {
	type stage struct {
		Enter *float64 `json:"enter"`
		Exit  *float64 `json:"exit"`
	}
	type ladderEntry struct {
		name    string
		enter   float64
		exit    float64
		hasExit bool
	}
	want := []ladderEntry{
		{"Форпост", 0, 0, false},
		{"Поселение", 1000, 750, true},
		{"Городок", 10000, 7500, true},
		{"Город", 100000, 75000, true},
		{"Мегаполис", 1000000, 750000, true},
		{"Метрополия", 10000000, 7500000, true},
		{"Экуменополис", 100000000, 75000000, true},
	}

	byName := map[string]seedProducer{}
	stages := 0
	for _, p := range seedProducers {
		byName[p.Name] = p
		if p.Parent == "Колония" {
			stages++
		}
	}
	require.Equal(t, 7, stages, "класс «Колония» — ровно 7 подтипов-ступеней")

	for _, w := range want {
		p, ok := byName[w.name]
		require.True(t, ok, "в сиде нет ступени %q", w.name)
		require.Equal(t, "goods", p.Kind)
		require.Equal(t, "Колония", p.Parent)
		require.Equal(t, "", p.Category, "ступени — без категории")
		var params struct {
			Stage *stage `json:"stage"`
		}
		require.NoError(t, json.Unmarshal([]byte(p.Params), &params))
		require.NotNil(t, params.Stage, "ступень %q без params.stage", w.name)
		require.NotNil(t, params.Stage.Enter, "ступень %q без enter", w.name)
		require.Equal(t, w.enter, *params.Stage.Enter, "enter ступени %q", w.name)
		if !w.hasExit {
			require.Nil(t, params.Stage.Exit, "у пола (Форпост) exit не задаётся")
			continue
		}
		require.NotNil(t, params.Stage.Exit, "ступень %q без exit", w.name)
		require.Equal(t, w.exit, *params.Stage.Exit, "exit ступени %q", w.name)
		require.Less(t, *params.Stage.Exit, *params.Stage.Enter, "зазор exit < enter (%q)", w.name)
	}
}

// TestSeedSettlementFloorStageConstant — резолв generation_config.
// default_settlement_type_id (SeedProducers) цепляется к ступени-полу по канон-
// константе settlementFloorStageName: у producer_types нет стабильной колонки
// code, поэтому имя централизовано в одной константе. Тест стережёт связь —
// переименование ступени-пола в сиде без правки константы красный («тест на
// переименование»), а TestSeedProducersFull проверяет, что ключ реально пишется.
func TestSeedSettlementFloorStageConstant(t *testing.T) {
	// Канон 2026-09-24: ступень-пол класса «Колония» — «Форпост».
	require.Equal(t, "Форпост", settlementFloorStageName)

	type stage struct {
		Enter *float64 `json:"enter"`
		Exit  *float64 `json:"exit"`
	}

	var floor []seedProducer
	for _, p := range seedProducers {
		if p.Name == settlementFloorStageName {
			floor = append(floor, p)
		}
	}
	require.Len(t, floor, 1, "в сиде ровно одна ступень с именем константы %q", settlementFloorStageName)

	var params struct {
		Stage *stage `json:"stage"`
	}
	require.NoError(t, json.Unmarshal([]byte(floor[0].Params), &params))
	require.NotNil(t, params.Stage, "ступень-пол %q несёт params.stage", settlementFloorStageName)
	require.NotNil(t, params.Stage.Enter, "у ступени-пола задан enter")
	require.Equal(t, float64(0), *params.Stage.Enter, "ступень-пол — enter 0")
	require.Nil(t, params.Stage.Exit, "у ступени-пола exit не задаётся")
	require.Equal(t, "Колония", floor[0].Parent, "ступень-пол принадлежит классу «Колония»")

	// enter = 0 — структурное свойство пола: на ладдере такая ступень одна.
	roots := 0
	for _, p := range seedProducers {
		if p.Parent != "Колония" {
			continue
		}
		var sp struct {
			Stage *stage `json:"stage"`
		}
		require.NoError(t, json.Unmarshal([]byte(p.Params), &sp))
		if sp.Stage != nil && sp.Stage.Enter != nil && *sp.Stage.Enter == 0 {
			roots++
		}
	}
	require.Equal(t, 1, roots, "ступень-пол (enter 0) на ладдеры ровно одна")
}
