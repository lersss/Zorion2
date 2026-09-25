// internal/repository/storage_cell_repository_test.go
//
// Тесты слоя доступа к ячейкам внутреннего хранилища (спека 2026-09-25-
// внутреннее-хранилище-и-рождение-заказов §4.1, ЧК2а): чтение ячеек владельца,
// инкремент с клампом ≥ 0 и ошибкой на отсутствующей строке, обнуление,
// удаление и размер хранилища поселения. Запросы утверждаются дословно
// (QueryMatcherEqual).
package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// storageCellTestRows — колонки таблицы в порядке SELECT-запроса.
func storageCellTestRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "owner_type", "owner_id", "good_id", "amount", "cap_share"})
}

// TestGetStorageCells — ячейки владельца читаются и разбираются (порядок
// good_id — из SQL), владелец другого типа не подмешивается.
func TestGetStorageCells(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().
			AddRow(int64(7), "settlement", "s1", int64(359), 12.5, 0.4).
			AddRow(int64(9), "settlement", "s1", int64(378), 0.0, 0.6))

	cells, err := NewStorageCellRepository(db).GetStorageCells("settlement", "s1")
	require.NoError(t, err)
	require.Len(t, cells, 2)
	require.Equal(t, models.StorageCell{ID: 7, OwnerType: "settlement", OwnerID: "s1", GoodID: 359, Amount: 12.5, CapShare: 0.4}, cells[0])
	require.Equal(t, models.StorageCell{ID: 9, OwnerType: "settlement", OwnerID: "s1", GoodID: 378, Amount: 0, CapShare: 0.6}, cells[1])
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetStorageCellsEmpty — ячеек нет → пустой срез, без ошибки.
func TestGetStorageCellsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(storageCellSelectSQL).WithArgs("building", "b1").WillReturnRows(storageCellTestRows())

	cells, err := NewStorageCellRepository(db).GetStorageCells(StorageOwnerBuilding, "b1")
	require.NoError(t, err)
	require.Empty(t, cells)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetStorageCellsQueryError — ошибка драйвера оборачивается.
func TestGetStorageCellsQueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").
		WillReturnError(errors.New("boom"))

	_, err = NewStorageCellRepository(db).GetStorageCells("settlement", "s1")
	require.ErrorContains(t, err, "failed to query storage cells")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetStorageCellsTx — чтение внутри транзакции (owner-проход §7.1).
func TestGetStorageCellsTx(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(359), 1.0, 1.0))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	cells, err := NewStorageCellRepository(db).GetStorageCellsTx(context.Background(), tx, "settlement", "s1")
	require.NoError(t, err)
	require.Len(t, cells, 1)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestIncrementStorageCellTx — относительный инкремент: отрицательная дельта
