// web/static/js/map/starfield.js
// Фон «звёздное небо» на близких зумах (спека 30c.1).
// Генерирует offscreen-тайл один раз (init + resize); рисует один fillRect за кадр.

const TILE_SIZE   = 256;
const TILE_COUNT  = 60;
const FADE_IN     = 1.0;
const FULL_ALPHA  = 1.5;
const PARALLAX    = 0.06;
const PARALLAX_DIM = PARALLAX * 0.1; // дальний слой — в 10 раз медленнее
const BASE_COLOR  = [200, 214, 229]; // #c8d6e5

let tileCanvas = null;     // offscreen canvas с тайлом (ближний слой)
let tileDimCanvas = null;  // offscreen canvas с тайлом (дальний, тусклый)
let tilePattern = null;    // CanvasPattern ближнего слоя (лениво)
let tileDimPattern = null; // CanvasPattern дальнего слоя (лениво)

// Накопленное экранное смещение фона (px). Фон двигается ТОЛЬКО при
// панорамировании (scale не менялся между кадрами); при зуме — стоит
// (дальние звёзды не должны ехать от изменения масштаба).
let bgX = 0, bgY = 0;
let prevOffsetX = 0, prevOffsetY = 0, prevScale = -1;

// mulberry32 — детерминированный seeded PRNG (seed фиксирован → тайл одинаков).
function mulberry32(seed) {
    return function () {
        seed |= 0;
        seed = (seed + 0x6D2B79F5) | 0;
        let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

// initStarfield() — генерирует тайлы. Вызывается при загрузке и при resize.
export function initStarfield() {
    tileCanvas = makeTile(0.15, 0.2);   // ближний слой: alpha 0.15..0.35
    tileDimCanvas = makeTile(0.05, 0.07); // дальний слой: совсем тусклый 0.05..0.12

    // TilePattern создаётся ПОСЛЕ resize canvas (чтобы размер совпадал)
    tilePattern = null;    // сброс — будет создан при первом drawStarfield
    tileDimPattern = null;
}

// makeTile(alphaMin, alphaRange) — один offscreen-тайл с точками.
// Детерминировано: seeded random от фиксированного seed (тайл одинаков).
function makeTile(alphaMin, alphaRange) {
    const c = document.createElement('canvas');
    c.width  = TILE_SIZE;
    c.height = TILE_SIZE;
    const tctx = c.getContext('2d');

    // Чёрный фон тайла — совпадает с CSS canvas (#0a0f1a)
    tctx.fillStyle = '#0a0f1a';
    tctx.fillRect(0, 0, TILE_SIZE, TILE_SIZE);

    const rng = mulberry32(42);
    for (let i = 0; i < TILE_COUNT; i++) {
        const x = rng() * TILE_SIZE;
        const y = rng() * TILE_SIZE;
        const r = 1.0 + rng();                 // 1.0..2.0 px
        const a = alphaMin + rng() * alphaRange;

        tctx.globalAlpha = a;
        tctx.fillStyle = `rgb(${BASE_COLOR[0]},${BASE_COLOR[1]},${BASE_COLOR[2]})`;
        tctx.beginPath();
        tctx.arc(x, y, r, 0, Math.PI * 2);
        tctx.fill();
    }
    tctx.globalAlpha = 1;
    return c;
}

// drawStarfield(ctx, w, h, scale, offsetX, offsetY, timeMs)
// Рисует фон. Вызывается из draw() ПОСЛЕ clearRect, ДО регионов.
export function drawStarfield(ctx, w, h, scale, offsetX, offsetY, timeMs) {
    // 1. Альфа-рамп
    if (scale < FADE_IN) return;
    const rampAlpha = Math.min(1, (scale - FADE_IN) / (FULL_ALPHA - FADE_IN));

    // 2. Паттерны (ленивое создание)
    if (!tilePattern) {
        tilePattern = ctx.createPattern(tileCanvas, 'repeat');
        tileDimPattern = ctx.createPattern(tileDimCanvas, 'repeat');
    }

    // 3. Накопление смещения только при панорамировании (scale не менялся).
    //    Зум меняет scale — фон в этом кадре не двигаем (дальние звёзды стоят).
    if (prevScale !== scale) {
        prevScale = scale;
        prevOffsetX = offsetX;
        prevOffsetY = offsetY;
        // Перепривязка фона при смене масштаба (запрос создателя «параллакс то
        // работает, то нет»): при зуме аргумент offset скачет (zoom-пересчёт
        // под курсор, в модалке ещё и ×4 + followOffset) — bgX/bgY НЕ
        // компенсировались, разъезд накапливался от зума к зуму → фон
        // «отвязывался» от мировой системы. Сброс накопления: фон
        // перепривязывается к текущему виду (bgX=0), при пане/слежении
        // накапливает от этой точки — согласованно. Для повторяющегося тайла
        // перепривязка визуально незаметна (карта: фон при зуме и так стоит).
        bgX = 0;
        bgY = 0;
    } else {
        // Накопление ПО МОДУЛЮ ТАЙЛА (запрос создателя «фон замирает»): bgX/bgY
        // никогда не растут неограниченно — при огромных offset (слежение камеры
        // в модалке) разница (offsetX - prevOffsetX) теряла точность float
        // (прирост 0) → фон замирал. Модуль 256: тайл повторяющийся, оба знака
        // корректны (отрицательный остаток эквивалентен положительному + 256).
        // Поведение карты не меняется (нормальные offset — тот же результат).
        bgX = ((bgX + (offsetX - prevOffsetX) * PARALLAX) % TILE_SIZE + TILE_SIZE) % TILE_SIZE;
        bgY = ((bgY + (offsetY - prevOffsetY) * PARALLAX) % TILE_SIZE + TILE_SIZE) % TILE_SIZE;
        prevOffsetX = offsetX;
        prevOffsetY = offsetY;
    }

    // 4. Микро-мерцание (среднее по всем точкам — один sin на кадр)
    const shimmer = 0.85 + 0.15 * Math.sin(timeMs * 0.001);

    // 5. Два слоя параллакса: дальний (тусклый, в 10 раз медленнее) под ближним.
    drawLayer(ctx, w, h, tileDimPattern, bgX * 0.1, bgY * 0.1, rampAlpha * shimmer);
    drawLayer(ctx, w, h, tilePattern,    bgX,       bgY,       rampAlpha * shimmer);
}

// drawLayer — один fillRect с паттерном, сдвинутым на (bgX, bgY).
function drawLayer(ctx, w, h, pattern, bgX, bgY, alpha) {
    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.translate(bgX % TILE_SIZE, bgY % TILE_SIZE);
    ctx.fillStyle = pattern;
    ctx.fillRect(
        -(bgX % TILE_SIZE) - TILE_SIZE,
        -(bgY % TILE_SIZE) - TILE_SIZE,
        w + TILE_SIZE * 2,
        h + TILE_SIZE * 2
    );
    ctx.restore();
}
