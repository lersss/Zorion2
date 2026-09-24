-- 000080: трюм — дельта (спека 2026-09-22-трюм-грузоподъёмность-корабля §20)
-- Решения создателя 2026-09-24: модуль cargo_1 80 → 30 т (старт 20 + 30 = 50 т);
-- универсальных слотов 1 → 3 (два новых пустые). Ключи universal2/universal3
-- в users.equipment НЕ заводим (пустой слот = отсутствие ключа, §20.3);
-- перегруженные трюмы обрезаются до новой ёмкости (§20.8 п.4).
-- Все шаги идемпотентны (повторный прогон не меняет результат).

-- 1. Значение существующего модуля: 80 → 30 т (глобально, у всех, у кого он стоит).
UPDATE equipment
SET params = jsonb_set(params, '{capacity}', '30')
WHERE id = 'cargo_1';

-- 2. Стартовая модель: три универсальных слота (1 → 3).
UPDATE ship_models
SET slots = slots || '{"universal": 3}'::jsonb
WHERE id = 'starter';

-- 3. Обрезка перегруженных трюмов до новой ёмкости (used ≤ total восстанавливается
--    для всех, а не только для новых записей). Ёмкость игрока = ship_models.base_capacity
--    + Σ params.capacity установленных cargo-модулей — та же формула, что
--    ship.CargoCapacityMass (§8.3, §11 И4). Обрезка детерминированная и идемпотентная:
--    строки идут в порядке good_id, масса набирается кумулятивно; «хвост» сверх
--    ёмкости удаляется, пограничная строка урезается до влезающего остатка
--    (лишнее убирается), строки с нулём удаляются (конвенция «строк с нулём нет», §8).
--    ВАЖНО: шаг идёт ПОСЛЕ шагов 1–2 — ёмкость считается уже по новым числам
--    (cargo_1 = 30 т), иначе обрезка считалась бы по старой ёмкости (100 т).
WITH cap AS (
    SELECT u.id AS user_id,
           COALESCE(sm.base_capacity, 0) + COALESCE((
               SELECT SUM((e.params->>'capacity')::double precision)
                 FROM jsonb_each_text(u.equipment) AS eq(key, module_id)
                 JOIN equipment e ON e.id = eq.module_id
                WHERE e.type = 'cargo'
                  AND (e.params->>'capacity') ~ '^[0-9]+(\.[0-9]+)?$'
           ), 0) AS total
      FROM users u
      LEFT JOIN ship_models sm ON sm.id = u.ship_model_id
),
walk AS (
    SELECT pc.user_id, pc.good_id, pc.quantity, g.weight, c.total,
           COALESCE(SUM(pc.quantity * g.weight) OVER (
               PARTITION BY pc.user_id ORDER BY pc.good_id
               ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING), 0) AS mass_before
      FROM player_cargo pc
      JOIN goods g ON g.id = pc.good_id
      JOIN cap c ON c.user_id = pc.user_id
),
tail AS (
    DELETE FROM player_cargo pc
     USING walk w
     WHERE pc.user_id = w.user_id AND pc.good_id = w.good_id
       AND w.mass_before >= w.total
)
UPDATE player_cargo pc
   SET quantity = (w.total - w.mass_before) / w.weight,
       updated_at = NOW()
  FROM walk w
 WHERE pc.user_id = w.user_id AND pc.good_id = w.good_id
   AND w.mass_before < w.total
   AND w.mass_before + w.quantity * w.weight > w.total;

-- 3b. Строки, усечённые до нуля, удаляются (конвенция «строк с нулём нет», §8/DB.md).
DELETE FROM player_cargo WHERE quantity = 0;
