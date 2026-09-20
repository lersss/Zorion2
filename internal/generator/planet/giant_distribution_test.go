// internal/generator/planet/giant_distribution_test.go
//
// Тесты реализма распределения (спека 2026-09-20 «Реализм распределения:
// газовые гиганты по классам звёзд, горячие юпитеры, число планет у звезды»,
// §10.2): per-системный триггер гиганта, горячие юпитеры (миграция + фолбэк),
// новые mean, металличность, один ролл на систему.
package planet

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// testMetallicity — металличность [Fe/H] как в galaxy.rollMetallicity
// (N(0, 0.3), кламп [−0.8, +0.5]) — для статистических тестов гигантов
// (спека §4.4: E[10^(0.5×[Fe/H])] ≈ 1.045).
func testMetallicity(rng *rand.Rand) float64 {
	met := rng.NormFloat64() * 0.3
	if met < -0.8 {
		met = -0.8
	}
	if met > 0.5 {
		met = 0.5
	}
	return met
}

// simulateSystem — один прогон системы: число планет + орбита гиганта.
// Порядок роллов как в generateWorldIntoBuffer: счёт планет → решение гиганта.
func (g *Generator) simulateSystem(cls string) (planetCount, giantOrbit int) {
	planetCount = g.determinePlanetCount(cls)
	sp := StellarParams{SpectralClass: cls, Metallicity: testMetallicity(g.rng)}
	return planetCount, g.rollGiantOrbit(sp, planetCount)
}

// ==================== ДОЛЯ ГИГАНТОВ ПО КЛАССАМ (§10.2 п.1) ====================

// TestGiantRateByClass — доля звёзд с гигантом по классам (спека §5.2/§10.2):
// G/K/F ∈ [8%, 12%], M ∈ [2%, 5%], O/B < 1%, A ≤ 3%, L/T/Y < 2%.
// С металличностью (E[10^(0.5×[Fe/H])] ≈ 1.045) — допуск на среднее.
func TestGiantRateByClass(t *testing.T) {
	g := NewGenerator(nil, 42)
	g.means = DefaultPlanetMeans()
	const n = 10000

	rate := func(cls string) float64 {
		giants := 0
		for i := 0; i < n; i++ {
			_, orbit := g.simulateSystem(cls)
			if orbit > 0 {
				giants++
			}
		}
		return float64(giants) / float64(n)
	}

	for _, cls := range []string{"G", "K", "F"} {
		r := rate(cls)
		assert.InDelta(t, 0.10, r, 0.02, "%s: цель 10%% (окно 8–12%%)", cls)
	}
	rM := rate("M")
	assert.InDelta(t, 0.035, rM, 0.015, "M: цель 3%% (окно 2–5%%)")
	for _, cls := range []string{"O", "B"} {
		assert.Less(t, rate(cls), 0.01, "%s: < 1%% (флип с 0.8)", cls)
	}
	assert.LessOrEqual(t, rate("A"), 0.03, "A: ≤ 3%% (A = 2%%)")
	for _, cls := range []string{"L", "T", "Y"} {
		assert.Less(t, rate(cls), 0.02, "%s: < 2%%", cls)
	}
}

// ==================== ГОРЯЧИЕ ЮПИТЕРЫ (§10.2 п.2) ====================

// TestHotJupiterRateAndOrbit — горячие юпитеры (спека §5.3/§10.2): у G/K/F
// ≈ 1.0% (±0.5 п.п.), орбита ∈ {1, 2}; глобально ∈ [0.5%, 1.0%] (цель Wright
// 2012, пересчёт с фолбэком «горячий по необходимости»); гиганты A всегда
// на орбите 1–2 (n ≡ 1, фолбэк). Тест — на дефолтных mean (М1 @critic:
// при множителях профиля/расы часть гигантов M уходит на орбиту 1–2
// фолбэком §4.2 — это покрывается механикой, не тестом).
func TestHotJupiterRateAndOrbit(t *testing.T) {
	g := NewGenerator(nil, 7)
	g.means = DefaultPlanetMeans()
	const n = 20000

	// G/K/F: доля систем с гигантом на орбите 1–2 ≈ 1.0% (миграция 10% × 10%).
	hot := 0
	for _, cls := range []string{"G", "K", "F"} {
		for i := 0; i < n; i++ {
			_, orbit := g.simulateSystem(cls)
			if orbit == 1 || orbit == 2 {
				hot++
			}
		}
	}
	rate := float64(hot) / float64(3*n)
	assert.InDelta(t, 0.01, rate, 0.005, "G/K/F: горячие ≈ 1.0%% (получено %.3f%%)", rate*100)

	// Глобально: взвешенная доля горячих по всем классам ∈ [0.5%, 1.0%]
	// (веса 99.2.4 §4.1; фолбэк A/L/T/Y/O/B + миграция F/G/K/M).
	weights := map[string]float64{
		"O": 0.005, "B": 0.02, "A": 0.04, "F": 0.07, "G": 0.12,
		"K": 0.17, "M": 0.32, "L": 0.08, "T": 0.08, "Y": 0.095,
	}
	global := 0.0
	for cls, w := range weights {
		hotCls := 0
		for i := 0; i < n; i++ {
			_, orbit := g.simulateSystem(cls)
			if orbit == 1 || orbit == 2 {
				hotCls++
			}
		}
		global += w * float64(hotCls) / float64(n)
	}
	assert.InDelta(t, 0.007, global, 0.003, "глобально ∈ [0.5%%, 1.0%%] (получено %.3f%%)", global*100)

	// Гиганты A всегда на орбите 1–2 (n ≡ 1, фолбэк «горячий по необходимости»).
	for i := 0; i < n; i++ {
		_, orbit := g.simulateSystem("A")
		if orbit > 0 {
			assert.LessOrEqual(t, orbit, 2, "A-гигант всегда на орбите 1–2 (фолбэк)")
		}
	}
}

