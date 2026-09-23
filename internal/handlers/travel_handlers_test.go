// internal/handlers/travel_handlers_test.go
package handlers

import (
	"database/sql"
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

func TestCalcTravelDuration(t *testing.T) {
	tests := []struct {
		name        string
		dist        float64
		speedFactor float64
		want        time.Duration
	}{
		{
			name:        "короткое расстояние: время равно dist*0.3",
			dist:        50,
			speedFactor: models.EngineSpeedDefault,
			want:        15 * time.Second,
		},
		{
			name:        "большое расстояние: без потолка, dist*0.3 (dist=100 -> 30s)",
			dist:        100,
			speedFactor: models.EngineSpeedDefault,
			want:        30 * time.Second,
		},
		{
			name:        "граница старого капа: dist=70 -> 21s, потолок не режет",
			dist:        70,
			speedFactor: models.EngineSpeedDefault,
			want:        21 * time.Second,
		},
		{
			name:        "минимум 3 секунды для очень близких миров",
			dist:        1,
			speedFactor: models.EngineSpeedDefault,
			want:        3 * time.Second,
		},
		{
			name:        "скорость из двигателя: другой speed_factor меняет длительность",
			dist:        100,
			speedFactor: 0.6,
			want:        60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, calcTravelDuration(tt.dist, tt.speedFactor))
		})
	}
}

// ==================== ХЕЛПЕРЫ ====================

// newTravelHarness — sqlmock-БД + репозитории + менеджер полётов.
// Каталог оборудования — дефолты (PITFALLS.md:185): HasEngine/EngineSpeed
// читают in-memory каталог, без него валидация двигателя всегда false.
func newTravelHarness(t *testing.T) (*TravelHandlers, *travel.Manager, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager(nil)
	return NewTravelHandlers(
		repository.NewWorldRepository(db),
		repository.NewUserRepository(db),
		tm,
	), tm, mock
}

// newTravelHarnessWithAutostart — харнесс + planetRepo/knowledgeRepo
// (SetIntrasystemAutostart): валидация destination (спека 99.2.30 §3.1) и
// автостарт требуют planetRepo. intraRepo/intraManager НЕ подключены — тесты
// destination-пути не трогают CancelAtomic (как остальные тесты харнесса).
func newTravelHarnessWithAutostart(t *testing.T) (*TravelHandlers, *travel.Manager, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager(nil)
	h := NewTravelHandlers(
		repository.NewWorldRepository(db),
		repository.NewUserRepository(db),
		tm,
	)
	h.SetIntrasystemAutostart(repository.NewPlanetRepository(db), repository.NewKnowledgeRepository(db))
	return h, tm, mock
}

// travelWorldRow — строка мира для sqlmock (порядок worldColumns).
func travelWorldRow(id string, x, y float64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
		"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
		"created_at", "updated_at",
	}).AddRow(id, "Мир "+id, x, y, "G", 5772, "star", "single", nil, nil, nil, now(), now())
}

// travelWorldRowWithMods — строка мира со stellar_mods (компаньон, 99.2.27
// §3.1): IsValidCompanionID читает companion/extra_companions.
func travelWorldRowWithMods(id string, x, y float64, mods string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
		"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
		"created_at", "updated_at",
	}).AddRow(id, "Мир "+id, x, y, "G", 5772, "star", "binary", mods, nil, nil, now(), now())
}

// jsonContains — sqlmock-матчер аргумента: значение (строка/[]byte) содержит
// все подстроки. Для проверки JSON позиции/намерения без привязки к порядку
// полей вне проверяемого фрагмента.
type jsonContains struct{ subs []string }

func (m jsonContains) Match(v driver.Value) bool {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		return false
	}
	for _, sub := range m.subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// userRow — строка пользователя для sqlmock (текущий мир — fromWorld).
// Стартовая комплектация 91a: радар + сканер + двигатель (спека 91a §7.1).
func userRow(id, fromWorld string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "password_hash", "email", "agent_id", "current_world_id",
		"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "race_id",
	}).AddRow(id, "player", "hash", nil, nil, fromWorld, "ship_strela.svg", nil, nil,
		`{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, "player", now(), now(), nil)
}

// userRowNoEngine — игрок без двигателя (слот engine пуст): полёт запрещён
// для role=player (спека 91a §6.1).
func userRowNoEngine(id, fromWorld string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "password_hash", "email", "agent_id", "current_world_id",
		"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "race_id",
	}).AddRow(id, "player", "hash", nil, nil, fromWorld, "ship_strela.svg", nil, nil,
		`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, "player", now(), now(), nil)
}

// userRowAdminNoEngine — админ без двигателя: летает всегда (исключение 91a §6.1).
func userRowAdminNoEngine(id, fromWorld string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "password_hash", "email", "agent_id", "current_world_id",
		"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "race_id",
	}).AddRow(id, "admin", "hash", nil, nil, fromWorld, "ship_strela.svg", nil, nil,
		`{"radar":"radar_1","scanner":"scanner_1","engine":null}`, "admin", now(), now(), nil)
}

// expectWorld — ожидание SELECT мира по id.
func expectWorld(mock sqlmock.Sqlmock, id string, x, y float64) {
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(travelWorldRow(id, x, y))
}

// expectWorldWithMods — ожидание SELECT мира по id со stellar_mods.
func expectWorldWithMods(mock sqlmock.Sqlmock, id string, x, y float64, mods string) {
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(travelWorldRowWithMods(id, x, y, mods))
}

// expectUser — ожидание SELECT пользователя по id.
func expectUser(mock sqlmock.Sqlmock, id, fromWorld string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(userRow(id, fromWorld))
}

// expectTravelQueries — ожидания БД для одного успешного POST /travel:
// цель, пользователь, проверка текущего мира, координаты текущего мира.
func expectTravelQueries(mock sqlmock.Sqlmock, userID, fromWorld string, fromX, fromY float64, target string, targetX, targetY float64) {
	expectWorld(mock, target, targetX, targetY)
	expectUser(mock, userID, fromWorld)
	expectWorld(mock, fromWorld, fromX, fromY)
	expectWorld(mock, fromWorld, fromX, fromY)
}

// expectTravelQueriesIdempotent — ожидания БД для повторного POST /travel
// с той же целью: цель, пользователь, проверка текущего мира (координаты
// не нужны — полёт не перезапускается, хендлер возвращается раньше).
func expectTravelQueriesIdempotent(mock sqlmock.Sqlmock, userID, fromWorld string, fromX, fromY float64, target string, targetX, targetY float64) {
	expectWorld(mock, target, targetX, targetY)
	expectUser(mock, userID, fromWorld)
	expectWorld(mock, fromWorld, fromX, fromY)
}

