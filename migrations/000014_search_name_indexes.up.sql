-- Индексы для точного поиска объектов по имени (без учёта регистра).
CREATE INDEX IF NOT EXISTS idx_worlds_name_lower ON worlds (LOWER(name));
CREATE INDEX IF NOT EXISTS idx_planets_name_lower ON planets (LOWER(name));