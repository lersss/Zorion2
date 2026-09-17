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
	got := decideRaceSettlements(sulfurPlanet(), 50, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.Equal(t, []string{"sulfur_nests"}, got, "планета кластера — только доминанта")
}

// Выброс у середины между кластерами (d1 = d2) — доминанта + сосед.
func TestDecideRaceSettlementsOutlierDominantPlusNeighbor(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (500, 0): d1 = d2 = 500 — середина; шанс = крутилка × 1.
	got := decideRaceSettlements(sulfurPlanet(), 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.ElementsMatch(t, []string{"sulfur_nests", "salt_bridge"}, got, "выброс: доминанта + сосед")
}

// Крутилка 0 — на выбросе только доминанта.
func TestDecideRaceSettlementsNeighborChanceZero(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	got := decideRaceSettlements(sulfurPlanet(), 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 0, Chance: 1.0}, rng)
	assert.Equal(t, []string{"sulfur_nests"}, got, "крутилка 0 — сосед не подселяется")
}

// Chance 0 (65a) — доминанта не селится даже на пригодной планете кластера.
func TestDecideRaceSettlementsChanceZero(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	got := decideRaceSettlements(sulfurPlanet(), 50, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 0}, rng)
	assert.Empty(t, got, "chance 0 — доминанта не селится")
}

// Chance 1 (65a) — доминанта селится (дефолт конфига).
func TestDecideRaceSettlementsChanceOne(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	got := decideRaceSettlements(sulfurPlanet(), 50, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.Equal(t, []string{"sulfur_nests"}, got, "chance 1 — доминанта селится")
}

// buildRaceSettlement (65a) — население по стратегии: fixed → Fixed,
// random → в [Min, Max].
func TestBuildRaceSettlementPopulation(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	fixed := buildRaceSettlement("p1", "humans", rng, Population{Kind: "fixed", Fixed: 12345})
	assert.Equal(t, 12345, fixed[2], "fixed: население = Fixed")

	random := buildRaceSettlement("p2", "humans", rng, Population{Kind: "random", Min: 100_000, Max: 200_000})
	pop := random[2].(int)
	assert.GreaterOrEqual(t, pop, 100_000, "random: население ≥ Min")
	assert.LessOrEqual(t, pop, 200_000, "random: население ≤ Max")
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
	got := decideRaceSettlements(data, 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
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
	got := decideRaceSettlements(data, 500, 0, raceRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.Equal(t, []string{"sulfur_nests"}, got, "сосед непригоден — только доминанта")
}

// bioRobotRegions — био (sulfur_nests) в (0,0) и робот (archivists) в (1000,0).
func bioRobotRegions() []*models.Region {
	return []*models.Region{
		{ID: "rA", CenterX: 0, CenterY: 0, Radius: 100, RaceID: "sulfur_nests"},
		{ID: "rB", CenterX: 1000, CenterY: 0, Radius: 100, RaceID: "archivists"},
	}
}

// robotRegions — два робота: archivists в (0,0), spark в (1000,0).
func robotRegions() []*models.Region {
	return []*models.Region{
		{ID: "rA", CenterX: 0, CenterY: 0, Radius: 100, RaceID: "archivists"},
		{ID: "rB", CenterX: 1000, CenterY: 0, Radius: 100, RaceID: "spark"},
	}
}

// archivistsPlanet — данные, пригодные архивариусам (робот, T 220–400),
// но не серным гнёздам (T 300 < 350).
func archivistsPlanet() map[string]interface{} {
	return map[string]interface{}{
		"temperature": 300.0,
		"gravity":     1.0,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 1.0,
			"composition":  map[string]interface{}{"N2": 78.0, "O2": 21.0, "CO2": 1.0},
		},
		"core": map[string]interface{}{
			"radioactivity": 10.0,
		},
	}
}

// robotSuitablePlanet — данные, пригодные и архивариусам, и искре
// (без O2/H2O — искра не переносит их).
func robotSuitablePlanet() map[string]interface{} {
	return map[string]interface{}{
		"temperature": 300.0,
		"gravity":     1.0,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 1.0,
			"composition":  map[string]interface{}{"N2": 90.0, "CO2": 10.0},
		},
		"core": map[string]interface{}{
			"radioactivity": 10.0,
		},
	}
}

// coexistPlanet — данные, пригодные и серным гнёздам (био), и архивариусам
// (робот): T 380, H2S есть.
func coexistPlanet() map[string]interface{} {
	return map[string]interface{}{
		"temperature": 380.0,
		"gravity":     1.5,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 10.0,
			"composition":  map[string]interface{}{"H2S": 2.0, "SO2": 5.0, "CO2": 30.0, "N2": 60.0},
		},
		"core": map[string]interface{}{
			"radioactivity":  20.0,
			"heat_flux_w_m2": 2.0,
		},
	}
}

