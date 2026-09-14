// Лог поселения — запись «Вымерло» (docs/gamedesign/18a_population_death.md,
// §«Лог поселения»). Здесь — только чистая математика: дата смерти из живой
// чек-точки и доминирующая причина. Запись в БД — транзакцией синка в
// репозитории (в той же tx, что продвигает чек-точку). Новых констант баланса
// нет — всё вывод из уже принятой модели (NDead, ChangeComponents, чек-точка).
package settlement

import (
	"math"
	"time"
)

// DeathTime считает момент пересечения порога NDead из живой чек-точки
// (99.2.12, R-модель): p(t) = pop·(1−R)^t·exp(−λ_др·t) = NDead →
// t = ln(pop/NDead)/|ln(1−R) + λ_др/3600| сек; R — рекурсивная компонента
// жары за секунду, λ_др — прочие факторы (холод/гравитация/радиоактивность),
// /ч. При λ_др = +Inf (холодный жёсткий ноль, временно) t = 0 → computed_at.
// Время — реальное (игровое = реальному, 1:1). ok=false — запись не
// создаётся (дата невычислима): чек-точка уже мёртвая (population_exact
// <= NDead, бэкфилл отменён) или изменения нет вовсе (R = 0 и λ_др = 0).
func DeathTime(populationExact float64, r float64, lambda float64, computedAt time.Time) (time.Time, bool) {
	if populationExact <= NDead {
		return time.Time{}, false
	}
	if r <= 0 && lambda <= 0 {
		return time.Time{}, false
	}
	if math.IsInf(lambda, 1) {
		return computedAt, true
	}
	rate := math.Abs(math.Log(1-r)) + lambda/3600
	tSec := math.Log(populationExact/NDead) / rate
	return computedAt.Add(time.Duration(tSec * float64(time.Second))), true
}

// DeathCause — код причины гибели (18a, §«Причина»; 99.2.12, R-модель):
// жара (R > 0) — «heat» (её компонента изменения — рекурсия, не λ); холодный
// жёсткий ноль (λ = +Inf) — «cold»; гравитация в жёстком нуле — high/low.
// Иначе — доминирующий фактор по вкладу в λ (холодная зона/гравитация/
// радиоактивность). При равенстве вкладов — порядок холод → гравитация →
// радиоактивность. Коды: heat/cold/gravity_high/gravity_low/radiation.
func DeathCause(input PlanetInput, scale Scale) string {
	if math.IsInf(ColdChangeRate(input.TemperatureK, scale), 1) {
		return "cold"
	}
	if hardZero(HumanGravityProfile, input.GravityG) {
		if input.GravityG > HumanGravityProfile.ComfortMax {
			return "gravity_high"
		}
		return "gravity_low"
	}
	if RecursiveChangeRate(input.TemperatureK) > 0 {
		return "heat"
	}

	tempSev := TwoSidedSeverity(HumanTemperatureProfile, input.TemperatureK)
	gravitySev := TwoSidedSeverity(HumanGravityProfile, input.GravityG)
	radioSev := OneSidedSeverity(HumanRadioactivityProfile, input.CoreRadioactivity)

	type contrib struct {
		code string
		val  float64
	}
	var cold, gHigh, gLow float64
	if input.TemperatureK < HumanTemperatureProfile.ComfortMin {
		cold = SeverityRate(tempSev, scale)
	}
	switch {
	case input.GravityG > HumanGravityProfile.ComfortMax:
		gHigh = SeverityRate(gravitySev, scale)
	case input.GravityG < HumanGravityProfile.ComfortMin:
		gLow = SeverityRate(gravitySev, scale)
	}
	rad := SeverityRate(radioSev, scale)

	best := contrib{code: "cold", val: cold}
	for _, c := range []contrib{
		{code: "gravity_high", val: gHigh},
		{code: "gravity_low", val: gLow},
		{code: "radiation", val: rad},
	} {
		if c.val > best.val {
			best = c
		}
	}
	return best.code
}
