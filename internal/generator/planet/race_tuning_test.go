// internal/generator/planet/race_tuning_test.go — подкрутка генератора под
// расу-дома (спека 99.2.22 §14): сдвиг орбиты, обогащение, давление, тепло,
// число планет, двухслойная мягкость, детерминизм, смоук «копилка».
// Синергия «звёзды+планеты» (решение создателя 2026-09-17): звёздный слой
// (home-классы ×1.6) + умеренный планетный (кламп орбиты [0.35, 3.0]).
package planet

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator/galaxy"
	"zorion/internal/races"
)

// ==================== РУЧКА 1: СДВИГ ОРБИТЫ (§14.2) ====================

// Аммиачники: T_target₀ ∈ [20, 237] (холодный режим), mult ∈ [0.35, 3.0]
// (умеренный кламп синергии «звёзды+планеты», решение создателя 2026-09-17).
func TestOrbitShiftAmmonia(t *testing.T) {
	g := NewGenerator(nil, 1)
	g.raceID = "ammonia"
	g.raceSoftness = 1
	sp := stellarParamsFromClass("G", 5772, g.rng)
	tune := g.raceTunePlanet(sp, orbitRadiusScaled(2, sp.Luminosity), true)
	require.NotNil(t, tune)

	tun := races.ByID("ammonia").Tuning()
	pTarget := racePTarget(tun, 1.0)
	tTarget0 := raceTTarget0(tun, pTarget, 1.0)
	assert.GreaterOrEqual(t, tTarget0, 20.0, "T_target₀ ≥ 20 (холодный пол)")
	assert.LessOrEqual(t, tTarget0, 237.0, "T_target₀ ≤ 237 (холодный потолок)")
	assert.GreaterOrEqual(t, tune.orbitMult, 0.35, "mult ≥ 0.35")
	assert.LessOrEqual(t, tune.orbitMult, 3.0, "mult ≤ 3.0")
}

// Лавовые: T_target₀ ∈ [330, 500] (горячий режим), mult ∈ [0.35, 3.0].
func TestOrbitShiftLava(t *testing.T) {
	g := NewGenerator(nil, 2)
	g.raceID = "lava"
	g.raceSoftness = 1
	sp := stellarParamsFromClass("G", 5772, g.rng)
	tune := g.raceTunePlanet(sp, orbitRadiusScaled(1, sp.Luminosity), true)
	require.NotNil(t, tune)

	tun := races.ByID("lava").Tuning()
	pTarget := racePTarget(tun, 1.0)
	tTarget0 := raceTTarget0(tun, pTarget, 1.0)
	assert.GreaterOrEqual(t, tTarget0, 330.0, "T_target₀ ≥ 330 (горячий пол)")
	assert.LessOrEqual(t, tTarget0, 500.0, "T_target₀ ≤ 500 (горячий потолок)")
	assert.GreaterOrEqual(t, tune.orbitMult, 0.35)
	assert.LessOrEqual(t, tune.orbitMult, 3.0)
}

// Метан-планктон: mult клампится к 3.0 (умеренный кламп синергии — планета
// добирает остаток до окна с ближних орбит, а не тащится через всю систему).
func TestOrbitShiftMethaneClamp(t *testing.T) {
	g := NewGenerator(nil, 3)
	g.raceID = "methane_plankton"
	g.raceSoftness = 1
	sp := stellarParamsFromClass("G", 5772, g.rng)
	tune := g.raceTunePlanet(sp, orbitRadiusScaled(2, sp.Luminosity), true)
	require.NotNil(t, tune)

	tun := races.ByID("methane_plankton").Tuning()
	pTarget := racePTarget(tun, 1.0)
	tTarget0 := raceTTarget0(tun, pTarget, 1.0)
	assert.GreaterOrEqual(t, tTarget0, 20.0)
	assert.LessOrEqual(t, tTarget0, 237.0)
	assert.LessOrEqual(t, tune.orbitMult, 3.0, "кламп 3.0")
}

// ==================== РУЧКА 2: ОБОГАЩЕНИЕ И РАЗБАВЛЕНИЕ (§14.3) ====================

// need {NH₃: 1} в холодном → доля NH₃ ≥ 5% после нормировки, сумма = 1.
func TestEnrichmentColdAmmonia(t *testing.T) {
	tun := races.ByID("ammonia").Tuning()
	override := compositionOverride(tun, 0)
	require.NotNil(t, override, "обогащение есть (need NH₃ 1 > 0)")

	base := normalizeComposition(atmosphereComposition("холодный", zoneOuter, 0.7, false, rand.New(rand.NewSource(1))))
	comp := applyCompositionOverride(override, base)
	assert.GreaterOrEqual(t, comp["NH3"], 0.05, "NH₃ ≥ 5% после нормировки")
	assert.InDelta(t, 1.0, sumComposition(comp), 1e-9, "сумма = 1 (аудит atmosphere_sum_not_100)")
}

