package handlers

import (
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

	// Получить информацию о мире (название, спектр, температура, координаты)
	var worldName, spectralClass string
	var worldTemp, coordX, coordY float64
	err = h.db.QueryRow("SELECT name, spectral_class, temperature, coord_x, coord_y FROM worlds WHERE id = $1", worldID).
		Scan(&worldName, &spectralClass, &worldTemp, &coordX, &coordY)
	if err != nil {
		http.Error(w, "Failed to fetch world info: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := struct {
		WorldName     string           `json:"world_name"`
		SpectralClass string           `json:"spectral_class"`
		Temperature   float64          `json:"temperature"`
		CoordX        float64          `json:"coord_x"`
		CoordY        float64          `json:"coord_y"`
		Planets       []models.Planet  `json:"planets"`
	}{
		WorldName:     worldName,
		SpectralClass: spectralClass,
		Temperature:   worldTemp,
		CoordX:        coordX,
		CoordY:        coordY,
		Planets:       planets,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}