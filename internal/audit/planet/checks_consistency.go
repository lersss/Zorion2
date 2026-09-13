// internal/audit/planet/checks_consistency.go
package planet

import (
	"fmt"
	"math"

	"zorion/internal/audit"
)

// ==================== ЖИЗНЬ И ОБИТАЕМОСТЬ ====================

// checkLifeWithoutWater — жизнь без воды.
func checkLifeWithoutWater(v *View) []audit.Issue {
	if !v.Life || v.WaterPercent >= 10 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "life_without_water", audit.SeverityHigh,
		fmt.Sprintf("Жизнь при water=%.1f%% (минимум 10%%)", v.WaterPercent),
		map[string]interface{}{"water": v.WaterPercent})}
}

// checkLifeWithoutTemperature — жизнь при экстремальной температуре.
func checkLifeWithoutTemperature(v *View) []audit.Issue {
	if !v.Life {
		return nil
	}
	if v.Temperature < 200 || v.Temperature > 400 {
		return []audit.Issue{newIssueWithDetails(v, "life_without_temperature", audit.SeverityHigh,
			fmt.Sprintf("Жизнь при температуре %.0f K (допустимо 200–400 K)", v.Temperature),
			map[string]interface{}{"temperature": v.Temperature})}
	}
	return nil
}

// ==================== TYPE ↔ SURFACE_DOMINANT ====================

// checkEarthlikeConsistency — землеподобная должна иметь горы и воду.
func checkEarthlikeConsistency(v *View) []audit.Issue {
	if v.Type != "землеподобная" {
		return nil
	}
	var issues []audit.Issue
	if v.Surface["горы"] < 1 {
		issues = append(issues, newIssue(v, "earthlike_without_rocks", audit.SeverityMedium,
			"Землеподобная без скал в композиции"))
	}
	if v.Surface["океаны"] < 1 && v.Surface["озёра_реки"] < 1 {
		issues = append(issues, newIssue(v, "earthlike_without_water", audit.SeverityMedium,
			"Землеподобная без океанов и озёр"))
	}
	return issues
}

// checkOceanicConsistency — океаническая должна иметь океаны и воду > 60%.
func checkOceanicConsistency(v *View) []audit.Issue {
	if v.Type != "океаническая" {
		return nil
	}
	var issues []audit.Issue
	if v.Surface["океаны"] < 1 {
		issues = append(issues, newIssue(v, "oceanic_without_oceans", audit.SeverityHigh,
			"Океаническая без океанов в композиции"))
	}
	if v.WaterPercent < 60 {
		issues = append(issues, newIssueWithDetails(v, "oceanic_low_water", audit.SeverityMedium,
			fmt.Sprintf("Океаническая при water=%.1f%% (ожидается > 60%%)", v.WaterPercent),
			map[string]interface{}{"water": v.WaterPercent}))
	}
	return issues
}

// checkIceConsistency — ледяная должна быть холодной или иметь ледники.
func checkIceConsistency(v *View) []audit.Issue {
	if v.Type != "ледяная" {
		return nil
	}
	hasGlaciers := v.Surface["ледники"] >= 1
	isCold := v.Temperature < 250
	if !hasGlaciers && !isCold {
		return []audit.Issue{newIssueWithDetails(v, "ice_not_cold", audit.SeverityHigh,
			fmt.Sprintf("Ледяная без ледников при температуре %.0f K", v.Temperature),
			map[string]interface{}{"temperature": v.Temperature})}
	}
	return nil
}

// checkGasGiantConsistency — газовый гигант должен иметь is_gas_giant=true.
func checkGasGiantConsistency(v *View) []audit.Issue {
	if v.Type != "газовый гигант" {
		return nil
	}
	if !v.IsGasGiant {
		return []audit.Issue{newIssue(v, "gas_giant_flag_mismatch", audit.SeverityHigh,
			"Тип = газовый гигант, но is_gas_giant=false")}
	}
	return nil
}

