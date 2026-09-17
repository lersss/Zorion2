package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
)

type WorldHandlers struct {
	worldRepo      *repository.WorldRepository
	locationRepo   *repository.LocationRepository
	assignmentRepo *repository.AssignmentRepository

	// visibility — серверная видимость игрока (спека 77a §11): legacy-роуты
	// /worlds и /worlds/{id} не должны отдавать имя/данные за-радарной звезды
	// (И11). nil в тестах и для admin/skycomposer (видят всё, И7).
	visibility *Visibility
}

func NewWorldHandlers(
	worldRepo *repository.WorldRepository,
	locationRepo *repository.LocationRepository,
	assignmentRepo *repository.AssignmentRepository,
) *WorldHandlers {
	return &WorldHandlers{
		worldRepo:      worldRepo,
		locationRepo:   locationRepo,
		assignmentRepo: assignmentRepo,
	}
}

// SetVisibility — подключает серверную видимость игрока (спека 77a §11).
func (h *WorldHandlers) SetVisibility(v *Visibility) {
	h.visibility = v
}

// GetAllWorlds возвращает список всех миров. Для role=player — только миры
// в радиусе радара или «зажжённые» знанием (иначе /worlds = полный каталог
// одним запросом, обход гибрида «звёздное поле», И11). admin/skycomposer —
// без фильтра (И7).
func (h *WorldHandlers) GetAllWorlds(w http.ResponseWriter, r *http.Request) {
	worlds, err := h.worldRepo.GetAll()
	if err != nil {
		log.Printf("GetAllWorlds error: %v", err)
		http.Error(w, "Ошибка получения миров", http.StatusInternalServerError)
		return
	}

	if h.visibility != nil && roleFromContext(r) == string(models.RolePlayer) {
		worlds = h.filterWorldsByVisibility(r, worlds)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(worlds)
}

// filterWorldsByVisibility — оставляет миры, видимые игроку (спека 77a §11.2).
// Позиция игрока неизвестна — пустой список (безопасное направление).
func (h *WorldHandlers) filterWorldsByVisibility(r *http.Request, worlds []*models.World) []*models.World {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	user, err := h.visibility.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return nil
	}
	out := make([]*models.World, 0, len(worlds))
	for _, w := range worlds {
		if h.visibility.CanSeeWorld(user, w.ID, w.CoordX, w.CoordY) {
			out = append(out, w)
		}
	}
	return out
}

// GetWorld возвращает мир по ID с его локациями и заданиями.
// Ключевая особенность: если locations или assignments падают —
// это не повод возвращать 500. Мир важнее. Логируем и отдаём что есть.
func (h *WorldHandlers) GetWorld(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/worlds/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "ID не указан", http.StatusBadRequest)
		return
	}
	id := strings.Split(path, "/")[0]
	if id == "" {
		http.Error(w, "ID не указан", http.StatusBadRequest)
		return
	}

	world, err := h.worldRepo.GetByID(id)
	if err != nil {
		log.Printf("GetWorld: worldRepo.GetByID(%s) error: %v", id, err)
		http.Error(w, "Ошибка получения мира", http.StatusInternalServerError)
		return
	}
	if world == nil {
		http.Error(w, "Мир не найден", http.StatusNotFound)
		return
	}

	// Видимость игрока (спека 77a §11.2/И11): мир вне радиуса и не известен —
	// 403 для player (иначе цепочка search → /worlds/{id} отдаёт имя
	// за-радарной звезды). Текущий мир и цель/источник активного полёта —
	// доступны (CanSeeWorld). admin/skycomposer — без фильтра (И7).
	if h.visibility != nil && roleFromContext(r) == string(models.RolePlayer) {
		userID, _ := r.Context().Value(auth.UserIDKey).(string)
		user, err := h.visibility.userRepo.GetByID(userID)
		if err != nil || user == nil {
			writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
			return
		}
		if !h.visibility.CanSeeWorld(user, id, world.CoordX, world.CoordY) {
			writeJSONError(w, "вне зоны видимости", http.StatusForbidden)
			return
		}
	}

	// locations — вспомогательные данные. Их падение не блокирует ответ.
	locations, err := h.locationRepo.GetByWorld(id)
	if err != nil {
		log.Printf("GetWorld: locationRepo.GetByWorld(%s) error (пропускаем): %v", id, err)
		locations = nil
	}

	// assignments — тоже вспомогательные.
	assignments, err := h.assignmentRepo.GetByWorld(id)
	if err != nil {
		log.Printf("GetWorld: assignmentRepo.GetByWorld(%s) error (пропускаем): %v", id, err)
		assignments = nil
	}

	response := map[string]interface{}{
		"world":       world,
		"locations":   locations,
		"assignments": assignments,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}