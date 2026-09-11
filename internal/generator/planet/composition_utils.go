// internal/generator/planet/composition_utils.go
package planet

import (
	"math"
	"math/rand"
)

// multiplyIfExists — умножает вес формы, если она есть в map.
// Результат ограничивается снизу нулём.
func multiplyIfExists(c map[string]float64, form string, factor float64) {
	v, ok := c[form]
	if !ok {
		return
	}
	newV := v * factor
	if newV < 0 {
		newV = 0
	}
	c[form] = newV
}

// keysWithPositive — ключи с весом > 0.001
func keysWithPositive(c map[string]float64) []string {
	result := make([]string, 0, len(c))
	for k, v := range c {
		if v > 0.001 {
			result = append(result, k)
		}
	}
	return result
}

// fallbackComposition — берёт форму с максимальным весом из base и делает её 100%.
// Используется, когда после всех корректировок композиция опустела.
func fallbackComposition(base map[string]float64) Composition {
	best := ""
	bestVal := -math.MaxFloat64
	for k, v := range base {
		if v > bestVal {
			best = k
			bestVal = v
		}
	}
	if best == "" {
		return Composition{}
	}
	return Composition{best: 100.0}
}

// copyWeights — глубокая копия map весов.
// Нужна, чтобы не мутировать исходные данные из кэша архетипов.
func copyWeights(src map[string]float64) map[string]float64 {
	if src == nil {
		return map[string]float64{}
	}
	dst := make(map[string]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// primitivePlanetProbability — доля «примитивных» планет: 1–2 типа поверхности и недр.
const primitivePlanetProbability = 0.035

// simpleSatelliteProbability — потолок доли «простых» спутников: 1–2 типа поверхности и недр.
// «До 20%» — фактическая доля ниже порога, т.к. часть тел и так имеет ≤2 форм.
const simpleSatelliteProbability = 0.20

// simplifyComposition — «примитивное» тело: оставляет 1–2 главные формы.
// Одна форма — 100%. Две формы — титульная 60–85%, вторичная — остальное.
// Результат уже в процентах (сумма = 100).
func simplifyComposition(c Composition, rng *rand.Rand) Composition {
	forms := c.SortedForms()
	if len(forms) <= 2 {
		return c
	}
	if rng.Float64() < 0.5 {
		main := (0.6 + rng.Float64()*0.25) * 100
		return Composition{forms[0]: main, forms[1]: 100 - main}
	}
	return Composition{forms[0]: 100}
}