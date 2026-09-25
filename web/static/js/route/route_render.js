// web/static/js/route/route_render.js
// Отрисовка страницы «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md; визуальное ТЗ §3.3–§3.7). Слои:
// фон → поле 1:1 (сетка, зоны, путь, ФИНИШ, СТАРТ, узлы) → эффекты; спрайты —
// только выбранные пулы; reduced-motion → без пульсов/дрейфа.
import * as C from './route_config.js';
import { getSprite, tintedSprite, bloomSprite } from './route_sprites.js';
import { shipDrawTransform } from '../map/ship_sprites.js';
import { CONFIG } from '../config.js';

// Цвет/оттенок звезды — из map/utils.js (формулу не дублируем). Но map/config.js
// читает DOM на верхнем уровне, а на route его нет, — создаём скрытый шов-канвас
// перед динамическим импортом (маршрут карту не рисует).
let getStarShade = null;
try {
    if (!document.getElementById('mapCanvas')) {
        const shim = document.createElement('canvas');
        shim.id = 'mapCanvas'; shim.style.display = 'none';
        document.documentElement.appendChild(shim);
    }
    ({ getStarShade } = await import('../map/utils.js'));
} catch (e) { getStarShade = null; }

const TAU = Math.PI * 2;
const fx = (v, p) => ({ x: v.x0 + p.x * v.size, y: v.y0 + p.y * v.size });
const hA = C.hexA;
const starColor = (s) => {
    if (getStarShade) return getStarShade(s.spectral_class, s.temperature, s.star_type);
    const c = CONFIG.map.starColors;
    return (s.star_type && c[s.star_type]) || c[s.spectral_class] || c.default;
};

// initBackground — детерминированный от seed фон (звёзды/пыль/яркие/туманности).
export function initBackground(st) {
    const rng = C.mulberry32(st.seed || 1);
    const pick = () => C.COLORS.nebula[Math.floor(rng() * C.COLORS.nebula.length)];
    st.bg = {
        stars: Array.from({ length: 170 }, () => ({ x: rng(), y: rng(), s: rng() < 0.85 ? 1 : 1.6, a: 0.25 + rng() * 0.6,
            ph: rng() * TAU, tw: 0.6 + rng() * 1.4, par: 0.1 + rng() * 0.25, c: rng() < 0.15 ? '#aac7ff' : '#e2e8f0' })),
        dust: Array.from({ length: 12 }, () => ({ x: rng(), y: rng(), r: 0.06 + rng() * 0.14, a: 0.02 + rng() * 0.05 })),
        bright: Array.from({ length: 5 }, () => ({ x: 0.1 + rng() * 0.8, y: 0.1 + rng() * 0.8 })),
        neb: [0, 1].map((i) => ({ nx: i ? 0.45 + rng() * 0.4 : 0.15 + rng() * 0.3, ny: i ? 0.5 + rng() * 0.4 : 0.12 + rng() * 0.3,
            s: 0.8 + rng() * 0.6, a: (i ? 0.15 : 0.22) + rng() * 0.1, c: pick(), ph: rng() * TAU, rot: (rng() - 0.5) * 0.6 })),
    };
}

export function drawScene(ctx, st, view, vw, vh, now) {
    ctx.fillStyle = C.COLORS.bg;
    ctx.fillRect(0, 0, vw, vh);
    if (st.bg) drawBackground(ctx, st.bg, st.chosen, st.reduced, vw, vh, now);
    drawVignette(ctx, vw, vh);
    drawField(ctx, st, view, now);
}

