package galaxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/races"
)

// mkRegions — N регионов с центрами по сетке (детерминированно).
func mkRegions(n int) []*models.Region {
	regions := make([]*models.Region, n)
	for i := range regions {
		regions[i] = &models.Region{
			ID:      string(rune('A'+i%26)) + string(rune('0'+i/26)),
			CenterX: float64(i % 20),
			CenterY: float64(i / 20),
			Radius:  1,
		}
	}
	return regions
}

// Раздача: все регионы получают расу, все 60 рас — по территории.
func TestAssignRacesToRegions(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	require.Len(t, races.Catalog(), 60)

	g := NewGenerator(&Config{Seed: 42})
	regions := mkRegions(200)
	g.assignRacesToRegions(regions)

	assigned := map[string]bool{}
	for _, r := range regions {
		assert.NotEmpty(t, r.RaceID, "регион %s должен получить расу", r.ID)
		assigned[r.RaceID] = true
	}
	assert.Len(t, assigned, 60, "все 60 рас получают территорию")
	for _, r := range races.Catalog() {
		assert.True(t, assigned[r.ID], "раса %s получает территорию", r.ID)
	}
}

// Детерминизм: одинаковый seed → одинаковое сопоставление рас.
func TestAssignRacesToRegionsDeterminism(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))

	mk := func() []string {
		g := NewGenerator(&Config{Seed: 7})
		regions := mkRegions(200)
		g.assignRacesToRegions(regions)
		out := make([]string, len(regions))
		for i, r := range regions {
			out[i] = r.RaceID
		}
		return out
	}
	a, b := mk(), mk()
	assert.Equal(t, a, b, "одинаковый seed → одинаковое сопоставление")
}

// Разные seed → разное сопоставление (с высокой вероятностью).
func TestAssignRacesToRegionsDifferentSeeds(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))

	mk := func(seed int64) []string {
		g := NewGenerator(&Config{Seed: seed})
		regions := mkRegions(200)
		g.assignRacesToRegions(regions)
		out := make([]string, len(regions))
		for i, r := range regions {
			out[i] = r.RaceID
		}
		return out
	}
	a, b := mk(1), mk(2)
	diff := 0
	for i := range a {
		if a[i] != b[i] {
			diff++
		}
	}
	assert.Greater(t, diff, 0, "разные seed дают разное сопоставление")
}

// Пустой список регионов — не падает (no-op).
func TestAssignRacesToRegionsNoRegions(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	g := NewGenerator(&Config{Seed: 42})
	g.assignRacesToRegions(nil) // не падает
}

// Группировка: 200 регионов → 50 непустых территорий, детерминированно.
func TestGroupRegionsIntoTerritories(t *testing.T) {
	regions := mkRegions(200)
	territories := groupRegionsIntoTerritories(regions, 50)
	require.Len(t, territories, 50)
	total := 0
	for _, g := range territories {
		assert.NotEmpty(t, g, "территория непустая")
		total += len(g)
	}
	assert.Equal(t, 200, total, "все регионы распределены")

	// Детерминизм.
	again := groupRegionsIntoTerritories(regions, 50)
	assert.Equal(t, territories, again)
}

// Группировка: регионов меньше цели — каждый регион своя территория.
func TestGroupRegionsIntoTerritoriesFewerThanTarget(t *testing.T) {
	regions := mkRegions(30)
	territories := groupRegionsIntoTerritories(regions, 50)
	require.Len(t, territories, 30)
	for _, g := range territories {
		assert.Len(t, g, 1)
	}
}

// robotTerritoriesOf — индексы территорий, занятых роботорасами
// (territory == "conditions", Robotic != nil), после раздачи.
func robotTerritoriesOf(regions []*models.Region) []int {
	territories := groupRegionsIntoTerritories(regions, len(races.Catalog()))
	var out []int
	for ti, group := range territories {
		if len(group) > 0 {
			r := races.ByID(regions[group[0]].RaceID)
			if r != nil && r.Robotic != nil {
				out = append(out, ti)
			}
		}
	}
	return out
}

// Роботорасы стремятся к рассеиванию (99.2.24 §5 п.2): на большой галактике
// большинство роботов разнесено (не соседствуют с другой роботорасой).
func TestAssignRacesToRegionsRobotsSpread(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	g := NewGenerator(&Config{Seed: 42})
	regions := mkRegions(200)
	g.assignRacesToRegions(regions)

	territories := groupRegionsIntoTerritories(regions, len(races.Catalog()))
	robotTerr := robotTerritoriesOf(regions)
	require.Len(t, robotTerr, 10, "10 роботорас получают территории")
	spread, total := robotSpreadingReport(territories, robotTerr, regions)
	assert.Equal(t, 10, total)
	assert.GreaterOrEqual(t, spread, 6, "большинство роботов разнесено")
}

// Best-effort (99.2.24 §5 п.2): на малой галактике (мало территорий) роботы
// всё равно получают территории — разносится сколько возможно.
func TestAssignRacesToRegionsRobotsBestEffort(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	g := NewGenerator(&Config{Seed: 42})
	regions := mkRegions(12) // 12 территорий < 60 рас
	g.assignRacesToRegions(regions)

	robotTerr := robotTerritoriesOf(regions)
	require.Len(t, robotTerr, 10, "все 10 роботов получают территории (best-effort)")
}

// Детерминизм рассеивания роботов: одинаковый seed → одинаковое
// сопоставление роботов территориям.
func TestAssignRacesToRegionsRobotsDeterminism(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))

	mk := func() []string {
		g := NewGenerator(&Config{Seed: 7})
		regions := mkRegions(200)
		g.assignRacesToRegions(regions)
		territories := groupRegionsIntoTerritories(regions, len(races.Catalog()))
		robotTerr := robotTerritoriesOf(regions)
		out := make([]string, len(robotTerr))
		for i, ti := range robotTerr {
			out[i] = regions[territories[ti][0]].RaceID
		}
		return out
	}
	a, b := mk(), mk()
	assert.Equal(t, a, b, "одинаковый seed → одинаковое рассеивание роботов")
}

// Интеграция: GenerateGalaxyWithRegions раздаёт расы (каталог загружен).
func TestGenerateGalaxyWithRegionsAssignsRaces(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../../config/races.json"))
	g := NewGenerator(&Config{
		Seed: 99, WorldCount: 300, MapSize: 3000, MinDist: 100,
		ClusterCount: 60, ClusterSpacing: 400, ClusterRadius: 150, WorldSpread: 5,
	})
	result := g.GenerateGalaxyWithRegions()
	require.Len(t, result.Regions, 60)
	assigned := map[string]bool{}
	for _, r := range result.Regions {
		assert.NotEmpty(t, r.RaceID, "регион %s должен получить расу", r.Name)
		assigned[r.RaceID] = true
	}
	assert.Len(t, assigned, 60, "все 60 рас получают территорию")
}