// web/static/js/surface/surface_world.js
// Процедурный бесконечный мир одного биома (спека 2026-09-21 §7.2): ландшафт
// (fbm 2 слоя), пещеры (порог 2D-шума), формации по biome_category (В9), декор,
// жизнь (если life). Детерминирован от seed — один и тот же мир при повторе.
// Никакого Math.random: локальный PRNG.
import { CHUNK, FORMATIONS, PPM, FLOAT_SPAN, FLOAT_GAP } from './surface_config.js';

// mulberry32 — локальный PRNG (не общий Math.random).
export function mulberry32(a) {
    return function () {
        a |= 0; a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

// hash1 — единственный хеш-механизм мира; экспортируется для частиц погоды
// (surface_weather.js §5.1 п.2): второй хеш дал бы разные миры при одном seed.
export function hash1(i, seed) {
    let h = Math.imul((i | 0) ^ (seed | 0), 2654435761);
    h ^= h >>> 15;
    h = Math.imul(h, 2246822519);
    h ^= h >>> 13;
    return (h >>> 0) / 4294967296;
}

function hash2(x, y, seed) {
    let h = Math.imul((x | 0) ^ (seed | 0), 374761393);
    h = Math.imul(h ^ (y | 0), 668265263);
    h ^= h >>> 13;
    h = Math.imul(h, 1274126177);
    h ^= h >>> 16;
    return (h >>> 0) / 4294967296;
}

const fade = (t) => t * t * (3 - 2 * t);
const clamp01 = (v) => (v < 0 ? 0 : v > 1 ? 1 : v);
function smoothstep(a, b, x) {
    const t = clamp01((x - a) / (b - a));
    return t * t * (3 - 2 * t);
}

function noise1(x, seed) {
    const i = Math.floor(x);
    const f = x - i;
    const a = hash1(i, seed);
    const b = hash1(i + 1, seed);
    return a + (b - a) * fade(f);
}

function noise2(x, y, seed) {
    const ix = Math.floor(x), iy = Math.floor(y);
    const fx = x - ix, fy = y - iy;
    const a = hash2(ix, iy, seed);
    const b = hash2(ix + 1, iy, seed);
    const c = hash2(ix, iy + 1, seed);
    const d = hash2(ix + 1, iy + 1, seed);
    const u = fade(fx), v = fade(fy);
    return (a + (b - a) * u) * (1 - v) + (c + (d - c) * u) * v;
}

function fbm1(x, seed, octaves) {
    let amp = 0.5, freq = 1, sum = 0, norm = 0;
    for (let o = 0; o < octaves; o++) {
        sum += amp * noise1(x * freq, seed + o * 1013);
        norm += amp;
        amp *= 0.5;
        freq *= 2;
    }
    return sum / norm;
}

// Плотность жизни по категории биома (§7.3): биосфера/вода — плотно,
// литосфера/крио — скудно (споры, лишайники).
const LIFE_DENSITY = {
    'биосфера': 0.40, 'вода': 0.35, 'экзотика': 0.16,
    'литосфера': 0.06, 'крио': 0.06, 'вулканизм': 0.03,
};

// resolveRange — [lo,hi] → детерминированное число от seed (§3.2); число — как
// есть. Дефолт — если параметра нет.
function resolveRange(v, col, seed, def) {
    if (typeof v === 'number') return v;
    if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
        return v[0] + hash1(col, seed) * (v[1] - v[0]);
    }
    return def;
}

// LIVING_DECOR — примитивы, изображающие жизнь (§3.2). На безжизненной планете
// (pkg.life=false) не рисуются (решение менеджера 2026-09-23); неживой декор
// (rock/crystal/bone/debris/vent) остаётся. Фолбэк-биомы (без рецепта) не
// затрагиваются — там своя ветка `_legacyDecorAt`.
const LIVING_DECOR = new Set(['tree', 'conifer', 'palm', 'mushroom', 'cactus', 'bush', 'fern', 'grass', 'lichen', 'growth']);

// keySeed — детерминированный seed из имени параметра (для разворота разных
// диапазонов одной записи независимо; без Math.random).
function keySeed(key) {
    let h = 0;
    for (let i = 0; i < key.length; i++) h = (Math.imul(h, 31) + key.charCodeAt(i)) | 0;
    return h >>> 0;
}

// decorAllowed — фильтр `where` записи декора (§3.2): высота и сторона склона.
// `alt` — насколько земля выше базовой линии (px, больше = выше); `slope` —
// знак уклона (terrainHeight(x+1) − terrainHeight(x), меньше = подъём вправо).
function decorAllowed(where, alt, slope) {
    if (!where) return true;
    if (typeof where.maxAlt === 'number' && alt > where.maxAlt) return false;
    if (typeof where.minAlt === 'number' && alt < where.minAlt) return false;
    if (where.side === 'left' && slope <= 0) return false;
    if (where.side === 'right' && slope >= 0) return false;
    return true;
}

// resolvePalette — палитра вида (§3.4): обязателен base, остальные ключи
// выводятся из base; без рецепта — от пакетного biome_color. Значения — hex
// (shadeHex), чтобы работали rgba()/shade().
function resolvePalette(view, color) {
    const src = (view && view.palette) || {};
    const base = src.base || color;
    return {
        base,
        dark: src.dark || shadeHex(base, 0.62),
        light: src.light || shadeHex(base, 1.15),
        accent: src.accent || shadeHex(base, 0.85),
        rock: src.rock || shadeHex(base, 0.55),
        trunk: src.trunk || shadeHex(base, 0.5),
        moss: src.moss || src.accent || shadeHex(base, 1.3),
        snow: src.snow || '#eef3f7',
        snowShade: src.snowShade || '#b3c6d6',
        ice: src.ice || '#a9c6dc',
        haze: src.haze || null,
        glow: src.glow || src.accent || shadeHex(base, 1.4),
        ember: src.ember || src.accent || base,
    };
}

// ==================== ЯРУСНЫЙ ГОРИЗОНТ (§3.6, Э2) ====================

// VIEW_SCHEMA_VERSION — версия схемы рецепта вида, которую понимает клиент
// (совпадает с planet.ViewSchemaVersion на сервере, §2.6). Пакет с другой версией
// игнорируется → фолбэк FORMATIONS/LIFE_DENSITY.
export const VIEW_SCHEMA_VERSION = 1;

// HORIZON_MAX_LAYERS — потолок поясов яруса (§6 п.8): 2 пояса (решение
// создателя 2026-09-23); третий — только отдельным решением.
export const HORIZON_MAX_LAYERS = 2;

// rangeMid — середина диапазона [lo,hi] или само число. Ярусу нужна низкая
// частота (крупные lambda/amp), пер-волновая рябь здесь не требуется.
function rangeMid(v, def) {
    if (typeof v === 'number') return v;
    if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
        return (v[0] + v[1]) / 2;
    }
    return def;
}

