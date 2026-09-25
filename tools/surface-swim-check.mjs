// tools/surface-swim-check.mjs
// ЧК6.2 «Мир прогулки: плавание и погружение» (спека 2026-09-25 §5.3/§6.1/§6.2).
// Детерминированная покадровая симуляция `Player.update` (образец —
// `tools/surface-liquid-check.mjs`). Проверяет:
//   * S1 вода включается по `liquidAt` (центр в жидкости → режим плавания);
//   * S2 легаси `waterY`/`inWater()` удалён (по исходнику физики);
//   * S3 `floatY` — одна реализация линии плавучести;
//   * S4 горизонталь = `WALK·SWIM_FACTOR`, спринт в воде не ускоряет;
//   * S5 ныряние `down` уводит ниже зеркала (до дна);
//   * S6 без `down` голова выше зеркала (клипы гравитации 0.2 и 2.5);
//   * S7 на дне стоит (плавучесть не срывает с опоры);
//   * S8 всплытие «прыжком» — рывок ≈ −ASCEND_SPEED;
//   * S9 подлёдные: ходит по корке, полынья → вход под лёд;
//   * S10 спавн — сухая/ледовая колонка без 2D-форм (или зеркало-фолбэк);
//   * S11 детерминизм спавна; S12 HP плаванием не меняется.
// Run: node tools/surface-swim-check.mjs
import { readFileSync } from 'node:fs';
import { SurfaceWorld } from '../web/static/js/surface/surface_world.js';
import { Player, serverHp } from '../web/static/js/surface/surface_player.js';
import { PPM, WALK_SPEED, SPRINT_SPEED, PLAYER_H, PLAYER_W, FLOAT_SUBMERGE, SWIM_FACTOR, DIVE_SPEED, ASCEND_SPEED } from '../web/static/js/surface/surface_config.js';

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

// longestWetRun — самый длинный непрерывный мокрый участок глубиной ≥ minDepth.
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
function putInWater(w, x, g = 1, below = 40) {
    const p = new Player(w, g);
    p.x = x; p.vx = 0; p.vy = 0; p.onGround = false;
    p.y = w.liquidLevel(x) + below;
    return p;
}
function sim(p, input, steps, dt = 1 / 60) {
    for (let i = 0; i < steps; i++) p.update(dt, input);
    return p;
}
// simMaxVx — установившаяся |vx| как максимум по прогону: на рельефе игрок может
// упереться в стену к концу окна (тогда финальная vx = 0), но разгон уже виден.
function simMaxVx(p, input, steps, dt = 1 / 60) {
    let m = 0;
    for (let i = 0; i < steps; i++) {
        p.update(dt, input);
        const v = Math.abs(p.vx);
        if (v > m) m = v;
    }
    return m;
}
// findWalkColumn — сухая колонка, с которой игрок реально идёт вправо (не упирается
// в стену сразу): проходимость зависит от рельефа, x=0 не гарантирован.
function findWalkColumn(w) {
    for (let x = 0; x < 600; x++) {
        const p = new Player(w, 1);
        p.x = x; p.y = w.floorY(x) - p.h / 2 - 2; p.vx = 0; p.vy = 0; p.onGround = true;
        sim(p, { left: false, right: true, jump: false, sprint: false, down: false }, 40);
        if (p.x - x > 8) return x;
    }
    return 0;
}
const NO = { left: false, right: false, jump: false, sprint: false, down: false };

// ==================== S1: вода включается по liquidAt ====================
{
    const w = mkWorld('океаны');
    const run = longestWetRun(w, 40);
    const wx = run ? run.mid : 0;
    check('S1a в теле жидкости liquidAt даёт среду', w.liquidAt(wx, w.liquidLevel(wx) + 20) === 'вода', `x=${wx}`);
    // В воде — скорость плавания; на суше (нет жидкости) — ходьба.
    const pw = putInWater(w, wx, 1, 40);
    const vSwim = simMaxVx(pw, { ...NO, right: true }, 180);
    const dry = mkWorld('инеевые_рощи');
    const dx = findWalkColumn(dry);
    const pd = new Player(dry, 1);
    pd.x = dx; pd.y = dry.floorY(dx) - pd.h / 2 - 2; pd.vx = 0; pd.vy = 0;
    pd.onGround = true;
    const vWalk = simMaxVx(pd, { ...NO, right: true }, 180);
    check('S1b вода включает режим плавания (v ≈ WALK·SWIM_FACTOR), суша — ходьбу',
        Math.abs(vSwim - WALK_SPEED * PPM * SWIM_FACTOR) < 3 && Math.abs(vWalk - WALK_SPEED * PPM) < 3,
        `vSwim=${vSwim.toFixed(1)} (ждём ${(WALK_SPEED * PPM * SWIM_FACTOR).toFixed(0)}), vWalk=${vWalk.toFixed(1)} (ждём ${(WALK_SPEED * PPM).toFixed(0)})`);
}

