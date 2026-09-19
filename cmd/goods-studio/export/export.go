// Package export — сборка export.json для игры (спека 99a.1 §11):
// только согласованные товары, complexity = эффективный тир (99a.3 §12:
// override ?? вычисленный), quantity = количество слота (≥ 1, 99a.2 §6.3),
// ресурсы — только те, на которые ссылаются выгруженные рецепты.
package export

import (
	"time"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
	"zorion/internal/goodsstudio/validate"
)

// Slot — слот выгрузки (quantity = количество слота, ≥ 1; 99a.2 §6.3).
type Slot struct {
	GoodID   string `json:"good_id"`
	Quantity int    `json:"quantity"`
}

// Good — товар выгрузки.
type Good struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Complexity int    `json:"complexity"`
	Recipe     []Slot `json:"recipe"`
}

// Resource — ресурс, на который ссылаются выгруженные рецепты, с
// источником каталога (layer/real + catalog_id) — мост резолвит их
// в ресурсную модель игры.
type Resource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Catalog   string `json:"catalog"`
	CatalogID string `json:"catalog_id"`
}

// Export — формат выгрузки (спека 99a.1 §11). Расширяем: окна свойств
// добавятся полем windows без слома schema_version.
type Export struct {
	SchemaVersion int                `json:"schema_version"`
	ExportedAt    string             `json:"exported_at"`
	Categories    []model.Category   `json:"categories"`
	Goods         []Good             `json:"goods"`
	Resources     []Resource         `json:"resources"`
	Warnings      []validate.Warning `json:"warnings"`
}

// Build собирает выгрузку из состояния (спека 99a.1 §11).
func Build(st *model.State) *Export {
	byID := graph.ByID(st.Goods)
	ex := &Export{
		SchemaVersion: 1,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Warnings:      validate.Validate(st),
	}
	catUsed := make(map[string]bool)
	resUsed := make(map[string]bool)
	for i := range st.Goods {
		g := &st.Goods[i]
		if g.Status != model.StatusApproved {
			continue // только согласованные
		}
		eg := Good{
			ID:         g.ID,
			Name:       g.Name,
			Category:   g.Category,
			Complexity: graph.EffectiveTier(g, byID), // эффективный тир (99a.3 §12): override ?? вычисленный
		}
		for _, slot := range g.Recipe {
			if slot.GoodID == "" {
				continue // пустые слоты не входят
			}
			q := slot.Quantity
			if q < 1 {
				q = 1 // страховка: реальное количество ≥ 1 (99a.2 §6.3)
			}
			eg.Recipe = append(eg.Recipe, Slot{GoodID: slot.GoodID, Quantity: q})
			if comp := byID[slot.GoodID]; comp != nil && comp.Kind == model.KindResource {
				resUsed[comp.ID] = true
			}
		}
		ex.Goods = append(ex.Goods, eg)
		catUsed[g.Category] = true
	}
	for i := range st.Categories {
		if catUsed[st.Categories[i].ID] {
			ex.Categories = append(ex.Categories, st.Categories[i])
		}
	}
	for i := range st.Goods {
		g := &st.Goods[i]
		if !resUsed[g.ID] || g.ResourceRef == nil {
			continue
		}
		ex.Resources = append(ex.Resources, Resource{
			ID:        g.ID,
			Name:      g.Name,
			Catalog:   g.ResourceRef.Catalog,
			CatalogID: g.ResourceRef.CatalogID,
		})
	}
	return ex
}