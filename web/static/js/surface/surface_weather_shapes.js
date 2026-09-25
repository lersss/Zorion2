// web/static/js/surface/surface_weather_shapes.js
// Примитивы отрисовки 5 новых явлений погоды прогулки (спека 2026-09-22 §4.2,
// направление @gdesigner §3.1–3.5): lightning (разряд+облако), glint (блеск),
// pellet (гранулы с отскоком), ash (тёмные хлопья), glow (аддитивное свечение).
// Модуль самодостаточен: координаты — мировые/экранно-логические ТОГО ЖЕ
// трансформа, что у слоёв мира (§6.5), зум/DPR повторно не применяется.
// Math.random запрещён — только hash1 из surface_world.js (§8 п.2). Ореол glow
// запекается один раз (как bakeAurora), покадровых градиентов на частицы нет.
import { ZOOM } from './surface_config.js';
import { hash1, parseHex } from './surface_world.js';

const TWO_PI = Math.PI * 2;

function mod(a, n) { return ((a % n) + n) % n; }
function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
function lerp(a, b, t) { return a + (b - a) * t; }
function rgbCss(c) { return `rgb(${c.r},${c.g},${c.b})`; }
function rgbaCss(c, a) { return `rgba(${c.r},${c.g},${c.b},${clamp(a, 0, 1)})`; }
// rnd — значение из диапазона [lo, hi] по доле h; уже развёрнутое число — как есть.
function rnd(r, h) { return Array.isArray(r) ? lerp(r[0], r[1], h) : r; }
function mixRgb(a, b, t) {
    return {
        r: Math.round(lerp(a.r, b.r, t)),
        g: Math.round(lerp(a.g, b.g, t)),
        b: Math.round(lerp(a.b, b.b, t)),
    };
}
function scaleRgb(c, mul) {
    if (mul === 1) return c;
    return { r: Math.round(c.r * mul), g: Math.round(c.g * mul), b: Math.round(c.b * mul) };
}

function viewWindow(vw, vh) {
    const hw = vw / (2 * ZOOM), hh = vh / (2 * ZOOM);
    return { x0: vw / 2 - hw, x1: vw / 2 + hw, y0: vh / 2 - hh, y1: vh / 2 + hh };
}

function horizonY(world, camera, vh) {
    return world.farHeight(camera.x * 0.35) - camera.y * 0.35 + vh * 0.35;
}

function groundY(world, wx, camera, vh) {
    // Якорь приземных эффектов — `skyTop` (§5 п.12): «поверхность неба» (низ
    // плиты/вал/верх свода) — осадки ложатся на кровлю, внутрь полости не сыплются.
    return world.skyTop(wx) - camera.y + vh / 2;
}

// windOffset — тот же снос, что у базовых поясов (§5.1 п.5 пакета 1): интеграл
// модулятора ветра; направление — цикловой PRNG. Копия формулы surface_weather.js.
function windOffset(p, tSec) {
    const { base, dir, period, range, phase } = p.wind;
    if (!base) return 0;
    const w = TWO_PI / Math.max(0.001, period);
    const A = range[0] + (range[1] - range[0]) * 0.5;
    const B = (range[1] - range[0]) * 0.5 / w;
    return dir * base * (A * tSec + B * (Math.cos(phase) - Math.cos(w * tSec + phase)));
}

function slantDir(p) {
    if (!p.slant) return { dx: 0, dy: 1 };
    const t = Math.tan(p.slant.deg * Math.PI / 180);
    return p.slant.from === 'horizontal' ? { dx: 1, dy: t } : { dx: t, dy: 1 };
}

// ==================== ГРОЗА: облачный пояс (не эмиссивный, × lightMul) ====================

function drawCloud(ctx, x, y, w, h, col, alpha) {
    if (alpha <= 0 || w <= 0 || h <= 0) return;
    ctx.save();
    ctx.translate(x, y);
    ctx.scale(1, h / w);
    const g = ctx.createRadialGradient(0, 0, 0, 0, 0, w / 2);
    g.addColorStop(0, rgbaCss(col, alpha));
    g.addColorStop(0.6, rgbaCss(col, alpha * 0.6));
    g.addColorStop(1, rgbaCss(col, 0));
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(0, 0, w / 2, 0, TWO_PI);
    ctx.fill();
    ctx.restore();
}

