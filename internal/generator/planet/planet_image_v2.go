// internal/generator/planet/planet_image_v2.go
//
// Честный генератор картинки планеты (спеки 2026-09-20 + 2026-09-21):
// поверхность из биомов ({form, share} → «поле высот + регионы», цвет по
// контракту «биом → цвет») + атмосферный слой (дымка/лимб/свечение/облака) +
// заслонение. Один генератор на все типоразмеры (гейт 2): малая (канвас 256)
// и большая «вид с орбиты» (512).
//
// Три режима (спека §3.3): stub — нейтральный силуэт по seed, без данных
// планеты (§4.5); honest — видимые клиенту параметры, производная атмосфера
// (инвариант 77a И1: знание атмосферу не содержит — скрытые числа не входят);
// full — присутствие/админ, реальная атмосфера (состав/давление/τ_IR/высота,
// контракт C).
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

// ImageMode — режим картинки (спека §3.3): stub (заглушка, чужая система без
// знания), honest (знание — производная атмосфера), full (присутствие/админ —
// реальная атмосфера).
type ImageMode string

const (
	ImageModeHonest ImageMode = "honest"
	ImageModeFull   ImageMode = "full"
	ImageModeStub   ImageMode = "stub"
)

// ImageGenVersion — версия генератора картинки (спека §5.2, S1): инкремент
// при ЛЮБОЙ визуальной правке генератора (форма/свет/атмосфера/заглушка).
// Входит в in-memory кэш-ключ и в префикс диск-кэша; стартовый клин удаляет
// файлы не с текущим префиксом (включая легаси без префикса, M1).
const ImageGenVersion = "v3"

// ImageSize — типоразмер (спека §5.1): малая (канвас 256) / большая (512).
type ImageSize string

const (
	ImageSizeSmall ImageSize = "small"
	ImageSizeBig   ImageSize = "big"
)

// PlanetImageInput — входы генератора (спека §3.1): видимые клиенту параметры
// + атмосферные поля (только режим full, контракт C). В honest атмосферные
// числа не входят (И1: знание атмосферу не содержит — производная).
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
	// Атмосферные поля — только full (спека §3.1): состав (газ → %, сумма 100),
	// давление (атм), τ_IR, масштабная высота (км).
	Composition   map[string]float64
	PressureAtm   float64
	TauIR         float64
	ScaleHeightKm float64
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

