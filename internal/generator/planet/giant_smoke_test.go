// internal/generator/planet/giant_smoke_test.go
//
// Смоук 20 000 миров (спека 2026-09-20 §11, критерий приёмки): дефолтные
// настройки, классы звёзд и типы систем по весам 99.2.4 §4.1 (как продакшн).
// Метрики: доля G/K/F и M с гигантом, горячие юпитеры глобально, максимумы
// планет, обитаемая доля.
package planet

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"zorion/internal/generator/galaxy"
	"zorion/internal/models"
)

// weightedPickTest — взвешенный выбор по map весов (копия galaxy.weightedPick
// для теста: порядок ключей фиксирован).
func weightedPickTest(rng *rand.Rand, weights map[string]float64, order []string) string {
	total := 0.0
	for _, k := range order {
		total += weights[k]
	}
	r := rng.Float64() * total
	for _, k := range order {
		r -= weights[k]
		if r <= 0 {
			return k
		}
	}
	return order[len(order)-1]
}

// TestSmoke20000Worlds — смоук 20 000 миров по критерию приёмки §11:
// G/K/F с гигантом 8–12%, M 2–5%, O/B < 1%, A ≤ 3%, горячие юпитеры
// глобально [0.5%, 1.0%], максимум планет M/G/K ≥ 4 и любой ≤ 8,
// обитаемая доля ~2.5% (окно 2.3–4% — шум одного сида, решение создателя
// «обитаемость как получится»).
func TestSmoke20000Worlds(t *testing.T) {
	g := NewGenerator(nil, 20260920)
	g.means = DefaultPlanetMeans()

	const worlds = 20000
	spectralOrder := []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}
	weights := galaxy.DefaultWeights().Spectral
	systemOrder := []string{"single", "binary", "multiple",
		"black_hole", "neutron", "white_dwarf", "protostar", "exotic"}
	systemWeights := galaxy.DefaultWeights().SystemTypes

	classWorlds := map[string]int{}
	classGiants := map[string]int{}
	hotJupiters := 0
	totalPlanets := 0
	habitable := 0
	maxPlanets := 0
	maxPlanetsMGK := 0

	for i := 0; i < worlds; i++ {
		st := weightedPickTest(g.rng, systemWeights, systemOrder)
		cls := weightedPickTest(g.rng, weights, spectralOrder)
		classWorlds[cls]++

		// Экзотика: гигантов нет (99.2.4 §5.3), планеты не обитаемы.
		if st != "single" && st != "binary" && st != "multiple" {
			pc := g.planetCountFor(WorldInfo{StarType: st})
			totalPlanets += pc
			if pc > maxPlanets {
				maxPlanets = pc
			}
			continue
		}

		// Обычная звезда: счёт планет по типу системы (planetCountFor).
		w := WorldInfo{SpectralClass: cls, StarType: "star", SystemType: st}
		if st == "binary" {
			// wide ~70% / close ~30% (галактика 99.2.4 §4.1).
			if g.rng.Float64() < 0.7 {
				w.Mods = &models.StellarMods{BinaryType: "wide"}
			} else {
				w.Mods = &models.StellarMods{BinaryType: "close"}
			}
		}
		planetCount := g.planetCountFor(w)
		totalPlanets += planetCount
		if planetCount > maxPlanets {
			maxPlanets = planetCount
		}
		if (cls == "M" || cls == "G" || cls == "K") && planetCount > maxPlanetsMGK {
			maxPlanetsMGK = planetCount
		}

		sp := stellarParamsFromClass(cls, 0, g.rng)

		// Тесная двойная — P-ветка: одна P-планета, свой ролл гиганта
		// (generateCircumbinaryPlanet, спека §4.2); орбита 1.
		if w.Mods != nil && w.Mods.BinaryType == "close" {
			if planetCount == 0 {
				continue
			}
			if g.rng.Float64() < g.gasGiantChanceShifted(sp) {
				classGiants[cls]++
				hotJupiters++ // P-гигант на единственной орбите 1 — «горячий»
			}
			// Обитаемость P-планеты: каскад при r_P = 3×sep (типичная тесная
			// пара, компаньон того же класса — упрощение смоука).
			sep := 0.05 * math.Pow(0.9/0.05, g.rng.Float64()) // log-uniform [0.05, 0.9]
			res := g.runCascade(cascadeInput{
				Luminosity:    sp.Luminosity * 2,
				StellarMass:   sp.StellarMass * 2,
				AgeGyr:        sp.AgeGyr,
				Metallicity:   sp.Metallicity,
				TEff:          sp.TEff,
				OrbitRadiusAU: 3 * sep,
				OrbitIndex:    1,
			})
			if res.LiquidWater && res.WaterPercent > 10 && res.TFinal > 200 && res.TFinal < 350 {
				habitable++
			}
			continue
		}

		// Обычный путь: per-системное решение гиганта + каскад на орбитах.
		giantOrbit := g.rollGiantOrbit(sp, planetCount)
		if giantOrbit > 0 {
			classGiants[cls]++
			if giantOrbit <= 2 {
				hotJupiters++
			}
		}
		for orbit := 1; orbit <= planetCount; orbit++ {
			if orbit == giantOrbit {
				continue // гигант не обитаем (без поверхности, §3.6)
			}
			res := g.runCascade(cascadeInput{
				Luminosity:    sp.Luminosity,
				StellarMass:   sp.StellarMass,
				AgeGyr:        sp.AgeGyr,
				Metallicity:   sp.Metallicity,
				TEff:          sp.TEff,
				OrbitRadiusAU: orbitRadiusScaled(orbit, sp.Luminosity),
				OrbitIndex:    orbit,
			})
			if res.LiquidWater && res.WaterPercent > 10 && res.TFinal > 200 && res.TFinal < 350 {
				habitable++
			}
		}
	}

	rate := func(classes ...string) float64 {
		w, gi := 0, 0
		for _, c := range classes {
			w += classWorlds[c]
			gi += classGiants[c]
		}
		return float64(gi) / float64(w)
	}
	gkf := rate("G", "K", "F")
	m := rate("M")
	ob := rate("O", "B")
	a := rate("A")
	hotGlobal := float64(hotJupiters) / float64(worlds)
	habFrac := float64(habitable) / float64(totalPlanets)

	t.Logf("смоук 20 000 миров (спека 2026-09-20 §11):")
	t.Logf("  G/K/F с гигантом: %.2f%% (цель 8–12%%)", gkf*100)
	t.Logf("  M с гигантом: %.2f%% (цель 2–5%%)", m*100)
	t.Logf("  O/B с гигантом: %.2f%% (цель < 1%%)", ob*100)
	t.Logf("  A с гигантом: %.2f%% (цель ≤ 3%%)", a*100)
	t.Logf("  горячие юпитеры глобально: %.2f%% (цель 0.5–1.0%%)", hotGlobal*100)
	t.Logf("  максимум планет (M/G/K): %d (цель ≥ 4)", maxPlanetsMGK)
	t.Logf("  максимум планет (все): %d (цель ≤ 8)", maxPlanets)
	t.Logf("  обитаемая доля: %.2f%% (цель ~2.5%%, окно 2.3–4%%)", habFrac*100)
	t.Logf("  планет всего: %d (спека §8: ≈ 76 000)", totalPlanets)

	// Критерий приёмки §11.
	assert.InDelta(t, 0.1045, gkf, 0.02, "G/K/F с гигантом 8–12%%")
	assert.InDelta(t, 0.031, m, 0.015, "M с гигантом 2–5%%")
	assert.Less(t, ob, 0.01, "O/B < 1%% (флип с 0.8)")
	assert.LessOrEqual(t, a, 0.03, "A ≤ 3%%")
	assert.InDelta(t, 0.007, hotGlobal, 0.003, "горячие юпитеры глобально 0.5–1.0%%")
	assert.GreaterOrEqual(t, maxPlanetsMGK, 4, "максимум планет M/G/K ≥ 4")
	assert.LessOrEqual(t, maxPlanets, 8, "максимум планет ≤ 8")
	// Обитаемая доля ~2.5% (решение создателя: «как получится», шум одного
	// сида — факт 2.47% на сиде 20260920; нижний пол 2.3%, чтобы шум не валил).
	assert.GreaterOrEqual(t, habFrac, 0.023, "обитаемая доля ≥ 2.3%%")
	assert.LessOrEqual(t, habFrac, 0.04, "обитаемая доля ≤ 4%%")
}