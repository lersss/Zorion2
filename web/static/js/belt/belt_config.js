// web/static/js/belt/belt_config.js
// Константы клиента мини-игры добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §5.1/§8.2). Мир — локальное пятно
// пояса (не бесконечность), единица — мировые единицы (м.е.); 1 м.е. ≈ 1 px
// при WORLD_SCALE = 1. Числа — клиентские константы сцены, не баланс.

export const WORLD_SCALE = 1;

// Пятно пояса (§8.2): диаметр 3000–5000 м.е.; мягкая граница — «дальше пояс
// рассеивается» (упругий отскок у края, игрок не теряется).
export const FIELD_DIAMETER = 4200;
export const FIELD_RADIUS = FIELD_DIAMETER / 2;
export const EDGE_SOFT = 260;

// Астероиды (§8.2): 20–40 тел на пятно, крупных «жил» 6–10; радиусы в м.е.
export const ASTEROID_MIN = 20;
export const ASTEROID_MAX = 40;
export const VEIN_MIN = 6;
export const VEIN_MAX = 10;
export const VEIN_RADIUS_MIN = 34;
export const VEIN_RADIUS_MAX = 70;
export const DECOR_RADIUS_MIN = 10;
export const DECOR_RADIUS_MAX = 26;
export const ASTEROID_DRIFT_MAX = 14; // м.е./с — медленный дрейф
export const ASTEROID_SPIN_MAX = 0.25; // рад/с — медленное вращение

// Радиус захвата бурения R_extract (§8.2): 40–60 м.е. (от поверхности тела).
export const EXTRACT_RADIUS = 52;

// Корабль игрока (§5.1): инерционный полёт, тяга по 4 направлениям, слабое
// торможение, тормоз — Shift (решение приёмной 2026-09-22).
export const SHIP_RADIUS = 12;
export const SHIP_SIZE = 42; // экранный размер спрайта, px
export const SHIP_THRUST = 430; // м.е./с²
export const SHIP_MAX_SPEED = 330; // м.е./с
export const SHIP_DRAG = 0.22; // слабое торможение (доля/с)
export const SHIP_BRAKE = 2.6; // Shift: торможение (доля/с)
export const SHIP_TURN_LERP = 0.16; // поворот к вектору тяги
export const COLLISION_BLOCK_MS = 260; // столкновение прерывает бурение

// Камера и слои (§9.2): звёздное поле (параллакс 0.2), далёкая пыль (0.4),
// мелкие обломки (0.6), крупные жилы (1.0).
export const CAMERA_LERP = 0.12;
export const STAR_TILE = 1100;
export const STAR_COUNT = 240;
export const DUST_COUNT = 26;
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
    edge: 'rgba(148,163,184,0.10)',
};
