-- 91a: двигатель — настоящий модуль (спека 91a §7.1/§7.4): engine_1 в каталоге
-- оборудования + бэкфилл существующих игроков (слот engine пуст/отсутствует).
-- Тип 'engine' уже заложен CHECK-ограничением 77a §3.2 (миграция 000040).

-- 1. Каталог: двигатель-1 (скорость полёта 0.3 сек/px — константа 66a,
--    становится характеристикой двигателя; замысел 77a §1.3, первый шаг).
INSERT INTO equipment (id, type, name, params)
VALUES ('engine_1', 'engine', 'Двигатель-1', '{"speed_factor": 0.3}');

-- 2. Бэкфилл существующих игроков: проставить engine_1 в users.equipment,
--    где слот engine пуст/отсутствует (по образцу бэкфилла 000040).
--    users.equipment JSONB: {"radar":"radar_1","scanner":"scanner_1","engine":null}
--    → {"radar":"radar_1","scanner":"scanner_1","engine":"engine_1"}
UPDATE users
SET equipment = jsonb_set(COALESCE(equipment, '{}'::jsonb), '{engine}', '"engine_1"')
WHERE equipment IS NULL
   OR NOT (equipment ? 'engine')
   OR equipment->>'engine' IS NULL
   OR equipment->>'engine' = '';