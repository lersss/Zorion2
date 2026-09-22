// internal/economy/settlement/needs_test.go
// Критерии спеки 2026-09-22-эффекты-снабжения-задержка-голод §12 для слоя
// потребности (этап 2): T2 (спрос/покрытие/дефицит → w), T3 (t* в секундах,
// сумму по веткам, расход ∝ O0_b), T4/T28 (нагрузка: w·len/3600 и
// recovery·len/3600, клампы), T5 (независимость от частоты чтения),
// T6 (порог), T-AB1..3/5 (анти-абьюз), T13 (две ветки одной позиции → один
// эффект). Фейковая кривая — без глобального store.
package settlement

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testLookup — резолвер кривой «hunger» с заданной функцией R(load).
func testLookup(rate func(load float64) float64) CurveLookup {
	return func(key string) (EffectCurveEval, bool) {
		if key != "hunger" {
			return nil, false
		}
		return rate, true
	}
}

// noRate — кривая-ноль (сила не влияет на проверяемую нагрузку).
func noRate() CurveLookup { return testLookup(func(float64) float64 { return 0 }) }

const testNorm = 2.5e-8 // батч/(чел·ч) — норма позиции (DefaultEatK)

// hungerBinding — привязка позиции «продовольствие» к типу 1.
func hungerBinding(norm float64) NeedsBinding {
	return NeedsBinding{Position: "продовольствие", EffectTypeID: 1, Impact: ImpactPopulationRate, Curve: "hunger", NormPerHour: norm}
}

// input собирает NeedsInput с одним типом, базисом load_at = t0 и now = t0+Δ.
func needsInput(t0 time.Time, delta time.Duration, population float64, load float64, recovery float64, srcs ...NeedsSource) NeedsInput {
	return NeedsInput{
		Population:   population,
		Bindings:     []NeedsBinding{hungerBinding(testNorm)},
		Sources:      srcs,
		StoredLoad:   map[int64]float64{1: load},
		StoredLoadAt: map[int64]time.Time{1: t0},
		Thresholds:   map[string]float64{"hunger": 24},
		Recoveries:   map[string]float64{"hunger": recovery},
		ComputedAt:   t0,
		Now:          t0.Add(delta),
		CurveLookup:  noRate(),
	}
}

// T28/T4: полное отсутствие покрытия (w=1) за час даёт ровно +1 сило-час.
func TestNeedsUncoveredOneHourAddsOne(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	in := needsInput(t0, time.Hour, 1000, 0, 0.25, NeedsSource{ID: "b1", Position: "продовольствие", Batches: 0, DeltaSec: 3600, OutputBase: 0})
	res := ComputeNeeds(in)
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 1.0, res.Effects[0].Load, 1e-9, "w=1 за час → +1 сило-час (М1)")
	require.InDelta(t, 1.0, res.Effects[0].W, 1e-12)
	require.Zero(t, res.DrawnBySource["b1"], "буфера нет — списывать нечего")
	require.InDelta(t, 24.0, res.Effects[0].Threshold, 1e-12)
}

// T2: покрытие не ниже спроса → w = 0, нагрузка не растёт.
func TestNeedsCoveredNoLoad(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	demandPerSec := testNorm * 1000 / 3600 // батч/сек
	// Производство ровно по спросу за час.
	batch := demandPerSec * 3600
	in := needsInput(t0, time.Hour, 1000, 0, 0.25, NeedsSource{ID: "b1", Position: "продовольствие", Batches: batch, DeltaSec: 3600, OutputBase: 0})
	res := ComputeNeeds(in)
	require.InDelta(t, 0.0, res.Effects[0].Load, 1e-12, "покрыто → w=0, нагрузки нет")
	require.InDelta(t, 0.0, res.Effects[0].W, 1e-12)
}

// T3: базис исчерпывается за t* (СЕКУНДЫ); списание — ровно O0 за интервал,
// расход ∝ O0_b при нескольких ветках.
func TestNeedsBufferBreakpointSecondsAndProportionalDraw(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	demandPerSec := testNorm * 1000 / 3600
	// Две ветки одной позиции: O0 = 3:1 (1.2e-5 : 4e-6), производство 0.
	// t* = O0_sum/demand = 1.6e-5/6.944e-9 ≈ 2304 с < 3600.
	o1, o2 := demandPerSec*1728, demandPerSec*576 // суммарно 2304 с базиса
	in := needsInput(t0, time.Hour, 1000, 0, 0,
		NeedsSource{ID: "b1", Position: "продовольствие", OutputBase: o1},
		NeedsSource{ID: "b2", Position: "продовольствие", OutputBase: o2},
	)
	res := ComputeNeeds(in)
	// Первые 2304 с w=0, оставшиеся 1296 с w=1 → нагрузка 1296/3600 = 0.36.
	require.InDelta(t, 1296.0/3600.0, res.Effects[0].Load, 1e-9)
	// Списано с буферов ровно O0_sum, пропорционально O0_b.
	require.InDelta(t, o1, res.DrawnBySource["b1"], 1e-15)
	require.InDelta(t, o2, res.DrawnBySource["b2"], 1e-15)
}