function drawBackground(ctx, b, chosen, reduced, vw, vh, now) {
    const t = reduced ? 0 : now;
    const neb = getSprite(chosen && chosen.nebula_bg);
    for (const n of b.neb) {
        const x = n.nx * vw + (reduced ? 0 : Math.sin(t * 0.00003 + n.ph) * 14);
        const y = n.ny * vh + (reduced ? 0 : Math.cos(t * 0.000025 + n.ph) * 12);
        const r = Math.max(vw, vh) * n.s * 0.7;
        ctx.save();
        ctx.globalCompositeOperation = 'lighter';
        ctx.globalAlpha = n.a;
        if (neb) { ctx.translate(x, y); ctx.rotate(n.rot); ctx.drawImage(tintedSprite(neb, n.c), -r, -r, r * 2, r * 2); }
        else radial(ctx, x, y, r, n.c, 0.5);
        ctx.restore();
    }
    for (const s of b.stars) {
        ctx.globalAlpha = s.a * (0.75 + 0.25 * Math.sin(t * 0.001 * s.tw + s.ph));
        ctx.fillStyle = s.c;
        ctx.fillRect(s.x * vw + (reduced ? 0 : Math.sin(t * 0.000004 * s.tw + s.ph) * 3 * s.par), s.y * vh, s.s, s.s);
    }
    ctx.globalAlpha = 1;
    for (const d of b.dust) radial(ctx, d.x * vw, d.y * vh, d.r * Math.max(vw, vh), '#7882a0', d.a);
    ctx.fillStyle = '#e2e8f0'; ctx.globalAlpha = 0.85;
    ctx.strokeStyle = 'rgba(226,232,240,0.3)'; ctx.lineWidth = 1;
    for (const s of b.bright) {
        const x = s.x * vw;
        const y = s.y * vh;
        ctx.fillRect(x - 1, y - 1, 2.4, 2.4);
        ctx.beginPath(); ctx.moveTo(x - 9, y); ctx.lineTo(x + 9, y);
        ctx.moveTo(x, y - 9); ctx.lineTo(x, y + 9); ctx.stroke();
    }
    ctx.globalAlpha = 1;
}

// radial — радиальный градиент цвета в прозрачность.
function radial(ctx, x, y, r, color, a) {
    const g = ctx.createRadialGradient(x, y, 0, x, y, r);
    g.addColorStop(0, hA(color, a)); g.addColorStop(1, hA(color, 0));
    ctx.fillStyle = g;
    ctx.fillRect(x - r, y - r, r * 2, r * 2);
}

