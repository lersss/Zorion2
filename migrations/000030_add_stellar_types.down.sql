ALTER TABLE worlds DROP CONSTRAINT IF EXISTS worlds_star_type_check;
ALTER TABLE worlds DROP CONSTRAINT IF EXISTS worlds_system_type_check;
ALTER TABLE worlds DROP COLUMN IF EXISTS stellar_mods;
ALTER TABLE worlds DROP COLUMN IF EXISTS star_type;
ALTER TABLE worlds DROP COLUMN IF EXISTS system_type;