// drawLightningCloud — тёмный слоистый облачный пояс (направление §3.1). Рисуется
// в погодном проходе (back), потому airColor пояса умножается на lightMul.
export function drawLightningCloud(ctx, world, camera, vw, vh, p, tSec, lightMul) {
    const L = p.lightning;
    if (!L) return;
    const C = L.cloud;
    const win = viewWindow(vw, vh);
    const S = L.tile, par = L.parallax;
    const col = scaleRgb(mixRgb(p.air.color, parseHex(C.color), C.blend), lightMul);
    const hy = horizonY(world, camera, vh);
    const seed = world.seed >>> 0;
    const count = Math.max(1, Math.round(rnd(C.count, 0.5)));
    for (let i = 0; i < count; i++) {
        const hx = hash1(i, (seed ^ 0xc10d) >>> 0);
        const hp = hash1(i, (seed ^ (0xc10d ^ 0x1234)) >>> 0);
        const hh = hash1(i, (seed ^ (0xc10d ^ 0x7777)) >>> 0);
        const w = rnd(C.w, hp);
        const h = rnd(C.h, hp);
        const alpha = rnd(C.alpha, hp);
        const drift = Math.sin(tSec * 0.22 + hp * TWO_PI) * rnd(C.drift, hh);
        const sx0 = mod(hx * S - camera.x * par + drift, S);
        const y = hy + rnd(C.offset, hh);
        for (let kx = Math.ceil((win.x0 - 120 - sx0) / S); kx <= Math.floor((win.x1 + 120 - sx0) / S); kx++) {
            drawCloud(ctx, sx0 + kx * S, y, w, h, col, alpha);
        }
    }
}

// ==================== ГРОЗА: разряд + вспышка + эхо + groundGlow (эмиссивные) ====================

function strokePolyline(ctx, pts, w, style) {
    if (pts.length < 2) return;
    ctx.strokeStyle = style;
    ctx.lineWidth = w;
    ctx.beginPath();
    ctx.moveTo(pts[0].x, pts[0].y);
    for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y);
    ctx.stroke();
}

// drawBranch — рекурсивная мелкая ветвь разряда (детерминирована от hash1).
function drawBranch(ctx, x, y, dx, dy, n, depth, seed, salt, style, w, alpha) {
    const pts = [{ x, y }];
    let px = x, py = y;
    for (let k = 0; k < n; k++) {
        const h = hash1(k + n * 13, (seed ^ salt) >>> 0);
        px += dx + (h - 0.5) * 26;
        py += dy;
        pts.push({ x: px, y: py });
    }
    strokePolyline(ctx, pts, w, rgbaCss(style, alpha));
    if (depth > 0 && n > 1) {
        const hb = hash1(depth, (seed ^ (salt + 313)) >>> 0);
        drawBranch(ctx, pts[1].x, pts[1].y, (hb < 0.5 ? -1 : 1) * 14, dy * 1.02, Math.max(2, n - 2), depth - 1, seed, salt + 7, style, w * 0.7, alpha * 0.85);
    }
}