function drawVignette(ctx, vw, vh) {
    const g = ctx.createRadialGradient(vw / 2, vh / 2, Math.min(vw, vh) * 0.3, vw / 2, vh / 2, Math.max(vw, vh) * 0.75);
    g.addColorStop(0, 'rgba(0,0,0,0)'); g.addColorStop(1, 'rgba(0,0,0,0.45)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, vw, vh);
}

function drawField(ctx, st, view, now) {
    const { x0, y0, size } = view;
    ctx.save();
    ctx.beginPath(); ctx.rect(x0, y0, size, size); ctx.clip();
    ctx.strokeStyle = C.COLORS.grid;
    ctx.lineWidth = 1;
    for (let i = 1; i < 10; i++) {
        const g = size * i / 10;
        ctx.beginPath(); ctx.moveTo(x0 + g, y0); ctx.lineTo(x0 + g, y0 + size); ctx.stroke();
        ctx.beginPath(); ctx.moveTo(x0, y0 + g); ctx.lineTo(x0 + size, y0 + g); ctx.stroke();
    }
    drawZones(ctx, st, view, now);
    drawPath(ctx, st, view, now);
    drawFinish(ctx, st, view, now);
    drawStart(ctx, st, view);
    drawNodes(ctx, st, view, now);
    ctx.restore();
    ctx.strokeStyle = C.COLORS.border; ctx.lineWidth = 1.5;
    ctx.strokeRect(x0 + 0.75, y0 + 0.75, size - 1.5, size - 1.5);
    drawZoneChips(ctx, st, view, now);
}

function drawZones(ctx, st, view, now) {
    const img = getSprite(st.chosen && st.chosen.hazard_cloud);
    for (const z of st.field.zones || []) {
        const p = fx(view, z);
        const r = z.r * view.size;
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(st.reduced ? 0 : now * 0.00025 * (1 + z.x));
        if (img) {
            ctx.beginPath(); ctx.arc(0, 0, r, 0, TAU); ctx.clip();
            ctx.globalAlpha = 0.5;
            ctx.drawImage(tintedSprite(img, C.COLORS.cold), -r, -r, r * 2, r * 2);
        } else radial(ctx, 0, 0, r, C.COLORS.cold, 0.4);
        ctx.restore();
        ctx.globalAlpha = 1;
        ring(ctx, p.x, p.y, r, C.COLORS.cold, 0.5, 1, [6, 5]);
    }
}

// ring — окружность (цвет/alpha/толщина/штрих); общий для зон и узлов.
function ring(ctx, x, y, r, color, a, w, dash) {
    ctx.strokeStyle = hA(color, a);
    ctx.lineWidth = w;
    if (dash) ctx.setLineDash(dash);
    ctx.beginPath(); ctx.arc(x, y, r, 0, TAU); ctx.stroke();
    if (dash) ctx.setLineDash([]);
}

function segInZone(a, b, zones) {
    for (const z of zones || []) if (C.pointSegDist({ x: z.x, y: z.y }, a, b) <= z.r) return true;
    return false;
}

// ---- Чип «дороже» у зоны потери времени (§3.2/§6.3) ----
// Качественная надпись без чисел/множителей (решение 14): как только
// зафиксированный сегмент или «свободная» линия входит в зону, у её границы
// мягко всплывает чип. Тон — «предупреждение» палитры §3.1 (не маяк/опасность).
// Экранное пространство; положение клампится квадратом поля 1:1 (§5) — чип не
// выходит за канвас и не заходит в полосы HUD/тач-целей (чип только рисуется,
// pointer-events не задействует). reduced-motion → сразу видно/скрыто.
const CHIP_TEXT = 'ДОРОЖЕ';
const CHIP_FADE_MS = 200;

// zoneCrossed — центр зоны ближе её радиуса к любому сегменту полилинии.
function zoneCrossed(z, pts) {
    for (let j = 0; j + 1 < pts.length; j++) {
        if (C.pointSegDist({ x: z.x, y: z.y }, pts[j], pts[j + 1]) <= z.r) return true;
    }
    return false;
}

function drawZoneChips(ctx, st, view, now) {
    const zones = st.field.zones || [];
    if (!zones.length) return;
    if (!st.chipAlpha) { st.chipAlpha = new Map(); st.chipPrev = now; }
    const dt = Math.max(0, now - st.chipPrev);
    st.chipPrev = now;
    const pts = st.dragging && st.drag ? st.path.concat([st.drag]) : st.path;
    const step = st.reduced ? 1 : dt / CHIP_FADE_MS;
    for (let i = 0; i < zones.length; i++) {
        const target = pts.length >= 2 && zoneCrossed(zones[i], pts) ? 1 : 0;
        const prev = st.chipAlpha.get(i) || 0;
        const a = prev < target ? Math.min(target, prev + step)
            : prev > target ? Math.max(target, prev - step) : prev;
        st.chipAlpha.set(i, a);
        if (a > 0.001) drawZoneChip(ctx, view, zones[i], a);
    }
}

// drawZoneChip — плашка «ДОРОЖЕ» над зоной (под зоной, если сверху нет места).
function drawZoneChip(ctx, view, z, alpha) {
    const p = fx(view, z);
    const r = z.r * view.size;
    ctx.save();
    ctx.font = '600 12px "Segoe UI", Roboto, system-ui, sans-serif';
    try { ctx.letterSpacing = '0.04em'; } catch (e) { /* не поддержано */ }
    const w = ctx.measureText(CHIP_TEXT).width + 16;
    const h = 22;
    const gap = 8;
    const { x0, y0, size } = view;
    const half = w / 2;
    const loX = x0 + half + 2;
    const hiX = x0 + size - half - 2;
    const cx = loX <= hiX ? Math.max(loX, Math.min(hiX, p.x)) : x0 + size / 2;
    let cy = p.y - r - gap - h / 2;
    if (cy - h / 2 < y0 + 2) cy = p.y + r + gap + h / 2;
    const loY = y0 + h / 2 + 2;
    const hiY = y0 + size - h / 2 - 2;
    cy = loY <= hiY ? Math.max(loY, Math.min(hiY, cy)) : y0 + size / 2;
    ctx.globalAlpha = alpha;
    ctx.fillStyle = hA('#0f172a', 0.85);
    roundRect(ctx, cx - half, cy - h / 2, w, h, 7);
    ctx.fill();
    ctx.strokeStyle = hA(C.COLORS.warning, 0.85);
    ctx.lineWidth = 1.2;
    ctx.stroke();
    ctx.fillStyle = C.COLORS.warning;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(CHIP_TEXT, cx, cy + 0.5);
    ctx.restore();
}

// roundRect — скруглённый прямоугольник (без ctx.roundRect ради совместимости).
function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
}

