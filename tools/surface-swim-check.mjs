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
// Регресс BUG-1: нырок в полынье → под коркой → игрок НЕ запирается и может вернуться
// в полынь (выход есть, §3.2/§6.6). До фикса головы в твёрдом `_blockedX` запирал бокс,
// а `_blockedUp` при `vy ≥ 0` не срабатывал.
//
// ПОЧЕМУ УШЛО ОЖИДАНИЕ `xRight > 972` (хвост №84): оно кодировало ПРОХОДИМОСТЬ
// мелководья — «игрок проплывает мимо прежнего лока 966.3», т.е. что подо льдом есть
// щель уже габарита. По решению создателя такой щели не бывает: там, где воды под
// коркой меньше `LIQUID_ICE_CLEAR_MIN`, лёд САДИТСЯ НА ДНО и прохода нет вовсе
// (твёрдая плита до дна), поэтому упереться в лёд штатно — не запирание. Инвариант
// §6.6 остаётся и проверяется: выход через полынью есть, и под коркой игрок нигде не
// остаётся в полосе уже роста (обе стороны заблокированы быть не могут).
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
    const gapRight = underIceGap(w, xRight);
    for (let i = 0; i < 2400; i++) p.update(1 / 60, { ...NO, down: true, left: true });
    const backExit = w.polynyaAt(p.x) || w.liquidAt(p.x, p.y) !== '';
    check('S13 underIce seed 777001: под коркой не запирается, выход через полынью',
        backExit && (gapRight === 0 || gapRight >= PLAYER_H),
        `polyX=${px} -> right=${xRight.toFixed(1)} (просвет ${gapRight} px: 0 = лёд на дне) -> back=${p.x.toFixed(1)} poly=${w.polynyaAt(p.x)} inLiq=${w.liquidAt(p.x, p.y)}`);
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

// ==================== S15: подлёдная щель уже габарита (хвост №84) ====================
// Инвариант: под коркой подлёдной колонки свободно по вертикали либо 0 (лёд сел на
// дно — мелководье, входа нет), либо ≥ PLAYER_H (реально проходимо). Промежуточных
// значений быть не может: в щели уже роста игрок переходит в режим плавания, но
// бокс (28 px) не проходит ни в одну сторону — колонка-ловушка. Проверяем на
// нескольких seed и по двум входам: полынья (вход под лёд) и сама корка.
//
// Все измерения — по ПУБЛИЧНОМУ API (crustTop/crustAt/bedY/liquidLevel/polynyaAt),
// чтобы тетст не зависел от внутренних кэшей колонок.
const ICE_SEEDS = [777001, 424242, 991];
const ICE_SAMPLE = 3000;   // px колонок от x=0 (шаг 1)

// _freeRows — сколько ЦЕЛЫХ px между `lo` (низ льда/зеркала) и `hi` (дно) свободно.
// Бокс игрока меряется целыми px, поэтому и полосу меряем целыми: дробный остаток дна
// (0.37 px) не «просвет» — иначе севший на дно лёд дал бы «щель 0.37 px».
function _freeRows(lo, hi) {
    return Math.max(0, Math.ceil(hi) - 1 - Math.floor(lo));
}
// iceGap — свободная по вертикали полоса под коркой: px между низом льда и `bedY`.
// null — колонка без корки (полынья/мелководье без льда), там полосу меряет
// underIceGap. 0 — лёд сел на дно.
function iceGap(w, x) {
    const top = w.crustTop(x);
    if (top === null) return null;
    const bd = w.bedY(x);
    let bot = null;
    for (let y = Math.floor(top); y <= bd; y++) if (w.crustAt(x, y)) bot = y;
    return bot === null ? 0 : _freeRows(bot, bd);
}
// underIceGap — та же полоса для ЛЮБОЙ колонки underIce: под полыньей льда нет,
// значит низ «льда» = зеркало, полоса = глубина воды от зеркала до дна.
function underIceGap(w, x) {
    if (w.crustTop(x) !== null) return iceGap(w, x);
    if (w.polynyaAt(x)) return _freeRows(w.liquidLevel(x), w.bedY(x));
    return 0;
}

