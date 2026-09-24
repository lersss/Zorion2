// tools/surface-profile-check.mjs
// Э3 «Словарь профилей» — Node-проверка формы рельефа (спека 2026-09-23):
//  - P1 T-budget (§6 п.6/§10 п.7): профиль не выходит за растр чанка;
//  - P2 wave: уклон подветренного склона ≤ leeMaxDeg (модель срыва);
//  - P3 fan: уклон ≤ angleMax;
//  - P4 carve вычитает (вниз) и имеет борт;
//  - P5 viewOnly-слой не влияет на terrainHeight, но влияет на viewHeight;
//  - P6 детерминизм (два прогона — одинаково);
//  - P7 фолбэк без рецепта (FORMATIONS, физика = отрисовка).
// Run: node tools/surface-profile-check.mjs
import { readFileSync } from 'node:fs';
import { SurfaceWorld, primHeight, hash1 } from '../web/static/js/surface/surface_world.js';
import { FORMATIONS } from '../web/static/js/surface/surface_config.js';

const results = [];
function check(name, ok, detail) {
    results.push({ name, ok });
    console.log(`[${ok ? 'PASS' : 'FAIL'}] ${name}${detail ? ' - ' + detail : ''}`);
}

const cat = JSON.parse(readFileSync(new URL('../config/biome_catalog.json', import.meta.url), 'utf8'));

function mergeView(preset, delta) {
    const out = { ...preset };
    for (const k of Object.keys(delta)) {
        const pv = out[k], dv = delta[k];
        if (pv && typeof pv === 'object' && !Array.isArray(pv) && dv && typeof dv === 'object' && !Array.isArray(dv)) {
            out[k] = mergeView(pv, dv);
        } else out[k] = dv;
    }
    return out;
}
function resolveView(biomeId) {
    const b = cat.biomes.find((x) => x.id === biomeId);
    if (!b || !b.view) return null;
    const v = JSON.parse(JSON.stringify(b.view));
    const famId = v.family;
    delete v.family;
    if (!famId) return v;
    const fam = cat.view_families.find((f) => f.id === famId);
    if (!fam) return v;
    const base = JSON.parse(JSON.stringify(fam));
    delete base.id; delete base.name;
    return mergeView(base, v);
}
function mkWorld(biomeId, over = {}) {
    const view = resolveView(biomeId);
    return new SurfaceWorld({ seed: 424242, biome: biomeId, biome_category: 'литосфера', biome_color: '#8a7a6a', life: true, view_source: 'catalog', view_version: 1, biome_view: view, ...over });
}

// Растр чанка: [baseY − CHUNK_TOP_MARGIN, baseY − CHUNK_TOP_MARGIN + CHUNK_HEIGHT].
// Верх ограничен полосой float (§6 п.6), низ — самим растром. Спека §6 п.6 даёт
// для низа `baseY + CHUNK_HEIGHT − 24`, но растр начинается на CHUNK_TOP_MARGIN
// ВЫШЕ baseY — верная граница ниже на этот margin (иначе тест пропускает рельеф
// ниже дна канваса — твёрдая невидимая земля).
const BASEY = 300, TOP_MARGIN = 1200, CHUNK_HEIGHT = 1700, FLOAT_SPAN = 260;
const RASTER_BOTTOM = BASEY - TOP_MARGIN + CHUNK_HEIGHT;   // 800
const MIN_OK = BASEY - TOP_MARGIN + 24 + FLOAT_SPAN;       // −616
const MAX_OK = RASTER_BOTTOM - 24;                         // 776

