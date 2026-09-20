// internal/generator/planet/exotic_test.go
// Тесты ветки экзотики (99.2.4 §5.2/§5.3): mean-модель числа планет,
// «тёмная» ЧД, WD-кламп, планеты остатков — без нарушения аудит-гейтов §6.2.
package planet

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== MEAN-МОДЕЛЬ (99.2.4 §5.2) ====================

func TestMeanPlanetCountDistribution(t *testing.T) {
	// mean 2.5 → floor 2 + Бернулли(0.5): E[n] = 2.5 на большой выборке.
	rng := rand.New(rand.NewSource(1))
	sum := 0.0
	const n = 200_000
	for i := 0; i < n; i++ {
		sum += float64(meanPlanetCount(rng, 2.5, 8))
	}
	assert.InDelta(t, 2.5, sum/n, 0.05, "E[n] должен быть ≈ mean")

	// mean < 1 → только {0, 1} («чаще 0», P(0) = 1 − mean).
	rng2 := rand.New(rand.NewSource(2))
	zeros, ones, over := 0, 0, 0
	for i := 0; i < 100_000; i++ {
		switch meanPlanetCount(rng2, 0.1, 8) {
		case 0:
			zeros++
		case 1:
			ones++
		default:
			over++
		}
	}
	assert.Zero(t, over, "mean < 1 не даёт n > 1")
	assert.InDelta(t, 0.9, float64(zeros)/100_000, 0.02, "P(0) = 1 − mean")

	// mean = 0 → 0; mean ≤ 0 → 0.
	assert.Zero(t, meanPlanetCount(rng2, 0, 8))
	assert.Zero(t, meanPlanetCount(rng2, -1, 8))

	// Потолок max: mean > max молча обрезает (валидация конфига это отклоняет, 422).
	for i := 0; i < 1000; i++ {
		assert.LessOrEqual(t, meanPlanetCount(rng2, 20, 8), 8)
	}
}

func TestDefaultPlanetMeansValid(t *testing.T) {
	m := DefaultPlanetMeans()
	require.NoError(t, m.Validate(), "дефолтные средние внутри [0, 8]")
	assert.Equal(t, 8, m.Max, "потолок 8 — Kepler-90 (решение §4е)")
}

// ==================== WD-КЛАМП (99.2.4 §5.3, баланс-проверка В4) ====================

func TestWdPlanetTempClampedTo25(t *testing.T) {
	// Кламп: равновесная формула на орбитах 5–8 при L 10⁻²–10⁻⁴ даёт 5.3–36.9 K,
	// итог всегда ≥ 25 K и ≤ 37 K — внутри [20, 2500] (аудит-гейт §6.2).
	rng := rand.New(rand.NewSource(1))
	g := &Generator{rng: rng}
	for i := 0; i < 5000; i++ {
		tm := g.wdPlanetTemp()
		assert.GreaterOrEqual(t, tm, 25.0, "WD-планета не холоднее 25 K (кламп)")
		assert.LessOrEqual(t, tm, 37.0, "WD-планета не горячее 37 K (формула)")
	}
}

// ==================== «ТЁМНАЯ» ЧД (99.2.4 §4д.4, §5.3) ====================

func TestBlackHolePlanetsDarkRocky(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	g := &Generator{rng: rng, usedNames: map[string]bool{}}
	w := WorldInfo{
		ID: "w1", Name: "ЧД-система",
		StarType: "black_hole", SystemType: "single",
	}

	for i := 0; i < 1000; i++ {
		p := g.generateExoticPlanet(w, 10+g.rng.Intn(3))
		require.NotNil(t, p)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Data, &data))

		temp := data["temperature"].(float64)
		assert.GreaterOrEqual(t, temp, 30.0, "ЧД-планета: фоновый нагрев 30–80 K")
		assert.LessOrEqual(t, temp, 80.0)
		assert.Equal(t, 0.0, data["water_percent"], "вода 0 (§5.3)")
		assert.Equal(t, false, data["life"], "жизнь 0 (§5.3)")
		assert.Equal(t, true, data["exotic_system"], "флаг exotic_system=true (читают аудит и UI)")
		assert.Equal(t, "black_hole", data["star_type"])
		assert.Equal(t, false, data["settleable"], "settleable=false всегда (§6.1)")
		// Без NaN/нулей, скомпрометирующих аудит (§6.2).
		assert.Greater(t, data["mass"].(float64), 0.0)
		assert.Greater(t, data["size"].(float64), 0.0)
		assert.Greater(t, data["density"].(float64), 0.0)
		assert.Contains(t, data["type"], "мёртвая", "свой тип, не землеподобная")
	}
}

