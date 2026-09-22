// web/static/js/surface/surface_environment.js
// Слой среды прогулки (спека 2026-09-22 §6): детерминированный суточный цикл
// (ночь → рассвет → день → закат) от (seed, elapsed). Палитра неба/дуга светила —
// для drawSky (surface_render.js); свет на мир — тремя проходами
// drawEnvironmentBack/Mid/Front (тот же порядок, что погода, §6.3 п.4).
// Math.random запрещён, единственный хеш — hash1 из surface_world.js (§8 п.2).
// Координаты среднего/переднего прохода — того же трансформа ZOOM, что слои мира
// (§6.5): модуль зум/DPR повторно НЕ применяет. Звёзды — вне трансформа (как sky).
import { ENV, WEATHER_RULES, ZOOM } from './surface_config.js';
import { hash1, parseHex } from './surface_world.js';

const TWO_PI = Math.PI * 2;

function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
function lerp(a, b, t) { return a + (b - a) * t; }
function frac(x) { return x - Math.floor(x); }
function mod(a, n) { return ((a % n) + n) % n; }

// toRgb — принимает и hex-строку, и уже разобранный {r,g,b} (dayTop/dayBottom).
function toRgb(c) { return (c && typeof c === 'object') ? c : parseHex(c); }
function mixRgb(a, b, t) {
    const x = toRgb(a), y = toRgb(b);
    return { r: Math.round(lerp(x.r, y.r, t)), g: Math.round(lerp(x.g, y.g, t)), b: Math.round(lerp(x.b, y.b, t)) };
}
function lerpHex(a, b, t) { return mixRgb(a, b, t); }

// u фазы, которую держит админский чип (полный вид фазы с первого кадра).
const PHASE_U = { 'ночь': 0.20, 'рассвет': 0.4625, 'день': 0.70, 'закат': 0.9375 };

// phaseForU / nightFactorAt — кусочно-линейные функции §6.1 (пороги — WEATHER_RULES.env).
export function phaseForU(u) {
    const { nightEnd, dawnEnd, duskStart } = WEATHER_RULES.env.nightFactor;
    const x = frac(u);
    if (x < nightEnd) return 'ночь';
    if (x < dawnEnd) return 'рассвет';
    if (x < duskStart) return 'день';
    return 'закат';
}

export function nightFactorAt(u) {
    const { nightEnd, dawnEnd, duskStart } = WEATHER_RULES.env.nightFactor;
    const x = frac(u);
    if (x < nightEnd) return 1;
    if (x < dawnEnd) return 1 - (x - nightEnd) / (dawnEnd - nightEnd);
    if (x < duskStart) return 0;
    return (x - duskStart) / (1 - duskStart);
}

// dayBase — база дневной палитры (вариант D) по полосам pressure_atm: границы —
// из suit.pressure_comfort_atm (своих порогов не заводим).
function dayBase(pkg) {
    const suit = pkg.suit || {};
    const [pMin, pMax] = Array.isArray(suit.pressure_comfort_atm) ? suit.pressure_comfort_atm : [0.5, 3.0];
    const p = pkg.pressure_atm;
    if (typeof p === 'number' && p < pMin) return ENV.dayByPressure.thin;
    if (typeof p === 'number' && p > pMax) return ENV.dayByPressure.dense;
    return ENV.dayByPressure.comfort;
}

export class SurfaceEnvironment {
    constructor(pkg) {
        this.seed = (pkg.seed | 0) >>> 0;
        this.sky = pkg.sky || {};
        this.starColor = (this.sky.star && this.sky.star.color) || '#ffd700';
        // Период суток 6–10 мин — детерминирован от seed.
        const [dMin, dMax] = WEATHER_RULES.env.dayMs;
        this.dayMs = Math.max(1, lerp(dMin, dMax, hash1(this.seed, 0x7d31)));
        this.offset = hash1(this.seed, 0x3f21);   // разные миры начинают в разное время
        this.elapsed = 0;                          // ms от старта прогулки (после брифинга)
        this.forced = null;                        // админская фаза; null = авто-цикл
        this.starCount = Math.round(lerp(ENV.stars.count[0], ENV.stars.count[1], hash1(this.seed, 0x2b1d)));
        // День (D): палитра по давлению + подмес star.color на sunTint.
        const base = dayBase(pkg);
        this.dayTop = lerpHex(base.top, this.starColor, ENV.sunTint);
        this.dayBottom = lerpHex(base.bottom, this.starColor, ENV.sunTint);
        // Ночной тинт мира (N2): worldTint.color с подмесом star.color на starMix.
        this.tint = mixRgb(parseHex(ENV.worldTint.color), parseHex(this.starColor), ENV.worldTint.starMix);
    }

