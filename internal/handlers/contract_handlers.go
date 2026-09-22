// internal/handlers/contract_handlers.go
//
// Контракты: доска планеты, «мои», взятие/отмена, публикация игроком и
// админ-инструмент (спеки 2026-09-22-контракт-модель-сущности §4–§6,
// 2026-09-22-контракт-перелёт-и-доска §2–§3). Доска гейтится знанием планеты
// (77a; образец — 2026-09-21-фабрики...): не знаешь планету — не знаешь её доску.
// Балансы на доске не показываются (канон 14_money §14.4).
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
)

// defaultContractOfferWindow — срок жизни предложения на доске, пока open,
// если клиент не задал expires_at. Заглушка «калибровать позже»
// (2026-09-22-контракт-перелёт-и-доска §4.3). Для перелёта срок жизни на доске
// выводится из времени полёта на референсной тяге (offer_window, §4.3).
const defaultContractOfferWindow = 24 * time.Hour

// Параметры срока перелёта (спека §4.3, заглушки «калибровать позже»):
// speed_factor_ref — референсная тяга (стартовый двигатель, самый медленный);
// reserve — запас сверх времени полёта; offerWindowFactor — жизнь предложения
// на доске как множитель времени полёта.
const (
	contractDeadlineReserve = 0.5
	contractOfferWindowMult = 10.0
	// contractTravelGearSpeedFactorRef — порог требования к двигателю у перелёта
	// (спека перелёта §1.3: «двигатель не хуже референсного»; референс — самый
	// медленный стартовый двигатель, speed_factor_ref). Заглушка «калибровать
	// позже», рядом со сроками.
	contractTravelGearSpeedFactorRef = models.EngineSpeedDefault
)

// ContractHandlers — игровые и админские операции над контрактами.
type ContractHandlers struct {
	contractRepo  *repository.ContractRepository
	planetRepo    *repository.PlanetRepository
	userRepo      *repository.UserRepository
	knowledgeRepo *repository.KnowledgeRepository
	worldRepo     *repository.WorldRepository
}

func NewContractHandlers(
	contractRepo *repository.ContractRepository,
	planetRepo *repository.PlanetRepository,
	userRepo *repository.UserRepository,
	knowledgeRepo *repository.KnowledgeRepository,
	worldRepo *repository.WorldRepository,
) *ContractHandlers {
	return &ContractHandlers{
		contractRepo:  contractRepo,
		planetRepo:    planetRepo,
		userRepo:      userRepo,
		knowledgeRepo: knowledgeRepo,
		worldRepo:     worldRepo,
	}
}

// requirementReq — требование в запросе публикации (§4.2).
type requirementReq struct {
	Kind          string   `json:"kind"`
	Subject       string   `json:"subject"`
	Op            string   `json:"op"`
	ThresholdNum  *float64 `json:"threshold_num"`
	ThresholdText *string  `json:"threshold_text"`
	Quantity      *int64   `json:"quantity"`
}

// requirementReqs — список требований запроса (именованный тип для метода).
type requirementReqs []requirementReq

// toModels — перевод требований запроса в модель.
func (reqs requirementReqs) toModels() []models.ContractRequirement {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]models.ContractRequirement, 0, len(reqs))
	for i, r := range reqs {
		out = append(out, models.ContractRequirement{
			Pos:           i + 1,
			Kind:          r.Kind,
			Subject:       r.Subject,
			Op:            r.Op,
			ThresholdNum:  r.ThresholdNum,
			ThresholdText: r.ThresholdText,
			Quantity:      r.Quantity,
		})
	}
	return out
}

// GetPlanetBoard — GET /api/planets/{planet_id}/contracts: доска планеты.
// Гейт: планета известна игроку (знание) либо роль не player. Ленивое
// истечение в области планеты (§6.3), затем живые публичные контракты.
func (h *ContractHandlers) GetPlanetBoard(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/planets/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[1] != "contracts" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	planetID := parts[0]

	planet, err := h.planetRepo.GetPlanetByID(planetID)
	if err != nil {
		writeJSONError(w, "Не удалось получить планету", http.StatusInternalServerError)
		return
	}
	if planet == nil {
		writeJSONError(w, "Планета не найдена", http.StatusNotFound)
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	if user.Role == models.RolePlayer {
		k, err := h.knowledgeRepo.GetKnowledge(userID, planetID)
		if err != nil {
			writeJSONError(w, "Не удалось проверить знание планеты", http.StatusInternalServerError)
			return
		}
		if k == nil {
			writeJSONError(w, "Планета не известна", http.StatusForbidden)
			return
		}
	}

	if _, err := h.contractRepo.ExpireDue(repository.ContractScope{PlanetID: planetID}); err != nil {
		log.Printf("GetPlanetBoard: expire due (%s): %v", planetID, err)
		writeJSONError(w, "Не удалось обновить доску", http.StatusInternalServerError)
		return
	}

	contracts, err := h.contractRepo.ListBoard(planetID)
	if err != nil {
		writeJSONError(w, "Не удалось получить доску", http.StatusInternalServerError)
		return
	}
	if contracts == nil {
		contracts = []*models.Contract{}
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"planet_id": planetID,
		"contracts": contracts,
	})
}