// S15a — под коркой нет щели уже габарита игрока.
{
    let cols = 0, bad = 0, shallow = 0, worst = null;
    for (const seed of ICE_SEEDS) {
        const w = mkWorld('подлёдные_океаны', { seed });
        for (let x = 0; x < ICE_SAMPLE; x++) {
            if (w.crustTop(x) === null) continue;
            const g = iceGap(w, x);
            cols++;
            if (g > 0 && g < PLAYER_H) shallow++;   // ровно эти колонки меняет фикс
            if (g !== 0 && g < PLAYER_H) {
                bad++;
                if (!worst || g < worst.g) worst = { seed, x, g };
            }
        }
    }
    check('S15a underIce: под коркой просвет 0 или ≥ PLAYER_H (щели-ловушки нет)',
        bad === 0,
        `колонок с коркой=${cols}, промежуточных (0<g<${PLAYER_H})=${bad} [станет лёд на дне: ${shallow}]` +
        (worst ? `, худшая seed${worst.seed}/x${worst.x}: ${worst.g.toFixed(1)} px` : ''));
}

// S15b — полынья засчитывается только над проходимой водой: окно без льда, где воды
// меньше габарита, — не вход под лёд, а яма (в воде прыжок запрещён — из неё не
// выбраться), а при сухом дне — сухая дыра в плите.
{
    let cols = 0, bad = 0, worst = null;
    for (const seed of ICE_SEEDS) {
        const w = mkWorld('подлёдные_океаны', { seed });
        for (let x = 0; x < ICE_SAMPLE; x++) {
            if (!w.polynyaAt(x)) continue;
            const g = _freeRows(w.liquidLevel(x), w.bedY(x));
            cols++;
            if (g < PLAYER_H) {
                bad++;
                if (!worst || g < worst.g) worst = { seed, x, g };
            }
        }
    }
    check('S15b underIce: полынья только над проходимой водой (≥ PLAYER_H)',
        bad === 0,
        `окон полыньи=${cols}, непроходимых=${bad}` +
        (worst ? `, худшая seed${worst.seed}/x${worst.x}: ${worst.g.toFixed(1)} px` : ''));
}

// S15c — физика: нырок в полынью и попытка уйти в обе стороны не оставляет игрока в
// щели уже габарита (состояние «оба направления заблокированы»). Прежний фикс
// S13 чинит только толчок головой; сама щель оставалась ловушкой.
{
    let tries = 0, bad = 0, worst = null;
    for (const seed of [777001, 424242]) {
        const w = mkWorld('подлёдные_океаны', { seed });
        let taken = 0;
        for (let x = 8; x < 40000 - 8 && taken < 20; x++) {
            if (!w.polynyaAt(x) || !w._liquidCol(x)) continue;
            let room = true;
            for (let d = -7; d <= 7; d++) if (!w.polynyaAt(x + d)) { room = false; break; }
            if (!room) continue;
            const p = new Player(w, 1);
            p.x = x; p.vx = 0; p.vy = 0; p.onGround = false; p.y = w.liquidLevel(x) + 20;
            sim(p, { ...NO, down: true }, 200);
            for (let i = 0; i < 2400; i++) p.update(1 / 60, { ...NO, down: true, right: true });
            for (let i = 0; i < 2400; i++) p.update(1 / 60, { ...NO, down: true, left: true });
            const g = underIceGap(w, p.x);
            tries++; taken++;
            if (g !== 0 && g < PLAYER_H) {
                bad++;
                if (!worst || g < worst.g) worst = { seed, x: p.x, g, y: p.y };
            }
        }
    }
    check('S15c underIce: после нырка и ухода в обе стороны игрок не в щели (оба направления не блокированы)',
        bad === 0,
        `полыней проверено=${tries}, застряли в щели=${bad}` +
        (worst ? `, худшая seed${worst.seed}/x${worst.x.toFixed(1)}: ${worst.g.toFixed(1)} px` : ''));
}

