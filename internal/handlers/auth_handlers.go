package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

type AuthHandlers struct {
	userRepo      *repository.UserRepository
	worldRepo     *repository.WorldRepository
	travelManager *travel.Manager
	// planetRepo — для пересчёта серверного HP на поверхности (§8.7, /me).
	planetRepo *repository.PlanetRepository
	// accountRepo — ленивая страховка счёта игрока при первом запросе
	// (спека 2026-09-22-деньги-и-эскроу §3.4). nil в тестах без денег.
	accountRepo *repository.AccountRepository
}

func NewAuthHandlers(userRepo *repository.UserRepository, worldRepo *repository.WorldRepository, travelManager *travel.Manager) *AuthHandlers {
	return &AuthHandlers{
		userRepo:      userRepo,
		worldRepo:     worldRepo,
		travelManager: travelManager,
	}
}

// SetPlanetRepo — подключение planetRepo для пересчёта HP прогулки (§8.7).
// Отдельный сеттер: не менять сигнатуру конструктора (легаси-тесты).
func (h *AuthHandlers) SetPlanetRepo(repo *repository.PlanetRepository) {
	h.planetRepo = repo
}

// SetAccountRepo — подключение accountRepo для ленивой страховки счёта
// игрока при первом запросе (§3.4). Отдельный сеттер: не менять сигнатуру
// конструктора (легаси-тесты).
func (h *AuthHandlers) SetAccountRepo(repo *repository.AccountRepository) {
	h.accountRepo = repo
}

