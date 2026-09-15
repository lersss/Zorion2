// internal/npc/agent_cache.go
// In-memory кэш агентов для интерполяции позиций (идея 26c A2, возврат к
// замыслу спеки 20a.1 §2.2.B «позиции in-memory»). До замеров A2 источником
// позиций был ListAll() на КАЖДЫЙ тик — SELECT всех 100k агентов каждые 5 с
// (~250 мс из ~389 мс тика). Кэш: один ListAll при старте/инвалидации +
// инкремент стартами и прибытиями тика.
//
// Конкурентность (AGENTS.md §0, И1): map читается и пишется ТОЛЬКО горутиной
// тика. Хендлеры внешних мутаций (Insert/Delete/BulkInsert/DeleteAll/TRUNCATE)
// ставят только agentsDirty (atomic.Bool) через MarkDirty/OnAgentsDeleted —
// сам кэш не трогают. Перезагрузку кэша делает тик в начале следующего
// итера.
package npc

import (
	"log"

	"zorion/internal/models"
)

// reloadAgentCache — полная перезагрузка кэша из БД одним ListAll.
// Вызывается из тика при старте (agentCache == nil) и после внешних
// мутаций (agentsDirty). Ошибка — лог + dirty сохраняется: следующий тик
// повторит перезагрузку; позиции не пересчитываются (как раньше при
// ошибке ListAll: лог + пропуск).
func (m *Manager) reloadAgentCache() {
	agents, err := m.store.ListAll()
	if err != nil {
		log.Printf("❌ NPCManager: reloadAgentCache: %v", err)
		if m.agentCache == nil {
			m.agentCache = map[string]models.NPCAgent{} // пустая, не nil — инкремент тика не паникует
		}
		return // dirty остаётся — ретрай следующего тика
	}
	cache := make(map[string]models.NPCAgent, len(agents))
	for _, a := range agents {
		cache[a.ID] = a
	}
	m.agentCache = cache
	m.agentsDirty.Store(false)
}