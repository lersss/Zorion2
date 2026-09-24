// Тесты владельца структур в чтении карточки (спека
// 2026-09-24-постройка-структур-на-планете §10.3): settlements.owner_type/
// owner_id читаются nullable (Г1), имя владельца резолвится пакетно;
// buildings несут producer_type_id/type_name/owner_name (у столиц
// producer_type_id = null).
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ownerNamesPattern — регулярка пакетного резолва имён владельцев.
const ownerNamesPattern = `SELECT 'player', id::text, username FROM users WHERE id::text = ANY\(\$1\)[\s\S]*SELECT 'faction'[\s\S]*SELECT 'agent'`

// settlementsSelectPattern — регулярка выборки поселений с владельцем.
const settlementsSelectPattern = `SELECT s\.id, s\.planet_id.*s\.owner_type, s\.owner_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`

func ownerSettlementCols() []string {
	return []string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat", "effects", "owner_type", "owner_id"}
}

// TestLoadSettlementsResolvesOwnerName — поселение с владельцем-фракцией:
// owner_type/owner_id читаются, owner_name резолвится пакетно.
func TestLoadSettlementsResolvesOwnerName(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(settlementsSelectPattern).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(ownerSettlementCols()).
			AddRow("s1", "p1", 1000, float64(1000), 100, now, now, now, nil, int64(148), "Аутпост", nil, nil, "faction", "f1"))
	mock.ExpectQuery(ownerNamesPattern).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"type", "id", "name"}).AddRow("faction", "f1", "Люди"))

	planets := []models.Planet{{ID: "p1"}}
	require.NoError(t, NewPlanetRepository(db).loadSettlements(planets))
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets[0].Settlements, 1)
	s := planets[0].Settlements[0]
	assert.Equal(t, "faction", s.OwnerType)
	assert.Equal(t, "f1", s.OwnerID)
	assert.Equal(t, "Люди", s.OwnerName, "имя владельца резолвится пакетно")
}

// TestLoadSettlementsNullOwnerNoQuery — поселение без владельца (Г1): чтение
// не падает, owner_name пуст, лишнего запроса нет.
func TestLoadSettlementsNullOwnerNoQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(settlementsSelectPattern).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(ownerSettlementCols()).
			AddRow("s1", "p1", 1000, float64(1000), 100, now, now, now, nil, int64(148), "Аутпост", nil, nil, nil, nil))

	planets := []models.Planet{{ID: "p1"}}
	require.NoError(t, NewPlanetRepository(db).loadSettlements(planets))
	require.NoError(t, mock.ExpectationsWereMet(), "owner NULL → запроса имён нет")

	require.Len(t, planets[0].Settlements, 1)
	assert.Empty(t, planets[0].Settlements[0].OwnerID)
	assert.Empty(t, planets[0].Settlements[0].OwnerName)
}

// TestAttachBuildingsProducerType — созданное строение несёт producer_type_id
// и имя типа, владелец резолвится; столица — producer_type_id = null.
func TestAttachBuildingsProducerType(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM factions WHERE homeworld_id = ANY\(\$1\) ORDER BY name ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "color", "description", "homeworld_id"}))
	mock.ExpectQuery(`FROM buildings b[\s\S]*WHERE b\.planet_id = ANY\(\$1\) ORDER BY b\.building_type ASC, b\.id ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "building_type", "owner_type", "owner_id", "producer_type_id", "name"}).
			AddRow("b1", "p1", "producer", "player", "u1", int64(152), "Фабрика").
			AddRow("b2", "p1", "capital", "faction", "f1", nil, nil))
	mock.ExpectQuery(ownerNamesPattern).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"type", "id", "name"}).
			AddRow("player", "u1", "Игрок").
			AddRow("faction", "f1", "Люди"))

	planets := []models.Planet{{ID: "p1"}}
	require.NoError(t, NewPlanetRepository(db).attachFactionsAndBuildings(planets))
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, planets[0].Buildings, 2)
	b := planets[0].Buildings[0]
	require.NotNil(t, b.ProducerTypeID, "созданное строение несёт producer_type_id")
	assert.Equal(t, int64(152), *b.ProducerTypeID)
	assert.Equal(t, "Фабрика", b.TypeName)
	assert.Equal(t, "Игрок", b.OwnerName)

	capital := planets[0].Buildings[1]
	assert.Nil(t, capital.ProducerTypeID, "столица: producer_type_id = null (не производит)")
	assert.Empty(t, capital.TypeName)
	assert.Equal(t, "Люди", capital.OwnerName)
}
