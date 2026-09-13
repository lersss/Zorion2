package galaxy

import (
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"zorion/internal/models"
	"zorion/internal/names"
)

// Config – параметры генерации галактики
type Config struct {
	Seed           int64
	WorldCount     int
	MapSize        float64   // радиус круга
	MinDist        float64   // минимальное расстояние между мирами
	ClusterCount   int
	ClusterSpacing float64
	ClusterRadius  float64
	OutlierPercent float64
	WorldSpread    float64
	// Shape — форма звёздных кластеров: "blob" (бесформенное облако из
	// перекрывающихся очагов, по умолчанию) или "circle" (классический круг).
	// Любое пустое/неизвестное значение трактуется как "blob".
	Shape string
}

// clusterShape — нормализованная форма кластеров.
func (c *Config) clusterShape() string {
	if strings.ToLower(strings.TrimSpace(c.Shape)) == "circle" {
		return "circle"
	}
	return "blob"
}

// Generator создаёт миры и галактику
type Generator struct {
	cfg *Config
	rng *rand.Rand

	// usedNames — имена, уже занятые в текущей вселенной (для уникальности);
	// инициализируется в NewGenerator, чтобы дубли не появлялись между вызовами.
	usedNames map[string]bool
}

// NewGenerator создаёт новый генератор
func NewGenerator(cfg *Config) *Generator {
	return &Generator{
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(cfg.Seed)),
		usedNames: make(map[string]bool),
	}
}

// GalaxyResult — результат генерации: миры и регионы галактики.
type GalaxyResult struct {
	Worlds  []*models.World
	Regions []*models.Region
}

// GenerateGalaxy генерирует все миры
func (g *Generator) GenerateGalaxy() []*models.World {
	return g.GenerateGalaxyWithRegions().Worlds
}

// GenerateGalaxyWithRegions генерирует миры и регионы.
// Регионы строятся только для кластерной генерации (ClusterCount > 0):
// каждый кластерный центр становится регионом. Выбросы и добивка
// приписываются ближайшему региону.
func (g *Generator) GenerateGalaxyWithRegions() *GalaxyResult {
	if g.cfg.ClusterCount > 0 {
		return g.generateWorldsPoisson()
	}
	return &GalaxyResult{Worlds: g.generateWorldsRandom(g.cfg.MinDist)}
}

// ==================== РЕГИОНЫ ====================

// regionColors — палитра цветов регионов (туманности на тёмной карте).
var regionColors = []string{
	"#7c6cff", "#ff6b9d", "#4dd6b3", "#f5a623", "#5ac8fa",
	"#ff8a5c", "#b388ff", "#69f0ae", "#ffd740", "#40c4ff",
	"#f06292", "#81c784", "#ffb74d", "#9575cd", "#4db6ac",
	"#e57373", "#7986cb", "#aed581",
}