// drawLightning — ломаная ветвь разряда от облачного пояса к земле + вспышка
// кадра и эхо (эмиссивно: НЕ × lightMul; вспышка/эхо × (1 + 1.2·nightFactor)).
export function drawLightning(ctx, world, camera, vw, vh, p, tSec, nightFactor) {
    const L = p.lightning;
    if (!L) return;
    const rate = L.boltRate;
    const period = 1 / Math.max(0.05, rate);
    const burst = Math.max(1, Math.round(L.boltBurst));
    const segs = Math.max(3, Math.round(L.segments));
    const branchDepth = Math.max(0, Math.round(L.branchDepth));
    const flashDur = L.flashMs / 1000;
    const echoDur = L.echoMs / 1000;
    const nightMul = 1 + 1.2 * nightFactor;
    const flashA = L.flashAlpha * nightMul;
    const echoA = L.echoAlpha * nightMul;
    const win = viewWindow(vw, vh);
    const S = L.tile, par = L.parallax;
    const hy = horizonY(world, camera, vh);
    const seed = world.seed >>> 0;
    const col = parseHex(L.color);
    const bright = mixRgb(col, { r: 255, g: 255, b: 255 }, 0.55);
    const ep = Math.floor(tSec / period);

    for (let e = ep; e >= ep - 1; e--) {
        // вспышка эпизода — максимум по его разрядам, рисуется один раз
        let flash = 0;
        for (let j = 0; j < burst; j++) {
            const life = tSec - (e * period + j * 0.09);
            if (life < 0 || life > flashDur + echoDur) continue;
            flash = Math.max(flash, life < flashDur ? flashA : echoA * (1 - (life - flashDur) / echoDur));
        }
        if (flash > 0.002) {
            ctx.save();
            ctx.globalCompositeOperation = 'lighter';
            ctx.fillStyle = rgbaCss(bright, flash);
            ctx.fillRect(0, 0, vw, vh);
            ctx.restore();
        }
        for (let j = 0; j < burst; j++) {
            const life = tSec - (e * period + j * 0.09);
            if (life < 0 || life > flashDur + echoDur) continue;
            const env = clamp(flash / Math.max(flashA, 1e-3), 0, 1);
            const salt = (0x1e5 ^ (e * 131 + j * 17)) >>> 0;
            const hx = hash1(e * 31 + j, (seed ^ 0xb017) >>> 0);
            const sx0 = mod(hx * S - camera.x * par, S);
            for (let kx = Math.ceil((win.x0 - 200 - sx0) / S); kx <= Math.floor((win.x1 + 200 - sx0) / S); kx++) {
                const x0 = sx0 + kx * S;
                const footX = x0 + (hash1(0, (seed ^ salt) >>> 0) - 0.5) * 40;
                const footY = groundY(world, footX - vw / 2 + camera.x, camera, vh);
                const pts = [{ x: x0, y: hy }];
                let cx = 0;
                for (let k = 1; k <= segs; k++) {
                    cx = cx * 0.55 + (hash1(k, (seed ^ salt) >>> 0) - 0.5) * 0.6;
                    pts.push({ x: x0 + cx * 70, y: lerp(hy, footY, k / segs) });
                }
                ctx.save();
                ctx.globalCompositeOperation = 'lighter';
                ctx.lineCap = 'round';
                strokePolyline(ctx, pts, L.width.glow, rgbaCss(col, 0.18 * (0.4 + env)));
                strokePolyline(ctx, pts, L.width.core, rgbaCss(col, 0.95 * (0.6 + 0.4 * env)));
                for (let k = 1; k < segs; k++) {
                    const hb = hash1(k, (seed ^ (salt + 977)) >>> 0);
                    if (hb > L.branchChance) continue;
                    drawBranch(ctx, pts[k].x, pts[k].y, (hb < L.branchChance / 2 ? -1 : 1) * 13, (footY - hy) / segs * 0.8, 3, branchDepth - 1, seed, salt + 131 * k, col, L.width.core * 0.7, 0.8);
                }
                // подсветка рельефа в точке удара
                if (L.groundGlow > 0 && env > 0.01) {
                    ctx.fillStyle = rgbaCss(bright, L.groundGlow * env);
                    ctx.beginPath();
                    ctx.ellipse(pts[segs].x, footY, 48, 9, 0, 0, TWO_PI);
                    ctx.fill();
                }
                ctx.restore();
            }
        }
    }
}

// ==================== ЛЕДЯНЫЕ ИГЛЫ: блеск-кристалл (glint) ====================

function drawGlint(ctx, x, y, size, alpha, col, cross, rayLen, haloR, haloA) {
    ctx.fillStyle = rgbaCss(col, alpha * haloA);
    ctx.beginPath();
    ctx.arc(x, y, haloR, 0, TWO_PI);
    ctx.fill();
    ctx.fillStyle = rgbaCss(col, alpha);
    ctx.fillRect(x - size / 2, y - size / 2, size, size);
    if (cross) {
        ctx.strokeStyle = rgbaCss(col, alpha * 0.7);
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(x - rayLen, y); ctx.lineTo(x + rayLen, y);
        ctx.moveTo(x, y - rayLen); ctx.lineTo(x, y + rayLen);
        ctx.stroke();
    }
}

