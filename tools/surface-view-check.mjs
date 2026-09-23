// tools/surface-view-check.mjs
// Node-проверка клиентского вида прогулки (спека 2026-09-23, Э1):
//  - V1: рецепт применяется только при view_source='catalog'; без него — фолбэк;
//  - V2: палитра земли из рецепта; отсутствующие ключи выводятся из base;
//  - V3: размещение clustered — кучки с зазорами (не частокол);
//  - V4: в песках нет деревьев; в джунглях дерево + лианы;
//  - V5: детерминизм (тот же seed/вид → тот же декор), Math.random не используется;
//  - V6: диспетчер декора — известные примитивы рисуют, неизвестный — пропуск.
// Запуск: node tools/surface-view-check.mjs
import { SurfaceWorld, horizonHeight } from '../web/static/js/surface/surface_world.js';
import { drawDecorPrim } from '../web/static/js/surface/surface_decor.js';
import { horizonProfile, drawHorizon } from '../web/static/js/surface/surface_render.js';

const results = [];
function check(name, ok, detail) {
    results.push({ name, ok });
    console.log(`[${ok ? 'PASS' : 'FAIL'}] ${name}${detail ? ' - ' + detail : ''}`);
}

function mkPkg(over = {}) {
    return {
        seed: 424242, biome: 'x', biome_category: 'литосфера',
        biome_color: '#8a7a6a', life: true, view_source: 'fallback',
        view_version: 1, ...over,
    };
}

const desertView = {
    palette: { base: '#cfa96a', dark: '#9e7c45', light: '#ecd6a4' },
    decor: [
        { prim: 'cactus', p: 0.02, h: [20, 52] },
        { prim: 'bush', p: 0.03, h: [6, 14] },
        { prim: 'rock', p: 0.01, h: [6, 20] },
    ],
    placement: { mode: 'clustered', tile: 260, gap: 16 },
    life_density: 0.05,
};

// V1/V2 — рецепт применяется; палитра из рецепта; без рецепта — фолбэк.
{
    const w = new SurfaceWorld(mkPkg({ biome_view: desertView, view_source: 'catalog' }));
    check('V1a catalog → рецепт применён', w.hasView === true);
    check('V1b fallback → рецепта нет', new SurfaceWorld(mkPkg()).hasView === false);
    check('V2a palette.base из рецепта', w.palette.base === '#cfa96a', w.palette.base);
    check('V2b palette.dark из рецепта', w.palette.dark === '#9e7c45');
    const minimal = new SurfaceWorld(mkPkg({ biome_view: { palette: { base: '#102030' } }, view_source: 'catalog' }));
    check('V2c отсутствующие ключи выведены из base (hex)',
        /^#[0-9a-f]{6}$/.test(minimal.palette.light) && minimal.palette.light !== '#102030',
        minimal.palette.light);
    // Фолбэк палитры — от пакетного biome_color.
    const legacy = new SurfaceWorld(mkPkg({ biome_color: '#112233' }));
    check('V2d без рецепта base = biome_color', legacy.palette.base === '#112233');
}

// V3 — clustered: есть зазоры (null) и кучки (не-null), длина кучки ≤ span.
{
    const view = {
        decor: [{ prim: 'rock', p: 1.0, h: [4, 8] }],
        placement: { mode: 'clustered', tile: 100, gap: 40 },
        life_density: 1.0,
    };
    const w = new SurfaceWorld(mkPkg({ biome_view: view, view_source: 'catalog' }));
    let nulls = 0, filled = 0, run = 0, maxRun = 0;
    for (let x = 0; x < 2000; x++) {
        const d = w.decorAt(x);
        if (d) { filled++; run++; maxRun = Math.max(maxRun, run); } else { nulls++; run = 0; }
    }
    check('V3a clustered: есть и кучки, и зазоры', nulls > 0 && filled > 0, `nulls=${nulls} filled=${filled}`);
    check('V3b длина кучки ≤ tile-gap', maxRun <= 60, `maxRun=${maxRun}`);
    check('V3c uniform: без зазоров', (() => {
        const u = new SurfaceWorld(mkPkg({ biome_view: { decor: [{ prim: 'rock', p: 1, h: 4 }], placement: { mode: 'uniform' }, life_density: 1 }, view_source: 'catalog' }));
        for (let x = 0; x < 200; x++) if (!u.decorAt(x)) return false;
        return true;
    })());
}

