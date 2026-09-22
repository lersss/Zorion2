// tools/surface-env-check.mjs
// Node-чек слоя среды прогулки (спека 2026-09-22 §6, тесты T11/T7):
//  - доли фаз и кусочно-линейная nightFactor(u);
//  - детерминизм f(seed, elapsed) и фиксация фазы админом;
//  - lightMul и alpha трёх проходов среды (drawEnvironmentMid/Front);
//  - drawSky без env — дневной вид (обратная совместимость), с env — палитра суток;
//  - alphaStars (порог + гашение густым явлением).
// Запуск из корня проекта:  node tools/surface-env-check.mjs
// (Node 24: ESM-синтаксис в .js определяется автоматически.)
import { SurfaceEnvironment, phaseForU, nightFactorAt, drawEnvironmentMid, drawEnvironmentFront } from '../web/static/js/surface/surface_environment.js';
import { drawSky } from '../web/static/js/surface/surface_render.js';
import { WEATHER_RULES, ENV, COLORS } from '../web/static/js/surface/surface_config.js';

const results = [];
function check(name, ok, detail) {
    results.push({ name, ok });
    console.log(`[${ok ? 'PASS' : 'FAIL'}] ${name}${detail ? ' - ' + detail : ''}`);
}
const near = (a, b, eps = 1e-9) => Math.abs(a - b) <= eps;

// --- заглушка ctx (2D-API, используемое drawSky/drawEnvironment*) ---
function stubCtx() {
    const rec = { fillRects: [], stops: [] };
    return {
        rec,
        ctx: {
            fillStyle: '', strokeStyle: '', globalAlpha: 1, lineWidth: 1, imageSmoothingEnabled: false,
            save() {}, restore() {}, beginPath() {}, closePath() {}, arc() {}, fill() {}, stroke() {},
            clip() {}, drawImage() {}, moveTo() {}, lineTo() {}, quadraticCurveTo() {}, ellipse() {},
            translate() {}, scale() {}, setTransform() {},
            fillRect(x, y, w, h) { rec.fillRects.push({ style: this.fillStyle, x, y, w, h }); },
            createLinearGradient() { return { addColorStop: (o, c) => rec.stops.push({ o, c }) }; },
            createRadialGradient() { return { addColorStop: (o, c) => rec.stops.push({ o, c }) }; },
        },
    };
}
const stubWorld = { farHeight: () => 200 };
const stubCamera = { x: 0, y: 0 };

function mkPkg(seed, extra = {}) {
    return {
        seed,
        sky: { star: { color: '#ffd700', spectral_class: 'G' } },
        suit: { pressure_comfort_atm: [0.5, 3.0] },
        pressure_atm: 1.0,
        biome_category: 'литосфера',
        ...extra,
    };
}
function alphaOf(style) {
    const m = /rgba?\([^)]*,\s*([0-9.]+)\s*\)/.exec(style || '');
    return m ? parseFloat(m[1]) : null;
}

// ==================== T11: доли фаз / функции ====================
const ph = WEATHER_RULES.env.phases;
check('T11 доли фаз 0.40/0.125/0.35/0.125',
    near(ph.night, 0.40) && near(ph.dawn, 0.125) && near(ph.day, 0.35) && near(ph.dusk, 0.125),
    JSON.stringify(ph));

const nf = WEATHER_RULES.env.nightFactor;
check('T11 пороги nightFactor согласованы с долями',
    near(nf.nightEnd, ph.night) && near(nf.dawnEnd, ph.night + ph.dawn) && near(nf.duskStart, 1 - ph.dusk),
    JSON.stringify(nf));

check('T11 phaseForU по границам',
    phaseForU(0.10) === 'ночь' && phaseForU(0.45) === 'рассвет' && phaseForU(0.60) === 'день' && phaseForU(0.95) === 'закат',
    [phaseForU(0.10), phaseForU(0.45), phaseForU(0.60), phaseForU(0.95)].join(','));

check('T11 nightFactor: 1 в ночи, 0 днём',
    nightFactorAt(0) === 1 && nightFactorAt(0.2) === 1 && nightFactorAt(0.4) === 1 &&
    nightFactorAt(0.6) === 0 && nightFactorAt(0.8) === 0 && nightFactorAt(0.875) === 0,
    `nf(0)=${nightFactorAt(0)} nf(0.6)=${nightFactorAt(0.6)} nf(0.875)=${nightFactorAt(0.875)}`);

check('T11 nightFactor: сумерки 1→0 и 0→1',
    near(nightFactorAt(0.4625), 0.5) && near(nightFactorAt(0.9375), 0.5) &&
    near(nightFactorAt(0.5000), 0.2) && near(nightFactorAt(0.9500), 0.6),
    `nf(0.4625)=${nightFactorAt(0.4625)} nf(0.9375)=${nightFactorAt(0.9375)}`);

// монотонность в сумерках + диапазон [0,1] на сетке
let mono = true, inRange = true, prev = nightFactorAt(0.4001);
for (let u = 0.4001; u <= 0.525; u += 0.0005) { const v = nightFactorAt(u); if (v > prev + 1e-12) mono = false; prev = v; }
prev = nightFactorAt(0.8751);
for (let u = 0.8751; u <= 1.0; u += 0.0005) { const v = nightFactorAt(u); if (v < prev - 1e-12) mono = false; prev = v; }
for (let u = 0; u < 1; u += 0.001) { const v = nightFactorAt(u); if (v < 0 || v > 1) inRange = false; }
check('T11 nightFactor монотонен в сумерках и ∈ [0,1]', mono && inRange, `mono=${mono} inRange=${inRange}`);

