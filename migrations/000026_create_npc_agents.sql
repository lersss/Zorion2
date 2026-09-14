-- NPC-агенты (спека 20a.1 §2.1): наблюдатель-агенты, автономно перемещаются
-- между мирами, по прибытии помечают мир посещённым (last_observed_at),
-- пересчёт населения не запускают (спека §4). Единственный писатель
-- состояния — NPCManager (спека §9 И1). Масштаб: сотни тысяч записей.
-- from/target/depart/arrive заполнены только во время полёта (flying);
-- current_world_id — всегда: idle = мир, где агент стоит, flying = откуда летит.
CREATE TABLE IF NOT EXISTS npc_agents (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text NOT NULL,
    status           text NOT NULL DEFAULT 'idle'
                     CHECK (status IN ('idle', 'flying', 'observing')),
    current_world_id uuid NOT NULL REFERENCES worlds(id),
    from_world_id    uuid REFERENCES worlds(id),
    target_world_id  uuid REFERENCES worlds(id),
    depart_at        timestamptz,
    arrive_at        timestamptz,  -- абсолютно — полёт переживает рестарт сервера (§3.1)
    notify_enabled   boolean NOT NULL DEFAULT true,
    last_observed_at timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
-- Индекс по status (спека §2.1); вторая колонка id — покрытие курсорной
-- выборки `WHERE status = $1 AND id > $2 ORDER BY id` (§2.2.A, batch 2000/тик).
CREATE INDEX IF NOT EXISTS idx_npc_agents_status ON npc_agents (status, id);