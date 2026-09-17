-- Лог поселения (docs/gamedesign/ideas/16a...md, 18b_settlement_log.md
-- §«Лог поселения»): отдельная таблица записей, одно поселение — много записей
-- (решение создателя 2026-09-14: лог, не поля у поселения). Первый тип —
-- 'extinct' (Вымерло); список открыт, второй тип влезает в ту же схему.
-- occurred_at NOT NULL — записи без даты не существует (бэкфилл отменён).
-- Анти-дубль: одна запись 'extinct' на поселение — частичный уникальный индекс.
CREATE TABLE IF NOT EXISTS settlement_log (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    settlement_id uuid NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    type          text NOT NULL,          -- 'extinct' — первый тип; список открыт
    occurred_at   timestamptz NOT NULL,   -- когда произошло; запись без даты не существует
    cause         text,                   -- код причины; NULL = неприменимо (будущие типы)
    created_at    timestamptz DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_settlement_log_settlement_id ON settlement_log (settlement_id);
-- Анти-дубль: одна запись «Вымерло» на поселение (инвариант, см. 18b §«Анти-дубль и синк»).
CREATE UNIQUE INDEX IF NOT EXISTS uq_settlement_log_extinct ON settlement_log (settlement_id) WHERE type = 'extinct';