// horizonHeight — мировая высота силуэта яруса (отрицательная = вверх) в точке x.
// Силуэт — из профильных примитивов §3.1 (отдельного kind нет, §2.1), низкая
// частота, без мелкой детализации. Реализованы примитивы, используемые ярусами
// (wave/crest/dome/spike); полный стек профилей — Э3. Детерминирован от seed.
export function horizonHeight(prim, params, x, seed) {
    const p = params || {};
    const s = (seed ^ 0x4f21) >>> 0;
    if (prim === 'crest') {
        // Гребень/хребет: ridged-шум (1−|2n−1|), заострение sharpness.
        const lambda = rangeMid(p.lambda, 800);
        const amp = rangeMid(p.amp, 140);
        const sharp = typeof p.sharpness === 'number' ? p.sharpness : 0.7;
        const ridged = 1 - Math.abs(2 * fbm1(x / lambda, s, 2) - 1);
        return -amp * Math.pow(ridged, 0.6 + 0.8 * sharp);
    }
    if (prim === 'dome') {
        // Купол/всхолмление: плавные холмы (линии крон).
        const lambda = rangeMid(p.lambda, 300);
        const amp = rangeMid(p.amp, 50);
        return -amp * (0.35 + 0.65 * fbm1(x / lambda, s, 2));
    }
    if (prim === 'spike') {
        // Отдельные узкие пики/шпили: треугольник у детерминированного центра.
        const tile = rangeMid(p.tile, rangeMid(p.lambda, 300));
        const h = rangeMid(p.h, 120);
        const w = rangeMid(p.w, 40);
        const i = Math.floor(x / tile);
        const c = i * tile + hash1(i, s ^ 0x51ce) * tile;
        const d = Math.abs(x - c);
        return d > w ? 0 : -h * (1 - d / w);
    }
    // wave (по умолчанию): асимметричная волна — гряды дюн/валы (skew = доля
    // длины на пологом наветренном склоне).
    const lambda = rangeMid(p.lambda, 900);
    const amp = rangeMid(p.amp, 60);
    // skew — доля длины волны на пологом склоне; клампим в (0,1): при 0 или 1
    // деление f/skew или (1−f)/(1−skew) дало бы NaN (валидатор skew не ограничен).
    const rawSkew = typeof p.skew === 'number' ? p.skew : 0.85;
    const skew = Math.max(0.01, Math.min(0.99, rawSkew));
    const ph = x / lambda;
    const i = Math.floor(ph);
    const f = ph - i;
    const localAmp = amp * (0.7 + 0.6 * hash1(i, s ^ 0x77aa));
    return -localAmp * (f < skew ? f / skew : (1 - f) / (1 - skew));
}

