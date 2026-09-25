// internal/handlers/property_handlers_test.go
//
// GET /me/property (спека 2026-09-26-собственность-игрока-в-дашборде §5/§13):
// свои поселения и строения по owner_id из JWT (И-1), форма ответа (§5.1),
// детерминированный порядок (§5.3), режимы знания (§4.3), пусто → 200
// {"items":[]} (И-4), пакетность (И-2), сироты чужого владельца (И-5).
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
)

// ==================== ХЕЛПЕРЫ ====================

// newPropertyHarness — sqlmock-БД + PropertyHandlers на реальных репозиториях.
func newPropertyHarness(t *testing.T) (*PropertyHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewPropertyHandlers(
		repository.NewPropertyRepository(db),
		repository.NewUserRepository(db),
		repository.NewPlanetRepository(db),
	), mock
}

// expectPropertySettlements — ожидание выборки поселений по владельцу (И-1).
func expectPropertySettlements(mock sqlmock.Sqlmock, userID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id, COALESCE\(s\.race_id, ''\), COALESCE\(pt\.name, ''\)\s+FROM settlements s\s+LEFT JOIN producer_types pt ON pt\.id = s\.settlement_type_id\s+WHERE s\.owner_type = 'player' AND s\.owner_id = \$1`).
		WithArgs(userID).WillReturnRows(rows)
}

// expectPropertyBuildings — ожидание выборки строений по владельцу (И-1).
func expectPropertyBuildings(mock sqlmock.Sqlmock, userID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT b\.id, b\.planet_id, COALESCE\(pt\.name, ''\)\s+FROM buildings b\s+LEFT JOIN producer_types pt ON pt\.id = b\.producer_type_id\s+WHERE b\.owner_type = 'player' AND b\.owner_id = \$1`).
		WithArgs(userID).WillReturnRows(rows)
}

// expectPropertyNames — ожидание пакетного запроса имён планет/систем (И-2).
func expectPropertyNames(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT p\.id, p\.name, p\.world_id, COALESCE\(w\.name, ''\)\s+FROM planets p\s+LEFT JOIN worlds w ON w\.id = p\.world_id\s+WHERE p\.id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)
}

