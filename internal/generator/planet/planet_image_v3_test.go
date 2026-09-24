// internal/generator/planet/planet_image_v3_test.go
//
// Тесты доработки картинки планеты (спека 2026-09-21 §8.2): форма поверхности
// «поле высот + регионы» (бюджет долей, отсутствие кругов, массы), блик от
// L(seed) (потолок спекла, заглушка без блика), освещённые кольца, реальная
// атмосфера (калибровка/фолбэки), версия кэша.
package planet

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// testImageInputFull — вход режима full (реальная атмосфера, спека §3.1).
func testImageInputFull() PlanetImageInput {
	in := testImageInput()
	in.Mode = ImageModeFull
	in.Composition = map[string]float64{"N2": 78, "O2": 21, "Ar": 1}
	in.PressureAtm = 1
	in.TauIR = 0.9
	in.ScaleHeightKm = 8
	return in
}

// ==================== №1: ДЕТЕРМИНИЗМ (3 РЕЖИМА) ====================

func TestSurfaceDeterministic(t *testing.T) {
	inputs := map[ImageMode]PlanetImageInput{
		ImageModeHonest: testImageInput(),
		ImageModeFull:   testImageInputFull(),
		ImageModeStub:   {PlanetID: "11111111-1111-1111-1111-111111111111", Mode: ImageModeStub, Size: ImageSizeSmall},
	}
	for mode, in := range inputs {
		a, err := testPlanetGenerator().GeneratePlanetImage(in)
		require.NoError(t, err)
		b, err := testPlanetGenerator().GeneratePlanetImage(in)
		require.NoError(t, err)
		var bufA, bufB bytes.Buffer
		require.NoError(t, png.Encode(&bufA, a))
		require.NoError(t, png.Encode(&bufB, b))
		require.True(t, bytes.Equal(bufA.Bytes(), bufB.Bytes()),
			"режим %s: одинаковый вход → побайтово одинаковый PNG", mode)
	}
}

// ==================== №2: БЮДЖЕТ ДОЛЕЙ (S2/C1) ====================

// colorDist — евклидово расстояние между цветами.
func colorDist(a, b color.RGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	return dr*dr + dg*dg + db*db
}

// nearestBiome — биом с ближайшим цветом (рельеф модулирует яркость).
func nearestBiome(c color.RGBA, biomes []models.Biome, in PlanetImageInput) string {
	best := ""
	bestDist := 1 << 30
	for _, b := range biomes {
		d := colorDist(c, biomeColorFor(b.Form, in))
		if d < bestDist {
			bestDist = d
			best = b.Form
		}
	}
	return best
}

