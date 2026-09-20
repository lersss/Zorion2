// internal/handlers/intrasystem_handlers.go
// Внутрисистемный полёт (спека 99.2.27 §4.1): POST /api/intrasystem-flight —
// старт полёта с орбиты объекта системы на орбиту другого (звезда/планета/
// спутник, компаньоны — синтетические id). Длительность честная по расстоянию
// в а.е. (двухрежимная формула §3.4). Старт атомарен (С-1): строка полёта +
// current_position = in_flight одной транзакцией.
package handlers

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// IntrasystemHandlers — внутрисистемные полёты.
type IntrasystemHandlers struct {
	worldRepo     *repository.WorldRepository
	userRepo      *repository.UserRepository
	planetRepo    *repository.PlanetRepository
	intraRepo     *repository.PlayerIntrasystemFlightRepository
	knowledgeRepo *repository.KnowledgeRepository
	travelManager *travel.Manager
	intraManager  *travel.IntrasystemManager
}

func NewIntrasystemHandlers(
	worldRepo *repository.WorldRepository,
	userRepo *repository.UserRepository,
	planetRepo *repository.PlanetRepository,
	intraRepo *repository.PlayerIntrasystemFlightRepository,
	knowledgeRepo *repository.KnowledgeRepository,
	travelManager *travel.Manager,
	intraManager *travel.IntrasystemManager,
) *IntrasystemHandlers {
	return &IntrasystemHandlers{
		worldRepo:     worldRepo,
		userRepo:      userRepo,
		planetRepo:    planetRepo,
		intraRepo:     intraRepo,
		knowledgeRepo: knowledgeRepo,
		travelManager: travelManager,
		intraManager:  intraManager,
	}
}

// CalcIntraDuration — длительность внутрисистемного полёта (спека §3.4, С4):
// двухрежимная формула с параметром sf (скорость двигателя, сек/а.е.):
//
//	dist ≤ 100 а.е.: max(3, dist×sf)                    — манёвренный режим
//	dist > 100 а.е.: 100×sf + (dist−100)×sf×0.1         — крейсерский режим
//
// Сшивка на 100 а.е. непрерывна при любом sf (100×sf, Н3). Минимум 3 сек (66a).
// Реальный диапазон: 3 с – ~5.5 мин (компаньон 10000 а.е. → 327 с), потолка нет.
func CalcIntraDuration(distAU, speedFactor float64) time.Duration {
	if distAU <= 100 {
		d := time.Duration(distAU * speedFactor * float64(time.Second))
		if d < 3*time.Second {
			d = 3 * time.Second
		}
		return d
	}
	return time.Duration((100*speedFactor+(distAU-100)*speedFactor*0.1) * float64(time.Second))
}

// ==================== СИНТЕТИЧЕСКИЕ ID КОМПАНЬОНОВ (§3.1) ====================

// companionID — синтетический стабильный id главного компаньона (один на систему).
func companionID(worldID string) string { return "companion:" + worldID }

// extraCompanionID — синтетический id внешнего компаньона кратной (i — индекс).
func extraCompanionID(worldID string, i int) string { return "extra:" + worldID + ":" + strconv.Itoa(i) }

