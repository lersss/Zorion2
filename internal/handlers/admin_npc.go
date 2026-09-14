// internal/handlers/admin_npc.go
// Вкладка «NPC» админки (спека 20a.1 §6, §8): список, создание, удаление,
// частичное обновление агентов. Все ручки закрыты auth.AdminAuth на уровне
// регистрации роутов. Настройки менеджера — npc_settings.go, позиции для
// карты — npc_positions.go.
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"zorion/internal/models"
	"zorion/internal/npc"
	"zorion/internal/repository"
)

// maxAgentNameLen — лимит длины имени агента (валидация POST/PATCH).
const maxAgentNameLen = 100

// AdminNPCHandlers — CRUD NPC-агентов + настройки + позиции.
type AdminNPCHandlers struct {
	npcRepo   *repository.NPCRepository
	worldRepo *repository.WorldRepository
	manager   *npc.Manager
}

func NewAdminNPCHandlers(npcRepo *repository.NPCRepository, worldRepo *repository.WorldRepository, manager *npc.Manager) *AdminNPCHandlers {
	return &AdminNPCHandlers{npcRepo: npcRepo, worldRepo: worldRepo, manager: manager}
}

// ==================== КОЛЛЕКЦИЯ: /admin/npc ====================

// HandleCollection — диспетчер по методу для /admin/npc.
func (h *AdminNPCHandlers) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListAgents(w, r)
	case http.MethodPost:
		h.CreateAgent(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// ListAgents — GET /admin/npc: список агентов с текущим состоянием
// (источник правды — БД, живой статус после тика планировщика).
func (h *AdminNPCHandlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.npcRepo.ListAll()
	if err != nil {
		log.Printf("ListAgents: ListAll error: %v", err)
		writeJSONError(w, "Не удалось получить список агентов", http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []models.NPCAgent{}
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"agents": agents})
}

// CreateAgent — POST /admin/npc: {name, start_world_id?} → 201, статус idle
// (спека §8). Стартовый мир — указанный (проверяется существование) или
// случайный из сетки, если не указан.
func (h *AdminNPCHandlers) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		StartWorldID string `json:"start_world_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSONError(w, "Имя обязательно", http.StatusBadRequest)
		return
	}
	if len(req.Name) > maxAgentNameLen {
		writeJSONError(w, "Имя слишком длинное (макс. 100 символов)", http.StatusBadRequest)
		return
	}

	startWorld, err := h.resolveStartWorld(req.StartWorldID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	agent := &models.NPCAgent{
		ID:             uuid.New().String(),
		Name:           req.Name,
		CurrentWorldID: startWorld,
	}
	if err := h.npcRepo.Insert(agent); err != nil {
		log.Printf("CreateAgent: Insert error: %v", err)
		writeJSONError(w, "Не удалось создать агента", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusCreated, agent)
}

// resolveStartWorld — стартовый мир агента: указанный (проверка
// существования) или случайный из галактики.
func (h *AdminNPCHandlers) resolveStartWorld(startWorldID string) (string, error) {
	if startWorldID == "" {
		worldID, ok := h.manager.RandomWorld()
		if !ok {
			return "", errors.New("Нет миров для старта (карта не загружена или галактика пуста)")
		}
		return worldID, nil
	}
	if _, err := uuid.Parse(startWorldID); err != nil {
		return "", errors.New("Невалидный start_world_id")
	}
	world, err := h.worldRepo.GetByID(startWorldID)
	if err != nil {
		log.Printf("CreateAgent: GetByID(%s) error: %v", startWorldID, err)
		return "", errors.New("Не удалось проверить стартовый мир")
	}
	if world == nil {
		return "", errors.New("Стартовый мир не найден")
	}
	return startWorldID, nil
}

// ==================== ОБЪЕКТ: /admin/npc/{id} и /admin/npc/settings ====================

// HandleObject — диспетчер по методу для /admin/npc/{id}; путь "settings"
// уводит на настройки менеджера (npc_settings.go).
func (h *AdminNPCHandlers) HandleObject(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/npc/")
	if path == "" || path == r.URL.Path || strings.Contains(path, "/") {
		writeJSONError(w, "ID не указан", http.StatusBadRequest)
		return
	}
	if path == "settings" {
		h.HandleSettings(w, r)
		return
	}
	// Не-UUID-строка в {id} упала бы в БД с pq: invalid input syntax for
	// type uuid → 500. Отсекаем явно (как в разделе «Пользователи»).
	if _, err := uuid.Parse(path); err != nil {
		writeJSONError(w, "Невалидный ID", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		h.PatchAgent(w, r, path)
	case http.MethodDelete:
		h.DeleteAgent(w, r, path)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// PatchAgent — PATCH /admin/npc/{id}: {name?, notify_enabled?} → 200
// (спека §8). Поля независимы: nil-указатель — поле не меняется.
func (h *AdminNPCHandlers) PatchAgent(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name          *string `json:"name"`
		NotifyEnabled *bool   `json:"notify_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.Name == nil && req.NotifyEnabled == nil {
		writeJSONError(w, "Нет полей для обновления", http.StatusBadRequest)
		return
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			writeJSONError(w, "Имя не может быть пустым", http.StatusBadRequest)
			return
		}
		if len(*req.Name) > maxAgentNameLen {
			writeJSONError(w, "Имя слишком длинное (макс. 100 символов)", http.StatusBadRequest)
			return
		}
	}

	if err := h.npcRepo.Update(id, req.Name, req.NotifyEnabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Агент не найден", http.StatusNotFound)
			return
		}
		log.Printf("PatchAgent: Update error: %v", err)
		writeJSONError(w, "Не удалось обновить агента", http.StatusInternalServerError)
		return
	}

	agent, err := h.npcRepo.GetByID(id)
	if err != nil || agent == nil {
		log.Printf("PatchAgent: reload error: %v", err)
		writeJSONError(w, "Не удалось получить агента", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, agent)
}

// DeleteAgent — DELETE /admin/npc/{id} → 204 (спека §8).
func (h *AdminNPCHandlers) DeleteAgent(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.npcRepo.Delete(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Агент не найден", http.StatusNotFound)
			return
		}
		log.Printf("DeleteAgent: Delete error: %v", err)
		writeJSONError(w, "Не удалось удалить агента", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}