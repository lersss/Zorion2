// internal/handlers/admin_stats_utils.go
package handlers

import "zorion/internal/generator/planet"

// extractComposition — вытаскивает map формы → доля из JSON-поля.
func extractComposition(data map[string]interface{}, key string) planet.Composition {
	result := planet.Composition{}
	raw, ok := data[key].(map[string]interface{})
	if !ok {
		return result
	}
	for k, v := range raw {
		if f, ok := v.(float64); ok && f > 0.01 {
			result[k] = f
		}
	}
	return result
}

// buildClassificationInput — собирает вход для ClassifyGameDesignType.
// settleable — планета имеет поселение (пригодность для классификации).
func buildClassificationInput(
	data map[string]interface{},
	surface planet.Composition,
	settleable bool,
) planet.PlanetClassificationInput {
	return planet.PlanetClassificationInput{
		IsGasGiant:    isGasGiant(data),
		IsRadioactive: getBool(data, "radioactive"),
		Surface:       surface,
		Temperature:   getFloat(data, "temperature"),
		WaterPercent:  getFloat(data, "water_percent"),
		Settleable:    settleable,
		Life:          getBool(data, "life"),
	}
}

// isGasGiant — является ли планета газовым гигантом.
func isGasGiant(data map[string]interface{}) bool {
	if v, ok := data["is_gas_giant"].(bool); ok && v {
		return true
	}
	return getString(data, "surface_dominant") == "газовый_гигант"
}

// incrementIfPresent — инкремент значения в map, если ключ не пустой.
func incrementIfPresent(m map[string]int, key string) {
	if key != "" {
		m[key]++
	}
}

// ==================== JSON-ХЕЛПЕРЫ ====================
// ВНИМАНИЕ: если эти функции уже объявлены в других файлах пакета handlers —
// удали их оттуда, оставь только здесь.

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getFloat(data map[string]interface{}, key string) float64 {
	if val, ok := data[key].(float64); ok {
		return val
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return false
}