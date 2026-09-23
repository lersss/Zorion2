// internal/handlers/presence_snapshot_test.go
// Снимок присутствия (спека 2026-09-23-орбита-планеты-присутствие-и-снимок
// §3.2, тесты T3–T6): фиксация при прибытии (A1/A2) и отлёте (D1/D2),
// определение присутствия на планете (§2.1), снимок пишется ДО смены позиции.
package handlers

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// presencePayloadMatcher — sqlmock-матчер payload записи снимка: JSON содержит
// data.snapshot с at/source и не содержит запрещённых полей (чек-точки,
// R-компоненты, вход веток, эффекты, лог).
type presencePayloadMatcher struct{}

func (presencePayloadMatcher) Match(v driver.Value) bool {
	var raw []byte
	switch x := v.(type) {
	case []byte:
		raw = x
	case string:
		raw = []byte(x)
	default:
		return false
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	snap, ok := data["snapshot"].(map[string]interface{})
	if !ok {
		return false
	}
	if snap["source"] != "presence" {
		return false
	}
	if at, _ := snap["at"].(string); at == "" {
		return false
	}
	rawSnap, err := json.Marshal(snap)
	if err != nil {
		return false
	}
	s := string(rawSnap)
	for _, forbidden := range []string{
		"population_exact", "computed_at", "r_per_sec", "lambda_per_hour", "n_dead",
		`"input"`, `"effects"`, `"log"`,
	} {
		if strings.Contains(s, forbidden) {
			return false
		}
	}
	return true
}

// expectPosition — ожидание GetCurrentPosition (хук D2).
func expectPosition(mock sqlmock.Sqlmock, userID, posJSON string) {
	mock.ExpectQuery(`SELECT current_position FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"current_position"}).AddRow(posJSON))
}

// expectFindPlanetBySatellite — ожидание FindPlanetBySatellite (родитель
// спутника): планета + её world_id.
func expectFindPlanetBySatellite(mock sqlmock.Sqlmock, worldID, satelliteID, parentID string) {
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at\s+FROM planets\s+WHERE world_id = \$1\s+AND EXISTS \(SELECT 1 FROM jsonb_array_elements\(data->'satellites'\) sat WHERE sat->>'id' = \$2\)`).
		WithArgs(worldID, satelliteID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow(parentID, worldID, "Планета", 0, `{}`, now(), now()))
}

// expectArriveAtomic — ожидание прибытия (позиция orbit + удаление строки).
func expectArriveAtomic(mock sqlmock.Sqlmock, userID string) {
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1 AND start_time = \$2 AND arrive_at = \$3`).
		WithArgs(userID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

// expectFixateUpsert — ожидание merge-записи снимка в player_planet_knowledge.
func expectFixateUpsert(mock sqlmock.Sqlmock, userID, planetID string) {
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*DO UPDATE SET data = player_planet_knowledge\.data \|\| EXCLUDED\.data`).
		WithArgs(userID, planetID, presencePayloadMatcher{}, "presence").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// ==================== T4/T6: ОПРЕДЕЛЕНИЕ ПРИСУТСТВИЯ (§2.1) ====================

// presencePlanetID: присутствие — только орбита/поверхность планеты или
// спутник её родителя. Орбита звезды/компаньона, пояс, полёт и NULL —
// присутствием не являются (снимок не пишется).
func TestPresencePlanetID(t *testing.T) {
	tests := []struct {
		name string
		pos  *models.CurrentPosition
		want string
	}{
		{name: "nil", pos: nil, want: ""},
		{name: "орбита планеты", pos: &models.CurrentPosition{Status: "orbit", ObjectType: "planet", ObjectID: "p1"}, want: "p1"},
		{name: "поверхность планеты", pos: &models.CurrentPosition{Status: "surface", ObjectType: "planet", ObjectID: "p1"}, want: "p1"},
		{name: "орбита звезды", pos: &models.CurrentPosition{Status: "orbit", ObjectType: "star", ObjectID: "w1"}, want: ""},
		{name: "орбита компаньона", pos: &models.CurrentPosition{Status: "orbit", ObjectType: "star", ObjectID: "companion:w1"}, want: ""},
		{name: "пояс (mining)", pos: &models.CurrentPosition{Status: "mining", ObjectType: "belt", ObjectID: "b1"}, want: ""},
		{name: "в полёте", pos: &models.CurrentPosition{Status: "in_flight", FromType: "planet", FromID: "p1", ToType: "star", ToID: "w1"}, want: ""},
		{name: "спутник без репозитория", pos: &models.CurrentPosition{Status: "orbit", ObjectType: "satellite", ObjectID: "s1"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, presencePlanetID(tt.pos, "w1", nil))
		})
	}
}

// Присутствие на спутнике = присутствие на родительской планете (п.9, M5).
func TestPresencePlanetID_SatelliteParent(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	expectFindPlanetBySatellite(mock, "w1", "s1", "p1")

	pos := &models.CurrentPosition{Status: "orbit", ObjectType: "satellite", ObjectID: "s1"}
	assert.Equal(t, "p1", presencePlanetID(pos, "w1", repository.NewPlanetRepository(db)))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T3: ОТЛЁТ D2 (старт межзвёздного с планеты) ====================

// D2: старт межзвёздного полёта с орбиты планеты → снимок планеты записан
// ДО NULL-ения позиции (best-effort).
func TestStartTravelDepartureFixatesPlanetPresence(t *testing.T) {
	h, _, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	expectPosition(mock, userID, `{"status":"orbit","object_type":"planet","object_id":"p1"}`)
	expectPlanetByID(mock, "p1", fromWorld)
	expectFixateUpsert(mock, userID, "p1")

	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "снимок планеты записан при отлёте")
}