// ==================== S2: легаси waterY/inWater удалён ====================
{
    const src = readFileSync(new URL('../web/static/js/surface/surface_player.js', import.meta.url), 'utf8');
    check('S2 легаси waterY/inWater удалён из физики',
        !/this\.waterY/.test(src) && !/\binWater\s*\(/.test(src),
        `this.waterY=${/this\.waterY/.test(src)} inWater()=${/\binWater\s*\(/.test(src)}`);
}

// ==================== S3: floatY — одна реализация ====================
{
    const w = mkWorld('океаны');
    const x = 1234;
    check('S3 floatY = liquidLevel + PLAYER_H·FLOAT_SUBMERGE (одна реализация)',
        Math.abs(w.floatY(x) - (w.liquidLevel(x) + PLAYER_H * FLOAT_SUBMERGE)) < 1e-9,
        `floatY=${w.floatY(x).toFixed(2)}`);
}

// ==================== S4: горизонталь и спринт ====================
{
    const w = mkWorld('океаны');
    const run = longestWetRun(w, 60);
    const wx = run ? run.mid : 0;
    const p1 = putInWater(w, wx, 1, 30);
    sim(p1, { ...NO, right: true }, 180);
    const v1 = p1.vx;
    const p2 = putInWater(w, wx, 1, 30);
    sim(p2, { ...NO, right: true, sprint: true }, 180);
    const v2 = p2.vx;
    const target = WALK_SPEED * PPM * SWIM_FACTOR;
    check('S4a скорость плавания = WALK·SWIM_FACTOR', Math.abs(v1 - target) < 3, `v=${v1.toFixed(1)} ждём ${target}`);
    check('S4b спринт в воде не ускоряет', Math.abs(v2 - v1) < 0.5, `sprint v=${v2.toFixed(1)} обычная v=${v1.toFixed(1)}`);
}

// ==================== S5/S6/S7/S8: вертикаль ====================
{
    const w = mkWorld('океаны');
    const run = longestWetRun(w, 90);
    const wx = run ? run.mid : 0;
    const lv = w.liquidLevel(wx);
    const bd = w.bedY(wx);

    // S6: без down — равновесие у зеркала, голова выше зеркала, не до дна.
    for (const g of [0.2, 2.5]) {
        const p = putInWater(w, wx, g, 40);
        sim(p, NO, 240);
        const head = p.y - p.h / 2;
        check(`S6 гравитация ${g}: без down голова выше зеркала (равновесие)`,
            head < lv - 0.5 && p.y < bd - 5 && !p.onGround,
            `head=${head.toFixed(1)} lv=${lv.toFixed(1)} y=${p.y.toFixed(1)} bd=${bd.toFixed(1)} onGround=${p.onGround}`);
    }
    {
        const p = putInWater(w, wx, 1, 40);
        sim(p, NO, 240);
        check('S6b равновесие ≈ floatY (допуск 4 px)',
            Math.abs(p.y - w.floatY(wx)) < 4, `y=${p.y.toFixed(1)} floatY=${w.floatY(wx).toFixed(1)}`);
    }

    // S5: down — уводит ниже зеркала (до дна).
    const pd = putInWater(w, wx, 1, 10);
    sim(pd, { ...NO, down: true }, 300);
    check('S5 ныряние down уводит ниже зеркала',
        pd.y > lv + 20 && (pd.onGround || pd.y > bd - 8),
        `y=${pd.y.toFixed(1)} lv=${lv.toFixed(1)} bd=${bd.toFixed(1)} onGround=${pd.onGround}`);
    check('S5b погружение ограничено dnom (bedY), не проваливается',
        pd.y <= bd + 1, `y=${pd.y.toFixed(1)} bd=${bd.toFixed(1)}`);

    // S7: на дне стоит — отпустил down, плавучесть не срывает с опоры.
    sim(pd, NO, 120);
    const foot = pd.y + pd.h / 2;
    check('S7 на дне стоит (onGround, плавучесть выключена)',
        pd.onGround && foot <= bd + 1 && foot >= bd - 3,
        `onGround=${pd.onGround} foot=${foot.toFixed(1)} bd=${bd.toFixed(1)}`);

    // S8: всплытие «прыжком» — рывок ≈ −ASCEND_SPEED.
    const pj = putInWater(w, wx, 1, 10);
    sim(pj, { ...NO, down: true }, 300);   // на дно
    pj.update(1 / 60, { ...NO, jump: true });
    check('S8 всплытие jump — рывок ≈ −ASCEND_SPEED',
        pj.vy <= -ASCEND_SPEED + 1e-6,
        `vy=${pj.vy.toFixed(1)} ждём ≈ ${-ASCEND_SPEED}`);
}

// ==================== S9: подлёдные океаны ====================
{
    const w = mkWorld('подлёдные_океаны');
    const lv = w.liquidLevel(0);
    // Ходит по корке: спавн на корке (floorY < liquidLevel), не в воде.
    const pc = new Player(w, 1);
    check('S9a underIce: спавн на корке (floorY = верх корки, не в воде)',
        w.crustTop(w.spawnX) !== null && w.floorY(w.spawnX) < lv && w.liquidAt(w.spawnX, w.spawnY) === '',
        `spawnX=${w.spawnX} floorY=${w.floorY(w.spawnX).toFixed(1)} lv=${lv.toFixed(1)}`);
    sim(pc, NO, 30);
    const footC = pc.y + pc.h / 2;
    check('S9b underIce: держится на корке (не провалился в воду)',
        w.liquidAt(pc.x, pc.y) === '' && footC <= w.floorY(pc.x) + 2,
        `y=${pc.y.toFixed(1)} foot=${footC.toFixed(1)} floorY=${w.floorY(pc.x).toFixed(1)}`);

    // В полынье — вход под лёд (падает в воду под коркой). Берём ЦЕНТР окна
    // (коробка игрока ±5.5 не должна задевать корку по краям полыньи).
    let px = -1;
    for (let x = 8; x < 40000 - 8; x++) {
        if (!w._liquidCol(x)) continue;
        let allPoly = true;
        for (let d = -7; d <= 7; d++) if (!w.polynyaAt(x + d)) { allPoly = false; break; }
        if (allPoly) { px = x; break; }
    }
    if (px >= 0) {
        const pp = new Player(w, 1);
        pp.x = px; pp.vx = 0; pp.vy = 0; pp.onGround = false;
        pp.y = w.liquidLevel(px) - 50;
        let entered = false;
        for (let i = 0; i < 400; i++) {
            pp.update(1 / 60, NO);
            if (w.liquidAt(pp.x, pp.y) !== '') { entered = true; break; }
        }
        check('S9c underIce: в полынье входит под лёд (liquidAt под коркой)',
            entered, `полинья x=${px} y=${pp.y.toFixed(1)} lv=${w.liquidLevel(px).toFixed(1)}`);
    } else {
        check('S9c underIce: в полынье входит под лёд (liquidAt под коркой)', false, 'полынья с водой не найдена');
    }
}

// ==================== S10: спавн ====================
{
    function spawnOk(id, w) {
        if (!Number.isFinite(w.spawnX)) return false;
        const dryCrust = w.floorY(w.spawnX) < w.liquidLevel(w.spawnX);
        const mirror = w.liquidDepth(w.spawnX) >= w.liquid.level.minDepth;
        if (!(dryCrust || mirror)) return false;
        if (!w._columnFormFree(w.spawnX)) return false;
        // Коробка спавна свободна — требование сухой/корковой колонки (§6.2);
        // зеркало-фолбэк стоит на воде (коробка уходит под зеркало — норма).
        if (dryCrust && !w._spawnBoxFree(w.spawnX, w.spawnY)) return false;
        return true;
    }
    for (const id of ['океаны', 'озёра_реки', 'подлёдные_океаны', 'магмовый_океан']) {
        const w = mkWorld(id);
        check(`S10 ${id}: спавн — сухая/ледовая колонка без 2D-форм (или зеркало)`,
            spawnOk(id, w), `spawnX=${w.spawnX} spawnY=${Number(w.spawnY).toFixed(1)}`);
    }
    for (const id of ['инеевые_рощи', 'струнные_рощи', 'коралловые_рифы']) {
        const w = mkWorld(id);
        check(`S10 ${id}: вне жидкости спавн не меняется (x=0)`,
            w.spawnX === 0 && w.spawnY === w.floorY(0) - PLAYER_H / 2 - 2,
            `spawnX=${w.spawnX}`);
    }
    // Спавн свободен по solidAt' (углы+центр).
    const w = mkWorld('океаны');
    check('S10b коробка спавна свободна по solidAt\'', w._spawnBoxFree(w.spawnX, w.spawnY) === true,
        `x=${w.spawnX} y=${w.spawnY.toFixed(1)}`);
}

// ==================== S11: детерминизм спавна ====================
{
    const a = mkWorld('океаны', { seed: 424242 });
    const b = mkWorld('океаны', { seed: 424242 });
    check('S11a спавн детерминирован (тот же seed → тот же x)', a.spawnX === b.spawnX, `x=${a.spawnX}`);
    const c = mkWorld('подлёдные_океаны', { seed: 424242 });
    const d = mkWorld('подлёдные_океаны', { seed: 424242 });
    check('S11b спавн подлёдных детерминирован', c.spawnX === d.spawnX, `x=${c.spawnX}`);
}

// ==================== S12: HP плаванием не меняется ====================
{
    const w = mkWorld('океаны');
    const run = longestWetRun(w, 40);
    const wx = run ? run.mid : 0;
    const p = putInWater(w, wx, 1, 40);
    sim(p, { ...NO, down: true }, 120);
    sim(p, NO, 60);
    const pkg = { landed_at: new Date(Date.now() - 5000).toISOString(), hazard: { total: 0.4 } };
    const now = Date.now();
    check('S12 плавание не заводит HP (физика без hp; serverHp не зависит от жидкости)',
        !('hp' in p) && serverHp(pkg, now) === serverHp(pkg, now) && serverHp(pkg, now) <= 100,
        `hp field=${'hp' in p} serverHp=${serverHp(pkg, now).toFixed(2)}`);
}

// ==================== S13: подлёдное запирание (seed 777001, §6.6) ====================
// Регресс BUG-1: нырок в полынье → под коркой → игрок НЕ запирается (проходит прежний
// лок x≈966.3) и может вернуться в полынь (выход есть, §3.2/§6.6). До фикса головы
// в твёрдом `_blockedX` запирал бокс, а `_blockedUp` при `vy ≥ 0` не срабатывал.
{
    const w = mkWorld('подлёдные_океаны', { seed: 777001 });
    let px = -1;
    for (let x = 8; x < 40000 - 8; x++) {
        if (!w._liquidCol(x)) continue;
        let all = true;
        for (let d = -7; d <= 7; d++) if (!w.polynyaAt(x + d)) { all = false; break; }
        if (all) { px = x; break; }
    }
    const p = new Player(w, 1);
    p.x = px; p.vx = 0; p.vy = 0; p.onGround = false; p.y = w.liquidLevel(px) + 20;
    sim(p, { ...NO, down: true }, 200);
    for (let i = 0; i < 2400; i++) p.update(1 / 60, { ...NO, down: true, right: true });
    const xRight = p.x;
    for (let i = 0; i < 2400; i++) p.update(1 / 60, { ...NO, down: true, left: true });
    const backExit = w.polynyaAt(p.x) || w.liquidAt(p.x, p.y) !== '';
    check('S13 underIce seed 777001: под коркой не запирается, выход через полынью',
        xRight > 972 && backExit,
        `polyX=${px} -> right=${xRight.toFixed(1)} (прежний лок 966.3) -> back=${p.x.toFixed(1)} poly=${w.polynyaAt(p.x)} inLiq=${w.liquidAt(p.x, p.y)}`);
}

// ==================== S14: независимость от порядка запросов колонок (OBS-1) ====================
// `liquidAt`/`liquidDepth`/`bedY` не должны зависеть от того, какие колонки
// опрашивали раньше (probe-миры выходимости не пишут в мемо мира; basin-уровень
// считается на представителе корзины 2 px). Иначе «нарисовано ≠ плывётся» (§3.1).
{
    const cases = [
        ['магмовый_океан', 777001, [8487, 8488, 8490]],
        ['подлёдные_океаны', 777001, [900, 964, 1200]],
        ['озёра_реки', 424242, [900, 901, 902]],
    ];
    let bad = 0, firstDiff = null;
    for (const [id, seed, cols] of cases) {
        for (const x of cols) {
            const a = mkWorld(id, { seed });
            a.liquidDepth(x + 1);   // прогрев соседней колонки (провокация порядка)
            const wa = a.liquidAt(x, a.liquidLevel(x) + 1);
            const wd = a.liquidDepth(x);
            const wb = a.bedY(x);
            const b = mkWorld(id, { seed });
            const fa = b.liquidAt(x, b.liquidLevel(x) + 1);
            const fd = b.liquidDepth(x);
            const fb = b.bedY(x);
            if (wa !== fa || Math.abs(wd - fd) > 1e-9 || Math.abs(wb - fb) > 1e-9) {
                bad++;
                if (!firstDiff) firstDiff = `${id}/s${seed}/x${x} depth:${wd}≠${fd} liquidAt:${wa}≠${fa}`;
            }
        }
    }
    check('S14 liquidAt/liquidDepth/bedY не зависят от порядка запросов колонок',
        bad === 0, firstDiff || 'чисто (прогрев соседа не меняет колонку)');
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exit(1);
