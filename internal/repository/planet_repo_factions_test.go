// Тесты фракций и строений в карточке планеты (спека
// 2026-09-21-фабрики-релиз-2-столицы-фракций §6): attachFactionsAndBuildings —
// два запроса на систему (`= ANY($1)`), фракции/строения привязываются к
// своим планетам, у планет без них — пусто.
package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// expectEmptyFactionsBuildings — ожидания attachFactionsAndBuildings для тестов
// без фракций/строений (два запроса на систему, пустые выборки). SQL указан
// дословно — тесты planet_repo_* работают с QueryMatcherEqual.
func expectEmptyFactionsBuildings(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`
		SELECT id, name, type, color, description, homeworld_id
		FROM factions WHERE homeworld_id = ANY($1) ORDER BY name ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "name", "type", "color", "description", "homeworld_id"}))
	mock.ExpectQuery(`
		SELECT id, planet_id, building_type, owner_type, owner_id
		FROM buildings WHERE planet_id = ANY($1) ORDER BY building_type ASC, id ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "planet_id", "building_type", "owner_type", "owner_id"}))
}

// expectEmptyDeposits — ожидание attachDeposits (пустая выборка залежей).
// SQL берётся из самой константы repository (QueryMatcherEqual — дословно).
func expectEmptyDeposits(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(depositSelectByPlanetsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "planet_id", "good_id", "name", "stratum", "wealth", "amount"}))
}

func TestAttachFactionsAndBuildings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	factionRows := sqlmock.NewRows([]string{"id", "name", "type", "color", "description", "homeworld_id"}).
		AddRow("f1", "Аквилонский Синдикат", "Торговый синдикат", "#ff6b6b", "Описание", "p1").
		AddRow("f2", "Дом Кхари", "Клан", nil, nil, "p2")
	mock.ExpectQuery(`FROM factions WHERE homeworld_id = ANY\(\$1\) ORDER BY name ASC`).
		WithArgs(sqlmock.AnyArg()).WillReturnRows(factionRows)

	buildingRows := sqlmock.NewRows([]string{"id", "planet_id", "building_type", "owner_type", "owner_id"}).
		AddRow("b1", "p1", "capital", "faction", "f1")
	mock.ExpectQuery(`FROM buildings WHERE planet_id = ANY\(\$1\) ORDER BY building_type ASC, id ASC`).
		WithArgs(sqlmock.AnyArg()).WillReturnRows(buildingRows)

	// Три планеты, выборка — одна на систему (= ANY($1)), не по одной планете.
	planets := []models.Planet{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}
	require.NoError(t, NewPlanetRepository(db).attachFactionsAndBuildings(planets))

	require.Len(t, planets[0].Factions, 1)
	assert.Equal(t, "Аквилонский Синдикат", planets[0].Factions[0].Name)
	assert.Equal(t, "Торговый синдикат", planets[0].Factions[0].Type)
	require.Len(t, planets[0].Buildings, 1)
	assert.Equal(t, "capital", planets[0].Buildings[0].BuildingType, "столица фракции")
	assert.Equal(t, "faction", planets[0].Buildings[0].OwnerType)
	assert.Equal(t, "f1", planets[0].Buildings[0].OwnerID)

	require.Len(t, planets[1].Factions, 1)
	assert.Equal(t, "Дом Кхари", planets[1].Factions[0].Name)
	assert.Empty(t, planets[1].Factions[0].Color, "NULL color → пусто")
	assert.Empty(t, planets[1].Factions[0].Description, "NULL description → пусто")
	assert.Empty(t, planets[1].Buildings, "у фракции без столицы строений нет")

	assert.Empty(t, planets[2].Factions, "планета без фракций — пусто")
	assert.Empty(t, planets[2].Buildings, "планета без строений — пусто")

	require.NoError(t, mock.ExpectationsWereMet())
}
