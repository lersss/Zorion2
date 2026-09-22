// Компоненты изменения населения — единое место для суммы R_total (99.2.12,
// 99.2.13, 99.2.16, 99.2.17, концепция создателя 2026-09-14): изменение =
// сумма слагаемых, каждое может быть убылью (отрицательное) или ростом
// (положительное). Все компоненты рекурсивные, за секунду (p·(1−r)^Δt_сек;
// рост — r < 0 → (1−r) > 1). Новые источники (убыль/рост) добавляются ЗДЕСЬ —
// единственная точка сборки ChangeComponents. Сейчас:
//   - NaturalComponent (естественная смертность: 1/СПЖ, дефолт 50 лет ≈ 6.34·10⁻¹⁰/сек);
//   - BirthComponent (естественная рождаемость: −k·NaturalComponent, дефолт k=2, 99.2.16);
//   - HeatTemperatureChangeRate (жара, 0 при T ≤ 30 °C);
//   - ColdChangeRate (холод, 0 при T ≥ 288 K);
//   - GravityChangeRate (двусторонняя, 0 в комфорте 0.8–1.2 g);
//   - RadiationChangeRate (радиация, 0 при rad ≤ 20 — фоновый уровень).
//
// Возрастного потолка нет (99.2.16, решение создателя 2026-09-15): возраст
// поселения не участвует. λ-механизм (exp(−λ·Δt)) удалён (99.2.13): всё
// изменение считается рекурсией. Гвард робастности r ≥ 1 (мгновенная гибель,
// без NaN) — в Population.
package settlement

import "log"

// Рампа включения естественной компоненты на [15, 30) °C (15 °C =
// 288.15 K — «288 K → R = 0» сохранено). Архитектурная константа, вне
// балансировщика (99.2.17 §12): рампа естественной компоненты — не баланс
// компонент, инструментом не редактируется.
const rRampT = 15.0

// NaturalComponent — естественная компонента изменения за 1 секунду
// (99.2.12, §Решение; 99.2.16: NaturalChangeRate теперь функция настроек):
// рампа [15, 30) °C → NaturalChangeRate()·(T−15)/15;
// T ≥ 30 °C → NaturalChangeRate(); T < 15 °C → 0 (комфорт).
func NaturalComponent(tempK float64) float64 {
	tempC := tempK - 273.15
	switch {
	case tempC >= 30:
		return NaturalChangeRate()
	case tempC >= 15:
		return NaturalChangeRate() * (tempC - rRampT) / 15
	default:
		return 0
	}
}

// BirthComponent — естественная рождаемость за 1 секунду (99.2.16, §2.1):
// обратная смертности по знаку и масштабу, R_рожд = −k·R_ест (k — коэффициент
// рождаемости, дефолт 2). Та же температурная рампа, что у NaturalComponent
// (отдельную воронку рождаемости не вводим — §2.2); в причинах гибели
// (DeathCause) не участвует (не «причина», §2.1).
func BirthComponent(tempK float64) float64 {
	return -BirthRateCoefficient() * NaturalComponent(tempK)
}

// HeatTemperatureChangeRate — рекурсивная компонента жары за 1 секунду
// (99.2.17): сегментная кривая из balancer store (узлы + изгибы, дефолты =
// контрольные точки 99.2.12). Кривая жары хранится в °C, мини-R принимает K
// → evaluateCurve(curve, tempK − 273.15). Контроль: 70 → 5.94e-9,
// 100 → 1.16e-4, 200 → 1.15e-3, 300 → 3.79e-3, 500 → 1.57e-2,
// 1000 → 9.77e-2, 2000 → 0.473, 4000 → 0.980 (допуск теста §9).
func HeatTemperatureChangeRate(tempK float64) float64 {
	c, _ := getCurveRef("heat")
	return evaluateCurve(c.Nodes, c.Bends, tempK-273.15) // кривая в °C
}

// ColdChangeRate — рекурсивная компонента холода за 1 секунду, R_холод(T)
// (99.2.17): сегментная кривая из balancer store (дефолты = контрольные
// точки 99.2.13). Кривая холода хранится в °C, мини-R принимает K
// → evaluateCurve(curve, tempK − 273.15). Контроль (вход K): 0 → 5.03e-2,
// 23 → 3.49e-2, 73 → 1.17e-2, 150 → 3.92e-5, 173 → 3.50e-5,
// 223 → 2.44e-5, 288 → 0.
func ColdChangeRate(tempK float64) float64 {
	c, _ := getCurveRef("cold")
	return evaluateCurve(c.Nodes, c.Bends, tempK-273.15) // кривая в °C
}

// GravityChangeRate — рекурсивная компонента гравитации за 1 секунду,
// R_гравитация(g) (99.2.17): сегментная кривая из balancer store (дефолты =
// контрольные точки 99.2.13). Контроль: 0 → 1.04e-7, 0.1 → 6.69e-8,
// 0.3 → 2.20e-8, 0.8 → 0, 1.2 → 0, 2 → 4.81e-5, 3 → 1.68e-4,
// 5 → 5.30e-4, 10 → 1.93e-3.
func GravityChangeRate(g float64) float64 {
	c, _ := getCurveRef("gravity")
	return evaluateCurve(c.Nodes, c.Bends, g)
}

