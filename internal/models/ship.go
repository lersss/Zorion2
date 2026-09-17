// internal/models/ship.go
// Модель корабля, оборудование и радар (спека 77a §2–§4): справочники
// ship_models/equipment (таблицы), установка на игрока — атрибуты
// users.ship_model_id / users.equipment. Визуал 61b (ship_icon/ship_color)
// остаётся отдельной системой (И3).
package models

import "time"

// ShipModel — модель корабля: рамка механических характеристик (слоты под
// оборудование), НЕ визуал (спека 77a §2).
type ShipModel struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Slots     map[string]interface{} `json:"slots"` // {"radar":1,"scanner":1,"engine":1}
	CreatedAt time.Time              `json:"created_at"`
}

// EquipmentType — тип оборудования (спека 77a §3.2).
type EquipmentType string

const (
	EquipmentTypeRadar   EquipmentType = "radar"
	EquipmentTypeScanner EquipmentType = "scanner"
	EquipmentTypeEngine  EquipmentType = "engine"
)

// EquipmentItem — предмет оборудования из справочника (спека 77a §3.2).
type EquipmentItem struct {
	ID        string                 `json:"id"`
	Type      EquipmentType          `json:"type"`
	Name      string                 `json:"name"`
	Params    map[string]interface{} `json:"params"`
	CreatedAt time.Time              `json:"created_at"`
}

// Радиусы радара (спека 77a §4.2, решения создателя 2026-09-17):
// стартовый радар radar_1 — 800 px (увеличен в 2 раза с 400 px 2026-09-17);
// без радара — минимум 200 px («не слепой»).
// Прогрессия уровней (600/900/1400/2000) — вне скоупа, вернётся с рынками.
const (
	RadarRadiusDefault = 800.0
	RadarRadiusMin     = 200.0
)

// KnowledgeTTL — срок протухания знания о планете (спека 77a §8.2,
// решение создателя 2026-09-17): 7 дней. Актуальность считается на чтении,
// без фоновых джобов; координаты системы не протухают (И10).
const KnowledgeTTL = 7 * 24 * time.Hour

// StarterShipModelID / StarterEquipment — стартовая комплектация (спека 77a
// §3.3): модель 'starter', радар-1 + сканер-1 в слотах, двигатель пуст.
const StarterShipModelID = "starter"

// StarterEquipment — JSON-значение users.equipment для нового игрока.
var StarterEquipment = map[string]interface{}{
	"radar":   "radar_1",
	"scanner": "scanner_1",
	"engine":  nil,
}