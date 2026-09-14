// internal/audit/planet/checks_physics.go
package planet

import (
	"fmt"
	"math"

	"zorion/internal/audit"
	genplanet "zorion/internal/generator/planet"
)

// ==================== ФИЗИКА: БАЗОВЫЕ ДИАПАЗОНЫ ====================

// checkTemperatureRange — температура в допустимых границах.
func checkTemperatureRange(v *View) []audit.Issue {
	if v.Temperature < 20 {
		return []audit.Issue{newIssue(v, "temp_too_low", audit.SeverityHigh,
			fmt.Sprintf("Температура %.1f K ниже абсолютного минимума (20 K)", v.Temperature))}
	}
	if v.Temperature > 2500 {
		return []audit.Issue{newIssue(v, "temp_too_high", audit.SeverityHigh,
			fmt.Sprintf("Температура %.1f K выше абсолютного максимума (2500 K)", v.Temperature))}
	}
	return nil
}

// checkMassSizeDensity — согласованность массы, размера, плотности.
//
// Для газовых гигантов — инвариант кривой M→R (gasGiantRadius): размер обязан
// лежать на кривой с насыщением (наивная формула (M/ρ)^(1/3) для гигантов
// отклонена, 99.2.15). Для обычных планет — как раньше: (M/ρ)^(1/3).
func checkMassSizeDensity(v *View) []audit.Issue {
	if v.Density <= 0 || v.Mass <= 0 {
		return nil
	}
	if v.IsGasGiant {
		expected := genplanet.GasGiantRadius(v.Mass)
		diff := math.Abs(expected - v.Size)
		if diff > 0.2 {
			return []audit.Issue{newIssueWithDetails(v, "giant_radius_mismatch", audit.SeverityMedium,
				fmt.Sprintf("R=%.3f, но кривая гиганта gasGiantRadius(M)=%.3f (отклонение %.3f)", v.Size, expected, diff),
				map[string]interface{}{
					"size":     v.Size,
					"mass":     v.Mass,
					"expected": expected,
				})}
		}
		return nil
	}
	expected := math.Cbrt(v.Mass / v.Density)
	diff := math.Abs(expected - v.Size)
	if diff > 0.05 {
		return []audit.Issue{newIssueWithDetails(v, "mass_size_density_mismatch", audit.SeverityMedium,
			fmt.Sprintf("R=%.3f, но (M/ρ)^(1/3)=%.3f (отклонение %.3f)", v.Size, expected, diff),
			map[string]interface{}{
				"size":     v.Size,
				"mass":     v.Mass,
				"density":  v.Density,
				"expected": expected,
			})}
	}
	return nil
}

// checkSurfaceSum — сумма композиции поверхности = 100.
func checkSurfaceSum(v *View) []audit.Issue {
	if len(v.Surface) == 0 {
		return nil
	}
	sum := sumComposition(v.Surface)
	if math.Abs(sum-100) > 0.5 {
		return []audit.Issue{newIssueWithDetails(v, "surface_sum_not_100", audit.SeverityHigh,
			fmt.Sprintf("Сумма форм поверхности = %.2f (ожидалось 100)", sum),
			map[string]interface{}{"sum": sum})}
	}
	return nil
}

// checkSubterrainSum — сумма композиции недр = 100.
func checkSubterrainSum(v *View) []audit.Issue {
	if len(v.Subterrain) == 0 {
		return nil
	}
	sum := sumComposition(v.Subterrain)
	if math.Abs(sum-100) > 0.5 {
		return []audit.Issue{newIssueWithDetails(v, "subterrain_sum_not_100", audit.SeverityHigh,
			fmt.Sprintf("Сумма типов недр = %.2f (ожидалось 100)", sum),
			map[string]interface{}{"sum": sum})}
	}
	return nil
}

// checkNegativeShares — отрицательные проценты в композициях.
func checkNegativeShares(v *View) []audit.Issue {
	var issues []audit.Issue
	for form, share := range v.Surface {
		if share < 0 {
			issues = append(issues, newIssueWithDetails(v, "negative_surface_share", audit.SeverityHigh,
				fmt.Sprintf("Отрицательная доля в поверхности: %s = %.2f", form, share),
				map[string]interface{}{"form": form, "share": share}))
		}
	}
	for t, share := range v.Subterrain {
		if share < 0 {
			issues = append(issues, newIssueWithDetails(v, "negative_subterrain_share", audit.SeverityHigh,
				fmt.Sprintf("Отрицательная доля в недрах: %s = %.2f", t, share),
				map[string]interface{}{"type": t, "share": share}))
		}
	}
	return issues
}