// D2: старт межзвёздного полёта с орбиты звезды → снимок НЕ пишется.
// Ожидания фиксации регистрируем намеренно — они обязаны остаться
// невыполненными (ExpectationsWereMet = error) — детектор «снимка нет».
func TestStartTravelDepartureNoFixateOffPlanet(t *testing.T) {
	h, _, mock := newTravelHarnessWithAutostart(t)
	const userID = "11111111-1111-1111-1111-111111111111"
	const fromWorld = "w1"
	const target = "w2"

	expectTravelQueries(mock, userID, fromWorld, 0, 0, target, 10, 0)
	expectPosition(mock, userID, `{"status":"orbit","object_type":"star","object_id":"w1"}`)
	// Детектор: снимок не должен писаться.
	expectPlanetByID(mock, "p1", fromWorld)
	expectFixateUpsert(mock, userID, "p1")

	rec := execJSON(h.StartTravel, travelRequest(userID, target))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Error(t, mock.ExpectationsWereMet(), "со звезды снимок не пишется")
}

// ==================== T3: ОТЛЁТ D1 (старт внутрисистемного с планеты) ====================

// D1: старт внутрисистемного полёта с планеты → FixatePresence до StartAtomic.
// sqlmock матчит ожидания по порядку регистрации: снимок идёт ДО позиции.
func TestStartIntraFlightDepartureFixatesPlanetPresence(t *testing.T) {
	h, _, mock := newIntraHarness(t)
	const userID = "11111111-1111-1111-1111-111111111111"

	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectIntraUser(mock, userID, pos)
	expectIntraWorld(mock, "w1")
	expectIntraPlanets(mock, "w1",
		planetRow("p1", "w1", "Планета1", 0, 1.0),
		planetRow("p2", "w1", "Планета2", 1, 2.0))
	// Снимок — ДО смены позиции (D1).
	expectPlanetByID(mock, "p1", "w1")
	expectFixateUpsert(mock, userID, "p1")
	// Позиция меняется только после записи снимка.
	expectIntraStartAtomic(mock, userID, "planet", "p1")

	rec := execJSON(h.StartIntraFlight, intraRequest(userID, "planet", "p2"))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T5: ПРИБЫТИЕ A1/A2 ====================

// A1: прибытие внутрисистемного полёта на планету → снимок этой планеты.
func TestIntraArrivalPlanetFixatesSnapshot(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	const userID = "11111111-1111-1111-1111-111111111111"

	expectPlanetByID(mock, "p1", "w1") // arrivalTargetValid
	expectArriveAtomic(mock, userID)
	expectPlanetByID(mock, "p1", "w1") // FixatePresence
	expectFixateUpsert(mock, userID, "p1")

	handler := NewIntraArrivalHandler(
		repository.NewPlayerIntrasystemFlightRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewKnowledgeRepository(db),
		nil,
	)
	handler(userID, &travel.IntraFlightInfo{
		WorldID: "w1", ToType: "planet", ToID: "p1", StartTime: now(), ArriveAt: now(),
	})
	require.NoError(t, mock.ExpectationsWereMet())
}

// A2: прибытие на спутник → снимок РОДИТЕЛЬСКОЙ планеты (п.9, M5).
func TestIntraArrivalSatelliteFixatesParent(t *testing.T) {
	ship.LoadDefaults()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	const userID = "11111111-1111-1111-1111-111111111111"

	expectFindPlanetBySatellite(mock, "w1", "s1", "p1") // arrivalTargetValid
	expectArriveAtomic(mock, userID)
	expectFindPlanetBySatellite(mock, "w1", "s1", "p1") // FixatePresence → родитель
	expectPlanetByID(mock, "p1", "w1")
	expectFixateUpsert(mock, userID, "p1")

	handler := NewIntraArrivalHandler(
		repository.NewPlayerIntrasystemFlightRepository(db),
		repository.NewPlanetRepository(db),
		repository.NewKnowledgeRepository(db),
		nil,
	)
	handler(userID, &travel.IntraFlightInfo{
		WorldID: "w1", ToType: "satellite", ToID: "s1", StartTime: now(), ArriveAt: now(),
	})
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== T6: НЕ-ОТЛЁТ (ScanSystem не двигает снимок) ====================

// Ленивый скан системы (открытие модалки) — не отлёт: он пишет только
// поверхность и счётчик поселений, без ключа snapshot, а merge-запись
// сохраняет ранее записанный снимок.
func TestScanSystemDoesNotTouchSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\).*FROM planets p WHERE p.world_id = \$1`).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}).
			AddRow("p1", "вода", `{"вода":100}`, 1))
	// Payload сканера обязан не нести snapshot — иначе merge перезапишет снимок.
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*DO UPDATE SET data = player_planet_knowledge\.data \|\| EXCLUDED\.data`).
		WithArgs("u1", "p1", jsonWithoutKeyHandlers{key: "snapshot"}, "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repository.NewKnowledgeRepository(db).ScanSystem("u1", "w1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// jsonWithoutKeyHandlers — локальная копия матчера (репозиторный живёт в
// пакете repository; дублируется здесь сознательно, тесты разных пакетов).
type jsonWithoutKeyHandlers struct{ key string }

func (m jsonWithoutKeyHandlers) Match(v driver.Value) bool {
	var raw []byte
	switch x := v.(type) {
	case []byte:
		raw = x
	case string:
		raw = []byte(x)
	default:
		return false
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false
	}
	_, ok := obj[m.key]
	return !ok
}
