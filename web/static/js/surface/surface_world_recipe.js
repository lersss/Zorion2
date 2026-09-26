import { hash1, num } from './surface_world_noise.js';
import { LIQUID_MEDIUM_COLORS, LIQUID_DEFAULTS } from './surface_config.js';

// VIEW_SCHEMA_VERSION — версия схемы рецепта вида, которую понимает клиент
// (совпадает с planet.ViewSchemaVersion на сервере, §2.6). Пакет с другой версией
// игнорируется → фолбэк FORMATIONS/LIFE_DENSITY.
export const VIEW_SCHEMA_VERSION = 1;

// HORIZON_MAX_LAYERS — потолок поясов яруса (§6 п.8): 2 пояса (решение
// создателя 2026-09-23); третий — только отдельным решением.
export const HORIZON_MAX_LAYERS = 2;

// Плотность жизни по категории биома (§7.3): биосфера/вода — плотно,
// литосфера/крио — скудно (споры, лишайники).
export const LIFE_DENSITY = {
    'биосфера': 0.40, 'вода': 0.35, 'экзотика': 0.16,
    'литосфера': 0.06, 'крио': 0.06, 'вулканизм': 0.03,
};

// LIVING_DECOR — примитивы, изображающие жизнь (§3.2). На безжизненной планете
// (pkg.life=false) не рисуются (решение менеджера 2026-09-23); неживой декор
// (rock/crystal/bone/debris/vent) остаётся. Фолбэк-биомы (без рецепта) не
// затрагиваются — там своя ветка `_legacyDecorAt`.
export const LIVING_DECOR = new Set(['tree', 'conifer', 'palm', 'mushroom', 'cactus', 'bush', 'fern', 'grass', 'lichen', 'growth']);

// resolveRange — [lo,hi] → детерминированное число от seed (§3.2); число — как
// есть. Дефолт — если параметра нет.
export function resolveRange(v, col, seed, def) {
    if (typeof v === 'number') return v;
    if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
        return v[0] + hash1(col, seed) * (v[1] - v[0]);
    }
    return def;
}

// polyW — диапазон ширины полыньи [lo,hi] с дефолтом (§3.2).
export function polyW(v) {
    const def = LIQUID_DEFAULTS.polynya.w;
    if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') return [v[0], v[1]];
    if (typeof v === 'number') return [v, v];
    return [def[0], def[1]];
}

// liquidColorFor — цвет жидкости: явный `liquid.color`, иначе оттенок среды (§3.4).
// Источник истины — данные биома; палитра сред — фолбэк, не второй реестр.
export function liquidColorFor(medium, color) {
    if (typeof color === 'string' && color) return color;
    return LIQUID_MEDIUM_COLORS[medium] || LIQUID_MEDIUM_COLORS['вода'];
}

// resolveLiquid — нормализация слоя жидкости из пакета (спека ЧК6 §3.4/§3.5):
// топ-уровневое `liquid` (серверный резолв), иначе `liquid` внутри biome_view
// (фолбэк). null — жидкости нет (старый пакет/сухой биом). Числа — с дефолтами
// LIQUID_DEFAULTS (§4.2).
export function resolveLiquid(pkg, view) {
    const src = (pkg && pkg.liquid) || (view && view.liquid) || null;
    if (!src || typeof src !== 'object') return null;
    const medium = typeof src.medium === 'string' ? src.medium : '';
    if (!medium) return null;
    const D = LIQUID_DEFAULTS;
    const lvl = src.level || {};
    const surf = src.surface || {};
    const wave = surf.wave || {};
    const poly = lvl.polynya || D.polynya;
    const mode = (lvl.mode === 'basin' || lvl.mode === 'underIce') ? lvl.mode : 'global';
    return {
        medium,
        color: liquidColorFor(medium, src.color),
        glow: (typeof src.glow === 'string' && src.glow) ? src.glow : null,
        ice: (typeof src.ice === 'string' && src.ice) ? src.ice : null,
        fog: (src.fog && typeof src.fog === 'object' && (src.fog.color || src.fog.alpha != null))
            ? { color: src.fog.color || '#123a55', alpha: num(src.fog.alpha, D.fogAlpha) } : null,
        level: {
            mode,
            offset: num(lvl.offset, D.offset),
            minDepth: num(lvl.minDepth, D.minDepth),
            maxDepth: num(lvl.maxDepth, D.maxDepth),
            iceH: num(lvl.iceH, D.iceH),
            window: num(lvl.window, D.window),
            polynya: { gap: Math.max(1, num(poly.gap, D.polynya.gap)), w: polyW(poly.w) },
        },
        surface: {
            alpha: Math.max(0, Math.min(1, num(surf.alpha, D.surface.alpha))),
            foam: Math.max(0, Math.min(1, num(surf.foam, D.surface.foam))),
            wave: {
                lambda: num(wave.lambda, D.surface.wave.lambda),
                amp: num(wave.amp, D.surface.wave.amp),
                speed: num(wave.speed, D.surface.wave.speed),
            },
        },
    };
}

// keySeed — детерминированный seed из имени параметра (для разворота разных
// диапазонов одной записи независимо; без Math.random).
export function keySeed(key) {
    let h = 0;
    for (let i = 0; i < key.length; i++) h = (Math.imul(h, 31) + key.charCodeAt(i)) | 0;
    return h >>> 0;
}

// decorAllowed — фильтр `where` записи декора (§3.2): высота и сторона склона.
// `alt` — насколько земля выше базовой линии (px, больше = выше); `slope` —
// знак уклона (terrainHeight(x+1) − terrainHeight(x), меньше = подъём вправо).
export function decorAllowed(where, alt, slope) {
    if (!where) return true;
    if (typeof where.maxAlt === 'number' && alt > where.maxAlt) return false;
    if (typeof where.minAlt === 'number' && alt < where.minAlt) return false;
    if (where.side === 'left' && slope <= 0) return false;
    if (where.side === 'right' && slope >= 0) return false;
    return true;
}
