package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
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
	var ageRaw sql.NullFloat64
	err = h.db.QueryRow("SELECT name, COALESCE(spectral_class,''), star_type, system_type, stellar_mods, stellar_mass, age, temperature, coord_x, coord_y FROM worlds WHERE id = $1", worldID).
		Scan(&worldName, &spectralClass, &starType, &systemType, &modsRaw, &massRaw, &ageRaw, &worldTemp, &coordX, &coordY)
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

	var worldAge *float64
	if ageRaw.Valid {
		worldAge = &ageRaw.Float64
	}

	// Видимость игрока (спека 77a §11.2): система вне радиуса радара и не
	// «зажжена» знанием координат — 403 для player. admin/skycomposer — без
	// фильтра (И7). В радиусе — ленивый прогон сканера (§6.1) и скрытие
	// деталей планет за знанием (§6.2).
	if h.visibility != nil && roleFromContext(r) == string(models.RolePlayer) {
		userID, _ := r.Context().Value(auth.UserIDKey).(string)
		user, err := h.visibility.userRepo.GetByID(userID)
		if err != nil || user == nil {
			writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
			return
		}
		centerX, centerY, ok := h.visibility.PlayerPosition(user)
		if !ok {
			writeJSONError(w, "вне зоны видимости", http.StatusForbidden)
			return
		}
		radius := h.visibility.RadarRadius(user)
		known := h.visibility.KnownWorldIDs(userID)
		if !IsVisible(coordX, coordY, centerX, centerY, radius) && !known[worldID] {
			writeJSONError(w, "вне зоны видимости", http.StatusForbidden)
			return
		}
		// Ленивый прогон сканера (спека 77a §6.1, режим A): при взгляде на
		// систему В РАДИУСЕ радара сервер обновляет знание игрока о планетах
		// системы (дата = now). «Зажжённая» знанием система вне радиуса —
		// модалка с ИМЕЮЩИМСЯ знанием (даже устаревшим, fresh:false), но
		// сканировать/обновлять знание НЕЛЬЗЯ — иначе знание известных систем
		// никогда не стареет из любой точки (подрыв И8/И9). Только со сканером
		// (без сканера знания нет).
		if ship.HasScanner(user.Equipment) && IsVisible(coordX, coordY, centerX, centerY, radius) {
			if err := h.visibility.knowledge.ScanSystem(userID, worldID); err != nil {
				log.Printf("⚠️ ScanSystem %s: %v", worldID, err)
			}
		}
		planets = applyPlanetVisibility(userID, planets, h.visibility.knowledge)
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
		WorldAge      *float64               `json:"age,omitempty"`
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
		WorldAge:      worldAge,
		Planets:       planets,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
