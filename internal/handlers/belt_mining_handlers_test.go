// internal/handlers/belt_mining_handlers_test.go
// Тесты добычи в поясе (спека 2026-09-22-пояса-малых-тел-этап-3-добыча §12.1,
// M1–M12, M16, M18, M19): вход/сбор/выход, клампы (скорость/запас/трюм),
// ленивая инициализация запаса, «нет данных» vs «выработан», перелив буфера.
package handlers

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/cargo"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// ==================== ХЕЛПЕРЫ ====================

// fakeBeltCargo — заглушка сервиса трюма (BeltCargo) для тестов хендлера.
type fakeBeltCargo struct {
	free, total, used float64
	addQty            float64
	addGood           int64
	addCalls          int
	addErr            error
}

func (f *fakeBeltCargo) View(userID string) (*cargo.View, error) {
	return &cargo.View{
		Limits: cargo.Limits{Mass: cargo.MassLimit{Used: f.used, Total: f.total}},
		Items:  []cargo.Item{},
	}, nil
}

func (f *fakeBeltCargo) Free(userID string) (float64, error) { return f.free, nil }

func (f *fakeBeltCargo) TryAddCargo(userID string, goodID int64, qty float64) (float64, error) {
	f.addCalls++
	f.addGood = goodID
	f.addQty += qty
	return qty, f.addErr
}

func (f *fakeBeltCargo) TryAddCargoTx(tx *sql.Tx, userID string, goodID int64, qty float64) (float64, error) {
	return f.TryAddCargo(userID, goodID, qty)
}

func newBeltMiningHarness(t *testing.T) (*BeltMiningHandlers, *fakeBeltCargo, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	c := &fakeBeltCargo{free: 88, total: 100, used: 12}
	h := NewBeltMiningHandlers(
		db,
		repository.NewUserRepository(db),
		repository.NewWorldRepository(db),
		repository.NewPlanetRepository(db),
		c,
		travel.NewManager(nil),
		travel.NewIntrasystemManager(nil),
	)
	return h, c, mock
}

const testBeltUID = "11111111-1111-1111-1111-111111111111"

// orbitBeltPos — позиция «на орбите пояса b1».
const orbitBeltPos = `{"status":"orbit","object_type":"belt","object_id":"b1","level":"orbit"}`

// miningPosJSON — позиция захода в пояс b1 с буфером mined; last_collect_at —
// секунду назад (Δt в клампе скорости не нулевой и не упирается в Δt_max).
func miningPosJSON(beltID string, mined float64) string {
	p := models.MiningPosition(beltID, time.Now().Add(-time.Second))
	m := mined
	p.Mined = &m
	b, _ := json.Marshal(p)
	return string(b)
}

// miningPosAt — позиция захода с явным last_collect_at (тесты клампа скорости).
func miningPosAt(beltID string, mined float64, lastCollectAt string) string {
	p := models.MiningPosition(beltID, time.Now())
	m := mined
	p.Mined = &m
	p.LastCollectAt = lastCollectAt
	b, _ := json.Marshal(p)
	return string(b)
}

func beltMineEnterRequest(userID, beltID string) *http.Request {
	body, _ := json.Marshal(map[string]string{"belt_id": beltID})
	req := httptest.NewRequest(http.MethodPost, "/api/belt/mine/enter", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

func beltMineCollectRequest(userID string, amount float64) *http.Request {
	body, _ := json.Marshal(BeltCollectRequest{Amount: amount})
	req := httptest.NewRequest(http.MethodPost, "/api/belt/mine/collect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

func beltMineLeaveRequest(userID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/belt/mine/leave", nil)
	return withUserID(req, userID)
}

// expectLockBelt — ожидание LockBeltForUpdate (запрос входа/сбора).
func expectLockBelt(mock sqlmock.Sqlmock, beltID, worldID, kind, name, composition string, visible bool, iron interface{}) {
	mock.ExpectQuery(`SELECT id, world_id, kind, name, composition, visible, iron_remaining\s+FROM system_belts\s+WHERE id = \$1 FOR UPDATE`).
		WithArgs(beltID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "world_id", "kind", "name", "composition", "visible", "iron_remaining",
		}).AddRow(beltID, worldID, kind, name, composition, visible, iron))
}

// expectGetBeltByID — ожидание GetBeltByID (пакет захода/сбор).
func expectGetBeltByID(mock sqlmock.Sqlmock, beltID, worldID, kind, name, composition string, visible bool, iron interface{}) {
	mock.ExpectQuery(`SELECT id, world_id, kind, name, orbit_index, radius_au, width_au, mass,\s+body_size_km, composition, visible, data, iron_remaining, created_at, updated_at\s+FROM system_belts\s+WHERE id = \$1`).
		WithArgs(beltID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "world_id", "kind", "name", "orbit_index", "radius_au", "width_au",
			"mass", "body_size_km", "composition", "visible", "data", "iron_remaining",
			"created_at", "updated_at",
		}).AddRow(beltID, worldID, kind, name, nil, 3.0, 0.6, 0.05, 120.0, composition, visible, "{}", iron, now(), now()))
}

