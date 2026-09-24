-- 000084_catalog_code.sql — номерная метка записи каталога (метка переноса
-- `code`), спека 2026-09-24-каталог-экспорт-импорт-контента-на-прод §3.1–3.3
-- (итерация И1). Метка — стабильная идентичность записи каталога для переноса
-- dev → прод: не `id` (автоинкремент, в каждой базе свой) и не `name_norm`
-- (меняется при переименовании), а бессмысленный НОМЕР (решение создателя
-- 2026-09-24 «пусть метка будет цифровой»).
--
-- Формат: префикс таблицы + номер (минимум 4 цифры, растёт без обрезки):
--   goods g_0001, producer_types p_0001, items i_0001, effect_types e_0001.
-- Уникальность — в пределах своей таблицы (частичный UNIQUE, `WHERE code IS NOT
-- NULL`), не глобально (§3.1). `categories` метки НЕ получает: у неё уже есть
-- семантический `code` (water/gas/…, 000045) — смешивать два смысла в одном
-- поле нельзя; идентичность `categories` — `(kind, name_norm)` (§3.4).
--
-- Заведение: SEQUENCE на таблицу (монотонный, безопасный при параллельных
-- правках студии); новая запись получает метку из счётчика (DEFAULT), существующие
-- — бэкфилл в порядке id (детерминированно). Порядок «id ↔ номер» тот же, что у
-- сида свежей БД, поэтому dev и свежая база совпадают (§3.2).
--
-- ⚠️ lpad(n::text, 4, '0') ОБРЕЗАЕТ номер ≥ 10000 до 4 цифр ('10001' → '1000'),
-- а to_char(n, 'FM0000') переполняется в '####' (проверено на PG): номер обязан
-- расти без обрезки (`g_10001`). Поэтому формат собран в функции с CASE
-- (единственный nextval на вызов), а не сырым lpad в DEFAULT.
--
-- Номер 000084 зафиксирован менеджером (последняя в дереве/DB.md — 000082;
-- 000083 занята ренейм-миграцией канон-имён этой же итерации).

-- 1) Счётчики меток (SEQUENCE на таблицу, §3.2).
CREATE SEQUENCE IF NOT EXISTS goods_code_seq;
CREATE SEQUENCE IF NOT EXISTS producer_types_code_seq;
CREATE SEQUENCE IF NOT EXISTS items_code_seq;
CREATE SEQUENCE IF NOT EXISTS effect_types_code_seq;

-- 1a) Форматтер номера: префикс + минимум 4 цифры, БЕЗ обрезки (g_0001 … g_10001).
CREATE OR REPLACE FUNCTION catalog_format_code(prefix text, n bigint)
RETURNS text
LANGUAGE sql IMMUTABLE AS $$
    SELECT prefix || CASE WHEN n < 10000 THEN lpad(n::text, 4, '0') ELSE n::text END
$$;

-- 1b) Выдача метки новой записи: один nextval из счётчика таблицы.
CREATE OR REPLACE FUNCTION catalog_next_code(prefix text, seq regclass)
RETURNS text
LANGUAGE sql VOLATILE AS $$
    SELECT catalog_format_code(prefix, nextval(seq))
$$;

-- 2) Колонка метки + DEFAULT из счётчика. Колонка nullable (§3.1): NULL —
--    «запись вне управления» (импорт такое отвергает, §5.4).
ALTER TABLE goods ADD COLUMN IF NOT EXISTS code TEXT;
ALTER TABLE goods ALTER COLUMN code SET DEFAULT catalog_next_code('g_', 'goods_code_seq');

ALTER TABLE producer_types ADD COLUMN IF NOT EXISTS code TEXT;
ALTER TABLE producer_types ALTER COLUMN code SET DEFAULT catalog_next_code('p_', 'producer_types_code_seq');

ALTER TABLE items ADD COLUMN IF NOT EXISTS code TEXT;
ALTER TABLE items ALTER COLUMN code SET DEFAULT catalog_next_code('i_', 'items_code_seq');