func TestSurfaceRegionAllocation(t *testing.T) {
	// Бюджет долей на 8 seed + крайние составы (практика PITFALLS «Дизайн и
	// числа» итерация 4 — «проверено на 8 seed»; замечание критика дельты
	// 2026-09-21: бюджет для крайних составов — вода-доминант ~90%,
	// крио-доминант ~90% — не был покрыт, дополнение из гейта создателя).
	compositions := []struct {
		name   string
		biomes []models.Biome
	}{
		{"типичный 40/35/22/3", []models.Biome{
			{Form: "горы", Share: 40},
			{Form: "океаны", Share: 35},
			{Form: "леса", Share: 22},
			{Form: "ледники", Share: 3},
		}},
		{"вода-доминант 90/7/3", []models.Biome{
			{Form: "океаны", Share: 90},
			{Form: "горы", Share: 7},
			{Form: "ледники", Share: 3},
		}},
		{"крио-доминант 90/7/3", []models.Biome{
			{Form: "ледники", Share: 90},
			{Form: "горы", Share: 7},
			{Form: "леса", Share: 3},
		}},
	}
	seeds := []int64{1, 7, 42, 100, 2024, 31337, 777777, 999983}

	for _, tc := range compositions {
		for _, seed := range seeds {
			t.Run(fmt.Sprintf("%s/seed%d", tc.name, seed), func(t *testing.T) {
				in := testImageInput()
				in.Biomes = tc.biomes

				img := image.NewRGBA(image.Rect(0, 0, 256, 256))
				generateBiomeSurface(img, 256, seed, in)

				// Детерминизм разбиения: тот же seed → те же пиксели.
				img2 := image.NewRGBA(image.Rect(0, 0, 256, 256))
				generateBiomeSurface(img2, 256, seed, in)
				require.Equal(t, img.Pix, img2.Pix, "тот же seed → те же пиксели")

				// Бюджет: крупные биомы (share ≥ 25%) — строго ±30%
				// (count/diskPixels ∈ [share×0.7, share×1.3]); остальные — только
				// «не исчезает» (≥ share×0.3, без жёсткого потолка: малые биомы в
				// крайних составах гуляют — решение создателя 2026-09-21). Заливка
				// зазоров — «ближайший регион» (взвешенный Вороной), не «два
				// ближайших + fbm-порог».
				diskPixels := 0
				counts := map[string]int{}
				for y := 0; y < 256; y++ {
					for x := 0; x < 256; x++ {
						c := img.RGBAAt(x, y)
						if c.A == 0 {
							continue
						}
						diskPixels++
						counts[nearestBiome(c, tc.biomes, in)]++
					}
				}
				require.Greater(t, diskPixels, 0)
				for _, b := range tc.biomes {
					frac := float64(counts[b.Form]) / float64(diskPixels)
					share := b.Share / 100
					t.Logf("DEBUG биом %s: share %.2f, frac %.3f", b.Form, share, frac)
				}
				for _, b := range tc.biomes {
					frac := float64(counts[b.Form]) / float64(diskPixels)
					share := b.Share / 100
					if b.Share >= 25 {
						require.InDelta(t, share, frac, share*0.3,
							"биом %s: бюджет ±30%% (share %v%%)", b.Form, b.Share)
					} else {
						require.GreaterOrEqual(t, frac, share*0.3,
							"биом %s: не исчезает (доля ≥ share×0.3, share %v%%)", b.Form, b.Share)
					}
				}
			})
		}
	}
}

// ==================== №3: КРУГИ НЕ ЧИТАЮТСЯ (M6) ====================

// isBiomePixel — пиксель цвета биома (с учётом модуляции яркости рельефа).
func isBiomePixel(c color.RGBA, form string, in PlanetImageInput) bool {
	return colorDist(c, biomeColorFor(form, in)) < 70*70
}

// boundaryRaggedness — (max−min)/mean расстояний граничных пикселей биома
// от центра масс границы. Круг → ~0; изломанная граница → > 0.2.
func boundaryRaggedness(img *image.RGBA, form string, in PlanetImageInput) float64 {
	var xs, ys []float64
	for y := 1; y < img.Bounds().Dy()-1; y++ {
		for x := 1; x < img.Bounds().Dx()-1; x++ {
			c := img.RGBAAt(x, y)
			if c.A == 0 || !isBiomePixel(c, form, in) {
				continue
			}
			boundary := false
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nc := img.RGBAAt(x+d[0], y+d[1])
				if nc.A == 0 || !isBiomePixel(nc, form, in) {
					boundary = true
					break
				}
			}
			if boundary {
				xs = append(xs, float64(x))
				ys = append(ys, float64(y))
			}
		}
	}
	if len(xs) < 20 {
		return 0
	}
	var sx, sy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
	}
	cx, cy := sx/float64(len(xs)), sy/float64(len(ys))
	minD, maxD, sumD := math.MaxFloat64, 0.0, 0.0
	for i := range xs {
		d := math.Hypot(xs[i]-cx, ys[i]-cy)
		if d < minD {
			minD = d
		}
		if d > maxD {
			maxD = d
		}
		sumD += d
	}
	meanD := sumD / float64(len(xs))
	if meanD == 0 {
		return 0
	}
	return (maxD - minD) / meanD
}