// (списание) доходит как есть, кламп ≥ 0 зашит в SQL (GREATEST), строка есть.
func TestIncrementStorageCellTx(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	require.Contains(t, storageCellIncrementSQL, "GREATEST(0,", "кламп ≥ 0 обязан жить в SQL")

	mock.ExpectBegin()
	mock.ExpectExec(storageCellIncrementSQL).WithArgs("settlement", "s1", -5.0, int64(359)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, NewStorageCellRepository(db).IncrementStorageCellTx(context.Background(), tx, "settlement", "s1", 359, -5.0))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestIncrementStorageCellTxMissingRow — строки нет → ErrStorageCellNotFound
// (молчаливого создания нет: набор ячеек задаёт реестр нужд §4.3).
func TestIncrementStorageCellTxMissingRow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(storageCellIncrementSQL).WithArgs("settlement", "s1", 1.0, int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = NewStorageCellRepository(db).IncrementStorageCellTx(context.Background(), tx, "settlement", "s1", 999, 1.0)
	require.ErrorIs(t, err, ErrStorageCellNotFound)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestIncrementStorageCellExecError — ошибка драйвера оборачивается.
func TestIncrementStorageCellExecError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(storageCellIncrementSQL).WithArgs("settlement", "s1", 1.0, int64(359)).
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = NewStorageCellRepository(db).IncrementStorageCellTx(context.Background(), tx, "settlement", "s1", 359, 1.0)
	require.ErrorContains(t, err, "failed to increment storage cell")
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestClearStorageCellsTx — обнуление всех ячеек владельца; ячеек нет — no-op
// без ошибки.
func TestClearStorageCellsTx(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rows        int64
		commentNote string
	}{
		{name: "ячейки есть", rows: 3},
		{name: "ячеек нет (no-op)", rows: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectBegin()
			mock.ExpectExec(storageCellsClearSQL).WithArgs("settlement", "s1").
				WillReturnResult(sqlmock.NewResult(0, tc.rows))
			mock.ExpectCommit()

			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, NewStorageCellRepository(db).ClearStorageCellsTx(context.Background(), tx, "settlement", "s1"))
			require.NoError(t, tx.Commit())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestDeleteStorageCellsTx — удаление всех ячеек владельца; ячеек нет — no-op.
func TestDeleteStorageCellsTx(t *testing.T) {
	for _, rows := range []int64{2, 0} {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)

		mock.ExpectBegin()
		mock.ExpectExec(storageCellsDeleteSQL).WithArgs("building", "b1").
			WillReturnResult(sqlmock.NewResult(0, rows))
		mock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		require.NoError(t, NewStorageCellRepository(db).DeleteStorageCellsTx(context.Background(), tx, "building", "b1"))
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

// TestGetSettlementStorageSize — размер читается; поселения нет →
// ErrSettlementNotFound.
func TestGetSettlementStorageSize(t *testing.T) {
	t.Run("есть", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(settlementStorageSizeSelectSQL).WithArgs("s1").
			WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))

		size, err := NewStorageCellRepository(db).GetSettlementStorageSize("s1")
		require.NoError(t, err)
		require.Equal(t, 1000.0, size)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("нет поселения", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(settlementStorageSizeSelectSQL).WithArgs("s1").WillReturnError(sql.ErrNoRows)

		_, err = NewStorageCellRepository(db).GetSettlementStorageSize("s1")
		require.ErrorIs(t, err, ErrSettlementNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestSetSettlementStorageSizeTx — запись размера; поселения нет →
// ErrSettlementNotFound (не тихий no-op).
func TestSetSettlementStorageSizeTx(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    int64
		wantErr error
	}{
		{name: "есть", rows: 1},
		{name: "нет поселения", rows: 0, wantErr: ErrSettlementNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectBegin()
			mock.ExpectExec(settlementStorageSizeUpdateSQL).WithArgs("s1", 1000.0).
				WillReturnResult(sqlmock.NewResult(0, tc.rows))
			if tc.wantErr != nil {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}

			tx, err := db.Begin()
			require.NoError(t, err)
			err = NewStorageCellRepository(db).SetSettlementStorageSizeTx(context.Background(), tx, "s1", 1000.0)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.NoError(t, tx.Rollback())
			} else {
				require.NoError(t, err)
				require.NoError(t, tx.Commit())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestStorageCellSQLReferencesSchemaColumns — SQL репозитория называет колонки
// схемы §4.1 дословно (расхождение имён ловится и живым интеграционным тестом;
// здесь — дешёвая ранняя страховка).
func TestStorageCellSQLReferencesSchemaColumns(t *testing.T) {
	for _, col := range []string{"owner_type", "owner_id", "good_id", "amount", "cap_share", "updated_at"} {
		require.True(t, strings.Contains(storageCellSelectSQL+storageCellIncrementSQL+storageCellsClearSQL+storageCellsDeleteSQL, col),
			"колонка %s не упомянута в SQL ячеек", col)
	}
	require.Contains(t, settlementStorageSizeSelectSQL, "storage_size")
	require.Contains(t, settlementStorageSizeUpdateSQL, "storage_size")
}