// ==================== S16: выход из воды на лёд без лодки (хвост №84, §6.6) ====================
// Спека §6.6: «выход — только через полыньи», проверка e2e — «нырнуть в полынье,
// проплыть под коркой, ВЫЙТИ в полынью». Значит выход из воды под коркой должен быть
// без лодки.
//
// Замер @tester 2026-09-27: штатное всплытие «прыжком» (ASCEND_SPEED ≈ 2.5 м/с →
// баллистический подъём ≈13 px) ниже корки `iceH` = 26 px — кромка выше головы,
// пешком/прыжком из воды не выйти (ловушка). Фикс: в подлёдной воде скорость прыжка
// подбирается под `iceH` и гравитацию (`Player._ascendSpeed`) — перескок плиты.
//
// Пробо-мир БЕЗ 2D-форм: каменные арки уводят игрока по верху плиты, и в воду он сам
// не попадает (кейс тогда недостижим в обычном мире, замер @tester). Здесь игрок
// ныряет в полынье и обязан вернуться на лёд/сушу пешком.
// (кромка-ступень не нужна — см. S16 ниже)

// mkIceNoForms — подлёдный мир БЕЗ 2D-форм (каменных арок): кейс «подо льдом».
function mkIceNoForms(seed) {
    const view = resolveView('подлёдные_океаны');
    view.relief = { ...(view.relief || {}), forms: [] };
    return mkWorld('подлёдные_океаны', { seed, biome_view: view });
}
// iceWindow — границы первого окна полыньи (публичный API `polynyaAt`).
function iceWindow(w, from = 8, to = 40000) {
    let a = -1;
    for (let x = from; x < to; x++) {
        if (w.polynyaAt(x)) { if (a < 0) a = x; }
        else if (a >= 0) return { a, b: x - 1, mid: (a + x - 1) / 2 };
    }
    return null;
}
// shoreDir — сторона, где вода кончается (берег): вода подо льдом до края не
// доходит — игрок идёт к берегу по пласту/краю.
function shoreDir(w, x) {
    let r = -1, l = -1;
    for (let i = Math.floor(x) + 1; i < 40000; i++) if (!w._liquidCol(i)) { r = i; break; }
    for (let i = Math.floor(x) - 1; i >= 0; i--) if (!w._liquidCol(i)) { l = i; break; }
    if (r >= 0 && l < 0) return 1;
    if (l >= 0 && r < 0) return -1;
    return r - x <= x - l ? 1 : -1;
}

// onIce — игрок стоит на льду: опора есть, стопа (+запас на зазор разрешения
// коллизий) внутри корки. Держать `jump` только пока в воде: иначе на льду
// срабатывает обычный прыжок и `onGround` гаснет (bunny-hop) — тогда лёд не засечь.
function onIce(w, p) {
    return p.onGround && w.crustAt(p.x, p.y + p.h / 2 + 1.5);
}
function onLand(w, p) {
    return p.onGround && !w._liquidCol(Math.floor(p.x)) && !w.crustAt(p.x, p.y + p.h / 2 + 1.5);
}
// escapeTrack — из воды в полынье игрок выбирается на лёд и доходит до суши пешком:
// «плыть к берегу + всплывать» (естественное действие), прыжок — только в воде.
function escapeTrack(w, g) {
    const win = iceWindow(w);
    const lv = w.liquidLevel(win.mid);
    const dir = shoreDir(w, win.mid);
    const p = new Player(w, g);
    p.x = win.mid; p.y = lv + 9; p.vx = 0; p.vy = 0; p.onGround = false;
    let iceAt = -1, landAt = -1, iceX = 0, landX = 0;
    for (let i = 0; i < 60 * 90; i++) {
        const inLiq = w.liquidAt(p.x, p.y) !== '';
        p.update(1 / 60, { ...NO, jump: inLiq, [dir > 0 ? 'right' : 'left']: true });
        if (iceAt < 0 && onIce(w, p)) { iceAt = i; iceX = p.x; }
        if (landAt < 0 && onLand(w, p)) { landAt = i; landX = p.x; break; }
    }
    return { win, dir, iceAt, iceX, landAt, landX };
}

