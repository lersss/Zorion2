// internal/generator/planet/planet_image_v2_test.go
//
// Тесты честного генератора картинки планеты (спека 2026-09-20 §8.2):
// детерминизм, заслонение (cloudCover по видимым параметрам), контракт
// «биом → цвет», алгоритм «share → пятна», кэш-ключ, валидация color.
package planet

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// testImageInput — базовый вход честной картинки (землеподобная с водой).
func testImageInput() PlanetImageInput {
	return PlanetImageInput{
		PlanetID:     "11111111-1111-1111-1111-111111111111",
		Mode:         ImageModeHonest,
		Size:         ImageSizeSmall,
		Biomes:       []models.Biome{{Form: "горы", Share: 40}, {Form: "океаны", Share: 35}, {Form: "леса", Share: 25}},
		Type:         "землеподобная",
		Temperature:  288,
		WaterPercent: 60,
		Habitable:    true,
		Life:         true,
	}
}

// ==================== №1: ДЕТЕРМИНИЗМ ====================

func TestPlanetImageDeterministic(t *testing.T) {
	in := testImageInput()

	// Два РАЗНЫХ генератора (без кэш-хита) — одинаковый вход → побайтово
	// одинаковый PNG (спека §9 «Детерминизм», М3/М4 закрыты).
	a, err := testPlanetGenerator().GeneratePlanetImage(in)
	require.NoError(t, err)
	b, err := testPlanetGenerator().GeneratePlanetImage(in)
	require.NoError(t, err)

	var bufA, bufB bytes.Buffer
	require.NoError(t, png.Encode(&bufA, a))
	require.NoError(t, png.Encode(&bufB, b))
	require.True(t, bytes.Equal(bufA.Bytes(), bufB.Bytes()),
		"одинаковый вход (планета, режим, размер) → побайтово одинаковый PNG")

	// Заглушка тоже детерминирована.
	stubIn := PlanetImageInput{PlanetID: in.PlanetID, Mode: ImageModeStub, Size: ImageSizeSmall}
	s1, err := testPlanetGenerator().GeneratePlanetImage(stubIn)
	require.NoError(t, err)
	s2, err := testPlanetGenerator().GeneratePlanetImage(stubIn)
	require.NoError(t, err)
	var bufS1, bufS2 bytes.Buffer
	require.NoError(t, png.Encode(&bufS1, s1))
	require.NoError(t, png.Encode(&bufS2, s2))
	require.True(t, bytes.Equal(bufS1.Bytes(), bufS2.Bytes()), "заглушка детерминирована по seed")
}

// ==================== №2: ЗАСЛОНЕНИЕ (cloudCover) ====================

func TestPlanetImageOcclusion(t *testing.T) {
	cases := []struct {
		name string
		in   PlanetImageInput
		want float64
	}{
		{"гигант → полное заслонение", PlanetImageInput{IsGasGiant: true}, 1.0},
		{"жаркий T≥350 → полное", PlanetImageInput{Temperature: 400}, 0.85},
		{"обитаемый → частичное", PlanetImageInput{Habitable: true}, 0.45},
		{"обитаемый с жизнью → 0.55", PlanetImageInput{Habitable: true, Life: true}, 0.55},
		{"умеренный с водой → 0.40", PlanetImageInput{Temperature: 300, WaterPercent: 50}, 0.40},
		{"умеренный сухой → 0.20", PlanetImageInput{Temperature: 300, WaterPercent: 10}, 0.20},
		{"холодный → дымка", PlanetImageInput{Temperature: 200}, 0.10},
		{"холодный с жизнью → 0.20", PlanetImageInput{Temperature: 200, Life: true}, 0.20},
		// Кламп ≤ 0.6 применяется к life-добавке, НЕ к гиганту/жаркому
		// (уточнение псевдокода §3.4): гигант/жаркий возвращаются раньше.
		{"гигант с жизнью — без life-добавки", PlanetImageInput{IsGasGiant: true, Life: true}, 1.0},
		{"жаркий с жизнью — без life-добавки", PlanetImageInput{Temperature: 400, Life: true}, 0.85},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.InDelta(t, tc.want, cloudCover(tc.in), 1e-9)
		})
	}
}

// ==================== №3: КОНТРАКТ «БИОМ → ЦВЕТ» ====================

func TestBiomeColorHybrid(t *testing.T) {
	in := testImageInput()

	// color из каталога переопределяет базу (приоритет, §4.2).
	b := &BiomeDef{ID: "x", Category: "литосфера", Color: "#aabbcc", Albedo: 0.5}
	require.Equal(t, color.RGBA{0xaa, 0xbb, 0xcc, 255}, biomeColor(b, in))

	// Без color — база категории + сдвиги (альбедо → яркость): не пустой цвет.
	b2 := &BiomeDef{ID: "y", Category: "вода", Albedo: 0.5}
	c2 := biomeColor(b2, in)
	require.Equal(t, uint8(255), c2.A)
	require.NotEqual(t, color.RGBA{}, c2)

	// Неизвестный id (каталог правлен после генерации) → фолбэк-цвет,
	// не паника (§4.2/§7).
	c3 := biomeColorFor("неизвестный_биом_xyz", in)
	require.Equal(t, uint8(255), c3.A)
}

