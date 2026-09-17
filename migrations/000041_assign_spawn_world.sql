-- 77a: бэкфилл стартового мира для игроков без current_world_id.
-- Баг (найден создателем после сдачи 77a): новый игрок с NULL не видит карту —
-- видимость 77a требует позицию (PlayerPosition → ok=false → все кластеры
-- урезаются до точек-огоньков, полёт невозможен). Решение создателя
-- 2026-09-17: «давай его пока к людям кидать» — стартовый мир, где есть
-- поселение расы humans (ближайший к центру галактики); фолбэк — ближайший
-- к центру мир вообще (как assignCurrentWorldsTx). Миров нет — NULL не трогаем
-- (EXISTS-гвард: UPDATE не выполняется, NULL не затирается значением).
-- admin/skycomposer НЕ трогаем (И7 — видят всё; skycomposer уже назначен
-- assignCurrentWorldsTx).
UPDATE users SET current_world_id = COALESCE(
    (SELECT w.id FROM worlds w
     WHERE EXISTS (
         SELECT 1 FROM settlements s
         JOIN planets p ON p.id = s.planet_id
         WHERE p.world_id = w.id AND s.race_id = 'humans'
     )
     ORDER BY (w.coord_x * w.coord_x + w.coord_y * w.coord_y)
     LIMIT 1),
    (SELECT id FROM worlds
     ORDER BY (coord_x * coord_x + coord_y * coord_y)
     LIMIT 1)
)
WHERE role = 'player' AND current_world_id IS NULL
  AND EXISTS (SELECT 1 FROM worlds);