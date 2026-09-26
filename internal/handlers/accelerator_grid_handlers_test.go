// internal/handlers/accelerator_grid_handlers_test.go
// Тесты доски v9 «Планшет» (спека ускорителя §14.1/§14.3/§14.4): контракт
// offer (публичная доска без утечки secret/realized/содержимого) и ручка
// POST /api/accelerator/scan (вскрытие сектора со списанием импульса, отказы).
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/routegame"
	"zorion/internal/travel"
)

// accelGridTestPassport — паспорт сегмента w1(0,0)->w2(10,0) — в точности как
// у sqlmock-строк travelWorldRow (G/5772/star/single); тест собирает ТО ЖЕ
// поле, что сервер (dist — от старта сегмента).
func accelGridTestPassport() routegame.Passport {
	from := models.World{ID: "w1", CoordX: 0, CoordY: 0, SpectralClass: "G", Temperature: 5772, StarType: "star", SystemType: "single"}
	to := models.World{ID: "w2", CoordX: 10, CoordY: 0, SpectralClass: "G", Temperature: 5772, StarType: "star", SystemType: "single"}
	return routegame.BuildPassport(10, from, to, nil)
}

// accelGridTestField — поле сегмента тем же seed/secret/dist/паспортом, что строит
// сервер; тестовый секрет обязан давать принятое поле (гейт §14.2).
func accelGridTestField(t *testing.T, flight *travel.TravelInfo, secret []byte, dist float64, passport routegame.Passport) routegame.GridField {
	t.Helper()
	field, ok := routegame.GenerateGridField(acceleratorSegmentSeed(flight), secret, dist, passport)
	require.True(t, ok, "тестовый секрет даёт принятое поле")
	return field
}

// accelGridTestLayoutJSON — публичный layout поля (JSONB-значение для строки).
func accelGridTestLayoutJSON(t *testing.T, field routegame.GridField) []byte {
	t.Helper()
	raw, err := json.Marshal(field.Layout())
	require.NoError(t, err)
	return raw
}

// expectRoutePuzzle — ожидание SELECT состояния задачи player_route_puzzle.
func expectRoutePuzzle(mock sqlmock.Sqlmock, userID string, hash, secret, layout, revealed []byte, pings int) {
	mock.ExpectQuery(`SELECT user_id, kind, segment_hash, secret, layout, revealed, pings_left, created_at FROM player_route_puzzle WHERE user_id = \$1 AND kind = \$2`).
		WithArgs(userID, "route").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "kind", "segment_hash", "secret", "layout", "revealed", "pings_left", "created_at"}).
			AddRow(userID, "route", hash, secret, layout, revealed, pings, now()))
}

// newAccelScanRequest — POST /api/accelerator/scan с телом {fingerprint, sector}.
func newAccelScanRequest(userID, fingerprint string, sector int) *http.Request {
	body, err := json.Marshal(acceleratorScanRequest{Fingerprint: fingerprint, Sector: sector})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/accelerator/scan", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

// expectScanSegmentQueries — общий префикс БД scan: состояние, пользователь,
// миры, пояса, строка задачи.
func expectScanSegmentQueries(t *testing.T, mock sqlmock.Sqlmock, userID string, secret, layout []byte, revealed []byte, pings int, flight *travel.TravelInfo) {
	t.Helper()
	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)
	expectWorld(mock, "w1", 0, 0)
	expectBelts(mock, "w2")
	expectRoutePuzzle(mock, userID, acceleratorSegmentHash(flight), secret, layout, revealed, pings)
}

// ==================== OFFER: ПЕРЕСОЗДАНИЕ ЗАДАЧИ ====================

// Смена сегмента (другой segment_hash) → строка пересоздаётся: импульсы
// сбрасываются, вскрытие старого сегмента не переносится.
func TestAcceleratorOfferRecreatesPuzzleOnSegmentChange(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)
	expectWorld(mock, "w1", 0, 0)
	expectBelts(mock, "w2")
	// Старая строка другого сегмента → Replace (новый secret/layout, revealed='[]').
	expectRoutePuzzle(mock, userID, []byte("stale-segment-hash"), []byte("old-secret"), []byte("{}"), []byte(`[{"sector":3,"content":"trap"}]`), 0)
	mock.ExpectExec(`INSERT INTO player_route_puzzle`).
		WithArgs(userID, "route", acceleratorSegmentHash(flight), sqlmock.AnyArg(), sqlmock.AnyArg(), acceleratorGridPings).
		WillReturnResult(sqlmock.NewResult(1, 1))

	rec := execJSON(h.AcceleratorOffer, accelOfferRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp acceleratorOfferResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, acceleratorGridPings, resp.PingsLeft, "новый сегмент: импульсы сброшены")
	require.Empty(t, resp.Revealed, "вскрытие старого сегмента не переносится")
	require.NotEmpty(t, resp.Board.Sectors, "новая доска собрана")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== SCAN ====================

