// internal/repository/storage_cell_repository.go
//
// Слой доступа к ячейкам внутреннего хранилища (спека 2026-09-25-внутреннее-
// хранилище-и-рождение-заказов §4.1, ЧК2а): таблица settlement_storage_cells
// (полиморфный владелец settlement/building) и settlements.storage_size.
//
// Подэтап 2а — фундамент: базовые операции чтения и записи. Авто-создание
// ячеек по реестру нужд (§4.3), расчёт весов (`settlement.GoodWeights`) и
// порогов — подэтап 2б; здесь их нет.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/lib/pq"

	"zorion/internal/models"
)

// ErrStorageCellNotFound — ячейки владельца по товару нет. Инкремент строку
// молча НЕ создаёт: набор ячеек задаёт реестр нужд (§4.3, подэтап 2б), а
// самопроизвольная вставка обошла бы cap_share/веса и родила бы ячейку вне
// набора нужд.
var ErrStorageCellNotFound = errors.New("ячейка хранилища не найдена")

// ErrSettlementNotFound — поселения нет (чтение/запись storage_size не no-op).
var ErrSettlementNotFound = errors.New("поселение не найдено")

const (
	// StorageOwnerSettlement / StorageOwnerBuilding — значения owner_type
	// ячейки (CHECK схемы, §4.1).
	StorageOwnerSettlement = "settlement"
	StorageOwnerBuilding   = "building"

	storageCellSelectSQL = `
		SELECT id, owner_type, owner_id, good_id, amount, cap_share
		FROM settlement_storage_cells
		WHERE owner_type = $1 AND owner_id = $2
		ORDER BY good_id`

	// storageCellIncrementSQL — относительный инкремент (delta может быть
	// отрицательным — списание) с клампом ≥ 0: страховка от гонки/ошибки, CHECK
	// amount >= 0 не должен падать. updated_at двигается только при изменении
	// строки.
	storageCellIncrementSQL = `
		UPDATE settlement_storage_cells
		SET amount = GREATEST(0, amount + $3), updated_at = NOW()
		WHERE owner_type = $1 AND owner_id = $2 AND good_id = $4`

	// storageCellsClearSQL — обнуление количеств всех ячеек владельца (переход
	// ступени, §5.6); строки сохраняются (остаток — реальный запас, §4.3).
	storageCellsClearSQL = `
		UPDATE settlement_storage_cells
		SET amount = 0, updated_at = NOW()
		WHERE owner_type = $1 AND owner_id = $2`

	// storageCellsDeleteSQL — удаление всех ячеек владельца (очистка/каскад
	// удаления поселения: FK на settlements нет, §4.1).
	storageCellsDeleteSQL = `
		DELETE FROM settlement_storage_cells
		WHERE owner_type = $1 AND owner_id = $2`

	// storageCellUpsertSQL — создание/обновление ячейки под потребность
	// (реестр нужд §4.3): нет строки → amount = 0, cap_share = вес; есть →
	// обновляется только вес (остаток сохраняется).
	storageCellUpsertSQL = `
		INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount, cap_share)
		VALUES ($1, $2, $3, 0, $4)
		ON CONFLICT (owner_type, owner_id, good_id)
		DO UPDATE SET cap_share = EXCLUDED.cap_share, updated_at = NOW()`

	// storageCellsDeleteEmptyOutsideSQL — удаление пустых ячеек вне набора нужд
	// (§4.3): ячейка с остатком сохраняется (реальный запас), пустая — удаляется.
	storageCellsDeleteEmptyOutsideSQL = `
		DELETE FROM settlement_storage_cells
		WHERE owner_type = $1 AND owner_id = $2 AND amount = 0
		  AND good_id <> ALL (COALESCE($3::bigint[], '{}'::bigint[]))`

	settlementStorageSizeSelectSQL = `SELECT storage_size FROM settlements WHERE id = $1`

	settlementStorageSizeUpdateSQL = `
		UPDATE settlements SET storage_size = $2, updated_at = NOW() WHERE id = $1`
)

// storageCellRowsQueryer — общее для *sql.DB и *sql.Tx (чтение ячеек).
type storageCellRowsQueryer interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// StorageCellRepository — доступ к ячейкам внутреннего хранилища и размеру.
type StorageCellRepository struct {
	db *sql.DB
}

func NewStorageCellRepository(db *sql.DB) *StorageCellRepository {
	return &StorageCellRepository{db: db}
}

// --- чтение ---

// GetStorageCells — ячейки владельца в детерминированном порядке good_id.
func (r *StorageCellRepository) GetStorageCells(ownerType, ownerID string) ([]models.StorageCell, error) {
	return queryStorageCells(context.Background(), r.db, ownerType, ownerID)
}

// GetStorageCellsTx — то же внутри транзакции (owner-проход §7.1, подэтап 2б).
func (r *StorageCellRepository) GetStorageCellsTx(ctx context.Context, tx *sql.Tx, ownerType, ownerID string) ([]models.StorageCell, error) {
	return queryStorageCells(ctx, tx, ownerType, ownerID)
}

