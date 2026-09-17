package races

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Каталог загружается и все 60 карточек проходят валидацию §16.
func TestLoadCatalogValidatesAllRaces(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.Len(t, Catalog(), 60, "в каталоге 60 рас (50 био + 10 роботов)")
	for _, r := range Catalog() {
		require.NoError(t, r.Validate(), "раса %s (%s)", r.ID, r.Name)
	}
}

// §16 п.1: композиция потребления = 100 у каждой био-расы. Роботы (наличие
// robotic) исключаются — их слой robotic, 13 биоосей пусты (99.2.24 §6 п.2).
func TestConsumptionSumsTo100(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	for _, r := range Catalog() {
		if r.Robotic != nil {
			continue
		}
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

// ==================== Роботы и AI (99.2.24, 51–60) ====================

// robotIDs — 10 рас семейства «Роботы и AI» (99.2.24 §4.1, файлы
// docs/gamedesign/races/*.md).
var robotIDs = []string{
	"archivists", "spark", "forges", "sleepers", "awakened",
	"arks", "world_machines", "scavengers", "philosophers", "echo_aliens",
}

// Все 10 роботов в каталоге: robotic-блок есть, consumption пуст,
// territory = "conditions", валидация проходит (99.2.24 §6 п.2, п.5).
func TestRobotsCards(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	for _, id := range robotIDs {
		r := ByID(id)
		require.NotNil(t, r, "робот %s есть в каталоге", id)
		require.NotNil(t, r.Robotic, "робот %s: блок robotic есть", id)
		assert.Empty(t, r.Consumption, "робот %s: consumption пуст (13 биоосей — не их слой)", id)
		assert.Equal(t, "conditions", r.Territory, "робот %s: территория по условиям среды", id)
		require.NoError(t, r.Validate(), "робот %s проходит валидацию", id)
	}
}

// Слой robotic валиден у всех 10 роботов (99.2.24 §6 п.3): power_source
// непуст (enum), heat из enum, categories ⊆ 6 категорий, axes ⊆ 10 осей
// (направление high|low).
func TestRoboticBlockValid(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	for _, id := range robotIDs {
		rb := ByID(id).Robotic
		require.NotEmpty(t, rb.PowerSource, "робот %s: power_source непуст", id)
		for _, ps := range rb.PowerSource {
			assert.True(t, powerSources[ps], "робот %s: power_source %q из enum", id, ps)
		}
		assert.True(t, heatModes[rb.Heat], "робот %s: heat %q из enum", id, rb.Heat)
		for _, c := range rb.Materials.Categories {
			assert.True(t, materialCategories[c], "робот %s: категория %q из 6 категорий", id, c)
		}
		for axis, dir := range rb.Materials.Axes {
			assert.True(t, materialAxes[axis], "робот %s: ось %q из 10 осей", id, axis)
			assert.Contains(t, []string{"high", "low"}, dir, "робот %s: направление оси %q", id, axis)
		}
	}
}

// Флаги механик-кандидатов (99.2.24 §4.2): dormancy только у 54 Спящие,
// famine_aggression у 52 Искра и 58 Санитары, heat_twist у 53/57/58/60.
func TestRobotsMechanicFlags(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	assert.True(t, ByID("sleepers").Dormancy, "Спящие: dormancy")
	assert.True(t, ByID("spark").FamineAggression, "Искра: famine_aggression")
	assert.True(t, ByID("scavengers").FamineAggression, "Санитары: famine_aggression")
	for _, id := range []string{"forges", "world_machines", "scavengers", "echo_aliens"} {
		assert.True(t, ByID(id).Robotic.HeatTwist, "%s: heat_twist", id)
	}
	for _, id := range robotIDs {
		if id == "sleepers" {
			continue
		}
		assert.False(t, ByID(id).Dormancy, "%s: dormancy только у Спящих", id)
	}
}

// Роботы с пустым consumption проходят валидацию (сумма-100 не применяется).
func TestValidateRoboticSkipsConsumptionSum(t *testing.T) {
	r := &Race{
		ID:   "test_robot",
		Name: "Тест-робот",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 200, Hi: f(300)}, Surv: Range{Lo: 100, Hi: f(400)}},
			Pressure:    Window{Opt: Range{Lo: 0, Hi: f(1)}, Surv: Range{Lo: 0, Hi: f(50)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(30)}, Surv: Range{Lo: 0, Hi: f(70)}},
		},
		Attributes:  Attributes{Aggression: 50, Curiosity: 50, Reproduction: 1, Intelligence: 50, Diplomacy: 50, Resilience: 50},
		Consumption: map[string]float64{}, // пусто — сумма 0, не 100
		Robotic: &Robotic{
			PowerSource: []string{"солнце"},
			Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"проводимость": "high"}},
			Heat:        "радиаторы",
		},
		Territory: "conditions",
		Bulge:     "hot",
	}
	require.NoError(t, r.Validate(), "робот с пустым consumption проходит валидацию")
}

