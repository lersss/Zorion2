// Тесты на очистку вселенной (admin_universe.go).
package handlers

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// TestWorldInsertValuesStellarModsValidJSONB — баг #1 (прогон @tester):
// []byte(nil) для stellar_mods lib/pq передаёт как ” → "invalid input syntax
// for type json", генерация вселенной падает. Пустые модификаторы обязаны
// давать валидный JSONB "{}".
func TestWorldInsertValuesStellarModsValidJSONB(t *testing.T) {
	// Обычная одиночная звезда (StellarMods == nil, 85%+ миров).
	spectral, modsJSON, mass, age := worldInsertValues(&models.World{SpectralClass: "G", StarType: "star"})
	require.Equal(t, "G", spectral)
	require.Equal(t, []byte("{}"), modsJSON, "nil-модификаторы → валидный JSONB {}, не nil")
	require.Nil(t, mass, "масса не задана → NULL")
	require.Nil(t, age, "возраст не задан → NULL (41a: обычные звёзды)")

	// Экзотика без модификаторов: спектр NULL, моды — "{}".
	spectral, modsJSON, mass, age = worldInsertValues(&models.World{SpectralClass: "", StarType: "black_hole"})
	require.Nil(t, spectral)
	require.Equal(t, []byte("{}"), modsJSON)
	require.Nil(t, mass)
	require.Nil(t, age)

	// С модификаторами — маршалл проходит; с массой — значение, не NULL.
	massVal := 30.0
	spectral, modsJSON, mass, age = worldInsertValues(&models.World{
		SpectralClass: "O",
		StellarMods:   &models.StellarMods{Phase: "I", Subtype: "lbv"},
		StellarMass:   &massVal,
	})
	require.Equal(t, "O", spectral)
	require.Contains(t, string(modsJSON), `"lbv"`)
	require.NotEqual(t, []byte("{}"), modsJSON)
	require.Equal(t, 30.0, mass, "масса проходит в INSERT")
	require.Nil(t, age)
}

// TestWorldInsertValuesAge — возраст мира в INSERT (41a §3.3): значение при
// заполненном Age, NULL при nil (старые миры/обычные звёзды).
func TestWorldInsertValuesAge(t *testing.T) {
	ageVal := 4.2
	_, _, _, age := worldInsertValues(&models.World{SpectralClass: "", StarType: "black_hole", Age: &ageVal})
	require.Equal(t, 4.2, age, "возраст проходит в INSERT")

	_, _, _, age = worldInsertValues(&models.World{SpectralClass: "", StarType: "black_hole"})
	require.Nil(t, age, "nil-возраст → NULL в INSERT")
}

