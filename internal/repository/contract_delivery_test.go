// internal/repository/contract_delivery_test.go
//
// Точка сдачи груза (спека 2026-09-25-сдача-груза-и-зачёт-ЧК2б §5.2): проверки под
// локами, объём min(трюм, остаток, 2·cap−amount), списание ровно delivered,
// приход в ячейку, частичная пропорциональная выплата, полная через completeTx,
// fail-closed ветки (нет контракта/требования/товара/ячейки, переполнение ×2).
package repository

import (
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// qre — regexp-литерал SQL-константы.
func qre(s string) string { return regexp.QuoteMeta(s) }

// fakeCargoTaker — подмена трюма для тестов оркестрации сдачи: возвращает
// переданное qty (или заданное returned/err). В БД не пишет.
type fakeCargoTaker struct {
	returned float64
	err      error
	gotQty   float64
	gotGood  int64
}

func (f *fakeCargoTaker) TakeCargoTx(tx *sql.Tx, userID string, goodID int64, qty float64) (float64, error) {
	f.gotQty = qty
	f.gotGood = goodID
	if f.err != nil {
		return 0, f.err
	}
	if f.returned > 0 {
		return f.returned, nil
	}
	return qty, nil
}

// newDeliveryHarness — репозиторий с sqlmock-БД и подменённым трюмом.
func newDeliveryHarness(t *testing.T) (*ContractRepository, sqlmock.Sqlmock, *fakeCargoTaker) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	cargo := &fakeCargoTaker{}
	repo := NewContractRepository(db)
	repo.SetCargo(cargo)
	return repo, mock, cargo
}

const (
	deliverContractLockRe = `(?s)SELECT author_type, author_id, escrow_amount, escrow_withdrawable, funding\s+FROM contracts\s+WHERE id = \$1 AND type = 'supply'`
	deliverReqLockRe      = `(?s)SELECT id, subject, quantity\s+FROM contract_requirements\s+WHERE contract_id = \$1 AND kind = 'goods' AND op = 'in'`
	deliverUsersLockRe    = `SELECT 1 FROM users WHERE id = \$1 FOR UPDATE`
	deliverCargoLockRe    = `(?s)SELECT quantity FROM player_cargo WHERE user_id = \$1 AND good_id = \$2 FOR UPDATE`
	deliverWeightRe       = `SELECT weight FROM goods WHERE id = \$1`
	deliverCellIncRe      = `(?s)UPDATE settlement_storage_cells\s+SET amount = GREATEST\(0, amount \+ \$3\)`
	deliverReqZeroRe      = `UPDATE contract_requirements SET quantity = 0 WHERE id = \$1`
	deliverReqDecRe       = `(?s)UPDATE contract_requirements SET quantity = quantity - \$2 WHERE id = \$1`
	deliverEscrowUpdRe    = `(?s)UPDATE contracts SET escrow_amount = \$2, escrow_withdrawable = \$3`
	deliverCompleteRe     = `(?s)UPDATE contracts\s+SET status = 'completed'.*expires_at > NOW\(\)`
	deliverReleaseRe      = `(?s)UPDATE accounts\s+SET balance = balance \+ \$3`
	deliverLogRe          = `INSERT INTO contract_log`
	deliverMoneyOpRe      = `INSERT INTO money_operations`
	deliverEnsureAcctRe   = `INSERT INTO accounts`
)

// deliverContractRows — строка лока контракта-цели.
func deliverContractRows(authorType, authorID, funding string, escrow, withdrawable int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"author_type", "author_id", "escrow_amount", "escrow_withdrawable", "funding"}).
		AddRow(authorType, authorID, escrow, withdrawable, funding)
}

// deliverCompleteRows — строка RETURNING завершения (completeTx).
func deliverCompleteRows(amount, withdrawable int64, funding string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_type", "executor_id",
		"escrow_amount", "escrow_withdrawable", "funding"}).
		AddRow("c1", "settlement", "s1", "player", "u1", amount, withdrawable, funding)
}

// expectDeliverPrelude — общие шаги до расчёта объёма: лок контракта, требование,
// вес, лок игрока, чтение трюма. Возвращает контракт-мок после Begin.
func expectDeliverPrelude(mock sqlmock.Sqlmock, have float64) {
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("settlement", "s1", "regular", 1000, 0))
	mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(40)))
	mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
	mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(have))
}