// checkVolcanicConsistency — вулканическая должна иметь лаву или вулканы.
func checkVolcanicConsistency(v *View) []audit.Issue {
	if v.Type != "вулканическая" {
		return nil
	}
	if v.Surface["лавовые_поля"] < 1 && v.Surface["вулканические_поля"] < 1 {
		return []audit.Issue{newIssue(v, "volcanic_without_lava", audit.SeverityMedium,
			"Вулканическая без лавовых или вулканических полей")}
	}
	return nil
}

// checkDesertConsistency — пустынная должна иметь пески.
func checkDesertConsistency(v *View) []audit.Issue {
	if v.Type != "пустынная" {
		return nil
	}
	if v.Surface["пески_пустыни"] < 1 {
		return []audit.Issue{newIssue(v, "desert_without_sands", audit.SeverityMedium,
			"Пустынная без песков в композиции")}
	}
	return nil
}

// checkRadioactiveConsistency — радиоактивная должна иметь флаг.
func checkRadioactiveConsistency(v *View) []audit.Issue {
	if v.Type != "радиоактивная" {
		return nil
	}
	if !v.IsRadioactive {
		return []audit.Issue{newIssue(v, "radioactive_flag_mismatch", audit.SeverityMedium,
			"Тип = радиоактивная, но radioactive=false")}
	}
	return nil
}

// ==================== ХИМИЯ ====================

// checkRadioactiveWithoutZones — флаг radioactive без радиоактивных зон.
func checkRadioactiveWithoutZones(v *View) []audit.Issue {
	if !v.IsRadioactive {
		return nil
	}
	if v.Subterrain["радиоактивные_зоны"] < 5 {
		return []audit.Issue{newIssueWithDetails(v, "radioactive_without_zones", audit.SeverityMedium,
			fmt.Sprintf("radioactive=true, но радиоактивных зон в недрах только %.1f%%",
				v.Subterrain["радиоактивные_зоны"]),
			map[string]interface{}{"zones_share": v.Subterrain["радиоактивные_зоны"]})}
	}
	return nil
}

// ==================== NaN / Inf / НУЛИ / ПУСТОТА ====================

// checkNaNValues — NaN или Inf в числовых полях.
func checkNaNValues(v *View) []audit.Issue {
	var issues []audit.Issue
	check := func(name string, val float64) {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			issues = append(issues, newIssueWithDetails(v, "nan_or_inf_value", audit.SeverityHigh,
				fmt.Sprintf("NaN/Inf в поле %s = %v", name, val),
				map[string]interface{}{"field": name, "value": fmt.Sprintf("%v", val)}))
		}
	}
	check("size", v.Size)
	check("mass", v.Mass)
	check("density", v.Density)
	check("temperature", v.Temperature)
	check("water_percent", v.WaterPercent)
	return issues
}

// checkZeroMass — масса = 0 или отрицательная.
func checkZeroMass(v *View) []audit.Issue {
	if v.Mass > 0 {
		return nil
	}
	return []audit.Issue{newIssue(v, "zero_mass", audit.SeverityHigh,
		"Масса планеты = 0 или отрицательная")}
}

// checkZeroSize — размер = 0 или отрицательный.
func checkZeroSize(v *View) []audit.Issue {
	if v.Size > 0 {
		return nil
	}
	return []audit.Issue{newIssue(v, "zero_size", audit.SeverityHigh,
		"Размер планеты = 0 или отрицательный")}
}

// checkZeroDensity — плотность = 0 или отрицательная (кроме газовых гигантов).
func checkZeroDensity(v *View) []audit.Issue {
	if v.Density > 0 || v.IsGasGiant {
		return nil
	}
	return []audit.Issue{newIssue(v, "zero_density", audit.SeverityHigh,
		"Плотность планеты = 0 или отрицательная")}
}

// checkEmptyName — пустое имя планеты.
func checkEmptyName(v *View) []audit.Issue {
	if v.Name != "" {
		return nil
	}
	return []audit.Issue{newIssue(v, "empty_name", audit.SeverityMedium,
		"У планеты нет имени")}
}