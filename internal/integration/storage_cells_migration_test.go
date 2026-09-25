// internal/integration/storage_cells_migration_test.go
// Проверки миграции 000086 «внутреннее хранилище» (спека 2026-09-25-внутреннее-
// хранилище-и-рождение-заказов §4.1, ЧК2а): колонка settlements.storage_size,
// таблица settlement_storage_cells с CHECK/UNIQUE/FK CASCADE/индексами,
// идемпотентность; плюс прогон слоя доступа repository.StorageCellRepository по
// живой схеме (колонки/имена SQL согласованы). Живой PostgreSQL в изолированной
// scratch-схеме (helpers_test.go); без TEST_DATABASE_URL/DATABASE_URL — skip.
//
// T1 покрыт в части СХЕМЫ и идемпотентности. Перенос данных из
// settlement_branch_buffers в ячейки идёт подэтапом 2б (в 2а буферы остаются
// рабочим источником), поэтому data-агрегация здесь не проверяется.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
)

// migration086SQL — содержимое миграции (для повторного прогона в тесте).
func migration086SQL(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000086_settlement_storage_cells.sql"))
	require.NoErrorf(t, err, "миграция 000086_settlement_storage_cells.sql")
	return string(src)
}

// migration087SQL — содержимое миграции ретайра буферов (для повторного прогона).
func migration087SQL(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000087_retire_branch_buffers.sql"))
	require.NoErrorf(t, err, "миграция 000087_retire_branch_buffers.sql")
	return string(src)
}

// TestMigration087RetiresBranchBuffers — буферы ветки выведены из обращения:
// живой таблицы нет, ретайренная есть; повторный прогон 000087 — no-op.
func TestMigration087RetiresBranchBuffers(t *testing.T) {
	s := openMigrated(t)

	require.Equal(t, int64(0), mustInt(t, s.db, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'settlement_branch_buffers'`),
		"живая таблица буферов должна быть ретайрена")
	require.Equal(t, int64(1), mustInt(t, s.db, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'settlement_branch_buffers_retired'`),
		"ретайренная таблица сохранена (данные для отката)")

	// Повторный прогон — no-op без ошибки (идемпотентность).
	if _, err := s.db.Exec(migration087SQL(t)); err != nil {
		t.Fatalf("повторный прогон 000087: %v", err)
	}
}

// insertTestGood — категория + товар во scratch-схеме (каталог Go-сидом не
// наполнен), id товара.
func insertTestGood(t *testing.T, s *scratchDB, nameNorm string) int64 {
	t.Helper()
	catID := mustInt(t, s.db, `INSERT INTO categories (name, name_norm, kind)
		VALUES ($1, $1, 'good')
		ON CONFLICT (kind, name_norm) DO UPDATE SET name = EXCLUDED.name RETURNING id`, "кат-"+nameNorm)
	return mustInt(t, s.db, `INSERT INTO goods (name, name_norm, category_id, kind, source)
		VALUES ('Тест-товар',$1,$2,'good','manual') RETURNING id`, nameNorm, catID)
}

// insertTestSettlement — мир/планета/поселение, id поселения.
func insertTestSettlement(t *testing.T, s *scratchDB) string {
	t.Helper()
	const worldID = "11111111-1111-1111-1111-111111111111"
	if _, err := s.db.Exec(`INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, 'W', 0, 0)`, worldID); err != nil {
		t.Fatalf("world: %v", err)
	}
	planetID := mustStr(t, s.db, `INSERT INTO planets (id, world_id, name, orbit_index, data)
		VALUES (gen_random_uuid(), $1, 'P', 0, '{}') RETURNING id`, worldID)
	return mustStr(t, s.db, `INSERT INTO settlements (id, planet_id, population, population_exact, stability, computed_at)
		VALUES (gen_random_uuid(), $1, 10, 10, 50, NOW()) RETURNING id`, planetID)
}

