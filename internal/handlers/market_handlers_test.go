// internal/handlers/market_handlers_test.go
//
// Витрина локального рынка планеты (спека 2026-09-24-магазин-модулей-локальный-
// рынок §7.1/§13): гейт знанием (none → 404 «Нет данных», settlements_count == 0
// → 200 offers:[], > 0 → каталог), can_trade (орбита планеты/спутника — true;
// surface/вне орбиты — false), 404 для несуществующей планеты.
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// newMarketHarness — sqlmock-БД + MarketHandlers на реальных репозиториях.
// Каталог оборудования — дефолты (PITFALLS): разворот item в витрине читает
// ship.EquipmentByID из in-memory каталога.
func newMarketHarness(t *testing.T) (*MarketHandlers, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	ship.LoadModelDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewMarketHandlers(
		db,
		repository.NewMarketRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewUserRepository(db),
		repository.NewKnowledgeRepository(db),
		travel.NewManager(nil),
	), mock
}

// marketOffersRows — 4 базовых модуля витрины (порядок колонок ListOffers).
func marketOffersRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "kind", "item_id", "price"}).
		AddRow(int64(1), "module", "cargo_1", int64(3000)).
		AddRow(int64(2), "module", "engine_1", int64(3000)).
		AddRow(int64(3), "module", "radar_1", int64(3000)).
		AddRow(int64(4), "module", "scanner_1", int64(3000))
}

// expectMarketOffers — ожидание ListOffers (4 базовых модуля).
func expectMarketOffers(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, kind, item_id, price FROM market_offers ORDER BY id ASC`).
		WillReturnRows(marketOffersRows())
}

// marketRequest — GET /api/planets/{planetID}/market с ролью player.
func marketRequest(userID, planetID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/planets/"+planetID+"/market", nil)
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))
	return req
}

// expectAdminUserWithPosition — ожидание GetByIDWithPosition админа
// (роль admin; позиция — как у игрока, чтобы тест проверял именно роль).
func expectAdminUserWithPosition(mock sqlmock.Sqlmock, id, worldID string, posRaw interface{}) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination, race_id FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id",
			"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "current_position", "pending_destination", "race_id",
		}).AddRow(id, "admin", "hash", nil, nil, worldID, "ship_strela.svg", nil, "starter",
			`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, "admin", now(), now(), posRaw, nil, nil))
}

// marketResp — разобранный ответ витрины (§7.1).
type marketResp struct {
	Offers   []models.MarketOffer `json:"offers"`
	CanTrade bool                 `json:"can_trade"`
}

// Без знания планеты витрина не отдаётся — 404 «Нет данных» (offers/can_trade
// не отдаются, режим none).
func TestMarketRequiresKnowledge(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectNoKnowledge(mock, "u1", "p1")

	rec := httptest.NewRecorder()
	h.GetMarket(rec, marketRequest("u1", "p1"))

	require.Equal(t, http.StatusNotFound, rec.Code, "нет знания → 404 «Нет данных»")
	require.NotContains(t, rec.Body.String(), "offers", "offers не отдаются в режиме none")
	require.NotContains(t, rec.Body.String(), "can_trade", "can_trade не отдаётся в режиме none")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Знание есть, но поселений нет (settlements_count == 0) — 200 offers:[],
// рынка нет по знанию, не по живости сейчас.
func TestMarketNoSettlements(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectModesKnowledge(mock, "u1", "p1", `{"settlements_count": 0}`, now())

	rec := httptest.NewRecorder()
	h.GetMarket(rec, marketRequest("u1", "p1"))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Empty(t, resp.Offers, "рынка нет по знанию → offers: []")
	require.False(t, resp.CanTrade)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Знание есть и поселения есть — 200 с каталогом 4 модулей (item развёрнут
// из каталога оборудования).
func TestMarketCatalog(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectModesKnowledge(mock, "u1", "p1", `{"settlements_count": 1}`, now())
	expectMarketOffers(mock)
	expectPlayerUserWithPosition(mock, "u1", "w1", nil) // позиции нет → can_trade false

	rec := httptest.NewRecorder()
	h.GetMarket(rec, marketRequest("u1", "p1"))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Offers, 4)
	require.Equal(t, "cargo_1", resp.Offers[0].Item.ID)
	require.Equal(t, models.EquipmentTypeCargo, resp.Offers[0].Item.Type)
	require.Equal(t, int64(3000), resp.Offers[0].Price)
	require.False(t, resp.CanTrade, "вне орбиты → can_trade false")
	require.NoError(t, mock.ExpectationsWereMet())
}

// can_trade: орбита ЭТОЙ планеты → true.
func TestMarketCanTradeOrbit(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectModesKnowledge(mock, "u1", "p1", `{"settlements_count": 1}`, now())
	expectMarketOffers(mock)
	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)

	rec := httptest.NewRecorder()
	h.GetMarket(rec, marketRequest("u1", "p1"))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.CanTrade, "орбита планеты → can_trade true")
	require.NoError(t, mock.ExpectationsWereMet())
}

