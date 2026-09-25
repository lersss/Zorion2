// tools/surface-boat-check.mjs
// ЧК6.3 «Мир прогулки: лодка» (спека 2026-09-26-мир-прогулки-лодка.md §3–§4, §7).
// Детерминированная покадровая симуляция `Boat`/`Player` (образец —
// `tools/surface-swim-check.mjs`). Проверяет:
//   * B1  посадка у зеркала (вплавь, dx = 0);
//   * B2  идемпотентность (второй `use` — выход, не вторая лодка);
//   * B3  y = liquidLevel − BOAT_HULL;
//   * B4  скорость ≈ WALK·BOAT_FACTOR, спринт не ускоряет;
//   * B5  снос: BOAT_DRAG < BOAT_ACCEL, затухание без ввода;
//   * B6  `down` не ныряет (y не ниже уровня);
//   * B7  мягкий стоп у края зеркала/твёрдого (полный силуэт, без проникновения);
//   * B8  запрет лавы и `underIce` (включая полыньи);
//   * B9  узкая вода (< BOAT_W) — посадки нет;
//   * B10 выход → aboard=false, следующий кадр Player.update (плавучесть);
//   * B11 регресс: вне лодки Player.update не изменился (легаси-путей нет);
//   * B12 детерминизм (тот же вход → тот же x/vx);
//   * B13 посадка с пологого берега (dx ≠ 0, центральная колонка сухая);
//   * B14 `player.distance` растёт при ходе (HUD не замирает).
// Run: node tools/surface-boat-check.mjs
import { readFileSync } from 'node:fs';
import { SurfaceWorld } from '../web/static/js/surface/surface_world.js';
import { Player } from '../web/static/js/surface/surface_player.js';
import { Boat } from '../web/static/js/surface/surface_boat.js';
import { PPM, WALK_SPEED, SWIM_FACTOR, PLAYER_H, BOAT_HULL, BOAT_FACTOR, BOAT_ACCEL, BOAT_DRAG } from '../web/static/js/surface/surface_config.js';

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
function longestWetRun(w, minDepth) {
    let best = null, s = -1;
    const end = 16000;
    for (let x = 0; x <= end; x++) {
        const wet = x < end && w.liquidDepth(x) >= minDepth;
        if (wet) { if (s < 0) s = x; }
        else if (s >= 0) {
            const len = x - s;
            if (!best || len > best.len) best = { s, e: x - 1, len, mid: Math.floor((s + x - 1) / 2) };
            s = -1;
        }
    }
    return best;
}
function putInWater(w, x, below = 30) {
    const p = new Player(w, 1);
    p.x = x; p.vx = 0; p.vy = 0; p.onGround = false;
    p.y = w.liquidLevel(x) + below;
    return p;
}
const NO = { left: false, right: false, jump: false, sprint: false, down: false };

// Синтетический мир для точных границ (§3.3): только интерфейс, что читает Boat.
// `dry` — колонки без жидкости (мельче minDepth), но не твёрдые; `solid` — стена.
const LV = 360;
function fakeWorld({ lv = LV, dry = [], wetLo = 0, wetHi = 4000, depth = 40, solid = null, medium = 'вода', mode = 'global' } = {}) {
    const drySet = new Set(dry);
    return {
        liquid: { medium, level: { mode } },
        liquidLevel(x) { return (x >= wetLo && x < wetHi) ? lv : Infinity; },
        _liquidCol(x) { if (x < wetLo || x >= wetHi) return null; if (drySet.has(x)) return null; return { lv, depth, bd: lv + depth }; },
        crustTop() { return null; },
        crustAt() { return false; },
        solidAt(x, y) { return solid ? solid(x, y) : false; },
    };
}
function stubPlayer(x, y) {
    return { x, y, h: PLAYER_H, vx: 0, vy: 0, facing: 1, distance: 0 };
}

// ==================== B1: посадка у зеркала (вплавь, dx = 0) ====================
const ocean = mkWorld('океаны');
const oceanRun = longestWetRun(ocean, 40);
const oceanX = oceanRun ? oceanRun.mid : 0;
{
    const p = putInWater(ocean, oceanX, 7);
    const boat = new Boat(ocean);
    const dx = boat.canBoard(ocean, p);
    check('B1a посадка у зеркала вплавь (dx = 0)', dx === 0, `canBoard=${dx} x=${oceanX} run=${oceanRun ? oceanRun.len : 0}`);
    const ok = boat.board(p);
    check('B1b board: игрок совмещён с boat.x, aboard', ok === true && boat.aboard === true && p.x === boat.x,
        `aboard=${boat.aboard} boat.x=${boat.x.toFixed(1)} player.x=${p.x.toFixed(1)}`);
}

