// Package settlement считает экономику поселения по запросу (ленивый
// пересчёт), без тика. Здесь — только смерть населения от физической среды
// планеты (docs/gamedesign/18a_population_death.md).
package settlement

import "math"

// TwoSidedProfile описывает переносимость величины, для которой одинаково
// плохо и завышенное, и заниженное значение — температура, гравитация (18a,
// «Профиль устойчивости»). Радиоактивность одностороння, сюда не подходит.
// Это данные, а не константы в теле функций: разные профили — для разных форм
// жизни, которых пока в игре нет.
type TwoSidedProfile struct {
	ComfortMin float64 // нижняя граница комфорта
	ComfortMax float64 // верхняя граница комфорта
	SaturateAt float64 // отклонение от края комфорта, после которого тяжесть = 1
}

// HumanTemperatureProfile — черновой профиль человека по температуре, в K.
// Числа не финальны, калибруются через internal/generator/planet/twin.go.
var HumanTemperatureProfile = TwoSidedProfile{
	ComfortMin: 200,
	ComfortMax: 350,
	SaturateAt: 300,
}

// HumanGravityProfile — черновой профиль человека по гравитации, в g (1 = как
// на Земле). Числа не финальны, калибруются через
// internal/generator/planet/twin.go.
var HumanGravityProfile = TwoSidedProfile{
	ComfortMin: 0.8,
	ComfortMax: 1.2,
	SaturateAt: 4,
}

// TwoSidedSeverity считает тяжесть отклонения величины от комфорта: 0 внутри
// комфортного диапазона, 1 на насыщении и дальше. Двусторонняя: превышение и
// недостаток равноценны при одинаковом отклонении.
func TwoSidedSeverity(profile TwoSidedProfile, value float64) float64 {
	var deviation float64
	switch {
	case value < profile.ComfortMin:
		deviation = profile.ComfortMin - value
	case value > profile.ComfortMax:
		deviation = value - profile.ComfortMax
	default:
		return 0
	}
	if profile.SaturateAt <= 0 {
		return 1
	}
	severity := deviation / profile.SaturateAt
	if severity > 1 {
		return 1
	}
	return severity
}

// OneSidedProfile описывает переносимость величины, для которой плохо только
// превышение порога, а недостаток безопасен — радиоактивность (18a, «Профиль
// устойчивости»). Температура и гравитация двусторонние, см. TwoSidedProfile.
type OneSidedProfile struct {
	Threshold  float64 // ниже и на пороге — безопасно, тяжесть = 0
	SaturateAt float64 // превышение порога, после которого тяжесть = 1
}

// HumanRadioactivityProfile — черновой профиль человека по фону ядра планеты
// (core.radioactivity, шкала 0..100). Числа не финальны, калибруются через
// internal/generator/planet/twin.go.
var HumanRadioactivityProfile = OneSidedProfile{
	Threshold:  20,
	SaturateAt: 60,
}

// OneSidedSeverity считает тяжесть превышения порога: 0 на пороге и ниже, 1 на
// насыщении и дальше.
func OneSidedSeverity(profile OneSidedProfile, value float64) float64 {
	if value <= profile.Threshold {
		return 0
	}
	deviation := value - profile.Threshold
	if profile.SaturateAt <= 0 {
		return 1
	}
	severity := deviation / profile.SaturateAt
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

// DefaultScale — черновое значение, не финальное, калибруется через
// internal/generator/planet/twin.go (18a, открытый вопрос №1).
var DefaultScale = Scale{MaxRatePerHour: 0.01}

// Lambda переводит тяжесть (0..1) в скорость убыли населения (долю в час).
func Lambda(severity float64, scale Scale) float64 {
	return severity * scale.MaxRatePerHour
}

// Population считает точное (дробное) население через deltaHours реального
// времени при постоянной суммарной скорости распада lambda. Округление — не
// здесь: только при показе игроку (18a, «Точность и округление»), иначе
// результат зависел бы от того, как часто поселение пересчитывают.
func Population(p0 float64, lambda float64, deltaHours float64) float64 {
	if p0 <= 0 {
		return 0
	}
	return p0 * math.Exp(-lambda*deltaHours)
}
