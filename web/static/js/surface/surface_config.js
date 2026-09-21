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

// Общий визуальный масштаб отрисовки (идея 2026-09-22 §8.2): мир и игрок
// крупнее. Только отрисовка — трансформ камеры в surface_main.js; физика
// (PPM/скорости/гравитация/прыжок) не меняется.
export const ZOOM = 1.8;

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

// Погода (спека 2026-09-22 §7): визуально, без урона. Явление выбирается из
// физики планеты (§4), визуал — в surface_weather.js. WEATHER_BY_CATEGORY
// удалён: биом больше не задаёт набор (спека §7.1). id явлений — контракт
// админских чипов и HUD, не переименовывать.
export const WEATHER_IDS = ['штиль', 'туман', 'пыльная буря', 'метель', 'кислотный дождь', 'полярное сияние'];

// WEATHER_RULES (§4.2–4.4): жёсткие гейты (в коде — allowedWeathers, пороги из
// pkg.suit) + мягкие веса W = base × M_temp × M_rad × M_tox × M_liq × M_biome.
// Числа — калибровка (решение создателя 2026-09-22 №1), не эталон мира.
export const WEATHER_RULES = {
    base: {
        'штиль': 1.0, 'туман': 1.0, 'пыльная буря': 1.0,
        'метель': 1.0, 'кислотный дождь': 1.0, 'полярное сияние': 0.15,
    },
    byTemp: {
        cold: {
            'штиль': 0.6, 'туман': 1.2, 'пыльная буря': 0.3,
            'метель': 2.0, 'кислотный дождь': 0.0, 'полярное сияние': 1.6,
        },
        comfort: {
            'штиль': 1.2, 'туман': 1.0, 'пыльная буря': 0.6,
            'метель': 0.05, 'кислотный дождь': 1.6, 'полярное сияние': 0.6,
        },
        hot: {
            'штиль': 0.5, 'туман': 0.8, 'пыльная буря': 2.0,
            'метель': 0.0, 'кислотный дождь': 0.7, 'полярное сияние': 0.3,
        },
    },
    byToxic: { 'туман': 1.3, 'кислотный дождь': 1.2 },
    byLiquidAbsent: { 'кислотный дождь': 0.2 },
    byBiome: {
        'литосфера': { 'штиль': 1.2, 'туман': 0.8, 'пыльная буря': 1.4, 'метель': 0.6, 'кислотный дождь': 0.7, 'полярное сияние': 1.0 },
        'вода': { 'штиль': 0.9, 'туман': 1.4, 'пыльная буря': 0.4, 'метель': 0.5, 'кислотный дождь': 1.3, 'полярное сияние': 0.9 },
        'биосфера': { 'штиль': 1.0, 'туман': 1.3, 'пыльная буря': 0.5, 'метель': 0.7, 'кислотный дождь': 1.4, 'полярное сияние': 0.9 },
        'вулканизм': { 'штиль': 0.8, 'туман': 1.2, 'пыльная буря': 1.5, 'метель': 0.4, 'кислотный дождь': 0.6, 'полярное сияние': 0.8 },
        'крио': { 'штиль': 1.1, 'туман': 1.2, 'пыльная буря': 0.6, 'метель': 1.6, 'кислотный дождь': 0.8, 'полярное сияние': 1.2 },
        'экзотика': { 'штиль': 1.0, 'туман': 1.1, 'пыльная буря': 1.0, 'метель': 0.8, 'кислотный дождь': 0.9, 'полярное сияние': 1.5 },
    },
    aurora: { radDivisor: 25, radMax: 5 },
    repeatGuard: true,
    crossfadeMs: [600, 1200],
};

