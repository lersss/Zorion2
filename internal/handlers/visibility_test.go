// internal/handlers/visibility_test.go
// Серверная видимость игрока (спека 77a §11): круг радара, позиция игрока
// (включая интерполяцию в полёте), проверка принадлежности.
package handlers

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// newVisibilityHarness — Visibility с sqlmock-БД, реальным travel.Manager и
// снапшотом mapcache (миры w1(0,0), w2(100,0), w3(0,100)).
func newVisibilityHarness(t *testing.T) (*Visibility, sqlmock.Sqlmock, *travel.Manager) {
	t.Helper()
	ship.LoadDefaults() // каталог оборудования: radar_1 → 800 px
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager()
	mc := mapcache.NewManager()
	mc.Replace(mapcache.NewSnapshot([]mapcache.World{
		{ID: "w1", Name: "Мир1", X: 0, Y: 0},
		{ID: "w2", Name: "Мир2", X: 100, Y: 0},
		{ID: "w3", Name: "Мир3", X: 0, Y: 100},
	}))

	v := NewVisibility(
		repository.NewUserRepository(db),
		tm,
		mc,
		repository.NewKnowledgeRepository(db),
	)
	return v, mock, tm
}

// visUserRow — строка пользователя с оборудованием (radar_1 → 800 px).
func visUserRow(id, world string, equipment string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "password_hash", "email", "agent_id", "current_world_id",
		"ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at",
	}).AddRow(id, "player", "hash", nil, nil, world, "ship_strela.svg", nil, "starter", equipment, "player", now(), now())
}

func TestIsVisible(t *testing.T) {
	require.True(t, IsVisible(0, 0, 0, 0, 400), "центр в круге")
	require.True(t, IsVisible(300, 0, 0, 0, 400), "внутри круга")
	require.False(t, IsVisible(401, 0, 0, 0, 400), "за кругом")
	require.True(t, IsVisible(400, 0, 0, 0, 400), "на границе — видима (≤)")
}

func TestRadarRadius(t *testing.T) {
	v, mock, _ := newVisibilityHarness(t)
	defer mock.ExpectationsWereMet()

	// С радаром radar_1 → 800.
	user := &models.User{Equipment: map[string]interface{}{"radar": "radar_1"}}
	require.Equal(t, models.RadarRadiusDefault, v.RadarRadius(user))

	// Без радара → 200.
	require.Equal(t, models.RadarRadiusMin, v.RadarRadius(&models.User{}))
	require.Equal(t, models.RadarRadiusMin, v.RadarRadius(nil))
}

func TestPlayerPositionNoFlight(t *testing.T) {
	v, mock, _ := newVisibilityHarness(t)

	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("u1").
		WillReturnRows(visUserRow("u1", "w2", `{"radar":"radar_1","scanner":"scanner_1","engine":null}`))

	user, err := v.userRepo.GetByID("u1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	x, y, ok := v.PlayerPosition(user)
	require.True(t, ok)
	require.Equal(t, 100.0, x, "позиция = координаты current_world_id")
	require.Equal(t, 0.0, y)
}

func TestPlayerPositionInFlight(t *testing.T) {
	v, _, tm := newVisibilityHarness(t)

	// Полёт w1(0,0) → w2(100,0), длительность 100 сек, старт 10 сек назад →
	// прогресс 0.1 → позиция (10, 0).
	tm.StartFlight("u1", "w1", "w2", 0, 0, 100*time.Second, nil)
	flight := tm.GetFlight("u1")
	flight.StartTime = time.Now().Add(-10 * time.Second)

	user := &models.User{ID: "u1", CurrentWorldID: strPtr("w1")}
	x, y, ok := v.PlayerPosition(user)
	require.True(t, ok)
	require.InDelta(t, 10.0, x, 0.5, "интерполяция по прогрессу полёта")
	require.InDelta(t, 0.0, y, 0.5)
}

func TestPlayerPositionUnknownWorld(t *testing.T) {
	v, mock, _ := newVisibilityHarness(t)

	// Мир удалён при перегенерации — позиция неизвестна.
	user := &models.User{ID: "u1", CurrentWorldID: strPtr("gone")}
	_, _, ok := v.PlayerPosition(user)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInterpolateFlight(t *testing.T) {
	f := &travel.TravelInfo{StartX: 0, StartY: 0, Duration: 100 * time.Second}
	f.StartTime = time.Now().Add(-50 * time.Second) // прогресс 0.5

	x, y := interpolateFlight(f, 100, 200)
	require.InDelta(t, 50.0, x, 0.5)
	require.InDelta(t, 100.0, y, 0.5)

	// Прогресс зажат: elapsed > duration → цель.
	f.StartTime = time.Now().Add(-500 * time.Second)
	x, y = interpolateFlight(f, 100, 200)
	require.Equal(t, 100.0, x)
	require.Equal(t, 200.0, y)

	// Прогресс зажат: elapsed < 0 → старт.
	f.StartTime = time.Now().Add(500 * time.Second)
	x, y = interpolateFlight(f, 100, 200)
	require.Equal(t, 0.0, x)
	require.Equal(t, 0.0, y)
}

func strPtr(s string) *string { return &s }