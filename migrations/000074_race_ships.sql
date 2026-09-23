-- 000074_race_ships.sql
-- Корабли рас: раса агента и игрока (спека
-- docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §4.3, подэтап П1).
-- Номер забронирован менеджером (2026-09-23).
--
-- 1) npc_agents.race_id — раса агента (слаг из config/races.json); NULL = не
--    задана → нейтральный корабль.
-- 2) users.race_id — раса игрока; DEFAULT 'humans' заполняет существующие
--    строки (задел под выбор расы).
-- 3) users.ship_icon — смена DEFAULT на людской корабль (член реестра
--    RaceShipSprites). Страховка для путей, не задающих колонку.
-- 4) Бэкфилл агентов — случайная раса из ДАННЫХ (factions.race_id), не из
--    литерального списка. Пул пуст → race_id остаётся NULL (не падаем).
-- 5) Бэкфилл users.ship_icon — всё, что не расовый файл, → людской корабль.
-- Бэкфиллы идемпотентны (WHERE race_id IS NULL / NOT LIKE 'race\_%').
-- VACUUM внутрь миграции не кладём — не работает в транзакции (PITFALLS.md).

ALTER TABLE npc_agents ADD COLUMN IF NOT EXISTS race_id TEXT;

ALTER TABLE users ADD COLUMN IF NOT EXISTS race_id TEXT NOT NULL DEFAULT 'humans';

ALTER TABLE users ALTER COLUMN ship_icon SET DEFAULT 'race_humans_starship.png';

-- Бэкфилл агентов: рандомная раса из пула данных. Пул строится один раз
-- (array_agg DISTINCT), пустой пул → UPDATE не трогает строки.
WITH pool AS (
    SELECT array_agg(DISTINCT race_id) AS races
    FROM factions
    WHERE race_id IS NOT NULL
)
UPDATE npc_agents
SET race_id = pool.races[1 + floor(random() * array_length(pool.races, 1))::int]
FROM pool
WHERE npc_agents.race_id IS NULL
  AND pool.races IS NOT NULL
  AND array_length(pool.races, 1) > 0;

-- Бэкфилл ship_icon: легаси SVG-имена, старые PNG-имена и пустое → людской
-- корабль. Расовые файлы (race_*) остаются как есть.
UPDATE users
SET ship_icon = 'race_humans_starship.png'
WHERE ship_icon NOT LIKE 'race\_%';
