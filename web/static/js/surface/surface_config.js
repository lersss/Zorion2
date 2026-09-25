// web/static/js/surface/surface_config.js
// Константы клиента прогулки (спека 2026-09-21 §7.2/§7.4). Формации — клиентская
// константа по biome_category (В9): НЕ второй контент-реестр рядом с каталогом.

// Физика игрока (§7.4): ходьба/спринт/прыжок + клип гравитации 0.2–2.5.
export const PPM = 40;                 // пикселей в метре (визуальный масштаб, не физический)
// Рост игрока в мировых px (surface_player.js: this.h) — единица измерения
// размера корабля на высадке (решение создателя 2026-09-25: ship_scale в ростах
// человека). Единственный источник — не дублировать 28 в отрисовке.
export const PLAYER_H = 28;
// Ширина бокса игрока (surface_player.js: this.w) — для проверки непроходимости
// 2D-форм (crack2d не должен заглатывать бокс, §5.1 п.5).
export const PLAYER_W = 12;
export const WALK_SPEED = 4;           // м/с
export const SPRINT_SPEED = 7;         // м/с
export const JUMP_SPEED = 4.5;         // м/с
export const G_CLAMP_MIN = 0.2;
export const G_CLAMP_MAX = 2.5;
export const G_EARTH = 9.81;

// Мир (§7.2): чанк 256 px, в памяти ±3.
export const CHUNK = 256;
export const CHUNK_RADIUS = 3;

// Геометрия растра чанка (surface_render.js, §2.3): единый источник — skyTop
// (surface_world.js) ищет «поверхность неба» в пределах кадра, tools берут те же
// числа. Рост CHUNK_TOP_MARGIN 700→1200 (Э3, 2026-09-23) память НЕ меняет
// (меняется только topY): высота канваса = CHUNK_HEIGHT.
export const CHUNK_TOP_MARGIN = 1200;
export const CHUNK_HEIGHT = 1700;

// Общий визуальный масштаб отрисовки (идея 2026-09-22 §8.2): мир и игрок
// крупнее. Только отрисовка — трансформ камеры в surface_main.js; физика
// (PPM/скорости/гравитация/прыжок) не меняется.
export const ZOOM = 1.8;

// Корабль игрока на месте посадки (ЧК-ship, идея 2026-09-23 §5): статичная
// декорация у точки спавна — ПАРИТ над землёй (решение создателя 2026-09-23:
// «подвесить его… без лестницы подвесить точно проще»), с лёгким покачиванием.
// Размер спрайта — ship_scale корабля × PLAYER_H (решение создателя 2026-09-25);
// SHIP_DECOR_SIZE остаётся ФОЛБЭКОМ для пакета без ship_scale (старый клиент/
// пакет). SHIP_DECOR_X — смещение от спавна (0 = игрок ровно под кораблём: «меня
// только что сбросили»); SHIP_HOVER_BOTTOM — просвет под низом спрайта;
// покачивание — синус кадра (физику/ввод не трогает).
export const SHIP_DECOR_SIZE = 72;
export const SHIP_DECOR_X = 0;
export const SHIP_HOVER_BOTTOM = 126;
export const SHIP_BOB_AMP = 3;
export const SHIP_BOB_PERIOD_MS = 3200;

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
export const WEATHER_IDS = ['штиль', 'туман', 'пыльная буря', 'метель', 'кислотный дождь', 'полярное сияние', 'гроза', 'ледяные иглы', 'ледяной град', 'пепельный дождь', 'свечение'];