// Робот селится вне кластера-дома по условиям среды (Race.Suitable, 99.2.24 §5.3).
func TestDecideRaceSettlementsRobotOutsideHomeCluster(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (500,0) — середина между био (sulfur_nests) и роботом
	// (archivists). Пригодна архивариусам (T 300), но не серным гнёздам
	// (T 300 < 350). Робот селится вне кластера-дома по Suitable.
	got := decideRaceSettlements(archivistsPlanet(), 500, 0, bioRobotRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.Equal(t, []string{"archivists"}, got, "робот селится вне кластера-дома по Suitable")
}

// На одной планете селится один робот (99.2.24 §5.4): при совпадении окон
// двух роботорас — один, с приоритетом ближайшего кластера-дома.
func TestDecideRaceSettlementsRobotOnePerPlanet(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (500,0) — середина между архивариусами и искрой. Обе
	// пригодны; селится одна — архивариусы (первый в каталоге, tiebreak).
	got := decideRaceSettlements(robotSuitablePlanet(), 500, 0, robotRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.Equal(t, []string{"archivists"}, got, "на планете селится один робот")
}

// Приоритет расе-дому кластера (99.2.24 §5.4): робот, чей кластер-дом
// ближайший к планете.
func TestDecideRaceSettlementsRobotHomePriority(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (50,0) — внутри кластера-дома архивариусов (50 ≤ 125).
	// Обе пригодны; архивариусы ближе (50 < 950) — приоритет расе-дому.
	got := decideRaceSettlements(robotSuitablePlanet(), 50, 0, robotRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.Equal(t, []string{"archivists"}, got, "приоритет расе-дому кластера")
}

// Ни один робот не пригоден — поселения роботов на планете нет (99.2.24 §5.4).
func TestDecideRaceSettlementsRobotNoneSuitable(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (50,0), T=1000 — вне окна архивариусов (surv [220,400])
	// и искры (surv [120,650]).
	data := map[string]interface{}{
		"temperature": 1000.0,
		"gravity":     1.0,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 1.0,
			"composition":  map[string]interface{}{"N2": 90.0, "CO2": 10.0},
		},
		"core": map[string]interface{}{
			"radioactivity": 10.0,
		},
	}
	got := decideRaceSettlements(data, 50, 0, robotRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.Empty(t, got, "ни один робот не пригоден — поселения нет")
}

// В кластере-доме робота применяется шанс Chance (как у доминанты био-расы).
func TestDecideRaceSettlementsRobotChanceInHome(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (50,0) — внутри кластера-дома архивариусов, пригодна.
	// Chance=0 → робот не селится (шанс в кластере-доме не прошёл).
	got := decideRaceSettlements(robotSuitablePlanet(), 50, 0, robotRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 0}, rng)
	assert.Empty(t, got, "Chance=0 в кластере-доме — робот не селится")
}

// Робот и био-раса сосуществуют на одной планете (99.2.24 §5.5): окна
// пересекаются, взаимных ядов нет.
func TestDecideRaceSettlementsRobotBioCoexist(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	rng := rand.New(rand.NewSource(1))

	// Планета в (500,0) — середина между био (sulfur_nests) и роботом
	// (archivists). Пригодна обоим (T 380, H2S есть). Селится био-доминанта
	// + робот.
	got := decideRaceSettlements(coexistPlanet(), 500, 0, bioRobotRegions(), RaceGenConfig{NeighborChance: 1.0, Chance: 1.0}, rng)
	assert.ElementsMatch(t, []string{"sulfur_nests", "archivists"}, got, "робот и био сосуществуют")
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
	count, unsettled, err := g.GenerateRaceSettlements(context.Background(), RaceGenConfig{NeighborChance: 0.3, Chance: 1.0}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, unsettled, 60, "все расы без поселений")
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
	count, unsettled, err := g.GenerateRaceSettlements(context.Background(), RaceGenConfig{NeighborChance: 0.3, Chance: 1.0}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, unsettled, 60, "все расы без поселений")
	require.NoError(t, mock.ExpectationsWereMet())
}