// ==================== ОСТАТКИ: ВОДА/ЖИЗНЬ/ПРИГОДНОСТЬ (99.2.4 §5.3) ====================

func TestExoticPlanetsNeverSuitable(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	g := &Generator{rng: rng, usedNames: map[string]bool{}}

	for _, st := range []string{"black_hole", "neutron", "white_dwarf"} {
		w := WorldInfo{ID: "w", Name: "x", StarType: st}
		for i := 0; i < 500; i++ {
			p := g.generateExoticPlanet(w, 1)
			require.NotNil(t, p)
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal(p.Data, &data))

			assert.Equal(t, false, data["life"])
			assert.Equal(t, 0.0, data["water_percent"])
			assert.Equal(t, false, data["settleable"])
			// T внутри [20, 2500] — аудит temp_too_low/high не срабатывает.
			tm := data["temperature"].(float64)
			assert.GreaterOrEqual(t, tm, 20.0)
			assert.LessOrEqual(t, tm, 2500.0)
		}
	}
}

func TestProtostarHasNoPlanets(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	g := &Generator{rng: rng}
	w := WorldInfo{ID: "w", Name: "x", StarType: "protostar"}
	assert.Nil(t, g.generateExoticPlanet(w, 1), "протозвезда — диск вместо планет (§5.3)")
	assert.Zero(t, g.planetCountFor(w), "planetCountFor протозвезды = 0")
}

// ==================== НАСЛЕДОВАНИЕ ВОЗРАСТА (41a §4.2) ====================

// TestExoticPlanetSystemAgeInherits — system_age планеты остатка наследует
// возраст мира (w.Age); старые миры без возраста — фолбэк-ролл в [2, 13]
// (вместо 2–10, решение создателя).
func TestExoticPlanetSystemAgeInherits(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	g := &Generator{rng: rng, usedNames: map[string]bool{}}

	// Наследование: w.Age → data["system_age"] планеты.
	age := 7.5
	w := WorldInfo{ID: "w", Name: "x", StarType: "black_hole", Age: &age}
	for i := 0; i < 50; i++ {
		p := g.buildExoticPlanet(w, 10, 50.0)
		require.NotNil(t, p)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Data, &data))
		assert.Equal(t, 7.5, data["system_age"], "system_age наследует возраст мира")
	}

	// Фолбэк старых миров (Age=nil): ролл в [2, 13].
	w2 := WorldInfo{ID: "w2", Name: "x2", StarType: "neutron"}
	for i := 0; i < 500; i++ {
		p := g.buildExoticPlanet(w2, 1, 80.0)
		require.NotNil(t, p)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Data, &data))
		sa := data["system_age"].(float64)
		assert.GreaterOrEqual(t, sa, 2.0, "фолбэк ≥ 2 млрд лет")
		assert.LessOrEqual(t, sa, 13.0, "фолбэк ≤ 13 млрд лет")
	}
}

// ==================== ЧИСЛО ПЛАНЕТ ПО ТИПУ (99.2.4 §5.2) ====================

func TestPlanetCountForExoticMaxOne(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	g := &Generator{rng: rng, means: DefaultPlanetMeans()}

	// Остатки: даже при mean > 1 — максимум генератора 1 (решение §4к.1).
	g.means.BlackHole = 5
	g.means.WhiteDwarf = 3
	for i := 0; i < 1000; i++ {
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "black_hole"}), 1)
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "white_dwarf"}), 1)
		assert.LessOrEqual(t, g.planetCountFor(WorldInfo{StarType: "neutron"}), 1)
	}
}

func TestPlanetCountForBinaryTypes(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	g := &Generator{rng: rng, means: DefaultPlanetMeans()}

	// Широкая двойная: ×0.9 к счёту главной; тесная: mean 0.2.
	sumWide, sumClose := 0.0, 0.0
	const n = 100_000
	for i := 0; i < n; i++ {
		sumWide += float64(g.planetCountFor(WorldInfo{
			SpectralClass: "G", SystemType: "binary",
			Mods: &models.StellarMods{BinaryType: "wide"},
		}))
		sumClose += float64(g.planetCountFor(WorldInfo{
			SpectralClass: "G", SystemType: "binary",
			Mods: &models.StellarMods{BinaryType: "close"},
		}))
	}
	assert.InDelta(t, 6.5*0.9, sumWide/n, 0.05, "S-тип: ×0.9 (99.2.4 §5.2; спека 2026-09-20 §5.1: G mean 6.5)")
	assert.InDelta(t, 0.2, sumClose/n, 0.05, "P-тип: mean 0.2")

	// Протозвезда и без модов — 0.
	assert.Zero(t, g.planetCountFor(WorldInfo{StarType: "protostar"}))
}