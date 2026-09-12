package handlers

import (
	"database/sql"
	"sync"

	"zorion/internal/mapcache"
	"zorion/internal/probe"
	"zorion/internal/repository"
)

type AdminHandlers struct {
	worldRepo *repository.WorldRepository
	db        *sql.DB
	mapCache  *mapcache.Manager
	probeRunner *probe.Runner

	planetStatsMu sync.RWMutex
	planetStats   *PlanetStats
}

func NewAdminHandlers(worldRepo *repository.WorldRepository, db *sql.DB, mapCache *mapcache.Manager, probeRunner *probe.Runner) *AdminHandlers {
	return &AdminHandlers{
		worldRepo:   worldRepo,
		db:          db,
		mapCache:    mapCache,
		probeRunner: probeRunner,
	}
}