func TestSurfaceNoCircles(t *testing.T) {
	// Крупнейший биом (share ≥ 25%, регионы сливаются) + средний
	// (share 5–15%, n_i = 1 — одиночный регион): граница «изломана» —
	// (max−min)/mean > 0.35 (варп/поле высот + агрессивный двухслойный
	// доменный варп и модуляция радиуса, дельта 2026-09-21, круг не
	// читается).
	//
	// Порог поднят 0.2 → 0.35 по фактическому росту метрики на эталоне
	// (планета Renyenmus, землеподобная: до дельты метрика была ~0.25–0.3,
	// после — ~0.4+, см. отчёт разработчика), а не «чтобы не краснел».
	// НЕ снижать ниже 0.3 без решения создателя.
	biomes := []models.Biome{
		{Form: "горы", Share: 45},
		{Form: "океаны", Share: 35},
		{Form: "леса", Share: 14},
		{Form: "ледники", Share: 6},
	}
	seed := int64(7)
	in := testImageInput()
	in.Biomes = biomes

	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	generateBiomeSurface(img, 256, seed, in)

	for _, target := range []string{"горы", "ледники"} {
		r := boundaryRaggedness(img, target, in)
		require.Greater(t, r, 0.35,
			"биом %s: граница изломана (круг не читается), (max−min)/mean = %.3f", target, r)
	}
}

// ==================== №4: МАССЫ ПОЛЯ ВЫСОТ ====================

func TestSurfaceMasses(t *testing.T) {
	// Поле высот даёт ≥ 2 крупные массы: связные компоненты {E > 0.5} ≥ 2
	// (континенты/океаны, спека §4.1 шаг 1).
	seed := int64(42)
	size := 256
	noise := newValueNoise(seed)
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2

	visited := make([][]bool, size)
	for i := range visited {
		visited[i] = make([]bool, size)
	}
	components := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			if math.Sqrt(dx*dx+dy*dy) > radius || visited[y][x] {
				continue
			}
			px, py, pz := spherePoint(dx, dy, radius, 1.3)
			if elevationAt(noise, [3]float64{px, py, pz}, 1.3) <= 0.5 {
				continue
			}
			components++
			queue := [][2]int{{x, y}}
			visited[y][x] = true
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					nx, ny := cur[0]+d[0], cur[1]+d[1]
					if nx < 0 || ny < 0 || nx >= size || ny >= size || visited[ny][nx] {
						continue
					}
					ndx := float64(nx) - cx
					ndy := float64(ny) - cy
					if math.Sqrt(ndx*ndx+ndy*ndy) > radius {
						continue
					}
					npx, npy, npz := spherePoint(ndx, ndy, radius, 1.3)
					if elevationAt(noise, [3]float64{npx, npy, npz}, 1.3) <= 0.5 {
						continue
					}
					visited[ny][nx] = true
					queue = append(queue, [2]int{nx, ny})
				}
			}
		}
	}
	require.GreaterOrEqual(t, components, 2, "поле высот даёт ≥ 2 крупные массы")
}

// ==================== №5: СПЕКЛ ОТ L(SEED) ====================

func TestSpecularBySeed(t *testing.T) {
	// Разные seed → разные L (az/el) — разнообразие между планетами.
	l1 := lightVector(1)
	l2 := lightVector(2)
	require.NotEqual(t, l1, l2, "разные seed → разные световые векторы")

	// Потолок силы спекла ≤ 0.25 (было 0.8, контракт B).
	table := []Specular{
		{Pow: 8, Strength: 0.18, Color: color.RGBA{0xd8, 0xc8, 0xa8, 255}},
		{Pow: 40, Strength: 0.25, Color: color.RGBA{0xf0, 0xf8, 0xff, 255}},
		{Pow: 20, Strength: 0.20, Color: color.RGBA{0xff, 0xf8, 0xe0, 255}},
		{Pow: 12, Strength: 0.10, Color: color.RGBA{0xf8, 0xf0, 0xe0, 255}},
		{Pow: 16, Strength: 0.15, Color: color.RGBA{0xff, 0xb0, 0x60, 255}},
	}
	for _, s := range table {
		for _, diffuse := range []float64{0.1, 0.5, 0.9} {
			for _, normDz := range []float64{0.3, 0.7, 1.0} {
				si := specIntensity(diffuse, normDz, 0.2, &s)
				require.LessOrEqual(t, si, 0.25, "потолок силы спекла ≤ 0.25")
			}
		}
	}
}

// ==================== №6: КОЛЬЦА ОСВЕЩЕНЫ (S3) ====================