// V4 — пески: без деревьев; джунгли: дерево + лианы.
{
    const w = new SurfaceWorld(mkPkg({ biome_view: { ...desertView, life_density: 1 }, view_source: 'catalog' }));
    const prims = new Set();
    for (let x = 0; x < 5000; x++) { const d = w.decorAt(x); if (d) prims.add(d.prim); }
    check('V4a пески: нет дерева', !prims.has('tree'), [...prims].join(','));
    check('V4b пески: есть кактус', prims.has('cactus'));

    const jungle = {
        decor: [{ prim: 'tree', p: 1.0, h: [70, 160], crown: [2, 3], buttress: true }],
        hang: [{ prim: 'liana', from: 'tree', p: 1.0, len: [20, 70] }],
        placement: { mode: 'clustered', tile: 220, gap: 14 },
        life_density: 1.0,
    };
    const jw = new SurfaceWorld(mkPkg({ biome_view: jungle, view_source: 'catalog' }));
    let tree = null;
    for (let x = 0; x < 2000 && !tree; x++) { const d = jw.decorAt(x); if (d && d.prim === 'tree') tree = d; }
    check('V4c джунгли: дерево есть', !!tree);
    check('V4d джунгли: у дерева лиана', !!(tree && tree.hang && tree.hang.prim === 'liana'));
    check('V4e джунгли: контрфорсы и крона форвардятся (crown — число)',
        !!(tree && tree.buttress === true && typeof tree.crown === 'number'), tree && JSON.stringify({ buttress: tree.buttress, crown: tree.crown }));
}

// V5 — детерминизм: тот же seed/вид → тот же декор.
{
    const a = new SurfaceWorld(mkPkg({ biome_view: desertView, view_source: 'catalog' }));
    const b = new SurfaceWorld(mkPkg({ biome_view: desertView, view_source: 'catalog' }));
    let same = true;
    for (let x = 0; x < 3000; x++) {
        const da = a.decorAt(x), db = b.decorAt(x);
        if (JSON.stringify(da) !== JSON.stringify(db)) { same = false; break; }
    }
    check('V5 детерминизм декора', same);
}

// V6 — диспетчер: известные примитивы рисуют, неизвестный — пропуск.
{
    const noop = () => { calls.n++; };
    const calls = { n: 0 };
    const ctx = {
        fillRect: noop, beginPath: noop, moveTo: noop, lineTo: noop,
        quadraticCurveTo: noop, arc: noop, ellipse: noop, closePath: noop,
        fill: noop, stroke: noop, save: noop, restore: noop,
        fillStyle: '', strokeStyle: '', lineWidth: 1,
    };
    const w = new SurfaceWorld(mkPkg());
    const prims = ['tree', 'conifer', 'palm', 'mushroom', 'cactus', 'bush', 'fern',
        'grass', 'lichen', 'crystal', 'growth', 'rock', 'bone', 'debris', 'vent', 'liana'];
    let drawn = 0;
    for (const prim of prims) {
        calls.n = 0;
        drawDecorPrim(ctx, w, { prim, h: 30, w: 20, kind: 'skeleton' }, 0, 0);
        if (calls.n > 0) drawn++;
    }
    check('V6a все 15 примитивов + лиана рисуют', drawn === prims.length, `drawn=${drawn}/${prims.length}`);
    calls.n = 0;
    drawDecorPrim(ctx, w, { prim: 'нет_такого', h: 10 }, 0, 0);
    check('V6b неизвестный примитив — пропуск', calls.n === 0);
}

