-- 99.2.27: внутрисистемная позиция игрока + внутрисистемные полёты.
-- 1. Внутрисистемная позиция игрока (JSONB, NULL = вне системы;
--    status: orbit | in_flight — решение создателя 2026-09-20)
ALTER TABLE users ADD COLUMN current_position JSONB;

-- 2. Внутрисистемные полёты (паттерн 97a: абсолютный arrive_at, Restore)
CREATE TABLE player_intrasystem_flights (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    world_id   UUID NOT NULL,
    from_type  TEXT NOT NULL CHECK (from_type IN ('star','planet','satellite')),
    from_id    TEXT NOT NULL,      -- UUID или синтетический id компаньона (спека 99.2.27 §3.1)
    to_type    TEXT NOT NULL CHECK (to_type   IN ('star','planet','satellite')),
    to_id      TEXT NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    arrive_at  TIMESTAMPTZ NOT NULL
);