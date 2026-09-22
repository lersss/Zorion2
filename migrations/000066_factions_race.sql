-- 000066_factions_race.sql (идея 2026-09-22 «Фракции: одна на расу со своей
-- столицей»): factions узнаёт расу. Одна фракция на расу — частичный UNIQUE
-- по race_id; NULL разрешён (легаси-фракции, раса неизвестна).
ALTER TABLE factions ADD COLUMN IF NOT EXISTS race_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_factions_race
    ON factions(race_id) WHERE race_id IS NOT NULL;
