// Интеграционный тест миграции 000082 (спека
// 2026-09-24-постройка-структур-на-планете §3, T11): идемпотентность,
// CHECK-пара владельца, FK RESTRICT у buildings.producer_type_id и отсутствие
// бэкфилла владельцев (Г1). Без TEST_DATABASE_URL/DATABASE_URL тест скипается.
package integration

import (
	"os"
	"path/filepath"
	"testing"
)

// migration082SQL — содержимое миграции (для повторного прогона в тесте).
func migration082SQL(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000082_structures_owner.sql"))
	if err != nil {
		t.Fatalf("миграция 000082_structures_owner.sql: %v", err)
	}
	return string(src)
}

// TestMigration082StructuresOwner — колонки/констрейнты на месте, повторный
// прогон идемпотентен, бэкфилла нет, FK RESTRICT работает.
func TestMigration082StructuresOwner(t *testing.T) {
	s := openMigrated(t)
	sql082 := migration082SQL(t)

	// Идемпотентность: повторное выполнение файла не падает.
	if _, err := s.db.Exec(sql082); err != nil {
		t.Fatalf("повторный прогон 000082: %v", err)
	}

	// Колонки существуют (таблица — в scratch-схеме: information_schema видит и
	// public, поэтому фильтруем по current_schema()).
	for _, col := range []string{"owner_type", "owner_id"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'settlements' AND column_name = $1`, col).Scan(&n); err != nil {
			t.Fatalf("колонка settlements.%s: %v", col, err)
		}
		if n != 1 {
			t.Fatalf("колонка settlements.%s отсутствует", col)
		}
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'buildings' AND column_name = 'producer_type_id'`).Scan(&n); err != nil {
		t.Fatalf("колонка buildings.producer_type_id: %v", err)
	}
	if n != 1 {
		t.Fatal("колонка buildings.producer_type_id отсутствует")
	}

	// FK producer_type_id → producer_types с ON DELETE RESTRICT (confdeltype 'r').
	var deltype string
	if err := s.db.QueryRow(`
		SELECT c.confdeltype FROM pg_constraint c
		JOIN pg_class rel ON rel.oid = c.conrelid
		JOIN pg_namespace ns ON ns.oid = rel.relnamespace
		WHERE ns.nspname = current_schema() AND rel.relname = 'buildings' AND c.contype = 'f'
		  AND c.conname LIKE '%producer_type_id%'`).Scan(&deltype); err != nil {
		t.Fatalf("FK buildings.producer_type_id: %v", err)
	}
	if deltype != "r" {
		t.Fatalf("FK buildings.producer_type_id: confdeltype = %q, ожидался 'r' (RESTRICT)", deltype)
	}

	// Легаси-поселение (без владельца): бэкфилла нет — после повторного прогона
	// миграции owner_id остаётся NULL (Г1).
	const (
		worldID      = "11111111-1111-1111-1111-111111111111"
		planetID     = "22222222-2222-2222-2222-222222222222"
		settlementID = "33333333-3333-3333-3333-333333333333"
	)
	if _, err := s.db.Exec(`INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, 'W', 0, 0)`, worldID); err != nil {
		t.Fatalf("world: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO planets (id, world_id, name, orbit_index, data) VALUES ($1, $2, 'P', 0, '{}')`, planetID, worldID); err != nil {
		t.Fatalf("planet: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO settlements (id, planet_id, population, population_exact, stability, computed_at)
		VALUES ($1, $2, 10, 10, 50, NOW())`, settlementID, planetID); err != nil {
		t.Fatalf("settlement: %v", err)
	}
	if _, err := s.db.Exec(sql082); err != nil {
		t.Fatalf("повторный прогон 000082 после вставки: %v", err)
	}
	if got := mustInt(t, s.db, `SELECT COUNT(*) FROM settlements WHERE id = $1 AND owner_id IS NULL`, settlementID); got != 1 {
		t.Fatal("бэкфилл владельца не должен выполняться (Г1): owner_id обязан остаться NULL")
	}

	// CHECK-пара: owner_type без owner_id → ошибка.
	if _, err := s.db.Exec(`UPDATE settlements SET owner_type = 'faction' WHERE id = $1`, settlementID); err == nil {
		t.Fatal("CHECK-пара владельца не сработала: owner_type без owner_id")
	}
	// CHECK-домен: owner_type вне {player,faction,agent} → ошибка.
	if _, err := s.db.Exec(`UPDATE settlements SET owner_type = 'npc', owner_id = gen_random_uuid() WHERE id = $1`, settlementID); err == nil {
		t.Fatal("CHECK-домен owner_type не сработал")
	}

	// FK RESTRICT: тип в использовании удалить нельзя.
	if _, err := s.db.Exec(`INSERT INTO producer_types (id, name, name_norm, kind) VALUES (9001, 'Тип', 'тип', 'goods')`); err != nil {
		t.Fatalf("producer_type: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO buildings (id, planet_id, building_type, owner_type, owner_id, producer_type_id)
		VALUES (gen_random_uuid(), $1, 'producer', 'player', gen_random_uuid(), 9001)`, planetID); err != nil {
		t.Fatalf("building: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM producer_types WHERE id = 9001`); err == nil {
		t.Fatal("FK RESTRICT не сработал: тип в использовании удалён")
	}
}
