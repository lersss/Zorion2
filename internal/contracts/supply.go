package contracts

import (
	"math"
	"sort"
)

// Константы кривой награды и капа долей.
//
// НЕ КАЛИБРОВАНО: числа — предмет @balancetester / создателя
// (спека 2026-09-23 «контракт-ленивая-доска-пакет-и-снабжение» §5.4/§5.3).
const (
	RewardBonusMax  = 0.5   // премия за единицу сверху: награда/объём ≤ база × (1 + RewardBonusMax)
	RewardBonusHalf = 100.0 // полунасыщение бонуса: при объёме = RewardBonusHalf бонус = RewardBonusMax/2

	// ShareCapFraction — кап «доля ≤ ½ дефицита» (F6/D). Низший приоритет критериев
	// нарезки: в v1 НЕ применяется — дробление ради капа увеличивает число долей
	// и ломает приоритет 4 («минимальное число долей»). Заведена как якорь.
	ShareCapFraction = 0.5
)

// Дефицит: max(0, target-actual).
func Deficit(target, actual float64) float64 {
	if d := target - actual; d > 0 {
		return d
	}
	return 0
}

// Share — пред-нарезанная доля нужды (пакет контрактов).
type Share struct {
	Index    int     // 1..N (contracts.share_index)
	Quantity int64   // объём доли в единицах товара
	Capacity float64 // вместимость трюма, под которую рассчитана доля (в тех же единицах)
}

// SliceShares — детерминированная нарезка нужды на доли; capacities — вместимости
// трюмов в ЕДИНИЦАХ ТОВАРА (масса/вес единицы — конверсию делает вызывающий).
//
// Приоритет критериев (спека §5.3): 1) Σ долей = ⌊дефицит⌋; 2) детерминизм;
// 3) доля ≤ своей вместимости; 4) минимальное число долей; 5) «доли под разные
// трюмы» — цель; 6) кап ≤ ½ дефицита в v1 не применяется.
//
// Критерий 5 — best-effort: минимальное число долей важнее, поэтому при узком
// диапазоне вместимостей доли одного размера допустимы.
func SliceShares(deficit float64, capacities []float64) []Share {
	d := int64(math.Floor(deficit))
	if d <= 0 {
		return nil
	}

	caps := normalizeCapacities(capacities)
	if len(caps) == 0 {
		return []Share{{Index: 1, Quantity: d, Capacity: 0}}
	}
	if float64(d) <= caps[0] {
		return []Share{{Index: 1, Quantity: d, Capacity: caps[0]}}
	}

	largest := caps[len(caps)-1]
	n := int(math.Ceil(float64(d) / largest))
	shares := make([]Share, 0, n)
	remaining := d
	for i := 0; i < n; i++ {
		q := int64(math.Min(largest, float64(remaining-int64(n-1-i))))
		remaining -= q
		shares = append(shares, Share{
			Index:    i + 1,
			Quantity: q,
			Capacity: smallestCapAtLeast(caps, float64(q)),
		})
	}
	return shares
}

// ShareReward — награда за долю:
// floor(base * quantity * (1 + RewardBonusMax*quantity/(quantity+RewardBonusHalf))).
func ShareReward(quantity int64, base float64) int64 {
	q := float64(quantity)
	bonus := RewardBonusMax * q / (q + RewardBonusHalf)
	return int64(math.Floor(base * q * (1 + bonus)))
}

// normalizeCapacities — только положительные, по возрастанию, без дубликатов.
func normalizeCapacities(capacities []float64) []float64 {
	positive := make([]float64, 0, len(capacities))
	for _, c := range capacities {
		if c > 0 {
			positive = append(positive, c)
		}
	}
	sort.Float64s(positive)

	out := make([]float64, 0, len(positive))
	for i, c := range positive {
		if i == 0 || c != positive[i-1] {
			out = append(out, c)
		}
	}
	return out
}

// smallestCapAtLeast — наименьшая вместимость ≥ q (caps отсортирован, q ≤ caps[last]).
func smallestCapAtLeast(caps []float64, q float64) float64 {
	for _, c := range caps {
		if c >= q {
			return c
		}
	}
	return caps[len(caps)-1]
}
