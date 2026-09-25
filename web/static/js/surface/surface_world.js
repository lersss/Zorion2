// web/static/js/surface/surface_world.js
// Процедурный бесконечный мир одного биома (спека 2026-09-21 §7.2): ландшафт
// (fbm 2 слоя), пещеры (порог 2D-шума), формации по biome_category (В9), декор,
// жизнь (если life). Детерминирован от seed — один и тот же мир при повторе.
// Никакого Math.random: локальный PRNG.
import { CHUNK, CHUNK_TOP_MARGIN, CHUNK_HEIGHT, FORMATIONS, PPM, FLOAT_SPAN, FLOAT_GAP } from './surface_config.js';

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

// ==================== ПРОФИЛЬНЫЕ ПРИМИТИВЫ РЕЛЬЕФА (§3.1, Э3) ====================
//
// Восемь примитивов — чистые функции (x, params, seed) → Δy (отрицательное = вверх),
// складываются с базой terrainHeight. Семантика общих параметров (§3.1): `amp` —
// полуразмах; `share` — доля регионов, где слой включён (детерминированно от seed
// и индекса региона); `viewOnly` — слой только в отрисовке (viewHeight); `dir` —
// сторона асимметрии. Единая реализация для боевого стека (terrainHeight) и яруса
// горизонта (§3.6) — второй реализации одной формы нет.

// num — число из рецепта или дефолт.
function num(v, def) {
    return typeof v === 'number' ? v : def;
}

// primRange — [lo,hi] по индексу фичи i (детерминированно от seed) или число.
function primRange(v, i, seed, def) {
    if (typeof v === 'number') return v;
    if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
        return v[0] + hash1(i, seed) * (v[1] - v[0]);
    }
    return def;
}

// primWave — асимметричная волна (дюны/барханы/валы/зыбь): пологий наветренный
// склон (доля `skew` длины) и крутой подветренный. `leeMaxDeg` — угол естественного
// отсыпа: подветренный склон не круче него, выше гребень «срезается» (модель
// лавинного срыва, §4.1). `dir` — сторона асимметрии (+1 = пологий слева).
function primWave(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0xa1, 600);
    const amp = primRange(l.amp, 0, seed ^ 0xa2, 40);
    const skew = Math.max(0.05, Math.min(0.95, num(l.skew, 0.85)));
    const ph = x / lambda;
    const i = Math.floor(ph);
    let f = ph - i;
    if (l.dir === -1) f = 1 - f; // зеркало стороны асимметрии
    let a = amp * (0.75 + 0.5 * hash1(i, seed ^ 0xa3));
    if (typeof l.leeMaxDeg === 'number') {
        const maxSlope = Math.tan(l.leeMaxDeg * Math.PI / 180);
        const leeLen = (1 - skew) * lambda;
        if (leeLen > 0 && a / leeLen > maxSlope) a = maxSlope * leeLen; // срыв гребня
    }
    return -a * (f < skew ? f / skew : (1 - f) / (1 - skew));
}

// primCrest — гребень/хребет: ridged-шум (1−|2n−1|), `sharpness` заостряет пики.
function primCrest(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0xc1, 700);
    const amp = primRange(l.amp, 0, seed ^ 0xc2, 120);
    const sharp = num(l.sharpness, 0.7);
    const ridged = 1 - Math.abs(2 * fbm1(x / lambda, seed ^ 0xc3, 2) - 1);
    return -amp * Math.pow(ridged, 0.6 + 0.8 * sharp);
}