// ==================== B2: идемпотентность ====================
{
    const p = putInWater(ocean, oceanX, 7);
    const boat = new Boat(ocean);
    boat.board(p);                       // use #1 — посадка
    const first = boat;
    const aboardAfterSecond = (() => { boat.disembark(p); return boat.aboard; })();  // use #2 — выход
    boat.board(p);                       // use #3 — снова посадка
    check('B2 второй use — выход (одна лодка, флаг aboard)',
        first === boat && aboardAfterSecond === false && boat.aboard === true,
        `sameObject=${first === boat} afterDisembark=${aboardAfterSecond} aboard=${boat.aboard}`);
}

// ==================== B3: уровень ====================
{
    const p = putInWater(ocean, oceanX, 7);
    const boat = new Boat(ocean);
    boat.board(p);
    boat.update(1 / 60, { ...NO, right: true }, p);
    const lv = ocean.liquidLevel(boat.x);
    check('B3 y = liquidLevel(x) − BOAT_HULL', Math.abs(p.y - (lv - BOAT_HULL)) < 1e-6 && p.vy === 0,
        `y=${p.y.toFixed(2)} lv−HULL=${(lv - BOAT_HULL).toFixed(2)}`);
}

// ==================== B4: скорость и спринт ====================
{
    const boat = new Boat(ocean);
    const p = putInWater(ocean, oceanX, 7);
    boat.board(p);
    let vmax = 0;
    for (let i = 0; i < 120; i++) { boat.update(1 / 60, { ...NO, right: true }, p); vmax = Math.max(vmax, Math.abs(boat.vx)); }
    const target = WALK_SPEED * PPM * BOAT_FACTOR;
    check('B4a скорость ≈ WALK·BOAT_FACTOR', Math.abs(vmax - target) < 3, `vmax=${vmax.toFixed(1)} ждём ${target.toFixed(0)}`);
    const boat2 = new Boat(ocean);
    const p2 = putInWater(ocean, oceanX, 7);
    boat2.board(p2);
    let vmax2 = 0;
    for (let i = 0; i < 120; i++) { boat2.update(1 / 60, { ...NO, right: true, sprint: true }, p2); vmax2 = Math.max(vmax2, Math.abs(boat2.vx)); }
    check('B4b спринт в лодке не ускоряет', Math.abs(vmax2 - vmax) < 0.5, `sprint=${vmax2.toFixed(1)} обычная=${vmax.toFixed(1)}`);
}

// ==================== B5: снос ====================
{
    check('B5a BOAT_DRAG < BOAT_ACCEL', BOAT_DRAG < BOAT_ACCEL, `drag=${BOAT_DRAG} accel=${BOAT_ACCEL}`);
    const boat = new Boat(ocean);
    const p = putInWater(ocean, oceanX, 7);
    boat.board(p);
    for (let i = 0; i < 120; i++) boat.update(1 / 60, { ...NO, right: true }, p);
    const v0 = boat.vx;
    let frames = 0;
    for (let i = 0; i < 600; i++) { boat.update(1 / 60, NO, p); frames++; if (Math.abs(boat.vx) < 1e-6) break; }
    check('B5b без ввода затухает до нуля (снос плавный)', v0 > 100 && boat.vx === 0 && frames > 1,
        `v0=${v0.toFixed(1)} → 0 за ${frames} кадров`);
}

// ==================== B6: down не ныряет ====================
{
    const boat = new Boat(ocean);
    const p = putInWater(ocean, oceanX, 7);
    boat.board(p);
    let minY = Infinity, lvRef = 0;
    for (let i = 0; i < 120; i++) {
        boat.update(1 / 60, { ...NO, down: true, jump: true }, p);
        const lv = ocean.liquidLevel(boat.x);
        lvRef = lv;
        minY = Math.min(minY, p.y - (lv - BOAT_HULL));
    }
    check('B6 down/jump в лодке — no-op (y не ныряет)', Math.abs(minY) < 1e-6 && p.y < lvRef,
        `Δ(y, lv−HULL)=${minY.toFixed(4)} y=${p.y.toFixed(1)} lv=${lvRef}`);
}

