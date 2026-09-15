// Тесты поисковых запросов на sqlmock: параметры, порядок вызовов, парсинг.
// Все запросы читают spectral_class через COALESCE — экзотика пишет NULL
// (99.2.4 §3), Scan NULL в string падает (баг #1, прогон @tester).
package handlers

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSearchDB — sqlmock-подключение для тестов запросов search_*.
func newSearchDB(t *testing.T) (sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return mock, db
}

func TestSearchWorldsByNameParsesRows(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name", "spectral_class", "coord_x", "coord_y"}).
		AddRow("w1", "Alpha", "G", 1.5, 2.5)

	mock.ExpectQuery(`(?i)SELECT id, name, COALESCE\(spectral_class,''\), coord_x, coord_y FROM worlds WHERE LOWER\(name\) = \$1 ORDER BY name LIMIT \$2`).
		WithArgs("alpha", 10).
		WillReturnRows(rows)

	results, err := searchWorldsByName(context.Background(), db, "alpha", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	r := results[0]
	assert.Equal(t, searchKindWorld, r.Kind)
	assert.Equal(t, "w1", r.ID)
	assert.Equal(t, "Alpha", r.Name)
	assert.Equal(t, "w1", r.WorldID)
	assert.Equal(t, "Alpha", r.WorldName)
	assert.Equal(t, "G", r.Spectral)
	assert.Equal(t, 1.5, r.CoordX)
	assert.Equal(t, 2.5, r.CoordY)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSearchPlanetsByNameParsesRows(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name", "world_id", "world_name", "spectral_class", "coord_x", "coord_y", "type"}).
		AddRow("p1", "Alpha Prime", "w1", "Alpha", "G", 1.5, 2.5, "землеподобная")

	mock.ExpectQuery(`(?i)SELECT p.id, p.name, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y, COALESCE\(p.data->>'type', ''\) FROM planets p JOIN worlds w ON w.id = p.world_id WHERE LOWER\(p.name\) = \$1 ORDER BY p.name LIMIT \$2`).
		WithArgs("alpha prime", 5).
		WillReturnRows(rows)

	results, err := searchPlanetsByName(context.Background(), db, "alpha prime", 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	r := results[0]
	assert.Equal(t, searchKindPlanet, r.Kind)
	assert.Equal(t, "p1", r.ID)
	assert.Equal(t, "Alpha Prime", r.Name)
	assert.Equal(t, "w1", r.WorldID)
	assert.Equal(t, "Alpha", r.WorldName)
	assert.Equal(t, "G", r.Spectral)
	assert.Equal(t, "землеподобная", r.Type)
	assert.Equal(t, 1.5, r.CoordX)
	assert.Equal(t, 2.5, r.CoordY)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSearchSatellitesByNameParsesRows(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name", "planet_id", "world_id", "world_name", "spectral_class", "coord_x", "coord_y"}).
		AddRow("s1", "Titan", "p1", "w1", "Alpha", "G", 1.5, 2.5)

	mock.ExpectQuery(`(?i)SELECT sat->>'id', sat->>'name', p.id, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y FROM planets p JOIN worlds w ON w.id = p.world_id CROSS JOIN LATERAL jsonb_array_elements\(p.data->'satellites'\) AS sat WHERE LOWER\(sat->>'name'\) = \$1 ORDER BY sat->>'name' LIMIT \$2`).
		WithArgs("titan", 3).
		WillReturnRows(rows)

	results, err := searchSatellitesByName(context.Background(), db, "titan", 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	r := results[0]
	assert.Equal(t, searchKindSatellite, r.Kind)
	assert.Equal(t, "s1", r.ID)
	assert.Equal(t, "Titan", r.Name)
	assert.Equal(t, "p1", r.PlanetID)
	assert.Equal(t, "w1", r.WorldID)
	assert.Equal(t, "Alpha", r.WorldName)
	assert.Equal(t, "G", r.Spectral)
	assert.Equal(t, 1.5, r.CoordX)
	assert.Equal(t, 2.5, r.CoordY)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSearchByNameOrchestratesAndMerges(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	// Миры: 2 результата → для планет остаётся лимит 5-2=3.
	mock.ExpectQuery(`(?i)SELECT id, name, COALESCE\(spectral_class,''\), coord_x, coord_y FROM worlds`).
		WithArgs("alpha", 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "spectral_class", "coord_x", "coord_y"}).
			AddRow("w1", "Alpha", "G", 1.5, 2.5).
			AddRow("w2", "Alpha Minor", "K", 3.0, 4.0))

	// Планеты: 1 результат → для спутников остаётся лимит 3-1=2.
	mock.ExpectQuery(`(?i)SELECT p.id, p.name, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y, COALESCE`).
		WithArgs("alpha", 3).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "world_id", "world_name", "spectral_class", "coord_x", "coord_y", "type"}).
			AddRow("p1", "Alpha Prime", "w1", "Alpha", "G", 1.5, 2.5, ""))

	mock.ExpectQuery(`(?i)SELECT sat->>'id', sat->>'name', p.id`).
		WithArgs("alpha", 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "planet_id", "world_id", "world_name", "spectral_class", "coord_x", "coord_y"}).
			AddRow("s1", "Alpha Moon", "p1", "w1", "Alpha", "G", 1.5, 2.5))

	results, err := searchByName(context.Background(), db, "alpha", 5)
	require.NoError(t, err)

	require.Len(t, results, 4)
	// Порядок после merge: звезды → планеты → спутники.
	assert.Equal(t, searchKindWorld, results[0].Kind)
	assert.Equal(t, "w1", results[0].ID)
	assert.Equal(t, searchKindWorld, results[1].Kind)
	assert.Equal(t, searchKindPlanet, results[2].Kind)
	assert.Equal(t, searchKindSatellite, results[3].Kind)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSearchByNamePropagatesQueryError(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	mock.ExpectQuery(`(?i)SELECT id, name, COALESCE\(spectral_class,''\), coord_x, coord_y FROM worlds`).
		WithArgs("alpha", 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "spectral_class", "coord_x", "coord_y"}))

	mock.ExpectQuery(`(?i)SELECT p.id, p.name, p.world_id, w.name`).
		WillReturnError(errors.New("boom"))

	_, err := searchByName(context.Background(), db, "alpha", 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSearchByNameStopsWhenLimitExhausted(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	// Миров вернулось столько же, сколько limit — планеты и спутники не запрашиваются.
	mock.ExpectQuery(`(?i)SELECT id, name, COALESCE\(spectral_class,''\), coord_x, coord_y FROM worlds`).
		WithArgs("alpha", 3).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "spectral_class", "coord_x", "coord_y"}).
			AddRow("w1", "Alpha", "G", 0, 0).
			AddRow("w2", "Beta", "G", 0, 0).
			AddRow("w3", "Gamma", "G", 0, 0))

	results, err := searchByName(context.Background(), db, "alpha", 3)
	require.NoError(t, err)
	require.Len(t, results, 3)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// ==================== NULL-СПЕКТР ЭКЗОТИКИ (99.2.4 §3, баг #1) ====================