// expectTravelQueriesRedirect — ожидания БД для редиректа в полёте:
// цель, пользователь, проверка текущего мира, координаты текущего мира,
// координаты цели старого полёта (для точки P).
func expectTravelQueriesRedirect(mock sqlmock.Sqlmock, userID, fromWorld string, fromX, fromY float64, target string, targetX, targetY float64, oldTo string, oldToX, oldToY float64) {
	expectWorld(mock, target, targetX, targetY)
	expectUser(mock, userID, fromWorld)
	expectWorld(mock, fromWorld, fromX, fromY)
	expectWorld(mock, fromWorld, fromX, fromY)
	expectWorld(mock, oldTo, oldToX, oldToY)
}

// travelRequest — POST /travel с телом {world_id} и userID в контексте.
func travelRequest(userID, worldID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(`{"world_id":"`+worldID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

// ==================== ТОЧКА P (ЧИСТАЯ ФУНКЦИЯ) ====================

func TestRedirectStartPoint(t *testing.T) {
	tests := []struct {
		name     string
		fromX    float64
		fromY    float64
		toX      float64
		toY      float64
		elapsed  time.Duration
		duration time.Duration
		wantX    float64
		wantY    float64
	}{
		{name: "progress 0 -> from", fromX: 0, fromY: 0, toX: 10, toY: 20, elapsed: 0, duration: 10 * time.Second, wantX: 0, wantY: 0},
		{name: "progress 0.5 -> середина", fromX: 0, fromY: 0, toX: 10, toY: 20, elapsed: 5 * time.Second, duration: 10 * time.Second, wantX: 5, wantY: 10},
		{name: "progress 1 -> to", fromX: 0, fromY: 0, toX: 10, toY: 20, elapsed: 10 * time.Second, duration: 10 * time.Second, wantX: 10, wantY: 20},
		{name: "clamp: elapsed > duration -> to", fromX: 0, fromY: 0, toX: 10, toY: 20, elapsed: 20 * time.Second, duration: 10 * time.Second, wantX: 10, wantY: 20},
		{name: "clamp: elapsed < 0 -> from", fromX: 0, fromY: 0, toX: 10, toY: 20, elapsed: -5 * time.Second, duration: 10 * time.Second, wantX: 0, wantY: 0},
		{name: "duration <= 0 -> from", fromX: 0, fromY: 0, toX: 10, toY: 20, elapsed: 5 * time.Second, duration: 0, wantX: 0, wantY: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y := redirectStartPoint(tt.fromX, tt.fromY, tt.toX, tt.toY, tt.elapsed, tt.duration)
			assert.Equal(t, tt.wantX, x)
			assert.Equal(t, tt.wantY, y)
		})
	}
}

// ==================== РЕДИРЕКТ В ПОЛЁТЕ ====================

func TestStartTravelRedirectInFlight(t *testing.T) {
	h, tm, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target1 = "w2"
	const target2 = "w3"

	// Первый полёт: w1(0,0) -> w2(10,0), dist=10 -> 3s.
	// Обычный старт: стартовые координаты == координатам FromWorld.
	expectTravelQueries(mock, userID, fromWorld, 0, 0, target1, 10, 0)
	rec := execJSON(h.StartTravel, travelRequest(userID, target1))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight := tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, target1, flight.ToWorld)
	require.Equal(t, 0.0, flight.StartX, "обычный старт: start_x = координата FromWorld")
	require.Equal(t, 0.0, flight.StartY, "обычный старт: start_y = координата FromWorld")
	firstStart := flight.StartTime
	firstDuration := flight.Duration

	// Редирект в полёте: w1(0,0) -> w3(100,0). Точка P — на отрезке
	// w1->w2 по прогрессу (elapsed ~5 мс из 3 с): P ≈ (0.017, 0).
	// Пауза: на Windows time.Now() имеет гранулярность ~0.5 мс, без неё
	// два StartFlight подряд могут получить одинаковый StartTime.
	time.Sleep(5 * time.Millisecond)
	expectTravelQueriesRedirect(mock, userID, fromWorld, 0, 0, target2, 100, 0, target1, 10, 0)
	rec = execJSON(h.StartTravel, travelRequest(userID, target2))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight = tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, target2, flight.ToWorld, "полёт перенаправлен на новую цель")
	require.Equal(t, 0.0, flight.StartY, "P лежит на линии w1->w2 (обе Y=0)")
	require.Greater(t, flight.StartX, 0.0, "прогресс > 0: P не в стартовой звезде")
	require.Less(t, flight.StartX, 10.0, "прогресс < 1: P не в старой цели")
	require.NotEqual(t, firstStart, flight.StartTime, "полёт перезапущен с новым start_time")
	require.NotEqual(t, firstDuration, flight.Duration, "длительность пересчитана от точки P")
	require.Greater(t, flight.Duration, 20*time.Second, "dist(P, w3) > 66 -> честная длительность > 20 сек")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== КАСКАДНЫЙ РЕДИРЕКТ ====================

// TestStartTravelCascadeRedirect — два редиректа подряд: стартовая точка
// второго считается от фактической точки старта первого (flight.StartX/Y),
// а не от мира отправления A (баг создателя 2026-09-16).
func TestStartTravelCascadeRedirect(t *testing.T) {
	h, tm, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1" // A (0,0)
	const targetB = "w2"   // B (10,0)
	const targetC = "w3"   // C (0,10)
	const targetD = "w4"   // D (10,10)

	// Полёт 1: A(0,0) -> B(10,0), dist=10 -> 3s. Обычный старт из A.
	expectTravelQueries(mock, userID, fromWorld, 0, 0, targetB, 10, 0)
	rec := execJSON(h.StartTravel, travelRequest(userID, targetB))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight := tm.GetFlight(userID)
	require.NotNil(t, flight)
	// Прогресс 0.5: сдвигаем StartTime на половину длительности назад.
	flight.StartTime = flight.StartTime.Add(-flight.Duration / 2)

	// Редирект 1: -> C. P1 = lerp(A, B, 0.5) = (5, 0).
	expectTravelQueriesRedirect(mock, userID, fromWorld, 0, 0, targetC, 0, 10, targetB, 10, 0)
	rec = execJSON(h.StartTravel, travelRequest(userID, targetC))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight = tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, targetC, flight.ToWorld)
	assert.InDelta(t, 5.0, flight.StartX, 0.05, "P1 на линии A->B при progress 0.5")
	assert.InDelta(t, 0.0, flight.StartY, 0.05)

	// Прогресс 0.5 второго полёта.
	flight.StartTime = flight.StartTime.Add(-flight.Duration / 2)

	// Редирект 2: -> D. P2 = lerp(P1, C, 0.5) = lerp((5,0), (0,10), 0.5) = (2.5, 5).
	// При баге было бы lerp(A, C, 0.5) = (0, 5) — корабль «перескакивал» на линию A->C.
	expectTravelQueriesRedirect(mock, userID, fromWorld, 0, 0, targetD, 10, 10, targetC, 0, 10)
	rec = execJSON(h.StartTravel, travelRequest(userID, targetD))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight = tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, targetD, flight.ToWorld)
	assert.InDelta(t, 2.5, flight.StartX, 0.05, "P2 на линии P1->C, а не A->C")
	assert.InDelta(t, 5.0, flight.StartY, 0.05)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ИДЕМПОТЕНТНОСТЬ ====================

