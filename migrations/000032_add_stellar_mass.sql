-- Масса звезды в солнечных массах M☉ (дополнение 29a §4м).
-- Nullable: старые миры без массы; новые — заполняются генератором
-- (диапазоны по спектральному классу/типу объекта, конфиг 99.2.3).

ALTER TABLE worlds ADD COLUMN stellar_mass DOUBLE PRECISION;

COMMENT ON COLUMN worlds.stellar_mass IS 'Масса звезды в солнечных массах M☉ (29a §4м); NULL у старых миров';