// RadiationChangeRate — рекурсивная компонента радиации за 1 секунду,
// R_радиация(rad) (99.2.17): сегментная кривая из balancer store (дефолты =
// контрольные точки 99.2.13). Контроль: 0 → 0, 20 → 0, 40 → 6.64e-8,
// 60 → 7.93e-6, 80 → 1.30e-4, 100 → 9.47e-4.
func RadiationChangeRate(rad float64) float64 {
	c, _ := getCurveRef("radiation")
	return evaluateCurve(c.Nodes, c.Bends, rad)
}

// EnvComponents — средовая (без эффектов) рекурсивная компонента изменения
// населения за 1 секунду: R_ест + R_рожд + R_жара + R_холод + R_гравитация +
// R_радиация. Человеческая модель ИЛИ расовая (RaceID ≠ NULL/"humans" →
// changeComponentsRace) — вынесено отдельно, чтобы Recompute считал вклад
// эффектов посегментно через EffectForcePoint, а не через ChangeComponents
// (иначе двойной учёт, спека 2026-09-22-эффекты-снабжения §5.3).
func EnvComponents(input PlanetInput) float64 {
	if input.RaceID != "" && input.RaceID != "humans" {
		return changeComponentsRace(input)
	}
	return NaturalComponent(input.TemperatureK) +
		BirthComponent(input.TemperatureK) +
		HeatTemperatureChangeRate(input.TemperatureK) +
		ColdChangeRate(input.TemperatureK) +
		GravityChangeRate(input.GravityG) +
		RadiationChangeRate(input.CoreRadioactivity)
}

// ChangeComponents — полная рекурсивная компонента изменения населения за
// 1 секунду, r (99.2.12, 99.2.13, 99.2.16): EnvComponents (среда, человеческая
// или расовая) + Σ R_e(AsOf) — вклад эффектов в сегменте, содержащем AsOf
// (спека 2026-09-22-эффекты-снабжения §5.2/§5.3). Единая точка сборки для
// «точечных» потребителей (DeathTime/Projection/витрина): новые источники
// (убыль/рост) добавляются здесь. λ-механизма нет (всё рекурсией, 99.2.13).
// Гвард r ≥ 1 (мгновенная гибель) — в Population. RaceID ≠ NULL/"humans" →
// active-кривые расы; расовые поселения голодают наравне (вклад эффекта —
// после диспетчеризации, О10/§7.2).
func ChangeComponents(input PlanetInput) float64 {
	r := EnvComponents(input)
	for _, p := range input.Effects {
		r += p.RateAt(input.AsOf)
	}
	return r
}

// changeComponentsRace — расовая R-модель (99.2.23 §3.2):
//
//	R_total_расы = active.reproduction · (1 − k) · R_ест
//	             + evaluateCurve(active.curves.heat, T°C)
//	             + evaluateCurve(active.curves.cold, T)
//	             + evaluateCurve(active.curves.gravity, g)
//	             + evaluateCurve(active.curves.radiation, rad)
//
// Естественная пара — константа (без человеческой рампы [15, 30] °C):
// R_ест_расы = active.reproduction·R_ест, R_рожд_расы = −k·active.reproduction·R_ест,
// нетто active.reproduction·(1−k)·R_ест. Resilience уже вшит в Y при
// генерации — отдельного масштаба в формуле нет. Гвард отсутствия записи:
// расы нет в расовом store (теоретически невозможно после авто-инициализации;
// страховка) → человеческая модель + лог-предупреждение (сервер не падает).
func changeComponentsRace(input PlanetInput) float64 {
	rc, ok := GetRaceActiveCurves(input.RaceID)
	if !ok {
		log.Printf("race balancer: раса %q без записи в store — человеческая модель (гвард 99.2.23 §3.2)", input.RaceID)
		return NaturalComponent(input.TemperatureK) +
			BirthComponent(input.TemperatureK) +
			HeatTemperatureChangeRate(input.TemperatureK) +
			ColdChangeRate(input.TemperatureK) +
			GravityChangeRate(input.GravityG) +
			RadiationChangeRate(input.CoreRadioactivity)
	}
	netto := rc.Reproduction * (1 - BirthRateCoefficient()) * NaturalChangeRate()
	return netto +
		evaluateCurve(rc.Curves["heat"].Nodes, rc.Curves["heat"].Bends, input.TemperatureK-273.15) +
		evaluateCurve(rc.Curves["cold"].Nodes, rc.Curves["cold"].Bends, input.TemperatureK-273.15) +
		evaluateCurve(rc.Curves["gravity"].Nodes, rc.Curves["gravity"].Bends, input.GravityG) +
		evaluateCurve(rc.Curves["radiation"].Nodes, rc.Curves["radiation"].Bends, input.CoreRadioactivity)
}
