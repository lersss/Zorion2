package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
)

type AuthHandlers struct {
	userRepo  *repository.UserRepository
	worldRepo *repository.WorldRepository
}

func NewAuthHandlers(userRepo *repository.UserRepository, worldRepo *repository.WorldRepository) *AuthHandlers {
	return &AuthHandlers{
		userRepo:  userRepo,
		worldRepo: worldRepo,
	}
}

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email,omitempty"`
		Role     string `json:"role"`
	} `json:"user"`
}

// Register обрабатывает регистрацию нового пользователя
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, "Имя пользователя и пароль обязательны", http.StatusBadRequest)
		return
	}

	existing, _ := h.userRepo.GetByUsername(req.Username)
	if existing != nil {
		writeJSONError(w, "Имя пользователя уже занято", http.StatusConflict)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSONError(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	user := &models.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		PasswordHash: string(hashed),
		ShipIcon:     "ship_strela.svg",
		Role:         models.RolePlayer,
	}
	if req.Email != "" {
		user.Email = &req.Email
	}
	// Устанавливаем текущий мир в nil — позже будет установлен при первом полёте
	if err := h.userRepo.Create(user); err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Гонка: имя заняли между pre-check'ом и INSERT'ом
			writeJSONError(w, "Имя пользователя уже занято", http.StatusConflict)
			return
		}
		log.Printf("register failed: %v", err)
		writeJSONError(w, "Не удалось создать пользователя", http.StatusInternalServerError)
		return
	}

	token, err := auth.GenerateToken(user.ID, string(user.Role))
	if err != nil {
		writeJSONError(w, "Не удалось сгенерировать токен", http.StatusInternalServerError)
		return
	}

	resp := AuthResponse{
		Token: token,
	}
	resp.User.ID = user.ID
	resp.User.Username = user.Username
	resp.User.Role = string(user.Role)
	if user.Email != nil {
		resp.User.Email = *user.Email
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// Login обрабатывает вход пользователя
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, "Имя пользователя и пароль обязательны", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetByUsername(req.Username)
	if err != nil || user == nil {
		writeJSONError(w, "Неверный логин или пароль", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeJSONError(w, "Неверный логин или пароль", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(user.ID, string(user.Role))
	if err != nil {
		writeJSONError(w, "Не удалось сгенерировать токен", http.StatusInternalServerError)
		return
	}

	resp := AuthResponse{
		Token: token,
	}
	resp.User.ID = user.ID
	resp.User.Username = user.Username
	resp.User.Role = string(user.Role)
	if user.Email != nil {
		resp.User.Email = *user.Email
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GetMe возвращает данные текущего пользователя с названием текущего мира
func (h *AuthHandlers) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		writeJSONError(w, "Пользователь не найден", http.StatusInternalServerError)
		return
	}
	if user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}

	var currentWorldName string
	if user.CurrentWorldID != nil {
		world, err := h.worldRepo.GetByID(*user.CurrentWorldID)
		if err == nil && world != nil {
			currentWorldName = world.Name
		}
	}

	response := map[string]interface{}{
		"id":                 user.ID,
		"username":           user.Username,
		"email":              user.Email,
		"role":               user.Role,
		"current_world_id":   user.CurrentWorldID,
		"current_world_name": currentWorldName,
		"ship_icon":          user.ShipIcon,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type ShipIconRequest struct {
	ShipIcon string `json:"ship_icon"`
}

// UpdateShipIcon сохраняет выбранную иконку корабля пользователя.
func (h *AuthHandlers) UpdateShipIcon(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	var req ShipIconRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.ShipIcon == "" || !validShipIcon(req.ShipIcon) {
		writeJSONError(w, "Некорректное имя иконки корабля", http.StatusBadRequest)
		return
	}

	if err := h.userRepo.UpdateShipIcon(userID, req.ShipIcon); err != nil {
		writeJSONError(w, "Не удалось сохранить иконку", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ship_icon": req.ShipIcon})
}

// validShipIcon допускает только безопасное имя SVG-файла спрайта.
func validShipIcon(name string) bool {
	if len(name) > 64 {
		return false
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' {
			continue
		}
		return false
	}
	return true
}

// writeJSONError возвращает ошибку в формате JSON {"error": "..."}.
// Фронтенд парсит ответ как JSON (res.json()), поэтому ошибки должны быть JSON,
// а не plain-text (http.Error), иначе fetch бросит исключение при парсинге.
func writeJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