// Фазовые реперы — тройные точки (NIST Chemistry WebBook, спека 2026-09-22
// §3.4/§12 п.12): новый класс sourced-констант рядом с полосами suit. Только
// используемые. НЕ путать с температурой кипения/сублимации/критической —
// это разные величины (§12 п.4); у каждой подписи — вещество.
export const TRIPLE_POINT_K = {
    H2O: 273.16,   // вода: граница «твёрдая↔жидкая», гейт N1/N2/N3
    CH4: 90.68,    // метан: граница «метановых» вариантов N2/N3
    N2: 63.15,     // азот: нижняя граница физической полосы `extreme`
    CO2: 216.55,   // углекислый газ: граница «углекислых» вариантов
    NH3: 195.40,   // аммиак: граница «аммиачных»/криовулканических
};
// Единственный эмпирический (НЕ фазовый) порог: −40 °C, устойчивые кристаллы
// «алмазной пыли» (метеорология). Порог полосы `deep_frost` (§3.4). Помечен
// как эмпирика — не тройная точка.
export const DIAMOND_DUST_K = 233;
// Порог «стеклянно-кристаллической» пыльной бури (спека §4.3): каталожные биомы
// `стеклянные_поля`/`металлические_поля` t[480..600] (config/biome_catalog.json).
// Не тройная точка и не «на глаз» — берётся из каталога (PITFALLS «Дизайн и числа»).
export const GLASS_FIELD_K = 480;

