-- 000089_accelerator.sql
-- Ускоритель перелёта — модуль корабля (спека
-- 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md §3.1–§3.3, ЧК1).
-- Тип оборудования accelerator + стартовый модуль accel_1 + выделенный слот
-- accelerator у модели starter + выдача новому игроку + состояние отката.
--
-- НОМЕР: спека предлагала 000088, но номер занят параллельной задачей
-- (000088_contract_author_settlement.sql в рабочем дереве) — взят следующий
-- свободный 000089 (PITFALLS «БД и шелл»: проверять рабочее дерево, не только
-- docs/DB.md).
--
-- Идемпотентно при повторном прогоне (DROP IF EXISTS / ON CONFLICT /
-- IF NOT EXISTS / jsonb || идемпотентен по ключу).

-- 1. Новый тип оборудования: + accelerator (по образцу cargo, 000068 §1).
ALTER TABLE equipment DROP CONSTRAINT IF EXISTS equipment_type_check;
ALTER TABLE equipment ADD CONSTRAINT equipment_type_check
    CHECK (type IN ('radar','scanner','engine','cargo','accelerator'));

-- 2. Каталожная запись стартового ускорителя (спека §3.1, числа §8):
--    игра "route" («Прокладка маршрута»), откат 25 мин, bonus_max 0.50,
--    порог показа 180 с, порог отправки 90 с.
INSERT INTO equipment (id, type, name, params)
VALUES ('accel_1', 'accelerator', 'Ускоритель-1',
        '{"game":"route","cooldown_min":25,"bonus_max":0.50,"min_remaining_offer_s":180,"min_remaining_boost_s":90}')
ON CONFLICT (id) DO NOTHING;

-- 3. Выделенный слот accelerator у стартовой модели (решение 10, §3.2).
--    jsonb || по существующему ключу идемпотентен.
UPDATE ship_models
SET slots = slots || '{"accelerator": 1}'::jsonb
WHERE id = 'starter';

-- 4. Выдача стартового модуля новому игроку (решение 12, §4.3): бэкфилл
--    существующих — где ключа нет/пуст (по образцу 000068 §6).
UPDATE users
SET equipment = jsonb_set(COALESCE(equipment, '{}'::jsonb), '{accelerator}', '"accel_1"')
WHERE equipment IS NULL
   OR NOT (equipment ? 'accelerator')
   OR equipment->>'accelerator' IS NULL
   OR equipment->>'accelerator' = '';

-- 5. Состояние отката игрока (спека §3.3): НЕ входит в truncateTables (состояние
--    игрока, переживает очистку вселенной; FK → users, как player_flights).
--    last_cooldown_min — снимок ПРИМЕНЁННОГО отката (М-5): смена модуля на ходу
--    не укорачивает и не сбрасывает уже запущенный откат. Признак «ускорение
--    действует» выводится из last_boost_at (сравнение UnixMilli) — отдельной
--    колонки нет (§3.3).
CREATE TABLE IF NOT EXISTS player_accelerator (
    user_id           UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    last_boost_at     TIMESTAMPTZ NULL,
    last_cooldown_min INT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
