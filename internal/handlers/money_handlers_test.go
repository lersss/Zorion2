// internal/handlers/money_handlers_test.go
//
// GET /me/money (спека 2026-09-22-деньги-и-эскроу §3.2, О-д4): свой баланс +
// своя история; чужие не показываются (owner_id из JWT). Счёт гарантируется
// лениво (§3.4).
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// Баланс + история операций по своему счёту; счёт гарантируется перед чтением.
func TestGetMyMoney(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	userID := "11111111-1111-1111-1111-111111111111"
	now := time.Now()

	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("player", userID, int64(models.PlayerBalanceSeed)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT owner_type, owner_id, balance, withdrawable, created_at, updated_at\s+FROM accounts WHERE owner_type = \$1 AND owner_id = \$2`).
		WithArgs("player", userID).
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id", "balance", "withdrawable", "created_at", "updated_at"}).
			AddRow("player", userID, int64(12000), int64(2000), now, now))
	mock.ExpectQuery(`SELECT id, owner_type, owner_id, delta, balance_after, kind, contract_id, occurred_at, created_at\s+FROM money_operations`).
		WithArgs("player", userID, moneyHistoryLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_type", "owner_id", "delta", "balance_after", "kind", "contract_id", "occurred_at", "created_at"}).
			AddRow(int64(1), "player", userID, int64(10000), int64(10000), "admin_seed", nil, now, now))

	h := NewMoneyHandlers(repository.NewAccountRepository(db))
	req := httptest.NewRequest(http.MethodGet, "/me/money", nil)
	rec := execJSON(h.GetMyMoney, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Balance      int64 `json:"balance"`
		Withdrawable int64 `json:"withdrawable"`
		Operations   []struct {
			Kind string `json:"kind"`
		} `json:"operations"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, int64(12000), resp.Balance)
	assert.Equal(t, int64(2000), resp.Withdrawable)
	require.Len(t, resp.Operations, 1)
	assert.Equal(t, "admin_seed", resp.Operations[0].Kind)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пустая история → operations: [] (не null) — фронт итерирует без guard.
func TestGetMyMoneyEmptyHistory(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	userID := "22222222-2222-2222-2222-222222222222"
	now := time.Now()

	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT owner_type, owner_id, balance, withdrawable, created_at, updated_at\s+FROM accounts`).
		WithArgs("player", userID).
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id", "balance", "withdrawable", "created_at", "updated_at"}).
			AddRow("player", userID, int64(10000), int64(0), now, now))
	mock.ExpectQuery(`SELECT id, owner_type, owner_id, delta, balance_after, kind, contract_id, occurred_at, created_at\s+FROM money_operations`).
		WithArgs("player", userID, moneyHistoryLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_type", "owner_id", "delta", "balance_after", "kind", "contract_id", "occurred_at", "created_at"}))

	h := NewMoneyHandlers(repository.NewAccountRepository(db))
	req := httptest.NewRequest(http.MethodGet, "/me/money", nil)
	rec := execJSON(h.GetMyMoney, withUserID(req, userID))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "[]", string(resp["operations"]), "пустая история — массив, не null")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Без JWT — 401, БД не трогается (чужой баланс не отдаётся).
func TestGetMyMoneyUnauthorized(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := NewMoneyHandlers(repository.NewAccountRepository(db))
	req := httptest.NewRequest(http.MethodGet, "/me/money", nil)
	rec := execJSON(h.GetMyMoney, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