// Успех: сектор вскрыт серверным secret, импульс списан, revealed вернулся.
func TestAcceleratorScanSpendsPing(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	require.NotEmpty(t, field.Sectors)
	layoutJSON := accelGridTestLayoutJSON(t, field)
	sector := 0
	wantContent, err := routegame.RevealGridSector(secret, field.Layout(), sector)
	require.NoError(t, err)
	contentArg, err := json.Marshal(wantContent)
	require.NoError(t, err)

	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)
	mock.ExpectQuery(`UPDATE player_route_puzzle`).
		WithArgs(userID, "route", sector, string(contentArg)).
		WillReturnRows(sqlmock.NewRows([]string{"revealed", "pings_left"}).
			AddRow([]byte(`[{"sector":0,"content":`+string(contentArg)+`}]`), 1))

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, acceleratorFingerprint(flight), sector))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp acceleratorScanResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, wantContent, resp.Content)
	require.Equal(t, 1, resp.PingsLeft, "импульс списан")
	require.Len(t, resp.Revealed, 1)
	require.Equal(t, sector, resp.Revealed[0].Sector)
	require.Equal(t, wantContent, resp.Revealed[0].Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Повторное вскрытие того же сектора снова списывает импульс (решение подэтапа
// A: дедупликации нет; клиентская блокировка — подэтап B). Гейт «нет импульсов»
// безусловен.
func TestAcceleratorScanRepeatSpendsAnotherPing(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)
	sector := 0
	wantContent, err := routegame.RevealGridSector(secret, field.Layout(), sector)
	require.NoError(t, err)
	contentArg, err := json.Marshal(wantContent)
	require.NoError(t, err)
	prior := []byte(`[{"sector":0,"content":` + string(contentArg) + `}]`)

	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, prior, 1, flight)
	mock.ExpectQuery(`UPDATE player_route_puzzle`).
		WithArgs(userID, "route", sector, string(contentArg)).
		WillReturnRows(sqlmock.NewRows([]string{"revealed", "pings_left"}).
			AddRow([]byte(`[{"sector":0,"content":`+string(contentArg)+`},{"sector":0,"content":`+string(contentArg)+`}]`), 0))

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, acceleratorFingerprint(flight), sector))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp acceleratorScanResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.PingsLeft)
	require.Len(t, resp.Revealed, 2, "повтор добавил дубликат")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Импульсов нет → 409 no_pings (Reveal: ErrNoPingsLeft), сектор не вскрыт.
func TestAcceleratorScanNoPings(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)
	wantContent, err := routegame.RevealGridSector(secret, field.Layout(), 0)
	require.NoError(t, err)
	contentArg, err := json.Marshal(wantContent)
	require.NoError(t, err)

	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 0, flight)
	mock.ExpectQuery(`UPDATE player_route_puzzle`).
		WithArgs(userID, "route", 0, string(contentArg)).
		WillReturnError(sql.ErrNoRows)

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, acceleratorFingerprint(flight), 0))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, false, m["available"])
	require.Equal(t, "no_pings", m["reason"])
	require.Contains(t, m, "cooldown_remaining_s", "отказ в единой форме writeAcceleratorRefusal")
	require.Nil(t, m["cooldown_remaining_s"])
	require.NotContains(t, m, "pings_left")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Индекс сектора вне диапазона → 400 bad_sector, импульс не тратится.
func TestAcceleratorScanBadSector(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)

	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, acceleratorFingerprint(flight), len(field.Sectors)))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, false, m["available"])
	require.Equal(t, "bad_sector", m["reason"])
	require.Contains(t, m, "cooldown_remaining_s", "отказ в единой форме writeAcceleratorRefusal")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Устаревший fingerprint → 409 changed без обращений к БД (кроме памяти).
func TestAcceleratorScanFingerprintChanged(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, "w1:w2:0", 0))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "changed", m["reason"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Вне полёта → 409 no_flight без обращений к БД.
func TestAcceleratorScanNoFlight(t *testing.T) {
	h, _, mock := newAccelGameHarness(t)

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest("u1", "x", 0))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "no_flight", m["reason"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ускорение уже действует на сегменте → 409 already_active (разведка не идёт).
func TestAcceleratorScanAlreadyActive(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)

	expectAccelState(mock, userID, flight.StartTime, 25)
	expectAccelGameUser(mock, userID, "w1")

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, acceleratorFingerprint(flight), 0))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "already_active", m["reason"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Модуля нет → 409 no_module.
func TestAcceleratorScanNoModule(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)

	expectAccelState(mock, userID, nil, nil)
	expectUser(mock, userID, "w1") // оборудование без ускорителя

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, acceleratorFingerprint(flight), 0))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "no_module", m["reason"])
	require.NoError(t, mock.ExpectationsWereMet())
}
