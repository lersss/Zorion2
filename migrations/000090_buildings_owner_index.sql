-- 000090_buildings_owner_index.sql
-- Индекс владельца строений под вкладку «Собственность игрока» (спека
-- 2026-09-26-собственность-игрока-в-дашборде §4.4): запрос «мои строения» идёт
-- по buildings (owner_type, owner_id).
--
-- Обычный индекс, БЕЗ частичного предиката: buildings.owner_id — NOT NULL
-- (миграция 000056), поэтому WHERE owner_id IS NOT NULL всегда истинен и
-- вырожден. Частичный индекс осмыслен только у settlements, где owner_id
-- nullable (миграция 000082) — «паритета» между индексами нет.
CREATE INDEX IF NOT EXISTS idx_buildings_owner ON buildings (owner_type, owner_id);