// P1 — T-budget по 4 образцам. Не один seed: профиль обязан влезать при ЛЮБОМ
// seed (мир детерминирован от seed, но «золотого» seed у планеты нет), поэтому
// свип по набору seed и широкому окну x.
{
    const samples = [
        'пески_пустыни', 'джунгли', 'леса', 'горы',
        // ЧК3 — вулканизм (§4.7.9 п.3: T-budget зелёный на всех 7).
        'лавовые_поля', 'вулканические_поля', 'обсидиановые_поля', 'серные_поля',
        'магмовый_океан', 'венерианские_плоскогорья', 'криовулканические_поля',
    ];
    const SEEDS = [1, 7, 42, 1337, 424242, 987654, 20260923, 55555];
    for (const id of samples) {
        let min = Infinity, max = -Infinity, minAt = 0, minSeed = 0;
        for (const seed of SEEDS) {
            const w = mkWorld(id, { seed });
            for (let x = -40000; x <= 40000; x += 1) {
                const y = w.viewHeight(x);
                if (y < min) { min = y; minAt = x; minSeed = seed; }
                if (y > max) max = y;
            }
        }
        const ok = min >= MIN_OK && max <= MAX_OK;
        check(`P1 T-budget ${id} (${SEEDS.length} seeds)`, ok, `y∈[${min.toFixed(0)}@${minAt}/s${minSeed}, ${max.toFixed(0)}] допустимо [${MIN_OK}, ${MAX_OK}]`);
    }
}

// P2 — wave: уклон ≤ leeMaxDeg.
{
    const l = { prim: 'wave', lambda: [320, 640], amp: [26, 64], skew: 0.85, dir: 1, leeMaxDeg: 34 };
    let maxSlope = 0;
    for (let x = 0; x < 5000; x += 0.5) {
        const dy = Math.abs(primHeight('wave', x + 0.5, l, 12345, 1400) - primHeight('wave', x, l, 12345, 1400));
        maxSlope = Math.max(maxSlope, dy / 0.5);
    }
    const limit = Math.tan(34 * Math.PI / 180);
    check('P2 wave: подветренный уклон ≤ leeMaxDeg(34°)', maxSlope <= limit + 1e-6, `maxSlope=${maxSlope.toFixed(3)} ≤ ${limit.toFixed(3)}`);
}

// P3 — fan: уклон ≤ angleMax.
{
    const l = { prim: 'fan', angleMax: 34, h: [40, 110], roughness: 0.2 };
    let maxSlope = 0;
    for (let x = 0; x < 8000; x += 0.5) {
        const dy = Math.abs(primHeight('fan', x + 0.5, l, 777, 1400) - primHeight('fan', x, l, 777, 1400));
        maxSlope = Math.max(maxSlope, dy / 0.5);
    }
    const limit = Math.tan(34 * Math.PI / 180);
    check('P3 fan: уклон ≤ angleMax(34°)', maxSlope <= limit + 1e-6, `maxSlope=${maxSlope.toFixed(3)} ≤ ${limit.toFixed(3)}`);
}

// P4 — carve вычитает (вниз) и имеет борт (поднятие у краёв).
{
    const l = { prim: 'carve', lambda: [500, 900], depth: [40, 90], w: [120, 220], rim: 0.25, shape: 'U' };
    let maxDown = 0, minUp = 0;
    for (let x = 0; x < 9000; x += 0.5) {
        const y = primHeight('carve', x, l, 555, 1400);
        maxDown = Math.max(maxDown, y);   // вниз (русло)
        minUp = Math.min(minUp, y);       // вверх (борт)
    }
    check('P4a carve: русло вычитает (вниз)', maxDown > 0, `maxDown=${maxDown.toFixed(1)}`);
    check('P4b carve: борт у края приподнят над базовой линией (rim)', minUp < 0, `minUp=${minUp.toFixed(1)}`);
}

// P5 — viewOnly: не влияет на terrainHeight, влияет на viewHeight.
{
    const full = mkWorld('пески_пустыни');
    const noView = mkWorld('пески_пустыни', {
        biome_view: (() => { const v = JSON.parse(JSON.stringify(resolveView('пески_пустыни'))); v.relief.layers = v.relief.layers.filter((l) => !l.viewOnly); return v; })(),
    });
    let same = true, differs = false;
    for (let x = 0; x < 4000; x++) {
        if (full.terrainHeight(x) !== noView.terrainHeight(x)) same = false;
        if (full.viewHeight(x) !== full.terrainHeight(x)) differs = true;
    }
    check('P5a viewOnly не влияет на terrainHeight (физика)', same);
    check('P5b viewOnly влияет на viewHeight (отрисовка)', differs);
}

