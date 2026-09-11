-- migrations/000013_add_planet_filter_indexes.up.sql

-- Индексы под EXISTS-подзапросы фильтра карты (B8).
-- См. internal/handlers/filter_worlds_handler.go, строки 118-131.
--
-- Без них каждый фильтр делал до 100 000 проб в planets (761 МБ при
-- shared_buffers 128 МБ) и читал ~100k страниц с диска: 1,0-2,2 с на запрос.
-- Предикаты частичных индексов дословно повторяют условия в хендлере —
-- иначе планировщик их не применит.

-- has_life: 46 289 планет из 319 573.
CREATE INDEX IF NOT EXISTS idx_planets_life_world
    ON planets (world_id)
    WHERE ((data ->> 'life')::boolean = true);

-- has_habitable.
CREATE INDEX IF NOT EXISTS idx_planets_habitable_world
    ON planets (world_id)
    WHERE ((data ->> 'habitable')::boolean = true);

-- planet_type: значение параметризовано, поэтому индекс полный,
-- по тому же выражению LOWER(data->>'type'), что и в хендлере.
CREATE INDEX IF NOT EXISTS idx_planets_type_world
    ON planets (LOWER(data ->> 'type'), world_id);
