// internal/models/ship_part.go
// Деталь корабля в каталоге ship_parts (спека 99.2.15 §2).
package models

import "time"

// ShipPart — одна деталь каталога: параметрический SVG-фрагмент в общем
// холсте 200×200 (нос вправо), основная масса — fill="currentColor",
// акценты — фиксированный fill (спека §3.3, инвариант И1).
type ShipPart struct {
	ID        string                 `json:"id"`      // "hull_arrow", ...
	Category  string                 `json:"category"` // hull/nose/wings/engine/tail
	Name      string                 `json:"name"`     // человеческое имя ("Стрела", "Клин")
	SVG       string                 `json:"svg"`      // SVG-фрагмент без <svg>-обёртки
	Params    map[string]interface{} `json:"-"`        // параметры генерации (для перегенерации)
	CreatedAt time.Time              `json:"created_at,omitempty"`
}