// primSpike — отдельные узкие пики/шпили/иглы: `perRegion` (или `count`) штук на
// регион, треугольник высоты `h` и полуширины `w`. `taper` заостряет вершину;
// `cluster` — иглы группируются в кусты/рощи (спека 2026-09-23 §4.7.12), а не
// стоят поодиночке. Без `cluster` — прежнее поведение (сданное не меняется).
function primSpike(x, l, seed, region) {
    const R = region || 1400;
    const cntRaw = l.perRegion != null ? l.perRegion : l.count;
    const taper = num(l.taper, 0);
    const cluster = !!l.cluster;
    const base = Math.floor(x / R);
    let sum = 0;
    // Соседние регионы тоже: пик у границы региона не должен «обрезаться»
    // (иначе разрыв профиля на границе — уклон-скачок).
    for (let rr = base - 1; rr <= base + 1; rr++) {
        const n = Math.max(0, Math.round(primRange(cntRaw, rr, seed ^ 0xd0, 1)));
        if (!cluster) {
            for (let k = 0; k < n; k++) {
                const idx = rr * 131 + k;
                const c = rr * R + hash1(idx, seed ^ 0xd1) * R;
                const h = primRange(l.h, idx, seed ^ 0xd2, 120);
                const w = primRange(l.w, idx, seed ^ 0xd3, 40);
                const d = Math.abs(x - c);
                if (d < w) sum -= h * Math.pow(1 - d / w, 1 + 2 * taper);
            }
            continue;
        }
        // cluster: `n` игл собираются в несколько кустов-рощ; центры кустов
        // распределены по региону, иглы теснятся вокруг центра (куст читается
        // как заросль, а не набор одиночек).
        const groves = Math.max(1, Math.round(Math.sqrt(n)));
        const per = Math.ceil(n / groves);
        for (let g = 0; g < groves; g++) {
            const gid = rr * 197 + g;
            const gc = rr * R + ((g + 0.5) / groves) * R
                + (hash1(gid, seed ^ 0xd4) - 0.5) * (R / groves) * 0.4;
            for (let k = 0; k < per; k++) {
                if (g * per + k >= n) break;
                const idx = gid * 131 + k;
                const c = gc + (hash1(idx, seed ^ 0xd5) - 0.5) * R * 0.03;
                const h = primRange(l.h, idx, seed ^ 0xd2, 120);
                const w = primRange(l.w, idx, seed ^ 0xd3, 40);
                const d = Math.abs(x - c);
                if (d < w) sum -= h * Math.pow(1 - d / w, 1 + 2 * taper);
            }
        }
    }
    return sum;
}

// primStep — террасы/пласты/меса: квантование низкочастотного профиля в уступы
// высоты `stepH`; `stepW` — горизонтальный масштаб, `jitter` — сдвиг границ.
function primStep(x, l, seed) {
    const stepH = primRange(l.stepH, 0, seed ^ 0xe1, 60);
    const stepW = Math.max(1, primRange(l.stepW, 0, seed ^ 0xe2, 240));
    const jitter = num(l.jitter, 0);
    const cell = Math.floor(x / stepW);
    const n = fbm1(x / (stepW * 3), seed ^ 0xe3, 2) + jitter * (hash1(cell, seed ^ 0xe4) - 0.5);
    return -stepH * Math.floor(clamp01(n) * 4);
}

// primDome — купол/всхолмление (холмы, пинго, тумули): плавные холмы ±`amp`.
// `squash` — «сплюснутость» (горизонтальное расширение купола).
function primDome(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0xf1, 300);
    const amp = primRange(l.amp, 0, seed ^ 0xf2, 50);
    const squash = Math.max(0.1, num(l.squash, 1));
    const n = fbm1(x / (lambda * squash), seed ^ 0xf3, 2);
    return -amp * (2 * n - 1);
}

// primCarve — вырез (каньон/русло/трещина/воронка): вниз на `depth` в центре русла
// полуширины `w`; `rim` — приподнятый борт у краёв («подмытые берега»), shape V/U.
// `closed` — гипотеза (образцами не задаётся).
function primCarve(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0x11, 700);
    const depth = primRange(l.depth, 0, seed ^ 0x12, 60);
    const w = Math.max(1, primRange(l.w, 0, seed ^ 0x13, 160));
    const rim = num(l.rim, 0);
    const ph = x / lambda;
    const f = ph - Math.floor(ph);
    const d = Math.abs(f - 0.5) * lambda; // расстояние до центра русла (px)
    if (d >= w) return 0;
    const t = 1 - d / w;                  // 1 в центре, 0 на краю
    const prof = l.shape === 'V' ? t : t * t * (3 - 2 * t);
    // Борт (rim): вал у края русла — поднятие над базовой линией, максимум при
    // t≈0.15, ноль на самом краю (t=0) и дальше от края; «подмытые берега» §4.2.
    const lip = t < 0.3 ? Math.sin(Math.PI * t / 0.3) : 0;
    return depth * prof - depth * rim * lip;
}

