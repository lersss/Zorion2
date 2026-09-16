// internal/audit/planet/checks_cascade_test.go
//
// Тесты правил физического каскада (99.2.20 §6.2): atmosphere_sum_not_100
// и liquid_water_mismatch. Аддитивные: пропускают планеты без новых полей
// (старые, сгенерированные до 45a).
package planet

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"zorion/internal/audit"
)

// TestCheckAtmosphereSumNot100 — сумма состава атмосферы-объекта ≠ 100
// (допуск 0.5) → High; сумма 100 → чисто; без atmosphere_data → пропуск.
func TestCheckAtmosphereSumNot100(t *testing.T) {
	// Сумма 80 → High.
	d := baseData()
	d["atmosphere_data"] = map[string]interface{}{
		"composition": map[string]interface{}{"N2": 50.0, "O2": 30.0},
		"pressure_atm": 1.0,
	}
	assertSingle(t, runCheck(checkAtmosphereSumNot100, d), "atmosphere_sum_not_100", audit.SeverityHigh)

	// Сумма 100 → чисто.
	d["atmosphere_data"].(map[string]interface{})["composition"] = map[string]interface{}{
		"N2": 78.0, "O2": 21.0, "CO2": 1.0,
	}
	assert.Empty(t, runCheck(checkAtmosphereSumNot100, d))

	// Без atmosphere_data (старая планета) → пропуск.
	delete(d, "atmosphere_data")
	assert.Empty(t, runCheck(checkAtmosphereSumNot100, d))
}

// TestCheckLiquidWaterMismatch — флаг true, но T ∉ (273, T_boil(P)) или
// P < 0.006 → High; флаг true и T в диапазоне → чисто; без флага → пропуск.
func TestCheckLiquidWaterMismatch(t *testing.T) {
	// Флаг true, T = 500 > T_boil(1) = 373 → High.
	d := baseData()
	d["liquid_water_possible"] = true
	d["temperature"] = 500.0
	d["atmosphere_data"] = map[string]interface{}{"pressure_atm": 1.0}
	assertSingle(t, runCheck(checkLiquidWaterMismatch, d), "liquid_water_mismatch", audit.SeverityHigh)

	// Флаг true, T = 300 ∈ (273, 373) → чисто.
	d["temperature"] = 300.0
	assert.Empty(t, runCheck(checkLiquidWaterMismatch, d))

	// Флаг true, P = 0.001 < 0.006 → High.
	d["temperature"] = 300.0
	d["atmosphere_data"] = map[string]interface{}{"pressure_atm": 0.001}
	assertSingle(t, runCheck(checkLiquidWaterMismatch, d), "liquid_water_mismatch", audit.SeverityHigh)

	// Флаг false → пропуск (правило проверяет только true).
	d["liquid_water_possible"] = false
	d["atmosphere_data"] = map[string]interface{}{"pressure_atm": 1.0}
	assert.Empty(t, runCheck(checkLiquidWaterMismatch, d))

	// Без флага (старая планета) → пропуск.
	delete(d, "liquid_water_possible")
	assert.Empty(t, runCheck(checkLiquidWaterMismatch, d))
}