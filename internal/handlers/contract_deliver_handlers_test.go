// internal/handlers/contract_deliver_handlers_test.go
//
// Точка сдачи на слое API (спека 2026-09-25-сдача-груза-и-зачёт-ЧК2б §5.1/§6):
// JWT-гейт, проверки контракта (не supply/taken/исполнитель/автор), орбитальный
// гейт (surface не подходит), ленивое истечение при сдаче и успешный ответ.
package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
)

// handlerCargoTaker — подмена трюма (интерфейс repository.CargoTaker) для
// сквозного хендлер-теста сдачи.
type handlerCargoTaker struct{}

func (handlerCargoTaker) TakeCargoTx(tx *sql.Tx, userID string, goodID int64, qty float64) (float64, error) {
	return qty, nil
}

// newDeliverHarness — ContractHandlers на sqlmock-БД с подключённым трюмом.
func newDeliverHarness(t *testing.T) (*ContractHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	repo := repository.NewContractRepository(db)
	repo.SetCargo(handlerCargoTaker{})
	return NewContractHandlers(
		repo,
		repository.NewPlanetRepository(db),
		repository.NewUserRepository(db),
		repository.NewKnowledgeRepository(db),
		repository.NewWorldRepository(db),
	), mock
}

// contractDeliverRow — строка contracts типа supply (порядок contractColumns).
func contractDeliverRow(id, typ, status, authorType, authorID string, execType, execID interface{}) *sqlmock.Rows {
	n := now()
	return sqlmock.NewRows([]string{
		"id", "type", "author_type", "author_id", "publication_planet_id", "title", "description",
		"payload", "reward", "funding", "escrow_amount", "escrow_withdrawable", "escrow_kind",
		"status", "visibility", "direct_target_type", "direct_target_id", "executor_type",
		"executor_id", "package_key", "share_index", "taken_at", "expires_at", "created_at", "updated_at",
	}).AddRow(id, typ, authorType, authorID, "p1", "Снабжение", "", []byte("{}"),
		500, "regular", 500, 0, "deposit", status, "public", nil, nil, execType, execID, nil, nil, nil, n, n, n)
}

// emptyDeliverContractRow — пустая выборка контракта (не найден).
func emptyDeliverContractRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "type", "author_type", "author_id", "publication_planet_id", "title", "description",
		"payload", "reward", "funding", "escrow_amount", "escrow_withdrawable", "escrow_kind",
		"status", "visibility", "direct_target_type", "direct_target_id", "executor_type",
		"executor_id", "package_key", "share_index", "taken_at", "expires_at", "created_at", "updated_at",
	})
}

