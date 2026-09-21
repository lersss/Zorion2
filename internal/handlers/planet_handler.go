package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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
	//
	// С-3 (спека 99.2.27 §4.4): my_position нужен для ВСЕХ ролей — user грузим
	// всегда (когда visibility подключён); для player — дополнительно фильтр
	// видимости ниже.
	var myPosition *models.CurrentPosition
	// inRadar — система в радиусе радара игрока (условие раскрытия состава
	// пояса, спека поясов этап 2 §4.3); для admin/skycomposer — false (не
	// используется: они видят пояса целиком).
	inRadar := false
	var belts []models.Belt
	if h.visibility != nil {
		userID, _ := r.Context().Value(auth.UserIDKey).(string)
		user, pos, _, err := h.visibility.userRepo.GetByIDWithPosition(userID)
		if err != nil || user == nil {
			writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
			return
		}
		// Гейт видимости (403) — ДО загрузки поясов: система вне радиуса и не
		// «зажжена» знанием → отказ, запрос поясов не тратится.
		if roleFromContext(r) == string(models.RolePlayer) {
			centerX, centerY, ok := h.visibility.PlayerPosition(user)
			if !ok {
				writeJSONError(w, "вне зоны видимости", http.StatusForbidden)
				return
			}
			radius := h.visibility.RadarRadius(user)
			known := h.visibility.KnownWorldIDs(userID)
			inRadar = IsVisible(coordX, coordY, centerX, centerY, radius)
			if !inRadar && !known[worldID] {
				writeJSONError(w, "вне зоны видимости", http.StatusForbidden)
				return
			}
		}
		// Пояса малых тел (спека 2026-09-22-пояса-малых-тел-этап-2-показ-
		// знание-полёт §4.1): аддитивное поле belts. Источник — system_belts
		// мира. Нужны ДО normalizeMyPosition (позиция-в-поясе валидна, §5.6).
		// Best-effort: сбой выборки поясов не роняет карточку системы (пояса —
		// аддитивная секция) — warn + пустой список.
		belts, err = planetRepo.GetBeltsByWorldID(worldID)
		if err != nil {
			log.Printf("⚠️ belts: не удалось загрузить пояса мира %s: %v", worldID, err)
			belts = nil
		}
		// my_position — только если игрок в этой системе (current_world_id ==
		// worldID); битая позиция (перегенерация) → фолбэк «орбита звезды» (ИП-4),
		// NULL-позиция (легаси) → «орбита звезды». Во время межзвёздного полёта
		// позиция NULL и НЕ нормализуется (игрок покинул систему, С1).
		if user.CurrentWorldID != nil && *user.CurrentWorldID == worldID &&
			h.visibility.travelMgr.GetFlight(userID) == nil {
			myPosition = normalizeMyPosition(pos, worldID, planets, belts, func(objID string) bool {
				return objID == worldID || companionIDFromMods(worldID, mods, objID)
			})
		}
		if roleFromContext(r) == string(models.RolePlayer) {
			// Ленивый прогон сканера (спека 77a §6.1, режим A): при взгляде на
			// систему В РАДИУСЕ радара сервер обновляет знание игрока о планетах
			// системы (дата = now). «Зажжённая» знанием система вне радиуса —
			// модалка с ИМЕЮЩИМСЯ знанием (даже устаревшим, fresh:false), но
			// сканировать/обновлять знание НЕЛЬЗЯ — иначе знание известных систем
			// никогда не стареет из любой точки (подрыв И8/И9). Только со сканером
			// (без сканера знания нет).
			if ship.HasScanner(user.Equipment) && inRadar {
				if err := h.visibility.knowledge.ScanSystem(userID, worldID); err != nil {
					log.Printf("⚠️ ScanSystem %s: %v", worldID, err)
				}
			}
			planets = applyPlanetVisibility(userID, planets, h.visibility.knowledge)
		}
	} else {
		// visibility не подключён (админ-путь без видимости) — пояса грузим
		// как есть (models.Belt целиком). Best-effort: сбой не роняет ответ.
		belts, err = planetRepo.GetBeltsByWorldID(worldID)
		if err != nil {
			log.Printf("⚠️ belts: не удалось загрузить пояса мира %s: %v", worldID, err)
			belts = nil
		}
	}

	// Выдача поясов: для player — BeltView (только visible=true, состав при
	// знании §4.2/§4.3); для admin/skycomposer — models.Belt целиком (И7).
	// Пустой результат — `belts: []`, не null (§4.2/§7.6: секция показывает
	// «Поясов нет»); nil-срез нормализуем в пустой массив для всех ролей.
	var beltsOut interface{}
	if roleFromContext(r) == string(models.RolePlayer) {
		beltsOut = applyBeltVisibility(belts, inRadar, myPosition)
	} else {
		if belts == nil {
			belts = []models.Belt{}
		}
		beltsOut = belts
	}

	// Компаньоны — синтетические id (спека 99.2.27 §3.1/§4.4, решение создателя
	// 2026-09-20): companion_id (companion:<worldID>, если есть компаньон) и
	// extra_companions[].id (extra:<worldID>:<i> по индексу). Старый контракт
	// не ломается (поля новые).
	var companionID string
	if c, ok := mods["companion"].(string); ok && c != "" {
		companionID = "companion:" + worldID
	}
	if ecs, ok := mods["extra_companions"].([]interface{}); ok {
		for i, ec := range ecs {
			if m, ok := ec.(map[string]interface{}); ok {
				m["id"] = "extra:" + worldID + ":" + strconv.Itoa(i)
			}
		}
	}

	response := struct {
		WorldName     string                  `json:"world_name"`
		SpectralClass string                  `json:"spectral_class"`
		Temperature   float64                 `json:"temperature"`
		CoordX        float64                 `json:"coord_x"`
		CoordY        float64                 `json:"coord_y"`
		StarType      string                  `json:"star_type,omitempty"`
		SystemType    string                  `json:"system_type,omitempty"`
		StellarMods   map[string]interface{}  `json:"stellar_mods,omitempty"`
		StellarMass   *float64                `json:"stellar_mass,omitempty"`
		WorldAge      *float64                `json:"age,omitempty"`
		Planets       []models.Planet         `json:"planets"`
		Belts         interface{}             `json:"belts"`
		CompanionID   string                  `json:"companion_id,omitempty"`
		MyPosition    *models.CurrentPosition `json:"my_position"`
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
		Belts:         beltsOut,
		CompanionID:   companionID,
		MyPosition:    myPosition,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