func TestStartTravelIdempotentSameTarget(t *testing.T) {
	h, tm, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight := tm.GetFlight(userID)
	require.NotNil(t, flight)
	firstStart := flight.StartTime
	firstStartX := flight.StartX
	firstStartY := flight.StartY

	// Тот же целевой мир — полёт не перезапускается.
	expectTravelQueriesIdempotent(mock, userID, fromWorld, 0, 0, target, 10, 0)
	// Спека 99.2.30 §3.5 (M2): на 202-идемпотентном пути без destination
	// намерение очищается (игрок явно «перелетел» к звезде).
	mock.ExpectExec(`UPDATE users SET pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	rec = execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight = tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, target, flight.ToWorld)
	require.Equal(t, firstStart, flight.StartTime, "полёт не перезапущен: start_time не изменился")
	require.Equal(t, firstStartX, flight.StartX, "стартовая точка не изменилась")
	require.Equal(t, firstStartY, flight.StartY, "стартовая точка не изменилась")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ДЕСТИНАЦИЯ КОМПОЗИТНОГО МАРШРУТА (спека 99.2.30 §3) ====================

// Валидация destination (§3.1): object_type вне {planet, satellite, companion}
// → 400 «Некорректный тип объекта назначения» (звезда — не цель destination,
// решение 5: «звезда → простой Лететь без намерения»).
func TestStartTravelDestinationValidationBadType(t *testing.T) {
	h, _, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	// Цель и пользователь — дальше валидация destination останавливает (400).
	expectWorld(mock, target, 10, 0)
	expectUser(mock, userID, fromWorld)

	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
		`{"world_id":"w2","destination":{"object_type":"star","object_id":"w2"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := execJSON(h.StartTravel, withUserID(req, userID))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Некорректный тип объекта назначения")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Валидация destination (§3.1): объект не принадлежит системе world_id → 400
// «Объект не найден в системе назначения» (битая цель — перегенерация между
// модалкой и кликом; модалка обновится по refreshPlanets).
func TestStartTravelDestinationValidationObjectNotInSystem(t *testing.T) {
	h, _, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectWorld(mock, target, 10, 0)
	expectUser(mock, userID, fromWorld)
	// Планеты системы w2: p1 есть, p999 — нет.
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(target).
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", target, "Планета1", 0, `{"type":"землеподобная","orbit_radius_au":1.0}`, now(), now()))

	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
		`{"world_id":"w2","destination":{"object_type":"planet","object_id":"p999"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := execJSON(h.StartTravel, withUserID(req, userID))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Объект не найден в системе назначения")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== КОМПАНЬОН — ЦЕЛЬ КОМПОЗИТНОГО МАРШРУТА (решение создателя 2026-09-21) ====================

// testCompanionMods — stellar_mods системы с главным компаньоном (99.2.27 §3.1).
const testCompanionMods = `{"binary_type":"wide","companion":"K","companion_sep_au":1000}`

// destination {companion, companion:<world>} при валидном компаньоне (есть в
// stellar_mods системы) → принят: намерение-компаньон пишется атомарно со
// стартом /travel (ИН-4). Формат object_id — синтетический id 99.2.27 §3.1.
func TestStartTravelDestinationCompanionValid(t *testing.T) {
	h, tm, _, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	// Цель (существование) → пользователь → повторная загрузка мира для
	// валидации компаньона (stellar_mods) → мир отправления (дважды).
	expectWorldWithMods(mock, target, 10, 0, testCompanionMods)
	expectUser(mock, userID, fromWorld)
	expectWorldWithMods(mock, target, 10, 0, testCompanionMods)
	expectWorld(mock, fromWorld, 0, 0)
	expectWorld(mock, fromWorld, 0, 0)
	// D2 (спека 2026-09-23 §3.2): позиции нет → снимок не пишется.
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).AddRow(nil))
	// С1 + ИН-4: отмена intra + позиция NULL + намерение одной транзакцией.
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET current_position = NULL, pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(`{"world_id":"w2","object_type":"companion","object_id":"companion:w2"}`, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
		`{"world_id":"w2","destination":{"object_type":"companion","object_id":"companion:w2"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := execJSON(h.StartTravel, withUserID(req, userID))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	flight := tm.GetFlight(userID)
	require.NotNil(t, flight, "межзвёздный сегмент запущен")
	require.Equal(t, target, flight.ToWorld)
}

// destination-компаньон, которого нет в stellar_mods системы (чужой id,
// компаньона нет вовсе, индекс внешнего вне диапазона) → 400 «Объект не
// найден в системе назначения».
func TestStartTravelDestinationCompanionInvalid(t *testing.T) {
	tests := []struct {
		name   string
		mods   string
		destID string
	}{
		{name: "чужой id (другая система)", mods: testCompanionMods, destID: "companion:w9"},
		{name: "компаньона нет в системе", mods: `{"binary_type":"single"}`, destID: "companion:w2"},
		{name: "внешний компаньон вне диапазона", mods: testCompanionMods, destID: "extra:w2:5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, mock := newTravelHarnessWithAutostart(t)
			const userID = "11111111-1111-1111-1111-111111111111"
			const fromWorld = "w1"
			const target = "w2"

			expectWorldWithMods(mock, target, 10, 0, tt.mods)
			expectUser(mock, userID, fromWorld)
			expectWorldWithMods(mock, target, 10, 0, tt.mods)

			req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
				`{"world_id":"w2","destination":{"object_type":"companion","object_id":"`+tt.destID+`"}}`))
			req.Header.Set("Content-Type", "application/json")
			rec := execJSON(h.StartTravel, withUserID(req, userID))
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "Объект не найден в системе назначения")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// 202-идемпотентный путь с destination-компаньоном (§3.5): полёт к req.WorldID
// уже идёт — намерение-компаньон записывается отдельным UPDATE, полёт не
// перезапускается (те же правила, что для planet/satellite).
func TestStartTravelIdempotentWithCompanionDestination(t *testing.T) {
	h, tm, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	// Первый полёт: w1 -> w2 (обычный, без destination).
	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	// D2 (спека 2026-09-23 §3.2): позиции нет → снимок не пишется.
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).AddRow(nil))
	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)

	// Повторный /travel с destination-компаньоном к той же системе: валидация
	// (мир с stellar_mods) + запись намерения отдельным UPDATE. Порядок: цель →
	// пользователь → мир (валидация companion) → fromWorld.
	expectWorldWithMods(mock, target, 10, 0, testCompanionMods)
	expectUser(mock, userID, fromWorld)
	expectWorldWithMods(mock, target, 10, 0, testCompanionMods)
	expectWorld(mock, fromWorld, 0, 0)
	mock.ExpectExec(`UPDATE users SET pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(`{"world_id":"w2","object_type":"companion","object_id":"companion:w2"}`, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
		`{"world_id":"w2","destination":{"object_type":"companion","object_id":"companion:w2"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec = execJSON(h.StartTravel, withUserID(req, userID))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	flight := tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, target, flight.ToWorld)
}

// 202-идемпотентный путь с destination (M2, §3.5): полёт к req.WorldID уже
// идёт — намерение записывается отдельным UPDATE (полёт не перезапускается).
func TestStartTravelIdempotentWithDestinationWrites(t *testing.T) {
	h, tm, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	// Первый полёт: w1 -> w2 (обычный, без destination).
	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	// D2 (спека 2026-09-23 §3.2): позиции нет → снимок не пишется.
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).AddRow(nil))
	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)

	// Повторный /travel с destination к той же системе: 202-путь — валидация
	// объекта (§3.1) + запись намерения отдельным UPDATE. Порядок запросов:
	// цель → пользователь → планеты (валидация destination) → fromWorld.
	expectWorld(mock, target, 10, 0)
	expectUser(mock, userID, fromWorld)
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(target).
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", target, "Планета1", 0, `{"type":"землеподобная","orbit_radius_au":1.0}`, now(), now()))
	expectWorld(mock, fromWorld, 0, 0)
	mock.ExpectExec(`UPDATE users SET pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
		`{"world_id":"w2","destination":{"object_type":"planet","object_id":"p1"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec = execJSON(h.StartTravel, withUserID(req, userID))
	require.Equal(t, http.StatusAccepted, rec.Code)

	// Полёт не перезапущен (202, без сброса прогресса).
	flight := tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, target, flight.ToWorld)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== РЕГРЕССИЯ ====================

func TestStartTravelAlreadyInThisWorld(t *testing.T) {
	h, _, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const world = "w1"

	// Цель = текущий мир: 400 без запуска полёта. Четвёртый expectWorld —
	// координаты текущего мира: 400 стоит после ветки редиректа (66a),
	// поэтому fromWorld фетчится до проверки.
	expectWorld(mock, world, 0, 0)
	expectUser(mock, userID, world)
	expectWorld(mock, world, 0, 0)
	expectWorld(mock, world, 0, 0)

	rec := execJSON(h.StartTravel, travelRequest(userID, world))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Already in this world")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ВАЛИДАЦИЯ ДВИГАТЕЛЯ (спека 91a §6.1) ====================

// Player без установленного двигателя не летает: 400 «Двигатель не установлен».
func TestStartTravelNoEngine(t *testing.T) {
	h, _, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	// Цель и пользователь (без двигателя) — дальше валидация останавливает.
	expectWorld(mock, target, 10, 0)
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(userRowNoEngine(userID, fromWorld))

	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Двигатель не установлен — полёт невозможен")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Admin/skycomposer без двигателя летает всегда (исключение 91a §6.1).
func TestStartTravelAdminNoEngine(t *testing.T) {
	h, tm, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectWorld(mock, target, 10, 0)
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(userRowAdminNoEngine(userID, fromWorld))
	expectWorld(mock, fromWorld, 0, 0)
	expectWorld(mock, fromWorld, 0, 0)

	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NotNil(t, tm.GetFlight(userID), "админ без двигателя летает")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Player с установленным двигателем летает; длительность — из двигателя (0.3).
func TestStartTravelWithEngine(t *testing.T) {
	h, tm, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight := tm.GetFlight(userID)
	require.NotNil(t, flight)
	require.Equal(t, 3*time.Second, flight.Duration, "dist=10 → 3 сек (минимум 66a)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ПРИБЫТИЕ: ПОЗИЦИЯ «ОРБИТА ЗВЕЗДЫ» (ИП-2, спека 99.2.27 §3.6.3) ====================

// Прибытие межзвёздного полёта пишет current_world_id + current_position =
// «орбита звезды» одним UPDATE (С-1): позиция никогда не остаётся битой.
func TestStartTravelArrivalWritesStarOrbitPosition(t *testing.T) {
	h, _, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)

	// Прибытие (короткий полёт): дефенсив onArrival (пакман, спека 2026-09-20
	// §7.2) проверяет существование цели → цель жива → атомарный UPDATE мира
	// + позиции.
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(target).
		WillReturnRows(travelWorldRow(target, 10, 0))
	mock.ExpectExec(`UPDATE users SET current_world_id = \$1, current_position = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs(target, sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Спека 99.2.30 §4.1: после ИП-2 onArrival читает намерение — NULL →
	// обычное прибытие (ничего не меняется).
	mock.ExpectQuery(`SELECT pending_destination FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"pending_destination"}).AddRow(nil))

	// Ждём, пока onArrival выполнит UPDATE (ExpectationsWereMet == nil — все
	// ожидания, включая UPDATE, потреблены; полёт удаляется из map ДО onArrival,
	// поэтому ждать GetFlight == nil недостаточно — под -race горутина может
	// не успеть до закрытия БД в t.Cleanup).
	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond)
}

// ==================== ДЕФЕНСИВ onArrival (пакман, спека 2026-09-20 §7.2) ====================

// Цель съедена между запросом и прибытием: onArrival обнуляет
// current_world_id/current_position (ClearCurrentWorld) вместо FK-violation
// (users.current_world_id → worlds NO ACTION).
func TestStartTravelArrivalEatenWorldClearsPosition(t *testing.T) {
	h, _, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)

	// Прибытие: цель съедена (GetByID → nil) → ClearCurrentWorld.
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(target).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`UPDATE users SET current_world_id = NULL, current_position = NULL, updated_at = NOW\(\) WHERE id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// ИН-3е (спека 99.2.30 §4.2): мир съеден — намерение тоже очищается.
	mock.ExpectExec(`UPDATE users SET pending_destination = NULL, updated_at = NOW\(\) WHERE id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond)
}

// ==================== ВОЗВРАТ В МИР ОТПРАВЛЕНИЯ ====================

// TestStartTravelReturnToFromWorld — во время полёта выбор мира отправления
// работает как обычный редирект (66a): корабль разворачивается из текущей
// точки P и летит обратно с честной длительностью, а не телепортируется.
func TestStartTravelReturnToFromWorld(t *testing.T) {
	h, tm, mock := newTravelHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1" // A (0,0)
	const target = "w2"    // B (10,0)

	// Старт A->B.
	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NotNil(t, tm.GetFlight(userID))

	// Возврат: /travel с целью A при активном полёте -> редирект из точки P.
	// P на отрезке A->B по прогрессу (elapsed ~5 мс из 3 с): P ≈ (0.017, 0).
	time.Sleep(5 * time.Millisecond)
	expectTravelQueriesRedirect(mock, userID, fromWorld, 0, 0, fromWorld, 0, 0, target, 10, 0)
	rec = execJSON(h.StartTravel, travelRequest(userID, fromWorld))
	require.Equal(t, http.StatusAccepted, rec.Code)

	flight := tm.GetFlight(userID)
	require.NotNil(t, flight, "полёт не отменён, а заменён редиректом")
	require.Equal(t, fromWorld, flight.ToWorld, "цель = мир отправления A")
	require.Equal(t, fromWorld, flight.FromWorld, "from остаётся миром отправления A")
	require.Greater(t, flight.StartX, 0.0, "стартовая точка = P: прогресс > 0")
	require.Less(t, flight.StartX, 10.0, "стартовая точка = P: не в старой цели B")
	require.Equal(t, 0.0, flight.StartY, "P лежит на линии A->B (обе Y=0)")
	// Честная длительность: dist(P, A) * 0.3, мин 3 сек. P ≈ 0.017 -> 3 сек.
	require.Equal(t, 3*time.Second, flight.Duration, "dist(P,A) мал -> минимум 3 сек")

	// Ответ — обычный TravelResponse: from == to == A, стартовая точка = P.
	var resp TravelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, fromWorld, resp.From)
	require.Equal(t, fromWorld, resp.To)
	require.Equal(t, int(flight.Duration.Seconds()), resp.Duration)
	require.Equal(t, flight.StartX, resp.StartX)
	require.Equal(t, flight.StartY, resp.StartY)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== АВТОСТАРТ КОМПОЗИТНОГО МАРШРУТА (спека 99.2.30 §4) ====================

// newTravelHarnessWithIntrasystem — полный харнесс композитного маршрута:
// intraRepo/intraManager (StartAtomic/StartIntraFlight) + planetRepo/
// knowledgeRepo (валидация «объект жив», авто-знание). Для тестов
// ArrivalHandler/autostartIntra/RestorePendingDestinations (спека 99.2.30 §4).
func newTravelHarnessWithIntrasystem(t *testing.T) (*TravelHandlers, *travel.Manager, *travel.IntrasystemManager, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager(nil)
	intraRepo := repository.NewPlayerIntrasystemFlightRepository(db)
	intraManager := travel.NewIntrasystemManager(nil) // nil store — без БД-дубля Upsert
	h := NewTravelHandlers(
		repository.NewWorldRepository(db),
		repository.NewUserRepository(db),
		tm,
	)
	h.SetIntrasystem(intraManager, intraRepo)
	h.SetIntrasystemAutostart(repository.NewPlanetRepository(db), repository.NewKnowledgeRepository(db))
	return h, tm, intraManager, mock
}

// travelPlanetRow — строка планеты для sqlmock (порядок колонок GetPlanetByID/
// GetPlanetsLightByWorldID). Имя с префиксом travel — в пакете уже есть
// planetRow (intrasystem_handlers_test.go, другой формат).
func travelPlanetRow(id, worldID, data string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
		AddRow(id, worldID, "Планета "+id, 0, data, now(), now())
}

// expectPlanetByID — ожидание GetPlanetByID (валидация «объект жив»,
// arrivalTargetValid): планета + пустые поселения (attachSettlements без
// записей — лог поселения не читается) + пустые фракции/строения
// (attachFactionsAndBuildings, спека 2026-09-21-фабрики-релиз-2).
func expectPlanetByID(mock sqlmock.Sqlmock, id, worldID string) {
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(travelPlanetRow(id, worldID, `{}`))
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat"}))
	expectEmptyFactionsBuildings(mock)
}

// expectPlanetsLight — ожидание GetPlanetsLightByWorldID (радиус орбиты цели
// для длительности автостарта).
func expectPlanetsLight(mock sqlmock.Sqlmock, worldID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE world_id = \$1 ORDER BY orbit_index ASC`).
		WithArgs(worldID).
		WillReturnRows(rows)
}

// expectStartAtomic — ожидание StartAtomic (С-1): строка полёта +
// current_position = in_flight одной транзакцией.
func expectStartAtomic(mock sqlmock.Sqlmock, userID, worldID, toType, toID string) {
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO player_intrasystem_flights \(user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs(userID, worldID, "star", worldID, toType, toID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

// expectClearPendingDestination — ожидание очистки намерения.
func expectClearPendingDestination(mock sqlmock.Sqlmock, userID string) {
	mock.ExpectExec(`UPDATE users SET pending_destination = NULL, updated_at = NOW\(\) WHERE id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectArrivalBase — ожидания ArrivalHandler до чтения намерения: дефенсив
// мира (GetByID) + ИП-2 (current_world_id + позиция «орбита звезды»).
func expectArrivalBase(mock sqlmock.Sqlmock, userID, target string, targetX, targetY float64) {
	expectWorld(mock, target, targetX, targetY)
	mock.ExpectExec(`UPDATE users SET current_world_id = \$1, current_position = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs(target, sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectPendingDestination — ожидание чтения намерения.
func expectPendingDestination(mock sqlmock.Sqlmock, userID, destJSON string) {
	mock.ExpectQuery(`SELECT pending_destination FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"pending_destination"}).AddRow(destJSON))
}

// (а) Намерение совпало → автостарт: intra-строка + позиция in_flight +
// очистка намерения (спека 99.2.30 §4.3/§4.4).
func TestArrivalHandlerAutostartSuccess(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w2","object_type":"planet","object_id":"p1"}`)
	// autostartIntra: мир → объект жив → двигатель есть → current_world_id ==
	// w2 → длительность от звезды → StartAtomic → очистка намерения.
	expectWorld(mock, target, 10, 0)
	expectPlanetByID(mock, "p1", target)
	expectUser(mock, userID, target)
	expectPlanetsLight(mock, target, travelPlanetRow("p1", target, `{"orbit_radius_au":1.0}`))
	expectBelts(mock, target) // пояса мира (спека поясов этап 2 §5.7)
	expectStartAtomic(mock, userID, target, "planet", "p1")
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())

	f := intraManager.GetIntraFlight(userID)
	require.NotNil(t, f, "автостарт запустил внутрисистемный полёт")
	require.Equal(t, target, f.WorldID)
	require.Equal(t, "planet", f.ToType)
	require.Equal(t, "p1", f.ToID)
}

// (б) Битая цель → фолбэк «орбита звезды» + очистка, БЕЗ 400 (спека 99.2.30
// §4.3.1): автостарт не запускается, намерение очищается.
func TestArrivalHandlerAutostartBrokenTarget(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w2","object_type":"planet","object_id":"p999"}`)
	// autostartIntra: мир → объект бит (GetPlanetByID → nil) → очистка, без старта.
	expectWorld(mock, target, 10, 0)
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs("p999").
		WillReturnError(sql.ErrNoRows)
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID), "битая цель — полёт не стартует")
}

