// internal/config/config.go
package config

import (
	"log"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ServerPort                   string
	DBURL                        string
	RedisURL                     string
	TickInterval                 time.Duration
	JWTSecret                    string
	SkycomposerBootstrapUsername string
	SkycomposerBootstrapPassword string
	BalancerPresetsFile          string
	RaceBalancerFile             string
	PlanetImageCacheDir          string
	OpenCodeURL                  string
	OpenCodeModel                string
	OpenCodeAgent                string
	OpenCodeTimeout              time.Duration
	OpenCodeMaxRetries           int
}

func Load() *Config {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL not set")
	}

	tickSec := os.Getenv("TICK_INTERVAL_SEC")
	if tickSec == "" {
		tickSec = "3"
	}
	sec, err := strconv.Atoi(tickSec)
	if err != nil {
		log.Fatalf("invalid TICK_INTERVAL_SEC: %v", err)
	}

	// JWT_SECRET — обязательная переменная окружения.
	// Без неё сервер не стартует: безопасность важнее удобства разработки.
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET not set — задайте переменную окружения (минимум 32 байта)")
	}

	// Bootstrap первого skycomposer (спека 99.2.14 §5): env необязательны,
	// срабатывают только пока в БД нет ни одной учётки с ролью skycomposer.
	skyUsername := os.Getenv("SKYCOMPOSER_BOOTSTRAP_USERNAME")
	skyPassword := os.Getenv("SKYCOMPOSER_BOOTSTRAP_PASSWORD")

	// Путь к JSON-файлу пресетов кривых балансировщика (спека 99.2.17 §5/§8):
	// дефолт config/balancer_presets.json (папка конфигов проекта, файл в git);
	// env необязательна — страховка на прод, где config/ может быть read-only.
	pfile := os.Getenv("BALANCER_PRESETS_FILE")
	if pfile == "" {
		pfile = "config/balancer_presets.json"
	}

	// Путь к JSON-файлу расовых R-кривых (спека 99.2.23 §3.1): дефолт
	// config/race_balancer.json; env RACE_BALANCER_FILE переопределяет
	// (паттерн BALANCER_PRESETS_FILE, 99.2.17 §8).
	rfile := os.Getenv("RACE_BALANCER_FILE")
	if rfile == "" {
		rfile = "config/race_balancer.json"
	}

	// Путь к каталогу диск-кэша большой картинки планеты (спека 2026-09-20
	// §5.2): дефолт data/planet_images (локально); env PLANET_IMAGE_CACHE_DIR
	// переопределяет (паттерн BALANCER_PRESETS_FILE) — на проде задаётся в
	// место с правом записи (data/ может быть read-only, DEPLOY.md §1).
	pimgDir := os.Getenv("PLANET_IMAGE_CACHE_DIR")
	if pimgDir == "" {
		pimgDir = "data/planet_images"
	}

	// Конфиг opencode — ИИ «заполнить комплектующие» (спека
	// переноса-студии-товаров-iterC §4): env с дефолтами из старого
	// config/goods/studio.json (удаляется со старой студией). На проде env
	// не заданы — fill честно падает «ИИ недоступен» (dev-инструмент).
	ocURL := os.Getenv("OPENCODE_URL")
	if ocURL == "" {
		ocURL = "http://127.0.0.1:3456"
	}
	ocModel := os.Getenv("OPENCODE_MODEL")
	if ocModel == "" {
		ocModel = "opencode/deepseek-v4-flash"
	}
	// Агент opencode: в 1.18 пустая строка резолвится в default_agent проекта
	// (opencode.json → "manager") — модель отвечает прозой менеджера, а не
	// JSON; поэтому агент задаётся явно (дефолт build).
	ocAgent := os.Getenv("OPENCODE_AGENT")
	if ocAgent == "" {
		ocAgent = "build"
	}
	ocTimeoutS := os.Getenv("OPENCODE_TIMEOUT_S")
	if ocTimeoutS == "" {
		ocTimeoutS = "120"
	}
	ocTimeout, err := strconv.Atoi(ocTimeoutS)
	if err != nil {
		log.Fatalf("invalid OPENCODE_TIMEOUT_S: %v", err)
	}
	ocRetries := os.Getenv("OPENCODE_MAX_RETRIES")
	if ocRetries == "" {
		ocRetries = "2"
	}
	ocMaxRetries, err := strconv.Atoi(ocRetries)
	if err != nil {
		log.Fatalf("invalid OPENCODE_MAX_RETRIES: %v", err)
	}

	return &Config{
		ServerPort:                   port,
		DBURL:                        dbURL,
		RedisURL:                     redisURL,
		TickInterval:                 time.Duration(sec) * time.Second,
		JWTSecret:                    jwtSecret,
		SkycomposerBootstrapUsername: skyUsername,
		SkycomposerBootstrapPassword: skyPassword,
		BalancerPresetsFile:          pfile,
		RaceBalancerFile:             rfile,
		PlanetImageCacheDir:          pimgDir,
		OpenCodeURL:                  ocURL,
		OpenCodeModel:                ocModel,
		OpenCodeAgent:                ocAgent,
		OpenCodeTimeout:              time.Duration(ocTimeout) * time.Second,
		OpenCodeMaxRetries:           ocMaxRetries,
	}
}