// ==================== ОДИН ГИГАНТ НА СИСТЕМУ (§10.2 п.3) ====================

// TestGiantOnePerSystem — ни одна система не имеет двух гигантов (спека
// §4.2/§10.2): горячий + дальний взаимоисключающие (один giantOrbit на
// систему). Проверка на мировом потоке generateWorldIntoBuffer.
func TestGiantOnePerSystem(t *testing.T) {
	g := NewGenerator(nil, 11)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(10000)

	const n = 1000
	for i := 0; i < n; i++ {
		cls := []string{"G", "K", "M", "F"}[i%4]
		w := WorldInfo{ID: fmt.Sprintf("w%d", i), Name: "W", SpectralClass: cls, StarType: "star"}
		g.generateWorldIntoBuffer(w, buf)
	}

	giants := map[string]int{}
	for _, row := range buf.planetRows {
		fields := row.([]interface{})
		worldID := fields[1].(string)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
		if data["is_gas_giant"] == true {
			giants[worldID]++
		}
	}
	for wid, cnt := range giants {
		assert.LessOrEqual(t, cnt, 1, "система %s: один гигант на систему", wid)
	}
	assert.Greater(t, len(giants), 0, "гиганты должны встречаться на выборке (иначе тест пустой)")
}

// ==================== РАЗМЕЩЕНИЕ ГИГАНТА (§10.2 п.4) ====================

// TestGiantOrbitPlacement — размещение гиганта (спека §5.4/§10.2): дальний
// гигант — орбита ∈ [3, planetCount]; система с planetCount < 3 дальнего
// гиганта не имеет (только горячий при planetCount ≥ 1); орбита никогда не
// превышает число планет; planetCount = 0 — гиганта нет.
func TestGiantOrbitPlacement(t *testing.T) {
	g := NewGenerator(nil, 13)
	for pc := 1; pc <= 8; pc++ {
		far, hot := 0, 0
		for i := 0; i < 20000; i++ {
			sp := StellarParams{SpectralClass: "G", Metallicity: 0}
			orbit := g.rollGiantOrbit(sp, pc)
			assert.GreaterOrEqual(t, orbit, 0)
			assert.LessOrEqual(t, orbit, pc, "орбита не превышает число планет")
			if orbit >= 3 {
				far++
			}
			if orbit == 1 || orbit == 2 {
				hot++
			}
		}
		if pc >= 3 {
			assert.Greater(t, far, 0, "pc=%d: дальние гиганты есть", pc)
			assert.Greater(t, hot, 0, "pc=%d: горячие (миграция) есть", pc)
		} else {
			assert.Zero(t, far, "pc=%d: дальних гигантов нет (нет орбиты ≥ 3)", pc)
		}
	}
	// planetCount = 0 — гиганта нет.
	sp := StellarParams{SpectralClass: "G", Metallicity: 0}
	for i := 0; i < 1000; i++ {
		assert.Zero(t, g.rollGiantOrbit(sp, 0))
	}
}

// ==================== НОВЫЕ MEAN (§10.2 п.5) ====================

// TestPlanetCountNewMeans — mean-модель на новых дефолтах (спека §5.1/§10.2):
// G/F ≈ 6.5, K/M ≈ 5.5, L/T/Y ≈ 1 (n ≡ 1), остатки ≤ 1, максимум ≤ 8;
// у M/G/K максимум ≥ 4.
func TestPlanetCountNewMeans(t *testing.T) {
	g := NewGenerator(nil, 17)
	g.means = DefaultPlanetMeans()

	assert.InDelta(t, 6.5, planetCountAverage(g, "G", 20000), 0.1, "G ≈ 6.5")
	assert.InDelta(t, 6.5, planetCountAverage(g, "F", 20000), 0.1, "F ≈ 6.5")
	assert.InDelta(t, 5.5, planetCountAverage(g, "K", 20000), 0.1, "K ≈ 5.5")
	assert.InDelta(t, 5.5, planetCountAverage(g, "M", 20000), 0.1, "M ≈ 5.5")
	assert.InDelta(t, 1.0, planetCountAverage(g, "L", 20000), 0.01, "L ≈ 1 (n ≡ 1)")
	assert.InDelta(t, 1.0, planetCountAverage(g, "T", 20000), 0.01, "T ≈ 1")
	assert.InDelta(t, 1.0, planetCountAverage(g, "Y", 20000), 0.01, "Y ≈ 1")

	// Остатки: максимум генератора 1 («чаще 0», 99.2.4 §5.2).
	for i := 0; i < 1000; i++ {
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "black_hole"}), 1)
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "neutron"}), 1)
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "white_dwarf"}), 1)
	}

	// Максимумы: ≤ 8 у всех, ≥ 4 у M/G/K.
	for _, cls := range []string{"M", "G", "K"} {
		maxN := 0
		for i := 0; i < 10000; i++ {
			n := g.determinePlanetCount(cls)
			assert.LessOrEqual(t, n, 8, "%s: потолок 8", cls)
			if n > maxN {
				maxN = n
			}
		}
		assert.GreaterOrEqual(t, maxN, 4, "%s: максимум ≥ 4", cls)
	}
}

