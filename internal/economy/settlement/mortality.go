// Package settlement считает экономику поселения по запросу (ленивый
// пересчёт), без тика. Здесь — изменение населения от физической среды
// планеты (docs/gamedesign/18a_population_death.md): дельта может быть
// убылью или ростом (сумма рекурсивных компонент, 99.2.12/99.2.13;
// компоненты — в change_components.go).
package settlement

import "math"

// MaxPopulation — расчётный кламп роста (99.2.23 §3.2, поправки В1+В2):
// технический порог ~5 порядков ниже math.MaxFloat64; недостижим в игре.
// Защита от +Inf при json.Marshal результата.
const MaxPopulation = 1e300

// MaxInt4Population — кламп записи в int4-колонку settlements.population
// (2^31−1): перед приведением к int в обоих путях записи (БД и «простой
// визит» — единая константа, защита от «integer out of range», 99.2.23 §3.2).
const MaxInt4Population = 2147483647

// Uninhabitable — истина, когда планета «необитаема» (99.2.12, 99.2.13):
// витринный порог «t_смерти(p0) < 1 ч» по полному r (ChangeComponents) —
// механика гладкая (жёстких нулей-обнулений больше нет: все компоненты
// рекурсивные, R < 1 с гвардом), порог только для витрины.
func Uninhabitable(input PlanetInput, p0 float64) bool {
	if p0 < 1 {
		return false
	}
	r := ChangeComponents(input)
	return r > 1-math.Exp(-math.Log(p0)/3600)
}

// DeathMomentSeconds — момент смерти в секундах от создания (99.2.12,
// §«Момент смерти», R-модель): p0·(1−R)^t = 1 → t = ln(p0)/|ln(1−R)|;
// дата = created_at + t. p0 ≤ 1 → 0 (уже мёртвые); r ≤ 0 → +Inf (не
// вымирают); r ≥ 1 → 0 (гибель 100% за секунду).
func DeathMomentSeconds(p0, r float64) float64 {
	if p0 <= 1 {
		return 0
	}
	switch {
	case r >= 1:
		return 0
	case r <= 0:
		return math.Inf(1)
	}
	return math.Log(p0) / math.Abs(math.Log(1-r))
}

// Population считает точное (дробное) население через deltaSeconds реального
// времени (99.2.12/99.2.13, R-модель): p = p_чек·(1−r)^Δt_сек, где r —
// полная рекурсивная компонента (ChangeComponents). Отрицательная компонента
// = рост: r < 0 → (1−r) > 1. Округление — только при показе игроку (18a,
// «Точность и округление»). Гвард робастности: r ≥ 1 (экстремальные входы,
// степенная неограничена) → мгновенная гибель p = 0 — иначе pow(1−r, Δt)
// дал бы NaN при r > 1 (99.2.13). Гвард переполнения роста (99.2.23 §3.2):
// p > MaxPopulation (1e300) → кламп (иначе +Inf при огромном Δt и росте).
// Guard: Δt ≤ 0 → p0; порог p < 1 → 0 (поселение мёртвое).
func Population(p0 float64, r float64, deltaSeconds float64) float64 {
	if p0 <= 0 {
		return 0
	}
	if deltaSeconds <= 0 {
		return p0
	}
	if r >= 1 {
		return 0
	}
	p := p0 * math.Pow(1-r, deltaSeconds)
	if p > MaxPopulation {
		p = MaxPopulation
	}
	if p < 1 {
		return 0
	}
	return p
}