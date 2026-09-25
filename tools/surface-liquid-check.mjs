// tools/surface-liquid-check.mjs
// ЧК6 «Мир прогулки: вода» — слой жидкости (спека 2026-09-25 §3/§5). Проверяет
// контракт `liquidLevel`/`bedY`/`liquidAt`/`solidAt'` и режимы уровня:
//   * L1 global — зеркало `baseY + offset`, тело воды, сухой берег;
//   * L2 жидкость ≠ твёрдость (`liquidAt` ∩ `solidAt = ∅`);
//   * L3 basin — уровень впадины (не плоское зеркало), есть вода и сушь;
//   * L4 underIce — корка (`solidAt' = solidAt ∨ crust`), `floorY` = верх корки,
//     `bedY` без корки (N1), полыньи `w ≥ PLAYER_W+2` и детерминизм от seed;
//   * L5 среды (метан/co2/аммиак/лава), L6 фолбэк без жидкости, детерминизм.
// Run: node tools/surface-liquid-check.mjs
import { readFileSync } from 'node:fs';
import { SurfaceWorld } from '../web/static/js/surface/surface_world.js';
import { PLAYER_W } from '../web/static/js/surface/surface_config.js';

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
        if (pv && typeof pv === 'object' && !Array.isArray(pv) && dv && typeof dv === 'object' && !Array.isArray(dv)) out[k] = mergeView(pv, dv);
        else out[k] = dv;
    }
    return out;
}
function resolveView(id) {
    const b = cat.biomes.find((x) => x.id === id);
    if (!b || !b.view) return null;
    const v = JSON.parse(JSON.stringify(b.view));
    const famId = v.family; delete v.family;
    if (!famId) return v;
    const fam = (cat.view_families || []).find((f) => f.id === famId);
    if (!fam) return v;
    const base = JSON.parse(JSON.stringify(fam)); delete base.id; delete base.name;
    return mergeView(base, v);
}
function mkWorld(id, over = {}) {
    const b = cat.biomes.find((x) => x.id === id);
    const view = resolveView(id);
    return new SurfaceWorld({
        seed: 424242, biome: id, biome_category: b ? b.category : 'вода', biome_color: (b && b.color) || '#8a7a6a',
        life: true, view_source: 'catalog', view_version: 1, biome_view: view,
        liquid: view && view.liquid, liquid_source: 'explicit', ...over,
    });
}
function wetColumns(w, from, to, step = 1) {
    const out = [];
    for (let x = from; x < to; x += step) if (w.liquidDepth(x) > 0) out.push(x);
    return out;
}

// ==================== L1: global — зеркало и тело ====================
{
    const w = mkWorld('океаны');
    check('L1a global: зеркало = baseY + offset (константа)', w.liquidLevel(0) === w.baseY + w.liquid.level.offset && w.liquidLevel(1234) === w.baseY + w.liquid.level.offset);
    const wet = wetColumns(w, 0, 4000);
    check('L1b global: есть затопленные колонки', wet.length > 0, `wet=${wet.length}/4000`);
    const x = wet[0];
    const lv = w.liquidLevel(x);
    check('L1c liquidAt в теле — среда', w.liquidAt(x, lv + 1) === 'вода');
    check('L1d bedY ≥ liquidLevel на затопленной колонке', w.bedY(x) >= lv, `bd=${w.bedY(x).toFixed(1)} lv=${lv.toFixed(1)}`);
    const dry = [];
    for (let i = 0; i < 4000; i++) if (w.liquidDepth(i) === 0) dry.push(i);
    check('L1e сухой берег: liquidAt ниже зеркала — нет жидкости', dry.length > 0 && w.liquidAt(dry[0], w.liquidLevel(dry[0]) + 1) === '');
}

// ==================== L2: жидкость ≠ твёрдость ====================
{
    const w = mkWorld('океаны');
    let tested = 0, bad = 0;
    for (const x of wetColumns(w, 0, 3000)) {
        const lv = w.liquidLevel(x);
        for (let y = lv; y <= w.bedY(x); y += 2) {
            tested++;
            if (w.solidAt(x, y) && w.liquidAt(x, y) !== '') bad++;
        }
        if (tested > 4000) break;
    }
    check('L2a liquidAt пуст внутри твёрдого (вода не в камне)', tested > 0 && bad === 0, `точек=${tested} нарушений=${bad}`);
}

// ==================== L3: basin — уровень впадины ====================
{
    const w = mkWorld('озёра_реки');
    check('L3a basin: режим уровня basin', w.liquid.level.mode === 'basin');
    const levels = [];
    for (let x = 0; x < 8000; x += 1) levels.push(w.liquidLevel(x));
    const min = Math.min(...levels), max = Math.max(...levels);
    check('L3b basin: уровень НЕ плоский (отличается от глобального зеркала)', max - min > 5, `lv∈[${min.toFixed(0)},${max.toFixed(0)}]`);
    const wet = wetColumns(w, 0, 8000);
    const dry = [];
    for (let x = 0; x < 8000; x++) if (w.liquidDepth(x) === 0) dry.push(x);
    check('L3c basin: есть вода (впадины) и суша (возвышенности)', wet.length > 0 && dry.length > 0, `wet=${wet.length} dry=${dry.length}`);
    const x = wet[0];
    check('L3d basin: liquidAt в теле — среда', w.liquidAt(x, w.liquidLevel(x) + 1) === 'вода');
}

