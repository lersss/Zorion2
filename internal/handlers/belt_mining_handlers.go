// internal/handlers/belt_mining_handlers.go
//
// Добыча в поясе малых тел — мини-игра (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §5–§8): POST /api/belt/mine/enter —
// вход в заход (позиция-уровень mining + пакет захода §7); POST
// /api/belt/mine/collect — событие сбора (сервер-касса: клампы скорости/запаса/
// трюма, §5.3); POST /api/belt/mine/leave — выход (буфер захода в трюм через
// сервис internal/cargo, позиция → orbit/belt, §5.4). Состояние захода —
// аддитивные ключи users.current_position (как surface); запас пояса — колонка
// system_belts.iron_remaining (миграция 000069), инициализируется ЛЕНИВО при
// первом enter (решение менеджера, §4). Новых таблиц нет.
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"log"
	"net/http"
	"time"

	"zorion/internal/auth"
	"zorion/internal/cargo"
	"zorion/internal/goodsstudio/graph"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/travel"
)

// ==================== ЧИСЛА (§8.2) ====================

const (
	// beltIronRef — опорное железо «богатого» пояса (единица нормировки, §8.2).
	beltIronRef = 0.30
	// beltReserveBase — базовый запас пояса, т (reserve = beltReserveBase·k_iron).
	beltReserveBase = 600.0
	// beltRateBase — базовая скорость идеального игрока, т/с (§8.2).
	beltRateBase = 0.25
	// beltDtMax — предохранитель интервала Δt, с (§8.2): офлайн-разрыв не даёт
	// «мгновенный трюм».
	beltDtMax = 2.0
	// ironGoodName — ресурс-заглушка каталога (резолв по ТОЧНОМУ name_norm
	// «железо fe», §3.2/§11 п.4; id не хардкодится).
	ironGoodName = "Железо Fe"
	// waterGoodName — второй ресурс пояса (лёд → «Вода неочищенная», спека
	// 2026-09-24 §2.3/§6): резолв по ТОЧНОМУ name_norm «вода неочищенная»,
	// kind='resource'. НЕ «вода-ресурс» (залежи) и НЕ «Вода H₂O».
	waterGoodName = "Вода неочищенная"

	// beltIceRef — опорный лёд «богатого ледяного» пояса (спека 2026-09-24 §5.1).
	beltIceRef = 0.50
	// beltIceReserve — запас льда на пояс, т (ВРЕМЕННО плоско — на тест, §4/§18 п.1).
	beltIceReserve = 20000.0
	// beltIceRateBase — базовая скорость идеального игрока по льду, т/с (§5.1).
	beltIceRateBase = 0.667
)

// errIronGoodNotFound — ресурс «Железо Fe» отсутствует в каталоге.
var errIronGoodNotFound = errors.New("ресурс «Железо Fe» не найден в каталоге")

// errWaterGoodNotFound — ресурс «Вода неочищенная» отсутствует в каталоге
// (фолбэк: resource_ice = null, ледяных жил нет, железо работает, §5.2).
var errWaterGoodNotFound = errors.New("ресурс «Вода неочищенная» не найден в каталоге")

// beltKIron — множитель пояса: k_iron = composition.iron / iron_ref (§8.2).
func beltKIron(iron float64) float64 { return iron / beltIronRef }

// beltReserve — запас пояса: reserve = 600 · k_iron, т (§8.2).
func beltReserve(iron float64) float64 { return beltReserveBase * beltKIron(iron) }

// beltRateCap — потолок сервера: R_cap = 2·R_base·k_iron, т/с (§8.2): читер
// ≤ 2× идеального игрока на любом поясе.
func beltRateCap(iron float64) float64 { return 2 * beltRateBase * beltKIron(iron) }

// beltKIce — множитель пояса по льду: k_ice = composition.ice / ice_ref (§5.1).
func beltKIce(ice float64) float64 { return ice / beltIceRef }

// beltIceRate — РЕАЛЬНО НАКАПЛИВАЕМАЯ скорость по льду, т/с (§5.1): её копит
// клиент, она уходит в пакет как limits.rate_ice и даёт цель 2–3 мин. НЕ потолок.
func beltIceRate(ice float64) float64 { return beltIceRateBase * beltKIce(ice) }

// beltIceRateCap — СЕРВЕРНЫЙ ПОТОЛОК по льду, т/с (§5.1): анти-абьюз (читер
// ≤ 2× идеала); в пакет НЕ выводится — живёт только в клампе collect.
func beltIceRateCap(ice float64) float64 { return 2 * beltIceRateBase * beltKIce(ice) }

// beltIceClassByIce — класс пояса по льду (§5.1): богатый k_ice > 0.60 /
// средний 0.25 < k_ice ≤ 0.60 / бедный k_ice ≤ 0.25.
func beltIceClassByIce(ice float64) string {
	k := beltKIce(ice)
	switch {
	case k > 0.60:
		return "богатый"
	case k > 0.25:
		return "средний"
	default:
		return "бедный"
	}
}

