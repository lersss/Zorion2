package settlement

import (
	"context"
	"math/rand"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/races"
)

// raceRegions — два региона: A (sulfur_nests) в (0,0), B (salt_bridge) в
// (1000,0). Окна совместимы (сера + терморедокс, §7.4) — обе расы могут
// поселиться на одной планете.
func raceRegions() []*models.Region {
	return []*models.Region{
		{ID: "rA", CenterX: 0, CenterY: 0, Radius: 100, RaceID: "sulfur_nests"},
		{ID: "rB", CenterX: 1000, CenterY: 0, Radius: 100, RaceID: "salt_bridge"},
	}
}

// sulfurPlanet — данные, пригодные серным гнёздам (и соляным).
func sulfurPlanet() map[string]interface{} {
	return map[string]interface{}{
		"temperature": 450.0,
		"gravity":     1.5,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 50.0,
			"composition":  map[string]interface{}{"H2S": 2.0, "SO2": 5.0, "CO2": 30.0, "N2": 60.0},
		},
		"core": map[string]interface{}{
			"radioactivity":  30.0,
			"heat_flux_w_m2": 2.0,
		},
	}
}

// Планета внутри кластера (d1 ≤ 1.25×радиус) — только доминанта.
func TestDecideRaceSettlementsClusterPlanetDominantOnly(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (50, 0): d1 = 50 ≤ 125 — планета кластера A.
	got := decideRaceSettlements(sulfurPlanet(), 50, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0}, rng)
	assert.Equal(t, []string{"sulfur_nests"}, got, "планета кластера — только доминанта")
}

// Выброс у середины между кластерами (d1 = d2) — доминанта + сосед.
func TestDecideRaceSettlementsOutlierDominantPlusNeighbor(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (500, 0): d1 = d2 = 500 — середина; шанс = крутилка × 1.
	got := decideRaceSettlements(sulfurPlanet(), 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0}, rng)
	assert.ElementsMatch(t, []string{"sulfur_nests", "salt_bridge"}, got, "выброс: доминанта + сосед")
}

// Крутилка 0 — на выбросе только доминанта.
func TestDecideRaceSettlementsNeighborChanceZero(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	got := decideRaceSettlements(sulfurPlanet(), 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 0}, rng)
	assert.Equal(t, []string{"sulfur_nests"}, got, "крутилка 0 — сосед не подселяется")
}

// Доминанта непригодна — планета остаётся пустой для неё; сосед может
// поселиться на выбросе (независимо от доминанты).
func TestDecideRaceSettlementsDominantUnsuitableNeighborSettles(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// T = 600: соляные (surv [420, 680]) пригодны, серные гнёзда (surv
	// [350, 550]) — нет; H2S отсутствует (серным нужен ≥ 1%).
	data := map[string]interface{}{
		"temperature": 600.0,
		"gravity":     1.5,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 50.0,
			"composition":  map[string]interface{}{"SO2": 5.0, "CO2": 40.0, "N2": 55.0},
		},
		"core": map[string]interface{}{
			"radioactivity":  30.0,
			"heat_flux_w_m2": 2.0,
		},
	}
	got := decideRaceSettlements(data, 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0}, rng)
	assert.Equal(t, []string{"salt_bridge"}, got, "доминанта непригодна — сосед на выбросе")
}

// Сосед непригоден — только доминанта.
func TestDecideRaceSettlementsNeighborUnsuitable(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// T = 400: серные гнёзда (surv [350, 550]) пригодны, соляные (surv
	// [420, 680]) — нет.
	data := map[string]interface{}{
		"temperature": 400.0,
		"gravity":     1.5,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 50.0,
			"composition":  map[string]interface{}{"H2S": 2.0, "SO2": 5.0, "CO2": 30.0, "N2": 60.0},
		},
		"core": map[string]interface{}{
			"radioactivity":  30.0,
			"heat_flux_w_m2": 2.0,
		},
	}
	got := decideRaceSettlements(data, 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0}, rng)
	assert.Equal(t, []string{"sulfur_nests"}, got, "сосед непригоден — только доминанта")
}

// Регионов с расой нет — 0 поселений, все расы в отчёте.
func TestGenerateRaceSettlementsNoRegions(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, center_x, center_y, radius, race_id FROM regions WHERE race_id IS NOT NULL AND race_id != ''
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "center_x", "center_y", "radius", "race_id"}))

	g := NewGenerator(db, 1)
	count, unsettled, err := g.GenerateRaceSettlements(context.Background(), RaceGenConfig{NeighborChance: 0.3}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, unsettled, 50, "все расы без поселений")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Регионы есть, но ни одна планета не пригодна доминанте — 0 поселений.
func TestGenerateRaceSettlementsNoSuitable(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, center_x, center_y, radius, race_id FROM regions WHERE race_id IS NOT NULL AND race_id != ''
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "center_x", "center_y", "radius", "race_id"}).
		AddRow("rA", 0.0, 0.0, 100.0, "sulfur_nests"))

	// Планета в (50, 0) — кластер A, но температура 300 вне окна серных
	// гнёзд (surv [350, 550]).
	mock.ExpectQuery(`
		SELECT p.id, p.data, w.coord_x, w.coord_y FROM planets p JOIN worlds w ON w.id = p.world_id
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "data", "coord_x", "coord_y"}).
		AddRow("p1", `{"temperature":300,"gravity":1.5,"atmosphere_data":{"pressure_atm":50,"composition":{"H2S":2,"SO2":5,"CO2":30,"N2":60}},"core":{"radioactivity":30,"heat_flux_w_m2":2}}`, 50.0, 0.0))

	g := NewGenerator(db, 1)
	count, unsettled, err := g.GenerateRaceSettlements(context.Background(), RaceGenConfig{NeighborChance: 0.3}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, unsettled, 50, "все расы без поселений")
	require.NoError(t, mock.ExpectationsWereMet())
}