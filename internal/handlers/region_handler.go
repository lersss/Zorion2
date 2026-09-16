// internal/handlers/region_handler.go
package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"zorion/internal/races"
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
	// Profile — ключ класса профиля региона (59a, дополнение гейта 2):
	// отладочный вывод на карте; NULL → пустая строка. В финале убрать —
	// профиль не публикуется как ярлык (спека §11.7 / GDD §2.6.1).
	Profile string `json:"profile"`
	// RaceName — человекочитаемое имя доминантной расы территории (спека
	// 99.2.21 §7.1): отладочный вывод на карте; NULL/нет расы → пустая
	// строка. В финале убрать вместе с профилем (расы — не ярлык).
	RaceName string `json:"race_name"`
}

// GetRegionsHandler — возвращает все регионы галактики.
//
// Для диаграммы Вороного на фронте нужны ВСЕ центры регионов (ячейки у границ
// viewport'а зависят от соседей за его пределами), поэтому bounds не берём.
// Регионов мало (сотни), запрос дешёвый.
func (h *AdminHandlers) GetRegionsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, name, center_x, center_y, radius, color, world_count, COALESCE(profile, ''), COALESCE(race_id, '')
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
		var raceID string
		if err := rows.Scan(&reg.ID, &reg.Name, &reg.X, &reg.Y, &reg.Radius, &reg.Color, &reg.WorldCount, &reg.Profile, &raceID); err != nil {
			log.Printf("❌ GetRegions scan error: %v", err)
			http.Error(w, "Scan error", http.StatusInternalServerError)
			return
		}
		reg.RaceName = regionRaceName(raceID)
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

// regionRaceName — человекочитаемое имя расы региона: из каталога рас
// (internal/races); пустой race_id (NULL = территория без расы) или
// неизвестный ключ (каталог не загружен) — «» (не падаем).
func regionRaceName(raceID string) string {
	if raceID == "" {
		return ""
	}
	if r := races.ByID(raceID); r != nil {
		return r.Name
	}
	return ""
}