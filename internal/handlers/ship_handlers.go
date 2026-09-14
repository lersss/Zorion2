// internal/handlers/ship_handlers.go
// Каталог деталей кораблей для клиента (спека 99.2.15 §5.1): GET /api/ship-parts
// (игровой JWT) отдаёт каталог из памяти (immutable snapshot + atomic.Pointer,
// O(1), И7 — БД не в хот-пате рендера). Клиент получает детали (id, category,
// name, svg) + палитру и порядок слоёв, чтобы собирать схему самостоятельно
// (assemblyFromSeed, §7.1).
package handlers

import (
	"net/http"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// ShipHandlers — ручки каталога кораблей для игрового клиента.
type ShipHandlers struct {
	catalog *repository.ShipCatalog
}

func NewShipHandlers(catalog *repository.ShipCatalog) *ShipHandlers {
	return &ShipHandlers{catalog: catalog}
}

// shipPartDTO — деталь для клиента: без параметров генерации и метаданных
// (спека §5.1: id, category, name, svg-фрагмент).
type shipPartDTO struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Name     string `json:"name"`
	SVG      string `json:"svg"`
}

// GetCatalog — GET /api/ship-parts: каталог из памяти + палитра + порядок
// слоёв. Пустой каталог — пустой список: клиент рисует фолбэк-примитив
// (И4, «не падать при пустом каталоге»).
func (h *ShipHandlers) GetCatalog(w http.ResponseWriter, r *http.Request) {
	snap := h.catalog.Snapshot()
	parts := snap.Parts()
	if parts == nil {
		parts = []models.ShipPart{}
	}
	dto := make([]shipPartDTO, 0, len(parts))
	for _, p := range parts {
		dto = append(dto, shipPartDTO{ID: p.ID, Category: p.Category, Name: p.Name, SVG: p.SVG})
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"parts":      dto,
		"palette":    snap.Palette(),
		"layerOrder": snap.LayerOrder(),
	})
}