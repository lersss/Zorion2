-- 000058_surface_deposits.sql
-- Залежи на поверхности (итерация 1 эпика «Экономика поселения: залежи +
-- производственная ветка», спека 2026-09-22-поселение-добыча-сырья-биома-
-- ленивый-буфер §3.3): залежь — инстанс ресурса каталога на планете.
--   planet_id — FK CASCADE: удаление планеты/вселенной сносит залежи;
--   good_id   — FK CASCADE (решение создателя §0 п.4: удаление ресурса в
--               студии удаляет его залежи; студия предупреждает счётчиком §3.3).
-- stratum: итерация 1 пишет только 'surface'; 'subsurface' — задел 99.2.5.
-- amount — конечный запас; CHECK >= 0 (ноль разрешён на добычу итерации 3).
-- Каталог goods миграция не трогает; числа дизайн-ручек здесь не задаются.
CREATE TABLE IF NOT EXISTS deposits (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    planet_id  UUID NOT NULL REFERENCES planets(id) ON DELETE CASCADE,
    good_id    BIGINT NOT NULL REFERENCES goods(id) ON DELETE CASCADE,
    stratum    TEXT NOT NULL DEFAULT 'surface' CHECK (stratum IN ('surface', 'subsurface')),
    wealth     DOUBLE PRECISION NOT NULL CHECK (wealth >= 0 AND wealth <= 1),
    amount     DOUBLE PRECISION NOT NULL CHECK (amount >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deposits_planet_id ON deposits (planet_id);
CREATE INDEX IF NOT EXISTS idx_deposits_good_id ON deposits (good_id);