// drawGlints — редкие искры в чистом воздухе + блики у земли (направление §3.2).
// Нерегулярное мерцание: у каждой искры своя фаза/частота (hash1), общего синуса
// нет. Блеск × (1 + (lowSunBoost−1)·lowSun), lowSun пик в сумерках (§6.4).
export function drawGlints(ctx, world, camera, vw, vh, p, tSec, lowSun) {
    const G = p.glint;
    if (!G) return;
    const win = viewWindow(vw, vh);
    const S = G.tile, V = G.vTile;
    const seed = world.seed >>> 0;
    const col = parseHex((G.palette && G.palette[p.variant]) || (G.palette && G.palette['иней']) || '#e2e9f2');
    const boost = 1 + (G.lowSunBoost - 1) * lowSun;
    const x0 = win.x0 - 40, x1 = win.x1 + 40, y0 = win.y0 - 40, y1 = win.y1 + 40;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';

    const nAir = Math.max(1, Math.round(rnd(G.airCount, 0.5)));
    for (let i = 0; i < nAir; i++) {
        const hx = hash1(i, (seed ^ 0x9111) >>> 0);
        const hy = hash1(i, (seed ^ (0x9111 ^ 0x7777)) >>> 0);
        const hp = hash1(i, (seed ^ (0x9111 ^ 0x1234)) >>> 0);
        const hc = hash1(i, (seed ^ (0x9111 ^ 0x9abc)) >>> 0);
        const hf = hash1(i, (seed ^ (0x9111 ^ 0x55)) >>> 0);
        const size = rnd(G.airSize, hp);
        const hz = rnd(G.pulseHz, hc) * rnd(G.pulseJitter, hf);
        const pulse = 0.5 + 0.5 * Math.sin(TWO_PI * hz * tSec + hf * TWO_PI);
        const alpha = rnd(G.airAlpha, hp) * boost * (0.35 + 0.65 * pulse);
        if (alpha <= 0.012) continue;
        const sx0 = mod(hx * S - camera.x, S);
        const sy0 = mod(hy * V - camera.y, V);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                drawGlint(ctx, sx, sy0 + ky * V, size, alpha, col, hc > G.crossMin, rnd(G.rayLen, hp), rnd(G.haloRadius, hp), rnd(G.haloAlpha, hp));
            }
        }
    }

    const nLow = Math.max(1, Math.round(rnd(G.lowCount, 0.5)));
    for (let i = 0; i < nLow; i++) {
        const hx = hash1(i, (seed ^ 0x9221) >>> 0);
        const hy = hash1(i, (seed ^ (0x9221 ^ 0x7777)) >>> 0);
        const hp = hash1(i, (seed ^ (0x9221 ^ 0x1234)) >>> 0);
        const hc = hash1(i, (seed ^ (0x9221 ^ 0x9abc)) >>> 0);
        const hf = hash1(i, (seed ^ (0x9221 ^ 0x55)) >>> 0);
        const size = rnd(G.lowSize, hp);
        const hz = rnd(G.pulseHz, hc) * rnd(G.pulseJitter, hf);
        const pulse = 0.5 + 0.5 * Math.sin(TWO_PI * hz * tSec + hf * TWO_PI);
        const alpha = rnd(G.lowAlpha, hp) * boost * (0.35 + 0.65 * pulse);
        if (alpha <= 0.012) continue;
        const sx0 = mod(hx * S - camera.x, S);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S;
            const gy = groundY(world, sx - vw / 2 + camera.x, camera, vh);
            const sy = gy - rnd(G.lowBand, hy);
            drawGlint(ctx, sx, sy, size, alpha, col, hc > G.crossMin, rnd(G.rayLen, hp), rnd(G.haloRadius, hp), rnd(G.haloAlpha, hp));
        }
    }
    ctx.restore();
}

// ==================== ЛЕДЯНОЙ ГРАД: гранулы с отскоком (pellet) ====================

export function drawPellets(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec, win) {
    const S = belt.tile, V = belt.vTile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const P = p.pellet;
    const seed = world.seed >>> 0;
    const core = parseHex(P.colorCore);
    const rim = parseHex(P.colorRim);
    const x0 = win.x0 - 40, x1 = win.x1 + 40, y0 = win.y0 - 40, y1 = win.y1 + 40;
    const salt = 0x9e11 + beltIndex * 0x1f3b;
    const bounce = !!belt.bounce;
    const bounceN = Math.max(1, Math.round(rnd(P.bounceCount, 0.5)));
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (seed ^ salt) >>> 0);
        const hy = hash1(i, (seed ^ (salt ^ 0x7777)) >>> 0);
        const hp = hash1(i, (seed ^ (salt ^ 0x1234)) >>> 0);
        const hf = hash1(i, (seed ^ (salt ^ 0x55)) >>> 0);
        const size = rnd(belt.size, hp);
        const alpha = rnd(belt.alpha, hp);
        const fall = rnd(belt.fall, hf);
        const bounceH = rnd(P.bounceH, hp);
        const rimA = rnd(P.rimAlpha, hp);
        const sway = belt.sway ? Math.sin(tSec * 1.3 + hp * TWO_PI) * belt.sway : 0;
        const sx0 = mod(hx * S - camera.x * par + wind, S);
        const sy0 = mod(hy * V - camera.y * par + fall * tSec, V);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S + sway;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                let sy = sy0 + ky * V;
                let a = alpha;
                if (bounce) {
                    const gy = groundY(world, sx - vw / 2 + camera.x, camera, vh);
                    const below = sy - gy;
                    if (below > 0) {
                        const damp = Math.pow(P.bounceDamping, Math.min(bounceN, Math.floor(below / 60)));
                        sy = gy - Math.abs(Math.sin(below * 0.15 + hp * TWO_PI)) * bounceH * damp * Math.exp(-below * 0.012);
                        a = alpha * (0.55 + 0.45 * damp);
                    }
                }
                ctx.fillStyle = rgbaCss(core, a);
                ctx.beginPath();
                ctx.arc(sx, sy, size, 0, TWO_PI);
                ctx.fill();
                if (rimA > 0.01) {
                    ctx.strokeStyle = rgbaCss(rim, a * rimA);
                    ctx.lineWidth = 1;
                    ctx.stroke();
                }
            }
        }
    }
}

