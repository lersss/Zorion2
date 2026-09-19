-- Каталог товаров и ресурсов студии (спека перенос-студии-товаров-iterA §4):
-- единая таблица категорий (товарные + 6 системных ресурсных), товары/ресурсы
-- (kind), слоты рецептов отдельной таблицей. Каталог — контент, не данные
-- вселенной: ClearUniverse его не трогает (admin_universe.go truncateTables).
--
-- name_norm — обычная колонка (НЕ generated): значение пишет приложение
-- (graph.NormalizeName, Unicode ToLower). PostgreSQL lower() зависит от
-- коллации БД: при datcollate = C кириллица НЕ приводится к нижнему
-- регистру (lower('QA-Категория') = 'qa-Категория') — generated-колонка
-- ломала бы контракт 409 и инвариант уникальности (BUG-1, @tester iterA).

CREATE TABLE categories (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    name_norm  TEXT NOT NULL,             -- graph.NormalizeName(name) из Go
    kind       TEXT NOT NULL CHECK (kind IN ('good', 'resource')),
    code       TEXT UNIQUE,              -- только для ресурсных: water/gas/mineral/organic/fuel/rare
    is_system  BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (kind, id),                    -- для составного FK из goods
    UNIQUE (kind, name_norm)              -- дубликаты имён в каждом пространстве отдельно
);

CREATE TABLE goods (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    name_norm     TEXT NOT NULL,          -- graph.NormalizeName(name) из Go
    category_id   BIGINT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('good', 'resource')),
    status        TEXT NOT NULL DEFAULT 'draft'
                  CHECK (status IN ('draft', 'approved', 'excluded', 'banned')),
    source        TEXT NOT NULL DEFAULT 'manual'
                  CHECK (source IN ('manual', 'ai', 'palette', 'import')),
    tier_override INT NULL CHECK (tier_override IS NULL OR tier_override >= 0),
    banned_at     TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    props         JSONB NULL,             -- только kind=resource (решение гейта 2026-09-19)
    UNIQUE (name_norm),                   -- одно пространство имён: товар с именем ресурса невозможен
    FOREIGN KEY (kind, category_id) REFERENCES categories (kind, id),
    CHECK ((kind = 'good' AND props IS NULL) OR (kind = 'resource'))
);

CREATE TABLE goods_slots (
    id             BIGSERIAL PRIMARY KEY,
    good_id        BIGINT NOT NULL REFERENCES goods (id) ON DELETE CASCADE,
    pos            INT NOT NULL,          -- порядок слота в рецепте (0-based)
    component_id   BIGINT NULL REFERENCES goods (id) ON DELETE SET NULL,  -- NULL = пустой слот
    quantity       INT NOT NULL DEFAULT 1 CHECK (quantity >= 1),
    reason         TEXT NULL,             -- ИИ-обоснование (тултип)
    allow_resource BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (good_id, pos)
);
CREATE INDEX idx_goods_slots_component ON goods_slots (component_id);