ALTER TABLE effect_types ADD COLUMN IF NOT EXISTS code TEXT;
ALTER TABLE effect_types ALTER COLUMN code SET DEFAULT catalog_next_code('e_', 'effect_types_code_seq');

-- 3) Бэкфилл существующих: номера в порядке id (детерминированно), только NULL
--    (повторный прогон — no-op). base.n — уже занятый максимум: новая порция
--    нумеруется после него (защита от частичного состояния).
WITH base AS (
    SELECT COALESCE(MAX((substring(code from 3))::int) FILTER (WHERE code ~ '^g_[0-9]+$'), 0) AS n FROM goods
), ordered AS (
    SELECT id, row_number() OVER (ORDER BY id) AS rn FROM goods WHERE code IS NULL
)
UPDATE goods g SET code = catalog_format_code('g_', o.rn + b.n)
FROM ordered o, base b WHERE g.id = o.id;

WITH base AS (
    SELECT COALESCE(MAX((substring(code from 3))::int) FILTER (WHERE code ~ '^p_[0-9]+$'), 0) AS n FROM producer_types
), ordered AS (
    SELECT id, row_number() OVER (ORDER BY id) AS rn FROM producer_types WHERE code IS NULL
)
UPDATE producer_types g SET code = catalog_format_code('p_', o.rn + b.n)
FROM ordered o, base b WHERE g.id = o.id;

WITH base AS (
    SELECT COALESCE(MAX((substring(code from 3))::int) FILTER (WHERE code ~ '^i_[0-9]+$'), 0) AS n FROM items
), ordered AS (
    SELECT id, row_number() OVER (ORDER BY id) AS rn FROM items WHERE code IS NULL
)
UPDATE items g SET code = catalog_format_code('i_', o.rn + b.n)
FROM ordered o, base b WHERE g.id = o.id;

WITH base AS (
    SELECT COALESCE(MAX((substring(code from 3))::int) FILTER (WHERE code ~ '^e_[0-9]+$'), 0) AS n FROM effect_types
), ordered AS (
    SELECT id, row_number() OVER (ORDER BY id) AS rn FROM effect_types WHERE code IS NULL
)
UPDATE effect_types g SET code = catalog_format_code('e_', o.rn + b.n)
FROM ordered o, base b WHERE g.id = o.id;

-- 4) Сдвиг счётчиков на максимум: следующая новая запись продолжит нумерацию
--    (§3.2). Пустая таблица — setval не трогаем (последовательность с 1).
SELECT setval('goods_code_seq', m.max_n, true)
FROM (SELECT MAX((substring(code from 3))::int) AS max_n FROM goods WHERE code ~ '^g_[0-9]+$') m
WHERE m.max_n IS NOT NULL;

SELECT setval('producer_types_code_seq', m.max_n, true)
FROM (SELECT MAX((substring(code from 3))::int) AS max_n FROM producer_types WHERE code ~ '^p_[0-9]+$') m
WHERE m.max_n IS NOT NULL;

SELECT setval('items_code_seq', m.max_n, true)
FROM (SELECT MAX((substring(code from 3))::int) AS max_n FROM items WHERE code ~ '^i_[0-9]+$') m
WHERE m.max_n IS NOT NULL;

SELECT setval('effect_types_code_seq', m.max_n, true)
FROM (SELECT MAX((substring(code from 3))::int) AS max_n FROM effect_types WHERE code ~ '^e_[0-9]+$') m
WHERE m.max_n IS NOT NULL;

-- 5) Частичный UNIQUE — уникальность метки в пределах таблицы (§3.1).
CREATE UNIQUE INDEX IF NOT EXISTS uq_goods_code ON goods (code) WHERE code IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_producer_types_code ON producer_types (code) WHERE code IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_items_code ON items (code) WHERE code IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_effect_types_code ON effect_types (code) WHERE code IS NOT NULL;