// ==================== B7: мягкий стоп ====================
{
    const w = fakeWorld({ wetLo: 0, wetHi: 1500, depth: 40 });
    const boat = new Boat(w);
    boat.aboard = true; boat.x = 1000; boat.vx = 0;
    for (let i = 0; i < 600; i++) boat.update(1 / 60, { ...NO, right: true }, stubPlayer(1000, LV - BOAT_HULL));
    check('B7a край зеркала: x не уходит на сушу, vx = 0',
        boat.vx === 0 && boat.x + 17 < 1500 && boat.x > 1400,
        `x=${boat.x.toFixed(2)} x+17=${(boat.x + 17).toFixed(2)} vx=${boat.vx}`);
    const ws = fakeWorld({ wetLo: 0, wetHi: 3000, depth: 40, solid: (x, y) => x >= 1500 && y <= LV - 5 });
    const bs = new Boat(ws);
    bs.aboard = true; bs.x = 1000; bs.vx = 0;
    let penetrated = false;
    for (let i = 0; i < 600; i++) {
        bs.update(1 / 60, { ...NO, right: true }, stubPlayer(1000, LV));
        for (let o = -17; o <= 17; o += 4) { if (ws.solidAt(bs.x + o, LV - 10)) { penetrated = true; break; } }
    }
    check('B7b твёрдое (свод): полный силуэт не входит, стоп перед стеной',
        !penetrated && bs.vx === 0 && bs.x + 17 < 1500,
        `x=${bs.x.toFixed(2)} penetrated=${penetrated}`);
}

// ==================== B8: запрет лавы и underIce ====================
{
    const lava = mkWorld('магмовый_океан');
    const lrun = longestWetRun(lava, 40);
    const lx = lrun ? lrun.mid : 0;
    const lp = putInWater(lava, lx, 7);
    const lb = new Boat(lava);
    check('B8a на лаве посадки нет', lb.canBoard(lava, lp) === null && !lb.board(lp),
        `medium=${lava.liquid.medium} canBoard=${lb.canBoard(lava, lp)}`);

    const ice = mkWorld('подлёдные_океаны');
    let ix = -1;
    for (let x = 8; x < 40000 - 8; x++) {
        if (!ice._liquidCol(x)) continue;
        let all = true;
        for (let d = -7; d <= 7; d++) if (!ice.polynyaAt(x + d)) { all = false; break; }
        if (all) { ix = x; break; }
    }
    const ip = new Player(ice, 1);
    ip.x = ix >= 0 ? ix : ice.spawnX; ip.y = ice.liquidLevel(ip.x) + 7; ip.vx = 0; ip.vy = 0; ip.onGround = false;
    const ib = new Boat(ice);
    check('B8b на underIce (в т.ч. в полынье) посадки нет',
        ib.canBoard(ice, ip) === null,
        `mode=${ice.liquid.level.mode} polynya x=${ix} canBoard=${ib.canBoard(ice, ip)}`);
    const fm = fakeWorld({ mode: 'underIce' });
    check('B8c режим underIce запрещает лодку (синтетика)', new Boat(fm).canBoard(fm, stubPlayer(1000, LV)) === null);
}

// ==================== B9: узкая вода ====================
{
    const w = fakeWorld({ wetLo: 100, wetHi: 124, depth: 40 });
    const boat = new Boat(w);
    check('B9 узкая вода (span < BOAT_W) — посадки нет', boat.canBoard(w, stubPlayer(112, LV)) === null,
        `span=${124 - 100} BOAT_W=34`);
}

// ==================== B10: выход → плавучесть ====================
{
    const boat = new Boat(ocean);
    const p = putInWater(ocean, oceanX, 7);
    boat.board(p);
    boat.disembark(p);
    const afterOut = !boat.aboard;
    for (let i = 0; i < 240; i++) p.update(1 / 60, NO);
    const lv = ocean.liquidLevel(p.x);
    const head = p.y - p.h / 2;
    check('B10 выход → aboard=false, следующий кадр Player.update (плавучесть)',
        afterOut && head < lv - 0.5 && Math.abs(p.y - ocean.floatY(p.x)) < 6,
        `head=${head.toFixed(1)} lv=${lv.toFixed(1)} y=${p.y.toFixed(1)} floatY=${ocean.floatY(p.x).toFixed(1)}`);
}