function drawPath(ctx, st, view, now) {
    const path = st.path;
    ctx.lineCap = 'round';
    for (let i = 0; i + 1 < path.length; i++) {
        const a = fx(view, path[i]);
        const b = fx(view, path[i + 1]);
        const z = segInZone(path[i], path[i + 1], st.field.zones);
        ctx.strokeStyle = z ? C.COLORS.cold : C.COLORS.path;
        ctx.lineWidth = z ? 4 : 3;
        ctx.beginPath(); ctx.moveTo(a.x, a.y); ctx.lineTo(b.x, b.y); ctx.stroke();
    }
    if (st.dragging && st.drag && path.length) {
        const a = fx(view, path[path.length - 1]);
        const b = fx(view, st.drag);
        ctx.strokeStyle = C.COLORS.pathFree;
        ctx.lineWidth = 2;
        ctx.setLineDash([6, 6]);
        ctx.beginPath(); ctx.moveTo(a.x, a.y); ctx.lineTo(b.x, b.y); ctx.stroke();
        ctx.setLineDash([]);
        ctx.fillStyle = C.COLORS.path;
        ctx.beginPath(); ctx.arc(b.x, b.y, 4, 0, TAU); ctx.fill();
    }
    if (st.boost) drawBoost(ctx, st, view, now);
}

// drawBoost — анимация успеха: световой фронт ~0.8–1.2 с + вспышка на ФИНИШЕ;
// reduced-motion — кроссфейд всего пути без разлёта.
function drawBoost(ctx, st, view, now) {
    const pts = st.path.map((q) => fx(view, q));
    const segs = [];
    let total = 0;
    for (let i = 0; i + 1 < pts.length; i++) {
        const d = Math.hypot(pts[i + 1].x - pts[i].x, pts[i + 1].y - pts[i].y);
        segs.push(d); total += d;
    }
    const p = Math.max(0, Math.min(1, (now - st.boost.t0) / st.boost.dur));
    ctx.save();
    ctx.lineCap = 'round';
    if (st.reduced) {
        ctx.globalAlpha = p < 1 ? 1 : Math.max(0, 1 - p);
        ctx.strokeStyle = C.COLORS.success; ctx.lineWidth = 3;
        for (let i = 0; i + 1 < pts.length; i++) {
            ctx.beginPath(); ctx.moveTo(pts[i].x, pts[i].y); ctx.lineTo(pts[i + 1].x, pts[i + 1].y); ctx.stroke();
        }
        ctx.restore();
        return;
    }
    const front = total * p;
    let acc = 0;
    for (let i = 0; i < segs.length && front > acc; i++) {
        const f = Math.min(1, (front - acc) / segs[i]);
        const ex = pts[i].x + (pts[i + 1].x - pts[i].x) * f;
        const ey = pts[i].y + (pts[i + 1].y - pts[i].y) * f;
        for (const [w, a, c] of [[9, 0.25, C.COLORS.success], [3, 1, C.COLORS.success]]) {
            ctx.strokeStyle = hA(c, a);
            ctx.lineWidth = w;
            ctx.beginPath(); ctx.moveTo(pts[i].x, pts[i].y); ctx.lineTo(ex, ey); ctx.stroke();
        }
        acc += segs[i];
    }
    const flash = Math.max(0, 1 - Math.abs(p - 1) * 3);
    if (flash > 0) {
        const f = fx(view, st.field.finish);
        radial(ctx, f.x, f.y, view.size * 0.12 * (0.6 + flash), C.COLORS.success, 0.7 * flash);
    }
    ctx.restore();
}

