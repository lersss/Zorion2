// internal/handlers/admin_stats_load.go
package handlers

import "encoding/json"

// worldInfo — минимальная информация о мире, нужная статистике.
type worldInfo struct {
	ID            string
	SpectralClass string // пусто у экзотики (NULL в БД, читается COALESCE)
	StarType      string // star/white_dwarf/neutron/black_hole/protostar (99.2.4 §2)
	SystemType    string // single/binary/multiple
	Temperature   int
}

// planetRecord — одна планета из БД в виде map.
type planetRecord struct {
	ID      string
	WorldID string
	Data    map[string]interface{}
}

// loadWorlds — загружает список миров.
// COALESCE(spectral_class,'') — NULL-спектр экзотики не роняет строку
// (99.2.4 §3): раньше Scan на NULL молча выкидывал мир из статистики.
func (h *AdminHandlers) loadWorlds() ([]worldInfo, error) {
	rows, err := h.db.Query(`SELECT id, COALESCE(spectral_class,''), star_type, system_type, temperature FROM worlds`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []worldInfo{}
	for rows.Next() {
		var w worldInfo
		if err := rows.Scan(&w.ID, &w.SpectralClass, &w.StarType, &w.SystemType, &w.Temperature); err != nil {
			continue
		}
		result = append(result, w)
	}
	return result, nil
}

// loadPlanets — загружает все планеты с распарсенным JSON.
func (h *AdminHandlers) loadPlanets() ([]planetRecord, error) {
	rows, err := h.db.Query(`SELECT id, world_id, data FROM planets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []planetRecord{}
	for rows.Next() {
		var id, worldID string
		var dataJSON []byte
		if err := rows.Scan(&id, &worldID, &dataJSON); err != nil {
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			continue
		}
		result = append(result, planetRecord{ID: id, WorldID: worldID, Data: data})
	}
	return result, nil
}

// loadSettlementPopulation — население по каждой планете (SUM settlements.population).
func (h *AdminHandlers) loadSettlementPopulation() (map[string]int64, error) {
	rows, err := h.db.Query(`SELECT planet_id, SUM(population) FROM settlements GROUP BY planet_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]int64{}
	for rows.Next() {
		var planetID string
		var population int64
		if err := rows.Scan(&planetID, &population); err != nil {
			continue
		}
		result[planetID] = population
	}
	return result, rows.Err()
}