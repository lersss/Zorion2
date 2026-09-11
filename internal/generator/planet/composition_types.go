// internal/generator/planet/composition_types.go
package planet

import "sort"

// Composition — форма/тип → процент (после Normalize сумма = 100)
type Composition map[string]float64

// Total — сумма всех долей
func (c Composition) Total() float64 {
	total := 0.0
	for _, v := range c {
		total += v
	}
	return total
}

// Normalize — привести к сумме 100. Если сумма ≤ 0 — вернуть пустую.
func (c Composition) Normalize() Composition {
	total := c.Total()
	if total <= 0 {
		return Composition{}
	}
	result := make(Composition, len(c))
	for k, v := range c {
		result[k] = v / total * 100.0
	}
	return result
}

// NonZero — только формы с долей > 0.01
func (c Composition) NonZero() Composition {
	result := make(Composition, len(c))
	for k, v := range c {
		if v > 0.01 {
			result[k] = v
		}
	}
	return result
}

// DominantForm — форма с наибольшей долей ("" если пусто)
func (c Composition) DominantForm() string {
	best := ""
	bestVal := 0.0
	for k, v := range c {
		if v > bestVal {
			best = k
			bestVal = v
		}
	}
	return best
}

// SortedForms — формы, отсортированные по убыванию доли
func (c Composition) SortedForms() []string {
	type kv struct {
		K string
		V float64
	}
	arr := make([]kv, 0, len(c))
	for k, v := range c {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		return arr[i].V > arr[j].V
	})
	result := make([]string, len(arr))
	for i, item := range arr {
		result[i] = item.K
	}
	return result
}

// ShareOf — доля формы (0, если её нет)
func (c Composition) ShareOf(form string) float64 {
	return c[form]
}

// Has — есть ли форма с долей > 0.01
func (c Composition) Has(form string) bool {
	return c[form] > 0.01
}

// BiosphereSum — сумма долей биосферных форм (жизнь-носителей).
func (c Composition) BiosphereSum() float64 {
	total := 0.0
	for form, v := range c {
		if isBiosphereForm(form) {
			total += v
		}
	}
	return total
}