package galaxy

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== 35b §3: параметры компаньона и сортировка ====================

// TestBinaryCompanionParamsFilled — у binary/multiple заполнены
// companion_mass/companion_temp/companion_sep_au; разделение внутри
// диапазона по binary_type (close [0.05, 0.9], wide [100, 3000]).
func TestBinaryCompanionParamsFilled(t *testing.T) {
	g := NewGenerator(&Config{Seed: 31, WorldCount: 60000, MapSize: 60000, MinDist: 40, WorldSpread: 0})
	worlds := g.generateWorldsRandom(40)

	binary := 0
	for _, w := range worlds {
		if w.SystemType != "binary" && w.SystemType != "multiple" {
			continue
		}
		binary++
		require.NotNil(t, w.StellarMods)
		mods := w.StellarMods
		require.NotNil(t, mods.CompanionMass, "companion_mass заполнен (35b §3.2)")
		require.NotNil(t, mods.CompanionTemp, "companion_temp заполнен (35b §3.2)")
		require.NotNil(t, mods.CompanionSepAU, "companion_sep_au заполнен (35b §3.2)")
		assert.Greater(t, *mods.CompanionMass, 0.0)
		assert.Greater(t, *mods.CompanionTemp, 0)

		sep := *mods.CompanionSepAU
		if mods.BinaryType == "close" {
			assert.GreaterOrEqual(t, sep, 0.05, "close: разделение ≥ 0.05 а.е.")
			assert.LessOrEqual(t, sep, 0.9, "close: разделение ≤ 0.9 а.е.")
		} else {
			assert.GreaterOrEqual(t, sep, 100.0, "wide: разделение ≥ 100 а.е.")
			assert.LessOrEqual(t, sep, 3000.0, "wide: разделение ≤ 3000 а.е.")
		}
	}
	assert.Greater(t, binary, 500, "в выборке должны быть двойные/кратные")
}

// TestBinaryMassInvariant — сортировка «главная = массивнее» (35c §ТЗ п.2):
// M_главной ≥ M_компаньона для всех binary/multiple (0 нарушений).
func TestBinaryMassInvariant(t *testing.T) {
	g := NewGenerator(&Config{Seed: 33, WorldCount: 60000, MapSize: 60000, MinDist: 40, WorldSpread: 0})
	worlds := g.generateWorldsRandom(40)

	binary := 0
	for _, w := range worlds {
		if w.SystemType != "binary" && w.SystemType != "multiple" {
			continue
		}
		binary++
		require.NotNil(t, w.StellarMods)
		require.NotNil(t, w.StellarMass, "у главной есть масса")
		require.NotNil(t, w.StellarMods.CompanionMass, "у компаньона есть масса")
		assert.GreaterOrEqual(t, *w.StellarMass, *w.StellarMods.CompanionMass,
			"M_главной ≥ M_компаньона (%s %g vs %s %g)",
			w.SpectralClass, *w.StellarMass, w.StellarMods.Companion, *w.StellarMods.CompanionMass)
	}
	assert.Greater(t, binary, 500)
}

// TestMultipleForcedWideWithExtraCompanion — multiple принудительно wide
// (реш. №3а) + extra_companions[0] с sep_au ∈ [1000, 10000] и ≥ 3×
// разделения внутренней пары (иерархия 3×).
func TestMultipleForcedWideWithExtraCompanion(t *testing.T) {
	g := NewGenerator(&Config{Seed: 37, WorldCount: 80000, MapSize: 80000, MinDist: 40, WorldSpread: 0})
	worlds := g.generateWorldsRandom(40)

	multi := 0
	for _, w := range worlds {
		if w.SystemType != "multiple" {
			continue
		}
		multi++
		require.NotNil(t, w.StellarMods)
		mods := w.StellarMods

		assert.Equal(t, "wide", mods.BinaryType, "кратная — принудительно wide (реш. №3а)")
		require.NotNil(t, mods.CompanionSepAU)
		require.Len(t, mods.ExtraCompanions, 1, "у кратной ровно 1 внешний компаньон")

		ec := mods.ExtraCompanions[0]
		assert.NotEmpty(t, ec.SpectralClass, "спектр внешнего компаньона задан")
		assert.GreaterOrEqual(t, ec.SepAU, 1000.0, "внешний: sep ≥ 1000 а.е.")
		assert.LessOrEqual(t, ec.SepAU, 10000.0, "внешний: sep ≤ 10000 а.е.")
		assert.GreaterOrEqual(t, ec.SepAU, 3.0*(*mods.CompanionSepAU),
			"внешний: sep ≥ 3× разделения внутренней пары (иерархия 3×)")
	}
	assert.Greater(t, multi, 150, "в выборке должны быть кратные")
}