    // u — доля суток [0,1); детерминирована от (seed, elapsed).
    u() {
        if (this.forced) return PHASE_U[this.forced];
        return frac(this.offset + this.elapsed / this.dayMs);
    }
    nightFactor() { return nightFactorAt(this.u()); }
    lightMul() {
        const nf = this.nightFactor();
        return ENV.lightMul.night + (ENV.lightMul.day - ENV.lightMul.night) * (1 - nf);
    }
    phase() { return this.forced || phaseForU(this.u()); }

    paletteByName(name) {
        if (name === 'день') return { top: this.dayTop, bottom: this.dayBottom };
        return ENV.palettes[name];
    }

    // skyColors — интерполяция палитры неба по u (не по nightFactor): каждая фаза
    // имеет свой точный цвет, переходы непрерывны, в т.ч. на стыке цикла.
    skyColors() {
        const u = this.u();
        const keys = ENV.skyKeys;
        for (let i = 0; i < keys.length - 1; i++) {
            const [u0, n0] = keys[i], [u1, n1] = keys[i + 1];
            if (u >= u0 && u <= u1) {
                const t = u1 > u0 ? (u - u0) / (u1 - u0) : 0;
                const a = this.paletteByName(n0), b = this.paletteByName(n1);
                return { top: lerpHex(a.top, b.top, t), bottom: lerpHex(a.bottom, b.bottom, t) };
            }
        }
        const p = this.paletteByName(keys[keys.length - 1][1]);
        return { top: p.top, bottom: p.bottom };
    }

    // sunPose — светило внутри окна дуги [arcStart, arcStart+arcSpan] (§6.2);
    // вне окна — за горизонтом (null). alt быстро поднимается из сумерек.
    sunPose(vw, vh, camera) {
        const s = ENV.sun;
        const p = (this.u() - s.arcStart) / s.arcSpan;
        if (p < 0 || p > 1) return null;
        const alt = Math.pow(Math.sin(Math.PI * p), s.altPow);
        const x = vw * lerp(s.xFrac[0], s.xFrac[1], p) - camera.x * s.parallax;
        const y = vh * lerp(s.yHorizon, s.yZenith, alt);
        const r = vh * lerp(s.radius[0], s.radius[1], alt);
        return { x, y, r, alpha: 1 - this.nightFactor(), color: this.starColor, haloMul: s.haloMul, haloAlpha: s.haloAlpha };
    }

    // alphaStars — единое правило §6.4: (nf − rampLo)/(rampHi − rampLo), затем
    // гашение густым явлением по air.sky (звёзды не поверх пелены).
    alphaStars(airSky) {
        const st = ENV.stars;
        let a = clamp((this.nightFactor() - st.rampLo) / (st.rampHi - st.rampLo), 0, 1);
        a *= clamp(1 - (airSky || 0) / st.hazeCut, 0, 1);
        return a;
    }

