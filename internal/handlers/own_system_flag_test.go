// internal/handlers/own_system_flag_test.go
// Баг 2026-09-22: «своя система» определяется ЯВНЫМ флагом in_own_system
// (current_world_id == worldID), а не косвенно по наличию my_position. В окне
// прибытия / при межзвёздном полёте my_position пуст, и клиент показывал
// композитную кнопку в своей системе → /travel отвечал 400 «Already in this
// world». Отдельный файл — belt_visibility_test.go не редактируется.
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

	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// ownSysHarness — AdminHandlers с visibility и доступом к менеджеру полётов
// (чтобы поднять состояние «игрок в системе, но my_position пуст»).
func ownSysHarness(t *testing.T) (*AdminHandlers, *travel.Manager, sqlmock.Sqlmock) {
	t.Helper()
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tm := travel.NewManager(nil)
	mc := mapcache.NewManager()
	mc.Replace(visWorlds())

	userRepo := repository.NewUserRepository(db)
	knowledge := repository.NewKnowledgeRepository(db)
	v := NewVisibility(userRepo, tm, mc, knowledge)

	h := NewAdminHandlers(repository.NewWorldRepository(db), db, mc)
	h.SetVisibility(v)
	h.SetTravelManager(tm)
	return h, tm, mock
}

// ownSysResponse — ответ модалки с явным флагом.
type ownSysResponse struct {
	Belts       []map[string]interface{} `json:"belts"`
	MyPosition  *models.CurrentPosition  `json:"my_position"`
	InOwnSystem bool                     `json:"in_own_system"`
}

// expectScanSystem — ожидание ленивого прогона сканера (система в радиусе,
// есть сканер): планетный список для скана, пусто.
func expectScanSystem(mock sqlmock.Sqlmock, worldID string) {
	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\), COALESCE\(p.data->'surface_composition', '\{\}'::jsonb\), \(SELECT COUNT\(\*\) FROM settlements s WHERE s.planet_id = p.id\) FROM planets p WHERE p.world_id = \$1`).
		WithArgs(worldID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}))
}

// РЕГРЕСС (баг): игрок в w2 (current_world_id == w2), но my_position пуст —
// активен межзвёздный полёт (окно прибытия). Сервер обязан отдать явный
// in_own_system = true (клиент покажет внутрисистемную кнопку), а не «чужая
// система» по пустому my_position.
func TestPlanetsInOwnSystemFlagWhenPositionEmpty(t *testing.T) {
	h, tm, mock := ownSysHarness(t)
	const userID = "u1"
	const world = "w2"

	// Полёт w2 → w3: current_world_id остаётся w2, my_position не отдаётся.
	tm.StartFlight(userID, world, "w3", 0, 0, time.Hour, nil)
	t.Cleanup(func() { tm.CancelFlight(userID) })

	expectBeltPlanets(mock, world, []driver.Value{"p1", world, "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, world, "")
	expectPlayerUserWithPosition(mock, userID, world, nil)
	expectKnownWorlds(mock)
	expectBelts(mock, world,
		beltRow("b1", world, "kuiper", "Пояс Койпера", nil, 35.0, 10.0, 0.2, 50.0, `{}`, true, `{}`))
	expectScanSystem(mock, world)

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, world, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp ownSysResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.InOwnSystem, "своя система — явный флаг, независимо от my_position")
	assert.Nil(t, resp.MyPosition, "во время полёта позиция не отдаётся")
	require.Len(t, resp.Belts, 1)
	assert.Equal(t, "b1", resp.Belts[0]["id"])
}

// Чужая система: игрок в w1, модалка w2 → in_own_system = false (композитная
// кнопка остаётся).
func TestPlanetsNotInOwnSystemFlag(t *testing.T) {
	h, _, mock := ownSysHarness(t)
	const userID = "u1"
	const world = "w2"

	expectBeltPlanets(mock, world, []driver.Value{"p1", world, "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, world, "")
	expectPlayerUserWithPosition(mock, userID, "w1", nil)
	expectKnownWorlds(mock)
	expectBelts(mock, world)
	expectScanSystem(mock, world)

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, world, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp ownSysResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.InOwnSystem, "игрок не в этой системе")
	assert.Nil(t, resp.MyPosition)
}

// Своя система с позицией (в поясе): in_own_system = true и my_position
// сохранён (не фолбэк) — баг-фикс не сломал существующее поведение.
func TestPlanetsInOwnSystemFlagWithPosition(t *testing.T) {
	h, _, mock := ownSysHarness(t)
	const userID = "u1"
	const world = "w2"

	pos := `{"status":"orbit","object_type":"belt","object_id":"b1","level":"orbit"}`
	expectBeltPlanets(mock, world, []driver.Value{"p1", world, "Планета1", 0, `{"type":"землеподобная"}`, now(), now()})
	expectBeltPlanetAttachments(mock)
	expectBeltWorldInfo(mock, world, "")
	expectPlayerUserWithPosition(mock, userID, world, pos)
	expectKnownWorlds(mock)
	expectBelts(mock, world,
		beltRow("b1", world, "kuiper", "Пояс Койпера", nil, 35.0, 10.0, 0.2, 50.0, `{}`, true, `{}`))
	expectScanSystem(mock, world)

	rec := execJSON(h.GetPlanetsByWorld, beltModalRequest(userID, world, string(models.RolePlayer)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp ownSysResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.InOwnSystem)
	require.NotNil(t, resp.MyPosition)
	assert.Equal(t, "belt", resp.MyPosition.ObjectType)
	assert.Equal(t, "b1", resp.MyPosition.ObjectID)
}

// Композитный запрос (destination) в СВОЮ систему без активного полёта →
// понятная деградация («используйте внутрисистемный полёт»), НЕ молчаливое
// «Already in this world» (баг 2026-09-22).
func TestTravelCompositeToOwnSystemClearMessage(t *testing.T) {
	h, _, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const world = "w1"

	// targetWorld + user.
	expectWorld(mock, world, 0, 0)
	expectUser(mock, userID, world)
	// validateTravelDestination: планеты + пояс b1 (цель валидна).
	expectPlanetsLight(mock, world, sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}))
	expectBelts(mock, world,
		beltRow("b1", world, "kuiper", "Пояс Койпера", nil, 35.0, 10.0, 0.2, 50.0, `{}`, true, `{}`))
	// fromWorld — дважды: выбор мира отправления (current_world_id) и стартовая
	// точка сегмента (порядок хендлера, как в TestStartTravelAlreadyInThisWorld).
	expectWorld(mock, world, 0, 0)
	expectWorld(mock, world, 0, 0)

	req := httptest.NewRequest(http.MethodPost, "/travel", strings.NewReader(
		`{"world_id":"w1","destination":{"object_type":"belt","object_id":"b1"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := execJSON(h.StartTravel, withUserID(req, userID))

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "используйте внутрисистемный полёт")
	assert.NotContains(t, rec.Body.String(), "Already in this world")
	require.NoError(t, mock.ExpectationsWereMet())
}
