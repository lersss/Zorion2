// internal/resource/diet.go — вывод окон расы (спека 94a §3.2): из consumption
// + шаблонов механически. Окна не хранятся (инвариант 8) — чистая функция.
package resource

import "sort"

// DietWindow — окно расы по одной consumption-оси (шаблон × resilience).
type DietWindow struct {
	Axis    string       // consumption-ось
	Weight  float64      // доля диеты (0–100), не ширина (§3.2 п.3)
	TMelt   *Interval    // окно T_melt (nil — нет окна)
	TBoil   *Interval    // окно T_boil (nil — нет окна)
	Phase   Phase        // пригодная фаза пищи
	Windows []AxisWindow // окна по осям свойств
}

// RaceWindows — потребность расы (§3.2): набор независимых окон (по одному
// на consumption-ось) + объединённые окна по осям свойств.
type RaceWindows struct {
	PerAxis []DietWindow          // по одному на ось с весом > 0
	Union   map[string][]Interval // объединение окон по осям свойств
	TMelt   []Interval            // объединение T_melt-интервалов шаблонов
	TBoil   []Interval            // объединение T_boil-интервалов шаблонов
}

// ResilienceMultiplier — мягкий множитель ширины окна (§3.2 п.3): устойчивые
// расы едят чуть шире. Дефолт ×1.0 при resilience 50 (человек); диапазон
// 0.9–1.15. Предположение дизайнера (проверит @balancetester): кусочно-
// линейно — 0→0.9, 50→1.0, 100→1.15.
func ResilienceMultiplier(resilience float64) float64 {
	switch {
	case resilience <= 0:
		return 0.9
	case resilience >= 100:
		return 1.15
	case resilience <= 50:
		return 0.9 + (resilience/50)*0.1
	default:
		return 1.0 + ((resilience-50)/50)*0.15
	}
}

// scaleInterval — множитель ширины окна вокруг центра.
func scaleInterval(iv Interval, m float64) Interval {
	if m == 1.0 {
		return iv
	}
	c := (iv.Lo + iv.Hi) / 2
	w := (iv.Hi - iv.Lo) * m / 2
	return Interval{Lo: c - w, Hi: c + w}
}

// deriveWindow — окно расы по одной consumption-оси (шаблон × resilience).
func deriveWindow(axis string, weight float64, tpl *ChemotypeTemplate, m float64) DietWindow {
	dw := DietWindow{Axis: axis, Weight: weight, Phase: tpl.Phase}
	if tpl.TMelt != nil {
		iv := scaleInterval(*tpl.TMelt, m)
		dw.TMelt = &iv
	}
	if tpl.TBoil != nil {
		iv := scaleInterval(*tpl.TBoil, m)
		dw.TBoil = &iv
	}
	for _, aw := range tpl.Windows {
		iv := scaleInterval(Interval{Lo: aw.Lo, Hi: aw.Hi}, m)
		dw.Windows = append(dw.Windows, AxisWindow{Axis: aw.Axis, Lo: iv.Lo, Hi: iv.Hi})
	}
	return dw
}

// sortedConsumptionAxes — оси consumption в детерминированном порядке
// (порядок итерации map в Go рандомизирован — вывод обязан быть стабильным).
func sortedConsumptionAxes(consumption map[string]float64) []string {
	axes := make([]string, 0, len(consumption))
	for axis := range consumption {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	return axes
}

// DeriveDiet — вывод окон расы (§3.2): из consumption + шаблонов механически.
// Чистая функция (инвариант 8: окна не хранятся, выводятся на лету).
func DeriveDiet(consumption map[string]float64, resilience float64, templates map[string]*ChemotypeTemplate) RaceWindows {
	m := ResilienceMultiplier(resilience)
	var perAxis []DietWindow
	union := map[string][]Interval{}
	var tMelt, tBoil []Interval
	for _, axis := range sortedConsumptionAxes(consumption) {
		weight := consumption[axis]
		if weight <= 0 {
			continue
		}
		tpl, ok := templates[axis]
		if !ok {
			continue // ось без шаблона — не участвует
		}
		dw := deriveWindow(axis, weight, tpl, m)
		perAxis = append(perAxis, dw)
		if dw.TMelt != nil {
			tMelt = append(tMelt, *dw.TMelt)
		}
		if dw.TBoil != nil {
			tBoil = append(tBoil, *dw.TBoil)
		}
		for _, aw := range dw.Windows {
			union[aw.Axis] = append(union[aw.Axis], Interval{Lo: aw.Lo, Hi: aw.Hi})
		}
	}
	return RaceWindows{PerAxis: perAxis, Union: union, TMelt: tMelt, TBoil: tBoil}
}