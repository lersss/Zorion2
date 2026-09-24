// internal/economy/settlement/arithmetic.go
//
// Витрина арифметики поселения на текущем населении (спека 2026-09-23-стадии-
// поселения-и-скорость-производства §8.1/§8.2): «производим / потребляем /
// сверх» по ПОЗИЦИИ корзины и «забираем» по ветке. Потребление идёт по позиции
// (сумма источников), поэтому сверх потребления по одной ветке не выводится —
// считается по позиции; «забираем» же выводится из состава рецепта ветки.
// Чистые функции, единица — та же через PerDay (§2.2).
package settlement

import "sort"

// PositionArithmetic — арифметика одной позиции корзины на текущем населении,
// ед/сутки (§8.2). NetPerDay < 0 — дефицит позиции. Поля нужды (§10.2)
// заполняет AttachNeedArithmetic: позиция без эффекта нужды не несёт.
type PositionArithmetic struct {
	Position string
	// NormPerDayPerBillion — норма позиции (params.eat, «ед/сутки/млрд», §10.2).
	NormPerDayPerBillion float64
	ProducedPerDay       float64
	ConsumedPerDay       float64
	NetPerDay            float64
	// Need — ключ нужды (= effect_types.name_norm); Effect — тип эффекта. Пусто —
	// позиция без эффекта. CoveredShare = 1 − DeficitShare, кламп [0,1]; в
	// DeficitShare — текущая сила условия w из слоя потребности (§10.2).
	Need         string
	Effect       string
	CoveredShare float64
	DeficitShare float64
}

// ArithmeticSource — ветка как источник позиции для арифметики: позиция
// товара-выхода (goods.name_norm) и число скорости пары (ед/сутки/млрд).
type ArithmeticSource struct {
	Position             string
	RatePerDayPerBillion float64
}

// ComputePositionArithmetic — «производим / потребляем / сверх» по позициям
// (§8.2):
//
//	производим   = Σ PerDay(rate, population) источников позиции;
//	потребляем   = PerDay(норма, population) ТОЛЬКО для позиций из effects
//	               (норма — eat[позиция], записи нет → DefaultEatK);
//	сверх        = производим − потребляем (может быть отрицательным).
//
// Позиция, объявленная только в eat (но не в effects), спроса НЕ создаёт —
// мёртвый ключ нормы. Позиции результата — объединение выходов источников и
// ключей effects, порядок детерминированный (по позиции).
func ComputePositionArithmetic(population float64, effects map[string]string, eat map[string]float64, sources []ArithmeticSource) []PositionArithmetic {
	produced := map[string]float64{}
	for _, s := range sources {
		if s.Position == "" {
			continue
		}
		produced[s.Position] += PerDay(s.RatePerDayPerBillion, population)
	}
	positions := make([]string, 0, len(produced)+len(effects))
	seen := map[string]bool{}
	for pos := range produced {
		if !seen[pos] {
			seen[pos] = true
			positions = append(positions, pos)
		}
	}
	for pos := range effects {
		if pos == "" || seen[pos] {
			continue
		}
		seen[pos] = true
		positions = append(positions, pos)
	}
	if len(positions) == 0 {
		return nil
	}
	sort.Strings(positions)

	out := make([]PositionArithmetic, 0, len(positions))
	for _, pos := range positions {
		prod := produced[pos]
		var cons float64
		if _, ok := effects[pos]; ok {
			cons = PerDay(EatK(eat, pos), population)
		}
		out = append(out, PositionArithmetic{
			Position:       pos,
			ProducedPerDay: prod,
			ConsumedPerDay: cons,
			NetPerDay:      prod - cons,
		})
	}
	return out
}

// AttachNeedArithmetic — строка нужды (§10.2) на позициях арифметики: позиция,
// привязанная к эффекту (bindings), получает норму, ключ/тип нужды и покрытие/
// дефицит по слою потребности (deficit = текущая сила условия w; covered =
// 1 − w, кламп [0,1]). Позиция без привязки остаётся без нужды. Порядок и число
// позиций не меняются (нужда живёт на существующих строках — §10.3, очистка
// блока арифметики убирает её вместе с позициями).
func AttachNeedArithmetic(positions []PositionArithmetic, bindings []NeedsBinding, effects []EffectRun) []PositionArithmetic {
	if len(positions) == 0 {
		return positions
	}
	typeByPos := make(map[string]int64, len(bindings))
	nameByType := make(map[int64]string, len(bindings))
	normByPos := make(map[string]float64, len(bindings))
	for _, b := range bindings {
		typeByPos[b.Position] = b.EffectTypeID
		nameByType[b.EffectTypeID] = b.EffectTypeName
		normByPos[b.Position] = b.NormPerDayPerBillion
	}
	wByType := make(map[int64]float64, len(effects))
	for _, e := range effects {
		wByType[e.EffectTypeID] = e.W
	}
	for i := range positions {
		pos := positions[i].Position
		typeID, ok := typeByPos[pos]
		if !ok {
			continue
		}
		name := nameByType[typeID]
		if name == "" {
			continue
		}
		deficit := clamp01(wByType[typeID])
		positions[i].NormPerDayPerBillion = normByPos[pos]
		positions[i].Need = name
		positions[i].Effect = name
		positions[i].DeficitShare = deficit
		positions[i].CoveredShare = clamp01(1 - deficit)
	}
	return positions
}

// clamp01 — доля в [0,1] для показа (покрытие/дефицит, §10.2).
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// BranchTake — компонент «забираем» ветки (расход входа), агрегированный по
// good_id: норма quantity и расчётный расход в сутки (выход × quantity).
type BranchTake struct {
	GoodID   int64
	Quantity int
	PerDay   float64
}

// BranchTakePerDay — «забираем» по ветке (§8.2): по каждому компоненту рецепта
// PerDay(rate, population) × quantity_i; дубликат component_id сворачивается в
// один компонент с суммарной нормой (как ProcessBranch, §3.2).
func BranchTakePerDay(ratePerDayPerBillion, population float64, components []BranchComponent) []BranchTake {
	agg := aggregateComponents(components)
	if len(agg) == 0 {
		return nil
	}
	output := PerDay(ratePerDayPerBillion, population)
	out := make([]BranchTake, 0, len(agg))
	for _, c := range agg {
		out = append(out, BranchTake{
			GoodID:   c.GoodID,
			Quantity: c.Quantity,
			PerDay:   output * float64(componentQuantity(c.Quantity)),
		})
	}
	return out
}

// BranchDepositShare — доля входа, добранная из залежей за последний проход
// (§8.2, «не атрибуция по позиции», а доля ветки): (Σ need_i − Σ фактически
// съеденного входа) / Σ need_i, где need_i = produced × quantity_i. Вход — это
// before−after по буферу; добор из залежей — остаток. produced ≤ 0 → 0.
func BranchDepositShare(produced float64, components []BranchComponent, inputBefore, inputAfter map[int64]float64) float64 {
	if produced <= 0 {
		return 0
	}
	var need, fromInput float64
	for _, c := range aggregateComponents(components) {
		n := produced * float64(componentQuantity(c.Quantity))
		need += n
		from := inputBefore[c.GoodID] - inputAfter[c.GoodID]
		if from < 0 {
			from = 0
		}
		if from > n {
			from = n
		}
		fromInput += from
	}
	if need <= 0 {
		return 0
	}
	deposit := need - fromInput
	if deposit < 0 {
		return 0
	}
	return deposit / need
}