// WEATHER_VISUALS (§5): база отрисовки 6 явлений. `air.blend = [target, k]` —
// сдвиг цвета воздуха (target: 'sky' | '#hex'); far/mid/near/frame/sky — доли
// дымки (§5.1 п.1). `belts` — пояса частиц (§5.1 п.2–3): tile/vTile — мировые
// тайлы, parallax — пояс, shape — 'dot'|'streak'|'pair'|'clump'. Диапазоны
// [lo, hi] разрешает цикловой PRNG один раз на окно (§7.2). ground — приземные
// эффекты по terrainHeight (§5.1 п.6). Цвет — производный от biome_color
// (server already appended liquid shift, §5.1 п.4 — повторный сдвиг запрещён).
export const WEATHER_VISUALS = {
    'штиль': {
        label: 'Штиль',
        air: { base: 1.15, blend: null, far: [0.08, 0.12], mid: [0.02, 0.04], near: [0, 0], frame: [0, 0], sky: [0.04, 0.08] },
        belts: [
            { parallax: 0.4, tile: 900, vTile: 1000, count: [8, 14], size: [1, 2], alpha: [0.25, 0.45], fall: [2, 4], shape: 'dot' },
        ],
        wind: { base: [2, 5], gust: { period: [8, 14], range: [0.8, 1.2] } },
        ground: null,
    },

    'туман': {
        label: 'Туман',
        air: { base: 1.6, blend: ['sky', 0.5], far: [0.55, 0.70], mid: [0.30, 0.40], near: [0.08, 0.15], frame: [0.08, 0.25], sky: [0.10, 0.20] },
        belts: [
            { parallax: 0.4, tile: 1400, vTile: 1200, count: [10, 16], w: [120, 320], h: [30, 70], alpha: [0.10, 0.20], shape: 'clump' },
            { parallax: 0.7, tile: 1800, vTile: 1400, count: [6, 10], w: [160, 380], h: [40, 90], alpha: [0.14, 0.26], shape: 'clump' },
            { parallax: 1.0, tile: 2200, vTile: 1600, count: [3, 6], w: [200, 460], h: [60, 120], alpha: [0.18, 0.30], shape: 'clump' },
        ],
        wind: { base: [5, 14], gust: { period: [8, 14], range: [0.6, 1.5] } },
        ground: { mode: 'valley', offset: [-10, 20] },
        variants: {
            cold: { air: { base: 1.7, blend: ['#d4e8ff', 0.5], frame: [0.10, 0.28] }, alphaMul: 1.3 },
            toxic: { air: { base: 1.45, blend: ['#b6c86a', 0.5] }, alphaMul: 1.3, ground: { mode: 'valley', offset: [10, 40] } },
            hot: { air: { base: 1.3, blend: ['#d8cfa8', 0.3], frame: [0.06, 0.12] }, alphaMul: 0.5, horizon: true, jitter: 2 },
        },
    },

    'пыльная буря': {
        label: 'Пыльная буря',
        air: { base: 1.25, blend: ['#c8a05a', 0.7], far: [0.35, 0.55], mid: [0.25, 0.40], near: [0.10, 0.20], frame: [0.25, 0.50], sky: [0.30, 0.50] },
        particle: '#c8a078',
        belts: [
            { parallax: 0.4, tile: 900, vTile: 800, count: [100, 180], size: [1, 2], alpha: [0.30, 0.50], fall: [20, 50], shape: 'dot' },
            { parallax: 0.7, tile: 1300, vTile: 1100, count: [60, 100], size: [2, 3], alpha: [0.35, 0.55], fall: [30, 70], sway: 10, shape: 'dot' },
            { parallax: 1.0, tile: 1700, vTile: 1400, count: [15, 30], size: [3, 6], alpha: [0.45, 0.70], fall: [50, 100], streak: [8, 20], shape: 'streak' },
        ],
        wind: { base: [250, 450], gust: { period: [3, 7], range: [0.6, 1.5] } },
        slant: { deg: [5, 15], from: 'horizontal' },
        ground: { mode: 'skirt', alpha: [0.25, 0.45] },
    },

    'метель': {
        label: 'Метель',
        air: { base: 1.7, blend: ['#eef4ff', 0.55], far: [0.30, 0.45], mid: [0.20, 0.30], near: [0.10, 0.18], frame: [0.20, 0.45], sky: [0.15, 0.30] },
        particle: '#f5faff',
        belts: [
            { parallax: 0.4, tile: 900, vTile: 1000, count: [120, 200], size: [1, 2], alpha: [0.50, 0.75], fall: [200, 350], shape: 'dot' },
            { parallax: 0.7, tile: 1300, vTile: 1400, count: [70, 110], size: [2, 3], alpha: [0.55, 0.80], fall: [350, 600], sway: 12, shape: 'dot' },
            { parallax: 1.0, tile: 1700, vTile: 1800, count: [20, 40], size: [4, 7], alpha: [0.60, 0.90], fall: [600, 900], streak: [10, 25], shape: 'streak' },
        ],
        wind: { base: [60, 130], gust: { period: [3, 8], range: [0.6, 1.5] } },
        slant: { deg: [20, 35], from: 'vertical' },
        ground: { mode: 'snow', alpha: [0.20, 0.40] },
    },

    'кислотный дождь': {
        label: 'Кислотный дождь',
        air: { base: 1.1, blend: ['#b4c850', 0.45], far: [0.25, 0.40], mid: [0.10, 0.20], near: [0.04, 0.08], frame: [0.08, 0.18], sky: [0.08, 0.16] },
        particle: '#a5c83c',
        belts: [
            { parallax: 0.4, tile: 900, vTile: 900, count: [80, 150], size: [1, 2], alpha: [0.25, 0.45], fall: [500, 700], shape: 'pair' },
            { parallax: 0.7, tile: 1300, vTile: 1200, count: [60, 90], size: [2, 3], alpha: [0.30, 0.50], fall: [550, 800], shape: 'pair' },
            { parallax: 1.0, tile: 1700, vTile: 1500, count: [10, 25], size: [2, 3], alpha: [0.35, 0.55], fall: [700, 950], streak: [30, 50], shape: 'streak' },
        ],
        wind: { base: [120, 200], gust: { period: [4, 9], range: [0.6, 1.5] } },
        slant: { deg: [10, 20], from: 'vertical' },
        ground: { mode: 'splash', alpha: [0.30, 0.55] },
        // Вариант определяет `toxic`: обычный дождь — цвет от liquid_medium
        // (уже в biome_color, §5.1 п.4), подпись — @uidesigner (§4.6).
        variants: {
            normal: { air: { blend: null }, particle: null },
            toxic: {},
        },
    },

    'полярное сияние': {
        label: 'Полярное сияние',
        air: { base: 1.0, blend: null, far: [0.08, 0.15], mid: [0, 0], near: [0, 0], frame: [0, 0], sky: [0, 0] },
        belts: [],
        wind: { base: [0, 0], gust: { period: [10, 14], range: [0.9, 1.1] } },
        ground: null,
        // Сияние (§5.7 спеки + направление @gdesigner 2026-09-22): широкие мягкие
        // занавесы, а не «частокол». Палитра [низ, верх] по biome_category —
        // источник истины цвета (запекается в спрайт шторы). widthFrac — ширина
        // занавеса долей экрана (§2.1), добавлено для техники.
        aurora: {
            tile: 2600,
            parallax: [0.05, 0.12],
            curtains: [2, 4],
            strands: [30, 80],
            period: [2, 5],
            widthFrac: [0.40, 0.80],
            bandHeight: [0.30, 0.45],
            strandWidth: [6, 22],
            strandLen: [0.40, 1.00],
            bottomWave: { len: [380, 820], amp: [16, 42] },
            bottomJitter: [0, 26],
            sway: [3, 8],
            warp: [4, 12],
            driftPeriod: [9, 18],
            driftAmp: [20, 60],
            taper: [0.10, 0.20],
            alpha: [0.12, 0.26],
            haloAlpha: [0.04, 0.10],
            reflection: [0.04, 0.08],
            palette: {
                'крио': ['#3fe6c8', '#8a6cff'],
                'экзотика': ['#b46bff', '#ff6ad0'],
                'вода': ['#35dfd0', '#5f7cff'],
                'биосфера': ['#5ff08a', '#9a7cff'],
                'вулканизм': ['#9fe060', '#e0b060'],
                'литосфера': ['#8ee06a', '#e0c860'],
            },
        },
    },
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