// expectIronGood — резолв ресурса «Железо Fe» по точному name_norm.
func expectIronGood(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, name, weight FROM goods WHERE name_norm = \$1 AND kind = 'resource'`).
		WithArgs("железо fe").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "weight"}).AddRow(int64(21), "Железо Fe", 1.0))
}

// expectSetIron — запись запаса (ленивая инициализация).
func expectSetIron(mock sqlmock.Sqlmock, beltID string) {
	mock.ExpectExec(`UPDATE system_belts SET iron_remaining = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), beltID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectPositionUpdate — точечный UPDATE позиции.
func expectPositionUpdate(mock sqlmock.Sqlmock, userID string) {
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectPlanetsAndBelts — планеты системы + пояса (полёт из пояса: belts нужны
// для радиуса from = belt, §6.5).
func expectPlanetsAndBelts(mock sqlmock.Sqlmock, worldID string, belt []driver.Value, planetRows ...[]driver.Value) {
	r := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"})
	for _, row := range planetRows {
		r.AddRow(row...)
	}
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(worldID).
		WillReturnRows(r)
	expectBelts(mock, worldID, belt)
}

// newIntraHarnessWithMining — IntrasystemHandlers + MiningBuffer (фейк трюма) на
// sqlmock: путь «взлёт из пояса в одной транзакции» (§6.5).
func newIntraHarnessWithMining(t *testing.T) (*IntrasystemHandlers, *fakeBeltCargo, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	c := &fakeBeltCargo{}
	h := NewIntrasystemHandlers(
		repository.NewWorldRepository(db),
		repository.NewUserRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewPlayerIntrasystemFlightRepository(db),
		repository.NewKnowledgeRepository(db),
		travel.NewManager(nil),
		travel.NewIntrasystemManager(nil),
	)
	h.SetMiningBuffer(db, NewMiningBuffer(c))
	return h, c, mock
}

// newTravelHarnessWithMining — TravelHandlers + intraRepo + MiningBuffer (фейк
// трюма): путь «межзвёздный старт из захода в одной транзакции» (§6.5).
func newTravelHarnessWithMining(t *testing.T) (*TravelHandlers, *fakeBeltCargo, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	c := &fakeBeltCargo{}
	h := NewTravelHandlers(
		repository.NewWorldRepository(db),
		repository.NewUserRepository(db),
		travel.NewManager(nil),
	)
	h.SetIntrasystem(travel.NewIntrasystemManager(nil), repository.NewPlayerIntrasystemFlightRepository(db))
	h.SetMiningBuffer(db, NewMiningBuffer(c))
	return h, c, mock
}

// expectMiningFlushTx — часть транзакции перелива буфера: лок позиции игрока
// (FOR UPDATE) + резолв железа.
func expectMiningFlushTx(mock sqlmock.Sqlmock, userID, beltID string, mined float64) {
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).AddRow(miningPosJSON(beltID, mined)))
	mock.ExpectQuery(`SELECT id, name, weight FROM goods WHERE name_norm = \$1 AND kind = 'resource'`).
		WithArgs("железо fe").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "weight"}).AddRow(int64(21), "Железо Fe", 1.0))
}

