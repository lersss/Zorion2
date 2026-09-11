// internal/generator/planet/composition_test.go
// Юнит-тесты композиций, классификации и совместимости форм.
package planet

import (
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== COMPOSITION (МЕТОДЫ) ====================

func TestCompositionTotalAndNormalize(t *testing.T) {
	c := Composition{SurfaceRocks: 60, SurfaceOceans: 40}
	assert.InDelta(t, 100, c.Total(), 0.001)

	// Normalize делает сумму ровно 100.
	n := c.Normalize()
	assert.InDelta(t, 100, n.Total(), 0.001)

	// Пустая и нулевая композиции.
	assert.Equal(t, Composition{}, Composition{}.Normalize())
	assert.Equal(t, Composition{}, Composition{SurfaceRocks: 0}.Normalize())
}

func TestCompositionNonZeroAndHas(t *testing.T) {
	c := Composition{SurfaceRocks: 60, SurfaceOceans: 40, SurfaceLakes: 0.005}
	nz := c.NonZero()
	assert.Equal(t, 2, len(nz), "доли ≤ 0.01 отбрасываются")
	assert.True(t, c.Has(SurfaceRocks))
	assert.False(t, c.Has("несуществующая"))
}

func TestCompositionDominantForm(t *testing.T) {
	assert.Equal(t, "", (Composition{}).DominantForm())
	c := Composition{SurfaceRocks: 40, SurfaceOceans: 60, SurfaceLakes: 20}
	assert.Equal(t, SurfaceOceans, c.DominantForm())
}

func TestCompositionSortedForms(t *testing.T) {
	c := Composition{SurfaceRocks: 30, SurfaceOceans: 60, SurfaceLakes: 10}
	forms := c.SortedForms()
	require.Len(t, forms, 3)
	assert.Equal(t, SurfaceOceans, forms[0])
	assert.Equal(t, SurfaceRocks, forms[1])
	assert.Equal(t, SurfaceLakes, forms[2])
}

func TestCompositionBiosphereSum(t *testing.T) {
	c := Composition{
		SurfaceMeadows: 10, SurfaceForests: 15, SurfaceJungles: 5,
		SurfaceRocks: 60, SurfaceOceans: 10, // не биосфера
	}
	assert.InDelta(t, 30, c.BiosphereSum(), 0.001)

	assert.Equal(t, 0.0, (Composition{}).BiosphereSum())
	assert.Equal(t, 0.0, (Composition{SurfaceRocks: 100}).BiosphereSum())
}

// ==================== КЛАССИФИКАЦИЯ ====================

func TestClassifyGasGiant(t *testing.T) {
	// Флаг важнее композиции.
	typ := ClassifyGameDesignType(PlanetClassificationInput{
		IsGasGiant: true, Surface: Composition{SurfaceRocks: 100},
	})
	assert.Equal(t, TypeGasGiant, typ)
}

func TestClassifyRadioactive(t *testing.T) {
	typ := ClassifyGameDesignType(PlanetClassificationInput{
		IsRadioactive: true, Surface: Composition{SurfaceRocks: 100},
	})
	assert.Equal(t, TypeRadioactive, typ)
}

func TestClassifyEarthlike(t *testing.T) {
	// Обитаема + жизнь + скалы + вода (океаны ИЛИ озёра).
	in := PlanetClassificationInput{
		Habitable: true, Life: true,
		Surface: Composition{SurfaceRocks: 60, SurfaceOceans: 40},
	}
	assert.Equal(t, TypeEarthlike, ClassifyGameDesignType(in))

	// Озёра тоже достаточно.
	in = PlanetClassificationInput{
		Habitable: true, Life: true,
		Surface: Composition{SurfaceRocks: 80, SurfaceLakes: 20},
	}
	assert.Equal(t, TypeEarthlike, ClassifyGameDesignType(in))

	// Землеподобность приоритетнее океаничности даже при огромных океанах.
	in = PlanetClassificationInput{
		Habitable: true, Life: true, WaterPercent: 90,
		Surface: Composition{SurfaceRocks: 30, SurfaceOceans: 70},
	}
	assert.Equal(t, TypeEarthlike, ClassifyGameDesignType(in))

	// Без жизни или скал — НЕ землеподобная.
	in = PlanetClassificationInput{
		Habitable: true, Life: false,
		Surface: Composition{SurfaceRocks: 60, SurfaceOceans: 40},
	}
	assert.NotEqual(t, TypeEarthlike, ClassifyGameDesignType(in))
}

func TestClassifyOceanic(t *testing.T) {
	in := PlanetClassificationInput{
		Temperature:  300,
		WaterPercent: 80,
		Surface:      Composition{SurfaceOceans: 70, SurfaceRocks: 30},
	}
	assert.Equal(t, TypeOceanic, ClassifyGameDesignType(in))

	in.Temperature = 300
	in.WaterPercent = 40
	assert.NotEqual(t, TypeOceanic, ClassifyGameDesignType(in))
}

func TestClassifyIce(t *testing.T) {
	// Доминируют ледники + холодно.
	in := PlanetClassificationInput{
		Temperature: 200,
		Surface:     Composition{SurfaceGlaciers: 60, SurfaceRocks: 40},
	}
	assert.Equal(t, TypeIce, ClassifyGameDesignType(in))

	// Очень холодно (< 150) — ледяная даже без ледников.
	in = PlanetClassificationInput{
		Temperature: 100,
		Surface:     Composition{SurfaceRocks: 100},
	}
	assert.Equal(t, TypeIce, ClassifyGameDesignType(in))

	// Тепло — не ледяная.
	in = PlanetClassificationInput{
		Temperature: 300,
		Surface:     Composition{SurfaceGlaciers: 60, SurfaceRocks: 40},
	}
	assert.NotEqual(t, TypeIce, ClassifyGameDesignType(in))
}

func TestClassifyVolcanic(t *testing.T) {
	in := PlanetClassificationInput{
		Temperature: 500,
		Surface:     Composition{SurfaceLavaFields: 15, SurfaceVolcanicFields: 10, SurfaceRocks: 75},
	}
	assert.Equal(t, TypeVolcanic, ClassifyGameDesignType(in))

	assert.Equal(t, TypeVolcanic, ClassifyGameDesignType(PlanetClassificationInput{
		Temperature: 500,
		Surface:     Composition{SurfaceVolcanicFields: 25, SurfaceRocks: 75},
	}))

	// Мало лавы — не вулканическая.
	in = PlanetClassificationInput{
		Temperature: 300,
		Surface:     Composition{SurfaceLavaFields: 10, SurfaceRocks: 90},
	}
	assert.NotEqual(t, TypeVolcanic, ClassifyGameDesignType(in))
}

func TestClassifyDesert(t *testing.T) {
	in := PlanetClassificationInput{
		Temperature:  300,
		WaterPercent: 10,
		Surface:      Composition{SurfaceSands: 40, SurfaceRocks: 60},
	}
	assert.Equal(t, TypeDesert, ClassifyGameDesignType(in))

	// Много воды — не пустыня.
	in.WaterPercent = 30
	assert.NotEqual(t, TypeDesert, ClassifyGameDesignType(in))
}

func TestClassifyGlassMetal(t *testing.T) {
	typ := ClassifyGameDesignType(PlanetClassificationInput{
		Temperature: 300,
		Surface:     Composition{SurfaceGlassFields: 20, SurfaceRocks: 80},
	})
	assert.Equal(t, TypeGlass, typ)

	typ = ClassifyGameDesignType(PlanetClassificationInput{
		Temperature: 300,
		Surface:     Composition{SurfaceMetalFields: 20, SurfaceRocks: 80},
	})
	assert.Equal(t, TypeMetal, typ)
}

func TestClassifyOrganic(t *testing.T) {
	in := PlanetClassificationInput{
		Temperature: 300,
		Surface:     Composition{SurfaceMeadows: 10, SurfaceForests: 15, SurfaceRocks: 75},
	}
	assert.Equal(t, TypeOrganic, ClassifyGameDesignType(in))
}

func TestClassifyFallbackRocky(t *testing.T) {
	typ := ClassifyGameDesignType(PlanetClassificationInput{
		Temperature: 300,
		Surface:     Composition{SurfaceRocks: 100},
	})
	assert.Equal(t, TypeRocky, typ)
}

func TestGameDesignTypeName(t *testing.T) {
	// Читаемое имя равно коду (симметрия выполнена намеренно).
	for _, code := range AllGameDesignTypes {
		assert.Equal(t, code, GameDesignTypeName(code))
	}
}

// ==================== СОВМЕСТИМОСТЬ ====================

// requireDefaultCompat — гарантирует дефолтную матрицу совместимости.
func requireDefaultCompat(t *testing.T) {
	t.Helper()
	err := LoadCompatibilityMatrix(filepath.Join(t.TempDir(), "no_such_matrix.json"))
	require.NoError(t, err)
}

func TestIsCompatibleDefault(t *testing.T) {
	requireDefaultCompat(t)

	assert.False(t, IsCompatible("surface", SurfaceLavaFields, SurfaceGlaciers))
	assert.False(t, IsCompatible("surface", SurfaceLavaFields, SurfaceOceans))
	assert.False(t, IsCompatible("surface", SurfaceGlaciers, SurfaceJungles))

	// Симметрия.
	assert.False(t, IsCompatible("surface", SurfaceGlaciers, SurfaceLavaFields))

	// Обычные пары совместимы.
	assert.True(t, IsCompatible("surface", SurfaceRocks, SurfaceOceans))
	assert.True(t, IsCompatible("surface", SurfaceOceans, SurfaceLakes))

	// Форма сама с собой — всегда совместима.
	assert.True(t, IsCompatible("surface", SurfaceLavaFields, SurfaceLavaFields))

	// Недра: магматические камеры несовместимы со льдом и водой.
	assert.False(t, IsCompatible("subterrain", SubterrainMagmaChambers, SubterrainGroundIce))
	assert.False(t, IsCompatible("subterrain", SubterrainMagmaChambers, SubterrainGroundwater))
	assert.True(t, IsCompatible("subterrain", SubterrainMagmaChambers, SubterrainMagmaticRocks))

	// Неизвестная категория — по умолчанию совместима.
	assert.True(t, IsCompatible("unknown", SurfaceLavaFields, SurfaceGlaciers))
}

func TestIsCompatibleBeforeLoad(t *testing.T) {
	compatMu.Lock()
	compatMatrix = nil
	compatMu.Unlock()

	// Пока матрица не загружена — всё совместимо.
	assert.True(t, IsCompatible("surface", SurfaceLavaFields, SurfaceGlaciers))

	// Восстанавливаем для остальных тестов.
	requireDefaultCompat(t)
}

func TestIsCompositionValid(t *testing.T) {
	requireDefaultCompat(t)

	valid := Composition{SurfaceRocks: 60, SurfaceOceans: 40}
	assert.True(t, IsCompositionValid("surface", valid))

	invalid := Composition{SurfaceLavaFields: 50, SurfaceGlaciers: 50}
	assert.False(t, IsCompositionValid("surface", invalid))

	// Нулевые доли не участвуют в проверке.
	almostEmpty := Composition{SurfaceLavaFields: 50, SurfaceGlaciers: 0.005}
	assert.True(t, IsCompositionValid("surface", almostEmpty))

	validSub := Composition{SubterrainMagmaChambers: 60, SubterrainMagmaticRocks: 40}
	assert.True(t, IsCompositionValid("subterrain", validSub))
}

func TestLoadCompatibilityMatrixBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad_matrix.json")
	require.NoError(t, os.WriteFile(path, []byte("{не-json"), 0o644))

	err := LoadCompatibilityMatrix(path)
	require.Error(t, err, "битый JSON должен вернуть ошибку")
}

