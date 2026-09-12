// cmd/server/main.go
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"zorion/internal/auth"
	"zorion/internal/config"
	"zorion/internal/generator/planet"
	"zorion/internal/handlers"
	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/travel"
	"zorion/migrations"
)

var db *sql.DB
var rdb *redis.Client

// noCache — запрещает браузеру использовать кэш без перевалидации.
// Без этого ES-модули (карта, админка) залипают в кэше и правки JS не видно.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func main() {
	cfg := config.Load()
	log.Printf("🚀 Запуск сервера Zorion на порту %s", cfg.ServerPort)
	log.Printf("⏱️  Интервал тика: %v", cfg.TickInterval)

	// Инициализация JWT-секрета. Делаем это ДО подключения к БД,
	// чтобы упасть как можно раньше, если секрет не задан или короткий.
	if err := auth.InitJWTSecret(cfg.JWTSecret); err != nil {
		log.Fatalf("❌ Не удалось инициализировать JWT-секрет: %v", err)
	}
	log.Println("✅ JWT-секрет инициализирован")

	var err error
	db, err = sql.Open("postgres", cfg.DBURL)
	if err != nil {
		log.Fatalf("❌ Ошибка подключения к PostgreSQL: %v", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatalf("❌ PostgreSQL не отвечает: %v", err)
	}
	log.Println("✅ PostgreSQL подключен")

	if err = migrations.Apply(db); err != nil {
		log.Fatalf("❌ Ошибка применения миграций: %v", err)
	}
	log.Println("✅ Миграции актуальны")

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("❌ Ошибка парсинга Redis URL: %v", err)
	}
	rdb = redis.NewClient(opt)
	if err = rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("❌ Redis не отвечает: %v", err)
	}
	log.Println("✅ Redis подключен")

	// Загрузка архетипов планет
	if err := planet.LoadArchetypes("config/planet_archetypes.json"); err != nil {
		log.Printf("⚠️ Не удалось загрузить архетипы планет: %v, использую fallback", err)
	} else {
		log.Println("✅ Архетипы планет загружены")
	}

	// Загрузка матрицы совместимости
	if err := loadCompatibilityMatrix(); err != nil {
		log.Printf("⚠️ Матрица совместимости: %v, использую встроенные дефолты", err)
	}

	// Загрузка библиотеки описаний планет.
	// Критично: без описаний все планеты получат fallback-текст.
	// Поэтому при ошибке — не стартуем.
	if err := planet.LoadDescriptionsGlobal("config/descriptions"); err != nil {
		log.Fatalf("❌ Не удалось загрузить описания планет: %v", err)
	}
	log.Println("✅ Описания планет загружены")

	worldRepo := repository.NewWorldRepository(db)
	locationRepo := repository.NewLocationRepository(db)
	assignmentRepo := repository.NewAssignmentRepository(db)
	userRepo := repository.NewUserRepository(db)

	travelManager := travel.NewManager()
	wsHub := handlers.NewWebSocketHub()

	testHandlers := handlers.NewTestHandlers(worldRepo, locationRepo, assignmentRepo)
	worldHandlers := handlers.NewWorldHandlers(worldRepo, locationRepo, assignmentRepo)
	authHandlers := handlers.NewAuthHandlers(userRepo, worldRepo)
	travelHandlers := handlers.NewTravelHandlers(worldRepo, userRepo, travelManager)
	wsHandler := handlers.NewWebSocketHandler(wsHub)
	contractHandlers := handlers.NewContractHandlers(assignmentRepo, userRepo)
	mapCache := mapcache.NewManager()
	adminHandlers := handlers.NewAdminHandlers(worldRepo, db, mapCache)
	compatHandlers := handlers.NewCompatibilityHandlers(db)

	// Снапшот карты подхватывается в фоне — сервер отвечает сразу,
	// карта заполняется за пару секунд после старта.
	mapCache.LoadAsync(db)

	// API открытые
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/register", authHandlers.Register)
	http.HandleFunc("/login", authHandlers.Login)

	// API защищённые JWT
	http.HandleFunc("/create-test-data", auth.AuthMiddleware(testHandlers.CreateTestData))
	http.HandleFunc("/worlds", auth.AuthMiddleware(worldHandlers.GetAllWorlds))
	http.HandleFunc("/worlds/", auth.AuthMiddleware(worldHandlers.GetWorld))
	http.HandleFunc("/travel", auth.AuthMiddleware(travelHandlers.StartTravel))
	http.HandleFunc("/me", auth.AuthMiddleware(authHandlers.GetMe))
	http.HandleFunc("/me/ship-icon", auth.AuthMiddleware(authHandlers.UpdateShipIcon))

	// API контрактов
	http.HandleFunc("/api/contracts", auth.AuthMiddleware(contractHandlers.GetContracts))
	http.HandleFunc("/api/contracts/take", auth.AuthMiddleware(contractHandlers.TakeContract))
	http.HandleFunc("/api/contracts/complete-test", auth.AuthMiddleware(contractHandlers.CompleteTestContract))

	// API планет
	http.HandleFunc("/api/worlds/", auth.AuthMiddleware(adminHandlers.GetPlanetsByWorld))

	// API фильтрации миров
	http.HandleFunc("/api/worlds/filter", auth.AuthMiddleware(adminHandlers.FilterWorldsHandler))

	// API регионов (для карты на малом зуме)
	http.HandleFunc("/api/regions", auth.AuthMiddleware(adminHandlers.GetRegionsHandler))

	// API поиска объектов (звезда/планета/спутник) по имени
	http.HandleFunc("/api/entities/search", auth.AuthMiddleware(adminHandlers.SearchEntitiesHandler))

	// API изображения планет
	http.HandleFunc("/api/planet-image", handlers.PlanetImageHandler)

	// WebSocket
	http.HandleFunc("/ws", auth.AuthMiddleware(wsHandler.ServeWS))

	// Админка (пароль)
	http.HandleFunc("/admin/worlds", auth.AdminAuth(adminHandlers.GetAllWorlds))
	http.HandleFunc("/admin/worlds/delete", auth.AdminAuth(adminHandlers.DeleteWorld))
	http.HandleFunc("/admin/worlds/create", auth.AdminAuth(adminHandlers.CreateWorld))
	http.HandleFunc("/admin/generate", auth.AdminAuth(adminHandlers.GenerateUniverse))
	http.HandleFunc("/admin/stats", auth.AdminAuth(adminHandlers.GetStats))
	http.HandleFunc("/admin/stats/planets", auth.AdminAuth(adminHandlers.GetPlanetStatsHandler))
	http.HandleFunc("/admin/generate-status", auth.AdminAuth(adminHandlers.GenerateStatus))
	http.HandleFunc("/admin/clear", auth.AdminAuth(adminHandlers.ClearUniverse))
	http.HandleFunc("/admin/generate-planets", auth.AdminAuth(adminHandlers.GeneratePlanets))
	http.HandleFunc("/admin/generate-prototype-planet", auth.AdminAuth(adminHandlers.GeneratePrototypePlanet))
	http.HandleFunc("/admin/generate-factions", auth.AdminAuth(adminHandlers.GenerateFactions))
	http.HandleFunc("/admin/generate-cancel", auth.AdminAuth(adminHandlers.CancelGeneration))

	// Аудит
	http.HandleFunc("/admin/audit", auth.AdminAuth(adminHandlers.GetAuditHandler))

	// Тесты
	http.HandleFunc("/admin/tests", auth.AdminAuth(adminHandlers.GetTestsHandler))

	// Матрица совместимости
	http.HandleFunc("/admin/compatibility", auth.AdminAuth(compatHandlers.HandleMatrix))
	http.HandleFunc("/admin/compatibility/reset", auth.AdminAuth(compatHandlers.ResetMatrix))

	http.Handle("/admin", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/admin.html")
	})))

	// Статика
	http.Handle("/static/", noCache(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static")))))

	// Страницы
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "./web/index.html")
			return
		}
		noCache(http.FileServer(http.Dir("./web"))).ServeHTTP(w, r)
	})
	http.Handle("/assignments", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/assignments.html")
	})))
	http.Handle("/login-page", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/login.html")
	})))
	http.Handle("/register-page", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/register.html")
	})))
	http.Handle("/map", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/map.html")
	})))
	http.Handle("/contracts", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/contracts.html")
	})))

	log.Println("🚀 Сервер Zorion запущен и работает")
	log.Fatal(http.ListenAndServe(":"+cfg.ServerPort, nil))
}

