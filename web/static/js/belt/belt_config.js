// web/static/js/belt/belt_config.js
// Константы клиента мини-игры добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §5.1/§8.2, ревизия 4). Мир —
// бесконечный вдоль кольца пояс (ось X — вдоль кольца, ось Y — поперёк);
// единица — мировые единицы (м.е.); 1 м.е. ≈ 1 px при WORLD_SCALE = 1.
// Числа — клиентские константы сцены, не баланс.

export const WORLD_SCALE = 1;

// Бесконечный пояс (§5.1/§8.2): поперёк — полоса ±BELT_HALF_WIDTH, плотность
// тел падает к краям (линейный спад до нуля), за краем камней нет; упругого
// отскока у края нет ни у корабля, ни у камней. Вдоль кольца тела стримятся
// ячейками BELT_CELL в радиусе BELT_STREAM_RADIUS вокруг корабля.
export const BELT_HALF_WIDTH = 900; // полоса сцены по Y: 1800 м.е.
export const BELT_CELL = 600; // ячейка стриминга (X и Y), м.е.
export const BELT_STREAM_RADIUS = 2600; // радиус стриминга, м.е.
export const BELT_ASTEROID_CAP = 120; // кап жил в памяти сцены
export const BELT_DUST_CAP = 60; // кап пятен пыли
export const BELT_CELL_BODIES_MIN = 4; // тел-кандидатов на ячейку
export const BELT_CELL_BODIES_MAX = 6;
export const BELT_CELL_VEINS_MIN = 1; // из них жил
export const BELT_CELL_VEINS_MAX = 2;
export const BELT_CELL_DUST_CHANCE = 0.4; // пятен пыли на ячейку (ожидание)

// Астероиды (§8.2): радиусы не меняются (их берёт арт-ТЗ art_belt_asteroids.md).
export const VEIN_RADIUS_MIN = 34;
export const VEIN_RADIUS_MAX = 70;
export const DECOR_RADIUS_MIN = 10;
export const DECOR_RADIUS_MAX = 26;
export const ASTEROID_DRIFT_MAX = 14; // м.е./с — медленный дрейф
export const ASTEROID_SPIN_MAX = 0.25; // рад/с — медленное вращение

// Радиус захвата бурения R_extract (§8.2): 40–60 м.е. (от поверхности тела).
export const EXTRACT_RADIUS = 52;

// Корабль игрока (§5.1, ревизия 4): «два стика» — W/S тяга с разгоном,
// A/D стрейф, мышь — ориентация носа с ограниченной угловой скоростью.
// Абсолютные тяга/скорость/торможение не меняются (меняется схема, не баланс).
export const SHIP_RADIUS = 12;
export const SHIP_SIZE = 42; // экранный размер спрайта, px
export const SHIP_THRUST = 430; // м.е./с²
export const SHIP_MAX_SPEED = 330; // м.е./с
export const SHIP_DRAG = 0.22; // слабое торможение (доля/с)
export const SHIP_BRAKE = 2.6; // Shift: торможение (доля/с)
export const SHIP_REVERSE_FACTOR = 0.5; // реверс (S) слабее
export const SHIP_STRAFE_FACTOR = 1.0; // стрейф (A/D) полной тяги
export const SHIP_THROTTLE_RATE_UP = 1.4; // нарастание тяги (1/с)
export const SHIP_THROTTLE_RATE_DOWN = 2.2; // сброс тяги (1/с)
export const SHIP_TURN_ACCEL = 35; // угловое ускорение доворота, рад/с²
export const SHIP_TURN_RATE_MAX = 7; // потолок угловой скорости, рад/с
export const SHIP_TURN_DEADZONE = 0.04; // мёртвая зона у цели, рад
export const COLLISION_BLOCK_MS = 260; // столкновение прерывает бурение

// Камера и слои (§9.2): звёздное поле (параллакс 0.2), далёкая пыль (0.4),
// мелкие обломки (0.6), крупные жилы (1.0).
export const CAMERA_LERP = 0.12;
export const STAR_TILE = 1100;
export const STAR_COUNT = 240;
export const PARALLAX_STARS = 0.2;
export const PARALLAX_DUST = 0.4;
export const PARALLAX_DECOR = 0.6;

// Сеть: событие сбора раз в ~0.5–1 с (§5.3).
export const COLLECT_INTERVAL_MS = 700;

// Палитра сцены — существующие цвета проекта (новых не вводим).
export const COLORS = {
    bg: '#05070f',
    asteroid: '#6b7280',
    asteroidVein: '#8b93a1',
    asteroidEdge: '#454c58',
    asteroidDark: '#3b414b',
    glint: '#fde68a',
    beam: '#fde68a',
    particle: '#d1d5db',
    shipFallback: '#e2e8f0',
    shipAccent: '#38bdf8',
};