// GetMyContracts — GET /api/contracts/mine: где игрок автор или исполнитель.
func (h *ContractHandlers) GetMyContracts(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	// Пустая область ContractScope{} = вся таблица: «мои контракты» видят все
	// статусы, поэтому истечение глобальное (не только по планете).
	if _, err := h.contractRepo.ExpireDue(repository.ContractScope{}); err != nil {
		log.Printf("GetMyContracts: expire due: %v", err)
	}
	contracts, err := h.contractRepo.ListMine(models.ContractActorPlayer, userID)
	if err != nil {
		writeJSONError(w, "Не удалось получить контракты", http.StatusInternalServerError)
		return
	}
	if contracts == nil {
		contracts = []*models.Contract{}
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"contracts": contracts})
}

// TakeContract — POST /api/contracts/take: атомарный flip open→taken.
func (h *ContractHandlers) TakeContract(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	var req struct {
		ContractID string `json:"contract_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ContractID == "" {
		writeJSONError(w, "contract_id обязателен", http.StatusBadRequest)
		return
	}
	contract, err := h.contractRepo.GetByID(req.ContractID)
	if err != nil {
		writeJSONError(w, "Не удалось получить контракт", http.StatusInternalServerError)
		return
	}
	if contract == nil {
		writeJSONError(w, "Контракт не найден", http.StatusNotFound)
		return
	}
	// Требование к двигателю проверяется при взятии (спека §1.3): speed_factor
	// установленного двигателя не хуже референсного. Игрок без двигателя летать
	// не может (91a §6.1) — то же правило, что в /travel (ship.HasEngine).
	if contract.Type == models.ContractTypeTravel || hasGearRequirements(contract.Requirements) {
		user, err := h.userRepo.GetByID(userID)
		if err != nil || user == nil {
			writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
			return
		}
		if contract.Type == models.ContractTypeTravel && user.Role == models.RolePlayer && !ship.HasEngine(user.Equipment) {
			writeJSONError(w, "Двигатель не установлен — нельзя взять контракт на перелёт", http.StatusBadRequest)
			return
		}
		if msg := checkGearRequirements(contract.Requirements, user.Equipment); msg != "" {
			writeJSONError(w, msg, http.StatusBadRequest)
			return
		}
	}
	// Перебазирование срока при взятии — правило типа travel (спека §4.3):
	// expires_at = now() + flight_time_ref·(1+reserve). У других типов срок не
	// перебазируется (nil → COALESCE сохраняет опубликованный).
	// Осознанное упрощение (решение создателя): from_world_id НЕ сверяется с
	// позицией берущего — перелёт = «проездной билет» A→B, ограничения места во
	// взятии нет ни в каноне, ни в спеке. Не «додумывать» проверку здесь.
	var rebase *time.Time
	if contract.Type == models.ContractTypeTravel {
		if d, ok := h.travelDistance(contract.Payload); ok {
			dl := time.Now().Add(travelDeadline(d))
			rebase = &dl
		}
	}
	taken, err := h.contractRepo.Take(req.ContractID, models.ContractExecutorPlayer, userID, rebase)
	if err != nil {
		writeJSONError(w, "Не удалось взять контракт", http.StatusInternalServerError)
		return
	}
	if !taken {
		writeJSONError(w, "Контракт недоступен (уже взят или истёк)", http.StatusConflict)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]string{"status": "taken"})
}

// CancelContract — POST /api/contracts/cancel: отмена автором (только open),
// залог возвращается автору.
func (h *ContractHandlers) CancelContract(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	var req struct {
		ContractID string `json:"contract_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ContractID == "" {
		writeJSONError(w, "contract_id обязателен", http.StatusBadRequest)
		return
	}
	cancelled, err := h.contractRepo.Cancel(req.ContractID, models.ContractActorPlayer, userID)
	if err != nil {
		writeJSONError(w, "Не удалось отменить контракт", http.StatusInternalServerError)
		return
	}
	if !cancelled {
		writeJSONError(w, "Контракт нельзя отменить (не ваш или уже взят)", http.StatusConflict)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// createContractReq — тело публикации контракта (игрок и админ-инструмент).
type createContractReq struct {
	PlanetID         string                 `json:"planet_id"`
	Type             string                 `json:"type"`
	Title            string                 `json:"title"`
	Description      string                 `json:"description"`
	Reward           int64                  `json:"reward"`
	Payload          map[string]interface{} `json:"payload"`
	ExpiresAt        *time.Time             `json:"expires_at"`
	Requirements     requirementReqs        `json:"requirements"`
	AuthorType       string                 `json:"author_type"`
	AuthorID         string                 `json:"author_id"`
	Funding          string                 `json:"funding"`
	Visibility       string                 `json:"visibility"`
	DirectTargetType *string                `json:"direct_target_type"`
	DirectTargetID   *string                `json:"direct_target_id"`
}

// validate — базовая проверка обязательных полей публикации.
func (req createContractReq) validate() string {
	switch {
	case req.PlanetID == "":
		return "planet_id обязателен"
	case req.Type == "":
		return "type обязателен"
	case req.Title == "":
		return "title обязателен"
	case req.Reward <= 0:
		return "reward должен быть больше нуля"
	case req.Visibility == models.ContractVisibilityDirect && (req.DirectTargetID == nil || *req.DirectTargetID == ""):
		return "прямой контракт требует direct_target_id"
	}
	if req.Type == models.ContractTypeTravel {
		if msg := validateTravelPayload(req.Payload); msg != "" {
			return msg
		}
	}
	return ""
}

// validateTravelPayload — payload перелёта (спека §1.2): from_world_id и
// dest_world_id обязательны, dest_planet_id опционален (nil = цель-система).
func validateTravelPayload(payload map[string]interface{}) string {
	if payload == nil {
		return "payload перелёта обязателен"
	}
	if s, _ := payload["from_world_id"].(string); s == "" {
		return "from_world_id обязателен"
	}
	if s, _ := payload["dest_world_id"].(string); s == "" {
		return "dest_world_id обязателен"
	}
	if v, ok := payload["dest_planet_id"]; ok && v != nil {
		if s, _ := v.(string); s == "" {
			return "dest_planet_id должен быть непустой строкой или null"
		}
	}
	return ""
}

// travelDistance — евклидово расстояние между системами payload перелёта
// (CoordX/CoordY, спека §4.1). false, если координаты недоступны.
func (h *ContractHandlers) travelDistance(payload map[string]interface{}) (float64, bool) {
	if h.worldRepo == nil || payload == nil {
		return 0, false
	}
	fromID, _ := payload["from_world_id"].(string)
	destID, _ := payload["dest_world_id"].(string)
	if fromID == "" || destID == "" {
		return 0, false
	}
	from, err := h.worldRepo.GetByID(fromID)
	if err != nil || from == nil {
		return 0, false
	}
	dest, err := h.worldRepo.GetByID(destID)
	if err != nil || dest == nil {
		return 0, false
	}
	dx := from.CoordX - dest.CoordX
	dy := from.CoordY - dest.CoordY
	return math.Sqrt(dx*dx + dy*dy), true
}

// travelFlightTimeRef — время полёта на референсной тяге (самый медленный
// двигатель); переиспользует формулу /travel (calcTravelDuration, спека §4.1).
func travelFlightTimeRef(dist float64) time.Duration {
	return calcTravelDuration(dist, models.EngineSpeedDefault)
}

// travelDeadline — срок исполнения перелёта: время полёта на референсной тяге
// плюс запас (спека §4.3). Отсчитывается от взятия (перебазирование).
func travelDeadline(dist float64) time.Duration {
	return time.Duration(float64(travelFlightTimeRef(dist)) * (1 + contractDeadlineReserve))
}

// travelOfferWindow — жизнь предложения на доске, пока контракт open (§4.3).
func travelOfferWindow(dist float64) time.Duration {
	return time.Duration(float64(travelFlightTimeRef(dist)) * contractOfferWindowMult)
}

// expiresAt — срок публикации: из запроса, иначе заглушка. Для перелёта срок
// **выводится** (offer_window от времени полёта, спека §4.3) — заказчик его не
// выбирает, клиентский expires_at игнорируется.
func (h *ContractHandlers) expiresAt(req createContractReq, now time.Time) time.Time {
	if req.Type == models.ContractTypeTravel {
		if d, ok := h.travelDistance(req.Payload); ok {
			return now.Add(travelOfferWindow(d))
		}
		return now.Add(defaultContractOfferWindow)
	}
	if req.ExpiresAt != nil && req.ExpiresAt.After(now) {
		return *req.ExpiresAt
	}
	return now.Add(defaultContractOfferWindow)
}

// gearRequirementValue — значение параметра снаряжения (kind='gear', §4.2).
// subject — имя параметра (speed_factor), не слот; значение берётся из
// установленного двигателя игрока (ship.EngineSpeed, спека 91a §7.1) —
// единый источник скорости, формула не переизобретается.
func gearRequirementValue(subject string, equipment map[string]interface{}) (float64, bool) {
	if subject == "speed_factor" {
		return ship.EngineSpeed(equipment), true
	}
	return 0, false
}

// hasGearRequirements — есть ли среди требований проверяемые при взятии.
func hasGearRequirements(reqs []models.ContractRequirement) bool {
	for _, r := range reqs {
		if r.Kind == "gear" {
			return true
		}
	}
	return false
}

// checkGearRequirements — проверка требований снаряжения при взятии (спека
// перелёта §1.3, канон 14_money §14.6.1). Пусто — все выполнены; иначе причина
// отказа. Только односторонние числовые сравнения (в итерации 1 — op='le').
func checkGearRequirements(reqs []models.ContractRequirement, equipment map[string]interface{}) string {
	for _, req := range reqs {
		if req.Kind != "gear" {
			continue
		}
		val, ok := gearRequirementValue(req.Subject, equipment)
		if !ok {
			return fmt.Sprintf("Неизвестное требование снаряжения: %s", req.Subject)
		}
		if req.ThresholdNum == nil {
			continue
		}
		switch req.Op {
		case "le":
			if val > *req.ThresholdNum {
				return "Двигатель недостаточно быстр для этого контракта"
			}
		case "ge":
			if val < *req.ThresholdNum {
				return "Снаряжение не соответствует требованию контракта"
			}
		case "eq":
			if val != *req.ThresholdNum {
				return "Снаряжение не соответствует требованию контракта"
			}
		default:
			return fmt.Sprintf("Неподдержанная проверка требования: %s", req.Op)
		}
	}
	return ""
}

// ensureTravelGearRequirement — гарантия требования к двигателю у перелёта
// (спека перелёта §1.3): требование — свойство типа, а не то, что заказчик
// угадывает руками. Если gear-требование по speed_factor уже задано — уважаем
// (не перезаписываем), иначе добавляем референсное (kind='gear',
// subject='speed_factor', op='le', threshold_num=contractTravelGearSpeedFactorRef).
func ensureTravelGearRequirement(reqs []models.ContractRequirement) []models.ContractRequirement {
	for _, r := range reqs {
		if r.Kind == "gear" && r.Subject == "speed_factor" {
			return reqs
		}
	}
	threshold := contractTravelGearSpeedFactorRef
	return append(reqs, models.ContractRequirement{
		Kind:         "gear",
		Subject:      "speed_factor",
		Op:           "le",
		ThresholdNum: &threshold,
	})
}

// publish — общий путь публикации (игрок/админ): проверка планеты и вызов
// репозитория (атомарная вставка + залог).
func (h *ContractHandlers) publish(w http.ResponseWriter, authorType, authorID string, req createContractReq) {
	if msg := req.validate(); msg != "" {
		writeJSONError(w, msg, http.StatusBadRequest)
		return
	}
	planet, err := h.planetRepo.GetPlanetByID(req.PlanetID)
	if err != nil {
		writeJSONError(w, "Не удалось получить планету", http.StatusInternalServerError)
		return
	}
	if planet == nil {
		writeJSONError(w, "Планета не найдена", http.StatusNotFound)
		return
	}
	// Свойства типа «перелёт», обязательные на публикации: требование к
	// двигателю (§1.3, гарантируется сервером) и funding='regular' (§6 R8,
	// 15_monetization §15.4.2: перелёт подрядом не бывает). Не-перелёт — B1.
	requirements := req.Requirements.toModels()
	funding := req.Funding
	if req.Type == models.ContractTypeTravel {
		requirements = ensureTravelGearRequirement(requirements)
		funding = models.ContractFundingRegular
	}
	now := time.Now()
	contract, err := h.contractRepo.Publish(repository.PublishContractParams{
		Type:                req.Type,
		AuthorType:          authorType,
		AuthorID:            authorID,
		PublicationPlanetID: req.PlanetID,
		Title:               req.Title,
		Description:         req.Description,
		Payload:             req.Payload,
		Reward:              req.Reward,
		Funding:             funding,
		Visibility:          req.Visibility,
		DirectTargetType:    req.DirectTargetType,
		DirectTargetID:      req.DirectTargetID,
		ExpiresAt:           h.expiresAt(req, now),
		Requirements:        requirements,
	})
	if err != nil {
		writePublishError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, contract)
}

// CreateContract — POST /api/contracts: публикация игроком. Только с планеты,
// где стоит игрок (орбита/поверхность; решение О-п1).
func (h *ContractHandlers) CreateContract(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req createContractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	user, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	// Публикация только с планеты, где стоит игрок (О-п1): не в полёте, объект —
	// планета, она же в текущем мире игрока.
	if pos == nil || pos.Status == "in_flight" || pos.ObjectType != "planet" ||
		pos.ObjectID != req.PlanetID {
		writeJSONError(w, "Опубликовать контракт можно только с планеты, где вы находитесь", http.StatusConflict)
		return
	}
	if user.CurrentWorldID == nil {
		writeJSONError(w, "Сначала переместитесь в систему", http.StatusConflict)
		return
	}
	planet, err := h.planetRepo.GetPlanetByID(req.PlanetID)
	if err != nil || planet == nil || planet.WorldID != *user.CurrentWorldID {
		writeJSONError(w, "Планета не в вашей системе", http.StatusConflict)
		return
	}

	h.publish(w, models.ContractActorPlayer, userID, req)
}

// AdminCreateContract — POST /admin/contracts: публикация вручную остальными
// авторами (фракция/постройка/агент) и отладка. UI — B3, серверная часть — B1.
// Планета публикации определяется по автору (спека перелёта §3), а не берётся
// из тела вслепую: player — где стоит игрок, faction — homeworld_id,
// building — planet_id, agent — не поддержан в итерации 1 (4xx).
func (h *ContractHandlers) AdminCreateContract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req createContractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}
	if req.AuthorType == "" || req.AuthorID == "" {
		writeJSONError(w, "author_type и author_id обязательны", http.StatusBadRequest)
		return
	}
	switch req.AuthorType {
	case models.ContractActorPlayer, models.ContractActorFaction,
		models.ContractActorBuilding, models.ContractActorAgent:
	default:
		writeJSONError(w, "недопустимый author_type", http.StatusBadRequest)
		return
	}
	planetID, err := h.resolvePublicationPlanet(req.AuthorType, req.AuthorID)
	if err != nil {
		if errors.Is(err, repository.ErrPublicationPlanetUnresolved) {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSONError(w, "Не удалось определить планету публикации", http.StatusInternalServerError)
		return
	}
	req.PlanetID = planetID
	h.publish(w, req.AuthorType, req.AuthorID, req)
}

// resolvePublicationPlanet — место публикации по автору (спека перелёта §3).
// Автор-player резолвится по позиции игрока — тем же правилом, что игровой
// путь CreateContract (планета, где стоит игрок; на орбите звезды нельзя).
// Остальные авторы — через репозиторий (faction/building); агент-автор в
// итерации 1 не поддержан.
func (h *ContractHandlers) resolvePublicationPlanet(authorType, authorID string) (string, error) {
	if authorType == models.ContractActorPlayer {
		_, pos, _, err := h.userRepo.GetByIDWithPosition(authorID)
		if err != nil {
			return "", err
		}
		if pos == nil || pos.Status == "in_flight" || pos.ObjectType != "planet" || pos.ObjectID == "" {
			return "", repository.ErrPublicationPlanetUnresolved
		}
		return pos.ObjectID, nil
	}
	return h.contractRepo.ResolvePublicationPlanet(authorType, authorID)
}

// writePublishError — маппинг ошибок публикации в HTTP-коды.
func writePublishError(w http.ResponseWriter, err error) {
	switch {
	case err == repository.ErrInsufficientFunds:
		writeJSONError(w, "Недостаточно средств для залога", http.StatusPaymentRequired)
	case err == repository.ErrInvalidReward:
		writeJSONError(w, err.Error(), http.StatusBadRequest)
	default:
		writeJSONError(w, "Не удалось опубликовать контракт", http.StatusInternalServerError)
	}
}
