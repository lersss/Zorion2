// internal/handlers/players_positions.go
// Позиции чужих игроков для карты (спека 77a §5.3 + 99.2.27 §4.5):
// GET /api/players/positions (игровой JWT). Сервер отдаёт игроков с
// определённым местоположением (изменение 90a, решение создателя С2):
// (а) активный межзвёздный полёт, (б) current_position != NULL (покой
// status=orbit / внутрисистемный полёт status=in_flight). Радиус-фильтр для
// role=player (77a); admin/skycomposer — без радиуса (И7). Клиент не получает
// скрытых позиций (И1). Маршрут/цель чужих полётов не показываются (§5.3).
// Статусы — готовой строкой с именем объекта от сервера (решение @uidesigner):
// клиент карты не знает планет чужих систем.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/lib/pq"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/travel"
)

// PlayersPositions — GET /api/players/positions.
func (h *AdminHandlers) PlayersPositions(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)

	// Круг видимости запрашивающего (для player; admin/skycomposer — всё).
	var pc *playerContext
	if h.visibility != nil && roleFromContext(r) == string(models.RolePlayer) {
		pc = h.visibility.playerContextFrom(r)
	}

	userRepo := repository.NewUserRepository(h.db)
	users, err := userRepo.PlayerPositions()
	if err != nil {
		writeJSONError(w, "Не удалось получить позиции игроков", http.StatusInternalServerError)
		return
	}

	// Координаты и имена миров — из снапшота mapcache (без SQL в хот-пате,
	// спека 77a §11.3).
	worldName := map[string]string{}
	worldCoords := map[string][2]float64{}
	if snap := h.mapCache.Snapshot(); snap != nil {
		for _, w := range snap.Worlds() {
			worldName[w.ID] = w.Name
			worldCoords[w.ID] = [2]float64{w.X, w.Y}
		}
	}

	// Имена планет/спутников и stellar_mods — батчем (стоящих игроков мало,
	// спека 99.2.27 §4.5: статус «у планеты X» с именем объекта от сервера).
	planetNames := map[string]string{}
	satNames := map[string]string{}
	worldMods := map[string]map[string]interface{}{}
	{
		var planetIDs, satIDs, modWorldIDs []string
		for _, u := range users {
			if u.ID == userID || u.CurrentWorldID == nil || u.CurrentPosition == nil {
				continue
			}
			pos := u.CurrentPosition
			if pos.Status != "orbit" {
				continue
			}
			switch pos.ObjectType {
			case "planet":
				planetIDs = append(planetIDs, pos.ObjectID)
			case "satellite":
				satIDs = append(satIDs, pos.ObjectID)
			case "star":
				if pos.ObjectID != *u.CurrentWorldID {
					modWorldIDs = append(modWorldIDs, *u.CurrentWorldID)
				}
			}
		}
		if len(planetIDs) > 0 {
			rows, err := h.db.Query(`SELECT id, name FROM planets WHERE id = ANY($1)`, pq.Array(planetIDs))
			if err == nil {
				for rows.Next() {
					var id, name string
					if rows.Scan(&id, &name) == nil {
						planetNames[id] = name
					}
				}
				rows.Close()
			}
		}
		if len(satIDs) > 0 {
			rows, err := h.db.Query(
				`SELECT sat->>'id', sat->>'name' FROM planets
				 CROSS JOIN LATERAL jsonb_array_elements(data->'satellites') AS sat
				 WHERE sat->>'id' = ANY($1)`, pq.Array(satIDs))
			if err == nil {
				for rows.Next() {
					var id, name string
					if rows.Scan(&id, &name) == nil {
						satNames[id] = name
					}
				}
				rows.Close()
			}
		}
		if len(modWorldIDs) > 0 {
			rows, err := h.db.Query(`SELECT id, stellar_mods FROM worlds WHERE id = ANY($1)`, pq.Array(modWorldIDs))
			if err == nil {
				for rows.Next() {
					var id string
					var modsRaw []byte
					if rows.Scan(&id, &modsRaw) == nil && len(modsRaw) > 0 && string(modsRaw) != "null" {
						var mods map[string]interface{}
						if json.Unmarshal(modsRaw, &mods) == nil {
							worldMods[id] = mods
						}
					}
				}
				rows.Close()
			}
		}
	}

	out := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		if u.ID == userID || u.CurrentWorldID == nil {
			continue
		}
		coords, ok := worldCoords[*u.CurrentWorldID]
		if !ok {
			continue // мир удалён при перегенерации — позиция неизвестна
		}
		x, y := coords[0], coords[1]
		status := "в системе " + worldName[*u.CurrentWorldID]
		var objType, objID string

		// Межзвёздный полёт: интерполированная позиция (77a §4.2), статус
		// «в полёте»; цель/маршрут не показываются (§5.3).
		var f *travel.TravelInfo
		if h.travelManager != nil {
			f = h.travelManager.GetFlight(u.ID)
		}
		if f != nil {
			if toCoords, toOK := worldCoords[f.ToWorld]; toOK {
				x, y = interpolateFlight(f, toCoords[0], toCoords[1])
			}
			status = "в полёте"
		} else if u.CurrentPosition != nil {
			// Внутрисистемная позиция (С2): стоящие/летящие видны в радиусе,
			// координаты = звезда системы (С3-якорь), статус с именем объекта.
			status, objType, objID = resolveIntraStatus(u, worldName[*u.CurrentWorldID], planetNames, satNames, worldMods)
		} else {
			continue // без полёта и без позиции — не отображается (90a-изменение)
		}

		// Фильтр по радиусу для player: вне круга — не отдаём (И1).
		if pc != nil && (!pc.ok || !IsVisible(x, y, pc.centerX, pc.centerY, pc.radius)) {
			continue
		}

		item := map[string]interface{}{
			"id":         u.ID,
			"username":   u.Username,
			"ship_icon":  models.ResolveShipIcon(u.ShipIcon),
			"ship_color": u.ShipColor,
			"status":     status,
			"x":          x,
			"y":          y,
		}
		// object_type/object_id — для модалки (маркеры «кто у какого объекта»,
		// спека 99.2.27 §5.4); у летящих — отсутствуют.
		if objType != "" {
			item["object_type"] = objType
			item["object_id"] = objID
		}
		out = append(out, item)
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"players": out})
}