export class SurfaceWorld {
    constructor(pkg) {
        this.seed = (pkg.seed | 0) >>> 0;
        this.category = pkg.biome_category || 'литосфера';
        this.life = !!pkg.life;
        this.biome = pkg.biome;
        this.color = pkg.biome_color || '#8a7a6a';
        this.baseY = 300;
        this.region = 1400;
        this.formations = FORMATIONS[this.category] || FORMATIONS['литосфера'];
        // Рецепт вида (спека 2026-09-23 §2.6): применяется только когда сервер
        // отдал резолвленный рецепт (view_source === 'catalog') И версия схемы
        // знакома клиенту (view_version === VIEW_SCHEMA_VERSION, §2.6 —
        // «клиент умеет игнорировать незнакомую версию»). Иначе — фолбэк 1:1 на
        // FORMATIONS/LIFE_DENSITY (старые пакеты/биомы без рецепта/новая схема).
        this.view = (pkg.biome_view && pkg.view_source === 'catalog'
            && pkg.view_version === VIEW_SCHEMA_VERSION) ? pkg.biome_view : null;
        this.hasView = !!this.view;
        this.palette = resolvePalette(this.view, this.color);
        this.placement = (this.view && this.view.placement) || null;
        const rawDecor = (this.view && Array.isArray(this.view.decor)) ? this.view.decor : [];
        // Живой декор гейтится по pkg.life (§3.2): нет жизни → нет живого декора.
        this.decorList = this.life ? rawDecor : rawDecor.filter((d) => !LIVING_DECOR.has(d.prim));
        this.hangList = (this.view && Array.isArray(this.view.hang)) ? this.view.hang : [];
        this._hasWhere = this.decorList.some((d) => d.where);
        this.lifeDensity = this.view
            ? (typeof this.view.life_density === 'number' ? this.view.life_density : 0)
            : (this.life ? (LIFE_DENSITY[this.category] || 0.08) : 0);
        // Ярусный горизонт (§3.6): 2 пояса из рецепта; нет рецепта — ярусов нет
        // (фолбэк 1:1). Потолок 2 (§6 п.8) — лишние пояса игнорируются.
        const hz = this.view && this.view.horizon;
        this.horizon = (hz && Array.isArray(hz.layers)) ? hz.layers.slice(0, HORIZON_MAX_LAYERS) : null;
    }

    _formationForRegion(idx) {
        const h = hash1(idx, this.seed ^ 0x51ed);
        return this.formations[Math.floor(h * this.formations.length) % this.formations.length];
    }

    // formationBlend — плавная (непрерывная) смесь формации региона с предыдущей:
    // на границе обе стороны дают одну высоту. С прежней тройкой prev+cur+next
    // на границе скачок до ~128 px — «обрыв мира», а парящий камень повисал бы
    // ниже игрока (упор в стену, идея 2026-09-21 §4). Смешиваем на входе региона
    // (prev→cur) — так высота в точке спавна x=0 (local=0) не меняется.
    formationBlend(x) {
        const R = this.region;
        const i = Math.floor(x / R);
        const local = (x - i * R) / R;
        const prev = this._formationForRegion(i - 1);
        const cur = this._formationForRegion(i);
        const wPrev = 1 - smoothstep(0, 0.18, local);
        const wCur = 1 - wPrev;
        const mix = (k) => prev[k] * wPrev + cur[k] * wCur;
        return {
            id: cur.id,
            ridge: mix('ridge'),
            flatten: mix('flatten'),
            offset: mix('offset'),
            caves: mix('caves'),
            float: cur.float,
        };
    }

    terrainHeight(x) {
        const f = this.formationBlend(x);
        const large = (fbm1(x * 0.0015, this.seed, 2) - 0.5) * 230 * (0.5 + 0.8 * f.ridge);
        const detail = (fbm1(x * 0.02, this.seed ^ 0x9e37, 3) - 0.5) * 80 * (1 - 0.7 * f.flatten);
        return this.baseY + f.offset - large - detail;
    }