// TestMigration086SettlementStorageCells — схема на месте, CHECK/UNIQUE/FK
// работают, повторный прогон идемпотентен.
func TestMigration086SettlementStorageCells(t *testing.T) {
	s := openMigrated(t)
	sql086 := migration086SQL(t)

	// Повторный прогон — no-op без ошибки (идемпотентность).
	if _, err := s.db.Exec(sql086); err != nil {
		t.Fatalf("повторный прогон 000086: %v", err)
	}

	// Колонка settlements.storage_size.
	require.Equal(t, int64(1), mustInt(t, s.db, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'settlements' AND column_name = 'storage_size'`))

	// Таблица и её колонки.
	require.Equal(t, int64(1), mustInt(t, s.db, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'settlement_storage_cells'`))
	for _, col := range []string{"id", "owner_type", "owner_id", "good_id", "amount", "cap_share", "created_at", "updated_at"} {
		require.Equalf(t, int64(1), mustInt(t, s.db, `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'settlement_storage_cells' AND column_name = $1`, col),
			"колонка settlement_storage_cells.%s", col)
	}

	// Оба индекса.
	require.Equal(t, int64(2), mustInt(t, s.db, `SELECT COUNT(*) FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname IN ('idx_storage_cells_owner','idx_storage_cells_good')`))

	settlementID := insertTestSettlement(t, s)
	goodID := insertTestGood(t, s, "тест-товар-1")
	goodID2 := insertTestGood(t, s, "тест-товар-2")

	// storage_size по умолчанию 0; CHECK >= 0.
	require.Equal(t, int64(1), mustInt(t, s.db,
		`SELECT COUNT(*) FROM settlements WHERE id = $1 AND storage_size = 0`, settlementID))
	if _, err := s.db.Exec(`UPDATE settlements SET storage_size = -1 WHERE id = $1`, settlementID); err == nil {
		t.Fatal("CHECK storage_size >= 0 не сработал")
	}

	// owner_type — домен {settlement, building}.
	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id)
		VALUES ('npc', $1, $2)`, settlementID, goodID); err == nil {
		t.Fatal("CHECK owner_type не сработал")
	}
	// amount / cap_share неотрицательны.
	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount)
		VALUES ('settlement', $1, $2, -1)`, settlementID, goodID); err == nil {
		t.Fatal("CHECK amount >= 0 не сработал")
	}
	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, cap_share)
		VALUES ('settlement', $1, $2, -1)`, settlementID, goodID); err == nil {
		t.Fatal("CHECK cap_share >= 0 не сработал")
	}

	// Уникальность (owner_type, owner_id, good_id).
	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id)
		VALUES ('settlement', $1, $2)`, settlementID, goodID); err != nil {
		t.Fatalf("первая ячейка: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id)
		VALUES ('settlement', $1, $2)`, settlementID, goodID); err == nil {
		t.Fatal("UNIQUE (owner_type, owner_id, good_id) не сработал")
	}
	// Другой товар у того же владельца — можно.
	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id)
		VALUES ('settlement', $1, $2)`, settlementID, goodID2); err != nil {
		t.Fatalf("вторая ячейка другого товара: %v", err)
	}

	// FK good_id → goods ON DELETE CASCADE: удаление товара уносит ячейку.
	if _, err := s.db.Exec(`DELETE FROM goods WHERE id = $1`, goodID); err != nil {
		t.Fatalf("delete good: %v", err)
	}
	require.Equal(t, int64(0), mustInt(t, s.db,
		`SELECT COUNT(*) FROM settlement_storage_cells WHERE good_id = $1`, goodID))

	// Повторный прогон после данных — тоже no-op.
	if _, err := s.db.Exec(sql086); err != nil {
		t.Fatalf("повторный прогон 000086 после вставки: %v", err)
	}
}

// TestStorageCellsCascadeOnWorldDelete — T14: путь удаления мира (как
// clearWorldReferencesForWorlds + DELETE worlds) не оставляет сирот-ячеек:
// owner_id полиморфный, FK на settlements нет — ячейки чистятся явно.
func TestStorageCellsCascadeOnWorldDelete(t *testing.T) {
	s := openMigrated(t)
	const worldID = "11111111-1111-1111-1111-111111111111"
	settlementID := insertTestSettlement(t, s)
	goodID := insertTestGood(t, s, "каскад-товар")

	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount)
		VALUES ('settlement', $1, $2, 5)`, settlementID, goodID); err != nil {
		t.Fatalf("ячейка: %v", err)
	}

	// Путь удаления мира: явная чистка ячеек поселений мира, затем DELETE worlds
	// (поселения уходят FK-каскадом).
	if _, err := s.db.Exec(`
		DELETE FROM settlement_storage_cells
		WHERE owner_type = 'settlement'
		  AND owner_id IN (
			SELECT s.id FROM settlements s
			JOIN planets p ON p.id = s.planet_id
			WHERE p.world_id = $1)`, worldID); err != nil {
		t.Fatalf("delete cells: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM worlds WHERE id = $1`, worldID); err != nil {
		t.Fatalf("delete world: %v", err)
	}

	require.Equal(t, int64(0), mustInt(t, s.db,
		`SELECT COUNT(*) FROM settlement_storage_cells WHERE owner_id = $1`, settlementID),
		"после удаления мира ячеек поселения не осталось")
	require.Equal(t, int64(0), mustInt(t, s.db,
		`SELECT COUNT(*) FROM settlements WHERE id = $1`, settlementID),
		"поселение ушло каскадом")
}