// (в) Двигатель снят → очистка, позиция «орбита звезды» (спека 99.2.30
// §4.3.2): автостарт не запускается, намерение очищается.
func TestArrivalHandlerAutostartNoEngine(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w2","object_type":"planet","object_id":"p1"}`)
	expectWorld(mock, target, 10, 0)
	expectPlanetByID(mock, "p1", target)
	// Игрок без двигателя (role=player) — автостарт не запускается.
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(userRowNoEngine(userID, target))
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID), "двигатель снят — полёт не стартует")
}

// (г) world_id не совпал → очистка (дефенсив, спека 99.2.30 §4.1): намерение
// пишется только для цели полёта — несовпадение означает битое состояние.
func TestArrivalHandlerWorldMismatch(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w9","object_type":"planet","object_id":"p1"}`)
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID), "world_id не совпал — полёт не стартует")
}

// Задача 2 (ревью): автостарт корректен, только если игрок уже в системе-цели
// (current_world_id == worldID). Краш между CancelAtomicWithDestination и
// StartFlight оставляет намерение при current_world_id мира отправления —
// намерение осиротело: очистить, позицию не трогать (ИП-1 99.2.27).
func TestAutostartIntraWorldMismatchClears(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"
	const departure = "w1"

	// autostartIntra напрямую (фаза 3а Restore): объект жив, но игрок ещё в
	// мире отправления (прибытие не засчитано) → очистка, без старта.
	expectWorld(mock, target, 10, 0)
	expectPlanetByID(mock, "p1", target)
	expectUser(mock, userID, departure) // current_world_id = w1 != w2
	expectClearPendingDestination(mock, userID)

	h.autostartIntra(userID, target, &models.PendingDestination{WorldID: target, ObjectType: "planet", ObjectID: "p1"})
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID), "игрок не в системе-цели — полёт не стартует")
}