// ==================== МЕТАЛЛИЧНОСТЬ (§10.2 п.6) ====================

// TestGiantMetallicityCorrelation — металличные G-звёзды дают гигантов чаще
// бедных (спека §4.4/§10.2): P_eff = P_giant × 10^(0.5×[Fe/H]), k = 0.5 —
// сигнал слабее, допуск шире.
func TestGiantMetallicityCorrelation(t *testing.T) {
	g := NewGenerator(nil, 19)
	const n = 30000
	richTotal, richGiant := 0, 0
	poorTotal, poorGiant := 0, 0
	for i := 0; i < n; i++ {
		met := testMetallicity(g.rng)
		sp := StellarParams{SpectralClass: "G", Metallicity: met}
		hasGiant := g.rollGiantOrbit(sp, 6) > 0
		if met > 0.2 {
			richTotal++
			if hasGiant {
				richGiant++
			}
		}
		if met < -0.2 {
			poorTotal++
			if hasGiant {
				poorGiant++
			}
		}
	}
	require.Greater(t, richTotal, 1000, "металличных выборок достаточно")
	require.Greater(t, poorTotal, 1000, "бедных выборок достаточно")
	richRate := float64(richGiant) / float64(richTotal)
	poorRate := float64(poorGiant) / float64(poorTotal)
	assert.Greater(t, richRate, poorRate,
		"металличные чаще: rich=%.3f%% poor=%.3f%%", richRate*100, poorRate*100)
}

// ==================== ОДИН РОЛЛ НА СИСТЕМУ (§10.2 п.7) ====================

// TestGiantNoEntropyInLoop — решение гиганта потребляет энтропию один раз на
// систему (не в цикле орбит, спека §4.2/§10.2): роллы «есть гигант»/
// «мигрировал?»/орбита — фиксированы и не масштабируются числом орбит.
// Проверка: поток RNG после rollGiantOrbit одинаков для planetCount = 4 и 6
// при одинаковом seed (Intn(2) и Intn(4) — оба по одному роллу).
func TestGiantNoEntropyInLoop(t *testing.T) {
	next := func(planetCount int) float64 {
		g := NewGenerator(nil, 555)
		sp := StellarParams{SpectralClass: "G", Metallicity: 0}
		g.rollGiantOrbit(sp, planetCount)
		return g.rng.Float64()
	}
	assert.Equal(t, next(4), next(6),
		"решение гиганта не зависит от числа орбит (один ролл на систему)")
}

// ==================== СВЕРХГИГАНТЫ БЕЗ ГИГАНТА (регрессия П1) ====================

// TestSupergiantNoGasGiant — сверхгиганты («прочая экзотика», фаза I +
// подтип lbv/wr, 99.2.4 §4.4) не получают газового гиганта (99.2.4 §5.3).
// Регрессия П1 (тестер): старый триггер orbitIndex >= 3 при n ≤ 1 гиганта
// не давал; per-системный ролл давал на орбиту 1 фолбэком (§5.4) — вопреки
// «без изменений» в §4.2. Проверка на мировом потоке с принудительным
// count = 1 (сверхгигант: mean 0.1, максимум 1 — планета существует).
func TestSupergiantNoGasGiant(t *testing.T) {
	g := NewGenerator(nil, 21)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(8)

	for _, cls := range []string{"O", "B", "A"} {
		for _, subtype := range []string{"lbv", "wr"} {
			w := WorldInfo{
				ID: "w", Name: "W", SpectralClass: cls, StarType: "star",
				Mods: &models.StellarMods{Phase: "I", Subtype: subtype},
			}
			planets := 0
			const n = 1000
			for i := 0; i < n; i++ {
				buf.reset()
				g.generateWorldWithCountIntoBuffer(w, 1, buf)
				for _, row := range buf.planetRows {
					fields := row.([]interface{})
					var data map[string]interface{}
					require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
					planets++
					assert.NotEqual(t, true, data["is_gas_giant"],
						"%s %s: сверхгигант без газового гиганта", cls, subtype)
				}
			}
			assert.Greater(t, planets, 0, "%s %s: планеты сверхгиганта генерируются (тест не пустой)", cls, subtype)
		}
	}
}