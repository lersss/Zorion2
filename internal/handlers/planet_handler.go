package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// GetPlanetsByWorld возвращает планеты для указанного мира
func (h *AdminHandlers) GetPlanetsByWorld(w http.ResponseWriter, r *http.Request) {
	// Извлекаем worldID из URL /api/worlds/{worldID}/planets
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	worldID := pathParts[3]

	planetRepo := repository.NewPlanetRepository(h.db)
	planets, err := planetRepo.GetPlanetsByWorldID(worldID)
	if err != nil {
		http.Error(w, "Failed to fetch planets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if planets == nil {
		planets = []models.Planet{}
	}

	// Получить информацию о мире (название, спектр, тип объекта/системы,
	// модификаторы, масса, температура, координаты). COALESCE — NULL-спектр
	// экзотики не роняет строку (99.2.4 §3).
	var worldName, spectralClass, starType, systemType string
	var modsRaw []byte
	var worldTemp, coordX, coordY float64
	var massRaw sql.NullFloat64
	err = h.db.QueryRow("SELECT name, COALESCE(spectral_class,''), star_type, system_type, stellar_mods, stellar_mass, temperature, coord_x, coord_y FROM worlds WHERE id = $1", worldID).
		Scan(&worldName, &spectralClass, &starType, &systemType, &modsRaw, &massRaw, &worldTemp, &coordX, &coordY)
	if err != nil {
		http.Error(w, "Failed to fetch world info: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var mods map[string]interface{}
	if len(modsRaw) > 0 && string(modsRaw) != "null" {
		json.Unmarshal(modsRaw, &mods)
	}

	var stellarMass *float64
	if massRaw.Valid {
		stellarMass = &massRaw.Float64
	}

	response := struct {
		WorldName     string                 `json:"world_name"`
		SpectralClass string                 `json:"spectral_class"`
		Temperature   float64                `json:"temperature"`
		CoordX        float64                `json:"coord_x"`
		CoordY        float64                `json:"coord_y"`
		StarType      string                 `json:"star_type,omitempty"`
		SystemType    string                 `json:"system_type,omitempty"`
		StellarMods   map[string]interface{} `json:"stellar_mods,omitempty"`
		StellarMass   *float64               `json:"stellar_mass,omitempty"`
		Planets       []models.Planet        `json:"planets"`
	}{
		WorldName:     worldName,
		SpectralClass: spectralClass,
		Temperature:   worldTemp,
		CoordX:        coordX,
		CoordY:        coordY,
		StarType:      starType,
		SystemType:    systemType,
		StellarMods:   mods,
		StellarMass:   stellarMass,
		Planets:       planets,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