// V7 — параметры примитивов форвардятся из рецепта в запись декора (§3.2).
{
    const view = {
        decor: [
            { prim: 'palm', p: 1, h: [60, 130], fronds: [6, 9] },
            { prim: 'grass', p: 1, h: [8, 22], blades: [5, 9] },
            { prim: 'mushroom', p: 1, h: [10, 20], capR: 7 },
            { prim: 'cactus', p: 1, h: [20, 52], arms: 3 },
        ],
        placement: { mode: 'uniform' },
        life_density: 1,
    };
    const w = new SurfaceWorld(mkPkg({ life: true, biome_view: view, view_source: 'catalog' }));
    const seen = {};
    for (let x = 0; x < 4000; x++) { const d = w.decorAt(x); if (d && !seen[d.prim]) seen[d.prim] = d; }
    check('V7a fronds форвардятся (число)', typeof (seen.palm && seen.palm.fronds) === 'number');
    check('V7b blades форвардятся (число)', typeof (seen.grass && seen.grass.blades) === 'number');
    check('V7c capR форвардится', !!(seen.mushroom && seen.mushroom.capR === 7));
    check('V7d arms форвардятся', !!(seen.cactus && seen.cactus.arms === 3));
}

// V8 — живой декор гейтится по pkg.life (§3.2); фолбэк-биом не меняется.
{
    const view = {
        decor: [
            { prim: 'conifer', p: 1, h: 40 }, { prim: 'lichen', p: 1, h: 5 },
            { prim: 'rock', p: 1, h: 8 }, { prim: 'crystal', p: 1, h: 8 },
        ],
        placement: { mode: 'uniform' }, life_density: 1,
    };
    const primsOf = (w) => { const s = new Set(); for (let x = 0; x < 4000; x++) { const d = w.decorAt(x); if (d) s.add(d.prim || d.kind); } return s; };
    const dead = primsOf(new SurfaceWorld(mkPkg({ life: false, biome_view: view, view_source: 'catalog' })));
    check('V8a безжизненный мир: нет живой хвои/лишайника', !dead.has('conifer') && !dead.has('lichen'), [...dead].join(','));
    check('V8b безжизненный мир: неживой декор остаётся', dead.has('rock') && dead.has('crystal'));
    const alive = primsOf(new SurfaceWorld(mkPkg({ life: true, biome_view: view, view_source: 'catalog' })));
    check('V8c живой мир: хвоя есть', alive.has('conifer'));
    const legacyDead = primsOf(new SurfaceWorld(mkPkg({ life: false })));
    check('V8d фолбэк без жизни: легаси lichen/rock', legacyDead.size > 0 && [...legacyDead].every(k => k === 'lichen' || k === 'rock'), [...legacyDead].join(','));
}

// V9 — фильтр `where` по высоте и стороне склона (§3.2).
{
    const mk = (where) => new SurfaceWorld(mkPkg({
        life: false,
        biome_view: { decor: [{ prim: 'rock', p: 1, h: 4, where }], placement: { mode: 'uniform' }, life_density: 1 },
        view_source: 'catalog',
    }));
    const anyHit = (w) => { for (let x = 0; x < 600; x++) if (w.decorAt(x)) return true; return false; };
    check('V9a where maxAlt отсекает всё', !anyHit(mk({ maxAlt: -1e9 })));
    check('V9b where maxAlt разрешает всё', anyHit(mk({ maxAlt: 1e9 })));
    const hits = (w) => { const s = new Set(); for (let x = 0; x < 3000; x++) if (w.decorAt(x)) s.add(x); return s; };
    const L = hits(mk({ side: 'left' })), R = hits(mk({ side: 'right' }));
    check('V9c where side: обе стороны встречаются', L.size > 0 && R.size > 0, `L=${L.size} R=${R.size}`);
    let overlap = 0; for (const x of L) if (R.has(x)) overlap++;
    check('V9d where side: стороны не пересекаются', overlap === 0, `overlap=${overlap}`);
}

