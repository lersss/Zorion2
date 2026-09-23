// web/static/js/map/ship_sprites.js
// Статичные спрайты кораблей (спека 61b §5.6): замена старого генератора
// схем кораблей. Игрок — цельный PNG из web/static/sprites/ (имя из /me,
// уже смаппленное), перекраска — тонирование gCO='hue' + ОБЯЗАТЕЛЬНОЕ
// восстановление альфы 'destination-in' (иначе фон красится в цвет, §5.3).
// Агенты — спрайт + цвет от spriteForAgent(race_id, id) (спека 2026-09-23
// §6.3: файл из пула расы по слоту, цвет — второй хеш); перекраска ленивая и
// кэшируется (recolorShipSprite). Прелоад (спека §7.3, N6) греет только
// корабль игрока — реестр 118×9 ≈ 170 МБ заранее недопустим. Фолбэк (И4):
// спрайт не загрузился / имени нет → null, рисующий код использует примитив
// (полёт) / ромб (агент) — без падений.

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

// shipFilesByRace — клиентский индекс «раса → файлы» (спека 2026-09-23 §6.2,
// N12): строится в setShipOptions из /me.ship_options (поле race). Порядок
// файлов расы = порядок реестра; ключ "" → нейтральный пул (NeutralShip).
// Источник состава пула для spriteForAgent/pickVariant (П3).
let shipFilesByRace = new Map(); // race -> [file]

// shipOrientIndex — пара показа (A, F) по имени файла из /me.ship_options
// (спека 2026-09-21 §6.2): единственный источник ориентации на клиенте.
const shipOrientIndex = new Map(); // file -> {angle, flip}

// setShipOptions — кладёт пару (A, F) и индекс «раса → файлы» из
// /me.ship_options (массив {id, name, file, race?, angle?, flip?}). Вызывается
// из data.js после загрузки /me (карта), belt_main.js (мини-игра пояса) и
// дашборда (loadShipSelector).
export function setShipOptions(options) {
    const list = Array.isArray(options) ? options : [];
    shipOrientIndex.clear();
    shipFilesByRace = new Map();
    for (const o of list) {
        if (!o || !o.file) continue;
        shipOrientIndex.set(o.file, { angle: Number(o.angle) || 0, flip: !!o.flip });
        const race = o.race || '';
        const pool = shipFilesByRace.get(race);
        if (pool) {
            pool.push(o.file);
        } else {
            shipFilesByRace.set(race, [o.file]);
        }
    }
}

// shipFilesForRace — пул файлов расы из /me.ship_options (порядок = реестр);
// ключ "" — нейтральный пул. Нет расы/пула → нейтральный пул, иначе пустой
// массив (фолбэк клиента, спека §6.3–§6.4).
export function shipFilesForRace(raceId) {
    const key = raceId || '';
    return shipFilesByRace.get(key) || shipFilesByRace.get('') || [];
}

// shipOrientFor — пара (A, F) файла; файл неизвестен/реестр пуст → (0, false) —
// легаси без регрессий (фолбэк, спека §6.2/§6.3).
export function shipOrientFor(file) {
    const o = shipOrientIndex.get(file);
    return o ? o : { angle: 0, flip: false };
}

// shipDrawTransform — полный трансформ отрисовки корабля (спека §6.2/§6.4):
// V = cos H < 0 ? −1 : 1 (антипереворот «верх всегда сверху» по КУРСУ H),
// rotate = H + V·A, scaleX = F ? −1 : 1, scaleY = V. ЕДИНСТВЕННОЕ место
// конвенции на клиенте: точки рендера не считают знак/π/180/cos сами.
// headingRad — курс в радианах; orient — {angle (градусы), flip}.
export function shipDrawTransform(headingRad, orient) {
    const h = Number(headingRad) || 0;
    const a = (orient && Number(orient.angle)) || 0;
    const flip = !!(orient && orient.flip);
    const V = Math.cos(h) < 0 ? -1 : 1;
    return {
        rotate: h + V * a * Math.PI / 180,
        scaleX: flip ? -1 : 1,
        scaleY: V,
    };
}

// ==================== КЭШ ИЗОБРАЖЕНИЙ ====================

const imageCache = new Map(); // name -> Image

// getShipImage — кэш Image по имени файла; загрузка /static/sprites/{name}.png.
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

