-- Снятие класса «согласование» в студии (спека
-- 2026-09-21-студия-производители-лестница-предложения.md §3, редакция v4).
-- Статусы draft/approved/excluded снимаются у goods/items/producer_types;
-- всё заведённое — «живое». Единственный обратимый флаг «скрыто» остаётся
-- ТОЛЬКО у записей-производителей (producer_types.hidden — скрытость
-- ЗАПИСИ-фабрики); у товаров/ресурсов (goods) и предметов (items) скрытия
-- нет — «убрать» только удалением. Вторая ось — producer_slots.hidden
-- (скрытость категории на уровне) — не трогается.
-- Колонка producer_types.hidden первой волны снята 000052 — возвращается (К1).
-- Вес/объём товара: значение есть всегда (Р2) — дефолт 1/1, NULL → 1, NOT NULL.

-- 1) producer_types: статус-класс → единственный обратимый флаг hidden.
ALTER TABLE producer_types ADD COLUMN hidden BOOLEAN NOT NULL DEFAULT false;
UPDATE producer_types SET hidden = true WHERE status = 'banned';
ALTER TABLE producer_types DROP COLUMN IF EXISTS status;

-- 2) goods (товары и ресурсы): статус-класс снимается БЕЗ замены флагом.
ALTER TABLE goods DROP COLUMN IF EXISTS status;
ALTER TABLE goods DROP COLUMN IF EXISTS banned_at;

-- 3) items: то же — статус снимается без замены флагом.
ALTER TABLE items DROP COLUMN IF EXISTS status;

-- 4) volume/weight: значение есть всегда — дефолт 1, NULL → 1, NOT NULL.
ALTER TABLE goods ALTER COLUMN volume SET DEFAULT 1;
ALTER TABLE goods ALTER COLUMN weight SET DEFAULT 1;
UPDATE goods SET volume = 1 WHERE volume IS NULL;
UPDATE goods SET weight = 1 WHERE weight IS NULL;
ALTER TABLE goods ALTER COLUMN volume SET NOT NULL;
ALTER TABLE goods ALTER COLUMN weight SET NOT NULL;
