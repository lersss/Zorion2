// internal/generator/planet/planet_image_v2.go
//
// Честный генератор картинки планеты (спека 2026-09-20): поверхность из
// биомов ({form, share} → пятна с шум-границами, цвет по контракту
// «биом → цвет») + атмосферный слой (дымка/лимб/свечение/облака) +
// заслонение по видимым параметрам. Один генератор на все типоразмеры
// (гейт 2): малая (канвас 256) и большая «вид с орбиты» (512).
//
// Входы — только видимые клиенту параметры (спека §3.1): атмосферные числа
// (composition/pressure_atm/tau_ir/cloudFraction/scale_height_km) НЕ входят —
// инвариант 77a И1 (картинка не раскрывает скрытое: чего нет на входе,
// того нет на картинке). Заглушка (чужая система без знания) — нейтральный
// силуэт по seed, без данных планеты (§4.5).
package planet

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"zorion/internal/models"
)

// ImageMode — режим картинки (спека §3.3): честная (своя система/знание/админ)
// или заглушка (чужая система без знания).
type ImageMode string

const (
	ImageModeHonest ImageMode = "honest"
	ImageModeStub   ImageMode = "stub"
)

// ImageSize — типоразмер (спека §5.1): малая (канвас 256) / большая (512).
type ImageSize string

const (
	ImageSizeSmall ImageSize = "small"
	ImageSizeBig   ImageSize = "big"
)

// PlanetImageInput — входы генератора (спека §3.1): только видимые клиенту
// параметры. Атмосферные числа не входят (И1).
type PlanetImageInput struct {
	PlanetID     string
	Mode         ImageMode
	Size         ImageSize
	Biomes       []models.Biome
	Type         string
	IsGasGiant   bool
	Temperature  float64
	WaterPercent float64
	Habitable    bool
	Life         bool
}

// imageSeed — детерминированный 64-бит seed из planet_id (FNV-1a 64).
// М3 закрыт: коллизии 32-бит хэша клиента уходят (спека §3.1).
func imageSeed(planetID string) int64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(planetID); i++ {
		h ^= uint64(planetID[i])
		h *= 1099511628211
	}
	return int64(h)
}

// hashForm — 64-бит хэш id биома (FNV-1a) для детерминированного RNG на биом.
func hashForm(form string) int64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(form); i++ {
		h ^= uint64(form[i])
		h *= 1099511628211
	}
	return int64(h)
}

// imageCacheKey — кэш-ключ (спека §5.2): hash64(planet_id) + "|" + mode + "|" + size.
// Роль/знание/позиция в ключ не входят — они определяют режим, режим уже в
// ключе. Смена знания (скан) меняет режим → новый ключ; старый вытесняется FIFO.
func imageCacheKey(planetID string, mode ImageMode, size ImageSize) string {
	return fmt.Sprintf("%d|%s|%s", imageSeed(planetID), mode, size)
}

// imageCanvasSize — канвас типоразмера (спека §5.1): small → 256, big → 512.
func imageCanvasSize(size ImageSize) int {
	if size == ImageSizeBig {
		return 512
	}
	return 256
}

// GeneratePlanetImage — один генератор на все типоразмеры (гейт 2): честная
// картинка из биомов + атмосферы или заглушка. Детерминизм: одинаковый
// (планета, режим, размер) → одинаковый PNG (кэш-ключ корректен, М3/М4).
func (pg *PlanetGenerator) GeneratePlanetImage(in PlanetImageInput) (*image.RGBA, error) {
	if in.PlanetID == "" {
		return nil, fmt.Errorf("planet image: пустой planet_id")
	}
	key := imageCacheKey(in.PlanetID, in.Mode, in.Size)
	if pg.enableCache {
		if p, ok := pg.cacheGet(key); ok {
			return p.Image, nil
		}
	}
	size := imageCanvasSize(in.Size)
	var img *image.RGBA
	if in.Mode == ImageModeStub {
		img = generateStub(size, imageSeed(in.PlanetID))
	} else {
		img = pg.generateHonest(in, size)
	}
	if pg.enableCache {
		pg.cachePut(key, &CachedPlanet{Image: img})
	}
	return img, nil
}

// ==================== ЗАСЛОНЕНИЕ (спека §3.4) ====================

