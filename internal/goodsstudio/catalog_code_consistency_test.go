// internal/goodsstudio/catalog_code_consistency_test.go
// Критерии T1/T14 спеки 2026-09-24-каталог-экспорт-импорт-контента-на-прод
// (§3.1–3.3, итерация И1): номерная метка переноса `code` на контентных
// таблицах каталога (goods/producer_types/items/effect_types) — колонка TEXT,
// DEFAULT из монотонного SEQUENCE, частичный UNIQUE (в пределах таблицы),
// бэкфилл существующих по порядку id (детерминированно, только NULL). Метка —
// номерная (префикс таблицы + минимум 4 цифры), НЕ слаг из имени; `categories`
// метки не получает (там свой семантический code, §3.4).
package goodsstudio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// findCatalogCodeMigration — текст миграции `*_catalog_code.sql` (номер не
// фиксируем: файл ищется по суффиксу, приём T31).
func findCatalogCodeMigration(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_catalog_code.sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		return string(b)
	}
	t.Fatal("миграция *_catalog_code.sql не найдена")
	return ""
}

// tablePrefix — пары «таблица каталога → префикс метки» (§3.1).
var tablePrefix = map[string]string{
	"goods":         "g_",
	"producer_types": "p_",
	"items":         "i_",
	"effect_types":  "e_",
}

// T1: миграция заводит метку у четырёх контентных таблиц — колонку `code`,
// счётчик-SEQUENCE на таблицу, DEFAULT из счётчика и частичный UNIQUE; бэкфилл
// существующих идёт по порядку id и только для NULL (идемпотентность).
func TestCatalogCodeMigrationShape(t *testing.T) {
	sql := stripSQLLineComments(findCatalogCodeMigration(t))

	for table, prefix := range tablePrefix {
		require.Containsf(t, sql, "ALTER TABLE "+table+" ADD COLUMN IF NOT EXISTS code TEXT",
			"%s: колонка метки (nullable, §3.1)", table)
		require.Containsf(t, sql, "CREATE SEQUENCE IF NOT EXISTS "+table+"_code_seq",
			"%s: монотонный счётчик (§3.2)", table)
		require.Containsf(t, sql, "catalog_next_code('"+prefix+"', '"+table+"_code_seq')",
			"%s: DEFAULT берёт номер из счётчика (§3.2)", table)
		require.Containsf(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS uq_"+table+"_code ON "+table+" (code) WHERE code IS NOT NULL",
			"%s: частичный UNIQUE в пределах таблицы (§3.1)", table)
	}

	// Метка — номерная, не слаг: форматтер раздаёт префикс + lpad, НЕ имя.
	require.Contains(t, sql, "catalog_format_code")
	require.Contains(t, sql, "lpad(n::text, 4, '0')", "минимум 4 цифры (§3.1)")
	require.NotContains(t, sql, "name_norm", "метка не выводится из имени (§3.1)")

	// Бэкфилл детерминирован: по порядку id, только NULL (повторный прогон — no-op).
	require.Contains(t, sql, "ORDER BY id")
	require.Contains(t, sql, "WHERE code IS NULL")
	require.Contains(t, sql, "row_number() OVER (ORDER BY id)")

	// Счётчик сдвигается на максимум: новая запись продолжит нумерацию (§3.2).
	require.Contains(t, sql, "setval(")

	// `categories` метку не получает (свой семантический code, §3.4).
	require.NotContains(t, sql, "ALTER TABLE categories ADD COLUMN IF NOT EXISTS code",
		"categories — без метки переноса (§3.4)")
}

// T1/§3.1: номер не обрезается при росте (g_10001): lpad(…,4,'0') обрезал бы
// ≥10000 до 4 цифр, to_char(…,'FM0000') переполняется в '####'. Форматтер
// реализует «минимум 4 цифры, растёт без обрезки» через CASE.
func TestCatalogCodeFormatGrowsWithoutTruncation(t *testing.T) {
	sql := stripSQLLineComments(findCatalogCodeMigration(t))
	require.Contains(t, sql, "CASE WHEN n < 10000", "номер ≥10000 не обрезается (§3.1)")
	require.NotContains(t, sql, "to_char(", "to_char(…,'FM0000') переполняется в '####'")
}
