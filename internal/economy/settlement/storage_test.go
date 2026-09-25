// internal/economy/settlement/storage_test.go
// Критерии T4/T22 (спека 2026-09-25-внутреннее-хранилище-и-рождение-заказов
// §4.2/§12): пороги ячеек (Σ порогов = size, нулевые веса → 0, детерминизм),
// вес ячейки F3 (явная ручка побеждает, вхождения, max(occurrences,1) для
// потребности населения), пропорциональное деление входной ячейки F2 и
// складской дефицит. Функции чистые — без БД/RNG.
package settlement

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// --- StorageCaps ---

// T4: Σ порогов = size при любом наборе весов (нормализация).
func TestStorageCapsSumEqualsSize(t *testing.T) {
	cases := []struct {
		name    string
		size    float64
		weights map[int64]float64
	}{
		{"равные веса", 1000, map[int64]float64{1: 1, 2: 1, 3: 1, 4: 1}},
		{"разные веса", 500, map[int64]float64{1: 3, 2: 1}},
		{"один товар", 120, map[int64]float64{7: 5}},
		{"дробные веса", 250, map[int64]float64{1: 0.4, 2: 0.6}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caps := StorageCaps(tc.size, tc.weights)
			var sum float64
			for _, c := range caps {
				sum += c
			}
			require.InDelta(t, tc.size, sum, 1e-9, "Σ порогов = size")
		})
	}
}

// T4: порог ∝ весу (3:1 от 1000 → 750/250).
func TestStorageCapsProportional(t *testing.T) {
	caps := StorageCaps(1000, map[int64]float64{1: 3, 2: 1})
	require.InDelta(t, 750, caps[1], 1e-9)
	require.InDelta(t, 250, caps[2], 1e-9)
}

// T4: нулевые веса (товар без эффекта и без вхождений) → пороги 0.
func TestStorageCapsZeroWeights(t *testing.T) {
	caps := StorageCaps(1000, map[int64]float64{1: 0, 2: 0})
	require.Len(t, caps, 2)
	require.InDelta(t, 0, caps[1], 1e-12)
	require.InDelta(t, 0, caps[2], 1e-12)
}

// Пустой набор весов → пустая карта.
func TestStorageCapsEmpty(t *testing.T) {
	require.Empty(t, StorageCaps(1000, nil))
	require.Empty(t, StorageCaps(1000, map[int64]float64{}))
}

// Защита от отрицательного размера: кламп в 0 → пороги 0.
func TestStorageCapsNegativeSizeClamps(t *testing.T) {
	caps := StorageCaps(-100, map[int64]float64{1: 1, 2: 1})
	require.Len(t, caps, 2)
	for _, c := range caps {
		require.InDelta(t, 0, c, 1e-12, "size < 0 → пороги 0")
	}
}

// T4: повторный вызов даёт тот же результат (детерминизм, без RNG).
func TestStorageCapsDeterministic(t *testing.T) {
	w := map[int64]float64{1: 3, 2: 1, 3: 2}
	first := StorageCaps(1000, w)
	for i := 0; i < 20; i++ {
		require.Equal(t, first, StorageCaps(1000, w), "повторный вызов детерминирован")
	}
}

// --- GoodWeights ---

// T22/F3: явная ручка shares побеждает вхождения и подстраховку max.
func TestGoodWeightsExplicitWins(t *testing.T) {
	got := GoodWeights(
		map[int64]float64{5: 0.3},
		map[int64]int{5: 7},
		map[int64]bool{5: true},
	)
	require.InDelta(t, 0.3, got[5], 1e-12, "явная ручка побеждает")
}

// T22/F3: явная ручка 0 не перезаписывается max(occurrences,1).
func TestGoodWeightsExplicitZeroWins(t *testing.T) {
	got := GoodWeights(
		map[int64]float64{5: 0},
		map[int64]int{5: 9},
		map[int64]bool{5: true},
	)
	require.InDelta(t, 0, got[5], 1e-12, "явная ручка задана → max не применяется")
}

// T22/F3: вхождения учитываются; товар с эффектом при 0 вхождений → вес 1;
// товар-компонент без эффекта и без вхождений → вес 0.
func TestGoodWeightsOccurrencesAndEffectFloor(t *testing.T) {
	got := GoodWeights(nil,
		map[int64]int{1: 3, 3: 0},
		map[int64]bool{2: true},
	)
	require.InDelta(t, 3, got[1], 1e-12, "вес = число вхождений")
	require.InDelta(t, 1, got[2], 1e-12, "товар с эффектом → max(0,1) = 1")
	require.InDelta(t, 0, got[3], 1e-12, "компонент без эффекта и вхождений → 0")
	require.Len(t, got, 3)
}