// imageCacheKey — кэш-ключ (спека §5.2, K1/S1): hash64(planet_id) + "|" + mode
// + "|" + size + "|" + imageGenVersion. Роль/знание/позиция в ключ не входят —
// они определяют режим, режим уже в ключе. Версия генератора — визуальная
// правка не хитятся старые кэши (in-memory и диск).
func imageCacheKey(planetID string, mode ImageMode, size ImageSize) string {
	return fmt.Sprintf("%d|%s|%s|%s", imageSeed(planetID), mode, size, ImageGenVersion)
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

// ==================== ФОРМА ПОВЕРХНОСТИ: «ПОЛЕ ВЫСОТ + РЕГИОНЫ» (спека §4.1) ====================

// region — регион биома на сфере: центр (3D, масштаб scale), радиус (3D),
// id биома.
type region struct {
	cx, cy, cz, r float64
	id            string
}

// elevationAt — поле высот E(p) (спека §4.1 шаг 1): fbm 3 октавы на сфере
// (scale — циклов на диаметр), доменный варп 12% — береговая линия хаотичная.
func elevationAt(noise *valueNoise, c [3]float64, scale float64) float64 {
	wx, wy, wz := noise.warp(c[0]*scale, c[1]*scale, c[2]*scale, 0.3)
	return noise.fbm(wx, wy, wz, 3, 2.0, 0.5)
}

// elevationZone — зона высот точки сферы (спека §4.1 шаг 2): низ E < 0.35,
// середина 0.35–0.65, верх E > 0.65; полюса |z| > 0.6 (z — компонента
// точки сферы).
func elevationZone(e, z float64) string {
	if math.Abs(z) > 0.6 {
		return "полюс"
	}
	switch {
	case e < 0.35:
		return "низ"
	case e <= 0.65:
		return "середина"
	default:
		return "верх"
	}
}

// biomeZones — зоны размещения биома по категории (спека §4.1 шаг 3):
// вода → низ; крио → верх + полюса; вулканизм → верх;
// биосфера/литосфера/экзотика → середина.
func biomeZones(category string) []string {
	switch category {
	case "вода":
		return []string{"низ"}
	case "крио":
		return []string{"верх", "полюс"}
	case "вулканизм":
		return []string{"верх"}
	default:
		return []string{"середина"}
	}
}

func zoneAllowed(zone string, zones []string) bool {
	for _, z := range zones {
		if z == zone {
			return true
		}
	}
	return false
}

// randomSpherePoint — равномерная случайная точка единичной сферы.
func randomSpherePoint(rng *rand.Rand) [3]float64 {
	z := rng.Float64()*2 - 1
	a := rng.Float64() * 2 * math.Pi
	r := math.Sqrt(1 - z*z)
	return [3]float64{r * math.Cos(a), r * math.Sin(a), z}
}

// surfaceRegions — «поле высот + регионы» (спека §4.1, контракт A): замена
// blobs(). Низкочастотный fbm E(p) даёт 2–5 крупных масс; внутри — регионы
// биомов по зонам высот (байас размещения); мелкие биомы (share < 5%) —
// 4–6 вкраплений. Радиус региона компенсирует перспективное сокращение
// (проекция сферической шапки на диск: площадь × cos β); перекрытия
// отбрасываются при размещении — бюджет площади безусловный (S2/C1, зоны
// площадь не ограничивают).
func surfaceRegions(seed int64, biomes []models.Biome) []region {
	rng := rand.New(rand.NewSource(seed ^ 0xA7A7))
	sorted := make([]models.Biome, len(biomes))
	copy(sorted, biomes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Share > sorted[j].Share })

	noise := newValueNoise(seed)
	scale := 1.3

	var out []region
	for _, b := range sorted {
		n := int(math.Round(b.Share / 5))
		if n < 1 {
			n = 1
		}
		if n > 6 {
			n = 6
		}
		if b.Share < 5 {
			n = 4 + rng.Intn(3) // 4–6 вкраплений
		}
		for j := 0; j < n; j++ {
			c := placeRegion(rng, noise, b, scale)
			out = append(out, region{cx: c[0], cy: c[1], cz: c[2], r: regionRadius(b, n, rng, c, scale), id: b.Form})
		}
	}
	return out
}

// placeRegion — центр региона: случайная точка сферы с мягким зона-байасом
// (контракт A: зона — байас размещения, бюджет безусловный — зоны площадь
// не ограничивают). Точка в зоне принимается всегда, вне зоны — с
// вероятностью 0.35 (регионы не кластеризуются в узких зонах). Крио: |z| > 0.5.
func placeRegion(rng *rand.Rand, noise *valueNoise, b models.Biome, scale float64) [3]float64 {
	cat := categoryOf(b.Form)
	zones := biomeZones(cat)
	for attempt := 0; attempt < 32; attempt++ {
		c := randomSpherePoint(rng)
		e := elevationAt(noise, c, scale)
		inZone := zoneAllowed(elevationZone(e, c[2]), zones)
		if cat == "крио" && math.Abs(c[2]) <= 0.5 {
			inZone = false
		}
		if !inZone && rng.Float64() > 0.35 {
			continue
		}
		return [3]float64{c[0] * scale, c[1] * scale, c[2] * scale}
	}
	c := randomSpherePoint(rng)
	return [3]float64{c[0] * scale, c[1] * scale, c[2] * scale}
}

// regionRadius — радиус региона в пространстве сферы (масштаб scale):
// компенсация перспективного сокращения — проекция шапки на диск
// ≈ share/n диска (кламп у лимба). Взвешенный Вороной даёт клетку площади
// ∝ r² — бюджет долей точен (S2/C1).
func regionRadius(b models.Biome, n int, rng *rand.Rand, c [3]float64, scale float64) float64 {
	beta := math.Acos(c[2] / scale) // угловое расстояние от оси наблюдения
	k := 1.0
	if cosBeta := math.Cos(beta); cosBeta > 0.25 {
		k = 1 / math.Sqrt(cosBeta)
	} else {
		k = 2.0
	}
	return scale * math.Sqrt(b.Share/100/float64(n)) * (0.9 + 0.2*rng.Float64()) * k
}

