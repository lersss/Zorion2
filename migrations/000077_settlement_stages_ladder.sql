-- 000077_settlement_stages_ladder.sql — ладдера ступеней поселения: 7
-- подтипов-ступеней класса «Поселение» (спека
-- 2026-09-23-стадии-поселения-и-скорость-производства §4.3): Аутпост → Посёлок
-- → Городок → Город → Мегаполис → Метрополия → Экуменополис.
--
-- Пороги — в людях (особях), params.stage = {enter, exit} (exit = 75 % от enter,
-- зазор гистерезиса exit < enter). Базовая ступень «Аутпост» — пол ладдеры:
-- её exit не читается, поэтому не задаётся (только enter = 0).
--
-- Базовая ступень = переименование существующего подтипа (на dev-БД id = 148,
-- name_norm = 'деревня'): id и все ссылки (settlements.settlement_type_id,
-- generation_config.default_settlement_type_id) сохраняются — связь по id
-- (решение создателя 2026-09-23, миграция 000075).
--
-- ⚠️ name_norm сравнивается ЛИТЕРАЛАМИ нижнего регистра, НЕ через lower():
-- коллация БД zorion = C, PostgreSQL lower() не конвертирует кириллицу
-- (docs/PITFALLS.md «БД и шелл», 000051). name_norm пишет Go
-- (graph.NormalizeName = strings.ToLower).
--
-- Идемпотентность и свежая БД: миграции применяются ДО Go-сида каталога
-- (cmd/server/main.go: migrations.Apply → goodsstudio.SeedProducers), поэтому на
-- свежей БД типа «Поселение» ещё нет. Вставка новых ступеней защищена CROSS JOIN
-- с подзапросом по типу-родителю: нет родителя → ноль строк (записи создаст сид).
-- Повторный прогон: UPDATE'ы перезаписывают то же значение, INSERT'ы — ON CONFLICT
-- DO NOTHING. Признак params.eat_units (000076) порядок не ломает: он отсекает
-- повторную конверсию норм, а не задаёт их.
--
-- Номер 000077 забронирован менеджером (docs/COORDINATION.md); последний в
-- рабочем дереве — 000076 (проверено, PITFALLS «БД и шелл»).

-- 1) Базовая ступень: «Деревня» / «Обычное поселение» → «Аутпост».
UPDATE producer_types SET name = 'Аутпост', name_norm = 'аутпост'
WHERE name_norm IN ('деревня', 'обычное поселение');

-- 2) Пороги существующих подтипов (слияние `||` — прочие ключи params
--    сохраняются). «Аутпост» — пол: exit не задаётся.
UPDATE producer_types
SET params = COALESCE(params, '{}'::jsonb) || '{"stage": {"enter": 0}}'::jsonb
WHERE name_norm = 'аутпост';

UPDATE producer_types
SET params = COALESCE(params, '{}'::jsonb) || '{"stage": {"enter": 100000, "exit": 75000}}'::jsonb
WHERE name_norm = 'город';

UPDATE producer_types
SET params = COALESCE(params, '{}'::jsonb) || '{"stage": {"enter": 1000000, "exit": 750000}}'::jsonb
WHERE name_norm = 'мегаполис';

-- 3) Новые ступени: Посёлок / Городок / Метрополия / Экуменополис. Вставка
--    только при существующем типе-родителе «Поселение» (свежая БД — пропуск);
--    ON CONFLICT (name_norm) DO NOTHING — повторный прогон и ручные правки
--    в студии не затираются.
INSERT INTO producer_types (name, name_norm, kind, parent_id, output, input, params)
SELECT v.name, v.name_norm, 'goods', root.id, '{}'::jsonb, '{}'::jsonb, v.params
FROM (VALUES
    ('Посёлок',      'посёлок',      '{"stage": {"enter": 1000, "exit": 750}}'::jsonb),
    ('Городок',      'городок',      '{"stage": {"enter": 10000, "exit": 7500}}'::jsonb),
    ('Метрополия',  'метрополия',  '{"stage": {"enter": 10000000, "exit": 7500000}}'::jsonb),
    ('Экуменополис', 'экуменополис', '{"stage": {"enter": 100000000, "exit": 75000000}}'::jsonb)
) AS v(name, name_norm, params)
CROSS JOIN (SELECT id FROM producer_types WHERE name_norm = 'поселение') AS root
ON CONFLICT (name_norm) DO NOTHING;
