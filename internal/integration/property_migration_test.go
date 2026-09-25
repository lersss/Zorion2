// internal/integration/property_migration_test.go
// Проверки миграции 000090 «индекс владельца строений» (спека
// 2026-09-26-собственность-игрока-в-дашборде §4.4/§13 T10): индекс
// idx_buildings_owner (owner_type, owner_id) создаётся, идемпотентен и НЕ
// частичный (owner_id NOT NULL — предикат вырожден). Планировщик использует
// индекс для запроса по владельцу. Живой PostgreSQL в изолированной
// scratch-схеме (helpers_test.go); реальная dev-БД не трогается.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// migration090SQL — содержимое миграции (для повторного прогона в тесте).
func migration090SQL(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000090_buildings_owner_index.sql"))
	require.NoErrorf(t, err, "миграция 000090_buildings_owner_index.sql")
	return string(src)
}

// TestMigration090BuildingsOwnerIndex — индекс на месте, не частичный,
// идемпотентен, используется планировщиком.
func TestMigration090BuildingsOwnerIndex(t *testing.T) {
	s := openMigrated(t)
	sql090 := migration090SQL(t)

	// Повторный прогон — no-op без ошибки (идемпотентность).
	if _, err := s.db.Exec(sql090); err != nil {
		t.Fatalf("повторный прогон 000090: %v", err)
	}

	// Индекс существует, по (owner_type, owner_id), БЕЗ частичного предиката.
	def := mustStr(t, s.db, `SELECT indexdef FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname = 'idx_buildings_owner'`)
	require.Contains(t, def, "(owner_type, owner_id)")
	require.NotContains(t, strings.ToUpper(def), "WHERE",
		"индекс не частичный: buildings.owner_id NOT NULL (§4.4)")

	// Планировщик использует индекс для запроса по владельцу (seqscan выключен
	// принудительно — на пустой таблице иначе выберется Seq Scan).
	tx, err := s.db.Begin()
	require.NoError(t, err)
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("SET LOCAL enable_seqscan: %v", err)
	}
	rows, err := tx.Query(`EXPLAIN (COSTS OFF) SELECT id FROM buildings
		WHERE owner_type = 'player' AND owner_id = '11111111-1111-1111-1111-111111111111'`)
	require.NoError(t, err)
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Contains(t, plan.String(), "idx_buildings_owner",
		"запрос по владельцу должен идти по индексу, не Seq Scan: %s", plan.String())
}
