// internal/economy/settlement/race_derive.go
// Генерация расовых R-кривых из карточки (спека 99.2.23 §3.3): сдвиг X
// (жара: opt_hi − 273.15 + растяжение горячей ветки; холод: opt_lo − 288 +
// пиннинг левого узла на 0 K; гравитация: комфорт (0.8, 1.2) → (opt_lo,
// opt_hi); радиация: opt_hi − 20) + масштаб Y × 50/resilience (кламп
// [0, 0.999)). Вызывается ТОЛЬКО при генерации (авто-инициализация §4.4,
// кнопка «Сгенерировать из карточки» §4.3); в R-модели не участвует (там —
// готовые active-кривые). Детерминизм: два вызова из одной карточки и одних
// человеческих кривых — одинаковый результат.
package settlement

import (
	"fmt"

	"zorion/internal/races"
)

// deriveRaceCurves — вывод 4 кривых из карточки расы + текущих человеческих
// кривых store (спека 99.2.23 §3.3): сдвиг X (жара: opt_hi − 273.15 + растяжение
// горячей ветки; холод: opt_lo − 288 + пиннинг левого узла на 0 K; гравитация:
// комфорт (0.8, 1.2) → (opt_lo, opt_hi); радиация: opt_hi − 20) + масштаб Y
// × 50/resilience (кламп [0, 0.999)). Вызывается ТОЛЬКО при генерации
// (авто-инициализация §4.4, кнопка «Сгенерировать из карточки» §4.3);
// в R-модели не участвует (там — готовые active-кривые). Детерминизм: два
// вызова из одной карточки и одних человеческих кривых — одинаковый результат.
func deriveRaceCurves(race *races.Race) (RaceCurves, error) {
	if race == nil {
		return RaceCurves{}, fmt.Errorf("раса не задана")
	}
	if race.ID == "humans" {
		return RaceCurves{}, fmt.Errorf("humans — спец-случай (99.2.23 §2.3), кривые не выводятся из карточки")
	}

	optT := race.Conditions.Temperature.Opt
	survT := race.Conditions.Temperature.Surv

	// Жара: X' = (opt_hi − 273.15) + (X − 30)·HeatStretch; первый узел
	// (30 °C, Y=0) → opt_hi − 273.15 °C (жара 0 при T ≤ opt_hi). Растяжение
	// горячей ветки (итерация 2, В1): HeatStretch = (surv_hi − opt_hi)/60 —
	// край surv расы на отклонение 60 K человеческой кривой (90 °C), где
	// R ≈ 3.9·10⁻⁵ (t50 ≈ 5 ч). Гвард: HeatStretch > 0 всегда (opt ⊂ surv
	// по валидации карточки; при surv_hi = opt_hi — 1e-3).
	heatStretch := 1e-3
	if survT.Hi != nil && optT.Hi != nil {
		heatStretch = (*survT.Hi - *optT.Hi) / 60
		if heatStretch < 1e-3 {
			heatStretch = 1e-3
		}
	}
	heat := shiftHeatCurve(humanCurve("heat"), *optT.Hi, heatStretch)

	// Холод: X' = X + (opt_lo − 288); правый узел (14.85 °C = 288 K, Y=0) →
	// opt_lo − 273.15 °C (холод 0 при T ≥ opt_lo). Левый узел пинится на
	// абсолютный ноль (−273.15 °C, Y = 0.05 — максимум человеческой кривой);
	// промежуточные узлы ниже −273.15 °C отбрасываются (с соответствующими
	// bends) — «максимум деградации на 0 K» для любой расы.
	cold := shiftColdCurve(humanCurve("cold"), optT.Lo)

	// Гравитация: узлы X < 0.8 → X + (opt_lo − 0.8); узлы X > 1.2 →
	// X + (opt_hi − 1.2); два узла комфорта (0.8, 1.2) → (opt_lo, opt_hi)
	// (линейный ремап комфорта на окно расы). Узлы, уходящие ниже 0 g,
	// отбрасываются (левая ветка клампится к 0 g). Окна нет («не влияет») —
	// человеческая кривая без изменений.
	gravity := humanCurve("gravity")
	if race.Conditions.Gravity != nil {
		gravity = shiftGravityCurve(gravity, race.Conditions.Gravity.Opt.Lo, *race.Conditions.Gravity.Opt.Hi)
	}

	// Радиация: X' = X + (opt_hi − 20); первый узел (20, Y=0) → opt_hi rad.
	// Узлы ниже 0 rad отбрасываются (кламп к 0).
	radiation := shiftRadiationCurve(humanCurve("radiation"), *race.Conditions.Radiation.Opt.Hi)

	// Масштаб Y: все узлы всех четырёх кривых × ResilienceScale = 50/resilience
	// (после сдвига X); кламп Y ≤ 0.999 (гвард, как в validateCurve).
	scale := 50.0 / race.Attributes.Resilience
	applyResilienceScale(heat, scale)
	applyResilienceScale(cold, scale)
	applyResilienceScale(gravity, scale)
	applyResilienceScale(radiation, scale)

	return RaceCurves{
		Reproduction: race.Attributes.Reproduction,
		Curves: map[string]*ComponentCurve{
			"heat":      heat,
			"cold":      cold,
			"gravity":   gravity,
			"radiation": radiation,
		},
	}, nil
}

