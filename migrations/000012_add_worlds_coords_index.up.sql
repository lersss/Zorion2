-- migrations/000012_add_worlds_coords_index.up.sql

-- Индекс под фильтр карты по видимой области:
--   WHERE coord_x BETWEEN $1 AND $2 AND coord_y BETWEEN $3 AND $4
-- См. internal/handlers/filter_worlds_handler.go, FilterWorlds.
-- Без CONCURRENTLY: таблица мелкая (~9.5 МБ), а CONCURRENTLY несовместим
-- с транзакцией, в которую migrations.go оборачивает каждую миграцию.
CREATE INDEX IF NOT EXISTS idx_worlds_coords ON worlds(coord_x, coord_y);