// cloudCover — производная облачности из видимых параметров. Атмосферные
// числа не входят (И1): калибровка под режимы 99.2.20 делает заслонение
// «честным» в типичных случаях. Кламп ≤ 0.6 применяется к life-добавке,
// НЕ к гиганту/жаркому (те возвращаются раньше — уточнение псевдокода).
func cloudCover(in PlanetImageInput) float64 {
	if in.IsGasGiant {
		return 1.0 // гигант: полосы, «поверхность» = полосы
	}
	if in.Temperature >= 350 {
		return 0.85 // жаркий: плотная дымка (Венера-режим, 99.2.20)
	}
	var cc float64
	switch {
	case in.Habitable:
		cc = 0.45 // обитаемый: умеренные облака
	case in.Temperature >= 250 && in.Temperature < 350:
		if in.WaterPercent >= 30 {
			cc = 0.40 // умеренный с водой: облака (Земля-режим)
		} else {
			cc = 0.20 // умеренный сухой: лёгкая дымка
		}
	default:
		cc = 0.10 // холодный: тонкая дымка (Марс-режим)
	}
	// life: +0.1, кламп <= 0.6 (жизненный проход — влажность, 99.2.20 §3.6).
	if in.Life {
		cc += 0.1
		if cc > 0.6 {
			cc = 0.6
		}
	}
	return cc
}

// ==================== КОНТРАКТ «БИОМ → ЦВЕТ» (спека §4.2) ====================

// categoryBase — палитра категорий (спека §4.2): база (lo → hi) по категории.
func categoryBase(category string) (lo, hi color.RGBA) {
	switch category {
	case "вода":
		return color.RGBA{0x2a, 0x5a, 0x8a, 255}, color.RGBA{0x4a, 0x9e, 0xff, 255}
	case "биосфера":
		return color.RGBA{0x2a, 0x6a, 0x3a, 255}, color.RGBA{0x7a, 0xc4, 0x7a, 255}
	case "вулканизм":
		return color.RGBA{0x3a, 0x2a, 0x2a, 255}, color.RGBA{0xc0, 0x39, 0x2b, 255}
	case "крио":
		return color.RGBA{0xd0, 0xe8, 0xf2, 255}, color.RGBA{0xf0, 0xf8, 0xff, 255}
	case "экзотика":
		return color.RGBA{0x7a, 0x4a, 0x9a, 255}, color.RGBA{0xc8, 0x6a, 0xe0, 255}
	default: // литосфера
		return color.RGBA{0x8a, 0x7a, 0x6a, 255}, color.RGBA{0xb8, 0xa8, 0x8a, 255}
	}
}

// lerpColor — линейная интерполяция двух цветов по t ∈ [0,1].
func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.RGBA{
		uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		255,
	}
}

// shiftColor — сдвиг каналов (r,g,b) с клампом 0..255.
func shiftColor(c color.RGBA, dr, dg, db int) color.RGBA {
	clamp := func(v int) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	return color.RGBA{clamp(int(c.R) + dr), clamp(int(c.G) + dg), clamp(int(c.B) + db), 255}
}

// parseHexColor — '#RRGGBB' → color.RGBA (ok=false при битом hex).
func parseHexColor(s string) (color.RGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, false
	}
	r, err1 := strconv.ParseUint(s[1:3], 16, 8)
	g, err2 := strconv.ParseUint(s[3:5], 16, 8)
	b, err3 := strconv.ParseUint(s[5:7], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}, true
}

// biomeColor — контракт «биом → цвет» (спека §4.2, гибрид): color из каталога
// переопределяет базу; иначе база категории + сдвиги (альбедо → яркость,
// liquid_medium → оттенок, температура планеты → тепло/холод).
func biomeColor(b *BiomeDef, in PlanetImageInput) color.RGBA {
	if b.Color != "" {
		if c, ok := parseHexColor(b.Color); ok {
			return c
		}
	}
	lo, hi := categoryBase(b.Category)
	albedo := b.Albedo
	if albedo < 0 {
		albedo = 0
	}
	if albedo > 1 {
		albedo = 1
	}
	c := lerpColor(lo, hi, albedo)
	switch b.LiquidMedium {
	case "вода":
		c = shiftColor(c, -5, 5, 25) // синева
	case "метан":
		c = shiftColor(c, -15, 10, 15) // бирюза
	case "аммиак":
		c = shiftColor(c, 20, -5, 10) // розоватость
	case "co2":
		c = shiftColor(c, 15, 10, -10) // желтизна
	}
	if in.Temperature >= 350 {
		c = shiftColor(c, 15, 0, -10) // теплее
	} else if in.Temperature < 250 {
		c = shiftColor(c, -5, 0, 15) // холоднее
	}
	return c
}

