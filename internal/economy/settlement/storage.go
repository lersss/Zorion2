// internal/economy/settlement/storage.go
//
// Чистое ядро внутреннего хранилища (спека 2026-09-25-внутреннее-хранилище-и-
// рождение-заказов §4.2, ЧК2а): пороги ячеек как доли от размера (F2/F3),
// вес ячейки по вхождениям товара во входы рецептов, пропорциональное деление
// входной ячейки между потребителями (F2) и складской дефицит. Функции чистые
// — без RNG, побочек, БД и времени; порядок карты на значения не влияет.
package settlement

import "sort"

// StorageSizeDefault — размер хранилища по умолчанию, когда у типа поселения
// нет ручки producer_types.params.storage.size (§1.3/§9). Число — гипотеза
// (калибровка @balancetester), не эталон мира.
const StorageSizeDefault = 1000.0

// CellKind — вид потребности ячейки (§1.4): производная классификация от
// привязки эффекта и состава рецептов, НЕ колонка.
type CellKind string

const (
	// CellKindPopulation — потребность населения: позиция с эффектом
	// (голод/жажда), дефицит бьёт по людям.
	CellKindPopulation CellKind = "population"
	// CellKindProduction — потребность производства: компонент рецепта,
	// дефицит = простой, эффекта нет.
	CellKindProduction CellKind = "production"
)

// CellKindFor — вид ячейки: товар с привязкой эффекта — потребность населения,
// иначе — потребность производства (компонент рецепта, §1.4).
func CellKindFor(effectBound bool) CellKind {
	if effectBound {
		return CellKindPopulation
	}
	return CellKindProduction
}

// ConsumerKind — вид потребителя входной ячейки (F2).
type ConsumerKind string

const (
	// ConsumerBranch — ветка производства (по settlement_branches.id).
	ConsumerBranch ConsumerKind = "branch"
	// ConsumerPopulation — население поселения (один потребитель на позицию,
	// ID не используется).
	ConsumerPopulation ConsumerKind = "population"
)

// Consumer — потребитель входной ячейки (F2): ветка производства по её id или
// население. Сравним (string + int64) — годится в ключ map; так ветка с id=1
// однозначно отличается от ветки с id=2 и от населения.
type Consumer struct {
	Kind ConsumerKind
	ID   int64
}

// StorageCaps — пороги ячеек: cap_i = size × w_i / Σw (нормализация гарантирует
// Σ порогов = size, §4.2). Веса неположительной суммы (в т.ч. пустая карта) →
// пороги 0; size < 0 клампится в 0. Результат не зависит от порядка обхода
// карты: сумма считается по возрастанию ключей (детерминизм, T4).
func StorageCaps(size float64, weights map[int64]float64) map[int64]float64 {
	out := make(map[int64]float64, len(weights))
	if size < 0 {
		size = 0
	}
	keys := make([]int64, 0, len(weights))
	for g := range weights {
		keys = append(keys, g)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var sum float64
	for _, g := range keys {
		sum += weights[g]
	}
	if sum <= 0 {
		for _, g := range keys {
			out[g] = 0
		}
		return out
	}
	for _, g := range keys {
		out[g] = size * weights[g] / sum
	}
	return out
}

// GoodWeights — вес ячейки (F3, §1.3/§4.2): явная ручка explicit
// (producer_types.params.storage.shares) побеждает всегда, когда задана; иначе
// вес = число вхождений товара во входы рецептов (occurrences), а для товара с
// привязкой эффекта (потребность населения) — max(occurrences, 1) (подстраховка
// от «веса 0»: ячейка и заказы есть всегда). Товар-компонент без эффекта и без
// вхождений получает вес 0. Ключи результата — объединение ключей explicit,
// occurrences и effectBound. Детерминирована, без RNG.
func GoodWeights(explicit map[int64]float64, occurrences map[int64]int, effectBound map[int64]bool) map[int64]float64 {
	out := make(map[int64]float64, len(explicit)+len(occurrences)+len(effectBound))
	for g, w := range explicit {
		out[g] = w
	}
	for g, n := range occurrences {
		if _, ok := explicit[g]; ok {
			continue
		}
		out[g] = cellWeight(n, effectBound[g])
	}
	for g := range effectBound {
		if _, ok := explicit[g]; ok {
			continue
		}
		if _, ok := occurrences[g]; ok {
			continue
		}
		out[g] = cellWeight(0, effectBound[g])
	}
	return out
}

// cellWeight — вес по вхождениям с подстраховкой для потребности населения:
// товар с эффектом всегда не меньше 1 (max(occurrences, 1)).
func cellWeight(occurrences int, effectBound bool) float64 {
	if effectBound && occurrences < 1 {
		return 1
	}
	return float64(occurrences)
}

// AllocateProportional — пропорциональное деление входной ячейки между
// потребителями (F2, §4.2/§5.7): один потребитель → весь available; несколько →
// alloc_c = available × need_c / Σ need; Σ need = 0 → потребители инертны;
// available ≤ 0 → нули. Заявку потребителя (need_c × Δt_c) функция НЕ
// применяет — это делает вызывающий. Значения не зависят от порядка обхода
// карты: сумма считается по стабильному порядку потребителей (для детерминизма
// float-сложения; тай-брейки при равных потребностях — за вызывающим).
func AllocateProportional(available float64, needs map[Consumer]float64) map[Consumer]float64 {
	out := make(map[Consumer]float64, len(needs))
	if len(needs) == 0 || available <= 0 {
		return out
	}
	keys := make([]Consumer, 0, len(needs))
	for c := range needs {
		keys = append(keys, c)
	}
	if len(keys) == 1 {
		out[keys[0]] = available
		return out
	}
	// Стабильный порядок потребителей — детерминированное float-сложение
	// (порядок обхода map в Go рандомизирован).
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Kind != keys[j].Kind {
			return keys[i].Kind < keys[j].Kind
		}
		return keys[i].ID < keys[j].ID
	})
	var sum float64
	for _, c := range keys {
		sum += needs[c]
	}
	if sum <= 0 {
		return out
	}
	for _, c := range keys {
		out[c] = available * needs[c] / sum
	}
	return out
}

// CellDeficit — складской дефицит ячейки: max(0, cap − amount) (§1.3). Это НЕ
// суточный дефицит позиции слоя эффектов (`contracts.Deficit`) — величины
// разные, ссылку на контракты не добавлять.
func CellDeficit(cap, amount float64) float64 {
	d := cap - amount
	if d < 0 {
		return 0
	}
	return d
}
