// internal/resource/checks.go — проверки каталога/шаблонов/вывода окон
// (спека 94a §8 критерии, §13 инварианты): генераторы-помощники §3.4.
package resource

import (
	"fmt"
	"math"

	"zorion/internal/races"
)

// upAxes — «↑»-оси требований товаров (порог ≥ 70, §5.2.3): биос, энерго,
// горюч, твёрд, эласт, провод, радио.
var upAxes = []string{
	AxisBiocompatibility, AxisEnergyDensity, AxisFlammability,
	AxisHardness, AxisElasticity, AxisConductivity, AxisRadioactivity,
}

// FedGap — непокрытая потребность (§8.1): раса, ось, вес.
type FedGap struct {
	RaceID string
	Axis   string
	Weight float64
}

// CheckAllFed — §8.1 «все накормлены»: для каждой био-расы и каждой
// consumption-оси с весом ≥ 10 существует ≥ 1 ресурс каталога, чей профиль
// (включая T как жёсткие интервалы) попадает в окно расы по этой оси.
// Окна включительные (инвариант 10). Возвращает дыры (пусто = все накормлены).
func CheckAllFed(catalog []*Resource, templates map[string]*ChemotypeTemplate, races []*races.Race) []FedGap {
	var gaps []FedGap
	for _, r := range races {
		if r.Robotic != nil {
			continue // роботы — отдельный слой (99.2.24), не в этой итерации
		}
		m := ResilienceMultiplier(r.Attributes.Resilience)
		for axis, weight := range r.Consumption {
			if weight < 10 {
				continue // доли < 10 необязательны (§8.1)
			}
			tpl, ok := templates[axis]
			if !ok {
				gaps = append(gaps, FedGap{RaceID: r.ID, Axis: axis, Weight: weight})
				continue
			}
			dw := deriveWindow(axis, weight, tpl, m)
			if !coveredByCatalog(dw, catalog) {
				gaps = append(gaps, FedGap{RaceID: r.ID, Axis: axis, Weight: weight})
			}
		}
	}
	return gaps
}

// coveredByCatalog — есть ли ресурс каталога, чей профиль попадает в окно.
func coveredByCatalog(w DietWindow, catalog []*Resource) bool {
	for _, res := range catalog {
		if covers(res, w) {
			return true
		}
	}
	return false
}

// CoveringResources — id ресурсов каталога, чей профиль (включая T как
// жёсткие интервалы) попадает в окно расы по consumption-оси (шаблон ×
// resilience, §3.2). Пуст, если окно не покрыто (дыра §8.1). Генератор-
// помощник §3.4 (функция 2 — поиск дыр/покрытия).
func CoveringResources(catalog []*Resource, templates map[string]*ChemotypeTemplate, race *races.Race, axis string) []string {
	tpl, ok := templates[axis]
	if !ok {
		return nil
	}
	m := ResilienceMultiplier(race.Attributes.Resilience)
	dw := deriveWindow(axis, race.Consumption[axis], tpl, m)
	var ids []string
	for _, res := range catalog {
		if covers(res, dw) {
			ids = append(ids, res.ID)
		}
	}
	return ids
}

// covers — попадает ли профиль ресурса в окно расы по consumption-оси:
// все окна по осям свойств + T_melt/T_boil как жёсткие интервалы (§8.1).
func covers(res *Resource, w DietWindow) bool {
	if w.TMelt != nil && !w.TMelt.Contains(res.TMelt) {
		return false
	}
	if w.TBoil != nil && !w.TBoil.Contains(res.TBoil) {
		return false
	}
	for _, aw := range w.Windows {
		v, ok := res.Value(aw.Axis)
		if !ok || v < aw.Lo || v > aw.Hi {
			return false
		}
	}
	return true
}

// CheckMediocrity — §8.2 посредственность: все «↑»-оси товаров у всех ресурсов
// каталога < 70 (порог §5.2.3). Проверка по конъюнкции: каждый товар требует
// ≥ 1 «↑»-ось, и ни один ресурс не проходит ни одну на уровне ≥ 70 → каталог
// не закрывает ни один товарный профиль. Возвращает максимумы по «↑»-осям.
func CheckMediocrity(catalog []*Resource) map[string]float64 {
	maxima := make(map[string]float64, len(upAxes))
	for _, axis := range upAxes {
		maxima[axis] = 0
	}
	for _, res := range catalog {
		for _, axis := range upAxes {
			if v, _ := res.Value(axis); v > maxima[axis] {
				maxima[axis] = v
			}
		}
	}
	return maxima
}

// CheckRanges — §8.4: оси в [0,100]; T в [10,6000]; T_melt < T_boil с зазором
// ≥ 5 K кроме сублимирующих; сумма consumption = 100 (инвариант 7).
// Возвращает список нарушений (пуст = всё в диапазонах).
func CheckRanges(catalog []*Resource, races []*races.Race) []string {
	var violations []string
	for _, res := range catalog {
		for _, axis := range allAxes {
			v, _ := res.Value(axis)
			if v < 0 || v > 100 {
				violations = append(violations, fmt.Sprintf("%s: ось %s = %v вне [0,100]", res.ID, axis, v))
			}
		}
		if res.TMelt < 10 || res.TMelt > 6000 {
			violations = append(violations, fmt.Sprintf("%s: T_melt = %v вне [10,6000]", res.ID, res.TMelt))
		}
		if res.TBoil < 10 || res.TBoil > 6000 {
			violations = append(violations, fmt.Sprintf("%s: T_boil = %v вне [10,6000]", res.ID, res.TBoil))
		}
		if !res.Sublimating() && res.TBoil-res.TMelt < 5 {
			violations = append(violations, fmt.Sprintf("%s: зазор T_boil−T_melt = %v < 5", res.ID, res.TBoil-res.TMelt))
		}
	}
	for _, r := range races {
		if r.Robotic != nil {
			continue
		}
		sum := 0.0
		for _, v := range r.Consumption {
			sum += v
		}
		if math.Abs(sum-100) > 0.01 {
			violations = append(violations, fmt.Sprintf("раса %s: сумма consumption = %v ≠ 100", r.ID, sum))
		}
	}
	return violations
}