// expectIntraStartAtomicTx — часть транзакции старта intra (строка + позиция
// in_flight) без Begin/Commit (вызывающий владеет транзакцией).
func expectIntraStartAtomicTx(mock sqlmock.Sqlmock, userID, fromType, fromID string) {
	mock.ExpectExec(`INSERT INTO player_intrasystem_flights \(user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs(userID, "w1", fromType, fromID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectCancelAtomicTx — часть транзакции отмены intra (DELETE + позиция NULL,
// намерение) без Begin/Commit.
func expectCancelAtomicTx(mock sqlmock.Sqlmock, userID string) {
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET current_position = NULL, pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// ==================== ENTER (M1–M5, M19) ====================

// M1: из позиции-пояса → 200, пакет захода, ленивая инициализация запаса.
func TestBeltMineEnter(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", orbitBeltPos)
	expectIntraWorld(mock, "w1")
	mock.ExpectBegin()
	expectLockBelt(mock, "b1", "w1", "asteroid", "Пояс астероидов", `{"rock":0.6,"iron":0.3,"ice":0.1}`, true, nil)
	expectSetIron(mock, "b1")
	expectPositionUpdate(mock, testBeltUID)
	// Пакет собирается ДО коммита (резолв ресурса — внутри транзакции).
	expectIronGood(mock)
	mock.ExpectCommit()

	rec := execJSON(h.Enter, beltMineEnterRequest(testBeltUID, "b1"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp BeltMineEnterResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "b1", resp.BeltID)
	assert.Equal(t, "asteroid", resp.BeltKind)
	assert.Equal(t, "богатый", resp.BeltClass, "iron 0.30 → k=1 → богатый")
	assert.Equal(t, "полный", resp.RemainingLevel)
	assert.Equal(t, 0.0, resp.Mined)
	assert.InDelta(t, 0.5, resp.Limits.RateCap, 1e-9, "R_cap = 2·0.25·1")
	assert.InDelta(t, 2.0, resp.Limits.DtMax, 1e-9)
	assert.Equal(t, int64(21), resp.Resource.GoodID)
	assert.NotZero(t, resp.Seed)
}

// M2: не в поясе → 400 «Сначала долетите до пояса».
func TestBeltMineEnterNotAtBelt(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`)
	expectIntraWorld(mock, "w1")

	rec := execJSON(h.Enter, beltMineEnterRequest(testBeltUID, "b1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Сначала долетите до пояса")
	require.NoError(t, mock.ExpectationsWereMet())
}

// M3: повторный enter → 200, та же сессия, запас не пере-инициализируется.
func TestBeltMineEnterIdempotent(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", miningPosJSON("b1", 5))
	expectGetBeltByID(mock, "b1", "w1", "asteroid", "Пояс", `{"rock":0.6,"iron":0.3}`, true, 300.0)
	expectIronGood(mock)

	rec := execJSON(h.Enter, beltMineEnterRequest(testBeltUID, "b1"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet(), "идемпотентный путь без транзакции (запас не пишется)")

	var resp BeltMineEnterResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 5.0, resp.Mined, "буфер сохранён")
	assert.Equal(t, "истощается", resp.RemainingLevel, "300/600 = 0.5")
}

// M4: iron_remaining = 0 → 400 «Пояс выработан».
func TestBeltMineEnterDepleted(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", orbitBeltPos)
	expectIntraWorld(mock, "w1")
	mock.ExpectBegin()
	expectLockBelt(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, true, 0.0)
	mock.ExpectRollback()

	rec := execJSON(h.Enter, beltMineEnterRequest(testBeltUID, "b1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Пояс выработан")
	require.NoError(t, mock.ExpectationsWereMet())
}

// M5: свободный трюм 0 → 400 «Трюм полон».
func TestBeltMineEnterCargoFull(t *testing.T) {
	h, c, mock := newBeltMiningHarness(t)
	c.free = 0

	expectSurfaceUser(mock, testBeltUID, "w1", orbitBeltPos)
	expectIntraWorld(mock, "w1")
	mock.ExpectBegin()
	expectLockBelt(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, true, nil)
	mock.ExpectRollback()

	rec := execJSON(h.Enter, beltMineEnterRequest(testBeltUID, "b1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Трюм полон")
	require.NoError(t, mock.ExpectationsWereMet())
}

// M19: composition без железа → 400 «нет данных о запасе», НЕ «выработан».
// Ленивая инициализация невозможна (не из чего считать reserve) — это и есть
// достижимая ветка «нет данных» (NULL сам по себе лениво инициализируется).
func TestBeltMineEnterNoDataReserve(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", orbitBeltPos)
	expectIntraWorld(mock, "w1")
	mock.ExpectBegin()
	expectLockBelt(mock, "b1", "w1", "asteroid", "Пояс", `{"rock":1.0}`, true, nil)
	mock.ExpectRollback()

	rec := execJSON(h.Enter, beltMineEnterRequest(testBeltUID, "b1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "нет данных о запасе")
	require.NotContains(t, rec.Body.String(), "выработан", "NULL ≠ «выработан» (§8.4)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Точка 3b: ресурса «Железо Fe» нет в каталоге → 500, позиция mining НЕ
// закоммичена (пакет собирается до коммита, транзакция откатывается).
func TestBeltMineEnterNoGoodRollsBack(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", orbitBeltPos)
	expectIntraWorld(mock, "w1")
	mock.ExpectBegin()
	expectLockBelt(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, true, 600.0)
	expectPositionUpdate(mock, testBeltUID)
	// Каталог без ресурса → resolveIronGood пусто.
	mock.ExpectQuery(`SELECT id, name, weight FROM goods WHERE name_norm = \$1 AND kind = 'resource'`).
		WithArgs("железо fe").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	rec := execJSON(h.Enter, beltMineEnterRequest(testBeltUID, "b1"))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "позиция не закоммичена (откат)")
}

// Точка 3a: идемпотентный enter в скрытом поясе для player → 400 (тот же гейт
// видимости, что у первого входа).
func TestBeltMineEnterIdempotentHidden(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", miningPosJSON("b1", 1))
	expectGetBeltByID(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, false, 600.0)

	req := withRole(beltMineEnterRequest(testBeltUID, "b1"), "player")
	rec := execJSON(h.Enter, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Пояс не найден")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== CLAMP (M6–M9) ====================

// M6: amount выше R_cap·Δt → granted урезан до потолка.
func TestBeltCollectClampRate(t *testing.T) {
	// R_cap = 0.5, Δ = 2 → потолок 1.0.
	assert.InDelta(t, 1.0, beltGranted(5, 0.5, 2, 100, 100), 1e-9)
}

// M7: granted ≤ iron_remaining (запас — узкое место).
func TestBeltCollectClampReserve(t *testing.T) {
	assert.InDelta(t, 0.3, beltGranted(5, 10, 2, 0.3, 100), 1e-9)
}

// M8: granted ≤ свободный трюм с учётом буфера (free − mined).
func TestBeltCollectClampCargo(t *testing.T) {
	assert.InDelta(t, 2.0, beltGranted(5, 10, 2, 100, 2), 1e-9)
	// Свободного места нет → 0, не отрицательно.
	assert.Equal(t, 0.0, beltGranted(5, 10, 2, 100, -1))
}

// M9: огромный интервал (офлайн) → Δ клампится Δt_max.
func TestBeltCollectDtMax(t *testing.T) {
	old := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	assert.InDelta(t, 2.0, beltDeltaSeconds(old, time.Now()), 1e-9)
	assert.InDelta(t, 2.0, beltDeltaSeconds("", time.Now()), 1e-9, "пусто → предохранитель")
	assert.InDelta(t, 2.0, beltDeltaSeconds("битая дата", time.Now()), 1e-9)
	// granted ≤ R_cap·Δt_max.
	assert.InDelta(t, 1.0, beltGranted(100, 0.5, beltDeltaSeconds(old, time.Now()), 1000, 1000), 1e-9)
}

// M6-конкурентность: два сбора в одном окне не обходят кламп скорости
// (§2.2-B1). Δ второго сбора считается из СВЕЖЕГО last_collect_at (записанного
// первым сбором под локом), поэтому суммарно зачитывается ≤ R_cap·Δt, а не 2×.
func TestBeltCollectConcurrentRateClamp(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	// Оба запроса приходят с ОДНИМ старым last_collect_at (как при гонке):
	// Δ упирается в Δt_max = 2 c → потолок R_cap·2 = 1.0.
	stale := time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339Nano)

	// Сбор 1: свежая позиция = старый интервал → granted = 1.0 (потолок).
	expectSurfaceUser(mock, testBeltUID, "w1", miningPosAt("b1", 0, stale))
	expectGetBeltByID(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, true, 1000.0)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT iron_remaining FROM system_belts WHERE id = \$1 FOR UPDATE`).WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"iron_remaining"}).AddRow(1000.0))
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1 FOR UPDATE`).WithArgs(testBeltUID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).AddRow(miningPosAt("b1", 0, stale)))
	mock.ExpectExec(`UPDATE system_belts SET iron_remaining = iron_remaining - \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(0, 1))
	expectPositionUpdate(mock, testBeltUID)
	mock.ExpectCommit()

	rec1 := execJSON(h.Collect, beltMineCollectRequest(testBeltUID, 100))
	require.Equal(t, http.StatusOK, rec1.Code, rec1.Body.String())
	var r1 BeltCollectResponse
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &r1))
	require.InDelta(t, 1.0, r1.Granted, 1e-6, "R_cap·Δt_max = 2·0.25·2")

	// Сбор 2: до-транзакционная позиция ТА ЖЕ (старая) — старый баг дал бы ещё
	// один полный потолок; свежая (под локом) уже обновлена первым сбором
	// (timestamp вперёд) → Δ = 0 → granted = 0. Значение вперёд делает проверку
	// детерминированной (не зависит от планировщика/`-race`).
	expectSurfaceUser(mock, testBeltUID, "w1", miningPosAt("b1", r1.Mined, stale))
	expectGetBeltByID(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, true, 1000.0)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT iron_remaining FROM system_belts WHERE id = \$1 FOR UPDATE`).WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"iron_remaining"}).AddRow(1000.0))
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1 FOR UPDATE`).WithArgs(testBeltUID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).
			AddRow(miningPosAt("b1", r1.Mined, time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano))))
	expectPositionUpdate(mock, testBeltUID) // granted = 0 → без UPDATE запаса
	mock.ExpectCommit()

	rec2 := execJSON(h.Collect, beltMineCollectRequest(testBeltUID, 100))
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())
	var r2 BeltCollectResponse
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &r2))
	require.Equal(t, 0.0, r2.Granted, "Δ от свежего timestamp = 0 → второй сбор пуст")

	require.LessOrEqual(t, r1.Granted+r2.Granted, 1.0001,
		"суммарно за окно ≤ R_cap·Δt_max, а не 2×")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== COLLECT (M12 — запас не уходит в минус) ====================

// expectCollectTx — транзакция сбора: лок пояса (FOR UPDATE) → лок игрока →
// (опц.) списание запаса → запись позиции.
func expectCollectTx(mock sqlmock.Sqlmock, userID, beltID string, remaining float64, updateBelt bool) {
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT iron_remaining FROM system_belts WHERE id = \$1 FOR UPDATE`).
		WithArgs(beltID).
		WillReturnRows(sqlmock.NewRows([]string{"iron_remaining"}).AddRow(remaining))
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).AddRow(miningPosJSON(beltID, 0)))
	if updateBelt {
		mock.ExpectExec(`UPDATE system_belts SET iron_remaining = iron_remaining - \$1, updated_at = NOW\(\) WHERE id = \$2`).
			WithArgs(sqlmock.AnyArg(), beltID).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	expectPositionUpdate(mock, userID)
	mock.ExpectCommit()
}

