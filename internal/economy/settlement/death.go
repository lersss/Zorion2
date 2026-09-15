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
// Возрастной потолок 120 лет удалён (99.2.16, решение создателя 2026-09-15):
// возраст поселения ни на что не влияет, дата смерти не срезается сверху.
// При r ≤ 0 (рост r < 0, равновесие r = 0) поселение не вымирает — ok=false.
// Дата — через Unix-секунды (решение создателя 2026-09-15): time.Duration
// ограничен ~292.5 года (max int64 нс), а медленная убыль даёт даты за
// тысячу лет; int64-секунд хватает на ~292 млрд лет — лимит недостижим,
// запись «Вымерло» создаётся у ВСЕХ вымерших. База Unix — computedAt (момент
// пересчёта: tSec отсчитывается от него; спека 99.2.16 §4.2); createdAt
// остаётся в сигнатуре (решение создателя «нужен для Unix-базы»), фактически
// не используется. ok=false также когда чек-точка уже мёртвая
// (population_exact <= NDead, бэкфилл отменён).
func DeathTime(populationExact float64, r float64, computedAt time.Time, createdAt time.Time) (time.Time, bool) {
	if populationExact <= NDead {
		return time.Time{}, false
	}
	if r >= 1 {
		return computedAt, true
	}
	if r <= 0 {
		return time.Time{}, false
	}
	tSec := math.Log(populationExact/NDead) / math.Abs(math.Log(1-r))
	unix := computedAt.Unix() + int64(tSec)
	return time.Unix(unix, 0), true
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

	// Ветки гравитации (99.2.13, R-модель; 99.2.17 — сегментная кривая из
	// balancer store): ветки не пересекаются по домену — ниже 0.8 g весь
	// вклад кривой = нижняя ветка (невесомость), выше 1.2 g — верхняя
	// (перегрузка), в комфорте 0.8–1.2 — ноль. Декомпозиция по домену
	// эквивалентна прежним формулам веток, константы удалены (99.2.17 §8).
	var gHigh, gLow float64
	switch {
	case input.GravityG > 1.2:
		gHigh = GravityChangeRate(input.GravityG)
	case input.GravityG < 0.8:
		gLow = GravityChangeRate(input.GravityG)
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