// Валидация ловит робота с непустым consumption (13 биоосей — не их слой).
func TestValidateRoboticConsumptionNotEmpty(t *testing.T) {
	r := &Race{
		ID:   "test_robot",
		Name: "Тест-робот",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 200, Hi: f(300)}, Surv: Range{Lo: 100, Hi: f(400)}},
			Pressure:    Window{Opt: Range{Lo: 0, Hi: f(1)}, Surv: Range{Lo: 0, Hi: f(50)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(30)}, Surv: Range{Lo: 0, Hi: f(70)}},
		},
		Attributes:  Attributes{Aggression: 50, Curiosity: 50, Reproduction: 1, Intelligence: 50, Diplomacy: 50, Resilience: 50},
		Consumption: map[string]float64{"ВОД": 100},
		Robotic: &Robotic{
			PowerSource: []string{"солнце"},
			Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"проводимость": "high"}},
			Heat:        "радиаторы",
		},
		Territory: "conditions",
		Bulge:     "hot",
	}
	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consumption должен быть пуст")
}

// Валидация ловит робота с territory ≠ "conditions" (99.2.24 §6 п.5).
func TestValidateRoboticTerritoryNotConditions(t *testing.T) {
	r := &Race{
		ID:   "test_robot",
		Name: "Тест-робот",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 200, Hi: f(300)}, Surv: Range{Lo: 100, Hi: f(400)}},
			Pressure:    Window{Opt: Range{Lo: 0, Hi: f(1)}, Surv: Range{Lo: 0, Hi: f(50)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(30)}, Surv: Range{Lo: 0, Hi: f(70)}},
		},
		Attributes:  Attributes{Aggression: 50, Curiosity: 50, Reproduction: 1, Intelligence: 50, Diplomacy: 50, Resilience: 50},
		Consumption: map[string]float64{},
		Robotic: &Robotic{
			PowerSource: []string{"солнце"},
			Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"проводимость": "high"}},
			Heat:        "радиаторы",
		},
		Territory: "adjacency",
		Bulge:     "hot",
	}
	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "territory")
}

// Валидация ловит невалидный robotic-блок: power_source вне enum.
func TestValidateRoboticBadPowerSource(t *testing.T) {
	rb := &Robotic{
		PowerSource: []string{"антиматерия"},
		Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"проводимость": "high"}},
		Heat:        "радиаторы",
	}
	err := rb.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "power_source")
}

// Валидация ловит пустой power_source.
func TestValidateRoboticEmptyPowerSource(t *testing.T) {
	rb := &Robotic{
		PowerSource: nil,
		Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"проводимость": "high"}},
		Heat:        "радиаторы",
	}
	err := rb.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "power_source пуст")
}

// Валидация ловит heat вне enum.
func TestValidateRoboticBadHeat(t *testing.T) {
	rb := &Robotic{
		PowerSource: []string{"солнце"},
		Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"проводимость": "high"}},
		Heat:        "кондиционер",
	}
	err := rb.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "heat")
}

// Валидация ловит категорию вне 6 категорий 09_resources §9.1.3.
func TestValidateRoboticBadCategory(t *testing.T) {
	rb := &Robotic{
		PowerSource: []string{"солнце"},
		Materials:   RoboticMaterials{Categories: []string{"металлы"}, Axes: map[string]string{"проводимость": "high"}},
		Heat:        "радиаторы",
	}
	err := rb.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "categories")
}

// Валидация ловит ось вне 10 осей 09_resources §9.1.2.
func TestValidateRoboticBadAxis(t *testing.T) {
	rb := &Robotic{
		PowerSource: []string{"солнце"},
		Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"магнетизм": "high"}},
		Heat:        "радиаторы",
	}
	err := rb.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "axes")
}

// Валидация ловит направление оси вне high|low.
func TestValidateRoboticBadAxisDirection(t *testing.T) {
	rb := &Robotic{
		PowerSource: []string{"солнце"},
		Materials:   RoboticMaterials{Categories: []string{"🪨"}, Axes: map[string]string{"проводимость": "medium"}},
		Heat:        "радиаторы",
	}
	err := rb.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "high|low")
}