// ==================== ЧЕСТНАЯ КАРТИНКА (спека §4) ====================

// generateHonest — честная картинка: поверхность из биомов (или фолбэк по
// visualType для старых миров/гигантов, §7) + рельеф + атмосферный слой
// (honest — производная, full — реальная, §4.3/§4.5) + заслонение +
// постобработка от L(seed) + освещённые кольца.
func (pg *PlanetGenerator) generateHonest(in PlanetImageInput, size int) *image.RGBA {
	seed := imageSeed(in.PlanetID)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	clearDisk(img, size)

	cc := cloudCover(in)
	ap := atmosphereParamsFor(in, cc)

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

	applyAtmosphereLayer(img, size, ap, in, seed)

	// Постобработка: тень/терминатор/спекл/ореол от единого L(seed)
	// (спека §4.2, контракт B/M3).
	rng := rand.New(rand.NewSource(seed))
	L := lightVector(seed)
	fx := postFXFor(in, L, ap, rng)
	applyPostProcessing(img, size, fx, rng)

	// Кольца: освещены от L (S3); в заглушке колец нет.
	if hasRingsBySeed(seed, in.IsGasGiant) {
		drawRings(img, size, rng, L, ap.haze, 0.15)
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

// generateBiomeSurface — поверхность из биомов (спека §4.1, контракт A):
// «поле высот + регионы» — регионы биомов по зонам высот (мягкий байас),
// растеризация на сфере (spherePoint): первый регион в порядке убывания share,
// где dist3D(p, c) < r·(1+0.3·n) — мягкие границы fbm; остаток → доминирующий
// биом. Рельеф: лёгкая модуляция яркости fbm, ridged для «гористых» категорий.
func generateBiomeSurface(img *image.RGBA, size int, seed int64, in PlanetImageInput) {
	noise := newValueNoise(seed)
	regs := surfaceRegions(seed, in.Biomes)

	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	scale := 1.3

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius {
				continue
			}
			px, py, pz := spherePoint(dx, dy, radius, scale)
			// Поле высот: границы регионов + яркость (один сэмпл, детерминизм).
			n := elevationAt(noise, [3]float64{px, py, pz}, 1.0)
			form := ""
			found := false
			for _, r := range regs {
				bdx := px - r.cx
				bdy := py - r.cy
				bdz := pz - r.cz
				if math.Sqrt(bdx*bdx+bdy*bdy+bdz*bdz) < r.r*(1+0.3*n) {
					form = r.id
					found = true
					break
				}
			}
			if !found {
				// Остаток (зазоры между регионами) → ближайший регион:
				// распределяется пропорционально долям (бюджет S2/C1).
				bestD := math.MaxFloat64
				for _, r := range regs {
					bdx := px - r.cx
					bdy := py - r.cy
					bdz := pz - r.cz
					if d := bdx*bdx + bdy*bdy + bdz*bdz; d < bestD {
						bestD = d
						form = r.id
					}
				}
			}
			c := biomeColorFor(form, in)
			// Рельеф: лёгкая модуляция яркости fbm.
			bright := 0.88 + 0.24*n
			// ridged для «гористых» категорий (литосфера/вулканизм).
			if cat := categoryOf(form); cat == "литосфера" || cat == "вулканизм" {
				rn := noise.ridged(px, py, pz, 3, 2.0, 0.5)
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

// ==================== АТМОСФЕРНЫЙ СЛОЙ (спека §4.3/§4.4) ====================

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

// gasTints — тинты газов (контракт C, спека §4.3): фиксированный порядок —
// детерминированное суммирование (M4: итерация map рандомизирована).
var gasTints = []struct {
	gas   string
	color color.RGBA
}{
	{"H2", color.RGBA{0xc8, 0xcc, 0xd8, 255}},
	{"He", color.RGBA{0xd8, 0xdc, 0xe4, 255}},
	{"N2", color.RGBA{0xb8, 0xc8, 0xdc, 255}},
	{"O2", color.RGBA{0xa8, 0xc0, 0xe8, 255}},
	{"CO2", color.RGBA{0xe0, 0xc8, 0x98, 255}},
	{"CH4", color.RGBA{0xa8, 0xcc, 0xd8, 255}},
	{"H2O", color.RGBA{0xc0, 0xd8, 0xe8, 255}},
	{"SO2", color.RGBA{0xe8, 0xd0, 0xa8, 255}},
	{"NH3", color.RGBA{0xd0, 0xc0, 0xe0, 255}},
	{"H2S", color.RGBA{0xe0, 0xd8, 0xb0, 255}},
	{"N2O", color.RGBA{0xb8, 0xd0, 0xe0, 255}},
	{"Ar", color.RGBA{0xd8, 0xd8, 0xd8, 255}},
}

func knownGas(gas string) bool {
	for _, gt := range gasTints {
		if gt.gas == gas {
			return true
		}
	}
	return false
}

// realAtmosphere — реальная атмосфера (спека §4.3, контракт C, режим full):
// состав → цвет дымки, давление → лимб, τ_IR → свечение, cloudFraction →
// облака. compPct — проценты (сумма 100); перед classifyAtmosphere —
// нормализация /100 (C2). Фолбэки §7: P ≤ 0 → лимб 1px/0.08, дымка 0.03,
// свечение 0, облака 0; tau ≤ 0 → свечение 0; SH ≤ 0 → 8.
func realAtmosphere(compPct map[string]float64, P, tau, SH float64) (haze color.RGBA, hazeAlpha, limbPx, limbAlpha, glowStrength float64, glowColor color.RGBA, fc float64) {
	if SH <= 0 {
		SH = 8
	}
	// Цвет дымки: Σ(тинт[газ]·compPct[газ])/100, неизвестный газ → #cccccc.
	// Детерминированное суммирование по фиксированному списку газов (M4).
	var r, g, b, total float64
	for _, gt := range gasTints {
		if v, ok := compPct[gt.gas]; ok {
			r += float64(gt.color.R) * v
			g += float64(gt.color.G) * v
			b += float64(gt.color.B) * v
			total += v
		}
	}
	var unknown []string
	for k := range compPct {
		if !knownGas(k) {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		r += 0xcc * compPct[k]
		g += 0xcc * compPct[k]
		b += 0xcc * compPct[k]
		total += compPct[k]
	}
	if total <= 0 {
		total = 1
	}
	hazeRGB := color.RGBA{uint8(r / total), uint8(g / total), uint8(b / total), 255}
	haze = lerpColor(hazeRGB, color.RGBA{255, 255, 255, 255}, 0.35)

	// Фолбэк P ≤ 0: лимб 1px/0.08, дымка 0.03, свечение 0, облака 0 (§7).
	if P <= 0 {
		return haze, 0.03, 1, 0.08, 0, haze, 0
	}
	hazeAlpha = clamp(0.05+0.20*math.Log10(1+P), 0.03, 0.45)
	limbPx = clamp(1+2.5*math.Log10(1+P), 1, 12) * math.Sqrt(SH/8)
	limbAlpha = clamp(0.12+0.22*math.Log10(1+P), 0.08, 0.65)
	glowStrength = clamp(tau/4, 0, 1)
	glowColor = haze
	// Облака: нормализация pct → доли перед classifyAtmosphere (C2).
	compFrac := make(map[string]float64, len(compPct))
	for k, v := range compPct {
		compFrac[k] = v / 100
	}
	fc = cloudFraction(classifyAtmosphere(compFrac, P), P)
	return haze, hazeAlpha, limbPx, limbAlpha, glowStrength, glowColor, fc
}

// atmosphereParams — параметры атмосферного слоя (дымка/лимб/свечение/облака).
type atmosphereParams struct {
	haze         color.RGBA
	hazeAlpha    float64
	limbPx       float64
	limbAlpha    float64
	glowStrength float64
	glowAlpha    float64
	glowColor    color.RGBA
	fc           float64
}

// atmosphereParamsFor — параметры атмосферного слоя по режиму (спека §4.3/§4.5):
// full — реальная атмосфера (контракт C); honest — производная (палитра по
// типу + cloudCover из видимых); full без atmosphere_data — фолбэк §7
// (производная, как honest).
func atmosphereParamsFor(in PlanetImageInput, cc float64) atmosphereParams {
	if in.Mode == ImageModeFull && len(in.Composition) > 0 {
		haze, hazeAlpha, limbPx, limbAlpha, glow, glowColor, fc := realAtmosphere(in.Composition, in.PressureAtm, in.TauIR, in.ScaleHeightKm)
		return atmosphereParams{haze: haze, hazeAlpha: hazeAlpha, limbPx: limbPx, limbAlpha: limbAlpha,
			glowStrength: glow, glowAlpha: 0.25 * glow, glowColor: glowColor, fc: fc}
	}
	// Производная (honest; full без данных — фолбэк §7).
	haze := hazeColor(in)
	limbPx := 1 + cc*4
	limbAlpha := 0.3 + 0.3*cc
	glowStrength, glowAlpha := 0.0, 0.0
	glowColor := haze
	if in.Temperature >= 350 || strings.Contains(strings.ToLower(in.Type), "вулкан") ||
		strings.Contains(strings.ToLower(in.Type), "лав") {
		glowStrength = 1.0
		glowAlpha = 0.25
		glowColor = color.RGBA{0xff, 0xc8, 0x78, 255}
	}
	return atmosphereParams{haze: haze, hazeAlpha: 0.06 + 0.10*cc, limbPx: limbPx, limbAlpha: limbAlpha,
		glowStrength: glowStrength, glowAlpha: glowAlpha, glowColor: glowColor, fc: cc}
}

// applyAtmosphereLayer — атмосферный слой (спека §4.3/§4.4): дымка, лимб,
// свечение, облака (ступени §3.4: полное/частичное/дымка). У гиганта
// «поверхность» = полосы — покров не добавляем (спека §7).
func applyAtmosphereLayer(img *image.RGBA, size int, ap atmosphereParams, in PlanetImageInput, seed int64) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	noise := newValueNoise(seed)

	limbPx := ap.limbPx
	if size < 512 {
		limbPx = limbPx * float64(size) / 512
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius+limbPx {
				continue
			}
			c := img.RGBAAt(x, y)

			// Дымка: лёгкая заливка по диску.
			if dist <= radius {
				hazeA := ap.hazeAlpha
				if in.IsGasGiant && in.Mode != ImageModeFull {
					hazeA = 0.04
				}
				c = blend(c, ap.haze, hazeA)
			}

			// Лимб: кольцо за краем диска.
			if dist > radius && dist <= radius+limbPx {
				t := (dist - radius) / limbPx
				c = blend(c, ap.haze, ap.limbAlpha*(1-t))
			}

			// Облака (ступени §3.4); у гиганта «поверхность» = полосы — покров
			// не добавляем (спека §7).
			if dist <= radius && !in.IsGasGiant {
				n := noise.fbm(dx/radius*2, dy/radius*2, 0, 4, 2.0, 0.5)
				switch {
				case ap.fc >= 0.7:
					// Полное заслонение: облачный покров вместо поверхности.
					if n > 0.25 {
						c = blend(c, ap.haze, 0.9)
					} else {
						c = blend(c, ap.haze, 0.7)
					}
				case ap.fc >= 0.35:
					// Частичное: пятна облаков, доля ~fc.
					if n > 1-ap.fc {
						c = blend(c, ap.haze, 0.75)
					}
				}
			}

			img.SetRGBA(x, y, c)
		}
	}

	// Свечение (glowBoost): ореол за краем диска, цвет = дымка.
	if ap.glowStrength > 0 {
		glowWidth := limbPx * (1 + ap.glowStrength)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx := float64(x) - cx
				dy := float64(y) - cy
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist > radius && dist <= radius+glowWidth {
					t := (dist - radius) / glowWidth
					c := img.RGBAAt(x, y)
					img.SetRGBA(x, y, blend(c, ap.glowColor, ap.glowAlpha*(1-t)))
				}
			}
		}
	}
}

// ==================== ЗАГЛУШКА (спека §4.5, контракт D) ====================

// generateStub — нейтральный силуэт: градиентная заливка (тёмно-серый →
// серо-голубой), лёгкая полосатость (детерминированный fbm, малая амплитуда
// — не читается как тип), мягкий терминатор от L(seed) (тень 0.4), без
// спекл-блика и свечения (M4), колец нет. Входы — только planet_id (seed) +
// size: не раскрывает тип/поверхность/атмосферу/кольца.
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
	// Мягкий терминатор от L(seed), без спекла и свечения (контракт D, M4).
	rng := rand.New(rand.NewSource(seed))
	L := lightVector(seed)
	applyPostProcessing(img, size, PostFX{L: L, ShadowStrength: 0.4}, rng)
	return img
}

