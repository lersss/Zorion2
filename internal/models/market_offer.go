// internal/models/market_offer.go
// Предложение витрины локального рынка планеты (спека
// 2026-09-24-магазин-модулей-локальный-рынок §5/§7.1): справочник-каталог
// «что продаётся и за сколько», бесконечный запас. kind — открытый список
// разделов (сейчас только 'module'); item_id — полиморфная ссылка без FK
// (для module — equipment.id); item — развёрнутый предмет из каталога
// (EquipmentByID, инвариант каталога §5).
package models

// MarketOffer — строка market_offers в ответе витрины (§7.1).
type MarketOffer struct {
	ID     int64          `json:"id"`
	Kind   string         `json:"kind"`
	ItemID string         `json:"-"`              // полиморфная ссылка (для module — equipment.id)
	Item   *EquipmentItem `json:"item,omitempty"` // развёрнутый предмет; nil — строка пропущена (§5)
	Price  int64          `json:"price"`
}