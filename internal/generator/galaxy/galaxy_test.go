package galaxy

import (
	"math"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

var validSpectralClasses = []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}

func TestTotalSpectralWeightIs100(t *testing.T) {
	assert.Equal(t, 100.0, totalSpectralWeight)
}

func TestSpectralWeightsConsistent(t *testing.T) {
	for i, sw := range spectralWeights {
		assert.Less(t, sw.TempMin, sw.TempMax, "%s: диапазон температур пуст", sw.Class)
		if i > 0 {
			assert.NotEqual(t, spectralWeights[i-1].Class, sw.Class, "классы должны быть уникальны")
		}
	}
}

func TestRandomSpectralClassValidAndDistributed(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	counts := map[string]int{}
	const n = 100_000
	for i := 0; i < n; i++ {
		cls := randomSpectralClass(rng)
		assert.Contains(t, validSpectralClasses, cls)
		counts[cls]++
	}

	for _, sw := range spectralWeights {
		fraction := float64(counts[sw.Class]) / n
		expected := sw.Weight / totalSpectralWeight
		// Отклонение не больше 25% от ожидаемой доли (случайная выборка).
		assert.InDelta(t, expected, fraction, expected*0.25, "доля класса %s", sw.Class)
	}
}

func TestRandomTemperatureConsistentWithClass(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for _, cls := range validSpectralClasses {
		lo, hi := temperatureRange(cls)
		for i := 0; i < 500; i++ {
			tmp := randomTemperature(cls, rng)
			assert.GreaterOrEqual(t, tmp, lo)
			assert.Less(t, tmp, hi)
		}
	}
}

func TestRandomTemperatureUnknownFallbackIsG(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 1000; i++ {
		tmp := randomTemperature("ZZZ", rng)
		assert.GreaterOrEqual(t, tmp, 5200)
		assert.Less(t, tmp, 6000)
	}
}

func TestRandomPointInCircle(t *testing.T) {
	g := NewGenerator(&Config{Seed: 4})
	const radius = 500.0
	const n = 10_000

	quadrants := [4]int{}
	sumR := 0.0
	for i := 0; i < n; i++ {
		x, y := g.randomPointInCircle(radius)
		r := math.Hypot(x, y)
		require.LessOrEqual(t, r, radius, "точка за пределами круга")
		sumR += r
		switch {
		case x >= 0 && y >= 0:
			quadrants[0]++
		case x < 0 && y >= 0:
			quadrants[1]++
		case x < 0 && y < 0:
			quadrants[2]++
		default:
			quadrants[3]++
		}
	}

	for i, q := range quadrants {
		assert.Greater(t, q, n/5, "квадрант %d недозаполнен", i)
	}
	// Ожидаемый средний радиус = 2R/3 ≈ 333.
	assert.InDelta(t, 2*radius/3, sumR/n, 40, "средний радиус не соответствует плотности")
}

func TestIsPointValid(t *testing.T) {
	g := NewGenerator(&Config{Seed: 4})
	points := []struct{ X, Y float64 }{{X: 0, Y: 0}}
	const minDist = 100.0

	assert.False(t, g.isPointValid(0, 50, minDist, points), "слишком близко")
	assert.True(t, g.isPointValid(0, 150, minDist, points), "дальше минимума")
	assert.True(t, g.isPointValid(0, 100, minDist, points), "ровно на границе — допустимо (строго меньше)")
}

func TestGenerateWorldsRandomProperties(t *testing.T) {
	g := NewGenerator(&Config{
		Seed: 42, WorldCount: 50, MapSize: 2000, MinDist: 120, WorldSpread: 0,
	})
	worlds := g.generateWorldsRandom(120)

	require.Len(t, worlds, 50)
	assertWorldsValid(t, worlds, 2000, 120)
}

func TestGenerateWorldsRandomCountLimitedBySpace(t *testing.T) {
	// Маленький круг и огромный minDist — миры физически не влезут.
	g := NewGenerator(&Config{
		Seed: 5, WorldCount: 60, MapSize: 100, MinDist: 200, WorldSpread: 0,
	})
	worlds := g.generateWorldsRandom(200)
	assert.Less(t, len(worlds), 60, "пространство не позволяет разместить все миры")
	assert.NotEmpty(t, worlds)
	assertWorldsValid(t, worlds, 100, 180)
}

func TestGenerateWorldsPoissonProperties(t *testing.T) {
	g := NewGenerator(&Config{
		Seed: 7, WorldCount: 40, MapSize: 2000, MinDist: 150,
		ClusterCount: 3, ClusterSpacing: 900, ClusterRadius: 150,
		OutlierPercent: 0.08, WorldSpread: 0,
	})
	result := g.generateWorldsPoisson()
	worlds := result.Worlds

	// Вместимости достаточно — добивка должна довести почти до цели.
	require.GreaterOrEqual(t, len(worlds), 30, "недостаточно миров")
	assert.LessOrEqual(t, len(worlds), 40)
	assertWorldsValid(t, worlds, 2000, 150)
}

