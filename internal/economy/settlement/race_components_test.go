// Тесты расовой R-модели (спека 99.2.23 §14): ChangeComponents с RaceID
// использует active-кривые расы; не-регрессия людей; нетто в оптимуме;
// смоук «дом расы»; растяжение горячей ветки; слабый холодный хвост.
package settlement

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadRaceStoreForTests — авто-инициализация расового store в temp-файле
// (все расы каталога получают factory/active из карточек).
func loadRaceStoreForTests(t *testing.T) {
	t.Helper()
	resetRaceBalancerStore()
	t.Cleanup(resetRaceBalancerStore)
	require.NoError(t, LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")))
}

// Тест 10 (§14): нетто в оптимуме — аммиачник при 215 K, g=1, rad=10 →
// r = 0.05·(1−2)·R_ест = −3.2·10⁻¹¹ (рост); термо-рои при 600 K →
// −200·R_ест = −1.27·10⁻⁷ (рост ×54.9/год); k=1 → нетто 0 для любой расы.
func TestRaceChangeComponentsNetto(t *testing.T) {
	loadRaceStoreForTests(t)
	t.Cleanup(ResetPopulationSettings)

	// Аммиачник в центре opt (215 K): кривые = 0 → нетто = 0.05·(1−2)·R_ест.
	input := PlanetInput{TemperatureK: 215, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"}
	got := ChangeComponents(input)
	want := 0.05 * (1 - 2) * NaturalChangeRate()
	require.InDelta(t, want, got, 1e-20, "аммиачник в оптимуме: нетто = 0.05·(1−2)·R_ест")
	require.InDelta(t, -3.2e-11, got, 0.5e-11, "≈ −3.2·10⁻¹¹ (рост +0.1%/год)")

	// Термо-рои при 600 K (центр opt): нетто = −200·R_ест = −1.27·10⁻⁷.
	input = PlanetInput{TemperatureK: 600, GravityG: 1, CoreRadioactivity: 10, RaceID: "thermo_swarms"}
	got = ChangeComponents(input)
	want = 200 * (1 - 2) * NaturalChangeRate()
	require.InDelta(t, want, got, 1e-20, "термо-рои в оптимуме: нетто = −200·R_ест")
	require.InDelta(t, -1.27e-7, got, 0.05e-7, "≈ −1.27·10⁻⁷")

	// Экспоненциальный рост за год: (1−r)^год ≈ ×54.9 (поправки В1+В2:
	// множитель ×54.9/год, не «+400%/год»; точное значение по R_ест ≈ ×54.45).
	year := 365.0 * 24 * 3600
	growth := math.Pow(1-got, year) - 1
	require.InDelta(t, 54.9, 1+growth, 0.5, "термо-рои: ×54.9/год (множитель)")

	// k=1 → нетто 0 для любой расы (равновесие).
	require.NoError(t, SetBirthRateCoefficient(1))
	got = ChangeComponents(PlanetInput{TemperatureK: 215, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"})
	require.InDelta(t, 0.0, got, 1e-20, "k=1 → нетто 0 (равновесие)")
	got = ChangeComponents(PlanetInput{TemperatureK: 600, GravityG: 1, CoreRadioactivity: 10, RaceID: "thermo_swarms"})
	require.InDelta(t, 0.0, got, 1e-20, "k=1 → нетто 0 для термо-роёв")
}

// Тест 1 (§14): не-регрессия людей — RaceID = "" и "humans" дают ровно
// текущую сумму (глобальный store + рампа [15, 30] °C + нетто (1−k)·R_ест).
func TestRaceChangeComponentsHumansNonRegression(t *testing.T) {
	loadRaceStoreForTests(t)

	base := PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}
	human := ChangeComponents(base)
	require.Equal(t, human, ChangeComponents(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5, RaceID: ""}))
	require.Equal(t, human, ChangeComponents(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5, RaceID: "humans"}))

	// Холодная планета: расовый путь не должен отличаться от человеческого
	// для RaceID = "humans".
	cold := PlanetInput{TemperatureK: 173, GravityG: 1.0, CoreRadioactivity: 5}
	require.Equal(t, ChangeComponents(cold), ChangeComponents(PlanetInput{TemperatureK: 173, GravityG: 1.0, CoreRadioactivity: 5, RaceID: "humans"}))
}

// Тест 9 (§14): раса без записи в store → человеческая модель + лог (без
// паники); после ручной правки узла (PUT) r_per_sec меняется (active).
func TestRaceChangeComponentsActiveEdit(t *testing.T) {
	loadRaceStoreForTests(t)

	// Раса без записи (нет в каталоге — гвард §3.2): человеческая модель.
	got := ChangeComponents(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5, RaceID: "no_such_race"})
	require.Equal(t, ChangeComponents(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5}), got,
		"раса без записи → человеческая модель (гвард)")

	// Ручная правка узла жары (PUT) → r_per_sec меняется (active-кривая).
	input := PlanetInput{TemperatureK: 215, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"}
	before := ChangeComponents(input)

	rec, _ := GetRaceRecordMeta("ammonia")
	heat := rec.Active.Curves["heat"]
	edited := *heat
	edited.Nodes = append([]SegmentNode(nil), heat.Nodes...)
	// Поднимаем первый узел жары: теперь жара > 0 даже в оптимуме.
	edited.Nodes[0].Y = 0.01
	require.NoError(t, SetRaceCurve("ammonia", "heat", edited))

	after := ChangeComponents(input)
	require.Greater(t, after, before, "правка active-кривой меняет r_per_sec")
}

