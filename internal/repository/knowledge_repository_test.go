// internal/repository/knowledge_repository_test.go
// Личный каталог знания о планетах (спека 77a §8): чтение по PK, UPSERT,
// «зажжённые» системы (KnownWorldIDs), ленивый прогон сканера (ScanSystem).
// Снимок присутствия (спека 2026-09-23 §3): merge-запись, состав снимка.
package repository

import (
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func TestGetKnowledgeFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs("u1", "p1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}).
			AddRow("u1", "p1", `{"surface_dominant":"вода","settlements_count":2}`, now(), "scanner"))

	k, err := NewKnowledgeRepository(db).GetKnowledge("u1", "p1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, k)
	require.Equal(t, "вода", k.Data["surface_dominant"])
	require.Equal(t, float64(2), k.Data["settlements_count"])
	require.Equal(t, "scanner", k.Source)
}

func TestGetKnowledgeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs("u1", "p1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}))

	k, err := NewKnowledgeRepository(db).GetKnowledge("u1", "p1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, k, "нет записи → nil")
}

func TestUpsertKnowledge(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO player_planet_knowledge \(user_id, planet_id, data, scanned_at, source\).*ON CONFLICT \(user_id, planet_id\).*DO UPDATE`).
		WithArgs("u1", "p1", sqlmock.AnyArg(), "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewKnowledgeRepository(db).UpsertKnowledge("u1", "p1",
		map[string]interface{}{"surface_dominant": "скалы"}, "scanner")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestKnownWorldIDs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT DISTINCT p.world_id FROM player_planet_knowledge k JOIN planets p ON p.id = k.planet_id WHERE k.user_id = \$1`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"world_id"}).
			AddRow("w1").AddRow("w2"))

	known, err := NewKnowledgeRepository(db).KnownWorldIDs("u1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, map[string]bool{"w1": true, "w2": true}, known)
}

