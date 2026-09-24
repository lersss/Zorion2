// internal/economy/settlement/effect_recompute_test.go
// Критерии спеки 2026-09-22-эффекты-снабжения-задержка-голод §12 для
// интеграции с R (этап 2): T8 (DeathCause → hunger), T9 (DeathTime — оценка по
// текущей силе), T10 (ChangeComponents = EnvComponents + Σ R(AsOf); Recompute
// НЕ вызывает ChangeComponents — посегментно), T19 (расовый путь), T30 (скаляр
// recovery только для hunger; заводское 0.25).
package settlement

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func effectInput(rate float64, since, until time.Time) PlanetInput {
	return PlanetInput{
		TemperatureK:      293.15,
		GravityG:          1.0,
		CoreRadioactivity: 5,
		Effects:           []EffectForcePoint{{Rate: rate, Since: since, Until: until}},
		AsOf:              since,
	}
}

// T10: ChangeComponents = EnvComponents + Σ R(AsOf); без эффектов — прежнее.
func TestChangeComponentsIncludesEffectAtAsOf(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	input := effectInput(2e-7, t0, t0.Add(time.Hour))
	require.InDelta(t, EnvComponents(input)+2e-7, ChangeComponents(input), 1e-15)

	plain := PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5}
	require.InDelta(t, EnvComponents(plain), ChangeComponents(plain), 1e-15, "без эффектов поведение не меняется")
}

// T10: Recompute с эффектом = Population(p0, EnvComponents+rate, Δt) — вклад
// идёт посегментно, НЕ через ChangeComponents (без двойного учёта).
func TestRecomputeWithEffectNoDoubleCounting(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	input := effectInput(1e-7, t0, t0.Add(time.Hour))
	p0 := 1_000_000.0
	got := Recompute(input, p0, t0, t0.Add(time.Hour), t0)
	want := Population(p0, EnvComponents(input)+1e-7, 3600)
	require.InDelta(t, want, got, 1e-6)
	// Без эффектов — прежний результат.
	plain := PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5}
	require.InDelta(t, Recompute(plain, p0, t0, t0.Add(time.Hour), t0),
		Population(p0, EnvComponents(plain), 3600), 1e-6)
}

// T9: DeathTime — «оценка по текущей силе» (r включает эффект).
func TestDeathTimeWithEffectForce(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	// Горячая среда: EnvComponents > 0 (убыль), эффект добавляется к ней.
	input := PlanetInput{TemperatureK: 350, GravityG: 1.0, CoreRadioactivity: 5,
		Effects: []EffectForcePoint{{Rate: 2e-7, Since: t0, Until: t0.Add(time.Hour)}}, AsOf: t0}
	require.Positive(t, EnvComponents(input), "в горячей среде среда даёт убыль")
	r := ChangeComponents(input)
	_, ok := DeathTime(1_000_000, r, t0, t0)
	require.True(t, ok)
	// Без эффекта r меньше → дата смерти дальше (или тоже есть).
	rPlain := EnvComponents(input)
	atEff, okEff := DeathTime(1_000_000, r, t0, t0)
	atPlain, okPlain := DeathTime(1_000_000, rPlain, t0, t0)
	require.True(t, okEff && okPlain)
	require.True(t, atEff.Before(atPlain), "текущая сила с эффектом ускоряет смерть (оценка завышает дату)")
}

// T8: эффект доминирует → «hunger»; среда 0 и эффект > 0 → «hunger».
func TestDeathCauseHunger(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	hot := PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5,
		Effects: []EffectForcePoint{{Rate: 1e-2, Since: t0, Until: t0}}, AsOf: t0}
	require.Equal(t, "hunger", DeathCause(hot), "эффект строго больше всех средовых → hunger")

	// Эффект слабее среды — прежний порядок (среда 0 → natural).
	cold := PlanetInput{TemperatureK: 150, GravityG: 1.0, CoreRadioactivity: 5,
		Effects: []EffectForcePoint{{Rate: 1e-12, Since: t0, Until: t0}}, AsOf: t0}
	require.Equal(t, "cold", DeathCause(cold))

	comfort := PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5,
		Effects: []EffectForcePoint{{Rate: 1e-9, Since: t0, Until: t0}}, AsOf: t0}
	require.Equal(t, "hunger", DeathCause(comfort), "среда = 0, эффект > 0 → hunger")

	plain := PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5}
	require.Equal(t, "natural", DeathCause(plain), "без эффекта поведение не меняется")
}

// T19: расовый путь тоже учитывает вклад эффекта (ChangeComponents после
// диспетчеризации).
func TestChangeComponentsRaceIncludesEffect(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	input := PlanetInput{RaceID: "nonexistent_race", TemperatureK: 293.15, GravityG: 1.0,
		CoreRadioactivity: 5, Effects: []EffectForcePoint{{Rate: 3e-8, Since: t0, Until: t0}}, AsOf: t0}
	// Расы нет в store → фолбэк человеческой модели + лог; вклад эффекта всё равно есть.
	require.InDelta(t, EnvComponents(input)+3e-8, ChangeComponents(input), 1e-15)
}