// TestMultipleMassHierarchy — пакет на 3 компонента (35c §ТЗ п.1): масса
// убывает наружу — M_главной ≥ M_компаньона ≥ M_внешнего (0 нарушений на
// выборке), плюс иерархия sep внешнего ≥ 3× sep компаньона.
func TestMultipleMassHierarchy(t *testing.T) {
	g := NewGenerator(&Config{Seed: 39, WorldCount: 90000, MapSize: 90000, MinDist: 40, WorldSpread: 0})
	worlds := g.generateWorldsRandom(40)

	multi := 0
	for _, w := range worlds {
		if w.SystemType != "multiple" {
			continue
		}
		multi++
		require.NotNil(t, w.StellarMods)
		mods := w.StellarMods
		require.NotNil(t, w.StellarMass, "у главной есть масса")
		require.NotNil(t, mods.CompanionMass, "у компаньона есть масса")
		require.NotNil(t, mods.CompanionSepAU)
		require.Len(t, mods.ExtraCompanions, 1)
		ec := mods.ExtraCompanions[0]
		require.NotNil(t, ec.Mass, "у внешнего есть масса")

		assert.GreaterOrEqual(t, *w.StellarMass, *mods.CompanionMass,
			"M_главной ≥ M_компаньона (%s %g vs %s %g)",
			w.SpectralClass, *w.StellarMass, mods.Companion, *mods.CompanionMass)
		assert.GreaterOrEqual(t, *mods.CompanionMass, *ec.Mass,
			"M_компаньона ≥ M_внешнего (%s %g vs %s %g)",
			mods.Companion, *mods.CompanionMass, ec.SpectralClass, *ec.Mass)
		assert.GreaterOrEqual(t, ec.SepAU, 3.0*(*mods.CompanionSepAU),
			"sep внешнего ≥ 3× sep компаньона (иерархия 3×)")
	}
	assert.Greater(t, multi, 150, "в выборке должны быть кратные")
}

// TestMultiplePackagePromotesHeavyComponent — 35c §ТЗ п.1: изначальный класс
// мира тусклее/легче — пакет продвигает массивный компонент в главные. Мир
// задан как M (масса 0.1 — низ диапазона M); в пакете почти всегда выпадает
// более массивный компонент → класс мира меняется; инвариант иерархии при
// этом не нарушается ни в одном прогоне.
func TestMultiplePackagePromotesHeavyComponent(t *testing.T) {
	const runs = 2000
	promoted := 0
	for i := 0; i < runs; i++ {
		g := NewGenerator(&Config{Seed: int64(1000 + i), WorldCount: 60000, MapSize: 60000, MinDist: 40, WorldSpread: 0})
		mainMass := 0.1 // низ диапазона M (0.1–0.6)
		w := &models.World{
			SpectralClass: "M",
			Temperature:   3000,
			StellarMass:   &mainMass,
		}
		mods := &models.StellarMods{}
		g.rollMultiple(w, mods)

		if w.SpectralClass != "M" {
			promoted++
		}
		require.NotNil(t, w.StellarMass)
		require.NotNil(t, mods.CompanionMass)
		require.NotNil(t, mods.CompanionSepAU)
		require.Len(t, mods.ExtraCompanions, 1)
		require.NotNil(t, mods.ExtraCompanions[0].Mass)

		assert.GreaterOrEqual(t, *w.StellarMass, *mods.CompanionMass,
			"итерация %d: M_главной ≥ M_компаньона", i)
		assert.GreaterOrEqual(t, *mods.CompanionMass, *mods.ExtraCompanions[0].Mass,
			"итерация %d: M_компаньона ≥ M_внешнего", i)
		assert.GreaterOrEqual(t, mods.ExtraCompanions[0].SepAU, 3.0*(*mods.CompanionSepAU),
			"итерация %d: sep внешнего ≥ 3× sep компаньона", i)
	}
	assert.Greater(t, promoted, runs/2,
		"массивный компонент должен продвигаться в главные (%d из %d)", promoted, runs)
}

// TestRollStarModsSingleNoBinaryParams — не-регрессия single (35c §ТЗ п.4):
// rollStarMods для одиночной звезды не проставляет параметры двойной.
func TestRollStarModsSingleNoBinaryParams(t *testing.T) {
	g := NewGenerator(&Config{Seed: 43, WorldCount: 60000, MapSize: 60000, MinDist: 40, WorldSpread: 0})
	mass := 1.0
	w := &models.World{SpectralClass: "G", Temperature: 5700, StellarMass: &mass}
	g.rollStarMods(w, "single", "G")

	if w.StellarMods != nil {
		assert.Empty(t, w.StellarMods.BinaryType, "у одиночной звезды нет binary_type")
		assert.Empty(t, w.StellarMods.Companion, "у одиночной звезды нет компаньона")
		assert.Nil(t, w.StellarMods.CompanionMass)
		assert.Nil(t, w.StellarMods.CompanionTemp)
		assert.Nil(t, w.StellarMods.CompanionSepAU)
		assert.Empty(t, w.StellarMods.ExtraCompanions, "у одиночной звезды нет внешних компаньонов")
	}
}