// need {NH₃: 0} в холодном → обогащения нет (фикс итерации 3: min = 0 =
// «не требуется»).
func TestEnrichmentNeedZero(t *testing.T) {
	tun := races.ByID("cryo_forest").Tuning()
	assert.Nil(t, compositionOverride(tun, 0), "need {NH₃: 0, CH₄: 0} — обогащать нечего")
}

// need {O₂: 10} в умеренном → обогащения нет (режим-гейт: умеренный = {}).
func TestEnrichmentTemperateGate(t *testing.T) {
	tun := races.ByID("humans").Tuning()
	assert.Nil(t, compositionOverride(tun, 0), "умеренный — режим-гейт {} (O₂ даёт жизненный проход)")
}

// Курильщики (P_window_lo = 50): разбавление парника w_CO₂ ≈ 0.05
// (N₂-доминанта), сумма = 1.
func TestDilutionSmokers(t *testing.T) {
	tun := races.ByID("smokers").Tuning()
	d := dilutionCO2(tun, 330.0)
	assert.InDelta(t, 0.05, d, 0.001, "w_CO₂ ≈ 0.05 (N₂-доминанта)")

	override := compositionOverride(tun, d)
	require.NotNil(t, override)
	assert.InDelta(t, 0.05, override["CO2"], 0.001)
	assert.InDelta(t, 0.95, override["N2"], 0.001)
	assert.InDelta(t, 1.0, sumComposition(override), 1e-9, "сумма = 1")
}

// Сверхкритические (P_window_lo = 100): w_CO₂ ≈ 0.19.
func TestDilutionSupercritical(t *testing.T) {
	tun := races.ByID("supercritical").Tuning()
	d := dilutionCO2(tun, 330.0)
	assert.InDelta(t, 0.19, d, 0.01, "w_CO₂ ≈ 0.19")
}

// H₂S в составе: молекулярная масса 64, ярлык «ядовитая» при
// SO₂+NH₃+H₂S > 5% (и N₂ ≤ 70 — иначе «азотная» раньше по порядку проверок).
func TestH2SMolecularWeightAndLabel(t *testing.T) {
	assert.InDelta(t, 64.0, meanMolecularWeight(map[string]float64{"H2S": 1}), 0.001, "μ(H₂S) = 64")
	assert.Equal(t, "ядовитая", classifyAtmosphere(
		map[string]float64{"H2S": 0.05, "SO2": 0.02, "NH3": 0.01, "N2": 0.60, "CO2": 0.32}, 1.0),
		"SO₂+NH₃+H₂S > 5% → «ядовитая»")
}

// ==================== РУЧКА 4: ТЕПЛО (МОЛОДЫЕ ПЛАНЕТЫ) (§14.4) ====================

// Глубинники: F_int_target = Lo×3 = 3, возраст кламп [0.1, 0.499],
// F_int_actual = F_int_target·√M_nom ∈ [2.25·Lo, 5·Lo] при √M_nom ∈ [0.75, 1.66].
func TestHeatAgeDeepDwellers(t *testing.T) {
	g := NewGenerator(nil, 4)
	g.raceID = "deep_dwellers"
	g.raceSoftness = 1
	sp := stellarParamsFromClass("G", 5772, g.rng)
	tune := g.raceTunePlanet(sp, orbitRadiusScaled(2, sp.Luminosity), true)
	require.NotNil(t, tune)
	assert.GreaterOrEqual(t, tune.ageGyr, 0.1, "age ≥ 0.1")
	assert.LessOrEqual(t, tune.ageGyr, 0.499, "age ≤ 0.499 (при 0.5 F_accretion = 0)")

	tun := races.ByID("deep_dwellers").Tuning()
	lo := 1.0 // opt Lo глубинников
	for _, sqrtM := range []float64{0.75, 1.0, 1.66} {
		fActual := tun.FIntTarget * sqrtM
		assert.GreaterOrEqual(t, fActual, 2.25*lo, "F_int ≥ 2.25·Lo (√M = %.2f)", sqrtM)
		assert.LessOrEqual(t, fActual, 5.0*lo, "F_int ≤ 5·Lo (√M = %.2f)", sqrtM)
	}
}

// ==================== РУЧКА 6: ЧИСЛО ПЛАНЕТ (§14.5) ====================

func TestPlanetCountMult(t *testing.T) {
	g := NewGenerator(nil, 5)
	g.raceID = "ammonia"
	g.raceSoftness = 1
	g.racePlanetCountMult = 1.1
	assert.InDelta(t, 1.1, g.planetCountMult(), 0.001, "mean × 1.1 (слой 1, s = 1)")

	g.raceSoftness = 0
	assert.InDelta(t, 1.0, g.planetCountMult(), 0.001, "s = 0 — без множителя")

	g.raceID = ""
	assert.InDelta(t, 1.0, g.planetCountMult(), 0.001, "без расы — без множителя")
}