// ==================== ГЕНЕРАТОР КОМПОЗИЦИИ ====================

func TestGenerateSurfaceCompositionSumAndValidity(t *testing.T) {
	requireDefaultCompat(t)
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 200; i++ {
		base := Composition{SurfaceRocks: 50, SurfaceLavaFields: 30, SurfaceOceans: 20}
		c := GenerateSurfaceComposition(base, 300+rng.Float64()*400, rng.Float64()*80, rng)
		require.NotEmpty(t, c, "композиция не должна быть пустой (fallback)")
		assert.InDelta(t, 100, c.Total(), 0.5, "сумма поверхности ~100")
		assert.True(t, IsCompositionValid("surface", c),
			"сгенерированная композиция не должна содержать несовместимых пар: %v", c)
	}
}

func TestGenerateSurfaceCompositionCold(t *testing.T) {
	requireDefaultCompat(t)
	rng := rand.New(rand.NewSource(7))

	for i := 0; i < 100; i++ {
		base := Composition{SurfaceRocks: 40, SurfaceGlaciers: 40, SurfaceFrozenGases: 20}
		c := GenerateSurfaceComposition(base, 100, 5, rng)
		assert.InDelta(t, 100, c.Total(), 0.5)
		assert.True(t, IsCompositionValid("surface", c))
	}
}