// M12: два сбора — запас (0.5) не уходит в минус (лок FOR UPDATE + кламп);
// второй сбор получает 0 и «выработан».
func TestBeltMineConcurrentReserve(t *testing.T) {
	h, _, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", miningPosJSON("b1", 0))
	expectGetBeltByID(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, true, 0.5)
	expectCollectTx(mock, testBeltUID, "b1", 0.5, true)

	rec := execJSON(h.Collect, beltMineCollectRequest(testBeltUID, 100))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp1 BeltCollectResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp1))
	assert.InDelta(t, 0.5, resp1.Granted, 1e-9, "granted ≤ запас")

	expectSurfaceUser(mock, testBeltUID, "w1", miningPosJSON("b1", 0.5))
	expectGetBeltByID(mock, "b1", "w1", "asteroid", "Пояс", `{"iron":0.3}`, true, 0.0)
	expectCollectTx(mock, testBeltUID, "b1", 0.0, false)

	rec2 := execJSON(h.Collect, beltMineCollectRequest(testBeltUID, 100))
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())
	var resp2 BeltCollectResponse
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	assert.Equal(t, 0.0, resp2.Granted, "пустой запас → 0, не отрицательно")
	assert.Equal(t, "выработан", resp2.RemainingLevel)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== LEAVE (M10–M11) ====================

