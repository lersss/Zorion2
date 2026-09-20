package handlers

import (
	"database/sql"
	"sync"

	"zorion/internal/mapcache"
	"zorion/internal/npc"
	"zorion/internal/repository"
	"zorion/internal/travel"
)

type AdminHandlers struct {
	worldRepo *repository.WorldRepository
	db        *sql.DB
	mapCache  *mapcache.Manager

	// npcManager — инвалидация кэша агентов при TRUNCATE npc_agents
	// (ClearUniverse/GenerateUniverse, идея 26c A2). nil в тестах, где
	// менеджер не подключён.
	npcManager *npc.Manager

	// visibility — серверная видимость игрока (спека 77a §11). nil в тестах
	// и для admin/skycomposer (видят всё, И7); для player применяется фильтр
	// по кругу радара.
	visibility *Visibility

	// travelManager — активные полёты игроков (спека 77a §5.3): позиции
	// чужих игроков в полёте интерполируются. nil в тестах.
	travelManager *travel.Manager

	// pacmanNotifier — WS-рассылка событий пакмана (спека 2026-09-20 §5):
	// джоб пишет неблокирующе, рассылку делает горутина нотификатора.
	// nil в тестах, где нотификатор не подключён.
	pacmanNotifier *PacmanNotifier

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

// SetVisibility — подключает серверную видимость игрока (спека 77a §11).
// Сеттер (не параметр конструктора): Visibility строится из репозиториев,
// которые создаются позже adminHandlers в main.go.
func (h *AdminHandlers) SetVisibility(v *Visibility) {
	h.visibility = v
}

// SetTravelManager — подключает менеджер полётов (спека 77a §5.3): позиции
// чужих игроков в полёте интерполируются. Сеттер (не параметр конструктора).
func (h *AdminHandlers) SetTravelManager(m *travel.Manager) {
	h.travelManager = m
}

// SetPacmanNotifier — подключает нотификатор пакмана (спека 2026-09-20 §2.1):
// джоб пишет события неблокирующе, рассылку делает горутина нотификатора.
// Сеттер (не параметр конструктора): нотификатор строится из WebSocketHub,
// который создаётся позже adminHandlers в main.go.
func (h *AdminHandlers) SetPacmanNotifier(n *PacmanNotifier) {
	h.pacmanNotifier = n
}