// internal/integration/producer_sections_migration_test.go
// Проверки миграции 000085 «раздел вида постройки» (спека
// 2026-09-25-студия-разделы-построек-по-видам §2/§3): колонка
// producer_types.section, бэкфилл 6 канон-корней по name_norm, идемпотентность,
// переименованный корень остаётся NULL, CHECK отсутствует. Живой PostgreSQL в
// изолированной scratch-схеме (helpers_test.go); реальная dev-БД не трогается.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// migration085SQL — содержимое миграции (для повторного прогона в тесте).
func migration085SQL(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000085_producer_sections.sql"))
	require.NoErrorf(t, err, "миграция 000085_producer_sections.sql")
	return string(src)
}

// TestMigration085ProducerSections — колонка заводится, бэкфилл даёт разделы
// шести канон-корней (только parent_id IS NULL, только NULL), идемпотентна,
// переименованный корень остаётся NULL, CHECK на section не ставится.
func TestMigration085ProducerSections(t *testing.T) {
	s := openMigrated(t)
	sql085 := migration085SQL(t)

	// «Лаборатория» создана миграцией 000051 (name_norm 'лаборатория'); 000085
	// идёт после неё и даёт lab.
	require.Equal(t, "lab", mustStr(t, s.db,
		`SELECT section FROM producer_types WHERE name_norm = 'лаборатория'`))

	// CHECK на section отсутствует (§2.2): список разделов — данные интерфейса.
	require.Equal(t, int64(0), mustInt(t, s.db,
		`SELECT COUNT(*) FROM pg_constraint c
		 JOIN pg_class t ON t.oid = c.conrelid
		 WHERE t.relname = 'producer_types' AND c.contype = 'c'
		   AND pg_get_constraintdef(c.oid) LIKE '%section%'`))

	// Данные после первого применения: канон-корни с NULL + переименованный
	// корень + подтип + корень с уже заданным разделом.
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind) VALUES ('Колония','колония','goods') RETURNING id`)
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind) VALUES ('Фабрика','фабрика','goods') RETURNING id`)
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind) VALUES ('Автофабрика','автофабрика','goods') RETURNING id`)
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind) VALUES ('Добывающая платформа','добывающая платформа','goods') RETURNING id`)
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind) VALUES ('Энергостанция','энергостанция','energy') RETURNING id`)
	// Переименованный корень (не канон) — остаётся NULL → «Прочее».
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind) VALUES ('Поселение','поселение','goods') RETURNING id`)
	// Подтип канон-корня: section не задаётся (наследуется).
	rootID := mustInt(t, s.db, `SELECT id FROM producer_types WHERE name_norm = 'фабрика'`)
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind, parent_id)
		VALUES ('Фабрика продовольствия','фабрика продовольствия','goods',$1) RETURNING id`, rootID)
	// Корень с уже заданным разделом — бэкфилл его не перетирает (только NULL).
	mustInt(t, s.db, `INSERT INTO producer_types (name, name_norm, kind, section)
		VALUES ('Энергостанция старая','энергостанция старая','energy','factory') RETURNING id`)

	if _, err := s.db.Exec(sql085); err != nil {
		t.Fatalf("прогон 000085: %v", err)
	}

	want := map[string]string{
		"колония":              "colony",
		"фабрика":              "factory",
		"автофабрика":          "factory",
		"лаборатория":          "lab",
		"добывающая платформа": "mining",
		"энергостанция":        "energy",
	}
	for norm, sec := range want {
		require.Equalf(t, sec, mustStr(t, s.db,
			`SELECT section FROM producer_types WHERE name_norm = $1`, norm), "канон-корень %s", norm)
	}
	// Переименованный корень и подтип — NULL (в «Прочее»/наследуется).
	require.Equal(t, int64(0), mustInt(t, s.db,
		`SELECT COUNT(*) FROM producer_types
		 WHERE name_norm IN ('поселение','фабрика продовольствия') AND section IS NOT NULL`))
	require.Equal(t, "factory", mustStr(t, s.db,
		`SELECT section FROM producer_types WHERE name_norm = 'энергостанция старая'`))

	// Идемпотентность: повторный прогон — no-op, значения не поехали.
	if _, err := s.db.Exec(sql085); err != nil {
		t.Fatalf("повторный прогон 000085: %v", err)
	}
	for norm, sec := range want {
		require.Equalf(t, sec, mustStr(t, s.db,
			`SELECT section FROM producer_types WHERE name_norm = $1`, norm), "после повтора: %s", norm)
	}
}
