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
// (99.2.12/99.2.13, R-модель): p(t) = pop·(1−r)^t = NDead →
// t = ln(pop/NDead)/|ln(1−r)| сек; r — полная рекурсивная компонента
// (ChangeComponents, за секунду). При r ≥ 1 (мгновенная гибель, гвард)
// t = 0 → computed_at. Время — реальное (игровое = реальному, 1:1).
// Возрастной потолок MaxLifespanSeconds (120 лет от created_at, 99.2.12) —
// компонента-ограничение: никакая смерть не позже ceiling =
// created_at + MaxLifespanSeconds, поэтому t = min(формула, ceiling)
// (сравнение до конвертации в time.Duration — формула natural даёт ~460 лет
// и переполнила бы int64 нс, уводя дату в 1613 г). При r ≤ 0 убыли от среды
// нет, смерть возможна только по потолку: возраст на момент now (синка)
// ≥ 120 лет → t = ceiling; возраст < 120 → ok=false (не вымирают).
// ok=false также когда чек-точка уже мёртвая (population_exact <= NDead,
// бэкфилл отменён).
func DeathTime(populationExact float64, r float64, computedAt time.Time, now time.Time, createdAt time.Time) (time.Time, bool) {
	if populationExact <= NDead {
		return time.Time{}, false
	}
	ceiling := createdAt.Add(time.Duration(MaxLifespanSeconds) * time.Second)
	if r >= 1 {
		return computedAt, true
	}
	if r <= 0 {
		if now.Sub(createdAt).Seconds() >= MaxLifespanSeconds {
			return ceiling, true
		}
		return time.Time{}, false
	}
	tSec := math.Log(populationExact/NDead) / math.Abs(math.Log(1-r))
	if tSec >= ceiling.Sub(computedAt).Seconds() {
		return ceiling, true
	}
	return computedAt.Add(time.Duration(tSec * float64(time.Second))), true
}

// DeathCause — код причины гибели (18a, §«Причина»; 99.2.13, R-модель):
// доминирующая рекурсивная компонента по вкладу — жары (HeatTemperatureChangeRate),
// холода (ColdChangeRate), гравитации верхней/нижней ветки
// (GravityChangeRate), радиации (RadiationChangeRate). При равенстве вкладов —
// порядок температура → гравитация → радиация (решение создателя 2026-09-14).
// Если все вклады среды ≤ 0 (полный комфорт, поселение вымирает от
// естественной убыли NaturalComponent) — причина "natural".
// Коды: heat/cold/gravity_high/gravity_low/radiation/natural.
func DeathCause(input PlanetInput) string {
	cold := ColdChangeRate(input.TemperatureK)
	heat := HeatTemperatureChangeRate(input.TemperatureK)

	var gHigh, gLow float64
	if input.GravityG > 1.2 {
		gHigh = gravityHighC * math.Pow(input.GravityG-1.2, 1.54)
	}
	if input.GravityG < 0.8 {
		gLow = gravityLowC * math.Pow(0.8-input.GravityG, 3.3)
	}
	rad := RadiationChangeRate(input.CoreRadioactivity)

	if heat <= 0 && cold <= 0 && gHigh <= 0 && gLow <= 0 && rad <= 0 {
		return "natural"
	}

	type contrib struct {
		code string
		val  float64
	}
	best := contrib{code: "cold", val: cold}
	for _, c := range []contrib{
		{code: "heat", val: heat},
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