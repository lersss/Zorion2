package galaxy

import (
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"zorion/internal/astro"
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
	cfg        *Config
	rng        *rand.Rand
	weights    Weights
	massRanges StellarMassRanges

	// usedNames — имена, уже занятые в текущей вселенной (для уникальности);
	// инициализируется в NewGenerator, чтобы дубли не появлялись между вызовами.
	usedNames map[string]bool
}

// NewGenerator создаёт новый генератор с дефолтными весами и массами.
func NewGenerator(cfg *Config) *Generator {
	return &Generator{
		cfg:        cfg,
		rng:        rand.New(rand.NewSource(cfg.Seed)),
		weights:    DefaultWeights(),
		massRanges: DefaultStellarMassRanges(),
		usedNames:  make(map[string]bool),
	}
}

// NewGeneratorWithWeights — генератор с конфигурируемыми весами
// (админка, спека 99.2.3 §4.4). Пустой конфиг — дефолтные веса.
func NewGeneratorWithWeights(cfg *Config, weights Weights) *Generator {
	g := NewGenerator(cfg)
	if len(weights.Spectral) > 0 || len(weights.SystemTypes) > 0 {
		g.weights = weights
	}
	return g
}

// SetMassRanges — диапазоны массы звёзд (29a §4м, конфиг 99.2.3).
// Пустой конфиг — дефолтные диапазоны.
func (g *Generator) SetMassRanges(r StellarMassRanges) {
	if len(r) > 0 {
		g.massRanges = r
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

// ==================== ВЕСА ГЕНЕРАЦИИ ЗВЁЗД (99.2.3 §4, дефолты 99.2.4 §4) ====================

// spectralOrder — порядок классов для взвешенного ролла (конфиг-веса).
var spectralOrder = []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}

// systemTypeOrder — порядок категорий систем/объектов (99.2.4 §4.1).
var systemTypeOrder = []string{
	"single", "binary", "multiple",
	"black_hole", "neutron", "white_dwarf", "protostar", "exotic",
}

// Weights — конфиг весов генерации звёзд (99.2.3 §4.1/§4.2).
// Spectral — веса спектральных классов O–Y; SystemTypes — веса типов
// систем/объектов (одиночная/двойная/кратная + экзотика).
type Weights struct {
	Spectral    map[string]float64 `json:"spectral"`
	SystemTypes map[string]float64 `json:"system_types"`
}

// DefaultWeights — дефолтные веса (99.2.4 §4.1/§4.2; сумма типов = 100).
func DefaultWeights() Weights {
	return Weights{
		Spectral: map[string]float64{
			"O": 0.5, "B": 2.0, "A": 4.0, "F": 7.0, "G": 12.0,
			"K": 17.0, "M": 32.0, "L": 8.0, "T": 8.0, "Y": 9.5,
		},
		SystemTypes: map[string]float64{
			"single": 85, "binary": 5, "multiple": 2,
			"black_hole": 1, "neutron": 1, "white_dwarf": 2, "protostar": 1, "exotic": 3,
		},
	}
}

// weightedPick — взвешенный выбор по map весов с фиксированным порядком ключей.
// Пустая/нулевая сумма — первый ключ (защита от деления на ноль).
func weightedPick(rng *rand.Rand, weights map[string]float64, order []string) string {
	total := 0.0
	for _, k := range order {
		total += weights[k]
	}
	if total <= 0 {
		return order[0]
	}
	r := rng.Float64() * total
	for _, k := range order {
		r -= weights[k]
		if r <= 0 {
			return k
		}
	}
	return order[len(order)-1]
}

// spectralClass — класс мира: конфиг-веса, если заданы, иначе дефолт.
func (g *Generator) spectralClass() string {
	if len(g.weights.Spectral) > 0 {
		return weightedPick(g.rng, g.weights.Spectral, spectralOrder)
	}
	return randomSpectralClass(g.rng)
}

// randomSystemType — категория системы/объекта (99.2.4 §4.1).
func (g *Generator) randomSystemType() string {
	if len(g.weights.SystemTypes) > 0 {
		return weightedPick(g.rng, g.weights.SystemTypes, systemTypeOrder)
	}
	return weightedPick(g.rng, DefaultWeights().SystemTypes, systemTypeOrder)
}

// defaultSpectralWeight — вес класса из таблицы spectralWeights (для подмножеств).
func defaultSpectralWeight(cls string) float64 {
	for _, sw := range spectralWeights {
		if sw.Class == cls {
			return sw.Weight
		}
	}
	return 1
}

// supergiantClass — класс «прочей экзотики»: O/B/A по спектральным весам (99.2.4 §4.4).
func (g *Generator) supergiantClass() string {
	weights := map[string]float64{}
	for _, cls := range []string{"O", "B", "A"} {
		weights[cls] = g.weights.Spectral[cls]
		if weights[cls] <= 0 {
			weights[cls] = defaultSpectralWeight(cls)
		}
	}
	return weightedPick(g.rng, weights, []string{"O", "B", "A"})
}

// isClassIn — класс входит в список.
func isClassIn(cls string, classes ...string) bool {
	for _, c := range classes {
		if cls == c {
			return true
		}
	}
	return false
}

// ==================== МАССА ЗВЁЗД (29a §4м) ====================

// MassRange — диапазон массы звезды (M☉): равномерно в [Min, Max].
type MassRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// StellarMassRanges — диапазоны массы по типу (29a §4м): спектральные классы
// O–Y + black_hole/neutron/white_dwarf/protostar/exotic. Конфиг генератора
// звёзд (99.2.3): дефолты здесь, override — в generation_config.
type StellarMassRanges map[string]MassRange

// DefaultStellarMassRanges — физические дефолты (29a §4м).
func DefaultStellarMassRanges() StellarMassRanges {
	return StellarMassRanges{
		"O": {Min: 15, Max: 60}, "B": {Min: 3, Max: 18}, "A": {Min: 1.5, Max: 3.5},
		"F": {Min: 1.1, Max: 1.6}, "G": {Min: 0.9, Max: 1.2}, "K": {Min: 0.6, Max: 0.9},
		"M": {Min: 0.1, Max: 0.6}, "L": {Min: 0.03, Max: 0.08}, "T": {Min: 0.015, Max: 0.03},
		"Y": {Min: 0.01, Max: 0.02},
		"black_hole":   {Min: 3, Max: 20},
		"neutron":      {Min: 1.1, Max: 2.2},
		"white_dwarf":  {Min: 0.2, Max: 1.4},
		"protostar":    {Min: 0.5, Max: 3},
		"exotic":       {Min: 10, Max: 60}, // сверхгиганты O–B–A (фаза I)
	}
}

// randomMass — масса звезды по типу: равномерно в [min, max] из конфига
// (или дефолтов). Ключ отсутствует/битый — масса не задаётся (NULL).
func (g *Generator) randomMass(key string) (float64, bool) {
	r, ok := g.massRanges[key]
	if !ok || r.Min <= 0 || r.Max < r.Min {
		return 0, false
	}
	return r.Min + g.rng.Float64()*(r.Max-r.Min), true
}

// ==================== ГЕНЕРАЦИЯ МИРА ====================

// generateWorld создаёт один мир по заданным координатам.
//
// Слои (99.2.4 §2): категория системы/объекта (§4.1) → обычная звезда O–Y
// (спектр + температура + модификаторы §4.3) или экзотика (своя ветка §4.1 п.3,
// спектр NULL).
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

	now := time.Now()
	world := &models.World{
		ID:         uuid.New().String(),
		Name:       names.GeneratePlanetName(g.rng, usedNames),
		CoordX:     x,
		CoordY:     y,
		StarType:   "star",
		SystemType: "single",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	category := g.randomSystemType()
	switch category {
	case "black_hole", "neutron", "white_dwarf", "protostar", "exotic":
		g.fillExoticWorld(world, category)
	default: // single / binary / multiple — обычная звезда O–Y
		world.SystemType = category
		cls := g.spectralClass()
		world.SpectralClass = cls
		world.Temperature = randomTemperature(cls, g.rng)
		if mass, ok := g.randomMass(cls); ok {
			world.StellarMass = &mass
		}
		g.rollStarMods(world, category, cls)
	}
	return world
}

// fillExoticWorld — ветка экзотики (99.2.4 §4.1 п.3): спектральный класс NULL,
// температура, масса и модификаторы — по типу объекта (§5.1, §4.3, 29a §4м).
func (g *Generator) fillExoticWorld(w *models.World, category string) {
	mods := &models.StellarMods{}
	switch category {
	case "black_hole":
		w.StarType = "black_hole"
		w.SpectralClass = ""          // фотосферы нет → NULL
		w.Temperature = 0             // «тёмная» (§4д.4, §5.1); UI — «тёмная», не «0 K»
		if mass, ok := g.randomMass("black_hole"); ok {
			w.StellarMass = &mass
		}
		if g.rng.Float64() < 0.10 {   // микроквазар: 10% (§4.3)
			mods.Subtype = "accretion"
			mods.DiskState = "accretion"
		}
	case "neutron":
		w.StarType = "neutron"
		w.SpectralClass = ""
		w.Temperature = 100000 + g.rng.Intn(900001) // 10⁵–10⁶ K (§5.1)
		if mass, ok := g.randomMass("neutron"); ok {
			w.StellarMass = &mass
		}
		if g.rng.Float64() < 0.60 {
			mods.Subtype = "pulsar" // 60% (§4.3)
		} else if g.rng.Float64() < 0.05 {
			mods.Subtype = "magnetar" // 5% (§4.3)
		}
	case "white_dwarf":
		w.StarType = "white_dwarf"
		w.SpectralClass = ""
		w.Temperature = 5000 + g.rng.Intn(25001) // 5000–30000 K (§5.1)
		if mass, ok := g.randomMass("white_dwarf"); ok {
			w.StellarMass = &mass
		}
		mods.DiskState = "debris"                // обломочный пояс, не планеты (§5.3)
	case "protostar":
		w.StarType = "protostar"
		w.Temperature = 3000 + g.rng.Intn(1001) // 3000–4000 K (§5.1)
		// Протозвезда — реальный спектральный класс по температуре
		// (решение создателя, баг #3): T < 3700 → M, иначе K (граница
		// классов M 2400–3700 / K 3700–5200 из spectralWeights).
		if w.Temperature < 3700 {
			w.SpectralClass = "M"
		} else {
			w.SpectralClass = "K"
		}
		if mass, ok := g.randomMass("protostar"); ok {
			w.StellarMass = &mass
		}
		mods.DiskState = "protoplanetary" // диск вместо планет (§5.3)
		if g.rng.Float64() < 0.50 {
			mods.VariableType = "t_tauri" // 50% (§4.3)
		}
	case "exotic":
		// «Прочая экзотика» = сверхгиганты O–B–A (99.2.4 §4.4): star + фаза I +
		// масса 10–60 M☉ (колонка stellar_mass, 29a §4м) + подтипы lbv/wr.
		w.StarType = "star"
		w.SpectralClass = g.supergiantClass()
		w.Temperature = randomTemperature(w.SpectralClass, g.rng) // диапазон класса
		if mass, ok := g.randomMass("exotic"); ok {
			w.StellarMass = &mass
		}
		mods.Phase = "I"
		r := g.rng.Float64()
		if r < 0.10 {
			mods.Subtype = "lbv"
		} else if r < 0.20 {
			mods.Subtype = "wr"
		}
	}
	w.StellarMods = mods

	// Возраст системы (41a §4.1): остатки (ЧД/НЗ/WD) — 2–13 млрд лет
	// (галактический максимум, решение создателя 2026-09-15); протозвезда —
	// 0.001–0.01 млрд лет (1–10 млн лет, §8.2 вариант А — до-ГП фаза).
	// Сверхгиганты-экзотика (category "exotic", star_type='star') и обычные
	// звёзды — NULL: возраст показывает фронтовый справочник по классу.
	switch category {
	case "black_hole", "neutron", "white_dwarf":
		age := 2 + g.rng.Float64()*11
		w.Age = &age
	case "protostar":
		age := 0.001 + g.rng.Float64()*0.009
		w.Age = &age
	}
}

// ==================== ПАРАМЕТРЫ ДВОЙНЫХ (35b §3) ====================

// Диапазоны разделений пар (35b §3.2, §5.2):
//   - close: log-uniform [0.05, 0.9] а.е. (реш. №7) — источник P-планет;
//   - wide:  log-uniform [100, 3000] а.е. (реш. @balancetester В4, нижняя
//     граница a ≳ 100 из 99.2.4 §4.3);
//   - внешний компаньон кратных: log-uniform [1000, 10000] а.е., инвариант
//     ≥ 3× разделения внутренней пары (реш. №3а; при дефолтных диапазонах
//     выполняется автоматически, пере-ролл — страховка от кастомизации).
const (
	closeSepMin      = 0.05
	closeSepMax      = 0.9
	wideSepMin       = 100.0
	wideSepMax       = 3000.0
	extraSepMin      = 1000.0
	extraSepMax      = 10000.0
	extraSepMinRatio = 3.0
)

// logUniform — log-uniform ролл в [min, max] (минимальный > 0).
func logUniform(rng *rand.Rand, min, max float64) float64 {
	return math.Exp(math.Log(min) + rng.Float64()*(math.Log(max)-math.Log(min)))
}

// compMassiveThan — «a массивнее b» (35c §ТЗ): сравнение по массе; при
// равенстве масс или отсутствии массы — по светимости (astro.
// LuminosityBySpectral). Возвращает true только при строгом превосходстве.
// Единый источник правила «главная = самая массивная»: используется и
// сортировкой пары (sortMassiveFirst), и пакетом кратной (rollMultiple).
func compMassiveThan(aClass string, aMass *float64, bClass string, bMass *float64) bool {
	if aMass != nil && bMass != nil {
		if *aMass != *bMass {
			return *aMass > *bMass
		}
		// Равные массы — tiebreak по светимости.
		return astro.LuminosityBySpectral(aClass) > astro.LuminosityBySpectral(bClass)
	}
	if aMass != nil {
		return true // у b массы нет — a массивнее
	}
	if bMass != nil {
		return false
	}
	// Массы нет у обоих — по светимости.
	return astro.LuminosityBySpectral(aClass) > astro.LuminosityBySpectral(bClass)
}

// sortMassiveFirst — сортировка «главная = массивнее» (35c §ТЗ п.2): если
// масса компаньона больше массы главной — компоненты меняются местами
// целиком (класс, температура, массы, companion-спектр/температура).
// Tiebreak при равенстве/отсутствии массы — по светимости; при равенстве —
// ничего. Инвариант после: M₂ ≤ M₁ (в пределах tiebreak). Следствие по
// цепочке генерации: счёт планет и gasGiantChance считаются от нового
// класса главной.
func sortMassiveFirst(w *models.World, mods *models.StellarMods) {
	if w == nil || mods == nil || mods.Companion == "" {
		return
	}
	if !compMassiveThan(mods.Companion, mods.CompanionMass, w.SpectralClass, w.StellarMass) {
		return
	}
	w.SpectralClass, mods.Companion = mods.Companion, w.SpectralClass
	if mods.CompanionTemp != nil {
		t := *mods.CompanionTemp
		worldTemp := w.Temperature
		w.Temperature = t
		mods.CompanionTemp = &worldTemp // НЕ &w.Temperature — алиасинг указателя
	}
	if w.StellarMass != nil || mods.CompanionMass != nil {
		w.StellarMass, mods.CompanionMass = mods.CompanionMass, w.StellarMass
	}
}

// rollExtraSepAU — sep внешнего компаньона кратной (35c §ТЗ п.4):
// log-uniform [1000, 10000] а.е., инвариант ≥ 3× разделения внутренней пары
// (пере-ролл при нарушении на случай кастомизированных диапазонов; страховка
// от бесконечного цикла — кламп к 3×). Отдельно от ролла класса/массы/
// температуры: в пакете кратной (rollMultiple) внешний роллится до выбора
// главной, а sep — после sep компаньона.
func (g *Generator) rollExtraSepAU(innerSepAU float64) float64 {
	for i := 0; i < 50; i++ {
		sep := logUniform(g.rng, extraSepMin, extraSepMax)
		if sep >= extraSepMinRatio*innerSepAU {
			return sep
		}
	}
	return extraSepMinRatio * innerSepAU
}

// starComp — компонент кратной системы для пакетной сортировки (35c §ТЗ п.1).
type starComp struct {
	class string
	mass  *float64
	temp  int
}

// rollMultiple — пакет на 3 компонента (35c §ТЗ п.1): главная = самая
// массивная. Тройка (главная из мира + компаньон + внешний) сортируется по
// массе убыванию (tiebreak — светимость, compMassiveThan); победитель
// перезаписывает мир, второй — компаньон (wide, sep log-uniform
// [100, 3000]), третий — внешний (sep log-uniform [1000, 10000], ≥ 3× sep
// компаньона). Класс/масса/температура внешнего роллятся до сортировки,
// sep отложен до выбора главной (зависит от sep компаньона).
func (g *Generator) rollMultiple(w *models.World, mods *models.StellarMods) {
	mods.BinaryType = "wide"

	compClass := randomSpectralClass(g.rng)
	compTemp := randomTemperature(compClass, g.rng)
	var compMass *float64
	if mass, ok := g.randomMass(compClass); ok {
		compMass = &mass
	}

	extClass := randomSpectralClass(g.rng)
	extTemp := randomTemperature(extClass, g.rng)
	var extMass *float64
	if mass, ok := g.randomMass(extClass); ok {
		extMass = &mass
	}

	trio := []starComp{
		{class: w.SpectralClass, mass: w.StellarMass, temp: w.Temperature},
		{class: compClass, mass: compMass, temp: compTemp},
		{class: extClass, mass: extMass, temp: extTemp},
	}
	sort.SliceStable(trio, func(i, j int) bool {
		return compMassiveThan(trio[i].class, trio[i].mass, trio[j].class, trio[j].mass)
	})

	// Победитель → главная.
	w.SpectralClass = trio[0].class
	w.Temperature = trio[0].temp
	w.StellarMass = trio[0].mass

	// Второй по массе → компаньон (multiple — принудительно wide, реш. №3а).
	mods.Companion = trio[1].class
	mods.CompanionMass = trio[1].mass
	mods.CompanionTemp = &trio[1].temp
	sep := logUniform(g.rng, wideSepMin, wideSepMax)
	mods.CompanionSepAU = &sep

	// Третий → внешний (sep ≥ 3× sep компаньона, иерархия 3×).
	ec := models.ExtraCompanion{
		SpectralClass: trio[2].class,
		Mass:          trio[2].mass,
		Temp:          &trio[2].temp,
	}
	ec.SepAU = g.rollExtraSepAU(sep)
	mods.ExtraCompanions = []models.ExtraCompanion{ec}
}

// rollStarMods — модификаторы обычной звезды (99.2.4 §4.3): фаза (V/III/I),
// переменность по гейтам (иначе «цефеида на главной последовательности»),
// параметры двойной. Вероятности — хардкод-дефолты фичи.
func (g *Generator) rollStarMods(w *models.World, systemType, cls string) {
	mods := &models.StellarMods{}

	phase := "V"
	// Фаза III: красные гиганты среди K/M (8% от популяции класса).
	if (cls == "K" || cls == "M") && g.rng.Float64() < 0.08 {
		phase = "III"
	}
	// Фаза I: сверхгиганты F–K (1%) — база цефеид.
	if isClassIn(cls, "F", "G", "K") && g.rng.Float64() < 0.01 {
		phase = "I"
	}
	if phase != "V" {
		mods.Phase = phase
	}

	// Переменность по гейтам §4.3.
	switch {
	case phase == "III" && cls == "M" && g.rng.Float64() < 0.50:
		mods.VariableType = "mira" // пульсирующий гигант
	case cls == "M" && phase == "V" && g.rng.Float64() < 0.04:
		mods.VariableType = "uv_ceti" // вспыхивающий красный карлик
	case phase == "I" && isClassIn(cls, "F", "G", "K") && g.rng.Float64() < 0.25:
		mods.VariableType = "cepheid" // стандартная свеча, период 1–50 дней
		period := 1 + g.rng.Float64()*49
		mods.VariablePeriodDays = &period
	}

	if systemType == "binary" || systemType == "multiple" {
		if g.rng.Float64() < 0.03 {
			mods.VariableType = "eclipsing" // затменные: 3% от двойных (§4.3)
		}
		// Параметры двойной (35b §3.2): multiple — принудительно wide (реш.
		// №3а, тесная иерархия у кратных — физический абсурд); binary —
		// широкие 75% (S-тип планет) / тесные 25% (P-тип).
		if systemType == "multiple" {
			// Кратная — пакет на 3 компонента: главная = самая массивная
			// (35c §ТЗ п.1). Пакет уже упорядочен — сортировка пары не нужна.
			g.rollMultiple(w, mods)
		} else {
			if g.rng.Float64() < 0.75 {
				mods.BinaryType = "wide"
			} else {
				mods.BinaryType = "close"
			}
			compClass := randomSpectralClass(g.rng)
			mods.Companion = compClass
			// Масса/температура компаньона (обычная звезда O–Y, экзотика
			// невозможна — 35b §3): randomMass/randomTemperature по классу.
			if mass, ok := g.randomMass(compClass); ok {
				mods.CompanionMass = &mass
			}
			compTemp := randomTemperature(compClass, g.rng)
			mods.CompanionTemp = &compTemp
			// Разделение по типу (35b §3.2): close [0.05, 0.9], wide [100, 3000].
			if mods.BinaryType == "close" {
				sep := logUniform(g.rng, closeSepMin, closeSepMax)
				mods.CompanionSepAU = &sep
			} else {
				sep := logUniform(g.rng, wideSepMin, wideSepMax)
				mods.CompanionSepAU = &sep
			}
			// Сортировка «главная = массивнее» (35c §ТЗ п.2).
			sortMassiveFirst(w, mods)
		}
	}

	if mods.Phase != "" || mods.VariableType != "" || mods.BinaryType != "" || mods.Companion != "" {
		w.StellarMods = mods
	}
}