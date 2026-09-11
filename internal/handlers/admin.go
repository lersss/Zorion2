package handlers

import (
	"database/sql"
	"sync"

	"zorion/internal/repository"
)

type AdminHandlers struct {
	worldRepo *repository.WorldRepository
	db        *sql.DB

	planetStatsMu sync.RWMutex
	planetStats   *PlanetStats
}

func NewAdminHandlers(worldRepo *repository.WorldRepository, db *sql.DB) *AdminHandlers {
	return &AdminHandlers{
		worldRepo: worldRepo,
		db:        db,
	}
}