// web/static/js/map/ship_sprites.js
// Статичные спрайты кораблей (спека 61b §5.6): замена старого генератора
// схем кораблей. Игрок — цельный PNG из web/static/sprites/ (имя из /me,
// уже смаппленное), перекраска — тонирование gCO='hue' + ОБЯЗАТЕЛЬНОЕ
// восстановление альфы 'destination-in' (иначе фон красится в цвет, §5.3).
// Агенты — спрайт + цвет от spriteForAgent(id) (FNV-1a % 21 / % 9),
// перекрашиваются ИЗ КЭША (прелоад 21×9, уточнение 2026-09-16) — ноль
// ленивых перекрасок в рантайме. Фолбэк (И4): спрайт не загрузился / имя
// неизвестно → null, рисующий код использует примитив (полёт) / ромб
// (агент) — без падений.

// redrawCallback — колбэк перерисовки после асинхронной загрузки спрайта.
// Карта ставит draw (main.js), дашборд — renderShipGrid (спека 61b §6.1):
// модуль не импортирует map_render.js, чтобы быть загружаемым на дашборде
// (там нет canvas карты).
let redrawCallback = null;

// setRedrawCallback — установка колбэка перерисовки (карта/дашборд).
export function setRedrawCallback(cb) {
    redrawCallback = cb;
}

// getRedrawCallback — текущий колбэк перерисовки. Модалка сохраняет его при
// открытии (ставит свой) и восстанавливает при закрытии, чтобы не сломать
// карту (запрос создателя 99.2.27: спрайт игрока догрузился после открытия
// модалки — перерисовывается модалка, а не карта).
export function getRedrawCallback() {
    return redrawCallback;
}

// shipFiles — порядок реестра ShipSprites с сервера (/me.ship_options,
// спека §3.2: порядок фиксирован — он же источник индексов spriteForAgent).
// Клиент НЕ дублирует список имён (И1): берёт его из /me.
let shipFiles = [];

// setShipOptions — кладёт порядок реестра спрайтов из /me.ship_options
// (массив {id, name, file}). Вызывается из data.js после загрузки /me.
export function setShipOptions(options) {
    shipFiles = Array.isArray(options) ? options.map(o => o.file) : [];
}

// ==================== КЭШ ИЗОБРАЖЕНИЙ ====================

const imageCache = new Map(); // name -> Image

// getShipImage — кэш 21 Image; загрузка /static/sprites/{name}.png.
// onload → scheduleRedraw (первый кадр мог уйти с фолбэком). null — имя
// неизвестно/пусто: рисующий код использует фолбэк (И4).
export function getShipImage(name) {
    if (!name || typeof name !== 'string') return null;
    let img = imageCache.get(name);
    if (img) return img;
    img = new Image();
    img.src = '/static/sprites/' + name;
    img.onload = scheduleRedraw;
    imageCache.set(name, img);
    return img;
}

// ==================== ПЕРЕКРАСКА (спека §5.3) ====================

const recolorCache = new Map(); // "name|color" -> canvas

// recolorShipSprite — спрайт игрока с перекраской. color = null → исходный
// Image («Оригинал»). Иначе offscreen canvas: drawImage → gCO='hue' →
// fillRect(color) → gCO='destination-in' → drawImage(оригинал) — вернуть
// альфу спрайта (без шага фон красится в цвет, §5.3). Кэш по ключу
// "name|color", лениво, один раз на комбинацию (И6). null — спрайт не
// загружен/имя неизвестно (фолбэк И4).
export function recolorShipSprite(name, color) {
    const img = getShipImage(name);
    if (!img || !img.complete || img.naturalWidth === 0) return null;
    if (!color) return img;

    const key = name + '|' + color;
    let canvas = recolorCache.get(key);
    if (canvas) return canvas;

    canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const ctx = canvas.getContext('2d');
    ctx.drawImage(img, 0, 0);
    ctx.globalCompositeOperation = 'hue';
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    // Восстановление альфы: возвращаем прозрачность исходного спрайта.
    ctx.globalCompositeOperation = 'destination-in';
    ctx.drawImage(img, 0, 0);
    ctx.globalCompositeOperation = 'source-over';

    recolorCache.set(key, canvas);
    return canvas;
}

