package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

type TravelHandlers struct {
	worldRepo     *repository.WorldRepository
	userRepo      *repository.UserRepository
	travelManager *travel.Manager
	// Внутрисистемные полёты (спека 99.2.27 §4.2, С1): старт межзвёздного
	// отменяет активный внутрисистемный полёт и NULL-ит позицию.
	intraManager *travel.IntrasystemManager
	intraRepo    *repository.PlayerIntrasystemFlightRepository
	// Автостарт композитного маршрута (спека 99.2.30 §4.3): onArrival
	// межзвёздного полёта сам запускает внутрисистемный сегмент (НЕ через
	// POST-хендлер — у POST известен баг битого from). Нужны planetRepo
	// (валидация «объект жив») и knowledgeRepo (авто-знание presence).
	planetRepo    *repository.PlanetRepository
	knowledgeRepo *repository.KnowledgeRepository
	// Контракты-перелёты (спека перелёта §1.1, B2a): закрытие по прибытии
	// межзвёздного полёта к звезде (цель-система, dest_planet_id IS NULL).
	// Сеттер: contractRepo создаётся в main.go.
	contractRepo *repository.ContractRepository
}

func NewTravelHandlers(
	worldRepo *repository.WorldRepository,
	userRepo *repository.UserRepository,
	travelManager *travel.Manager,
) *TravelHandlers {
	return &TravelHandlers{
		worldRepo:     worldRepo,
		userRepo:      userRepo,
		travelManager: travelManager,
	}
}

// SetIntrasystem — подключает внутрисистемные полёты (С1-дельта /travel).
// Сеттер (не параметр конструктора): менеджер создаётся позже в main.go.
func (h *TravelHandlers) SetIntrasystem(intraManager *travel.IntrasystemManager, intraRepo *repository.PlayerIntrasystemFlightRepository) {
	h.intraManager = intraManager
	h.intraRepo = intraRepo
}

// SetIntrasystemAutostart — подключает репозитории автостарта композитного
// маршрута (спека 99.2.30 §4.3): planetRepo (валидация «объект жив»),
// knowledgeRepo (авто-знание presence при прибытии к планете, 99.2.27 §3.6).
// Сеттер: planetRepo/knowledgeRepo создаются позже в main.go.
func (h *TravelHandlers) SetIntrasystemAutostart(planetRepo *repository.PlanetRepository, knowledgeRepo *repository.KnowledgeRepository) {
	h.planetRepo = planetRepo
	h.knowledgeRepo = knowledgeRepo
}

// SetContracts — подключает репозиторий контрактов (спека перелёта §1.1, B2a):
// закрытие контрактов-перелётов по прибытии (межзвёздная точка — цель-система,
// внутрисистемная — цель-планета). Сеттер: contractRepo создаётся в main.go.
func (h *TravelHandlers) SetContracts(contractRepo *repository.ContractRepository) {
	h.contractRepo = contractRepo
}

type TravelRequest struct {
	WorldID string `json:"world_id"`
	// Спека 99.2.30 §3.1: необязательная цель композитного маршрута —
	// объект системы назначения (планета/спутник/компаньон). Отсутствует =
	// обычный «Перелететь» к звезде (намерение очищается, M1).
	Destination *TravelDestination `json:"destination"`
}

// TravelDestination — цель композитного маршрута (спека 99.2.30 §2.2):
// object_type ∈ {planet, satellite, companion, belt} (компаньон — решение
// создателя 2026-09-21; пояс — решение создателя 2026-09-22, спека поясов
// этап 2 §5.7; object_id — синтетический id companion:<world> / extra:<world>:<i>
// для компаньона, system_belts.id для пояса).
type TravelDestination struct {
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
}

