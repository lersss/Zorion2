package integration

import (
	"testing"

	"zorion/migrations"
)

// TestMigrationsApplyOnEmptySchema — миграции накатываются на пустую схему
// и повторный Apply ничего не выполняет (версии уже в schema_migrations).
func TestMigrationsApplyOnEmptySchema(t *testing.T) {
	s := openScratch(t)

	if err := migrations.Apply(s.db); err != nil {
		t.Fatalf("первый Apply: %v", err)
	}
	applied := mustInt(t, s.db, `SELECT COUNT(*) FROM schema_migrations`)
	if applied == 0 {
		t.Fatal("schema_migrations пуста после Apply")
	}

	// Миграции создали объекты в scratch-схеме, а не в public.
	var owner string
	if err := s.db.QueryRow(`SELECT n.nspname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.oid = to_regclass('contracts')`).Scan(&owner); err != nil {
		t.Fatalf("contracts не найдена после Apply: %v", err)
	}
	if owner != s.schema {
		t.Fatalf("contracts создана в схеме %q, ожидалась %q", owner, s.schema)
	}

	if err := migrations.Apply(s.db); err != nil {
		t.Fatalf("повторный Apply: %v", err)
	}
	again := mustInt(t, s.db, `SELECT COUNT(*) FROM schema_migrations`)
	if again != applied {
		t.Fatalf("повторный Apply изменил число применённых миграций: %d → %d", applied, again)
	}
}
