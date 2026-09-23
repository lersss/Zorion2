// internal/handlers/intrasystem_handlers_test.go
// Тесты POST /api/intrasystem-flight (спека 99.2.27 §4.1): валидации в порядке
// спеки (двигатель 91a, активный межзвёздный полёт, current_world_id, цель в
// системе, идемпотентность по паре to_type+to_id, «уже на орбите»), формула
// длительности (С4), компаньоны — синтетические id, атомарный старт (С-1).
package handlers

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// ==================== ФОРМУЛА ДЛИТЕЛЬНОСТИ (С4) ====================

func TestCalcIntraDuration(t *testing.T) {
	tests := []struct {
		name        string
		distAU      float64
		speedFactor float64
		want        time.Duration
	}{
		{name: "dist 0 (спутник → своя планета): минимум 3 сек", distAU: 0, speedFactor: 0.3, want: 3 * time.Second},
		{name: "dist 1: 0.3 сек < минимум → 3 сек", distAU: 1, speedFactor: 0.3, want: 3 * time.Second},
		{name: "dist 10: 3 сек (ровно минимум)", distAU: 10, speedFactor: 0.3, want: 3 * time.Second},
		{name: "dist 50: 15 сек (манёвренный)", distAU: 50, speedFactor: 0.3, want: 15 * time.Second},
		{name: "dist 100: 30 сек (граница режимов)", distAU: 100, speedFactor: 0.3, want: 30 * time.Second},
		{name: "dist 152: крейсерский 30 + 52×0.03 ≈ 31.56", distAU: 152, speedFactor: 0.3, want: 31*time.Second + 560*time.Millisecond},
		{name: "dist 1000: крейсерский 30 + 900×0.03 = 57 сек", distAU: 1000, speedFactor: 0.3, want: 57 * time.Second},
		{name: "dist 10000: крейсерский 30 + 9900×0.03 = 327 сек", distAU: 10000, speedFactor: 0.3, want: 327 * time.Second},
		{name: "сшивка непрерывна при sf=0.6: dist 100 → 60 сек", distAU: 100, speedFactor: 0.6, want: 60 * time.Second},
		{name: "сшивка непрерывна при sf=0.6: dist 101 → 60.06 сек", distAU: 101, speedFactor: 0.6, want: 60*time.Second + 60*time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CalcIntraDuration(tt.distAU, tt.speedFactor))
		})
	}
}

// ==================== ХЕЛПЕРЫ ====================

// newIntraHarness — sqlmock-БД + IntrasystemHandlers + оба менеджера.
func newIntraHarness(t *testing.T) (*IntrasystemHandlers, *travel.IntrasystemManager, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager(nil)
	intraMgr := travel.NewIntrasystemManager(nil)
	return NewIntrasystemHandlers(
		repository.NewWorldRepository(db),
		repository.NewUserRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewPlayerIntrasystemFlightRepository(db),
		repository.NewKnowledgeRepository(db),
		tm,
		intraMgr,
	), intraMgr, mock
}

// intraUserCols — колонки users для GetByIDWithPosition (15 колонок:
// + pending_destination, спека 99.2.30 §6.3).
var intraUserCols = []string{
	"id", "username", "password_hash", "email", "agent_id", "current_world_id",
	"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "current_position", "pending_destination",
}

// expectIntraUser — ожидание GetByIDWithPosition (в w1, двигатель установлен).
func expectIntraUser(mock sqlmock.Sqlmock, id string, posRaw interface{}) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(intraUserCols).
			AddRow(id, "player", "hash", nil, nil, "w1", "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, "player", now(), now(), posRaw, nil))
}

// expectIntraUserNoEngine — игрок без двигателя (полёт запрещён для player).
func expectIntraUserNoEngine(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(intraUserCols).
			AddRow(id, "player", "hash", nil, nil, "w1", "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, "player", now(), now(), nil, nil))
}

// expectIntraWorld — ожидание мира w1 (G-звезда, без компаньонов).
func expectIntraWorld(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
			"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
			"created_at", "updated_at",
		}).AddRow(id, "Мир1", 0, 0, "G", 5772, "star", "single", nil, nil, nil, now(), now()))
}

// expectIntraWorldBinary — мир с компаньоном (companion_sep_au = 1000).
func expectIntraWorldBinary(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
			"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
			"created_at", "updated_at",
		}).AddRow(id, "Мир1", 0, 0, "G", 5772, "star", "binary",
			`{"binary_type":"wide","companion":"K","companion_sep_au":1000}`, nil, nil, now(), now()))
}