// T4/T-AB5: восстановление w=0 идёт со скоростью recovery (сило-ч/ч) и
// клампится ≥ 0; recovery отсутствует (0) → нагрузки нет.
func TestNeedsRecoveryAndClamp(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	covered := []NeedsSource{{ID: "b1", Position: "продовольствие", Batches: 1e9, DeltaSec: 3600}}
	in := needsInput(t0, time.Hour, 1000, 10, 0.25, covered...)
	res := ComputeNeeds(in)
	require.InDelta(t, 9.75, res.Effects[0].Load, 1e-9, "w=0 за час снимает recovery=0.25 сило-часа")

	in2 := needsInput(t0, 100*time.Hour, 1000, 1, 0.25, covered...)
	res2 := ComputeNeeds(in2)
	require.Zero(t, res2.Effects[0].Load, "кламп: восстановление не уводит ниже 0")

	in3 := needsInput(t0, time.Hour, 1000, 5, 0, NeedsSource{ID: "b1", Position: "продовольствие"})
	res3 := ComputeNeeds(in3)
	require.InDelta(t, 6.0, res3.Effects[0].Load, 1e-9, "recovery=0 → без восстановления (источник не найден → w=1)")
}

// T6: порог — нулевой префикс кривой «голод»; сила в точке порога 0.
func TestHungerCurveThresholdAndForce(t *testing.T) {
	nodes := defaultHungerCurve().Nodes
	require.InDelta(t, 24.0, ZeroPrefixThreshold(nodes), 1e-12, "порог 24 = сутки полного условия (§8.1)")
	eval, ok := BalancerCurveLookup(HungerCurveKey)
	require.True(t, ok)
	require.Zero(t, eval(24), "в точке порога R = 0 (М2)")
	require.Greater(t, eval(456), eval(240), "кривая растёт по нагрузке до плато")
	require.InDelta(t, 1e-7, eval(5000), 1e-15, "плато за последним узлом")
}

// T10/T5: сила эффекта — кусочная траектория R(load(t)) на [ComputedAt, Now);
// load(now) — чистая функция базиса (повторный расчёт даёт то же).
func TestNeedsForceTrajectoryAndPurity(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	lookup := testLookup(func(load float64) float64 { return load * 1e-6 })
	in := needsInput(t0, time.Hour, 1000, 0, 0.25, NeedsSource{ID: "b1", Position: "продовольствие"})
	in.CurveLookup = lookup
	res1 := ComputeNeeds(in)
	res2 := ComputeNeeds(in)
	require.Equal(t, res1.Effects[0].Load, res2.Effects[0].Load, "повторное чтение с тем же базисом — тот же load(now)")
	require.Len(t, res1.Effects[0].Force, 1)
	fp := res1.Effects[0].Force[0]
	require.Equal(t, t0, fp.Since)
	require.Equal(t, in.Now, fp.Until)
	require.InDelta(t, 0.5*1e-6, fp.Rate, 1e-15, "сила по нагрузке в середине сегмента")

	// ForcePoint.RateAt — полуоткрытый интервал, без двойного учёта на стыке.
	a := EffectForcePoint{Rate: 1, Since: t0, Until: t0.Add(time.Hour)}
	b := EffectForcePoint{Rate: 2, Since: t0.Add(time.Hour), Until: t0.Add(2 * time.Hour)}
	require.InDelta(t, 1.0, a.RateAt(t0.Add(30*time.Minute)), 1e-12)
	require.InDelta(t, 2.0, b.RateAt(t0.Add(time.Hour)), 1e-12)
	require.Zero(t, a.RateAt(t0.Add(time.Hour)), "стык принадлежит ровно одному сегменту")
}

// T-AB1/2/3: крошки (w≈1) → нагрузка растёт; краткое покрытие не обнуляет;
// длинное покрытие уводит ровно в 0.
func TestNeedsAntiAbuse(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	// Крошка: за час произведена 1/1000 спроса → w ≈ 0.999.
	demandPerSec := testNorm * 1000 / 3600
	crumb := []NeedsSource{{ID: "b1", Position: "продовольствие", Batches: demandPerSec * 3600 / 1000, DeltaSec: 3600}}
	res := ComputeNeeds(needsInput(t0, time.Hour, 1000, 0, 0.25, crumb...))
	require.Greater(t, res.Effects[0].Load, 0.99, "крошка → нагрузка растёт (не сбрасывается)")
	require.Greater(t, res.Effects[0].W, 0.99)

	// Краткое покрытие: один проход w=0 за 6 минут → падение лишь 0.25*0.1.
	covered := []NeedsSource{{ID: "b1", Position: "продовольствие", Batches: 1e9, DeltaSec: 21600}}
	res2 := ComputeNeeds(needsInput(t0, 6*time.Minute, 1000, 5, 0.25, covered...))
	require.InDelta(t, 5-0.25*0.1, res2.Effects[0].Load, 1e-9, "краткое покрытие не обнуляет (T-AB2)")

	// Длительное покрытие: w=0 ≥ load/recovery → ровно 0.
	res3 := ComputeNeeds(needsInput(t0, 40*time.Hour, 1000, 5, 0.25, covered...))
	require.Zero(t, res3.Effects[0].Load, "длительное покрытие уводит ровно в 0 (T-AB3)")
}

