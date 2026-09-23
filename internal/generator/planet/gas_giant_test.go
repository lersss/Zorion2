// internal/generator/planet/gas_giant_test.go
//
// Тесты газовых гигантов (99.2.15 с переанкеровкой кривой решением создателя
// 2026-09-23 — спека 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны §7.5):
// кривая масса→радиус на реальных анкерах, производные ρ = M/R³ и g = M/R²,
// усечённый логнормаль массы P-ветки (gasGiantMass жив для двойных, §9.2).
// Масса пути перелива (аккреция) — O9/O10 в overflow_giant_test.go.
package planet

import (
	"encoding/json"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Переанкеренная кривая M→R проходит через реальные опорные точки:
// 16 → 3.7 (стык с мини-нептуном), 95 → 9.4 (Сатурн), 317.8 → 11.2 (Юпитер),
// плато до 4131. Плотность и гравитация — производные: ρ = M/R³, g = M/R².
func TestGasGiantCurveReferencePoints(t *testing.T) {
	assert.InDelta(t, GasGiantRadiusMin, GasGiantRadius(GasGiantMassMin), 0.01, "низ кривой 16 → 3.7")
	assert.InDelta(t, 9.4, GasGiantRadius(95), 0.01, "Сатурн 95 → 9.4")
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassRef), "Юпитер 317.8 → 11.2")
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassMax), "плато до 4131")

	// Нептун (17.1) — проверка стыка: на кривой гигантов R ≈ 3.83 (промах 1.3%).
	assert.InDelta(t, 3.83, GasGiantRadius(17.1), 0.02, "Нептун на кривой гигантов")

	// Производные у низа: ρ(16) ≈ 0.316, g(16) ≈ 1.17 (было 0.077/0.46).
	rho0 := GasGiantMassMin / math.Pow(GasGiantRadiusMin, 3)
	assert.InDelta(t, 0.316, rho0, 0.005, "ρ(16) — фактический минимум плотности")
	assert.InDelta(t, 1.17, computeGravity(GasGiantMassMin, GasGiantRadiusMin), 0.02, "g(16)")

	// Сатурн: R = 9.4, ρ = 95/9.4³ = 0.114 — минимум плотности кривой.
	rhoSaturn := 95.0 / math.Pow(9.4, 3)
	assert.InDelta(t, 0.1144, rhoSaturn, 0.002, "ρ(Сатурн)")

	// 13 MJ (4131): ρ = 4131/11.2³ = 2.9403 ≈ «~2.9»; g = 4131/11.2² = 32.93.
	rho := GasGiantMassMax / math.Pow(GasGiantRadiusMax, 3)
	assert.InDelta(t, 2.9403, rho, 0.01)
	assert.InDelta(t, 32.93, computeGravity(GasGiantMassMax, GasGiantRadiusMax), 0.1)

	// 1 MJ (317.8): R = 11.2, ρ = 0.226, g = 2.53 — сходится с пресетом GIANT.
	rho1 := GasGiantMassRef / math.Pow(GasGiantRadiusMax, 3)
	assert.InDelta(t, 0.226, rho1, 0.005)
	assert.InDelta(t, 2.53, computeGravity(GasGiantMassRef, GasGiantRadiusMax), 0.01)
}

// Инварианты кривой на всём диапазоне: R не убывает, значения конечны.
// Плотность и гравитация на переанкеренной кривой НЕ монотонны (реальные
// объекты: ρ(Нептун) = 0.316 > ρ(Сатурн) = 0.114 < ρ(Юпитер) = 0.226) —
// строгий рост из старой двухточечной кривой снят вместе с анкером 5.9.
func TestGasGiantCurveMonotonic(t *testing.T) {
	prevR := 0.0
	steps := 200.0
	for i := 0; i <= int(steps); i++ {
		m := GasGiantMassMin + float64(i)*(GasGiantMassMax-GasGiantMassMin)/steps
		r := GasGiantRadius(m)
		rho := m / math.Pow(r, 3)
		gv := computeGravity(m, r)
		require.False(t, math.IsNaN(r) || math.IsInf(r, 0), "R конечен при M=%.2f", m)
		require.GreaterOrEqual(t, r, prevR-1e-9, "R не убывает при M=%.2f", m)
		require.Greater(t, rho, 0.0, "ρ > 0 при M=%.2f", m)
		require.Greater(t, gv, 0.0, "g > 0 при M=%.2f", m)
		prevR = r
	}
	// Плато насыщения выше 1 MJ.
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassRef*1.5))
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassMax))
}

// Распределение масс P-ветки двойных (gasGiantMass жив, §9.2): усечённое
// логнормальное (медиана 317.8 M⊕, σ = 0.9 декады, [16, 4131]) через
// пересэмплинг — НЕ кламп. Доли: >1000 ≈ 22%, <100 ≈ 26% (±5 п.п.).
func TestGasGiantMassDistribution(t *testing.T) {
	g := NewGenerator(nil, 7)
	const n = 1000
	masses := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		m := g.gasGiantMass()
		require.GreaterOrEqual(t, m, GasGiantMassMin, "масса ниже нижней границы")
		require.LessOrEqual(t, m, GasGiantMassMax, "масса выше верхней границы")
		masses = append(masses, m)
	}

	// Медиана ≈ 317.8 (1 MJ).
	sorted := append([]float64(nil), masses...)
	sort.Float64s(sorted)
	median := sorted[n/2]
	assert.InDelta(t, GasGiantMassRef, median, 100, "медиана масс %.1f", median)

	over, under, atMax := 0, 0, 0
	for _, m := range masses {
		if m > 1000 {
			over++
		}
		if m < 100 {
			under++
		}
		if m == GasGiantMassMax {
			atMax++
		}
	}
	assert.InDelta(t, 0.22, float64(over)/n, 0.05, "доля >1000 M⊕")
	assert.InDelta(t, 0.26, float64(under)/n, 0.05, "доля <100 M⊕")
	assert.Zero(t, atMax, "клампа нет: масса не должна пайковаться ровно на 4131")
}

// Сгенерированный гигант (легаси-путь, масса из gasGiantMass) честен по
// построению: R = GasGiantRadius(M), ρ = M/R³, g = M/R², масса в [16, 4131].
func TestGenerateGasGiantHonest(t *testing.T) {
	g := NewGenerator(nil, 99)
	for i := 0; i < 200; i++ {
		pd := g.generateGasGiant("w", "World", 4, stellarParamsFromClass("G", 0, g.rng), nil, 0, 0)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(pd.Data, &data))

		mass := data["mass"].(float64)
		size := data["size"].(float64)
		density := data["density"].(float64)
		gravity := data["gravity"].(float64)

		require.GreaterOrEqual(t, mass, GasGiantMassMin, "масса гиганта ниже минимума")
		require.LessOrEqual(t, mass, GasGiantMassMax, "масса гиганта выше максимума")
		assert.InDelta(t, GasGiantRadius(mass), size, 1e-9, "size != gasGiantRadius(mass)")
		assert.InDelta(t, mass/(size*size*size), density, 1e-9, "ρ != M/R³")
		assert.InDelta(t, mass/(size*size), gravity, 1e-9, "g != M/R²")
	}
}