// H1–H5 — ярусный горизонт (§3.6, Э2): разбор рецепта, потолок 2 пояса,
// детерминизм, цельный уезд силуэта, отрисовка и фолбэк.
{
    const desertHz = {
        ...desertView,
        horizon: { layers: [
            { parallax: 0.5, profile: { prim: 'wave', lambda: [900, 1400], amp: [45, 75], skew: 0.85 }, fill: 'dark', haze: 0.55, step: 24 },
            { parallax: 0.7, profile: { prim: 'wave', lambda: [520, 820], amp: [55, 95], skew: 0.85 }, fill: 'dark', haze: 0.3, step: 24 },
        ] },
    };
    const wv = new SurfaceWorld(mkPkg({ biome_view: desertHz, view_source: 'catalog' }));
    check('H1a рецепт с horizon → 2 пояса', Array.isArray(wv.horizon) && wv.horizon.length === 2, 'layers=' + (wv.horizon && wv.horizon.length));
    check('H1b без рецепта → ярусов нет (фолбэк 1:1)', new SurfaceWorld(mkPkg()).horizon === null);
    check('H1c незнакомая view_version → фолбэк (§2.6)',
        new SurfaceWorld(mkPkg({ biome_view: desertHz, view_source: 'catalog', view_version: 2 })).hasView === false);
    check('H1d view_version=1 → рецепт применён',
        new SurfaceWorld(mkPkg({ biome_view: desertHz, view_source: 'catalog', view_version: 1 })).hasView === true);

    const w3 = new SurfaceWorld(mkPkg({
        biome_view: { ...desertHz, horizon: { layers: [...desertHz.horizon.layers, { parallax: 0.8, profile: { prim: 'wave' } }] } },
        view_source: 'catalog',
    }));
    check('H2 потолок 2 пояса — третий отброшен', w3.horizon.length === 2, 'layers=' + w3.horizon.length);

    const cam = { x: 0, y: 0 };
    const ptsA = horizonProfile(wv, cam, 800, 600, wv.horizon[0]);
    const ptsB = horizonProfile(new SurfaceWorld(mkPkg({ biome_view: desertHz, view_source: 'catalog' })), cam, 800, 600, wv.horizon[0]);
    check('H3 профиль детерминирован (тот же seed → те же точки)', JSON.stringify(ptsA) === JSON.stringify(ptsB));

    // Силуэт уезжает цельно: сдвиг камеры на Δ → экранный сдвиг Δ·p (точка wx общая).
    const ptsC = horizonProfile(wv, { x: 100, y: 0 }, 800, 600, wv.horizon[0]);
    const a0 = ptsA.find(p => p.wx === 0), c0 = ptsC.find(p => p.wx === 0);
    check('H4a общая точка wx=0 на обеих решётках', !!a0 && !!c0);
    check('H4b уезжает цельно (Δ·p=50, y без изменений)',
        !!a0 && !!c0 && Math.abs((a0.sx - c0.sx) - 50) < 1e-9 && a0.y === c0.y,
        a0 && c0 ? `dsx=${(a0.sx - c0.sx).toFixed(2)} dy=${(a0.y - c0.y).toFixed(2)}` : 'нет точки');

    // H5 — отрисовка: нет ярусов → ничего; с ярусами → силуэт + дымка на каждый пояс.
    const fills = { n: 0 };
    const noop = () => {};
    const ctx = {
        fillRect: noop, beginPath: noop, moveTo: noop, lineTo: noop, closePath: noop,
        fill: () => { fills.n++; }, fillStyle: '', globalAlpha: 1,
    };
    fills.n = 0; drawHorizon(ctx, new SurfaceWorld(mkPkg()), cam, 800, 600, null);
    check('H5a без рецепта drawHorizon не рисует', fills.n === 0, 'fills=' + fills.n);
    fills.n = 0; drawHorizon(ctx, wv, cam, 800, 600, null);
    check('H5b 2 пояса → 4 заливки (силуэт+дымка каждый)', fills.n === 4, 'fills=' + fills.n);

    // H6 — устойчивость skew (§3.1): 0/1 не должны давать NaN (кламп в (0,1)).
    const ySkew0 = horizonHeight('wave', { lambda: 900, amp: 60, skew: 0 }, 1234, 7);
    const ySkew1 = horizonHeight('wave', { lambda: 900, amp: 60, skew: 1 }, 1234, 7);
    check('H6a skew=0 → конечное число', Number.isFinite(ySkew0), String(ySkew0));
    check('H6b skew=1 → конечное число', Number.isFinite(ySkew1), String(ySkew1));
}

const failed = results.filter(r => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exitCode = 1;