// T13: две ветки одной позиции дают ОДИН эффект (одна строка), не два.
func TestNeedsTwoBranchesOneEffect(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	in := needsInput(t0, time.Hour, 1000, 0, 0,
		NeedsSource{ID: "b1", Position: "продовольствие"},
		NeedsSource{ID: "b2", Position: "продовольствие"},
	)
	res := ComputeNeeds(in)
	require.Len(t, res.Effects, 1, "позиция → один эффект, две ветки — один источник-эффект")
}

// С1 (§4.3): ветка, созданная ВНУТРИ интервала (Since > load_at), входит в
// покрытие только с момента Since — до него её не было, w = 1. Ветка,
// существовавшая на load_at (Since ≤ load_at), покрывает весь интервал.
func TestNeedsSourceSinceInsideInterval(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	demandPerSec := testNorm * 1000 / 3600 // батч/сек — спрос при 1000 человек
	since := t0.Add(30 * time.Minute)

	// Ветка создана на середине часа; далее производит ровно по спросу.
	// recovery = 0 — изолируем вклад покрытия от восстановления нагрузки.
	newSrc := NeedsSource{ID: "b_new", Position: "продовольствие", Batches: demandPerSec * 1800, DeltaSec: 1800, Since: since}
	res := ComputeNeeds(needsInput(t0, time.Hour, 1000, 0, 0, newSrc))
	require.InDelta(t, 0.5, res.Effects[0].Load, 1e-9, "до Since ветки не было → w=1 половину часа")
	require.InDelta(t, 0.0, res.Effects[0].W, 1e-12, "после Since покрыто → w=0")
	require.Zero(t, res.DrawnBySource["b_new"], "у новой ветки O0_b = 0")

	// Ветка, существовавшая весь интервал, покрывает и первые 30 минут.
	oldSrc := NeedsSource{ID: "b_old", Position: "продовольствие", Batches: demandPerSec * 3600, DeltaSec: 3600, Since: t0}
	resOld := ComputeNeeds(needsInput(t0, time.Hour, 1000, 0, 0, oldSrc))
	require.Zero(t, resOld.Effects[0].Load, "ветка весь интервал → покрыто всегда")
	require.Greater(t, res.Effects[0].Load, resOld.Effects[0].Load,
		"ветка с Since внутри даёт больше нагрузки, чем существовавшая весь интервал")
}

// Чистота (§4.3): итог — функция от базиса, разбиение интервала на под-чтения
// не меняет накопленную нагрузку. Прямое чтение [t0, now] и два чтения с
// базисом на стыке Since дают одно и то же.
func TestNeedsSplitReadsInvariant(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	demandPerSec := testNorm * 1000 / 3600
	since := t0.Add(30 * time.Minute)
	src := NeedsSource{ID: "b1", Position: "продовольствие", Batches: demandPerSec * 1800, DeltaSec: 1800, Since: since}

	direct := ComputeNeeds(needsInput(t0, time.Hour, 1000, 0, 0, src))

	// Под-чтение 1: [t0, since] — ветки ещё нет (recovery = 0, изолируем Since).
	first := ComputeNeeds(needsInput(t0, 30*time.Minute, 1000, 0, 0, src))
	// Под-чтение 2: базис load_at = since (память базис не двигает, Since ветки
	// сохраняется) → нагрузка продолжается от first, ветка покрывает интервал.
	second := ComputeNeeds(needsInput(since, 30*time.Minute, 1000, first.Effects[0].Load, 0, src))

	require.InDelta(t, direct.Effects[0].Load, first.Effects[0].Load, 1e-9, "первое чтение = нагрузка до Since")
	require.InDelta(t, direct.Effects[0].Load, second.Effects[0].Load, 1e-9,
		"разбиение интервала на под-чтения не меняет итог (§4.3)")
}

// Спрос нулевой (норма 0 = «не ест») → w = 0, нагрузки нет.
func TestNeedsZeroNormNoDeficit(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	in := needsInput(t0, time.Hour, 1000, 0, 0.25, NeedsSource{ID: "b1", Position: "продовольствие"})
	in.Bindings = []NeedsBinding{hungerBinding(0)}
	res := ComputeNeeds(in)
	require.Zero(t, res.Effects[0].Load)
	require.InDelta(t, 0.0, res.Effects[0].W, 1e-12)
	require.False(t, math.IsNaN(res.Effects[0].W))
}
