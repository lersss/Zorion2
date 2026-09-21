// internal/repository/account_repository_test.go
//
// Деньги: счёт и журнал движений (спека 2026-09-22-деньги-и-эскроу §3).
// Тесты: идемпотентность ensureAccount (§3.4), атомарное списание (0 строк при
// нехватке, §4), инвариант withdrawable <= balance (кламп при трате, §3.3),
// чтение баланса и истории (§3.2).
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ensureAccount идемпотентен: повторный вызов — тот же INSERT ... ON CONFLICT
// DO NOTHING (второй вызов не ошибка, баланс не переписывается).
func TestEnsureAccountIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	re := `INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`
	// Второй вызов — 0 затронутых строк (ON CONFLICT DO NOTHING), не ошибка.
	mock.ExpectExec(re).WithArgs("player", "u1", int64(10000)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(re).WithArgs("player", "u1", int64(10000)).WillReturnResult(sqlmock.NewResult(0, 0))

	repo := NewAccountRepository(db)
	require.NoError(t, repo.EnsureAccount("player", "u1", 10000))
	require.NoError(t, repo.EnsureAccount("player", "u1", 10000))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Нехватка средств: 0 строк → applied=false, ошибки нет (условие balance >= amount).
func TestDebitAccountInsufficientFunds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE accounts\s+SET balance = balance - \$3`).
		WithArgs("player", "u1", int64(5000)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	applied, err := NewAccountRepository(db).Debit("player", "u1", 5000)
	require.NoError(t, err)
	assert.False(t, applied, "нехватка средств — 0 строк, списание не прошло")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Достаточно средств: 1 строка → applied=true.
func TestDebitAccountApplied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE accounts\s+SET balance = balance - \$3`).
		WithArgs("player", "u1", int64(300)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	applied, err := NewAccountRepository(db).Debit("player", "u1", 300)
	require.NoError(t, err)
	assert.True(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Нулевая/отрицательная сумма — отказ без запроса к БД: иначе отрицательный
// amount обошёл бы `balance >= $3` и увеличил баланс.
func TestDebitAccountRejectsNonPositive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewAccountRepository(db)
	for _, amount := range []int64{0, -100} {
		applied, err := repo.Debit("player", "u1", amount)
		require.NoError(t, err)
		assert.False(t, applied, "amount=%d должен быть отвергнут", amount)
	}
	require.NoError(t, mock.ExpectationsWereMet(), "запрос к БД не должен выполняться")
}

// Инвариант withdrawable <= balance держится и при трате: списание урезает
// withdrawable до нового баланса (LEAST), иначе CHECK отверг бы UPDATE (§3.3).
func TestDebitAccountSQLClampsWithdrawable(t *testing.T) {
	assert.Contains(t, debitAccountSQL, "LEAST(withdrawable, balance - $3)",
		"списание обязано урезать withdrawable, иначе CHECK withdrawable <= balance падает")
	assert.Contains(t, debitAccountSQL, "balance >= $3",
		"проверка достаточности — в том же операторе (атомарность)")
}

// Начисление: balance растёт, found=true.
func TestCreditAccount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE accounts\s+SET balance = balance \+ \$3`).
		WithArgs("player", "u1", int64(700)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := NewAccountRepository(db).Credit("player", "u1", 700)
	require.NoError(t, err)
	assert.True(t, found)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Начисление в несуществующий счёт: 0 строк → found=false.
func TestCreditAccountNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE accounts\s+SET balance = balance \+ \$3`).
		WithArgs("player", "ghost", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	found, err := NewAccountRepository(db).Credit("player", "ghost", 1)
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, mock.ExpectationsWereMet())
}

// GetBalance читает счёт по ключу.
func TestGetBalance(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`SELECT owner_type, owner_id, balance, withdrawable, created_at, updated_at\s+FROM accounts WHERE owner_type = \$1 AND owner_id = \$2`).
		WithArgs("player", "u1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id", "balance", "withdrawable", "created_at", "updated_at"}).
			AddRow("player", "u1", int64(12000), int64(2000), now, now))

	acc, err := NewAccountRepository(db).GetBalance("player", "u1")
	require.NoError(t, err)
	require.NotNil(t, acc)
	assert.Equal(t, int64(12000), acc.Balance)
	assert.Equal(t, int64(2000), acc.Withdrawable)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Счёта нет → nil, без ошибки.
func TestGetBalanceNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT owner_type, owner_id, balance, withdrawable, created_at, updated_at\s+FROM accounts WHERE owner_type = \$1 AND owner_id = \$2`).
		WithArgs("player", "ghost").
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id", "balance", "withdrawable", "created_at", "updated_at"}))

	acc, err := NewAccountRepository(db).GetBalance("player", "ghost")
	require.NoError(t, err)
	assert.Nil(t, acc)
	require.NoError(t, mock.ExpectationsWereMet())
}

// GetOperations — история движений, свежие сверху; contract_id может быть NULL.
func TestGetOperations(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	contract := "c-1"
	mock.ExpectQuery(`SELECT id, owner_type, owner_id, delta, balance_after, kind, contract_id, occurred_at, created_at\s+FROM money_operations\s+WHERE owner_type = \$1 AND owner_id = \$2\s+ORDER BY occurred_at DESC, id DESC\s+LIMIT \$3`).
		WithArgs("player", "u1", 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_type", "owner_id", "delta", "balance_after", "kind", "contract_id", "occurred_at", "created_at"}).
			AddRow(int64(2), "player", "u1", int64(-300), int64(9700), "escrow_lock", contract, now, now).
			AddRow(int64(1), "player", "u1", int64(10000), int64(10000), "admin_seed", nil, now, now))

	ops, err := NewAccountRepository(db).GetOperations("player", "u1", 50)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, "escrow_lock", ops[0].Kind)
	assert.Equal(t, int64(-300), ops[0].Delta)
	require.NotNil(t, ops[0].ContractID)
	assert.Equal(t, "c-1", *ops[0].ContractID)
	assert.Nil(t, ops[1].ContractID)
	require.NoError(t, mock.ExpectationsWereMet())
}