func TestGenerateWorldsPoissonRegions(t *testing.T) {
	g := NewGenerator(&Config{
		Seed: 7, WorldCount: 40, MapSize: 2000, MinDist: 150,
		ClusterCount: 3, ClusterSpacing: 900, ClusterRadius: 150,
		OutlierPercent: 0.08, WorldSpread: 0,
	})
	result := g.generateWorldsPoisson()

	// Регион на каждый кластерный центр.
	require.Len(t, result.Regions, 3)

	names := map[string]bool{}
	for _, r := range result.Regions {
		assert.NotEmpty(t, r.ID)
		assert.NotEmpty(t, r.Name)
		assert.False(t, names[r.Name], "дубль имени региона: %s", r.Name)
		names[r.Name] = true
		assert.Greater(t, r.Radius, 0.0)
		assert.NotEmpty(t, r.Color)
	}

	// Каждый мир приписан к региону.
	total := 0
	for _, r := range result.Regions {
		total += r.WorldCount
	}
	assert.Equal(t, len(result.Worlds), total, "миры должны быть распределены по регионам")
}

func TestGenerateGalaxyWithRegionsRandomHasNone(t *testing.T) {
	// Без кластеров регионов нет.
	g := NewGenerator(&Config{Seed: 5, WorldCount: 20, MapSize: 2000, MinDist: 200, WorldSpread: 0})
	res := g.GenerateGalaxyWithRegions()
	require.NotEmpty(t, res.Worlds)
	assert.Empty(t, res.Regions)
}

func TestClusterSpacingEnforcedNonOverlap(t *testing.T) {
	// ClusterSpacing (100) меньше 2×ClusterRadius (1000) — генератор должен
	// форсировать расстояние между центрами кластеров, чтобы их территории
	// не наслаивались друг на друга.
	g := NewGenerator(&Config{
		Seed: 42, WorldCount: 200, MapSize: 5000, MinDist: 150,
		ClusterCount: 8, ClusterSpacing: 100, ClusterRadius: 500,
		OutlierPercent: 0.05, WorldSpread: 0,
	})
	result := g.generateWorldsPoisson()
	require.NotEmpty(t, result.Regions)

	for i := 0; i < len(result.Regions); i++ {
		for j := i + 1; j < len(result.Regions); j++ {
			d := math.Hypot(
				result.Regions[i].CenterX-result.Regions[j].CenterX,
				result.Regions[i].CenterY-result.Regions[j].CenterY,
			)
			assert.GreaterOrEqual(t, d, 2*500-0.01, "центры кластеров слишком близко — территории пересекаются")
		}
	}
}

func TestGenerateOutlierPositionNearCluster(t *testing.T) {
	// Выбросы должны тяготеть к кластерным центрам (гауссово смещение),
	// а не разбрасываться равномерно по галактике.
	g := NewGenerator(&Config{Seed: 3, MapSize: 100000, ClusterRadius: 500})
	centers := []struct{ X, Y float64 }{{X: 0, Y: 0}}
	regions := g.buildRegions(centers)

	const n = 10000
	var sum float64
	placed := 0
	for i := 0; i < n; i++ {
		x, y, ok := g.generateOutlierPosition(regions, 100000, 150, nil)
		if !ok {
			continue
		}
		sum += math.Hypot(x, y)
		placed++
	}
	require.Greater(t, placed, n/2, "слишком много неудачных попыток")

	// Гауссово распределение со std = ClusterRadius (500): среднее расстояние
	// (Рэлеевское) ≈ 500 × sqrt(π/2) ≈ 626. Равномерный разброс дал бы ~2/3 R.
	mean := sum / float64(placed)
	assert.Less(t, mean, 500*2.0, "выбросы должны быть ближе к кластерам, mean=%.0f", mean)
	assert.Greater(t, mean, 500*0.5, "выбросы не должны сидеть в самом центре, mean=%.0f", mean)
}

func TestGenerateGalaxyWithRegionsDeterminism(t *testing.T) {
	mk := func() *GalaxyResult {
		return NewGenerator(&Config{
			Seed: 99, WorldCount: 15, MapSize: 1500, MinDist: 200,
			ClusterCount: 2, ClusterSpacing: 800, ClusterRadius: 150, WorldSpread: 5,
		}).GenerateGalaxyWithRegions()
	}
	a, b := mk(), mk()
	assert.Equal(t, worldSignatures(a.Worlds), worldSignatures(b.Worlds), "миры должны совпадать")
	require.Len(t, a.Regions, len(b.Regions))
	for i := range a.Regions {
		ar, br := a.Regions[i], b.Regions[i]
		assert.Equal(t, ar.Name, br.Name)
		assert.Equal(t, ar.Color, br.Color)
		assert.Equal(t, ar.CenterX, br.CenterX)
		assert.Equal(t, ar.WorldCount, br.WorldCount)
	}
}

