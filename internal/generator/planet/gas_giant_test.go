// internal/generator/planet/gas_giant_test.go
//
// Тесты честных газовых гигантов (99.2.15): кривая масса→радиус с насыщением
// (опорные точки эталона), логнормальное распределение масс с пересэмплингом,
// производные ρ = M/R³ и g = M/R² у сгенерированного гиганта.
package planet

import (
	"encoding/json"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Кривая M→R проходит через опорные точки эталона: P1=(15.9; 5.9),
// P2=(317.8; 11.2), P3=(4131; 11.2). Плотность и гравитация — производные:
// ρ = M/R³, g = M/R².
func TestGasGiantCurveReferencePoints(t *testing.T) {
	assert.InDelta(t, GasGiantRadiusMin, GasGiantRadius(GasGiantMassMin), 0.01)
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassRef))
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassMax))

	// 13 MJ (4131): ρ = 4131/11.2³ = 2.9403 ≈ «~2.9»; g = 4131/11.2² = 32.93.
	rho := GasGiantMassMax / math.Pow(GasGiantRadiusMax, 3)
	assert.InDelta(t, 2.9403, rho, 0.01)
	assert.InDelta(t, 32.93, computeGravity(GasGiantMassMax, GasGiantRadiusMax), 0.1)

	// 1 MJ (317.8): R=11.2, ρ=0.226, g=2.53 — сходится с пресетом GIANT.
	rho1 := GasGiantMassRef / math.Pow(GasGiantRadiusMax, 3)
	assert.InDelta(t, 0.226, rho1, 0.005)
	assert.InDelta(t, 2.53, computeGravity(GasGiantMassRef, GasGiantRadiusMax), 0.01)

	// 0.05 MJ (15.9): R=5.9, ρ=0.077.
	rho0 := GasGiantMassMin / math.Pow(GasGiantRadiusMin, 3)
	assert.InDelta(t, 0.077, rho0, 0.005)
}

// Инварианты кривой на всём диапазоне: R не убывает, ρ и g строго растут.
func TestGasGiantCurveMonotonic(t *testing.T) {
	prevR, prevRho, prevG := 0.0, 0.0, 0.0
	steps := 200.0
	for i := 0; i <= int(steps); i++ {
		m := GasGiantMassMin + float64(i)*(GasGiantMassMax-GasGiantMassMin)/steps
		r := GasGiantRadius(m)
		rho := m / math.Pow(r, 3)
		gv := computeGravity(m, r)
		require.GreaterOrEqual(t, r, prevR-1e-9, "R не убывает при M=%.2f", m)
		require.Greater(t, rho, prevRho, "ρ строго растёт при M=%.2f", m)
		require.Greater(t, gv, prevG, "g строго растёт при M=%.2f", m)
		prevR, prevRho, prevG = r, rho, gv
	}
}

// Распределение масс: усечённое логнормальное (медиана 317.8 M⊕, σ = 0.9
// декады, [15.9, 4131]) через пересэмплинг — НЕ кламп. Доли усечённого
// распределения: >1000 ≈ 22%, <100 ≈ 26% (допуск ±5 п.п. — случайность).
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

// Сгенерированный гигант честен по построению: R = gasGiantRadius(M),
// ρ = M/R³, g = M/R², масса в [15.9, 4131] — ни один показатель не вылезает
// за рамки генератора.
func TestGenerateGasGiantHonest(t *testing.T) {
	g := NewGenerator(nil, 99)
	for i := 0; i < 200; i++ {
		pd := g.generateGasGiant("w", "World", 4, "G", 5.0)
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