// ==================== ЗАГРУЗКА МАТРИЦЫ ====================

// loadCompatibilityMatrix — загружает матрицу совместимости.
// Сначала пробует из БД. Если БД пуста — из JSON-файла дефолтов.
func loadCompatibilityMatrix() error {
	repo := repository.NewCompatibilityRepository(db)

	count, err := repo.CountAll()
	if err != nil {
		return err
	}

	if count > 0 {
		if err := rebuildCacheFromDB(repo); err != nil {
			return err
		}
		log.Printf("✅ Матрица совместимости загружена из БД (%d пар)", count)
		return nil
	}

	if err := planet.LoadCompatibilityMatrix("config/compatibility_defaults.json"); err != nil {
		return err
	}
	log.Println("✅ Матрица совместимости загружена из JSON (БД пуста)")
	return nil
}

// rebuildCacheFromDB — читает обе категории из БД и пересобирает кеш.
func rebuildCacheFromDB(repo *repository.CompatibilityRepository) error {
	surfacePairs, err := repo.LoadAll(models.CompatCategorySurface)
	if err != nil {
		return err
	}
	subterrainPairs, err := repo.LoadAll(models.CompatCategorySubterrain)
	if err != nil {
		return err
	}

	surfaceMap := groupPairs(surfacePairs)
	subterrainMap := groupPairs(subterrainPairs)

	planet.RebuildCompatibilityMatrix(surfaceMap, subterrainMap)
	return nil
}

// groupPairs — группирует плоский список пар в map «A → [B, C]».
func groupPairs(pairs []*models.CompatibilityPair) map[string][]string {
	result := map[string][]string{}
	for _, p := range pairs {
		result[p.TypeA] = append(result[p.TypeA], p.TypeB)
	}
	return result
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		http.Error(w, "DB connection failed", http.StatusInternalServerError)
		return
	}
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		http.Error(w, "Redis connection failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("All systems ready"))
}