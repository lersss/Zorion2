-- Экзотические типы звёзд и модификаторы (спека 99.2.4 §3):
-- worlds.system_type — структура системы (single/binary/multiple),
-- worlds.star_type — тип центрального объекта (star/white_dwarf/neutron/black_hole/protostar),
-- worlds.stellar_mods — модификаторы (JSONB, открытый пакет: phase, variable_type,
--   subtype, binary_type, disk_state, companion, stellar_mass, metallicity и пр.).
--
-- Дефолты миграции: существующие миры становятся single/star без модификаторов —
-- их фактическое состояние (обратная совместимость, 99.2.4 §3).
-- spectral_class допускает NULL (экзотика пишет NULL — колонка не имеет NOT NULL).

ALTER TABLE worlds ADD COLUMN system_type TEXT NOT NULL DEFAULT 'single';
ALTER TABLE worlds ADD COLUMN star_type   TEXT NOT NULL DEFAULT 'star';
ALTER TABLE worlds ADD COLUMN stellar_mods JSONB;

ALTER TABLE worlds ADD CONSTRAINT worlds_system_type_check
  CHECK (system_type IN ('single','binary','multiple'));
ALTER TABLE worlds ADD CONSTRAINT worlds_star_type_check
  CHECK (star_type IN ('star','white_dwarf','neutron','black_hole','protostar'));

COMMENT ON COLUMN worlds.system_type  IS 'Тип системы: single/binary/multiple';
COMMENT ON COLUMN worlds.star_type    IS 'Тип объекта: star/white_dwarf/neutron/black_hole/protostar';
COMMENT ON COLUMN worlds.stellar_mods IS 'Модификаторы: phase, variable_type, subtype, binary_type, disk_state, companion, stellar_mass, metallicity и пр. (открытый пакет)';