// Пакет handlers реализует HTTP-хендлеры игровой карты.
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Кинды объектов в результатах поиска.
const (
	searchKindWorld     = "world"
	searchKindPlanet    = "planet"
	searchKindSatellite = "satellite"
)

// Лимиты количества результатов поиска.
const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
)

// searchResult — один результат поиска по точному имени.
type searchResult struct {
	Kind      string  `json:"kind"`
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	WorldID   string  `json:"world_id"`
	WorldName string  `json:"world_name"`
	Spectral  string  `json:"spectral"`
	CoordX    float64 `json:"coord_x"`
	CoordY    float64 `json:"coord_y"`
	PlanetID  string  `json:"planet_id,omitempty"` // для спутников
	Type      string  `json:"type,omitempty"`      // для планет
}

// searchResponse — ответ поиска.
type searchResponse struct {
	Results []searchResult `json:"results"`
}

// SearchEntitiesHandler — поиск звезды/планеты/спутника по точному имени
// (без учёта регистра). Нужен для фокусировки карты на найденном объекте.
func (h *AdminHandlers) SearchEntitiesHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "missing q param", http.StatusBadRequest)
		return
	}
	limit, err := parseSearchLimit(r.URL.Query().Get("limit"))
	if err != nil {
		http.Error(w, "invalid limit", http.StatusBadRequest)
		return
	}

	results, err := searchByName(r.Context(), h.db, normalizeSearchName(q), limit)
	if err != nil {
		log.Printf("❌ SearchEntities query error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(searchResponse{Results: results}); err != nil {
		log.Printf("⚠️ SearchEntities encode error: %v", err)
	}
}

// ==================== ХЕЛПЕРЫ ====================

// normalizeSearchName — нормализует запрос для точного поиска без учёта регистра.
func normalizeSearchName(q string) string {
	return strings.ToLower(strings.TrimSpace(q))
}

// parseSearchLimit — разбирает параметр limit: пустое → default, > max → max.
func parseSearchLimit(raw string) (int, error) {
	if raw == "" {
		return defaultSearchLimit, nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v < 1 {
		return 0, errors.New("invalid limit")
	}
	if v > maxSearchLimit {
		return maxSearchLimit, nil
	}
	return v, nil
}

// searchKindRank — порядок показа результатов: звёзды, планеты, спутники.
func searchKindRank(kind string) int {
	switch kind {
	case searchKindWorld:
		return 0
	case searchKindPlanet:
		return 1
	case searchKindSatellite:
		return 2
	default:
		return 3
	}
}

// mergeSearchResults — объединяет результаты из трёх источников в порядке
// «звезда → планета → спутник», убирает дубли по (kind, id) и усекает до limit.
func mergeSearchResults(limit int, groups ...[]searchResult) []searchResult {
	seen := make(map[string]bool)
	all := make([]searchResult, 0)
	for _, group := range groups {
		for _, res := range group {
			key := res.Kind + "|" + res.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, res)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		return searchKindRank(all[i].Kind) < searchKindRank(all[j].Kind)
	})

	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

// ==================== ЗАПРОСЫ ====================

// searchByName — ищет миры, планеты и спутники с точным именем.
func searchByName(ctx context.Context, db *sql.DB, name string, limit int) ([]searchResult, error) {
	remaining := limit

	worlds, err := searchWorldsByName(ctx, db, name, remaining)
	if err != nil {
		return nil, err
	}
	remaining -= len(worlds)

	planets, err := searchPlanetsByName(ctx, db, name, remaining)
	if err != nil {
		return nil, err
	}
	remaining -= len(planets)

	satellites, err := searchSatellitesByName(ctx, db, name, remaining)
	if err != nil {
		return nil, err
	}

	return mergeSearchResults(limit, worlds, planets, satellites), nil
}

// searchWorldsByName ищет звёздные миры с точным именем.
func searchWorldsByName(ctx context.Context, db *sql.DB, name string, limit int) ([]searchResult, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, COALESCE(spectral_class,''), coord_x, coord_y
		FROM worlds
		WHERE LOWER(name) = $1
		ORDER BY name
		LIMIT $2
	`, name, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]searchResult, 0, limit)
	for rows.Next() {
		var r searchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.Spectral, &r.CoordX, &r.CoordY); err != nil {
			return nil, err
		}
		r.Kind = searchKindWorld
		r.WorldID = r.ID
		r.WorldName = r.Name
		results = append(results, r)
	}
	return results, rows.Err()
}

// searchPlanetsByName ищет планеты с точным именем.
func searchPlanetsByName(ctx context.Context, db *sql.DB, name string, limit int) ([]searchResult, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.name, p.world_id, w.name, COALESCE(w.spectral_class,''), w.coord_x, w.coord_y,
		       COALESCE(p.data->>'type', '')
		FROM planets p
		JOIN worlds w ON w.id = p.world_id
		WHERE LOWER(p.name) = $1
		ORDER BY p.name
		LIMIT $2
	`, name, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]searchResult, 0, limit)
	for rows.Next() {
		var r searchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.WorldID, &r.WorldName, &r.Spectral, &r.CoordX, &r.CoordY, &r.Type); err != nil {
			return nil, err
		}
		r.Kind = searchKindPlanet
		results = append(results, r)
	}
	return results, rows.Err()
}

// searchSatellitesByName ищет спутники (jsonb-массив в planets.data) с точным именем.
func searchSatellitesByName(ctx context.Context, db *sql.DB, name string, limit int) ([]searchResult, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT sat->>'id', sat->>'name', p.id, p.world_id, w.name, COALESCE(w.spectral_class,''), w.coord_x, w.coord_y
		FROM planets p
		JOIN worlds w ON w.id = p.world_id
		CROSS JOIN LATERAL jsonb_array_elements(p.data->'satellites') AS sat
		WHERE LOWER(sat->>'name') = $1
		ORDER BY sat->>'name'
		LIMIT $2
	`, name, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]searchResult, 0, limit)
	for rows.Next() {
		var r searchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.PlanetID, &r.WorldID, &r.WorldName, &r.Spectral, &r.CoordX, &r.CoordY); err != nil {
			return nil, err
		}
		r.Kind = searchKindSatellite
		results = append(results, r)
	}
	return results, rows.Err()
}