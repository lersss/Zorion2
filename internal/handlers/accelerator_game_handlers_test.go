// internal/handlers/accelerator_game_handlers_test.go
// Тесты ручек мини-игры «Прокладка маршрута» (спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §3.4/§4.1, ЧК3, §9):
// offer (доступность, порог показа, field/passport, детерминизм поля) и boost
// (порог отправки, fingerprint, невалидный путь, мир-цель, отказы Booster без
// списания), контракт 409 без field/passport.
package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/routegame"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// ==================== ХЕЛПЕРЫ ====================

// accelGameUserRow — игрок со стартовым ускорителем (route зарегистрирована).
func accelGameUserRow(id, fromWorld string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "password_hash", "email", "agent_id", "current_world_id",
		"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "race_id",
	}).AddRow(id, "player", "hash", nil, nil, fromWorld, "ship_strela.svg", nil, nil,
		`{"accelerator":"accel_1","engine":"engine_1"}`, "player", now(), now(), nil)
}

// expectAccelGameUser — ожидание SELECT пользователя с ускорителем.
func expectAccelGameUser(mock sqlmock.Sqlmock, id, fromWorld string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(accelGameUserRow(id, fromWorld))
}

// expectAccelState — ожидание чтения состояния отката.
func expectAccelState(mock sqlmock.Sqlmock, userID string, lastBoostAt, lastCooldown interface{}) {
	mock.ExpectQuery(`SELECT last_boost_at, last_cooldown_min FROM player_accelerator WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"last_boost_at", "last_cooldown_min"}).AddRow(lastBoostAt, lastCooldown))
}

// expectWorldMissing — ожидание SELECT мира, которого нет (пакман съел).
func expectWorldMissing(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(id).
		WillReturnError(sql.ErrNoRows)
}

// accelFakeBooster — заглушка travel.Booster: управляемый исход ApplyBoost.
type accelFakeBooster struct {
	applied bool
	reason  string
	err     error
	calls   int
}

func (b *accelFakeBooster) ApplyBoost(userID string, expectStartTime time.Time, seg models.PlayerFlight, cooldownMin int, now time.Time) (bool, string, error) {
	b.calls++
	return b.applied, b.reason, b.err
}

// newAccelGameHarness — TravelHandlers с sqlmock-БД, planetRepo (пояса) и
// accelRepo (состояние отката) — как wiring в main.go.
func newAccelGameHarness(t *testing.T) (*TravelHandlers, *travel.Manager, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	h := NewTravelHandlers(repository.NewWorldRepository(db), repository.NewUserRepository(db), tm)
	h.SetIntrasystemAutostart(repository.NewPlanetRepository(db), repository.NewKnowledgeRepository(db))
	h.SetAccelerator(repository.NewPlayerAcceleratorRepository(db))
	h.SetRoutePuzzle(repository.NewRoutePuzzleRepository(db))
	return h, tm, mock
}

// accelOfferRequest — GET /api/accelerator/offer с userID в контексте.
func accelOfferRequest(userID string) *http.Request {
	return withUserID(httptest.NewRequest(http.MethodGet, "/api/accelerator/offer", nil), userID)
}

// newAccelBoostRequest — POST /api/accelerator/boost с телом {fingerprint, path}.
func newAccelBoostRequest(userID, fingerprint string, path []routegame.Point) *http.Request {
	body, err := json.Marshal(acceleratorBoostRequest{Fingerprint: fingerprint, Path: path})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/accelerator/boost", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

// decodeMap — тело ответа как объект.
func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))
	return m
}

// accelTestField — то же поле, что строит сервер: seed hash(from,to) + дальность
// от старта сегмента до цели + паспорт из тех же миров/поясов.
func accelTestField(from, to *models.World, startX, startY float64, belts []models.Belt) routegame.Field {
	dist := math.Hypot(to.CoordX-startX, to.CoordY-startY)
	passport := routegame.BuildPassport(dist, *from, *to, visibleBelts(belts))
	return routegame.GenerateField(routegame.HashSeed(from.ID, to.ID), dist, passport)
}

// accelPathThroughBeacons — валидный путь СТАРТ → все маяки → ФИНИШ.
func accelPathThroughBeacons(field routegame.Field) []routegame.Point {
	path := []routegame.Point{field.Start}
	for _, n := range field.Nodes {
		if n.Type == "beacon" {
			path = append(path, routegame.Point{X: n.X, Y: n.Y})
		}
	}
	return append(path, field.Finish)
}

// ==================== OFFER ====================

// Вне полёта — 409 no_flight без обращений к БД.
func TestAcceleratorOfferNoFlight(t *testing.T) {
	h, _, mock := newAccelGameHarness(t)

	rec := execJSON(h.AcceleratorOffer, accelOfferRequest("u1"))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, false, m["available"])
	require.Equal(t, "no_flight", m["reason"])
	require.Nil(t, m["cooldown_remaining_s"])
	require.NotContains(t, m, "field")
	require.NotContains(t, m, "passport")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Идущий откат (остаток больше порога показа) → 409 cooldown.
func TestAcceleratorOfferCooldown(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)

	expectAccelState(mock, userID, time.Now().Add(-time.Minute), 25)
	expectAccelGameUser(mock, userID, "w1")

	rec := execJSON(h.AcceleratorOffer, accelOfferRequest(userID))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "cooldown", m["reason"])
	require.NotNil(t, m["cooldown_remaining_s"])
	require.NotContains(t, m, "field")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Остаток между порогами (boost-порог 90 < remaining < offer-порог 180) →
// offer отклоняется по ПОРОГУ ПОКАЗА.
func TestAcceleratorOfferTooShort(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, 120*time.Second, nil)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")

	rec := execJSON(h.AcceleratorOffer, accelOfferRequest(userID))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "too_short", m["reason"])
	require.Nil(t, m["cooldown_remaining_s"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Доступно → 200 с доской v9/паспортом; доска не зависит от remaining (два
// вызова с изменившимся остатком дают одну доску), скрытые слои не утекают.
func TestAcceleratorOfferAvailableAndBoardStable(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	hash := acceleratorSegmentHash(flight)
	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)
	expectWorld(mock, "w1", 0, 0)
	expectBelts(mock, "w2")
	expectRoutePuzzle(mock, userID, hash, secret, layoutJSON, []byte("[]"), 2)

	rec := execJSON(h.AcceleratorOffer, accelOfferRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code)

	// Скрытые слои не отдаются: secret/realized/якоря/содержимое секторов.
	body := rec.Body.String()
	for _, leak := range []string{"secret", "realized", "l_naive", "l_safe", "l_risk", "vis_path", "l0_cells", "\"content\""} {
		require.NotContains(t, body, leak, "утечка скрытого слоя: %s", leak)
	}

	var first acceleratorOfferResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))
	require.Equal(t, acceleratorFingerprint(flight), first.Fingerprint)
	require.Equal(t, "route", first.Game)
	require.Equal(t, 90, first.MinRemainingBoostS)
	require.Equal(t, 10.0, first.Passport.Dist, "dist — от старта сегмента до цели")
	require.Equal(t, 2, first.PingsLeft)
	require.Len(t, first.Board.Visible, first.Board.N*first.Board.N)
	require.NotEmpty(t, first.Board.Beacons, "поле с маяками")
	require.NotEmpty(t, first.Board.Sectors, "публичные секторы с σ")
	require.Empty(t, first.Revealed)
	require.GreaterOrEqual(t, first.RemainingS, 3500)
	require.Nil(t, first.CooldownRemainingS)

	// Повторный вызов спустя время: доска — та же чистая функция seed+secret+dist.
	time.Sleep(5 * time.Millisecond)
	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)
	expectWorld(mock, "w1", 0, 0)
	expectBelts(mock, "w2")
	expectRoutePuzzle(mock, userID, hash, secret, layoutJSON, []byte("[]"), 2)
	rec = execJSON(h.AcceleratorOffer, accelOfferRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code)
	var second acceleratorOfferResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &second))
	require.Equal(t, first.Board, second.Board, "доска не зависит от живого remaining")
	require.LessOrEqual(t, second.RemainingS, first.RemainingS, "остаток не растёт")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== BOOST ====================

// Устаревший fingerprint (разворот/перебазирование) → 409 changed, Booster не вызван.
func TestAcceleratorBoostFingerprintChanged(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	old := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, "w1:w2:0", nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "changed", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, old, tm.GetFlight(userID), "сегмент не менялся")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ускорение уже действует на сегменте → 409 already_active без списания.
func TestAcceleratorBoostAlreadyActive(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, flight.StartTime, 25)
	expectAccelGameUser(mock, userID, "w1")

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "already_active", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Остаток ниже порога ОТПРАВКИ → 409 too_short без списания (мир-цель уже загружен).
func TestAcceleratorBoostTooShort(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, 60*time.Second, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "too_short", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Модуля нет → 409 no_module (доступность не нужна, но сегмент не трогаем).
func TestAcceleratorBoostNoModule(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectUser(mock, userID, "w1") // оборудование без ускорителя

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "no_module", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Мир-цель съеден пакманом → 404 без списания.
func TestAcceleratorBoostTargetEaten(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorldMissing(mock, "w2")

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Невалидный путь (пропущены маяки) → 400 с кодом EvaluatePath, без списания.
func TestAcceleratorBoostInvalidPath(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)
	expectWorld(mock, "w1", 0, 0)
	expectBelts(mock, "w2")

	field := accelTestField(&models.World{ID: "w1"}, &models.World{ID: "w2", CoordX: 10}, 0, 0, nil)
	require.NotEmpty(t, field.Nodes)
	// Путь только СТАРТ → ФИНИШ: обязательные маяки не захвачены.
	badPath := []routegame.Point{field.Start, field.Finish}

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), badPath))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, false, m["available"])
	require.Equal(t, "beacon_not_captured", m["reason"])
	require.NotContains(t, m, "cooldown_remaining_s")
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Успех: сервер пересобрал ТО ЖЕ поле (путь из offer-поля проходит), сегмент
// сокращён, выигрыш зафиксирован.
func TestAcceleratorBoostSuccess(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)
	expectWorld(mock, "w1", 0, 0)
	expectBelts(mock, "w2")

	field := accelTestField(&models.World{ID: "w1"}, &models.World{ID: "w2", CoordX: 10}, 0, 0, nil)
	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), accelPathThroughBeacons(field)))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp acceleratorBoostResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.GreaterOrEqual(t, resp.Quality, 0.0)
	require.LessOrEqual(t, resp.Quality, 1.0)
	require.GreaterOrEqual(t, resp.Bonus, ship.AcceleratorBonusMin)
	require.LessOrEqual(t, resp.Bonus, 0.50)
	require.Greater(t, resp.RemainingS, 0)
	require.Less(t, resp.RemainingS, 3600, "сегмент сокращён")

	boosted := tm.GetFlight(userID)
	require.NotNil(t, boosted)
	require.Less(t, boosted.Duration, time.Hour, "остаток сегмента уменьшился")
	require.Equal(t, "w2", boosted.ToWorld, "цель не изменилась")
	require.Equal(t, 1, b.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Отказы Booster (already_active/cooldown) → 409 без замены сегмента.
func TestAcceleratorBoostBoosterRefusal(t *testing.T) {
	for _, reason := range []string{"already_active", "cooldown"} {
		t.Run(reason, func(t *testing.T) {
			h, tm, mock := newAccelGameHarness(t)
			const userID = "u1"
			tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
			flight := tm.GetFlight(userID)
			b := &accelFakeBooster{applied: false, reason: reason}
			tm.SetBooster(b)

			expectAccelState(mock, userID, nil, nil)
			expectAccelGameUser(mock, userID, "w1")
			expectWorld(mock, "w2", 10, 0)
			expectWorld(mock, "w1", 0, 0)
			expectBelts(mock, "w2")

			field := accelTestField(&models.World{ID: "w1"}, &models.World{ID: "w2", CoordX: 10}, 0, 0, nil)
			rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), accelPathThroughBeacons(field)))
			require.Equal(t, http.StatusConflict, rec.Code)
			m := decodeMap(t, rec)
			require.Equal(t, false, m["available"])
			require.Equal(t, reason, m["reason"])
			require.Equal(t, 1, b.calls)
			require.Same(t, flight, tm.GetFlight(userID), "сегмент не заменён")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