// drawPelletGround — тонкий слой крупы на локальных максимумах рельефа (гранулы,
// не «шапки», направление §3.3).
export function drawPelletGround(ctx, world, camera, vw, vh, p) {
    const P = p.pellet;
    if (!P || !P.grain) return;
    const G = P.grain;
    const win = viewWindow(vw, vh);
    const core = parseHex(P.colorCore);
    const seed = world.seed >>> 0;
    const count = Math.max(1, Math.round(rnd(G.count, 0.5)));
    for (let i = 0; i < count; i++) {
        const h = hash1(i, (seed ^ 0x6a17) >>> 0);
        const hp = hash1(i, (seed ^ (0x6a17 ^ 0x1234)) >>> 0);
        const x = win.x0 + h * (win.x1 - win.x0);
        const wx = x - vw / 2 + camera.x;
        const th0 = world.skyTop(wx - 8), th1 = world.skyTop(wx), th2 = world.skyTop(wx + 8);
        if (!(th1 <= th0 && th1 <= th2)) continue;
        const gy = th1 - camera.y + vh / 2;
        const size = rnd(G.size, hp);
        ctx.fillStyle = rgbaCss(core, rnd(G.alpha, hp));
        ctx.beginPath();
        ctx.ellipse(x, gy + 1, size * 1.6, size * 0.9, 0, 0, TWO_PI);
        ctx.fill();
    }
}

// ==================== ПЕПЕЛЬНЫЙ ДОЖДЬ: тёмные хлопья + налёт (ash) ====================

function ashRgb(p) {
    const A = p.ash;
    const biome = parseHex(p.biomeColor || '#8a7a6a');
    return mixRgb(scaleRgb(biome, A.shade), parseHex(A.tint), 0.35);
}

export function drawAsh(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec, win) {
    const S = belt.tile, V = belt.vTile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const A = p.ash;
    const col = ashRgb(p);
    const seed = world.seed >>> 0;
    const x0 = win.x0 - 40, x1 = win.x1 + 40, y0 = win.y0 - 40, y1 = win.y1 + 40;
    const salt = 0xa511 + beltIndex * 0x1f3b;
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (seed ^ salt) >>> 0);
        const hy = hash1(i, (seed ^ (salt ^ 0x7777)) >>> 0);
        const hp = hash1(i, (seed ^ (salt ^ 0x1234)) >>> 0);
        const hf = hash1(i, (seed ^ (salt ^ 0x55)) >>> 0);
        const hr = hash1(i, (seed ^ (salt ^ 0x9abc)) >>> 0);
        const size = rnd(belt.size, hp);
        const alpha = rnd(belt.alpha, hp);
        const fall = rnd(belt.fall, hf);
        const lenF = rnd(A.flakeLen, hp);
        const rot = (hr - 0.5) * rnd(A.flakeJitter, hp) * 1.5;
        const sway = belt.sway ? Math.sin(tSec * 1.1 + hp * TWO_PI) * belt.sway : 0;
        const sx0 = mod(hx * S - camera.x * par + wind, S);
        const sy0 = mod(hy * V - camera.y * par + fall * tSec, V);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S + sway;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                const sy = sy0 + ky * V;
                ctx.save();
                ctx.translate(sx, sy);
                ctx.rotate(rot);
                ctx.fillStyle = rgbaCss(col, alpha);
                ctx.beginPath();
                ctx.ellipse(0, 0, size, size * (1 + lenF * 0.5), 0, 0, TWO_PI);
                ctx.fill();
                ctx.restore();
            }
        }
    }
}

