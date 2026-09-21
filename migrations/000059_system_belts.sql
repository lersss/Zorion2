-- 000059_system_belts.sql
-- Пояса малых тел — объекты системы (спека
-- 2026-09-21-пояса-малых-тел-объект-системы, этап 1б).
-- Владелец — мир (world_id; у компонента кратной свой диск ⇒ свои пояса —
-- компаньоны §7.3–7.4); записей много; типы открыты (kind — свободный ключ,
-- как buildings.building_type). Пояс — АГРЕГАТ малых тел (не именованное
-- тело: поля bodies/Плутон-класса нет). Данные вселенной — чистится вместе
-- с мирами.
CREATE TABLE IF NOT EXISTS system_belts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id     UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,              -- asteroid | kuiper | oort | debris | dust_ring (открытый ключ)
    name         TEXT NOT NULL,              -- имя пояса (для модалки, этап 2)
    orbit_index  INTEGER,                    -- якорь-номер накрытой орбиты (астероидный: g−1 из резонансного окна; якорь ≠ середина, §4.1); NULL у Койпера/Оорта
    radius_au    DOUBLE PRECISION NOT NULL,  -- физическая середина окна пояса, а.е. (не равна радиусу орбиты-якоря; §4.1)
    width_au     DOUBLE PRECISION NOT NULL,  -- протяжённость (внешний − внутренний), а.е.
    mass         DOUBLE PRECISION NOT NULL,  -- суммарная масса, M⊕ (из остатка бюджета, κ_kind, §4.2)
    body_size_km DOUBLE PRECISION NOT NULL,  -- типичный размер тела, км
    composition  JSONB NOT NULL DEFAULT '{}',-- доли породы/железа/льда (та же зона, что у планет)
    visible      BOOLEAN NOT NULL DEFAULT true, -- показ в модалке (этап 2)
    data         JSONB NOT NULL DEFAULT '{}',-- расширяемость (этап 3: data.resources — сводка ресурсов)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_belts_world_id ON system_belts(world_id);
CREATE INDEX IF NOT EXISTS idx_system_belts_world_kind ON system_belts(world_id, kind);
