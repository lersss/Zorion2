// internal/economy/settlement/branch.go
//
// Ленивая переработка ветки поселения вход → выход по Δt (спека 2026-09-22-
// поселение-ветка-буферы-переработка §4): чистая функция от чек-точки ветки,
// её буферов, состава рецепта и населения. Скорость — вариант A (линейная),
// k = 2.78·10⁻⁸ батч/(чел·ч), решение создателя 2026-09-22 (§4.1). Числа не
// калиброваны (@balancetester). Отдельного тика нет — расчёт по Δt при чтении
// (инвариант 2 GD_PROMPT).
package settlement

import (
	"math"
	"time"
)

// BranchRateK — вариант A (спека §4.1): rate_батч/час = k · population /
// complexity, k = 2.78·10⁻⁸ батч/(чел·ч). Разлёт времени по населению (1e5 →
// ≈41 год, 1e9 → ≈1.5 суток) принят создателем осознанно 2026-09-22.
const BranchRateK = 2.78e-8

// BranchComponent — заполненный компонент рецепта ветки (recipe_components,
// component_id IS NOT NULL): норма расхода quantity за один батч. Компонент —
// любой kind (§3.3): вход фильтруется по component_id, не по kind='resource'.
type BranchComponent struct {
	GoodID   int64
	Quantity int
}

// Branch — состояние ветки для переработки: население (скорость), состав
// рецепта, сложность (делитель), входной буфер (good_id → amount), накопленное
// количество выхода и своя чек-точка processed_at. Идентификатор товара-выхода
// чистой функции не нужен (выход — один скаляр Output; привязка good_id к
// строке output-буфера — забота репозитория).
type Branch struct {
	Population  float64
	Complexity  *int // nil = NULL («вычисляется по графу») → временный фолбэк 1 (§4.1)
	Components  []BranchComponent
	Input       map[int64]float64
	Output      float64
	ProcessedAt time.Time
}

// BranchRate — батчей в час: k · population / max(1, complexity). complexity
// NULL → 1, 0 → 1 (делитель ≥ 1, §4.1); сложность не увеличивает скорость.
func BranchRate(population float64, complexity *int) float64 {
	c := 1
	if complexity != nil && *complexity > 1 {
		c = *complexity
	}
	return BranchRateK * population / float64(c)
}

// ProcessBranch — чистая функция переработки вход → выход по Δt (§4.1/§4.2):
//
//	hours      = (now − processed_at) в часах; ≤ 0 → без изменений
//	desired    = BranchRate(population) · hours
//	affordable = min по компонентам (input_i / quantity_i); компонентов нет → 0
//	batches    = min(desired, affordable)                     (дробное)
//	input_i   -= batches · quantity_i                         (по компонентам)
//	output    += batches
//	processed_at = now
//
// Идемпотентна: после записи processed_at = now повторный вызов с тем же now
// даёт Δt = 0 → без изменений. Дефицит входа — естественный предел (batches =
// affordable); в минус вход не уходит (amount ≥ 0).
func ProcessBranch(b Branch, now time.Time) Branch {
	hours := now.Sub(b.ProcessedAt).Hours()
	if hours <= 0 {
		return b
	}

	desired := BranchRate(b.Population, b.Complexity) * hours

	affordable := math.Inf(1)
	if len(b.Components) == 0 {
		affordable = 0
	}
	for _, c := range b.Components {
		q := float64(componentQuantity(c.Quantity))
		if aff := b.Input[c.GoodID] / q; aff < affordable {
			affordable = aff
		}
	}

	batches := math.Min(desired, affordable)
	if batches < 0 {
		batches = 0
	}

	out := b
	out.Input = make(map[int64]float64, len(b.Input))
	for k, v := range b.Input {
		out.Input[k] = v
	}
	for _, c := range b.Components {
		q := float64(componentQuantity(c.Quantity))
		left := out.Input[c.GoodID] - batches*q
		if left < 0 {
			left = 0
		}
		out.Input[c.GoodID] = left
	}
	out.Output = b.Output + batches
	out.ProcessedAt = now
	return out
}

// componentQuantity — норма расхода ≥ 1 (canon 000055: quantity INT NOT NULL
// DEFAULT 1 CHECK >= 1); защита от нуля — делитель не обнуляется.
func componentQuantity(q int) int {
	if q < 1 {
		return 1
	}
	return q
}
