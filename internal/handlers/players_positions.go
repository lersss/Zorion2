// internal/handlers/players_positions.go
// Позиции чужих игроков для карты (спека 77a §5.3): GET /api/players/positions
// (игровой JWT). Сервер отдаёт только корабли в полёте (90a) в радиусе радара
// запрашивающего (id, username, ship_icon, ship_color, статус, x, y) — клиент
// не получает скрытых позиций (И1). Маршрут/цель полёта не показываются
// (знание, которое должно добываться). admin/skycomposer — без фильтра радиуса
// (И7), фильтр «только в полёте» применяется ко всем ролям (90a).
package handlers

import (
	"net/http"

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

	out := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		if u.ID == userID || u.CurrentWorldID == nil {
			continue
		}
		// Только корабли в полёте (90a): без активного полёта игрок не
		// отображается (для всех ролей, вариант a). Один вызов GetFlight —
		// результат переиспользуется ниже (без микро-окна между проверкой
		// и интерполяцией, замечание @tester).
		var f *travel.TravelInfo
		if h.travelManager != nil {
			f = h.travelManager.GetFlight(u.ID)
		}
		if f == nil {
			continue
		}
		coords, ok := worldCoords[*u.CurrentWorldID]
		if !ok {
			continue // мир удалён при перегенерации — позиция неизвестна
		}
		x, y := coords[0], coords[1]
		status := "в системе " + worldName[*u.CurrentWorldID]

		// В полёте — интерполированная позиция (спека 77a §4.2), статус
		// «в полёте»; цель/маршрут не показываются (§5.3).
		if toCoords, toOK := worldCoords[f.ToWorld]; toOK {
			x, y = interpolateFlight(f, toCoords[0], toCoords[1])
		}
		status = "в полёте"

		// Фильтр по радиусу для player: вне круга — не отдаём (И1).
		if pc != nil && (!pc.ok || !IsVisible(x, y, pc.centerX, pc.centerY, pc.radius)) {
			continue
		}

		out = append(out, map[string]interface{}{
			"id":         u.ID,
			"username":   u.Username,
			"ship_icon":  models.ResolveShipIcon(u.ShipIcon),
			"ship_color": u.ShipColor,
			"status":     status,
			"x":          x,
			"y":          y,
		})
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"players": out})
}