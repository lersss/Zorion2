// Package integration — интеграционные тесты на НАСТОЯЩЕЙ PostgreSQL
// (гигиена P1, ЧП4, 2026-09-22). Мок не знает про CHECK/FK/блокировки/гонки
// транзакций — здесь атомарные переходы контрактов и эскроу проверяются на
// живой БД.
//
// Каждый тест работает в СВОЕЙ случайной схеме (`zorion_it_<rand>`): рабочая
// схема public не затрагивается. Миграции накатываются migrations.Apply,
// в конце схема удаляется (DROP SCHEMA ... CASCADE).
//
// search_path задаётся через options DSN, а не разовым SET: database/sql
// выдаёт из пула произвольное соединение, и SET применился бы лишь к одному.
//
// Без TEST_DATABASE_URL/DATABASE_URL или без доступной БД тесты скипаются —
// обычный `go test ./...` остаётся зелёным.
package integration

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"zorion/migrations"
)

// scratchDB — изолированная схема для одного теста.
type scratchDB struct {
	schema string
	db     *sql.DB // пул соединений с search_path=<schema>
	admin  *sql.DB // пул с дефолтным search_path (CREATE/DROP SCHEMA)
}

// openScratch создаёт случайную схему и возвращает соединение с ней.
// Схема удаляется в t.Cleanup. БД недоступна — t.Skip.
func openScratch(t *testing.T) *scratchDB {
	t.Helper()

	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		base = os.Getenv("DATABASE_URL")
	}
	if base == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL не задан — интеграционные тесты на БД пропущены")
	}

	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Skipf("не удалось открыть соединение с БД: %v", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		t.Skipf("БД недоступна (%v) — интеграционные тесты на БД пропущены", err)
	}

	schema := fmt.Sprintf("zorion_it_%d", rand.Int63())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}

	dsn, err := scratchDSN(base, schema)
	if err != nil {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		t.Fatalf("scratch DSN: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		t.Skipf("не удалось открыть соединение со scratch-схемой: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		admin.Close()
	})

	return &scratchDB{schema: schema, db: db, admin: admin}
}

// openMigrated — scratch-схема с накатанными миграциями.
func openMigrated(t *testing.T) *scratchDB {
	t.Helper()
	s := openScratch(t)
	if err := migrations.Apply(s.db); err != nil {
		t.Fatalf("migrations.Apply: %v", err)
	}
	return s
}

// scratchDSN добавляет к DSN options=-csearch_path=<schema> (на каждое
// соединение пула, а не разовым SET).
func scratchDSN(base, schema string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// mustInt — одиночное целое из запроса.
func mustInt(t *testing.T, db *sql.DB, query string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

// mustStr — одиночная строка из запроса.
func mustStr(t *testing.T, db *sql.DB, query string, args ...interface{}) string {
	t.Helper()
	var s string
	if err := db.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return s
}
