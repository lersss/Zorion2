// web/static/js/route/route_figures.js
// Общие примитивы отрисовки фигур доски v9 «Планшет» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.4/§4.9). Их используют и доска
// (route_render), и попап-легенда (route_legend): мини-фигура в легенде — это
// та же фигура, нарисованная тем же кодом, а не спрайт и не цветной свотч.
// Спрайт НЕ несёт игровой истины: цены/τ/содержимое — только код.
import * as C from './route_config.js';

export const TAU = Math.PI * 2;
export const hA = C.hexA;

// radial — радиальный градиент цвета в прозрачность.
export function radial(ctx, x, y, r, color, a) {
    const g = ctx.createRadialGradient(x, y, 0, x, y, r);
    g.addColorStop(0, hA(color, a)); g.addColorStop(1, hA(color, 0));
    ctx.fillStyle = g;
    ctx.fillRect(x - r, y - r, r * 2, r * 2);
}

// roundRect — скруглённый прямоугольник (без ctx.roundRect ради совместимости).
export function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
}

// ring — окружность (цвет/alpha/толщина/штрих).
export function ring(ctx, x, y, r, color, a, w, dash) {
    ctx.strokeStyle = hA(color, a);
    ctx.lineWidth = w;
    if (dash) ctx.setLineDash(dash);
    ctx.beginPath(); ctx.arc(x, y, r, 0, TAU); ctx.stroke();
    if (dash) ctx.setLineDash([]);
}

// ---- Блоки поля: одна клетка (rect r) ----

export function figWall(ctx, r) {
    ctx.fillStyle = C.COLORS.wallDark;
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.strokeStyle = hA(C.COLORS.wall, 0.9);
    ctx.lineWidth = 1;
    for (let k = -r.h; k < r.w; k += 6) {
        ctx.beginPath(); ctx.moveTo(r.x + k, r.y + r.h); ctx.lineTo(r.x + k + r.h, r.y); ctx.stroke();
    }
}

export function figMud(ctx, r) {
    ctx.fillStyle = hA(C.COLORS.mud, 0.28);
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.fillStyle = hA(C.COLORS.mud, 0.5);
    for (let k = 0; k < 4; k++) {
        const px = r.x + ((k * 7 + 3) % Math.max(4, r.w - 4)) + 2;
        const py = r.y + ((k * 5 + 4) % Math.max(4, r.h - 4)) + 2;
        ctx.fillRect(px, py, 1.6, 1.6);
    }
}

export function figLane(ctx, r) {
    ctx.fillStyle = hA(C.COLORS.lane, 0.16);
    ctx.fillRect(r.x, r.y, r.w, r.h);
}

// figWarm — дорогая клетка (visible > 1), кроме топи/стены.
export function figWarm(ctx, r) {
    ctx.fillStyle = hA(C.COLORS.warning, 0.16);
    ctx.fillRect(r.x, r.y, r.w, r.h);
}

export function figBottleneck(ctx, r) {
    ctx.fillStyle = hA(C.COLORS.bottleneck, 0.55);
    ctx.fillRect(r.x + r.w * 0.3, r.y + r.h * 0.1, r.w * 0.4, r.h * 0.8);
}

export function figGate(ctx, r) {
    ctx.save();
    ctx.strokeStyle = hA(C.COLORS.gate, 0.95);
    ctx.lineWidth = Math.max(2, r.w * 0.14);
    ctx.beginPath();
    ctx.moveTo(r.x + r.w * 0.5, r.y + r.h * 0.06);
    ctx.lineTo(r.x + r.w * 0.5, r.y + r.h * 0.94);
    ctx.stroke();
    ctx.fillStyle = hA(C.COLORS.gate, 0.95);
    ctx.beginPath();
    ctx.arc(r.x + r.w * 0.5, r.y + r.h * 0.5, Math.max(1.6, r.w * 0.09), 0, TAU);
    ctx.fill();
    ctx.restore();
}

