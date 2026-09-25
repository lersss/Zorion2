-- 000086_settlement_storage_cells.sql — внутреннее хранилище: размер у
-- поселения и таблица ячеек (спека 2026-09-25-внутреннее-хранилище-и-рождение-
-- заказов §4.1, ЧК2а).
--
-- Ячейка — одна запись «владелец × товар»: физический запас объекта; сумма
-- порогов ячеек = размер хранилища (§1.1). Владелец полиморфный
-- (owner_type/owner_id, как buildings.owner_id): поселение сейчас, постройка/
-- флот — будущее. owner_id — без FK (полиморфный), good_id — FK CASCADE.
--
-- Подэтап 2а: только схема + слой доступа. Механика пока НЕ переключена —
-- settlement_branch_buffers остаётся рабочим источником, данные из него в
-- ячейки НЕ переносятся (иначе двойной учёт: «один факт — одно место»).
-- Перенос данных и снятие буферов — подэтап 2б; таблица буферов этой
-- миграцией не трогается.
--
-- Идемпотентно: колонка/таблица/индексы IF NOT EXISTS; повторный прогон —
-- no-op. Номер 000086 забронирован менеджером (docs/COORDINATION.md).

-- 1) Размер хранилища — свойство экземпляра (F2-A). Пишет owner-проход (2б).
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS storage_size DOUBLE PRECISION NOT NULL DEFAULT 0
    CHECK (storage_size >= 0);

-- 2) Ячейки: одна на (владелец, товар).
CREATE TABLE IF NOT EXISTS settlement_storage_cells (
    id          BIGSERIAL PRIMARY KEY,
    owner_type  TEXT   NOT NULL CHECK (owner_type IN ('settlement', 'building')),
    owner_id    UUID   NOT NULL,
    good_id     BIGINT NOT NULL REFERENCES goods (id) ON DELETE CASCADE,
    amount      DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (amount >= 0),
    cap_share   DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (cap_share >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_type, owner_id, good_id)
);

CREATE INDEX IF NOT EXISTS idx_storage_cells_owner ON settlement_storage_cells (owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_storage_cells_good  ON settlement_storage_cells (good_id);