function drawFinish(ctx, st, view, now) {
    const f = fx(view, st.field.finish);
    const to = (st.passport && st.passport.to) || {};
    const black = to.star_type === 'black_hole';
    const r = view.size * 0.11 * (st.reduced ? 1 : 1 + 0.08 * Math.sin(now * 0.0015));
    const img = getSprite(st.chosen && (black ? st.chosen.black_hole : st.chosen.star_core));
    if (black) {
        radial(ctx, f.x, f.y, r * 1.6, C.COLORS.voidHalo, 0.7);
        if (img) {
            bloomSprite(ctx, img, f.x, f.y, r * 2, 0);
            ctx.fillStyle = '#02030a';
            ctx.beginPath(); ctx.arc(f.x, f.y, r * 0.42, 0, TAU); ctx.fill();
            ctx.globalAlpha = 1;
        } else ring(ctx, f.x, f.y, r, C.COLORS.warmHalo, 1, 2);
        return;
    }
    const col = starColor(to);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    radial(ctx, f.x, f.y, r * 1.7, col, 0.55);
    if (img) bloomSprite(ctx, img, f.x, f.y, r * 2, 0.25);
    ctx.restore();
    if (!img) radial(ctx, f.x, f.y, r, col, 1);
}

function drawStart(ctx, st, view) {
    const s = fx(view, st.field.start);
    const r = view.size * 0.045;
    if (st.shipSprite) {
        const tf = shipDrawTransform(0, st.shipOrient);
        const sz = r * 1.7;
        ctx.save();
        ctx.translate(s.x, s.y); ctx.rotate(tf.rotate); ctx.scale(tf.scaleX, tf.scaleY);
        ctx.drawImage(st.shipSprite, -sz / 2, -sz / 2, sz, sz);
        ctx.restore();
    } else {
        ctx.save(); ctx.translate(s.x, s.y); ctx.fillStyle = C.COLORS.cold;
        ctx.beginPath(); ctx.moveTo(r * 0.9, 0); ctx.lineTo(-r * 0.6, -r * 0.7); ctx.lineTo(-r * 0.6, r * 0.7);
        ctx.closePath(); ctx.fill(); ctx.restore();
    }
    ring(ctx, s.x, s.y, C.ENDPOINT_R * view.size, C.COLORS.cold, 0.6, 1.2);
}

function drawNodes(ctx, st, view, now) {
    const imgB = getSprite(st.chosen && st.chosen.beacon);
    const imgF = getSprite(st.chosen && st.chosen.false_signal);
    (st.field.nodes || []).forEach((n, i) => {
        const p = fx(view, n);
        const r = n.r * view.size;
        const isB = n.type === 'beacon';
        const sz = Math.max(10, r * 2);
        if (isB) {
            const pulse = st.reduced ? 0.6 : (now % 1600) / 1600;
            ring(ctx, p.x, p.y, r * (0.6 + 0.6 * pulse), C.COLORS.capture, 0.55 * (1 - pulse), 2);
            ring(ctx, p.x, p.y, r, C.COLORS.capture, 0.7, 1.2);
            drawCaptureFlash(ctx, p, r, st.flash.get(i), now);
        } else {
            const a = st.reduced ? 0.4 : 0.25 + 0.3 * Math.sin(now * 0.005 + i * 2.3) * Math.sin(now * 0.0017 + i);
            ring(ctx, p.x, p.y, r, C.COLORS.danger, Math.max(0.15, a), 1, [4, 5]);
        }
        if (imgB && isB) {
            ctx.save(); ctx.globalCompositeOperation = 'lighter'; bloomSprite(ctx, imgB, p.x, p.y, sz, 0.25); ctx.restore();
        } else if (imgF && !isB) {
            ctx.save(); ctx.globalAlpha = 0.7; ctx.drawImage(imgF, p.x - sz / 2, p.y - sz / 2, sz, sz); ctx.restore();
        } else {
            ctx.globalAlpha = isB ? 0.9 : 0.6;
            ctx.fillStyle = isB ? C.COLORS.capture : C.COLORS.danger;
            ctx.fillRect(p.x - 3, p.y - 3, 6, 6); ctx.globalAlpha = 1;
        }
    });
}

// drawCaptureFlash — вспышка захвата маяка 180 мс + кольцо-разлёт 400 мс.
function drawCaptureFlash(ctx, p, r, flash, now) {
    if (flash == null) return;
    const dt = now - flash;
    if (dt < 180) radial(ctx, p.x, p.y, r * 1.8, C.COLORS.captureSoft, 0.8 * (1 - dt / 180));
    else if (dt < 580) ring(ctx, p.x, p.y, r * (1 + (dt - 180) / 400 * 1.2), C.COLORS.captureSoft, 0.7 * (1 - (dt - 180) / 400), 2);
}