// primFan — осыпь/конус выноса: уклон не круче `angleMax` (угол отсыпа), вынос =
// h / tan(angleMax). `w` (спека 2026-09-23 §4.7.12) — желаемая МИНИМАЛЬНАЯ ширина
// основания (полная): основание не уже, чем требует угол, угол никогда не круче
// `angleMax` — `halfW = max(h/tanA, w/2)`. `roughness` — неровность конуса —
// применяется ТОЛЬКО при явном `w` (гейт совместимости §4.7.13: сданные ЧК0/ЧК1
// несут инертную `roughness` и без `w` остаются гладкими).
function primFan(x, l, seed, region) {
    const R = region || 1400;
    const angleMax = num(l.angleMax, 34);
    const tanA = Math.tan(angleMax * Math.PI / 180);
    const hasW = l.w != null;
    const rough = hasW ? num(l.roughness, 0) : 0;
    const base = Math.floor(x / R);
    let sum = 0;
    // Соседние регионы — как у spike: конус у границы региона не обрезается.
    for (let rr = base - 1; rr <= base + 1; rr++) {
        const h = primRange(l.h, rr, seed ^ 0x21, 60);
        const wHalf = hasW ? primRange(l.w, rr, seed ^ 0x24, 0) / 2 : 0;
        const halfW = Math.max(8, h / tanA, wHalf);
        const c = rr * R + hash1(rr, seed ^ 0x23) * R;
        if (!hasW || rough <= 0) {
            const d = Math.abs(x - c);
            if (d <= halfW) sum -= h * (1 - d / halfW);
            continue;
        }
        // Шероховатый конус: несколько апексов внутри основания (верхняя
        // огибающая через min), каждый слагаемый не круче angleMax — угол
        // общей формы остаётся в пределах угла отсыпа.
        const sub = 3;
        for (let sl = 0; sl < sub; sl++) {
            const sid = rr * 53 + sl;
            const hh = h * (0.6 + 0.4 * hash1(sid, seed ^ 0x26));
            const cc = c + (hash1(sid, seed ^ 0x27) - 0.5) * (halfW * 0.6);
            const hw = Math.max(8, hh / tanA, wHalf);
            const dd = Math.abs(x - cc);
            if (dd <= hw) sum = Math.min(sum, -hh * (1 - dd / hw));
        }
    }
    return sum;
}

// primFlow — язык потока (лава/грязь/лёд/сель). Новые поля (спека 2026-09-23
// §4.7.12): `lobes` — число языков-лопастей на период (несколько стекающих
// языков, а не один вал); `slope` — асимметрия языка: знак = сторона срыва,
// модуль = крутизна (0 — симметричный вал); `levees` — доля боковых валов-гребней
// по краям потока. Без этих полей — прежний одиночный низкочастотный вал
// (сданные ЧК0/ЧК1/ЧК2 не меняются, §4.7.13).
function primFlow(x, l, seed) {
    const len = primRange(l.len, 0, seed ^ 0x31, 300);
    const w = primRange(l.w, 0, seed ^ 0x32, 120);
    if (l.lobes == null && l.slope == null && l.levees == null) {
        const n = fbm1(x / len, seed ^ 0x33, 1);
        return -w * 0.25 * (2 * n - 1);
    }
    const amp = w * 0.25;
    const lobes = Math.max(1, Math.round(primRange(l.lobes, 0, seed ^ 0x34, 1)));
    const slope = num(l.slope, 0);
    const levees = Math.max(0, num(l.levees, 0));
    const ph = x / len;
    const i = Math.floor(ph);
    let f = ph - i;
    if (slope < 0) f = 1 - f;                       // знак — сторона срыва
    const skew = Math.min(0.8, Math.abs(slope));    // модуль — крутизна асимметрии
    const half = 0.5 / lobes;
    // `lobes` языков-лопастей внутри периода: левый склон пологий, правый — срыв
    // (или наоборот при slope < 0); при skew = 0 языки симметричны. Языки не
    // доходят до границ периода (профиль 0 на стыке) — период непрерывен.
    let prof = 0;
    for (let k = 0; k < lobes; k++) {
        const c = (k + 0.5) / lobes;
        const s = f - c;
        const a = s < 0 ? half : half * (1 - skew);
        if (a <= 0 || Math.abs(s) >= a) continue;
        prof = Math.max(prof, 1 - Math.abs(s) / a);
    }
    // Боковые валы-гребни по краям потока (f → 0 и f → 1, §4.7.12).
    if (levees > 0) {
        const edge = 1 - Math.min(f, 1 - f) / half;
        if (edge > 0) prof = Math.max(prof, levees * edge);
    }
    const rise = amp * (0.75 + 0.5 * hash1(i, seed ^ 0x36));
    return -rise * prof;
}

// primHeight — диспетчер примитивов (§3.1). Неизвестный prim → 0 (forward-compat).
export function primHeight(prim, x, l, seed, region) {
    switch (prim) {
        case 'wave': return primWave(x, l, seed);
        case 'crest': return primCrest(x, l, seed);
        case 'spike': return primSpike(x, l, seed, region);
        case 'step': return primStep(x, l, seed);
        case 'dome': return primDome(x, l, seed);
        case 'carve': return primCarve(x, l, seed);
        case 'fan': return primFan(x, l, seed, region);
        case 'flow': return primFlow(x, l, seed);
        default: return 0;
    }
}

