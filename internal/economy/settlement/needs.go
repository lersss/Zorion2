// internal/economy/settlement/needs.go
//
// Слой потребности (спека 2026-09-22-эффекты-снабжения-задержка-голод §4.2,
// решение создателя 2026-09-23 «производство и потребности — разные слои»):
// спрос по ПОЗИЦИИ корзины, покрытие как СУММА источников (сегодня — выход
// веток), дефицит → сила условия `w`, траектория нагрузки `load` и сила
// эффекта R(load(t)) на сегментах. Производство (ветки) — лишь один из
// источников покрытия; связь мягкая.
//
// Позиция корзины = существующая категория `categories` (любой kind):
// товар-выход ветки принадлежит позиции через categories.name_norm.
// Единицы: `demand`/`p_b` — батч/СЕК; нормы `params.eat` — батч/(чел·ч) →
// /3600; `t*` — СЕКУНДЫ; `load` — сило-часы; `recovery` — сило-часы/час.
// Функция чистая (никакого доступа к БД/store): кривая и порог передаются
// резолверами, поэтому `load(now)` — чистая функция от базиса.
package settlement

import (
	"log"
	"math"
	"sort"
	"time"
)

// needSecondsPerHour — перевод сек → ч (М1): и накопление нагрузки, и
// восстановление приводятся к часам одним и тем же множителем (§4.3).
const needSecondsPerHour = 3600.0

// NeedsBinding — привязка позиции корзины к эффекту (params.effects +
// params.eat типа поселения, §4.2). NormPerHour — норма позиции
// (params.eat[position]); записи нет → фолбэк делает вызывающий (DefaultEatK).
type NeedsBinding struct {
	Position     string
	EffectTypeID int64
	Impact       string
	Curve        string
	NormPerHour  float64
}

// NeedsSource — ветка как источник покрытия позиции (§4.2): Position —
// categories.name_norm товара-выхода; Batches — произведено за Δt (batches_b);
// DeltaSec — Δt_b в секундах; OutputBase — выходной буфер на processed_at
// (O0_b, ДО производства, finding 6); Since — момент появления ветки
// (processed_at_b, §4.3 крайний случай С1): ветка, созданная ВНУТРИ интервала
// [loadAt, now], входит в покрытие только с этого момента (у такой ветки
// O0_b = 0). Для веток, существовавших на loadAt, Since ≤ loadAt —
// вклад как весь интервал.
type NeedsSource struct {
	ID         string
	Position   string
	Batches    float64
	DeltaSec   float64
	OutputBase float64
	Since      time.Time
}

// NeedsInput — вход слоя потребности. StoredLoad/StoredLoadAt — хранимый базис
// нагрузки по типу эффекта (§4.3); Thresholds — порог (нулевой префикс кривой)
// по ссылке curve; CurveLookup — резолв R(load); Recoveries — скаляр компоненты
// «Балансировки» (сило-ч/ч), КЛЮЧ — ссылка кривой эффекта (b.Curve), не
// жёстко hunger: второй эффект со своей кривой не получает чужую скорость
// восстановления. Нет записи для кривой → 0 («без восстановления», безопасно).
type NeedsInput struct {
	Population   float64
	Bindings     []NeedsBinding
	Sources      []NeedsSource
	StoredLoad   map[int64]float64
	StoredLoadAt map[int64]time.Time
	Thresholds   map[string]float64
	Recoveries   map[string]float64
	ComputedAt   time.Time
	Now          time.Time
	CurveLookup  CurveLookup
}

// EffectRun — результат расчёта одного эффекта за проход: новое значение
// нагрузки, порог, текущая сила условия `w` и траектория силы на [ComputedAt, Now).
type EffectRun struct {
	EffectTypeID int64
	Positions    []string
	Impact       string
	Curve        string
	Load         float64
	LoadAt       time.Time
	Threshold    float64
	W            float64
	Force        []EffectForcePoint
}

// NeedsResult — выход слоя потребности: эффекты и физическое списание
// выходных буферов по источникам (batches, единственная точка записи §4.2).
type NeedsResult struct {
	Effects       []EffectRun
	DrawnBySource map[string]float64
}

// wSeg — кусочно-постоянный сегмент силы условия `w` (§4.2).
type wSeg struct {
	start time.Time
	end   time.Time
	w     float64
}