func TestRingsLit(t *testing.T) {
	// Кольца: средняя яркость дневной стороны > ночной; пиксели в тени
	// планеты темнее (×0.25–0.35).
	seed := int64(42)
	size := 512
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	clearDisk(img, size)
	rng := rand.New(rand.NewSource(seed))
	L := [3]float64{1, 0, 0} // свет справа — дневная сторона +x
	drawRings(img, size, rng, L, color.RGBA{0xd8, 0xc8, 0xa8, 255}, 0.15)

	cx := float64(size) / 2
	radius := float64(size)/2 - 2
	var daySum, nightSum, shadowSum float64
	var dayN, nightN, shadowN int
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := img.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			dx := float64(x) - cx
			dy := float64(y) - cx
			if math.Sqrt(dx*dx+dy*dy) <= radius {
				continue // диск планеты, не кольцо
			}
			bright := (float64(c.R) + float64(c.G) + float64(c.B)) / 3
			switch {
			case dx > 0:
				daySum += bright
				dayN++
			case dx < 0 && math.Abs(dy) < radius:
				shadowSum += bright // тень планеты (цилиндр вдоль L)
				shadowN++
			case dx < 0:
				nightSum += bright
				nightN++
			}
		}
	}
	require.Greater(t, dayN, 0, "дневная сторона колец есть")
	require.Greater(t, nightN+shadowN, 0, "ночная сторона колец есть")
	require.Greater(t, daySum/float64(dayN), (nightSum+shadowSum)/float64(nightN+shadowN),
		"дневная сторона колец ярче ночной")
	require.Greater(t, shadowN, 0, "тень планеты на кольце есть")
	require.Less(t, shadowSum/float64(shadowN), daySum/float64(dayN)*0.6,
		"пиксели в тени планеты темнее (×0.25–0.35)")
}

// ==================== №7: РЕАЛЬНАЯ АТМОСФЕРА (КАЛИБРОВКА) ====================

func TestAtmosphereReal(t *testing.T) {
	cases := []struct {
		name      string
		comp      map[string]float64
		P, tau, SH float64
		haze      color.RGBA
		limbPx    float64
		limbAlpha float64
		glow      float64
		fc        float64
	}{
		// Таблица §4.3 (формула — исполняемое правило; допуск ±0.03/±0.5 px).
		{"Земля", map[string]float64{"N2": 78, "O2": 21, "Ar": 1}, 1, 0.9, 8,
			color.RGBA{0xcf, 0xda, 0xea, 255}, 1.75, 0.19, 0.225, 0.30},
		{"Венера", map[string]float64{"CO2": 96}, 92, 100, 15.9,
			color.RGBA{0xe5, 0xd6, 0xb8, 255}, 8.35, 0.55, 1.0, 0.90},
		{"Марс", map[string]float64{"CO2": 95}, 0.006, 0.05, 11,
			color.RGBA{0xe4, 0xd5, 0xb7, 255}, 1.18, 0.12, 0.0125, 0},
		{"Юпитер", map[string]float64{"H2": 90, "He": 10}, 1, 0.5, 27,
			color.RGBA{0xdd, 0xdf, 0xe6, 255}, 3.22, 0.19, 0.125, 0.30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			haze, _, limbPx, limbAlpha, glow, _, fc := realAtmosphere(tc.comp, tc.P, tc.tau, tc.SH)
			require.InDelta(t, tc.limbPx, limbPx, 0.5, "limbPx")
			require.InDelta(t, tc.limbAlpha, limbAlpha, 0.03, "limbAlpha")
			require.InDelta(t, tc.glow, glow, 0.03, "glowStrength")
			require.InDelta(t, tc.fc, fc, 0.03, "fc")
			require.InDelta(t, float64(tc.haze.R), float64(haze.R), 8, "haze.R")
			require.InDelta(t, float64(tc.haze.G), float64(haze.G), 8, "haze.G")
			require.InDelta(t, float64(tc.haze.B), float64(haze.B), 8, "haze.B")
		})
	}
}

// ==================== №8: ФОЛБЭКИ АТМОСФЕРЫ ====================

