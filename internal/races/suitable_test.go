package races

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planetData — минимальные данные планеты каскада (99.2.20 §4.1).
func planetData() map[string]interface{} {
	return map[string]interface{}{
		"temperature": 288.0,
		"gravity":     1.0,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": 1.0,
			"composition": map[string]interface{}{
				"N2": 78.0, "O2": 21.0, "CO2": 0.04,
			},
		},
		"core": map[string]interface{}{
			"radioactivity":  10.0,
			"heat_flux_w_m2": 0.1,
		},
		"liquid_water_possible": true,
	}
}

// humans — карточка людей для тестов (без загрузки каталога).
func humans() *Race {
	lw := true
	return &Race{
		ID:   "humans",
		Name: "Люди",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 273, Hi: f(310)}, Surv: Range{Lo: 250, Hi: f(330)}},
			Pressure:    Window{Opt: Range{Lo: 0.5, Hi: f(3)}, Surv: Range{Lo: 0.2, Hi: f(10)}},
			Atmosphere: Atmosphere{
				Need:   map[string]float64{"O2": 10},
				Poison: map[string]float64{"H2S": 0, "CO2": 5},
			},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(20)}, Surv: Range{Lo: 0, Hi: f(50)}},
			Gravity:     &Window{Opt: Range{Lo: 0.8, Hi: f(1.2)}, Surv: Range{Lo: 0.5, Hi: f(2)}},
			LiquidWater: &lw,
		},
	}
}

// Пригодность: все оси в surv-окнах — true.
func TestSuitableAllAxesMatch(t *testing.T) {
	assert.True(t, humans().Suitable(planetData()))
}

// Пригодность: одна ось вне surv-окна — false (конъюнкция §16 п.4).
func TestSuitableTemperatureOut(t *testing.T) {
	d := planetData()
	d["temperature"] = 400.0 // вне surv [250, 330]
	assert.False(t, humans().Suitable(d))
}

func TestSuitablePressureOut(t *testing.T) {
	d := planetData()
	d["atmosphere_data"].(map[string]interface{})["pressure_atm"] = 50.0 // вне surv [0.2, 10]
	assert.False(t, humans().Suitable(d))
}

func TestSuitableRadiationOut(t *testing.T) {
	d := planetData()
	d["core"].(map[string]interface{})["radioactivity"] = 80.0 // вне surv [0, 50]
	assert.False(t, humans().Suitable(d))
}

func TestSuitableGravityOut(t *testing.T) {
	d := planetData()
	d["gravity"] = 5.0 // вне surv [0.5, 2]
	assert.False(t, humans().Suitable(d))
}

// Пригодность: poison-газ выше максимума — false.
func TestSuitablePoisonGas(t *testing.T) {
	d := planetData()
	d["atmosphere_data"].(map[string]interface{})["composition"].(map[string]interface{})["H2S"] = 1.0 // яд > 0
	assert.False(t, humans().Suitable(d))
}

// Пригодность: need-газ ниже минимума — false.
func TestSuitableNeedGasBelowMin(t *testing.T) {
	d := planetData()
	d["atmosphere_data"].(map[string]interface{})["composition"].(map[string]interface{})["O2"] = 5.0 // need ≥ 10
	assert.False(t, humans().Suitable(d))
}

// Пригодность: отсутствующая ось = «не влияет» (полный диапазон).
func TestSuitableMissingAxisDoesNotAffect(t *testing.T) {
	d := planetData()
	delete(d, "gravity")
	delete(d, "core") // radioactivity + heat_flux
	delete(d, "liquid_water_possible")
	assert.True(t, humans().Suitable(d), "отсутствующие оси не влияют на пригодность")
}

// Пригодность: отсутствующий состав атмосферы = «не влияет».
func TestSuitableMissingCompositionDoesNotAffect(t *testing.T) {
	d := planetData()
	delete(d["atmosphere_data"].(map[string]interface{}), "composition")
	assert.True(t, humans().Suitable(d))
}

// Пригодность: liquid_water=true требуется — планета без флага не подходит.
func TestSuitableLiquidWaterRequired(t *testing.T) {
	d := planetData()
	d["liquid_water_possible"] = false
	assert.False(t, humans().Suitable(d), "людям нужна жидкая вода")
}

// Пригодность: heat_flux окно (Глубинники: surv ≥ 0.1) — ниже не подходит.
func TestSuitableHeatFluxWindow(t *testing.T) {
	deep := &Race{
		ID:   "deep_dwellers",
		Name: "Глубинники",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 273, Hi: f(300)}, Surv: Range{Lo: 268, Hi: f(310)}},
			Pressure:    Window{Opt: Range{Lo: 1, Hi: f(100)}, Surv: Range{Lo: 0.5, Hi: f(300)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(30)}, Surv: Range{Lo: 0, Hi: f(60)}},
			HeatFlux:    &Window{Opt: Range{Lo: 1, Hi: nil}, Surv: Range{Lo: 0.1, Hi: nil}},
		},
	}
	d := planetData()
	d["core"].(map[string]interface{})["heat_flux_w_m2"] = 0.05 // ниже surv 0.1
	assert.False(t, deep.Suitable(d))
	d["core"].(map[string]interface{})["heat_flux_w_m2"] = 2.0
	assert.True(t, deep.Suitable(d))
}

// Пригодность: раса без heat_flux/gravity окон — оси не влияют.
func TestSuitableNoOptionalWindows(t *testing.T) {
	r := &Race{
		ID:   "test",
		Name: "Тест",
		Conditions: Conditions{
			Temperature: Window{Opt: Range{Lo: 100, Hi: f(200)}, Surv: Range{Lo: 80, Hi: f(250)}},
			Pressure:    Window{Opt: Range{Lo: 1, Hi: f(10)}, Surv: Range{Lo: 0.5, Hi: f(50)}},
			Radiation:   Window{Opt: Range{Lo: 0, Hi: f(30)}, Surv: Range{Lo: 0, Hi: f(60)}},
		},
	}
	d := planetData()
	d["temperature"] = 150.0
	d["gravity"] = 100.0 // не влияет — окна нет
	d["core"].(map[string]interface{})["heat_flux_w_m2"] = 1e9
	assert.True(t, r.Suitable(d))
}

// Каталог: RaceSuitable на реальных карточках — люди подходят землеподобной.
func TestSuitableHumansFromCatalog(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	h := ByID("humans")
	require.NotNil(t, h)
	assert.True(t, h.Suitable(planetData()))
}