// ==================== детерминизм и фиксация фазы ====================
const e1 = new SurfaceEnvironment(mkPkg(123456));
const e2 = new SurfaceEnvironment(mkPkg(123456));
e1.elapsed = 100000; e2.elapsed = 100000;
check('детерминизм: (seed, elapsed) → та же фаза/nightFactor/lightMul',
    e1.u() === e2.u() && e1.nightFactor() === e2.nightFactor() && e1.lightMul() === e2.lightMul(),
    `u=${e1.u().toFixed(6)} nf=${e1.nightFactor()} lm=${e1.lightMul()}`);

check('lightMul: ночь 0.25, день 1.0',
    (() => { const e = new SurfaceEnvironment(mkPkg(1)); e.forced = 'ночь'; const a = e.lightMul(); e.forced = 'день'; const b = e.lightMul(); return near(a, 0.25) && near(b, 1.0); })());

check('фиксация фазы держится независимо от elapsed',
    (() => { const e = new SurfaceEnvironment(mkPkg(7)); e.forced = 'ночь'; e.elapsed = 0; const a = e.nightFactor(); e.elapsed = 9e9; const b = e.nightFactor(); return a === 1 && b === 1; })());

// разные seed → разный старт (хотя бы у одного из набора)
const offsets = new Set([1, 2, 3, 4, 5].map((s) => new SurfaceEnvironment(mkPkg(s)).offset));
check('разные seed → разные offset-start (мир начинает в разное время)', offsets.size > 1, `uniq=${offsets.size}`);

// ==================== §6.4: alpha проходов среды ====================
{
    const e = new SurfaceEnvironment(mkPkg(42)); e.forced = 'ночь';
    const { ctx, rec } = stubCtx();
    drawEnvironmentFront(ctx, stubWorld, stubCamera, 1280, 800, e);
    const a = rec.fillRects.length ? alphaOf(rec.fillRects[0].style) : null;
    check('T7 front-тинт ночью: alpha = 1 − lightMul = 0.75', near(a, 0.75), `alpha=${a}`);
}
{
    const e = new SurfaceEnvironment(mkPkg(42)); e.forced = 'день';
    const { ctx, rec } = stubCtx();
    drawEnvironmentFront(ctx, stubWorld, stubCamera, 1280, 800, e);
    check('T7 front-тинт днём отсутствует (alpha 0)', rec.fillRects.length === 0, `fills=${rec.fillRects.length}`);
}
{
    const e = new SurfaceEnvironment(mkPkg(42)); e.forced = 'ночь';
    const { ctx, rec } = stubCtx();
    drawEnvironmentMid(ctx, stubWorld, stubCamera, 1280, 800, e);
    const a = rec.fillRects.length ? alphaOf(rec.fillRects[0].style) : null;
    const expect = 1 - Math.pow(0.25, ENV.worldTint.wMid);
    check('T7 mid-тинт ночью: alpha = 1 − lightMul^wMid', near(a, expect, 1e-9), `alpha=${a} expect=${expect.toFixed(6)}`);
}

// ==================== drawSky: без env — прежний вид; с env — палитра ====================
{
    const { ctx, rec } = stubCtx();
    drawSky(ctx, 1280, 800, { star: { color: '#ffd700' }, bodies: [] }, stubCamera, 1000, undefined);
    check('T11 drawSky без env: палитра = COLORS.skyTop/skyBottom (дневной вид)',
        rec.stops.length >= 2 && rec.stops[0].c === COLORS.skyTop && rec.stops[1].c === COLORS.skyBottom,
        `stops=${rec.stops.slice(0, 2).map((s) => s.c).join('|')}`);
}
{
    const e = new SurfaceEnvironment(mkPkg(42)); e.forced = 'день'; e.elapsed = 0;
    const { ctx, rec } = stubCtx();
    drawSky(ctx, 1280, 800, { star: { color: '#ffd700' }, bodies: [] }, stubCamera, 1000, e);
    const top = rec.stops[0] && rec.stops[0].c;
    // День (вариант D): comfort-палитра '#1b3a63' + подмес star '#ffd700' на 0.25 → rgb(84,97,74).
    check('T11 drawSky с env(день): палитра дня D (давление + sunTint)',
        top === 'rgb(84,97,74)', `top=${top}`);
}

// ==================== T7: alphaStars ====================
{
    const e = new SurfaceEnvironment(mkPkg(42));
    e.forced = 'день';
    const day = e.alphaStars(0);
    e.forced = 'ночь';
    const night = e.alphaStars(0);
    check('T7 alphaStars: 0 днём, 1 в ночи', day === 0 && near(night, 1), `day=${day} night=${night}`);
}
{
    const e = new SurfaceEnvironment(mkPkg(42)); e.forced = 'ночь';
    check('T7 alphaStars гаснут под густым явлением (air.sky ≥ hazeCut → 0)',
        near(e.alphaStars(ENV.stars.hazeCut), 0) && near(e.alphaStars(ENV.stars.hazeCut / 2), 0.5),
        `cut=${e.alphaStars(ENV.stars.hazeCut)} half=${e.alphaStars(ENV.stars.hazeCut / 2)}`);
}

const failed = results.filter((r) => !r.ok).length;
console.log(`SUMMARY: ${results.length - failed} PASS / ${failed} FAIL`);
process.exit(failed ? 1 : 0);