// ==================== АГЕНТЫ (спека §5.6, И2; расовый визуал — §6.3) ====================

// SHIP_VARIANT_SLOTS — число слотов вариантов расы (как у палитры цветов,
// спека 2026-09-23 §6.2/§6.3, N8); совпадает с серверным
// models.SHIP_VARIANT_SLOTS.
const SHIP_VARIANT_SLOTS = 9;

// HUMAN_TYPES — фиксированный список типов людских кораблей (спека §6.3):
// тип выбирается вторым хешем, вариант — слотом внутри типа.
const HUMAN_TYPES = ['starship', 'cruiser', 'carrier', 'fighter'];

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

// shipVariantOf — номер варианта файла (спека §6.3, N8): суффикс `_0<v>`;
// базовый файл без суффикса (только у людей, `race_humans_<type>.png`) — вариант 1.
function shipVariantOf(file) {
    const m = /_(\d+)\.png$/.exec(file);
    return m ? Number(m[1]) : 1;
}

// humanTypeOf — тип людского файла (`race_humans_<type>[_NN].png`), иначе null.
function humanTypeOf(file) {
    const m = /^race_humans_([a-z]+?)(?:_\d+)?\.png$/.exec(file);
    return m ? m[1] : null;
}

// pickVariant — детерминированный выбор файла пула по слоту (спека §6.3, N8):
// v = 1 + FNV-1a(key + '|v') % SHIP_VARIANT_SLOTS; файл с суффиксом `_0<v>`,
// иначе запись с НАИМЕНЬШИМ суффиксом пула (НЕ уходит в нейтральный, НЕ `% len`).
// Стабильность при росте пула: добавление `_0<w>` меняет только слот w.
function pickVariant(pool, key) {
    const v = 1 + (hashSeed(key + '|v') % SHIP_VARIANT_SLOTS);
    let exact = null;
    let smallest = null;
    let smallestV = Infinity;
    for (const file of pool) {
        const fv = shipVariantOf(file);
        if (fv === v) exact = file;
        if (fv < smallestV) { smallestV = fv; smallest = file; }
    }
    return exact || smallest;
}

// spriteForAgent — детерминированный выбор спрайта агента от (race_id, id)
// (спека 2026-09-23 §6.3): пул расы, иначе нейтральный пул (""), иначе null
// (фолбэк-ромб, И4). Файл — по слоту (pickVariant), цвет — второй хеш. У людей
// тип выбирается вторым хешем по фиксированному списку, вариант — слотом
// внутри типа. Ноль хранилища: чистая функция от (race_id, id) (И2).
export function spriteForAgent(raceId, id) {
    const race = raceId || '';
    let pool = shipFilesByRace.get(race);
    if (!pool || pool.length === 0) pool = shipFilesByRace.get('');
    if (!pool || pool.length === 0) return null;
    const key = race + '|' + String(id);
    let file;
    if (race === 'humans') {
        const type = HUMAN_TYPES[hashSeed(key + '|t') % HUMAN_TYPES.length];
        const typePool = pool.filter(f => humanTypeOf(f) === type);
        file = pickVariant(typePool.length ? typePool : pool, key);
    } else {
        file = pickVariant(pool, key);
    }
    const color = SHIP_COLOR_PALETTE[hashSeed(key + '|c') % SHIP_COLOR_PALETTE.length];
    const orient = shipOrientFor(file);
    return { file, color, angle: orient.angle, flip: orient.flip };
}

// ==================== ПРЕЛОАД (ревизия И6, спека §7.3, N6) ====================

// preloadShipSprites — прогрев кэша перекраски ТОЛЬКО корабля игрока (спека
// 2026-09-23 §7.3, N6: прежнее «все N×9 заранее» отменено — 118×9 ≈ 170 МБ
// недопустимы). Остальные спрайты догружаются лениво при первом появлении
// агента (recolorShipSprite кэширует результат). playerFile — файл корабля
// игрока (/me.ship_icon); пусто — прогрева нет. Вызывается из main.js после /me.
export function preloadShipSprites(playerFile) {
    if (!playerFile) return;
    const img = getShipImage(playerFile);
    if (!img) return;
    if (img.complete && img.naturalWidth > 0) {
        preloadSpriteColors(playerFile);
    } else {
        img.addEventListener('load', () => preloadSpriteColors(playerFile));
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
