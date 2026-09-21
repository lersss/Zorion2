// internal/repository/account_repository.go
//
// Деньги: счёт актора и журнал движений (спека 2026-09-22-деньги-и-эскроу
// §3). Счёт создаётся идемпотентно (ensureAccount), списание и начисление —
// ОДНИМ условным оператором (инвариант «проверка + действие атомарно»,
// AGENTS.md §0). Залог/эскроу (lock/release/return) — этап B, здесь только
// база: счёт + чтение баланса и истории.
package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"zorion/internal/models"
)

// ErrAccountNotFound — счёт актора не найден.
var ErrAccountNotFound = errors.New("счёт не найден")

// execer — общее для *sql.DB и *sql.Tx: вставка счёта идёт и одиночно
// (EnsureAccount), и внутри транзакции регистрации (CreateWithAccount).
type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// ensureAccountSQL — идемпотентное создание счёта (§3.4): ON CONFLICT
// DO NOTHING — повторный вызов не меняет баланс.
const ensureAccountSQL = `
	INSERT INTO accounts (owner_type, owner_id, balance, withdrawable, created_at, updated_at)
	VALUES ($1, $2, $3, 0, NOW(), NOW())
	ON CONFLICT (owner_type, owner_id) DO NOTHING
`

// ensureAccount создаёт счёт актора с начальным балансом seed, если его нет.
func ensureAccount(q execer, ownerType, ownerID string, seed int64) error {
	if _, err := q.Exec(ensureAccountSQL, ownerType, ownerID, seed); err != nil {
		return fmt.Errorf("ensure account: %w", err)
	}
	return nil
}

// debitAccountSQL — списание ОДНИМ условным оператором (§4): проверка
// достаточности баланса и списание атомарны (AGENTS.md §0). withdrawable —
// подмножество balance, поэтому при трате урезается до нового баланса
// (LEAST; в SET используются значения строки до UPDATE) — иначе CHECK
// (withdrawable <= balance) отверг бы списание. 0 строк → недостаточно средств.
const debitAccountSQL = `
	UPDATE accounts
	SET balance = balance - $3,
	    withdrawable = LEAST(withdrawable, balance - $3),
	    updated_at = NOW()
	WHERE owner_type = $1 AND owner_id = $2 AND balance >= $3
`

// creditAccountSQL — начисление (§3.3): balance растёт, withdrawable — только
// у подряда (contract_work_earn) и передаётся отдельным методом этапа B.
const creditAccountSQL = `
	UPDATE accounts
	SET balance = balance + $3, updated_at = NOW()
	WHERE owner_type = $1 AND owner_id = $2
`

// accountByOwnerSQL — чтение баланса по ключу (PK).
const accountByOwnerSQL = `
	SELECT owner_type, owner_id, balance, withdrawable, created_at, updated_at
	FROM accounts WHERE owner_type = $1 AND owner_id = $2
`

// moneyOperationsSQL — своя история операций, свежие сверху; индекс
// idx_money_operations_owner (owner_type, owner_id, occurred_at DESC).
const moneyOperationsSQL = `
	SELECT id, owner_type, owner_id, delta, balance_after, kind, contract_id, occurred_at, created_at
	FROM money_operations
	WHERE owner_type = $1 AND owner_id = $2
	ORDER BY occurred_at DESC, id DESC
	LIMIT $3
`

// AccountRepository — доступ к accounts/money_operations.
type AccountRepository struct {
	db *sql.DB
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

// EnsureAccount — идемпотентная страховка счёта (§3.4): повторный вызов с
// другим seed ничего не меняет (ON CONFLICT DO NOTHING).
func (r *AccountRepository) EnsureAccount(ownerType, ownerID string, seed int64) error {
	return ensureAccount(r.db, ownerType, ownerID, seed)
}

// Debit — списание amount одним условным оператором (§4): applied=false при
// нехватке средств (0 строк), ошибки нет. Проверка баланса и списание
// атомарны — TOCTOU между проверкой и действием невозможен.
func (r *AccountRepository) Debit(ownerType, ownerID string, amount int64) (bool, error) {
	// Отрицательная/нулевая сумма обошла бы условие `balance >= $3` (при $3 ≤ 0
	// оно всегда истинно) и УВЕЛИЧИЛА бы баланс — отказ без запроса к БД.
	if amount <= 0 {
		return false, nil
	}
	res, err := r.db.Exec(debitAccountSQL, ownerType, ownerID, amount)
	if err != nil {
		return false, fmt.Errorf("debit account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("debit account rows: %w", err)
	}
	return n == 1, nil
}

// Credit — начисление amount (§3.3): found=false, если счёта нет (перед
// начислением аккаунт гарантируется ensureAccount).
func (r *AccountRepository) Credit(ownerType, ownerID string, amount int64) (bool, error) {
	res, err := r.db.Exec(creditAccountSQL, ownerType, ownerID, amount)
	if err != nil {
		return false, fmt.Errorf("credit account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("credit account rows: %w", err)
	}
	return n == 1, nil
}

// GetBalance — счёт актора; nil, если счёта нет.
func (r *AccountRepository) GetBalance(ownerType, ownerID string) (*models.Account, error) {
	var a models.Account
	err := r.db.QueryRow(accountByOwnerSQL, ownerType, ownerID).Scan(
		&a.OwnerType, &a.OwnerID, &a.Balance, &a.Withdrawable, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &a, nil
}

// GetOperations — история движений по счёту (не более limit), свежие сверху.
func (r *AccountRepository) GetOperations(ownerType, ownerID string, limit int) ([]models.MoneyOperation, error) {
	rows, err := r.db.Query(moneyOperationsSQL, ownerType, ownerID, limit)
	if err != nil {
		return nil, fmt.Errorf("get money operations: %w", err)
	}
	defer rows.Close()

	var ops []models.MoneyOperation
	for rows.Next() {
		var op models.MoneyOperation
		if err := rows.Scan(
			&op.ID, &op.OwnerType, &op.OwnerID, &op.Delta, &op.BalanceAfter,
			&op.Kind, &op.ContractID, &op.OccurredAt, &op.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan money operation: %w", err)
		}
		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("money operations iteration error: %w", err)
	}
	return ops, nil
}
