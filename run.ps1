#!/bin/bash
# Скрипт запуска Zorion локально
# Перед запуском убедитесь, что PostgreSQL и Redis запущены

$env:SERVER_PORT = "8080"
$env:DATABASE_URL = "postgres://zorion:zorion123@127.0.0.1:5432/zorion?sslmode=disable"
$env:REDIS_URL = "redis://localhost:6379/0"
$env:TICK_INTERVAL_SEC = "3"
$env:ADMIN_PASSWORD = "admin123"
$env:JWT_SECRET = "dev-secret-change-me-0123456789abcdef0123456789abcdef"

$env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH", "User")

Write-Host "🚀 Запуск Zorion..." -ForegroundColor Green
Write-Host "PostgreSQL: $env:DATABASE_URL" -ForegroundColor Yellow
Write-Host "Redis: $env:REDIS_URL" -ForegroundColor Yellow
Write-Host "Порт: $env:SERVER_PORT" -ForegroundColor Yellow

go run ./cmd/server
