-- Слоты родителя: конфигурация категорий строений, расово-зависимо (спека
-- 2026-09-21-студия-скрытые-категории-строений §1.2/§1.3, вторая волна).
-- Слот = «родитель (тип kind=goods) ПРЕДЛАГАЕТ категорию на уровне расовости»;
-- hidden = «предлагает, но скрыто». Автоматика категорий по имени типа
-- удаляется — дерево показывает настроенные слоты.

-- 1) Снять флаг hidden первой волны (фича НЕ релизнута; на свежей БД колонки нет).
ALTER TABLE producer_types DROP COLUMN IF EXISTS hidden;

-- 2) Слоты родителя: конфигурация (не лог). Уникальность — UNIQUE NULLS
--    NOT DISTINCT (PostgreSQL 15+, нижняя граница проекта): NULL-safe на
--    уровне БД; идемпотентная data-миграция использует тот же индекс как
--    ON CONFLICT. FK: слот → категория/родитель CASCADE (конфигурация
--    следует за сущностью); подтипы-заводы заблокированы существующими FK
--    000048/000051 (NO ACTION / RESTRICT) — не наша проблема.
CREATE TABLE producer_slots (
    id          BIGSERIAL PRIMARY KEY,
    parent_id   BIGINT NOT NULL REFERENCES producer_types (id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    race_family TEXT NULL,
    race        TEXT NULL,
    hidden      BOOL NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (race IS NULL OR race_family IS NOT NULL),
    UNIQUE NULLS NOT DISTINCT (parent_id, category_id, race_family, race)
);

-- Data-миграция (§1.3, путь 1 — существующие БД; сид producer_catalog_seed
-- уже отработал). Путь 2 — Go-сид (seed_producers.go) для свежих БД, где
-- типов Фабрика/Автофабрика/Платформа ещё нет в момент миграции.

-- 1a) Слоты под ВСЕ существующие подтипы kind=goods (по их уровню расовости):
--     ничего не пропадает (инвариант С4 §1.4 п.1 для существующих данных).
INSERT INTO producer_slots (parent_id, category_id, race_family, race)
SELECT DISTINCT parent_id, category_id, race_family, race
FROM producer_types
WHERE parent_id IS NOT NULL AND kind = 'goods' AND category_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- 1b) Базовый сид универсального уровня (hidden=false): Фабрика/Автофабрика —
--     все товарные категории (kind='good', 13), Платформа — ресурсные
--     (kind='resource', 6). По name_norm родителя (паттерн 000051: переименованный
--     тип → слоты не создаются, риск зафиксирован в отчёте).
-- ⚠️ name_norm сравнивается ЛИТЕРАЛАМИ нижнего регистра, НЕ через lower():
--    коллация БД zorion = C, PostgreSQL lower() не конвертирует кириллицу
--    (docs/PITFALLS.md «БД и шелл»). name_norm пишет Go (graph.NormalizeName =
--    strings.ToLower, Unicode) — литералы ниже совпадают с ним.
INSERT INTO producer_slots (parent_id, category_id)
SELECT p.id, c.id FROM producer_types p CROSS JOIN categories c
WHERE p.name_norm IN ('фабрика', 'автофабрика') AND c.kind = 'good'
ON CONFLICT DO NOTHING;
INSERT INTO producer_slots (parent_id, category_id)
SELECT p.id, c.id FROM producer_types p CROSS JOIN categories c
WHERE p.name_norm = 'добывающая платформа' AND c.kind = 'resource'
ON CONFLICT DO NOTHING;