// ==================== КОМПОЗИЦИЯ vs ТЕМПЕРАТУРА ====================

// checkFormMinTemp — универсальная проверка «форма при слишком низкой T».
//
// Severity задаётся снаружи, потому что для разных форм оно разное:
//   - биосферные формы (луга, леса, болота) — SeverityLow (аномалия);
//   - лава, океаны и т.п. — SeverityHigh (физически невозможно).
func checkFormMinTemp(v *View, form string, minTemp float64, severity string) []audit.Issue {
	share := v.Surface[form]
	if share < 1 {
		return nil
	}
	if v.Temperature >= minTemp {
		return nil
	}
	code := formCode(form) + "_in_cold"
	return []audit.Issue{newIssueWithDetails(v, code, severity,
		fmt.Sprintf("%s %.1f%% при температуре %.0f K (минимум %.0f K)", form, share, v.Temperature, minTemp),
		map[string]interface{}{"form": form, "share": share, "temperature": v.Temperature, "min_temp": minTemp})}
}

// formCode — английский код для формы поверхности (для кодов аудита).
var formCodes = map[string]string{
	"джунгли":         "jungles",
	"леса":            "forests",
	"луга_степи":      "meadows",
	"болота":          "swamps",
	"коралловые_рифы": "coral_reefs",
	"ледники":         "glaciers",
	"мёрзлые_газы":    "frozen_gases",
	"лавовые_поля":    "lava",
	"океаны":          "oceans",
}

func formCode(form string) string {
	if code, ok := formCodes[form]; ok {
		return code
	}
	return form
}

// --- Биосферные формы в холоде: низкая критичность (аномалия) ---

// checkJunglesInCold — джунгли при низкой температуре.
func checkJunglesInCold(v *View) []audit.Issue {
	return checkFormMinTemp(v, "джунгли", 280, audit.SeverityLow)
}

// checkForestsInCold — леса при низкой температуре.
func checkForestsInCold(v *View) []audit.Issue {
	return checkFormMinTemp(v, "леса", 250, audit.SeverityLow)
}

// checkMeadowsInCold — луга/степи при низкой температуре.
func checkMeadowsInCold(v *View) []audit.Issue {
	return checkFormMinTemp(v, "луга_степи", 250, audit.SeverityLow)
}

// checkSwampsInCold — болота при низкой температуре.
func checkSwampsInCold(v *View) []audit.Issue {
	return checkFormMinTemp(v, "болота", 260, audit.SeverityLow)
}

// checkCoralReefsInCold — коралловые рифы при низкой температуре.
func checkCoralReefsInCold(v *View) []audit.Issue {
	return checkFormMinTemp(v, "коралловые_рифы", 275, audit.SeverityLow)
}

// --- Прочие формы: средняя/высокая критичность ---

// checkGlaciersInHeat — ледники при высокой температуре.
func checkGlaciersInHeat(v *View) []audit.Issue {
	share := v.Surface["ледники"]
	if share < 1 || v.Temperature <= 300 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "glaciers_in_heat", audit.SeverityMedium,
		fmt.Sprintf("Ледники %.1f%% при температуре %.0f K (максимум 300 K)", share, v.Temperature),
		map[string]interface{}{"share": share, "temperature": v.Temperature})}
}

// checkLavaInCold — лавовые поля при низкой температуре.
func checkLavaInCold(v *View) []audit.Issue {
	share := v.Surface["лавовые_поля"]
	if share < 1 || v.Temperature >= 500 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "lava_in_cold", audit.SeverityHigh,
		fmt.Sprintf("Лавовые поля %.1f%% при температуре %.0f K (минимум 500 K)", share, v.Temperature),
		map[string]interface{}{"share": share, "temperature": v.Temperature})}
}

// checkFrozenGasesInHeat — мёрзлые газы при высокой температуре.
func checkFrozenGasesInHeat(v *View) []audit.Issue {
	share := v.Surface["мёрзлые_газы"]
	if share < 1 || v.Temperature <= 250 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "frozen_gases_in_heat", audit.SeverityMedium,
		fmt.Sprintf("Мёрзлые газы %.1f%% при температуре %.0f K (максимум 250 K)", share, v.Temperature),
		map[string]interface{}{"share": share, "temperature": v.Temperature})}
}