// expectPropertyKnowledge — ожидание пакетного запроса знания игрока (И-2).
func expectPropertyKnowledge(mock sqlmock.Sqlmock, userID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT planet_id, data, scanned_at\s+FROM player_planet_knowledge\s+WHERE user_id = \$1 AND planet_id = ANY\(\$2\)`).
		WithArgs(userID, sqlmock.AnyArg()).WillReturnRows(rows)
}

// propertySettlementRows — колонки выборки поселений (порядок Scan).
func propertySettlementRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "planet_id", "race_id", "type_name"})
}

// propertyBuildingRows — колонки выборки строений (порядок Scan).
func propertyBuildingRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "planet_id", "type_name"})
}

// propertyNameRows — колонки запроса имён.
func propertyNameRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "world_id", "world_name"})
}

// propertyKnowledgeRows — колонки запроса знания.
func propertyKnowledgeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"planet_id", "data", "scanned_at"})
}

// propertyResp — разобранный ответ (§5.1).
type propertyResp struct {
	Items []struct {
		Kind       string `json:"kind"`
		ID         string `json:"id"`
		Name       string `json:"name"`
		Subtitle   string `json:"subtitle"`
		PlanetID   string `json:"planet_id"`
		PlanetName string `json:"planet_name"`
		WorldID    string `json:"world_id"`
		WorldName  string `json:"world_name"`
		Knowledge  struct {
			Mode  string     `json:"mode"`
			At    *time.Time `json:"at"`
			Fresh bool       `json:"fresh"`
		} `json:"knowledge"`
	} `json:"items"`
}

// propertyRequest — GET /me/property.
func propertyRequest(userID string) *http.Request {
	return withUserID(httptest.NewRequest(http.MethodGet, "/me/property", nil), userID)
}

// ==================== T1: БЕЗ ТОКЕНА ====================

func TestGetMyPropertyUnauthorized(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := NewPropertyHandlers(
		repository.NewPropertyRepository(db),
		repository.NewUserRepository(db),
		repository.NewPlanetRepository(db),
	)
	rec := execJSON(h.GetMyProperty, httptest.NewRequest(http.MethodGet, "/me/property", nil))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T2/T3: ТОЛЬКО СВОЁ И ФОРМА ОТВЕТА ====================

func TestGetMyPropertyOnlyOwnAndShape(t *testing.T) {
	h, mock := newPropertyHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectPropertySettlements(mock, userID, propertySettlementRows().
		AddRow("s-mine", "p1", "", "Поселение"))
	expectPropertyBuildings(mock, userID, propertyBuildingRows().
		AddRow("b-mine", "p2", "Аутпост"))
	expectPropertyNames(mock, propertyNameRows().
		AddRow("p1", "Кеплер-3 b", "w1", "Кеплер-3").
		AddRow("p2", "Кеплер-3 c", "w1", "Кеплер-3"))
	expectPropertyKnowledge(mock, userID, propertyKnowledgeRows())

	rec := execJSON(h.GetMyProperty, propertyRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp propertyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)

	byID := map[string]int{}
	for i, it := range resp.Items {
		byID[it.ID] = i
	}
	require.Contains(t, byID, "s-mine")
	require.Contains(t, byID, "b-mine")

	settlement := resp.Items[byID["s-mine"]]
	assert.Equal(t, "settlement", settlement.Kind)
	assert.Equal(t, "Люди", settlement.Name, "пустой race_id → «Люди» (как в моделях)")
	assert.Equal(t, "Поселение", settlement.Subtitle)
	assert.Equal(t, "p1", settlement.PlanetID)
	assert.Equal(t, "Кеплер-3 b", settlement.PlanetName)
	assert.Equal(t, "w1", settlement.WorldID)
	assert.Equal(t, "Кеплер-3", settlement.WorldName)

	building := resp.Items[byID["b-mine"]]
	assert.Equal(t, "building", building.Kind)
	assert.Equal(t, "Аутпост", building.Name)
	assert.Equal(t, "Строение", building.Subtitle)
	assert.Equal(t, "p2", building.PlanetID)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T4: ПОРЯДОК СТРОК (§5.3) ====================

func TestGetMyPropertyDeterministicOrder(t *testing.T) {
	h, mock := newPropertyHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	// Поселения приходят в «сыром» порядке: s-beta (мир Бета) раньше s-alpha
	// (мир Альфа) — сервер обязан отсортировать по world/planet/name.
	expectPropertySettlements(mock, userID, propertySettlementRows().
		AddRow("s-beta", "p2", "", "Поселение").
		AddRow("s-alpha", "p1", "", "Поселение"))
	expectPropertyBuildings(mock, userID, propertyBuildingRows().
		AddRow("b-alpha", "p1", "Аутпост"))
	expectPropertyNames(mock, propertyNameRows().
		AddRow("p1", "Бета-1", "w1", "Альфа").
		AddRow("p2", "Альфа-2", "w2", "Бета"))
	expectPropertyKnowledge(mock, userID, propertyKnowledgeRows())

	rec := execJSON(h.GetMyProperty, propertyRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp propertyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 3)
	// Сначала поселения (по world/planet), затем строения.
	assert.Equal(t, []string{"s-alpha", "s-beta", "b-alpha"},
		[]string{resp.Items[0].ID, resp.Items[1].ID, resp.Items[2].ID})
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T5: ПУСТО → 200 {"items":[]} (И-4) ====================

func TestGetMyPropertyEmpty(t *testing.T) {
	h, mock := newPropertyHarness(t)
	const userID = "22222222-2222-2222-2222-222222222222"

	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectPropertySettlements(mock, userID, propertySettlementRows())
	expectPropertyBuildings(mock, userID, propertyBuildingRows())

	rec := execJSON(h.GetMyProperty, propertyRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, `{"items":[]}`, strings.TrimSpace(rec.Body.String()))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T6: РЕЖИМЫ ЗНАНИЯ (§4.3) ====================

func TestGetMyPropertyKnowledgeModes(t *testing.T) {
	h, mock := newPropertyHarness(t)
	const userID = "33333333-3333-3333-3333-333333333333"
	now := time.Now()
	snapshotAt := now.Add(-time.Hour)          // свежий снимок
	oldScanAt := now.Add(-30 * 24 * time.Hour) // протухший скан

	// Присутствие — на p1 (позиция — орбита планеты).
	expectPlayerUserWithPosition(mock, userID, "w1",
		`{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`)
	expectPropertySettlements(mock, userID, propertySettlementRows().
		AddRow("s-presence", "p1", "", "Поселение").
		AddRow("s-snapshot", "p2", "", "Поселение"))
	expectPropertyBuildings(mock, userID, propertyBuildingRows().
		AddRow("b-scan", "p3", "Аутпост").
		AddRow("b-none", "p4", "Аутпост"))
	expectPropertyNames(mock, propertyNameRows().
		AddRow("p1", "P1", "w1", "W").
		AddRow("p2", "P2", "w1", "W").
		AddRow("p3", "P3", "w1", "W").
		AddRow("p4", "P4", "w1", "W"))
	expectPropertyKnowledge(mock, userID, propertyKnowledgeRows().
		AddRow("p2", `{"snapshot":{"at":"`+snapshotAt.UTC().Format(time.RFC3339)+`"}}`, now).
		AddRow("p3", `{"settlements_count":1}`, oldScanAt))

	rec := execJSON(h.GetMyProperty, propertyRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp propertyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	byID := map[string]int{}
	for i, it := range resp.Items {
		byID[it.ID] = i
	}

	presence := resp.Items[byID["s-presence"]].Knowledge
	assert.Equal(t, "presence", presence.Mode)
	assert.Nil(t, presence.At, "presence — без даты")
	assert.False(t, presence.Fresh)

	snapshot := resp.Items[byID["s-snapshot"]].Knowledge
	assert.Equal(t, "snapshot", snapshot.Mode)
	require.NotNil(t, snapshot.At)
	assert.True(t, snapshot.Fresh, "свежий снимок — fresh")
	assert.True(t, snapshot.At.After(now.Add(-2*time.Hour)))

	scan := resp.Items[byID["b-scan"]].Knowledge
	assert.Equal(t, "scan", scan.Mode)
	require.NotNil(t, scan.At)
	assert.False(t, scan.Fresh, "протухший скан — не fresh")

	none := resp.Items[byID["b-none"]].Knowledge
	assert.Equal(t, "none", none.Mode, "mode выставляется явно")
	assert.Nil(t, none.At)
	assert.False(t, none.Fresh)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T7: НЕТ ЗНАНИЯ — НЕТ ДАННЫХ ПЛАНЕТЫ (И-3) ====================

func TestGetMyPropertyNoKnowledgeNoLeak(t *testing.T) {
	h, mock := newPropertyHarness(t)
	const userID = "44444444-4444-4444-4444-444444444444"

	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectPropertySettlements(mock, userID, propertySettlementRows().
		AddRow("s1", "p1", "", "Поселение"))
	expectPropertyBuildings(mock, userID, propertyBuildingRows())
	expectPropertyNames(mock, propertyNameRows().
		AddRow("p1", "P1", "w1", "W"))
	expectPropertyKnowledge(mock, userID, propertyKnowledgeRows())

	rec := execJSON(h.GetMyProperty, propertyRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "surface_dominant")
	assert.NotContains(t, rec.Body.String(), "population")
	assert.NotContains(t, rec.Body.String(), "stability")

	var resp propertyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "none", resp.Items[0].Knowledge.Mode)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T8: ПАКЕТНОСТЬ (И-2) ====================

// Число SQL-запросов не зависит от числа планет: на 3 планеты приходится ровно
// один запрос имён и один запрос знания. Лишний (per-planet) запрос сделал бы
// ожидания невыполненными и ответ 500.
func TestGetMyPropertyBatching(t *testing.T) {
	h, mock := newPropertyHarness(t)
	const userID = "55555555-5555-5555-5555-555555555555"

	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectPropertySettlements(mock, userID, propertySettlementRows().
		AddRow("s1", "p1", "", "Поселение").
		AddRow("s2", "p2", "", "Поселение").
		AddRow("s3", "p3", "", "Поселение"))
	expectPropertyBuildings(mock, userID, propertyBuildingRows().
		AddRow("b1", "p1", "Аутпост"))
	expectPropertyNames(mock, propertyNameRows().
		AddRow("p1", "P1", "w1", "W").
		AddRow("p2", "P2", "w1", "W").
		AddRow("p3", "P3", "w1", "W"))
	expectPropertyKnowledge(mock, userID, propertyKnowledgeRows())

	rec := execJSON(h.GetMyProperty, propertyRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp propertyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 4)
	require.NoError(t, mock.ExpectationsWereMet(),
		"ровно два запроса по владельцу + один имён + один знания")
}

// ==================== T9: СИРОТА ЧУЖОГО ВЛАДЕЛЬЦА (И-5) ====================

// Выборка строго по своему owner_id: в выдаче только своё строение, чужое в
// ответ не попадает.
func TestGetMyPropertyForeignOrphanNotReturned(t *testing.T) {
	h, mock := newPropertyHarness(t)
	const userID = "66666666-6666-6666-6666-666666666666"

	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectPropertySettlements(mock, userID, propertySettlementRows())
	expectPropertyBuildings(mock, userID, propertyBuildingRows().
		AddRow("b-mine", "p1", "Аутпост"))
	expectPropertyNames(mock, propertyNameRows().
		AddRow("p1", "P1", "w1", "W"))
	expectPropertyKnowledge(mock, userID, propertyKnowledgeRows())

	rec := execJSON(h.GetMyProperty, propertyRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp propertyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "b-mine", resp.Items[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}
