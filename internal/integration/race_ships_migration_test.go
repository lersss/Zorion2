package integration

import (
	"os"
	"strings"
	"testing"
)

// Путь к файлу миграции относительно пакета (тест исполняет его повторно,
// чтобы проверить бэкфиллы на заполненной схеме и идемпотентность).
const raceShipsMigrationPath = "../../migrations/000074_race_ships.sql"

func execRaceShipsMigration(t *testing.T, s *scratchDB) {
	t.Helper()
	raw, err := os.ReadFile(raceShipsMigrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := s.db.Exec(string(raw)); err != nil {
		t.Fatalf("exec migration 000074: %v", err)
	}
}

// TestRaceShipsMigrationBackfill — миграция 000074 на заполненной схеме
// (спека 2026-09-23 §4.3, П1): колонки и дефолты, бэкфилл агентов (случайная
// раса из factions.race_id), бэкфилл users.ship_icon; повторный прогон
// идемпотентен (значения не меняются).
func TestRaceShipsMigrationBackfill(t *testing.T) {
	s := openScratch(t)

	stmts := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, ship_icon TEXT NOT NULL DEFAULT 'ship_strela.svg')`,
		`CREATE TABLE npc_agents (id TEXT PRIMARY KEY)`,
		`CREATE TABLE factions (id TEXT PRIMARY KEY, race_id TEXT)`,
		`INSERT INTO factions (id, race_id) VALUES ('f1','humans'), ('f2','coastal')`,
		`INSERT INTO npc_agents (id) VALUES ('a1'), ('a2'), ('a3')`,
		`INSERT INTO users (id, ship_icon) VALUES
			('u1','ship_strela.svg'), ('u2','boomerang.png'), ('u3','race_humans_cruiser.png')`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	execRaceShipsMigration(t, s)

	// users.race_id NOT NULL DEFAULT 'humans' — существующие строки получили дефолт.
	if got := mustStr(t, s.db, `SELECT race_id FROM users WHERE id = 'u1'`); got != "humans" {
		t.Fatalf("users.race_id u1 = %q, ожидалось humans", got)
	}
	// Легаси SVG/PNG переписаны в людской корабль; расовый файл не тронут.
	for _, id := range []string{"u1", "u2"} {
		if got := mustStr(t, s.db, `SELECT ship_icon FROM users WHERE id = $1`, id); got != "race_humans_starship.png" {
			t.Fatalf("ship_icon %s = %q, ожидался race_humans_starship.png", id, got)
		}
	}
	if got := mustStr(t, s.db, `SELECT ship_icon FROM users WHERE id = 'u3'`); got != "race_humans_cruiser.png" {
		t.Fatalf("расовый ship_icon переписан: %q", got)
	}
	// DEFAULT users.ship_icon сменился на член реестра.
	var def string
	if err := s.db.QueryRow(`SELECT column_default FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'ship_icon'`).Scan(&def); err != nil {
		t.Fatalf("column_default: %v", err)
	}
	if !strings.Contains(def, "race_humans_starship.png") {
		t.Fatalf("DEFAULT users.ship_icon = %q", def)
	}
	// Агенты получили расу из пула factions.race_id.
	for _, id := range []string{"a1", "a2", "a3"} {
		got := mustStr(t, s.db, `SELECT COALESCE(race_id,'') FROM npc_agents WHERE id = $1`, id)
		if got != "humans" && got != "coastal" {
			t.Fatalf("agent %s race_id = %q, ожидалась раса из пула", id, got)
		}
	}

	// Идемпотентность: повторный прогон не меняет значения (WHERE-гварды).
	agentsBefore := mustStr(t, s.db, `SELECT string_agg(id || ':' || race_id, ',' ORDER BY id) FROM npc_agents`)
	iconsBefore := mustStr(t, s.db, `SELECT string_agg(id || ':' || ship_icon, ',' ORDER BY id) FROM users`)
	execRaceShipsMigration(t, s)
	if got := mustStr(t, s.db, `SELECT string_agg(id || ':' || race_id, ',' ORDER BY id) FROM npc_agents`); got != agentsBefore {
		t.Fatalf("повторный прогон изменил расы агентов: %q → %q", agentsBefore, got)
	}
	if got := mustStr(t, s.db, `SELECT string_agg(id || ':' || ship_icon, ',' ORDER BY id) FROM users`); got != iconsBefore {
		t.Fatalf("повторный прогон изменил ship_icon: %q → %q", iconsBefore, got)
	}
}

// TestRaceShipsMigrationEmptyPool — пул рас пуст (фракции не сгенерированы):
// агенты остаются с NULL-расой (нейтральный корабль), миграция не падает.
func TestRaceShipsMigrationEmptyPool(t *testing.T) {
	s := openScratch(t)

	stmts := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, ship_icon TEXT NOT NULL DEFAULT 'ship_strela.svg')`,
		`CREATE TABLE npc_agents (id TEXT PRIMARY KEY)`,
		`CREATE TABLE factions (id TEXT PRIMARY KEY, race_id TEXT)`,
		`INSERT INTO npc_agents (id) VALUES ('a1')`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	execRaceShipsMigration(t, s)

	// NULL-раса допустима (колонка nullable) — не падаем, агент нейтральный.
	if got := mustStr(t, s.db, `SELECT COALESCE(race_id, '<null>') FROM npc_agents WHERE id = 'a1'`); got != "<null>" {
		t.Fatalf("agent race_id = %q, ожидался NULL", got)
	}
}
