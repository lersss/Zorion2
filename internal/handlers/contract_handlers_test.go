// internal/handlers/contract_handlers_test.go
//
// Контракты на слое API (спеки 2026-09-22-контракт-модель-сущности §4–§6,
// 2026-09-22-контракт-перелёт-и-доска §2). Тесты: гейт доски знанием планеты,
// ленивое истечение на чтении доски, фильтры доски (open+public), атомарное
// взятие, отмена только автором/open, публикация игроком и админом.
package handlers

import (
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
)

// newContractHarness — sqlmock-БД + ContractHandlers на реальных репозиториях.
// Каталог оборудования — дефолты (PITFALLS): проверка двигателя при взятии
// перелёта читает ship.HasEngine/EngineSpeed из in-memory каталога.
func newContractHarness(t *testing.T) (*ContractHandlers, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewContractHandlers(
		repository.NewContractRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewUserRepository(db),
		repository.NewKnowledgeRepository(db),
		repository.NewWorldRepository(db),
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
	// Публикация перелёта гарантирует gear/speed_factor le ref (§1.3).
	mock.ExpectExec(`INSERT INTO contract_requirements`).
		WithArgs(sqlmock.AnyArg(), 1, "gear", "speed_factor", "le", contractTravelGearSpeedFactorRef, nil, nil).
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
		expectContractGetByID(mock, contractRows("c1", "open", "public", 500))
		expectUser(mock, "u1", "w1") // travel: проверка двигателя при взятии (91a)
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)UPDATE contracts\s+SET status = 'taken'.*expires_at > NOW\(\)`).
			WithArgs("c1", "player", "u1", sqlmock.AnyArg(), nil).
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
		expectContractGetByID(mock, contractRows("c1", "open", "public", 500))
		expectUser(mock, "u1", "w1") // travel: проверка двигателя при взятии (91a)
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)UPDATE contracts\s+SET status = 'taken'.*expires_at > NOW\(\)`).
			WithArgs("c1", "player", "u1", sqlmock.AnyArg(), nil).
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
	expectWorld(mock, "w1", 100, 0)    // offer_window: коорд. систем перелёта
	expectWorld(mock, "w2", 100, 0)
	expectPublishChain(mock, "player", "u1", "player", "u1", "p1", 500, 10000)
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts",
		strings.NewReader(`{"planet_id":"p1","type":"travel","title":"T","reward":500,"payload":`+travelPayload+`}`)), "u1")
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
	expectWorld(mock, "w1", 100, 0)
	expectWorld(mock, "w2", 100, 0)
	expectPublishChain(mock, "faction", "f1", "faction", "f1", "p1", 300, 1000000000000000)
	expectContractGetByID(mock, contractRows("c1", "open", "public", 300))

	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p1","type":"travel","title":"T","reward":300,"author_type":"faction","author_id":"f1","payload":`+travelPayload+`}`))
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
	expectWorld(mock, "w1", 100, 0)
	expectWorld(mock, "w2", 100, 0)
	expectPublishChain(mock, "player", "u1", "player", "u1", "p1", 500, 10000)
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	// В теле указана чужая планета — она не должна использоваться.
	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p-other","type":"travel","title":"T","reward":500,"author_type":"player","author_id":"u1","payload":`+travelPayload+`}`))
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
	expectWorld(mock, "w1", 100, 0)
	expectWorld(mock, "w2", 100, 0)

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
	// Публикация перелёта гарантирует gear/speed_factor le ref (§1.3).
	mock.ExpectExec(`INSERT INTO contract_requirements`).
		WithArgs(sqlmock.AnyArg(), 1, "gear", "speed_factor", "le", contractTravelGearSpeedFactorRef, nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs("faction", "f1", int64(-300), sqlmock.AnyArg(), "escrow_lock", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectContractGetByID(mock, contractRows("c1", "open", "public", 300))

	req := httptest.NewRequest(http.MethodPost, "/admin/contracts", strings.NewReader(
		`{"planet_id":"p-other","type":"travel","title":"T","reward":300,"author_type":"building","author_id":"b1","payload":`+travelPayload+`}`))
	rec := httptest.NewRecorder()
	h.AdminCreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== B2a: перелёт ====================

// contractRowsTravel — строка contracts с payload перелёта (порядок contractColumns).
func contractRowsTravel(id, status, payload string) *sqlmock.Rows {
	n := now()
	return sqlmock.NewRows([]string{
		"id", "type", "author_type", "author_id", "publication_planet_id", "title", "description",
		"payload", "reward", "funding", "escrow_amount", "escrow_withdrawable", "escrow_kind",
		"status", "visibility", "direct_target_type", "direct_target_id", "executor_type",
		"executor_id", "taken_at", "expires_at", "created_at", "updated_at",
	}).AddRow(id, "travel", "player", "u2", "p1", "T", "", []byte(payload),
		500, "regular", 500, 0, "deposit", status, "public", nil, nil, nil, nil, nil, n, n, n)
}

// contractReqRowsGear — одно требование снаряжения (kind='gear').
func contractReqRowsGear(contractID, subject, op string, threshold float64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "contract_id", "pos", "kind", "subject", "op",
		"threshold_num", "threshold_text", "quantity"}).
		AddRow(int64(1), contractID, 1, "gear", subject, op, threshold, nil, nil)
}

