// internal/handlers/region_handler.go
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// regionDTO — регион для карты.
type regionDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Radius     float64 `json:"radius"`
	Color      string  `json:"color"`
	WorldCount int     `json:"world_count"`
}

// GetRegionsHandler — возвращает все регионы галактики.
//
// Для диаграммы Вороного на фронте нужны ВСЕ центры регионов (ячейки у границ
// viewport'а зависят от соседей за его пределами), поэтому bounds не берём.
// Регионов мало (сотни), запрос дешёвый.
func (h *AdminHandlers) GetRegionsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, name, center_x, center_y, radius, color, world_count
		FROM regions
		ORDER BY name`)
	if err != nil {
		log.Printf("❌ GetRegions query error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	regions := make([]regionDTO, 0, 256)
	for rows.Next() {
		var reg regionDTO
		if err := rows.Scan(&reg.ID, &reg.Name, &reg.X, &reg.Y, &reg.Radius, &reg.Color, &reg.WorldCount); err != nil {
			log.Printf("❌ GetRegions scan error: %v", err)
			http.Error(w, "Scan error", http.StatusInternalServerError)
			return
		}
		regions = append(regions, reg)
	}
	if err = rows.Err(); err != nil {
		log.Printf("❌ GetRegions rows error: %v", err)
		http.Error(w, "Rows error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(regions); err != nil {
		log.Printf("⚠️ GetRegions encode error: %v", err)
	}
}