// P6 — детерминизм.
{
    const a = mkWorld('горы'), b = mkWorld('горы');
    let same = true;
    for (let x = 0; x < 5000; x += 3) if (a.terrainHeight(x) !== b.terrainHeight(x)) { same = false; break; }
    check('P6 детерминизм профиля', same);
}

// P7 — фолбэк без рецепта.
{
    const w = new SurfaceWorld({ seed: 424242, biome: 'x', biome_category: 'литосфера', biome_color: '#8a7a6a', life: true });
    const ok = !w.hasView && w.viewHeight(0) === w.terrainHeight(0) && w.physLayers === null && w.viewLayers === null;
    const f = w.formationBlend(123);
    const inFormations = (FORMATIONS['литосфера'] || []).some((x) => x.id === f.id);
    check('P7 фолбэк без рецепта (FORMATIONS, физика = отрисовка)', ok && inFormations, `id=${f.id}`);
}

// P8 — знак слоя: Δy<0 (вверх) обязан ОПУСКАТЬ y (поднимать землю), Δy>0 — опускать.
// Ловушка, пойманная на Э3: сложение Σlayers внутри отрицаемого `rise` переворачивало
// форму (гребень → впадина, русло → вал) — база вычитается, а слои прибавляются.
{
    const cases = [
        ['crest', 'горы', { prim: 'crest', lambda: [420, 900], amp: [90, 220], sharpness: 0.75, share: 1.0 }],
        ['wave', 'пески_пустыни', { prim: 'wave', lambda: [320, 640], amp: [26, 64], skew: 0.85, dir: 1, leeMaxDeg: 34, share: 1.0 }],
        ['carve', 'джунгли', { prim: 'carve', lambda: [500, 900], depth: [40, 90], w: [120, 220], rim: 0.25, shape: 'U', share: 1.0 }],
    ];
    const withLayers = (biome, layers) => {
        const v = JSON.parse(JSON.stringify(resolveView(biome)));
        v.relief.layers = layers;
        return mkWorld(biome, { biome_view: v });
    };
    for (const [name, biome, l] of cases) {
        const off = withLayers(biome, []);
        const on = withLayers(biome, [l]);
        let ok = true, bad = null, n = 0;
        for (let x = 0; x < 8000; x += 5) {
            const d = primHeight(l.prim, x, l, off.seed, off.region);
            if (Math.abs(d) < 5) continue;                 // у нуля знак неустойчив
            const dy = on.terrainHeight(x) - off.terrainHeight(x);
            n++;
            if (d < 0 ? dy > 0.5 : dy < -0.5) { ok = false; bad = `x=${x} layerDy=${d.toFixed(1)} dTH=${dy.toFixed(1)}`; break; }
        }
        check(`P8 ${name}: Δy<0 поднимает землю (знак слоя не перевёрнут)`, ok && n > 0, bad || `samples=${n}`);
    }
}

// P9 — низ растра: рельеф fallback-биомов (без рецепта, FORMATIONS) не должен
// уходить ниже дна канваса (иначе земля твёрдая, но не нарисована). Рост
// CHUNK_TOP_MARGIN опускает дно — тест ловит регресс рамки на всех категориях.
{
    const seeds = [1, 42, 424242, 987654, 20260923, 7, 1337, 55555, 999, 123456];
    let gMax = -Infinity, who = '';
    for (const [categ] of Object.entries(FORMATIONS)) {
        for (const seed of seeds) {
            const w = new SurfaceWorld({ seed, biome: 'x', biome_category: categ, biome_color: '#8a7a6a', life: false });
            for (let x = -20000; x <= 20000; x += 3) {
                const y = w.terrainHeight(x);
                if (y > gMax) { gMax = y; who = categ; }
            }
        }
    }
    check(`P9 низ растра: fallback-рельеф ≤ ${MAX_OK}`, gMax <= MAX_OK, `max=${gMax.toFixed(0)} (${who})`);
}

