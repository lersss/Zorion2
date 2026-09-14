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

	return &Config{
		ServerPort:                   port,
		DBURL:                        dbURL,
		RedisURL:                     redisURL,
		TickInterval:                 time.Duration(sec) * time.Second,
		JWTSecret:                    jwtSecret,
		SkycomposerBootstrapUsername: skyUsername,
		SkycomposerBootstrapPassword: skyPassword,
	}
}