// drawAshGround — тёмный налёт/полоса у «поверхности неба» skyTop (§5 п.12);
// у «влажного» — потёки.
export function drawAshGround(ctx, world, camera, vw, vh, p) {
    const A = p.ash;
    if (!A || !A.groundBand) return;
    const band = A.groundBand;
    const win = viewWindow(vw, vh);
    const col = ashRgb(p);
    const step = band.step || 18;
    const seed = world.seed >>> 0;
    for (let sx = win.x0; sx <= win.x1; sx += step) {
        const wx = sx - vw / 2 + camera.x;
        const h0 = hash1(Math.floor(wx / step), (seed ^ 0x4a11) >>> 0);
        const gy = world.skyTop(wx) - camera.y + vh / 2;
        const h = rnd(band.h, h0);
        ctx.fillStyle = rgbaCss(col, rnd(band.alpha, h0));
        ctx.fillRect(sx, gy - h * 0.4, step + 1, h);
    }
    if (A.wetStreaks) {
        ctx.strokeStyle = rgbaCss(col, 0.45);
        ctx.lineWidth = 2;
        for (let sx = win.x0; sx <= win.x1; sx += step * 2) {
            const wx = sx - vw / 2 + camera.x;
            const h0 = hash1(Math.floor(wx / step), (seed ^ 0x7e11) >>> 0);
            if (h0 < 0.5) continue;
            const gy = world.skyTop(wx) - camera.y + vh / 2;
            ctx.beginPath();
            ctx.moveTo(sx, gy - 2);
            ctx.lineTo(sx + (h0 - 0.5) * 6, gy + 10 + h0 * 18);
            ctx.stroke();
        }
    }
}

// ==================== СВЕЧЕНИЕ: аддитивные светящиеся частицы (glow) ====================

const _haloCache = new Map();
function bakeHalo(hex) {
    if (_haloCache.has(hex)) return _haloCache.get(hex);
    const size = 64;
    const c = document.createElement('canvas');
    c.width = size; c.height = size;
    const g = c.getContext('2d');
    const col = parseHex(hex);
    const grad = g.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
    grad.addColorStop(0, rgbaCss(col, 1));
    grad.addColorStop(0.35, rgbaCss(col, 0.5));
    grad.addColorStop(1, rgbaCss(col, 0));
    g.fillStyle = grad;
    g.fillRect(0, 0, size, size);
    _haloCache.set(hex, c);
    return c;
}

// drawGlow — светящиеся тела + запечённые ореолы, аддитивно (lighter). Яркость
// × (nightBoost[0] + nightBoost[1]·nightFactor) (§6.4): ночью ярко, днём бледно.
export function drawGlow(ctx, world, camera, vw, vh, p, tSec, nightFactor) {
    const G = p.glow;
    if (!G) return;
    const win = viewWindow(vw, vh);
    const S = G.tile, V = G.vTile;
    const pal = (G.palette && (G.palette[world.category] || G.palette['биосфера'])) || ['#6ef0a0', '#8f7cff'];
    const c1 = parseHex(pal[0]), c2 = parseHex(pal[1]);
    const halo1 = bakeHalo(pal[0]), halo2 = bakeHalo(pal[1]);
    const seed = world.seed >>> 0;
    const count = Math.max(1, Math.round(rnd(G.count, 0.5)));
    const brightness = G.nightBoost[0] + G.nightBoost[1] * nightFactor;
    const x0 = win.x0 - 40, x1 = win.x1 + 40, y0 = win.y0 - 40, y1 = win.y1 + 40;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    for (let i = 0; i < count; i++) {
        const hx = hash1(i, (seed ^ 0x9101) >>> 0);
        const hy = hash1(i, (seed ^ (0x9101 ^ 0x7777)) >>> 0);
        const hp = hash1(i, (seed ^ (0x9101 ^ 0x1234)) >>> 0);
        const hc = hash1(i, (seed ^ (0x9101 ^ 0x9abc)) >>> 0);
        const hz = hash1(i, (seed ^ (0x9101 ^ 0x55)) >>> 0);
        const size = rnd(G.size, hp);
        const haloR = rnd(G.haloSize, hp);
        const drift = rnd(G.drift, hc);
        const rise = rnd(G.rise, hz);
        const hzHz = rnd(G.pulseHz, hc);
        const pulse = 0.55 + 0.45 * Math.sin(TWO_PI * hzHz * tSec + hz * TWO_PI);
        const alpha = brightness * pulse;
        const first = hp < 0.5;
        const col = first ? c1 : c2;
        const halo = first ? halo1 : halo2;
        const sx0 = mod(hx * S - camera.x + drift * tSec, S);
        const sy0 = mod(hy * V - camera.y + rise * tSec, V);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                const sy = sy0 + ky * V;
                ctx.globalAlpha = clamp(rnd(G.alphaHalo, hp) * alpha, 0, 1);
                ctx.drawImage(halo, sx - haloR, sy - haloR, haloR * 2, haloR * 2);
                ctx.globalAlpha = clamp(rnd(G.alphaBody, hp) * alpha, 0, 1);
                ctx.fillStyle = rgbCss(col);
                ctx.beginPath();
                ctx.arc(sx, sy, size, 0, TWO_PI);
                ctx.fill();
            }
        }
    }
    ctx.restore();
}

// ==================== МЕТЕЛЬ: снежинка (flake, §5.1) ====================

