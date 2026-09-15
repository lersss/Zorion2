// internal/handlers/admin_npc.go
// Вкладка «NPC» админки (спека 20a.1 §6, §8; спека 26a.1 §4–5): постраничный
// список, создание, удаление, частичное обновление, массовая генерация
// (асинхронный джоб). Все ручки закрыты auth.AdminAuth на уровне регистрации
// роутов. Настройки менеджера — npc_settings.go, метрики — npc_metrics.go,
// поиск на карте — npc_search.go, позиции для карты — npc_positions.go.
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"zorion/internal/generator"
	"zorion/internal/models"
	"zorion/internal/names"
	"zorion/internal/npc"
	"zorion/internal/repository"
)

// maxAgentNameLen — лимит длины имени агента (валидация POST/PATCH).
const maxAgentNameLen = 100

// npcPageDefaultLimit / npcPageMaxLimit — размер страницы списка агентов
// (спека 26a.1 §5.2): default 200, max 1000 (защита от гигантских ответов).
const (
	npcPageDefaultLimit = 200
	npcPageMaxLimit     = 1000
)

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

// ListAgents — GET /admin/npc?limit=&cursor=: страница агентов с текущим
// состоянием (источник правды — БД, живой статус после тика планировщика).
// Keyset-пагинация по (created_at, id), свежие сверху (спека 26a.1 §5):
// «показать ещё» передаёт next_cursor; null — конец списка.
func (h *AdminNPCHandlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	limit := npcPageDefaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			writeJSONError(w, "limit должен быть целым ≥ 1", http.StatusBadRequest)
			return
		}
		if n > npcPageMaxLimit {
			writeJSONError(w, "limit не может превышать 1000", http.StatusBadRequest)
			return
		}
		limit = n
	}

	agents, next, err := h.npcRepo.ListPage(limit, r.URL.Query().Get("cursor"))
	if err != nil {
		if errors.Is(err, repository.ErrInvalidCursor) {
			writeJSONError(w, "Невалидный cursor", http.StatusBadRequest)
			return
		}
		log.Printf("ListAgents: ListPage error: %v", err)
		writeJSONError(w, "Не удалось получить список агентов", http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []models.NPCAgent{}
	}
	var nextCursor interface{}
	if next != "" {
		nextCursor = next
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"agents":      agents,
		"next_cursor": nextCursor,
	})
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
	h.manager.MarkDirty() // кэш позиций: следующий тик перезагрузит (идея 26c A2)
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