// ==================== ЧК3: РАСШИРЕНИЕ ДВИЖКА (§4.7.12–§4.7.13) ====================
// Контрастные пары: новые поля реально читаются; сданные рецепты (без полей)
// дают прежний (legacy) профиль.

// F1 — flow.lobes: несколько языков-лопастей (пиков) внутри периода.
function flowPeaks(lobes) {
    const l = { prim: 'flow', len: 800, w: 200, lobes, slope: 0 };
    let peaks = 0, prev = null, cur = null;
    for (let i = 0; i <= 800; i += 1) {
        const y = primHeight('flow', i, l, 11, 1400);
        if (prev !== null && cur !== null && cur < prev && cur <= y && cur < -1) peaks++;
        prev = cur; cur = y;
    }
    return peaks;
}
{
    const p3 = flowPeaks(3);
    const p1 = flowPeaks(1);
    check('F1a flow.lobes=3 → несколько языков (≥3 пиков)', p3 >= 3, `peaks=${p3}`);
    check('F1b flow.lobes=1 → один язык (1 пик)', p1 === 1, `peaks=${p1}`);
}

// F2 — flow.slope: знак задаёт сторону срыва, модуль — крутизну; 0 — симметрия.
{
    const mk = (slope) => ({ prim: 'flow', len: 800, w: 200, lobes: 1, slope });
    const mid = 400;
    const lVal = primHeight('flow', mid - 60, mk(0.5), 11, 1400);
    const rVal = primHeight('flow', mid + 60, mk(0.5), 11, 1400);
    check('F2a flow.slope>0: левый склон положе (выше) правого', lVal < rVal, `L=${lVal.toFixed(1)} R=${rVal.toFixed(1)}`);
    let mirror = true;
    for (const f of [0.1, 0.3, 0.45, 0.6, 0.8]) {
        const a = primHeight('flow', f * 800, mk(0.5), 11, 1400);
        const b = primHeight('flow', (1 - f) * 800, mk(-0.5), 11, 1400);
        if (Math.abs(a - b) > 1e-9) { mirror = false; break; }
    }
    check('F2b flow.slope: знак зеркалит язык (f ↔ 1−f)', mirror);
    let sym = true;
    for (const f of [0.1, 0.3, 0.45, 0.6, 0.8]) {
        const a = primHeight('flow', f * 800, mk(0), 11, 1400);
        const b = primHeight('flow', (1 - f) * 800, mk(0), 11, 1400);
        if (Math.abs(a - b) > 1e-9) { sym = false; break; }
    }
    check('F2c flow.slope=0 → симметричный вал', sym);
}

// F3 — flow.levees: боковые валы-гребни у краёв потока.
{
    const base = { prim: 'flow', len: 800, w: 200, lobes: 2, slope: 0 };
    const withLev = { ...base, levees: 0.3 };
    const y0 = primHeight('flow', 0, base, 11, 1400);
    const y1 = primHeight('flow', 0, withLev, 11, 1400);
    check('F3a flow.levees: вал-гребень у края потока (f=0)', y1 < y0 - 1, `без=${y0.toFixed(1)} с=${y1.toFixed(1)}`);
    const c0 = primHeight('flow', 200, base, 11, 1400);
    const c1 = primHeight('flow', 200, withLev, 11, 1400);
    check('F3b flow.levees не ломает язык в центре', c1 <= c0 + 1e-9, `без=${c0.toFixed(1)} с=${c1.toFixed(1)}`);
}