// ==================== ПАЛИТРА (спека §5.5, уточнение 2026-09-16) ====================

// SHIP_COLOR_PALETTE — 9 хроматических цветов перекраски, порядок совпадает
// с серверным ShipColorPalette (детерминизм агентов: один и тот же id → та же
// пара file+color на любом клиенте). Единый источник на клиенте: дашборд
// импортирует её отсюда (не дублирует).
export const SHIP_COLOR_PALETTE = [
    '#ef4444', '#f97316', '#eab308', '#22c55e', '#14b8a6',
    '#0ea5e9', '#3b82f6', '#8b5cf6', '#ec4899',
];

// ==================== АГЕНТЫ (спека §5.6, И2) ====================

// hashSeed — FNV-1a (32 бита): стабильный хеш строки (перенесён из старого
// модуля схем кораблей). Детерминизм агента: один и тот же визуал на любом
// клиенте.
function hashSeed(s) {
    let h = 2166136261;
    for (let i = 0; i < s.length; i++) {
        h ^= s.charCodeAt(i);
        h = Math.imul(h, 16777619);
    }
    return h >>> 0;
}

// spriteForAgent — детерминированный выбор спрайта агента от id (уточнение
// 2026-09-16): file = shipFiles[FNV-1a(id) % 21], color =
// SHIP_COLOR_PALETTE[FNV-1a(id+1) % 9]. ЕДИНСТВЕННАЯ точка замены под расовый
// визуал (тело функции, сигнатура фиксирована). null — реестр не загружен
// (фолбэк-ромб, И4). Агенты перекрашиваются из кэша (прелоад), не в хот-пате.
export function spriteForAgent(id) {
    if (!id || shipFiles.length === 0) return null;
    const file = shipFiles[hashSeed(String(id)) % shipFiles.length];
    const color = SHIP_COLOR_PALETTE[hashSeed(String(id) + '1') % SHIP_COLOR_PALETTE.length];
    return { file, color };
}

// ==================== ПРЕЛОАД (уточнение 2026-09-16) ====================

// preloadShipSprites — прогрев кэша перекраски для всех 21×9 комбинаций +
// 21 оригинал (Image). Вызывается из main.js после /me (когда shipFiles
// заполнен). Не блокирует старт карты: прогрев идёт по мере onload каждого
// Image (как только спрайт загружен — прогреваются его 9 цветов); уже
// загруженные — сразу. После прелоада отрисовка агентов/игрока — всегда из
// готового кэша, ноль ленивых перекрасок в рантайме.
export function preloadShipSprites() {
    if (shipFiles.length === 0) return;
    for (const file of shipFiles) {
        const img = getShipImage(file);
        if (!img) continue;
        if (img.complete && img.naturalWidth > 0) {
            preloadSpriteColors(file);
        } else {
            img.addEventListener('load', () => preloadSpriteColors(file));
        }
    }
}

// preloadSpriteColors — прогрев 9 цветов одного спрайта (ленивый кэш
// recolorShipSprite заполняется заранее).
function preloadSpriteColors(file) {
    for (const color of SHIP_COLOR_PALETTE) {
        recolorShipSprite(file, color);
    }
}

// ==================== ПЕРЕРИСОВКА ====================

let redrawPending = false;

// scheduleRedraw — перерисовка кадра после асинхронной загрузки спрайта
// (первый кадр мог уйти с фолбэком), без лавины: одна перерисовка на кадр.
// Колбэк не установлен (дашборд до инициализации) — тихий пропуск.
function scheduleRedraw() {
    if (!redrawCallback) return;
    if (redrawPending) return;
    redrawPending = true;
    requestAnimationFrame(() => {
        redrawPending = false;
        redrawCallback();
    });
}