// drawFlakeShape — мягкий двухтоновый диск (светлое ядро coreRatio радиуса, край
// темнее) + у крупных (size ≥ 6 px) 5–6 лучей. Диск — два плоских круга, без
// покадрового радиального градиента (§8). Наклон лучей — косой лёт (tilt, град.).
function drawFlakeShape(ctx, x, y, size, alpha, core, edge, coreRatio, arms, armLen, tiltDeg) {
    const r = size / 2;
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(tiltDeg * Math.PI / 180);
    ctx.fillStyle = rgbaCss(edge, alpha);
    ctx.beginPath();
    ctx.arc(0, 0, r, 0, TWO_PI);
    ctx.fill();
    ctx.fillStyle = rgbaCss(core, alpha);
    ctx.beginPath();
    ctx.arc(0, 0, r * coreRatio, 0, TWO_PI);
    ctx.fill();
    if (size >= 6) {
        ctx.strokeStyle = rgbaCss(core, alpha * 0.9);
        ctx.lineWidth = Math.max(1, r * 0.22);
        const L = r * (1 + armLen);
        for (let a = 0; a < arms; a++) {
            const ang = (a / arms) * TWO_PI;
            ctx.beginPath();
            ctx.moveTo(Math.cos(ang) * r * 0.2, Math.sin(ang) * r * 0.2);
            ctx.lineTo(Math.cos(ang) * L, Math.sin(ang) * L);
            ctx.stroke();
        }
    }
    ctx.restore();
}

// drawFlake — ближний пояс метели (направление §5): мировые колонки, косой лёт
// 20–35° (tilt), мягкий двухтоновый диск; у крупных — лучи. Штрихов-«сосулек» нет.
export function drawFlake(ctx, world, camera, p, belt, beltIndex, tSec, win) {
    const F = p.flake;
    if (!F) return;
    const S = belt.tile, V = belt.vTile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const core = parseHex(F.coreColor);
    const edge = parseHex(F.edgeColor);
    const dir = p.wind.dir < 0 ? -1 : 1;
    const x0 = win.x0 - 40, x1 = win.x1 + 40, y0 = win.y0 - 40, y1 = win.y1 + 40;
    const salt = 0x7a17 + beltIndex * 0x1f3b;
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (world.seed ^ salt) >>> 0);
        const hy = hash1(i, (world.seed ^ (salt ^ 0x7777)) >>> 0);
        const hp = hash1(i, (world.seed ^ (salt ^ 0x1234)) >>> 0);
        const hf = hash1(i, (world.seed ^ (salt ^ 0x55)) >>> 0);
        const ha = hash1(i, (world.seed ^ (salt ^ 0x9abc)) >>> 0);
        const size = rnd(belt.size, hp);
        const alpha = rnd(belt.alpha, hp);
        const fall = rnd(belt.fall, hf);
        const arms = Math.max(3, Math.round(rnd(F.arms, ha)));
        const armLen = rnd(F.armLen, hp);
        const tilt = rnd(F.tilt, ha) * dir;
        const phase = hash1(i, (world.seed ^ (salt ^ 0x2468)) >>> 0) * TWO_PI;
        const sway = belt.sway ? Math.sin(tSec * 1.3 + phase) * belt.sway : 0;
        const sx0 = mod(hx * S - camera.x * par + wind, S);
        const sy0 = mod(hy * V - camera.y * par + fall * tSec, V);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S + sway;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                drawFlakeShape(ctx, sx, sy0 + ky * V, size, alpha, core, edge, F.coreRatio, arms, armLen, tilt);
            }
        }
    }
}

// ==================== ТУМАН: слоистая гряда (bank, §5.2) ====================

// drawBankShape — вытянутая по X гряда: вертикальный alpha-градиент (верх рваный
// по topRag), низ привязан к terrainHeight + offset. Покадрового радиального
// градиента нет — гряда не читается круглым «блобом».
function drawBankShape(ctx, x, yBottom, w, h, topRag, col, alpha) {
    if (w <= 0 || h <= 0 || alpha <= 0) return;
    const r = w / 2;
    ctx.save();
    ctx.translate(x, yBottom - h / 2);
    ctx.scale(1, h / w);
    const g = ctx.createLinearGradient(0, r, 0, -r);
    const solid = clamp(1 - topRag, 0, 1);
    g.addColorStop(0, rgbaCss(col, alpha));
    g.addColorStop(solid, rgbaCss(col, alpha * 0.85));
    g.addColorStop(1, rgbaCss(col, 0));
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(0, 0, r, 0, TWO_PI);
    ctx.fill();
    ctx.restore();
}