// recomputeSurfaceHP — hp на поверхности пересчитывается от landed_at и профиля
// планеты (§8.7, ИП-5'): сохранённое hp — не источник истины. Не падает при
// битой планете (оставляет сохранённое значение). worldID — система игрока.
func (h *AuthHandlers) recomputeSurfaceHP(pos *models.CurrentPosition, worldID string) {
	if h.planetRepo == nil || pos == nil || pos.Status != "surface" || pos.ObjectID == "" || worldID == "" {
		return
	}
	planets, err := h.planetRepo.GetPlanetsLightByWorldID(worldID)
	if err != nil {
		return
	}
	p := findPlanetByID(planets, pos.ObjectID)
	if p == nil {
		return
	}
	biome := NormalizeSurfaceBiome(p, pos.Biome)
	hazard := surfaceHazardFor(p, biome)
	landedAt, _ := time.Parse(time.RFC3339, pos.LandedAt)
	hp := surfaceHPAt(landedAt, hazard.Total, time.Now())
	pos.HP = &hp
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
		ShipIcon:     models.DefaultShipIcon,
		Role:         models.RolePlayer,
	}
	if req.Email != "" {
		user.Email = &req.Email
	}
	// Стартовый мир назначается СРАЗУ при регистрации (решение создателя
	// 2026-09-17: «давай его пока к людям кидать»): игрок без current_world_id
	// не видит карту — видимость 77a требует позицию (PlayerPosition → ok=false
	// → все кластеры урезаются до точек-огоньков, полёт невозможен). Выбор:
	// мир с поселением расы humans (ближайший к центру), фолбэк — ближайший
	// к центру вообще; миров нет — NULL (регистрация не ломается).
	spawnWorld, err := h.worldRepo.PickSpawnWorld(r.Context())
	if err != nil {
		log.Printf("register: PickSpawnWorld: %v (current_world_id = NULL)", err)
	} else {
		user.CurrentWorldID = spawnWorld
	}
	// Счёт создаётся в ОДНОЙ транзакции с пользователем (спека
	// 2026-09-22-деньги-и-эскроу §3.4): регистрация не может оставить
	// игрока без счёта — иначе основная петля «новичок берёт перелёт и
	// получает оплату» ломается на зачислении.
	if err := h.userRepo.CreateWithAccount(user); err != nil {
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

	user, pos, pendingDest, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil {
		writeJSONError(w, "Пользователь не найден", http.StatusInternalServerError)
		return
	}
	if user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}

	// Ленивая идемпотентная страховка счёта игрока при первом запросе
	// (спека 2026-09-22-деньги-и-эскроу §3.4): покрывает старых игроков и
	// админ-созданные учётки. Best-effort — сбой не ломает /me.
	if h.accountRepo != nil && user.Role == models.RolePlayer {
		if err := h.accountRepo.EnsureAccount(models.AccountOwnerPlayer, userID, models.PlayerBalanceSeed); err != nil {
			log.Printf("/me: ensureAccount(%s): %v", userID, err)
		}
	}

	var currentWorldName string
	if user.CurrentWorldID != nil {
		world, err := h.worldRepo.GetByID(*user.CurrentWorldID)
		if err == nil && world != nil {
			currentWorldName = world.Name
		}
	}

	// Спека 2026-09-21 §7.6 п.6: на поверхности hp пересчитывается от landed_at
	// на этом чтении (§8.7) — клиент показывает актуальное значение.
	worldID := ""
	if user.CurrentWorldID != nil {
		worldID = *user.CurrentWorldID
	}
	h.recomputeSurfaceHP(pos, worldID)

	// Активный полёт (идея 42a): сервер помнит его в travel.Manager, фронт
	// восстанавливает состояние после рефреша. Полёта нет — null.
	var flight interface{}
	if f := h.travelManager.GetFlight(userID); f != nil {
		flight = map[string]interface{}{
			"from":       f.FromWorld,
			"to":         f.ToWorld,
			"start_time": f.StartTime.UnixMilli(),
			"duration":   int(f.Duration.Seconds()),
			"start_x":    f.StartX,
			"start_y":    f.StartY,
		}
	}

	// Спека 91a §7.3: раздел «Корабль» — имя модели (ship_models), каталог
	// оборудования (id/type/name/params, включая engine_1) и скорость из
	// установленного двигателя. Существующие поля не меняются (И3);
	// ship_speed_factor отдаётся только при установленном двигателе (иначе
	// ячейка «не установлен», скорость не показывается как текущая, §6.1).
	var shipModel interface{}
	if user.ShipModelID != nil {
		if m := ship.ShipModelByID(*user.ShipModelID); m != nil {
			shipModel = m
		}
	}
	// Роль для UI-гейта — из токена (её же проверяют админские ручки), а не из
	// БД: смена роли после выдачи токена иначе рассинхронит панель и ручки
	// (403-спам в админке, идея 2026-09-23). Пусто в контексте — фолбэк на БД.
	role := string(user.Role)
	if ctxRole, ok := r.Context().Value(auth.RoleKey).(string); ok && ctxRole != "" {
		role = ctxRole
	}

	response := map[string]interface{}{
		"id":                 user.ID,
		"username":           user.Username,
		"email":              user.Email,
		"role":               role,
		"current_world_id":   user.CurrentWorldID,
		"current_world_name": currentWorldName,
		// Спека 61b §4: ship_icon маппится при чтении (legacy SVG-имя → PNG-имя,
		// неизвестное → дефолт); ship_color — NULL = «Оригинал»; ship_options —
		// реестр 21 спрайта для дашборда (И1: клиент не дублирует список).
		"ship_icon":    models.ResolveShipIcon(user.ShipIcon),
		"ship_color":   user.ShipColor,
		"ship_options": models.ShipSprites,
		// Спека 77a §12: модель корабля, установленное оборудование и
		// вычисленный радиус радара (для отрисовки границы видимости на карте).
		"ship_model_id": user.ShipModelID,
		"equipment":     user.Equipment,
		"radar_radius":  ship.RadarRadius(user.Equipment),
		// Спека 91a §7.3: раздел «Корабль» (аддитивно, И3).
		"ship_model":   shipModel,
		"ship_catalog": ship.AllEquipment(),
		"flight":       flight,
		// Спека 99.2.27 §4.3: внутрисистемная позиция (users.current_position
		// JSONB) или null. При активном внутрисистемном полёте — status=in_flight
		// с from/to/start_time/arrive_at: восстановление после рефреша (42a)
		// идёт из позиции; отдельного поля intrasystem_flight нет (один источник).
		"current_position": pos,
		// Спека 99.2.30 §6.3: намерение композитного маршрута
		// (users.pending_destination JSONB) или null. Аддитивное поле —
		// существующие не меняются; клиент карты распознаёт автостарт
		// внутрисистемного сегмента по прибытии (M5, §6.8).
		"pending_destination": pendingDest,
	}
	if ship.HasEngine(user.Equipment) {
		response["ship_speed_factor"] = ship.EngineSpeed(user.Equipment)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type ShipIconRequest struct {
	ShipIcon string `json:"ship_icon"`
}

// UpdateShipIcon сохраняет выбранную иконку корабля пользователя.
// Спека 61b §4: принимаются только имена из реестра ShipSprites (21 PNG-имя);
// legacy-имена и прочие → 400 (выбор теперь делается из нового набора).
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
	if !models.IsValidShipIcon(req.ShipIcon) {
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

type ShipColorRequest struct {
	ShipColor *string `json:"ship_color"`
}

// UpdateShipColor сохраняет цвет перекраски спрайта корабля (спека 61b §7):
// NULL = «Оригинал» (без перекраски), иначе hex из ShipColorPalette; вне
// палитры → 400.
func (h *AuthHandlers) UpdateShipColor(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	var req ShipColorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.ShipColor != nil && !models.IsValidShipColor(*req.ShipColor) {
		writeJSONError(w, "Некорректный цвет корабля", http.StatusBadRequest)
		return
	}

	if err := h.userRepo.UpdateShipColor(userID, req.ShipColor); err != nil {
		writeJSONError(w, "Не удалось сохранить цвет", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ship_color": req.ShipColor})
}

// writeJSONError возвращает ошибку в формате JSON {"error": "..."}.
// Фронтенд парсит ответ как JSON (res.json()), поэтому ошибки должны быть JSON,
// а не plain-text (http.Error), иначе fetch бросит исключение при парсинге.
func writeJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
