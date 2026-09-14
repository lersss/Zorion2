// internal/generator/ship/assembly.go
// Детерминированная сборка схемы корабля от seed (спека §7.1, инвариант И3):
// SHA-256 seed → цвет из палитры + индекс детали по каждой категории
// (всегда 5 деталей, И2). Чистая функция: ноль хранилища, дешёво при 100k
// агентов (И7). Используется для NPC-агентов (seed = id агента) и дефолта
// игрока (seed = id ⊕ legacy ship_icon, спека §8).
package ship

import (
	"crypto/sha256"

	"zorion/internal/models"
)

// AssemblyFromSeed — схема корабля от seed. byCategory: category → детали
// каталога (отсортированные). Пустая категория → пустой id детали: клиент
// рисует фолбэк-примитив (И4, каталог пуст → примитив, не падение).
func AssemblyFromSeed(seed []byte, palette []string, layerOrder []string, byCategory map[string][]models.ShipPart) models.ShipVisual {
	h := sha256.Sum256(seed)
	color := ""
	if len(palette) > 0 {
		color = palette[int(h[0])%len(palette)]
	}
	parts := make(map[string]string, len(layerOrder))
	for i, cat := range layerOrder {
		list := byCategory[cat]
		if len(list) == 0 {
			parts[cat] = ""
			continue
		}
		parts[cat] = list[int(h[i+1])%len(list)].ID
	}
	return models.ShipVisual{Color: color, Parts: parts}
}

// DefaultSeed — seed дефолтной сборки игрока: id ⊕ legacy ship_icon
// (спека §8, мост): старый выбор не выбрасывается молча, а «переводится»
// в схему.
func DefaultSeed(userID, legacyIcon string) []byte {
	return []byte(userID + "|" + legacyIcon)
}