// drawBank — гряда тумана на воде (спека §5.2): медленный снос (drift) + «дыхание»
// alpha (breathAmp/breathPeriod). Низ — по terrainHeight + offset.
export function drawBank(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec) {
    const B = p.bank;
    if (!B) return;
    const S = belt.tile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const win = viewWindow(vw, vh);
    const x0 = win.x0 - 80, x1 = win.x1 + 80;
    const salt = 0x3b9f + beltIndex * 0x1f3b;
    const col = p.air.color;
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (world.seed ^ salt) >>> 0);
        const hp = hash1(i, (world.seed ^ (salt ^ 0x1234)) >>> 0);
        const hb = hash1(i, (world.seed ^ (salt ^ 0x9abc)) >>> 0);
        const w = rnd(B.w, hp);
        const h = rnd(B.h, hp);
        const alpha = rnd(B.alpha, hp);
        const topRag = rnd(B.topRag, hp);
        const drift = rnd(B.drift, hb);
        const offset = rnd(B.offset, hp);
        const breathAmp = rnd(B.breathAmp, hp);
        const breathPeriod = rnd(B.breathPeriod, hb);
        const breath = 1 - breathAmp * (0.5 + 0.5 * Math.sin(TWO_PI * tSec / Math.max(0.001, breathPeriod) + hb * TWO_PI));
        // «Кипящая» кромка плотного CO₂ (§4.3): вертикальная пульсация ±pulseY px.
        const pulseY = rnd(B.pulseY, hb);
        const sx0 = mod(hx * S - camera.x * par + wind + drift * tSec, S);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S;
            const gy = groundY(world, sx - vw / 2 + camera.x, camera, vh) + offset;
            const pulse = pulseY > 0 ? Math.sin(TWO_PI * tSec / Math.max(0.001, breathPeriod) + hb * TWO_PI) * pulseY : 0;
            drawBankShape(ctx, sx, gy + pulse, w, h, topRag, col, alpha * breath);
        }
    }
}

// ==================== ПЫЛЬНАЯ БУРЯ: плоские грани (facet, вариант §4.3) ====================

// drawFacetShape — плоская грань (ромб) вместо песчинки; у доли частиц — короткий
// луч-блеск (glint-подобный). Без покадровых градиентов.
function drawFacetShape(ctx, x, y, size, alpha, col, ray, rayLen) {
    ctx.save();
    ctx.translate(x, y);
    ctx.fillStyle = rgbaCss(col, alpha);
    ctx.beginPath();
    ctx.moveTo(0, -size);
    ctx.lineTo(size * 0.6, 0);
    ctx.lineTo(0, size);
    ctx.lineTo(-size * 0.6, 0);
    ctx.closePath();
    ctx.fill();
    if (ray) {
        ctx.strokeStyle = rgbaCss(col, alpha * 0.7);
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(-rayLen, 0); ctx.lineTo(rayLen, 0);
        ctx.stroke();
    }
    ctx.restore();
}

// drawFacets — пояса «стеклянно-кристаллической» бури: плоские грани в мировых
// колонках (как drawDots), у частиц (rayShare) — короткий луч. Поток (count/fall)
// не меняется — это сухая форма (§4.3).
export function drawFacets(ctx, world, camera, p, belt, beltIndex, tSec, win) {
    const F = p.facet;
    if (!F) return;
    const S = belt.tile, V = belt.vTile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const col = p.particle ? parseHex(p.particle) : p.air.color;
    const x0 = win.x0 - 40, x1 = win.x1 + 40, y0 = win.y0 - 40, y1 = win.y1 + 40;
    const salt = 0xf0a7 + beltIndex * 0x1f3b;
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (world.seed ^ salt) >>> 0);
        const hy = hash1(i, (world.seed ^ (salt ^ 0x7777)) >>> 0);
        const hp = hash1(i, (world.seed ^ (salt ^ 0x1234)) >>> 0);
        const hf = hash1(i, (world.seed ^ (salt ^ 0x55)) >>> 0);
        const hb = hash1(i, (world.seed ^ (salt ^ 0x9abc)) >>> 0);
        const fall = lerp(belt.fall[0], belt.fall[1], hf);
        const size = lerp(belt.size[0], belt.size[1], hp) * 1.4;
        const alpha = lerp(belt.alpha[0], belt.alpha[1], hp);
        const ray = hb < (F.rayShare || 0);
        const rayLen = rnd(F.rayLen, hp);
        const sx0 = mod(hx * S - camera.x * par + wind, S);
        const sy0 = mod(hy * V - camera.y * par + fall * tSec, V);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                drawFacetShape(ctx, sx, sy0 + ky * V, size, alpha, col, ray, rayLen);
            }
        }
    }
}