// checkOceansInHeat — океаны при температуре выше точки кипения.
//
// Срабатывает только на глобальные океаны (доля > 1% поверхности): локальный
// оазис на ≤1% объясним и «невозможным» не считается.
func checkOceansInHeat(v *View) []audit.Issue {
	share := v.Surface["океаны"]
	if share <= 1 || v.Temperature <= 400 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "oceans_in_heat", audit.SeverityHigh,
		fmt.Sprintf("Океаны %.1f%% при температуре %.0f K (испарятся выше 373 K)", share, v.Temperature),
		map[string]interface{}{"share": share, "temperature": v.Temperature})}
}

// ==================== КОМПОЗИЦИЯ vs ВОДА ====================

// checkOceansWithoutWater — океаны при отсутствии воды.
func checkOceansWithoutWater(v *View) []audit.Issue {
	share := v.Surface["океаны"]
	if share < 1 || v.WaterPercent >= 5 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "oceans_without_water", audit.SeverityHigh,
		fmt.Sprintf("Океаны %.1f%% при water=%.1f%% (минимум 5%%)", share, v.WaterPercent),
		map[string]interface{}{"share": share, "water": v.WaterPercent})}
}

// checkBiosphereWithoutWater — биосферные формы при малом количестве воды.
//
// Пониженная критичность: возможно, это локальные оазисы.
func checkBiosphereWithoutWater(v *View) []audit.Issue {
	if v.WaterPercent >= 10 {
		return nil
	}
	biosphereForms := []string{"леса", "луга_степи", "джунгли", "болота", "коралловые_рифы"}
	var issues []audit.Issue
	for _, form := range biosphereForms {
		share := v.Surface[form]
		if share >= 1 {
			issues = append(issues, newIssueWithDetails(v, "biosphere_without_water", audit.SeverityLow,
				fmt.Sprintf("%s %.1f%% при water=%.1f%% (минимум 10%%)", form, share, v.WaterPercent),
				map[string]interface{}{"form": form, "share": share, "water": v.WaterPercent}))
		}
	}
	return issues
}

// ==================== АТМОСФЕРА vs ТЕМПЕРАТУРА ====================

// checkGreenhouseInCold — парниковая атмосфера при низкой температуре.
func checkGreenhouseInCold(v *View) []audit.Issue {
	if v.Atmosphere != "парниковая" || v.Temperature >= 250 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "greenhouse_in_cold", audit.SeverityHigh,
		fmt.Sprintf("Парниковая атмосфера при температуре %.0f K (CO₂ замёрзнет ниже 250 K)", v.Temperature),
		map[string]interface{}{"temperature": v.Temperature})}
}

// checkOxygenInHeat — азотно-кислородная атмосфера при высокой температуре.
func checkOxygenInHeat(v *View) []audit.Issue {
	if v.Atmosphere != "азотно-кислородная" || v.Temperature <= 500 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "oxygen_in_heat", audit.SeverityMedium,
		fmt.Sprintf("Азотно-кислородная атмосфера при температуре %.0f K (улетучится)", v.Temperature),
		map[string]interface{}{"temperature": v.Temperature})}
}

// checkMethaneInHeat — метановая атмосфера при высокой температуре.
func checkMethaneInHeat(v *View) []audit.Issue {
	if v.Atmosphere != "метановая" || v.Temperature <= 300 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "methane_in_heat", audit.SeverityMedium,
		fmt.Sprintf("Метановая атмосфера при температуре %.0f K (метан разложится)", v.Temperature),
		map[string]interface{}{"temperature": v.Temperature})}
}

// checkHydrogenInHeat — водородно-гелиевая, водородная, гелиевая при высокой T.
//
// Газовые гиганты исключены: у них H-He атмосфера по определению,
// и даже горячие газовые гиганты — норма.
func checkHydrogenInHeat(v *View) []audit.Issue {
	if v.IsGasGiant {
		return nil
	}
	if v.Temperature <= 700 {
		return nil
	}
	switch v.Atmosphere {
	case "водородно-гелиевая", "водородная", "гелиевая":
		return []audit.Issue{newIssueWithDetails(v, "hydrogen_in_heat", audit.SeverityMedium,
			fmt.Sprintf("Водородная атмосфера при температуре %.0f K (водород улетучится)", v.Temperature),
			map[string]interface{}{"atmosphere": v.Atmosphere, "temperature": v.Temperature})}
	}
	return nil
}

// ==================== ЯДРО ====================

