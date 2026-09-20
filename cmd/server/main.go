// cmd/server/main.go
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"zorion/internal/auth"
	"zorion/internal/config"
	economySettlement "zorion/internal/economy/settlement"
	"zorion/internal/generator/planet"
	"zorion/internal/generator/settlement"
	"zorion/internal/goodsstudio"
	"zorion/internal/goodsstudio/ai"
	"zorion/internal/handlers"
	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/npc"
	"zorion/internal/races"
	"zorion/internal/regionprofile"
	"zorion/internal/repository"
	"zorion/internal/ship"
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
	// Go не читает .env автоматически. Загружаем его ДО config.Load(),
	// чтобы JWT_SECRET/DBURL/REDIS_URL брались единообразно из файла и рестарты
	// не меняли секрет. Уже заданные переменные окружения имеют приоритет
	// (godotenv их не перетирает). Отсутствие .env — не ошибка: прод задаёт
	// env иначе (Docker/Amvera).
	if err := godotenv.Load(); err != nil {
		log.Printf("⚠️ .env не загружен (%v) — использую переменные окружения", err)
	}

	cfg := config.Load()
	log.Printf("🚀 Запуск сервера Zorion на порту %s", cfg.ServerPort)
	log.Printf("⏱️  Интервал тика: %v", cfg.TickInterval)

	// Пресеты кривых балансировщика (спека 99.2.17 §5/§8): файл читается
	// при старте — активные кривые восстанавливаются; нет файла/битый —
	// кривые = дефолты, пресеты сессионные (сервер не падает).
	if err := economySettlement.LoadBalancerPresets(cfg.BalancerPresetsFile); err != nil {
		log.Printf("⚠️ Балансировщик: пресеты кривых: %v (кривые = дефолты, пресеты сессионно)", err)
	} else {
		log.Println("✅ Пресеты кривых балансировщика загружены")
	}

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

	// Сидер каталога товаров/ресурсов (спека переноса-студии-товаров-iterA
	// §5): при первом старте (маркер goods_catalog_seed в generation_config)
	// сеет 6 ресурсных + 13 товарных категорий и 131 ресурс. Ошибка —
	// log.Fatal: сервер без базового каталога не стартует (решение гейта №4).
	if err := goodsstudio.Seed(db); err != nil {
		log.Fatalf("❌ Сидер каталога товаров: %v", err)
	}
	log.Println("✅ Каталог товаров: сид актуален")

	// Сидер каталога типов производителей и предметов (спека
	// 2026-09-20-фабрики §10.1 п.2): после goodsstudio.Seed — категории
	// уже посеяны (producer_types.category_id → categories). Маркер
	// producer_catalog_seed в generation_config; повторные старты — пропуск.
	if err := goodsstudio.SeedProducers(db); err != nil {
		log.Fatalf("❌ Сидер каталога производителей: %v", err)
	}
	log.Println("✅ Каталог производителей: сид актуален")

	// Каталог оборудования (спека 77a §3): справочник из БД (миграция 000040),
	// дефолты при пустой БД. Нужен до старта HTTP — радиус радара считается
	// из него (спека 77a §4.2).
	if err := ship.LoadCatalog(db); err != nil {
		log.Printf("⚠️ Каталог оборудования: %v (дефолты)", err)
	} else {
		log.Println("✅ Каталог оборудования загружен")
	}

	// Модели кораблей (спека 91a §7.3): справочник из БД (миграция 000040),
	// дефолт — стартовая модель. Нужен до старта HTTP — /me отдаёт имя модели.
	if err := ship.LoadModels(db); err != nil {
		log.Printf("⚠️ Модели кораблей: %v (дефолты)", err)
	} else {
		log.Println("✅ Модели кораблей загружены")
	}

	// Bootstrap первого skycomposer — после миграций, до старта HTTP (спека 99.2.14 §5).
	bootstrapSkycomposer(db, cfg.SkycomposerBootstrapUsername, cfg.SkycomposerBootstrapPassword)

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

	// Справочник биомов (99.2.28 §15.1): биомы поверхности, типы недр,
	// правила типов планет, параметры токсичности. Нет файла/битый JSON/
	// невалидный каталог → встроенный сид (сервер не падает, паттерн
	// LoadCompatibilityMatrix).
	if err := planet.LoadBiomeCatalog("config/biome_catalog.json"); err != nil {
		log.Printf("⚠️ Справочник биомов: %v — использую встроенный сид", err)
	} else {
		cat := planet.GetBiomeCatalog()
		log.Printf("✅ Справочник биомов загружен: %d биомов, %d типов недр, %d правил типов",
			len(cat.Biomes), len(cat.SubterrainTypes), len(cat.PlanetTypes))
	}

	// Каталог профилей регионов (59a, спека 99.2.10 §9): один JSON на класс.
	// Имена форм — точные константы composition_forms.go (валидация §9).
	// Ошибка загрузки — регионы остаются фоновыми (профиля нет), сервер живёт.
	if err := regionprofile.LoadProfiles("config/region_profiles", planet.AllSurfaceForms, planet.AllSubterrainTypes); err != nil {
		log.Printf("⚠️ Профили регионов: %v, регионы будут фоновыми", err)
	} else {
		log.Printf("✅ Профили регионов загружены: %d классов", len(regionprofile.Profiles()))
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

	// Загрузка пресета генерации поселений. При ошибке — дефолты.
	// СКРЫТ (65a): пресет человеческого генератора устарел — расовый
	// генератор заменяет; файл остаётся помеченным, не используется.
	if err := settlement.LoadPreset("config/settlement_preset.json"); err != nil {
		log.Printf("⚠️ Пресет поселений: %v, использую дефолты", err)
	} else {
		log.Println("✅ Пресет поселений загружен")
	}

	// Пресет расового генератора поселений (65a): шанс доминанты, крутилка
	// подселения соседа, население (config/race_settlement.json).
	// При ошибке — дефолты.
	if err := settlement.LoadRacePreset("config/race_settlement.json"); err != nil {
		log.Printf("⚠️ Пресет расовых поселений: %v, использую дефолты", err)
	} else {
		log.Println("✅ Пресет расовых поселений загружен")
	}

	// Каталог рас (спека 99.2.21 §2.3): config/races.json, 50 карточек.
	// Ошибка загрузки — расы не раздаются по территориям, генерация
	// поселений рас недоступна (сервер живёт).
	if err := races.LoadCatalog("config/races.json"); err != nil {
		log.Printf("⚠️ Каталог рас: %v, расы не раздаются", err)
	} else {
		log.Printf("✅ Каталог рас загружен: %d рас", len(races.Catalog()))
	}

	// Лор рас (спека 86a §5.1.1): config/race_lore.json — машиночитаемая
	// проекция 22_races.md §3/§4. Ошибка загрузки — энциклопедия отдаёт расы
	// без лора (сервер живёт, паттерн каталога рас).
	if err := races.LoadLore("config/race_lore.json"); err != nil {
		log.Printf("⚠️ Лор рас: %v, энциклопедия без лора", err)
	} else {
		log.Printf("✅ Лор рас загружен: %d записей", len(races.LoreCatalog()))
	}

	// Расовые R-кривые (спека 99.2.23 §4.4): файл читается при старте —
	// активные кривые восстанавливаются; раса без записи → factory/active
	// из карточки («один раз при создании расы»); битый JSON → лог, расы
	// инициализируются из карточек (сервер не падает).
	if err := economySettlement.LoadRaceBalancer(cfg.RaceBalancerFile); err != nil {
		log.Printf("⚠️ Расовый балансировщик: %v (расы инициализируются из карточек)", err)
	} else {
		log.Println("✅ Расовый балансировщик загружен")
	}

	worldRepo := repository.NewWorldRepository(db)
	locationRepo := repository.NewLocationRepository(db)
	assignmentRepo := repository.NewAssignmentRepository(db)
	userRepo := repository.NewUserRepository(db)

	// Активные полёты игроков (97a): персистентность в БД — полёт переживает
	// рестарт сервера. Restore — ДО старта HTTP (гонок нет): прошлые прибытия
	// засчитываются сразу (как у NPC), будущие — перерегистрируются с
	// оставшимся временем; битые миры (перегенерация) — полёт не восстанавливается.
	playerFlightRepo := repository.NewPlayerFlightRepository(db)
	travelManager := travel.NewManager(playerFlightRepo)

	// Внутрисистемные полёты (спека 99.2.27 §3.3/§3.5): персистентность в БД,
	// Restore ПОСЛЕ межзвёздных (С-1) — межзвёздная строка побеждает, intra
	// удаляется без onArrival (иначе позиция {planet, старый мир} запишется
	// при новом current_world_id — нарушение ИП-1).
	planetRepo := repository.NewPlanetRepository(db)
	knowledgeRepo := repository.NewKnowledgeRepository(db)
	intraFlightRepo := repository.NewPlayerIntrasystemFlightRepository(db)
	intraManager := travel.NewIntrasystemManager(intraFlightRepo)

	// Композитный маршрут (спека 99.2.30 §4.3): автостарт внутрисистемного
	// сегмента по onArrival межзвёздного полёта — нужны planetRepo
	// (валидация «объект жив») и knowledgeRepo (авто-знание presence).
	// Хендлер создаётся ДО Restore-фаз: onArrival-колбэк фазы 1 идёт через
	// общий ArrivalHandler (автостарт работает и для восстановленных полётов).
	travelHandlers := handlers.NewTravelHandlers(worldRepo, userRepo, travelManager)
	travelHandlers.SetIntrasystem(intraManager, intraFlightRepo)
	travelHandlers.SetIntrasystemAutostart(planetRepo, knowledgeRepo)

	// Фаза 1: Restore межзвёздных (97a) — onArrival через общий ArrivalHandler
	// (спека 99.2.30 §4/И6): прибывшие засчитываются сразу (ИП-2 + автостарт
	// композитного маршрута по намерению), летящие продолжаются с остатка.
	travelManager.Restore(time.Now(),
		func(id string) bool {
			w, err := worldRepo.GetByID(id)
			return err == nil && w != nil
		},
		travelHandlers.ArrivalHandler,
	)

	// Фаза 2: Restore внутрисистемных (99.2.27) — ПОСЛЕ межзвёздных (С-1).
	intraManager.RestoreIntra(time.Now(),
		func(worldID, objType, objID string) bool {
			w, err := worldRepo.GetByID(worldID)
			if err != nil || w == nil {
				return false
			}
			switch objType {
			case "star":
				if objID == worldID {
					return true
				}
				return handlers.IsValidCompanionID(w, objID)
			case "planet":
				p, err := planetRepo.GetPlanetByID(objID)
				return err == nil && p != nil && p.WorldID == worldID
			case "satellite":
				p, err := planetRepo.FindPlanetBySatellite(worldID, objID)
				return err == nil && p != nil
			}
			return false
		},
		func(userID string) bool { return travelManager.IsInFlight(userID) },
		func(userID string) string {
			u, err := userRepo.GetByID(userID)
			if err != nil || u == nil || u.CurrentWorldID == nil {
				return ""
			}
			return *u.CurrentWorldID
		},
		handlers.NewIntraArrivalHandler(intraFlightRepo, planetRepo, knowledgeRepo),
	)
	// Фаза 3: намерения композитного маршрута (спека 99.2.30 §4.5) — ПОСЛЕ
	// фаз 1–2: автостарт для живых целей, очистка призраков/битых/съеденных.
	// O(игроки с намерением).
	travelHandlers.RestorePendingDestinations()
	wsHub := handlers.NewWebSocketHub()

	testHandlers := handlers.NewTestHandlers(worldRepo, locationRepo, assignmentRepo)
	worldHandlers := handlers.NewWorldHandlers(worldRepo, locationRepo, assignmentRepo)
	authHandlers := handlers.NewAuthHandlers(userRepo, worldRepo, travelManager)
	// Внутрисистемные полёты (спека 99.2.27 §4.1): POST /api/intrasystem-flight.
	intrasystemHandlers := handlers.NewIntrasystemHandlers(
		worldRepo, userRepo, planetRepo, intraFlightRepo, knowledgeRepo, travelManager, intraManager,
	)
	wsHandler := handlers.NewWebSocketHandler(wsHub)
	contractHandlers := handlers.NewContractHandlers(assignmentRepo, userRepo)
	mapCache := mapcache.NewManager()
	adminHandlers := handlers.NewAdminHandlers(worldRepo, db, mapCache)
	compatHandlers := handlers.NewCompatibilityHandlers(db)

	// Серверная видимость игрока (спека 77a §11): круг радара для role=player.
	// Подключается к хендлерам карты/полёта; admin/skycomposer — без фильтра (И7).
	visibility := handlers.NewVisibility(userRepo, travelManager, mapCache, knowledgeRepo)
	adminHandlers.SetVisibility(visibility)
	adminHandlers.SetTravelManager(travelManager)
	worldHandlers.SetVisibility(visibility)

	// Нотификатор пакмана (спека 2026-09-20 §5): события вайпа видны всем
	// игрокам на карте через WebSocket Broadcast; джоб пишет неблокирующе,
	// рассылку делает горутина нотификатора (джоб не блокируется на WS).
	pacmanNotifier := handlers.NewPacmanNotifier(wsHub)
	pacmanNotifier.Start()
	adminHandlers.SetPacmanNotifier(pacmanNotifier)

	// Снапшот карты подхватывается в фоне — сервер отвечает сразу,
	// карта заполняется за пару секунд после старта.
	mapCache.LoadAsync(db)

	// NPC-агенты (спека 20a.1): фоновый планировщик, одна горутина,
	// тик каждые npcTickInterval. Стартует после загрузки карты —
	// сетка миров строится из снапшота mapcache.
	npcRepo := repository.NewNPCRepository(db)
	npcManager := npc.NewManager(npcRepo, npc.NewMapCacheSource(mapCache), npc.DefaultSettings())
	// Инвалидация кэша агентов при TRUNCATE npc_agents (ClearUniverse/
	// GenerateUniverse, идея 26c A2): позиции и кэш сбросятся сразу.
	adminHandlers.SetNPCManager(npcManager)
	// Уведомления (этап 5): WSNotifier подменяет заглушку LogNotifier ДО
	// старта тика, чтобы первые прибытия не терялись.
	npcAdminHandlers := handlers.NewAdminNPCHandlers(npcRepo, worldRepo, npcManager)
	npcAdminHandlers.SetVisibility(visibility)
	wsNotifier := handlers.NewWSNotifier(wsHub, npcManager,
		npcManager.Settings().NotifyInterval(), npcManager.Settings().NotificationMaxBatch())
	npcManager.SetNotifier(wsNotifier)
	wsNotifier.Start()
	npcManager.Start()

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
	http.HandleFunc("/api/intrasystem-flight", auth.AuthMiddleware(intrasystemHandlers.StartIntraFlight))
	http.HandleFunc("/me", auth.AuthMiddleware(authHandlers.GetMe))
	http.HandleFunc("/me/ship-icon", auth.AuthMiddleware(authHandlers.UpdateShipIcon))
	http.HandleFunc("/me/ship-color", auth.AuthMiddleware(authHandlers.UpdateShipColor))

	// Энциклопедия (спека 86a §8.1): публичный срез каталога рас + лор.
	// Игровой JWT (как /me); без токена — 401.
	encyclopediaHandlers := handlers.NewEncyclopediaHandlers()
	http.HandleFunc("/api/encyclopedia/races", auth.AuthMiddleware(encyclopediaHandlers.GetRaces))

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

	// API изображения планет (спека 2026-09-20 §3.2): авторизованный
	// /api/planet-image (planet_id + size, JWT обязателен; режим честная/
	// заглушка решает сервер). Диск-кэш большой картинки — каталог при старте
	// (MkdirAll, спека §5.2); на проде env PLANET_IMAGE_CACHE_DIR.
	if err := handlers.InitPlanetImageCacheDir(cfg.PlanetImageCacheDir); err != nil {
		log.Printf("⚠️ Диск-кэш картинок планет: %v", err)
	}
	planetImageHandler := handlers.NewPlanetImageHandler(planetRepo, userRepo, knowledgeRepo, cfg.PlanetImageCacheDir)
	http.HandleFunc("/api/planet-image", auth.AuthMiddleware(planetImageHandler.ServeHTTP))

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
	http.HandleFunc("/admin/pacman/start", auth.AdminAuth(adminHandlers.StartPacman))
	http.HandleFunc("/admin/generate-planets", auth.AdminAuth(adminHandlers.GeneratePlanets))
	http.HandleFunc("/admin/generate-prototype-planet", auth.AdminAuth(adminHandlers.GeneratePrototypePlanet))
	http.HandleFunc("/admin/generate-factions", auth.AdminAuth(adminHandlers.GenerateFactions))
	// СКРЫТ (65a): старый человеческий генератор поселений заменён расовым
	// (/admin/generate-race-settlements). Код хендлера остаётся в
	// internal/handlers/admin_settlements.go, роут не регистрируется.
	// http.HandleFunc("/admin/generate-settlements", auth.AdminAuth(adminHandlers.GenerateSettlements))
	http.HandleFunc("/admin/generate-race-settlements", auth.AdminAuth(adminHandlers.GenerateRaceSettlements))
	http.HandleFunc("/admin/settlement-fields", auth.AdminAuth(adminHandlers.SettlementFields))
	http.HandleFunc("/admin/generate-cancel", auth.AdminAuth(adminHandlers.CancelGeneration))
	http.HandleFunc("/admin/clear-settlements", auth.AdminAuth(adminHandlers.ClearSettlements))
	http.HandleFunc("/admin/hypothesis/run", auth.AdminAuth(adminHandlers.RunHypothesis))
	http.HandleFunc("/admin/mortality-preview", auth.AdminAuth(adminHandlers.MortalityPreview))
	http.HandleFunc("/admin/settlement-settings", auth.AdminAuth(adminHandlers.HandleSettlementSettings))

	// Балансировщик компонент изменения населения (спека 99.2.17 §6):
	// сегментные кривые R(X) — чтение/сохранение/сброс/серверная оцифровка/
	// эталоны. In-memory store, дефолты при рестарте.
	http.HandleFunc("/admin/balancer/curve", auth.AdminAuth(adminHandlers.HandleBalancerCurve))
	http.HandleFunc("/admin/balancer/curve/reset", auth.AdminAuth(adminHandlers.HandleBalancerCurveReset))
	http.HandleFunc("/admin/balancer/curve/sample", auth.AdminAuth(adminHandlers.HandleBalancerCurveSample))
	http.HandleFunc("/admin/balancer/etalons", auth.AdminAuth(adminHandlers.HandleBalancerEtalons))
	// Пресеты кривых (итерация 7, спека 99.2.17 §6).
	http.HandleFunc("/admin/balancer/presets", auth.AdminAuth(adminHandlers.HandleBalancerPresets))
	http.HandleFunc("/admin/balancer/presets/apply", auth.AdminAuth(adminHandlers.HandleBalancerPresetsApply))
	http.HandleFunc("/admin/balancer/presets/reset-default", auth.AdminAuth(adminHandlers.HandleBalancerPresetsResetDefault))

	// Расовые R-кривые (спека 99.2.23 §4.3): расширение вкладки «Балансировка»
	// селектором расы — active/factory кривые, reproduction, перегенерация из
	// карточки, возврат заводских, статус (селектор + пометки).
	http.HandleFunc("/admin/race-balancer/curve", auth.AdminAuth(adminHandlers.HandleRaceBalancerCurve))
	http.HandleFunc("/admin/race-balancer/reproduction", auth.AdminAuth(adminHandlers.HandleRaceBalancerReproduction))
	http.HandleFunc("/admin/race-balancer/generate", auth.AdminAuth(adminHandlers.HandleRaceBalancerGenerate))
	http.HandleFunc("/admin/race-balancer/reset-factory", auth.AdminAuth(adminHandlers.HandleRaceBalancerResetFactory))
	http.HandleFunc("/admin/race-balancer/factory", auth.AdminAuth(adminHandlers.HandleRaceBalancerFactory))
	http.HandleFunc("/admin/race-balancer/status", auth.AdminAuth(adminHandlers.HandleRaceBalancerStatus))

	// Каталог ресурсов и покрытие рас (спека 94a): read-only просмотр
	// универсального слоя (admin + skycomposer).
	http.HandleFunc("/admin/resources", auth.AdminAuth(adminHandlers.GetAdminResources))

	// Справочник биомов (99.2.28 §22): правка — admin, чтение — admin +
	// skycomposer (роль проверяется в хендлере для PATCH/POST — AdminAuth
	// пропускает обе роли). Атомарная запись файла + hot-reload store.
	http.HandleFunc("/admin/biome-catalog", auth.AdminAuth(adminHandlers.HandleBiomeCatalog))
	http.HandleFunc("/admin/biome-catalog/reset", auth.AdminAuth(adminHandlers.ResetBiomeCatalog))
	// Полосы климатов (99.2.28 §22.4): веса/списки planet_archetypes.json.
	http.HandleFunc("/admin/planet-archetypes", auth.AdminAuth(adminHandlers.HandlePlanetArchetypes))

	// Конфиг генерации и пересчёт планет (99.2.3 §4.5/§5)
	http.HandleFunc("/admin/generation/config", auth.AdminAuth(adminHandlers.HandleGenerationConfig))
	http.HandleFunc("/admin/regenerate-planets", auth.AdminAuth(adminHandlers.RegeneratePlanets))

	// Аудит
	http.HandleFunc("/admin/audit", auth.AdminAuth(adminHandlers.GetAuditHandler))

	// Тесты
	http.HandleFunc("/admin/tests", auth.AdminAuth(adminHandlers.GetTestsHandler))

	// Матрица совместимости
	http.HandleFunc("/admin/compatibility", auth.AdminAuth(compatHandlers.HandleMatrix))
	http.HandleFunc("/admin/compatibility/reset", auth.AdminAuth(compatHandlers.ResetMatrix))

	// Раздел «Пользователи» — только Skycomposer (спека 99.2.14 §6, И3)
	adminUsersHandlers := handlers.NewAdminUsersHandlers(userRepo, worldRepo)
	http.HandleFunc("/admin/users", auth.SkycomposerAuth(adminUsersHandlers.HandleCollection))
	http.HandleFunc("/admin/users/", auth.SkycomposerAuth(adminUsersHandlers.HandleUser))

	// NPC-агенты (спека 20a.1 §8): админка (AdminAuth) + позиции для карты (JWT)
	// + поиск агента на карте (спека 26a.1 §6: игровая ручка, JWT).
	http.HandleFunc("/admin/npc", auth.AdminAuth(npcAdminHandlers.HandleCollection))
	http.HandleFunc("/admin/npc/", auth.AdminAuth(npcAdminHandlers.HandleObject))
	http.HandleFunc("/api/npc/positions", auth.AuthMiddleware(npcAdminHandlers.Positions))
	http.HandleFunc("/api/npc/search", auth.AuthMiddleware(npcAdminHandlers.SearchAgent))

	// Позиции чужих игроков (спека 77a §5.3): игровой JWT, фильтр по радиусу
	// радара запрашивающего на сервере (И1).
	http.HandleFunc("/api/players/positions", auth.AuthMiddleware(adminHandlers.PlayersPositions))

	// Студия товаров (спека переноса-студии-товаров-iterA §6): каталог
	// товаров/ресурсов на БД. API — только admin/skycomposer (player → 403);
	// HTML-страница — публична (паттерн админки, JWT в localStorage).
	// ИИ «заполнить комплектующие» (iterC §4): конфиг opencode из env
	// (OPENCODE_URL/MODEL/TIMEOUT_S/MAX_RETRIES, дефолты из studio.json);
	// на проде env не заданы — fill честно падает «ИИ недоступен».
	aiClient := ai.NewClient(cfg.OpenCodeURL, cfg.OpenCodeModel, cfg.OpenCodeTimeout, cfg.OpenCodeMaxRetries)
	studioHandlers := handlers.NewStudioHandlers(db, aiClient, cfg.OpenCodeModel)
	http.HandleFunc("/studio/api/state", auth.AdminAuth(studioHandlers.State))
	http.HandleFunc("/studio/api/resources", auth.AdminAuth(studioHandlers.Resources))
	http.HandleFunc("/studio/api/categories", auth.AdminAuth(studioHandlers.Categories))
	http.HandleFunc("/studio/api/categories/", auth.AdminAuth(studioHandlers.CategoryByID))
	http.HandleFunc("/studio/api/goods", auth.AdminAuth(studioHandlers.Goods))
	// bulk — отдельный роут: subtree /studio/api/goods/ (GoodByID) парсит
	// первый сегмент как id и вернул бы 404 на "bulk" (ревью iterA).
	http.HandleFunc("/studio/api/goods/bulk", auth.AdminAuth(studioHandlers.Goods))
	// fill/apply/cancel — ветки в GoodByID (паттерн status/tier/slots, спека
	// iterC §5: отдельные роуты не нужны — конфликта парсинга id нет).
	http.HandleFunc("/studio/api/goods/", auth.AdminAuth(studioHandlers.GoodByID))
	http.HandleFunc("/studio/api/validate", auth.AdminAuth(studioHandlers.Validate))
	// Ветки «Производители»/«Предметы» (спека 2026-09-20-фабрики §4):
	// producer_types/items/producer_items — тот же контракт, что goods.
	http.HandleFunc("/studio/api/producers", auth.AdminAuth(studioHandlers.Producers))
	http.HandleFunc("/studio/api/producers/", auth.AdminAuth(studioHandlers.ProducerByID))
	// Слоты родителя (спека 2026-09-21-студия-скрытые-категории-строений §4):
	// конфигурация категорий типа kind=goods, расово-зависимо.
	http.HandleFunc("/studio/api/slots", auth.AdminAuth(studioHandlers.Slots))
	http.HandleFunc("/studio/api/slots/", auth.AdminAuth(studioHandlers.SlotByID))
	// Уровни расовости дерева построек (спека 2026-09-21-студия-дерево-построек-канвас §3):
	// семейства F0–F9 + robotic и расы из internal/races (Go-конфиги, единый источник).
	http.HandleFunc("/studio/api/races", auth.AdminAuth(studioHandlers.Races))
	http.HandleFunc("/studio/api/items", auth.AdminAuth(studioHandlers.Items))
	http.HandleFunc("/studio/api/items/", auth.AdminAuth(studioHandlers.ItemByID))

	http.Handle("/studio", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/studio.html")
	})))
	http.Handle("/studio/", noCache(http.StripPrefix("/studio/", http.FileServer(http.Dir("./web/static/studio")))))

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