// biomeColorFor — цвет биома по id: определение из каталога; неизвестный id
// (каталог правлен после генерации) → фолбэк-цвет по категории из имени,
// иначе нейтральный #8a7a6a (не падать, спека §4.2/§7).
func biomeColorFor(id string, in PlanetImageInput) color.RGBA {
	if b := GetBiomeCatalog().BiomeByID(id); b != nil {
		return biomeColor(b, in)
	}
	lo, hi := categoryBase(categoryFromID(id))
	return lerpColor(lo, hi, 0.5)
}

// categoryFromID — категория неизвестного биома по ключевым словам имени
// (фолбэк §4.2: «по категории из type_tags/имени»; type_tags нет — имя/id).
func categoryFromID(id string) string {
	switch {
	case strings.Contains(id, "лед"), strings.Contains(id, "крио"), strings.Contains(id, "иней"),
		strings.Contains(id, "мёрзл"), strings.Contains(id, "тундр"):
		return "крио"
	case strings.Contains(id, "лав"), strings.Contains(id, "вулкан"), strings.Contains(id, "магм"),
		strings.Contains(id, "обсидиан"), strings.Contains(id, "серн"):
		return "вулканизм"
	case strings.Contains(id, "океан"), strings.Contains(id, "озёр"), strings.Contains(id, "мор"),
		strings.Contains(id, "вод"), strings.Contains(id, "рек"), strings.Contains(id, "мелковод"),
		strings.Contains(id, "планктон"):
		return "вода"
	case strings.Contains(id, "лес"), strings.Contains(id, "луг"), strings.Contains(id, "степ"),
		strings.Contains(id, "джунгл"), strings.Contains(id, "болот"), strings.Contains(id, "рощ"),
		strings.Contains(id, "сад"), strings.Contains(id, "чащ"), strings.Contains(id, "риф"),
		strings.Contains(id, "трав"):
		return "биосфера"
	case strings.Contains(id, "метан"), strings.Contains(id, "аммиак"), strings.Contains(id, "кремн"),
		strings.Contains(id, "кристал"), strings.Contains(id, "струн"), strings.Contains(id, "светящ"),
		strings.Contains(id, "терминатор"), strings.Contains(id, "химическ"), strings.Contains(id, "радиацион"),
		strings.Contains(id, "углеводород"), strings.Contains(id, "карбид"):
		return "экзотика"
	default:
		return "литосфера"
	}
}

// ==================== АЛГОРИТМ «SHARE → ПЯТНА» (спека §4.3) ====================

// blob — пятно биома: центр, радиус, id биома.
type blob struct {
	cx, cy, r float64
	id        string
}

// blobs — посев пятен от крупных к мелким: сортировка по доле, крупные
// сеются первыми, мелкие втискиваются; RNG на биом — детерминированный
// (seed XOR hash(form)). Минимальный биом (share ≥ 1%) всегда получает ≥ 1
// пятно. Доли приближённые — картинка «вид, не отчёт» (С1/С4).
//
// R_base — калибровка (спека §4.3 оставляет его неопределённым): 0.5 радиуса
// диска — суммарная площадь пятен ≈ площади диска (типичный состав), каждый
// биом состава виден; при R_base = радиусу диска крупнейший биом (share ≥ 35%)
// покрывал бы весь диск (перекрытие ~3×) — мелкие биомы не видны.
func blobs(seed int64, biomes []models.Biome, canvasSize int) []blob {
	K := float64(canvasSize) / 16
	if K < 8 {
		K = 8
	}
	sorted := make([]models.Biome, len(biomes))
	copy(sorted, biomes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Share > sorted[j].Share })

	R := (float64(canvasSize)/2 - 2) * 0.5
	var out []blob
	for _, b := range sorted {
		rng := rand.New(rand.NewSource(seed ^ hashForm(b.Form)))
		k := int(math.Round(b.Share / 100 * K))
		if k < 1 {
			k = 1
		}
		for j := 0; j < k; j++ {
			ang := rng.Float64() * 2 * math.Pi
			rad := math.Sqrt(rng.Float64()) * (float64(canvasSize)/2 - 2)
			cx := float64(canvasSize)/2 + math.Cos(ang)*rad
			cy := float64(canvasSize)/2 + math.Sin(ang)*rad
			r := R * math.Sqrt(b.Share/100) * (0.5 + rng.Float64()*0.5)
			out = append(out, blob{cx: cx, cy: cy, r: r, id: b.Form})
		}
	}
	return out
}