// M10: mined уходит в трюм (через сервис), позиция → orbit/belt, буфер 0.
func TestBeltMineLeave(t *testing.T) {
	h, c, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", miningPosJSON("b1", 5))
	expectIronGood(mock)
	mock.ExpectBegin()
	expectPositionUpdate(mock, testBeltUID)
	mock.ExpectCommit()

	rec := execJSON(h.Leave, beltMineLeaveRequest(testBeltUID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp BeltLeaveResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Position)
	assert.Equal(t, "orbit", resp.Position.Status)
	assert.Equal(t, "belt", resp.Position.ObjectType)
	assert.Equal(t, "b1", resp.Position.ObjectID)
	assert.InDelta(t, 5.0, resp.Added, 1e-9)
	assert.Equal(t, 1, c.addCalls, "перелив через сервис трюма")
	assert.Equal(t, int64(21), c.addGood)
	assert.InDelta(t, 5.0, c.addQty, 1e-9)
}

// M11: leave без mining → 200 no-op (идемпотентность).
func TestBeltMineLeaveNoOp(t *testing.T) {
	h, c, mock := newBeltMiningHarness(t)

	expectSurfaceUser(mock, testBeltUID, "w1", orbitBeltPos)

	rec := execJSON(h.Leave, beltMineLeaveRequest(testBeltUID))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp BeltLeaveResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0.0, resp.Added)
	assert.Equal(t, 0, c.addCalls)
}

