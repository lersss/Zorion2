package galaxy

import (
	"math"
	"math/rand"
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

// GenerateGalaxy генерирует все миры
func (g *Generator) GenerateGalaxy() []*models.World {
	if g.cfg.ClusterCount > 0 {
		return g.generateWorldsPoisson()
	}
	return g.generateWorldsRandom(g.cfg.MinDist)
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