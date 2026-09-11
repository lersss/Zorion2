package galaxy

import (
	"log"
	"math"

	"zorion/internal/models"
)

// ---------- ВСПОМОГАТЕЛЬНАЯ ФУНКЦИЯ ДЛЯ КРУГА ----------
func (g *Generator) randomPointInCircle(radius float64) (x, y float64) {
	r := radius * math.Sqrt(g.rng.Float64())
	angle := 2 * math.Pi * g.rng.Float64()
	return r * math.Cos(angle), r * math.Sin(angle)
}

// generateOutlierPosition — позиция выброса: случайный кластерный центр +
// гауссово смещение (std = ClusterRadius). Так выбросы чаще оказываются
// ближе к кластерам и реже — у их границ / краёв галактики.
// Если кластеров нет — старый равномерный способ.
func (g *Generator) generateOutlierPosition(
	regions []*models.Region,
	halfSize, minDist float64,
	allPoints []struct{ X, Y float64 },
) (float64, float64, bool) {
	if len(regions) == 0 {
		x, y := g.randomPointInCircle(halfSize)
		return x, y, g.isPointValid(x, y, minDist, allPoints)
	}

	idx := g.rng.Intn(len(regions))
	cx, cy := regions[idx].CenterX, regions[idx].CenterY
	std := regions[idx].Radius
	if std <= 0 {
		std = 80.0
	}

	for attempt := 0; attempt < 30; attempt++ {
		x := cx + g.gaussian(std)
		y := cy + g.gaussian(std)
		if math.Hypot(x, y) > halfSize {
			continue
		}
		if g.isPointValid(x, y, minDist, allPoints) {
			return x, y, true
		}
	}
	return 0, 0, false
}

// ---------- ГЕНЕРАЦИЯ МИРОВ МЕТОДОМ ПУАССОНА ----------
func (g *Generator) generateWorldsPoisson() *GalaxyResult {
	targetCount := g.cfg.WorldCount
	halfSize := g.cfg.MapSize
	clusterCount := g.cfg.ClusterCount
	minDist := g.cfg.MinDist
	if minDist <= 0 {
		minDist = 150.0
	}

	clusterSpacing := g.cfg.ClusterSpacing
	if clusterSpacing < minDist {
		clusterSpacing = minDist
	}

	clusterRadius := g.cfg.ClusterRadius
	if clusterRadius <= 0 {
		clusterRadius = 80.0
	}
	// Кластеры не должны наслаиваться друг на друга: территории кластеров
	// (круги радиуса ClusterRadius) не пересекаются, если расстояние между
	// центрами >= 2×радиус. Иначе регионы выглядят «смазанными».
	if clusterSpacing < 2*clusterRadius {
		clusterSpacing = 2 * clusterRadius
	}

	if clusterCount <= 0 {
		return &GalaxyResult{Worlds: g.generateWorldsRandom(minDist)}
	}

	centers := g.generateClusterCenters(clusterCount, halfSize, clusterSpacing)
	regions := g.buildRegions(centers)

	outlierPercent := g.cfg.OutlierPercent
	if outlierPercent <= 0 {
		outlierPercent = 0.08
	}
	outlierCount := int(float64(targetCount) * outlierPercent)
	if outlierCount < 1 {
		outlierCount = 1
	}
	clusterPoints := targetCount - outlierCount

	allPoints := make([]struct{ X, Y float64 }, 0, targetCount)
	pointRegion := make([]int, 0, targetCount) // индекс региона (или -1)
	dropped := 0                               // счётчик отброшенных точек

	perCluster := clusterPoints / clusterCount
	if perCluster < 1 {
		perCluster = 1
	}
	remaining := clusterPoints
	for i := 0; i < clusterCount && remaining > 0; i++ {
		count := perCluster + g.rng.Intn(perCluster/2) - perCluster/4
		if count < 1 {
			count = 1
		}
		if count > remaining {
			count = remaining
		}
		remaining -= count

		cx, cy := centers[i].X, centers[i].Y
		clusterPointsList := g.clusterPointsGaussian(cx, cy, clusterRadius, minDist, count)
		for _, p := range clusterPointsList {
			// ГЛОБАЛЬНАЯ ПРОВЕРКА: точка должна быть внутри круга
			if math.Hypot(p.X, p.Y) > halfSize {
				dropped++
				continue
			}
			if g.isPointValid(p.X, p.Y, minDist, allPoints) {
				allPoints = append(allPoints, p)
				pointRegion = append(pointRegion, i)
			}
		}
	}

	// Добивка кластерных точек – только внутри круга
	attempts := clusterPoints * 200
	for len(allPoints) < clusterPoints && attempts > 0 {
		attempts--
		x, y := g.randomPointInCircle(halfSize)
		if g.isPointValid(x, y, minDist, allPoints) {
			allPoints = append(allPoints, struct{ X, Y float64 }{X: x, Y: y})
			pointRegion = append(pointRegion, g.nearestRegionIndex(x, y, regions))
		}
	}
	if len(allPoints) < clusterPoints {
		log.Printf("⚠️ Generated only %d cluster points out of %d (not enough space)", len(allPoints), clusterPoints)
	}

	// Выбросы – чаще ближе к кластерам, реже у их границ.
	outlierGenerated := 0
	maxAttempts := outlierCount * 200
	for outlierGenerated < outlierCount && maxAttempts > 0 {
		maxAttempts--
		x, y, ok := g.generateOutlierPosition(regions, halfSize, minDist, allPoints)
		if !ok {
			continue
		}
		allPoints = append(allPoints, struct{ X, Y float64 }{X: x, Y: y})
		pointRegion = append(pointRegion, g.nearestRegionIndex(x, y, regions))
		outlierGenerated++
	}
	if outlierGenerated < outlierCount {
		log.Printf("⚠️ Generated only %d outliers out of %d (not enough space)", outlierGenerated, outlierCount)
	}

	if dropped > 0 {
		log.Printf("⚠️ Dropped %d points because they were outside the galaxy circle", dropped)
	}
	if len(allPoints) < targetCount {
		log.Printf("⚠️ Total generated worlds: %d out of %d (minDist=%.1f)", len(allPoints), targetCount, minDist)
	}

	worlds := make([]*models.World, len(allPoints))
	for i, p := range allPoints {
		worlds[i] = g.generateWorld(struct{ X, Y float64 }{X: p.X, Y: p.Y})
		if idx := pointRegion[i]; idx >= 0 && idx < len(regions) {
			regions[idx].WorldCount++
		}
	}
	return &GalaxyResult{Worlds: worlds, Regions: regions}
}