// ==================== №4: АЛГОРИТМ «SHARE → ПЯТНА» ====================

func TestBlobAllocation(t *testing.T) {
	biomes := []models.Biome{
		{Form: "горы", Share: 40},
		{Form: "океаны", Share: 35},
		{Form: "леса", Share: 25},
	}
	seed := int64(42)

	// Каждый биом с share ≥ 1% присутствует (≥ 1 пятно, §4.3).
	bl := blobs(seed, biomes, 256)
	for _, b := range biomes {
		found := false
		for _, blob := range bl {
			if blob.id == b.Form {
				found = true
				break
			}
		}
		require.True(t, found, "биом %s должен иметь ≥ 1 пятно", b.Form)
	}

	// Детерминизм разбиения: тот же seed → те же пятна.
	bl2 := blobs(seed, biomes, 256)
	require.Equal(t, len(bl), len(bl2))
	for i := range bl {
		require.Equal(t, bl[i].id, bl2[i].id)
		require.InDelta(t, bl[i].cx, bl2[i].cx, 1e-9)
		require.InDelta(t, bl[i].cy, bl2[i].cy, 1e-9)
		require.InDelta(t, bl[i].r, bl2[i].r, 1e-9)
	}

	// Пиксели цвета каждого биома присутствуют на поверхности (до атмосферы/
	// постобработки — слой 1, §4.1). Толеранс — лёгкая модуляция яркости fbm.
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	in := testImageInput()
	in.Biomes = biomes
	generateBiomeSurface(img, 256, seed, in)
	for _, b := range biomes {
		want := biomeColorFor(b.Form, in)
		found := false
		for y := 0; y < 256 && !found; y++ {
			for x := 0; x < 256; x++ {
				c := img.RGBAAt(x, y)
				if c.A == 0 {
					continue
				}
				if colorClose(c, want, 70) {
					found = true
					break
				}
			}
		}
		require.True(t, found, "биом %s должен иметь пиксели своего цвета", b.Form)
	}
}

// colorClose — близость цветов в пределах tol по каждому каналу.
func colorClose(a, b color.RGBA, tol int) bool {
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return abs(int(a.R)-int(b.R)) <= tol &&
		abs(int(a.G)-int(b.G)) <= tol &&
		abs(int(a.B)-int(b.B)) <= tol
}

// ==================== №6: КЭШ-КЛЮЧ ====================

func TestCacheKey(t *testing.T) {
	// Ключ различает (режим, размер) — 2–3 варианта на планету (§5.2).
	kHonestSmall := imageCacheKey("p1", ImageModeHonest, ImageSizeSmall)
	kStubSmall := imageCacheKey("p1", ImageModeStub, ImageSizeSmall)
	kHonestBig := imageCacheKey("p1", ImageModeHonest, ImageSizeBig)
	require.NotEqual(t, kHonestSmall, kStubSmall, "режим в ключе")
	require.NotEqual(t, kHonestSmall, kHonestBig, "размер в ключе")

	keys := map[string]bool{kHonestSmall: true, kStubSmall: true, kHonestBig: true}
	require.Len(t, keys, 3, "2–3 варианта на планету")

	// Смена знания меняет режим → новый ключ (stub → honest).
	require.NotEqual(t, kStubSmall, imageCacheKey("p1", ImageModeHonest, ImageSizeSmall))

	// Разные планеты — разные ключи.
	require.NotEqual(t, kHonestSmall, imageCacheKey("p2", ImageModeHonest, ImageSizeSmall))
}

// ==================== №10: ВАЛИДАЦИЯ COLOR (инвариант 17) ====================

func TestBiomeColorValidation(t *testing.T) {
	cat := GetBiomeCatalog()

	// Валидный hex → ок.
	good := *cat
	good.Biomes = append([]BiomeDef{}, cat.Biomes...)
	good.Biomes[0].Color = "#aabbcc"
	require.NoError(t, good.Validate(), "валидный hex #aabbcc — ок")

	// Битый hex → ошибка Validate().
	for _, bad := range []string{"abc", "#12", "#gggggg", "#aabbccd", "aabbcc"} {
		c := *cat
		c.Biomes = append([]BiomeDef{}, cat.Biomes...)
		c.Biomes[0].Color = bad
		require.Error(t, c.Validate(), "color %q должен быть ошибкой", bad)
	}
}