func TestAtmosphereFallbacks(t *testing.T) {
	// P ≤ 0 → лимб 1px/0.08, дымка 0.03, свечение 0, облака 0 (§7).
	_, hazeAlpha, limbPx, limbAlpha, glow, _, fc := realAtmosphere(map[string]float64{"N2": 100}, 0, 0.5, 8)
	require.InDelta(t, 1, limbPx, 0.01, "P≤0: лимб 1 px")
	require.InDelta(t, 0.08, limbAlpha, 0.01, "P≤0: limbAlpha 0.08")
	require.InDelta(t, 0.03, hazeAlpha, 0.01, "P≤0: дымка 0.03")
	require.InDelta(t, 0, glow, 0.01, "P≤0: свечение 0")
	require.InDelta(t, 0, fc, 0.01, "P≤0: облака 0")

	// tau_ir ≤ 0 → свечение 0.
	_, _, _, _, glow2, _, _ := realAtmosphere(map[string]float64{"N2": 100}, 1, 0, 8)
	require.InDelta(t, 0, glow2, 0.01, "tau_ir≤0: свечение 0")

	// SH ≤ 0 → SH = 8 (фолбэк).
	_, _, limbPx3, _, _, _, _ := realAtmosphere(map[string]float64{"N2": 100}, 1, 0.5, 0)
	_, _, limbPx4, _, _, _, _ := realAtmosphere(map[string]float64{"N2": 100}, 1, 0.5, 8)
	require.InDelta(t, limbPx4, limbPx3, 0.01, "SH≤0 → SH=8")

	// Пустой состав → не падать: realAtmosphere возвращает валидный цвет
	// (нейтральная дымка), а полный путь (atmosphereParamsFor) уходит на
	// фолбэк §7 — производная дымка по палитре типа (hazeColor).
	hazeNil, _, _, _, _, _, _ := realAtmosphere(nil, 1, 0.5, 8)
	require.Equal(t, uint8(255), hazeNil.A, "пустой состав: валидный цвет, не паника")

	in := testImageInputFull()
	in.Composition = nil // full без atmosphere_data (старый мир)
	ap := atmosphereParamsFor(in, cloudCover(in))
	require.Equal(t, hazeColor(in), ap.haze, "full без atmosphere_data → производная дымка (hazeColor)")
}

// ==================== №9: ВЕРСИЯ КЭША (S1) ====================

func TestCacheKeyVersion(t *testing.T) {
	kHonest := imageCacheKey("p1", ImageModeHonest, ImageSizeSmall)
	kFull := imageCacheKey("p1", ImageModeFull, ImageSizeSmall)
	kStub := imageCacheKey("p1", ImageModeStub, ImageSizeSmall)
	kBig := imageCacheKey("p1", ImageModeHonest, ImageSizeBig)
	require.NotEqual(t, kHonest, kFull, "режим full в ключе")
	require.NotEqual(t, kHonest, kStub, "режим stub в ключе")
	require.NotEqual(t, kHonest, kBig, "размер в ключе")

	// Версия генератора в ключе (S1): смена версии → новый ключ.
	require.Contains(t, kHonest, ImageGenVersion, "версия генератора в ключе")
	// ЧК3: 7 новых color биомов вулканизма меняют картинку с орбиты — бамп
	// v7 → v8 обязателен, иначе диск-кэш отдаёт старый цвет (§4.7.9 п.6).
	require.Equal(t, "v8", ImageGenVersion, "ЧК3: ImageGenVersion v8")
	other := fmt.Sprintf("%d|%s|%s|%s", imageSeed("p1"), ImageModeHonest, ImageSizeSmall, "v2")
	require.NotEqual(t, kHonest, other, "смена версии → новый ключ")
}

// ==================== №12: ЗАГЛУШКА БЕЗ БЛИКА (M4) ====================

func TestStubNoSpecularGlow(t *testing.T) {
	// Заглушка: нет спекл-блика и свечения (контракт D, M4) — max яркость
	// диска ниже порога (нет белых/ярких пикселей блика).
	img := generateStub(256, 42)
	maxBright := 0.0
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			c := img.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			b := (float64(c.R) + float64(c.G) + float64(c.B)) / 3
			if b > maxBright {
				maxBright = b
			}
		}
	}
	require.Less(t, maxBright, 200.0, "заглушка без спекл-блика (M4)")
}