// resolveIntraStatus — статус игрока с внутрисистемной позицией (спека 99.2.27
// §4.5): «в полёте (система X)» / «у планеты X» / «у спутника X» / «в системе X»
// / «у компаньона X» / «у внешнего компаньона X». Битая цель (перегенерация) →
// фолбэк «в системе X» (ИП-4, М-2): позиция трактуется как «орбита звезды».
func resolveIntraStatus(u *models.User, worldName string, planetNames, satNames map[string]string, worldMods map[string]map[string]interface{}) (status, objType, objID string) {
	pos := u.CurrentPosition
	worldID := *u.CurrentWorldID
	if pos.Status == "in_flight" {
		return "в полёте (система " + worldName + ")", "", ""
	}
	switch pos.ObjectType {
	case "star":
		if pos.ObjectID == worldID {
			return "в системе " + worldName, "star", worldID
		}
		mods := worldMods[worldID]
		if pos.ObjectID == companionID(worldID) {
			label := ""
			if mods != nil {
				label, _ = mods["companion"].(string)
			}
			if label == "" {
				return "в системе " + worldName, "star", worldID // битый компаньон → фолбэк
			}
			return "у компаньона " + label, "star", pos.ObjectID
		}
		if wID, i, ok := parseExtraCompanionID(pos.ObjectID); ok && wID == worldID {
			label := ""
			if mods != nil {
				if ecs, ok := mods["extra_companions"].([]interface{}); ok && i < len(ecs) {
					if m, ok := ecs[i].(map[string]interface{}); ok {
						label, _ = m["spectral_class"].(string)
					}
				}
			}
			if label == "" {
				return "в системе " + worldName, "star", worldID // битый компаньон → фолбэк
			}
			return "у внешнего компаньона " + label, "star", pos.ObjectID
		}
		return "в системе " + worldName, "star", worldID // битая звезда → фолбэк
	case "planet":
		name, ok := planetNames[pos.ObjectID]
		if !ok {
			return "в системе " + worldName, "star", worldID // битая планета → фолбэк
		}
		return "у планеты " + name, "planet", pos.ObjectID
	case "satellite":
		name, ok := satNames[pos.ObjectID]
		if !ok {
			return "в системе " + worldName, "star", worldID // битый спутник → фолбэк
		}
		return "у спутника " + name, "satellite", pos.ObjectID
	}
	return "в системе " + worldName, "star", worldID
}