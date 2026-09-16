package galaxy

import (
	"log"
	"math"

	"zorion/internal/models"
	"zorion/internal/regionprofile"
)

// ---------- ВСПОМОГАТЕЛЬНАЯ ФУНКЦИЯ ДЛЯ КРУГА ----------
func (g *Generator) randomPointInCircle(radius float64) (x, y float64) {
	r := radius * math.Sqrt(g.rng.Float64())
	angle := 2 * math.Pi * g.rng.Float64()
	return r * math.Cos(angle), r * math.Sin(angle)
}

// galaxyEdgeThinning — модуль разрежения к краю галактики: вероятность
// принять точку линейно падает от 1 в центре до (1 − galaxyEdgeThinning)
// на границе MapSize. Меньше — слабее разрежение, 0 — выключено.
const galaxyEdgeThinning = 0.7

// galaxyEdgeAccept — приём точки по плотности к краям: чем ближе точка
// к внешней границе MapSize, тем ниже вероятность её принять. Центром
// галактики считается начало координат (как и в остальных проверках —
// hypot(x, y) > MapSize). При MapSize <= 0 (юнит-тесты форм кластеров)
// разрежение выключено.
func (g *Generator) galaxyEdgeAccept(x, y float64) bool {
	halfSize := g.cfg.MapSize
	if halfSize <= 0 {
		return true
	}
	u := math.Hypot(x, y) / halfSize
	return g.rng.Float64() >= galaxyEdgeThinning*u
}

