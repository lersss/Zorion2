-- Регионы галактики: сектора вокруг кластерных центров.
-- Заполняется при генерации вселенной (GenerateUniverse), пусто на старых данных.
CREATE TABLE IF NOT EXISTS regions (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    center_x    DOUBLE PRECISION NOT NULL,
    center_y    DOUBLE PRECISION NOT NULL,
    radius      DOUBLE PRECISION NOT NULL,
    color       TEXT NOT NULL,
    world_count INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Индекс для выборки регионов, пересекающих viewport карты.
CREATE INDEX IF NOT EXISTS idx_regions_center ON regions (center_x, center_y);