    // farHeight — свой низкочастотный профиль дальнего плана (идея 2026-09-21
    // §2.2): только крупная составляющая, без мелкой ряби ближнего рельефа, и
    // форма своя (не сжатая копия terrainHeight). Детерминирован от seed —
    // локальные хеши, без Math.random. Масштаб согласован с ближним рельефом.
    farHeight(x) {
        const large = fbm1(x * 0.0008, this.seed ^ 0x7a11, 2);
        const mid = fbm1(x * 0.0022, this.seed ^ 0x3c05, 2);
        return this.baseY - 220 - (large - 0.5) * 300 - (mid - 0.5) * 90;
    }

    caveValue(x, y) {
        const n = noise2(x * 0.012, y * 0.02, this.seed ^ 0x1234);
        const n2 = noise2(x * 0.03, y * 0.05, this.seed ^ 0x77);
        return n * 0.7 + n2 * 0.3;
    }

    isCave(x, y) {
        const d = y - this.terrainHeight(x);
        if (d < 8) return false; // тонкая корка поверхности держит игрока
        const f = this.formationBlend(x);
        const threshold = 0.66 - 0.10 * f.caves;
        const depthBonus = Math.min(0.12, d / 3000);
        return this.caveValue(x, y) > threshold - depthBonus;
    }

    isSolid(x, y) {
        const th = this.terrainHeight(x);
        if (y >= th) return !this.isCave(x, y);
        const f = this.formationBlend(x);
        if (f.float) {
            const n = noise2(x * 0.01, y * 0.01, this.seed ^ 0xa5a5);
            // Парящая порода не доходит до земли: зазор FLOAT_GAP (> роста
            // игрока) — под камнем всегда проход, стен «до земли» нет (§4 п.2).
            if (n > 0.72 && y > th - FLOAT_SPAN && y < th - FLOAT_GAP) return true;
        }
        return false;
    }

    // decorAt — декор колонки: по рецепту вида (если есть) или легаси-фолбэк.
    decorAt(x) {
        if (this.hasView) return this._viewDecorAt(x);
        return this._legacyDecorAt(x);
    }

