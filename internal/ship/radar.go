// internal/ship/radar.go
// Радиус видимости игрока (спека 77a §4.2): определяется ТОЛЬКО установленным
// радаром (И4) — users.equipment.radar → equipment.params.radius; без радара —
// минимум 200 px («не слепой»). Слот engine на радиус не влияет.
package ship

import (
	"zorion/internal/models"
)

// RadarRadius — радиус видимости игрока по установленному оборудованию.
// equipment — users.equipment (JSONB): {"radar":"radar_1","scanner":"scanner_1","engine":null}.
// Неизвестный/битый радар — минимум 200 px (безопасный фолбэк: не «слепит»,
// но и не расширяет видимость сверх справочника).
func RadarRadius(userEquipment map[string]interface{}) float64 {
	if userEquipment == nil {
		return models.RadarRadiusMin
	}
	radarID, _ := userEquipment["radar"].(string)
	if radarID == "" {
		return models.RadarRadiusMin
	}
	it := EquipmentByID(radarID)
	if it == nil || it.Type != models.EquipmentTypeRadar {
		return models.RadarRadiusMin
	}
	if r, ok := it.Params["radius"].(float64); ok && r > 0 {
		return r
	}
	return models.RadarRadiusMin
}