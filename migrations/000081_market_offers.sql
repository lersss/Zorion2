-- 000081: магазин модулей — справочник предложений витрины (спека
-- 2026-09-24-магазин-модулей-локальный-рынок §5). Раздел «Модули»:
-- 4 базовых модуля, бесконечный запас. kind — открытый список разделов
-- (позже good/report), CHECK сужен до текущего типа.
-- Идемпотентность сида: ON CONFLICT (id) DO NOTHING (повторный прогон
-- не дублирует).

CREATE TABLE market_offers (
    id         BIGSERIAL PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind = 'module'),
    item_id    TEXT NOT NULL,
    price      BIGINT NOT NULL CHECK (price >= 0),
    params     JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO market_offers (kind, item_id, price) VALUES
    ('module', 'cargo_1',   3000),
    ('module', 'engine_1',  3000),
    ('module', 'radar_1',   3000),
    ('module', 'scanner_1', 3000)
ON CONFLICT (id) DO NOTHING;