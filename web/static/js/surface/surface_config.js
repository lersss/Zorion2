// web/static/js/surface/surface_config.js
// Константы клиента прогулки (спека 2026-09-21 §7.2/§7.4). Формации — клиентская
// константа по biome_category (В9): НЕ второй контент-реестр рядом с каталогом.

// Физика игрока (§7.4): ходьба/спринт/прыжок + клип гравитации 0.2–2.5.
export const PPM = 40;                 // пикселей в метре (визуальный масштаб, не физический)
export const WALK_SPEED = 4;           // м/с
export const SPRINT_SPEED = 7;         // м/с
export const JUMP_SPEED = 4.5;         // м/с
export const G_CLAMP_MIN = 0.2;
export const G_CLAMP_MAX = 2.5;
export const G_EARTH = 9.81;

// Мир (§7.2): чанк 256 px, в памяти ±3.
export const CHUNK = 256;
export const CHUNK_RADIUS = 3;

// Парящая порода float-формаций (идея 2026-09-21 §4): полоса над рельефом и
// обязательный зазор под камнем. Зазор > роста игрока (28 px) — под аркой
// всегда есть проход, стен «до земли» не бывает.
export const FLOAT_SPAN = 260;
export const FLOAT_GAP = 48;

// Погода (§7.2): одно явление за 2–4 мин, без урона.
export const WEATHER_MIN_MS = 120000;
export const WEATHER_MAX_MS = 240000;

// Поведение камеры: доля сглаживания.
export const CAMERA_LERP = 0.12;

// Формации по категории биома (В9): 5–7 на категорию. Каждая — модулятор
// рельефа: ridge (крутизна), flatten (выравнивание), offset (сдвиг высоты),
// float (висячие скалы), caves (вероятность пещер).
export const FORMATIONS = {
    'литосфера': [
        { id: 'хребет', ridge: 1.6, flatten: 0.0, offset: -120, caves: 0.6, float: false },
        { id: 'каньон', ridge: 0.5, flatten: 0.9, offset: 140, caves: 0.8, float: false },
        { id: 'плато', ridge: 0.2, flatten: 1.0, offset: -60, caves: 0.4, float: false },
        { id: 'висячие скалы', ridge: 1.2, flatten: 0.2, offset: -40, caves: 1.0, float: true },
        { id: 'арки', ridge: 1.0, flatten: 0.3, offset: -30, caves: 1.0, float: true },
        { id: 'осыпи', ridge: 0.7, flatten: 0.6, offset: 30, caves: 0.5, float: false },
    ],
    'вода': [
        { id: 'рифы', ridge: 1.0, flatten: 0.3, offset: 0, caves: 0.9, float: false },
        { id: 'приливные каналы', ridge: 0.4, flatten: 0.8, offset: 60, caves: 0.7, float: false },
        { id: 'островки', ridge: 0.9, flatten: 0.4, offset: -20, caves: 0.5, float: false },
        { id: 'гроты', ridge: 1.1, flatten: 0.2, offset: 20, caves: 1.2, float: true },
        { id: 'отмели', ridge: 0.3, flatten: 1.0, offset: 80, caves: 0.4, float: false },
    ],
    'биосфера': [
        { id: 'холмы', ridge: 0.8, flatten: 0.4, offset: 0, caves: 0.7, float: false },
        { id: 'рощи', ridge: 0.5, flatten: 0.7, offset: -10, caves: 0.4, float: false },
        { id: 'овраги', ridge: 1.2, flatten: 0.1, offset: 40, caves: 0.9, float: false },
        { id: 'террасы', ridge: 0.6, flatten: 0.9, offset: -30, caves: 0.5, float: false },
        { id: 'скальные выходы', ridge: 1.5, flatten: 0.2, offset: -80, caves: 1.0, float: true },
    ],
    'вулканизм': [
        { id: 'лавовые потоки', ridge: 0.7, flatten: 0.6, offset: 30, caves: 0.7, float: false },
        { id: 'конусы', ridge: 1.8, flatten: 0.0, offset: -160, caves: 0.6, float: false },
        { id: 'кратеры', ridge: 0.6, flatten: 0.8, offset: 90, caves: 0.9, float: false },
        { id: 'обсидиановые стены', ridge: 1.4, flatten: 0.2, offset: -100, caves: 1.0, float: true },
        { id: 'фумаролы', ridge: 0.9, flatten: 0.5, offset: 0, caves: 0.8, float: false },
        { id: 'купола', ridge: 1.1, flatten: 0.4, offset: -40, caves: 0.6, float: false },
    ],
    'крио': [
        { id: 'ледники', ridge: 0.6, flatten: 0.8, offset: -50, caves: 0.6, float: false },
        { id: 'торосы', ridge: 1.5, flatten: 0.2, offset: 0, caves: 0.9, float: false },
        { id: 'трещины', ridge: 0.4, flatten: 0.9, offset: 120, caves: 1.0, float: false },
        { id: 'ледяные арки', ridge: 1.0, flatten: 0.3, offset: -20, caves: 1.1, float: true },
        { id: 'снежные дюны', ridge: 0.7, flatten: 0.7, offset: 40, caves: 0.4, float: false },
    ],
    'экзотика': [
        { id: 'кристаллические шпили', ridge: 1.7, flatten: 0.1, offset: -140, caves: 0.7, float: false },
        { id: 'грибные башни', ridge: 1.3, flatten: 0.3, offset: -60, caves: 0.6, float: true },
        { id: 'светящиеся жилы', ridge: 0.8, flatten: 0.6, offset: 0, caves: 1.0, float: false },
        { id: 'разломы', ridge: 0.5, flatten: 0.9, offset: 130, caves: 1.2, float: false },
        { id: 'плавучие камни', ridge: 1.0, flatten: 0.4, offset: -30, caves: 1.0, float: true },
    ],
};

// Погода (§7.2): визуально/звуком, без урона.
export const WEATHER = [
    { id: 'штиль', particles: 'none' },
    { id: 'туман', particles: 'fog' },
    { id: 'пыльная буря', particles: 'dust' },
    { id: 'метель', particles: 'snow' },
    { id: 'кислотный дождь', particles: 'rain' },
    { id: 'полярное сияние', particles: 'aurora' },
];

// Погода по категории — предпочтительный набор.
export const WEATHER_BY_CATEGORY = {
    'литосфера': ['штиль', 'пыльная буря', 'туман'],
    'вода': ['штиль', 'туман', 'кислотный дождь'],
    'биосфера': ['штиль', 'туман', 'кислотный дождь'],
    'вулканизм': ['туман', 'пыльная буря', 'штиль'],
    'крио': ['метель', 'туман', 'штиль'],
    'экзотика': ['полярное сияние', 'туман', 'штиль'],
};

// Палитра служебных цветов.
export const COLORS = {
    skyTop: '#05070f',
    skyBottom: '#101b2e',
    cave: '#050608',
    hud: '#e2e8f0',
    hpGood: '#4ade80',
    hpBad: '#ef4444',
    player: '#f8fafc',
    playerAccent: '#38bdf8',
};
