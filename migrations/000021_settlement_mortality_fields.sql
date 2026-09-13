-- Смерть населения от среды (docs/gamedesign/18a_population_death.md):
-- population_exact — точное (дробное) состояние для пересчёта, отдельно от
-- population (integer) — тот остаётся округлённой витриной для игрока.
-- computed_at — точка отсчёта Δt для ленивого пересчёта.
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS population_exact double precision NOT NULL DEFAULT 0;
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS computed_at timestamptz NOT NULL DEFAULT now();
UPDATE settlements SET population_exact = population::double precision WHERE population_exact = 0;