// CheckTemplateRanges — конвенция §3.1: T-окна шаблонов не соприкасаются
// вплотную (иначе реализация может дать melt ≥ boil): для нормальных
// T_boil_lo − T_melt_hi ≥ 5. Сублимирующие (CO2) и без-T-оконные (ГРД/ПЫЛ/ИЗЛ)
// пропускаются — для них перекрытие/отсутствие окон по построению.
func CheckTemplateRanges(templates map[string]*ChemotypeTemplate) []string {
	var violations []string
	for axis, tpl := range templates {
		if tpl.TMelt == nil || tpl.TBoil == nil || tpl.Sublimating {
			continue
		}
		if tpl.TBoil.Lo-tpl.TMelt.Hi < 5 {
			violations = append(violations, fmt.Sprintf("%s: зазор T_boil−T_melt = %v < 5", axis, tpl.TBoil.Lo-tpl.TMelt.Hi))
		}
	}
	return violations
}

// CheckTemperatureCorrelation — §8.5: для каждой био-расы существует
// пересечение объединённого T-окна пищи с окном обитания
// (conditions.temperature.surv). Модель фазы — только по T, без P (§9.1.2).
// Возвращает список рас без пересечения (пуст = все прошли).
func CheckTemperatureCorrelation(templates map[string]*ChemotypeTemplate, races []*races.Race) []string {
	var failed []string
	for _, r := range races {
		if r.Robotic != nil {
			continue
		}
		if !raceFoodIntersectsHabitat(r, templates) {
			failed = append(failed, r.ID)
		}
	}
	return failed
}

// raceFoodIntersectsHabitat — есть ли у расы ось, чья пища в пригодной фазе
// хотя бы в части окна обитания. Без T-ограничений — корреляция тривиальна.
func raceFoodIntersectsHabitat(r *races.Race, templates map[string]*ChemotypeTemplate) bool {
	constrained := false
	for axis, weight := range r.Consumption {
		if weight <= 0 {
			continue
		}
		tpl, ok := templates[axis]
		if !ok {
			continue
		}
		edible := edibleInterval(tpl)
		if edible == nil {
			continue // фаза не ограничена
		}
		constrained = true
		if intersectsHabitat(edible, r.Conditions.Temperature.Surv) {
			return true
		}
	}
	return !constrained
}

// edibleInterval — интервал температур, где пища хемотипа в пригодной фазе
// (§3.3; модель фазы только по T, без P). nil — фаза не ограничена.
func edibleInterval(tpl *ChemotypeTemplate) *Interval {
	if tpl.TMelt == nil || tpl.TBoil == nil {
		return nil
	}
	switch tpl.Phase {
	case PhaseLiquid:
		// жидкая фаза: T_melt ≤ T < T_boil
		return &Interval{Lo: tpl.TMelt.Lo, Hi: tpl.TBoil.Hi}
	case PhaseSolid:
		// твёрдая фаза: T < min(T_melt, T_boil) (§4 уточнение 1)
		return &Interval{Lo: math.Inf(-1), Hi: math.Min(tpl.TMelt.Lo, tpl.TBoil.Lo)}
	case PhaseSupercritical:
		// сверхкритическая: T > T_boil
		return &Interval{Lo: tpl.TBoil.Lo, Hi: math.Inf(1)}
	}
	return nil
}

// intersectsHabitat — пересекается ли интервал пригодной фазы с окном обитания.
func intersectsHabitat(edible *Interval, habitat races.Range) bool {
	if edible == nil {
		return true
	}
	hhi := math.Inf(1)
	if habitat.Hi != nil {
		hhi = *habitat.Hi
	}
	return edible.Lo <= hhi && edible.Hi >= habitat.Lo
}

// CheckBridgeLiveness — инвариант 9: точечный профиль каждого мостового
// (включая T как жёсткие интервалы) попадает в окна шаблонов заявленных
// адресатов (Closes) — мёртвых записей нет. Возвращает список нарушений
// (пуст = все живы).
func CheckBridgeLiveness(catalog []*Resource, templates map[string]*ChemotypeTemplate) []string {
	var violations []string
	for _, res := range catalog {
		if !res.Bridge {
			continue
		}
		for _, axis := range res.Closes {
			tpl, ok := templates[axis]
			if !ok {
				violations = append(violations, fmt.Sprintf("%s: адресат %s без шаблона", res.ID, axis))
				continue
			}
			// Живость — против шаблонных окон (без resilience, §5.2 проверка).
			dw := deriveWindow(axis, 0, tpl, 1.0)
			if !covers(res, dw) {
				violations = append(violations, fmt.Sprintf("%s: профиль не попадает в окно адресата %s", res.ID, axis))
			}
		}
	}
	return violations
}