// can_trade: орбита спутника планеты → true (присутствие по родителю, §5.5).
func TestMarketCanTradeSatellite(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectModesKnowledge(mock, "u1", "p1", `{"settlements_count": 1}`, now())
	expectMarketOffers(mock)
	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"satellite","object_id":"sat1"}`)
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 AND EXISTS`).
		WithArgs("w1", "sat1").
		WillReturnRows(travelPlanetRow("p1", "w1", `{}`))

	rec := httptest.NewRecorder()
	h.GetMarket(rec, marketRequest("u1", "p1"))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.CanTrade, "орбита спутника → присутствие на родителе → can_trade true")
	require.NoError(t, mock.ExpectationsWereMet())
}

// can_trade: поверхность планеты НЕ даёт (решение создателя §7.1).
func TestMarketNoCanTradeSurface(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectModesKnowledge(mock, "u1", "p1", `{"settlements_count": 1}`, now())
	expectMarketOffers(mock)
	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"surface","object_type":"planet","object_id":"p1"}`)

	rec := httptest.NewRecorder()
	h.GetMarket(rec, marketRequest("u1", "p1"))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.CanTrade, "surface не даёт can_trade")
	require.NoError(t, mock.ExpectationsWereMet())
}

// can_trade: админ на орбите планеты с знанием → false (спека §7.1:113,
// §13: «false ... для админа»; у админа нет игрового присутствия). Витрина
// при этом видна — 200 с каталогом.
func TestMarketNoCanTradeAdmin(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlanetByID(mock, "p1", "w1")
	expectModesKnowledge(mock, "u1", "p1", `{"settlements_count": 1}`, now())
	expectMarketOffers(mock)
	expectAdminUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/planets/p1/market", nil)
	req = withUserID(req, "u1")
	req = withRole(req, string(models.RoleAdmin))
	h.GetMarket(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Offers, 4, "витрина видна админу")
	require.False(t, resp.CanTrade, "админ → can_trade false")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Несуществующая планета — 404.
func TestMarketPlanetNotFound(t *testing.T) {
	h, mock := newMarketHarness(t)

	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs("p999").
		WillReturnError(sql.ErrNoRows)

	rec := httptest.NewRecorder()
	h.GetMarket(rec, marketRequest("u1", "p999"))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== POST buy (§7.2) ====================

// marketBuyRequest — POST /api/planets/{planetID}/market/buy с ролью player.
func marketBuyRequest(userID, planetID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/planets/"+planetID+"/market/buy", strings.NewReader(body))
	req = withUserID(req, userID)
	req = withRole(req, string(models.RolePlayer))
	return req
}

// marketBuyResp — разобранный ответ покупки (§7.2 п.9).
type marketBuyResp struct {
	OK      bool   `json:"ok"`
	Amount  int64  `json:"amount"`
	Tradein int64  `json:"tradein"`
	Slot    string `json:"slot"`
	ItemID  string `json:"item_id"`
}

// expectLiveSettlement — ожидание гейта живости поселения (§7.2 п.2).
func expectLiveSettlement(mock sqlmock.Sqlmock, planetID string) {
	mock.ExpectQuery(`SELECT 1 FROM settlements WHERE planet_id = \$1 AND population_exact > 0 LIMIT 1`).
		WithArgs(planetID).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
}

// expectOfferByID — ожидание предложения по id (§7.2 п.3).
func expectOfferByID(mock sqlmock.Sqlmock, id int64, kind, itemID string, price int64) {
	mock.ExpectQuery(`SELECT id, kind, item_id, price FROM market_offers WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kind", "item_id", "price"}).
			AddRow(id, kind, itemID, price))
}

