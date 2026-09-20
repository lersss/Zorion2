-- Переименование товарных категорий (решение создателя 2026-09-21):
--   «комплектующие» → «электроника», «детали» → «механика».
-- Смысл категорий сохранён, меняется имя: электроника — проводящая начинка
-- (проводимость ↑, T_melt ↑, химактивность ↓), механика — несущее/подвижное
-- (твёрдость, плотность, T_melt). Состав 13 категорий не меняется.
--
-- Data-миграция: сид goods_catalog_seed на существующих БД уже отработал
-- (маркер) — категории и товары правятся здесь, свежие БД получают те же
-- имена из model.DefaultCategories (internal/goodsstudio/model/state.go).
-- Оба пути дают одинаковый результат.
--
-- UPDATE — по name_norm (устойчиво к переименованиям в студии: запись не
-- найдена → пропуск, не падать). name_norm сравнивается ЛИТЕРАЛАМИ нижнего
-- регистра, НЕ через lower(): коллация БД zorion = C, PostgreSQL lower() не
-- конвертирует кириллицу (docs/PITFALLS.md «БД и шелл»). name_norm пишет Go
-- (graph.NormalizeName = strings.ToLower, Unicode) — литералы совпадают с ним.
--
-- ⚠️ Порядок относительно сида: миграции применяются ДО goodsstudio.Seed
-- (cmd/server/main.go). На свежей БД категорий ещё нет — три UPDATE ниже
-- работают вхолостую, а сид создаёт уже новые имена. На существующей БД
-- категории есть, UPDATE переименовывает их, и маркерный сид пропускается.

-- 1) Переименовать категории (только kind='good'; ресурсные не трогаются).
UPDATE categories SET name = 'Электроника', name_norm = 'электроника'
WHERE name_norm = 'комплектующие' AND kind = 'good';
UPDATE categories SET name = 'Механика', name_norm = 'механика'
WHERE name_norm = 'детали' AND kind = 'good';

-- 2) Товары этих категорий переехали (category_id по kind, FK составной;
--    уникальность (kind, name_norm) не затрагивается — переименование id).
UPDATE goods SET category_id = (SELECT id FROM categories WHERE name_norm = 'электроника' AND kind = 'good')
WHERE category_id = (SELECT id FROM categories WHERE name_norm = 'комплектующие' AND kind = 'good');
UPDATE goods SET category_id = (SELECT id FROM categories WHERE name_norm = 'механика' AND kind = 'good')
WHERE category_id = (SELECT id FROM categories WHERE name_norm = 'детали' AND kind = 'good');

-- 3) Корзина роботов Автофабрики: input.consumables — строки имён категорий
--    (сид producer_catalog_seed уже отработал на существующих БД).
--    Свежие БД получают новые имена из seed_producers.go.
UPDATE producer_types
SET input = jsonb_set(
        input,
        '{consumables}',
        (SELECT jsonb_agg(
                    CASE elem
                        WHEN '"детали"'::jsonb THEN '"механика"'::jsonb
                        WHEN '"комплектующие"'::jsonb THEN '"электроника"'::jsonb
                        ELSE elem
                    END
                    ORDER BY ordinality)
         FROM jsonb_array_elements(input -> 'consumables') WITH ORDINALITY AS t(elem, ordinality))
    )
WHERE name_norm = 'автофабрика'
  AND input ? 'consumables';