// ComputeNeeds — расчёт слоя потребности: по привязанным позициям — спрос,
// покрытие, дефицит → `w`; по типу эффекта — накопление/восстановление нагрузки
// и кусочная сила R(load(t)). Детерминирован; повторный вызов с тем же базисом
// даёт тот же результат (независимость от частоты чтения, §4.3).
func ComputeNeeds(in NeedsInput) NeedsResult {
	res := NeedsResult{DrawnBySource: map[string]float64{}}

	srcByPos := map[string][]NeedsSource{}
	for _, s := range in.Sources {
		srcByPos[s.Position] = append(srcByPos[s.Position], s)
	}

	groups := map[int64]*needsGroup{}
	var order []int64
	for _, b := range in.Bindings {
		g := groups[b.EffectTypeID]
		if g == nil {
			g = &needsGroup{curve: b.Curve, impact: b.Impact, recovery: in.Recoveries[b.Curve]}
			groups[b.EffectTypeID] = g
			order = append(order, b.EffectTypeID)
		}
		g.positions = append(g.positions, b.Position)

		sources := srcByPos[b.Position]
		if len(sources) == 0 {
			log.Printf("⚠️ effect: позиция %q без источника покрытия — coverage=0, w=1 (position_no_source)", b.Position)
		}
		loadAt := in.StoredLoadAt[b.EffectTypeID]
		segs, drawn := positionTrajectory(b, in.Population, loadAt, in.Now, sources)
		g.segs = append(g.segs, segs...)
		for id, v := range drawn {
			res.DrawnBySource[id] += v
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	for _, id := range order {
		g := groups[id]
		res.Effects = append(res.Effects, g.compute(in, id))
	}
	return res
}

// needsGroup — позиции, привязанные к одному типу эффекта (одна строка
// active_effects на (владелец, тип), §4.4). recovery — скаляр этой кривой
// (сило-ч/ч), не общий для всех эффектов.
type needsGroup struct {
	curve     string
	impact    string
	recovery  float64
	positions []string
	segs      []wSeg
}

// compute — нагрузка и сила эффекта группы: объединить траектории позиций
// (w = max — эффект ведёт «худшая» покрытая позиция), накопить/восстановить
// нагрузку по сегментам, собрать силу R(load(t)) на [ComputedAt, Now).
func (g *needsGroup) compute(in NeedsInput, typeID int64) EffectRun {
	loadAt := in.StoredLoadAt[typeID]
	load := in.StoredLoad[typeID]
	merged := mergeTrajectories(g.segs, loadAt, in.Now)

	cur := load
	if cur < 0 {
		cur = 0
	}
	force := []EffectForcePoint{}
	runW := 0.0
	for _, s := range merged {
		lenH := s.end.Sub(s.start).Hours()
		endLoad := cur
		if s.w > 0 {
			endLoad = cur + s.w*lenH
		} else {
			endLoad = cur - g.recovery*lenH
			if endLoad < 0 {
				endLoad = 0
			}
		}

		// Сила — на пересечении сегмента с [ComputedAt, Now): сегменты ниже
		// computed_at уже учтены прошлым пересчётом (§5.1).
		fs, fe := s.start, s.end
		if fs.Before(in.ComputedAt) {
			fs = in.ComputedAt
		}
		if fs.Before(fe) {
			force = append(force, EffectForcePoint{
				Rate:  effectRateOnSegment(g.impact, g.curve, g.recovery, in, cur, s, fs, fe),
				Since: fs,
				Until: fe,
			})
		}
		runW = s.w
		cur = endLoad
	}
	if cur < 0 {
		cur = 0
	}

	positions := append([]string(nil), g.positions...)
	sort.Strings(positions)
	return EffectRun{
		EffectTypeID: typeID,
		Positions:    positions,
		Impact:       g.impact,
		Curve:        g.curve,
		Load:         cur,
		LoadAt:       in.Now,
		Threshold:    in.Thresholds[g.curve],
		W:            runW,
		Force:        force,
	}
}

// effectRateOnSegment — R(load(середина сегмента)): нагрузка в середине
// пересечения считается от нагрузки `startLoad` на начале исходного сегмента.
func effectRateOnSegment(impact, curve string, recovery float64, in NeedsInput, startLoad float64, s wSeg, fs, fe time.Time) float64 {
	mid := fs.Add(fe.Sub(fs) / 2)
	midLoad := startLoad
	if s.w > 0 {
		midLoad = startLoad + s.w*mid.Sub(s.start).Hours()
	} else {
		midLoad = startLoad - recovery*mid.Sub(s.start).Hours()
		if midLoad < 0 {
			midLoad = 0
		}
	}
	return EffectRate(impact, curve, midLoad, in.CurveLookup)
}

// positionTrajectory — траектория `w(t)` для одной позиции и физическое
// списание выходных буферов источников (§4.2). Возвращает кусочную `w` на
// [loadAt, now] и объём, списанный с каждого источника (∝ O0_b).
//
// Интервал разбивается точками появления веток, созданных ВНУТРИ него
// (Since_b ∈ (loadAt, now), §4.3 крайний случай С1): до своего Since ветка в
// покрытии не участвует (вклад 0), поэтому `P_sum` — кусочно-постоянная по
// под-интервалам. На каждом под-интервале базис O0_sum истощается нетто-расходом
// `demand − P_sum`; исчерпание внутри под-интервала даёт разрыв `t*`. Ветки,
// существовавшие на loadAt (Since ≤ loadAt), активны весь интервал — их
// поведение не меняется (один под-интервал [loadAt, now]). Итог — чистая функция
// от (loadAt, now, набор веток с их Since, буферы).
func positionTrajectory(b NeedsBinding, population float64, loadAt, now time.Time, sources []NeedsSource) ([]wSeg, map[string]float64) {
	drawn := map[string]float64{}
	delta := now.Sub(loadAt).Seconds()
	if delta <= 0 {
		return nil, drawn
	}

	demand := b.NormPerHour * population / needSecondsPerHour // батч/сек

	// Точки разбиения интервала [loadAt, now]: loadAt, now и моменты создания
	// веток внутри интервала (Since_b строго между ними) — §4.3.
	points := []time.Time{loadAt, now}
	for _, s := range sources {
		if s.Since.After(loadAt) && s.Since.Before(now) {
			points = append(points, s.Since)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
	uniq := points[:0:0]
	for _, p := range points {
		if len(uniq) == 0 || uniq[len(uniq)-1].Before(p) {
			uniq = append(uniq, p)
		}
	}

	var o0Sum float64
	for _, s := range sources {
		o0Sum += math.Max(0, s.OutputBase)
	}

	// Спрос нулевой (нет привязки/явный 0 нормы) — дефицита нет весь интервал
	// (w = 0), буфер не расходуется.
	if demand <= 0 {
		return []wSeg{{start: loadAt, end: now, w: 0}}, drawn
	}

	// Посегментный проход: на под-интервале [a, e] активны ветки с Since ≤ a;
	// базис buf истощается нетто-расходом. Буфера нет / исчерпан → w = w1
	// (текущая выработка minus спрос), пока не появится новая ветка.
	buf := o0Sum
	segs := make([]wSeg, 0, len(uniq))
	for i := 0; i+1 < len(uniq); i++ {
		a, e := uniq[i], uniq[i+1]
		if !e.After(a) {
			continue
		}
		var pSum float64
		for _, s := range sources {
			if s.Since.After(a) {
				continue // ветка ещё не создана — вклада в покрытие нет
			}
			p := 0.0
			if s.DeltaSec > 0 {
				p = s.Batches / s.DeltaSec // батч/сек
			}
			if p < 0 {
				p = 0
			}
			pSum += p
		}
		if demand <= pSum {
			segs = append(segs, wSeg{start: a, end: e, w: 0})
			continue
		}
		net := demand - pSum // батч/сек — нетто-расход базиса
		w1 := net / demand
		if w1 > 1 {
			w1 = 1
		}
		lenSec := e.Sub(a).Seconds()
		if buf >= net*lenSec {
			// Базиса хватает на весь под-интервал — дефицита нет.
			buf -= net * lenSec
			segs = append(segs, wSeg{start: a, end: e, w: 0})
			continue
		}
		if buf > 0 {
			// t* — время исчерпания базиса внутри под-интервала (СЕКУНДЫ).
			tStar := buf / net
			breakAt := a.Add(time.Duration(tStar * float64(time.Second)))
			if breakAt.After(a) && breakAt.Before(e) {
				segs = append(segs, wSeg{start: a, end: breakAt, w: 0})
				segs = append(segs, wSeg{start: breakAt, end: e, w: w1})
			} else {
				segs = append(segs, wSeg{start: a, end: e, w: w1})
			}
			buf = 0
			continue
		}
		segs = append(segs, wSeg{start: a, end: e, w: w1})
	}

	if len(segs) == 0 {
		return nil, drawn
	}

	// Списано с буферов за интервал: min(o0Sum, нетто-расход) — остаток базиса
	// вычитается из суммы; распределение ∝ O0_b (детерминированно). buf — остаток
	// базиса на end; кламп в [0, o0Sum] (страховка от float-эпсилона).
	if buf < 0 {
		buf = 0
	}
	if buf > o0Sum {
		buf = o0Sum
	}
	drawnTotal := o0Sum - buf
	if o0Sum > 0 && drawnTotal > 0 {
		for _, s := range sources {
			base := math.Max(0, s.OutputBase)
			if base > 0 {
				drawn[s.ID] = drawnTotal * base / o0Sum
			}
		}
	}
	return segs, drawn
}

// mergeTrajectories — объединение кусочных `w` нескольких позиций одного
// эффекта по общим точкам разрыва: на каждом под-сегменте w = max (эффект
// ведёт худшая непокрытая позиция). Интервал — [loadAt, now].
func mergeTrajectories(segs []wSeg, loadAt, now time.Time) []wSeg {
	if len(segs) == 0 {
		return []wSeg{{start: loadAt, end: now, w: 0}}
	}
	points := []time.Time{loadAt, now}
	for _, s := range segs {
		if s.start.After(loadAt) && s.start.Before(now) {
			points = append(points, s.start)
		}
		if s.end.After(loadAt) && s.end.Before(now) {
			points = append(points, s.end)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
	uniq := points[:0:0]
	for _, p := range points {
		if len(uniq) == 0 || uniq[len(uniq)-1].Before(p) {
			uniq = append(uniq, p)
		}
	}
	out := make([]wSeg, 0, len(uniq))
	for i := 0; i+1 < len(uniq); i++ {
		a, e := uniq[i], uniq[i+1]
		if !e.After(a) {
			continue
		}
		w := 0.0
		for _, s := range segs {
			if !a.Before(s.start) && a.Before(s.end) && s.w > w {
				w = s.w
			}
		}
		out = append(out, wSeg{start: a, end: e, w: w})
	}
	return out
}
