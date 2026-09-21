-- 000056_create_buildings.sql
-- Строения (первая итерация релиза 2 фабрик, спека
-- 2026-09-21-фабрики-релиз-2-столицы-фракций §2): сущность «строение» +
-- владелец. Первый экземпляр — столица фракции (building_type='capital',
-- owner_type='faction', owner_id=factions.id) на её родной планете.
-- Колонки producer_type_id/population/slots/status/data/name — отложены до
-- следующих итераций (§2.2), колонок без потребителя не заводим.
CREATE TABLE IF NOT EXISTS buildings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    planet_id UUID NOT NULL REFERENCES planets(id) ON DELETE CASCADE,
    building_type TEXT NOT NULL,
    owner_type TEXT NOT NULL CHECK (owner_type IN ('player', 'faction', 'agent')),
    -- Полиморфный владелец (users/factions/npc_agents) — FK нет намеренно (§2.3).
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Чтение карточки планеты: выборка по планетам одной системы (инвариант 2).
CREATE INDEX IF NOT EXISTS idx_buildings_planet_id ON buildings (planet_id);

-- Одна столица на фракцию: гарантия на уровне БД вместо соглашения об именах
-- и страховка от гонки двух джоб (INSERT ... ON CONFLICT DO NOTHING).
CREATE UNIQUE INDEX IF NOT EXISTS uq_buildings_capital_owner
    ON buildings (owner_type, owner_id) WHERE building_type = 'capital';
