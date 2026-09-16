-- migrations/000035_restore_assignments_type_reward.sql
--
-- Восстанавливает колонки assignments.type и assignments.reward, удалённые
-- вручную после применения 000010 (000010 числится применённой, но колонок
-- в БД нет — запросы repository/assignment_repository.go падают с
-- "column \"type\" does not exist").
--
-- IF NOT EXISTS — идемпотентность на случай повторного применения.

ALTER TABLE assignments
    ADD COLUMN IF NOT EXISTS type text NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS reward integer NOT NULL DEFAULT 0;
