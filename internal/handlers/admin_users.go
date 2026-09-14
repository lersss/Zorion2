// internal/handlers/admin_users.go
// Раздел «Пользователи» админки (спека 99.2.14 §6). Все ручки закрыты
// auth.SkycomposerAuth на уровне регистрации роутов (И3). Инварианты §7
// (И1 — ≥ 1 skycomposer, И2 — нельзя менять/удалять себя) проверяются
// в репозитории транзакционно; здесь ошибки переводятся в 403/404.
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
)

type AdminUsersHandlers struct {
	userRepo  *repository.UserRepository
	worldRepo *repository.WorldRepository
}

func NewAdminUsersHandlers(userRepo *repository.UserRepository, worldRepo *repository.WorldRepository) *AdminUsersHandlers {
	return &AdminUsersHandlers{userRepo: userRepo, worldRepo: worldRepo}
}

// userListItem — строка списка §6.1 (без чувствительных и служебных полей).
type userListItem struct {
	ID        string      `json:"id"`
	Username  string      `json:"username"`
	Email     *string     `json:"email,omitempty"`
	Role      models.Role `json:"role"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// ==================== КОЛЛЕКЦИЯ: GET /admin/users, POST /admin/users ====================

// HandleCollection — диспетчер по методу для /admin/users.
func (h *AdminUsersHandlers) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListUsers(w, r)
	case http.MethodPost:
		h.CreateUser(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// ListUsers — GET /admin/users?query=&role=&page=&limit= (§6.1).
func (h *AdminUsersHandlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	role := r.URL.Query().Get("role")
	if role != "" && !validRole(role) {
		writeJSONError(w, "Неверный фильтр роли", http.StatusBadRequest)
		return
	}
	page, err := parsePositiveInt(r.URL.Query().Get("page"), 1)
	if err != nil {
		writeJSONError(w, "Неверный параметр page", http.StatusBadRequest)
		return
	}
	limit, err := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	if err != nil {
		writeJSONError(w, "Неверный параметр limit", http.StatusBadRequest)
		return
	}
	if limit > 100 {
		limit = 100
	}

	total, err := h.userRepo.Count(query, role)
	if err != nil {
		log.Printf("ListUsers: Count error: %v", err)
		writeJSONError(w, "Не удалось получить список пользователей", http.StatusInternalServerError)
		return
	}
	users, err := h.userRepo.List(query, role, page, limit)
	if err != nil {
		log.Printf("ListUsers: List error: %v", err)
		writeJSONError(w, "Не удалось получить список пользователей", http.StatusInternalServerError)
		return
	}

	items := make([]userListItem, 0, len(users))
	for _, u := range users {
		items = append(items, userListItem{
			ID:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			Role:      u.Role,
			CreatedAt: u.CreatedAt,
			UpdatedAt: u.UpdatedAt,
		})
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"total": total,
		"page":  page,
		"limit": limit,
		"users": items,
	})
}

// CreateUser — POST /admin/users (§6.3). Skycomposer вправе создать любую роль.
func (h *AdminUsersHandlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, "Имя пользователя и пароль обязательны", http.StatusBadRequest)
		return
	}
	role := req.Role
	if role == "" {
		role = string(models.RolePlayer)
	}
	if !validRole(role) {
		writeJSONError(w, "Неверная роль", http.StatusBadRequest)
		return
	}

	existing, _ := h.userRepo.GetByUsername(req.Username)
	if existing != nil {
		writeJSONError(w, "Имя пользователя уже занято", http.StatusConflict)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("CreateUser: bcrypt error: %v", err)
		writeJSONError(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	user := &models.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		PasswordHash: string(hashed),
		ShipIcon:     "ship_strela.svg",
		Role:         models.Role(role),
	}
	if req.Email != "" {
		user.Email = &req.Email
	}
	if err := h.userRepo.Create(user); err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Гонка: имя заняли между pre-check'ом и INSERT'ом
			writeJSONError(w, "Имя пользователя уже занято", http.StatusConflict)
			return
		}
		log.Printf("CreateUser: repo error: %v", err)
		writeJSONError(w, "Не удалось создать пользователя", http.StatusInternalServerError)
		return
	}

	h.writeProfile(w, http.StatusCreated, user)
}

// ==================== ОБЪЕКТ: /admin/users/{id} ====================

// HandleUser — диспетчер по методу для /admin/users/{id} и
// /admin/users/{id}/reset-password.
func (h *AdminUsersHandlers) HandleUser(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/users/")
	if path == "" || path == r.URL.Path {
		writeJSONError(w, "ID не указан", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		writeJSONError(w, "ID не указан", http.StatusBadRequest)
		return
	}
	// Не-UUID-строка в {id} упала бы в БД с pq: invalid input syntax for type
	// uuid → 500. Отсекаем явно, единый 400 на всех ручках раздела.
	if _, err := uuid.Parse(id); err != nil {
		writeJSONError(w, "Невалидный ID", http.StatusBadRequest)
		return
	}

	if len(parts) == 2 && parts[1] == "reset-password" {
		if r.Method != http.MethodPost {
			writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}
		h.ResetPassword(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.GetUser(w, r, id)
	case http.MethodPatch:
		h.UpdateRole(w, r, id)
	case http.MethodDelete:
		h.DeleteUser(w, r, id)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// GetUser — GET /admin/users/{id} (§6.2): полный профиль.
func (h *AdminUsersHandlers) GetUser(w http.ResponseWriter, r *http.Request, id string) {
	user, err := h.userRepo.GetByID(id)
	if err != nil {
		log.Printf("GetUser: GetByID(%s) error: %v", id, err)
		writeJSONError(w, "Не удалось получить профиль", http.StatusInternalServerError)
		return
	}
	if user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	h.writeProfile(w, http.StatusOK, user)
}

// UpdateRole — PATCH /admin/users/{id}/role (§6.4). Инварианты И1/И2 — в
// транзакции репозитория; здесь — перевод ошибок в 403/404.
func (h *AdminUsersHandlers) UpdateRole(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if !validRole(req.Role) {
		writeJSONError(w, "Неверная роль", http.StatusBadRequest)
		return
	}

	callerID := contextUserID(r)
	err := h.userRepo.UpdateRole(id, models.Role(req.Role), callerID)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		case errors.Is(err, repository.ErrSelfChange), errors.Is(err, repository.ErrLastSkycomposer):
			writeJSONError(w, err.Error(), http.StatusForbidden)
		default:
			log.Printf("UpdateRole: repo error: %v", err)
			writeJSONError(w, "Не удалось сменить роль", http.StatusInternalServerError)
		}
		return
	}

	user, err := h.userRepo.GetByID(id)
	if err != nil || user == nil {
		log.Printf("UpdateRole: reload profile error: %v", err)
		writeJSONError(w, "Не удалось получить профиль", http.StatusInternalServerError)
		return
	}
	h.writeProfile(w, http.StatusOK, user)
}

// ResetPassword — POST /admin/users/{id}/reset-password (§6.5).
// Самому себе — нельзя (И2, спека §9.3).
func (h *AdminUsersHandlers) ResetPassword(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.Password == "" {
		writeJSONError(w, "Пароль обязателен", http.StatusBadRequest)
		return
	}
	if id == contextUserID(r) {
		writeJSONError(w, repository.ErrSelfChange.Error(), http.StatusForbidden)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("ResetPassword: bcrypt error: %v", err)
		writeJSONError(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	err = h.userRepo.UpdatePassword(id, string(hashed))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
			return
		}
		log.Printf("ResetPassword: repo error: %v", err)
		writeJSONError(w, "Не удалось сбросить пароль", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteUser — DELETE /admin/users/{id} (§6.6). Инварианты И1/И2 — в
// транзакции репозитория; здесь — перевод ошибок в 403/404.
func (h *AdminUsersHandlers) DeleteUser(w http.ResponseWriter, r *http.Request, id string) {
	err := h.userRepo.Delete(id, contextUserID(r))
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		case errors.Is(err, repository.ErrSelfChange), errors.Is(err, repository.ErrLastSkycomposer):
			writeJSONError(w, err.Error(), http.StatusForbidden)
		default:
			log.Printf("DeleteUser: repo error: %v", err)
			writeJSONError(w, "Не удалось удалить пользователя", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ==================== ПОМОЩНИКИ ====================

// writeProfile — ответ профиля §6.2 (с названием текущего мира).
func (h *AdminUsersHandlers) writeProfile(w http.ResponseWriter, status int, user *models.User) {
	var currentWorldName string
	if user.CurrentWorldID != nil {
		world, err := h.worldRepo.GetByID(*user.CurrentWorldID)
		if err == nil && world != nil {
			currentWorldName = world.Name
		}
	}
	writeJSONStatus(w, status, map[string]interface{}{
		"id":                 user.ID,
		"username":           user.Username,
		"email":              user.Email,
		"role":               user.Role,
		"created_at":         user.CreatedAt,
		"updated_at":         user.UpdatedAt,
		"current_world_id":   user.CurrentWorldID,
		"current_world_name": currentWorldName,
		"ship_icon":          user.ShipIcon,
		"agent_id":           user.AgentID,
	})
}

// contextUserID — id текущего пользователя из контекста (кладёт AuthMiddleware).
func contextUserID(r *http.Request) string {
	id, _ := r.Context().Value(auth.UserIDKey).(string)
	return id
}

// validRole — допустимые значения роли (дублирует CHECK из миграции 000025).
func validRole(role string) bool {
	switch models.Role(role) {
	case models.RolePlayer, models.RoleAdmin, models.RoleSkycomposer:
		return true
	}
	return false
}

// parsePositiveInt — непустой целочисленный параметр (page/limit), 1-based.
func parsePositiveInt(raw string, def int) (int, error) {
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, errors.New("invalid positive int")
	}
	return n, nil
}

// writeJSONStatus — ответ JSON с явным кодом (дополнение к writeJSON без статуса).
func writeJSONStatus(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("⚠️ JSON encode failed: %v", err)
	}
}