func TestGenerateSurfaceCompositionEmptyBase(t *testing.T) {
	assert.Equal(t, Composition{}, GenerateSurfaceComposition(nil, 300, 50, rand.New(rand.NewSource(1))))
}

func TestGenerateSubterrainComposition(t *testing.T) {
	requireDefaultCompat(t)
	rng := rand.New(rand.NewSource(8))

	for i := 0; i < 200; i++ {
		base := Composition{
			SubterrainEmptyRock:     30, SubterrainMagmaticRocks: 20,
			SubterrainOreVeins:      20, SubterrainGroundIce: 15, SubterrainGroundwater: 15,
		}
		surface := GenerateSurfaceComposition(
			Composition{SurfaceRocks: 60, SurfaceOceans: 40}, 250, 30, rng)
		c := GenerateSubterrainComposition(base, surface, 250, 30, rng)
		require.NotEmpty(t, c)
		assert.InDelta(t, 100, c.Total(), 0.5)
		assert.True(t, IsCompositionValid("subterrain", c))
	}
}

// ==================== СИМПЛИФИКАЦИЯ (ПРИМИТИВНЫЕ ТЕЛА) ====================

func TestSimplifyCompositionOneOrTwoForms(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	complex := Composition{
		SurfaceRocks: 30, SurfaceSands: 25, SurfaceOceans: 20,
		SurfaceLakes: 15, SurfaceCraters: 10,
	}

	for i := 0; i < 200; i++ {
		simple := simplifyComposition(complex, rng)
		require.Contains(t, []int{1, 2}, len(simple), "одна или две формы")
		require.InDelta(t, 100.0, simple.Total(), 0.001, "сумма 100")

		for form := range simple {
			assert.Contains(t, []string{SurfaceRocks, SurfaceSands, SurfaceOceans, SurfaceLakes, SurfaceCraters}, form)
		}

		if len(simple) == 1 {
			for _, share := range simple {
				assert.InDelta(t, 100.0, share, 0.001, "одиночная форма = 100%")
			}
			continue
		}

		// Две формы: титульная 60–85%, вторичная — остаток (15–40%).
		shares := make([]float64, 0, 2)
		for _, share := range simple {
			shares = append(shares, share)
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(shares)))

		assertInDeltaRange(t, 60.0, 85.0, shares[0], "титульная форма")
		assertInDeltaRange(t, 15.0, 40.0, shares[1], "вторичная форма")
		assert.InDelta(t, 100.0, shares[0]+shares[1], 0.001)
	}
}

func TestSimplifyCompositionAlreadySimple(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	// 1 и 2 формы — не трогаются.
	one := Composition{SurfaceRocks: 100}
	assert.Equal(t, one, simplifyComposition(one, rng))

	two := Composition{SurfaceRocks: 60, SurfaceOceans: 40}
	assert.Equal(t, two, simplifyComposition(two, rng))
}

// ==================== ГРАНИЦЫ НАНО-ПРОВЕРОК ====================

func TestCompositionRoundingMatters(t *testing.T) {
	// Разница в знаках после запятой не должна ломать проверку суммы=100.
	c := Composition{SurfaceRocks: 100.0 / 3.0, SurfaceOceans: 100.0 / 3.0, SurfaceLakes: 100.0 / 3.0}
	n := c.Normalize()
	assert.InDelta(t, 100, n.Total(), 0.0001)
}