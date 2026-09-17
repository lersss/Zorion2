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
// Расовый путь (99.2.23 §4.2): вклады среды — из active-кривых расы
// (RaceID ≠ NULL/"humans"); механизм argmax не меняется.
func DeathCause(input PlanetInput) string {
	cold, heat, gHigh, gLow, rad := envComponentRates(input)

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

// envComponentRates — вклады среды (жара/холод/гравитация/радиация) по
// RaceID: расовые active-кривые (99.2.23 §4.2) или глобальный store 99.2.17.
// Ветки гравитации не пересекаются по домену: ниже комфорта — нижняя ветка,
// выше — верхняя, в комфорте — ноль (для рас комфорт = интервал нулевых Y
// узлов кривой; для людей — 0.8–1.2 g, как раньше).
func envComponentRates(input PlanetInput) (cold, heat, gHigh, gLow, rad float64) {
	if input.RaceID != "" && input.RaceID != "humans" {
		if rc, ok := GetRaceActiveCurves(input.RaceID); ok {
			heat = evaluateCurve(rc.Curves["heat"].Nodes, rc.Curves["heat"].Bends, input.TemperatureK-273.15)
			cold = evaluateCurve(rc.Curves["cold"].Nodes, rc.Curves["cold"].Bends, input.TemperatureK-273.15)
			gHigh, gLow = raceGravityBranch(rc.Curves["gravity"], input.GravityG)
			rad = evaluateCurve(rc.Curves["radiation"].Nodes, rc.Curves["radiation"].Bends, input.CoreRadioactivity)
			return
		}
	}
	heat = HeatTemperatureChangeRate(input.TemperatureK)
	cold = ColdChangeRate(input.TemperatureK)
	switch {
	case input.GravityG > 1.2:
		gHigh = GravityChangeRate(input.GravityG)
	case input.GravityG < 0.8:
		gLow = GravityChangeRate(input.GravityG)
	}
	rad = RadiationChangeRate(input.CoreRadioactivity)
	return
}

// raceGravityBranch — ветка гравитации расовой кривой: комфорт = интервал
// узлов с (почти) нулевым Y (opt_lo..opt_hi); ниже — low, выше — high,
// внутри — 0. «Нулевой узел» — по порогу |Y| < 1e-9, не по точному == 0:
// ручная правка комфорта в UI (лог-шкала даёт ~1e-14 вместо ровного нуля)
// не должна уводить в fallback на человеческие границы (ревью 99.2.23).
// Нулевых узлов нет (вся кривая > 0) — fallback на человеческие границы
// 0.8/1.2 (недостижимо для валидных карточек: комфорт всегда Y=0).
func raceGravityBranch(curve *ComponentCurve, g float64) (high, low float64) {
	lo, hi := -1.0, -1.0
	for _, n := range curve.Nodes {
		if math.Abs(n.Y) < 1e-9 {
			if lo < 0 {
				lo = n.X
			}
			hi = n.X
		}
	}
	if lo < 0 {
		if g > 1.2 {
			return evaluateCurve(curve.Nodes, curve.Bends, g), 0
		}
		return 0, evaluateCurve(curve.Nodes, curve.Bends, g)
	}
	v := evaluateCurve(curve.Nodes, curve.Bends, g)
	switch {
	case g < lo:
		return 0, v
	case g > hi:
		return v, 0
	default:
		return 0, 0
	}
}