// ==================== АВТОСТАРТ КОМПАНЬОНА (решение создателя 2026-09-21) ====================

// Намерение {companion, companion:w2} → автостарт: внутрисистемный слой ждёт
// ToType='star' + синтетический id (99.2.27 §3.1), позиция in_flight.
func TestArrivalHandlerAutostartCompanion(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w2","object_type":"companion","object_id":"companion:w2"}`)
	// autostartIntra: мир (валидность компаньона) → двигатель → планеты (радиус).
	expectWorldWithMods(mock, target, 10, 0, testCompanionMods)
	expectUser(mock, userID, target)
	expectPlanetsLight(mock, target, travelPlanetRow("p1", target, `{"orbit_radius_au":1.0}`))
	expectBelts(mock, target) // пояса мира (спека поясов этап 2 §5.7)
	// StartAtomic: строка полёта — ToType='star' (маппинг компаньона), позиция
	// in_flight с теми же to_type/to_id.
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO player_intrasystem_flights \(user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs(userID, target, "star", target, "star", "companion:w2", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(jsonContains{[]string{`"status":"in_flight"`, `"from_type":"star"`, `"from_id":"w2"`, `"to_type":"star"`, `"to_id":"companion:w2"`}}, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())

	f := intraManager.GetIntraFlight(userID)
	require.NotNil(t, f, "автостарт компаньона запустил внутрисистемный полёт")
	require.Equal(t, "star", f.ToType, "внутрисистемный слой ждёт ToType='star'")
	require.Equal(t, "companion:w2", f.ToID, "синтетический id компаньона сохранён")
}

// Битая цель-компаньон (нет в stellar_mods) → очистка намерения + фолбэк
// «орбита звезды», БЕЗ 400 (позиция уже выставлена onArrival, §4.3.1).
func TestArrivalHandlerAutostartBrokenCompanion(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w2","object_type":"companion","object_id":"companion:w9"}`)
	// autostartIntra: мир загружен → IsValidCompanionID(companion:w9) == false.
	expectWorldWithMods(mock, target, 10, 0, testCompanionMods)
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID), "битый компаньон — полёт не стартует")
}

