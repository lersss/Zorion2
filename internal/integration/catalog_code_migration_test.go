// internal/integration/catalog_code_migration_test.go
// Интеграционные проверки номерной метки записи каталога (миграция И1 спеки
// 2026-09-24-каталог-экспорт-импорт-контента-на-прод §3.1–3.3). Мок не знает
// про SEQUENCE/DEFAULT/частичный UNIQUE — здесь метка проверяется на живой
// PostgreSQL в изолированной scratch-схеме (см. helpers_test.go).
package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio"
)

// TestCatalogCodeMigrationBackfillAndDefault — метка заводится: бэкфилл
// существующих записей идёт по порядку id (детерминированно), а новая запись
// получает номер из счётчика (DEFAULT); частичный UNIQUE держит уникальность,
// NULL допустим (метка — «запись вне управления»).
func TestCatalogCodeMigrationBackfillAndDefault(t *testing.T) {
	s := openMigrated(t)

	// Миграции создали effect_types до миграции меток (000070 «Голод»,
	// 000079 «Жажда») — бэкфилл раздал им коды в порядке id.
	require.Equal(t, "e_0001", mustStr(t, s.db, `SELECT code FROM effect_types ORDER BY id LIMIT 1`))
	require.Equal(t, "e_0002", mustStr(t, s.db, `SELECT code FROM effect_types ORDER BY id OFFSET 1 LIMIT 1`))

	// «Лаборатория» создана миграцией 000051 → бэкфилл p_0001 (свежая схема:
	// в producer_types пока только она).
	require.Equal(t, "p_0001", mustStr(t, s.db, `SELECT code FROM producer_types ORDER BY id LIMIT 1`))

	// Новая запись получает метку из счётчика (DEFAULT): префикс + 4 цифры.
	require.Equal(t, "e_0003", mustStr(t, s.db,
		`INSERT INTO effect_types (name, name_norm, impact, params) VALUES ('Тест','тест код','population_rate','{}') RETURNING code`))
	require.Equal(t, "p_0002", mustStr(t, s.db,
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ('Тест','тест код','goods') RETURNING code`))
	require.Equal(t, "i_0001", mustStr(t, s.db,
		`INSERT INTO items (name, name_norm, slot_type) VALUES ('Тест','тест код','модуль') RETURNING code`))

	catID := mustInt(t, s.db,
		`INSERT INTO categories (name, name_norm, kind) VALUES ('Кат','кат','good') RETURNING id`)
	require.Equal(t, "g_0001", mustStr(t, s.db,
		`INSERT INTO goods (name, name_norm, category_id, kind) VALUES ('Товар','товар',$1,'good') RETURNING code`, catID))

	// NULL допустим схемой (метка необязательна).
	require.Equal(t, "e_0004", mustStr(t, s.db,
		`INSERT INTO effect_types (name, name_norm, impact, params) VALUES ('Тест2','тест2','population_rate','{}') RETURNING code`))
	if _, err := s.db.Exec(`UPDATE effect_types SET code = NULL WHERE name_norm = 'тест2'`); err != nil {
		t.Fatalf("NULL-метка должна быть допустима: %v", err)
	}

	// Частичный UNIQUE: дубль метки в пределах таблицы отвергается.
	if _, err := s.db.Exec(`UPDATE effect_types SET code = 'e_0001' WHERE name_norm = 'тест2'`); err == nil {
		t.Fatal("дубль метки в таблице должен нарушать уникальность (§3.1)")
	}

	// Бэкфилл детерминирован: порядок по метке совпадает с порядком по id.
	require.Equal(t, int64(0), mustInt(t, s.db,
		`SELECT COUNT(*) FROM (
		     SELECT id, code, row_number() OVER (ORDER BY id) rn_id, row_number() OVER (ORDER BY code) rn_code
		     FROM effect_types WHERE code IS NOT NULL
		 ) x WHERE rn_id <> rn_code`))
}

// TestCatalogCodeFreshSeedMarksAllContent — свежая БД (миграции + Go-сид):
// у всех контентных записей непустая уникальная метка, порядок метки совпадает
// с порядком вставки (id). Первый ресурс — g_0001, последний (131-й) — g_0131.
func TestCatalogCodeFreshSeedMarksAllContent(t *testing.T) {
	s := openMigrated(t)

	if err := goodsstudio.Seed(s.db); err != nil {
		t.Fatalf("goodsstudio.Seed: %v", err)
	}
	if err := goodsstudio.SeedProducers(s.db); err != nil {
		t.Fatalf("goodsstudio.SeedProducers: %v", err)
	}

	for _, table := range []string{"goods", "producer_types", "items", "effect_types"} {
		require.Equalf(t, int64(0), mustInt(t, s.db,
			`SELECT COUNT(*) FROM `+table+` WHERE code IS NULL`), "%s: есть записи без метки", table)
		require.Equalf(t, int64(0), mustInt(t, s.db,
			`SELECT COUNT(*) - COUNT(DISTINCT code) FROM `+table), "%s: метки не уникальны", table)
		require.Equalf(t, int64(0), mustInt(t, s.db,
			`SELECT COUNT(*) FROM (
			     SELECT id, code, row_number() OVER (ORDER BY id) rn_id, row_number() OVER (ORDER BY code) rn_code
			     FROM `+table+`
			 ) x WHERE rn_id <> rn_code`), "%s: порядок метки не совпадает с порядком id", table)
	}

	// Сид каталога — 131 ресурс: g_0001 … g_0131.
	require.Equal(t, int64(131), mustInt(t, s.db, `SELECT COUNT(*) FROM goods`))
	require.Equal(t, "g_0001", mustStr(t, s.db, `SELECT code FROM goods ORDER BY id LIMIT 1`))
	require.Equal(t, "g_0131", mustStr(t, s.db, `SELECT code FROM goods ORDER BY id DESC LIMIT 1`))
}