// F4 — spike.cluster: иглы группируются в кусты, а не стоят поодиночке.
{
    const mk = (cluster) => ({ prim: 'spike', perRegion: 9, h: [40, 70], w: 20, taper: 0.2, cluster });
    const peaks = (l) => {
        const out = [];
        let prev = null, cur = null;
        for (let i = 0; i < 1400; i++) {
            const y = primHeight('spike', i, l, 77, 1400);
            if (prev !== null && cur !== null && cur < prev && cur <= y && cur < -1) out.push(i);
            prev = cur; cur = y;
        }
        return out;
    };
    // Доля игл с близким соседом (куст): в кустах высокая, у одиночек — нулевая.
    const closeFrac = (xs) => {
        let c = 0;
        for (let i = 0; i < xs.length; i++) {
            let best = Infinity;
            for (let j = 0; j < xs.length; j++) if (j !== i) best = Math.min(best, Math.abs(xs[i] - xs[j]));
            if (best < 50) c++;
        }
        return xs.length ? c / xs.length : 0;
    };
    const fc = closeFrac(peaks(mk(true)));
    const fu = closeFrac(peaks(mk(false)));
    check('F4a spike.cluster: большинство игл в кустах (близкий сосед)', fc > 0.5, `cluster=${fc.toFixed(2)}`);
    check('F4b spike без cluster: иглы поодиночке (близких соседей вдвое меньше)', fu < fc / 2, `uniform=${fu.toFixed(2)}`);
}

// F5 — fan.w: желаемая минимальная ширина основания; угол не круче angleMax.
{
    const tan = (deg) => Math.tan(deg * Math.PI / 180);
    const support = (l) => { let n = 0; for (let x = 0; x < 1400; x += 0.5) if (primHeight('fan', x, l, 5, 1400) < -0.01) n += 0.5; return n; };
    const noW = { prim: 'fan', angleMax: 34, h: 40 };
    const withW = { prim: 'fan', angleMax: 34, h: 40, w: 400 };
    check('F5a fan.w расширяет основание', support(withW) > support(noW) + 100, `без=${support(noW).toFixed(0)} с=${support(withW).toFixed(0)}`);
    const maxSlope = (l) => { let s = 0; for (let x = 0; x < 3000; x += 0.25) s = Math.max(s, Math.abs(primHeight('fan', x + 0.25, l, 5, 1400) - primHeight('fan', x, l, 5, 1400)) / 0.25); return s; };
    check('F5b fan.w: угол не круче angleMax', maxSlope(withW) <= tan(34) + 1e-6, `maxSlope=${maxSlope(withW).toFixed(3)} ≤ ${tan(34).toFixed(3)}`);
    check('F5c fan.w+roughness: угол не круче angleMax', maxSlope({ ...withW, roughness: 0.5 }) <= tan(34) + 1e-6, `maxSlope=${maxSlope({ ...withW, roughness: 0.5 }).toFixed(3)}`);
}

// F6 — fan.roughness применяется только при явном w (гейт §4.7.13).
{
    const l = (roughness, w) => ({ prim: 'fan', angleMax: 32, h: 50, ...(w != null ? { w } : {}), ...(roughness != null ? { roughness } : {}) });
    const sample = (x) => { const a = []; for (let i = 0; i < 2000; i += 3) a.push(primHeight('fan', i, x, 9, 1400)); return a; };
    const diff = (a, b) => a.some((v, i) => Math.abs(v - b[i]) > 1e-9);
    check('F6a fan.roughness при явном w читается', diff(sample(l(0.4, 400)), sample(l(0, 400))));
    check('F6b fan.roughness без w НЕ читается (legacy, §4.7.13)', !diff(sample(l(0.4)), sample(l(0))));
    check('F6c fan без w: roughness=0.4 == legacy-формула', !diff(sample(l(0.4)), sample(l(null))));
}