// queryStorageCells — единый SQL чтения ячеек владельца.
func queryStorageCells(ctx context.Context, q storageCellRowsQueryer, ownerType, ownerID string) ([]models.StorageCell, error) {
	rows, err := q.QueryContext(ctx, storageCellSelectSQL, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query storage cells: %w", err)
	}
	defer rows.Close()
	var out []models.StorageCell
	for rows.Next() {
		var c models.StorageCell
		if err := rows.Scan(&c.ID, &c.OwnerType, &c.OwnerID, &c.GoodID, &c.Amount, &c.CapShare); err != nil {
			return nil, fmt.Errorf("failed to scan storage cell: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage cells iteration error: %w", err)
	}
	return out, nil
}

// --- запись ---

// IncrementStorageCellTx — атомарный инкремент ячейки: amount += delta (delta
// может быть отрицательным — списание), результат клампится ≥ 0. Строки нет —
// ErrStorageCellNotFound (молчаливого создания нет, см. выше).
func (r *StorageCellRepository) IncrementStorageCellTx(ctx context.Context, tx *sql.Tx, ownerType, ownerID string, goodID int64, delta float64) error {
	res, err := tx.ExecContext(ctx, storageCellIncrementSQL, ownerType, ownerID, delta, goodID)
	if err != nil {
		return fmt.Errorf("failed to increment storage cell: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read increment result: %w", err)
	}
	if n == 0 {
		return ErrStorageCellNotFound
	}
	return nil
}

// EnsureStorageCellTx — создание ячейки владельца по товару, если её нет
// (amount = 0, cap_share = 0; вес добьёт owner-проход). Идемпотентна.
func (r *StorageCellRepository) EnsureStorageCellTx(ctx context.Context, tx *sql.Tx, ownerType, ownerID string, goodID int64) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount, cap_share)
		VALUES ($1, $2, $3, 0, 0) ON CONFLICT (owner_type, owner_id, good_id) DO NOTHING`,
		ownerType, ownerID, goodID,
	); err != nil {
		return fmt.Errorf("failed to ensure storage cell: %w", err)
	}
	return nil
}

// EnsureStorageCellsTx — сверка набора ячеек с реестром нужд (§4.3): под каждый
// товар набора создаётся ячейка (amount = 0, cap_share = вес) или обновляется
// вес; пустые ячейки вне набора удаляются (ячейка с остатком сохраняется —
// реальный запас). Веса — из settlement.GoodWeights (F3). Идемпотентна.
func (r *StorageCellRepository) EnsureStorageCellsTx(ctx context.Context, tx *sql.Tx, ownerType, ownerID string, weights map[int64]float64) error {
	goodIDs := make([]int64, 0, len(weights))
	for gid := range weights {
		goodIDs = append(goodIDs, gid)
	}
	sort.Slice(goodIDs, func(i, j int) bool { return goodIDs[i] < goodIDs[j] })
	for _, gid := range goodIDs {
		if _, err := tx.ExecContext(ctx, storageCellUpsertSQL, ownerType, ownerID, gid, weights[gid]); err != nil {
			return fmt.Errorf("failed to ensure storage cell: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, storageCellsDeleteEmptyOutsideSQL, ownerType, ownerID, pq.Array(goodIDs)); err != nil {
		return fmt.Errorf("failed to prune empty storage cells: %w", err)
	}
	return nil
}

// ClearStorageCellsTx — обнуление количества всех ячеек владельца (переход
// ступени, §5.6). Ячеек нет — no-op без ошибки.
func (r *StorageCellRepository) ClearStorageCellsTx(ctx context.Context, tx *sql.Tx, ownerType, ownerID string) error {
	if _, err := tx.ExecContext(ctx, storageCellsClearSQL, ownerType, ownerID); err != nil {
		return fmt.Errorf("failed to clear storage cells: %w", err)
	}
	return nil
}

// DeleteStorageCellsTx — удаление всех ячеек владельца (очистка/каскад). Ячеек
// нет — no-op без ошибки.
func (r *StorageCellRepository) DeleteStorageCellsTx(ctx context.Context, tx *sql.Tx, ownerType, ownerID string) error {
	if _, err := tx.ExecContext(ctx, storageCellsDeleteSQL, ownerType, ownerID); err != nil {
		return fmt.Errorf("failed to delete storage cells: %w", err)
	}
	return nil
}

// --- размер хранилища поселения ---

// GetSettlementStorageSize — размер хранилища поселения. Поселения нет —
// ErrSettlementNotFound.
func (r *StorageCellRepository) GetSettlementStorageSize(settlementID string) (float64, error) {
	var size float64
	err := r.db.QueryRow(settlementStorageSizeSelectSQL, settlementID).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSettlementNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read storage size: %w", err)
	}
	return size, nil
}

// SetSettlementStorageSizeTx — запись размера хранилища поселения (owner-проход
// §1.3). Поселения нет — ErrSettlementNotFound (запись не no-op).
func (r *StorageCellRepository) SetSettlementStorageSizeTx(ctx context.Context, tx *sql.Tx, settlementID string, size float64) error {
	res, err := tx.ExecContext(ctx, settlementStorageSizeUpdateSQL, settlementID, size)
	if err != nil {
		return fmt.Errorf("failed to write storage size: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read storage size result: %w", err)
	}
	if n == 0 {
		return ErrSettlementNotFound
	}
	return nil
}
