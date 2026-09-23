// internal/generator/planet/giant_smoke_test.go
//
// Смоук 20 000 миров (спека 2026-09-20 §11, критерий приёмки): дефолтные
// настройки, классы звёзд и типы систем по весам 99.2.4 §4.1 (как продакшн).
// Метрики: доля G/K/F и M с гигантом, горячие юпитеры глобально, максимумы
// планет, обитаемая доля.
package planet

import (
	"encoding/json"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator/galaxy"
	"zorion/internal/models"
	"zorion/internal/races"
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

// rollSmokeBinaryMods — модификаторы двойной/кратной системы для смоука:
// wide ~70% / close ~30% (модель теста), у close — CompanionSepAU
// (log-uniform [0.05, 0.9] — galaxy.closeSepMin/Max), чтобы сработала
// P-ветка generateCircumbinaryPlanet (ревью 2026-09-21, правка 2).
func rollSmokeBinaryMods(g *Generator, cls string) *models.StellarMods {
	mods := &models.StellarMods{Companion: cls}
	if g.rng.Float64() < 0.7 {
		mods.BinaryType = "wide"
	} else {
		mods.BinaryType = "close"
		sep := 0.05 * math.Pow(0.9/0.05, g.rng.Float64())
		mods.CompanionSepAU = &sep
	}
	return mods
}

// TestSmoke20000Worlds — смоук 20 000 миров по критерию приёмки §13 спеки
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны: G/F с гигантом 8–12%
// (≈10.1%), K/M ≈5.7% (вилка по числу орбит n, не по классу), A/O/B/L/T/Y < 1%,
// горячие юпитеры глобально [0.5%, 1.0%] (Wright 2012, ≈0.94%), максимум планет
// M/G/K ≥ 4 и любой ≤ 8, обитаемая доля (окно 2.0–4%), мини-нептуны непусты.
// Доля обитаемых — не инвариант (решение создателя 2026-09-21): может быть
// больше или меньше; значение измеряется и докладывается, подгонки нет.
//
// Мировой поток: планеты генерируются generateWorldWithCountIntoBuffer —
// ролится бюджет облака M_диск, газовый резервуар M_gas и пред-слой перелива
// (как в игре). Горячие юпитеры — только мигрировавшие гиганты перелива
// (P-ветка двойных исключена: `circumbinary`). Пригодность — настоящий флаг
// Settleable (races.HumansSuitable), а не широкая эвристика.
func TestSmoke20000Worlds(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
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
	settleable := 0
	broadHabitable := 0
	maxPlanets := 0
	maxPlanetsMGK := 0
	miniNeptunes := 0
	worldsWithMini := 0

	buf := newBatchBuffers(64)
	for i := 0; i < worlds; i++ {
		st := weightedPickTest(g.rng, systemWeights, systemOrder)
		cls := weightedPickTest(g.rng, weights, spectralOrder)
		classWorlds[cls]++

		// WorldInfo как в продакшне (handlers/admin_universe.go): обычные
		// звёзды — StarType "star" + SystemType; экзотика — StarType.
		w := WorldInfo{ID: "w", Name: "World", SpectralClass: cls, StarType: "star"}
		switch st {
		case "single":
			// одиночная звезда — без модификаторов
		case "binary", "multiple":
			w.SystemType = st
			w.Mods = rollSmokeBinaryMods(g, cls)
		default:
			// Экзотика (ЧД/НЗ/WD/протозвезда/прочая): гигантов нет,
			// планеты не обитаемы (99.2.4 §5.3).
			w.StarType = st
			w.SpectralClass = ""
		}

		count := g.planetCountFor(w)
		buf.reset()
		generated := g.generateWorldWithCountIntoBuffer(w, count, buf)
		totalPlanets += generated
		if generated > maxPlanets {
			maxPlanets = generated
		}
		if (cls == "M" || cls == "G" || cls == "K") && generated > maxPlanetsMGK {
			maxPlanetsMGK = generated
		}

		worldHasGiant := false
		worldHasMini := false
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			orbit, _ := fields[3].(int)
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				worldHasGiant = true
				// Горячий юпитер — мигрировавший гигант перелива на орбите
				// 1–2. Циркумбинарные P-гиганты (orbit_center = barycenter,
				// своя P-ветка §9.2) в метрику НЕ входят — это не миграция.
				if orbit <= 2 && data["circumbinary"] != true {
					hotJupiters++
				}
				continue
			}
			if data["is_mini_neptune"] == true {
				worldHasMini = true
				miniNeptunes++
				continue
			}
			// Настоящий флаг пригодности (races.HumansSuitable) — тот же
			// источник, что res.Settleable в каскаде (65a).
			if races.HumansSuitable(data) {
				settleable++
			}
			// Широкая эвристика — информационная метрика (лог, без окна).
			if data["liquid_water_possible"] == true {
				tv, _ := data["temperature"].(float64)
				wv, _ := data["water_percent"].(float64)
				if wv > 10 && tv > 200 && tv < 350 {
					broadHabitable++
				}
			}
		}
		if worldHasGiant {
			classGiants[cls]++
		}
		if worldHasMini {
			worldsWithMini++
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
	gf := rate("G", "F")
	km := rate("K", "M")
	gkf := rate("G", "K", "F")
	m := rate("M")
	ob := rate("O", "B")
	a := rate("A")
	lty := rate("L", "T", "Y")
	hotGlobal := float64(hotJupiters) / float64(worlds)
	settleableFrac := float64(settleable) / float64(totalPlanets)
	broadFrac := float64(broadHabitable) / float64(totalPlanets)

	t.Logf("смоук 20 000 миров (спека 2026-09-23 §13):")
	t.Logf("  G/F с гигантом: %.2f%% (цель ≈10.1%%, окно 8–12%%)", gf*100)
	t.Logf("  K/M с гигантом: %.2f%% (цель ≈5.7%%, вилка по n)", km*100)
	t.Logf("  G/K/F с гигантом: %.2f%% (инфо, устаревший пул)", gkf*100)
	t.Logf("  M с гигантом: %.2f%% (инфо)", m*100)
	t.Logf("  O/B с гигантом: %.2f%% (цель < 1%%)", ob*100)
	t.Logf("  A с гигантом: %.2f%% (цель < 1%%)", a*100)
	t.Logf("  L/T/Y с гигантом: %.2f%% (цель < 1%%)", lty*100)
	t.Logf("  горячие юпитеры глобально: %.2f%% (цель ≈0.94%%, вилка Wright 0.5–1.0%%)", hotGlobal*100)
	t.Logf("  мини-нептуны: планет %d (%.2f%% планет), миров %d (%.2f%%)",
		miniNeptunes, float64(miniNeptunes)/float64(totalPlanets)*100,
		worldsWithMini, float64(worldsWithMini)/float64(worlds)*100)
	t.Logf("  максимум планет (M/G/K): %d (цель ≥ 4)", maxPlanetsMGK)
	t.Logf("  максимум планет (все): %d (цель ≤ 8)", maxPlanets)
	t.Logf("  пригодная доля (Settleable): %.2f%% (окно 2.0–4%%, этап 1)", settleableFrac*100)
	t.Logf("  широкая эвристика (инфо, без окна): %.2f%%", broadFrac*100)
	t.Logf("  планет всего: %d (спека §8: ≈ 76 000)", totalPlanets)

	// Критерий приёмки §13 (частота — по числу орбит n, не по классу звезды).
	assert.GreaterOrEqual(t, gf, 0.08, "G/F с гигантом ≥ 8%%")
	assert.LessOrEqual(t, gf, 0.12, "G/F с гигантом ≤ 12%%")
	// K/M: спека окна не задаёт (§13: окно [8,12]% — только для G/F, K/M —
	// вилка по n), но даёт ЗАМЕР §10.7 — 5.7%. Допуск ±2 п.п. назван:
	// сэмплинг 20 000 миров (SE ≈ 0.4 п.п.) + разброс по сидам; это не
	// спековое окно, а привязка к измеренной величине.
	assert.InDelta(t, 0.057, km, 0.02, "K/M с гигантом ≈ 5.7%% (замер §10.7, окно спека не задаёт)")
	assert.Less(t, ob, 0.01, "O/B < 1%%")
	assert.Less(t, a, 0.01, "A < 1%% (n ≡ 1)")
	assert.Less(t, lty, 0.01, "L/T/Y < 1%% (n ≡ 1)")
	// §13: горячие юпитеры глобально 0.5–1.0% (Wright 2012; цель ≈0.94%).
	// Метрика — только мигрировавшие гиганты перелива (P-ветка исключена).
	assert.GreaterOrEqual(t, hotGlobal, 0.005, "горячие юпитеры ≥ 0.5%%")
	assert.LessOrEqual(t, hotGlobal, 0.010, "горячие юпитеры ≤ 1.0%%")
	// Мини-нептуны: класс непуст (критерий приёмки §13); доля = P_ov·(1−f_eff).
	assert.Greater(t, worldsWithMini, 0, "мини-нептуны встречаются (класс непуст)")
	// Санити-граница (не спековое окно): доля миров с мини-нептуном по модели
	// P_ov·(1−f_eff) ≈ 4%; 15% отсекает грубую ошибку, спекой окно не задано.
	assert.Less(t, float64(worldsWithMini)/float64(worlds), 0.15, "мини-нептуны не доминируют (санити)")
	assert.GreaterOrEqual(t, maxPlanetsMGK, 4, "максимум планет M/G/K ≥ 4")
	assert.LessOrEqual(t, maxPlanets, 8, "максимум планет ≤ 8")
	// Доля обитаемых — не инвариант (решение создателя 2026-09-21): может
	// быть больше или меньше; значение измеряется и докладывается, подгонки
	// под окно нет (настоящий флаг Settleable — races.HumansSuitable).
	assert.GreaterOrEqual(t, settleableFrac, 0.02, "пригодная доля ≥ 2.0%%")
	assert.LessOrEqual(t, settleableFrac, 0.04, "пригодная доля ≤ 4%%")
}
