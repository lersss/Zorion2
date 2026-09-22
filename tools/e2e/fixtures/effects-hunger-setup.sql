-- effects-hunger-setup.sql — живая фикстура приёмки «голод работает» (этап 4).
--
-- ЧТО СОЗДАЁТ (идемпотентно; повторный прогон не дублирует):
--   * категорию-позицию «продовольствие» (если её нет — сид товаров уже мог создать);
--   * товар-выход «QA ГолодТовар» и сырьё «QA ГолодСырьё» в этой категории;
--   * рецепт complexity=1 с одним компонентом (сырьё);
--   * тестовое поселение (фиксированный id) на ПЕРВОЙ планете БД, тип «Обычное
--     поселение» (несёт params.effects/params.eat из миграции 000070);
--   * ветку этого поселения с пустым входом (производство 0 → coverage=0 → w=1 —
--     ровно приёмка «выход обнулён → w≈1, нагрузка растёт»);
--   * строку нагрузки выше порога (load=30 > 24) — на первом же живом проходе
--     видно «включён, сила > 0» и убыль населения.
--
-- КАК ЗАПУСКАТЬ (dev-БД):
--   powershell -File tools/db.ps1 -File tools/e2e/fixtures/effects-hunger-setup.sql
--   уборка: powershell -File tools/db.ps1 -File tools/e2e/fixtures/effects-hunger-resolve.sql
--
-- ВЫВОД: строки EFFECTS_HUNGER_PLANET / EFFECTS_HUNGER_WORLD — id планеты/мира
-- для e2e (`QA_PLANET`/`QA_WORLD` в tools/e2e/qa-effects-hunger.js).
--
-- ПРЕДУСЛОВИЯ: в БД есть ≥1 планета; применены миграции 000067 (тип «обычное
-- поселение») и 000070 (каталог эффектов «голод»).
-- ВНИМАНИЕ: не запускать поверх чужого живого джоба генерации (AGENTS.md §4.16).
--
-- Чтобы увидеть обратный случай «покрытие → w=0», доложите во вход ветки сырьё:
--   UPDATE settlement_branch_buffers SET amount = 1e6
--    WHERE branch_id = 'e0000000-0000-4000-8000-000000000002' AND direction = 'input';

\set ON_ERROR_STOP on
BEGIN;

-- Категория-позиция «продовольствие» (любой kind; здесь kind='good').
INSERT INTO categories (name, name_norm, kind)
VALUES ('Продовольствие', 'продовольствие', 'good')
ON CONFLICT (kind, name_norm) DO NOTHING;

-- Товары фикстуры: выход (закрывает позицию) и сырьё (компонент рецепта).
INSERT INTO goods (name, name_norm, category_id, kind)
SELECT 'QA ГолодТовар', 'qa голодтовар', c.id, 'good'
FROM categories c WHERE c.kind = 'good' AND c.name_norm = 'продовольствие'
ON CONFLICT (name_norm) DO NOTHING;

INSERT INTO goods (name, name_norm, category_id, kind)
SELECT 'QA ГолодСырьё', 'qa голодсырьё', c.id, 'good'
FROM categories c WHERE c.kind = 'good' AND c.name_norm = 'продовольствие'
ON CONFLICT (name_norm) DO NOTHING;

-- Рецепт complexity=1 + компонент (вход); вход ветки пуст → производство 0.
INSERT INTO recipes (good_id, complexity)
SELECT g.id, 1 FROM goods g WHERE g.name_norm = 'qa голодтовар'
ON CONFLICT (good_id) DO NOTHING;

INSERT INTO recipe_components (recipe_id, pos, component_id, quantity)
SELECT r.id, 0, c.id, 1
FROM recipes r
JOIN goods og ON og.id = r.good_id AND og.name_norm = 'qa голодтовар'
JOIN goods c ON c.name_norm = 'qa голодсырьё'
ON CONFLICT (recipe_id, pos) DO NOTHING;

-- Тестовое поселение на первой планете БД (фиксированный id — уборка по нему).
SELECT p.id AS planet_id, p.world_id AS world_id
FROM planets p
ORDER BY p.created_at ASC NULLS FIRST, p.id ASC
LIMIT 1
\gset

INSERT INTO settlements (id, planet_id, population, population_exact, computed_at, created_at, settlement_type_id)
VALUES ('e0000000-0000-4000-8000-000000000001', :'planet_id', 100000, 100000,
        NOW() - interval '2 hours', NOW() - interval '2 hours',
        (SELECT id FROM producer_types WHERE name_norm = 'обычное поселение'))
ON CONFLICT (id) DO UPDATE SET
    population          = EXCLUDED.population,
    population_exact    = EXCLUDED.population_exact,
    computed_at         = EXCLUDED.computed_at,
    settlement_type_id  = EXCLUDED.settlement_type_id,
    updated_at          = NOW();

-- Ветка (фиксированный id) + буферы: вход 0 (производство 0), выход 0.
INSERT INTO settlement_branches (id, settlement_id, recipe_id, processed_at)
SELECT 'e0000000-0000-4000-8000-000000000002',
       'e0000000-0000-4000-8000-000000000001',
       r.id, NOW() - interval '2 hours'
FROM recipes r
JOIN goods og ON og.id = r.good_id AND og.name_norm = 'qa голодтовар'
ON CONFLICT DO NOTHING;

INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
SELECT 'e0000000-0000-4000-8000-000000000002', 'input', g.id, 0
FROM goods g WHERE g.name_norm = 'qa голодсырьё'
ON CONFLICT DO NOTHING;

INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
SELECT 'e0000000-0000-4000-8000-000000000002', 'output', g.id, 0
FROM goods g WHERE g.name_norm = 'qa голодтовар'
ON CONFLICT DO NOTHING;

-- Нагрузка выше порога (24): витрина «включён, сила > 0»; живой проход накопит ещё.
INSERT INTO active_effects (effect_type_id, owner_type, owner_id, owner_settlement_id, source_position, load, load_at)
SELECT et.id, 'settlement', 'e0000000-0000-4000-8000-000000000001',
       'e0000000-0000-4000-8000-000000000001', 'продовольствие',
       30, NOW() - interval '2 hours'
FROM effect_types et WHERE et.name_norm = 'голод'
ON CONFLICT (owner_type, owner_id, effect_type_id) DO UPDATE SET
    load = EXCLUDED.load, load_at = EXCLUDED.load_at, updated_at = NOW();

COMMIT;

\echo 'EFFECTS_HUNGER_PLANET=' :'planet_id'
\echo 'EFFECTS_HUNGER_WORLD=' :'world_id'
