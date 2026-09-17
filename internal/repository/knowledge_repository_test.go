// internal/repository/knowledge_repository_test.go
// Личный каталог знания о планетах (спека 77a §8): чтение по PK, UPSERT,
// «зажжённые» системы (KnownWorldIDs), ленивый прогон сканера (ScanSystem).
package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
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