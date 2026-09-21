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
	"log"
	"net/http"
	"strings"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// defaultContractOfferWindow — срок жизни предложения на доске, пока open,
// если клиент не задал expires_at. Заглушка «калибровать позже»
// (2026-09-22-контракт-перелёт-и-доска §4.3).
const defaultContractOfferWindow = 24 * time.Hour

// ContractHandlers — игровые и админские операции над контрактами.
type ContractHandlers struct {
	contractRepo  *repository.ContractRepository
	planetRepo    *repository.PlanetRepository
	userRepo      *repository.UserRepository
	knowledgeRepo *repository.KnowledgeRepository
}

func NewContractHandlers(
	contractRepo *repository.ContractRepository,
	planetRepo *repository.PlanetRepository,
	userRepo *repository.UserRepository,
	knowledgeRepo *repository.KnowledgeRepository,
) *ContractHandlers {
	return &ContractHandlers{
		contractRepo:  contractRepo,
		planetRepo:    planetRepo,
		userRepo:      userRepo,
		knowledgeRepo: knowledgeRepo,
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
	taken, err := h.contractRepo.Take(req.ContractID, models.ContractExecutorPlayer, userID)
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

// expiresAtOr — срок предложения: из запроса либо заглушка по умолчанию.
func (req createContractReq) expiresAtOr(now time.Time) time.Time {
	if req.ExpiresAt != nil && req.ExpiresAt.After(now) {
		return *req.ExpiresAt
	}
	return now.Add(defaultContractOfferWindow)
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
	return ""
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
		Funding:             req.Funding,
		Visibility:          req.Visibility,
		DirectTargetType:    req.DirectTargetType,
		DirectTargetID:      req.DirectTargetID,
		ExpiresAt:           req.expiresAtOr(now),
		Requirements:        req.Requirements.toModels(),
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