// expectBuyLockUser — ожидание лока строки игрока + её equipment/ship_model_id (§7.2 п.0).
func expectBuyLockUser(mock sqlmock.Sqlmock, userID, equipJSON string, modelID interface{}) {
	mock.ExpectQuery(`SELECT equipment, ship_model_id FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"equipment", "ship_model_id"}).AddRow(equipJSON, modelID))
}

// expectTradein — ожидание цены старого модуля из market_offers (§7.2 п.5).
func expectTradein(mock sqlmock.Sqlmock, itemID string, price int64) {
	mock.ExpectQuery(`SELECT price FROM market_offers WHERE kind = 'module' AND item_id = \$1 ORDER BY id ASC LIMIT 1`).
		WithArgs(itemID).
		WillReturnRows(sqlmock.NewRows([]string{"price"}).AddRow(price))
}

// expectDebit — ожидание списания с проверкой достатка и урезанием withdrawable
// (§7.2 п.6: паттерн «прочая трата» — проверка + списание одним оператором).
func expectDebit(mock sqlmock.Sqlmock, userID string, amount, balanceAfter int64) {
	mock.ExpectQuery(`UPDATE accounts[\s\S]*withdrawable = LEAST\(withdrawable, balance - \$2\)[\s\S]*balance >= \$2[\s\S]*RETURNING balance`).
		WithArgs(userID, amount).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(balanceAfter))
}

// expectDebitNoFunds — списание не прошло (0 строк) → 409.
func expectDebitNoFunds(mock sqlmock.Sqlmock, userID string, amount int64) {
	mock.ExpectQuery(`UPDATE accounts[\s\S]*RETURNING balance`).
		WithArgs(userID, amount).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}))
}

// expectInstall — ожидание установки модуля в слот через jsonb_set (§7.2 п.7).
func expectInstall(mock sqlmock.Sqlmock, userID, slot, itemID string) {
	mock.ExpectExec(`UPDATE users[\s\S]*jsonb_set`).
		WithArgs(userID, slot, itemID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectJournal — ожидание записи журнала kind='purchase' (§7.2 п.8).
func expectJournal(mock sqlmock.Sqlmock, userID string, delta, balanceAfter int64) {
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs(userID, delta, balanceAfter, "purchase").
		WillReturnResult(sqlmock.NewResult(1, 1))
}

// Успех: пустой слот — полная цена, установка в слот, журнал purchase.
func TestMarketBuyEmptySlot(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 3, "module", "radar_1", 3000)
	mock.ExpectBegin()
	expectBuyLockUser(mock, "u1", `{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, "starter")
	expectDebit(mock, "u1", 3000, 7000)
	expectInstall(mock, "u1", "universal2", "radar_1")
	expectJournal(mock, "u1", -3000, 7000)
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":3,"slot":"universal2"}`))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketBuyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.OK)
	require.Equal(t, int64(3000), resp.Amount, "пустой слот → полная цена")
	require.Equal(t, int64(0), resp.Tradein)
	require.Equal(t, "universal2", resp.Slot)
	require.Equal(t, "radar_1", resp.ItemID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Успех: занятый слот — трейд-ин floor(price/2) из market_offers.
func TestMarketBuyTradeIn(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 3, "module", "radar_1", 3000)
	mock.ExpectBegin()
	expectBuyLockUser(mock, "u1", `{"radar":"radar_1","universal":"cargo_1"}`, "starter")
	expectTradein(mock, "cargo_1", 3000)
	expectDebit(mock, "u1", 1500, 8500)
	expectInstall(mock, "u1", "universal", "radar_1")
	expectJournal(mock, "u1", -1500, 8500)
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":3,"slot":"universal"}`))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketBuyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(1500), resp.Amount, "3000 − tradein 1500")
	require.Equal(t, int64(1500), resp.Tradein)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Успех: модуль в слоте отсутствует в market_offers → выкуп 0 (полная цена).
func TestMarketBuyTradeInZero(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 1, "module", "cargo_1", 3000)
	mock.ExpectBegin()
	expectBuyLockUser(mock, "u1", `{"universal":"legacy_mod"}`, "starter")
	mock.ExpectQuery(`SELECT price FROM market_offers WHERE kind = 'module' AND item_id = \$1 ORDER BY id ASC LIMIT 1`).
		WithArgs("legacy_mod").
		WillReturnError(sql.ErrNoRows)
	expectDebit(mock, "u1", 3000, 7000)
	expectInstall(mock, "u1", "universal", "cargo_1")
	expectJournal(mock, "u1", -3000, 7000)
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":1,"slot":"universal"}`))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp marketBuyResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(0), resp.Tradein, "модуль вне витрины → выкуп 0")
	require.Equal(t, int64(3000), resp.Amount)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 403: не на орбите планеты (поверхность) — до обращения к рынку.
func TestMarketBuyForbiddenNotOrbit(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"surface","object_type":"planet","object_id":"p1"}`)

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":1,"slot":"universal"}`))

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "орбиты")
	require.NoError(t, mock.ExpectationsWereMet())
}

// 409: рынок закрыт — живого поселения нет (§7.2 п.2).
func TestMarketBuyMarketClosed(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	mock.ExpectQuery(`SELECT 1 FROM settlements WHERE planet_id = \$1 AND population_exact > 0 LIMIT 1`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}))

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":1,"slot":"universal"}`))

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "Рынок закрыт")
	require.NoError(t, mock.ExpectationsWereMet())
}

