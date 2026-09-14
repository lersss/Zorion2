// Package settlement считает экономику поселения по запросу (ленивый
// пересчёт), без тика. Здесь — только изменение населения от физической
// среды планеты (docs/gamedesign/18a_population_death.md): дельта может быть
// убылью или ростом (сумма компонент, 99.2.12).
package settlement

import "math"

// SeverityShape — форма кривой тяжести профиля (99.2.12): линейная
// (severity = dev/Sat, кап на 1) или полюсная квадратичная
// (severity = (dev/Sat)² / (1 − (dev/H)²), полюс при dev = HardZero — вариант B).
type SeverityShape int

const (
	ShapeLinear SeverityShape = iota
	ShapeQuadratic
)

// TwoSidedProfile описывает переносимость величины, для которой одинаково
// плохо и завышенное, и заниженное значение — температура, гравитация (18a,
// «Профиль устойчивости»). Радиоактивность одностороння, сюда не подходит.
// Это данные, а не константы в теле функций: разные профили — для разных форм
// жизни, которых пока в игре нет.
type TwoSidedProfile struct {
	ComfortMin   float64      // нижняя граница комфорта
	ComfortMax   float64      // верхняя граница комфорта
	SaturateCold float64      // отклонение вниз от ComfortMin, масштаб кривой (dev/Sat)
	SaturateHot  float64      // отклонение вверх от ComfortMax, масштаб кривой (dev/Sat)
	HardZeroCold float64      // отклонение вниз до жёсткого нуля; 0 = не задан
	HardZeroHot  float64      // отклонение вверх до жёсткого нуля; 0 = не задан
	Shape        SeverityShape // форма кривой тяжести
}

// HumanTemperatureProfile — профиль человека по температуре для ХОЛОДНОЙ
// стороны (99.2.12): полюсная кривая B sev = (dev/Sat)²/(1−(dev/H)²), полюс
// холода на T ≤ 100 K (эталон «−200 °C → минуты»; временно, до перевода
// холода на R-модель). Жара больше НЕ severity — рекурсивная компонента
// изменения HeatChangeRate (см. ChangeComponents); горячие поля профиля не
// используются.
var HumanTemperatureProfile = TwoSidedProfile{
	ComfortMin:   200,
	ComfortMax:   350,
	SaturateCold: 350,
	SaturateHot:  60,
	HardZeroCold: 100,
	HardZeroHot:  100,
	Shape:        ShapeQuadratic,
}

// HumanGravityProfile — черновой профиль человека по гравитации, в g (1 = как
// на Земле). Поведение не менялось (99.2.12): линейная форма с капом на 1,
// константы черновые, калибруются отдельно.
var HumanGravityProfile = TwoSidedProfile{
	ComfortMin:   0.8,
	ComfortMax:   1.2,
	SaturateCold: 4,
	SaturateHot:  4,
	Shape:        ShapeLinear,
}

// TwoSidedSeverity считает тяжесть отклонения величины от комфорта: 0 внутри
// комфортного диапазона. Линейные профили капятся на 1; полюсные
// (квадратичные, вариант B) растут без капа и стремятся к +∞ у жёсткого нуля:
// sev = (dev/Sat)² / (1 − (dev/H)²), H = HardZero стороны (99.2.12).
// Guard: при dev ≥ HardZero возвращает +Inf ДО вычисления severity — иначе
// знаменатель неположителен (ноль при dev = H, отрицателен при dev > H →
// λ < 0 → население растёт). Линейные профили имеют HardZero = 0 — guard не
// срабатывает.
func TwoSidedSeverity(profile TwoSidedProfile, value float64) float64 {
	var deviation float64
	var saturateAt float64
	var hardZeroH float64
	switch {
	case value < profile.ComfortMin:
		deviation = profile.ComfortMin - value
		saturateAt = profile.SaturateCold
		hardZeroH = profile.HardZeroCold
	case value > profile.ComfortMax:
		deviation = value - profile.ComfortMax
		saturateAt = profile.SaturateHot
		hardZeroH = profile.HardZeroHot
	default:
		return 0
	}
	if saturateAt <= 0 {
		return 1
	}
	if profile.Shape == ShapeQuadratic && hardZeroH > 0 && deviation >= hardZeroH {
		return math.Inf(1)
	}
	severity := deviation / saturateAt
	if profile.Shape == ShapeQuadratic {
		// Полюсная форма B: sev = (dev/Sat)² / (1 − (dev/H)²), полюс при dev = H.
		if hardZeroH > 0 {
			return severity * severity / (1 - (deviation/hardZeroH)*(deviation/hardZeroH))
		}
		return severity * severity
	}
	if severity > 1 {
		return 1
	}
	return severity
}

// hardZero — истина, когда отклонение достигло жёсткого нуля стороны профиля
// (dev ≥ HardZero_стороны; 0 = не задан). Периметр проверки один: на него
// смотрят OtherChangeRate и Uninhabitable (99.2.12, H4).
func hardZero(profile TwoSidedProfile, value float64) bool {
	switch {
	case value < profile.ComfortMin:
		return profile.HardZeroCold > 0 && profile.ComfortMin-value >= profile.HardZeroCold
	case value > profile.ComfortMax:
		return profile.HardZeroHot > 0 && value-profile.ComfortMax >= profile.HardZeroHot
	}
	return false
}