//
// Экзотика пишет NULL в spectral_class; Scan NULL в string падает → поиск давал
// 500. Запросы обязаны читать COALESCE (паттерн planet_handler.go/world_repository.go).
// sqlmock не применяет SQL-функции, поэтому строки возвращают пост-COALESCE
// значение (''), а regex жёстко требует COALESCE в SQL — без него тест красный.

func TestSearchWorldsByNameNullSpectralCoalesced(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name", "spectral_class", "coord_x", "coord_y"}).
		AddRow("bh1", "ЧД-1", "", 1.5, 2.5) // пост-COALESCE: NULL → ''

	mock.ExpectQuery(`(?i)SELECT id, name, COALESCE\(spectral_class,''\), coord_x, coord_y FROM worlds WHERE LOWER\(name\) = \$1 ORDER BY name LIMIT \$2`).
		WithArgs("чд-1", 10).
		WillReturnRows(rows)

	results, err := searchWorldsByName(context.Background(), db, "чд-1", 10)
	require.NoError(t, err, "NULL-спектр не должен ронять Scan (500)")
	require.Len(t, results, 1)
	assert.Equal(t, "", results[0].Spectral, "экзотика: пустой спектральный класс")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSearchPlanetsByNameNullSpectralCoalesced(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name", "world_id", "world_name", "spectral_class", "coord_x", "coord_y", "type"}).
		AddRow("p1", "ЧД-планета", "bh1", "ЧД-1", "", 1.5, 2.5, "мёртвая")

	mock.ExpectQuery(`(?i)SELECT p.id, p.name, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y, COALESCE\(p.data->>'type', ''\) FROM planets p JOIN worlds w ON w.id = p.world_id WHERE LOWER\(p.name\) = \$1 ORDER BY p.name LIMIT \$2`).
		WithArgs("чд-планета", 10).
		WillReturnRows(rows)

	results, err := searchPlanetsByName(context.Background(), db, "чд-планета", 10)
	require.NoError(t, err, "NULL-спектр у планеты экзотики не должен давать 500")
	require.Len(t, results, 1)
	assert.Equal(t, "", results[0].Spectral)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSearchSatellitesByNameNullSpectralCoalesced(t *testing.T) {
	mock, db := newSearchDB(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name", "planet_id", "world_id", "world_name", "spectral_class", "coord_x", "coord_y"}).
		AddRow("s1", "ЧД-спутник", "p1", "bh1", "ЧД-1", "", 1.5, 2.5)

	mock.ExpectQuery(`(?i)SELECT sat->>'id', sat->>'name', p.id, p.world_id, w.name, COALESCE\(w.spectral_class,''\), w.coord_x, w.coord_y FROM planets p JOIN worlds w ON w.id = p.world_id CROSS JOIN LATERAL jsonb_array_elements\(p.data->'satellites'\) AS sat WHERE LOWER\(sat->>'name'\) = \$1 ORDER BY sat->>'name' LIMIT \$2`).
		WithArgs("чд-спутник", 10).
		WillReturnRows(rows)

	results, err := searchSatellitesByName(context.Background(), db, "чд-спутник", 10)
	require.NoError(t, err, "NULL-спектр у спутника экзотики не должен давать 500")
	require.Len(t, results, 1)
	assert.Equal(t, "", results[0].Spectral)

	assert.NoError(t, mock.ExpectationsWereMet())
}