// Компоненты изменения населения — единое место для суммы R_total (99.2.12,
// 99.2.13, концепция создателя 2026-09-14): изменение = сумма слагаемых,
// каждое может быть убылью (отрицательное) или ростом (положительное). Все
// компоненты рекурсивные, за секунду (p·(1−r)^Δt_сек; будущий рост — r < 0 →
// (1−r) > 1). Новые источники (убыль/рост/ограничения) добавляются ЗДЕСЬ —
// единственная точка сборки ChangeComponents. Сейчас:
//   - NaturalComponent (естественная: 1/СПЖ 50 лет ≈ 6.34·10⁻¹⁰/сек);
//   - HeatTemperatureChangeRate (жара, 0 при T ≤ 30 °C);
//   - ColdChangeRate (холод, 0 при T ≥ 288 K);
//   - GravityChangeRate (двусторонняя, 0 в комфорте 0.8–1.2 g);
//   - RadiationChangeRate (радиация, 0 при rad ≤ 20 — фоновый уровень);
//   - ограничение-потолок: возраст ≥ 120 лет → p = 0 (MaxLifespanSeconds,
//     применяется в Recompute).
// λ-механизм (exp(−λ·Δt)) удалён (99.2.13): всё изменение считается
// рекурсией. Гвард робастности r ≥ 1 (мгновенная гибель, без NaN) — в
// Population.
package settlement

import "math"

// LifeExpectancySeconds — средняя продолжительность жизни человека: 50 лет
// (решение создателя 2026-09-14, 99.2.12 §Решение).
const LifeExpectancySeconds = 50 * 365.25 * 24 * 3600 // 1.57788·10⁹

// MaxLifespanSeconds — возрастной потолок поселения: 120 лет от created_at
// (компонента-ограничение: при age ≥ 120 лет население = 0, 99.2.12).
const MaxLifespanSeconds = 120 * 365.25 * 24 * 3600

// NaturalChangeRate — естественная компонента изменения за 1 секунду:
// 1/СПЖ (СПЖ = 50 лет, решение создателя 2026-09-14) ≈ 6.34·10⁻¹⁰/сек —
// 2%/год, t50 ≈ 34.7 года, полное вымирание 1·10⁹ ≈ 1036 лет (но не позже
// потолка 120 лет).
const NaturalChangeRate = 1.0 / LifeExpectancySeconds

// Константы чисто температурной сшивки R_жара(T) (99.2.12, §Решение;
// приняты создателем 2026-09-14; контрольные точки — критерий тестера, в коде
// их нет). R_жара НЕ содержит естественную компоненту: R_жара(30 °C) = 0.
const (
	// Температурная зона «натурального роста» 30–~90 °C:
	// R_натур_темп(T) = c·(T−30)² — подобрано так, что R_total(70) ≈ 6.6·10⁻⁹.
	heatTempC = 3.71e-12

	// Тепловая зона ~90–4000 °C: R_тепл_темп = 1 − exp(−a·((T−30)/100)^n).
	rThermalA = 2.92e-4
	rThermalN = 2.581

	// Сшивка: σ(T) = 1/(1 + exp(−kσ·(T−T_сш))), центр в 90 °C
	// («порог кипения» — скачок 70→100 держит сшивка, не одна формула).
	rSigmaK = 1.0
	rSigmaT = 90.0

	// Рампа включения естественной компоненты на [15, 30) °C (15 °C =
	// 288.15 K — «288 K → R = 0» сохранено).
	rRampT = 15.0
)

// Константы R_холод(T) (99.2.13, §Рекомендация; калибруются численно по
// контрольным точкам — критерий тестера; c_у/c_к/k_х предварительные).
const (
	coldModerateC = 1.76e-6 // c_у: R_ум = c_у·(288−T)^0.63 (умеренная зона)
	coldCryoC     = 8.2e-7  // c_к: R_кр = c_к·(150−T)^2.2 + R_ум(150) (крио-зона)
	coldSigmaK    = 1.0     // k_х: сшивка σ(T) = 1/(1+exp(+k_х·(T−150)))
	coldSigmaT    = 150.0   // центр сшивки, K
)

// Константы R_гравитация(g) (99.2.13, §Рекомендация).
const (
	gravityLowC  = 2.17e-7 // c_н: R_н = c_н·(0.8−g)^3.3 (невесомость)
	gravityHighC = 6.78e-5 // c_в: R_в = c_в·(g−1.2)^1.54 (перегрузка)
)

// Константы R_радиация(rad) (99.2.13, §Рекомендация; шкала 0..100, порог
// 20 = фоновый уровень — предположение «логарифм дозы»; n=6.9, c_р=7.0e-17
// — калибровка по 4 точкам).
const (
	radiationC    = 7.0e-17 // c_р: R = c_р·(rad−20)^n
	radiationN    = 6.9
	radiationPort = 20.0 // порог включения (фон; ниже — 0)
)

