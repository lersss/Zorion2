// internal/audit/planet/checks_cascade.go
//
// Правила физического каскада (99.2.20 §6.2): атмосфера-объект и флаг
// жидкой воды. Аддитивные: пропускают планеты без новых полей (старые,
// сгенерированные до 45a — фолбэки §7).
package planet

import (
	"fmt"
	"math"

	"zorion/internal/audit"
)

// checkAtmosphereSumNot100 — сумма состава атмосферы-объекта ≠ 100
// (допуск 0.5). Пропускает планеты без atmosphere_data (старые).
func checkAtmosphereSumNot100(v *View) []audit.Issue {
	raw, ok := v.Raw["atmosphere_data"].(map[string]interface{})
	if !ok {
		return nil
	}
	comp, ok := raw["composition"].(map[string]interface{})
	if !ok {
		return nil
	}
	sum := 0.0
	for _, val := range comp {
		if f, ok := val.(float64); ok {
			sum += f
		}
	}
	if math.Abs(sum-100) > 0.5 {
		return []audit.Issue{newIssueWithDetails(v, "atmosphere_sum_not_100", audit.SeverityHigh,
			fmt.Sprintf("Сумма состава атмосферы = %.2f (ожидалось 100)", sum),
			map[string]interface{}{"sum": sum})}
	}
	return nil
}

// checkLiquidWaterMismatch — инвариант флага жидкой воды (99.2.20 §3.7):
// флаг true ⟺ P ≥ 0.006 ∧ 273 < T < 373 + 30.2·ln(P). High при флаге true,
// но T ∉ (273, T_boil(P)) или P < 0.006. Пропускает планеты без флага (старые).
func checkLiquidWaterMismatch(v *View) []audit.Issue {
	flag, ok := v.Raw["liquid_water_possible"].(bool)
	if !ok || !flag {
		return nil
	}
	raw, ok := v.Raw["atmosphere_data"].(map[string]interface{})
	if !ok {
		return nil
	}
	p, ok := raw["pressure_atm"].(float64)
	if !ok {
		return nil
	}
	if p < 0.006 || v.Temperature <= 273 || v.Temperature >= 373+30.2*math.Log(p) {
		return []audit.Issue{newIssueWithDetails(v, "liquid_water_mismatch", audit.SeverityHigh,
			fmt.Sprintf("Флаг жидкой воды true, но T=%.1f K, P=%.4f атм — инвариант нарушен", v.Temperature, p),
			map[string]interface{}{"temperature": v.Temperature, "pressure_atm": p})}
	}
	return nil
}