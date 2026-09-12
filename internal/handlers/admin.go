package handlers

import (
	"database/sql"
	"sync"

	"zorion/internal/mapcache"
	"zorion/internal/repository"
)

type AdminHandlers struct {
	worldRepo *repository.WorldRepository
	db        *sql.DB
	mapCache  *mapcache.Manager

	planetStatsMu sync.RWMutex
	planetStats   *PlanetStats
}

func NewAdminHandlers(worldRepo *repository.WorldRepository, db *sql.DB, mapCache *mapcache.Manager) *AdminHandlers {
	return &AdminHandlers{
		worldRepo: worldRepo,
		db:        db,
		mapCache:  mapCache,
	}
}