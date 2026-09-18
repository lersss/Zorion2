-- 97a: активный полёт игрока переживает рестарт сервера (персистентность).
-- Одна запись на игрока (PK user_id); редирект 61a заменяет сегмент
-- (INSERT ON CONFLICT DO UPDATE в репозитории). Строка удаляется при
-- прибытии (после onArrival), отмене и обработке Restore.
CREATE TABLE IF NOT EXISTS player_flights (
    user_id       UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    from_world_id UUID NOT NULL,
    to_world_id   UUID NOT NULL,
    start_x       DOUBLE PRECISION NOT NULL,  -- стартовая точка сегмента (61a, точка P)
    start_y       DOUBLE PRECISION NOT NULL,
    start_time    TIMESTAMPTZ NOT NULL,
    arrive_at     TIMESTAMPTZ NOT NULL        -- абсолютное время прибытия
);