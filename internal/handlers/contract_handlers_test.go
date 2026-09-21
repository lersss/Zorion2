// internal/handlers/contract_handlers_test.go
//
// Контракты на слое API (спеки 2026-09-22-контракт-модель-сущности §4–§6,
// 2026-09-22-контракт-перелёт-и-доска §2). Тесты: гейт доски знанием планеты,
// ленивое истечение на чтении доски, фильтры доски (open+public), атомарное
// взятие, отмена только автором/open, публикация игроком и админом.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
)

// newContractHarness — sqlmock-БД + ContractHandlers на реальных репозиториях.
func newContractHarness(t *testing.T) (*ContractHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewContractHandlers(
		repository.NewContractRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewUserRepository(db),
		repository.NewKnowledgeRepository(db),
	), mock
}

// contractUserRow — строка users для GetByID с заданной ролью.
func contractUserRow(id, world, role string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "password_hash", "email", "agent_id", "current_world_id",
		"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at",
	}).AddRow(id, "player", "hash", nil, nil, world, "ship_strela.svg", nil, "starter",
		`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, role, now(), now())
}

// expectContractUser — ожидание GetByID (роль любая; player — expectPlayerUser).
func expectContractUser(mock sqlmock.Sqlmock, id, world, role string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(contractUserRow(id, world, role))
}

// contractRows — строка contracts (порядок contractColumns).
func contractRows(id, status, visibility string, escrow int64) *sqlmock.Rows {
	n := now()
	return sqlmock.NewRows([]string{
		"id", "type", "author_type", "author_id", "publication_planet_id", "title", "description",
		"payload", "reward", "funding", "escrow_amount", "escrow_withdrawable", "escrow_kind",
		"status", "visibility", "direct_target_type", "direct_target_id", "executor_type",
		"executor_id", "taken_at", "expires_at", "created_at", "updated_at",
	}).AddRow(id, "travel", "player", "u1", "p1", "T", "", []byte("{}"),
		escrow, "regular", escrow, 0, "deposit", status, visibility, nil, nil, nil, nil, nil, n, n, n)
}

// contractReqRows — пустая выборка contract_requirements.
func contractReqRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "contract_id", "pos", "kind", "subject", "op",
		"threshold_num", "threshold_text", "quantity"})
}

// expectContractGetByID — чтение контракта после публикации (GetByID + требования).
func expectContractGetByID(mock sqlmock.Sqlmock, row *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT id, type, author_type, author_id, publication_planet_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(row)
	mock.ExpectQuery(`SELECT id, contract_id, pos, kind, subject, op,`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(contractReqRows())
}

// expectPublishChain — публикация одной tx: залог (lock) + контракт + лог +
// money_operations('escrow_lock'). Завершается Commit (без чтения обратно).
func expectPublishChain(mock sqlmock.Sqlmock, ownerType, ownerID, authorType, authorID, planetID string,
	reward, seed int64) {
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs(ownerType, ownerID, seed).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs(ownerType, ownerID, reward).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}).AddRow(1000, 0, 0))
	mock.ExpectExec(`INSERT INTO contracts`).
		WithArgs(sqlmock.AnyArg(), "travel", authorType, authorID, planetID, "T", "",
			sqlmock.AnyArg(), reward, "regular", reward, int64(0),
			"deposit", "open", "public", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs(ownerType, ownerID, -reward, sqlmock.AnyArg(), "escrow_lock", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

// ==================== Доска планеты ====================

// Без знания планеты (player) доска не отдаётся — 403.
func TestContractBoardRequiresKnowledge(t *testing.T) {
	h, mock := newContractHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectPlayerUser(mock, "u1")
	expectNoKnowledge(mock, "u1", "p1")

	req := withUserID(httptest.NewRequest(http.MethodGet, "/api/planets/p1/contracts", nil), "u1")
	rec := httptest.NewRecorder()
	h.GetPlanetBoard(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, "нет знания планеты → доска недоступна")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Админ видит доску без знания (обход гейта); на чтении доски срабатывает
// ленивое истечение (просроченный контракт закрывается, залог возвращается).
// Доска показывает только open+public (фильтр в SQL — прямые/взятые не видны).
func TestContractBoardAdminBypassLazyExpiry(t *testing.T) {
	h, mock := newContractHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectContractUser(mock, "a1", "w1", "admin")

	// Ленивое истечение: одна просроченная открытая → возврат залога + лог.
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE contracts\s+SET status = 'expired'`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id",
			"escrow_amount", "escrow_withdrawable"}).AddRow("expired1", "faction", "f1", nil, 700, 0))
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("faction", "f1", int64(1000000000000000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3, withdrawable = withdrawable \+ \$4`).
		WithArgs("faction", "f1", int64(700), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1000000000000700))
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // expired
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_returned
	mock.ExpectCommit()

	// Доска: только open+public (прямые/взятые отсечены SQL-фильтром).
	mock.ExpectQuery(`FROM contracts\s+WHERE publication_planet_id = \$1 AND status = 'open' AND expires_at > NOW\(\) AND visibility = 'public'`).
		WithArgs("p1").
		WillReturnRows(contractRows("c1", "open", "public", 500))
	mock.ExpectQuery(`SELECT id, contract_id, pos, kind, subject, op,`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(contractReqRows())

	req := withUserID(httptest.NewRequest(http.MethodGet, "/api/planets/p1/contracts", nil), "a1")
	rec := httptest.NewRecorder()
	h.GetPlanetBoard(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "админ видит доску без знания")
	require.Contains(t, rec.Body.String(), `"c1"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== Взятие ====================

// Взятие: успех (flip open→taken) и гонка/истёк срок (0 строк → 409).
// Условие срока (`expires_at > NOW()`) входит в тот же атомарный UPDATE.
func TestContractTake(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		h, mock := newContractHarness(t)
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)UPDATE contracts\s+SET status = 'taken'.*expires_at > NOW\(\)`).
			WithArgs("c1", "player", "u1", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/take",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.TakeContract(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("гонка/истёк", func(t *testing.T) {
		h, mock := newContractHarness(t)
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)UPDATE contracts\s+SET status = 'taken'.*expires_at > NOW\(\)`).
			WithArgs("c1", "player", "u1", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/take",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.TakeContract(rec, req)

		require.Equal(t, http.StatusConflict, rec.Code, "уже взят/истёк → отказ")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ==================== Отмена ====================

// Отмена: успех (только автор, только open) и отказ (0 строк).
func TestContractCancel(t *testing.T) {
	t.Run("успех — return залога автору", func(t *testing.T) {
		h, mock := newContractHarness(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE contracts\s+SET status = 'cancelled'`).
			WithArgs("c1", "player", "u1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id",
				"escrow_amount", "escrow_withdrawable"}).AddRow("c1", "player", "u1", nil, 500, 50))
		mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
			WithArgs("player", "u1", int64(10000)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3, withdrawable = withdrawable \+ \$4`).
			WithArgs("player", "u1", int64(500), int64(50)).
			WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1500))
		mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/cancel",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.CancelContract(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("отказ — не автор или уже взят", func(t *testing.T) {
		h, mock := newContractHarness(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE contracts\s+SET status = 'cancelled'`).
			WithArgs("c1", "player", "u1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id",
				"escrow_amount", "escrow_withdrawable"}))
		mock.ExpectRollback()

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/cancel",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.CancelContract(rec, req)

		require.Equal(t, http.StatusConflict, rec.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ==================== Публикация ====================

const contractPlanetPos = `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`

// Публикация игроком: с планеты, где стоит игрок; залог списывается одной tx.
func TestContractCreatePlayer(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectPlanetByID(mock, "p1", "w1") // проверка присутствия
	expectPlanetByID(mock, "p1", "w1") // проверка в publish
	expectPublishChain(mock, "player", "u1", "player", "u1", "p1", 500, 10000)
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts",
		strings.NewReader(`{"planet_id":"p1","type":"travel","title":"T","reward":500}`)), "u1")
	rec := httptest.NewRecorder()
	h.CreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Невалидный ввод (reward ≤ 0) → 400 ещё до записи в БД.
func TestContractCreateInvalidReward(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectPlanetByID(mock, "p1", "w1")

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts",
		strings.NewReader(`{"planet_id":"p1","type":"travel","title":"T","reward":0}`)), "u1")
	rec := httptest.NewRecorder()
	h.CreateContract(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Публикация игроком не с планеты (нет позиции на планете) → 409, без записи.
func TestContractCreateNotOnPlanet(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", `{"status":"orbit","object_type":"star","object_id":"w1","level":"orbit"}`, "player")

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts",
		strings.NewReader(`{"planet_id":"p1","type":"travel","title":"T","reward":500}`)), "u1")
	rec := httptest.NewRecorder()
	h.CreateContract(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code, "публикация только с планеты (О-п1)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Админ-публикация: автор-фракция, планета берётся из factions.homeworld_id
// (спека перелёта §3), плательщик — казна фракции.
func TestContractAdminCreate(t *testing.T) {
	h, mock := newContractHarness(t)

	// Планета публикации резолвится по автору, а не из тела.
	mock.ExpectQuery(`SELECT homeworld_id FROM factions WHERE id = \$1`).
		WithArgs("f1").
		WillReturnRows(sqlmock.NewRows([]string{"homeworld_id"}).AddRow("p1"))
	expectPlanetByID(mock, "p1", "w1")
	expectPublishChain(mock, "faction", "f1", "faction", "f1", "p1", 300, 1000000000000000)
	expectContractGetByID(mock, contractRows("c1", "open", "public", 300))

	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p1","type":"travel","title":"T","reward":300,"author_type":"faction","author_id":"f1"}`))
	rec := httptest.NewRecorder()
	h.AdminCreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Админ-публикация игроком: планета — где стоит игрок (то же правило, что в
// CreateContract), тело planet_id игнорируется.
func TestContractAdminCreatePlayerFromPosition(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectPlanetByID(mock, "p1", "w1")
	expectPublishChain(mock, "player", "u1", "player", "u1", "p1", 500, 10000)
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	// В теле указана чужая планета — она не должна использоваться.
	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p-other","type":"travel","title":"T","reward":500,"author_type":"player","author_id":"u1"}`))
	rec := httptest.NewRecorder()
	h.AdminCreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Админ-публикация игроком не с планеты (на орбите звезды) → 400, без записи.
func TestContractAdminCreatePlayerNotOnPlanet(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", `{"status":"orbit","object_type":"star","object_id":"w1","level":"orbit"}`, "player")

	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p1","type":"travel","title":"T","reward":500,"author_type":"player","author_id":"u1"}`))
	rec := httptest.NewRecorder()
	h.AdminCreateContract(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "игрок не на планете → 400")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Админ-публикация агентом: у агента нет планетного слоя — итерация 1 не
// поддерживает агента-автора → 400, без записи.
func TestContractAdminCreateAgentUnsupported(t *testing.T) {
	h, mock := newContractHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p1","type":"travel","title":"T","reward":500,"author_type":"agent","author_id":"a1"}`))
	rec := httptest.NewRecorder()
	h.AdminCreateContract(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "агент-автор не поддержан в итерации 1")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Админ-публикация постройкой: планета — buildings.planet_id, плательщик —
// владелец постройки (резолв §4 — запрос внутри транзакции публикации).
func TestContractAdminCreateBuilding(t *testing.T) {
	h, mock := newContractHarness(t)

	mock.ExpectQuery(`SELECT planet_id FROM buildings WHERE id = \$1`).
		WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"planet_id"}).AddRow("p1"))
	expectPlanetByID(mock, "p1", "w1")

	// Публикация: Begin → резолв плательщика-постройки → залог → контракт → лог.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT owner_type, owner_id FROM buildings WHERE id = \$1`).
		WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id"}).AddRow("faction", "f1"))
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("faction", "f1", int64(1000000000000000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs("faction", "f1", int64(300)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}).AddRow(1000, 0, 0))
	mock.ExpectExec(`INSERT INTO contracts`).
		WithArgs(sqlmock.AnyArg(), "travel", "building", "b1", "p1", "T", "",
			sqlmock.AnyArg(), int64(300), "regular", int64(300), int64(0),
			"deposit", "open", "public", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs("faction", "f1", int64(-300), sqlmock.AnyArg(), "escrow_lock", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectContractGetByID(mock, contractRows("c1", "open", "public", 300))

	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p-other","type":"travel","title":"T","reward":300,"author_type":"building","author_id":"b1"}`))
	rec := httptest.NewRecorder()
	h.AdminCreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}