// dominantBiome — биом с максимальной долей (остаток незанятых пикселей).
func dominantBiome(biomes []models.Biome) string {
	best := ""
	bestShare := -1.0
	for _, b := range biomes {
		if b.Share > bestShare {
			best = b.Form
			bestShare = b.Share
		}
	}
	return best
}

// ==================== ЧЕСТНАЯ КАРТИНКА (спека §4) ====================

// generateHonest — честная картинка: поверхность из биомов (или фолбэк по
// visualType для старых миров/гигантов, §7) + рельеф + атмосферный слой +
// заслонение + постобработка + кольца.
func (pg *PlanetGenerator) generateHonest(in PlanetImageInput, size int) *image.RGBA {
	seed := imageSeed(in.PlanetID)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	clearDisk(img, size)

	cc := cloudCover(in)

	if in.IsGasGiant {
		// Гигант: полосы (существующий generateGas) + cloudCover = 1.0 —
		// «поверхность» гиганта = полосы (спека §7, консистентно).
		rng := rand.New(rand.NewSource(seed))
		generateGas(img, size, rng)
	} else if len(in.Biomes) > 0 {
		generateBiomeSurface(img, size, seed, in)
	} else {
		// Старый мир без biomes (до 99.2.28): текущий генератор по visualType
		// (спека §7) + атмосферный слой ниже.
		rng := rand.New(rand.NewSource(seed))
		generateTextureByType(img, size, rng, determineVisualTypeFromInput(in), in.Temperature)
	}

	applyAtmosphereLayer(img, size, in, cc, seed)

	// Постобработка: тень/блик/терминатор (существующий applyPostProcessing);
	// атмосферный «ободок» по флагу заменён слоем 4 — hasAtmosphere=false.
	rng := rand.New(rand.NewSource(seed))
	applyPostProcessing(img, size, postVisualType(in), false, rng)

	// Кольца (существующий drawRings): для гигантов по seed; в заглушке нет.
	if hasRingsBySeed(seed, in.IsGasGiant) {
		drawRings(img, size, rng)
	}
	return img
}

// clearDisk — прозрачный фон вне диска планеты.
func clearDisk(img *image.RGBA, size int) {
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
}

// generateBiomeSurface — поверхность из биомов (спека §4.1 слои 1–3): пятна
// по {form, share} с шум-границами, цвет по контракту «биом → цвет», рельеф
// fbm поверх (лёгкая модуляция яркости), ridged для «гористых» категорий
// (литосфера/вулканизм).
func generateBiomeSurface(img *image.RGBA, size int, seed int64, in PlanetImageInput) {
	noise := newValueNoise(seed)
	bl := blobs(seed, in.Biomes, size)
	dominant := dominantBiome(in.Biomes)

	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius {
				continue
			}
			// Шум-границы пятен + рельеф — один сэмпл fbm (детерминизм).
			n := noise.fbm(dx/radius*3, dy/radius*3, 0, 3, 2.0, 0.5)
			form := dominant
			for _, b := range bl {
				// Расстояние до ЦЕНТРА пятна (не диска): пиксель внутри пятна,
				// граница искажена fbm (спека §4.3).
				bdx := float64(x) - b.cx
				bdy := float64(y) - b.cy
				if math.Sqrt(bdx*bdx+bdy*bdy) < b.r*(1+0.3*n) {
					form = b.id
					break
				}
			}
			c := biomeColorFor(form, in)
			// Рельеф: лёгкая модуляция яркости fbm.
			bright := 0.88 + 0.24*n
			// ridged для «гористых» категорий (литосфера/вулканизм).
			if cat := categoryOf(form); cat == "литосфера" || cat == "вулканизм" {
				rn := noise.ridged(dx/radius*3, dy/radius*3, 0, 3, 2.0, 0.5)
				bright += (rn - 0.5) * 0.15
			}
			img.SetRGBA(x, y, scaleBrightness(c, bright))
		}
	}
}

