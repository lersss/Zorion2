// internal/handlers/accelerator_block_test.go
// Тесты блока accelerator в контрактах /travel и /me (спека ускорителя §3.4,
// ЧК1): приоритет причин no_module → unknown_game → already_active → too_short
// → cooldown, признак active (UnixMilli), применённый откат в /me. С ЧК3 игра
// route зарегистрирована (ship.AcceleratorGameRegistered) → валидный `accel_1`
// даёт available по состоянию сегмента; ветку unknown_game держит фиктивное
// имя игры в TestAcceleratorTravelReason.
package handlers

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

func accelRepoMock(t *testing.T, state [2]interface{}) (*repository.PlayerAcceleratorRepository, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	q := `SELECT last_boost_at, last_cooldown_min FROM player_accelerator WHERE user_id = \$1`
	rows := sqlmock.NewRows([]string{"last_boost_at", "last_cooldown_min"}).AddRow(state[0], state[1])
	mock.ExpectQuery(q).WithArgs("u1").WillReturnRows(rows)
	return repository.NewPlayerAcceleratorRepository(db), mock
}

func accelUser(equipment map[string]interface{}) *models.User {
	return &models.User{ID: "u1", Equipment: equipment}
}

func accelBlockMap(t *testing.T, block interface{}) map[string]interface{} {
	t.Helper()
	require.NotNil(t, block)
	m, ok := block.(map[string]interface{})
	require.True(t, ok, "блок — объект")
	return m
}

func TestAcceleratorTravelBlockNoModule(t *testing.T) {
	repo, mock := accelRepoMock(t, [2]interface{}{nil, nil})
	flight := &travel.TravelInfo{StartTime: time.Now(), Duration: time.Hour}

	m := accelBlockMap(t, acceleratorTravelBlock(repo, accelUser(map[string]interface{}{"radar": "radar_1"}), flight, time.Now()))
	require.Equal(t, false, m["available"])
	require.Equal(t, "no_module", m["reason"])
	require.Equal(t, false, m["active"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// ЧК3: игра route зарегистрирована → валидный модуль при доступном сегменте
// даёт available=true (не unknown_game).
func TestAcceleratorTravelBlockAvailable(t *testing.T) {
	repo, _ := accelRepoMock(t, [2]interface{}{nil, nil})
	flight := &travel.TravelInfo{StartTime: time.Now(), Duration: time.Hour}

	m := accelBlockMap(t, acceleratorTravelBlock(repo, accelUser(map[string]interface{}{"accelerator": "accel_1"}), flight, time.Now()))
	require.Equal(t, true, m["available"])
	require.NotContains(t, m, "reason")
	require.Equal(t, "accel_1", m["module_id"])
	require.Equal(t, "route", m["game"])
	require.Equal(t, false, m["active"])
	require.Nil(t, m["cooldown_remaining_s"])
}

// Действующее ускорение на сегменте → available=false, reason=already_active;
// `active` и таймер отката отдаются (их читает клиент).
func TestAcceleratorTravelBlockAlreadyActive(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	repo, _ := accelRepoMock(t, [2]interface{}{start, 25})
	flight := &travel.TravelInfo{StartTime: start, Duration: time.Hour}

	m := accelBlockMap(t, acceleratorTravelBlock(repo, accelUser(map[string]interface{}{"accelerator": "accel_1"}), flight, time.Now()))
	require.Equal(t, true, m["active"])
	require.Equal(t, false, m["available"])
	require.Equal(t, "already_active", m["reason"])
	require.NotNil(t, m["cooldown_remaining_s"])
}

// Приоритет причин (§3.4/§4.1): no_module → unknown_game → already_active →
// too_short → cooldown. gameRegistered вынесен параметром, чтобы проверить ветки
// при реализованной игре; ветка unknown_game — на фиктивном имени игры
// (AcceleratorGameRegistered("fake_game") == false). Хелпер — единый источник
// причин для offer/boost/scan: порог остатка и проверка отката параметрами.
func TestAcceleratorTravelReason(t *testing.T) {
	long := time.Hour
	short := 60 * time.Second

	cases := []struct {
		name           string
		found, valid   bool
		gameRegistered bool
		active         bool
		remaining      time.Duration
		minRemainingS  int
		cooldown       time.Duration
		checkCooldown  bool
		want           string
	}{
		{"no_module", false, true, true, false, long, 180, 0, true, "no_module"},
		{"invalid_params", true, false, true, false, long, 180, 0, true, "unknown_game"},
		{"unknown_game (фиктивная игра)", true, true, ship.AcceleratorGameRegistered("fake_game"), true, long, 180, 0, true, "unknown_game"},
		{"already_active", true, true, true, true, long, 180, 0, true, "already_active"},
		{"too_short", true, true, true, false, short, 180, 0, true, "too_short"},
		{"no_remaining", true, true, true, false, 0, 180, 0, true, "too_short"},
		{"cooldown", true, true, true, false, long, 180, time.Minute, true, "cooldown"},
		{"available", true, true, true, false, long, 180, 0, true, ""},
		{"boost_no_cooldown", true, true, true, false, long, 90, time.Minute, false, ""},
		{"scan_skips_thresholds", true, true, true, false, 0, 0, time.Minute, false, ""},
	}
	for _, c := range cases {
		got := acceleratorTravelReason(c.found, c.valid, c.gameRegistered, c.active, c.remaining, c.minRemainingS, c.cooldown, c.checkCooldown)
		require.Equalf(t, c.want, got, "reason %q", c.name)
	}
}

func TestAcceleratorMeBlock(t *testing.T) {
	nowT := time.Now().Truncate(time.Millisecond)
	repo, _ := accelRepoMock(t, [2]interface{}{nowT.Add(-time.Minute), 25})

	m := accelBlockMap(t, acceleratorMeBlock(repo, accelUser(map[string]interface{}{"accelerator": "accel_1"}), nil, time.Now()))
	require.Equal(t, "accel_1", m["module_id"])
	require.Equal(t, false, m["active"])
	require.Equal(t, false, m["ready"])
	require.Equal(t, 25, m["cooldown_min"], "применённый откат (last_cooldown_min)")
	require.NotNil(t, m["last_boost_at"])
	require.NotNil(t, m["cooldown_remaining_s"])
}

// Существующая строка отката с истёкшим временем → готов.
func TestAcceleratorMeBlockReady(t *testing.T) {
	repo, _ := accelRepoMock(t, [2]interface{}{time.Now().Add(-time.Hour), 25})

	m := accelBlockMap(t, acceleratorMeBlock(repo, accelUser(map[string]interface{}{"accelerator": "accel_1"}), nil, time.Now()))
	require.Equal(t, true, m["ready"])
	require.Nil(t, m["cooldown_remaining_s"])
}