// ==================== СВЕТ: ЕДИНЫЙ L(SEED) (спека §4.2, контракт B) ====================

// lightVector — единый световой вектор L от seed: az = 2π·rng,
// el = 20°+40°·rng (20–60° над плоскостью). Терминатор/тень/спекл/кольца —
// от одного L (детерминизм, разнообразие по seed).
func lightVector(seed int64) [3]float64 {
	rng := rand.New(rand.NewSource(seed ^ 0x11CE))
	az := 2 * math.Pi * rng.Float64()
	el := (20 + 40*rng.Float64()) * math.Pi / 180
	return [3]float64{math.Cos(el) * math.Cos(az), math.Cos(el) * math.Sin(az), math.Sin(el)}
}

// specIntensity — сила спекла: pow(max(0, 2·diffuse·normDz − nz), pow)·сила·2,
// кламп ≤ 0.25 (было 0.8 — приглушён в 3–5 раз, контракт B).
func specIntensity(diffuse, normDz, nz float64, s *Specular) float64 {
	spec := math.Max(0, 2*diffuse*normDz-nz)
	si := math.Pow(spec, s.Pow) * s.Strength * 2
	if si > 0.25 {
		si = 0.25
	}
	return si
}

// specularForType — спекл по типу (спека §4.2, таблица): сила в диапазоне
// (rng — детерминизм от seed), цвет по типу.
func specularForType(vt string, rng *rand.Rand) *Specular {
	switch vt {
	case "gas":
		return &Specular{Pow: 8, Strength: 0.10 + 0.08*rng.Float64(), Color: color.RGBA{0xd8, 0xc8, 0xa8, 255}}
	case "ice":
		return &Specular{Pow: 30 + 10*rng.Float64(), Strength: 0.15 + 0.10*rng.Float64(), Color: color.RGBA{0xf0, 0xf8, 0xff, 255}}
	case "lava":
		return &Specular{Pow: 16, Strength: 0.08 + 0.07*rng.Float64(), Color: color.RGBA{0xff, 0xb0, 0x60, 255}}
	case "rocky":
		return &Specular{Pow: 12, Strength: 0.05 + 0.05*rng.Float64(), Color: color.RGBA{0xf8, 0xf0, 0xe0, 255}}
	default: // earth
		return &Specular{Pow: 20, Strength: 0.12 + 0.08*rng.Float64(), Color: color.RGBA{0xff, 0xf8, 0xe0, 255}}
	}
}

