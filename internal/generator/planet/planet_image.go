// internal/generator/planet/planet_image.go
package planet

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
)

// ---------- Структуры для климатов ----------

type ClimateWeight struct {
	O float64 `json:"O"`
	B float64 `json:"B"`
	A float64 `json:"A"`
	F float64 `json:"F"`
	G float64 `json:"G"`
	K float64 `json:"K"`
	M float64 `json:"M"`
}

type Climate struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Weight              ClimateWeight `json:"weight"`
	AllowedSurfaces     []string      `json:"allowed_surfaces"`
	AllowedHydrospheres []string      `json:"allowed_hydrospheres"`
	AllowedAtmospheres  []string      `json:"allowed_atmospheres"`
	AllowedBiospheres   []string      `json:"allowed_biospheres"`
	TemperatureMin      int           `json:"temperature_min"`
	TemperatureMax      int           `json:"temperature_max"`
	WaterChance         float64       `json:"water_chance"`
	LifeChance          float64       `json:"life_chance"`
}

type ClimateData struct {
	Climates []Climate `json:"climates"`
}

// ---------- Генератор изображений ----------

// PlanetGenerator — потокобезопасный генератор.
//
// Все обращения к cache, cacheOrder и rand защищены мьютексом mu.
// climates/canvasSize/enableCache/maxCacheSize — только читаются после
// создания, поэтому не требуют защиты.
type PlanetGenerator struct {
	mu sync.Mutex

	climates     []Climate
	canvasSize   int
	enableCache  bool
	cache        map[string]*CachedPlanet
	cacheOrder   []string
	maxCacheSize int
	rand         *rand.Rand
}

type CachedPlanet struct {
	Image *image.RGBA
	Meta  PlanetMeta
}

type PlanetMeta struct {
	Type          string
	Radius        int
	Seed          int64
	Climate       ClimateInfo
	Surface       string
	Hydrosphere   string
	Atmosphere    string
	Biosphere     string
	HasAtmosphere bool
	HasRings      bool
	StarType      string
	Temperature   float64
}

type ClimateInfo struct {
	ID   string
	Name string
	Temp float64
}

// ---------- Конструктор ----------

// newPlanetGeneratorBase — базовая инициализация генератора (общая для
// NewPlanetGenerator и NewImageGenerator).
func newPlanetGeneratorBase() *PlanetGenerator {
	return &PlanetGenerator{
		canvasSize:   128,
		enableCache:  true,
		cache:        make(map[string]*CachedPlanet),
		cacheOrder:   []string{},
		maxCacheSize: 2000,
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func NewPlanetGenerator(climateFile string, opts ...func(*PlanetGenerator)) (*PlanetGenerator, error) {
	data, err := os.ReadFile(climateFile)
	if err != nil {
		return nil, err
	}
	var climateData ClimateData
	if err := json.Unmarshal(data, &climateData); err != nil {
		return nil, err
	}
	pg := newPlanetGeneratorBase()
	pg.climates = climateData.Climates
	for _, opt := range opts {
		opt(pg)
	}
	return pg, nil
}

// NewImageGenerator — генератор картинок без климат-файла (спека 2026-09-20):
// GeneratePlanetImage не использует climates (входы — только видимые параметры,
// §3.1); климат-файл нужен только старому GeneratePlanet (фолбэк старых миров
// в generateHonest его не использует).
func NewImageGenerator(opts ...func(*PlanetGenerator)) *PlanetGenerator {
	pg := newPlanetGeneratorBase()
	for _, opt := range opts {
		opt(pg)
	}
	return pg
}

func WithCanvasSize(size int) func(*PlanetGenerator) {
	return func(pg *PlanetGenerator) { pg.canvasSize = size }
}
func WithCacheEnabled(enabled bool) func(*PlanetGenerator) {
	return func(pg *PlanetGenerator) { pg.enableCache = enabled }
}
func WithMaxCacheSize(size int) func(*PlanetGenerator) {
	return func(pg *PlanetGenerator) { pg.maxCacheSize = size }
}

// ---------- Опции для генерации ----------

type GenerateOptions struct {
	Radius        int
	StarType      string
	ClimateID     string
	Surface       string
	Hydrosphere   string
	Atmosphere    string
	Biosphere     string
	HasRings      *bool
	Seed          int64
	Lit           bool
}

func WithRadius(r int) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.Radius = r }
}
func WithSeed(s int64) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.Seed = s }
}
func WithStarType(s string) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.StarType = s }
}
func WithClimateID(id string) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.ClimateID = id }
}
func WithSurface(s string) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.Surface = s }
}
func WithHydrosphere(s string) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.Hydrosphere = s }
}
func WithAtmosphere(s string) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.Atmosphere = s }
}
func WithBiosphere(s string) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.Biosphere = s }
}
func WithRings(b bool) func(*GenerateOptions) {
	return func(o *GenerateOptions) { o.HasRings = &b }
}