// categoryOf — категория биома по id (каталог или фолбэк по имени).
func categoryOf(id string) string {
	if b := GetBiomeCatalog().BiomeByID(id); b != nil {
		return b.Category
	}
	return categoryFromID(id)
}

// scaleBrightness — умножение яркости цвета (0..1) с клампом каналов.
func scaleBrightness(c color.RGBA, k float64) color.RGBA {
	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	return color.RGBA{clamp(float64(c.R) * k), clamp(float64(c.G) * k), clamp(float64(c.B) * k), 255}
}

// determineVisualTypeFromInput — фолбэк-тип визуала для старых миров без
// биомов (спека §7): из type/temperature/water (как determineVisualType).
func determineVisualTypeFromInput(in PlanetImageInput) string {
	if in.IsGasGiant {
		return "gas"
	}
	t := strings.ToLower(in.Type)
	if strings.Contains(t, "вулкан") || strings.Contains(t, "лав") {
		return "lava"
	}
	if in.Temperature < 200 || strings.Contains(t, "лед") || strings.Contains(t, "крио") {
		return "ice"
	}
	if in.WaterPercent >= 30 {
		return "earth"
	}
	return "rocky"
}

// generateTextureByType — существующие генераторы текстур по visualType
// (фолбэк старых миров, спека §7).
func generateTextureByType(img *image.RGBA, size int, rng *rand.Rand, visualType string, temperature float64) {
	switch visualType {
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
}

// postVisualType — тип для постобработки (тень/блик): из видимых параметров.
func postVisualType(in PlanetImageInput) string {
	if in.IsGasGiant {
		return "gas"
	}
	t := strings.ToLower(in.Type)
	if strings.Contains(t, "вулкан") || strings.Contains(t, "лав") || in.Temperature >= 350 {
		return "lava"
	}
	if in.Temperature < 250 || strings.Contains(t, "лед") || strings.Contains(t, "крио") {
		return "ice"
	}
	return "earth"
}

// hasRingsBySeed — кольца по seed (существующая фича): гиганты 40%, остальные
// 5% (как в старом генераторе, детерминизм по seed).
func hasRingsBySeed(seed int64, isGasGiant bool) bool {
	rng := rand.New(rand.NewSource(seed))
	if isGasGiant {
		return rng.Float64() < 0.4
	}
	return rng.Float64() < 0.05
}

// ==================== АТМОСФЕРНЫЙ СЛОЙ (спека §4.4) ====================

// hazeColor — цвет дымки по типу и температуре (спека §4.4, палитра типов).
func hazeColor(in PlanetImageInput) color.RGBA {
	t := strings.ToLower(in.Type)
	switch {
	case in.IsGasGiant:
		return color.RGBA{0xd8, 0xc8, 0xa8, 255}
	case in.Temperature >= 350 || strings.Contains(t, "вулкан") || strings.Contains(t, "лав"):
		return color.RGBA{0xe8, 0xc8, 0x78, 255}
	case in.WaterPercent >= 30 || in.Life || in.Habitable:
		return color.RGBA{0x7a, 0xb8, 0xe8, 255}
	case in.Temperature < 250 || strings.Contains(t, "лед") || strings.Contains(t, "крио"):
		return color.RGBA{0xc8, 0xdc, 0xe8, 255}
	case strings.Contains(t, "пустын"):
		return color.RGBA{0xd8, 0xc8, 0xa8, 255}
	default:
		return color.RGBA{0xd8, 0xc8, 0xa8, 255}
	}
}

// blend — смешение цвета с наложением (alpha 0..1).
func blend(base, over color.RGBA, alpha float64) color.RGBA {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	return color.RGBA{
		uint8(float64(base.R)*(1-alpha) + float64(over.R)*alpha),
		uint8(float64(base.G)*(1-alpha) + float64(over.G)*alpha),
		uint8(float64(base.B)*(1-alpha) + float64(over.B)*alpha),
		255,
	}
}

// applyAtmosphereLayer — атмосферный слой (спека §4.4): дымка (палитра по
// типу), лимб (толщина по cloudCover), свечение (по типу), облака (по
// cloudCover, ступени §3.4: полное/частичное/дымка).
func applyAtmosphereLayer(img *image.RGBA, size int, in PlanetImageInput, cc float64, seed int64) {
	haze := hazeColor(in)
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	noise := newValueNoise(seed)

	// Лимб: толщина 1 + cc*4 px (канвас 512), альфа 0.3–0.6 по cloudCover.
	limbPx := 1 + cc*4
	if size < 512 {
		limbPx = limbPx * float64(size) / 512
	}
	limbAlpha := 0.3 + 0.3*cc

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius+limbPx {
				continue
			}
			c := img.RGBAAt(x, y)

			// Дымка: лёгкая заливка по диску (альфа растёт с облачностью).
			if dist <= radius {
				hazeA := 0.06 + 0.10*cc
				if in.IsGasGiant {
					hazeA = 0.04
				}
				c = blend(c, haze, hazeA)
			}

			// Лимб: кольцо за краем диска.
			if dist > radius && dist <= radius+limbPx {
				t := (dist - radius) / limbPx
				c = blend(c, haze, limbAlpha*(1-t))
			}

			// Облака (ступени §3.4); у гиганта «поверхность» = полосы — покров
			// не добавляем (спека §7).
			if dist <= radius && !in.IsGasGiant {
				n := noise.fbm(dx/radius*2, dy/radius*2, 0, 4, 2.0, 0.5)
				switch {
				case cc >= 0.7:
					// Полное заслонение: облачный покров вместо поверхности.
					if n > 0.25 {
						c = blend(c, haze, 0.9)
					} else {
						c = blend(c, haze, 0.7)
					}
				case cc >= 0.35:
					// Частичное: пятна облаков, доля ~cc.
					if n > 1-cc {
						c = blend(c, haze, 0.75)
					}
				}
			}

			img.SetRGBA(x, y, c)
		}
	}

	// Свечение (glowBoost): тёплый ореол у вулканических/жарких.
	if in.Temperature >= 350 || strings.Contains(strings.ToLower(in.Type), "вулкан") ||
		strings.Contains(strings.ToLower(in.Type), "лав") {
		glow := color.RGBA{0xff, 0xc8, 0x78, 255}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx := float64(x) - cx
				dy := float64(y) - cy
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist > radius && dist <= radius+limbPx*2 {
					t := (dist - radius) / (limbPx * 2)
					c := img.RGBAAt(x, y)
					img.SetRGBA(x, y, blend(c, glow, 0.25*(1-t)))
				}
			}
		}
	}
}

