// internal/resource/bias_test.go — весовой выбор категорий (59a §8 P2).
package resource

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPickCategoryWeighted(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	cats := []string{"mineral", "rare", "fuel"}
	bias := map[string]float64{"rare": 10.0}

	counts := map[string]int{}
	const n = 100_000
	for i := 0; i < n; i++ {
		counts[pickCategory(cats, rng, bias)]++
	}
	// rare: 10/12 ≈ 0.833, mineral/fuel: 1/12 ≈ 0.083.
	assert.InDelta(t, 10.0/12.0, float64(counts["rare"])/n, 0.02)
	assert.InDelta(t, 1.0/12.0, float64(counts["mineral"])/n, 0.02)
	assert.InDelta(t, 1.0/12.0, float64(counts["fuel"])/n, 0.02)
}

func TestPickCategoryUniformWithoutBias(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	cats := []string{"mineral", "rare", "fuel"}
	counts := map[string]int{}
	const n = 100_000
	for i := 0; i < n; i++ {
		counts[pickCategory(cats, rng, nil)]++
	}
	for _, c := range cats {
		assert.InDelta(t, 1.0/3.0, float64(counts[c])/n, 0.02, "категория %s", c)
	}
}

func TestPickCategoryBiasDoesNotAddCategory(t *testing.T) {
	// Bias упоминает категорию, которой нет в наборе — она не появится.
	rng := rand.New(rand.NewSource(3))
	cats := []string{"mineral"}
	bias := map[string]float64{"water": 100.0}
	for i := 0; i < 1000; i++ {
		assert.Equal(t, "mineral", pickCategory(cats, rng, bias))
	}
}

func TestGenerateResources_ResourceBiasDominates(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	// "горы" → mineral+rare; bias rare 10 → редкие доминируют.
	res := GenerateResources("p1", "горы", map[string]float64{"рудные_жилы": 100},
		"G", rng, map[string]float64{"rare": 10.0})
	assert.NotEmpty(t, res)
	rare := 0
	for _, r := range res {
		if r.Category == CategoryRare {
			rare++
		}
	}
	assert.Greater(t, float64(rare)/float64(len(res)), 0.5)
}