// ==================== B11: регресс (вне лодки Player.update не изменился) ====================
{
    const src = readFileSync(new URL('../web/static/js/surface/surface_player.js', import.meta.url), 'utf8');
    check('B11a surface_player.js не знает о лодке (обход Player.update)',
        !/boat/i.test(src) && !/BOAT_/.test(src), 'нет ссылок на Boat/BOAT_');
    const pw = putInWater(ocean, oceanX, 30);
    let swimV = 0;
    for (let i = 0; i < 180; i++) { pw.update(1 / 60, { ...NO, right: true }); swimV = Math.max(swimV, Math.abs(pw.vx)); }
    check('B11b плавание (ЧК6.2) = WALK·SWIM_FACTOR',
        Math.abs(swimV - WALK_SPEED * PPM * SWIM_FACTOR) < 3, `v=${swimV.toFixed(1)}`);
    const dry = mkWorld('инеевые_рощи');
    let dxc = 0;
    for (let x = 0; x < 600; x++) {
        const p = new Player(dry, 1);
        p.x = x; p.y = dry.floorY(x) - p.h / 2 - 2; p.vx = 0; p.vy = 0; p.onGround = true;
        for (let i = 0; i < 40; i++) p.update(1 / 60, { ...NO, right: true });
        if (p.x - x > 8) { dxc = x; break; }
    }
    const pd = new Player(dry, 1);
    pd.x = dxc; pd.y = dry.floorY(dxc) - pd.h / 2 - 2; pd.vx = 0; pd.vy = 0; pd.onGround = true;
    let walkV = 0;
    for (let i = 0; i < 180; i++) { pd.update(1 / 60, { ...NO, right: true }); walkV = Math.max(walkV, Math.abs(pd.vx)); }
    check('B11c ходьба на суше = WALK_SPEED', Math.abs(walkV - WALK_SPEED * PPM) < 3, `v=${walkV.toFixed(1)}`);
}

// ==================== B12: детерминизм ====================
{
    function run() {
        const w = fakeWorld({ wetLo: 0, wetHi: 2000, depth: 40 });
        const boat = new Boat(w);
        boat.aboard = true; boat.x = 200; boat.vx = 0;
        const p = stubPlayer(200, LV - BOAT_HULL);
        for (let i = 0; i < 120; i++) boat.update(1 / 60, { ...NO, right: true }, p);
        return { x: boat.x, vx: boat.vx, d: p.distance };
    }
    const a = run(), b = run();
    check('B12 детерминизм (тот же вход → тот же x/vx)', a.x === b.x && a.vx === b.vx && a.d === b.d,
        `x=${a.x.toFixed(3)}==${b.x.toFixed(3)} vx=${a.vx}==${b.vx}`);
}

// ==================== B13: посадка с пологого берега ====================
{
    // Центральная колонка игрока (1000) сухая (мельче minDepth); борта лодки — на
    // мокрых колонках. Сухая полоса смещена так, что dx=0 невозможен (сухая колонка
    // попадает на борт), а сдвиг ставит её под центральное ложе → dx ≠ 0.
    const w = fakeWorld({ dry: [1000, 1012, 1013, 1014, 1015, 1016, 1017] });
    const boat = new Boat(w);
    const p = stubPlayer(1000, LV);
    const dx = boat.canBoard(w, p);
    const dryCentral = w._liquidCol(1000) === null;
    check('B13 посадка с пологого берега (dx ≠ 0, центральная колонка сухая)',
        dx !== null && dx !== 0 && dryCentral, `dx=${dx} dryCentral=${dryCentral}`);
}

// ==================== B14: player.distance растёт ====================
{
    const boat = new Boat(ocean);
    const p = putInWater(ocean, oceanX, 7);
    boat.board(p);
    const d0 = p.distance;
    for (let i = 0; i < 120; i++) boat.update(1 / 60, { ...NO, right: true }, p);
    check('B14 player.distance растёт при ходе (HUD не замирает)',
        p.distance > d0 + 50, `d0=${d0.toFixed(1)} → ${p.distance.toFixed(1)} (PPM=${PPM})`);
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exit(1);