// Тест 11 (§14): смоук «дом расы» — аммиачник на 215 K не вымирает (рост);
// кремниевые стражи на 900 K — статика (нетто ~0), не вымирают.
func TestRaceSmokeHome(t *testing.T) {
	loadRaceStoreForTests(t)

	// Аммиачник в своём доме: рост, не вымирание (раньше — t50 ≈ 7.4 ч).
	r := ChangeComponents(PlanetInput{TemperatureK: 215, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"})
	require.Less(t, r, 0.0, "аммиачник в оптимуме растёт (r < 0)")
	require.True(t, math.IsInf(DeathMomentSeconds(1e9, r), 1), "не вымирает (t50 = ∞)")

	// Кремниевые стражи (35): opt [750, 1050] K, reproduction 5e-6 → статика.
	r = ChangeComponents(PlanetInput{TemperatureK: 900, GravityG: 1, CoreRadioactivity: 10, RaceID: "silicon_guardians"})
	require.InDelta(t, 5e-6*(1-2)*NaturalChangeRate(), r, 1e-20, "кремниевые стражи: нетто ~0 (статика)")
	require.Less(t, r, 0.0, "не вымирают в оптимуме")
}

// Тест 12 (§14): растяжение горячей ветки — край surv жары → R ≈ 3.9·10⁻⁵
// × ResilienceScale (t50 часы); холод — слабый хвост (t50 дни).
func TestRaceHeatStretchAndColdTail(t *testing.T) {
	loadRaceStoreForTests(t)

	// Аммиачник на краю surv жары (255 K): R_жара ≈ 3.9·10⁻⁵·0.769 ≈ 3.0·10⁻⁵.
	// Полный r в точке: нетто + жара (холод/гравитация/радиация = 0).
	r := ChangeComponents(PlanetInput{TemperatureK: 255, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"})
	heatOnly := r - 0.05*(1-2)*NaturalChangeRate()
	require.InDelta(t, 3.9068e-5*50.0/65.0, heatOnly, 0.5e-5, "аммиачник 255 K: R_жара ≈ 3.0·10⁻⁵ (t50 ≈ 6 ч)")
	t50 := math.Log(2) / math.Abs(math.Log(1-heatOnly))
	require.InDelta(t, 6.0, t50/3600, 3.0, "t50 ≈ 6 ч (критерий §6 «часы»)")

	// Метан-грибницы (11): opt [95, 110], surv [90, 120], resilience 70.
	// Край surv жары (120 K): R_жара ≈ 3.9·10⁻⁵·0.714 ≈ 2.8·10⁻⁵ (t50 ≈ 7 ч).
	r = ChangeComponents(PlanetInput{TemperatureK: 120, GravityG: 1, CoreRadioactivity: 10, RaceID: "methane_fungi"})
	heatOnly = r - 0.001*(1-2)*NaturalChangeRate()
	require.InDelta(t, 3.9068e-5*50.0/70.0, heatOnly, 0.5e-5, "метан-грибницы 120 K: R_жара ≈ 2.8·10⁻⁵")

	// Термо-рои на краю surv жары (780 K): R_жара ≈ 3.9·10⁻⁵·1.429 ≈ 5.6·10⁻⁵.
	r = ChangeComponents(PlanetInput{TemperatureK: 780, GravityG: 1, CoreRadioactivity: 10, RaceID: "thermo_swarms"})
	heatOnly = r - 200*(1-2)*NaturalChangeRate()
	require.InDelta(t, 3.9068e-5*50.0/35.0, heatOnly, 0.5e-5, "термо-рои 780 K: R_жара ≈ 5.6·10⁻⁵ (t50 ≈ 3.5 ч)")

	// Холод (поправка В2): метан-грибницы на краю surv холода (90 K) →
	// R_холод ≈ 4.9·10⁻⁶ (t50 ≈ 1.7 дня) — слабый хвост, «квази-порога» нет.
	r = ChangeComponents(PlanetInput{TemperatureK: 90, GravityG: 1, CoreRadioactivity: 10, RaceID: "methane_fungi"})
	coldOnly := r - 0.001*(1-2)*NaturalChangeRate()
	require.InDelta(t, 4.9e-6, coldOnly, 1.5e-6, "метан-грибницы 90 K: R_холод ≈ 4.9·10⁻⁶ (t50 ≈ 1.7 дня)")
	t50 = math.Log(2) / math.Abs(math.Log(1-coldOnly))
	require.InDelta(t, 1.7, t50/86400, 1.0, "t50 ≈ 1.7 дня (критерий §6 «часы-дни»)")
}

// DeathCause для расы: argmax по вкладам среды из active-кривых; natural при
// нулевых вкладах (полный комфорт в оптимуме) — тест 15 (§14).
func TestRaceDeathCause(t *testing.T) {
	loadRaceStoreForTests(t)

	// Аммиачник на краю surv жары: причина heat.
	cause := DeathCause(PlanetInput{TemperatureK: 255, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"})
	require.Equal(t, "heat", cause)

	// Аммиачник в оптимуме (215 K): все вклады среды = 0 → natural.
	cause = DeathCause(PlanetInput{TemperatureK: 215, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"})
	require.Equal(t, "natural", cause)

	// Метан-грибницы на краю surv холода: причина cold.
	cause = DeathCause(PlanetInput{TemperatureK: 90, GravityG: 1, CoreRadioactivity: 10, RaceID: "methane_fungi"})
	require.Equal(t, "cold", cause)
}