// R1 — сданные слои (fan без w / spike без cluster / flow без новых полей)
// дают прежний профиль: сверка с legacy-формулами (§4.7.9 п.14).
function rangeAt(v, i, seed, def) {
    if (typeof v === 'number') return v;
    if (Array.isArray(v) && v.length === 2) return v[0] + hash1(i, seed) * (v[1] - v[0]);
    return def;
}
function legacyFan(x, l, seed, region) {
    const R = region || 1400;
    const angleMax = typeof l.angleMax === 'number' ? l.angleMax : 34;
    const tanA = Math.tan(angleMax * Math.PI / 180);
    const base = Math.floor(x / R);
    let sum = 0;
    for (let rr = base - 1; rr <= base + 1; rr++) {
        const h = rangeAt(l.h, rr, seed ^ 0x21, 60);
        const halfW = Math.max(8, h / tanA);
        const c = rr * R + hash1(rr, seed ^ 0x23) * R;
        const d = Math.abs(x - c);
        if (d <= halfW) sum -= h * (1 - d / halfW);
    }
    return sum;
}
function legacySpike(x, l, seed, region) {
    const R = region || 1400;
    const cntRaw = l.perRegion != null ? l.perRegion : l.count;
    const taper = typeof l.taper === 'number' ? l.taper : 0;
    const base = Math.floor(x / R);
    let sum = 0;
    for (let rr = base - 1; rr <= base + 1; rr++) {
        const n = Math.max(0, Math.round(rangeAt(cntRaw, rr, seed ^ 0xd0, 1)));
        for (let k = 0; k < n; k++) {
            const idx = rr * 131 + k;
            const c = rr * R + hash1(idx, seed ^ 0xd1) * R;
            const h = rangeAt(l.h, idx, seed ^ 0xd2, 120);
            const w = rangeAt(l.w, idx, seed ^ 0xd3, 40);
            const d = Math.abs(x - c);
            if (d < w) sum -= h * Math.pow(1 - d / w, 1 + 2 * taper);
        }
    }
    return sum;
}
function legacyFlow(x, l, seed) {
    const len = rangeAt(l.len, 0, seed ^ 0x31, 300);
    const w = rangeAt(l.w, 0, seed ^ 0x32, 120);
    const n = (() => { // fbm1(x/len, seed^0x33, 1)
        const X = x / len, s = seed ^ 0x33;
        const i = Math.floor(X), f = X - i;
        const a = hash1(i, s), b = hash1(i + 1, s);
        const vt = f * f * (3 - 2 * f);
        return a + (b - a) * vt;
    })();
    return -w * 0.25 * (2 * n - 1);
}
{
    const layers = [];
    for (const f of cat.view_families) if (f.relief && f.relief.layers) for (const l of f.relief.layers) layers.push(['family:' + f.id, l]);
    for (const b of cat.biomes) if (b.view && b.view.relief && b.view.relief.layers) for (const l of b.view.relief.layers) layers.push([b.id, l]);
    let checked = 0, bad = null;
    for (const [where, l] of layers) {
        for (let x = -3000; x < 3000 && !bad; x += 7) {
            if (l.prim === 'fan' && l.w == null) {
                if (Math.abs(primHeight('fan', x, l, 12345, 1400) - legacyFan(x, l, 12345, 1400)) > 1e-9) bad = `${where}: fan`;
                checked++;
            } else if (l.prim === 'spike' && !l.cluster) {
                if (Math.abs(primHeight('spike', x, l, 12345, 1400) - legacySpike(x, l, 12345, 1400)) > 1e-9) bad = `${where}: spike`;
                checked++;
            } else if (l.prim === 'flow' && l.lobes == null && l.slope == null && l.levees == null) {
                if (Math.abs(primHeight('flow', x, l, 12345, 1400) - legacyFlow(x, l, 12345, 1400)) > 1e-9) bad = `${where}: flow`;
                checked++;
            }
        }
        if (bad) break;
    }
    check('R1 сданные слои (fan без w / spike без cluster / flow) = прежний профиль', !bad, bad || `проверок: ${checked}`);
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exitCode = 1;
