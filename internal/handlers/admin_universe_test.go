// Тесты на очистку вселенной (admin_universe.go).
package handlers

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// TestClearUniverseTx — точная последовательность SQL очистки вселенной:
// снятие current_world_id у users, снятие FK, TRUNCATE всех таблиц, возврат FK.
// Если truncateTables изменится, тест упадёт и заставит обновить ожидание.
func TestClearUniverseTx(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_world_id = NULL WHERE current_world_id IS NOT NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_current_world_id_fkey`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`TRUNCATE TABLE ` + truncateTables).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE users ADD CONSTRAINT users_current_world_id_fkey FOREIGN KEY (current_world_id) REFERENCES worlds(id) ON DELETE SET NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	require.NoError(t, clearUniverseTx(context.Background(), tx))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestTruncateTablesCoverMigrationFK — защита от регрессии вида:
// «миграция создала таблицу с FK на усекаемую, но её забыли добавить
// в truncateTables» (TRUNCATE падает с cannot truncate a table referenced...).
//
// Инвариант: каждая существующая таблица, созданная миграциями и ссылающаяся
// на усекаемую таблицу, обязана быть в truncateTables. users исключена
// намеренно — её FK снимается отдельно в clearUniverseTx.
// TestPlanetStatsInvalidate — защита механизма сброса кэша статистики:
// GeneratePlanets вызывает invalidatePlanetStats в начале генерации,
// чтобы вкладка статистики не показывала устаревшее в окне пересчёта.
func TestPlanetStatsInvalidate(t *testing.T) {
	h := &AdminHandlers{}

	h.setPlanetStats(&PlanetStats{TotalPlanets: 7})
	require.NotNil(t, h.planetStats, "кэш должен быть заполнен после setPlanetStats")

	h.invalidatePlanetStats()
	require.Nil(t, h.planetStats, "после invalidatePlanetStats кэш должен быть пуст")
}

// TestClearPlanets — B11: перед генерацией планет старые строки удаляются,
// возвращается их число. Если SQL-последовательность изменится — тест упадёт.
func TestClearPlanets(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT(*) FROM planets`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))
	mock.ExpectExec(`DELETE FROM planets`).
		WillReturnResult(sqlmock.NewResult(0, 42))

	h := &AdminHandlers{db: db}
	n, err := h.clearPlanets()
	require.NoError(t, err)
	require.Equal(t, 42, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTruncateTablesCoverMigrationFK(t *testing.T) {
	migs := readMigrations(t)

	existing := map[string]bool{}     // живущие на конец миграций таблицы
	refs := map[string][]string{}    // таблица → на кого ссылается

	reCreate := regexp.MustCompile(`(?i)CREATE TABLE (IF NOT EXISTS )?(\w+)`)
	reRef := regexp.MustCompile(`(?i)REFERENCES (\w+)`)
	reDrop := regexp.MustCompile(`(?i)DROP TABLE IF EXISTS (\w+)`)
	reAlterRef := regexp.MustCompile(`(?i)ALTER TABLE (\w+)[\s\S]*?REFERENCES (\w+)`)

	for _, m := range migs {
		b, err := os.ReadFile(filepath.Join("..", "..", "migrations", m.name))
		require.NoError(t, err)
		src := string(b)

		// CREATE TABLE: имя таблицы + REFERENCES внутри её блока.
		idx := reCreate.FindAllStringSubmatchIndex(src, -1)
		for ci, match := range idx {
			table := src[match[4]:match[5]] // группа 2 — имя таблицы
			existing[table] = true
			start := match[1]
			end := len(src)
			if ci+1 < len(idx) {
				end = idx[ci+1][0]
			}
			for _, r := range reRef.FindAllStringSubmatch(src[start:end], -1) {
				refs[table] = append(refs[table], r[1])
			}
		}

		// DROP TABLE (в т.ч. таблицы, позже удалённые миграциями).
		for _, d := range reDrop.FindAllStringSubmatch(src, -1) {
			delete(existing, d[1])
		}

		// ALTER TABLE ... ADD ... REFERENCES (например, users → worlds).
		for _, a := range reAlterRef.FindAllStringSubmatch(src, -1) {
			refs[a[1]] = append(refs[a[1]], a[2])
		}
	}

	truncated := map[string]bool{}
	for _, t := range strings.Split(truncateTables, ",") {
		truncated[strings.TrimSpace(t)] = true
	}

	// Замыкание: если усекается R, все существующие ссылки на R тоже усекаются.
	var missing []string
	for {
		added := false
		for table, targets := range refs {
			if table == "users" || truncated[table] || !existing[table] {
				continue
			}
			for _, target := range targets {
				if truncated[target] {
					missing = append(missing, table)
					truncated[table] = true
					added = true
					break
				}
			}
		}
		if !added {
			break
		}
	}

	require.Empty(t, missing,
		"таблицы с FK на усекаемые отсутствуют в truncateTables (см. admin_universe.go): %v", missing)
}

// readMigrations — SQL-файлы миграций в порядке версий (без .down.sql).
func readMigrations(t *testing.T) []struct {
	name string
} {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", "migrations"))
	require.NoError(t, err)

	type mig struct {
		version int
		name    string
	}
	var migs []mig
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}
		base := strings.TrimSuffix(name, ".sql")
		base = strings.TrimSuffix(base, ".up")
		i := 0
		for i < len(base) && base[i] >= '0' && base[i] <= '9' {
			i++
		}
		if i == 0 {
			continue
		}
		v, err := strconv.Atoi(strings.TrimLeft(base[:i], "0"))
		require.NoError(t, err)
		migs = append(migs, mig{version: v, name: name})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })

	out := make([]struct{ name string }, len(migs))
	for i, m := range migs {
		out[i] = struct{ name string }{name: m.name}
	}
	return out
}