// ==================== RESTORE НАМЕРЕНИЙ (спека 99.2.30 §4.5) ====================

// expectListPendingDestinations — ожидание ListPendingDestinations.
func expectListPendingDestinations(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT id, pending_destination FROM users WHERE pending_destination IS NOT NULL`).
		WillReturnRows(rows)
}

// (а) Нет активного полёта, объект жив → автостарт (StartAtomic +
// StartIntraFlight; намерение — через ветку «исполнение», §4.4).
func TestRestorePendingDestinationsAutostart(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectListPendingDestinations(mock, sqlmock.NewRows([]string{"id", "pending_destination"}).
		AddRow(userID, `{"world_id":"w2","object_type":"planet","object_id":"p1"}`))
	expectWorld(mock, target, 10, 0) // мир жив (ветка д не срабатывает)
	// autostartIntra: мир → объект жив → двигатель есть → current_world_id ==
	// w2 → StartAtomic → очистка.
	expectWorld(mock, target, 10, 0)
	expectPlanetByID(mock, "p1", target)
	expectUser(mock, userID, target)
	expectPlanetsLight(mock, target, travelPlanetRow("p1", target, `{"orbit_radius_au":1.0}`))
	expectBelts(mock, target) // пояса мира (спека поясов этап 2 §5.7)
	expectStartAtomic(mock, userID, target, "planet", "p1")
	expectClearPendingDestination(mock, userID)

	h.RestorePendingDestinations()
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, intraManager.GetIntraFlight(userID), "автостарт запустил внутрисистемный полёт")
}

// (б) Нет активного полёта, объект бит → очистка намерения (фолбэк «орбита
// звезды», позиция уже выставлена onArrival).
func TestRestorePendingDestinationsBrokenTarget(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	expectListPendingDestinations(mock, sqlmock.NewRows([]string{"id", "pending_destination"}).
		AddRow(userID, `{"world_id":"w2","object_type":"planet","object_id":"p999"}`))
	expectWorld(mock, target, 10, 0)
	expectWorld(mock, target, 10, 0)
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs("p999").
		WillReturnError(sql.ErrNoRows)
	expectClearPendingDestination(mock, userID)

	h.RestorePendingDestinations()
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID))
}

// (в) Нет активного межзвёздного, но есть активный внутрисистемный полёт в
// world_id → намерение-призрак микро-окна StartAtomic→NULL: очистить.
func TestRestorePendingDestinationsGhostIntra(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	// Активный внутрисистемный полёт в w2 (исполнение уже произошло).
	intraManager.StartIntraFlight(userID, target, "star", target, "planet", "p1", 3*time.Second, nil)

	expectListPendingDestinations(mock, sqlmock.NewRows([]string{"id", "pending_destination"}).
		AddRow(userID, `{"world_id":"w2","object_type":"planet","object_id":"p1"}`))
	expectWorld(mock, target, 10, 0)
	expectClearPendingDestination(mock, userID)

	h.RestorePendingDestinations()
	require.NoError(t, mock.ExpectationsWereMet())
}

// (г) Есть активный межзвёздный полёт к world_id → намерение живёт (автостарт
// по прибытии; очистки нет).
func TestRestorePendingDestinationsInterstellarAlive(t *testing.T) {
	h, tm, _, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	// Активный межзвёздный полёт к w2.
	tm.StartFlight(userID, "w1", target, 0, 0, 3*time.Second, nil)

	expectListPendingDestinations(mock, sqlmock.NewRows([]string{"id", "pending_destination"}).
		AddRow(userID, `{"world_id":"w2","object_type":"planet","object_id":"p1"}`))
	expectWorld(mock, target, 10, 0)

	h.RestorePendingDestinations()
	require.NoError(t, mock.ExpectationsWereMet())
	// Намерение не очищено (нет ожидания ClearPendingDestination) — автостарт
	// произойдёт по прибытии.
}

// (д) Мир world_id съеден/удалён → очистить намерение.
func TestRestorePendingDestinationsWorldEaten(t *testing.T) {
	h, _, _, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	expectListPendingDestinations(mock, sqlmock.NewRows([]string{"id", "pending_destination"}).
		AddRow(userID, `{"world_id":"w9","object_type":"planet","object_id":"p1"}`))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("w9").
		WillReturnError(sql.ErrNoRows)
	expectClearPendingDestination(mock, userID)

	h.RestorePendingDestinations()
	require.NoError(t, mock.ExpectationsWereMet())
}

// Задача 2 (ревью): фаза 3а Restore — автостарт корректен, только если игрок
// уже в системе-цели. Краш между CancelAtomicWithDestination и StartFlight
// оставил current_world_id = мир отправления → намерение осиротело: очистить,
// позицию не трогать (ИП-1 99.2.27).
func TestRestorePendingDestinationsWorldMismatchClears(t *testing.T) {
	h, _, intraManager, mock := newTravelHarnessWithIntrasystem(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"
	const departure = "w1"

	expectListPendingDestinations(mock, sqlmock.NewRows([]string{"id", "pending_destination"}).
		AddRow(userID, `{"world_id":"w2","object_type":"planet","object_id":"p1"}`))
	expectWorld(mock, target, 10, 0)
	expectWorld(mock, target, 10, 0)
	expectPlanetByID(mock, "p1", target)
	expectUser(mock, userID, departure) // current_world_id = w1 != w2
	expectClearPendingDestination(mock, userID)

	h.RestorePendingDestinations()
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, intraManager.GetIntraFlight(userID), "игрок не в системе-цели — полёт не стартует")
}

// ==================== B2a: закрытие контрактов-перелётов ====================

// travelCompleteRow — RETURNING-строка завершения (8 колонок).
func travelCompleteRow(id, executorID string, amount int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "author_type", "author_id", "executor_type", "executor_id",
		"escrow_amount", "escrow_withdrawable", "funding",
	}).AddRow(id, "faction", "f1", "player", executorID, amount, 0, "regular")
}

// expectTravelCompleteStmt — оператор завершения + выпуск залога без
// Begin/Commit: CloseTravelArrivals ведёт весь проход в одной транзакции.
func expectTravelCompleteStmt(mock sqlmock.Sqlmock, executorID string, amount int64) {
	mock.ExpectQuery(`(?s)UPDATE contracts\s+SET status = 'completed'.*expires_at > NOW\(\)`).
		WithArgs("c1", executorID).
		WillReturnRows(travelCompleteRow("c1", executorID, amount))
	mock.ExpectExec(`INSERT INTO accounts`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3`).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(amount))
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectTravelExpireDueStmt — оператор истечения без Begin/Commit (0 строк):
// в одной транзакции с завершением.
func expectTravelExpireDueStmt(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`UPDATE contracts\s+SET status = 'expired'`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id",
			"escrow_amount", "escrow_withdrawable"}))
}