// ---------- ГЕНЕРАЦИЯ ЦЕНТРОВ КЛАСТЕРОВ (БЕЗ ИЗМЕНЕНИЙ) ----------
func (g *Generator) generateClusterCenters(count int, halfSize float64, minSpacing float64) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	if minSpacing <= 0 {
		minSpacing = 150.0
	}

	centers := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 200

	for len(centers) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.randomPointInCircle(halfSize)

		valid := true
		for _, c := range centers {
			dx := c.X - x
			dy := c.Y - y
			if dx*dx+dy*dy < minSpacing*minSpacing {
				valid = false
				break
			}
		}
		if valid {
			centers = append(centers, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	if len(centers) < count {
		log.Printf("⚠️ Generated only %d cluster centers out of %d", len(centers), count)
	}
	return centers
}

// ---------- ТОЧКИ КЛАСТЕРА С ГАУССОВОЙ ПЛОТНОСТЬЮ ----------

// clusterPointsGaussian — точки кластера с гауссовой плотностью:
// плотнее к центру, реже к краям (std = радиус/2, усечение по кругу).
// Соблюдает minDist между точками (отбор отбрасыванием).
func (g *Generator) clusterPointsGaussian(cx, cy, radius, minDist float64, count int) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	std := radius / 2.0
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 50
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x := cx + g.gaussian(std)
		y := cy + g.gaussian(std)
		if math.Hypot(x-cx, y-cy) > radius {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// ---------- ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ----------
func (g *Generator) gaussian(std float64) float64 {
	u1 := g.rng.Float64()
	u2 := g.rng.Float64()
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return z * std
}

func (g *Generator) isPointValid(x, y, minDist float64, points []struct{ X, Y float64 }) bool {
	for _, p := range points {
		dx := p.X - x
		dy := p.Y - y
		if dx*dx+dy*dy < minDist*minDist {
			return false
		}
	}
	return true
}

func (g *Generator) generateWorldsRandom(minDist float64) []*models.World {
	targetCount := g.cfg.WorldCount
	halfSize := g.cfg.MapSize
	points := make([]struct{ X, Y float64 }, 0, targetCount)
	maxAttempts := targetCount * 200
	for len(points) < targetCount && maxAttempts > 0 {
		maxAttempts--
		x, y := g.randomPointInCircle(halfSize)
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	if len(points) < targetCount {
		log.Printf("⚠️ Generated only %d random worlds out of %d (not enough space with minDist=%.1f)", len(points), targetCount, minDist)
	}
	worlds := make([]*models.World, len(points))
	for i, p := range points {
		worlds[i] = g.generateWorld(struct{ X, Y float64 }{X: p.X, Y: p.Y})
	}
	return worlds
}