    // _viewDecorAt — декор по рецепту (§3.2/§3.3): правила размещения
    // uniform/clustered, плотность life_density (доля кластерных колонок),
    // фильтр `where` по высоте/склону, выбор примитива взвешенно по p. Всё
    // детерминировано от seed.
    _viewDecorAt(x) {
        if (!this.decorList.length) return null;
        const col = Math.floor(x);
        const p = this.placement || {};
        if (p.mode === 'clustered') {
            const tile = p.tile || 256;
            const gap = p.gap || 16;
            const t = Math.floor(col / tile);
            const span = Math.max(1, tile - gap);
            const start = t * tile + Math.floor(hash1(t, this.seed ^ 0xc105) * gap);
            if (col < start || col >= start + span) return null; // зазор между кучками
        }
        if (hash1(col, this.seed ^ 0xdec0) >= this.lifeDensity) return null;
        let list = this.decorList;
        if (this._hasWhere) {
            const alt = this.baseY - this.terrainHeight(col);
            const slope = this.terrainHeight(col + 1) - this.terrainHeight(col);
            list = list.filter((d) => decorAllowed(d.where, alt, slope));
            if (!list.length) return null;
        }
        let total = 0;
        for (const d of list) total += (d.p || 0);
        if (total <= 0) return null;
        let u = hash1(col, this.seed ^ 0xd3c1) * total;
        let pick = list[list.length - 1];
        for (const d of list) {
            u -= (d.p || 0);
            if (u < 0) { pick = d; break; }
        }
        // Форвардим ВСЕ параметры примитива (buttress/crown/arms/fronds/blades/
        // capR/plume/…); диапазоны [lo,hi] разворачиваем в число детерминированно
        // от seed (§3.2) — иначе `for (i < [2,3])` не рисует крону/вайи.
        const out = { ...pick, col };
        for (const key of Object.keys(out)) {
            if (key === 'h' || key === 'w') continue;
            const v = out[key];
            if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
                out[key] = resolveRange(v, col, (this.seed ^ keySeed(key)) >>> 0, v[0]);
            }
        }
        out.h = resolveRange(pick.h, col, this.seed ^ 0x4848, 24);
        out.w = resolveRange(pick.w, col, this.seed ^ 0x5757, out.h);
        // Лианы (hang, §3.2): привязаны к дереву — только к нему и только
        // детерминированно по колонке.
        if (pick.prim === 'tree' && this.hangList.length) {
            for (const hg of this.hangList) {
                if (hg.from && hg.from !== 'tree') continue;
                if (hash1(col, this.seed ^ 0x11a5) < (hg.p || 0)) {
                    out.hang = { prim: hg.prim, len: resolveRange(hg.len, col, this.seed ^ 0x6c6c, 30) };
                    break;
                }
            }
        }
        return out;
    }

    // _legacyDecorAt — прежний декор по категории (фолбэк 1:1, §2.3/§6 п.10).
    _legacyDecorAt(x) {
        const col = Math.floor(x);
        if (this.life) {
            const r = hash1(col, this.seed ^ 0xdec0);
            if (r < this.lifeDensity * 0.25) {
                return { kind: 'tree', h: 40 + hash1(col, 0x71) * 60 };
            }
            if (r < this.lifeDensity) {
                return { kind: 'plant', h: 8 + hash1(col, 0x72) * 26 };
            }
            return null;
        }
        // Стерильный мир: редкие споры/лишайники/камни — «одиночество/масштаб».
        const r = hash1(col, this.seed ^ 0xd0d0);
        if (r < 0.02) return { kind: 'lichen', h: 4 + hash1(col, 0x73) * 6 };
        if (r < 0.03) return { kind: 'rock', h: 6 + hash1(col, 0x74) * 14 };
        return null;
    }

    // rareDecorAt — редкая декорация-находка (любопытство, §9/§3.3): ≥1 на
    // участок landmark.perRegion (по умолчанию 6000 px); вид — landmark.prim
    // рецепта (например, сухая котловина), иначе легаси-набор; детерминирован
    // участком.
    rareDecorAt(x) {
        const lm = (this.view && this.view.landmark) || null;
        const R = (lm && lm.perRegion) || 6000;
        const i = Math.floor(x / R);
        const cx = i * R + R * 0.5;
        if (Math.abs(x - cx) > 30) return null;
        if (lm && lm.prim) return { prim: lm.prim, x: cx };
        const kinds = ['окаменелость', 'кристалл', 'обломок'];
        const kind = kinds[Math.floor(hash1(i, this.seed ^ 0xbeef) * kinds.length) % kinds.length];
        return { kind, x: cx };
    }

    // creaturesFor — животные чанка (не бой, §7.3): 2–3 поведения.
    creaturesFor(chunkIndex) {
        if (!this.life || this.lifeDensity < 0.1) return [];
        const rng = mulberry32((this.seed ^ Math.imul(chunkIndex, 0x9e3779b1)) >>> 0);
        const count = Math.floor(rng() * 3 * this.lifeDensity + this.lifeDensity);
        const behaviors = ['graze', 'flee', 'approach', 'herd', 'juvenile'];
        const out = [];
        for (let i = 0; i < count; i++) {
            const x = chunkIndex * CHUNK + rng() * CHUNK;
            out.push({
                x,
                y: this.terrainHeight(x) - 12,
                behavior: behaviors[Math.floor(rng() * behaviors.length) % behaviors.length],
                size: 6 + rng() * 8,
                hue: rng(),
                phase: rng() * Math.PI * 2,
                vx: 0,
            });
        }
        return out;
    }
}

// Утилиты цвета (палитра биома → оттенки рельефа).
export function parseHex(hex) {
    const h = hex && hex[0] === '#' ? hex.slice(1) : '8a7a6a';
    return {
        r: parseInt(h.slice(0, 2), 16) || 0,
        g: parseInt(h.slice(2, 4), 16) || 0,
        b: parseInt(h.slice(4, 6), 16) || 0,
    };
}

export function shade(hex, factor) {
    const c = parseHex(hex);
    const f = (v) => Math.max(0, Math.min(255, Math.round(v * factor)));
    return `rgb(${f(c.r)},${f(c.g)},${f(c.b)})`;
}

// shadeHex — как shade(), но возвращает hex: производные ключи палитры (§3.4)
// должны оставаться hex, иначе rgba()/shade() по ним ломаются.
export function shadeHex(hex, factor) {
    const c = parseHex(hex);
    const f = (v) => Math.max(0, Math.min(255, Math.round(v * factor)));
    const to = (v) => f(v).toString(16).padStart(2, '0');
    return '#' + to(c.r) + to(c.g) + to(c.b);
}

export function rgba(hex, alpha) {
    const c = parseHex(hex);
    return `rgba(${c.r},${c.g},${c.b},${alpha})`;
}

export { PPM };
