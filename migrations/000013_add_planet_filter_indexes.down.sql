-- migrations/000013_add_planet_filter_indexes.down.sql

DROP INDEX IF EXISTS idx_planets_life_world;
DROP INDEX IF EXISTS idx_planets_habitable_world;
DROP INDEX IF EXISTS idx_planets_type_world;
