-- Пакман (спека 2026-09-20 §3.2): порционный DELETE npc_agents по
-- current/from/target_world_id. Без индексов это seq scan всей таблицы
-- (~670 мс на батч при 46к агентов, замер 2026-09-20) — критерий §9.6
-- (100к миров ≤ 90 с) не выполняется. Индексы на все три колонки OR —
-- bitmap OR трёх index scan'ов вместо полного скана.
CREATE INDEX IF NOT EXISTS idx_npc_agents_current_world ON npc_agents(current_world_id);
CREATE INDEX IF NOT EXISTS idx_npc_agents_from_world ON npc_agents(from_world_id);
CREATE INDEX IF NOT EXISTS idx_npc_agents_target_world ON npc_agents(target_world_id);