// ==================== ДВУХСЛОЙНАЯ МЯГКОСТЬ (§14.1) ====================

// s = 0 / s = 1 → ролл не потребляет энтропию (early-exit, спека §4.3):
// RNG-поток идентичен генератору без ролла. 0 < s < 1 — один Float64.
// (Точные значения планет между прогонами не детерминированы — порядок
// итерации map в генерации композиции, 99.2.20; тестируется сам ролл.)
func TestSoftnessZeroNoEntropy(t *testing.T) {
	next := func(s float64, roll bool) float64 {
		g := NewGenerator(nil, 42)
		g.raceID = "ammonia"
		g.raceSoftness = s
		if roll {
			g.raceTuned()
		}
		return g.rng.Float64()
	}
	base := next(0, false) // без ролла
	assert.Equal(t, base, next(0, true), "s = 0: ролл не потребляет энтропию")
	assert.Equal(t, base, next(1, true), "s = 1: ролл не потребляет энтропию")
	assert.NotEqual(t, base, next(0.5, true), "0 < s < 1: ролл потребляет один Float64")
}

// s = 0 → планета не подстроена (raceTunePlanet возвращает nil).
func TestSoftnessZeroNotTuned(t *testing.T) {
	g := NewGenerator(nil, 42)
	g.raceID = "ammonia"
	g.raceSoftness = 0
	sp := stellarParamsFromClass("G", 5772, g.rng)
	assert.Nil(t, g.raceTunePlanet(sp, orbitRadiusScaled(2, sp.Luminosity), true),
		"s = 0: ни один мир не подстроен")
}

// s = 1 → все планеты подстроены (ролл не потребляет энтропию).
func TestSoftnessOneAllTuned(t *testing.T) {
	g := NewGenerator(nil, 42)
	g.raceID = "ammonia"
	g.raceSoftness = 1
	tuned := 0
	for i := 0; i < 200; i++ {
		if g.raceTuned() {
			tuned++
		}
	}
	assert.Equal(t, 200, tuned, "s = 1 — все подстроены")
}

// s = 0.5 → доля подстроенных ≈ 0.5 (ролл потребляет один Float64).
func TestSoftnessHalfFraction(t *testing.T) {
	g := NewGenerator(nil, 42)
	g.raceID = "ammonia"
	g.raceSoftness = 0.5
	tuned := 0
	n := 1000
	for i := 0; i < n; i++ {
		if g.raceTuned() {
			tuned++
		}
	}
	assert.InDelta(t, 0.5, float64(tuned)/float64(n), 0.05, "доля подстроенных ≈ s")
}

// s = 0.5 → доля планет с обогащением NH₃ ≈ 0.5 (интеграция: орбиты 1–2,
// гигантов нет — все каменистые).
func TestSoftnessHalfEnrichmentFraction(t *testing.T) {
	g := NewGenerator(nil, 7)
	g.raceID = "ammonia"
	g.raceSoftness = 0.5
	g.racePlanetCountMult = 1.1
	enriched := 0
	n := 200
	for i := 0; i < n; i++ {
		cls := []string{"G", "K", "M"}[i%3]
		pd := g.generatePlanet("w", "World", 1+i%2, stellarParamsFromClass(cls, 0, g.rng))
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(pd.Data, &data))
		comp := compositionPctFromData(data)
		if comp["NH3"] >= 5 {
			enriched++
		}
	}
	assert.InDelta(t, 0.5, float64(enriched)/float64(n), 0.2, "доля обогащённых ≈ s (бимодальность)")
}

// ==================== ДЕТЕРМИНИЗМ ПО SEED (§14.1) ====================

// Детерминизм по seed: вывод подкрутки — функция карточки + s + seed
// (спека §7.3): два вызова с тем же seed дают одинаковые ручки
// (orbitMult/fVol/ageGyr/surface/composition). Точные значения планет не
// детерминированы между прогонами (порядок итерации map в генерации
// композиции, 99.2.20) — сравнивается вывод подкрутки, не планеты.
func TestRaceTuningDeterminism(t *testing.T) {
	run := func() []raceTune {
		g := NewGenerator(nil, 7)
		g.raceID = "ammonia"
		g.raceSoftness = 0.5
		g.racePlanetCountMult = 1.1
		var out []raceTune
		for i := 0; i < 40; i++ {
			cls := []string{"G", "K", "M"}[i%3]
			sp := stellarParamsFromClass(cls, 0, g.rng)
			tune := g.raceTunePlanet(sp, orbitRadiusScaled(1+i%6, sp.Luminosity), true)
			if tune != nil {
				out = append(out, *tune)
			}
		}
		return out
	}
	a, b := run(), run()
	require.Equal(t, len(a), len(b), "одинаковое число подстроенных планет")
	for i := range a {
		assert.Equal(t, a[i].orbitMult, b[i].orbitMult, "планета %d: orbitMult", i)
		assert.Equal(t, a[i].fVol, b[i].fVol, "планета %d: fVol", i)
		assert.Equal(t, a[i].ageGyr, b[i].ageGyr, "планета %d: ageGyr", i)
		assert.Equal(t, a[i].surface, b[i].surface, "планета %d: surface", i)
		assert.Equal(t, a[i].composition, b[i].composition, "планета %d: composition", i)
	}
}