// humanCurve — текущая человеческая кривая компоненты из глобального store
// (99.2.17) в момент генерации; дефолт, если store пуст (недостижимо).
func humanCurve(component string) *ComponentCurve {
	if c, ok := getCurveRef(component); ok && c != nil {
		return cloneCurve(c)
	}
	return defaultCurve(component)
}

// shiftHeatCurve — сдвиг X + растяжение горячей ветки (§3.3, жара).
func shiftHeatCurve(c *ComponentCurve, optHiK, stretch float64) *ComponentCurve {
	anchor := optHiK - 273.15 // °C: первый узел (30 °C) → opt_hi
	out := &ComponentCurve{
		Nodes: make([]SegmentNode, len(c.Nodes)),
		Bends: append([]float64(nil), c.Bends...),
	}
	for i, n := range c.Nodes {
		out.Nodes[i] = SegmentNode{X: anchor + (n.X-30)*stretch, Y: n.Y}
	}
	return out
}

// shiftColdCurve — сдвиг X + пиннинг левого узла на 0 K (§3.3, холод).
func shiftColdCurve(c *ComponentCurve, optLoK float64) *ComponentCurve {
	shift := optLoK - 288 // °C: правый узел (14.85 °C = 288 K) → opt_lo
	nodes := make([]SegmentNode, len(c.Nodes))
	for i, n := range c.Nodes {
		nodes[i] = SegmentNode{X: n.X + shift, Y: n.Y}
	}
	// Пиннинг левого узла на абсолютный ноль с Y = 0.05 (максимум
	// человеческой кривой); узлы ниже −273.15 °C отбрасываются (с bends).
	nodes[0] = SegmentNode{X: -273.15, Y: 0.05}
	keep := []SegmentNode{nodes[0]}
	keepBends := []float64{}
	for i := 1; i < len(nodes); i++ {
		if nodes[i].X < -273.15 {
			continue // узел и его bend (i−1) отбрасываются
		}
		keep = append(keep, nodes[i])
		keepBends = append(keepBends, c.Bends[i-1])
	}
	return &ComponentCurve{Nodes: keep, Bends: keepBends}
}

// shiftGravityCurve — ремап комфорта (0.8, 1.2) → (opt_lo, opt_hi) (§3.3).
func shiftGravityCurve(c *ComponentCurve, optLo, optHi float64) *ComponentCurve {
	loShift := optLo - 0.8
	hiShift := optHi - 1.2
	nodes := make([]SegmentNode, len(c.Nodes))
	for i, n := range c.Nodes {
		x := n.X
		switch {
		case n.X <= 0.8:
			x = n.X + loShift
		case n.X >= 1.2:
			x = n.X + hiShift
		}
		nodes[i] = SegmentNode{X: x, Y: n.Y}
	}
	// Узлы ниже 0 g отбрасываются (левая ветка клампится к 0 g).
	keep := []SegmentNode{}
	keepBends := []float64{}
	for i, n := range nodes {
		if n.X < 0 {
			continue
		}
		if len(keep) > 0 {
			keepBends = append(keepBends, c.Bends[i-1])
		}
		keep = append(keep, n)
	}
	return &ComponentCurve{Nodes: keep, Bends: keepBends}
}

// shiftRadiationCurve — сдвиг X: первый узел (20, Y=0) → opt_hi (§3.3).
func shiftRadiationCurve(c *ComponentCurve, optHi float64) *ComponentCurve {
	shift := optHi - 20
	nodes := make([]SegmentNode, len(c.Nodes))
	for i, n := range c.Nodes {
		nodes[i] = SegmentNode{X: n.X + shift, Y: n.Y}
	}
	// Узлы ниже 0 rad отбрасываются (кламп к 0).
	keep := []SegmentNode{}
	keepBends := []float64{}
	for i, n := range nodes {
		if n.X < 0 {
			continue
		}
		if len(keep) > 0 {
			keepBends = append(keepBends, c.Bends[i-1])
		}
		keep = append(keep, n)
	}
	return &ComponentCurve{Nodes: keep, Bends: keepBends}
}

// applyResilienceScale — Y всех узлов × scale, кламп [0, 0.999) (§3.3):
// максимум человеческой кривой 0.98 × 1.43 = 1.40 → кламп строго ниже
// 0.999 (валидация кривой требует y < 0.999).
func applyResilienceScale(c *ComponentCurve, scale float64) {
	for i := range c.Nodes {
		y := c.Nodes[i].Y * scale
		if y >= 0.999 {
			y = 0.999 - 1e-9
		}
		c.Nodes[i].Y = y
	}
}