// ==================== ЗАГЛУШКА (спека §4.5) ====================

// generateStub — нейтральный силуэт: градиентная заливка (тёмно-серый →
// серо-голубой), лёгкая полосатость (детерминированный fbm, малая амплитуда
// — не читается как тип), тень/блик (постобработка без атмосферы), колец нет.
// Входы — только planet_id (seed) + size: не раскрывает тип/поверхность/
// атмосферу/кольца.
func generateStub(size int, seed int64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	clearDisk(img, size)
	noise := newValueNoise(seed)
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius {
				continue
			}
			// Градиент тёмно-серый → серо-голубой.
			base := lerpColor(color.RGBA{0x3c, 0x3e, 0x46, 255}, color.RGBA{0x6e, 0x76, 0x82, 255}, dist/radius)
			// Лёгкая полосатость (малая амплитуда ±8).
			n := noise.fbm(dx/radius*3, dy/radius*3, 0, 3, 2.0, 0.5)
			band := int((n - 0.5) * 16)
			img.SetRGBA(x, y, shiftColor(base, band, band, band))
		}
	}
	// Тень/блик (существующая постобработка без атмосферы), колец нет.
	rng := rand.New(rand.NewSource(seed))
	applyPostProcessing(img, size, "rocky", false, rng)
	return img
}