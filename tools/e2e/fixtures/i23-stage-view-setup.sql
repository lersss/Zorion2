-- i23-stage-view-setup.sql — живая фикстура приёмки витрины поселения И2.3
-- (спека 2026-09-23-стадии-поселения-и-скорость-производства §8.2/§11.3).
--
-- ЧТО СОЗДАЁТ (идемпотентно):
--   * поселение (фиксированный id) на ПЕРВОЙ планете БЕЗ поселений, тип
--     «Аутпост» (id=148, params.stage={"enter":0}, нормы eat/effects), население
--     500 (< 1000 — остаётся на низшей ступени, ленивый проход не переводит);
--   * ветку (фиксированный id) на рецепт 69 (число пары 148×69 = 10000
--     ед/сутки/млрд) с большим входным буфером (производит) и пустым выходным.
--
-- ВЫВОД: строки I23_PLANET / I23_WORLD — id планеты/мира для e2e.
--
-- КАК ЗАПУСКАТЬ (dev-БД):
--   powershell -File tools/db.ps1 -File tools/e2e/fixtures/i23-stage-view-setup.sql
--   уборка: …-resolve.sql
-- ВНИМАНИЕ: не запускать поверх чужого живого джоба генерации (AGENTS.md §4.16).

\set ON_ERROR_STOP on
BEGIN;

-- Планета без поселений (ленивый owner-проход тронет только фикстуру).
SELECT p.id AS planet_id, p.world_id AS world_id
FROM planets p
LEFT JOIN settlements s ON s.planet_id = p.id
WHERE s.id IS NULL
ORDER BY p.created_at ASC NULLS FIRST, p.id ASC
LIMIT 1
\gset

INSERT INTO settlements (id, planet_id, population, population_exact, computed_at, created_at, settlement_type_id)
VALUES ('e0000000-0000-4000-8000-0000000000a1', :'planet_id', 500, 500,
        NOW(), NOW() - interval '2 days',
        (SELECT id FROM producer_types WHERE id = 148))
ON CONFLICT (id) DO UPDATE SET
    planet_id          = EXCLUDED.planet_id,
    population         = EXCLUDED.population,
    population_exact   = EXCLUDED.population_exact,
    computed_at        = EXCLUDED.computed_at,
    settlement_type_id = EXCLUDED.settlement_type_id,
    updated_at         = NOW();

-- Ветка на рецепт 69 (в наборе стадии 148, число пары 10000).
INSERT INTO settlement_branches (id, settlement_id, recipe_id, processed_at)
VALUES ('e0000000-0000-4000-8000-0000000000a2',
        'e0000000-0000-4000-8000-0000000000a1', 69, NOW())
ON CONFLICT DO NOTHING;

-- Вход (Мясо 359) — с запасом, ветка производит; выход (Пища 378) — пуст.
-- Ячейки хранилища поселения (буферы ветки ретайрены, 000087).
INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount, cap_share)
VALUES ('settlement', 'e0000000-0000-4000-8000-0000000000a1', 359, 1000000, 0)
ON CONFLICT (owner_type, owner_id, good_id) DO UPDATE SET amount = EXCLUDED.amount;

INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount, cap_share)
VALUES ('settlement', 'e0000000-0000-4000-8000-0000000000a1', 378, 0, 0)
ON CONFLICT (owner_type, owner_id, good_id) DO NOTHING;

COMMIT;

\echo 'I23_PLANET=' :'planet_id'
\echo 'I23_WORLD=' :'world_id'