// WEATHER_RULES (§4.2–4.4): жёсткие гейты (в коде — allowedWeathers, пороги из
// pkg.suit) + мягкие веса W = base × M_temp × M_rad × M_tox × M_liq × M_biome.
// Числа — калибровка (решение создателя 2026-09-22 №1), не эталон мира.
export const WEATHER_RULES = {
    base: {
        'штиль': 1.0, 'туман': 1.0, 'пыльная буря': 1.0,
        'метель': 1.0, 'кислотный дождь': 1.0, 'полярное сияние': 0.15,
        'гроза': 1.0, 'ледяные иглы': 1.0, 'ледяной град': 1.0,
        'пепельный дождь': 1.0, 'свечение': 1.0,
    },
    byTemp: {
        cold: {
            'штиль': 0.6, 'туман': 1.2, 'пыльная буря': 0.3,
            'метель': 2.0, 'кислотный дождь': 0.0, 'полярное сияние': 1.6,
            'гроза': 1.0, 'пепельный дождь': 0.6, 'свечение': 1.0,
        },
        comfort: {
            'штиль': 1.2, 'туман': 1.0, 'пыльная буря': 0.6,
            'метель': 0.05, 'кислотный дождь': 1.6, 'полярное сияние': 0.6,
            'гроза': 1.2, 'пепельный дождь': 1.0, 'свечение': 1.2,
        },
        hot: {
            'штиль': 0.5, 'туман': 0.8, 'пыльная буря': 2.0,
            'метель': 0.0, 'кислотный дождь': 0.7, 'полярное сияние': 0.3,
            'гроза': 1.6, 'пепельный дождь': 1.4, 'свечение': 1.0,
        },
    },
    // Физические полосы N2/N3 (тройные точки, без перекрытий, спека §4.2):
    // M_temp для «ледяных игл»/«ледяного града»; suit-полосы к ним НЕ применяются.
    byTempPhysical: {
        extreme: { 'ледяные иглы': 3.0, 'ледяной град': 1.4 },   // T < 63.15 (N₂)
        severe: { 'ледяные иглы': 2.6, 'ледяной град': 1.0 },    // 63.15 ≤ T < 90.68 (CH₄)
        deep_frost: { 'ледяные иглы': 1.8, 'ледяной град': 1.2 }, // 90.68 ≤ T < 233
        ice: { 'ледяные иглы': 1.2, 'ледяной град': 1.8 },       // 233 ≤ T < 273.16 (H₂O)
    },
    byToxic: { 'туман': 1.3, 'кислотный дождь': 1.2, 'гроза': 1.2, 'пепельный дождь': 1.4 },
    // M_liq: отдельные значения для «есть жидкость» / «нет жидкости». Для дождя
    // остаётся прежняя семантика (нет жидкости → 0.2), у града — 1.4 / 0.3 (§4.2 N3).
    byLiquidAbsent: { 'кислотный дождь': 0.2, 'ледяной град': 0.3 },
    byLiquidPresent: { 'ледяной град': 1.4 },
    byBiome: {
        'литосфера': { 'штиль': 1.2, 'туман': 0.8, 'пыльная буря': 1.4, 'метель': 0.6, 'кислотный дождь': 0.7, 'полярное сияние': 1.0, 'гроза': 1.2, 'ледяные иглы': 0.6, 'ледяной град': 0.8 },
        'вода': { 'штиль': 0.9, 'туман': 1.4, 'пыльная буря': 0.4, 'метель': 0.5, 'кислотный дождь': 1.3, 'полярное сияние': 0.9, 'гроза': 1.4, 'ледяные иглы': 1.2, 'ледяной град': 1.6, 'свечение': 1.2 },
        'биосфера': { 'штиль': 1.0, 'туман': 1.3, 'пыльная буря': 0.5, 'метель': 0.7, 'кислотный дождь': 1.4, 'полярное сияние': 0.9, 'гроза': 1.2, 'ледяные иглы': 0.8, 'ледяной град': 0.8, 'свечение': 1.8 },
        'вулканизм': { 'штиль': 0.8, 'туман': 1.2, 'пыльная буря': 1.5, 'метель': 0.4, 'кислотный дождь': 0.6, 'полярное сияние': 0.8, 'гроза': 1.8, 'ледяные иглы': 0.6, 'ледяной град': 0.6 },
        'крио': { 'штиль': 1.1, 'туман': 1.2, 'пыльная буря': 0.6, 'метель': 1.6, 'кислотный дождь': 0.8, 'полярное сияние': 1.2, 'гроза': 0.0, 'ледяные иглы': 1.8, 'ледяной град': 1.2 },
        'экзотика': { 'штиль': 1.0, 'туман': 1.1, 'пыльная буря': 1.0, 'метель': 0.8, 'кислотный дождь': 0.9, 'полярное сияние': 1.5, 'ледяные иглы': 1.2, 'ледяной град': 0.8 },
    },
    aurora: { radDivisor: 25, radMax: 5 },
    repeatGuard: true,
    crossfadeMs: [600, 1200],
    // Слой среды (спека 2026-09-22 §6.1): период суток (6–10 мин, детерминирован
    // от seed) и доли фаз. Пороги nightFactor — границы фаз на таймлайне u∈[0,1):
    // ночь [0, nightEnd), рассвет [nightEnd, dawnEnd), день [dawnEnd, duskStart),
    // закат [duskStart, 1). Визуал (палитры/светило/звёзды) — блок ENV ниже.
    env: {
        dayMs: [360000, 600000],
        phases: { night: 0.40, dawn: 0.125, day: 0.35, dusk: 0.125 },
        nightFactor: { nightEnd: 0.400, dawnEnd: 0.525, duskStart: 0.875 },
    },
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
        primitive: 'dot',
        air: { base: 1.15, blend: null, far: [0.08, 0.12], mid: [0.02, 0.04], near: [0, 0], frame: [0, 0], sky: [0.04, 0.08] },
        belts: [
            { parallax: 0.4, tile: 900, vTile: 1000, count: [8, 14], size: [1, 2], alpha: [0.25, 0.45], fall: [2, 4], shape: 'dot' },
        ],
        wind: { base: [2, 5], gust: { period: [8, 14], range: [0.8, 1.2] } },
        ground: null,
    },

    'туман': {
        label: 'Туман',
        primitive: ['clump', 'bank'],
        air: { base: 1.6, blend: ['sky', 0.5], far: [0.55, 0.70], mid: [0.30, 0.40], near: [0.08, 0.15], frame: [0.08, 0.25], sky: [0.10, 0.20] },
        belts: [
            { parallax: 0.4, tile: 1400, vTile: 1200, count: [10, 16], w: [120, 320], h: [30, 70], alpha: [0.10, 0.20], shape: 'clump' },
            { parallax: 0.7, tile: 1800, vTile: 1400, count: [6, 10], w: [160, 380], h: [40, 90], alpha: [0.14, 0.26], shape: 'clump' },
            { parallax: 1.0, tile: 2200, vTile: 1600, count: [3, 6], w: [200, 460], h: [60, 120], alpha: [0.18, 0.30], shape: 'clump' },
        ],
        wind: { base: [5, 14], gust: { period: [8, 14], range: [0.6, 1.5] } },
        ground: { mode: 'valley', offset: [-10, 20] },
        // Слоистая гряда на воде (§5.2): заменяет круглые клочья `clump`.
        bank: {
            w: [300, 700], h: [30, 90], topRag: [0.25, 0.60], alpha: [0.14, 0.30],
            drift: [5, 12], breathAmp: [0.05, 0.12], breathPeriod: [4, 9],
            offset: [-10, 20], watersOnly: true,
        },
        // Подписи вариантов (UI §5). Порядок приоритета — variantFor (§4.6).
        variantLabels: {
            'серный': 'Смог',
            'плотный CO₂': 'Густой туман',
            'морозный': 'Морозный туман',
            'марево': 'Марево',
            'аммиачно-метановый': 'Мутный туман',
        },
        variants: {
            // §4.6: плотный CO₂ → аммиачно-метановый → серный (toxic) →
            // морозный (cold) → марево (hot) → базовый (нет варианта).
            'плотный CO₂': { bankAll: true, bank: { h: [40, 100], alpha: [0.30, 0.45], pulseY: 4 } },
            'аммиачно-метановый': { air: { blend: ['#7f8f66', 0.5] }, bankAll: true, bank: { offset: [10, 25] } },
            'серный': { air: { base: 1.45, blend: ['#b6c86a', 0.5] }, alphaMul: 1.3, ground: { mode: 'valley', offset: [10, 40] } },
            'морозный': { air: { base: 1.7, blend: ['#d4e8ff', 0.5], frame: [0.10, 0.28] }, alphaMul: 1.3 },
            'марево': { air: { base: 1.3, blend: ['#d8cfa8', 0.3], frame: [0.06, 0.12] }, alphaMul: 0.5, horizon: true, jitter: 2 },
        },
    },

    'пыльная буря': {
        label: 'Пыльная буря',
        primitive: ['dot', 'streak'],
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
        // Плоская грань (вариант §4.3): у доли частиц — короткий луч-блеск.
        facet: { rayShare: 0.2, rayLen: [4, 9] },
        // §4.3/§4.6: стеклянно-кристаллическая — сухая форма (T ≥ GLASS_FIELD_K,
        // каталожные поля); иначе базовые песчинки. Поток (count/fall) не меняем.
        variantLabels: { 'стеклянно-кристаллическая': 'Стеклянная буря' },
        variants: {
            'стеклянно-кристаллическая': {
                particle: '#dfe8f0',
                belts: [
                    { shape: 'facet', alpha: [0.4, 0.7] },
                    { shape: 'facet', alpha: [0.4, 0.7] },
                    {},
                ],
            },
        },
    },

    'метель': {
        label: 'Метель',
        primitive: ['dot', 'flake'],
        air: { base: 1.7, blend: ['#eef4ff', 0.55], far: [0.30, 0.45], mid: [0.20, 0.30], near: [0.10, 0.18], frame: [0.20, 0.45], sky: [0.15, 0.30] },
        particle: '#f5faff',
        belts: [
            { parallax: 0.4, tile: 900, vTile: 1000, count: [120, 200], size: [1, 2], alpha: [0.50, 0.75], fall: [200, 350], shape: 'dot' },
            { parallax: 0.7, tile: 1300, vTile: 1400, count: [70, 110], size: [2, 3], alpha: [0.55, 0.80], fall: [350, 600], sway: 12, shape: 'dot' },
            { parallax: 1.0, tile: 1700, vTile: 1800, count: [20, 40], size: [3, 8], alpha: [0.60, 0.90], fall: [600, 900], sway: 12, shape: 'flake' },
        ],
        wind: { base: [60, 130], gust: { period: [3, 8], range: [0.6, 1.5] } },
        slant: { deg: [20, 35], from: 'vertical' },
        ground: { mode: 'snow', alpha: [0.20, 0.40] },
        // Снежинка ближнего пояса (§5.1): двухтоновый диск + лучи у крупных.
        flake: {
            coreRatio: 0.55,
            coreColor: '#ffffff',
            edgeColor: '#dbe7f7',
            arms: [5, 6],
            armLen: [0.45, 0.70],
            tilt: [20, 35],
        },
    },

    'кислотный дождь': {
        // Базовая подпись — нейтральная: `обычный` (fallback) самый частый,
        // «Кислотный дождь» — частный случай (UI §4). id остаётся контрактом.
        label: 'Дождь',
        variantLabels: { 'обычный': 'Дождь', 'кислотный': 'Кислотный дождь', 'радиоактивный': 'Радиоактивный дождь', 'инверсионный': 'Ливень' },
        primitive: ['pair', 'streak'],
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
        // Порядок §4.6: радиоактивный → инверсионный (p > suit-max) → кислотный
        // (toxic) → обычный (fallback). Цвет обычного — от liquid_medium (уже в
        // biome_color, §5.1 п.4); подписи — @uidesigner (UI §5).
        variants: {
            'обычный': { air: { blend: null }, particle: null },
            'кислотный': {},
            // §4.3: зелёное свечение струй/всплесков + аддитивный блик всплеска.
            'радиоактивный': { particle: '#9fe060', splashGlow: [0.06, 0.12], ground: { mode: 'splash', alpha: [0.36, 0.66] } },
            // §4.3: короткие тяжёлые струи (fall ×1.3, streak [18,30]), «потоп».
            'инверсионный': {
                belts: [ { fall: [650, 910] }, { fall: [715, 1040] }, { fall: [910, 1235], streak: [18, 30] } ],
                ground: { mode: 'splash', alpha: [0.39, 0.715] },
            },
        },
    },

    'полярное сияние': {
        label: 'Полярное сияние',
        primitive: 'aurora',
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
        // Вариант (§4.3): магнитная буря — шире/ниже, многоцветные полосы
        // [низ, середина #5f7cff, верх]; alpha штор × nightFactor (как у базы).
        variantLabels: { 'магнитная буря': 'Магнитная буря' },
        variants: {
            'магнитная буря': {
                aurora: {
                    curtains: [4, 6],
                    bandHeight: [0.45, 0.60],
                    palette: {
                        'крио': ['#3fe6c8', '#5f7cff', '#8a6cff'],
                        'экзотика': ['#b46bff', '#5f7cff', '#ff6ad0'],
                        'вода': ['#35dfd0', '#5f7cff', '#5f8cff'],
                        'биосфера': ['#5ff08a', '#5f7cff', '#9a7cff'],
                        'вулканизм': ['#9fe060', '#5f7cff', '#e0b060'],
                        'литосфера': ['#8ee06a', '#5f7cff', '#e0c860'],
                    },
                },
            },
        },
    },

    // ---- Новые 5 явлений (спека §4.2 N1–N5, направление @gdesigner §3.1–3.5).
    // Блоки lightning/glint/pellet/ash/glow — аналогичны aurora: диапазоны
    // разворачивает цикловой PRNG один раз на окно (surface_weather.js §7.2).
    // Поле `primitive` — контракт анти-дубля (T2), отрисовка — по shape/блокам.

    'гроза': {
        label: 'Гроза',
        variantLabels: { 'токсичная': 'Кислотная гроза', 'сухая': 'Сухая гроза' },
        primitive: 'lightning',
        air: { base: 1.10, blend: ['#39414f', 0.55], far: [0.30, 0.45], mid: [0.18, 0.28], near: [0.06, 0.12], frame: [0.10, 0.22], sky: [0.14, 0.26] },
        particle: '#c9d6e8',
        // Базовые пояса = ливневая (fallback-вариант). «сухая» снимает belts и ground.
        belts: [
            { parallax: 0.4, tile: 900, vTile: 900, count: [80, 150], size: [1, 2], alpha: [0.25, 0.45], fall: [500, 700], shape: 'pair' },
            { parallax: 0.7, tile: 1300, vTile: 1200, count: [60, 90], size: [2, 3], alpha: [0.30, 0.50], fall: [550, 800], shape: 'pair' },
            { parallax: 1.0, tile: 1700, vTile: 1500, count: [10, 25], size: [2, 3], alpha: [0.35, 0.55], fall: [700, 950], streak: [30, 50], shape: 'streak' },
        ],
        wind: { base: [80, 160], gust: { period: [4, 9], range: [0.6, 1.5] } },
        slant: { deg: [8, 18], from: 'vertical' },
        ground: { mode: 'splash', alpha: [0.30, 0.55] },
        lightning: {
            tile: 2200,
            parallax: [0.18, 0.28],
            cloud: { count: [6, 10], w: [420, 900], h: [50, 110], alpha: [0.26, 0.42], drift: [6, 12], color: '#242a36', blend: 0.55, offset: [-40, 6] },
            boltRate: [0.33, 1.0],
            boltBurst: [1, 3],
            segments: [4, 7],
            branchChance: 0.45,
            branchDepth: [1, 2],
            width: { core: 2, glow: 6 },
            color: '#eaf2ff',
            flashMs: [60, 120],
            flashAlpha: [0.10, 0.25],
            echoMs: [180, 320],
            echoAlpha: [0.04, 0.09],
            groundGlow: [0.06, 0.12],
        },
        variants: {
            'ливневая': {},
            'сухая': { belts: [], ground: null },
            'токсичная': { lightning: { color: '#c8f07a' } },
        },
    },

    'ледяные иглы': {
        label: 'Ледяные иглы',
        primitive: 'glint',
        air: { base: 1.15, blend: null, far: [0.08, 0.15], mid: [0.02, 0.04], near: [0, 0], frame: [0, 0], sky: [0.02, 0.05] },
        belts: [],
        wind: { base: [0, 2], gust: { period: [10, 16], range: [0.9, 1.1] } },
        ground: { mode: 'glint', alpha: [0.25, 0.55] },
        glint: {
            tile: 1400, vTile: 1100,
            airCount: [30, 80], airSize: [1, 3], airAlpha: [0.20, 0.55],
            lowCount: [20, 60], lowSize: [1, 4], lowAlpha: [0.30, 0.75],
            lowBand: [4, 26],
            pulseHz: [0.12, 0.45],
            pulseJitter: [0.35, 1.0],
            crossMin: 0.70,
            rayLen: [6, 14],
            haloRadius: [3, 7],
            haloAlpha: [0.05, 0.12],
            lowSunBoost: 1.7,
            palette: {
                'водяные': '#dff0ff',
                'углекислые': '#eaf6ff',
                'аммиачные': '#e9f7ee',
                'метановые': '#d8f0ea',
                'иней': '#e2e9f2',
            },
        },
        variants: {
            'водяные': { glint: { airCount: [30, 70], airSize: [1, 3] } },
            'углекислые': { glint: { airCount: [35, 80], airSize: [2, 4], rayLen: [8, 16] } },
            'аммиачные': { glint: { airCount: [30, 75], airSize: [1, 3] } },
            'метановые': { glint: { airCount: [30, 70], airSize: [1, 3] } },
            'иней': { glint: { airCount: [40, 80], airSize: [1, 2], lowCount: [30, 60] } },
        },
    },

    'ледяной град': {
        label: 'Ледяной град',
        // Вещество игроку не видно — все *_крупа дают «Ледяная крупа», «град» — «Град» (UI §5).
        variantLabels: { 'град': 'Град', 'крупа': 'Ледяная крупа', 'базовая крупа': 'Ледяная крупа', 'углекислая крупа': 'Ледяная крупа', 'метановая крупа': 'Ледяная крупа' },
        primitive: 'pellet',
        air: { base: 1.05, blend: ['#cfe0f2', 0.30], far: [0.25, 0.38], mid: [0.14, 0.22], near: [0.05, 0.10], frame: [0.08, 0.16], sky: [0.10, 0.18] },
        particle: null,
        belts: [
            { parallax: 0.4, tile: 900, vTile: 900, count: [60, 110], size: [1, 2], alpha: [0.55, 0.80], fall: [700, 1000], shape: 'pellet', bounce: false },
            { parallax: 0.7, tile: 1300, vTile: 1200, count: [25, 50], size: [2, 3], alpha: [0.70, 0.90], fall: [800, 1100], sway: 4, shape: 'pellet', bounce: true },
            { parallax: 1.0, tile: 1700, vTile: 1500, count: [8, 18], size: [3, 5], alpha: [0.80, 0.98], fall: [900, 1200], shape: 'pellet', bounce: true },
        ],
        wind: { base: [20, 60], gust: { period: [5, 10], range: [0.7, 1.3] } },
        slant: { deg: [0, 8], from: 'vertical' },
        ground: { mode: 'grain', alpha: [0.30, 0.55] },
        pellet: {
            colorCore: '#eaf4ff', colorRim: '#9fc4e8',
            rimAlpha: [0.25, 0.45],
            bounceH: [8, 18],
            bounceDamping: 0.35,
            bounceCount: [1, 2],
            grain: { count: [30, 70], size: [2, 5], alpha: [0.35, 0.60], step: 22 },
        },
        variants: {
            // «град»/«крупа» переопределяют пояса ПО ИНДЕКСУ (0.4 → 0.7 → 1.0);
            // отсутствующие поля пояса берутся из базового.
            'град': { pellet: { colorCore: '#eaf4ff', colorRim: '#9fc4e8', bounceH: [12, 24], bounceCount: [1, 3] }, belts: [{ count: [40, 80] }, { count: [15, 30], size: [3, 5] }, { count: [6, 14], size: [4, 6] }] },
            'крупа': { pellet: { colorCore: '#e4eefb', colorRim: '#a9bdd4', bounceH: [5, 10], bounceCount: [1, 1] }, belts: [{ count: [80, 140] }, { count: [40, 70], size: [2, 3] }, { count: [12, 24], size: [3, 4] }] },
            'углекислая крупа': { pellet: { colorCore: '#e8f2ff', colorRim: '#a9c2e0' } },
            'метановая крупа': { pellet: { colorCore: '#e2f6f0', colorRim: '#9fd0c4' } },
            'базовая крупа': { pellet: { colorCore: '#dfe6ee', colorRim: '#9aa7b8' } },
        },
    },

    'пепельный дождь': {
        label: 'Пепельный дождь',
        variantLabels: { 'сухой': 'Пепелопад', 'криовулканический': 'Пепелопад' },
        primitive: 'ash',
        air: { base: 1.0, blend: ['#4a423c', 0.75], far: [0.35, 0.55], mid: [0.25, 0.40], near: [0.10, 0.20], frame: [0.20, 0.40], sky: [0.25, 0.45] },
        particle: null,
        belts: [
            { parallax: 0.4, tile: 900, vTile: 900, count: [120, 200], size: [1, 2], alpha: [0.30, 0.55], fall: [150, 240], shape: 'ash' },
            { parallax: 0.7, tile: 1300, vTile: 1200, count: [70, 120], size: [1, 3], alpha: [0.35, 0.60], fall: [200, 320], sway: 10, shape: 'ash' },
            { parallax: 1.0, tile: 1700, vTile: 1500, count: [20, 40], size: [2, 4], alpha: [0.40, 0.70], fall: [280, 400], sway: 14, shape: 'ash' },
        ],
        wind: { base: [30, 90], gust: { period: [4, 9], range: [0.6, 1.4] } },
        slant: { deg: [10, 25], from: 'vertical' },
        ground: { mode: 'ash', alpha: [0.35, 0.60] },
        ash: {
            shade: 0.45,
            tint: '#3c3833',
            flakeJitter: [0.30, 0.70],
            flakeLen: [1.0, 1.8],
            groundBand: { h: [3, 9], step: 18, alpha: [0.35, 0.60] },
            wetStreaks: false,
        },
        variants: {
            'влажный': { ash: { wetStreaks: true, groundBand: { h: [4, 12], step: 18 } } },
            'криовулканический': { ash: { shade: 0.75, tint: '#cfd8e2', flakeLen: [1.0, 1.4] } },
            'сухой': {},
        },
    },

    'свечение': {
        label: 'Свечение',
        variantLabels: { 'светящийся туман': 'Светящийся туман', 'светящийся ливень': 'Светящийся ливень', 'биоаэрозоль': 'Светящиеся споры' },
        primitive: 'glow',
        air: { base: 1.0, blend: null, far: [0.10, 0.16], mid: [0.04, 0.08], near: [0, 0], frame: [0.03, 0.07], sky: [0.02, 0.05] },
        belts: [],
        wind: { base: [3, 9], gust: { period: [9, 16], range: [0.8, 1.2] } },
        ground: null,
        glow: {
            tile: 1600, vTile: 1200,
            count: [40, 120],
            size: [2, 6],
            haloSize: [8, 20],
            alphaBody: [0.18, 0.42],
            alphaHalo: [0.05, 0.12],
            drift: [3, 9],
            rise: [-6, 4],
            pulseHz: [0.20, 0.60],
            palette: { 'биосфера': ['#6ef0a0', '#8f7cff'], 'вода': ['#4fe0d0', '#5f8cff'] },
            airTint: [0.04, 0.09],
            nightBoost: [0.4, 0.6],
        },
        variants: {
            'светящийся туман': { glow: { count: [60, 120], haloSize: [12, 24], alphaHalo: [0.07, 0.16] } },
            // «Светящийся ливень»: дождевые пояса/всплеск (числа — как у «кислотного дождя»).
            'светящийся ливень': { belts: [
                { parallax: 0.4, tile: 900, vTile: 900, count: [80, 150], size: [1, 2], alpha: [0.25, 0.45], fall: [500, 700], shape: 'pair' },
                { parallax: 0.7, tile: 1300, vTile: 1200, count: [60, 90], size: [2, 3], alpha: [0.30, 0.50], fall: [550, 800], shape: 'pair' },
                { parallax: 1.0, tile: 1700, vTile: 1500, count: [10, 25], size: [2, 3], alpha: [0.35, 0.55], fall: [700, 950], streak: [30, 50], shape: 'streak' },
            ], ground: { mode: 'splash', alpha: [0.30, 0.55] } },
            'биоаэрозоль': { glow: { count: [40, 100], rise: [-8, 2], drift: [4, 10] } },
        },
    },
};

