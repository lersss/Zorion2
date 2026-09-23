-- 000073_planet_knowledge_cascade.sql — B25: знание о планете чистится каскадом
-- при удалении планеты (как settlement/building/deposit/faction).
--
-- player_planet_knowledge.planet_id создан в 000040 без ON DELETE CASCADE,
-- в отличие от прочих «детей» планет. Из-за этого падали пути удаления планет:
--   GeneratePlanets   (clearPlanets, admin_universe.go)            DELETE FROM planets;
--   RegeneratePlanets (clearPlanetsOf, admin_regenerate_planets.go) DELETE ... WHERE world_id = ANY($1);
--   DeleteWorld       (admin_worlds.go) — каскад worlds → planets упирается в знание.
-- Знание об удаляемой планете бессмысленно (planet_id новый при перегенерации) —
-- чистим каскадом. DROP IF EXISTS + ADD повторяемы: файл идемпотентен.
--
-- Номер 000073 забронирован менеджером (000072 — за спекой
-- 2026-09-23-орбита-планеты-присутствие-и-снимок).

ALTER TABLE player_planet_knowledge DROP CONSTRAINT IF EXISTS player_planet_knowledge_planet_id_fkey;
ALTER TABLE player_planet_knowledge ADD CONSTRAINT player_planet_knowledge_planet_id_fkey
    FOREIGN KEY (planet_id) REFERENCES planets(id) ON DELETE CASCADE;