// postFXFor — параметры постобработки (спека §4.2, контракт M3): L от seed;
// shadowStrength по типу (ice 0.4, остальные 0.6); specular по таблице типов
// (full: поправка по давлению — P < 0.1 → спекла нет, P ≥ 1 → широкий ореол
// цветом дымки); atmGlint — только full (P ≥ 0.1).
func postFXFor(in PlanetImageInput, L [3]float64, ap atmosphereParams, rng *rand.Rand) PostFX {
	vt := postVisualType(in)
	shadow := 0.6
	if vt == "ice" {
		shadow = 0.4
	}
	fx := PostFX{L: L, ShadowStrength: shadow}

	if in.Mode == ImageModeFull {
		// Атмосферная составляющая: ореол на дневном лимбе (только full).
		if in.PressureAtm >= 0.1 {
			fx.AtmGlint = &Glint{GlowStrength: ap.glowStrength, Haze: ap.haze}
		}
		// Спекл: поправка по давлению (контракт B).
		switch {
		case in.PressureAtm < 0.1:
			// Тонкая атмосфера — спекла нет (сила 0).
		case in.PressureAtm >= 1:
			// Плотная — широкий ореол цветом дымки вместо точечного спекла.
			fx.Specular = &Specular{Pow: 5, Strength: 0.2, Color: ap.haze}
		default:
			fx.Specular = specularForType(vt, rng)
		}
	} else {
		fx.Specular = specularForType(vt, rng)
	}
	return fx
}