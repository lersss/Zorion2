package races

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Каталог загружается и все 50 карточек проходят валидацию §16.
func TestLoadCatalogValidatesAllRaces(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.Len(t, Catalog(), 50, "в каталоге 50 рас")
	for _, r := range Catalog() {
		require.NoError(t, r.Validate(), "раса %s (%s)", r.ID, r.Name)
	}
}

// §16 п.1: композиция потребления = 100 у каждой расы.
func TestConsumptionSumsTo100(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	for _, r := range Catalog() {
		sum := 0.0
		for _, v := range r.Consumption {
			sum += v
		}
		assert.InDelta(t, 100.0, sum, 0.01, "раса %s: сумма потребления", r.ID)
	}
}

// §16 п.2: opt ⊆ surv по каждой оси.
func TestOptSubsetSurv(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	for _, r := range Catalog() {
		for name, w := range r.windows() {
			assert.True(t, w.Opt.SubsetOf(w.Surv),
				"раса %s: opt ⊄ surv по оси %s (opt [%v, %v], surv [%v, %v])",
				r.ID, name, w.Opt.Lo, w.Opt.Hi, w.Surv.Lo, w.Surv.Hi)
		}
	}
}

// §16 п.2: surv в пределах диапазонов каскада (T [20,2500], P [0,1000], rad [0,100]).
func TestSurvWithinCascadeRanges(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	for _, r := range Catalog() {
		if r.OffCascade {
			continue // осознанное исключение (раса 40 Водородные)
		}
		for name, w := range r.windows() {
			rng, ok := cascadeRanges[name]
			if !ok {
				continue
			}
			assert.GreaterOrEqual(t, w.Surv.Lo, rng[0], "раса %s: surv.Lo по оси %s", r.ID, name)
			if w.Surv.Hi != nil {
				assert.LessOrEqual(t, *w.Surv.Hi, rng[1], "раса %s: surv.Hi по оси %s", r.ID, name)
			}
		}
	}
}

// §16 п.2 исключение: раса 40 Водородные помечена off_cascade, её P-окно
// «1000+» вне диапазона каскада — валидация допускает это явно.
func TestOffCascadeException(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	h := ByID("hydrogen")
	require.NotNil(t, h, "раса 40 Водородные есть в каталоге")
	assert.True(t, h.OffCascade, "раса 40 помечена off_cascade")
	assert.Nil(t, h.Conditions.Pressure.Surv.Hi, "P-окно «1000+» без верхней границы")
	assert.Equal(t, 1000.0, h.Conditions.Pressure.Surv.Lo)
	require.NoError(t, h.Validate(), "валидация допускает off_cascade-расу")
}

// Люди — раса 1: id "humans", liquid_water=true, bulge в холод (§7.3 пример).
func TestHumansCard(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	h := ByID("humans")
	require.NotNil(t, h)
	assert.Equal(t, "Люди", h.Name)
	require.NotNil(t, h.Conditions.LiquidWater)
	assert.True(t, *h.Conditions.LiquidWater, "людям нужна жидкая вода")
	assert.Equal(t, "cold", h.Bulge, "люди подселяются в холод (§7.3)")
}

// Валидация ловит сумму потребления ≠ 100.
func TestValidateConsumptionSum(t *testing.T) {
	r := &Race{
		ID:   "test",
		Name: "Тест",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 200, Hi: f(300)}, Surv: Range{Lo: 100, Hi: f(400)}},
			Pressure:    Window{Opt: Range{Lo: 1, Hi: f(10)}, Surv: Range{Lo: 0.5, Hi: f(50)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(20)}, Surv: Range{Lo: 0, Hi: f(50)}},
		},
		Attributes:   Attributes{Aggression: 50, Curiosity: 50, Reproduction: 1, Intelligence: 50, Diplomacy: 50, Resilience: 50},
		Consumption:  map[string]float64{"ВОД": 60, "ОРГ": 30}, // сумма 90
		Bulge:        "none",
	}
	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "сумма потребления")
}

// Валидация ловит opt ⊄ surv.
func TestValidateOptNotSubsetSurv(t *testing.T) {
	r := &Race{
		ID:   "test",
		Name: "Тест",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 200, Hi: f(300)}, Surv: Range{Lo: 100, Hi: f(250)}}, // opt.Hi 300 > surv.Hi 250
			Pressure:    Window{Opt: Range{Lo: 1, Hi: f(10)}, Surv: Range{Lo: 0.5, Hi: f(50)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(20)}, Surv: Range{Lo: 0, Hi: f(50)}},
		},
		Attributes:   Attributes{Aggression: 50, Curiosity: 50, Reproduction: 1, Intelligence: 50, Diplomacy: 50, Resilience: 50},
		Consumption:  map[string]float64{"ВОД": 100},
		Bulge:        "none",
	}
	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opt ⊄ surv")
}

// Валидация ловит surv вне диапазона каскада (без off_cascade).
func TestValidateSurvOutsideCascade(t *testing.T) {
	r := &Race{
		ID:   "test",
		Name: "Тест",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 200, Hi: f(300)}, Surv: Range{Lo: 10, Hi: f(400)}}, // surv.Lo 10 < 20
			Pressure:    Window{Opt: Range{Lo: 1, Hi: f(10)}, Surv: Range{Lo: 0.5, Hi: f(50)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(20)}, Surv: Range{Lo: 0, Hi: f(50)}},
		},
		Attributes:   Attributes{Aggression: 50, Curiosity: 50, Reproduction: 1, Intelligence: 50, Diplomacy: 50, Resilience: 50},
		Consumption:  map[string]float64{"ВОД": 100},
		Bulge:        "none",
	}
	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "вне диапазона каскада")
}

// Range.Contains: nil Hi = без верхней границы.
func TestRangeContains(t *testing.T) {
	r := Range{Lo: 1, Hi: nil}
	assert.True(t, r.Contains(1))
	assert.True(t, r.Contains(1e9))
	assert.False(t, r.Contains(0.9))

	r2 := Range{Lo: 0, Hi: f(100)}
	assert.True(t, r2.Contains(0))
	assert.True(t, r2.Contains(100))
	assert.False(t, r2.Contains(100.1))
}

func f(v float64) *float64 { return &v }