// TestClearUniverseTx — точная последовательность SQL очистки вселенной:
// снятие current_world_id у users, снятие FK, TRUNCATE всех таблиц, возврат FK.
// Если truncateTables изменится, тест упадёт и заставит обновить ожидание.
func TestClearUniverseTx(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	// §6.5: возврат залога живых контрактов ДО TRUNCATE (пустой набор в тесте).
	mock.ExpectQuery(`UPDATE contracts\s+SET status = CASE WHEN executor_id IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id", "escrow_amount", "escrow_withdrawable"}))
	mock.ExpectExec(`UPDATE users SET current_world_id = NULL WHERE current_world_id IS NOT NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_current_world_id_fkey`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`TRUNCATE TABLE ` + truncateTables).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// §3.5: счета фракций и агентов удаляются, кошелёк игрока остаётся.
	mock.ExpectExec(`DELETE FROM accounts WHERE owner_type IN \('faction', 'agent'\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE users ADD CONSTRAINT users_current_world_id_fkey FOREIGN KEY (current_world_id) REFERENCES worlds(id) ON DELETE SET NULL`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	require.NoError(t, clearUniverseTx(context.Background(), tx))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestClearUniverseRetriesOnDeadlock — дедлок на TRUNCATE (40P01) не роняет
// очистку: первая попытка убита Postgres (TRUNCATE × фоновый NPC-тик), вторая
// идёт со свежим BeginTx и доходит до Commit. Итог — успех без ошибки.
func TestClearUniverseRetriesOnDeadlock(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Попытка 1: TRUNCATE падает дедлоком, транзакция откатывается.
	expectClearUniversePrefix(mock)
	mock.ExpectExec(`TRUNCATE TABLE ` + truncateTables).
		WillReturnError(&pq.Error{Code: "40P01", Message: "deadlock detected"})
	mock.ExpectRollback()

	// Попытка 2: свежая транзакция проходит целиком.
	expectClearUniversePrefix(mock)
	mock.ExpectExec(`TRUNCATE TABLE ` + truncateTables).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM accounts WHERE owner_type IN \('faction', 'agent'\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE users ADD CONSTRAINT users_current_world_id_fkey FOREIGN KEY (current_world_id) REFERENCES worlds(id) ON DELETE SET NULL`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	require.NoError(t, clearUniverseWithRetry(context.Background(), db, clearUniverseAttempts, time.Millisecond))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestClearUniverseRetryExhausted — все попытки в дедлоке: функция
// возвращает ошибку 40P01 (без паники), транзакции откатываются.
func TestClearUniverseRetryExhausted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	for i := 0; i < 2; i++ {
		expectClearUniversePrefix(mock)
		mock.ExpectExec(`TRUNCATE TABLE ` + truncateTables).
			WillReturnError(&pq.Error{Code: "40P01", Message: "deadlock detected"})
		mock.ExpectRollback()
	}

	err = clearUniverseWithRetry(context.Background(), db, 2, time.Millisecond)
	require.Error(t, err)
	require.True(t, isDeadlockErr(err), "исчерпание попыток отдаёт исходный дедлок: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestClearUniverseNoRetryOnOtherError — не-дедлок не ретраится: одна
// транзакция, ошибка наружу (защита от лишних попыток).
func TestClearUniverseNoRetryOnOtherError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectClearUniversePrefix(mock)
	mock.ExpectExec(`TRUNCATE TABLE ` + truncateTables).
		WillReturnError(&pq.Error{Code: "23503", Message: "foreign key violation"})
	mock.ExpectRollback()

	err = clearUniverseWithRetry(context.Background(), db, clearUniverseAttempts, time.Millisecond)
	require.Error(t, err)
	require.False(t, isDeadlockErr(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

// expectClearUniversePrefix — шаги попытки очистки до TRUNCATE (Begin,
// возврат залога, снятие current_world_id, снятие FK).
func expectClearUniversePrefix(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE contracts\s+SET status = CASE WHEN executor_id IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id", "escrow_amount", "escrow_withdrawable"}))
	mock.ExpectExec(`UPDATE users SET current_world_id = NULL WHERE current_world_id IS NOT NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_current_world_id_fkey`).
		WillReturnResult(sqlmock.NewResult(0, 0))
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
// возвращается их число. Пояса малых тел (system_belts) удаляются в той же
// точке (спека поясов §4.6/§4.7): GeneratePlanets перегенерирует все миры,
// иначе повторный прогон дублирует пояса. Если SQL-последовательность
// изменится — тест упадёт.
func TestClearPlanets(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	// §6.5: возврат залога живых контрактов в той же транзакции, что DELETE.
	mock.ExpectQuery(`UPDATE contracts\s+SET status = CASE WHEN executor_id IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id", "escrow_amount", "escrow_withdrawable"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM planets`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))
	mock.ExpectExec(`DELETE FROM system_belts`).
		WillReturnResult(sqlmock.NewResult(0, 7))
	// Ячейки хранилища поселений (ЧК2а §4.1): owner_id без FK — явный DELETE.
	mock.ExpectExec(`DELETE FROM settlement_storage_cells WHERE owner_type = 'settlement'`).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec(`DELETE FROM planets`).
		WillReturnResult(sqlmock.NewResult(0, 42))
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	n, err := h.clearPlanets()
	require.NoError(t, err)
	require.Equal(t, 42, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAssignCurrentWorldsTx — B27: автоназначение текущего мира после
// генерации. Skycomposer без мира получает ближайший к центру мир; UPDATE
// ограничен ролью skycomposer и IS NULL (player и уже заданные миры не
// трогаются — это условие WHERE). Пустая вселенная — no-op без UPDATE.
func TestAssignCurrentWorldsTx(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	// 1. Миры есть: ближайший к (0,0) найден, назначен skycomposer-ам без мира.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM worlds ORDER BY (coord_x * coord_x + coord_y * coord_y) LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("11111111-1111-1111-1111-111111111111"))
	mock.ExpectExec(`UPDATE users SET current_world_id = $1 WHERE role = 'skycomposer' AND current_world_id IS NULL`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, assignCurrentWorldsTx(context.Background(), tx))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())

	// 2. Миров нет — ничего не назначаем (UPDATE не выполняется).
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM worlds ORDER BY (coord_x * coord_x + coord_y * coord_y) LIMIT 1`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	tx2, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, assignCurrentWorldsTx(context.Background(), tx2))
	require.NoError(t, tx2.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTruncateTablesCoverMigrationFK(t *testing.T) {
	migs := readMigrations(t)

	existing := map[string]bool{} // живущие на конец миграций таблицы
	refs := map[string][]string{} // таблица → на кого ссылается

	reCreate := regexp.MustCompile(`(?i)CREATE TABLE (IF NOT EXISTS )?(\w+)`)
	reRef := regexp.MustCompile(`(?i)REFERENCES (\w+)`)
	reDrop := regexp.MustCompile(`(?i)DROP TABLE IF EXISTS (\w+)`)
	reAlterRef := regexp.MustCompile(`(?i)ALTER TABLE (\w+)[\s\S]*?REFERENCES (\w+)`)
	reRename := regexp.MustCompile(`(?i)ALTER TABLE (\w+) RENAME TO (\w+)`)

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

		// ALTER TABLE ... RENAME TO ... (ретайр таблицы): переносим имя и ссылки.
		for _, r := range reRename.FindAllStringSubmatch(src, -1) {
			old, renamed := r[1], r[2]
			if existing[old] {
				delete(existing, old)
				existing[renamed] = true
			}
			if rs, ok := refs[old]; ok {
				delete(refs, old)
				refs[renamed] = rs
			}
		}
	}

	// Ретаиренные таблицы (RENAME ... _retired) выведены из обращения: FK сняты
	// в той же миграции, но статический разбор этого не видит — исключаем их.
	for t := range existing {
		if strings.HasSuffix(t, "_retired") {
			delete(existing, t)
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
