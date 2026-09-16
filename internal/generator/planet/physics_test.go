// internal/generator/planet/physics_test.go
// Юнит-тесты физики, ядра и спутниковой термодинамики.
package planet

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ==================== РАВНОВЕСНАЯ ТЕМПЕРАТУРА ====================

func TestComputeEquilibriumTemp(t *testing.T) {
	// L=1, r=1 → 278.7
	assert.InDelta(t, 278.7, computeEquilibriumTemp(1, 1), 0.01)

	// L=16 → 278.7 * 16^0.25 = 278.7 * 2 = 557.4
	assert.InDelta(t, 557.4, computeEquilibriumTemp(16, 1), 0.01)

	// L раз в 16 раз больше → T в 2 раза выше.
	assert.InDelta(t,
		computeEquilibriumTemp(1, 1)*2,
		computeEquilibriumTemp(16, 1), 0.01)

	// r в 4 раза больше → T в 2 раза ниже.
	assert.InDelta(t,
		computeEquilibriumTemp(1, 1)/2,
		computeEquilibriumTemp(1, 4), 0.01)
}

func TestComputeEquilibriumTempDefaults(t *testing.T) {
	// L≤0 → 1.0 (L=1, r=1 → 278.7)
	assert.InDelta(t, 278.7, computeEquilibriumTemp(0, 1), 0.01)
	assert.InDelta(t, 278.7, computeEquilibriumTemp(-5, 1), 0.01)

	// r≤0 → 0.4 а.е.
	assert.InDelta(t, 278.7/math.Sqrt(0.4), computeEquilibriumTemp(1, 0), 0.01)
}

// ==================== АЛЬБЕДО ====================

func TestComputeAlbedo(t *testing.T) {
	// Пустая композиция → среднее по умолчанию.
	assert.Equal(t, 0.3, computeAlbedo(Composition{}))

	// Ледники — самые светлые (0.65).
	assert.Equal(t, 0.65, computeAlbedo(Composition{SurfaceGlaciers: 100}))

	// Океаны — тёмные (0.06).
	assert.Equal(t, 0.06, computeAlbedo(Composition{SurfaceOceans: 100}))

	// Неизвестная форма → 0.3.
	assert.Equal(t, 0.3, computeAlbedo(Composition{"неизвестная_форма": 100}))
}

// ==================== ГРАВИТАЦИЯ ====================

func TestComputeGravity(t *testing.T) {
	// Земля: M=1, R=1 → 1g.
	assert.Equal(t, 1.0, computeGravity(1, 1))
	// M=4, R=2 → 1g (та же поверхностная гравитация).
	assert.Equal(t, 1.0, computeGravity(4, 2))
	// Меньше радиус при той же массе → сильнее.
	assert.Equal(t, 4.0, computeGravity(4, 1))
	// R ≤ 0 → защита от деления на ноль (R=1).
	assert.Equal(t, 9.0, computeGravity(9, 0))
}

// ==================== ОРБИТЫ И СВЕТИМОСТЬ ====================

func TestOrbitRadiusByIndex(t *testing.T) {
	assert.InDelta(t, 0.4*1.7, orbitRadiusByIndex(1), 0.0001)
	assert.InDelta(t, 0.4*1.7*1.7, orbitRadiusByIndex(2), 0.0001)
	// Индекс < 1 → приравнивается к 1.
	assert.InDelta(t, orbitRadiusByIndex(1), orbitRadiusByIndex(0), 0.0001)
}

func TestLuminosityBySpectral(t *testing.T) {
	assert.Equal(t, 1.0, luminosityBySpectral("G"))
	assert.Equal(t, 0.01, luminosityBySpectral("M"))
	assert.Equal(t, 1000.0, luminosityBySpectral("B"))
	// Неизвестный класс → 1.0.
	assert.Equal(t, 1.0, luminosityBySpectral("X"))
}

// ==================== ТЕМПЕРАТУРА СПУТНИКОВ ====================

func TestComputeSatelliteTempCeiling(t *testing.T) {
	// Спутник никогда не горячее гиганта + 50 K.
	for giant := 20.0; giant <= 2500; giant += 137.3 {
		temp := computeSatelliteTemp(giant, 1)
		assert.LessOrEqual(t, temp, giant+50.001,
			"спутник горячее гиганта %.0f: %.0f", giant, temp)
	}

	// Холодный гигант: ceiling срабатывает.
	assert.Equal(t, 100.0, computeSatelliteTemp(50, 1))  // raw 115 → ceiling 100
	assert.Equal(t, 60.0, computeSatelliteTemp(10, 1))   // raw 103 → ceiling 60

	// orbitIndex < 1 → 1.
	assert.Equal(t, computeSatelliteTemp(500, 1), computeSatelliteTemp(500, 0))
}

func TestComputeSatelliteTempTidalDecays(t *testing.T) {
	// Внутренний нагрев постоянен, приливный спадает ∝ 1/orbit^3.
	// Чем дальше орбита — тем холоднее.
	r1 := computeSatelliteTemp(300, 1)
	r2 := computeSatelliteTemp(300, 3)
	assert.Greater(t, r1, r2, "внутренний спутник должен быть горячее")
}

func TestComputeSatelliteTempAbsoluteBounds(t *testing.T) {
	for orbit := 1; orbit <= 20; orbit++ {
		temp := computeSatelliteTemp(2500, orbit)
		assert.LessOrEqual(t, temp, TempAbsoluteMax)
		temp = computeSatelliteTemp(20, orbit)
		assert.GreaterOrEqual(t, temp, TempAbsoluteMin)
	}
}

// ==================== ВОДА СПУТНИКОВ ====================