// expectIntraPlanets — планеты системы (лёгкий запрос без поселений) + пояса
// (спека поясов этап 2 §5.4: StartIntraFlight грузит belts после планет).
func expectIntraPlanets(mock sqlmock.Sqlmock, worldID string, rows ...[]driver.Value) {
	r := sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"})
	for _, row := range rows {
		r.AddRow(row...)
	}
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(worldID).
		WillReturnRows(r)
	expectBelts(mock, worldID)
}

// planetRow — строка планеты (orbit_radius_au в data).
func planetRow(id, worldID, name string, orbitIndex int, orbitRadiusAU float64) []driver.Value {
	return []driver.Value{id, worldID, name, orbitIndex,
		`{"type":"землеподобная","orbit_radius_au":` + strings.TrimRight(strings.TrimRight(f64(orbitRadiusAU), "0"), ".") + `}`,
		now(), now()}
}

func f64(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// expectIntraStartAtomic — транзакция старта (строка + позиция in_flight).
func expectIntraStartAtomic(mock sqlmock.Sqlmock, userID, fromType, fromID string) {
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO player_intrasystem_flights \(user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs(userID, "w1", fromType, fromID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

// intraRequest — POST /api/intrasystem-flight с телом {object_type, object_id}.
func intraRequest(userID, objectType, objectID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/intrasystem-flight",
		strings.NewReader(`{"object_type":"`+objectType+`","object_id":"`+objectID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

// ==================== УСПЕШНЫЙ СТАРТ ====================

// Старт к планете: 202, ответ с from/to/start_time/arrive_at, полёт в менеджере.
func TestStartIntraFlightSuccess(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil) // позиция NULL (легаси) → from = звезда
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))
	expectIntraStartAtomic(mock, userID, "star", "w1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "star", resp.FromType)
	assert.Equal(t, "w1", resp.FromID)
	assert.Equal(t, "planet", resp.ToType)
	assert.Equal(t, "p1", resp.ToID)
	// dist = |0 − 1| = 1 а.е. → 3 сек (минимум).
	assert.Equal(t, 3, resp.Duration)
	assert.Equal(t, resp.StartTime+3000, resp.ArriveAt)

	flight := intraMgr.GetIntraFlight(userID)
	require.NotNil(t, flight, "полёт зарегистрирован в менеджере")
	assert.Equal(t, "p1", flight.ToID)
}

// Старт с орбиты планеты: from = позиция (planet p1), dist = |1 − 2| = 1 → 3 сек.
func TestStartIntraFlightFromPlanetOrbit(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1",
		planetRow("p1", "w1", "Планета1", 0, 1.0),
		planetRow("p2", "w1", "Планета2", 1, 2.0))
	// Отлёт D1 (спека 2026-09-23 §3.2): снимок планеты ДО StartAtomic.
	expectPlanetByID(mock, "p1", "w1")
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs(userID, "p1", sqlmock.AnyArg(), "presence").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectIntraStartAtomic(mock, userID, "planet", "p1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p2"))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "planet", resp.FromType)
	assert.Equal(t, "p1", resp.FromID, "from = объект позиции")
	assert.Equal(t, 3, resp.Duration, "dist = |1−2| = 1 → минимум 3 сек")
}

// ==================== ВАЛИДАЦИИ ====================

// Player без двигателя → 400 (91a §6.1).
func TestStartIntraFlightNoEngine(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUserNoEngine(mock, userID)

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Двигатель не установлен")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Активный межзвёздный полёт → 400 (И2).
func TestStartIntraFlightInterstellarActive(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	// Межзвёздный полёт в менеджере.
	h.travelManager.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы в межзвёздном полёте")
	require.NoError(t, mock.ExpectationsWereMet())
}

// current_world_id NULL → 400 «Вы не находитесь в системе».
func TestStartIntraFlightNoWorld(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows(intraUserCols).
			AddRow(userID, "player", "hash", nil, nil, nil, "ship_strela.svg", nil, "starter",
				`{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, "player", now(), now(), nil, nil))

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы не находитесь в системе")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Цель не принадлежит системе → 400 «Объект не найден в вашей системе».
func TestStartIntraFlightTargetNotInSystem(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p-other"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Объект не найден в вашей системе")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Идемпотентность: активный полёт к той же цели (пара to_type+to_id) → 202
// с текущим полётом, без перезапуска (М-7).
func TestStartIntraFlightIdempotent(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	// Активный полёт к p1.
	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "planet", "p1", time.Hour, nil)

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "p1", resp.ToID)
	assert.Equal(t, 3600, resp.Duration, "текущий полёт, без перезапуска")
}

// «Уже на орбите»: позиция orbit на цели → 400.
func TestStartIntraFlightAlreadyOnOrbit(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы уже на орбите этого объекта")
	require.NoError(t, mock.ExpectationsWereMet())
}

// «Уже на орбите» в полёте: цель == объект отправления → 400.
func TestStartIntraFlightTargetIsFromInFlight(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	pos := `{"status":"in_flight","from_type":"star","from_id":"w1","to_type":"planet","to_id":"p1","start_time":1,"arrive_at":2}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))
	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "planet", "p1", time.Hour, nil)

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "star", "w1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Вы уже на орбите этого объекта")
	require.NoError(t, mock.ExpectationsWereMet())
}