// parseExtraCompanionID — разбор extra:<world>:<i>.
func parseExtraCompanionID(id string) (worldID string, i int, ok bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[0] != "extra" {
		return "", 0, false
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil || idx < 0 {
		return "", 0, false
	}
	return parts[1], idx, true
}

// IsValidCompanionID — валидный синтетический id компаньона системы
// (companion:<world> — есть компаньон; extra:<world>:<i> — i в диапазоне).
// Экспорт — для Restore в main.go.
func IsValidCompanionID(world *models.World, objID string) bool {
	if world == nil || world.StellarMods == nil {
		return false
	}
	if objID == companionID(world.ID) {
		return world.StellarMods.Companion != ""
	}
	if wID, i, ok := parseExtraCompanionID(objID); ok && wID == world.ID {
		return i < len(world.StellarMods.ExtraCompanions)
	}
	return false
}

// companionLabel — подпись компаньона для статуса (§4.5): спектральный класс
// (главный) / спектральный класс внешнего (по индексу).
func companionLabel(world *models.World, objID string) string {
	if world == nil || world.StellarMods == nil {
		return ""
	}
	if objID == companionID(world.ID) {
		return world.StellarMods.Companion
	}
	if wID, i, ok := parseExtraCompanionID(objID); ok && wID == world.ID {
		if i < len(world.StellarMods.ExtraCompanions) {
			return world.StellarMods.ExtraCompanions[i].SpectralClass
		}
	}
	return ""
}

// ==================== РАССТОЯНИЕ И ВАЛИДАЦИЯ ЦЕЛИ (§3.4) ====================

// objectRadiusAU — расстояние объекта от главной звезды в а.е.: звезда
// (главная) = 0; планета = orbit_radius_au; спутник = r родительской планеты;
// компаньон = companion_sep_au / extra_companions[i].sep_au.
func objectRadiusAU(world *models.World, planets []models.Planet, objType, objID string) (float64, bool) {
	switch objType {
	case "star":
		if objID == world.ID {
			return 0, true
		}
		if world.StellarMods == nil {
			return 0, false
		}
		if objID == companionID(world.ID) {
			if world.StellarMods.CompanionSepAU != nil {
				return *world.StellarMods.CompanionSepAU, true
			}
			return 0, false
		}
		if wID, i, ok := parseExtraCompanionID(objID); ok && wID == world.ID {
			if i < len(world.StellarMods.ExtraCompanions) {
				return world.StellarMods.ExtraCompanions[i].SepAU, true
			}
		}
		return 0, false
	case "planet":
		for _, p := range planets {
			if p.ID == objID {
				return p.OrbitRadiusAU, true
			}
		}
		return 0, false
	case "satellite":
		for _, p := range planets {
			for _, s := range p.Satellites {
				if s.ID == objID {
					return p.OrbitRadiusAU, true
				}
			}
		}
		return 0, false
	}
	return 0, false
}

// targetInSystem — цель принадлежит системе current_world_id (ИП-1).
func targetInSystem(world *models.World, planets []models.Planet, objType, objID string) bool {
	switch objType {
	case "star":
		if objID == world.ID {
			return true
		}
		return IsValidCompanionID(world, objID)
	case "planet":
		for _, p := range planets {
			if p.ID == objID {
				return true
			}
		}
		return false
	case "satellite":
		for _, p := range planets {
			for _, s := range p.Satellites {
				if s.ID == objID {
					return true
				}
			}
		}
		return false
	}
	return false
}

// positionObjectValid — объект позиции принадлежит системе (ИП-4 проверка).
// validStar — проверка звезды/компаньона (главная = worldID или синтетический
// id компаньона системы).
func positionObjectValid(objType, objID, worldID string, planets []models.Planet, validStar func(objID string) bool) bool {
	switch objType {
	case "star":
		return validStar(objID)
	case "planet":
		for _, p := range planets {
			if p.ID == objID {
				return true
			}
		}
		return false
	case "satellite":
		for _, p := range planets {
			for _, s := range p.Satellites {
				if s.ID == objID {
					return true
				}
			}
		}
		return false
	}
	return false
}

// normalizeMyPosition — ИП-4 фолбэк (спека §2.3): битая позиция (объект удалён
// перегенерацией) → «орбита звезды». NULL-позиция (легаси-игрок в системе) →
// «орбита звезды» (игрок физически у звезды — старая модель). Применяется при
// отдаче my_position (§4.4) — ничего не падает (М-2).
func normalizeMyPosition(pos *models.CurrentPosition, worldID string, planets []models.Planet, validStar func(objID string) bool) *models.CurrentPosition {
	if pos == nil {
		return models.StarOrbitPosition(worldID)
	}
	switch pos.Status {
	case "orbit":
		if positionObjectValid(pos.ObjectType, pos.ObjectID, worldID, planets, validStar) {
			return pos
		}
	case "in_flight":
		if positionObjectValid(pos.FromType, pos.FromID, worldID, planets, validStar) &&
			positionObjectValid(pos.ToType, pos.ToID, worldID, planets, validStar) {
			return pos
		}
	}
	return models.StarOrbitPosition(worldID)
}

// companionIDFromMods — валидный синтетический id компаньона по raw
// stellar_mods (planet_handler работает с map, не с models.World).
func companionIDFromMods(worldID string, mods map[string]interface{}, objID string) bool {
	if objID == companionID(worldID) {
		c, _ := mods["companion"].(string)
		return c != ""
	}
	if wID, i, ok := parseExtraCompanionID(objID); ok && wID == worldID {
		ecs, _ := mods["extra_companions"].([]interface{})
		return i < len(ecs)
	}
	return false
}

// ==================== ПРИБЫТИЕ (onArrival, §3.6) ====================

// NewIntraArrivalHandler — колбэк прибытия внутрисистемного полёта: позиция
// orbit на цели (атомарно с удалением строки, С-1) + авто-знание
// (source=presence, С6). Битая цель на момент прибытия → фолбэк «орбита
// звезды» (ИП-4). Общая для хендлера и Restore (main.go).
func NewIntraArrivalHandler(
	intraRepo *repository.PlayerIntrasystemFlightRepository,
	planetRepo *repository.PlanetRepository,
	knowledgeRepo *repository.KnowledgeRepository,
) travel.IntraArrivalFunc {
	return func(userID string, f *travel.IntraFlightInfo) {
		// 1. Позиция orbit на цели (или фолбэк «орбита звезды» при битой цели).
		pos := models.OrbitPosition(f.ToType, f.ToID)
		if !arrivalTargetValid(planetRepo, f.WorldID, f.ToType, f.ToID) {
			pos = models.StarOrbitPosition(f.WorldID)
		}
		if err := intraRepo.ArriveAtomic(userID, pos, f.StartTime, f.ArriveAt); err != nil {
			log.Printf("⚠️ intrasystem: onArrival (user %s): %v", userID, err)
			return
		}
		// 2. Авто-знание (С6): планета → UPSERT; спутник → родительская планета.
		switch f.ToType {
		case "planet":
			if err := knowledgeRepo.ScanPlanet(userID, f.ToID, "presence"); err != nil {
				log.Printf("⚠️ intrasystem: knowledge (user %s, planet %s): %v", userID, f.ToID, err)
			}
		case "satellite":
			parent, err := planetRepo.FindPlanetBySatellite(f.WorldID, f.ToID)
			if err == nil && parent != nil {
				if err := knowledgeRepo.ScanPlanet(userID, parent.ID, "presence"); err != nil {
					log.Printf("⚠️ intrasystem: knowledge (user %s, satellite %s): %v", userID, f.ToID, err)
				}
			}
		}
	}
}

// arrivalTargetValid — цель прибытия существует в системе (ИП-4).
func arrivalTargetValid(planetRepo *repository.PlanetRepository, worldID, objType, objID string) bool {
	switch objType {
	case "planet":
		p, err := planetRepo.GetPlanetByID(objID)
		return err == nil && p != nil && p.WorldID == worldID
	case "satellite":
		p, err := planetRepo.FindPlanetBySatellite(worldID, objID)
		return err == nil && p != nil
	case "star":
		return true // звезда/компаньон: система существует (проверено при старте)
	}
	return false
}

// ==================== ХЕНДЛЕР СТАРТА (§4.1) ====================

type IntraFlightRequest struct {
	ObjectType string `json:"object_type"` // star|planet|satellite
	ObjectID   string `json:"object_id"`
}

type IntraFlightResponse struct {
	Duration  int    `json:"duration"`
	FromType  string `json:"from_type"`
	FromID    string `json:"from_id"`
	ToType    string `json:"to_type"`
	ToID      string `json:"to_id"`
	StartTime int64  `json:"start_time"` // UnixMilli
	ArriveAt  int64  `json:"arrive_at"`  // UnixMilli
}

// StartIntraFlight — POST /api/intrasystem-flight. Валидации в порядке спеки
// §4.1: авторизация → двигатель (91a) → активный межзвёздный полёт →
// current_world_id → цель в системе → идемпотентность (пара to_type+to_id) →
// «уже на орбите».
func (h *IntrasystemHandlers) StartIntraFlight(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	var req IntraFlightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.ObjectType != "star" && req.ObjectType != "planet" && req.ObjectType != "satellite" {
		writeJSONError(w, "Некорректный тип объекта", http.StatusBadRequest)
		return
	}
	if req.ObjectID == "" {
		writeJSONError(w, "object_id обязателен", http.StatusBadRequest)
		return
	}

	user, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}

	// 2. Двигатель (спека 91a §6.1): role=player без установленного двигателя
	// не летает; админ/skycomposer — исключение.
	if user.Role == models.RolePlayer && !ship.HasEngine(user.Equipment) {
		writeJSONError(w, "Двигатель не установлен — полёт невозможен", http.StatusBadRequest)
		return
	}

	// 3. Активный межзвёздный полёт → отказ (И2: не сосуществуют).
	if h.travelManager != nil && h.travelManager.GetFlight(userID) != nil {
		writeJSONError(w, "Вы в межзвёздном полёте", http.StatusBadRequest)
		return
	}

	// 4. current_world_id NULL или мир битый → «Вы не находитесь в системе».
	if user.CurrentWorldID == nil {
		writeJSONError(w, "Вы не находитесь в системе", http.StatusBadRequest)
		return
	}
	world, err := h.worldRepo.GetByID(*user.CurrentWorldID)
	if err != nil || world == nil {
		writeJSONError(w, "Вы не находитесь в системе", http.StatusBadRequest)
		return
	}
	worldID := *user.CurrentWorldID

	// 5. Цель принадлежит системе (ИП-1).
	planets, err := h.planetRepo.GetPlanetsLightByWorldID(worldID)
	if err != nil {
		writeJSONError(w, "Не удалось загрузить систему", http.StatusInternalServerError)
		return
	}
	if !targetInSystem(world, planets, req.ObjectType, req.ObjectID) {
		writeJSONError(w, "Объект не найден в вашей системе", http.StatusBadRequest)
		return
	}

	// 6. Идемпотентность: активный полёт к той же цели (пара to_type+to_id,
	// М-7) → 202 с текущим полётом (без перезапуска).
	if f := h.intraManager.GetIntraFlight(userID); f != nil && f.ToType == req.ObjectType && f.ToID == req.ObjectID {
		writeJSONStatus(w, http.StatusAccepted, IntraFlightResponse{
			Duration:  int(f.ArriveAt.Sub(f.StartTime).Seconds()),
			FromType:  f.FromType,
			FromID:    f.FromID,
			ToType:    f.ToType,
			ToID:      f.ToID,
			StartTime: f.StartTime.UnixMilli(),
			ArriveAt:  f.ArriveAt.UnixMilli(),
		})
		return
	}

	// 7. «Уже на орбите» (§3.5): в покое — цель == объект позиции; в полёте —
	// цель == объект отправления (UX-5 блокирует кнопку клиентски).
	if pos != nil {
		if pos.Status == "orbit" && pos.ObjectType == req.ObjectType && pos.ObjectID == req.ObjectID {
			writeJSONError(w, "Вы уже на орбите этого объекта", http.StatusBadRequest)
			return
		}
		if pos.Status == "in_flight" && pos.FromType == req.ObjectType && pos.FromID == req.ObjectID {
			writeJSONError(w, "Вы уже на орбите этого объекта", http.StatusBadRequest)
			return
		}
	}

	// From: объект позиции (покой) или объект отправления активного полёта
	// (редирект от объекта отправления, §3.5); NULL-позиция (легаси) —
	// «орбита звезды».
	fromType, fromID := "star", worldID
	if pos != nil {
		if pos.Status == "orbit" {
			fromType, fromID = pos.ObjectType, pos.ObjectID
		} else if pos.Status == "in_flight" {
			fromType, fromID = pos.FromType, pos.FromID
		}
	}

	// Длительность: dist = |r(from) − r(to)|, двухрежимная формула (С4).
	fromR, ok1 := objectRadiusAU(world, planets, fromType, fromID)
	toR, ok2 := objectRadiusAU(world, planets, req.ObjectType, req.ObjectID)
	if !ok1 || !ok2 {
		writeJSONError(w, "Объект не найден в вашей системе", http.StatusBadRequest)
		return
	}
	dist := math.Abs(fromR - toR)
	duration := CalcIntraDuration(dist, ship.EngineSpeed(user.Equipment))

	// Атомарный старт (С-1): строка полёта + current_position = in_flight
	// одной транзакцией — позиция и таблица не расходятся.
	now := time.Now()
	arriveAt := now.Add(duration)
	flight := models.PlayerIntrasystemFlight{
		UserID:    userID,
		WorldID:   worldID,
		FromType:  fromType,
		FromID:    fromID,
		ToType:    req.ObjectType,
		ToID:      req.ObjectID,
		StartTime: now,
		ArriveAt:  arriveAt,
	}
	if err := h.intraRepo.StartAtomic(flight, models.InFlightPosition(fromType, fromID, req.ObjectType, req.ObjectID, now, arriveAt)); err != nil {
		log.Printf("⚠️ intrasystem: StartAtomic (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось начать полёт", http.StatusInternalServerError)
		return
	}

	h.intraManager.StartIntraFlight(userID, worldID, fromType, fromID, req.ObjectType, req.ObjectID, duration,
		NewIntraArrivalHandler(h.intraRepo, h.planetRepo, h.knowledgeRepo))

	writeJSONStatus(w, http.StatusAccepted, IntraFlightResponse{
		Duration:  int(duration.Seconds()),
		FromType:  fromType,
		FromID:    fromID,
		ToType:    req.ObjectType,
		ToID:      req.ObjectID,
		StartTime: now.UnixMilli(),
		ArriveAt:  arriveAt.UnixMilli(),
	})
}