// internal/economy/settlement/branch.go
//
// Ленивая переработка ветки поселения вход → выход по Δt (спека 2026-09-22-
// поселение-ветка-буферы-переработка §4) + добыча из залежей своей планеты —
// первая половина того же прохода (спека 2026-09-22-поселение-добыча-из-
// залежи-итерация-3 §1/§4): чистая функция от чек-точки ветки, её буферов,
// состава рецепта, залежей планеты и населения. Скорость ветки — число скорости
// пары «тип поселения × рецепт» (producer_recipes.rate, ед/сутки/млрд), спека
// 2026-09-23-стадии-поселения §3.2: формула k·population/complexity заменена.
// Числа не калиброваны (@balancetester). Отдельного тика нет — расчёт по Δt
// при чтении (инвариант 2 GD_PROMPT).
package settlement

import (
	"math"
	"sort"
	"time"
)

// DefaultEatK — норма еды по умолчанию, ед/сутки/млрд (спека 2026-09-23
// §2.3/§2.5): фолбэк, когда у типа поселения нет записи params.eat для ПОЗИЦИИ
// корзины (ключ — позиция, спека 2026-09-22-эффекты-снабжения §4.2). Одно
// утверждённое число с признаком params.eat_units и Go-сидом (§8, T18):
// согласованность мест нормы закреплена тестом.
const DefaultEatK = 600

// BranchComponent — заполненный компонент рецепта ветки (recipe_components,
// component_id IS NOT NULL): норма расхода quantity за один батч. Компонент —
// любой kind (§3.3): вход фильтруется по component_id, не по kind='resource'.
type BranchComponent struct {
	GoodID   int64
	Quantity int
}

// DepositLot — залежь планеты как источник добычи ветки (спека итерации 3 §4):
// ID нужен репозиторию, чтобы записать остаток `amount`, Amount — текущий запас.
// Порядок выбора «от крупной к мелкой» обеспечивает ProcessBranch сортировкой
// (`amount DESC, id ASC`, п.25/F3-A), поэтому входной порядок среза не важен.
type DepositLot struct {
	ID     string
	GoodID int64
	Amount float64
}

// Branch — состояние ветки для переработки: население, число скорости пары
// (ед/сутки/млрд), состав рецепта, входной буфер (good_id → amount), накопленное
// количество выхода и своя чек-точка processed_at. Идентификатор товара-выхода
// чистой функции не нужен (выход — один скаляр Output; привязка good_id к
// строке output-буфера — забота репозитория). Deposits — залежи своей планеты по
// good_id (источник добора); ProcessBranch их не мутирует, результат несёт
// уменьшенные копии (для записи репозиторием). Complexity из скорости убран
// (спека 2026-09-23 §3.2) — сложность рецепта на темп не влияет.
type Branch struct {
	Population float64
	// RatePerDayPerBillion — число скорости пары «тип поселения × рецепт»
	// (producer_recipes.rate), ед/сутки/млрд. NULL/0 → ветка инертна (§3.2).
	RatePerDayPerBillion float64
	Components           []BranchComponent
	Input                map[int64]float64
	Output               float64
	ProcessedAt          time.Time
	Deposits             map[int64][]DepositLot
	// ProducedLast — транзитный результат последнего прохода (для карточки,
	// §6): сколько произведено за Δt. Не состояние БД. Потребление населением
	// из ProcessBranch УБРАНО (спека 2026-09-22-эффекты-снабжения §4.1/§10.9):
	// нужда населения — слой потребности (needs.go), а не хвост ветки.
	ProducedLast float64
}

// EatK — норма еды типа поселения для ПОЗИЦИИ корзины (спека итерации 4 §3.2;
// ключ — позиция, спека 2026-09-22-эффекты-снабжения §4.2): запись
// params.eat[position] есть → берём буквально (в т.ч. явный 0 — «не ест»);
// записи нет / структуры нет → DefaultEatK. Единая семантика для слоя
// потребности (T2/T6/T8/T19); нормы разных позиций изолированы (п.45).
func EatK(eat map[string]float64, position string) float64 {
	if k, ok := eat[position]; ok {
		return k
	}
	return DefaultEatK
}

