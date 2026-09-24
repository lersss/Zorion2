package integration

import (
	"database/sql"
	"os"
	"testing"
)

// Путь к файлу миграции относительно пакета (тест исполняет его повторно,
// чтобы проверить идемпотентность и обрезку перегруза на заполненной схеме).
const cargoDeltaMigrationPath = "../../migrations/000080_cargo_hold_delta.sql"

func execCargoDeltaMigration(t *testing.T, s *scratchDB) {
	t.Helper()
	raw, err := os.ReadFile(cargoDeltaMigrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := s.db.Exec(string(raw)); err != nil {
		t.Fatalf("exec migration 000080: %v", err)
	}
}

func mustFloat(t *testing.T, db *sql.DB, query string, args ...interface{}) float64 {
	t.Helper()
	var f float64
	if err := db.QueryRow(query, args...).Scan(&f); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return f
}

// TestCargoDeltaMigrationCapacityAndSlots — миграция 000080 (спека трюма §20.5,
// шаги 1–2): значение модуля cargo_1 80 → 30 т и три универсальных слота у
// модели starter. Повторный прогон идемпотентен.
func TestCargoDeltaMigrationCapacityAndSlots(t *testing.T) {
	s := openScratch(t)
	seedCargoDeltaSchema(t, s)

	execCargoDeltaMigration(t, s)

	// Шаг 1: params.capacity 80 → 30.
	if got := mustFloat(t, s.db, `SELECT (params->>'capacity')::double precision FROM equipment WHERE id = 'cargo_1'`); got != 30 {
		t.Fatalf("cargo_1 capacity = %v, ожидалось 30", got)
	}
	// Шаг 2: slots.universal 1 → 3.
	if got := mustFloat(t, s.db, `SELECT (slots->>'universal')::double precision FROM ship_models WHERE id = 'starter'`); got != 3 {
		t.Fatalf("starter slots.universal = %v, ожидалось 3", got)
	}

	// Идемпотентность: повторный прогон не меняет значения.
	execCargoDeltaMigration(t, s)
	if got := mustFloat(t, s.db, `SELECT (params->>'capacity')::double precision FROM equipment WHERE id = 'cargo_1'`); got != 30 {
		t.Fatalf("повторный прогон изменил capacity: %v", got)
	}
	if got := mustFloat(t, s.db, `SELECT (slots->>'universal')::double precision FROM ship_models WHERE id = 'starter'`); got != 3 {
		t.Fatalf("повторный прогон изменил slots.universal: %v", got)
	}
}

// TestCargoDeltaMigrationTruncatesOverload — миграция 000080, шаг 3 (§20.5/§20.8
// п.4): перегруженные трюмы обрезаются до новой ёмкости. Ёмкость = base_capacity
// модели + Σ cargo-модулей; «хвост» по good_id удаляется, пограничная строка
// урезается. Повторный прогон результат не меняет (идемпотентность).
func TestCargoDeltaMigrationTruncatesOverload(t *testing.T) {
	s := openScratch(t)
	seedCargoDeltaSchema(t, s)

	execCargoDeltaMigration(t, s)

	// u1: ёмкость 20 + 30 = 50 т, груз 95 т (60 + 35).
	// good_id 1 идёт первым: mass_before 0 < 50, 60 > 50 → урезается до 50.
	// good_id 2: mass_before 60 >= 50 → «хвост», удаляется.
	if got := mustInt(t, s.db, `SELECT COUNT(*) FROM player_cargo WHERE user_id = '11111111-1111-1111-1111-111111111111'`); got != 1 {
		t.Fatalf("u1: строк в трюме %d, ожидалась 1 (хвост удалён)", got)
	}
	if got := mustFloat(t, s.db, `SELECT quantity FROM player_cargo WHERE user_id = '11111111-1111-1111-1111-111111111111' AND good_id = 1`); got != 50 {
		t.Fatalf("u1: quantity good_id=1 = %v, ожидалось 50 (пограничная урезана)", got)
	}
	if got := mustFloat(t, s.db, `SELECT COALESCE(SUM(pc.quantity * g.weight), 0) FROM player_cargo pc JOIN goods g ON g.id = pc.good_id WHERE pc.user_id = '11111111-1111-1111-1111-111111111111'`); got != 50 {
		t.Fatalf("u1: занято %v, ожидалось 50 (used ≤ total)", got)
	}

	// u2: без cargo-модуля ёмкость = врождённая 20 т, груз 30 т → 20 т.
	if got := mustFloat(t, s.db, `SELECT quantity FROM player_cargo WHERE user_id = '22222222-2222-2222-2222-222222222222' AND good_id = 3`); got != 20 {
		t.Fatalf("u2: quantity = %v, ожидалось 20 (только врождённая ёмкость)", got)
	}

	// Идемпотентность обрезки: повторный прогон ничего не меняет.
	before := mustStr(t, s.db, `SELECT string_agg(user_id || ':' || good_id || ':' || round(quantity::numeric, 3)::text, ',' ORDER BY user_id, good_id) FROM player_cargo`)
	execCargoDeltaMigration(t, s)
	if got := mustStr(t, s.db, `SELECT string_agg(user_id || ':' || good_id || ':' || round(quantity::numeric, 3)::text, ',' ORDER BY user_id, good_id) FROM player_cargo`); got != before {
		t.Fatalf("повторный прогон изменил трюм: %q → %q", before, got)
	}
}

// seedCargoDeltaSchema — минимальная схема под шаги миграции 000080:
// equipment / ship_models / users / goods / player_cargo (по образцу
// race_ships_migration_test.go). u1 — с грузовым модулем (ёмкость 50 т,
// перегруз 95 т), u2 — без модуля (ёмкость 20 т, перегруз 30 т).
func seedCargoDeltaSchema(t *testing.T, s *scratchDB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE equipment (id TEXT PRIMARY KEY, type TEXT NOT NULL, name TEXT NOT NULL, params JSONB NOT NULL)`,
		`CREATE TABLE ship_models (id TEXT PRIMARY KEY, slots JSONB NOT NULL, base_capacity DOUBLE PRECISION NOT NULL DEFAULT 0)`,
		`CREATE TABLE users (id UUID PRIMARY KEY, ship_model_id TEXT, equipment JSONB)`,
		`CREATE TABLE goods (id BIGINT PRIMARY KEY, weight DOUBLE PRECISION NOT NULL)`,
		`CREATE TABLE player_cargo (
			user_id UUID NOT NULL, good_id BIGINT NOT NULL,
			quantity DOUBLE PRECISION NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, good_id))`,
		`INSERT INTO equipment (id, type, name, params) VALUES
			('cargo_1', 'cargo', 'Грузовой модуль-1', '{"capacity": 80}')`,
		`INSERT INTO ship_models (id, slots, base_capacity) VALUES
			('starter', '{"universal": 1}', 20)`,
		`INSERT INTO goods (id, weight) VALUES (1, 1.0), (2, 1.0), (3, 1.0)`,
		`INSERT INTO users (id, ship_model_id, equipment) VALUES
			('11111111-1111-1111-1111-111111111111', 'starter', '{"universal":"cargo_1"}'),
			('22222222-2222-2222-2222-222222222222', 'starter', '{}')`,
		`INSERT INTO player_cargo (user_id, good_id, quantity) VALUES
			('11111111-1111-1111-1111-111111111111', 1, 60),
			('11111111-1111-1111-1111-111111111111', 2, 35),
			('22222222-2222-2222-2222-222222222222', 3, 30)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}
