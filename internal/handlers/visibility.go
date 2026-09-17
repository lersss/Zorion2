// internal/handlers/visibility.go
// Серверная видимость игрока (спека 77a §11): сервер — единственный источник
// видимости (И1). Для role=player вычисляется круг (центр = текущая позиция
// игрока, радиус = текущий радар из users.equipment) и данные вне круга не
// отдаются. admin/skycomposer — без фильтра (И7).
package handlers

import (
	"net/http"
	"time"

	"zorion/internal/auth"
	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// Visibility — вычисление круга видимости игрока и проверки принадлежности.
type Visibility struct {
	userRepo  *repository.UserRepository
	travelMgr *travel.Manager
	mapCache  *mapcache.Manager
	knowledge *repository.KnowledgeRepository
}

func NewVisibility(
	userRepo *repository.UserRepository,
	travelMgr *travel.Manager,
	mapCache *mapcache.Manager,
	knowledge *repository.KnowledgeRepository,
) *Visibility {
	return &Visibility{userRepo: userRepo, travelMgr: travelMgr, mapCache: mapCache, knowledge: knowledge}
}

// RadarRadius — радиус видимости игрока (спека 77a §4.2): из установленного
// радара (users.equipment.radar → equipment.params.radius); без радара —
// минимум 200 px. Слот engine на радиус не влияет (И4).
func (v *Visibility) RadarRadius(user *models.User) float64 {
	if user == nil {
		return models.RadarRadiusMin
	}
	return ship.RadarRadius(user.Equipment)
}

// PlayerPosition — текущая позиция игрока (спека 77a §4.2): координаты
// current_world_id; в полёте — интерполированная позиция по 66a (радар
// «едет» вместе с кораблём). ok=false, если позиция неизвестна (нет
// current_world_id или мир удалён при перегенерации).
func (v *Visibility) PlayerPosition(user *models.User) (x, y float64, ok bool) {
	if user == nil || user.CurrentWorldID == nil {
		return 0, 0, false
	}
	fromX, fromY, fromOK := v.worldCoords(*user.CurrentWorldID)
	if !fromOK {
		return 0, 0, false
	}

	// В полёте — интерполяция от стартовой точки сегмента (61a) к цели.
	if f := v.travelMgr.GetFlight(user.ID); f != nil {
		toX, toY, toOK := v.worldCoords(f.ToWorld)
		if !toOK {
			// Цель удалена — позиция = стартовая точка сегмента.
			return f.StartX, f.StartY, true
		}
		x, y := interpolateFlight(f, toX, toY)
		return x, y, true
	}

	return fromX, fromY, true
}

// interpolateFlight — позиция в полёте на момент now (спека 77a §4.2):
// линейная интерполяция от стартовой точки сегмента (61a) к цели по
// прогрессу (зажат 0..1). Радар «едет» вместе с кораблём.
func interpolateFlight(f *travel.TravelInfo, toX, toY float64) (x, y float64) {
	progress := 0.0
	if f.Duration > 0 {
		progress = float64(timeSince(f.StartTime)) / float64(f.Duration)
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return f.StartX + (toX-f.StartX)*progress, f.StartY + (toY-f.StartY)*progress
}

// worldCoords — координаты мира из снапшота mapcache (без SQL в хот-пате,
// спека 77a §11.3). Линейный поиск по id — O(миров), разово на запрос.
func (v *Visibility) worldCoords(worldID string) (x, y float64, ok bool) {
	snap := v.mapCache.Snapshot()
	if snap == nil {
		return 0, 0, false
	}
	for _, w := range snap.Worlds() {
		if w.ID == worldID {
			return w.X, w.Y, true
		}
	}
	return 0, 0, false
}

// IsVisible — точка (worldX, worldY) в круге радиуса вокруг центра.
func IsVisible(worldX, worldY, centerX, centerY, radius float64) bool {
	dx := worldX - centerX
	dy := worldY - centerY
	return dx*dx+dy*dy <= radius*radius
}

// KnownWorldIDs — системы, «зажжённые» знанием координат (спека 77a §5.1):
// полёт и полные данные разрешены. Пустая map при ошибке — безопасный фолбэк
// (ничего не «зажигаем» сверх радиуса).
func (v *Visibility) KnownWorldIDs(userID string) map[string]bool {
	known, err := v.knowledge.KnownWorldIDs(userID)
	if err != nil {
		return map[string]bool{}
	}
	return known
}

// playerContext — пользователь + круг видимости для role=player (один раз на
// запрос). ok=false — позиция неизвестна (безопасное направление: скрывать).
type playerContext struct {
	user    *models.User
	centerX float64
	centerY float64
	radius  float64
	ok      bool
}

// playerContextFrom — собирает контекст видимости из запроса (userID из JWT).
func (v *Visibility) playerContextFrom(r *http.Request) *playerContext {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	user, err := v.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return &playerContext{ok: false}
	}
	cx, cy, ok := v.PlayerPosition(user)
	return &playerContext{user: user, centerX: cx, centerY: cy, radius: v.RadarRadius(user), ok: ok}
}

// CanSeeWorld — видит ли игрок мир (спека 77a §11.2/И11): в радиусе радара,
// «зажжён» знанием координат, либо является текущим миром игрока или
// целью/источником его активного полёта (знание цели принадлежит игроку —
// восстановление полёта после рефреша, 42a, не ломается).
func (v *Visibility) CanSeeWorld(user *models.User, worldID string, worldX, worldY float64) bool {
	if user == nil {
		return false
	}
	// Текущий мир и цель/источник активного полёта — всегда доступны.
	if user.CurrentWorldID != nil && *user.CurrentWorldID == worldID {
		return true
	}
	if f := v.travelMgr.GetFlight(user.ID); f != nil && (f.ToWorld == worldID || f.FromWorld == worldID) {
		return true
	}
	cx, cy, ok := v.PlayerPosition(user)
	if !ok {
		return false
	}
	if IsVisible(worldX, worldY, cx, cy, v.RadarRadius(user)) {
		return true
	}
	return v.KnownWorldIDs(user.ID)[worldID]
}

// timeSince — обёртка над time.Since для тестируемости (инъекция времени).
var timeSince = func(t time.Time) time.Duration { return time.Since(t) }