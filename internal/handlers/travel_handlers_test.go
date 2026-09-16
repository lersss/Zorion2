// internal/handlers/travel_handlers_test.go
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
	"zorion/internal/travel"
)

func TestCalcTravelDuration(t *testing.T) {
	tests := []struct {
		name string
		dist float64
		want time.Duration
	}{
		{
			name: "короткое расстояние: время равно dist*0.3",
			dist: 50,
			want: 15 * time.Second,
		},
		{
			name: "большое расстояние: без потолка, dist*0.3 (dist=100 -> 30s)",
			dist: 100,
			want: 30 * time.Second,
		},
		{
			name: "граница старого капа: dist=70 -> 21s, потолок не режет",
			dist: 70,
			want: 21 * time.Second,
		},
		{
			name: "минимум 3 секунды для очень близких миров",
			dist: 1,
			want: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, calcTravelDuration(tt.dist))
		})
	}
}

// ==================== ХЕЛПЕРЫ ====================

// newTravelHarness — sqlmock-БД + репозитории + менеджер полётов.
func newTravelHarness(t *testing.T) (*TravelHandlers, *travel.Manager, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager()
	return NewTravelHandlers(
		repository.NewWorldRepository(db),
		repository.NewUserRepository(db),
		tm,
	), tm, mock
}

// travelWorldRow — строка мира для sqlmock (порядок worldColumns).
func travelWorldRow(id string, x, y float64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
		"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
		"created_at", "updated_at",
	}).AddRow(id, "Мир "+id, x, y, "G", 5772, "star", "single", nil, nil, nil, now(), now())
}

// userRow — строка пользователя для sqlmock (текущий мир — fromWorld).
func userRow(id, fromWorld string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "password_hash", "email", "agent_id", "current_world_id",
		"ship_icon", "ship_color", "role", "created_at", "updated_at",
	}).AddRow(id, "player", "hash", nil, nil, fromWorld, "ship_strela.svg", nil, "player", now(), now())
}

// expectWorld — ожидание SELECT мира по id.
func expectWorld(mock sqlmock.Sqlmock, id string, x, y float64) {
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(travelWorldRow(id, x, y))
}

// expectUser — ожидание SELECT пользователя по id.
func expectUser(mock sqlmock.Sqlmock, id, fromWorld string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users WHERE id = \$1`).
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