// Межзвёздная точка: прибытие к звезде закрывает контракт-перелёт с
// dest_planet_id IS NULL (спека §1.1). Цель-планета здесь НЕ закрывается
// (условие в SQL).
func TestArrivalHandlerClosesSystemTravelContract(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"

	h := NewTravelHandlers(repository.NewWorldRepository(db), repository.NewUserRepository(db), travel.NewManager(nil))
	h.SetContracts(repository.NewContractRepository(db))

	expectArrivalBase(mock, userID, target, 10, 0)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM contracts\s+WHERE type = 'travel' AND status = 'taken' AND executor_type = \$1 AND executor_id = \$2\s+AND payload->>'dest_world_id' = \$3 AND payload->>'dest_planet_id' IS NULL`).
		WithArgs("player", userID, target).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("c1"))
	expectTravelExpireDueStmt(mock)
	expectTravelCompleteStmt(mock, userID, 500)
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT pending_destination FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"pending_destination"}).AddRow(nil))

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Внутрисистемная точка: прибытие intra-полёта к планете-цели закрывает
// контракт-перелёт с dest_planet_id = прибывшая планета (спека §1.1).
func TestIntraArrivalClosesPlanetTravelContract(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	const userID = "22222222-2222-2222-2222-222222222222"

	// 1. Валидация цели прибытия: планета существует в системе (полное чтение
	// GetPlanetByID: планета + поселения + фракции/строения).
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
	// 3. Снимок присутствия (FixatePresence, спека 2026-09-23 §3.2).
	expectPlanetByID(mock, "p1", "w1")
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs(userID, "p1", sqlmock.AnyArg(), "presence").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 4. Закрытие контракта-перелёта (цель-планета) — одной транзакцией.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM contracts\s+WHERE type = 'travel' AND status = 'taken' AND executor_type = \$1 AND executor_id = \$2\s+AND payload->>'dest_world_id' = \$3 AND payload->>'dest_planet_id' = \$4`).
		WithArgs("player", userID, "w1", "p1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("c1"))
	expectTravelExpireDueStmt(mock)
	expectTravelCompleteStmt(mock, userID, 500)
	mock.ExpectCommit()

	intraRepo := repository.NewPlayerIntrasystemFlightRepository(db)
	planetRepo := repository.NewPlanetRepository(db)
	knowledgeRepo := repository.NewKnowledgeRepository(db)
	intraMgr := travel.NewIntrasystemManager(nil)
	intraMgr.StartIntraFlight(userID, "w1", "star", "w1", "planet", "p1", 30*time.Millisecond,
		NewIntraArrivalHandler(intraRepo, planetRepo, knowledgeRepo, repository.NewContractRepository(db)))

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond)
}

