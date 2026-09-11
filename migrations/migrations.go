// Пакет migrations встраивает SQL-миграции в бинарник и применяет
// неприменённые при старте сервера.
//
// Названия файлов:   NNN_name.sql  или  NNN_name.up.sql / NNN_name.down.sql.
//  - версия извлекается из числового префикса NNN;
//  - .down.sql (откаты) автоматически НЕ применяются;
//  - каждая миграция выполняется в своей транзакции; в случае ошибки —
//    откат и запись в schema_migrations не происходит, повторная попытка
//    будет при следующем запуске.
package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var fs embed.FS

// Apply создаёт таблицу учёта schema_migrations и применяет все
// неприменённые миграции в порядке возрастания версии.
func Apply(db *sql.DB) error {
	tracked, err := relationExists(db, "schema_migrations")
	if err != nil {
		return err
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Учёта не было, но схема уже на месте — миграции накатывали вручную.
	// Отмечаем их применёнными, ничего не выполняя: 003, 006, 000009 и 000011
	// не идемпотентны и упали бы на существующих таблицах.
	if !tracked {
		legacy, err := relationExists(db, "worlds")
		if err != nil {
			return err
		}
		if legacy {
			return baseline(db)
		}
	}

	applied, err := readApplied(db)
	if err != nil {
		return err
	}

	pending, err := listPending(applied)
	if err != nil {
		return err
	}

	for _, m := range pending {
		if err := applyOne(db, m); err != nil {
			return err
		}
	}

	return nil
}

// relationExists проверяет наличие таблицы в схеме public.
func relationExists(db *sql.DB, name string) (bool, error) {
	var exists bool
	if err := db.QueryRow(
		`SELECT to_regclass('public.' || $1) IS NOT NULL`, name,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check relation %s: %w", name, err)
	}
	return exists, nil
}

// baseline отмечает все миграции применёнными, не выполняя их. Срабатывает
// один раз — когда база, которую накатывали вручную, впервые переходит
// на автоматический учёт.
func baseline(db *sql.DB) error {
	all, err := listPending(map[string]bool{})
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, m := range all {
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
			m.version, m.name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("baseline %s: %w", m.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("⚠️  Схема уже существовала, учёта миграций не было: %d миграций "+
		"отмечены применёнными БЕЗ выполнения. Сверь схему БД с файлами.", len(all))
	return nil
}

func readApplied(db *sql.DB) (map[string]bool, error) {
	applied := map[string]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

type migration struct {
	version string
	name    string
}

func listPending(applied map[string]bool) ([]migration, error) {
	entries, err := fs.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var pending []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".down.sql") {
			continue
		}
		version := versionFromName(name)
		if version == "" {
			continue
		}
		if applied[version] {
			continue
		}
		pending = append(pending, migration{version: version, name: name})
	}

	sort.Slice(pending, func(i, j int) bool {
		return numValue(pending[i].version) < numValue(pending[j].version)
	})
	return pending, nil
}

// versionFromName извлекает числовой префикс из имени файла.
func versionFromName(name string) string {
	base := strings.TrimSuffix(name, ".sql")
	base = strings.TrimSuffix(base, ".up")
	i := 0
	for i < len(base) && base[i] >= '0' && base[i] <= '9' {
		i++
	}
	return base[:i]
}

func numValue(v string) int {
	v = strings.TrimLeft(v, "0")
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func applyOne(db *sql.DB, m migration) error {
	content, err := fs.ReadFile(m.name)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		tx.Rollback()
		return err
	}

	// Пустые файлы (например, заглушки) — просто фиксируем версию.
	trimmed := strings.TrimSpace(string(content))
	if trimmed != "" {
		if _, err := tx.Exec(trimmed); err != nil {
			return rollback(fmt.Errorf("apply migration %s: %w", m.name, err))
		}
	}

	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		m.version, m.name,
	); err != nil {
		return rollback(fmt.Errorf("record migration %s: %w", m.name, err))
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("✅ Миграция применена: %s (v%s)", m.name, m.version)
	return nil
}