// HandleObject — диспетчер по методу для /admin/npc/{id}; пути "settings",
// "generate" и "metrics" — служебные (не UUID): настройки менеджера
// (npc_settings.go), массовая генерация (спека 26a.1 §4), метрики (§8).
func (h *AdminNPCHandlers) HandleObject(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/npc/")
	if path == "" || path == r.URL.Path || strings.Contains(path, "/") {
		writeJSONError(w, "ID не указан", http.StatusBadRequest)
		return
	}
	switch path {
	case "settings":
		h.HandleSettings(w, r)
		return
	case "generate":
		h.GenerateNPC(w, r)
		return
	case "clear":
		h.ClearAllAgents(w, r)
		return
	case "metrics":
		h.Metrics(w, r)
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
	h.manager.MarkDirty() // имя видно на карте — кэш позиций перезагрузится (идея 26c A2)

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
	h.manager.MarkDirty() // кэш позиций: следующий тик перезагрузит (идея 26c A2)
	w.WriteHeader(http.StatusNoContent)
}

// ==================== МАССОВАЯ ГЕНЕРАЦИЯ (спека 26a.1 §4) ====================

// GenerateNPC — POST /admin/npc/generate: {count} → 202, асинхронный джоб
// (спека §4.1, паттерн генерации вселенной/гипотез: TryStart + 202 + poll).
// count ≥ 1, без верхнего лимита (§4.3); 409 — джоб уже крутится; 400 —
// count невалиден или нет миров для старта. Джоб: seed имён из БД →
// генерация имён + стартовые миры (случайные по галактике) → BulkInsert
// одной COPY-транзакцией → отчёт «Создано агентов: N за X.X с».
func (h *AdminNPCHandlers) GenerateNPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Count *int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Count == nil {
		writeJSONError(w, "count обязателен (целое ≥ 1)", http.StatusBadRequest)
		return
	}
	count := *req.Count
	if count < 1 {
		writeJSONError(w, "count должен быть ≥ 1", http.StatusBadRequest)
		return
	}

	// Стартовые миры — случайные по всей галактике: предвычисленный слайс
	// id из сетки, O(1) на агента, без N запросов к БД (§4.4).
	worlds, ok := h.manager.RandomWorlds(count)
	if !ok {
		writeJSONError(w, "Нет миров для старта (карта не загружена или галактика пуста)", http.StatusBadRequest)
		return
	}

	// Контекст от фоновой задачи, НЕ от запроса (паттерн admin_hypothesis.go).
	_, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobGenerateNPC, count, cancel) {
		cancel()
		writeJSONError(w, "Генерация уже идёт", http.StatusConflict)
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ GenerateNPC panic: %v", rec)
				statusManager.Fail(generator.JobGenerateNPC, "panic: "+recoverErr(rec))
			}
		}()

		start := time.Now()

		// 1. Seed имён: существующие имена из БД — уникальность между пачками
		// (§3.2). При ошибке чтения джоб падает — пачка не начнётся.
		usedNames := map[string]bool{}
		existing, err := h.npcRepo.ListNames()
		if err != nil {
			log.Printf("❌ GenerateNPC: ListNames: %v", err)
			statusManager.Fail(generator.JobGenerateNPC, err.Error())
			return
		}
		for _, n := range existing {
			usedNames[n] = true
		}

		// 2. Цикл генерации: имя + случайный стартовый мир; прогресс каждые
		// 10к (§4.1). Локальный rand на джоб — общие *rand.Rand не
		// потокобезопасны (AGENTS.md §0).
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		agents := make([]models.NPCAgent, 0, count)
		for i := 0; i < count; i++ {
			agents = append(agents, models.NPCAgent{
				ID:             uuid.New().String(),
				Name:           names.GenerateAgentName(rnd, usedNames),
				CurrentWorldID: worlds[i],
			})
			if (i+1)%10000 == 0 || i == count-1 {
				statusManager.Progress(generator.JobGenerateNPC, i+1)
			}
		}

		// 3. Вставка — одна COPY-транзакция (§4.2): всё или ничего.
		if err := h.npcRepo.BulkInsert(agents); err != nil {
			log.Printf("❌ GenerateNPC: BulkInsert: %v", err)
			statusManager.Fail(generator.JobGenerateNPC, err.Error())
			return
		}
		// Кэш позиций: массовая генерация — внешняя мутация, следующий тик
		// перезагрузит кэш одним ListAll (идея 26c A2).
		h.manager.MarkDirty()

		// 4. Метрика пачки + отчёт джоба (§8.1: last_bulk, §4.1: отчёт).
		duration := time.Since(start)
		h.manager.RecordBulk(count, duration)
		statusManager.SetReport(generator.JobGenerateNPC,
			fmt.Sprintf("✅ Создано агентов: %d за %.1f с", count, duration.Seconds()))
		statusManager.Done(generator.JobGenerateNPC)
	}()

	writeJSONStatus(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// ==================== МАССОВОЕ УДАЛЕНИЕ (правка создателя 2026-09-15) ====================

// ClearAllAgents — POST /admin/npc/clear: удаляет ВСЕХ агентов одним DELETE
// без WHERE (атомарно и быстро, НЕ одиночные DELETE по id) → {"deleted": N}.
// 409 — идёт джоб генерации (паттерн ClearUniverse: живые прогоны, пишущие
// в одни таблицы, не пересекаются — AGENTS.md §23; параллельный DELETE
// порвал бы пачку COPY пополам). После DELETE очищается in-memory:
// позиции для карты (без «призраков» между удалением и следующим тиком)
// и last_bulk. Следующий тик пересчитает позиции из БД (там пусто).
func (h *AdminNPCHandlers) ClearAllAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	if statusManager.IsRunning(generator.JobGenerateNPC) {
		writeJSONError(w, "Генерация уже идёт", http.StatusConflict)
		return
	}
	deleted, err := h.npcRepo.DeleteAll()
	if err != nil {
		log.Printf("ClearAllAgents: DeleteAll error: %v", err)
		writeJSONError(w, "Не удалось удалить агентов", http.StatusInternalServerError)
		return
	}
	h.manager.OnAgentsDeleted()
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"deleted": deleted})
}