// TestDeliverFullSupplyUsesCompleteTx (T6/T7): полная сдача идёт существующим
// completeTx — выпуск всего остатка залога, ровно одна запись money_operations,
// логи completed+escrow_released+delivered; лишнее в трюме не берётся.
func TestDeliverFullSupplyUsesCompleteTx(t *testing.T) {
	repo, mock, cargo := newDeliveryHarness(t)

	mock.ExpectBegin()
	expectDeliverPrelude(mock, 100)
	// Ячейки владельца под локом: цель (426) + соседняя; размер — поселения.
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().
			AddRow(int64(1), "settlement", "s1", int64(426), 10.0, 1.0).
			AddRow(int64(2), "settlement", "s1", int64(7), 0.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 40.0, int64(426)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverReqZeroRe).WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// completeTx: flip + выпуск всего остатка залога + лог completed/escrow_released.
	mock.ExpectQuery(deliverCompleteRe).WithArgs("c1", "u1").
		WillReturnRows(deliverCompleteRows(1000, 0, "regular"))
	mock.ExpectExec(deliverEnsureAcctRe).WithArgs("player", "u1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverReleaseRe).WithArgs("player", "u1", int64(1000), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1000))
	mock.ExpectExec(deliverMoneyOpRe).WithArgs("player", "u1", int64(1000), int64(1000), "escrow_release", "c1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // completed
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_released
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // delivered
	mock.ExpectCommit()

	res, err := repo.Deliver("u1", "c1")
	require.NoError(t, err)
	require.Equal(t, models.ContractStatusCompleted, res.Status)
	require.Equal(t, int64(40), res.Delivered, "лишнее (трюм 100) не берётся — только остаток требования 40")
	require.Equal(t, int64(0), res.Remaining)
	require.Equal(t, int64(1000), res.Paid, "полный остаток залога")
	require.Equal(t, int64(426), res.GoodID)
	require.Equal(t, 40.0, cargo.gotQty)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverPartialProportionalPayment (T8/T15): частичная сдача — требование
// уменьшается, escrow_amount уменьшается на pay, escrow_withdrawable — пропорционально,
// статус taken, пропорциональная выплата.
func TestDeliverPartialProportionalPayment(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)

	mock.ExpectBegin()
	// Контракт: escrow 1000, withdrawable 200.
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("settlement", "s1", "regular", 1000, 200))
	mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(100)))
	mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
	mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(40.0))
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 0.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 40.0, int64(426)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// pay = 1000·40/100 = 400; остаток 600; withdrawable = 200·600/1000 = 120.
	mock.ExpectExec(deliverEscrowUpdRe).WithArgs("c1", int64(600), int64(120)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverReqDecRe).WithArgs(int64(7), int64(40)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverEnsureAcctRe).WithArgs("player", "u1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverReleaseRe).WithArgs("player", "u1", int64(400), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(400))
	mock.ExpectExec(deliverMoneyOpRe).WithArgs("player", "u1", int64(400), int64(400), "escrow_release", "c1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // delivered
	mock.ExpectCommit()

	res, err := repo.Deliver("u1", "c1")
	require.NoError(t, err)
	require.Equal(t, models.ContractStatusTaken, res.Status)
	require.Equal(t, int64(40), res.Delivered)
	require.Equal(t, int64(60), res.Remaining)
	require.Equal(t, int64(400), res.Paid)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverClampToDoubleCap (T4/T13): delivered режется до 2·cap − amount.
func TestDeliverClampToDoubleCap(t *testing.T) {
	repo, mock, cargo := newDeliveryHarness(t)

	mock.ExpectBegin()
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("settlement", "s1", "regular", 1000, 0))
	mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(1000)))
	mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
	mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(500.0))
	// Ячейка: amount 50, size 100, share 1 → cap 100, allowedCap = 150.
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 50.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(100.0))
	mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 150.0, int64(426)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverEscrowUpdRe).WithArgs("c1", int64(850), int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverReqDecRe).WithArgs(int64(7), int64(150)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverEnsureAcctRe).WithArgs("player", "u1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverReleaseRe).WithArgs("player", "u1", int64(150), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(150))
	mock.ExpectExec(deliverMoneyOpRe).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	res, err := repo.Deliver("u1", "c1")
	require.NoError(t, err)
	require.Equal(t, int64(150), res.Delivered, "кламп по 2·cap − amount = 150")
	require.Equal(t, int64(850), res.Remaining)
	require.Equal(t, 150.0, cargo.gotQty)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverPartialThenFull (T8): сумма выплат по контракту = исходному залогу,
// двойной выплаты нет.
func TestDeliverPartialThenFull(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)

	// Часть 1: 40 из 100, escrow 1000 → pay 400.
	mock.ExpectBegin()
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("settlement", "s1", "regular", 1000, 0))
	mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(100)))
	mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
	mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(40.0))
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 0.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 40.0, int64(426)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverEscrowUpdRe).WithArgs("c1", int64(600), int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverReqDecRe).WithArgs(int64(7), int64(40)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverEnsureAcctRe).WithArgs("player", "u1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverReleaseRe).WithArgs("player", "u1", int64(400), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(400))
	mock.ExpectExec(deliverMoneyOpRe).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	first, err := repo.Deliver("u1", "c1")
	require.NoError(t, err)
	require.Equal(t, int64(400), first.Paid)

	// Часть 2 (финал): 60 из 60, остаток залога 600 → выпуск целиком через completeTx.
	mock.ExpectBegin()
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("settlement", "s1", "regular", 600, 0))
	mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(60)))
	mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
	mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(60.0))
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 40.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 60.0, int64(426)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverReqZeroRe).WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(deliverCompleteRe).WithArgs("c1", "u1").
		WillReturnRows(deliverCompleteRows(600, 0, "regular"))
	mock.ExpectExec(deliverEnsureAcctRe).WithArgs("player", "u1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverReleaseRe).WithArgs("player", "u1", int64(600), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1000))
	mock.ExpectExec(deliverMoneyOpRe).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // completed
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_released
	mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // delivered
	mock.ExpectCommit()

	second, err := repo.Deliver("u1", "c1")
	require.NoError(t, err)
	require.Equal(t, models.ContractStatusCompleted, second.Status)
	require.Equal(t, int64(600), second.Paid)
	require.Equal(t, int64(1000), first.Paid+second.Paid, "Σ выплат = исходному залогу")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverFreeSupplyZeroEscrow (T9): бесплатный supply-0 — полная сдача идёт
// completeTx (нулевой op допустим), частичная при pay=0 движения счёта не пишет.
func TestDeliverFreeSupplyZeroEscrow(t *testing.T) {
	t.Run("полная — нулевой op через completeTx", func(t *testing.T) {
		repo, mock, _ := newDeliveryHarness(t)
		mock.ExpectBegin()
		mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
			WillReturnRows(deliverContractRows("settlement", "s1", "regular", 0, 0))
		mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(10)))
		mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
			WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
		mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
			WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(10.0))
		mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
			WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 0.0, 1.0))
		mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
			WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
		mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 10.0, int64(426)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(deliverReqZeroRe).WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(deliverCompleteRe).WithArgs("c1", "u1").
			WillReturnRows(deliverCompleteRows(0, 0, "regular"))
		mock.ExpectExec(deliverEnsureAcctRe).WithArgs("player", "u1", int64(0)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(deliverReleaseRe).WithArgs("player", "u1", int64(0), int64(0)).
			WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(0))
		mock.ExpectExec(deliverMoneyOpRe).WillReturnResult(sqlmock.NewResult(0, 1)) // нулевой op существующим completeTx
		mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		res, err := repo.Deliver("u1", "c1")
		require.NoError(t, err)
		require.Equal(t, models.ContractStatusCompleted, res.Status)
		require.Equal(t, int64(0), res.Paid)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("частичная pay=0 — движения счёта нет", func(t *testing.T) {
		repo, mock, _ := newDeliveryHarness(t)
		mock.ExpectBegin()
		mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
			WillReturnRows(deliverContractRows("settlement", "s1", "regular", 0, 0))
		mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(40)))
		mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
			WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
		mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
			WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(10.0))
		mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
			WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 0.0, 1.0))
		mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
			WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
		mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 10.0, int64(426)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(deliverEscrowUpdRe).WithArgs("c1", int64(0), int64(0)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(deliverReqDecRe).WithArgs(int64(7), int64(10)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(deliverLogRe).WillReturnResult(sqlmock.NewResult(0, 1)) // delivered, paid:0
		mock.ExpectCommit()

		res, err := repo.Deliver("u1", "c1")
		require.NoError(t, err)
		require.Equal(t, models.ContractStatusTaken, res.Status)
		require.Equal(t, int64(0), res.Paid)
		require.Equal(t, int64(30), res.Remaining)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestDeliverNoCargo (T5): товара требования в трюме нет — 422-ошибка, откат.
func TestDeliverNoCargo(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)
	mock.ExpectBegin()
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("settlement", "s1", "regular", 1000, 0))
	mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(40)))
	mock.ExpectQuery(deliverWeightRe).WithArgs(int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
	mock.ExpectExec(deliverUsersLockRe).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(deliverCargoLockRe).WithArgs("u1", int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}))
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.ErrorIs(t, err, ErrDeliveryNoCargo)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverStorageFull (T13): 2·cap − amount ≤ 0 → 409-ошибка, ничего не меняется.
func TestDeliverStorageFull(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)
	mock.ExpectBegin()
	expectDeliverPrelude(mock, 100)
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().
			AddRow(int64(1), "settlement", "s1", int64(426), 2000.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.ErrorIs(t, err, ErrDeliveryStorageFull)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverMissingCell (T14): ячейки владельца по товару нет → 409-ошибка,
// груз остаётся в трюме, авто-создания нет.
func TestDeliverMissingCell(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)
	mock.ExpectBegin()
	expectDeliverPrelude(mock, 100)
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().
			AddRow(int64(2), "settlement", "s1", int64(7), 0.0, 1.0))
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.ErrorIs(t, err, ErrDeliveryStorageCellMissing)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverContractDamaged (T2): не ровно одно goods-требование → fail-closed.
func TestDeliverContractDamaged(t *testing.T) {
	t.Run("ноль требований", func(t *testing.T) {
		repo, mock, _ := newDeliveryHarness(t)
		mock.ExpectBegin()
		mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
			WillReturnRows(deliverContractRows("settlement", "s1", "regular", 1000, 0))
		mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}))
		mock.ExpectRollback()

		_, err := repo.Deliver("u1", "c1")
		require.ErrorIs(t, err, ErrContractDamaged)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("два требования — первое не выбирается молча", func(t *testing.T) {
		repo, mock, _ := newDeliveryHarness(t)
		mock.ExpectBegin()
		mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
			WillReturnRows(deliverContractRows("settlement", "s1", "regular", 1000, 0))
		mock.ExpectQuery(deliverReqLockRe).WithArgs("c1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).
				AddRow(int64(7), "426", int64(10)).
				AddRow(int64(8), "427", int64(10)))
		mock.ExpectRollback()

		_, err := repo.Deliver("u1", "c1")
		require.ErrorIs(t, err, ErrContractDamaged)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestDeliverNotAvailable (T2): контракт не supply/taken/истёк (0 строк под локом)
// → 409-ошибка, откат.
func TestDeliverNotAvailable(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)
	mock.ExpectBegin()
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(sqlmock.NewRows([]string{"author_type", "author_id", "escrow_amount", "escrow_withdrawable", "funding"}))
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.ErrorIs(t, err, ErrDeliveryNotAvailable)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverAuthorUnsupported: автор без хранилища (player/faction) → 422-ошибка.
func TestDeliverAuthorUnsupported(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)
	mock.ExpectBegin()
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("player", "u2", "regular", 1000, 0))
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.ErrorIs(t, err, ErrDeliveryAuthorUnsupported)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverBuildingUnsupported (T19, честная правка ревьюера): автор-строение
// принимается общей логикой, но честно отклоняется отдельной ошибкой —
// источника размера хранилища и реестра ячеек у строений нет (задел §0 п.15),
// ложное «переполнено» не выдаём.
func TestDeliverBuildingUnsupported(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)
	mock.ExpectBegin()
	mock.ExpectQuery(deliverContractLockRe).WithArgs("c1", "u1").
		WillReturnRows(deliverContractRows("building", "b1", "regular", 1000, 0))
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.ErrorIs(t, err, ErrDeliveryBuildingUnsupported)
	require.NotErrorIs(t, err, ErrDeliveryStorageFull, "текст отказа не врёт про переполнение")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverCargoMismatchFailsClosed (T10): рассинхрон трюма (снято ≠ delivered)
// → ошибка, полный откат без частичного учёта.
func TestDeliverCargoMismatchFailsClosed(t *testing.T) {
	repo, mock, cargo := newDeliveryHarness(t)
	cargo.returned = 1 // фейк снял не то, что посчитал Deliver
	mock.ExpectBegin()
	expectDeliverPrelude(mock, 100)
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 0.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "рассинхрон трюма")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverStorageCellFailureRollsBack (T10): сбой инкремента ячейки откатывает
// всё (списание трюма и приход ячейки — в одной транзакции).
func TestDeliverStorageCellFailureRollsBack(t *testing.T) {
	repo, mock, _ := newDeliveryHarness(t)
	mock.ExpectBegin()
	expectDeliverPrelude(mock, 100)
	mock.ExpectQuery(qre(storageCellSelectForUpdateSQL)).WithArgs("settlement", "s1").
		WillReturnRows(storageCellTestRows().AddRow(int64(1), "settlement", "s1", int64(426), 0.0, 1.0))
	mock.ExpectQuery(qre(deliveryStorageSizeSQL)).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectExec(deliverCellIncRe).WithArgs("settlement", "s1", 40.0, int64(426)).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	_, err := repo.Deliver("u1", "c1")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
