// Package settlement считает экономику поселения по запросу (ленивый
// пересчёт), без тика. Здесь — только смерть населения от физической среды
// планеты (docs/gamedesign/18a_population_death.md).
package settlement

import "math"

// SeverityShape — форма кривой тяжести профиля (99.2.12, H2): линейная
// (severity = dev/Sat, кап на 1) или квадратичная (severity = (dev/Sat)²,
// без капа — градиент до жёсткого нуля задают поля HardZero*).
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
	SaturateCold float64      // отклонение вниз от ComfortMin, после которого тяжесть = 1
	SaturateHot  float64      // отклонение вверх от ComfortMax, после которого тяжесть = 1
	HardZeroCold float64      // отклонение вниз до жёсткого нуля; 0 = не задан
	HardZeroHot  float64      // отклонение вверх до жёсткого нуля; 0 = не задан
	Shape        SeverityShape // форма кривой тяжести
}

// HumanTemperatureProfile — профиль человека по температуре, в K (99.2.12):
// квадратичная форма, асимметрия жары/холода (SatHot = 200, SatCold = 350),
// жёсткий ноль жары на T ≥ 700 K (dev 350 от ComfortMax). Холодный жёсткий
// ноль не задан — все холодные планеты генератора (мин 50 K) принципиально
// обитаемы.
var HumanTemperatureProfile = TwoSidedProfile{
	ComfortMin:   200,
	ComfortMax:   350,
	SaturateCold: 350,
	SaturateHot:  200,
	HardZeroHot:  350,
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
// комфортного диапазона, 1 на насыщении своей стороны (99.2.12). Линейные
// профили капятся на 1, квадратичные растут без капа — жёсткий ноль там
// обрабатывает TotalLambda, а не тяжесть. Возвращает только конечную тяжесть.
func TwoSidedSeverity(profile TwoSidedProfile, value float64) float64 {
	var deviation float64
	var saturateAt float64
	switch {
	case value < profile.ComfortMin:
		deviation = profile.ComfortMin - value
		saturateAt = profile.SaturateCold
	case value > profile.ComfortMax:
		deviation = value - profile.ComfortMax
		saturateAt = profile.SaturateHot
	default:
		return 0
	}
	if saturateAt <= 0 {
		return 1
	}
	severity := deviation / saturateAt
	if profile.Shape == ShapeQuadratic {
		return severity * severity
	}
	if severity > 1 {
		return 1
	}
	return severity
}

// hardZero — истина, когда отклонение достигло жёсткого нуля стороны профиля
// (dev ≥ HardZero_стороны; 0 = не задан). Периметр проверки один: на него
// смотрят TotalLambda и Uninhabitable (99.2.12, H4).
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
	SaturateAt float64      // превышение порога, после которого тяжесть = 1
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

// OneSidedSeverity считает тяжесть превышения порога: 0 на пороге и ниже, 1 на
// насыщении и дальше (для линейной формы — с капом на 1; квадратичная растёт
// без капа, 99.2.12).
func OneSidedSeverity(profile OneSidedProfile, value float64) float64 {
	if value <= profile.Threshold {
		return 0
	}
	deviation := value - profile.Threshold
	if profile.SaturateAt <= 0 {
		return 1
	}
	severity := deviation / profile.SaturateAt
	if profile.Shape == ShapeQuadratic {
		return severity * severity
	}
	if severity > 1 {
		return 1
	}
	return severity
}

// Scale — общий масштаб скорости смерти, один на все факторы среды (18a,
// «Масштаб»): у каждого фактора своя форма тяжести, но перевод тяжести в
// реальные часы — общий.
type Scale struct {
	MaxRatePerHour float64 // скорость убыли населения (доля в час) при тяжести = 1
}

// DefaultScale — масштаб скорости: 0.1 принят калибровкой температуры
// (99.2.12, H1); гравитация и радиоактивность сохраняют форму и константы, их
// калибровка под новый масштаб — отдельный блок (риски спеки).
var DefaultScale = Scale{MaxRatePerHour: 0.1}

// Lambda переводит тяжесть (0..1) в скорость убыли населения (долю в час).
func Lambda(severity float64, scale Scale) float64 {
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

// Uninhabitable — истина, когда планета за жёстким нулём хотя бы одного
// фактора: население там невозможно (99.2.12, H4). Для JSON-границ: +Inf в
// ответ не уедет, а админский предпросмотр скажет «поселение невозможно».
func Uninhabitable(input PlanetInput) bool {
	return hardZero(HumanTemperatureProfile, input.TemperatureK) ||
		hardZero(HumanGravityProfile, input.GravityG)
}

// Population считает точное (дробное) население через deltaHours реального
// времени при постоянной суммарной скорости распада lambda. Округление — не
// здесь: только при показе игроку (18a, «Точность и округление»), иначе
// результат зависел бы от того, как часто поселение пересчитывают.
// Guard: +Inf убивает мгновенно (0 при Δt > 0), неположительный Δt не двигает
// чек-точку (иначе exp(−Inf·0) = NaN) — 99.2.12, H4.
func Population(p0 float64, lambda float64, deltaHours float64) float64 {
	if p0 <= 0 {
		return 0
	}
	if deltaHours <= 0 {
		return p0
	}
	if math.IsInf(lambda, 1) {
		return 0
	}
	return p0 * math.Exp(-lambda*deltaHours)
}
