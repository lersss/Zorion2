-- effects-hunger-resolve.sql — уборка живой фикстуры приёмки «голод работает».
--
-- Удаляет ВСЁ, что создал tools/e2e/fixtures/effects-hunger-setup.sql
-- (поселение, ветка+буферы, активный эффект, рецепт с компонентами, товары).
-- Категорию-позицию «продовольствие» НЕ трогает: она часть сида товаров.
--
-- КАК ЗАПУСКАТЬ:
--   powershell -File tools/db.ps1 -File tools/e2e/fixtures/effects-hunger-resolve.sql

\set ON_ERROR_STOP on
BEGIN;

DELETE FROM active_effects
 WHERE owner_type = 'settlement' AND owner_id = 'e0000000-0000-4000-8000-000000000001';

DELETE FROM settlement_branches
 WHERE settlement_id = 'e0000000-0000-4000-8000-000000000001';

DELETE FROM settlements
 WHERE id = 'e0000000-0000-4000-8000-000000000001';

DELETE FROM recipes
 WHERE good_id IN (SELECT id FROM goods WHERE name_norm = 'qa голодтовар');

DELETE FROM goods
 WHERE name_norm IN ('qa голодтовар', 'qa голодсырьё');

COMMIT;