// OneSidedProfile описывает переносимость величины, для которой плохо только
// превышение порога, а недостаток безопасен — радиоактивность (18a, «Профиль
// устойчивости»). Температура и гравитация двусторонние, см. TwoSidedProfile.
type OneSidedProfile struct {
	Threshold  float64      // ниже и на пороге — безопасно, тяжесть = 0
	SaturateAt float64      // превышение порога, масштаб кривой (dev/Sat)
	HardZeroAt float64      // превышение порога до жёсткого нуля; 0 = не задан
	Shape      SeverityShape // форма кривой тяжести
}

// HumanRadioactivityProfile — черновой профиль человека по фону ядра планеты
// (core.radioactivity, шкала 0..100). Поведение не менялось (99.2.12):
// линейная форма с капом на 1, константы черновые, калибруются отдельно.
var HumanRadioactivityProfile = OneSidedProfile{
	Threshold:  20,
	SaturateAt: 60,
	Shape:      ShapeLinear,
}

// OneSidedSeverity считает тяжесть превышения порога: 0 на пороге и ниже.
// Линейная форма — с капом на 1; полюсная квадратичная растёт без капа к +∞
// у жёсткого нуля: sev = (dev/Sat)²/(1 − (dev/H)²), H = HardZeroAt (99.2.12,
// вариант B). Guard при dev ≥ HardZeroAt → +Inf ДО вычисления severity
// (знаменатель неположителен за полюсом; линейные профили имеют HardZero = 0 —
// guard не срабатывает).
func OneSidedSeverity(profile OneSidedProfile, value float64) float64 {
	if value <= profile.Threshold {
		return 0
	}
	deviation := value - profile.Threshold
	if profile.SaturateAt <= 0 {
		return 1
	}
	if profile.Shape == ShapeQuadratic && profile.HardZeroAt > 0 && deviation >= profile.HardZeroAt {
		return math.Inf(1)
	}
	severity := deviation / profile.SaturateAt
	if profile.Shape == ShapeQuadratic {
		// Полюсная форма B: sev = (dev/Sat)² / (1 − (dev/H)²), полюс при dev = H.
		if profile.HardZeroAt > 0 {
			return severity * severity / (1 - (deviation/profile.HardZeroAt)*(deviation/profile.HardZeroAt))
		}
		return severity * severity
	}
	if severity > 1 {
		return 1
	}
	return severity
}

// Scale — общий масштаб перевода тяжести в скорость изменения населения,
// один на все факторы среды (18a, «Масштаб»): у каждого фактора своя форма
// тяжести, но перевод тяжести в реальные часы — общий.
type Scale struct {
	MaxRatePerHour float64 // скорость изменения населения (доля в час) при тяжести = 1
}

// DefaultScale — масштаб скорости: 0.1 принят калибровкой температуры
// (99.2.12, H1); гравитация и радиоактивность сохраняют форму и константы, их
// калибровка под новый масштаб — отдельный блок (риски спеки).
var DefaultScale = Scale{MaxRatePerHour: 0.1}

// SeverityRate переводит тяжесть (0..1) в λ-компоненту изменения населения
// (доля за час; отрицательная при росте — сейчас тяжесть ≥ 0, рост добавится
// будущими слагаемыми, см. ChangeComponents).
func SeverityRate(severity float64, scale Scale) float64 {
	return severity * scale.MaxRatePerHour
}

// MaxSerializedLambda — потолок λ на JSON-границе (99.2.12, §Сериализация):
// json.Marshal(+Inf) падает, а клиентская экстраполяция exp(−λ·Δt)
// косметическая (население на сервере уже 0). t50% ≈ 25 с: за минуту
// экстраполяция даёт 0.
const MaxSerializedLambda = 100.0

// ClampLambda — λ для JSON-границы: +Inf → MaxSerializedLambda.
func ClampLambda(lambda float64) float64 {
	if math.IsInf(lambda, 1) {
		return MaxSerializedLambda
	}
	return lambda
}

// Uninhabitable — истина, когда планета «необитаема» (99.2.12, R-модель):
// жара — витринный порог «t_смерти(p0) < 1 ч» (поселение с текущим
// населением вымирает за час: R > 1 − exp(−ln(p0)/3600)) — механика гладкая,
// порог только для витрины; холод — по-прежнему λ = +Inf при T ≤ 100 K
// (временно); гравитация — жёсткий ноль профиля. Компоненты изменения
// населения — в change_components.go.
func Uninhabitable(input PlanetInput, p0 float64) bool {
	if hardZero(HumanGravityProfile, input.GravityG) {
		return true
	}
	if math.IsInf(ColdChangeRate(input.TemperatureK, DefaultScale), 1) {
		return true
	}
	if p0 < 1 {
		return false
	}
	r := RecursiveChangeRate(input.TemperatureK)
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
// времени (99.2.12, R-модель): изменение = сумма компонент —
// p = p_чек·(1−r)^Δt_сек·exp(−λ_др·Δt_ч), где r — рекурсивная компонента
// (жара, HeatChangeRate), λ_др — прочие факторы (холод/гравитация/
// радиоактивность, OtherChangeRate). Отрицательная компонента = рост:
// r < 0 → (1−r) > 1. Округление — только при показе игроку (18a, «Точность
// и округление»). Guard: +Inf (холод) → 0; Δt ≤ 0 → p0; порог p < 1 → 0
// (поселение мёртвое).
func Population(p0 float64, r float64, lambdaOther float64, deltaSeconds float64) float64 {
	if p0 <= 0 {
		return 0
	}
	if deltaSeconds <= 0 {
		return p0
	}
	if math.IsInf(lambdaOther, 1) {
		return 0
	}
	p := p0 * math.Pow(1-r, deltaSeconds) * math.Exp(-lambdaOther*deltaSeconds/3600)
	if p < 1 {
		return 0
	}
	return p
}
