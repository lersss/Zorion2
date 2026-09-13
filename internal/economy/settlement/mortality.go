// Package settlement считает экономику поселения по запросу (ленивый
// пересчёт), без тика. Здесь — только смерть населения от физической среды
// планеты (docs/gamedesign/18a_population_death.md).
package settlement

import "math"

// ToleranceProfile описывает переносимость температуры формой жизни. Это
// данные, а не константы в теле функций: разные профили — для разных форм
// жизни, которых пока в игре нет (docs/gamedesign/18a_population_death.md,
// «Профиль устойчивости»).
type ToleranceProfile struct {
	ComfortMinK float64 // нижняя граница комфорта, K
	ComfortMaxK float64 // верхняя граница комфорта, K
	SaturateAtK float64 // отклонение от края комфорта (K), после которого тяжесть = 1
}

// HumanProfile — черновой профиль человека. Числа не финальны, калибруются
// через internal/generator/planet/twin.go.
var HumanProfile = ToleranceProfile{
	ComfortMinK: 200,
	ComfortMaxK: 350,
	SaturateAtK: 300,
}

// TemperatureSeverity считает тяжесть отклонения температуры от комфорта: 0
// внутри комфортного диапазона, 1 на насыщении и дальше. Двусторонняя: перегрев
// и переохлаждение равноценны при одинаковом отклонении (18a, «Профиль
// устойчивости» — гравитация устроена так же, радиоактивность — односторонняя).
func TemperatureSeverity(profile ToleranceProfile, temperatureK float64) float64 {
	var deviation float64
	switch {
	case temperatureK < profile.ComfortMinK:
		deviation = profile.ComfortMinK - temperatureK
	case temperatureK > profile.ComfortMaxK:
		deviation = temperatureK - profile.ComfortMaxK
	default:
		return 0
	}
	if profile.SaturateAtK <= 0 {
		return 1
	}
	severity := deviation / profile.SaturateAtK
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
