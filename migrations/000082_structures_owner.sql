-- 000082_structures_owner.sql
-- Постройка структур на планете из игрового интерфейса (спека
-- 2026-09-24-постройка-структур-на-планете.md §3, ред. 3): владелец у поселений
-- (settlements.owner_type/owner_id) и связь строений с деревом типов
-- (buildings.producer_type_id).
--
-- Бэкфилла владельцев НЕТ (решение создателя Г1): существующие поселения
-- остаются без владельца (owner_id IS NULL — легален и штатен); владелец
-- появляется только у новых (постройка инструментом + генерация галактики,
-- проход faction.EnsureSettlementOwners).

-- ==================== settlements: владелец ====================
-- Классы владельца — те же, что у buildings.owner_type (player/faction/agent):
-- полиморфный владелец читается одинаково у обеих таблиц. owner_id — без FK
-- (полиморфный), как у buildings.
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS owner_type TEXT NULL;
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS owner_id UUID NULL;

-- Идемпотентность CHECK: ALTER TABLE ... ADD CONSTRAINT IF NOT EXISTS в PG НЕ
-- существует — проверяем pg_constraint. Проверка скоупится по conrelid:
-- pg_constraint — БД-глобальная, а имя констрейнта уникально лишь в пределах
-- таблицы; без `conrelid = 'settlements'::regclass` чужой одноимённый
-- констрейнт (другая схема, напр. integration-тесты) маскировал бы создание.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint
                 WHERE conname = 'settlements_owner_type_chk'
                   AND conrelid = 'settlements'::regclass) THEN
    ALTER TABLE settlements ADD CONSTRAINT settlements_owner_type_chk
      CHECK (owner_type IS NULL OR owner_type IN ('player','faction','agent'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint
                 WHERE conname = 'settlements_owner_pair_chk'
                   AND conrelid = 'settlements'::regclass) THEN
    ALTER TABLE settlements ADD CONSTRAINT settlements_owner_pair_chk
      CHECK ((owner_type IS NULL) = (owner_id IS NULL));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_settlements_owner
    ON settlements (owner_type, owner_id) WHERE owner_id IS NOT NULL;

-- ==================== buildings: связь с деревом типов ====================
-- RESTRICT — как у settlements.settlement_type_id (миграция 000067): тип в
-- использовании удалить нельзя (студия отдаёт 409). Столицы
-- (building_type='capital') сохраняют producer_type_id = NULL.
ALTER TABLE buildings ADD COLUMN IF NOT EXISTS producer_type_id BIGINT NULL
    REFERENCES producer_types (id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS idx_buildings_producer_type_id
    ON buildings (producer_type_id) WHERE producer_type_id IS NOT NULL;
