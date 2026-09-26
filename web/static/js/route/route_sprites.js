// web/static/js/route/route_sprites.js
// Выбор вариантов спрайтов из пулов и прелоад ТОЛЬКО выбранных (§3.7/§5
// визуального ТЗ): режим наигрыша — случайный вариант на запуск, заморозка —
// фиксированный. Ошибка загрузки → null, рисующий код берёт кодовую фигуру
// (инвариант §5: сцена не падает без ассетов).
import * as C from './route_config.js';

const cache = new Map();
let chosen = {};

// pickVariant — замороженное имя, если задано и есть в пуле, иначе случайный.
function pickVariant(type) {
    const pool = C.POOLS[type] || [];
    if (!pool.length) return null;
    const frozen = C.SPRITE_FREEZE[type];
    if (frozen && pool.indexOf(frozen) >= 0) return frozen;
    return pool[Math.floor(Math.random() * pool.length)];
}

// pickSprites — вариант на каждый тип; Цель берёт пул по типу звезды
// (black_hole → black_hole_*, иначе star_core_*), поэтому прелоад ≤6 PNG.
export function pickSprites(finishStarType) {
    chosen = {};
    for (const type in C.POOLS) {
        if (type === 'star_core' || type === 'black_hole') continue;
        chosen[type] = pickVariant(type);
    }
    const finishType = finishStarType === 'black_hole' ? 'black_hole' : 'star_core';
    chosen[finishType] = pickVariant(finishType);
    return chosen;
}

// getSprite — загруженный Image или null (файл отсутствует/ошибка загрузки).
export function getSprite(name) {
    if (!name) return null;
    const img = cache.get(name);
    if (!img || !img.complete || img.naturalWidth === 0) return null;
    return img;
}

export function chosenSprites() {
    return chosen;
}

// preloadSprites — прелоад выбранных вариантов до первого кадра. Резолвится,
// когда все либо загружены, либо провалились (ошибка → фолбэк-фигура).
export function preloadSprites(finishStarType) {
    pickSprites(finishStarType);
    const names = [];
    for (const type in chosen) {
        if (chosen[type]) names.push(chosen[type]);
    }
    const jobs = names.map((name) => new Promise((resolve) => {
        if (cache.has(name)) { resolve(); return; }
        const img = new Image();
        img.onload = () => resolve();
        img.onerror = () => resolve();
        img.src = C.SPRITE_BASE + name + '.png';
        cache.set(name, img);
    }));
    return Promise.all(jobs);
}

// tintedSprite — grayscale-спрайт, окрашенный кодом (multiply по яркости,
// затем destination-in — возврат альфы). Кэш по «src|цвет».
const tintCache = new Map();
export function tintedSprite(img, color) {
    const key = img.src + '|' + color;
    let c = tintCache.get(key);
    if (c) return c;
    c = document.createElement('canvas');
    c.width = img.naturalWidth;
    c.height = img.naturalHeight;
    const x = c.getContext('2d');
    x.drawImage(img, 0, 0);
    x.globalCompositeOperation = 'multiply';
    x.fillStyle = color;
    x.fillRect(0, 0, c.width, c.height);
    x.globalCompositeOperation = 'destination-in';
    x.drawImage(img, 0, 0);
    tintCache.set(key, c);
    return c;
}

// bloomSprite — bloom двойной отрисовкой (alpha ~0.25, scale 1.4), без
// полноэкранного размытия (мобильный бюджет §3.6).
export function bloomSprite(ctx, img, x, y, size, alpha) {
    if (alpha > 0) {
        ctx.globalAlpha = alpha;
        const s = size * 1.4;
        ctx.drawImage(img, x - s / 2, y - s / 2, s, s);
    }
    ctx.globalAlpha = alpha > 0 ? 1 : 0.95;
    ctx.drawImage(img, x - size / 2, y - size / 2, size, size);
}
