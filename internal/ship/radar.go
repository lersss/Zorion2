// internal/ship/radar.go
// Радиус видимости игрока (спека 77a §4.2): определяется установленным
// радаром — equipment.params.radius; без радара — минимум 200 px («не слепой»).
// Спека магазина §9 (И-М5): радар ищется по ВСЕМ значениям equipment (любой
// слот), радиус — максимум из установленных радаров (несколько не суммируются —
// радиус свойство корабля). Слот engine на радиус не влияет.
package ship

import (
	"zorion/internal/models"
)

// RadarRadius — радиус видимости игрока по установленному оборудованию.
// equipment — users.equipment (JSONB): {"radar":"radar_1","scanner":"scanner_1","engine":null}.
// Радар читается по типу из ЛЮБОГО слота; берётся максимум радиуса. Неизвестный/
// битый радар — минимум 200 px (безопасный фолбэк: не «слепит», но и не
// расширяет видимость сверх справочника).
func RadarRadius(userEquipment map[string]interface{}) float64 {
	best := 0.0
	for _, v := range userEquipment {
		id, ok := v.(string)
		if !ok || id == "" {
			continue
		}
		it := EquipmentByID(id)
		if it == nil || it.Type != models.EquipmentTypeRadar {
			continue
		}
		if r, ok := it.Params["radius"].(float64); ok && r > best {
			best = r
		}
	}
	if best <= 0 {
		return models.RadarRadiusMin
	}
	return best
}