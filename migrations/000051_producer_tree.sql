-- Дерево построек студии (спека 2026-09-21-студия-дерево-построек-канвас §1.2/§1.3):
-- 1) Базовый тип/подтип: parent_id — ссылка на тип-родителя (подтип → тип).
--    Удаление родителя — RESTRICT: тип с подтипами не удаляется (§1.2 п.7),
--    каскад запрещён — случайный снос Фабрики не должен уносить фабрики категорий.
-- 2) Второй уровень расовости: race — id расы из config/races.json
--    (задана → race_family обязана быть задана и соответствовать, §1.2 п.6).
--
-- Data-миграция (§1.3): сид producer_catalog_seed уже отработал на существующих
-- БД (маркер) — правки каталога для текущих БД делает data-миграция, для свежих —
-- обновлённый сид (seed_producers.go). Оба пути дают одинаковый результат.
-- Все UPDATE — по name_norm (устойчиво к переименованиям в студии: запись не
-- найдена → пропуск, не падать).
--
-- ⚠️ name_norm сравнивается ЛИТЕРАЛАМИ нижнего регистра, НЕ через lower():
-- коллация БД zorion = C, PostgreSQL lower() не конвертирует кириллицу
-- (docs/PITFALLS.md «БД и шелл»). name_norm пишет Go (graph.NormalizeName =
-- strings.ToLower, Unicode) — литералы ниже совпадают с ним.

ALTER TABLE producer_types ADD COLUMN parent_id BIGINT NULL
    REFERENCES producer_types (id) ON DELETE RESTRICT;
ALTER TABLE producer_types ADD COLUMN race TEXT NULL;

-- 1) Создать тип «Лаборатория» (kind=items, родитель NULL, approved) —
--    INSERT ... ON CONFLICT (name_norm) DO NOTHING (если уже заведена вручную).
INSERT INTO producer_types (name, name_norm, kind, status)
VALUES ('Лаборатория', 'лаборатория', 'items', 'approved')
ON CONFLICT (name_norm) DO NOTHING;

-- 2) Переименовать лаборатории (решение создателя, идея §2 п.4):
--    связи producer_items не трогаются (связи по id, переименование их не
--    затрагивает).
UPDATE producer_types SET name = 'Лаборатория космических технологий',
    name_norm = 'лаборатория космических технологий'
WHERE name_norm = 'лаборатория корабельных модулей';
UPDATE producer_types SET name = 'Лаборатория экипировки',
    name_norm = 'лаборатория экипировки'
WHERE name_norm = 'лаборатория инструментов игрока';
UPDATE producer_types SET name = 'Исследовательская лаборатория',
    name_norm = 'исследовательская лаборатория'
WHERE name_norm = 'лаборатория исследовательская';

-- 3) Проставить parent_id: три лаборатории → id типа «Лаборатория»;
--    «Фабрика продовольствия» (создана создателем) → id типа «Фабрика».
UPDATE producer_types SET parent_id = (SELECT id FROM producer_types WHERE name_norm = 'лаборатория')
WHERE name_norm = 'лаборатория космических технологий';
UPDATE producer_types SET parent_id = (SELECT id FROM producer_types WHERE name_norm = 'лаборатория')
WHERE name_norm = 'лаборатория экипировки';
UPDATE producer_types SET parent_id = (SELECT id FROM producer_types WHERE name_norm = 'лаборатория')
WHERE name_norm = 'исследовательская лаборатория';
UPDATE producer_types SET parent_id = (SELECT id FROM producer_types WHERE name_norm = 'фабрика')
WHERE name_norm = 'фабрика продовольствия';

-- 4) «Добывающая платформа» — сбросить category_id в NULL: она становится
--    чистым типом уровня 3; категории живут в подтипах-платформах (§2.2).
--    Подтип «Платформа минералов» не создаётся — остаётся узлом-приглашением.
UPDATE producer_types SET category_id = NULL
WHERE name_norm = 'добывающая платформа';