// 404: предложения нет (§7.2 п.3).
func TestMarketBuyOfferNotFound(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	mock.ExpectQuery(`SELECT id, kind, item_id, price FROM market_offers WHERE id = \$1`).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kind", "item_id", "price"}))

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":999,"slot":"universal"}`))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "Предложение не найдено")
	require.NoError(t, mock.ExpectationsWereMet())
}

// 404: модуль предложения отсутствует в каталоге equipment (§7.2 п.3).
func TestMarketBuyOfferModuleMissing(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 9, "module", "radar_99", 3000)

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":9,"slot":"universal"}`))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 400: невалидный слот — ключ (не универсальный) — до обращения к БД.
func TestMarketBuyBadSlotKey(t *testing.T) {
	h, mock := newMarketHarness(t)

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":1,"slot":"radar"}`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 400: невалидное тело.
func TestMarketBuyBadBody(t *testing.T) {
	h, mock := newMarketHarness(t)

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{not-json`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 400: номер слота > slots.universal модели игрока (starter → 3) — под локом.
func TestMarketBuySlotTooBig(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 1, "module", "cargo_1", 3000)
	mock.ExpectBegin()
	expectBuyLockUser(mock, "u1", `{}`, "starter")
	mock.ExpectRollback()

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":1,"slot":"universal9"}`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "слот")
	require.NoError(t, mock.ExpectationsWereMet())
}

// 409: недостаточно средств — списание не прошло (0 строк, §7.2 п.6).
func TestMarketBuyInsufficientFunds(t *testing.T) {
	h, mock := newMarketHarness(t)

	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 1, "module", "cargo_1", 3000)
	mock.ExpectBegin()
	expectBuyLockUser(mock, "u1", `{}`, "starter")
	expectDebitNoFunds(mock, "u1", 3000)
	mock.ExpectRollback()

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, marketBuyRequest("u1", "p1", `{"offer_id":1,"slot":"universal"}`))

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "Недостаточно средств")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Сериализация (§7.2 п.0): вторая покупка в тот же слот видит оборудование
// ПОСЛЕ первой установки и считает трейд-ин по обновлённому слоту (не теряет
// выкуп). Лок строки игрока (SELECT ... FOR UPDATE) — точка сериализации;
// истинную параллельность мок не воспроизводит (блокировок нет) — механика
// «вторая видит обновлённый слот» проверяется двумя последовательными вызовами.
func TestMarketBuySecondSeesUpdatedSlot(t *testing.T) {
	h, mock := newMarketHarness(t)

	// Первая покупка: слот universal2 пуст → полная цена 3000.
	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 3, "module", "radar_1", 3000)
	mock.ExpectBegin()
	expectBuyLockUser(mock, "u1", `{"engine":"engine_1"}`, "starter")
	expectDebit(mock, "u1", 3000, 7000)
	expectInstall(mock, "u1", "universal2", "radar_1")
	expectJournal(mock, "u1", -3000, 7000)
	mock.ExpectCommit()

	rec1 := httptest.NewRecorder()
	h.BuyMarket(rec1, marketBuyRequest("u1", "p1", `{"offer_id":3,"slot":"universal2"}`))
	require.Equal(t, http.StatusOK, rec1.Code)
	var r1 marketBuyResp
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &r1))
	require.Equal(t, int64(0), r1.Tradein, "первая покупка — пустой слот")

	// Вторая покупка в тот же слот: он теперь занят radar_1 (радар из витрины,
	// 3000) → трейд-ин 1500, и мы покупаем другой модуль (scanner_1).
	expectPlayerUserWithPosition(mock, "u1", "w1", `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectLiveSettlement(mock, "p1")
	expectOfferByID(mock, 4, "module", "scanner_1", 3000)
	mock.ExpectBegin()
	expectBuyLockUser(mock, "u1", `{"engine":"engine_1","universal2":"radar_1"}`, "starter")
	expectTradein(mock, "radar_1", 3000)
	expectDebit(mock, "u1", 1500, 5500)
	expectInstall(mock, "u1", "universal2", "scanner_1")
	expectJournal(mock, "u1", -1500, 5500)
	mock.ExpectCommit()

	rec2 := httptest.NewRecorder()
	h.BuyMarket(rec2, marketBuyRequest("u1", "p1", `{"offer_id":4,"slot":"universal2"}`))
	require.Equal(t, http.StatusOK, rec2.Code)
	var r2 marketBuyResp
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &r2))
	require.Equal(t, int64(1500), r2.Tradein, "вторая покупка видит обновлённый слот → трейд-ин 1500")
	require.Equal(t, int64(1500), r2.Amount)
	require.NoError(t, mock.ExpectationsWereMet())
}