// ---------- Основной метод ----------

func (pg *PlanetGenerator) GeneratePlanet(radius int, opts ...func(*GenerateOptions)) (*CachedPlanet, error) {
	options := &GenerateOptions{
		Radius:      radius,
		StarType:    "",
		ClimateID:   "",
		Surface:     "",
		Hydrosphere: "",
		Atmosphere:  "",
		Biosphere:   "",
		HasRings:    nil,
		Seed:        0,
	}
	for _, opt := range opts {
		opt(options)
	}

	seed := options.Seed
	if seed == 0 {
		seed = pg.nextInt63()
	}

	if pg.enableCache {
		if p, ok := pg.cacheGet(generateKey(seed, options)); ok {
			return p, nil
		}
	}
	rng := rand.New(rand.NewSource(seed))

	// Дальше работаем ТОЛЬКО с локальным rng и локальными переменными.
	// Общий стейт (pg.cache, pg.cacheOrder) трогаем только под мьютексом.

	var climate *Climate
	if options.ClimateID != "" {
		for i := range pg.climates {
			if pg.climates[i].ID == options.ClimateID {
				climate = &pg.climates[i]
				break
			}
		}
	}
	if climate == nil {
		starType := options.StarType
		if starType == "" {
			starTypes := []string{"O", "B", "A", "F", "G", "K", "M"}
			starType = starTypes[rng.Intn(len(starTypes))]
		}
		climate = pg.selectClimateByStarType(starType, rng)
	}

	surface := options.Surface
	if surface == "" {
		surface = pickRandom(climate.AllowedSurfaces, rng)
	}
	hydrosphere := options.Hydrosphere
	if hydrosphere == "" {
		hydrosphere = pickRandom(climate.AllowedHydrospheres, rng)
	}
	atmosphere := options.Atmosphere
	if atmosphere == "" {
		atmosphere = pickRandom(climate.AllowedAtmospheres, rng)
	}
	biosphere := options.Biosphere
	if biosphere == "" {
		biosphere = pickRandom(climate.AllowedBiospheres, rng)
	}

	tempMin := float64(climate.TemperatureMin)
	tempMax := float64(climate.TemperatureMax)
	temperature := tempMin + rng.Float64()*(tempMax-tempMin)

	visualType := pg.determineVisualType(climate, surface, hydrosphere, temperature, rng)

	hasAtmosphere := (atmosphere != "разреженная" && atmosphere != "")

	hasRings := false
	if options.HasRings != nil {
		hasRings = *options.HasRings
	} else {
		if visualType == "gas" && rng.Float64() < 0.4 {
			hasRings = true
		} else if visualType != "gas" && rng.Float64() < 0.05 {
			hasRings = true
		}
	}

	size := pg.canvasSize
	img := pg.generateTexture(visualType, size, rng, options, temperature)
	fx := legacyPostFX(visualType, hasAtmosphere)
	applyPostProcessing(img, size, fx, rng)
	if hasRings {
		drawRings(img, size, rng, fx.L, color.RGBA{200, 180, 150, 255}, 0)
	}
	finalImg := scaleImage(img, radius)

	meta := PlanetMeta{
		Type:          visualType,
		Radius:        radius,
		Seed:          seed,
		Climate:       ClimateInfo{ID: climate.ID, Name: climate.Name, Temp: temperature},
		Surface:       surface,
		Hydrosphere:   hydrosphere,
		Atmosphere:    atmosphere,
		Biosphere:     biosphere,
		HasAtmosphere: hasAtmosphere,
		HasRings:      hasRings,
		StarType:      options.StarType,
		Temperature:   temperature,
	}
	planet := &CachedPlanet{Image: finalImg, Meta: meta}

	if pg.enableCache {
		key := generateKey(seed, options)
		pg.cachePut(key, planet)
	}
	return planet, nil
}

// nextInt63 — безопасный доступ к общему рандому.
func (pg *PlanetGenerator) nextInt63() int64 {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	return pg.rand.Int63()
}

// cachePut — безопасная запись в кэш с вытеснением по FIFO.
func (pg *PlanetGenerator) cachePut(key string, planet *CachedPlanet) {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	if pg.maxCacheSize > 0 && len(pg.cache) >= pg.maxCacheSize && len(pg.cacheOrder) > 0 {
		oldest := pg.cacheOrder[0]
		delete(pg.cache, oldest)
		pg.cacheOrder = pg.cacheOrder[1:]
	}

	pg.cache[key] = planet
	pg.cacheOrder = append(pg.cacheOrder, key)
}