// ==================== L4: underIce — корка и полыньи ====================
{
    const w = mkWorld('подлёдные_океаны');
    check('L4a underIce: режим underIce, корка активна', w.liquid.level.mode === 'underIce' && w._liquidCrust === true);
    const lv = w.liquidLevel(0);
    // Первая колонка с коркой.
    let crustX = -1;
    for (let x = 0; x < 6000; x++) if (w.crustTop(x) !== null) { crustX = x; break; }
    check('L4b underIce: корка есть на затопленной колонке', crustX >= 0, `x=${crustX}`);
    if (crustX >= 0) {
        const top = w.crustTop(crustX);
        check('L4c корка твёрдая (по ней ходят): solidAt(верх корки)', w.solidAt(crustX, top + 1) === true, `top=${top.toFixed(1)}`);
        check('L4d floorY = верх корки (опора)', Math.abs(w.floorY(crustX) - top) < 1e-9, `floorY=${w.floorY(crustX).toFixed(1)}`);
        check('L4e bedY БЕЗ корки (N1): дно ниже зеркала, корка — выше',
            w.bedY(crustX) > lv && w.floorY(crustX) < lv,
            `bd=${w.bedY(crustX).toFixed(1)} lv=${lv.toFixed(1)} floorY=${w.floorY(crustX).toFixed(1)}`);
        check('L4f под коркой жидкость: liquidAt в теле', w.liquidAt(crustX, lv + 1) === 'вода' || w.bedY(crustX) <= lv);
        check('L4g корка — не вода: liquidAt в точке корки пуст', w.liquidAt(crustX, top + 1) === '');
    }
    // Полыньи: окна без корки, ширина ≥ PLAYER_W+2, детерминированы.
    const poly = [];
    for (let x = 0; x < 12000; x++) if (w.polynyaAt(x)) poly.push(x);
    check('L4h полыньи есть', poly.length > 0, `точек полыней=${poly.length}`);
    // Непрерывные окна.
    const runs = [];
    let s = null;
    for (let x = 0; x < 12000; x++) {
        if (w.polynyaAt(x)) { if (s === null) s = x; } else if (s !== null) { runs.push([s, x - 1]); s = null; }
    }
    const widths = runs.map(([a, b]) => b - a + 1);
    const minW = widths.length ? Math.min(...widths) : 0;
    check('L4i ширина полыньи ≥ PLAYER_W+2', widths.length > 0 && minW >= PLAYER_W + 2, `окон=${runs.length} мин.ширина=${minW} (нужно ≥ ${PLAYER_W + 2})`);
    const maxW = widths.length ? Math.max(...widths) : 0;
    check('L4j ширина полыньи ≤ w_max данных', maxW <= Math.max(...w.liquid.level.polynya.w) + 1, `макс.ширина=${maxW}`);
    // В полынье корки нет, опора — грунт (не корка).
    if (runs.length) {
        const px = Math.floor((runs[0][0] + runs[0][1]) / 2);
        check('L4k в полынье корки нет', w.crustTop(px) === null && w.polynyaAt(px) === true);
    }
    // Детерминизм от seed.
    const w2 = mkWorld('подлёдные_океаны', { seed: 424242 });
    let same = true;
    for (let x = 0; x < 12000; x += 3) if (w.polynyaAt(x) !== w2.polynyaAt(x)) { same = false; break; }
    check('L4l полыньи детерминированы (тот же seed → те же окна)', same);
    const w3 = mkWorld('подлёдные_океаны', { seed: 987654 });
    let differs = false;
    for (let x = 0; x < 12000; x += 3) if (w.polynyaAt(x) !== w3.polynyaAt(x)) differs = true;
    check('L4m другой seed → другие полыньи', differs);
}

// ==================== L5: среды ====================
{
    const cases = [
        ['метановые_моря', 'метан'], ['углеводородные_равнины', 'метан'],
        ['аммиачные_крио-океаны', 'аммиак'], ['co2_океаны', 'co2'], ['магмовый_океан', 'лава'],
    ];
    for (const [id, medium] of cases) {
        const w = mkWorld(id);
        check(`L5 ${id}: среда ${medium}`, w.liquid && w.liquid.medium === medium, `medium=${w.liquid && w.liquid.medium}`);
    }
    const lava = mkWorld('магмовый_океан');
    check('L5b лава самосветится (glow задан)', !!lava.liquid.glow, `glow=${lava.liquid.glow}`);
}

// ==================== L6: фолбэк без жидкости ====================
{
    for (const id of ['струнные_рощи', 'коралловые_рифы', 'пещерный_мир_с_потолком', 'инеевые_рощи']) {
        const w = mkWorld(id);
        const ok = !w.liquid && w.liquidLevel(0) === Infinity && w.liquidAt(0, 100) === '' && w.liquidDepth(0) === 0;
        check(`L6 ${id}: жидкости нет (фолбэк 1:1)`, ok);
    }
}

// ==================== L7: детерминизм уровня basin ====================
{
    const a = mkWorld('озёра_реки', { seed: 42 });
    const b = mkWorld('озёра_реки', { seed: 42 });
    let same = true;
    for (let x = 0; x < 4000; x += 7) if (a.liquidLevel(x) !== b.liquidLevel(x)) { same = false; break; }
    check('L7 basin-уровень детерминирован (тот же seed)', same);
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exit(1);