// ==================== RESTORE (M16) ====================

// M16: рестарт — mining-позиция восстанавливается (JSON round-trip), буфер цел;
// цель Restore (живой пояс) валидна.
func TestBeltMineRestore(t *testing.T) {
	p := models.MiningPosition("b1", time.Now())
	m := 7.5
	p.Mined = &m
	raw, err := json.Marshal(p)
	require.NoError(t, err)

	var back models.CurrentPosition
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, "mining", back.Status)
	require.Equal(t, "belt", back.ObjectType)
	require.Equal(t, "b1", back.ObjectID)
	require.NotNil(t, back.Mined, "буфер захода не теряется")
	assert.InDelta(t, 7.5, *back.Mined, 1e-9)
	assert.NotEmpty(t, back.StartedAt)

	valid, mock := newRestoreValidator(t)
	expectIntraWorld(mock, "w1")
	expectBelts(mock, "w1",
		beltRow("b1", "w1", "asteroid", "Пояс астероидов", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`))
	require.True(t, valid("w1", "belt", "b1"), "живой пояс — цель Restore принимается")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== КАСКАД (M18) ====================

// M18: system_belts (и запас) чистится каскадом Пакмана/очистки; player_cargo —
// намеренно НЕ в truncateTables (личный трюм переживает сброс вселенной).
func TestBeltMineSystemBeltCascade(t *testing.T) {
	require.Contains(t, truncateTables, "system_belts",
		"system_belts обязана быть в truncateTables (запас чистится каскадом)")
	require.NotContains(t, truncateTables, "player_cargo",
		"player_cargo — личный трюм, не чистится сбросом вселенной")
}