// NaturalComponent — естественная компонента изменения за 1 секунду
// (99.2.12, §Решение): рампа [15, 30) °C → NaturalChangeRate·(T−15)/15;
// T ≥ 30 °C → NaturalChangeRate; T < 15 °C → 0 (комфорт).
func NaturalComponent(tempK float64) float64 {
	tempC := tempK - 273.15
	switch {
	case tempC >= 30:
		return NaturalChangeRate
	case tempC >= 15:
		return NaturalChangeRate * (tempC - rRampT) / 15
	default:
		return 0
	}
}

// HeatTemperatureChangeRate — рекурсивная компонента жары за 1 секунду
// (99.2.12, §Решение): R_жара(T), БЕЗ вшитой естественной компоненты.
// 0 при T ≤ 30 °C; выше — двухрежимная сшивка:
// R_жара = R_натур_темп·(1−σ) + R_тепл_темп·σ, где
// R_натур_темп = heatTempC·(T−30)², R_тепл_темп = 1 − exp(−rThermalA·((T−30)/100)^n).
func HeatTemperatureChangeRate(tempK float64) float64 {
	tempC := tempK - 273.15
	if tempC <= 30 {
		return 0
	}
	rNatTemp := heatTempC * (tempC - 30) * (tempC - 30)
	rThermTemp := 1 - math.Exp(-rThermalA*math.Pow((tempC-30)/100, rThermalN))
	sigma := 1 / (1 + math.Exp(-rSigmaK*(tempC-rSigmaT)))
	return rNatTemp*(1-sigma) + rThermTemp*sigma
}

// ColdChangeRate — рекурсивная компонента холода за 1 секунду, R_холод(T)
// (99.2.13, §Рекомендация): 0 при T ≥ 288 K; умеренная зона
// R_ум = c_у·(288−T)^0.63; крио-зона R_кр = c_к·(150−T)^2.2 + R_ум(150) при
// T < 150 K; сшивка σ(T) = 1/(1+exp(+k_х·(T−150))) — при T > 150 σ → 0
// (R_ум), при T < 150 σ → 1 (R_кр), σ(150) = 0.5 и R_ум(150) = R_кр(150).
// Контроль: 223 K → 2.44·10⁻⁵, 173 → 3.50·10⁻⁵, 150 → 3.92·10⁻⁵,
// 73 → 0.0117, 23 → 0.0349.
func ColdChangeRate(tempK float64) float64 {
	if tempK >= 288 {
		return 0
	}
	rMod := coldModerateC * math.Pow(288-tempK, 0.63)
	rMod150 := coldModerateC * math.Pow(288-150, 0.63)
	rCryo := rMod150
	if tempK < 150 {
		rCryo = coldCryoC*math.Pow(150-tempK, 2.2) + rMod150
	}
	sigma := 1 / (1 + math.Exp(coldSigmaK*(tempK-coldSigmaT)))
	return rMod*(1-sigma) + rCryo*sigma
}

// GravityChangeRate — рекурсивная компонента гравитации за 1 секунду,
// R_гравитация(g) (99.2.13, §Рекомендация): двусторонняя — низкая ветка
// R_н = c_н·(0.8−g)^3.3 при g < 0.8 (невесомость), высокая
// R_в = c_в·(g−1.2)^1.54 при g > 1.2 (перегрузка); 0 в комфорте 0.8–1.2.
// Контроль: 0.3 → 2.19·10⁻⁸, 0.1 → 6.6·10⁻⁸, 2 → 4.8·10⁻⁵,
// 3 → 1.2·10⁻⁴, 5 → 4.8·10⁻⁴, 10 → 1.92·10⁻³.
func GravityChangeRate(g float64) float64 {
	var r float64
	if g < 0.8 {
		r += gravityLowC * math.Pow(0.8-g, 3.3)
	}
	if g > 1.2 {
		r += gravityHighC * math.Pow(g-1.2, 1.54)
	}
	return r
}

// RadiationChangeRate — рекурсивная компонента радиации за 1 секунду,
// R_радиация(rad) (99.2.13, §Рекомендация): 0 при rad ≤ 20 (фоновый
// уровень); R = c_р·(rad−20)^6.9 при rad > 20. Контроль: 40 → 6.7·10⁻⁸,
// 60 → 8·10⁻⁶, 80 → 1.3·10⁻⁴, 100 → 9.5·10⁻⁴.
func RadiationChangeRate(rad float64) float64 {
	if rad <= radiationPort {
		return 0
	}
	return radiationC * math.Pow(rad-radiationPort, radiationN)
}

// ChangeComponents — полная рекурсивная компонента изменения населения за
// 1 секунду, r (99.2.12, 99.2.13): сумма всех слагаемых —
// R_ест + R_жара + R_холод + R_гравитация + R_радиация. Единая точка
// сборки: новые источники (убыль/рост) добавляются здесь. λ-механизма нет
// (всё рекурсией, 99.2.13). Гвард r ≥ 1 (мгновенная гибель) — в Population.
func ChangeComponents(input PlanetInput) float64 {
	return NaturalComponent(input.TemperatureK) +
		HeatTemperatureChangeRate(input.TemperatureK) +
		ColdChangeRate(input.TemperatureK) +
		GravityChangeRate(input.GravityG) +
		RadiationChangeRate(input.CoreRadioactivity)
}