func TestComputeSatelliteWaterRanges(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		// > 400 K — вода испарилась.
		assert.Equal(t, 0.0, computeSatelliteWater(450, rng))

		// 250–400 K — жидкая вода 10–50%.
		w := computeSatelliteWater(300, rng)
		assert.GreaterOrEqual(t, w, 10.0)
		assert.Less(t, w, 50.0)

		// 180–250 K — лёд + подлёдный океан 30–80%.
		w = computeSatelliteWater(200, rng)
		assert.GreaterOrEqual(t, w, 30.0)
		assert.Less(t, w, 80.0)

		// < 180 K — сплошной лёд 20–80%.
		w = computeSatelliteWater(100, rng)
		assert.GreaterOrEqual(t, w, 20.0)
		assert.Less(t, w, 80.0)
	}
}

// ==================== ЯДРО ====================

func TestCoreFlags(t *testing.T) {
	assert.True(t, (Core{Type: CoreMetallic}).IsMetallic())
	assert.False(t, (Core{Type: CoreSilicate}).IsMetallic())

	assert.True(t, (Core{Activity: 41}).IsActive())
	assert.False(t, (Core{Activity: 40}).IsActive())

	assert.True(t, (Core{Radioactivity: 51}).IsRadioactive())
	assert.False(t, (Core{Radioactivity: 50}).IsRadioactive())
}

func TestCoreHeatContribution(t *testing.T) {
	// Activity=10 → 15 K при массдоле 30% и возрасте 0.
	assert.InDelta(t, 15.0,
		(Core{Activity: 10, MassPercent: 30, Age: 0}).HeatContribution(), 0.001)

	// Radioactivity=20 → 50 K.
	assert.InDelta(t, 50.0,
		(Core{Radioactivity: 20, MassPercent: 30, Age: 0}).HeatContribution(), 0.001)

	// Старое ядро: age_factor ограничен 0.15.
	assert.InDelta(t, 50*0.15,
		(Core{Radioactivity: 20, MassPercent: 30, Age: 30}).HeatContribution(), 0.001)

	// Микроскопическое ядро: mass_factor ограничен 0.1.
	assert.InDelta(t, 50*0.1,
		(Core{Radioactivity: 20, MassPercent: 1, Age: 0}).HeatContribution(), 0.001)

	// Нулевое ядро — ноль.
	assert.Equal(t, 0.0, (Core{}).HeatContribution())
}

func TestDetermineCoreTypeBigPlanet(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	metallic := 0
	for i := 0; i < 200; i++ {
		typ := determineCoreType(10, rng) // > massGasGiant
		assert.Contains(t, []string{CoreMetallic, CoreSilicate}, typ,
			"суперземля не может иметь ледяное/экзотическое ядро")
		if typ == CoreMetallic {
			metallic++
		}
	}
	assert.Greater(t, metallic, 150, "металлическое ядро должно преобладать (95%)")
}

func TestDetermineCoreTypeSmallPlanet(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 200; i++ {
		typ := determineCoreType(0.1, rng) // малая планета
		assert.Contains(t, []string{CoreSilicate, CoreIce, CoreExotic}, typ,
			"малая планета не может иметь металлическое ядро")
	}
}

func TestDetermineCoreMassPercentRanges(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 100; i++ {
		p := determineCoreMassPercent(10, rng) // газовый гигант 20–40
		assertInDeltaRange(t, 20, 40, p, "газовый гигант")

		p = determineCoreMassPercent(1, rng) // землеподобная 25–40
		assertInDeltaRange(t, 25, 40, p, "землеподобная")

		p = determineCoreMassPercent(0.1, rng) // малая 5–25
		assertInDeltaRange(t, 5, 25, p, "малая")
	}
}

func TestDetermineCoreActivityByArchetype(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for i := 0; i < 100; i++ {
		a := determineCoreActivity("экстремальный", nil, rng)
		assertInDeltaRange(t, 70, 100, a, "экстремальный")

		a = determineCoreActivity("холодный", nil, rng)
		assertInDeltaRange(t, 5, 25, a, "холодный")

		a = determineCoreActivity("unknown", nil, rng)
		assertInDeltaRange(t, 20, 50, a, "unknown (дефолт)")
	}
}

func TestDetermineCoreActivityMagmaBoost(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	sub := Composition{SubterrainMagmaChambers: 10, SubterrainMagmaticRocks: 20}
	for i := 0; i < 100; i++ {
		a := determineCoreActivity("холодный", sub, rng)
		// Базовые 5–25 + 10×1.5 + 20×0.5 = +25 → 30–50.
		assertInDeltaRange(t, 30, 50, a, "холодный с магматическими структурами")
	}
}

func TestDetermineCoreRadioactivity(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	for i := 0; i < 100; i++ {
		r := determineCoreRadioactivity(Composition{SubterrainRadioactiveZones: 10}, rng)
		// 60 + [0,10) → 60–70
		assertInDeltaRange(t, 60, 70, r, "10% радиоактивных зон")

		r = determineCoreRadioactivity(nil, rng)
		// 0 + [0,10) → 0–10
		assertInDeltaRange(t, 0, 10, r, "без радиоактивных зон")
	}
}

// ==================== ХЕЛПЕР ====================

// assertInDeltaRange проверяет a ≤ v ≤ b.
func assertInDeltaRange(t *testing.T, a, b, v float64, msg string) {
	t.Helper()
	assert.GreaterOrEqual(t, v, a, "%s: %.2f < %v", msg, v, a)
	assert.LessOrEqual(t, v, b, "%s: %.2f > %v", msg, v, b)
}