// ENV (спека 2026-09-22 §6.1 + направление @gdesigner §6.1): слой среды — сутки.
// Палитра неба интерполируется по u (не по nightFactor) между 4 опорами; `ночь`
// совпадает с сегодняшними COLORS.skyTop/skyBottom (пакет 1 не «поехал»).
// Характер дня — вариант D (решения создателя 2026-09-22): база по полосам
// давления + подмес sky.star.color на `sunTint`; ночной тинт — N2 (starMix 0.12).
export const ENV = {
    // Таймлайн палитры: [u, имя палитры]; между точками — линейная интерполяция по u.
    skyKeys: [
        [0.0000, 'ночь'], [0.4000, 'ночь'],
        [0.4625, 'рассвет'], [0.5250, 'день'],
        [0.8750, 'день'], [0.9375, 'закат'], [1.0000, 'ночь'],
    ],
    palettes: {
        'день':    { top: '#1b3a63', bottom: '#6d97bd' },
        'рассвет': { top: '#1a2749', bottom: '#b9787f' },
        'закат':   { top: '#2b2350', bottom: '#c9683f' },
        'ночь':    { top: '#05070f', bottom: '#101b2e' },
    },
    // Вариант D (§7.1 направления): дневная палитра — база по полосам давления
    // (`suit.pressure_comfort_atm`), затем подмес star.color на sunTint.
    // comfort-строка совпадает с palettes['день'].
    dayByPressure: {
        thin:    { top: '#02030a', bottom: '#0a1122' },
        comfort: { top: '#1b3a63', bottom: '#6d97bd' },
        dense:   { top: '#3a4a5e', bottom: '#98a8b4' },
    },
    sunTint: 0.25,

    // Свет на мир (§6.3 п.3 спеки): тинт ниже горизонта, alpha = 1 − lightMul^w
    // (wMid — добавка дальнему плану). Цвет — worldTint.color с подмесом star.color.
    worldTint: { color: '#050912', starMix: 0.12, wMid: 0.40 },
    lightMul: { night: 0.25, day: 1.0 },          // 0.25 + 0.75·(1 − nightFactor)

    sun: {
        arcStart: 0.400, arcSpan: 0.600,          // окно дуги u∈[0.400, 1.000]
        xFrac: [0.08, 0.92],                      // положение по X, доля vw
        yHorizon: 0.62, yZenith: 0.16,            // высота, доля vh
        altPow: 0.6,                              // alt = sin(π·p)^altPow
        radius: [0.035, 0.050],                   // доля vh
        haloMul: 1.9, haloAlpha: 0.16,
        parallax: 0.05,
    },

    stars: {
        tile: 900, vTile: 1000, count: [160, 260],
        size: [1, 2.4], alpha: [0.25, 0.85],
        parallax: [0.02, 0.06],
        twinkleHz: [0.08, 0.40], twinkleAmp: [0.10, 0.35],
        cool: '#dfe8ff', warm: '#ffe9c4', warmShare: 0.20,
        rampLo: 0.5, rampHi: 1.0,
        hazeCut: 0.30,
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