// generateOutlierPosition — позиция выброса: гауссово смещение от случайного
// кластерного центра (std = 3×ClusterRadius), чтобы выбросы тяготели к кластерам,
// но заполняли пустоты между ними. Если кластеров нет — равномерный способ.
func (g *Generator) generateOutlierPosition(
	regions []*models.Region,
	halfSize, minDist float64,
	grid *spatialGrid,
) (float64, float64, bool) {
	if len(regions) == 0 {
		x, y := g.randomPointInCircle(halfSize)
		return x, y, !grid.HasNear(x, y, minDist)
	}

	idx := g.rng.Intn(len(regions))
	cx, cy := regions[idx].CenterX, regions[idx].CenterY
	std := regions[idx].Radius * 3.0
	if std <= 0 {
		std = 80.0
	}

	for attempt := 0; attempt < 30; attempt++ {
		x := cx + g.gaussian(std)
		y := cy + g.gaussian(std)
		if math.Hypot(x, y) > halfSize {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if !grid.HasNear(x, y, minDist) {
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

	// Раздача рас по территориям (спека 99.2.21 §7): после создания
	// регионов, до генерации миров. Каталог не загружен — no-op.
	g.assignRacesToRegions(regions)

	// Форма и параметры формы на каждый кластерный центр (индекс == индекс
	// центра/региона). Роллятся заранее, чтобы добивка генерировала точки
	// в той же геометрии, что и основные точки кластера.
	clusterShapes := make([]string, len(centers))
	clusterParams := make([]clusterShapeParams, len(centers))
	for i := range centers {
		shape := g.rollClusterShape()
		clusterShapes[i] = shape
		clusterParams[i] = g.rollShapeParams(shape, clusterRadius)
	}

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
	grid := newSpatialGrid(-halfSize, -halfSize, halfSize, halfSize, minDist)

	perCluster := clusterPoints / clusterCount
	if perCluster < 1 {
		perCluster = 1
	}
	// Если центров кластеров поставилось меньше, чем запрошено
	// (не хватило места при ClusterSpacing ≥ 2×ClusterRadius) — идём
	// по фактически созданным, а не по clusterCount (иначе index out of range).
	remaining := clusterPoints
	for i := 0; i < len(centers) && remaining > 0; i++ {
		count := perCluster
		// Защита от Intn(0) (регрессия 20b): при perCluster=1 (малый world_count /
		// большой cluster_count) разброс не применяем — Intn(perCluster/2)=Intn(0)
		// паникует. При perCluster ≥ 2 поведение прежнее (Intn(1)=0 детерминирован,
		// при perCluster ≥ 4 — настоящий разброс).
		if perCluster >= 2 {
			count = perCluster + g.rng.Intn(perCluster/2) - perCluster/4
		}
		if count < 1 {
			count = 1
		}
		if count > remaining {
			count = remaining
		}
		remaining -= count

		cx, cy := centers[i].X, centers[i].Y
		clusterPointsList := g.clusterPoints(clusterShapes[i], clusterParams[i], cx, cy, clusterRadius, minDist, count)
		for _, p := range clusterPointsList {
			// ГЛОБАЛЬНАЯ ПРОВЕРКА: точка должна быть внутри круга
if math.Hypot(p.X, p.Y) > halfSize {
			dropped++
			continue
		}
		if !grid.HasNear(p.X, p.Y, minDist) {
			allPoints = append(allPoints, p)
			grid.Add(p.X, p.Y)
			pointRegion = append(pointRegion, i)
		}
	}
}

	// Добивка кластерных точек – внутри территории своего региона: точка
	// генерируется в форме региона (та же геометрия, что у основных точек
	// кластера) и принимается в пределах 1.25×радиус от центра (как обычные
	// точки кластера). Раньше точки кидались гауссом вокруг центра региона —
	// для не-blob форм (spiral, ring, bar) остаток заливался гауссовым пятном
	// и размазывал форму.
	attempts := clusterPoints * 200
	for len(allPoints) < clusterPoints && attempts > 0 {
		attempts--
		if len(regions) == 0 {
			x, y := g.randomPointInCircle(halfSize)
			if !g.galaxyEdgeAccept(x, y) {
				continue
			}
			if !grid.HasNear(x, y, minDist) {
				allPoints = append(allPoints, struct{ X, Y float64 }{X: x, Y: y})
				grid.Add(x, y)
				pointRegion = append(pointRegion, -1)
			}
			continue
		}
		ri := g.rng.Intn(len(regions))
		r := regions[ri]
		x, y := g.shapeCandidate(clusterShapes[ri], r.CenterX, r.CenterY, r.Radius, clusterParams[ri])
		if math.Hypot(x, y) > halfSize {
			continue
		}
		if !g.clusterEdgeAccept(r.CenterX, r.CenterY, x, y, r.Radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if !grid.HasNear(x, y, minDist) {
			allPoints = append(allPoints, struct{ X, Y float64 }{X: x, Y: y})
			grid.Add(x, y)
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
		x, y, ok := g.generateOutlierPosition(regions, halfSize, minDist, grid)
		if !ok {
			continue
		}
		allPoints = append(allPoints, struct{ X, Y float64 }{X: x, Y: y})
		grid.Add(x, y)
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
		// Профиль региона точки (59a §10): из pointRegion[i]; фоновый регион
		// (Profile пуст) или вне региона — профиля нет.
		var profile *regionprofile.Profile
		var intensity regionprofile.Intensity
		if idx := pointRegion[i]; idx >= 0 && idx < len(regions) {
			if r := regions[idx]; r.Profile != "" {
				profile = regionprofile.ByID(r.Profile)
				intensity = regionprofile.Intensity(r.ProfileIntensity)
			}
		}
		worlds[i] = g.generateWorld(struct{ X, Y float64 }{X: p.X, Y: p.Y}, profile, intensity)
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

// ---------- ТОЧКИ КЛАСТЕРА ПО ФОРМЕ ----------

// clusterForms — формы для режима "random": на каждый кластер выбирается
// одна из них через g.rng (детерминизм по seed сохраняется).
var clusterForms = []string{"blob", "circle", "ring", "bar", "spiral", "dumbbell", "stream", "core_halo"}

// rollClusterShape — форма кластера: нормализованная Config.Shape; при
// "random" — случайная из clusterForms через g.rng.
func (g *Generator) rollClusterShape() string {
	shape := g.cfg.clusterShape()
	if shape == "random" {
		shape = clusterForms[g.rng.Intn(len(clusterForms))]
	}
	return shape
}

// clusterShapeParams — параметры формы кластера, роллятся один раз на кластер
// (rollShapeParams), чтобы основные точки и добивка ложились в одну геометрию:
// поворот оси (bar/dumbbell), поворот кластера (spiral), радиус кольца (ring),
// очаги (blob/stream). Для форм без параметров (circle/core_halo) — нулевой.
type clusterShapeParams struct {
	ringR float64
	cosA  float64
	sinA  float64
	cosR  float64
	sinR  float64
	lobes []struct{ x, y float64 }
	seeds []struct{ x, y, std float64 }
}

// rollShapeParams — роллит параметры формы один раз на кластер.
func (g *Generator) rollShapeParams(shape string, radius float64) clusterShapeParams {
	var p clusterShapeParams
	switch shape {
	case "blob":
		// Очаги смещаются от центра до 60% радиуса — не вылетают за территорию
		// кластера; их std — 0.25–0.5 радиуса (перекрываются, облако бесшовное).
		n := 2 + g.rng.Intn(3)
		p.seeds = make([]struct{ x, y, std float64 }, n)
		for i := range p.seeds {
			d := g.rng.Float64() * radius * 0.6
			a := 2 * math.Pi * g.rng.Float64()
			p.seeds[i].x = d * math.Cos(a)
			p.seeds[i].y = d * math.Sin(a)
			p.seeds[i].std = radius * (0.25 + g.rng.Float64()*0.25)
		}
	case "ring":
		p.ringR = radius * (0.75 + g.rng.Float64()*0.05)
	case "bar", "dumbbell":
		angle := 2 * math.Pi * g.rng.Float64()
		p.cosA, p.sinA = math.Cos(angle), math.Sin(angle)
	case "spiral":
		rot := 2 * math.Pi * g.rng.Float64()
		p.cosR, p.sinR = math.Cos(rot), math.Sin(rot)
	case "stream":
		nLobes := 2 + g.rng.Intn(3)
		step := radius * 0.45
		rot := 2 * math.Pi * g.rng.Float64()
		p.lobes = make([]struct{ x, y float64 }, nLobes)
		dir := rot
		x, y := 0.0, 0.0
		for i := 0; i < nLobes; i++ {
			p.lobes[i].x = x
			p.lobes[i].y = y
			dir += (g.rng.Float64() - 0.5) * 0.6 // изгиб ±0.3 рад на шаг
			x += step * math.Cos(dir)
			y += step * math.Sin(dir)
		}
	}
	return p
}

// shapeCandidate — одна случайная точка формы кластера (без проверок
// границ/minDist — их делают вызывающие). Параметры формы (поворот оси,
// очаги, радиус кольца) берутся из params, роллятся один раз на кластер
// (rollShapeParams), чтобы все точки кластера и добивка ложились в одну
// геометрию. Неизвестная форма → blob.
func (g *Generator) shapeCandidate(shape string, cx, cy, radius float64, params clusterShapeParams) (x, y float64) {
	switch shape {
	case "circle":
		std := radius * 0.7
		return cx + g.gaussian(std), cy + g.gaussian(std)
	case "ring":
		r := params.ringR + g.gaussian(radius*0.10)
		a := 2 * math.Pi * g.rng.Float64()
		return cx + r*math.Cos(a), cy + r*math.Sin(a)
	case "bar":
		u := g.gaussian(radius * 0.85)
		v := g.gaussian(radius * 0.12)
		return cx + u*params.cosA - v*params.sinA, cy + u*params.sinA + v*params.cosA
	case "spiral":
		const thetaMax = 2.5 * math.Pi
		theta := thetaMax * g.rng.Float64()
		r := radius * (theta / thetaMax)
		// Нормаль к спирали — перпендикуляр к радиус-вектору.
		nx, ny := -math.Sin(theta), math.Cos(theta)
		off := g.gaussian(radius * 0.07)
		x = r*math.Cos(theta) + off*nx
		y = r*math.Sin(theta) + off*ny
		// Поворот кластера на случайный угол.
		x, y = x*params.cosR-y*params.sinR, x*params.sinR+y*params.cosR
		return cx + x, cy + y
	case "dumbbell":
		sign := 1.0
		if g.rng.Float64() < 0.5 {
			sign = -1.0
		}
		u := sign*radius*0.65 + g.gaussian(radius*0.25)
		v := g.gaussian(radius * 0.25)
		return cx + u*params.cosA - v*params.sinA, cy + u*params.sinA + v*params.cosA
	case "stream":
		l := params.lobes[g.rng.Intn(len(params.lobes))]
		return cx + l.x + g.gaussian(radius*0.18), cy + l.y + g.gaussian(radius*0.18)
	case "core_halo":
		std := radius * 0.55
		if g.rng.Float64() < 0.75 {
			std = radius * 0.18
		}
		return cx + g.gaussian(std), cy + g.gaussian(std)
	default: // "blob" и неизвестное
		s := params.seeds[g.rng.Intn(len(params.seeds))]
		return cx + s.x + g.gaussian(s.std), cy + s.y + g.gaussian(s.std)
	}
}

// clusterPoints — точки кластера выбранной формы (см. Config.Shape).
// Форма и параметры формы передаются из generateWorldsPoisson (там же
// роллятся), чтобы добивка генерировала точки в той же геометрии.
// Соблюдает minDist между точками (отбор отбрасыванием).
func (g *Generator) clusterPoints(shape string, params clusterShapeParams, cx, cy, radius, minDist float64, count int) []struct{ X, Y float64 } {
	switch shape {
	case "circle":
		return g.clusterPointsCircle(cx, cy, radius, minDist, count, params)
	case "ring":
		return g.clusterPointsRing(cx, cy, radius, minDist, count, params)
	case "bar":
		return g.clusterPointsBar(cx, cy, radius, minDist, count, params)
	case "spiral":
		return g.clusterPointsSpiral(cx, cy, radius, minDist, count, params)
	case "dumbbell":
		return g.clusterPointsDumbbell(cx, cy, radius, minDist, count, params)
	case "stream":
		return g.clusterPointsStream(cx, cy, radius, minDist, count, params)
	case "core_halo":
		return g.clusterPointsCoreHalo(cx, cy, radius, minDist, count, params)
	default: // "blob" и неизвестное
		return g.clusterPointsBlob(cx, cy, radius, minDist, count, params)
	}
}

// clusterPointsCircle — круглая форма кластера: гауссова плотность,
// плотнее к центру (std = радиус×0.7), с размытой границей (см. clusterEdgeAccept).
// Соблюдает minDist между точками (отбор отбрасыванием).
func (g *Generator) clusterPointsCircle(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 50
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("circle", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterPointsBlob — бесформенная форма кластера: точки тянутся вокруг
// 2–4 перекрывающихся гауссовых очагов со случайными смещениями и масштабами.
// Кластер получается вытянутым и асимметричным, без круговой симметрии.
// Соблюдает minDist между точками (отбор отбрасыванием).
func (g *Generator) clusterPointsBlob(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 60
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("blob", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterPointsRing — кольцо: точки на кольце среднего радиуса 0.75–0.8×radius
// (роллится на кластер) с гауссовой толщиной (std = 0.10×radius) и равномерным
// углом. Центр пустой. Геометрия укладывается в территорию кластера:
// 0.8R + 3×0.10R = 1.10R ≤ 1.25R. Соблюдает minDist (отбор отбрасыванием).
func (g *Generator) clusterPointsRing(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 60
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("ring", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterPointsBar — перемычка: вытянута вдоль случайной оси (угол роллится
// на кластер), std вдоль оси = 0.85×radius, поперёк = 0.12×radius. Генерация
// в локальных координатах → поворот на угол; хвосты за 1.25×radius
// отбрасываются clusterEdgeAccept'ом. Соблюдает minDist (отбор отбрасыванием).
func (g *Generator) clusterPointsBar(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 60
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("bar", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterPointsSpiral — спиральный рукав (один): r(θ) = radius×(θ/2.5π),
// θ ∈ [0, 2.5π], ширина рукава (std по нормали к спирали) = 0.07×radius,
// случайный поворот кластера. Соблюдает minDist (отбор отбрасыванием).
func (g *Generator) clusterPointsSpiral(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 60
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("spiral", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterPointsDumbbell — две доли: два гауссовых очага на ±0.65×radius от
// центра вдоль случайной оси, std каждого = 0.25×radius, точки делятся ~50/50.
// Соблюдает minDist (отбор отбрасыванием).
func (g *Generator) clusterPointsDumbbell(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 60
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("dumbbell", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterPointsStream — цепочка/поток: 2–4 очага вдоль плавной дуги от центра
// (первый очаг в центре), std очага = 0.18×radius, шаг очагов = 0.45×radius,
// случайный изгиб (±0.3 рад на шаг) и поворот. Очаги за 1.25×radius
// отбрасываются clusterEdgeAccept'ом. Соблюдает minDist (отбор отбрасыванием).
func (g *Generator) clusterPointsStream(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 60
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("stream", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterPointsCoreHalo — ядро + гало: 75% точек — ядро (std = 0.18×radius),
// 25% — гало (std = 0.55×radius), оба вокруг центра. Соблюдает minDist
// (отбор отбрасыванием).
func (g *Generator) clusterPointsCoreHalo(cx, cy, radius, minDist float64, count int, params clusterShapeParams) []struct{ X, Y float64 } {
	if count <= 0 {
		return nil
	}
	points := make([]struct{ X, Y float64 }, 0, count)
	maxAttempts := count * 60
	for len(points) < count && maxAttempts > 0 {
		maxAttempts--
		x, y := g.shapeCandidate("core_halo", cx, cy, radius, params)
		if !g.clusterEdgeAccept(cx, cy, x, y, radius) {
			continue
		}
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if g.isPointValid(x, y, minDist, points) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
		}
	}
	return points
}

// clusterEdgeAccept — размытая граница кластера вместо жёсткого обреза:
// внутри радиуса принимаем всегда, на кольце R…1.25R — с линейно убывающей
// вероятностью, дальше — никогда. Убирает видимое «кольцо» из звёзд у края.
func (g *Generator) clusterEdgeAccept(cx, cy, x, y, radius float64) bool {
	r := math.Hypot(x-cx, y-cy)
	if r <= radius {
		return true
	}
	const tail = 0.25
	if r > radius*(1+tail) {
		return false
	}
	return g.rng.Float64() < 1-(r-radius)/(radius*tail)
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

// ---------- СЕТОЧНЫЙ ИНДЕКС ДЛЯ ПРОВЕРКИ MinDist ----------

// spatialGrid — равномерная сетка со стороной ячейки = minDist. Позволяет
// проверять minDist-конфликты за O(1): точка может конфликтовать только с
// точками в своей ячейке и соседних (|dx| < minDist ⇒ максимум на одну ячейку
// в обе стороны), поэтому смотрим окно 3×3.
//
// Линейный проход (isPointValid по всему списку) давал O(n²): при плотных
// запросах «добивка» и выбросы упирались в полный скан всех точек на каждую
// неудачную попытку — генерация 100k миров зависала на десятки минут.
type spatialGrid struct {
	x0, y0 float64
	cell   float64
	cols   int
	rows   int
	cells  [][]int // [row*cols+col] — индексы точек в xs/ys
	xs     []float64
	ys     []float64
}

func newSpatialGrid(x0, y0, x1, y1, cell float64) *spatialGrid {
	cols := int((x1-x0)/cell) + 2
	rows := int((y1-y0)/cell) + 2
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return &spatialGrid{
		x0: x0, y0: y0, cell: cell,
		cols: cols, rows: rows,
		cells: make([][]int, cols*rows),
		xs:    make([]float64, 0, 1024),
		ys:    make([]float64, 0, 1024),
	}
}

func (g *spatialGrid) cellOf(x, y float64) (c, r int) {
	c = int((x - g.x0) / g.cell)
	if c < 0 {
		c = 0
	} else if c >= g.cols {
		c = g.cols - 1
	}
	r = int((y - g.y0) / g.cell)
	if r < 0 {
		r = 0
	} else if r >= g.rows {
		r = g.rows - 1
	}
	return c, r
}

// Add добавляет точку в сетку.
func (g *spatialGrid) Add(x, y float64) {
	i := len(g.xs)
	g.xs = append(g.xs, x)
	g.ys = append(g.ys, y)
	c, r := g.cellOf(x, y)
	g.cells[r*g.cols+c] = append(g.cells[r*g.cols+c], i)
}

// HasNear — есть ли уже добавленная точка на расстоянии < minDist от (x, y).
func (g *spatialGrid) HasNear(x, y, minDist float64) bool {
	c, r := g.cellOf(x, y)
	d2 := minDist * minDist
	for cc := c - 1; cc <= c+1; cc++ {
		if cc < 0 || cc >= g.cols {
			continue
		}
		for rr := r - 1; rr <= r+1; rr++ {
			if rr < 0 || rr >= g.rows {
				continue
			}
			for _, i := range g.cells[rr*g.cols+cc] {
				dx := g.xs[i] - x
				dy := g.ys[i] - y
				if dx*dx+dy*dy < d2 {
					return true
				}
			}
		}
	}
	return false
}

func (g *Generator) generateWorldsRandom(minDist float64) []*models.World {
	targetCount := g.cfg.WorldCount
	halfSize := g.cfg.MapSize
	points := make([]struct{ X, Y float64 }, 0, targetCount)
	grid := newSpatialGrid(-halfSize, -halfSize, halfSize, halfSize, minDist)
	maxAttempts := targetCount * 200
	for len(points) < targetCount && maxAttempts > 0 {
		maxAttempts--
		x, y := g.randomPointInCircle(halfSize)
		if !g.galaxyEdgeAccept(x, y) {
			continue
		}
		if !grid.HasNear(x, y, minDist) {
			points = append(points, struct{ X, Y float64 }{X: x, Y: y})
			grid.Add(x, y)
		}
	}
	if len(points) < targetCount {
		log.Printf("⚠️ Generated only %d random worlds out of %d (not enough space with minDist=%.1f)", len(points), targetCount, minDist)
	}
	worlds := make([]*models.World, len(points))
	for i, p := range points {
		// Случайная генерация — регионов нет, профиля нет (59a §10).
		worlds[i] = g.generateWorld(struct{ X, Y float64 }{X: p.X, Y: p.Y}, nil, 0)
	}
	return worlds
}