// ProcessBranch — чистая функция переработки вход → выход по Δt (§4.1/§4.2) с
// добором недостающего из залежей планеты (спека итерации 3 §1/§4):
//
//	seconds    = (now − processed_at) в секундах; ≤ 0 → без изменений
//	components = строки рецепта, свёрнутые по good_id (quantity_i = Σ quantity)
//	stock_i    = input_i + Σ запас залежей планеты с good_id = i
//	desired    = PerSecond(rate, population) · seconds
//	affordable = min_i(stock_i / quantity_i); компонентов нет → 0
//	batches    = min(desired, affordable)                     (дробное)
//	потребление_i = batches · quantity_i: сначала из input_i, остаток — из
//	                залежей (от крупной к мелкой, amount DESC, id ASC)
//	output    += batches
//	processed_at = now
//
// Скорость — из числа пары (rate, ед/сутки/млрд), complexity из формулы убран
// (спека 2026-09-23 §3.2). rate = 0/NULL → desired = 0 → ветка инертна:
// processed_at продвигается, залежь не трогается, выход не растёт.
//
// Хвоста `eaten` НЕТ (спека 2026-09-22-эффекты-снабжения §4.1/§4.2, решение
// создателя 2026-09-23 «производство и потребности — разные слои»): вход
// рецепта — производство, нужда населения — слой потребности (needs.go);
// физическое списание выходного буфера делает он же (единственная точка записи
// O0_b + batches_b − drawn_b). Ветка возвращает объём производства (ProducedLast).
// Идемпотентна: после записи processed_at = now повторный вызов с тем же now
// даёт Δt = 0 → без изменений. Дефицит источников — естественный предел
// (batches = affordable); ни вход, ни залежи в минус не уходят (кламп ≥ 0).
func ProcessBranch(b Branch, now time.Time) Branch {
	seconds := now.Sub(b.ProcessedAt).Seconds()
	if seconds <= 0 {
		return b
	}

	// Дубликат component_id (один good_id на разных pos, 000055) — ОДИН
	// компонент с суммарной нормой (§3.2/T16): запас списывается один раз,
	// выход не завышается.
	comps := aggregateComponents(b.Components)

	desired := PerSecond(b.RatePerDayPerBillion, b.Population) * seconds

	// stock_i = вход + суммарный запас залежей планеты по этому ресурсу (§1).
	stock := make(map[int64]float64, len(comps))
	for _, c := range comps {
		stock[c.GoodID] = b.Input[c.GoodID] + depositTotal(b.Deposits[c.GoodID])
	}

	affordable := math.Inf(1)
	if len(comps) == 0 {
		affordable = 0
	}
	for _, c := range comps {
		q := float64(componentQuantity(c.Quantity))
		if aff := stock[c.GoodID] / q; aff < affordable {
			affordable = aff
		}
	}

	batches := math.Min(desired, affordable)
	if batches < 0 {
		batches = 0
	}

	out := b
	out.Input = cloneAmounts(b.Input)
	out.Deposits = cloneDeposits(b.Deposits)
	for _, c := range comps {
		need := batches * float64(componentQuantity(c.Quantity))
		// Порядок расходования: вход первым, залежь — добор (§4.3, F4-A).
		fromInput := math.Min(out.Input[c.GoodID], need)
		if fromInput < 0 {
			fromInput = 0
		}
		out.Input[c.GoodID] = math.Max(0, out.Input[c.GoodID]-fromInput)
		out.Deposits[c.GoodID] = withdrawDeposits(out.Deposits[c.GoodID], need-fromInput)
	}
	out.Output = b.Output + batches
	out.ProducedLast = batches
	out.ProcessedAt = now
	return out
}

// aggregateComponents — строки рецепта сворачиваются по good_id в один компонент
// с суммарной нормой (§3.2): повтор component_id на разных pos трактуется как
// один компонент. Порядок — по первому появлению.
func aggregateComponents(comps []BranchComponent) []BranchComponent {
	if len(comps) == 0 {
		return nil
	}
	idx := make(map[int64]int, len(comps))
	out := make([]BranchComponent, 0, len(comps))
	for _, c := range comps {
		if i, ok := idx[c.GoodID]; ok {
			out[i].Quantity += c.Quantity
			continue
		}
		idx[c.GoodID] = len(out)
		out = append(out, c)
	}
	return out
}

// depositTotal — суммарный запас положительных залежей ресурса (нулевые не в счёт).
func depositTotal(lots []DepositLot) float64 {
	var sum float64
	for _, l := range lots {
		if l.Amount > 0 {
			sum += l.Amount
		}
	}
	return sum
}

// cloneAmounts — копия карты входного буфера (чистая функция не мутирует вход).
func cloneAmounts(m map[int64]float64) map[int64]float64 {
	out := make(map[int64]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// cloneDeposits — глубокая копия залежей (срезы не разделяются с входом);
// результат всегда не-nil, чтобы запись уменьшенных залежей не падала на nil-карте.
func cloneDeposits(m map[int64][]DepositLot) map[int64][]DepositLot {
	out := make(map[int64][]DepositLot, len(m))
	for k, lots := range m {
		cp := make([]DepositLot, len(lots))
		copy(cp, lots)
		out[k] = cp
	}
	return out
}

// withdrawDeposits — списать need из залежей ресурса, от крупной к мелкой
// (amount DESC, id ASC, п.25/F3-A); остаток клампится ≥ 0 (запас в минус не
// уходит). Возвращает новый срез; исходный не меняется.
func withdrawDeposits(lots []DepositLot, need float64) []DepositLot {
	if len(lots) == 0 {
		return lots
	}
	out := make([]DepositLot, len(lots))
	copy(out, lots)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Amount != out[j].Amount {
			return out[i].Amount > out[j].Amount
		}
		return out[i].ID < out[j].ID
	})
	for i := range out {
		if need <= 0 {
			break
		}
		take := math.Min(out[i].Amount, need)
		if take < 0 {
			take = 0
		}
		out[i].Amount -= take
		if out[i].Amount < 0 {
			out[i].Amount = 0
		}
		need -= take
	}
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