// cacheGet — безопасное чтение из кэша.
// Вызывается в GeneratePlanet до генерации; хит возвращает готовую текстуру.
func (pg *PlanetGenerator) cacheGet(key string) (*CachedPlanet, bool) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	p, ok := pg.cache[key]
	return p, ok
}

func (pg *PlanetGenerator) selectClimateByStarType(starType string, rng *rand.Rand) *Climate {
	type wc struct {
		climate *Climate
		weight  float64
	}
	var weighted []wc
	for i := range pg.climates {
		c := &pg.climates[i]
		var w float64
		switch starType {
		case "O":
			w = c.Weight.O
		case "B":
			w = c.Weight.B
		case "A":
			w = c.Weight.A
		case "F":
			w = c.Weight.F
		case "G":
			w = c.Weight.G
		case "K":
			w = c.Weight.K
		case "M":
			w = c.Weight.M
		default:
			w = 0
		}
		if w > 0 {
			weighted = append(weighted, wc{climate: c, weight: w})
		}
	}
	if len(weighted) == 0 {
		return &pg.climates[0]
	}
	total := 0.0
	for _, w := range weighted {
		total += w.weight
	}
	r := rng.Float64() * total
	for _, w := range weighted {
		r -= w.weight
		if r <= 0 {
			return w.climate
		}
	}
	return weighted[len(weighted)-1].climate
}

func pickRandom(list []string, rng *rand.Rand) string {
	if len(list) == 0 {
		return ""
	}
	return list[rng.Intn(len(list))]
}

func (pg *PlanetGenerator) determineVisualType(climate *Climate, surface, hydrosphere string, temperature float64, rng *rand.Rand) string {
	if surface == "лавовая" {
		return "lava"
	}
	if surface == "ледяная" {
		return "ice"
	}
	if temperature < 200 {
		return "ice"
	}
	if hydrosphere == "океаны" || hydrosphere == "озёра" {
		return "earth"
	}
	return "rocky"
}

func (pg *PlanetGenerator) generateTexture(visualType string, size int, rng *rand.Rand, opts *GenerateOptions, temperature float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			if math.Sqrt(dx*dx+dy*dy) > radius {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}
	switch visualType {
	case "rocky":
		generateRocky(img, size, rng)
	case "earth":
		generateEarth(img, size, rng, temperature)
	case "ice":
		generateIce(img, size, rng)
	case "lava":
		generateLava(img, size, rng)
	case "gas":
		generateGas(img, size, rng)
	default:
		generateRocky(img, size, rng)
	}
	return img
}

// generateKey — ключ кэша по детерминированным входам (seed + options).
// Производные мета-поля (тип, кольца, атмосфера, температура) однозначно
// определяются этими входами, поэтому одинаковый ключ = одинаковый результат.
func generateKey(seed int64, opts *GenerateOptions) string {
	rings := 2
	if opts.HasRings != nil {
		if *opts.HasRings {
			rings = 1
		} else {
			rings = 0
		}
	}
	return fmt.Sprintf("%d|%d|%s|%s|%s|%s|%s|%s|%d",
		seed, opts.Radius, opts.StarType, opts.ClimateID, opts.Surface,
		opts.Hydrosphere, opts.Atmosphere, opts.Biosphere, rings)
}

// legacyPostFX — постобработка старого косметического генератора
// (GeneratePlanet): фиксированный свет (-0.5,-0.4,0.2), тень/спекл по типу,
// атмосферный ободок по флагу — сохранение прежнего вида (спека 2026-09-21
// §4.2: контракт applyPostProcessing заменён на PostFX).
func legacyPostFX(visualType string, hasAtmosphere bool) PostFX {
	L := [3]float64{-0.5, -0.4, 0.2}
	lenL := math.Sqrt(L[0]*L[0] + L[1]*L[1] + L[2]*L[2])
	L = [3]float64{L[0] / lenL, L[1] / lenL, L[2] / lenL}
	shadow := 0.6
	highlight := 0.3
	atmColor := color.RGBA{100, 150, 255, 50}
	switch visualType {
	case "ice":
		shadow = 0.4
		highlight = 0.8
		atmColor = color.RGBA{200, 230, 255, 40}
	case "lava":
		shadow = 0.6
		highlight = 0.2
		atmColor = color.RGBA{255, 100, 50, 60}
	case "earth":
		shadow = 0.6
		highlight = 0.3
		atmColor = color.RGBA{70, 150, 255, 50}
	case "gas":
		shadow = 0.6
		highlight = 0.2
		atmColor = color.RGBA{200, 180, 150, 40}
	}
	fx := PostFX{
		L:              L,
		ShadowStrength: shadow,
		Specular:       &Specular{Pow: 20, Strength: highlight, Color: color.RGBA{255, 255, 255, 230}},
	}
	if hasAtmosphere {
		fx.AtmGlint = &Glint{GlowStrength: 0.2, Haze: atmColor}
	}
	return fx
}