export function figBridge(ctx, r) {
    ctx.save();
    ctx.fillStyle = hA(C.COLORS.bridge, 0.28);
    ctx.fillRect(r.x + r.w * 0.08, r.y + r.h * 0.34, r.w * 0.84, r.h * 0.32);
    ctx.strokeStyle = hA(C.COLORS.bridge, 0.9);
    ctx.lineWidth = 1.2;
    ctx.strokeRect(r.x + r.w * 0.08, r.y + r.h * 0.34, r.w * 0.84, r.h * 0.32);
    ctx.restore();
}

// figChevron — шевроны течения по dir (0:+i,1:−i,2:+j,3:−j).
export function figChevron(ctx, r, dir, alpha) {
    const v = C.DIR_VECTORS[dir];
    if (!v) return;
    const cx = r.x + r.w / 2;
    const cy = r.y + r.h / 2;
    const len = r.w * 0.28;
    ctx.save();
    ctx.lineWidth = Math.max(1.4, r.w * 0.09);
    ctx.lineCap = 'round';
    ctx.strokeStyle = hA(C.COLORS.current, alpha);
    const hx = v.di * len;
    const hy = v.dj * len;
    const px = v.dj * len * 0.7;
    const py = v.di * len * 0.7;
    for (const off of [-0.5, 0.5]) {
        ctx.beginPath();
        ctx.moveTo(cx - hx + px * off, cy - hy + py * off);
        ctx.lineTo(cx + hx * 0.5 + px * off, cy + hy * 0.5 + py * off);
        ctx.stroke();
    }
    ctx.restore();
}

// figDeadEnd — тупиковое русло: кайма + «стоп»-маркер (или спрайт false_signal).
export function figDeadEnd(ctx, r, img) {
    ctx.save();
    ctx.strokeStyle = hA(C.COLORS.deadEnd, 0.9);
    ctx.lineWidth = Math.max(1.4, r.w * 0.08);
    ctx.strokeRect(r.x + 1.5, r.y + 1.5, r.w - 3, r.h - 3);
    const sz = r.w * 0.5;
    if (img) {
        ctx.globalAlpha = 0.75;
        ctx.drawImage(img, r.x + r.w / 2 - sz / 2, r.y + r.h / 2 - sz / 2, sz, sz);
    } else {
        ctx.fillStyle = C.COLORS.deadEnd;
        ctx.fillRect(r.x + r.w * 0.3, r.y + r.h * 0.42, r.w * 0.4, r.h * 0.16);
    }
    ctx.restore();
}

// ---- Секторы: плотность по σ, кайма по содержимому (нейтрально без вскрытия) ----

export function sectorDensity(sig) { return [0.20, 0.32, 0.46][sig] || 0.3; }

export function sectorRim(content) {
    if (content === 'unstable') return C.COLORS.unstable;
    if (content === 'jackpot' || content === 'lure') return C.COLORS.jackpot;
    return C.COLORS.sector;
}

// figSector — одна клетка сектора-облака: заливка по σ + кайма по содержимому.
export function figSector(ctx, r, sig, content) {
    ctx.save();
    ctx.fillStyle = C.COLORS.sector;
    ctx.globalAlpha = sectorDensity(sig);
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.restore();
    ctx.save();
    ctx.strokeStyle = hA(sectorRim(content), content ? 0.9 : 0.45);
    ctx.lineWidth = content ? 1.6 : 1;
    ctx.strokeRect(r.x + 0.5, r.y + 0.5, r.w - 1, r.h - 1);
    ctx.restore();
    if (content === 'unstable') heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'hazard', 1);
    else if (content === 'jackpot' || content === 'lure') heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'jackpot', 1);
}

// ---- Локальный «жар» (§4.6): форма метки одна, раскладку задаёт вызывающий ----

const HEAT_COLORS = {
    mud: C.COLORS.mud, wall: C.COLORS.wall, gate: C.COLORS.gate, gate_twice: C.COLORS.gate,
    bridge: C.COLORS.bridge, bridge_twice: C.COLORS.bridge, dead_end: C.COLORS.deadEnd,
    current_against: C.COLORS.deadEnd, current_along: C.COLORS.bridge,
    hazard: C.COLORS.unstable, jackpot: C.COLORS.jackpot, turn: C.COLORS.captureSoft,
};

