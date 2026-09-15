package handlers

import (
	"database/sql"
	"sync"

	"zorion/internal/mapcache"
	"zorion/internal/npc"
	"zorion/internal/repository"
)

type AdminHandlers struct {
	worldRepo *repository.WorldRepository
	db        *sql.DB
	mapCache  *mapcache.Manager

	// npcManager — инвалидация кэша агентов при TRUNCATE npc_agents
	// (ClearUniverse/GenerateUniverse, идея 26c A2). nil в тестах, где
	// менеджер не подключён.
	npcManager *npc.Manager

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

// SetNPCManager — подключает менеджер NPC-агентов для инвалидации кэша:
// ClearUniverse/GenerateUniverse TRUNCATE npc_agents (admin_universe.go),
// позиции на карте и кэш агентов должны сброситься. Сеттер (не параметр
// конструктора): менеджер создаётся позже adminHandlers в main.go.
func (h *AdminHandlers) SetNPCManager(m *npc.Manager) {
	h.npcManager = m
}