// horizonHeight — силуэт яруса горизонта (§3.6): та же библиотека примитивов, что и
// боевой стек (§3.1) — единая реализация формы, второго набора нет.
export function horizonHeight(prim, params, x, seed) {
    return primHeight(prim, x, params || {}, seed, 1400);
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
        // _viewKey — идентичность рецепта для сброса кэша чанков (§2.5): версия
        // схемы + хеш relief. Правка рецепта (hot-reload) меняет ключ — старый
        // растр не переиспользуется. Рецепт — константа мира (в сессии не меняется).
        const relKey = (this.view && this.view.relief) || null;
        this._viewKey = (pkg.view_version || 0) + ':' + keySeed(relKey ? JSON.stringify(relKey) : '');
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
        // Профиль формы (§3.1, Э3): базовые скаляры рецепта (ridge/flatten/offset/
        // caves/float/base) + стек слоёв; `scale` — общий множитель амплитуд.
        const rel = (this.view && this.view.relief) || null;
        this.relief = rel;
        // 2D-формы (relief.forms, спека Э5 §3.1): список операторов relief2d
        // (Э5.2 — overhang/crack2d). Нет рецепта/списка — форм нет (фолбэк 1:1).
        this.forms = (rel && Array.isArray(rel.forms) && rel.forms.length) ? rel.forms : null;
        this.reliefScale = rel ? Math.max(0, num(rel.scale, 1)) : 1;
        const layers = (rel && Array.isArray(rel.layers)) ? rel.layers : null;
        // viewOnly-слои — только отрисовка (viewHeight), физика их не знает (§3.1).
        this.physLayers = layers ? layers.filter((l) => !l.viewOnly) : null;
        this.viewLayers = layers ? layers.filter((l) => !!l.viewOnly) : null;
        // Снеговая линия (§4.3): px ниже ЛОКАЛЬНОГО максимума профиля; абсолютной
        // шкалы высот в прогулке нет — линия относительная (осознанное отклонение).
        this.snowLine = (this.view && typeof this.view.snowLine === 'number') ? this.view.snowLine : null;
        this.snowLineShadow = num(this.view && this.view.snowLineShadow, 0.85);
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
        // Рецепт формы (§3.1): базовые скаляры заданы рецептом и постоянны; регион
        // нужен только для `share` слоёв. Нет рецепта — прежняя смесь формаций.
        if (this.relief) {
            const r = this.relief;
            return {
                id: this.biome,
                ridge: num(r.ridge, 0),
                flatten: num(r.flatten, 0),
                offset: num(r.offset, 0),
                caves: num(r.caves, 0),
                float: !!r.float,
                base: num(r.base, 1),
            };
        }
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
            base: 1,
        };
    }

    // layerHeight — Δy слоя (§3.1): 0, если слой выключен по региону (share < 1).
    layerHeight(l, x) {
        const share = num(l.share, 1);
        if (share < 1 && hash1(Math.floor(x / this.region), this.seed ^ 0x5a5a) >= share) return 0;
        return primHeight(l.prim, x, l, this.seed, this.region);
    }

    // sumLayers — сумма Δy слоёв стека.
    sumLayers(layers, x) {
        let s = 0;
        for (const l of layers) s += this.layerHeight(l, x);
        return s;
    }

    // terrainHeight — ФИЗИЧЕСКИЙ профиль: база (ridge/flatten/offset + fbm) + стек
    // relief.layers (без viewOnly), общий множитель relief.scale (§3.1, §6 п.6).
    // Нет рецепта — прежняя формула 1:1 (фолбэк §6 п.10).
    terrainHeight(x) {
        const f = this.formationBlend(x);
        const large = (fbm1(x * 0.0015, this.seed, 2) - 0.5) * 230 * (0.5 + 0.8 * f.ridge) * f.base;
        const detail = (fbm1(x * 0.02, this.seed ^ 0x9e37, 3) - 0.5) * 80 * (1 - 0.7 * f.flatten);
        // scale — общий множитель амплитуд (база + деталь + слои), как в бюджете
        // §4.3/§6 п.6 и в серверном валидаторе declaredReliefTop. Фолбэк: scale = 1.
        // База вычитается (крупное положительное = выше), а слои ПРИБАВЛЯЮТСЯ: у
        // примитивов Δy отрицательна = вверх (§3.1), поэтому формула спеки —
        // `baseY + offset − large − detail + Σ layers`. Внутри отрицаемого `rise`
        // слои складывать нельзя — форма переворачивается (гребень = впадина).
        let y = this.baseY + f.offset - this.reliefScale * (large + detail);
        if (this.physLayers) y += this.reliefScale * this.sumLayers(this.physLayers, x);
        return y;
    }

    // viewHeight — профиль ОТРИСОВКИ: физический профиль + viewOnly-слои (рябь
    // песков, §3.1). Физика (isSolid/terrainHeight) их не знает.
    viewHeight(x) {
        if (!this.viewLayers || !this.viewLayers.length) return this.terrainHeight(x);
        return this.terrainHeight(x) + this.reliefScale * this.sumLayers(this.viewLayers, x);
    }

    // surfaceY — верх отрисовки поверхности: минимум физического и видового профиля
    // (растр — надмножество твёрдой области, §6 п.4: viewOnly-рябь поднимает гребни,
    // но не оставляет непокрашенной физическую кромку).
    surfaceY(x) {
        if (!this.viewLayers || !this.viewLayers.length) return this.terrainHeight(x);
        return Math.min(this.terrainHeight(x), this.viewHeight(x));
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

    // caveAt — isCave с ЗАРАНЕЕ вычисленным профилем th: растр (консервативная
    // маска) вызывает её на каждую пробу и не должен пересчитывать terrainHeight
    // (fbm) в цикле — иначе шаг 1 px у границы пещеры упирается в бюджет
    // генерации чанка. Поведение идентично `isCave(x, y)`.
    caveAt(x, y, th) {
        const d = y - th;
        if (d < 8) return false; // тонкая корка поверхности держит игрока
        const f = this.formationBlend(x);
        const threshold = 0.66 - 0.10 * f.caves;
        const depthBonus = Math.min(0.12, d / 3000);
        return this.caveValue(x, y) > threshold - depthBonus;
    }

    isCave(x, y) {
        return this.caveAt(x, y, this.terrainHeight(x));
    }

    // ==================== ПОЛЕ ТВЁРДОСТИ (Э5.1, §2) ====================
    //
    // solidAt — ЕДИНСТВЕННЫЙ источник твёрдости (§2.1): база ⊕ формы. И физика
    // (Player), и растр чанка читают только его — второй реализации коллизии/
    // растеризации нет. В Э5.1 формы пусты → поле равно текущему миру 1:1.

    // baseSolid — базовое твёрдое тело: профиль без пещер (`y ≥ terrainHeight`)
    // плюс полоса парящей породы (float). Пещеры (`isCave`, 2D-шум) ВЫЧИТАЮТ
    // твёрдое из базы — это часть тела, не косметика (S1-bis). Формы (relief2d)
    // сюда не входят — их добавляет formsSolid (§2.2).
    baseSolid(x, y) {
        return this._baseSolidAt(x, y, this.terrainHeight(x));
    }

    // _baseSolidAt — база с ПРЕДвычисленным профилем th: без повторного fbm там,
    // где профиль уже есть (растр/единое поле solidAt). Поведение идентично
    // `baseSolid` (caveAt(x, y, terrainHeight(x))).
    _baseSolidAt(x, y, th) {
        if (y >= th) return !this.caveAt(x, y, th);
        const f = this.formationBlend(x);
        if (f.float) {
            const n = noise2(x * 0.01, y * 0.01, this.seed ^ 0xa5a5);
            // Парящая порода не доходит до земли: зазор FLOAT_GAP (> роста
            // игрока) — под камнем всегда проход, стен «до земли» нет (§4 п.2).
            if (n > 0.72 && y > th - FLOAT_SPAN && y < th - FLOAT_GAP) return true;
        }
        return false;
    }

    // ==================== 2D-ФОРМЫ (Э5.2, §3) ====================
    //
    // formsAdditive/formsSubtractive — вклад 2D-форм рецепта (relief.forms, §3.1)
    // двух знаков: аддитивные несущие (`overhang`: плита-арка `arch=1` / козырёк
    // `arch=0`) добавляют материал, вычитающие (`crack2d`) убирают его. Порядок
    // поля — §2.2: `solidAt = (baseSolid ∨ formsAdditive) ∧ ¬formsSubtractive`.
    // Инстансы размещаются детерминированно по регионам (как primSpike: соседние
    // регионы base−1…base+1, только hash1/mulberry32, Math.random запрещён, §5
    // п.2); формы статичны. `relief.scale` к формам НЕ применяется (абсолютные px).

    // _collectFormIntervals — интервалы ТВЁРДЫХ форм колонки x: [{add, top, bottom}, …].
    // thx — предвычисленный `terrainHeight(x)` (растр передаёт кэш; физика —
    // undefined, считается здесь). Один источник геометрии для растра
    // (`formSpans`) и физики (`formsAdditive`/`formsSubtractive`) — второй
    // реализации формы нет (§2.1). `crack2d` сюда НЕ входит: решением гейта
    // 2026-09-25 он переведён в косметику (viewOnly, `crackSpans`) — сквозная
    // трещина и тёмная расселина в 2D-виде неотличимы, физику не трогаем
    // (§5.1 п.5, дефект D1). void/crater — Э5.3.
    _collectFormIntervals(x, thx) {
        const out = [];
        const forms = this.forms;
        if (!forms || !forms.length) return out;
        if (thx === undefined) thx = this.terrainHeight(x);
        const R = this.region;
        const regionBase = Math.floor(x / R);
        for (let fi = 0; fi < forms.length; fi++) {
            const f = forms[fi];
            const prim = f.prim;
            // Э5.2-операторы; void/crater — Э5.3 (здесь не применяются, §3.4).
            // crack2d сюда НЕ входит: косметика (`crackSpans`), физику не трогает (D1).
            if (prim !== 'overhang') continue;
            const formSalt = Math.imul(fi + 1, 0x9e3779b1) >>> 0;
            const sBase = (this.seed ^ formSalt) >>> 0;
            const rawN = f.perRegion != null ? f.perRegion : 1;
            for (let rr = regionBase - 1; rr <= regionBase + 1; rr++) {
                const n = Math.max(0, Math.round(primRange(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
                for (let k = 0; k < n; k++) {
                    const idx = rr * 131 + k;
                    const c = rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R;
                    if (prim === 'overhang') {
                        const w = primRange(f.w, idx, (sBase ^ 0x0f03) >>> 0, 40);
                        const h = primRange(f.h, idx, (sBase ^ 0x0f04) >>> 0, 20);
                        const taper = Math.max(0, Math.min(0.95, primRange(f.taper, idx, (sBase ^ 0x0f05) >>> 0, 0)));
                        if (f.arch === 1) {
                            // Замкнутая арка/плита-свод: висит НАД землёй (§3.2,
                            // знак исправлен в Э5.2, y растёт вниз): подошва =
                            // th(x) − opening, верх = th(x) − opening − h_eff;
                            // `taper` утоньшает плиту к замку (центру пролёта).
                            if (w <= 0) continue;
                            const opening = primRange(f.opening, idx, (sBase ^ 0x0f06) >>> 0, 60);
                            const dx = Math.abs(x - c);
                            if (dx > w) continue;
                            const u = dx / w;
                            const hEff = h * (1 - taper * (1 - u));
                            out.push({ add: true, top: thx - opening - hEff, bottom: thx - opening });
                        } else {
                            // Консоль/козырёк/шляпа: опорная колонка x₀ = c, подошва =
                            // th(x₀), вынос `reach` в сторону `dir`, `taper` к краю
                            // (§3.2). Просвета-параметра нет — он производен от
                            // рельефа под выносом.
                            const reach = Math.max(1, primRange(f.reach, idx, (sBase ^ 0x0f07) >>> 0, 60));
                            const dir = f.dir === -1 ? -1 : 1;
                            const d = (x - c) * dir;
                            if (d < 0 || d > reach) continue;
                            // Кэш опорной высоты инстанса: `terrainHeight(c)` — дорогой
                            // fbm, а столбцов под одним козырьком сотни; без кэша
                            // генерация чанка взлетает в разы (§2.4, бюджет ≤2×).
                            const ck = fi + ':' + rr + ':' + k;
                            const support = this._formSupport || (this._formSupport = new Map());
                            let th0 = support.get(ck);
                            if (th0 === undefined) {
                                th0 = this.terrainHeight(c);
                                if (support.size > 20000) support.clear();
                                support.set(ck, th0);
                            }
                            const t = d / reach;
                            const hEff = h * (1 - taper * t);
                            out.push({ add: true, top: th0 - hEff, bottom: th0 });
                        }
                    }
                }
            }
        }
        return out;
    }

    // formSpans — интервалы форм колонки для растра (§2.3): {add, sub} или null
    // (столбец формой не задет). Единственный источник геометрии форм.
    formSpans(x, thx) {
        const iv = this._collectFormIntervals(x, thx);
        if (!iv.length) return null;
        const add = [], sub = [];
        for (const it of iv) (it.add ? add : sub).push({ top: it.top, bottom: it.bottom });
        return { add, sub };
    }

    // crackSpans — КОСМЕТИЧЕСКИЕ интервалы трещин crack2d колонки (решение гейта
    // 2026-09-25): тёмный клин поверх рельефа, ВНЕ равенства по твёрдому телу
    // (§2.1) — физику не трогает (дефект D1: наклонная трещина запирала игрока,
    // §5.1 п.5). Геометрия та же, что раньше вычитала твёрдое (`w`/`depth`/`taper`/
    // `tilt`, детерминированно от seed); соль инстанса берётся от индекса формы
    // `fi`, как в `_collectFormIntervals`, — положение трещин сохранено.
    crackSpans(x, thx) {
        const iv = this._collectCrackIntervals(x, thx);
        return iv.length ? iv : null;
    }

    _collectCrackIntervals(x, thx) {
        const out = [];
        const forms = this.forms;
        if (!forms || !forms.length) return out;
        if (thx === undefined) thx = this.terrainHeight(x);
        const R = this.region;
        const regionBase = Math.floor(x / R);
        for (let fi = 0; fi < forms.length; fi++) {
            const f = forms[fi];
            if (f.prim !== 'crack2d') continue;
            const formSalt = Math.imul(fi + 1, 0x9e3779b1) >>> 0;
            const sBase = (this.seed ^ formSalt) >>> 0;
            const rawN = f.perRegion != null ? f.perRegion : 1;
            for (let rr = regionBase - 1; rr <= regionBase + 1; rr++) {
                const n = Math.max(0, Math.round(primRange(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
                for (let k = 0; k < n; k++) {
                    const idx = rr * 131 + k;
                    const c = rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R;
                    // Клин шире вверху, сужение `taper`, наклон `tilt`. Полная ширина
                    // `w` (без прежнего клампа PLAYER_W−2, ставшего ненужным): форма
                    // косметична, игрока не заглатывает.
                    const wFull = primRange(f.w, idx, (sBase ^ 0x0f08) >>> 0, 10);
                    const depth = Math.max(1, primRange(f.depth, idx, (sBase ^ 0x0f09) >>> 0, 100));
                    const taper = Math.max(0, Math.min(0.95, primRange(f.taper, idx, (sBase ^ 0x0f0a) >>> 0, 0.5)));
                    const tiltDeg = primRange(f.tilt, idx, (sBase ^ 0x0f0b) >>> 0, 0);
                    const tiltSign = hash1(idx, (sBase ^ 0x0f0c) >>> 0) < 0.5 ? -1 : 1;
                    const a = wFull * 0.5;
                    const b = a * taper / depth;
                    const kk = tiltSign * Math.tan(tiltDeg * Math.PI / 180);
                    const denom = kk - b;
                    let dyLo, dyHi;
                    if (Math.abs(denom) < 1e-9) {
                        // Вертикальные стенки (taper≈0, tilt≈0): ширина постоянна.
                        if (Math.abs(x - c) >= a) continue;
                        dyLo = 0;
                        dyHi = depth;
                    } else {
                        // Границы клина в глубину (линейны): |x − c − k·dy| < a − b·dy.
                        const d1 = (x - c + a) / denom;
                        const d2 = (x - c - a) / denom;
                        dyLo = Math.min(d1, d2);
                        dyHi = Math.max(d1, d2);
                    }
                    if (dyHi < 0) continue;
                    if (dyLo < 0) dyLo = 0;
                    if (dyHi > depth) dyHi = depth;
                    if (dyHi <= dyLo) continue;
                    out.push({ top: thx + dyLo, bottom: thx + dyHi });
                }
            }
        }
        return out;
    }

    // formsSolid — аддитивный вклад 2D-форм (прежнее имя контракта §2.1/§3.1).
    formsSolid(x, y) {
        return this.formsAdditive(x, y);
    }

    // formsAdditive/formsSubtractive — точечная проба знака формы (физика).
    formsAdditive(x, y) {
        if (!this.forms) return false;
        const iv = this._collectFormIntervals(x, undefined);
        for (const it of iv) if (it.add && y >= it.top && y <= it.bottom) return true;
        return false;
    }

    formsSubtractive(x, y) {
        if (!this.forms) return false;
        const iv = this._collectFormIntervals(x, undefined);
        for (const it of iv) if (!it.add && y >= it.top && y <= it.bottom) return true;
        return false;
    }

    // solidAt — единое поле твёрдости (контракт §2.1, порядок §2.2: аддитивные →
    // вычитающие). Нет форм — только база (нулевая цена для прочих биомов).
    // Формы считаются ОДИН раз на пробу (thx + интервалы: иначе дорогой fbm
    // множится на число проверок знака — бюджет генерации чанка, §2.4).
    solidAt(x, y) {
        const forms = this.forms;
        if (!forms) return this.baseSolid(x, y);
        const thx = this.terrainHeight(x);
        const iv = this._collectFormIntervals(x, thx);
        for (const it of iv) if (!it.add && y >= it.top && y <= it.bottom) return false;
        if (this._baseSolidAt(x, y, thx)) return true;
        for (const it of iv) if (it.add && y >= it.top && y <= it.bottom) return true;
        return false;
    }

    // isSolid — алиас solidAt (§2.1): прежнее имя сохранено для потребителей
    // (растр, тесты, инструменты), реализация одна.
    isSolid(x, y) {
        return this.solidAt(x, y);
    }

    // columnSpans — аналитические интервалы БАЗЫ столбца (класс A, §2.3):
    // [{top, bottom}, …] сверху вниз. База — интервал [terrainHeight, +∞); полоса
    // float — отдельный интервал над рельефом; последний интервал ВСЕГДА база.
    // Пещеры — 2D-маска (класс A их не выражает), накладываются поверх. 2D-формы
    // идут ОТДЕЛЬНЫМ путём (`formSpans`, §2.3 — «задетые формой столбцы»), чтобы
    // не ломать контракт базы; единственный источник твёрдости — `solidAt`.
    columnSpans(x) {
        const th = this.terrainHeight(x);
        const spans = [{ top: th, bottom: Infinity }];
        const f = this.formationBlend(x);
        if (f.float) {
            // Границы полосы — те же константы, что у физики (FLOAT_SPAN/GAP):
            // интервал очерчивает диапазон сканирования, твёрдость внутри решает
            // `solidAt` (шум).
            spans.unshift({ top: th - FLOAT_SPAN, bottom: th - FLOAT_GAP, float: true });
        }
        return spans;
    }

    // skyTop — «поверхность неба» (§2.1): МИНИМАЛЬНЫЙ y в пределах растра, где
    // solidAt(x,y)=true (верх твёрдого поля, ВКЛЮЧАЯ висящую плиту arch=1 и полосу
    // float; погода/осадки). Идём от кандидата сверху вниз ДО первой реально твёрдой
    // точки: зарытый козырёк arch=0 (top НИЖЕ th) верх не занижает — над ним всё
    // равно твёрдая корка; вскрытая трещиной корка не даёт ложного «твёрдого»
    // (прежний `it.bottom + 1` мог вернуть воздух). В Э5.2 потребителей НЕ
    // переключаем (якоря — Э5.3); функция — часть контракта.
    skyTop(x) {
        const th = this.terrainHeight(x);
        const iv = this.forms ? this._collectFormIntervals(x, th) : null;
        let top = Infinity;
        // Висящие аддитивные формы: их верх — граница твёрдого (берём, только если
        // точка реально твёрдая: вычитающая форма могла её вскрыть). Зарытый
        // козырёк arch=0 (top НИЖЕ th) станет кандидатом, но его перебьёт твёрдая
        // корка th — верх ниже неё не опускается.
        if (iv) for (const it of iv) {
            if (it.add && it.top < top && this.solidAt(x, it.top)) top = it.top;
        }
        // Полоса float: первое твёрдое вниз от её верхней границы (шум).
        const f = this.formationBlend(x);
        if (f.float) {
            for (let wy = th - FLOAT_SPAN; wy < th - FLOAT_GAP; wy += 1) {
                if (this.solidAt(x, wy)) { if (wy < top) top = wy; break; }
            }
        }
        // Корка поверхности: твёрдая, если её не вскрыла вычитающая форма
        // (Э5.3: void/crater). Вскрыта — первая твёрдая точка вниз от вскрытия
        // (до низа растра, §2.3): прежний `it.bottom + 1` возвращал воздух, если
        // ниже пещера.
        if (this.solidAt(x, th)) {
            if (th < top) top = th;
        } else {
            const bottom = this.baseY - CHUNK_TOP_MARGIN + CHUNK_HEIGHT;
            for (let wy = th; wy <= bottom; wy += 1) {
                if (this.solidAt(x, wy)) { if (wy < top) top = wy; break; }
            }
        }
        return top === Infinity ? th : top;
    }

    // floorY — «пол/опора» (§2.1): верх твёрдого, СВЯЗАННОГО с базой (землёй).
    // Висящие плиты (arch=1) и полоса float в опору НЕ входят (арка — не пол под
    // игроком). КОНТРАКТ: `solidAt(floorY)=true` — вычитающие интервалы
    // включительны, их `bottom` ещё воздух, поэтому точку доводим до твёрдой
    // (Э5.2-трещина косметична, но контракт держим и под `void`/`crater` Э5.3).
    // Для обычной колонки floorY = terrainHeight. Якоря (корабль/фауна/спавн) — Э5.3.
    floorY(x) {
        const th = this.terrainHeight(x);
        if (!this.forms) return th;
        let y = th;
        const iv = this._collectFormIntervals(x, th);
        // Вскрывающие базу вычитающие формы (void/crater, Э5.3) опускают опору за
        // свой низ (интервал включительный: `bottom` — ещё воздух).
        for (const it of iv) {
            if (it.add) continue;
            if (it.top <= y + 1e-9 && it.bottom + 1e-9 > y) y = it.bottom + 1e-9;
        }
        if (this.solidAt(x, y)) return y;
        // Страховка (перекрытие вычитающих форм/пещеры): первая твёрдая точка вниз.
        const bottom = this.baseY - CHUNK_TOP_MARGIN + CHUNK_HEIGHT;
        for (let wy = Math.ceil(y); wy <= bottom; wy += 1) if (this.solidAt(x, wy)) return wy;
        return y;
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
