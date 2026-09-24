// internal/integration/settlement_rename_migration_test.go
// T14 спеки 2026-09-24-каталог-экспорт-импорт-контента-на-прод §6/§9:
// ренейм-миграция `000083` приводит «до-переименованную» БД к канон-именам
// (класс «Поселение» → «Колония», «Аутпост» → «Форпост», «Посёлок» →
// «Поселение»), идемпотентна и НЕ двигает id-ссылки
// (`settlements.settlement_type_id`, `generation_config.default_settlement_type_id`).
// Коллизия целевого имени — явный отказ (не молчаливая порча, п.6 ревьюера).
// Живой PostgreSQL в изолированной scratch-схеме; реальная dev-БД не трогается.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// migration083SQL — содержимое миграции (для повторного прогона в тесте).
func migration083SQL(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000083_settlement_rename.sql"))
	require.NoErrorf(t, err, "миграция 000083_settlement_rename.sql")
	return string(src)
}

// seedPreRenameSettlement — «до-переименованная» БД: класс-корень «Поселение»
// (parent_id NULL), ступень-пол «Аутпост» (params.stage), вторая ступень
// «Посёлок»; поселение ссылается на пол, якорь generation_config — на пол.
// Возвращает id корня и пола.
func seedPreRenameSettlement(t *testing.T, s *scratchDB) (rootID, floorID int64) {
	t.Helper()

	rootID = mustInt(t, s.db,
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ('Поселение','поселение','goods') RETURNING id`)
	floorID = mustInt(t, s.db,
		`INSERT INTO producer_types (name, name_norm, kind, parent_id, params)
		 VALUES ('Аутпост','аутпост','goods',$1,'{"stage":{"enter":0}}') RETURNING id`, rootID)
	_ = mustInt(t, s.db,
		`INSERT INTO producer_types (name, name_norm, kind, parent_id, params)
		 VALUES ('Посёлок','посёлок','goods',$1,'{"stage":{"enter":1000,"exit":750}}') RETURNING id`, rootID)

	worldID := "11111111-1111-1111-1111-111111111111"
	planetID := "22222222-2222-2222-2222-222222222222"
	settlementID := "33333333-3333-3333-3333-333333333333"
	_, err := s.db.Exec(`INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, 'W', 0, 0)`, worldID)
	require.NoError(t, err)
	_, err = s.db.Exec(`INSERT INTO planets (id, world_id, name, orbit_index, data) VALUES ($1, $2, 'P', 0, '{}')`, planetID, worldID)
	require.NoError(t, err)
	_, err = s.db.Exec(`INSERT INTO settlements (id, planet_id, population, population_exact, stability, computed_at, settlement_type_id)
		VALUES ($1, $2, 10, 10, 50, NOW(), $3)`, settlementID, planetID, floorID)
	require.NoError(t, err)

	_, err = s.db.Exec(`INSERT INTO generation_config (key, payload) VALUES ('default_settlement_type_id', to_jsonb($1::bigint))
		ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload`, floorID)
	require.NoError(t, err)
	return rootID, floorID
}

// T14: миграция приводит класс/ступени к канону, идемпотентна, id-ссылки целы.
func TestMigration083SettlementRename(t *testing.T) {
	s := openMigrated(t)
	sql083 := migration083SQL(t)

	// На свежей схеме класса ещё нет — прогон no-op и не падает.
	if _, err := s.db.Exec(sql083); err != nil {
		t.Fatalf("прогон 000083 на свежей схеме (нет класса): %v", err)
	}

	rootID, floorID := seedPreRenameSettlement(t, s)

	if _, err := s.db.Exec(sql083); err != nil {
		t.Fatalf("прогон 000083: %v", err)
	}

	// Канон-имена: корень → «Колония», пол → «Форпост», вторая ступень → «Поселение».
	require.Equal(t, "Колония", mustStr(t, s.db, `SELECT name FROM producer_types WHERE id = $1`, rootID))
	require.Equal(t, "колония", mustStr(t, s.db, `SELECT name_norm FROM producer_types WHERE id = $1`, rootID))
	require.Equal(t, "Форпост", mustStr(t, s.db, `SELECT name FROM producer_types WHERE id = $1`, floorID))
	require.Equal(t, "форпост", mustStr(t, s.db, `SELECT name_norm FROM producer_types WHERE id = $1`, floorID))
	require.Equal(t, "Поселение", mustStr(t, s.db,
		`SELECT name FROM producer_types WHERE parent_id = $1 AND name_norm = 'поселение'`, rootID))

	// id-ссылки целы: поселение и якорь по-прежнему указывают на тот же пол.
	require.Equal(t, floorID, mustInt(t, s.db,
		`SELECT settlement_type_id FROM settlements WHERE id = '33333333-3333-3333-3333-333333333333'`))
	require.Equal(t, floorID, mustInt(t, s.db,
		`SELECT (payload #>> '{}')::bigint FROM generation_config WHERE key = 'default_settlement_type_id'`))

	// Идемпотентность: повторный прогон — no-op, ссылки и имена не поехали.
	if _, err := s.db.Exec(sql083); err != nil {
		t.Fatalf("повторный прогон 000083: %v", err)
	}
	require.Equal(t, "Колония", mustStr(t, s.db, `SELECT name FROM producer_types WHERE id = $1`, rootID))
	require.Equal(t, "Форпост", mustStr(t, s.db, `SELECT name FROM producer_types WHERE id = $1`, floorID))
	require.Equal(t, floorID, mustInt(t, s.db,
		`SELECT settlement_type_id FROM settlements WHERE id = '33333333-3333-3333-3333-333333333333'`))
	require.Equal(t, int64(2), mustInt(t, s.db,
		`SELECT COUNT(*) FROM producer_types WHERE parent_id = $1`, rootID), "состав ступеней не изменился")
}

// п.6 ревьюера: если целевое имя `колония` уже занято иным типом, шаг 1
// обязан ЯВНО отказать (не молчаливая порча) — корень остаётся как был.
func TestMigration083ConflictRefused(t *testing.T) {
	s := openMigrated(t)
	sql083 := migration083SQL(t)

	rootID, _ := seedPreRenameSettlement(t, s)
	// Шумный тип с целевым именем (класс-корень ещё «Поселение»).
	_ = mustInt(t, s.db,
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ('Колония','колония','goods') RETURNING id`)

	if _, err := s.db.Exec(sql083); err == nil {
		t.Fatal("коллизия целевого имени должна давать явный отказ, а не молчаливую порчу")
	}
	// Корень не переименован — состояние не испорчено.
	require.Equal(t, "Поселение", mustStr(t, s.db, `SELECT name FROM producer_types WHERE id = $1`, rootID))
}
