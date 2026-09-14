// Компоненты изменения населения — единое место для суммы R_total (99.2.12,
// концепция создателя 2026-09-14): изменение = сумма слагаемых, каждое может
// быть убылью (отрицательное) или ростом (положительное). Новые источники
// (убыль/рост/ограничения) добавляются ЗДЕСЬ — единственная точка сборки
// ChangeComponents. Сейчас:
//   - рекурсивные компоненты, за 1 секунду (p·(1−r)^Δt_сек; будущий рост —
//     r < 0 → (1−r) > 1), собираются в RecursiveChangeRate:
//       · NaturalComponent (естественная: 1/СПЖ 50 лет ≈ 6.34·10⁻¹⁰/сек);
//       · HeatTemperatureChangeRate (чисто температурная, 0 при T ≤ 30 °C);
//   - ограничение-потолок: возраст поселения ≥ 120 лет → население 0
//     (MaxLifespanSeconds, применяется в Recompute — не слагаемое, а жёсткий
//     предел);
//   - экспоненциальная компонента, за час (p·exp(−λ·Δt_ч)): OtherChangeRate —
//     холод/гравитация/радиоактивность.
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

// Константы чисто температурной сшивки R_темп(T) (99.2.12, §Решение;
// приняты создателем 2026-09-14; контрольные точки — критерий тестера, в коде
// их нет). R_темп НЕ содержит естественную компоненту: R_темп(30 °C) = 0.
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

// HeatTemperatureChangeRate — чисто температурная компонента изменения за
// 1 секунду (99.2.12, §Решение): R_темп(T), БЕЗ вшитой естественной
// компоненты. 0 при T ≤ 30 °C; выше — двухрежимная сшивка:
// R_темп = R_натур_темп·(1−σ) + R_тепл_темп·σ, где
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

// RecursiveChangeRate — рекурсивная компонента изменения за 1 секунду:
// сумма естественной (NaturalComponent) и чисто температурной
// (HeatTemperatureChangeRate). Полное r для ChangeComponents, Uninhabitable и
// DeathCause (порог «t_смерти» считает по полному r).
func RecursiveChangeRate(tempK float64) float64 {
	return NaturalComponent(tempK) + HeatTemperatureChangeRate(tempK)
}

// ColdChangeRate — λ-компонента изменения от холода, за час (99.2.12,
// R-модель; холод временно на полюсе B): T < 200 K — полюсная кривая B
// (T ≤ 100 K → +Inf); T ≥ 200 K — 0 (комфорт; жара считается рекурсивной
// компонентой, в λ не входит).
func ColdChangeRate(tempK float64, scale Scale) float64 {
	if tempK < 200 {
		return SeverityRate(TwoSidedSeverity(HumanTemperatureProfile, tempK), scale)
	}
	return 0
}

// OtherChangeRate считает λ-компоненты изменения населения от «прочих»
// факторов среды (холод — полюс B временно, гравитация, радиоактивность),
// за час. Жара в λ НЕ входит: её рекурсивная компонента — HeatChangeRate
// (99.2.12, R-модель). Компоненты складываются, а не берётся худший —
// несколько угроз меняют население быстрее одной (18a, «Механизм»).
// Гравитация/холод в жёстком нуле → +Inf (+Inf + конечное = +Inf).
func OtherChangeRate(input PlanetInput, scale Scale) float64 {
	if hardZero(HumanGravityProfile, input.GravityG) {
		return math.Inf(1)
	}
	temperature := ColdChangeRate(input.TemperatureK, scale)
	gravity := SeverityRate(TwoSidedSeverity(HumanGravityProfile, input.GravityG), scale)
	radioactivity := SeverityRate(OneSidedSeverity(HumanRadioactivityProfile, input.CoreRadioactivity), scale)
	return temperature + gravity + radioactivity
}

// ChangeComponents — компоненты изменения населения для профиля человека
// (99.2.12, концепция создателя 2026-09-14): единая точка сборки слагаемых.
// Возвращает рекурсивную компоненту r (за секунду, RecursiveChangeRate =
// NaturalComponent + HeatTemperatureChangeRate) и λ-компоненту прочих
// факторов (за час, OtherChangeRate). Новые источники изменения (в т.ч.
// рост: отрицательное слагаемое) добавляются здесь.
func ChangeComponents(input PlanetInput, scale Scale) (r float64, lambdaPerHour float64) {
	return RecursiveChangeRate(input.TemperatureK), OtherChangeRate(input, scale)
}