export function heatColor(kind) { return HEAT_COLORS[kind] || C.COLORS.capture; }

// turnNotch — засечка-уголок излома в клетке r.
export function turnNotch(ctx, r, color, pulse) {
    ctx.save();
    ctx.strokeStyle = hA(color, pulse);
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(r.x + r.w * 0.2, r.y + r.h * 0.8);
    ctx.lineTo(r.x + r.w * 0.2, r.y + r.h * 0.5);
    ctx.lineTo(r.x + r.w * 0.5, r.y + r.h * 0.5);
    ctx.stroke();
    ctx.restore();
}

// heatMark — точка-маркер «жара» (для *_twice — с кольцом) в точке (x,y).
export function heatMark(ctx, x, y, cell, kind, pulse) {
    const color = heatColor(kind);
    ctx.fillStyle = hA(color, 0.9 * pulse);
    ctx.beginPath();
    ctx.arc(x, y, Math.max(2, cell * 0.08), 0, TAU);
    ctx.fill();
    if (kind === 'gate_twice' || kind === 'bridge_twice') {
        ctx.strokeStyle = hA(color, 0.9);
        ctx.lineWidth = 1.4;
        ctx.beginPath();
        ctx.arc(x, y, Math.max(3.4, cell * 0.15), 0, TAU);
        ctx.stroke();
    }
}

// ---- Ориентиры: кодовые фигуры (легенда — не спрайт) ----

export function startTriangle(ctx, x, y, r, color) {
    ctx.save();
    ctx.translate(x, y);
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.moveTo(r * 0.9, 0); ctx.lineTo(-r * 0.6, -r * 0.7); ctx.lineTo(-r * 0.6, r * 0.7);
    ctx.closePath(); ctx.fill();
    ctx.restore();
}

export function beaconRings(ctx, x, y, base, pulse, captured) {
    ring(ctx, x, y, base * 0.42 * (0.6 + 0.6 * pulse), C.COLORS.capture, 0.5 * (1 - pulse), 2);
    ring(ctx, x, y, base * 0.42, C.COLORS.capture, captured ? 0.95 : 0.7, 1.2);
}

export function beaconSquare(ctx, x, y, on) {
    ctx.globalAlpha = on ? 1 : 0.9;
    ctx.fillStyle = C.COLORS.capture;
    ctx.fillRect(x - 3, y - 3, 6, 6);
    ctx.globalAlpha = 1;
}

export function figStart(ctx, r) {
    const x = r.x + r.w / 2;
    const y = r.y + r.h / 2;
    const base = Math.min(r.w, r.h);
    startTriangle(ctx, x, y, base * 0.7, C.COLORS.cold);
    ring(ctx, x, y, base * 0.46, C.COLORS.cold, 0.6, 1.2);
}

export function figBeacon(ctx, r) {
    const x = r.x + r.w / 2;
    const y = r.y + r.h / 2;
    const base = Math.min(r.w, r.h);
    beaconRings(ctx, x, y, base, 0.35, false);
    beaconSquare(ctx, x, y, false);
}

export function figStar(ctx, r) {
    const x = r.x + r.w / 2;
    const y = r.y + r.h / 2;
    const base = Math.min(r.w, r.h);
    radial(ctx, x, y, base * 0.75, C.COLORS.warmHalo, 0.55);
    radial(ctx, x, y, base * 0.4, C.COLORS.warmHalo, 1);
    ctx.save();
    ctx.strokeStyle = hA(C.COLORS.warmHalo, 0.9);
    ctx.lineWidth = 1.4;
    ctx.beginPath();
    ctx.moveTo(x - base * 0.5, y); ctx.lineTo(x + base * 0.5, y);
    ctx.moveTo(x, y - base * 0.5); ctx.lineTo(x, y + base * 0.5);
    ctx.stroke();
    ctx.restore();
}

// ---- Прочее: путь, излом, предпросмотр ----