type TravelResponse struct {
	TravelID  string  `json:"travel_id"`
	Duration  int     `json:"duration"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	StartX    float64 `json:"start_x"` // стартовая точка сегмента (61a)
	StartY    float64 `json:"start_y"`
	StartTime int64   `json:"start_time"` // UnixMilli из фактического полёта
}

// calcTravelDuration вычисляет длительность полёта по расстоянию между мирами:
// dist * speedFactor секунд, минимум 3 секунды (решение создателя 2026-09-16;
// потолок 20 секунд от 2026-09-14 убран). speedFactor — скорость из
// установленного двигателя игрока (спека 91a §7.1: ship.EngineSpeed, 0.3 —
// значение 66a не меняется, меняется источник). Формула — общий
// models.TravelDuration: её же использует полёт NPC (npc.FlightDuration, свой
// npcSpeedFactor) и срок/окно перелёта; NPC летит по своему пути (не через
// travel.Manager/ArrivalHandler), общий здесь только расчёт длительности.
func calcTravelDuration(dist float64, speedFactor float64) time.Duration {
	return models.TravelDuration(dist, speedFactor)
}

// redirectStartPoint — стартовая точка нового полёта при редиректе (61a):
// точка P на отрезке from→to по прогрессу старого полёта (зажат 0..1).
// elapsed < 0 или duration <= 0 → P = from; elapsed >= duration → P = to.
func redirectStartPoint(fromX, fromY, toX, toY float64, elapsed, duration time.Duration) (float64, float64) {
	progress := 0.0
	if duration > 0 {
		progress = float64(elapsed) / float64(duration)
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return fromX + (toX-fromX)*progress, fromY + (toY-fromY)*progress
}

// destinationInSystem — объект принадлежит системе (спека 99.2.30 §3.1):
// планета с world_id == worldID; спутник в satellites планеты этой системы;
// компаньон — по stellar_mods системы (главный companion / внешний extra,
// IsValidCompanionID 99.2.27 §3.1); пояс — запись system_belts системы (спека
// поясов этап 2 §5.7). Источник planet/satellite — GetPlanetsLightByWorldID
// (тот же, что модалка 99.2.27 §4.4); компаньона — stellar_mods мира
// (world == nil → невалиден); пояса — GetBeltsByWorldID.
func destinationInSystem(world *models.World, planets []models.Planet, belts []models.Belt, objType, objID string) bool {
	switch objType {
	case "planet":
		for _, p := range planets {
			if p.ID == objID {
				return true
			}
		}
	case "satellite":
		for _, p := range planets {
			for _, s := range p.Satellites {
				if s.ID == objID {
					return true
				}
			}
		}
	case "companion":
		return IsValidCompanionID(world, objID)
	case "belt":
		for _, b := range belts {
			if b.ID == objID {
				return true
			}
		}
	}
	return false
}

// validateTravelDestination — валидация destination (спека 99.2.30 §3.1):
// object_type ∈ {planet, satellite, companion, belt} (пояс — решение создателя
// 2026-09-22, спека поясов этап 2 §5.7), object_id непустой, объект принадлежит
// системе world_id. Компаньон валиден, только если есть в stellar_mods системы
// world_id (IsValidCompanionID), — иначе 400 «Объект не найден в системе
// назначения». Источник planet/satellite — планетный список, компаньона — мир
// (worldRepo.GetByID), пояса — GetBeltsByWorldID. Битая цель (перегенерация
// между модалкой и кликом) → честный 400 — модалка обновится по refreshPlanets.
func (h *TravelHandlers) validateTravelDestination(worldID string, dest *TravelDestination) error {
	if dest.ObjectType != "planet" && dest.ObjectType != "satellite" && dest.ObjectType != "companion" && dest.ObjectType != "belt" {
		return errors.New("Некорректный тип объекта назначения")
	}
	if dest.ObjectID == "" {
		return errors.New("Некорректный объект назначения")
	}
	// Компаньон — объект системы: валидность определяется stellar_mods системы
	// (планетный список не нужен); planet/satellite/belt — списками системы.
	var world *models.World
	var planets []models.Planet
	var belts []models.Belt
	if dest.ObjectType == "companion" {
		if h.worldRepo == nil {
			return errors.New("Не удалось загрузить систему назначения")
		}
		w, err := h.worldRepo.GetByID(worldID)
		if err != nil || w == nil {
			return errors.New("Не удалось загрузить систему назначения")
		}
		world = w
	} else {
		if h.planetRepo == nil {
			return errors.New("Не удалось загрузить систему назначения")
		}
		ps, err := h.planetRepo.GetPlanetsLightByWorldID(worldID)
		if err != nil {
			return errors.New("Не удалось загрузить систему назначения")
		}
		planets = ps
		if dest.ObjectType == "belt" {
			bs, err := h.planetRepo.GetBeltsByWorldID(worldID)
			if err != nil {
				return errors.New("Не удалось загрузить систему назначения")
			}
			belts = bs
		}
	}
	if !destinationInSystem(world, planets, belts, dest.ObjectType, dest.ObjectID) {
		return errors.New("Объект не найден в системе назначения")
	}
	return nil
}

func (h *TravelHandlers) StartTravel(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req TravelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.WorldID == "" {
		http.Error(w, "world_id required", http.StatusBadRequest)
		return
	}

	targetWorld, err := h.worldRepo.GetByID(req.WorldID)
	if err != nil || targetWorld == nil {
		http.Error(w, "World not found", http.StatusNotFound)
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Валидация двигателя (спека 91a §6.1): role=player без установленного
	// двигателя не летает («не может летать без двигателя» — создатель);
	// админ/skycomposer — исключение (летают всегда, как видимость 77a).
	// NPC-агенты не затрагиваются — у них своя настройка npcSpeedFactor.
	if user.Role == models.RolePlayer && !ship.HasEngine(user.Equipment) {
		writeJSONError(w, "Двигатель не установлен — полёт невозможен", http.StatusBadRequest)
		return
	}

	// Спека 99.2.30 §3.1: валидация destination (только если поле присутствует;
	// порядок после существующих валидаций двигателя/цели). Намерение
	// композитного маршрута строится из валидного destination.
	var dest *models.PendingDestination
	if req.Destination != nil {
		if err := h.validateTravelDestination(req.WorldID, req.Destination); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		dest = &models.PendingDestination{
			WorldID:    req.WorldID,
			ObjectType: req.Destination.ObjectType,
			ObjectID:   req.Destination.ObjectID,
		}
	}

	fromWorldID := ""
	if user.CurrentWorldID != nil {
		fromWorldID = *user.CurrentWorldID
		// Битый current_world_id (мир удалён при перегенерации вселенной) —
		// сбрасываем и берём первый доступный мир.
		fromWorld, err := h.worldRepo.GetByID(fromWorldID)
		if err != nil || fromWorld == nil {
			fromWorldID = ""
		}
	}
	if fromWorldID == "" {
		worlds, err := h.worldRepo.GetAll()
		if err != nil || len(worlds) == 0 {
			http.Error(w, "No worlds available", http.StatusInternalServerError)
			return
		}
		fromWorldID = worlds[0].ID
		// Возвращаем пользователю валидный текущий мир.
		_ = h.userRepo.UpdateCurrentWorld(userID, fromWorldID)
	}

	// Идемпотентность: повторный запрос той же цели во время полёта не
	// перезапускает полёт — возвращаем текущий без сброса прогресса.
	if flight := h.travelManager.GetFlight(userID); flight != nil && flight.ToWorld == req.WorldID {
		// Спека 99.2.30 §3.5 (M2): намерение пишется/очищается и на
		// 202-идемпотентном пути — иначе игрок прилетит к звезде, хотя
		// кликнул «Лететь» (с destination), либо намерение {X,P} висит
		// весь полёт к X (без destination — игрок явно «перелетел» к звезде).
		// Полёт не перезапускается (202, без сброса прогресса).
		if err := h.userRepo.SetPendingDestination(userID, dest); err != nil {
			log.Printf("⚠️ travel: set pending destination (user %s): %v", userID, err)
		}
		resp := TravelResponse{
			TravelID:  userID + "-" + time.Now().Format("20060102150405"),
			Duration:  int(flight.Duration.Seconds()),
			From:      flight.FromWorld,
			To:        flight.ToWorld,
			StartX:    flight.StartX,
			StartY:    flight.StartY,
			StartTime: flight.StartTime.UnixMilli(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Полёт к любой звезде разрешён (спека 77a §7.3, решение создателя
	// 2026-09-17): валидация цели — только существование мира (404 выше);
	// «слепой прыжок» отменён, знание координат для полёта не требуется (И6).
	// Топливо/дальность — будущее ограничение (задел, не реализуется).

	fromWorld, err := h.worldRepo.GetByID(fromWorldID)
	if err != nil || fromWorld == nil {
		http.Error(w, "Current world not found", http.StatusInternalServerError)
		return
	}

	// Стартовая точка сегмента: при обычном старте — координаты FromWorld;
	// при редиректе (активный полёт, новая цель) — текущая точка P маршрута
	// (61a, решение создателя 2026-09-16: корабль не отскакивает к стартовой
	// звезде, а продолжает из текущей точки).
	startX, startY := fromWorld.CoordX, fromWorld.CoordY
	if flight := h.travelManager.GetFlight(userID); flight != nil {
		// Координаты цели старого полёта. Если мир удалён при перегенерации
		// вселенной — фолбэк на текущую позицию (flight.StartX/StartY).
		toX, toY := flight.StartX, flight.StartY
		if oldTo, err := h.worldRepo.GetByID(flight.ToWorld); err == nil && oldTo != nil {
			toX, toY = oldTo.CoordX, oldTo.CoordY
		}
		// Точка P считается от фактической стартовой точки предыдущего
		// сегмента (flight.StartX/StartY), а не от мира отправления A —
		// иначе при каскадных редиректах корабль «перескакивал» на линию
		// A→новая цель (баг создателя 2026-09-16).
		startX, startY = redirectStartPoint(
			flight.StartX, flight.StartY, toX, toY,
			time.Since(flight.StartTime), flight.Duration,
		)
	}

	// «Already in this world» — только без полёта (66a): при активном полёте
	// выбор мира отправления обрабатывается редиректом выше (разворот из
	// текущей точки P), а не 400.
	if flight := h.travelManager.GetFlight(userID); flight == nil && fromWorldID == req.WorldID {
		// Композитный запрос (destination) в СВОЮ систему (баг 2026-09-22):
		// игрок уже здесь, цель внутри этой же системы — это внутрисистемный
		// полёт, а не «Already in this world». Понятная деградация вместо
		// молчаливой ошибки: клиент после фикса шлёт такой запрос только при
		// рассинхроне, ответ подсказывает корректный путь.
		if dest != nil {
			http.Error(w, "Вы уже в этой системе — используйте внутрисистемный полёт", http.StatusBadRequest)
			return
		}
		http.Error(w, "Already in this world", http.StatusBadRequest)
		return
	}

	dx := startX - targetWorld.CoordX
	dy := startY - targetWorld.CoordY
	dist := math.Sqrt(dx*dx + dy*dy)

	// Скорость полёта — из установленного двигателя (спека 91a §7.1):
	// значение 0.3 (66a) не меняется, меняется источник (замысел 77a §1.3).
	duration := calcTravelDuration(dist, ship.EngineSpeed(user.Equipment))

	// Прибытие межзвёздного полёта (спека 99.2.27 §3.6.3/ИП-2 + 99.2.30 §4):
	// общий ArrivalHandler — current_world_id + позиция «орбита звезды» одним
	// UPDATE (С-1), затем автостарт композитного маршрута по намерению. Тот же
	// колбэк используется Restore-фазой 1 в main.go (И6: автостарт работает и
	// для восстановленного после рестарта полёта).

	// С1 (спека 99.2.27 §4.2): старт межзвёздного полёта отменяет активный
	// внутрисистемный полёт и NULL-ит позицию (игрок покидает систему).
	// Атомарность: отмена intra + позиция NULL — одной транзакцией
	// (CancelAtomic), не остаётся окна, где intra отменён, а позиция ещё нет.
	// Дельта 99.2.30 §3.2/§3.3 (ИН-4): в той же транзакции пишется/очищается
	// намерение композитного маршрута (CancelAtomicWithDestination) — dest
	// != nil → запись атомарно со стартом; dest == nil → очистка (M1).
	// Порядок: СНАЧАЛА транзакция (строка + позиция NULL + намерение), ПОТОМ
	// in-memory отмена — при краше в окне между ними позиция уже NULL (не
	// in_flight без строки полёта).
	if h.intraRepo != nil {
		if err := h.intraRepo.CancelAtomicWithDestination(userID, dest); err != nil {
			log.Printf("⚠️ travel: cancel intrasystem (user %s): %v", userID, err)
		}
	}
	if h.intraManager != nil {
		h.intraManager.CancelIntraFlight(userID)
	}

	h.travelManager.StartFlight(userID, fromWorldID, req.WorldID, startX, startY, duration, h.ArrivalHandler)

	newFlight := h.travelManager.GetFlight(userID)
	resp := TravelResponse{
		TravelID:  userID + "-" + time.Now().Format("20060102150405"),
		Duration:  int(newFlight.Duration.Seconds()),
		From:      fromWorldID,
		To:        req.WorldID,
		StartX:    newFlight.StartX,
		StartY:    newFlight.StartY,
		StartTime: newFlight.StartTime.UnixMilli(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

// ArrivalHandler — общий обработчик прибытия межзвёздного полёта (спека
// 99.2.27 §3.6.3/ИП-2 + 99.2.30 §4.1): current_world_id + current_position =
// «орбита звезды» одним UPDATE (С-1), затем чтение намерения композитного
// маршрута: NULL → обычное прибытие; world_id != worldID → очистить
// (дефенсив); world_id == worldID → автостарт внутрисистемного сегмента
// (§4.3). Общий для live-onArrival (StartTravel) и Restore-колбэка фазы 1
// (main.go) — автостарт работает и для восстановленного после рестарта
// межзвёздного полёта (И6, §7.2 DoD «рестарт → автостарт по прибытии»).
func (h *TravelHandlers) ArrivalHandler(uid, worldID string) {
	// Дефенсив (пакман, спека 2026-09-20 §7.2): цель съедена между
	// запросом и прибытием — обнуляем current_world_id/current_position
	// вместо FK-violation (users.current_world_id → worlds NO ACTION).
	w, err := h.worldRepo.GetByID(worldID)
	if err != nil || w == nil {
		if err := h.userRepo.ClearCurrentWorld(uid); err != nil {
			log.Printf("Failed to clear current world for user %s: %v", uid, err)
		}
		// ИН-3е (спека 99.2.30 §4.2): мир съеден — намерение тоже
		// очищается (страховка от гонки «пакман съел между запросом
		// и прибытием»; основную очистку делает пакман, §5).
		if err := h.userRepo.ClearPendingDestination(uid); err != nil {
			log.Printf("Failed to clear pending destination for user %s: %v", uid, err)
		}
		return
	}
	if err := h.userRepo.UpdateCurrentWorldAndPosition(uid, worldID, models.StarOrbitPosition(worldID)); err != nil {
		log.Printf("Failed to update current world for user %s: %v", uid, err)
	}
	// Контракт-перелёт с целью-системой (dest_planet_id IS NULL) закрывается
	// здесь (спека перелёта §1.1, B2a). Цель-планета закрывается во
	// внутрисистемной точке (NewIntraArrivalHandler) — иначе игрок, летящий к
	// планете композитным маршрутом, закрыл бы контракт на звезде раньше срока.
	h.closeSystemArrivalContracts(uid, worldID)
	// Спека 99.2.30 §4.1: чтение намерения ПОСЛЕ ИП-2 (позиция «орбита
	// звезды» уже выставлена — фолбэк автостарта готов).
	dest, err := h.userRepo.GetPendingDestination(uid)
	if err != nil {
		log.Printf("Failed to read pending destination for user %s: %v", uid, err)
		return
	}
	if dest == nil {
		return // NULL → обычное прибытие (ничего не меняется)
	}
	if dest.WorldID != worldID {
		// Дефенсив (не должно случаться — намерение пишется только для
		// цели полёта): очистить намерение.
		if err := h.userRepo.ClearPendingDestination(uid); err != nil {
			log.Printf("Failed to clear pending destination for user %s: %v", uid, err)
		}
		return
	}
	// world_id == worldID → автостарт внутрисистемного сегмента (§4.3).
	h.autostartIntra(uid, worldID, dest)
}

// closeSystemArrivalContracts — закрытие контрактов-перелётов с целью-системой
// при прибытии к звезде (спека перелёта §1.1, B2a). Nil-безопасно.
func (h *TravelHandlers) closeSystemArrivalContracts(uid, worldID string) {
	closeTravelContractsForArrival(h.contractRepo, uid, worldID, "")
}

// closeTravelContractsForArrival — общая точка закрытия контрактов-перелётов по
// прибытии (спека §1.1/§1.4): planetID == "" — цель-система (межзвёздная
// точка), иначе цель-планета (внутрисистемная). Nil-безопасно.
func closeTravelContractsForArrival(contractRepo *repository.ContractRepository, executorID, worldID, planetID string) {
	if contractRepo == nil {
		return
	}
	n, err := contractRepo.CloseTravelArrivals(executorID, worldID, planetID)
	if err != nil {
		log.Printf("⚠️ contracts: закрытие перелёта (executor %s, world %s, planet %s): %v",
			executorID, worldID, planetID, err)
		return
	}
	if n > 0 {
		log.Printf("✅ contracts: закрыто перелётов: %d (executor %s, world %s, planet %s)",
			n, executorID, worldID, planetID)
	}
}

// ==================== АВТОСТАРТ КОМПОЗИТНОГО МАРШРУТА (спека 99.2.30 §4) ====================

// autostartIntra — автостарт внутрисистемного сегмента композитного маршрута
// (спека 99.2.30 §4.3/§4.4): серверный запуск по onArrival межзвёздного полёта.
// НЕ через POST /api/intrasystem-flight (у POST известен баг — битый from без
// ИП-4-фолбэка, идея модалки §8 п.9; у автостарта from всегда = звезда
// прибытия, живая). Валидация ДО старта:
//  1. Объект жив: planet/satellite → arrivalTargetValid; companion →
//     IsValidCompanionID (stellar_mods системы). Битая цель → фолбэк
//     «орбита звезды» (позиция уже выставлена onArrival), намерение
//     очищается, БЕЗ 400;
//  2. Двигатель (91a §6.1): role=player без ship.HasEngine → автостарт НЕ
//     запускается, намерение очищается, позиция остаётся «орбита звезды»
//     (иначе намерение-призрак висит до следующего /travel). Админ/skycomposer
//     — исключение;
//  3. Межзвёздный полёт завершён (мы в onArrival) — внутрисистемный старт не
//     конфликтует (И3 99.2.27).
//
// Старт (паттерн 99.2.27 §3.2/§4.1, атомарность С-1): StartAtomic (строка +
// current_position = in_flight) + StartIntraFlight с NewIntraArrivalHandler
// (авто-знание presence при прибытии к планете, 99.2.27 §3.6 — повторно не
// реализуется). Длительность — CalcIntraDuration от звезды (позиция прибытия
// = орбита звезды; dist = |0 − r(объекта)|, §3.4 99.2.27).
//
// Очистка намерения (§4.4): в любом исходе (успех, битая цель, двигатель
// снят) — UPDATE pending_destination = NULL; после StartAtomic (окно «intra
// активен + намерение ещё стоит» — микроскопическое, закрыто фазой 3в
// Restore-обработки §4.5; ИН-1 допускает это окно).
func (h *TravelHandlers) autostartIntra(uid, worldID string, dest *models.PendingDestination) {
	clearDest := func() {
		if err := h.userRepo.ClearPendingDestination(uid); err != nil {
			log.Printf("⚠️ travel: clear pending destination (user %s): %v", uid, err)
		}
	}
	// 1. Мир системы назначения — нужен для валидации компаньона
	// (IsValidCompanionID) и радиуса цели (objectRadiusAU); грузим ДО
	// валидации цели (спека 99.2.30 §4.3).
	world, err := h.worldRepo.GetByID(worldID)
	if err != nil || world == nil {
		clearDest()
		return
	}
	// 2. Объект жив (валидация §4.3.1): компаньон — по stellar_mods системы
	// (главный companion / внешний extra); планета/спутник/пояс —
	// arrivalTargetValid (пояс — запись system_belts, спека поясов этап 2 §5.7).
	// Битая цель → фолбэк «орбита звезды» (позиция уже выставлена), намерение
	// очищается, БЕЗ 400.
	targetValid := false
	if dest.ObjectType == "companion" {
		targetValid = IsValidCompanionID(world, dest.ObjectID)
	} else {
		targetValid = h.planetRepo != nil && arrivalTargetValid(h.planetRepo, worldID, dest.ObjectType, dest.ObjectID)
	}
	if !targetValid {
		clearDest()
		return
	}
	// 3. Двигатель (91a §6.1): role=player без двигателя → автостарт НЕ
	// запускается, намерение очищается, позиция остаётся «орбита звезды».
	user, err := h.userRepo.GetByID(uid)
	if err != nil || user == nil {
		clearDest()
		return
	}
	if user.Role == models.RolePlayer && !ship.HasEngine(user.Equipment) {
		clearDest()
		return
	}
	// Согласованность (спека 99.2.30 §4.5 фаза 3а, ИП-1 99.2.27): автостарт
	// корректен, только если игрок уже в системе-цели — current_world_id ==
	// worldID (прибытие засчитано onArrival до краша). Иначе полёт фактически
	// не стартовал (краш между CancelAtomicWithDestination и StartFlight
	// оставил намерение при current_world_id мира отправления) — намерение
	// осиротело: очистить, позицию не трогать (игрок остаётся в мире
	// отправления без полёта).
	if user.CurrentWorldID == nil || *user.CurrentWorldID != worldID {
		clearDest()
		return
	}
	// 4. Длительность от звезды (позиция прибытия = орбита звезды).
	if h.planetRepo == nil {
		clearDest()
		return
	}
	planets, err := h.planetRepo.GetPlanetsLightByWorldID(worldID)
	if err != nil {
		clearDest()
		return
	}
	// Пояса мира — для радиуса цели-belt (спека поясов этап 2 §5.2/§5.7).
	belts, err := h.planetRepo.GetBeltsByWorldID(worldID)
	if err != nil {
		clearDest()
		return
	}
	// Компаньон во внутрисистемном слое — 'star' + синтетический id (99.2.27
	// §3.1): таблица player_intrasystem_flights / current_position / onArrival
	// ждут ToType='star' (иначе позиция и прибытие не поймут цель). Пояс —
	// без трансляции: внутрисистемный слой понимает 'belt' напрямую (§5.7).
	intraToType := dest.ObjectType
	if intraToType == "companion" {
		intraToType = "star"
	}
	toR, ok := objectRadiusAU(world, planets, belts, intraToType, dest.ObjectID)
	if !ok {
		clearDest()
		return
	}
	duration := CalcIntraDuration(math.Abs(toR), ship.EngineSpeed(user.Equipment))
	now := time.Now()
	arriveAt := now.Add(duration)
	flight := models.PlayerIntrasystemFlight{
		UserID:    uid,
		WorldID:   worldID,
		FromType:  "star",
		FromID:    worldID,
		ToType:    intraToType,
		ToID:      dest.ObjectID,
		StartTime: now,
		ArriveAt:  arriveAt,
	}
	// Старт (атомарность С-1): строка полёта + current_position = in_flight
	// одной транзакцией — позиция и таблица не расходятся.
	if err := h.intraRepo.StartAtomic(flight, models.InFlightPosition("star", worldID, intraToType, dest.ObjectID, now, arriveAt)); err != nil {
		log.Printf("⚠️ travel: autostart StartAtomic (user %s): %v", uid, err)
		clearDest()
		return
	}
	h.intraManager.StartIntraFlight(uid, worldID, "star", worldID, intraToType, dest.ObjectID, duration,
		NewIntraArrivalHandler(h.intraRepo, h.planetRepo, h.knowledgeRepo, h.contractRepo))
	// 5. Очистка намерения ПОСЛЕ StartAtomic (§4.4).
	clearDest()
}

// RestorePendingDestinations — фаза 3 Restore-обработки (спека 99.2.30 §4.5,
// паттерн 97a/99.2.27): обработка намерений ПОСЛЕ Restore межзвёздных (фаза 1)
// и внутрисистемных (фаза 2 — сторона внутрисистемного менеджера, намерения
// не трогает). Для каждого игрока с pending_destination != NULL:
//
//	а) нет активного межзвёздного полёта к world_id И объект жив → автостарт
//	   (StartAtomic + StartIntraFlight; намерение — через ветку «исполнение»,
//	   §4.4); старт ПОСЛЕ внутрисистемного Restore — двойной регистрации нет;
//	б) нет активного межзвёздного полёта И объект бит / двигатель снят →
//	   очистить намерение, позиция «орбита звезды» (фолбэк §4.3);
//	в) нет активного межзвёздного полёта И уже есть активный внутрисистемный
//	   полёт в world_id → очистить намерение (микро-окно «StartAtomic → UPDATE
//	   NULL» после рестарта: исполнение уже произошло, намерение — призрак);
//	г) есть активный межзвёздный полёт к world_id → намерение живёт (автостарт
//	   по прибытии);
//	д) мир world_id съеден/удалён → очистить (в т.ч. дефенсив §4.2).
//
// Стоимость — O(игроки с намерением) — инвариант 2 проекта держится (И8).
func (h *TravelHandlers) RestorePendingDestinations() {
	if h.intraRepo == nil || h.intraManager == nil {
		return
	}
	users, err := h.userRepo.ListPendingDestinations()
	if err != nil {
		log.Printf("❌ travel: RestorePendingDestinations ListAll: %v", err)
		return
	}
	for _, u := range users {
		dest := u.PendingDestination
		if dest == nil {
			continue
		}
		// д) мир съеден/удалён → очистить.
		w, err := h.worldRepo.GetByID(dest.WorldID)
		if err != nil || w == nil {
			if err := h.userRepo.ClearPendingDestination(u.ID); err != nil {
				log.Printf("⚠️ travel: Restore clear (world eaten, user %s): %v", u.ID, err)
			}
			continue
		}
		// г) есть активный межзвёздный полёт к world_id → намерение живёт.
		if f := h.travelManager.GetFlight(u.ID); f != nil && f.ToWorld == dest.WorldID {
			continue
		}
		// в) уже есть активный внутрисистемный полёт в world_id → очистить
		// (намерение-призрак микро-окна StartAtomic→NULL).
		if f := h.intraManager.GetIntraFlight(u.ID); f != nil && f.WorldID == dest.WorldID {
			if err := h.userRepo.ClearPendingDestination(u.ID); err != nil {
				log.Printf("⚠️ travel: Restore clear (ghost, user %s): %v", u.ID, err)
			}
			continue
		}
		// а) объект жив → автостарт (двигатель/битая цель — внутри
		// autostartIntra, ветка б: очистка + позиция «орбита звезды»).
		h.autostartIntra(u.ID, dest.WorldID, dest)
	}
}
