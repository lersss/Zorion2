// internal/generator/planet/surface_color.go
//
// Контракт «биом → цвет» для пакета прогулки (спека 2026-09-21 §7.1/§7.2,
// И9): палитра прогулки берётся из того же контракта, что картинка планеты с
// орбиты (спека визуализации 2026-09-20 §4.2), не изобретается заново.
package planet

import "fmt"

// SurfaceBiomeColorHex — цвет биома прогулки в виде '#RRGGBB': color каталога
// переопределяет базу категории; иначе база категории + сдвиги (температура
// планеты; жидкость — из определения биома). Неизвестный id → фолбэк по
// категории/имени, иначе нейтральный (не падать, спека §4.2).
func SurfaceBiomeColorHex(biomeID string, temperature float64) string {
	c := biomeColorFor(biomeID, PlanetImageInput{Temperature: temperature})
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