    // drawStars — звёздное поле: мировые колонки (тайл), своя фаза/частота мерцания
    // на звезду, цвет cool/warm по warmShare, fillRect (без градиентов). Рисуется
    // вне трансформа ZOOM (как sky). Только выше линии горизонта (звёзды — небо).
    drawStars(ctx, world, camera, vw, vh, airSky, tSec) {
        const a = this.alphaStars(airSky);
        if (a <= 0.001) return;
        const st = ENV.stars;
        const S = st.tile, V = st.vTile;
        const cool = parseHex(st.cool), warm = parseHex(st.warm);
        const seed = this.seed;
        const hyLogical = world.farHeight(camera.x * 0.35) - camera.y * 0.35 + vh * 0.35;
        const hyScreen = (hyLogical - vh / 2) * ZOOM + vh / 2;
        for (let i = 0; i < this.starCount; i++) {
            const hx = hash1(i, (seed ^ 0x5717) >>> 0);
            const hy = hash1(i, (seed ^ (0x5717 ^ 0x7777)) >>> 0);
            const hp = hash1(i, (seed ^ (0x5717 ^ 0x1234)) >>> 0);
            const hz = hash1(i, (seed ^ (0x5717 ^ 0x9abc)) >>> 0);
            const hw = hash1(i, (seed ^ (0x5717 ^ 0x55)) >>> 0);
            const par = lerp(st.parallax[0], st.parallax[1], hp);
            const size = lerp(st.size[0], st.size[1], hp);
            const hzHz = lerp(st.twinkleHz[0], st.twinkleHz[1], hz);
            const amp = lerp(st.twinkleAmp[0], st.twinkleAmp[1], hz);
            const tw = 1 - amp * (0.5 + 0.5 * Math.sin(TWO_PI * hzHz * tSec + hw * TWO_PI));
            const alpha = lerp(st.alpha[0], st.alpha[1], hp) * a * tw;
            if (alpha <= 0.01) continue;
            const col = hp < st.warmShare ? warm : cool;
            const sx0 = mod(hx * S - camera.x * par, S);
            const sy0 = mod(hy * V - camera.y * par, V);
            ctx.fillStyle = `rgba(${col.r},${col.g},${col.b},${clamp(alpha, 0, 1)})`;
            const yMax = Math.min(vh, hyScreen);
            for (let kx = Math.ceil((0 - sx0) / S); kx <= Math.floor((vw - sx0) / S); kx++) {
                const sx = sx0 + kx * S;
                for (let ky = Math.ceil((0 - sy0) / V); ky <= Math.floor((yMax - sy0) / V); ky++) {
                    const sy = sy0 + ky * V;
                    if (sy > hyScreen) continue;
                    ctx.fillRect(sx, sy, size, size);
                }
            }
        }
    }
}

function rgbaRgb(c, a) { return `rgba(${c.r},${c.g},${c.b},${clamp(a, 0, 1)})`; }

// horizonY — та же линия горизонта, что у дальнего силуэта/погоды (§6.3).
function horizonY(world, camera, vh) {
    return world.farHeight(camera.x * 0.35) - camera.y * 0.35 + vh * 0.35;
}

// drawEnvironmentBack — после неба, до погодного back: звёзды. Небо НЕ темнит —
// палитра темнеет один раз в drawSky (один источник цвета).
export function drawEnvironmentBack(ctx, world, camera, vw, vh, env, weather) {
    if (!env) return;
    const airSky = (weather && weather.params) ? weather.params.air.sky : 0;
    env.drawStars(ctx, world, camera, vw, vh, airSky, performance.now() / 1000);
}

// drawEnvironmentMid — после дальнего силуэта: тинт ниже горизонта,
// alpha = 1 − lightMul^wMid. Дальний/средний план уходят в темноту сильнее ближнего
// (Mid ложится на силуэт, но ПОД рельеф — §6.3 п.3).
export function drawEnvironmentMid(ctx, world, camera, vw, vh, env) {
    if (!env) return;
    const alpha = 1 - Math.pow(env.lightMul(), ENV.worldTint.wMid);
    if (alpha <= 0.001) return;
    const hy = horizonY(world, camera, vh);
    ctx.fillStyle = rgbaRgb(env.tint, alpha);
    ctx.fillRect(0, hy, vw, vh);
}

// drawEnvironmentFront — после погодного front: тинт ниже горизонта, alpha = 1 − lightMul
// (ближний рельеф/декор/игрок получают ровно lightMul).
export function drawEnvironmentFront(ctx, world, camera, vw, vh, env) {
    if (!env) return;
    const alpha = 1 - env.lightMul();
    if (alpha <= 0.001) return;
    const hy = horizonY(world, camera, vh);
    ctx.fillStyle = rgbaRgb(env.tint, alpha);
    ctx.fillRect(0, hy, vw, vh);
}