// TestMassiveCompanionSwap — «компаньон массивнее главной» → swap целиком:
// классы, температуры, массы (главной — в stellar_mass, компаньона — в
// companion_mass), companion-спектр.
func TestMassiveCompanionSwap(t *testing.T) {
	mainMass := 0.2
	compMass := 1.1
	mainTemp := 3000
	compTemp := 5700

	w := &models.World{
		SpectralClass: "M", // масса 0.2 — лёгкая главная
		Temperature:   mainTemp,
		StellarMass:   &mainMass,
	}
	mods := &models.StellarMods{
		Companion:     "G", // масса 1.1 — массивный компаньон
		CompanionMass: &compMass,
		CompanionTemp: &compTemp,
	}

	sortMassiveFirst(w, mods)

	assert.Equal(t, "G", w.SpectralClass, "главная становится массивным классом")
	assert.Equal(t, "M", mods.Companion, "компаньон становится лёгким классом")
	assert.Equal(t, compTemp, w.Temperature, "температуры поменялись местами")
	require.NotNil(t, mods.CompanionTemp)
	assert.Equal(t, mainTemp, *mods.CompanionTemp)
	require.NotNil(t, w.StellarMass)
	assert.InDelta(t, 1.1, *w.StellarMass, 1e-9, "масса главной = прежняя масса компаньона")
	require.NotNil(t, mods.CompanionMass)
	assert.InDelta(t, 0.2, *mods.CompanionMass, 1e-9, "масса компаньона = прежняя масса главной")
	// Инвариант после swap.
	assert.LessOrEqual(t, *mods.CompanionMass, *w.StellarMass)
}

// TestMassiveFirstAlreadySortedNoSwap — если главная уже массивнее — swap не
// происходит (ничего не трогаем).
func TestMassiveFirstAlreadySortedNoSwap(t *testing.T) {
	mainMass := 1.1
	compMass := 0.2
	mainTemp := 5700
	compTemp := 3000

	w := &models.World{
		SpectralClass: "G",
		Temperature:   mainTemp,
		StellarMass:   &mainMass,
	}
	mods := &models.StellarMods{
		Companion:     "M",
		CompanionMass: &compMass,
		CompanionTemp: &compTemp,
	}

	sortMassiveFirst(w, mods)

	assert.Equal(t, "G", w.SpectralClass)
	assert.Equal(t, "M", mods.Companion)
	assert.Equal(t, mainTemp, w.Temperature)
	assert.InDelta(t, 1.1, *w.StellarMass, 1e-9)
	assert.InDelta(t, 0.2, *mods.CompanionMass, 1e-9)
}

// TestBrighterButLighterCompSwapsByMass — «ярче, но легче» (35c): A (L=10)
// ярче F (L=2), но A может быть легче F — приоритет у массы, главной
// становится F (старая сортировка по светимости не свопнула бы).
func TestBrighterButLighterCompSwapsByMass(t *testing.T) {
	mainMass := 1.5
	compMass := 1.6
	mainTemp := 8700 // A-класс
	compTemp := 6500 // F-класс

	w := &models.World{
		SpectralClass: "A",
		Temperature:   mainTemp,
		StellarMass:   &mainMass,
	}
	mods := &models.StellarMods{
		Companion:     "F",
		CompanionMass: &compMass,
		CompanionTemp: &compTemp,
	}

	sortMassiveFirst(w, mods)

	assert.Equal(t, "F", w.SpectralClass, "главной становится массивный класс, а не яркий")
	assert.Equal(t, "A", mods.Companion)
	assert.Equal(t, compTemp, w.Temperature)
	require.NotNil(t, w.StellarMass)
	assert.InDelta(t, 1.6, *w.StellarMass, 1e-9)
	require.NotNil(t, mods.CompanionMass)
	assert.InDelta(t, 1.5, *mods.CompanionMass, 1e-9)
}

// TestCompanionSepLogUniformDistribution — разделение лог-равномерно:
// логарифм sep равномерно распределён в диапазоне (среднее логарифма ≈
// середине диапазона).
func TestCompanionSepLogUniformDistribution(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const n = 20000
	sum := 0.0
	for i := 0; i < n; i++ {
		s := logUniform(rng, 0.05, 0.9)
		sum += math.Log(s)
	}
	meanLog := sum / n
	expected := (math.Log(0.05) + math.Log(0.9)) / 2
	assert.InDelta(t, expected, meanLog, 0.05, "лог-равномерность close")
}