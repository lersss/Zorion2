-- 000062_contracts.sql
-- Контракт как сущность + эскроу — фундамент (спеки
-- 2026-09-22-контракт-модель-сущности §4/§6, 2026-09-22-контракт-перелёт-и-доска §5):
-- состояние контракта + требования + журнал жизни. Место — планета (решение 4),
-- типы открыты (type+payload), залог живёт на контракте (спека денег §2).
-- Заменяет муляж assignments (миграции 001_init/000010/000035).
-- Идёт ПОСЛЕ 000061_money.sql (escrow_amount ссылается на счёт автора).

CREATE TABLE IF NOT EXISTS contracts (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- type — открытый список (CHECK намеренно нет, GD_PROMPT.md): валидация в коде.
    type                  TEXT NOT NULL,
    author_type           TEXT NOT NULL CHECK (author_type IN ('player', 'faction', 'building', 'agent')),
    -- Полиморфный автор (users/factions/buildings/npc_agents) — FK нет намеренно.
    author_id             UUID NOT NULL,
    -- Место публикации — планета (решение 4); система выводится через planets.world_id.
    publication_planet_id UUID NOT NULL REFERENCES planets(id) ON DELETE CASCADE,
    title                 TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    -- Нагрузка типа (открыто): travel → {from_world_id, dest_world_id, dest_planet_id}.
    payload               JSONB NOT NULL DEFAULT '{}',
    reward                BIGINT NOT NULL CHECK (reward >= 0),
    -- Признак финансирования: обычный контракт / подряд (15_monetization §15.4).
    funding               TEXT NOT NULL DEFAULT 'regular' CHECK (funding IN ('regular', 'contract_work')),
    -- Залог: escrow_amount не обнуляется при release/return (исторический факт,
    -- §4.1); escrow_withdrawable — выводимая доля, восстанавливается при возврате.
    escrow_amount         BIGINT NOT NULL DEFAULT 0 CHECK (escrow_amount >= 0),
    escrow_withdrawable   BIGINT NOT NULL DEFAULT 0 CHECK (escrow_withdrawable >= 0),
    escrow_kind           TEXT NOT NULL DEFAULT 'deposit',
    status                TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'taken', 'completed', 'cancelled', 'expired')),
    visibility            TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'direct')),
    direct_target_type    TEXT NULL CHECK (direct_target_type IN ('player', 'faction', 'building', 'agent')),
    direct_target_id      UUID NULL,
    executor_type         TEXT NULL CHECK (executor_type IN ('player', 'agent')),
    executor_id           UUID NULL,
    taken_at              TIMESTAMPTZ NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Инварианты §4.1: прямой без адресата невозможен; взятый/выполненный имеет
    -- исполнителя; живой контракт всегда с залогом; выводимая доля ≤ залога.
    CONSTRAINT contracts_direct_has_target CHECK (visibility = 'public' OR direct_target_id IS NOT NULL),
    CONSTRAINT contracts_taken_has_executor CHECK (status NOT IN ('taken', 'completed') OR executor_id IS NOT NULL),
    CONSTRAINT contracts_live_has_escrow CHECK (status NOT IN ('open', 'taken') OR escrow_amount > 0),
    CONSTRAINT contracts_escrow_withdrawable_le_amount CHECK (escrow_withdrawable <= escrow_amount)
);

-- Индексы §6.2: частичные — закрытые контракты не занимают индекс доски.
CREATE INDEX IF NOT EXISTS idx_contracts_board
    ON contracts (publication_planet_id, created_at DESC) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_contracts_expiry
    ON contracts (expires_at) WHERE status IN ('open', 'taken');
CREATE INDEX IF NOT EXISTS idx_contracts_author
    ON contracts (author_type, author_id, status);
CREATE INDEX IF NOT EXISTS idx_contracts_executor
    ON contracts (executor_type, executor_id) WHERE executor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_contracts_direct_target
    ON contracts (direct_target_type, direct_target_id) WHERE visibility = 'direct';

-- §4.2. Требования (окно свойств): дочерняя таблица, много строк на контракт.
-- kind — открытый список (axis/goods/gear); интервал = две односторонние строки.
CREATE TABLE IF NOT EXISTS contract_requirements (
    id             BIGSERIAL PRIMARY KEY,
    contract_id    UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    pos            INT NOT NULL,
    kind           TEXT NOT NULL,
    subject        TEXT NOT NULL,
    op             TEXT NOT NULL CHECK (op IN ('ge', 'le', 'eq', 'in')),
    threshold_num  DOUBLE PRECISION NULL,
    threshold_text TEXT NULL,
    quantity       BIGINT NULL,
    UNIQUE (contract_id, pos),
    CONSTRAINT contract_requirements_threshold
        CHECK (threshold_num IS NOT NULL OR threshold_text IS NOT NULL OR op = 'in')
);

-- §4.3. Журнал жизни контракта. FK на contracts НЕТ намеренно — лог переживает
-- удаление контракта (архив; по духу settlement_log, но без CASCADE).
CREATE TABLE IF NOT EXISTS contract_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL,
    type        TEXT NOT NULL,
    actor_type  TEXT NULL,
    actor_id    UUID NULL,
    data        JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contract_log_contract
    ON contract_log (contract_id, occurred_at DESC);

-- §9 п.4. Муляж заданий — тестовые данные, ценности нет.
DROP TABLE IF EXISTS assignments;
