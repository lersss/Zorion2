-- Снос легаси-таблицы production_units (эпоха 000018, спека
-- 2026-09-20-фабрики §11.6, решение создателя 3b.6.8 — СНЕСТИ).
-- Таблица не описана в ГДД, читается/пишется только удаляемым
-- internal/repository/production_unit_repository.go; входит в truncateTables
-- (admin_universe.go) и каскады пакмана (locations → production_units).
-- Индекс idx_production_units_location_id (001_init) удаляется вместе с таблицей.

DROP TABLE IF EXISTS production_units;