// timeArg — sqlmock-матчер: аргумент — time.Time (перебазирование срока).
type timeArg struct{}

func (timeArg) Match(v driver.Value) bool { _, ok := v.(time.Time); return ok }

// timeWithin — sqlmock-матчер: аргумент — time.Time, отстоящий от now() примерно
// на want (допуск tol). Для проверки, что срок публикации перелёта вычислен, а
// не взят из тела.
type timeWithin struct {
	want time.Duration
	tol  time.Duration
}

func (m timeWithin) Match(v driver.Value) bool {
	t, ok := v.(time.Time)
	if !ok {
		return false
	}
	d := time.Until(t)
	return d >= m.want-m.tol && d <= m.want+m.tol
}

// expectContractGetByIDWithReq — чтение контракта + заданный набор требований.
func expectContractGetByIDWithReq(mock sqlmock.Sqlmock, row, reqs *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT id, type, author_type, author_id, publication_planet_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(row)
	mock.ExpectQuery(`SELECT id, contract_id, pos, kind, subject, op,`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(reqs)
}

const travelPayload = `{"from_world_id":"w1","dest_world_id":"w2","dest_planet_id":null}`

// Требование к двигателю проверяется при взятии (спека §1.3): подходит (le 0.3,
// engine_1=0.3) → 200 + перебазирование срока; не подходит (le 0.2) → 400.
func TestContractTakeTravelEngineRequirement(t *testing.T) {
	t.Run("двигатель подходит и срок перебазируется", func(t *testing.T) {
		h, mock := newContractHarness(t)
		expectContractGetByIDWithReq(mock,
			contractRowsTravel("c1", "open", travelPayload),
			contractReqRowsGear("c1", "speed_factor", "le", 0.3))
		expectUser(mock, "u1", "w1") // установлен engine_1
		expectWorld(mock, "w1", 0, 0)
		expectWorld(mock, "w2", 100, 0)
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)UPDATE contracts\s+SET status = 'taken'.*expires_at = COALESCE\(\$5::timestamptz, expires_at\)`).
			WithArgs("c1", "player", "u1", sqlmock.AnyArg(), timeArg{}).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/take",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.TakeContract(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("двигатель не подходит — контракт не берётся", func(t *testing.T) {
		h, mock := newContractHarness(t)
		expectContractGetByIDWithReq(mock,
			contractRowsTravel("c1", "open", travelPayload),
			contractReqRowsGear("c1", "speed_factor", "le", 0.2))
		expectUser(mock, "u1", "w1") // engine_1=0.3 > 0.2

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/take",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.TakeContract(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code, "медленный двигатель → 400")
		require.Contains(t, rec.Body.String(), "Двигатель")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("без двигателя — контракт не берётся", func(t *testing.T) {
		h, mock := newContractHarness(t)
		expectContractGetByIDWithReq(mock,
			contractRowsTravel("c1", "open", travelPayload),
			contractReqRowsGear("c1", "speed_factor", "le", contractTravelGearSpeedFactorRef))
		// Игрок без двигателя (role=player): полёт невозможен (91a §6.1), и
		// дефолт EngineSpeed 0.3 не должен «пропускать» его через le 0.3.
		mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
			WithArgs("u1").
			WillReturnRows(userRowNoEngine("u1", "w1"))

		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/take",
			strings.NewReader(`{"contract_id":"c1"}`)), "u1")
		rec := httptest.NewRecorder()
		h.TakeContract(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code, "без двигателя перелёт не взять (91a §6.1)")
		require.Contains(t, rec.Body.String(), "Двигатель")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// Контракт не найден → 404 (до проверок требования/срока).
func TestContractTakeNotFound(t *testing.T) {
	h, mock := newContractHarness(t)
	mock.ExpectQuery(`SELECT id, type, author_type, author_id, publication_planet_id`).
		WithArgs("c404").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "type", "author_type", "author_id", "publication_planet_id", "title", "description",
			"payload", "reward", "funding", "escrow_amount", "escrow_withdrawable", "escrow_kind",
			"status", "visibility", "direct_target_type", "direct_target_id", "executor_type",
			"executor_id", "taken_at", "expires_at", "created_at", "updated_at",
		}))

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/take",
		strings.NewReader(`{"contract_id":"c404"}`)), "u1")
	rec := httptest.NewRecorder()
	h.TakeContract(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Сроки перелёта (спека §4.3): срок исполнения = flight_time_ref·1.5,
// окно предложения = flight_time_ref·10; flight_time_ref = max(dist·0.3, 3с).
// Формула вынесена в models (единый источник с NPC-путём).
func TestTravelDeadlineFormula(t *testing.T) {
	// dist=100 → flight 30с → срок 45с, окно 300с.
	require.Equal(t, 45*time.Second, models.TravelDeadline(100))
	require.Equal(t, 300*time.Second, models.TravelOfferWindow(100))
	// dist=1 → min 3с → срок 4.5с, окно 30с.
	require.Equal(t, 4500*time.Millisecond, models.TravelDeadline(1))
	require.Equal(t, 30*time.Second, models.TravelOfferWindow(1))
}

// Публикация перелёта валидирует payload (спека §1.2): from_world_id и
// dest_world_id обязательны; dest_planet_id опционален (=null).
func TestContractCreateTravelPayloadValidation(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"нет dest_world_id", `{"planet_id":"p1","type":"travel","title":"T","reward":500,"payload":{"from_world_id":"w1"}}`, true},
		{"нет from_world_id", `{"planet_id":"p1","type":"travel","title":"T","reward":500,"payload":{"dest_world_id":"w2"}}`, true},
		{"нет payload", `{"planet_id":"p1","type":"travel","title":"T","reward":500}`, true},
		{"валидный с null", `{"planet_id":"p1","type":"travel","title":"T","reward":500,"payload":{"from_world_id":"w1","dest_world_id":"w2","dest_planet_id":null}}`, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h, mock := newContractHarness(t)
			expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
			expectPlanetByID(mock, "p1", "w1")
			if !tt.wantErr {
				expectPlanetByID(mock, "p1", "w1") // проверка в publish
				expectWorld(mock, "w1", 100, 0)
				expectWorld(mock, "w2", 100, 0)
				expectPublishChain(mock, "player", "u1", "player", "u1", "p1", 500, 10000)
				expectContractGetByID(mock, contractRows("c1", "open", "public", 500))
			}
			req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts", strings.NewReader(tt.body)), "u1")
			rec := httptest.NewRecorder()
			h.CreateContract(rec, req)

			if tt.wantErr {
				require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
			} else {
				require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Публикация перелёта форсит свойства типа (§1.3, §6 R8): сервер сам добавляет
// gear/speed_factor le ref и выставляет funding='regular', даже если в теле
// прислали contract_work и не прислали requirements. Оба факта проверяет
// expectPublishChain (жёсткий funding="regular" + вставка авто-требования).
func TestContractTravelForcesGearRequirementAndFunding(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectPlanetByID(mock, "p1", "w1")
	expectPlanetByID(mock, "p1", "w1")
	expectWorld(mock, "w1", 100, 0)
	expectWorld(mock, "w2", 100, 0)
	expectPublishChain(mock, "player", "u1", "player", "u1", "p1", 500, 10000)
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	body := `{"planet_id":"p1","type":"travel","title":"T","reward":500,"funding":"contract_work","payload":` + travelPayload + `}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	h.CreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Срок публикации перелёта выводится и НЕ перекрывается телом (спека §4.3):
// клиентский expires_at в будущем игнорируется — в INSERT уходит вычисленный
// offer_window (dist=100 → 300с), а не значение из тела.
func TestContractTravelOfferWindowIgnoresClientExpiry(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectPlanetByID(mock, "p1", "w1")
	expectPlanetByID(mock, "p1", "w1")
	expectWorld(mock, "w1", 0, 0)
	expectWorld(mock, "w2", 100, 0)

	clientExpiry := "2099-01-01T00:00:00Z" // заведомо в будущем и далеко
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("player", "u1", int64(10000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs("player", "u1", int64(500)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}).AddRow(1000, 0, 0))
	mock.ExpectExec(`INSERT INTO contracts`).
		WithArgs(sqlmock.AnyArg(), "travel", "player", "u1", "p1", "T", "",
			sqlmock.AnyArg(), int64(500), "regular", int64(500), int64(0),
			"deposit", "open", "public", nil, nil,
			timeWithin{want: models.TravelOfferWindow(100), tol: 20 * time.Second},
			sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_requirements`).
		WithArgs(sqlmock.AnyArg(), 1, "gear", "speed_factor", "le", contractTravelGearSpeedFactorRef, nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	body := `{"planet_id":"p1","type":"travel","title":"T","reward":500,"expires_at":"` + clientExpiry + `","payload":` + travelPayload + `}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	h.CreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Явное gear/speed_factor требование публикатора уважается (не перезаписывается
// авто): порог 0.25 сохраняется, второго требования не добавляется (§1.3).
func TestContractTravelPreservesExplicitGearRequirement(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectPlanetByID(mock, "p1", "w1")
	expectPlanetByID(mock, "p1", "w1")
	expectWorld(mock, "w1", 100, 0)
	expectWorld(mock, "w2", 100, 0)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("player", "u1", int64(10000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs("player", "u1", int64(500)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}).AddRow(1000, 0, 0))
	mock.ExpectExec(`INSERT INTO contracts`).
		WithArgs(sqlmock.AnyArg(), "travel", "player", "u1", "p1", "T", "",
			sqlmock.AnyArg(), int64(500), "regular", int64(500), int64(0),
			"deposit", "open", "public", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_requirements`).
		WithArgs(sqlmock.AnyArg(), 1, "gear", "speed_factor", "le", 0.25, nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	body := `{"planet_id":"p1","type":"travel","title":"T","reward":500,"payload":` + travelPayload +
		`,"requirements":[{"kind":"gear","subject":"speed_factor","op":"le","threshold_num":0.25}]}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	h.CreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Не-перелёт не затронут: funding из тела сохраняется (contract_work), авто
// gear-требования нет (B1-поведение). Отсутствие вставки требования закреплено
// ordered-матчингом (следующий ожидаемый шаг — contract_log).
func TestContractNonTravelKeepsFundingAndNoGearRequirement(t *testing.T) {
	h, mock := newContractHarness(t)

	expectSurfaceUserRole(mock, "u1", "w1", contractPlanetPos, "player")
	expectPlanetByID(mock, "p1", "w1")
	expectPlanetByID(mock, "p1", "w1")

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("player", "u1", int64(10000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs("player", "u1", int64(500)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}).AddRow(1000, 0, 0))
	mock.ExpectExec(`INSERT INTO contracts`).
		WithArgs(sqlmock.AnyArg(), "supply", "player", "u1", "p1", "T", "",
			sqlmock.AnyArg(), int64(500), "contract_work", int64(500), int64(0),
			"deposit", "open", "public", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectContractGetByID(mock, contractRows("c1", "open", "public", 500))

	body := `{"planet_id":"p1","type":"supply","title":"T","reward":500,"funding":"contract_work"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	h.CreateContract(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Взятие на двигателе хуже референсного (авто-требование le 0.3) → 400: каталог
// подменён двигателем speed_factor 0.5 (> ref), затем восстановлен.
func TestContractTakeRejectsSlowerThanRefRequirement(t *testing.T) {
	h, mock := newContractHarness(t) // грузит дефолтный каталог

	// Подменяем каталог двигателем медленнее референсного (0.5 > 0.3).
	catDB, catMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { catDB.Close() })
	catMock.ExpectQuery(`SELECT id, type, name, params FROM equipment`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "name", "params"}).
			AddRow("engine_1", "engine", "Двигатель-1", []byte(`{"speed_factor":0.5}`)))
	require.NoError(t, ship.LoadCatalog(catDB))
	require.NoError(t, catMock.ExpectationsWereMet())
	t.Cleanup(ship.LoadDefaults)

	expectContractGetByIDWithReq(mock,
		contractRowsTravel("c1", "open", travelPayload),
		contractReqRowsGear("c1", "speed_factor", "le", contractTravelGearSpeedFactorRef))
	expectUser(mock, "u1", "w1")

	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/contracts/take",
		strings.NewReader(`{"contract_id":"c1"}`)), "u1")
	rec := httptest.NewRecorder()
	h.TakeContract(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "двигатель хуже ref → контракт не берётся")
	require.Contains(t, rec.Body.String(), "Двигатель")
	require.NoError(t, mock.ExpectationsWereMet())
}