// expectDeliverExpireDueNoop — ленивое истечение без истёкших (пустой RETURNING).
func expectDeliverExpireDueNoop(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE contracts\s+SET status = 'expired'`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id",
			"escrow_amount", "escrow_withdrawable"}))
	mock.ExpectCommit()
}

// expectDeliverContractGetByIDOnly — только чтение контракта (без требований:
// не найдено — attachRequirements не вызывается).
func expectDeliverContractGetByIDOnly(mock sqlmock.Sqlmock, row *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT id, type, author_type, author_id, publication_planet_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(row)
}

// TestDeliverContractRequiresAuth (T1): без JWT-контекста — 401, без запросов к БД.
func TestDeliverContractRequiresAuth(t *testing.T) {
	h, mock := newDeliverHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
		strings.NewReader(`{"contract_id":"c1"}`))
	rec := httptest.NewRecorder()
	h.DeliverContract(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverContractGates (T2): проверки контракта до транзакции.
func TestDeliverContractGates(t *testing.T) {
	cases := []struct {
		name     string
		contract *sqlmock.Rows
		noReqs   bool
		wantCode int
	}{
		{"нет контракта", emptyDeliverContractRow(), true, http.StatusNotFound},
		{"не supply", contractDeliverRow("c1", "travel", "taken", "settlement", "s1", "player", "u1"), false, http.StatusUnprocessableEntity},
		{"не taken", contractDeliverRow("c1", "supply", "open", "settlement", "s1", "player", "u1"), false, http.StatusConflict},
		{"не исполнитель", contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u2"), false, http.StatusForbidden},
		{"автор без хранилища", contractDeliverRow("c1", "supply", "taken", "player", "u2", "player", "u1"), false, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, mock := newDeliverHarness(t)
			expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
			if tc.noReqs {
				expectDeliverContractGetByIDOnly(mock, tc.contract)
			} else {
				expectContractGetByID(mock, tc.contract)
			}

			req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
				strings.NewReader(`{"contract_id":"c1"}`)), "u1")
			rec := httptest.NewRecorder()
			h.DeliverContract(rec, req)

			require.Equal(t, tc.wantCode, rec.Code, "body=%s", rec.Body.String())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestDeliverContractRoleForbidden: роль не player → 403 (до чтения контракта).
func TestDeliverContractRoleForbidden(t *testing.T) {
	h, mock := newDeliverHarness(t)
	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "admin")

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
		strings.NewReader(`{"contract_id":"c1"}`)), "u1")
	rec := httptest.NewRecorder()
	h.DeliverContract(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverContractNotOnOrbit (T3): surface и чужая планета/звезда → 422.
func TestDeliverContractNotOnOrbit(t *testing.T) {
	const surfacePos = `{"status":"surface","object_type":"planet","object_id":"p1","level":"surface"}`
	t.Run("surface не подходит", func(t *testing.T) {
		h, mock := newDeliverHarness(t)
		expectSurfaceUserRole(mock, "u1", "w1", surfacePos, "player")
		expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u1"))
		expectDeliverExpireDueNoop(mock)
		expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u1"))

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.DeliverContract(rec, req)

		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body=%s", rec.Body.String())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("орбита другой планеты", func(t *testing.T) {
		const otherPos = `{"status":"orbit","object_type":"planet","object_id":"p2","level":"orbit"}`
		h, mock := newDeliverHarness(t)
		expectSurfaceUserRole(mock, "u1", "w1", otherPos, "player")
		expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u1"))
		expectDeliverExpireDueNoop(mock)
		expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u1"))

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.DeliverContract(rec, req)

		require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestDeliverContractExpiredAfterDue (T2): после ExpireDue статус уже не taken → 409.
func TestDeliverContractExpiredAfterDue(t *testing.T) {
	h, mock := newDeliverHarness(t)
	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u1"))
	expectDeliverExpireDueNoop(mock)
	expectContractGetByID(mock, contractDeliverRow("c1", "supply", "expired", "settlement", "s1", "player", "u1"))

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
		strings.NewReader(`{"contract_id":"c1"}`)), "u1")
	rec := httptest.NewRecorder()
	h.DeliverContract(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code, "body=%s", rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverContractBuildingUnsupported (T19, честная правка ревьюера):
// автор-строение — 422 «сдача от строений пока не поддерживается» (задел), а не
// ложное «хранилище переполнено».
func TestDeliverContractBuildingUnsupported(t *testing.T) {
	h, mock := newDeliverHarness(t)
	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "building", "b1", "player", "u1"))
	expectDeliverExpireDueNoop(mock)
	expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "building", "b1", "player", "u1"))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT author_type, author_id, escrow_amount`).WithArgs("c1", "u1").
		WillReturnRows(sqlmock.NewRows([]string{"author_type", "author_id", "escrow_amount", "escrow_withdrawable", "funding"}).
			AddRow("building", "b1", int64(1000), int64(0), "regular"))
	mock.ExpectRollback()

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
		strings.NewReader(`{"contract_id":"c1"}`)), "u1")
	rec := httptest.NewRecorder()
	h.DeliverContract(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "строений")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeliverContractSuccess (T18-сервер): гейты пройдены — атомарный Deliver,
// ответ 200 с delivered/paid/remaining.
func TestDeliverContractSuccess(t *testing.T) {
	h, mock := newDeliverHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u1"))
	expectDeliverExpireDueNoop(mock)
	expectContractGetByID(mock, contractDeliverRow("c1", "supply", "taken", "settlement", "s1", "player", "u1"))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT author_type, author_id, escrow_amount`).WithArgs("c1", "u1").
		WillReturnRows(sqlmock.NewRows([]string{"author_type", "author_id", "escrow_amount", "escrow_withdrawable", "funding"}).
			AddRow("settlement", "s1", int64(1000), int64(0), "regular"))
	mock.ExpectQuery(`SELECT id, subject, quantity`).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "quantity"}).AddRow(int64(7), "426", int64(40)))
	mock.ExpectQuery(`SELECT weight FROM goods`).WithArgs(int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.5))
	mock.ExpectExec(`SELECT 1 FROM users WHERE id = \$1 FOR UPDATE`).WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT quantity FROM player_cargo`).WithArgs("u1", int64(426)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(100.0))
	mock.ExpectQuery(`SELECT id, owner_type, owner_id, good_id, amount, cap_share`).WithArgs("settlement", "s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_type", "owner_id", "good_id", "amount", "cap_share"}).
			AddRow(int64(1), "settlement", "s1", int64(426), 0.0, 1.0))
	mock.ExpectQuery(`SELECT storage_size FROM settlements`).WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"storage_size"}).AddRow(1000.0))
	mock.ExpectExec(`UPDATE settlement_storage_cells`).WithArgs("settlement", "s1", 40.0, int64(426)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contract_requirements SET quantity = 0`).WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`UPDATE contracts\s+SET status = 'completed'`).WithArgs("c1", "u1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_type", "executor_id",
			"escrow_amount", "escrow_withdrawable", "funding"}).
			AddRow("c1", "settlement", "s1", "player", "u1", int64(1000), int64(0), "regular"))
	mock.ExpectExec(`INSERT INTO accounts`).WithArgs("player", "u1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance`).WithArgs("player", "u1", int64(1000), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1000))
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // completed
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_released
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // delivered
	mock.ExpectCommit()

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/deliver",
		strings.NewReader(`{"contract_id":"c1"}`)), "u1")
	rec := httptest.NewRecorder()
	h.DeliverContract(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), `"status":"completed"`)
	require.Contains(t, rec.Body.String(), `"delivered":40`)
	require.Contains(t, rec.Body.String(), `"paid":1000`)
	require.NoError(t, mock.ExpectationsWereMet())
}