func TestGenerateClusterCentersSpacing(t *testing.T) {
	g := NewGenerator(&Config{Seed: 11, MapSize: 2000})
	centers := g.generateClusterCenters(6, 2000, 500)

	require.Len(t, centers, 6)
	for i := 0; i < len(centers); i++ {
		assert.LessOrEqual(t, math.Hypot(centers[i].X, centers[i].Y), 2000.0)
		for j := i + 1; j < len(centers); j++ {
			d := math.Hypot(centers[i].X-centers[j].X, centers[i].Y-centers[j].Y)
			assert.GreaterOrEqual(t, d, 500.0, "центры кластеров слишком близко")
		}
	}
}

func TestGenerateGalaxyDispatch(t *testing.T) {
	random := NewGenerator(&Config{Seed: 13, WorldCount: 20, MapSize: 2000, MinDist: 200, WorldSpread: 0})
	poisson := NewGenerator(&Config{
		Seed: 13, WorldCount: 20, MapSize: 2000, MinDist: 200,
		ClusterCount: 2, ClusterSpacing: 900, ClusterRadius: 150, WorldSpread: 0,
	})

	rWorlds := random.GenerateGalaxy()
	assert.False(t, random.cfg.ClusterCount > 0, "random path")
	assertWorldsValid(t, rWorlds, 2000, 200)

	pWorlds := poisson.GenerateGalaxy()
	assert.True(t, poisson.cfg.ClusterCount > 0, "poisson path")
	assertWorldsValid(t, pWorlds, 2000, 200)

	// Одна и та же seed с разными стратегиями расстановки даёт разные позиции.
	assert.NotEqual(t, worldSignatures(rWorlds), worldSignatures(pWorlds))
}

func TestGeneratorDeterminism(t *testing.T) {
	mk := func() *Generator {
		return NewGenerator(&Config{
			Seed: 99, WorldCount: 15, MapSize: 1500, MinDist: 200,
			ClusterCount: 2, ClusterSpacing: 800, ClusterRadius: 150, WorldSpread: 5,
		})
	}
	a := worldSignatures(mk().GenerateGalaxy())
	b := worldSignatures(mk().GenerateGalaxy())
	assert.Equal(t, a, b, "одинаковый seed должен давать идентичную генерацию")
}

func TestGeneratorDifferentSeeds(t *testing.T) {
	mk := func(seed int64) *Generator {
		return NewGenerator(&Config{Seed: seed, WorldCount: 15, MapSize: 1500, MinDist: 200})
	}
	a := worldSignatures(mk(1).GenerateGalaxy())
	b := worldSignatures(mk(2).GenerateGalaxy())
	require.Len(t, a, len(b))
	diff := 0
	for i := range a {
		if a[i] != b[i] {
			diff++
		}
	}
	assert.Greater(t, diff, 0, "разные seed должны давать разную генерацию")
}

func TestGenerateWorldNamesUnique(t *testing.T) {
	g := NewGenerator(&Config{Seed: 21, WorldCount: 40, MapSize: 2000, MinDist: 120, WorldSpread: 0})
	worlds := g.generateWorldsRandom(120)

	seen := map[string]bool{}
	for _, w := range worlds {
		assert.False(t, seen[w.Name], "дубль имени мира: %s", w.Name)
		seen[w.Name] = true
		assert.NotEmpty(t, w.ID)
	}
}

// ==================== ХЕЛПЕРЫ ====================

func temperatureRange(cls string) (min, max int) {
	for _, sw := range spectralWeights {
		if sw.Class == cls {
			return sw.TempMin, sw.TempMax
		}
	}
	return 0, 0
}

func assertWorldsValid(t *testing.T, worlds []*models.World, circleRadius, minDist float64) {
	t.Helper()
	require.NotEmpty(t, worlds)

	names := map[string]bool{}
	ids := map[string]bool{}
	points := make([]struct{ X, Y float64 }, 0, len(worlds))
	for _, w := range worlds {
		assert.LessOrEqual(t, math.Hypot(w.CoordX, w.CoordY), circleRadius, "мир за пределами галактики")
		assert.Contains(t, validSpectralClasses, w.SpectralClass)

		minT, maxT := temperatureRange(w.SpectralClass)
		assert.GreaterOrEqual(t, w.Temperature, minT, "температура ниже диапазона класса %s", w.SpectralClass)
		assert.Less(t, w.Temperature, maxT, "температура выше диапазона класса %s", w.SpectralClass)

		assert.False(t, names[w.Name], "дубль имени: %s", w.Name)
		names[w.Name] = true
		assert.False(t, ids[w.ID], "дубль ID: %s", w.ID)
		ids[w.ID] = true

		points = append(points, struct{ X, Y float64 }{X: w.CoordX, Y: w.CoordY})
	}

	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			d := math.Hypot(points[i].X-points[j].X, points[i].Y-points[j].Y)
			assert.GreaterOrEqual(t, d, minDist-0.01, "миры слишком близко друг к другу")
		}
	}
}

func worldSignatures(worlds []*models.World) []string {
	sig := make([]string, 0, len(worlds))
	for _, w := range worlds {
		sig = append(sig, w.Name+"|"+w.SpectralClass+"|"+strconv.Itoa(w.Temperature)+
			"|"+strconv.FormatFloat(w.CoordX, 'f', 4, 64)+"|"+strconv.FormatFloat(w.CoordY, 'f', 4, 64))
	}
	return sig
}