// T19 (спека 2026-09-24-потребление-по-товарам §8.3): DeathCause различает
// ТИП эффекта — «жажда» → thirst, «голод» → hunger; при двух активных эффектах
// доминирует больший вклад, равенство решает первый по порядку следования
// (владелец отдаёт эффекты по возрастанию effect_type_id — детерминированно).
func TestDeathCauseDistinguishesThirstAndHunger(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	comfort := func(effects ...EffectForcePoint) PlanetInput {
		return PlanetInput{TemperatureK: 293.15, GravityG: 1.0, CoreRadioactivity: 5,
			Effects: effects, AsOf: t0}
	}
	pt := func(name string, rate float64) EffectForcePoint {
		return EffectForcePoint{Rate: rate, Since: t0, Until: t0, EffectTypeName: name}
	}

	require.Equal(t, "thirst", DeathCause(comfort(pt("жажда", 1e-2))), "доминирует «жажда» → thirst")
	require.Equal(t, "hunger", DeathCause(comfort(pt("голод", 1e-2))), "доминирует «голод» → hunger")

	// Два активных эффекта: код — по большему вкладу.
	require.Equal(t, "thirst", DeathCause(comfort(pt("голод", 1e-3), pt("жажда", 1e-2))),
		"жажда сильнее → thirst")
	require.Equal(t, "hunger", DeathCause(comfort(pt("голод", 2e-2), pt("жажда", 1e-2))),
		"голод сильнее → hunger")

	// Равенство вкладов — первый по порядку следования (детерминированно).
	require.Equal(t, "thirst", DeathCause(comfort(pt("жажда", 1e-2), pt("голод", 1e-2))))
	require.Equal(t, "hunger", DeathCause(comfort(pt("голод", 1e-2), pt("жажда", 1e-2))))

	// Эффект слабее среды — причина средовая, тип эффекта не влияет.
	hot := comfort(pt("жажда", 1e-12))
	hot.TemperatureK = 400
	require.Equal(t, "heat", DeathCause(hot))
}

// Ревью И1b: со средой сравнивается СУММА вкладов ВСЕХ эффектов, а код причины
// берётся по доминирующему типу (argmax). Два эффекта, каждый ≤ среды, но сумма
// > среды → причина = доминирующий тип (не средовая); равные вклады решает
// порядок следования — детерминированно.
func TestDeathCauseEffectSumExceedsEnv(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	pt := func(name string, rate float64) EffectForcePoint {
		return EffectForcePoint{Rate: rate, Since: t0, Until: t0, EffectTypeName: name}
	}
	// Жара 400 K задаёт средовой вклад heat; эффекты — доли heat: каждый ≤ heat,
	// но вдвоём пересиливают среду.
	base := PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5, AsOf: t0}
	_, heat, _, _, _ := envComponentRates(base)
	require.Positive(t, heat, "жара даёт средовой вклад")

	// 0.5·heat + 0.6·heat = 1.1·heat > heat, каждый ≤ heat → доминирует «жажда».
	sumOver := base
	sumOver.Effects = []EffectForcePoint{pt("голод", heat*0.5), pt("жажда", heat*0.6)}
	require.Equal(t, "thirst", DeathCause(sumOver),
		"сумма эффектов > среды → причина по доминирующему типу, не средовая")

	// 0.3·heat + 0.3·heat = 0.6·heat ≤ heat → причина средовая (heat).
	sumUnder := base
	sumUnder.Effects = []EffectForcePoint{pt("голод", heat*0.3), pt("жажда", heat*0.3)}
	require.Equal(t, "heat", DeathCause(sumUnder), "сумма эффектов ≤ среды → причина средовая")

	// Равные вклады при сумме > среды — выигрывает первый по порядку следования.
	tieThirst := base
	tieThirst.Effects = []EffectForcePoint{pt("жажда", heat*0.6), pt("голод", heat*0.6)}
	require.Equal(t, "thirst", DeathCause(tieThirst), "равенство при сумме > среды — первый (жажда)")

	tieHunger := base
	tieHunger.Effects = []EffectForcePoint{pt("голод", heat*0.6), pt("жажда", heat*0.6)}
	require.Equal(t, "hunger", DeathCause(tieHunger), "равенство при сумме > среды — первый (голод)")
}

// T30: скаляр recovery — только для hunger; заводское 0.25.
func TestComponentScalarHunger(t *testing.T) {
	defer ResetComponentScalar(HungerCurveKey)
	v, ok := ComponentScalar(HungerCurveKey)
	require.True(t, ok)
	require.InDelta(t, 0.25, v, 1e-12, "заводское значение recovery = 0.25")

	require.NoError(t, SetComponentScalar(HungerCurveKey, 0.4))
	v, _ = ComponentScalar(HungerCurveKey)
	require.InDelta(t, 0.4, v, 1e-12)

	require.NoError(t, ResetComponentScalar(HungerCurveKey))
	v, _ = ComponentScalar(HungerCurveKey)
	require.InDelta(t, 0.25, v, 1e-12)

	_, ok = ComponentScalar("heat")
	require.False(t, ok, "не-эффект-компонента скаляра не несёт")
	require.Error(t, SetComponentScalar("heat", 1))
	require.Error(t, SetComponentScalar(HungerCurveKey, -1))
}

// Порог кривой эффекта — нулевой префикс; у среды (cold) префикса нет → 0.
func TestCurveThresholds(t *testing.T) {
	thr, ok := BalancerCurveThreshold(HungerCurveKey)
	require.True(t, ok)
	require.InDelta(t, 24.0, thr, 1e-12)
	coldThr, ok := BalancerCurveThreshold("cold")
	require.True(t, ok)
	require.Zero(t, coldThr, "у кривых среды нулевого префикса нет")
	_, ok = BalancerCurveThreshold("nope")
	require.False(t, ok)
	require.False(t, math.IsNaN(thr))
}
