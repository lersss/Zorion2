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
import { SurfaceWorld, primHeight } from '../web/static/js/surface/surface_world.js';
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
    const samples = ['пески_пустыни', 'джунгли', 'леса', 'горы'];
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

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exitCode = 1;