// beltClassByIron — класс пояса по k_iron (§8.4-а): богатый k > 0.60 /
// средний 0.25 < k ≤ 0.60 / бедный k ≤ 0.25.
func beltClassByIron(iron float64) string {
	k := beltKIron(iron)
	switch {
	case k > 0.60:
		return "богатый"
	case k > 0.25:
		return "средний"
	default:
		return "бедный"
	}
}

// beltRemainingLevel — уровень остатка по f = remaining/reserve (§8.4-б):
// полный f > 0.60 / истощается 0 < f ≤ 0.60 / выработан f = 0.
func beltRemainingLevel(remaining, reserve float64) string {
	if reserve <= 0 || remaining <= 0 {
		return "выработан"
	}
	if remaining/reserve > 0.60 {
		return "полный"
	}
	return "истощается"
}

// beltGranted — сколько зачесть из присланного amount (§5.3): потолок скорости
// R_cap·Δ, запас пояса, свободный трюм с учётом буфера (mined + granted ≤ free).
func beltGranted(amount, rateCap, dt, reserve, freeMinusMined float64) float64 {
	if amount <= 0 {
		return 0
	}
	g := amount
	if cap := rateCap * dt; g > cap {
		g = cap
	}
	if g > reserve {
		g = reserve
	}
	if g > freeMinusMined {
		g = freeMinusMined
	}
	if g < 0 {
		return 0
	}
	return g
}

// beltDeltaSeconds — Δ = min(now − last_collect_at, Δt_max) (§5.3.1). Пустое/
// битое last_collect_at → Δt_max (предохранитель).
func beltDeltaSeconds(lastCollectAt string, now time.Time) float64 {
	if lastCollectAt == "" {
		return beltDtMax
	}
	t, err := time.Parse(time.RFC3339, lastCollectAt)
	if err != nil {
		return beltDtMax
	}
	d := now.Sub(t).Seconds()
	if d < 0 {
		d = 0
	}
	if d > beltDtMax {
		d = beltDtMax
	}
	return d
}

// beltMiningSeed — детерминированный seed мира захода: crc32(belt_id + "|belt").
func beltMiningSeed(beltID string) uint32 {
	return crc32.ChecksumIEEE([]byte(beltID + "|belt"))
}

// resolveIronGood — резолв ресурса-заглушки по ТОЧНОМУ нормализованному имени
// каталога (graph.NormalizeName("Железо Fe") = "железо fe"), kind='resource'
// (§3.2/§11 п.4; образец — пилоты залежей admin_deposits.go). НЕ SQL lower().
func resolveIronGood(q rowQuerier) (int64, string, float64, error) {
	var id int64
	var name string
	var weight float64
	err := q.QueryRow(
		`SELECT id, name, weight FROM goods WHERE name_norm = $1 AND kind = 'resource'`,
		graph.NormalizeName(ironGoodName),
	).Scan(&id, &name, &weight)
	if err == sql.ErrNoRows {
		return 0, "", 0, errIronGoodNotFound
	}
	if err != nil {
		return 0, "", 0, err
	}
	return id, name, weight, nil
}

// resolveWaterGood — резолв ресурса «Вода неочищенная» по ТОЧНОМУ
// нормализованному имени каталога (graph.NormalizeName("Вода неочищенная") =
// "вода неочищенная"), kind='resource' (спека 2026-09-24 §2.3/§6; id не
// хардкодится). Отсутствие ресурса → errWaterGoodNotFound (фолбэк §5.2).
func resolveWaterGood(q rowQuerier) (int64, string, float64, error) {
	var id int64
	var name string
	var weight float64
	err := q.QueryRow(
		`SELECT id, name, weight FROM goods WHERE name_norm = $1 AND kind = 'resource'`,
		graph.NormalizeName(waterGoodName),
	).Scan(&id, &name, &weight)
	if err == sql.ErrNoRows {
		return 0, "", 0, errWaterGoodNotFound
	}
	if err != nil {
		return 0, "", 0, err
	}
	return id, name, weight, nil
}

// findBeltByID — пояс по id (nil, если нет).
func findBeltByID(belts []models.Belt, id string) *models.Belt {
	for i := range belts {
		if belts[i].ID == id {
			return &belts[i]
		}
	}
	return nil
}

// ==================== ТРЮМ (сервис internal/cargo) ====================

// BeltCargo — часть сервиса трюма, нужная добыче (интерфейс — для тестов).
type BeltCargo interface {
	View(userID string) (*cargo.View, error)
	Free(userID string) (float64, error)
	TryAddCargo(userID string, goodID int64, qty float64) (float64, error)
	TryAddCargoTx(tx *sql.Tx, userID string, goodID int64, qty float64) (float64, error)
}

// ==================== ХЕНДЛЕР ====================

// BeltMiningHandlers — ручки добычи в поясе.
type BeltMiningHandlers struct {
	db            *sql.DB
	userRepo      *repository.UserRepository
	worldRepo     *repository.WorldRepository
	planetRepo    *repository.PlanetRepository
	cargo         BeltCargo
	travelManager *travel.Manager
	intraManager  *travel.IntrasystemManager
}

