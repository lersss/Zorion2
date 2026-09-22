-- 000067_settlement_type.sql — тип поселения (спека 2026-09-22-поселение-
-- потребление-населением-итерация-4 §3.1, решения создателя 32/37/38/40).
--
-- Номер: спека бронирует 000066, но параллельная задача «фракции: одна на расу
-- со столицей» уже создала migrations/000066_factions_race.sql (незакоммичен,
-- залочен в docs/COORDINATION.md). Две миграции с одним числовым префиксом дают
-- одну версию schema_migrations (PRIMARY KEY) → падение старта сервера
-- (PITFALLS «БД и шелл», «Номер миграции может быть занят»). Берём следующий
-- свободный номер — 000067.

-- Связь «поселение → его тип» (решение п.32). RESTRICT: тип, на который
-- ссылаются поселения, удалить нельзя (студия отдаёт 409).
ALTER TABLE settlements ADD COLUMN settlement_type_id BIGINT NULL
    REFERENCES producer_types (id) ON DELETE RESTRICT;
CREATE INDEX idx_settlements_type ON settlements (settlement_type_id);

-- Снятие неиспользуемого флага `residual` у типа-родителя «Поселение»
-- (решение гейта п.38 — «убрать»; флаг нигде не читается).
-- Свежая БД: сид ставит output='{}' сразу — оба пути дают один результат.
UPDATE producer_types SET output = '{}'
WHERE name_norm = 'поселение' AND parent_id IS NULL AND output = '{"residual": true}';

-- Дефолтный тип (существующие БД, где сид producer_catalog_seed уже отработал):
-- подтип под «Поселением», БЕЗ категории (Поселение — тип без слотов).
-- Свежая БД: «Поселения» ещё нет (сид идёт после миграций) → 0 строк, тип
-- создаст Go-сид (seed_producers.go). Оба пути дают один результат.
-- ⚠️ Число 2.5e-08 — ОДНО утверждённое: то же в сиде и в DefaultEatK;
-- согласованность закреплена тестом T18. Структура норм — на товар-выход
-- ветки (п.45): ключ — name_norm товара-выхода. name_norm сравниваем
-- литералами нижнего регистра (collation C не конвертирует кириллицу).
INSERT INTO producer_types (name, name_norm, kind, parent_id, output, input, params)
SELECT 'Обычное поселение', 'обычное поселение', 'goods', p.id, '{}', '{}',
       '{"eat": {"вода": 2.5e-08, "пища": 2.5e-08}}'
FROM producer_types p
WHERE p.name_norm = 'поселение' AND p.parent_id IS NULL
ON CONFLICT (name_norm) DO NOTHING;

-- Бэкфилл существующих поселений на единственный дефолтный тип (§3.4).
-- Свежая БД: поселений нет → no-op; если тип не создан — подзапрос NULL → no-op.
UPDATE settlements SET settlement_type_id = (
    SELECT id FROM producer_types WHERE name_norm = 'обычное поселение'
) WHERE settlement_type_id IS NULL;
