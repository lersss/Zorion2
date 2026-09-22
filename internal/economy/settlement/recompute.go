package settlement

import (
	"sort"
	"time"
)

// NDead — порог (абсолютное число людей), ниже которого население считается
// вымершим и обнуляется целиком, а не тает дальше дробно. Стыковка с уже
// принятым разовым обвалом населения (docs/gamedesign/18a_population_death.md,
// «Механизм»; 18_needs.md §18.3.1). Черновое значение, не финальное.
var NDead = 100.0

// MinPersistInterval — прошедшее время между чек-точками, после которого
// «простой визит» игрока становится «событием»: только тогда ленивый
// пересчёт продвигает сохранённую чек-точку (population_exact/computed_at) в
// БД. Частые просмотры считают население только в памяти — пересчёт это
// чистая функция от чек-точки, запись на хот-пате чтения не нужна (модель
// «правда на сервере, синк по событию», docs/gamedesign/18a_population_death.md).
// Противоположный предел — запись при каждом просмотре; его цена в том, что
// сохранённое население устаревает для читателей без пересчёта (admin-stats).
var MinPersistInterval = 30 * time.Minute

// Recompute считает новое точное население поселения на момент now по
// физике планеты и точному населению на момент since (99.2.12/99.2.13,
// R-модель): изменение = полная рекурсивная компонента ChangeComponents
// (сумма всех R, включая рождаемость BirthComponent, 99.2.16). Возрастного
// потолка нет (99.2.16, решение создателя 2026-09-15): возраст поселения
// ни на что не влияет, параметр createdAt не читается (оставлен для
// совместимости вызовов). Инвариант: два последовательных пересчёта дают
// тот же результат, что один (рекурсия — функция от времени, а не числа
// пересчётов). Ниже NDead — население обнуляется, а не продолжает таять
// дробно (механизм 18_needs; порог температуры — p < 1 внутри Population).
// Эффекты (опционально, спека 2026-09-22-эффекты-снабжения-задержка-голод
// §5.1/§5.3): Recompute идёт КУСОЧНО по границам EffectForcePoint —
// R_i = EnvComponents(input) + Σ p.Rate(сегмент b_i); он НЕ вызывает
// ChangeComponents (иначе вклад эффекта попал бы дважды). Вклад покрывает
// ровно [computed_at, now): сегменты ниже since отсекаются. Инвариант
// аддитивности сохранён — два последовательных пересчёта дают то же, что один.
func Recompute(input PlanetInput, populationExact float64, since time.Time, now time.Time, createdAt time.Time) float64 {
	if now.Sub(since).Seconds() <= 0 {
		return populationExact
	}

	env := EnvComponents(input)
	bounds := effectBoundaries(input.Effects, since, now)
	next := populationExact
	for i := 0; i+1 < len(bounds); i++ {
		s, e := bounds[i], bounds[i+1]
		if !e.After(s) {
			continue
		}
		r := env + effectsRateAt(input.Effects, s, e)
		next = Population(next, r, e.Sub(s).Seconds())
		if next < NDead {
			return 0
		}
	}
	return next
}

// effectBoundaries — границы сегментов [since, now] с добавлением точек
// разрыва силы эффектов (Since/Until строго внутри интервала), отсортированные
// и без дублей. Без эффектов — ровно [since, now].
func effectBoundaries(effects []EffectForcePoint, since, now time.Time) []time.Time {
	bounds := []time.Time{since, now}
	for _, p := range effects {
		if p.Since.After(since) && p.Since.Before(now) {
			bounds = append(bounds, p.Since)
		}
		if p.Until.After(since) && p.Until.Before(now) {
			bounds = append(bounds, p.Until)
		}
	}
	sort.Slice(bounds, func(i, j int) bool { return bounds[i].Before(bounds[j]) })
	out := bounds[:0:0]
	for _, b := range bounds {
		if len(out) == 0 || out[len(out)-1].Before(b) {
			out = append(out, b)
		}
	}
	return out
}

// effectsRateAt — суммарная сила эффектов на сегменте [s, e): берётся сила
// точки, покрывающей середину сегмента (кусочно-постоянна, §5.1).
func effectsRateAt(effects []EffectForcePoint, s, e time.Time) float64 {
	if len(effects) == 0 {
		return 0
	}
	mid := s.Add(e.Sub(s) / 2)
	var sum float64
	for _, p := range effects {
		sum += p.RateAt(mid)
	}
	return sum
}