// ==================== СМОУК: КОПИЛКА (§14.8) ====================

// piggyBankRaces — 20 рас копилки (идея 56a, спека §14.8): холодные газовые
// (5, 6, 8, 11, 12, 14), серные умеренно-горячие (15, 18, 20), курильщики
// (16), очень горячие (24, 25, 26, 27, 28, 29, 32, 35, 37), глубинники (3).
var piggyBankRaces = []string{
	"deep_dwellers", "ammonia", "cryo_forest", "mistfolk",
	"methane_fungi", "methane_plankton", "mist_swarms",
	"sulfur_nests", "smokers", "volcanites", "nether",
	"lava", "supercritical", "thermo_swarms", "microcrack",
	"hot_ash", "geysers", "crystallites", "silicon_guardians", "diamond",
}

// Смоук: кластер расы с s = 0.5 — все 20 рас копилки получают ненулевую
// частоту подходящих планет (Race.Suitable = true) в своём кластере.
// 800 планет на расу — размер кластера из спеки §14.8 («кластер из ~800»).
// Синергия «звёзды+планеты» (2026-09-17): класс звезды берётся из сдвинутого
// распределения (звёздный слой: веса × race_mult, eff = 1 + (config−1)·s),
// планетный слой умеренный (кламп орбиты [0.35, 3.0]).
func TestSmokePiggyBankRacesGetPlanets(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	report := map[string]int{}
	for _, raceID := range piggyBankRaces {
		r := races.ByID(raceID)
		require.NotNil(t, r, raceID)

		g := NewGenerator(nil, int64(len(report))+100)
		g.raceID = raceID
		g.raceSoftness = 0.5
		g.racePlanetCountMult = 1.1

		suitable := 0
		for i := 0; i < 800; i++ {
			cls := spectralClassFromRace(g.rng, r, 0.5)
			pd := g.generatePlanet("w", "World", 1+i%8, stellarParamsFromClass(cls, 0, g.rng))
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal(pd.Data, &data))
			if r.Suitable(data) {
				suitable++
			}
		}
		report[raceID] = suitable
		assert.Greater(t, suitable, 0, "раса %s получает планеты в своём кластере (s = 0.5)", raceID)
	}
	t.Logf("копилка: подходящих планет на расу (800 планет, s = 0.5, звёздный сдвиг + умеренный планетный): %v", report)
}

// spectralClassFromRace — класс звезды из сдвинутого распределения кластера
// расы (звёздный слой 99.2.22 §3.3 ручка 5, §3.4): дефолтные веса × race_mult
// (eff = 1 + (config−1)·s), взвешенный выбор. Повторяет формулу
// applyRaceSpectralMult (galaxy) — тестовая копия, чтобы смоук честно
// отражал синергию «звёзды+планеты» (не 100% home-классов, а сдвиг).
func spectralClassFromRace(rng *rand.Rand, r *races.Race, s float64) string {
	weights := galaxy.DefaultWeights().Spectral
	for cls, m := range r.Tuning().SpectralMult {
		weights[cls] *= 1 + (m-1)*s
	}
	order := []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}
	total := 0.0
	for _, k := range order {
		total += weights[k]
	}
	x := rng.Float64() * total
	for _, k := range order {
		x -= weights[k]
		if x <= 0 {
			return k
		}
	}
	return order[len(order)-1]
}

// ==================== ХЕЛПЕРЫ ====================

func sumComposition(comp Composition) float64 {
	sum := 0.0
	for _, v := range comp {
		sum += v
	}
	return sum
}

// compositionPctFromData — состав атмосферы (%) из planet.data.
func compositionPctFromData(data map[string]interface{}) map[string]float64 {
	atm, ok := data["atmosphere_data"].(map[string]interface{})
	if !ok {
		return nil
	}
	comp, ok := atm["composition"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]float64, len(comp))
	for gas, v := range comp {
		if f, ok := v.(float64); ok {
			out[gas] = f
		}
	}
	return out
}
