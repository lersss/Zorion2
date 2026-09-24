// Лог поселения — запись «Вымерло» (docs/gamedesign/18b_settlement_log.md,
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
//
// r передаётся как «текущая сила» (ChangeComponents при AsOf, включая вклад
// эффектов): при РАСТУЩЕЙ нагрузке DeathTime ЗАВЫШАЕТ дату — это оценка «по
// текущей силе» (спека 2026-09-22-эффекты-снабжения §5.2); точное решение с
// кривой не в закрытой форме, численный DeathTime — задел. Поведение функции
// не меняется.
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

// DeathCause — код причины гибели (18b, §«Причина»; 99.2.13, R-модель):
// доминирующая рекурсивная компонента по вкладу — жары (HeatTemperatureChangeRate),
// холода (ColdChangeRate), гравитации верхней/нижней ветки
// (GravityChangeRate), радиации (RadiationChangeRate). При равенстве вкладов —
// порядок температура → гравитация → радиация (решение создателя 2026-09-14).
// Если все вклады среды ≤ 0 (полный комфорт, поселение вымирает от
// естественной убыли NaturalComponent) — причина "natural".
// Коды: heat/cold/gravity_high/gravity_low/radiation/natural.
// Голод (спека 2026-09-22-эффекты-снабжения-задержка-голод §5.2): вклад
// эффектов населения — отдельное слагаемое; сравнивается с максимумом средовых
// вкладов по СУММЕ всех эффектов (как раньше effectsRateAt суммировал вклады:
// «голод» + «жажда» вместе могут пересилить среду, даже если каждый по
// отдельности её не превышает), а код причины берётся по ДОМИНИРУЮЩЕМУ типу
// (argmax) — так различаются «голод» и «жажда» (спека 2026-09-24-потребление-
// по-товарам §8.3: «голод» → "hunger", «жажда» → "thirst").
// Расовый путь (99.2.23 §4.2): вклады среды — из active-кривых расы
// (RaceID ≠ NULL/"humans"); механизм argmax не меняется.
func DeathCause(input PlanetInput) string {
	cold, heat, gHigh, gLow, rad := envComponentRates(input)

	maxEnv := 0.0
	for _, v := range []float64{cold, heat, gHigh, gLow, rad} {
		if v > maxEnv {
			maxEnv = v
		}
	}

	// Вклад эффектов (спека 2026-09-22-эффекты-снабжения-задержка-голод §5.2):
	// death.go считает среду собственной envComponentRates (мимо
	// ChangeComponents), поэтому вклад эффектов добавляется здесь явно — иначе
	// эффект не попадал бы в причину гибели. Со средой сравнивается СУММА всех
	// эффектов (среда = 0 и вклад > 0 → тоже эффект); код причины — по
	// ДОМИНИРУЮЩЕМУ типу (argmax, спека 2026-09-24 §8.3), а не всегда "hunger".
	if name, _, total, ok := dominantEffect(input.Effects, input.AsOf); ok && total > maxEnv {
		return effectCauseCode(name)
	}

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

// effectCauseCode — код причины гибели по name_norm типа эффекта (спека
// 2026-09-24-потребление-по-товарам §8.3): «жажда» → thirst, «голод» → hunger.
// Прочий/пустой тип — hunger (поведение до И1b: любой вклад эффекта давал
// "hunger"; читатели без owner-прохода идентичность не заполняют).
func effectCauseCode(effectTypeName string) string {
	if effectTypeName == "жажда" {
		return "thirst"
	}
	return "hunger"
}

// dominantEffect — разбор эффектов на момент asOf: name/best — доминирующий
// тип по СУММЕ силы сегментов одного типа (argmax; идентичность —
// EffectTypeName, §8.3), total — суммарный вклад ВСЕХ типов (то, что раньше
// давал effectsRateAt и с чем сравнивается среда). При равенстве вкладов
// выигрывает первый по порядку следования точек — детерминированно: владелец
// (collectForce) отдаёт эффекты по возрастанию effect_type_id (ComputeNeeds
// сортирует группы). Нулевой/пустой вклад → ok=false.
func dominantEffect(effects []EffectForcePoint, asOf time.Time) (name string, best, total float64, ok bool) {
	var names []string
	sum := map[string]float64{}
	for _, p := range effects {
		rate := p.RateAt(asOf)
		if rate == 0 {
			continue
		}
		if _, seen := sum[p.EffectTypeName]; !seen {
			names = append(names, p.EffectTypeName)
		}
		sum[p.EffectTypeName] += rate
	}
	for _, n := range names {
		v := sum[n]
		total += v
		if v > best {
			best = v
			name = n
		}
	}
	if best <= 0 {
		return "", 0, 0, false
	}
	return name, best, total, true
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