// buildRegions — создаёт регион вокруг каждого кластерного центра.
func (g *Generator) buildRegions(centers []struct{ X, Y float64 }) []*models.Region {
	regions := make([]*models.Region, 0, len(centers))
	radius := g.cfg.ClusterRadius
	if radius <= 0 {
		radius = 80.0
	}
	now := time.Now()
	for i, c := range centers {
		regions = append(regions, &models.Region{
			ID:        uuid.New().String(),
			Name:      names.GenerateRegionName(g.rng, g.usedNames),
			CenterX:   c.X,
			CenterY:   c.Y,
			Radius:    radius,
			Color:     regionColors[i%len(regionColors)],
			WorldCount: 0,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return regions
}

// nearestRegionIndex — индекс ближайшего региона к точке (для выбросов и добивки).
func (g *Generator) nearestRegionIndex(x, y float64, regions []*models.Region) int {
	if len(regions) == 0 {
		return -1
	}
	best := 0
	bestDist := math.Inf(1)
	for i, r := range regions {
		dx := r.CenterX - x
		dy := r.CenterY - y
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// ==================== СПЕКТРАЛЬНЫЕ КЛАССЫ ====================

// SpectralWeight — вес спектрального класса для генерации.
//
// Распределение близко к реальному (Mleчный Путь), но с чуть большим
// количеством «интересных» звёзд для геймплея:
//
//	O: 0.5%   (реально 0.00003%)
//	B: 2%     (реально 0.1%)
//	A: 4%     (реально 0.6%)
//	F: 7%     (реально 3%)
//	G: 12%    (реально 7%)
//	K: 17%    (реально 12%)
//	M: 32%    (реально 76%)
//	L: 8%     (не звёзды, но в игре есть)
//	T: 8%
//	Y: 9.5%
type spectralWeight struct {
	Class    string
	Weight   float64
	TempMin  int
	TempMax  int
}

var spectralWeights = []spectralWeight{
	{"O", 0.5, 30000, 50000},
	{"B", 2.0, 10000, 30000},
	{"A", 4.0, 7500, 10000},
	{"F", 7.0, 6000, 7500},
	{"G", 12.0, 5200, 6000},
	{"K", 17.0, 3700, 5200},
	{"M", 32.0, 2400, 3700},
	{"L", 8.0, 1300, 2400},
	{"T", 8.0, 700, 1300},
	{"Y", 9.5, 300, 700},
}

// totalSpectralWeight — сумма всех весов (100.0).
var totalSpectralWeight = func() float64 {
	sum := 0.0
	for _, sw := range spectralWeights {
		sum += sw.Weight
	}
	return sum
}()

// randomSpectralClass — взвешенный выбор спектрального класса.
func randomSpectralClass(rng *rand.Rand) string {
	r := rng.Float64() * totalSpectralWeight
	for _, sw := range spectralWeights {
		r -= sw.Weight
		if r <= 0 {
			return sw.Class
		}
	}
	return "M" // fallback
}

// RandomSpectralClass — экспорт randomSpectralClass для внешних генераторов
// (параметрическая генерация «Проверка гипотез»).
func RandomSpectralClass(rng *rand.Rand) string {
	return randomSpectralClass(rng)
}

// randomTemperature — температура звезды, согласованная со спектром.
//
// Каждый класс имеет свой диапазон. Это гарантирует, что G-звезда
// не получит 38000 K, а O-звезда — 300 K.
func randomTemperature(spectralClass string, rng *rand.Rand) int {
	for _, sw := range spectralWeights {
		if sw.Class == spectralClass {
			span := sw.TempMax - sw.TempMin
			return sw.TempMin + rng.Intn(span)
		}
	}
	// fallback — G-звезда
	return 5200 + rng.Intn(800)
}

// RandomTemperature — экспорт randomTemperature для внешних генераторов.
func RandomTemperature(spectralClass string, rng *rand.Rand) int {
	return randomTemperature(spectralClass, rng)
}

// ==================== ГЕНЕРАЦИЯ МИРА ====================

// generateWorld создаёт один мир по заданным координатам.
func (g *Generator) generateWorld(center struct{ X, Y float64 }) *models.World {
	spread := g.cfg.WorldSpread
	x := center.X + g.rng.NormFloat64()*spread
	y := center.Y + g.rng.NormFloat64()*spread

	radius := g.cfg.MapSize
	if math.Hypot(x, y) > radius {
		// Точка вышла за круг — оставляем как есть (MVP),
		// в будущем можно отбрасывать.
	}

	usedNames := g.usedNames

	// Сначала спектр, потом температура — они связаны
	spectralClass := randomSpectralClass(g.rng)
	temperature := randomTemperature(spectralClass, g.rng)

	now := time.Now()
	world := &models.World{
		ID:            uuid.New().String(),
		Name:          names.GeneratePlanetName(g.rng, usedNames),
		CoordX:        x,
		CoordY:        y,
		SpectralClass: spectralClass,
		Temperature:   temperature,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return world
}