// С поверхности планеты взлёт разрешён (идея 2026-09-21): from = планета
// поверхности, позиция сразу in_flight (атомарный StartAtomic). «взлёт»
// отдельного шага не делает — двигатель проверяется как обычно (шаг 2).
func TestStartIntraFlightFromSurface(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	pos := `{"status":"surface","level":"surface","object_type":"planet","object_id":"p1","biome":"горы","hp":90,"landed_at":"2026-09-21T12:00:00Z"}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1",
		planetRow("p1", "w1", "Планета1", 0, 1.0),
		planetRow("p2", "w1", "Планета2", 1, 2.0))
	// Отлёт D1 с поверхности (спека 2026-09-23 §3.2): снимок планеты.
	expectPlanetByID(mock, "p1", "w1")
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs(userID, "p1", sqlmock.AnyArg(), "presence").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectIntraStartAtomic(mock, userID, "planet", "p1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p2"))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "planet", resp.FromType, "from = планета, на которой стоит игрок")
	assert.Equal(t, "p1", resp.FromID)
	assert.Equal(t, "planet", resp.ToType)
	assert.Equal(t, "p2", resp.ToID)
	assert.Equal(t, 3, resp.Duration, "dist = |1−2| = 1 → минимум 3 сек")

	flight := intraMgr.GetIntraFlight(userID)
	require.NotNil(t, flight, "полёт зарегистрирован (позиция сразу in_flight)")
	assert.Equal(t, "planet", flight.FromType)
	assert.Equal(t, "p1", flight.FromID)
}

// ==================== КОМПАНЬОНЫ (решение создателя 2026-09-20) ====================

// Цель — компаньон по синтетическому id (companion:<world>): dist = 1000 а.е.
// → крейсерский режим 57 сек.
func TestStartIntraFlightCompanionTarget(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	expectIntraWorldBinary(mock, "w1")
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))
	expectIntraStartAtomic(mock, userID, "star", "w1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "star", "companion:w1"))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "star", resp.ToType)
	assert.Equal(t, "companion:w1", resp.ToID)
	assert.Equal(t, 57, resp.Duration, "dist = 1000 а.е. → крейсерский 30 + 900×0.03")
}

// Невалидный синтетический id компаньона (нет компаньона в системе) → 400.
func TestStartIntraFlightInvalidCompanionID(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, nil)
	expectIntraWorld(mock, "w1") // одиночная система — компаньона нет
	expectIntraPlanets(mock, "w1", planetRow("p1", "w1", "Планета1", 0, 1.0))

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "star", "companion:w1"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Объект не найден в вашей системе")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ПРИБЫТИЕ: АВТО-ЗНАНИЕ (С6, спека §3.6) ====================

// Прибытие на орбиту планеты: позиция orbit (атомарно с удалением строки) +
// снимок присутствия (FixatePresence, спека 2026-09-23 §3.2): запись
// player_planet_knowledge с source=presence и data.snapshot.
func TestIntraArrivalWritesKnowledge(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	// Короткий полёт — прибытие в течение теста. ВАЖНО: ожидания БД
	// регистрируются ДО StartIntraFlight — иначе гонка внутри теста
	// (горутина onArrival стартует раньше, чем sqlmock-ожидания готовы),
	// флейк ~1 раз на 7–20 прогонов.

	// 1. Валидация цели прибытия: планета существует в системе (GetPlanetByID).
	expectPlanetByID(mock, "p1", "w1")

	// 2. Атомарно: позиция orbit + удаление строки.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1 AND start_time = \$2 AND arrive_at = \$3`).
		WithArgs(userID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// 3. Снимок присутствия: FixatePresence (GetPlanetByID + merge-UPSERT).
	expectPlanetByID(mock, "p1", "w1")
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs(userID, "p1", sqlmock.AnyArg(), "presence").
		WillReturnResult(sqlmock.NewResult(0, 1))

	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "planet", "p1", 30*time.Millisecond,
		NewIntraArrivalHandler(h.intraRepo, h.planetRepo, h.knowledgeRepo, nil))

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond)
}

// ==================== С1: /travel ОТМЕНЯЕТ INTRA (спека §4.2) ====================