func TestScanSystem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Чтение планет системы.
	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\), COALESCE\(p.data->'surface_composition', '\{\}'::jsonb\), \(SELECT COUNT\(\*\) FROM settlements s WHERE s.planet_id = p.id\) FROM planets p WHERE p.world_id = \$1`).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}).
			AddRow("p1", "вода", `{"вода":100}`, 2).
			AddRow("p2", "скалы", `{"скалы":80}`, 0))

	// UPSERT для каждой планеты.
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs("u1", "p1", sqlmock.AnyArg(), "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*ON CONFLICT.*DO UPDATE`).
		WithArgs("u1", "p2", sqlmock.AnyArg(), "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewKnowledgeRepository(db).ScanSystem("u1", "w1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== СНИМОК ПРИСУТСТВИЯ (спека 2026-09-23 §3) ====================

// jsonWithoutKey — sqlmock-матчер: значение (JSON-строка/[]byte) валидно и НЕ
// содержит ключ на верхнем уровне. Гарантия «скан не затирает снимок»: payload
// сканера обязан не нести ключ snapshot (иначе merge `||` перезапишет снимок).
type jsonWithoutKey struct{ key string }

func (m jsonWithoutKey) Match(v driver.Value) bool {
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

// captureArg — sqlmock-матчер: сохраняет переданное значение для дальнейших
// проверок (JSON payload записи знания).
type captureArg struct{ raw []byte }

func (c *captureArg) Match(v driver.Value) bool {
	switch x := v.(type) {
	case []byte:
		c.raw = x
	case string:
		c.raw = []byte(x)
	default:
		return false
	}
	return true
}

// T1: запись знания — merge `data = player_planet_knowledge.data || EXCLUDED.data`,
// поэтому ключи, которых нет во входящем payload (в т.ч. snapshot), сохраняются.
func TestUpsertKnowledgeMergesData(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO player_planet_knowledge \(user_id, planet_id, data, scanned_at, source\).*ON CONFLICT \(user_id, planet_id\) DO UPDATE SET data = player_planet_knowledge\.data \|\| EXCLUDED\.data, scanned_at = NOW\(\), source = EXCLUDED\.source`).
		WithArgs("u1", "p1", sqlmock.AnyArg(), "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewKnowledgeRepository(db).UpsertKnowledge("u1", "p1",
		map[string]interface{}{"surface_dominant": "скалы"}, "scanner"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// T1: Scanner-payload не несёт ключ snapshot → merge сохраняет уже записанный
// снимок (ленивый скан не может его стереть).
func TestScanPlanetPayloadKeepsSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COALESCE\(p.data->>'surface_dominant', ''\), COALESCE\(p.data->'surface_composition', '\{\}'::jsonb\), \(SELECT COUNT\(\*\) FROM settlements s WHERE s.planet_id = p.id\) FROM planets p WHERE p.id = \$1`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"surface_dominant", "surface_composition", "settlements_count"}).
			AddRow("вода", `{"вода":100}`, 2))
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*DO UPDATE SET data = player_planet_knowledge\.data \|\| EXCLUDED\.data`).
		WithArgs("u1", "p1", jsonWithoutKey{key: "snapshot"}, "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewKnowledgeRepository(db).ScanPlanet("u1", "p1", "scanner"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// T1: скан системы — тот же merge, payload без snapshot.
func TestScanSystemPayloadKeepsSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT p.id, COALESCE\(p.data->>'surface_dominant', ''\).*FROM planets p WHERE p.world_id = \$1`).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "surface_dominant", "surface_composition", "settlements_count"}).
			AddRow("p1", "вода", `{"вода":100}`, 2))
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*DO UPDATE SET data = player_planet_knowledge\.data \|\| EXCLUDED\.data`).
		WithArgs("u1", "p1", jsonWithoutKey{key: "snapshot"}, "scanner").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewKnowledgeRepository(db).ScanSystem("u1", "w1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// T2: состав снимка — две расы раздельно, целое население, стабильность, ветки
// с выходом и скалярами; запрещённых полей (чек-точки, R-компоненты, input,
// effects, log) нет.
func TestBuildPresenceSnapshotComposition(t *testing.T) {
	at := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	planet := &models.Planet{
		ID:                 "p1",
		SurfaceDominant:    "горы",
		SurfaceComposition: map[string]float64{"горы": 62.0, "пыль": 38.0},
		Settlements: []models.Settlement{
			{
				ID: "s1", RaceID: "spark", RaceName: "Искры",
				Population: 876000000, PopulationExact: 876000000.4,
				Stability: 69, ComputedAt: at, RPerSec: 0.5, LambdaPerHour: 0.02, NDead: 1.0,
				Effects: []models.ActiveEffect{{}},
				Log:     []models.SettlementLogEntry{{ID: "l1", Type: "extinct"}},
				Branches: []models.SettlementBranch{{
					ID: "b1", RecipeID: 73, RecipeName: "Вода", Complexity: 3,
					Produced: 12.5, Eaten: 4.0, EatenRate: 0.02,
				}},
			},
			{ID: "s2", RaceID: "human", RaceName: "Люди", Population: 1000, Stability: 40},
		},
		Factions:  []models.PlanetFaction{{ID: "f1", Name: "Фракция", Type: "t", Color: "#fff", Description: "d"}},
		Buildings: []models.PlanetBuilding{{ID: "bld1", BuildingType: "capital", OwnerType: "faction", OwnerID: "f1"}},
	}

	snap := buildPresenceSnapshot(planet, "presence", at)

	require.Equal(t, "2026-09-23T12:00:00Z", snap.At)
	require.Equal(t, "presence", snap.Source)
	require.Equal(t, "горы", snap.SurfaceDominant)
	require.Equal(t, map[string]float64{"горы": 62.0, "пыль": 38.0}, snap.SurfaceComposition)

	require.Len(t, snap.Settlements, 2, "две расы — два поселения (М4)")
	require.Equal(t, "spark", snap.Settlements[0].RaceID)
	require.Equal(t, "Искры", snap.Settlements[0].RaceName)
	require.Equal(t, 876000000, snap.Settlements[0].Population, "население — целое-результат")
	require.Equal(t, 69, snap.Settlements[0].Stability)
	require.Equal(t, "human", snap.Settlements[1].RaceID)
	require.Equal(t, "Люди", snap.Settlements[1].RaceName)

	require.Len(t, snap.Settlements[0].Branches, 1)
	br := snap.Settlements[0].Branches[0]
	require.Equal(t, int64(73), br.RecipeID)
	require.Equal(t, "Вода", br.RecipeName)
	require.Equal(t, 3, br.Complexity)
	require.Equal(t, 12.5, br.Produced)
	require.Equal(t, 4.0, br.Eaten)
	require.Equal(t, 0.02, br.EatenRate)

	require.Len(t, snap.Factions, 1)
	require.Len(t, snap.Buildings, 1)

	raw, err := json.Marshal(snap)
	require.NoError(t, err)
	s := string(raw)
	for _, forbidden := range []string{
		`population_exact`, `computed_at`, `r_per_sec`, `lambda_per_hour`, `n_dead`,
		`"input"`, `"output"`, `"effects"`, `"log"`,
	} {
		assert.NotContains(t, s, forbidden, "запрещённое поле в снимке: %s", forbidden)
	}
}

// T1/T6: FixatePresence пишет data.snapshot (at/source/поверхность) и счётчик
// поселений через merge-UPSERT; население — из читающего пути.
func TestFixatePresenceWritesSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// GetPlanetByID: планета + пустые поселения/фракции/строения.
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
			AddRow("p1", "w1", "Планета", 0,
				`{"surface_dominant":"горы","surface_composition":{"горы":62.0,"пыль":38.0}}`, now(), now()))
	mock.ExpectQuery(`FROM settlements s`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat", "effects"}))
	mock.ExpectQuery(`FROM factions WHERE homeworld_id = ANY\(\$1\)`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "color", "description", "homeworld_id"}))
	mock.ExpectQuery(`FROM buildings b[\s\S]*WHERE b\.planet_id = ANY\(\$1\)`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "building_type", "owner_type", "owner_id", "producer_type_id", "name"}))

	captured := &captureArg{}
	mock.ExpectExec(`INSERT INTO player_planet_knowledge.*DO UPDATE SET data = player_planet_knowledge\.data \|\| EXCLUDED\.data`).
		WithArgs("u1", "p1", captured, "presence").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewKnowledgeRepository(db).FixatePresence("u1", "p1", "presence"))
	require.NoError(t, mock.ExpectationsWereMet())

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(captured.raw, &data))
	require.Equal(t, "горы", data["surface_dominant"])
	require.Equal(t, float64(0), data["settlements_count"])
	snap, ok := data["snapshot"].(map[string]interface{})
	require.True(t, ok, "data.snapshot записан")
	require.Equal(t, "presence", snap["source"])
	require.NotEmpty(t, snap["at"])
	require.Equal(t, "горы", snap["surface_dominant"])
	rawSnap, err := json.Marshal(snap)
	require.NoError(t, err)
	for _, forbidden := range []string{`population_exact`, `computed_at`, `r_per_sec`} {
		assert.NotContains(t, string(rawSnap), forbidden)
	}
}