export function figPathSample(ctx, r, withTurn) {
    const w = Math.max(3, Math.min(r.w, r.h) * 0.22);
    const xm = r.x + r.w * 0.5;
    const y1 = r.y + r.h * 0.78;
    const y2 = r.y + r.h * 0.42;
    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.strokeStyle = C.COLORS.path;
    ctx.lineWidth = w;
    ctx.beginPath();
    ctx.moveTo(r.x + r.w * 0.1, y1);
    ctx.lineTo(xm, y1);
    ctx.lineTo(xm, y2);
    ctx.lineTo(r.x + r.w * 0.9, y2);
    ctx.stroke();
    ctx.restore();
    if (withTurn) turnNotch(ctx, r, C.COLORS.captureSoft, 1);
}

export function figPreviewSample(ctx, r) {
    ctx.save();
    ctx.strokeStyle = C.COLORS.pathFree;
    ctx.lineWidth = Math.max(2, Math.min(r.w, r.h) * 0.12);
    ctx.setLineDash([6, 6]);
    ctx.beginPath();
    ctx.moveTo(r.x + r.w * 0.15, r.y + r.h * 0.7);
    ctx.lineTo(r.x + r.w * 0.85, r.y + r.h * 0.3);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = C.COLORS.path;
    ctx.beginPath();
    ctx.arc(r.x + r.w * 0.85, r.y + r.h * 0.3, 3, 0, TAU);
    ctx.fill();
    ctx.restore();
}

// ---- Диспетчер мини-фигур легенды (§4.9): id из LEGEND_ITEMS → фигура ----

export function drawLegendFigure(ctx, id, r) {
    switch (id) {
        case 'lane': figLane(ctx, r); break;
        case 'mud': figMud(ctx, r); break;
        case 'wall': figWall(ctx, r); break;
        case 'bottleneck': figBottleneck(ctx, r); break;
        case 'gate': figGate(ctx, r); break;
        case 'bridge': figBridge(ctx, r); break;
        case 'current_along':
            figChevron(ctx, r, 0, 0.85);
            heatMark(ctx, r.x + r.w * 0.5, r.y + r.h * 0.2, r.w, 'current_along', 1);
            break;
        case 'current_against':
            figChevron(ctx, r, 0, 0.85);
            heatMark(ctx, r.x + r.w * 0.5, r.y + r.h * 0.2, r.w, 'current_against', 1);
            break;
        case 'dead_end': figDeadEnd(ctx, r, null); break;
        case 'start': figStart(ctx, r); break;
        case 'beacon': figBeacon(ctx, r); break;
        case 'finish': figStar(ctx, r); break;
        case 'sig_quiet': figSector(ctx, r, 0, null); break;
        case 'sig_mid': figSector(ctx, r, 1, null); break;
        case 'sig_loud': figSector(ctx, r, 2, null); break;
        case 'jackpot': figSector(ctx, r, 1, 'jackpot'); break;
        case 'lure': figSector(ctx, r, 1, 'lure'); break;
        case 'trap': figSector(ctx, r, 1, 'trap'); break;
        case 'decoy': figSector(ctx, r, 1, 'decoy'); break;
        case 'unstable': figSector(ctx, r, 2, 'unstable'); break;
        case 'empty': figSector(ctx, r, 0, 'empty'); break;
        case 'path': figPathSample(ctx, r, false); break;
        case 'turn': figPathSample(ctx, r, true); break;
        case 'preview': figPreviewSample(ctx, r); break;
        case 'heat_cost':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'mud', 1);
            break;
        case 'heat_turn':
            turnNotch(ctx, r, heatColor('turn'), 1);
            break;
        case 'heat_dead_end':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'dead_end', 1);
            break;
        case 'heat_against':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'current_against', 1);
            break;
        case 'heat_gate_twice':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'gate_twice', 1);
            break;
        case 'heat_bridge_twice':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'bridge_twice', 1);
            break;
        case 'heat_hazard':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'hazard', 1);
            break;
        case 'heat_along':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'current_along', 1);
            break;
        case 'heat_jackpot':
            heatMark(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, 'jackpot', 1);
            break;
        default: break;
    }
}