// Фолбэк битой цели: внутрисистемное прибытие к несуществующей планете НЕ
// закрывает контракт (arrivedAtTarget=false — «прибыл» на орбиту звезды).
func TestIntraArrivalBrokenTargetDoesNotCloseTravelContract(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	const userID = "33333333-3333-3333-3333-333333333333"
	now := time.Now()

	// 1. Цель бита: планета не найдена (arrivalTargetValid=false) → фолбэк.
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs("p1").
		WillReturnError(sql.ErrNoRows)
	// 2. Фолбэк «орбита звезды»: ArriveAtomic.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1 AND start_time = \$2 AND arrive_at = \$3`).
		WithArgs(userID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 3. Снимок по битой планете: FixatePresence → GetPlanetByID → ErrNoRows
	// (лог, не падение).
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs("p1").
		WillReturnError(sql.ErrNoRows)
	// Ожидание закрытия регистрируем, но оно НЕ должно быть востребовано: тогда
	// ExpectationsWereMet вернёт ошибку — контракт не закрыт.
	mock.ExpectQuery(`SELECT id FROM contracts\s+WHERE type = 'travel'`).
		WithArgs("player", userID, "w1", "p1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	handler := NewIntraArrivalHandler(
		repository.NewPlayerIntrasystemFlightRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewKnowledgeRepository(db),
		repository.NewContractRepository(db),
	)
	handler(userID, &travel.IntraFlightInfo{
		WorldID: "w1", FromType: "star", FromID: "w1", ToType: "planet", ToID: "p1",
		StartTime: now, ArriveAt: now,
	})

	require.Error(t, mock.ExpectationsWereMet(), "битая цель: контракт не закрывается")
}

// Межзвёздная точка закрывает только контракты с dest_planet_id IS NULL:
// цель-планета (dest_planet_id <> NULL) сюда не попадает — предикат SQL.
func TestArrivalHandlerSystemPointOnlyNullPlanetTarget(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	const userID = "44444444-4444-4444-4444-444444444444"
	const target = "w2"

	h := NewTravelHandlers(repository.NewWorldRepository(db), repository.NewUserRepository(db), travel.NewManager(nil))
	h.SetContracts(repository.NewContractRepository(db))

	expectArrivalBase(mock, userID, target, 10, 0)
	// Планетная цель отфильтрована предикатом IS NULL → пустая выборка; ничего
	// не закрывается (нет ExpireDue/Complete).
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM contracts\s+WHERE type = 'travel' AND status = 'taken' AND executor_type = \$1 AND executor_id = \$2\s+AND payload->>'dest_world_id' = \$3 AND payload->>'dest_planet_id' IS NULL`).
		WithArgs("player", userID, target).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT pending_destination FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"pending_destination"}).AddRow(nil))

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ЗАХОД В ПОЯС (спека поясов этап 3 §6.5) ====================

// M20: межзвёздный старт из захода: буфер перелит в трюм в ОДНОЙ транзакции с
// обнулением позиции (§6.5).
func TestStartTravelFromMining(t *testing.T) {
	h, c, mock := newTravelHarnessWithMining(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	// Целевой мир w2.
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("w2").WillReturnRows(travelWorldRow("w2", 1000, 0))
	// Пользователь (GetByID) в w1 — двигатель установлен.
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, race_id FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id",
			"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at", "race_id",
		}).AddRow(userID, "player", "hash", nil, nil, "w1", "ship_strela.svg", nil, "starter",
			`{"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}`, "player", now(), now(), nil))
	// Мир отправления w1 (StartTravel читает его дважды: проверка current_world
	// и стартовая точка сегмента).
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("w1").WillReturnRows(travelWorldRow("w1", 0, 0))
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("w1").WillReturnRows(travelWorldRow("w1", 0, 0))

	// Одна транзакция: FlushTx (лок позиции + резолв железа) → отмена intra.
	mock.ExpectBegin()
	expectMiningFlushTx(mock, userID, "b1", 3)
	expectCancelAtomicTx(mock, userID)
	mock.ExpectCommit()

	rec := execJSON(h.StartTravel, travelRequest(userID, "w2"))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, 1, c.addCalls, "буфер захода перелит в трюм при взлёте")
	assert.Equal(t, int64(21), c.addGood)
	assert.InDelta(t, 3.0, c.addQty, 1e-9)
}
