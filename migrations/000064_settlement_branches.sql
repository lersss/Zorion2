-- 000064_settlement_branches.sql
-- Ветка поселения — итерация 2 эпика «Экономика поселения» (спека
-- 2026-09-22-поселение-ветка-буферы-переработка §3.2): связь «поселение ↔
-- рецепт каталога» + своя чек-точка processed_at, и буферы ветки (вход/выход)
-- записями «ресурс → количество».
--   settlement_id — FK CASCADE: удаление поселения/вселенной сносит ветки;
--   recipe_id     — FK CASCADE: удаление рецепта (или товара-выхода) сносит
--                   ветку с буферами (§8);
--   branch_id     — FK CASCADE: буферы живут только с веткой;
--   good_id       — FK CASCADE: удаление записи каталога сносит её остатки.
-- amount DOUBLE PRECISION (дробные батчи, §4.2), CHECK >= 0.
-- UNIQUE (settlement_id, recipe_id) — одна ветка на рецепт у поселения;
-- UNIQUE (branch_id, direction, good_id) — один ресурс — одна строка на буфер.
-- НЕ вводим: capacity/потолок, rate/workers, depleted/статусы, name (§3.2).
-- Каталог (goods/recipes) миграция не трогает; числа дизайн-ручек не задаются.
CREATE TABLE IF NOT EXISTS settlement_branches (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    settlement_id UUID   NOT NULL REFERENCES settlements (id) ON DELETE CASCADE,
    recipe_id     BIGINT NOT NULL REFERENCES recipes (id)     ON DELETE CASCADE,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (settlement_id, recipe_id)
);
CREATE INDEX IF NOT EXISTS idx_settlement_branches_settlement ON settlement_branches (settlement_id);
CREATE INDEX IF NOT EXISTS idx_settlement_branches_recipe     ON settlement_branches (recipe_id);

CREATE TABLE IF NOT EXISTS settlement_branch_buffers (
    id         BIGSERIAL PRIMARY KEY,
    branch_id  UUID   NOT NULL REFERENCES settlement_branches (id) ON DELETE CASCADE,
    direction  TEXT   NOT NULL CHECK (direction IN ('input', 'output')),
    good_id    BIGINT NOT NULL REFERENCES goods (id)               ON DELETE CASCADE,
    amount     DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (amount >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (branch_id, direction, good_id)
);
CREATE INDEX IF NOT EXISTS idx_settlement_branch_buffers_branch ON settlement_branch_buffers (branch_id);
CREATE INDEX IF NOT EXISTS idx_settlement_branch_buffers_good   ON settlement_branch_buffers (good_id);