// checkCoreRanges — активность/радиоактивность/масса ядра в [0, 100].
func checkCoreRanges(v *View) []audit.Issue {
	if v.Core == nil {
		return nil
	}
	var issues []audit.Issue
	c := v.Core
	if c.Activity < 0 || c.Activity > 100 {
		issues = append(issues, newIssueWithDetails(v, "core_activity_out_of_range", audit.SeverityHigh,
			fmt.Sprintf("Активность ядра %.2f вне [0, 100]", c.Activity),
			map[string]interface{}{"activity": c.Activity}))
	}
	if c.Radioactivity < 0 || c.Radioactivity > 100 {
		issues = append(issues, newIssueWithDetails(v, "core_radioactivity_out_of_range", audit.SeverityHigh,
			fmt.Sprintf("Радиоактивность ядра %.2f вне [0, 100]", c.Radioactivity),
			map[string]interface{}{"radioactivity": c.Radioactivity}))
	}
	if c.MassPercent < 0 || c.MassPercent > 100 {
		issues = append(issues, newIssueWithDetails(v, "core_mass_percent_out_of_range", audit.SeverityHigh,
			fmt.Sprintf("Доля ядра %.2f%% вне [0, 100]", c.MassPercent),
			map[string]interface{}{"mass_percent": c.MassPercent}))
	}
	return issues
}

// checkCoreAge — возраст ядра не превышает возраст вселенной.
func checkCoreAge(v *View) []audit.Issue {
	if v.Core == nil || v.Core.Age <= 13.8 {
		return nil
	}
	return []audit.Issue{newIssueWithDetails(v, "core_age_too_high", audit.SeverityHigh,
		fmt.Sprintf("Возраст ядра %.2f млрд лет > возраст вселенной (13.8)", v.Core.Age),
		map[string]interface{}{"age": v.Core.Age})}
}

// checkCoreTypeMismatch — флаг is_metallic не совпадает с типом.
func checkCoreTypeMismatch(v *View) []audit.Issue {
	if v.Core == nil {
		return nil
	}
	if v.Core.IsMetallic && v.Core.Type != "металлическое" {
		return []audit.Issue{newIssueWithDetails(v, "core_metallic_flag_mismatch", audit.SeverityMedium,
			fmt.Sprintf("is_metallic=true, но type=%s", v.Core.Type),
			map[string]interface{}{"is_metallic": true, "type": v.Core.Type})}
	}
	if !v.Core.IsMetallic && v.Core.Type == "металлическое" {
		return []audit.Issue{newIssueWithDetails(v, "core_metallic_flag_mismatch", audit.SeverityMedium,
			"is_metallic=false, но type=металлическое",
			map[string]interface{}{"is_metallic": false, "type": v.Core.Type})}
	}
	return nil
}

// ==================== СПУТНИКИ ====================

// checkSatelliteTemperature — спутник горячее газового гиганта.
func checkSatelliteTemperature(v *View) []audit.Issue {
	if !v.IsGasGiant || len(v.Satellites) == 0 {
		return nil
	}
	var issues []audit.Issue
	for _, s := range v.Satellites {
		if s.Temperature > v.Temperature+50 {
			issues = append(issues, newIssueWithDetails(v, "satellite_hotter_than_giant", audit.SeverityMedium,
				fmt.Sprintf("Спутник %s (%.0f K) горячее газового гиганта (%.0f K)", s.Name, s.Temperature, v.Temperature),
				map[string]interface{}{"satellite_name": s.Name, "satellite_temp": s.Temperature, "giant_temp": v.Temperature}))
		}
	}
	return issues
}

// checkSatelliteMass — масса спутника слишком большая.
func checkSatelliteMass(v *View) []audit.Issue {
	if !v.IsGasGiant || len(v.Satellites) == 0 {
		return nil
	}
	var issues []audit.Issue
	for _, s := range v.Satellites {
		if s.Mass > 5 {
			issues = append(issues, newIssueWithDetails(v, "satellite_mass_too_high", audit.SeverityLow,
				fmt.Sprintf("Спутник %s имеет массу %.2f M⊕ (слишком крупный)", s.Name, s.Mass),
				map[string]interface{}{"satellite_name": s.Name, "mass": s.Mass}))
		}
	}
	return issues
}

// ==================== ХЕЛПЕРЫ ====================

// sumComposition — сумма всех значений в map.
func sumComposition(c map[string]float64) float64 {
	sum := 0.0
	for _, v := range c {
		sum += v
	}
	return sum
}