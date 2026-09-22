-- 000071_contract_board.sql
-- Контракт как состояние: чек-точка ленивой доски + «пакет контрактов» —
-- фундамент схемы (спека 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение
-- §3.1/§4.2, Поставка 1 / чекпоинт 1). Идёт после 000062_contracts.sql
-- (колонки и индексы — на её таблице contracts).
-- Номер подтверждён менеджером: максимум в дереве — 000069; 000070 зарезервирован
-- спекой 2026-09-22-эффекты-снабжения-задержка-голод.
-- Числа не задаются (числа — пресеты владельца/константы кода, §8).

-- §3.1. Чек-точка доски планеты — своя ленивая петля (не settlements.computed_at
-- и не settlement_branches.processed_at). Строка спарсена: появляется только у
-- планеты с нуждами, правило — в коде материализации.
CREATE TABLE IF NOT EXISTS contract_board_state (
    planet_id       UUID PRIMARY KEY REFERENCES planets (id) ON DELETE CASCADE,
    materialized_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- §4.2. Пакет — поле-группа: package_key NULL = контракт вне пакета (перелёты,
-- ручные публикации); share_index — позиция доли в пакете (1..N). Объём и предмет
-- доли живут в contract_requirements (отдельной колонки объёма нет).
ALTER TABLE contracts ADD COLUMN IF NOT EXISTS package_key TEXT NULL;
ALTER TABLE contracts ADD COLUMN IF NOT EXISTS share_index INTEGER NULL;

-- Идемпотентность материализации: одна открытая доля на индекс пакета.
CREATE UNIQUE INDEX IF NOT EXISTS uq_contracts_package_open_share
    ON contracts (package_key, share_index)
    WHERE package_key IS NOT NULL AND status = 'open';

-- «Один игрок — одна взятая доля пакета» (мягкий запрет монополии).
CREATE UNIQUE INDEX IF NOT EXISTS uq_contracts_package_taken_executor
    ON contracts (package_key, executor_id)
    WHERE package_key IS NOT NULL AND status = 'taken';

-- Поиск открытых долей пакета при сверке.
CREATE INDEX IF NOT EXISTS idx_contracts_package_open
    ON contracts (package_key) WHERE package_key IS NOT NULL AND status = 'open';
