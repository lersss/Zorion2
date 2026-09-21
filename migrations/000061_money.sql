-- 000061_money.sql
-- Деньги и эскроу — фундамент (спека 2026-09-22-деньги-и-эскроу §3, §7):
-- счёт актора (player/faction/agent) + журнал движений + связь агента с
-- фракцией-владельцем. Залог/эскроу живёт на контракте (спека §2, этап B) —
-- здесь только база под него. Счёта поселений/построек нет (решение 11:
-- кошелёк поселения — лимит, не счёт).
--
-- Аддитивна, кроме ALTER npc_agents: новые таблицы; owner_faction_id — NULL
-- без бэкфилла (заполняется генератором/лениво, §6).

-- §6. Владелец-фракция NPC-агента: агент берёт бюджет у фракции. NULL —
-- фракция не назначена; ON DELETE SET NULL — фракция удалена/перегенерирована,
-- агент остаётся без владельца (FK не ломается). Обе таблицы в одном
-- truncateTables, поэтому TRUNCATE вселенной не падает.
-- (ALTER — до CREATE TABLE: сторож TestTruncateTablesCoverMigrationFK
-- приписывает REFERENCES после последнего CREATE TABLE этой таблице.)
ALTER TABLE npc_agents
    ADD COLUMN IF NOT EXISTS owner_faction_id UUID NULL
    REFERENCES factions(id) ON DELETE SET NULL;

-- §3.1. Счёт/казна/бюджет актора. PK (owner_type, owner_id) — один счёт на
-- владельца; индексов сверх PK нет (доступ всегда по ключу). balance — целое
-- (деньги не дробим). withdrawable — корзина «заработанное», подмножество
-- balance (инвариант CHECK withdrawable <= balance, §3.3).
CREATE TABLE IF NOT EXISTS accounts (
    owner_type   TEXT NOT NULL CHECK (owner_type IN ('player', 'faction', 'agent')),
    -- Полиморфный владелец (users/factions/npc_agents) — FK нет намеренно.
    owner_id     UUID NOT NULL,
    balance      BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    withdrawable BIGINT NOT NULL DEFAULT 0 CHECK (withdrawable >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (owner_type, owner_id),
    CONSTRAINT accounts_withdrawable_le_balance CHECK (withdrawable <= balance)
);

-- §3.2. Журнал движений по счёту: ось «что случилось с деньгами актора»
-- (в отличие от contract_log — жизнь контракта). kind — открытый список.
-- contract_id без FK: журнал переживает удаление контракта/вселенной.
CREATE TABLE IF NOT EXISTS money_operations (
    id            BIGSERIAL PRIMARY KEY,
    owner_type    TEXT NOT NULL,
    owner_id      UUID NOT NULL,
    delta         BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    kind          TEXT NOT NULL,
    contract_id   UUID NULL,
    occurred_at   TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- История операций игрока: выборка по владельцу, свежие сверху (§3.2).
CREATE INDEX IF NOT EXISTS idx_money_operations_owner
    ON money_operations (owner_type, owner_id, occurred_at DESC);

-- §5/§7. Бэкфилл: счёт каждому существующему игроку (PlayerBalanceSeed=10000)
-- и каждой фракции (FactionBalanceSeed=10^15, заглушка «очень большое число»,
-- решение 12). ON CONFLICT DO NOTHING — идемпотентно.
INSERT INTO accounts (owner_type, owner_id, balance, withdrawable)
SELECT 'player', id, 10000, 0 FROM users
ON CONFLICT (owner_type, owner_id) DO NOTHING;

INSERT INTO accounts (owner_type, owner_id, balance, withdrawable)
SELECT 'faction', id, 1000000000000000, 0 FROM factions
ON CONFLICT (owner_type, owner_id) DO NOTHING;
