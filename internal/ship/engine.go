// internal/ship/engine.go
// Скорость полёта игрока (спека 91a §7.1): определяется ТОЛЬКО установленным
// двигателем — users.equipment.engine → equipment.params.speed_factor (0.3,
// константа 66a). Без двигателя полёт запрещён (спека 91a §6.1) — EngineSpeed
// вызывается после проверки HasEngine; битый params у валидного двигателя —
// безопасное чтение 0.3 (значение по умолчанию).
package ship

import "zorion/internal/models"

// EngineSpeed — скорость полёта (сек/px) по установленному двигателю.
// equipment — users.equipment (JSONB): {"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}.
// Неизвестный/битый двигатель — 0.3 (безопасное чтение, значение 66a);
// без двигателя полёт невозможен — запрет на стороне /travel (HasEngine).
func EngineSpeed(userEquipment map[string]interface{}) float64 {
	if userEquipment == nil {
		return models.EngineSpeedDefault
	}
	engineID, _ := userEquipment["engine"].(string)
	if engineID == "" {
		return models.EngineSpeedDefault
	}
	it := EquipmentByID(engineID)
	if it == nil || it.Type != models.EquipmentTypeEngine {
		return models.EngineSpeedDefault
	}
	if s, ok := it.Params["speed_factor"].(float64); ok && s > 0 {
		return s
	}
	return models.EngineSpeedDefault
}