// Старт межзвёздного полёта отменяет активный внутрисистемный полёт и NULL-ит
// позицию (атомарно, CancelAtomic): intra и межзвёздный не сосуществуют (И2/И3).
func TestStartTravelCancelsIntrasystem(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	intraMgr := travel.NewIntrasystemManager(nil)
	intraRepo := repository.NewPlayerIntrasystemFlightRepository(db)
	h := NewTravelHandlers(repository.NewWorldRepository(db), repository.NewUserRepository(db), tm)
	h.SetIntrasystem(intraMgr, intraRepo)

	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	// Активный внутрисистемный полёт.
	intraMgr.StartIntraFlight(userID, fromWorld, "star", fromWorld, "planet", "p1", time.Hour, nil)
	require.True(t, intraMgr.IsInIntraFlight(userID))

	// Обычные запросы /travel: цель, пользователь, текущий мир (дважды).
	expectWorld(mock, target, 10, 0)
	expectUser(mock, userID, fromWorld)
	expectWorld(mock, fromWorld, 0, 0)
	expectWorld(mock, fromWorld, 0, 0)

	// С1: отмена intra + позиция NULL + намерение NULL (M1) одной транзакцией.
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET current_position = NULL, pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	require.False(t, intraMgr.IsInIntraFlight(userID), "intra отменён при старте межзвёздного")
	require.NotNil(t, tm.GetFlight(userID), "межзвёздный полёт запущен")
}

// ==================== РЕДИРЕКТ ====================

// Редирект: активный полёт к p1, новая цель p2 → from = объект отправления
// старого полёта (star w1), позиция переходит в новый in_flight.
func TestStartIntraFlightRedirect(t *testing.T) {
	h, intraMgr, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	pos := `{"status":"in_flight","from_type":"star","from_id":"w1","to_type":"planet","to_id":"p1","start_time":1,"arrive_at":2}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1",
		planetRow("p1", "w1", "Планета1", 0, 1.0),
		planetRow("p2", "w1", "Планета2", 1, 2.0))
	expectIntraStartAtomic(mock, userID, "star", "w1")
	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "planet", "p1", time.Hour, nil)

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p2"))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "star", resp.FromType, "редирект от объекта отправления")
	assert.Equal(t, "w1", resp.FromID)
	assert.Equal(t, "p2", resp.ToID)
	assert.Equal(t, 3, resp.Duration, "dist = |0−2| = 2 → минимум 3 сек")
}

// ==================== ЗАХОД В ПОЯС (спека поясов этап 3 §6.4/§6.5) ====================

// M14: валидный mining сохраняется; битый пояс → «орбита звезды» (ИП-4).
func TestNormalizeMyPositionMining(t *testing.T) {
	belts := []models.Belt{{ID: "b1"}}
	validStar := func(id string) bool { return id == "w1" }

	pos := &models.CurrentPosition{Status: "mining", ObjectType: "belt", ObjectID: "b1", Level: "mining"}
	got := normalizeMyPosition(pos, "w1", nil, belts, validStar)
	require.Equal(t, "mining", got.Status, "валидный заход не проваливается в фолбэк")
	require.Equal(t, "belt", got.ObjectType)
	require.Equal(t, "b1", got.ObjectID)

	broken := &models.CurrentPosition{Status: "mining", ObjectType: "belt", ObjectID: "GONE", Level: "mining"}
	got2 := normalizeMyPosition(broken, "w1", nil, belts, validStar)
	require.Equal(t, "orbit", got2.Status)
	require.Equal(t, "star", got2.ObjectType)
	require.Equal(t, "w1", got2.ObjectID)
}

// M15: взлёт из пояса — from = (belt, id); буфер перелит в трюм в ОДНОЙ
// транзакции со стартом полёта (§6.5).
func TestStartIntraFlightFromMining(t *testing.T) {
	h, c, mock := newIntraHarnessWithMining(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectIntraUser(mock, userID, miningPosJSON("b1", 3))
	expectIntraWorld(mock, "w1")
	expectPlanetsAndBelts(mock, "w1",
		beltRow("b1", "w1", "asteroid", "Пояс", 2, 3.0, 0.6, 0.05, 120.0, `{}`, true, `{}`),
		planetRow("p1", "w1", "Планета1", 0, 5.0))
	// Одна транзакция: FlushTx (лок позиции + резолв железа) → StartAtomicTx.
	mock.ExpectBegin()
	expectMiningFlushTx(mock, userID, "b1", 3)
	expectIntraStartAtomicTx(mock, userID, "belt", "b1")
	mock.ExpectCommit()

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p1"))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp IntraFlightResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "belt", resp.FromType, "from = пояс захода")
	assert.Equal(t, "b1", resp.FromID)
	assert.Equal(t, 1, c.addCalls, "буфер перелит в трюм")
	assert.Equal(t, int64(21), c.addGood)
	assert.InDelta(t, 3.0, c.addQty, 1e-9)
}
