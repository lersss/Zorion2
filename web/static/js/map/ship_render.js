// web/static/js/map/ship_render.js
// Каталог деталей кораблей + композиция схемы и кэш спрайтов (спека 99.2.15
// §5): один запрос GET /api/ship-parts (O(1) из памяти сервера, И7), кэш по
// ключу (color + отсортированные id деталей); схема — строка
// `<svg viewBox="0 0 200 200" style="color:{color}"><g>{hull}</g>...` в порядке
// слоёв layerOrder. Фолбэк (И4): каталог пуст / деталь бита → null, рисующий
// код использует примитив — без падений.
import { draw } from './map_render.js';

let catalog = null; // { byId, byCategory, palette, layerOrder }
let catalogLoaded = false;
let catalogLoading = false;
const spriteCache = new Map(); // schemaKey -> Image

// ==================== ЗАГРУЗКА КАТАЛОГА ====================

// loadShipCatalog — однократная загрузка каталога (/api/ship-parts).
// При ошибке — тихий отказ: рендер рисует фолбэк-примитив (И4, «не падать»);
// 401/403 разруливает общий поток карты (loadClusters → handleUnauthorized).
export async function loadShipCatalog() {
    if (catalogLoaded || catalogLoading) return;
    catalogLoading = true;
    try {
        const token = localStorage.getItem('token');
        if (!token) return;
        const res = await fetch('/api/ship-parts', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const byId = new Map();
        const byCategory = new Map();
        for (const p of (data.parts || [])) {
            byId.set(p.id, p);
            if (!byCategory.has(p.category)) byCategory.set(p.category, []);
            byCategory.get(p.category).push(p);
        }
        catalog = {
            byId,
            byCategory,
            palette: Array.isArray(data.palette) ? data.palette : [],
            layerOrder: Array.isArray(data.layerOrder) ? data.layerOrder : [],
        };
        catalogLoaded = true;
        spriteCache.clear();
        draw();
    } catch (err) {
        console.warn('loadShipCatalog error:', err);
    } finally {
        catalogLoading = false;
    }
}

// ==================== СБОРКА СХЕМЫ ====================

// hashSeed — FNV-1a (32 бита): стабильный хеш строки seed для выбора
// цвета и деталей (клиентская копия детерминированной сборки §7.1 — сервер
// схему не хранит и не отдаёт, И3 соблюдается в рамках сессии).
function hashSeed(s) {
    let h = 2166136261;
    for (let i = 0; i < s.length; i++) {
        h ^= s.charCodeAt(i);
        h = Math.imul(h, 16777619);
    }
    return h >>> 0;
}

// assemblyFromSeed — детерминированная схема от seed (спека §7.1): цвет из
// палитры + индекс детали по каждой категории (всегда 5, И2). Пустая
// категория в каталоге → пустой id (фолбэк-примитив, И4).
export function assemblyFromSeed(seedStr) {
    const parts = {};
    if (!catalog || catalog.layerOrder.length === 0) {
        return { color: '', parts };
    }
    let color = '';
    if (catalog.palette.length > 0) {
        color = catalog.palette[hashSeed(seedStr + '#c') % catalog.palette.length];
    }
    catalog.layerOrder.forEach((cat, i) => {
        const list = catalog.byCategory.get(cat) || [];
        parts[cat] = list.length ? list[hashSeed(seedStr + '#p' + i) % list.length].id : '';
    });
    return { color, parts };
}

// schemaKey — ключ кэша спрайта: color + отсортированные id деталей
// (спека §5.1: акценты входят в id детали, color трогает только currentColor).
function schemaKey(visual) {
    const ids = Object.values(visual.parts || {}).filter(Boolean).sort().join(',');
    return (visual.color || '') + '|' + ids;
}

// composeShipSVG — строка схемы (спека §5.1), порядок слоёв из layerOrder.
// Деталь из схемы отсутствует в каталоге → дефолтная деталь категории
// (первая в каталоге); цвет вне палитры → дефолтный (первый из палитры);
// каталог пуст → null (рисующий код использует примитив, И4).
function composeShipSVG(visual) {
    if (!catalog || catalog.layerOrder.length === 0) return null;
    const color = visual.color && catalog.palette.includes(visual.color)
        ? visual.color
        : (catalog.palette[0] || '');
    const frags = [];
    for (const cat of catalog.layerOrder) {
        let p = catalog.byId.get((visual.parts || {})[cat]);
        if (!p) p = (catalog.byCategory.get(cat) || [])[0];
        frags.push(`<g>${p ? p.svg : ''}</g>`);
    }
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" style="color:${color}">${frags.join('')}</svg>`;
}

// ==================== СПРАЙТЫ ====================

let redrawPending = false;

// scheduleRedraw — перерисовка кадра после асинхронной загрузки спрайта
// (первый кадр мог уйти с фолбэком), без лавины: одна перерисовка на кадр.
function scheduleRedraw() {
    if (redrawPending) return;
    redrawPending = true;
    requestAnimationFrame(() => {
        redrawPending = false;
        draw();
    });
}

// getShipSprite — спрайт схемы от seed: сборка → кэш по ключу. null —
// каталог пуст или битая деталь: рисующий код использует фолбэк-примитив
// (И4). Изображение грузится асинхронно; по загрузке — перерисовка кадра.
export function getShipSprite(seedStr) {
    if (!catalog || catalog.layerOrder.length === 0) return null;
    const visual = assemblyFromSeed(seedStr);
    const key = schemaKey(visual);
    const cached = spriteCache.get(key);
    if (cached) return cached;
    const svg = composeShipSVG(visual);
    if (!svg) return null;
    const img = new Image();
    img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
    img.onload = scheduleRedraw;
    spriteCache.set(key, img);
    return img;
}