-- 000068: трюм — грузоподъёмность корабля (спека
-- 2026-09-22-трюм-грузоподъёмность-корабля §8). Ёмкость — производная:
-- врождённая ёмкость модели корабля (ship_models.base_capacity) + сумма
-- грузовых модулей (equipment.type='cargo', params.capacity) в универсальных
-- слотах. Содержимое — отдельная таблица player_cargo (решение создателя №4).
-- Состояние игрока: player_cargo НЕ входит в truncateTables (И5) — переживает
-- очистку вселенной, как кошелёк (accounts) и активный полёт (player_flights);
-- FK player_cargo → goods/users (не усекаемые) сторож admin_universe_test.go
-- не задевает. Каталог (goods) не меняем.

-- 1. Расширить CHECK типа оборудования: + cargo (грузовой модуль).
ALTER TABLE equipment DROP CONSTRAINT equipment_type_check;
ALTER TABLE equipment ADD CONSTRAINT equipment_type_check
    CHECK (type IN ('radar','scanner','engine','cargo'));

-- 2. Врождённая ёмкость модели корабля (тонны).
ALTER TABLE ship_models ADD COLUMN base_capacity DOUBLE PRECISION NOT NULL DEFAULT 0;

-- 3. Стартовая модель: врождённая ёмкость 20 т + один универсальный слот.
UPDATE ship_models
SET base_capacity = 20,
    slots = slots || '{"universal": 1}'::jsonb
WHERE id = 'starter';

-- 4. Справочник: стартовый грузовой модуль (80 т; суммарная ёмкость 100 т).
INSERT INTO equipment (id, type, name, params)
VALUES ('cargo_1', 'cargo', 'Грузовой модуль-1', '{"capacity": 80}')
ON CONFLICT (id) DO NOTHING;

-- 5. Содержимое трюма: игрок → товар → количество (решение создателя №4).
CREATE TABLE player_cargo (
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    good_id    BIGINT NOT NULL REFERENCES goods (id) ON DELETE CASCADE,
    quantity   DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, good_id)
);

-- 6. Бэкфилл существующих игроков: стартовый модуль в универсальный слот.
UPDATE users
SET equipment = jsonb_set(COALESCE(equipment, '{}'::jsonb), '{universal}', '"cargo_1"')
WHERE equipment IS NULL
   OR NOT (equipment ? 'universal')
   OR equipment->>'universal' IS NULL
   OR equipment->>'universal' = '';