// S16a — из воды под коркой игрок ВЫЛЕЗАЕТ на лёд (без лодки, §6.6).
{
    const w = mkIceNoForms(424242);
    const r = escapeTrack(w, 1);
    check('S16a underIce: из воды под коркой игрок вылезает на лёд (без лодки)',
        r.iceAt >= 0,
        `окно=[${r.win.a}..${r.win.b}] dir=${r.dir > 0 ? '+' : '-'} ` +
        (r.iceAt < 0 ? 'НЕ ВЫШЕЛ' : `на лёд через ${(r.iceAt / 60).toFixed(2)} с, x=${r.iceX.toFixed(1)}`));
}

// S16b — с льда игрок доходит до СУШИ пешком (выход из «подо льдом» полный, §6.6).
{
    const w = mkIceNoForms(424242);
    const r = escapeTrack(w, 1);
    check('S16b underIce: выйдя на лёд, игрок доходит до суши пешком (без лодки)',
        r.landAt >= 0,
        `лёд через ${r.iceAt < 0 ? 'НЕ ВЫШЕЛ' : (r.iceAt / 60).toFixed(2) + ' с'}, ` +
        `суша через ${r.landAt < 0 ? 'НЕ ДОШЁЛ' : (r.landAt / 60).toFixed(2) + ' с'}, x=${r.landX.toFixed(1)}`);
}

// S16c — выход на сушу пешком не зависит от гравитации планеты (клип 0.2–2.5 g):
// прыжок в воде подобран под `g`, иначе на тяжёлых планетах подъём короче корки.
{
    const rows = [];
    let bad = 0;
    for (const g of [0.2, 1, 2.5]) {
        const w = mkIceNoForms(424242);
        const r = escapeTrack(w, g);
        rows.push(`g=${g}: ${r.iceAt < 0 ? 'лёд НЕ ВЫШЕЛ' : 'лёд@' + (r.iceAt / 60).toFixed(2) + 'с'}/${r.landAt < 0 ? 'суша НЕ ДОШЁЛ' : 'суша@' + (r.landAt / 60).toFixed(2) + 'с'}`);
        if (r.landAt < 0) bad++;
    }
    check('S16c underIce: выход на сушу пешком не зависит от гравитации (0.2/1/2.5 g)',
        bad === 0, rows.join('; '));
}

// S16d — замер высоты: прыжок в полынье поднимает стопы ВЫШЕ верха корки (иначе
// выхода нет). Числа для отчёта: `iceH`, подъём от зеркала.
{
    const w = mkIceNoForms(424242);
    const win = iceWindow(w);
    const lv = w.liquidLevel(win.mid);
    const top = w.crustTop(win.a - 5);   // верх корки рядом с окном полыньи
    const p = new Player(w, 1);
    p.x = win.mid; p.y = lv + 9; p.vx = 0; p.vy = 0; p.onGround = false;
    let minY = p.y;
    for (let i = 0; i < 300; i++) {
        const inLiq = w.liquidAt(p.x, p.y) !== '';
        p.update(1 / 60, { ...NO, jump: inLiq });
        if (p.y < minY) minY = p.y;
    }
    const feetApex = minY + p.h / 2;
    check('S16d underIce: стопы на всплытии поднимаются выше верха корки (выход возможен)',
        feetApex < top,
        `lv=${lv.toFixed(0)} верх корки=${top.toFixed(0)} (iceH=${(lv - top).toFixed(0)} px); ` +
        `стопы на всплытии=${feetApex.toFixed(1)} — на ${(top - feetApex).toFixed(1)} px выше кромки, ` +
        `подъём от зеркала=${(lv - feetApex).toFixed(1)} px`);
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exit(1);
