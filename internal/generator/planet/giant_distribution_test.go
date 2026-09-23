// internal/generator/planet/giant_distribution_test.go
//
// Тесты распределения планет и регрессии «экзотика без гигантов».
// Тесты гигантов переписаны под перелив массы (спека
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны §12.2: O3/O4/O5/O6/O8)
// и живут в overflow_giant_test.go; здесь остаются mean-модель и защита
// сверхгигантов/экзотики (O18).
package planet

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// testMetallicity — металличность [Fe/H] как в galaxy.rollMetallicity
// (N(0, 0.3), кламп [−0.8, +0.5]) — для статистических тестов перелива.
func testMetallicity(rng *rand.Rand) float64 {
	met := rng.NormFloat64() * 0.3
	if met < -0.8 {
		met = -0.8
	}
	if met > 0.5 {
		met = 0.5
	}
	return met
}

// ==================== НОВЫЕ MEAN (§10.2 п.5) ====================

// TestPlanetCountNewMeans — mean-модель на новых дефолтах (спека 2026-09-20
// §5.1/§10.2): G/F ≈ 6.5, K/M ≈ 5.5, L/T/Y ≈ 1 (n ≡ 1), остатки ≤ 1, максимум
// ≤ 8; у M/G/K максимум ≥ 4.
func TestPlanetCountNewMeans(t *testing.T) {
	g := NewGenerator(nil, 17)
	g.means = DefaultPlanetMeans()

	assert.InDelta(t, 6.5, planetCountAverage(g, "G", 20000), 0.1, "G ≈ 6.5")
	assert.InDelta(t, 6.5, planetCountAverage(g, "F", 20000), 0.1, "F ≈ 6.5")
	assert.InDelta(t, 5.5, planetCountAverage(g, "K", 20000), 0.1, "K ≈ 5.5")
	assert.InDelta(t, 5.5, planetCountAverage(g, "M", 20000), 0.1, "M ≈ 5.5")
	assert.InDelta(t, 1.0, planetCountAverage(g, "L", 20000), 0.01, "L ≈ 1 (n ≡ 1)")
	assert.InDelta(t, 1.0, planetCountAverage(g, "T", 20000), 0.01, "T ≈ 1")
	assert.InDelta(t, 1.0, planetCountAverage(g, "Y", 20000), 0.01, "Y ≈ 1")

	// Остатки: максимум генератора 1 («чаще 0», 99.2.4 §5.2).
	for i := 0; i < 1000; i++ {
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "black_hole"}), 1)
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "neutron"}), 1)
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "white_dwarf"}), 1)
	}

	// Максимумы: ≤ 8 у всех, ≥ 4 у M/G/K.
	for _, cls := range []string{"M", "G", "K"} {
		maxN := 0
		for i := 0; i < 10000; i++ {
			n := g.determinePlanetCount(cls)
			assert.LessOrEqual(t, n, 8, "%s: потолок 8", cls)
			if n > maxN {
				maxN = n
			}
		}
		assert.GreaterOrEqual(t, maxN, 4, "%s: максимум ≥ 4", cls)
	}
}

// ==================== СВЕРХГИГАНТЫ БЕЗ ГИГАНТА (O18, регрессия П1) ====================

// TestSupergiantNoGasGiant — сверхгиганты («прочая экзотика», фаза I +
// подтип lbv/wr, 99.2.4 §4.4) не получают газового гиганта (99.2.4 §5.3):
// перелив на них не распространяется (спека 2026-09-23 §4.2). Проверка на
// мировом потоке с принудительным count = 1.
func TestSupergiantNoGasGiant(t *testing.T) {
	g := NewGenerator(nil, 21)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(8)

	for _, cls := range []string{"O", "B", "A"} {
		for _, subtype := range []string{"lbv", "wr"} {
			w := WorldInfo{
				ID: "w", Name: "W", SpectralClass: cls, StarType: "star",
				Mods: &models.StellarMods{Phase: "I", Subtype: subtype},
			}
			planets := 0
			const n = 1000
			for i := 0; i < n; i++ {
				buf.reset()
				g.generateWorldWithCountIntoBuffer(w, 1, buf)
				for _, row := range buf.planetRows {
					fields := row.([]interface{})
					var data map[string]interface{}
					require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
					planets++
					assert.NotEqual(t, true, data["is_gas_giant"],
						"%s %s: сверхгигант без газового гиганта", cls, subtype)
					assert.NotEqual(t, true, data["is_mini_neptune"],
						"%s %s: сверхгигант без мини-нептуна", cls, subtype)
				}
			}
			assert.Greater(t, planets, 0, "%s %s: планеты сверхгиганта генерируются (тест не пустой)", cls, subtype)
		}
	}
}