// TestStorageCellsRepositoryAgainstSchema — SQL репозитория согласован со
// схемой: чтение/инкремент с клампом/обнуление/удаление и storage_size
// выполняются на живой таблице.
func TestStorageCellsRepositoryAgainstSchema(t *testing.T) {
	s := openMigrated(t)
	repo := repository.NewStorageCellRepository(s.db)
	ctx := context.Background()

	settlementID := insertTestSettlement(t, s)
	goodID := insertTestGood(t, s, "тест-репозиторий-товар")

	// storage_size: по умолчанию 0, запись/чтение.
	size, err := repo.GetSettlementStorageSize(settlementID)
	require.NoError(t, err)
	require.Equal(t, 0.0, size)

	tx, err := s.db.Begin()
	require.NoError(t, err)
	require.NoError(t, repo.SetSettlementStorageSizeTx(ctx, tx, settlementID, 1000))
	require.NoError(t, tx.Commit())
	size, err = repo.GetSettlementStorageSize(settlementID)
	require.NoError(t, err)
	require.Equal(t, 1000.0, size)

	// CHECK storage_size >= 0 ловится на живой БД.
	tx, err = s.db.Begin()
	require.NoError(t, err)
	require.Error(t, repo.SetSettlementStorageSizeTx(ctx, tx, settlementID, -1))
	require.NoError(t, tx.Rollback())

	// Ячейка под поселение.
	if _, err := s.db.Exec(`INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount, cap_share)
		VALUES ('settlement', $1, $2, 12.5, 0.4)`, settlementID, goodID); err != nil {
		t.Fatalf("ячейка: %v", err)
	}
	cells, err := repo.GetStorageCells(repository.StorageOwnerSettlement, settlementID)
	require.NoError(t, err)
	require.Len(t, cells, 1)
	require.Equal(t, goodID, cells[0].GoodID)
	require.Equal(t, 12.5, cells[0].Amount)
	require.Equal(t, 0.4, cells[0].CapShare)

	// Инкремент отрицательный — кламп ≥ 0 (живой GREATEST).
	tx, err = s.db.Begin()
	require.NoError(t, err)
	require.NoError(t, repo.IncrementStorageCellTx(ctx, tx, repository.StorageOwnerSettlement, settlementID, goodID, -100))
	require.NoError(t, tx.Commit())
	cells, err = repo.GetStorageCells(repository.StorageOwnerSettlement, settlementID)
	require.NoError(t, err)
	require.Equal(t, 0.0, cells[0].Amount, "amount не должен уходить ниже 0")

	// Инкремент положительный.
	tx, err = s.db.Begin()
	require.NoError(t, err)
	require.NoError(t, repo.IncrementStorageCellTx(ctx, tx, repository.StorageOwnerSettlement, settlementID, goodID, 5))
	require.NoError(t, tx.Commit())
	cells, err = repo.GetStorageCells(repository.StorageOwnerSettlement, settlementID)
	require.NoError(t, err)
	require.Equal(t, 5.0, cells[0].Amount)

	// Инкремент несуществующей ячейки — ошибка, строка НЕ создаётся.
	tx, err = s.db.Begin()
	require.NoError(t, err)
	require.ErrorIs(t, repo.IncrementStorageCellTx(ctx, tx, repository.StorageOwnerSettlement, settlementID, goodID+1, 1), repository.ErrStorageCellNotFound)
	require.NoError(t, tx.Rollback())

	// Обнуление сохраняет строку.
	tx, err = s.db.Begin()
	require.NoError(t, err)
	require.NoError(t, repo.ClearStorageCellsTx(ctx, tx, repository.StorageOwnerSettlement, settlementID))
	require.NoError(t, tx.Commit())
	cells, err = repo.GetStorageCells(repository.StorageOwnerSettlement, settlementID)
	require.NoError(t, err)
	require.Len(t, cells, 1)
	require.Equal(t, 0.0, cells[0].Amount)

	// Удаление убирает все ячейки владельца.
	tx, err = s.db.Begin()
	require.NoError(t, err)
	require.NoError(t, repo.DeleteStorageCellsTx(ctx, tx, repository.StorageOwnerSettlement, settlementID))
	require.NoError(t, tx.Commit())
	cells, err = repo.GetStorageCells(repository.StorageOwnerSettlement, settlementID)
	require.NoError(t, err)
	require.Empty(t, cells)

	// Несуществующее поселение: чтение/запись — sentinel-ошибка, не no-op.
	const missing = "99999999-9999-9999-9999-999999999999"
	_, err = repo.GetSettlementStorageSize(missing)
	require.ErrorIs(t, err, repository.ErrSettlementNotFound)

	tx, err = s.db.Begin()
	require.NoError(t, err)
	require.ErrorIs(t, repo.SetSettlementStorageSizeTx(ctx, tx, missing, 1), repository.ErrSettlementNotFound)
	require.NoError(t, tx.Rollback())
}