// T22/F3: max не занижает существующий вес товара с эффектом.
func TestGoodWeightsEffectWithOccurrences(t *testing.T) {
	got := GoodWeights(nil, map[int64]int{2: 5}, map[int64]bool{2: true})
	require.InDelta(t, 5, got[2], 1e-12, "max(occurrences, 1) не занижает")
}

// F3: набор ключей результата — объединение explicit/occurrences/effectBound.
func TestGoodWeightsKeysUnion(t *testing.T) {
	got := GoodWeights(
		map[int64]float64{4: 0.5},
		map[int64]int{1: 2, 3: 1},
		map[int64]bool{2: true, 5: true},
	)
	require.Len(t, got, 5)
	for _, k := range []int64{1, 2, 3, 4, 5} {
		require.Contains(t, got, k)
	}
}

// F3: повторный вызов детерминирован.
func TestGoodWeightsDeterministic(t *testing.T) {
	explicit := map[int64]float64{4: 0.5}
	occurrences := map[int64]int{1: 2, 3: 1}
	effectBound := map[int64]bool{2: true, 5: true}
	first := GoodWeights(explicit, occurrences, effectBound)
	for i := 0; i < 20; i++ {
		require.Equal(t, first, GoodWeights(explicit, occurrences, effectBound))
	}
}

// --- AllocateProportional ---

// T21/F2: один потребитель → весь запас; несколько → пропорционально
// потребности (не поровну); Σ need = 0 → инертно; available = 0 → нули.
func TestAllocateProportional(t *testing.T) {
	pop := Consumer{Kind: ConsumerPopulation}
	b1 := Consumer{Kind: ConsumerBranch, ID: 1}
	b2 := Consumer{Kind: ConsumerBranch, ID: 2}

	t.Run("один потребитель получает весь запас", func(t *testing.T) {
		got := AllocateProportional(100, map[Consumer]float64{b1: 7})
		require.InDelta(t, 100, got[b1], 1e-9)
	})

	t.Run("два потребителя делят пропорционально", func(t *testing.T) {
		got := AllocateProportional(100, map[Consumer]float64{b1: 3, b2: 1})
		require.InDelta(t, 75, got[b1], 1e-9)
		require.InDelta(t, 25, got[b2], 1e-9)
		require.InDelta(t, 100, got[b1]+got[b2], 1e-9, "запас распределён весь")
		require.NotEqual(t, got[b1], got[b2], "деление пропорционально, не поровну")
	})

	t.Run("население и ветка — тот же принцип", func(t *testing.T) {
		got := AllocateProportional(90, map[Consumer]float64{b1: 2, pop: 1})
		require.InDelta(t, 60, got[b1], 1e-9)
		require.InDelta(t, 30, got[pop], 1e-9)
	})

	t.Run("Σ need = 0 → инертно", func(t *testing.T) {
		got := AllocateProportional(100, map[Consumer]float64{b1: 0, b2: 0})
		require.InDelta(t, 0, got[b1], 1e-12)
		require.InDelta(t, 0, got[b2], 1e-12)
	})

	t.Run("available = 0 → нули", func(t *testing.T) {
		got := AllocateProportional(0, map[Consumer]float64{b1: 1, b2: 1})
		require.InDelta(t, 0, got[b1], 1e-12)
		require.InDelta(t, 0, got[b2], 1e-12)
	})
}

// F2: результат не зависит от порядка обхода карты.
func TestAllocateProportionalDeterministic(t *testing.T) {
	needs := map[Consumer]float64{
		{Kind: ConsumerBranch, ID: 1}: 3,
		{Kind: ConsumerBranch, ID: 2}: 1,
		{Kind: ConsumerPopulation}:    2,
	}
	first := AllocateProportional(123.456, needs)
	for i := 0; i < 20; i++ {
		require.Equal(t, first, AllocateProportional(123.456, needs))
	}
}

// --- CellDeficit ---

// T6: складской дефицит = max(0, cap − amount).
func TestCellDeficit(t *testing.T) {
	cases := []struct {
		name   string
		cap    float64
		amount float64
		want   float64
	}{
		{"положительный", 100, 30, 70},
		{"нулевой", 100, 100, 0},
		{"отрицательный (переполнение)", 100, 130, 0},
		{"пустая ячейка", 50, 0, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.InDelta(t, tc.want, CellDeficit(tc.cap, tc.amount), 1e-12)
		})
	}
}

// --- Классификация ячейки ---

// §1.4: привязка эффекта → population, иначе — production.
func TestCellKindFor(t *testing.T) {
	require.Equal(t, CellKindPopulation, CellKindFor(true))
	require.Equal(t, CellKindProduction, CellKindFor(false))
}