func NewBeltMiningHandlers(
	db *sql.DB,
	userRepo *repository.UserRepository,
	worldRepo *repository.WorldRepository,
	planetRepo *repository.PlanetRepository,
	cargoSvc BeltCargo,
	travelManager *travel.Manager,
	intraManager *travel.IntrasystemManager,
) *BeltMiningHandlers {
	return &BeltMiningHandlers{
		db:            db,
		userRepo:      userRepo,
		worldRepo:     worldRepo,
		planetRepo:    planetRepo,
		cargo:         cargoSvc,
		travelManager: travelManager,
		intraManager:  intraManager,
	}
}

// ==================== ПАКЕТ ЗАХОДА (§7) ====================

// BeltMineResource — ресурс пояса в пакете (§7): id/имя/вес каталога.
type BeltMineResource struct {
	GoodID int64   `json:"good_id"`
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

// BeltMineCargo — трюм в пакете (§7): занято/всего/свободно (т).
type BeltMineCargo struct {
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
	Free  float64 `json:"free"`
}

// BeltMineLimits — пределы сбора (§7 + спека 2026-09-24 §5.1): rate_cap (т/с,
// железо — серверный потолок) и rate_ice (т/с, лёд — РЕАЛЬНО НАКАПЛИВАЕМАЯ
// скорость, не потолок), dt_max (с).
type BeltMineLimits struct {
	RateCap float64 `json:"rate_cap"`
	RateIce float64 `json:"rate_ice,omitempty"`
	DtMax   float64 `json:"dt_max"`
}

// BeltMineEnterResponse — пакет захода (§7 + спека 2026-09-24 §5.2) —
// единственный вход клиентского генератора: клиент не читает composition пояса
// сам и не знает точный запас. Поля льда аддитивны: resource_ice == nil, когда
// ресурса «Вода неочищенная» нет в каталоге (фолбэк §5.2).
type BeltMineEnterResponse struct {
	BeltID            string            `json:"belt_id"`
	BeltName          string            `json:"belt_name"`
	BeltKind          string            `json:"belt_kind"`
	Seed              uint32            `json:"seed"`
	CompositionIron   float64           `json:"composition_iron"`
	CompositionIce    float64           `json:"composition_ice,omitempty"`
	Resource          BeltMineResource  `json:"resource"`
	ResourceIce       *BeltMineResource `json:"resource_ice"`
	BeltClass         string            `json:"belt_class"`
	IceClass          string            `json:"ice_class,omitempty"`
	RemainingLevel    string            `json:"remaining_level"`
	RemainingLevelIce string            `json:"remaining_level_ice,omitempty"`
	StartedAt         string            `json:"started_at"`
	Mined             float64           `json:"mined"`
	MinedIce          *float64          `json:"mined_ice,omitempty"`
	Cargo             BeltMineCargo     `json:"cargo"`
	Limits            BeltMineLimits    `json:"limits"`
}

// resolveIceGood — фолбэк-резолв воды (§5.2): nil без ошибки, если ресурса нет
// в каталоге; прочие ошибки БД — наверх (500, как у железа).
func resolveIceGood(q rowQuerier) (*BeltMineResource, error) {
	id, name, weight, err := resolveWaterGood(q)
	if err != nil {
		if errors.Is(err, errWaterGoodNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &BeltMineResource{GoodID: id, Name: name, Weight: weight}, nil
}

// beltWaterResource — ресурс воды для пояса (nil, если льда в составе нет или
// ресурса нет в каталоге; §5.2). Единая точка: лёд считается «живым» только при
// успешном резолве — и в гейте входа, и в пакете захода.
func (h *BeltMiningHandlers) beltWaterResource(belt *models.Belt) (*BeltMineResource, error) {
	ice := 0.0
	if belt.Composition != nil {
		ice = belt.Composition["ice"]
	}
	if ice <= 0 {
		return nil, nil
	}
	return resolveIceGood(h.db)
}

// buildMinePackage собирает пакет захода (§7/§5.2) из пояса, позиции и уже
// резолвнутого ресурса воды (nil — ледяных жил нет).
func (h *BeltMiningHandlers) buildMinePackage(userID string, belt *models.Belt, pos *models.CurrentPosition, now time.Time, water *BeltMineResource) (BeltMineEnterResponse, error) {
	iron, ice := 0.0, 0.0
	if belt.Composition != nil {
		iron = belt.Composition["iron"]
		ice = belt.Composition["ice"]
	}
	goodID, goodName, weight, err := resolveIronGood(h.db)
	if err != nil {
		return BeltMineEnterResponse{}, err
	}
	view, err := h.cargo.View(userID)
	if err != nil {
		return BeltMineEnterResponse{}, err
	}
	used := view.Limits.Mass.Used
	total := view.Limits.Mass.Total
	free := total - used
	if free < 0 {
		free = 0
	}
	mined := 0.0
	if pos.Mined != nil {
		mined = *pos.Mined
	}
	minedIce := 0.0
	if pos.MinedIce != nil {
		minedIce = *pos.MinedIce
	}
	remainingLevel := ""
	if belt.IronRemaining != nil {
		remainingLevel = beltRemainingLevel(*belt.IronRemaining, beltReserve(iron))
	}
	beltClass := ""
	if iron > 0 {
		beltClass = beltClassByIron(iron)
	}
	iceClass := ""
	remainingLevelIce := ""
	rateIce := 0.0
	if water != nil {
		iceClass = beltIceClassByIce(ice)
		rateIce = beltIceRate(ice)
		if belt.IceRemaining != nil {
			remainingLevelIce = beltRemainingLevel(*belt.IceRemaining, beltIceReserve)
		}
	}
	resp := BeltMineEnterResponse{
		BeltID:            belt.ID,
		BeltName:          belt.Name,
		BeltKind:          belt.Kind,
		Seed:              beltMiningSeed(belt.ID),
		CompositionIron:   iron,
		CompositionIce:    ice,
		Resource:          BeltMineResource{GoodID: goodID, Name: goodName, Weight: weight},
		ResourceIce:       water,
		BeltClass:         beltClass,
		IceClass:          iceClass,
		RemainingLevel:    remainingLevel,
		RemainingLevelIce: remainingLevelIce,
		StartedAt:         pos.StartedAt,
		Mined:             mined,
		Cargo:             BeltMineCargo{Used: used, Total: total, Free: free},
		Limits:            BeltMineLimits{RateCap: beltRateCap(iron), RateIce: rateIce, DtMax: beltDtMax},
	}
	if water != nil {
		resp.MinedIce = &minedIce
	}
	return resp, nil
}

// ==================== ENTER (§6.2) ====================

// Enter — POST /api/belt/mine/enter. Валидации §6.2 строго по порядку:
// идемпотентность (шаг 2) до «на орбите пояса»; затем мир/полёт/позиция,
// запас (ленивая инициализация) и свободный трюм. Действие — одной транзакцией.
func (h *BeltMiningHandlers) Enter(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	var req struct {
		BeltID string `json:"belt_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.BeltID == "" {
		writeJSONError(w, "belt_id обязателен", http.StatusBadRequest)
		return
	}

	user, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	now := time.Now()

	// 2. Идемпотентность: уже добываем в этом поясе → та же сессия (mined/
	// started_at как хранятся), запас НЕ пере-инициализируется (§6.2 п.2).
	if pos != nil && pos.Status == "mining" && pos.ObjectID == req.BeltID && user.CurrentWorldID != nil {
		belt, err := h.planetRepo.GetBeltByID(req.BeltID)
		if err != nil {
			log.Printf("⚠️ belt mining: enter idempotent belt (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
			return
		}
		if belt != nil && belt.WorldID == *user.CurrentWorldID {
			// Идемпотентный вход — тот же гейт видимости, что у первого (п.6):
			// player не «возобновляет» заход в скрытом поясе.
			if roleFromContext(r) == string(models.RolePlayer) && !belt.Visible {
				writeJSONError(w, "Пояс не найден", http.StatusBadRequest)
				return
			}
			water, err := h.beltWaterResource(belt)
			if err != nil {
				writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
				return
			}
			pkg, err := h.buildMinePackage(userID, belt, pos, now, water)
			if err != nil {
				writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
				return
			}
			writeJSONStatus(w, http.StatusOK, pkg)
			return
		}
	}

	// 3. Игрок в системе, мир жив.
	if user.CurrentWorldID == nil {
		writeJSONError(w, "Вы не в системе", http.StatusBadRequest)
		return
	}
	world, err := h.worldRepo.GetByID(*user.CurrentWorldID)
	if err != nil || world == nil {
		writeJSONError(w, "Вы не в системе", http.StatusBadRequest)
		return
	}
	worldID := *user.CurrentWorldID

	// 4. Нет активного межзвёздного/внутрисистемного полёта.
	if (pos != nil && pos.Status == "in_flight") ||
		(h.intraManager != nil && h.intraManager.GetIntraFlight(userID) != nil) ||
		(h.travelManager != nil && h.travelManager.GetFlight(userID) != nil) {
		writeJSONError(w, "Вы в полёте", http.StatusBadRequest)
		return
	}

	// 5. Позиция = этот пояс.
	if pos == nil || pos.Status != "orbit" || pos.ObjectType != "belt" || pos.ObjectID != req.BeltID {
		writeJSONError(w, "Сначала долетите до пояса", http.StatusBadRequest)
		return
	}

	// 6–8 + действие одной транзакцией.
	tx, err := h.db.Begin()
	if err != nil {
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	belt, err := h.planetRepo.LockBeltForUpdate(tx, req.BeltID)
	if err != nil {
		log.Printf("⚠️ belt mining: enter lock (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}
	// 6. Пояс принадлежит системе; visible=true для player (как этап 2 §5.5 п.5).
	if belt == nil || belt.WorldID != worldID {
		writeJSONError(w, "Пояс не найден", http.StatusBadRequest)
		return
	}
	if roleFromContext(r) == string(models.RolePlayer) && !belt.Visible {
		writeJSONError(w, "Пояс не найден", http.StatusBadRequest)
		return
	}

	// 7. Есть хотя бы один добываемый ресурс (спека 2026-09-24 §5.4): состав
	// пуст (iron ≤ 0 и ice ≤ 0) → «нет данных о запасе». Запасы инициализируются
	// лениво ниже, поэтому «выработан» проверяется после инициализации.
	iron, ice := 0.0, 0.0
	if belt.Composition != nil {
		iron = belt.Composition["iron"]
		ice = belt.Composition["ice"]
	}
	if iron <= 0 && ice <= 0 {
		writeJSONError(w, "нет данных о запасе", http.StatusBadRequest)
		return
	}

	// Ресурс воды резолвится здесь: он определяет, «жив» ли лёд (гейт входа) и
	// идёт ли он в пакет (§5.2). Отсутствие ресурса — фолбэк (не 500).
	water, err := h.beltWaterResource(belt)
	if err != nil {
		log.Printf("⚠️ belt mining: enter resolve water (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}

	// 8. Есть свободный трюм.
	free, err := h.cargo.Free(userID)
	if err != nil {
		log.Printf("⚠️ belt mining: enter cargo (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}
	if free <= 0 {
		writeJSONError(w, "Трюм полон", http.StatusBadRequest)
		return
	}

	// Ленивая инициализация запасов (первое обращение, §4/§5.4): железо
	// reserve = 600·k_iron; лёд reserve = 20000 (ВРЕМЕННО плоско — на тест).
	if iron > 0 && belt.IronRemaining == nil {
		reserve := beltReserve(iron)
		if err := h.planetRepo.SetBeltIronRemaining(tx, req.BeltID, reserve); err != nil {
			log.Printf("⚠️ belt mining: enter init iron reserve (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
			return
		}
		v := reserve
		belt.IronRemaining = &v
	}
	if ice > 0 && belt.IceRemaining == nil {
		reserve := beltIceReserve
		if err := h.planetRepo.SetBeltIceRemaining(tx, req.BeltID, reserve); err != nil {
			log.Printf("⚠️ belt mining: enter init ice reserve (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
			return
		}
		v := reserve
		belt.IceRemaining = &v
	}

	// Оба ДОСТУПНЫХ запаса = 0 → «Пояс выработан» (§5.4). Пояс с железом = 0,
	// но живым льдом — входим (добывать лёд; старый гейт «железо = 0» смягчён).
	// Лёд «жив» только если ресурс воды резолвится (иначе ледяных жил нет, §5.2):
	// пояс с выработанным железом и недоступным льдом — «Пояс выработан».
	iceAvailable := ice > 0 && water != nil
	allDepleted := true
	if iron > 0 && (belt.IronRemaining == nil || *belt.IronRemaining > 0) {
		allDepleted = false
	}
	if iceAvailable && (belt.IceRemaining == nil || *belt.IceRemaining > 0) {
		allDepleted = false
	}
	if allDepleted {
		writeJSONError(w, "Пояс выработан", http.StatusBadRequest)
		return
	}

	newPos := models.MiningPosition(req.BeltID, now)
	posJSON, err := json.Marshal(newPos)
	if err != nil {
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(
		`UPDATE users SET current_position = $1, updated_at = NOW() WHERE id = $2`,
		posJSON, userID,
	); err != nil {
		log.Printf("⚠️ belt mining: enter update position (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}

	// Пакет собираем ДО коммита: сбой резолва ресурса («Железо Fe» нет в
	// каталоге) или чтения трюма откатывает позицию — игрок не остаётся в
	// mining без ресурса (иначе 500 уже после коммита позиции).
	pkg, err := h.buildMinePackage(userID, belt, newPos, now, water)
	if err != nil {
		log.Printf("⚠️ belt mining: enter build package (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSONError(w, "Не удалось открыть заход", http.StatusInternalServerError)
		return
	}

	writeJSONStatus(w, http.StatusOK, pkg)
}

// ==================== COLLECT (§5.3) ====================

// BeltCollectRequest — тело POST /api/belt/mine/collect: amount — сколько
// клиент «набрал» за интервал (по ощущениям/скорости бурения), не «итог»;
// resource — какой ресурс бурят (""/"iron" → железо, "ice" → лёд; спека
// 2026-09-24 §5.3). Отсутствие resource = старое поведение (железо).
type BeltCollectRequest struct {
	Amount   float64 `json:"amount"`
	Resource string  `json:"resource"`
}

// BeltCollectResponse — ответ сбора (§5.3 + спека 2026-09-24 §5.3): зачтённое/
// ресурс/оба буфера/оба уровня остатка/свободный трюм/признак «полный трюм».
type BeltCollectResponse struct {
	Granted           float64 `json:"granted"`
	Resource          string  `json:"resource"`
	Mined             float64 `json:"mined"`
	MinedIce          float64 `json:"mined_ice"`
	RemainingLevel    string  `json:"remaining_level"`
	RemainingLevelIce string  `json:"remaining_level_ice,omitempty"`
	CargoFree         float64 `json:"cargo_free"`
	Full              bool    `json:"full"`
}

// Collect — POST /api/belt/mine/collect. Клампы §5.3 (по выбранному ресурсу):
// скорость (R_cap·Δ), запас (лок FOR UPDATE), свободный трюм с учётом ОБОИХ
// буферов (mined + mined_ice + granted ≤ free). Прибавление к буферу и списание
// из запаса — в одной транзакции. last_collect_at общий, Δ делится честно.
func (h *BeltMiningHandlers) Collect(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	var req BeltCollectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	resource := req.Resource
	if resource == "" {
		resource = "iron"
	}
	if resource != "iron" && resource != "ice" {
		writeJSONError(w, "Неизвестный ресурс", http.StatusBadRequest)
		return
	}
	useIce := resource == "ice"

	_, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil {
		writeJSONError(w, "Не удалось получить позицию", http.StatusInternalServerError)
		return
	}
	if pos == nil || pos.Status != "mining" {
		writeJSONError(w, "Вы не в поясе", http.StatusBadRequest)
		return
	}
	beltID := pos.ObjectID

	belt, err := h.planetRepo.GetBeltByID(beltID)
	if err != nil {
		writeJSONError(w, "Не удалось получить пояс", http.StatusInternalServerError)
		return
	}
	if belt == nil {
		writeJSONError(w, "Пояс не найден", http.StatusBadRequest)
		return
	}
	iron, ice := 0.0, 0.0
	if belt.Composition != nil {
		iron = belt.Composition["iron"]
		ice = belt.Composition["ice"]
	}
	if useIce {
		// ice при composition.ice ≤ 0 или без ресурса воды → 400 «нет данных»
		// (не тихое «granted 0» — иначе клиент не отличит сбой от исчерпания, §5.3).
		if ice <= 0 {
			writeJSONError(w, "нет данных о запасе", http.StatusBadRequest)
			return
		}
		if _, _, _, err := resolveWaterGood(h.db); err != nil {
			if errors.Is(err, errWaterGoodNotFound) {
				writeJSONError(w, "нет данных о запасе", http.StatusBadRequest)
				return
			}
			writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
			return
		}
	} else if iron <= 0 {
		writeJSONError(w, "нет данных о запасе", http.StatusBadRequest)
		return
	}
	free, err := h.cargo.Free(userID)
	if err != nil {
		writeJSONError(w, "Не удалось получить трюм", http.StatusInternalServerError)
		return
	}

	now := time.Now()

	tx, err := h.db.Begin()
	if err != nil {
		writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Лок пояса (FOR UPDATE): два параллельных сбора не уводят запас в минус.
	// Обе колонки читаются под одним локом строки — ответ несёт оба уровня.
	var ironRem, iceRem sql.NullFloat64
	if err := tx.QueryRow(
		`SELECT iron_remaining, ice_remaining FROM system_belts WHERE id = $1 FOR UPDATE`, beltID,
	).Scan(&ironRem, &iceRem); err != nil {
		log.Printf("⚠️ belt mining: collect lock belt (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
		return
	}

	// Лок игрока (FOR UPDATE): свежий буфер захода — два сбора одного игрока
	// не теряют накопленное (порядок локов пояс → игрок, как у enter).
	var posRaw []byte
	if err := tx.QueryRow(
		`SELECT current_position FROM users WHERE id = $1 FOR UPDATE`, userID,
	).Scan(&posRaw); err != nil {
		log.Printf("⚠️ belt mining: collect lock user (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
		return
	}
	var fresh models.CurrentPosition
	if len(posRaw) > 0 {
		_ = json.Unmarshal(posRaw, &fresh)
	}
	if fresh.Status != "mining" || fresh.ObjectID != beltID {
		writeJSONError(w, "Вы не в поясе", http.StatusBadRequest)
		return
	}
	// Норма чтения: mined_ice отсутствует в старой позиции → 0 (§5.7).
	mined := 0.0
	if fresh.Mined != nil {
		mined = *fresh.Mined
	}
	minedIce := 0.0
	if fresh.MinedIce != nil {
		minedIce = *fresh.MinedIce
	}

	var rateCap, reserve float64
	if useIce {
		if !iceRem.Valid {
			writeJSONError(w, "нет данных о запасе", http.StatusBadRequest)
			return
		}
		// Потолок сервера (не то, что копит клиент): 2·beltIceRate (§5.1).
		rateCap = beltIceRateCap(ice)
		reserve = iceRem.Float64
	} else {
		if !ironRem.Valid {
			writeJSONError(w, "нет данных о запасе", http.StatusBadRequest)
			return
		}
		rateCap = beltRateCap(iron)
		reserve = ironRem.Float64
	}

	// Δ считается от СВЕЖЕГО last_collect_at (позиция прочитана под FOR UPDATE
	// в этой же транзакции, §5.3.1): иначе N конкурентных collect читали бы один
	// старый интервал и каждый получал бы полный R_cap·Δt (≈N·R_cap — обход
	// клампа скорости, нарушение анти-абьюза §2.2-B1). Трюм — общий на оба
	// буфера: free − (mined + mined_ice) (анти-ловушка §5.3.2).
	delta := beltDeltaSeconds(fresh.LastCollectAt, now)
	granted := beltGranted(req.Amount, rateCap, delta, reserve, free-(mined+minedIce))

	if granted > 0 {
		q := `UPDATE system_belts SET iron_remaining = iron_remaining - $1, updated_at = NOW() WHERE id = $2`
		if useIce {
			q = `UPDATE system_belts SET ice_remaining = ice_remaining - $1, updated_at = NOW() WHERE id = $2`
		}
		if _, err := tx.Exec(q, granted, beltID); err != nil {
			log.Printf("⚠️ belt mining: collect update reserve (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
			return
		}
	}
	minedNew, minedIceNew := mined, minedIce
	if useIce {
		minedIceNew = minedIce + granted
	} else {
		minedNew = mined + granted
	}
	fresh.Mined = &minedNew
	fresh.MinedIce = &minedIceNew
	// Секундная точность RFC3339 обнуляла бы кламп скорости: два сбора в одну
	// секунду получали бы Δ ≈ 1 c каждый. Nano-точность держит Δ честным.
	fresh.LastCollectAt = now.UTC().Format(time.RFC3339Nano)
	posJSON, err := json.Marshal(&fresh)
	if err != nil {
		writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(
		`UPDATE users SET current_position = $1, updated_at = NOW() WHERE id = $2`,
		posJSON, userID,
	); err != nil {
		log.Printf("⚠️ belt mining: collect update position (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSONError(w, "Не удалось выполнить сбор", http.StatusInternalServerError)
		return
	}

	ironAfter := ironRem.Float64
	iceAfter := iceRem.Float64
	if useIce {
		iceAfter -= granted
	} else {
		ironAfter -= granted
	}
	ironLevel := ""
	if iron > 0 && ironRem.Valid {
		ironLevel = beltRemainingLevel(ironAfter, beltReserve(iron))
	}
	iceLevel := ""
	if ice > 0 && iceRem.Valid {
		iceLevel = beltRemainingLevel(iceAfter, beltIceReserve)
	}
	writeJSONStatus(w, http.StatusOK, BeltCollectResponse{
		Granted:           granted,
		Resource:          resource,
		Mined:             minedNew,
		MinedIce:          minedIceNew,
		RemainingLevel:    ironLevel,
		RemainingLevelIce: iceLevel,
		CargoFree:         free,
		Full:              free-(minedNew+minedIceNew) <= 1e-9,
	})
}

// ==================== LEAVE (§5.4) ====================

// BeltLeaveResponse — ответ выхода: позиция орбиты пояса и принятое в трюм
// (added — железо, added_ice — вода; спека 2026-09-24 §5.5).
type BeltLeaveResponse struct {
	Position *models.CurrentPosition `json:"position"`
	Added    float64                 `json:"added"`
	AddedIce float64                 `json:"added_ice"`
}

// Leave — POST /api/belt/mine/leave. Не в заходе → 200 no-op (идемпотентность,
// инвариант 9 «много причин — один исход»). Иначе: ОБА буфера захода в трюм
// (TryAddCargoTx железо + вода) и позиция → orbit/belt одной транзакцией;
// буферы обнуляются. Порядок списания — железо, затем лёд (§5.5).
func (h *BeltMiningHandlers) Leave(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	_, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil {
		writeJSONError(w, "Не удалось получить позицию", http.StatusInternalServerError)
		return
	}
	if pos == nil || pos.Status != "mining" {
		writeJSONStatus(w, http.StatusOK, BeltLeaveResponse{Position: pos})
		return
	}
	beltID := pos.ObjectID
	mined := 0.0
	if pos.Mined != nil {
		mined = *pos.Mined
	}
	minedIce := 0.0
	if pos.MinedIce != nil {
		minedIce = *pos.MinedIce
	}

	ironGoodID := int64(0)
	if mined > 0 {
		gid, _, _, err := resolveIronGood(h.db)
		if err != nil {
			log.Printf("⚠️ belt mining: leave resolve iron (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
			return
		}
		ironGoodID = gid
	}
	// Вода: отсутствие ресурса в каталоге посреди сессии — NAMED EDGE (§5.5/§5.6,
	// как у FlushTx): ледяной буфер НЕ переливается и ТЕРЯЕТСЯ (accepted_ice = 0),
	// только log; выход не блокируется. В норме ресурс есть, иначе ледяной буфер
	// возникнуть не может (collect{ice} → 400).
	waterGoodID := int64(0)
	if minedIce > 0 {
		gid, _, _, err := resolveWaterGood(h.db)
		if errors.Is(err, errWaterGoodNotFound) {
			log.Printf("⚠️ belt mining: leave water good absent (user %s): ледяной буфер потерян (named edge §5.5)", userID)
		} else if err != nil {
			log.Printf("⚠️ belt mining: leave resolve water (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
			return
		} else {
			waterGoodID = gid
		}
	}

	tx, err := h.db.Begin()
	if err != nil {
		writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	added := 0.0
	if mined > 0 {
		added, err = h.cargo.TryAddCargoTx(tx, userID, ironGoodID, mined)
		if err != nil {
			log.Printf("⚠️ belt mining: leave add iron cargo (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
			return
		}
	}
	addedIce := 0.0
	if minedIce > 0 && waterGoodID != 0 {
		addedIce, err = h.cargo.TryAddCargoTx(tx, userID, waterGoodID, minedIce)
		if err != nil {
			log.Printf("⚠️ belt mining: leave add water cargo (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
			return
		}
	}

	newPos := models.OrbitPosition("belt", beltID)
	posJSON, err := json.Marshal(newPos)
	if err != nil {
		writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(
		`UPDATE users SET current_position = $1, updated_at = NOW() WHERE id = $2`,
		posJSON, userID,
	); err != nil {
		log.Printf("⚠️ belt mining: leave update position (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSONError(w, "Не удалось вернуться на корабль", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, BeltLeaveResponse{Position: newPos, Added: added, AddedIce: addedIce})
}

// ==================== ПЕРЕЛИВ БУФЕРА ПРИ ВЗЛЁТЕ (§6.5) ====================

// MiningBuffer — перелив буфера захода в трюм при взлёте из пояса (спека §6.5,
// решение §10.2-(A) «без последствий»): набранное не теряется. Перелив идёт в
// ОДНОЙ транзакции с записью новой позиции полёта (StartAtomicTx /
// CancelAtomicWithDestinationTx) — сбой между ними невозможен, повторный зачёт
// буфера исключён. БД для резолва ресурса берётся из переданной tx.
type MiningBuffer struct {
	cargo BeltCargo
}

// NewMiningBuffer — прод-перелив буфера (main.go).
func NewMiningBuffer(cargoSvc BeltCargo) *MiningBuffer {
	return &MiningBuffer{cargo: cargoSvc}
}

// FlushTx — внутри транзакции вызывающего: читает позицию игрока под FOR UPDATE;
// если это заход (mining) и хотя бы один буфер > 0 — резолвит ресурсы и кладёт
// ОБА буфера в трюм (TryAddCargoTx, железо затем вода; спека 2026-09-24 §5.6).
// Возвращает суммарно принятое. No-op (0, nil), если игрок не в заходе или оба
// буфера пусты. Ошибка — транзакцию откатывает вызывающий.
func (b *MiningBuffer) FlushTx(tx *sql.Tx, userID string) (float64, error) {
	var posRaw []byte
	if err := tx.QueryRow(
		`SELECT current_position FROM users WHERE id = $1 FOR UPDATE`, userID,
	).Scan(&posRaw); err != nil {
		return 0, fmt.Errorf("belt mining: flush read position: %w", err)
	}
	var pos models.CurrentPosition
	if len(posRaw) > 0 {
		_ = json.Unmarshal(posRaw, &pos)
	}
	if pos.Status != "mining" {
		return 0, nil
	}
	// Норма чтения: mined_ice отсутствует в старой позиции → 0 (§5.7).
	mined := 0.0
	if pos.Mined != nil {
		mined = *pos.Mined
	}
	minedIce := 0.0
	if pos.MinedIce != nil {
		minedIce = *pos.MinedIce
	}
	if mined <= 0 && minedIce <= 0 {
		return 0, nil
	}

	total := 0.0
	if mined > 0 {
		goodID, _, _, err := resolveIronGood(tx)
		if err != nil {
			return 0, fmt.Errorf("belt mining: flush resolve iron: %w", err)
		}
		accepted, err := b.cargo.TryAddCargoTx(tx, userID, goodID, mined)
		if err != nil {
			return 0, fmt.Errorf("belt mining: flush add iron cargo: %w", err)
		}
		total += accepted
	}
	if minedIce > 0 {
		goodID, _, _, err := resolveWaterGood(tx)
		if errors.Is(err, errWaterGoodNotFound) {
			// Ресурс воды исчез посреди сессии (админский сценарий, §5.6):
			// ледяной буфер не переливается, полёт не блокируется.
			log.Printf("⚠️ belt mining: flush water good absent (user %s): ледяной буфер не перелит", userID)
		} else if err != nil {
			return 0, fmt.Errorf("belt mining: flush resolve water: %w", err)
		} else {
			accepted, err := b.cargo.TryAddCargoTx(tx, userID, goodID, minedIce)
			if err != nil {
				return 0, fmt.Errorf("belt mining: flush add water cargo: %w", err)
			}
			total += accepted
		}
	}
	return total, nil
}
