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

// lensLine — продольная сужающаяся линия (линза) от x0 к x1 в y, макс. полуширина hw.
function lensLine(ctx, x0, x1, y, hw, steps) {
    ctx.beginPath();
    for (let k = 0; k <= steps; k++) {
        const t = k / steps;
        const h = Math.sin(Math.PI * t) * hw;
        ctx.lineTo(x0 + (x1 - x0) * t, y - h);
    }
    for (let k = steps; k >= 0; k--) {
        const t = k / steps;
        const h = Math.sin(Math.PI * t) * hw;
        ctx.lineTo(x0 + (x1 - x0) * t, y + h);
    }
    ctx.closePath();
    ctx.fill();
}

// hatch — семейство косых штрихов под angDeg с шагом step и пунктиром dash.
function hatch(ctx, r, angDeg, step, dash, phase) {
    const rad = angDeg * Math.PI / 180;
    const dx = Math.cos(rad);
    const dy = Math.sin(rad);
    const diag = Math.hypot(r.w, r.h);
    ctx.setLineDash(dash);
    for (let t = -diag + phase; t <= diag; t += step) {
        const cx = r.x + r.w / 2 - dy * t;
        const cy = r.y + r.h / 2 + dx * t;
        ctx.beginPath();
        ctx.moveTo(cx - dx * diag, cy - dy * diag);
        ctx.lineTo(cx + dx * diag, cy + dy * diag);
        ctx.stroke();
    }
    ctx.setLineDash([]);
}

// figWall — «шумовая решётка» (§12.13): плотная тёмная подложка + две семьи
// перекрещенных штрихов ±60° с разрывами (фаза от координаты клетки) + крапины.
export function figWall(ctx, r) {
    ctx.fillStyle = C.COLORS.wallDark;
    ctx.fillRect(r.x, r.y, r.w, r.h);
    const phase = ((Math.round(r.x) * 5 + Math.round(r.y) * 11) % 7);
    ctx.save();
    ctx.beginPath(); ctx.rect(r.x, r.y, r.w, r.h); ctx.clip();
    ctx.strokeStyle = hA(C.COLORS.wallLine, 0.9);
    ctx.lineWidth = 1;
    hatch(ctx, r, 60, 7, [3, 3], phase);
    hatch(ctx, r, -60, 7, [2, 5], phase);
    ctx.fillStyle = hA(C.COLORS.wallLine, 0.6);
    for (let k = 0; k < 4; k++) {
        const px = r.x + ((k * 11 + 5 + phase) % Math.max(4, r.w - 4)) + 2;
        const py = r.y + ((k * 7 + 9 + phase) % Math.max(4, r.h - 4)) + 2;
        ctx.fillRect(px, py, 1.5, 1.5);
    }
    ctx.restore();
}

// figMud — вязкая туманность: тёмное ядро + мягкое свечение + крупинки (§12.2).
export function figMud(ctx, r) {
    const s = Math.min(r.w, r.h);
    ctx.fillStyle = hA(C.COLORS.mud, 0.3);
    ctx.fillRect(r.x, r.y, r.w, r.h);
    radial(ctx, r.x + r.w / 2, r.y + r.h / 2, s * 0.55, C.COLORS.mud, 0.4);
    ctx.fillStyle = hA(C.COLORS.mudGrain, 0.8);
    const grains = 4 + (((Math.round(r.x) + Math.round(r.y)) % 3));
    for (let k = 0; k < grains; k++) {
        const px = r.x + ((k * 7 + 3) % Math.max(4, r.w - 4)) + 2;
        const py = r.y + ((k * 5 + 4) % Math.max(4, r.h - 4)) + 2;
        ctx.fillRect(px, py, 1.6, 1.6);
    }
}

// figLane — «струи потока» (§12.13): тихая заливка + 3 продольные непрерывные
// сужающиеся линии + светлое ядро; без рамки, пунктира и диагоналей.
export function figLane(ctx, r) {
    const s = Math.min(r.w, r.h);
    ctx.fillStyle = hA(C.COLORS.lane, 0.11);
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.save();
    ctx.fillStyle = hA(C.COLORS.lane, 0.85);
    const x0 = r.x + s * 0.1;
    const x1 = r.x + s * 0.9;
    const hw = Math.max(0.8, s * 0.028);
    for (const f of [0.28, 0.5, 0.72]) lensLine(ctx, x0, x1, r.y + s * f, hw, 12);
    ctx.strokeStyle = hA(C.COLORS.lane, 0.55);
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(r.x + s * 0.16, r.y + s * 0.5);
    ctx.lineTo(r.x + s * 0.84, r.y + s * 0.5);
    ctx.stroke();
    ctx.restore();
}

// figWarm — дорогая клетка (visible > 1), кроме Мглы/Помех.
export function figWarm(ctx, r) {
    ctx.fillStyle = hA(C.COLORS.warning, 0.16);
    ctx.fillRect(r.x, r.y, r.w, r.h);
}

// figBottleneck — «ворота»: две скобки + светлая вертикальная щель (§12.2).
export function figBottleneck(ctx, r) {
    ctx.save();
    ctx.fillStyle = hA(C.COLORS.bottleneck, 0.55);
    ctx.fillRect(r.x + r.w * 0.42, r.y + r.h * 0.06, r.w * 0.16, r.h * 0.88);
    ctx.strokeStyle = hA(C.COLORS.bottleneck, 0.9);
    ctx.lineWidth = Math.max(1.4, r.w * 0.08);
    ctx.lineCap = 'round';
    const lx = r.x + r.w * 0.3;
    const rx = r.x + r.w * 0.7;
    const y0 = r.y + r.h * 0.14;
    const y1 = r.y + r.h * 0.86;
    ctx.beginPath();
    ctx.moveTo(lx + r.w * 0.08, y0); ctx.lineTo(lx, y0); ctx.lineTo(lx, y1); ctx.lineTo(lx + r.w * 0.08, y1);
    ctx.moveTo(rx - r.w * 0.08, y0); ctx.lineTo(rx, y0); ctx.lineTo(rx, y1); ctx.lineTo(rx - r.w * 0.08, y1);
    ctx.moveTo(r.x + r.w * 0.5, r.y + r.h * 0.16); ctx.lineTo(r.x + r.w * 0.5, r.y + r.h * 0.3);
    ctx.moveTo(r.x + r.w * 0.5, r.y + r.h * 0.7); ctx.lineTo(r.x + r.w * 0.5, r.y + r.h * 0.84);
    ctx.stroke();
    ctx.restore();
}

// figGate — кордон: створка + ромб оплаты + двойная поперечная полоса (§12.2).
export function figGate(ctx, r) {
    ctx.save();
    ctx.strokeStyle = hA(C.COLORS.gate, 0.95);
    ctx.lineWidth = Math.max(2, r.w * 0.14);
    ctx.beginPath();
    ctx.moveTo(r.x + r.w * 0.5, r.y + r.h * 0.06);
    ctx.lineTo(r.x + r.w * 0.5, r.y + r.h * 0.94);
    ctx.stroke();
    ctx.fillStyle = hA(C.COLORS.gate, 0.85);
    ctx.fillRect(r.x + r.w * 0.16, r.y + r.h * 0.4, r.w * 0.68, r.h * 0.06);
    ctx.fillRect(r.x + r.w * 0.16, r.y + r.h * 0.54, r.w * 0.68, r.h * 0.06);
    ctx.fillStyle = hA(C.COLORS.gate, 0.95);
    const d = Math.max(2.2, r.w * 0.12);
    diamondPath(ctx, r.x + r.w * 0.5, r.y + r.h * 0.5, d);
    ctx.fill();
    ctx.restore();
}

// figBridge — тоннель: перила + пунктирная настилка (§12.2).
export function figBridge(ctx, r) {
    ctx.save();
    ctx.fillStyle = hA(C.COLORS.bridge, 0.28);
    ctx.fillRect(r.x + r.w * 0.08, r.y + r.h * 0.34, r.w * 0.84, r.h * 0.32);
    const y0 = r.y + r.h * 0.34;
    const y1 = r.y + r.h * 0.66;
    ctx.strokeStyle = hA(C.COLORS.bridge, 0.9);
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.moveTo(r.x + r.w * 0.08, y0); ctx.lineTo(r.x + r.w * 0.92, y0);
    ctx.moveTo(r.x + r.w * 0.08, y1); ctx.lineTo(r.x + r.w * 0.92, y1);
    ctx.stroke();
    ctx.lineWidth = 1.2;
    ctx.setLineDash([3, 3]);
    ctx.beginPath();
    ctx.moveTo(r.x + r.w * 0.08, (y0 + y1) / 2);
    ctx.lineTo(r.x + r.w * 0.92, (y0 + y1) / 2);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();
}

// figChevron — шевроны течения по dir (0:+i,1:−i,2:+j,3:−j) + пунктирные хвосты.
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
    ctx.setLineDash([3, 3]);
    ctx.lineWidth = 1.2;
    ctx.strokeStyle = hA(C.COLORS.current, alpha * 0.7);
    for (const off of [-0.45, 0.45]) {
        const bx = cx - hx * 0.6 + px * off;
        const by = cy - hy * 0.6 + py * off;
        ctx.beginPath();
        ctx.moveTo(bx, by);
        ctx.lineTo(bx - hx * 0.7, by - hy * 0.7);
        ctx.stroke();
    }
    ctx.setLineDash([]);
    ctx.restore();
}

// figDeadEnd — обрыв: кайма + «стоп»-маркер (или спрайт false_signal).
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

// sectorRim — цвет каймы сектора по содержимому (§12.9): опасность трап/сбой
// красная, находки золотые, нейтральные мираже/вакуум стальные.
export function sectorRim(content) {
    if (content === 'trap' || content === 'unstable') return C.COLORS.unstable;
    if (content === 'jackpot' || content === 'lure') return C.COLORS.jackpot;
    if (content === 'decoy') return C.COLORS.decoy;
    if (content === 'empty') return C.COLORS.empty;
    return C.COLORS.sector;
}

// sectorRimDash — пунктир каймы (пульс у Сбоя, пунктир у Зова/Миража), §12.9.
export function sectorRimDash(content) {
    if (content === 'unstable') return [3, 2];
    if (content === 'lure' || content === 'decoy') return [4, 3];
    return null;
}

// sigTexture — σ-текстура сектора (§12.2): 3 / 5 дуг, у Гулкого — 8 хаотичных штрихов.
function sigTexture(ctx, r, sig) {
    const n = [3, 5, 8][sig] || 3;
    const s = Math.min(r.w, r.h);
    ctx.save();
    ctx.strokeStyle = hA(C.COLORS.sector, 0.5);
    ctx.lineWidth = 1;
    if (sig >= 2) {
        for (let k = 0; k < n; k++) {
            const x0 = r.x + ((k * 13 + 5) % Math.max(4, r.w - 6)) + 3;
            const y0 = r.y + ((k * 7 + 3) % Math.max(4, r.h - 6)) + 3;
            ctx.beginPath();
            ctx.moveTo(x0, y0);
            ctx.lineTo(x0 + (k % 2 ? 7 : -6), y0 + (k % 3 ? 5 : -4));
            ctx.stroke();
        }
    } else {
        for (let k = 0; k < n; k++) {
            ctx.beginPath();
            ctx.arc(r.x + r.w / 2, r.y + r.h / 2, s * (0.18 + k * 0.1), Math.PI * 0.15, Math.PI * 0.85);
            ctx.stroke();
        }
    }
    ctx.restore();
}

// figSector — одна клетка сектора-облака: заливка по σ + σ-текстура + кайма и
// центральный глиф по содержимому (глиф — только после вскрытия, §12.9).
export function figSector(ctx, r, sig, content) {
    ctx.save();
    ctx.fillStyle = C.COLORS.sector;
    ctx.globalAlpha = sectorDensity(sig);
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.restore();
    sigTexture(ctx, r, sig);
    ctx.save();
    ctx.strokeStyle = hA(sectorRim(content), content ? 0.9 : 0.45);
    ctx.lineWidth = content ? 1.6 : 1;
    const dash = sectorRimDash(content);
    if (dash) ctx.setLineDash(dash);
    ctx.strokeRect(r.x + 0.5, r.y + 0.5, r.w - 1, r.h - 1);
    ctx.setLineDash([]);
    ctx.restore();
    if (content) contentGlyph(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, content, 1);
}

// ---- Глиф содержимого сектора (§12.9): свой силуэт у каждого содержимого ----

// diamondPath — ромб как путь (вызывающий сам решает fill/stroke).
function diamondPath(ctx, x, y, r) {
    ctx.beginPath();
    ctx.moveTo(x, y - r); ctx.lineTo(x + r, y); ctx.lineTo(x, y + r); ctx.lineTo(x - r, y);
    ctx.closePath();
}

export function contentGlyph(ctx, x, y, cell, content, pulse) {
    const a = pulse == null ? 1 : pulse;
    const r = Math.max(3, cell * 0.22);
    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    if (content === 'jackpot') {
        ctx.strokeStyle = hA(C.COLORS.jackpot, 0.95 * a);
        ctx.lineWidth = Math.max(1.6, cell * 0.06);
        for (const [dx, dy] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
            ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + dx * r, y + dy * r); ctx.stroke();
        }
    } else if (content === 'lure') {
        ctx.strokeStyle = hA(C.COLORS.jackpot, 0.95 * a);
        ctx.lineWidth = Math.max(1.4, cell * 0.05);
        diamondPath(ctx, x, y, r); ctx.stroke();
        diamondPath(ctx, x, y, r * 0.55); ctx.stroke();
    } else if (content === 'trap') {
        ctx.strokeStyle = hA(C.COLORS.unstable, 0.95 * a);
        ctx.lineWidth = Math.max(1.6, cell * 0.06);
        ctx.beginPath();
        ctx.moveTo(x - r, y - r); ctx.lineTo(x + r, y + r);
        ctx.moveTo(x + r, y - r); ctx.lineTo(x - r, y + r);
        ctx.stroke();
    } else if (content === 'decoy') {
        ctx.strokeStyle = hA(C.COLORS.decoy, 0.9 * a);
        ctx.lineWidth = Math.max(1.4, cell * 0.05);
        ctx.beginPath(); ctx.arc(x, y, r, Math.PI * 0.25, Math.PI * 1.75); ctx.stroke();
    } else if (content === 'unstable') {
        ctx.strokeStyle = hA(C.COLORS.unstable, 0.95 * a);
        ctx.lineWidth = Math.max(1.6, cell * 0.06);
        ctx.beginPath();
        ctx.moveTo(x - r * 0.5, y - r); ctx.lineTo(x + r * 0.2, y - r * 0.1);
        ctx.lineTo(x - r * 0.2, y + r * 0.1); ctx.lineTo(x + r * 0.5, y + r);
        ctx.stroke();
    }
    ctx.restore();
}

// ---- Метки курса (§12.10): у каждой свой микро-глиф; цвет — вспомогательный ----

const MARK_COLORS = {
    mud: C.COLORS.heatCost, wall: C.COLORS.heatCost,
    gate: C.COLORS.gate, gate_twice: C.COLORS.gate,
    bridge: C.COLORS.bridge, bridge_twice: C.COLORS.bridge,
    current_against: C.COLORS.unstable, current_along: C.COLORS.current,
    dead_end: C.COLORS.deadEnd, turn: C.COLORS.captureSoft,
    hazard: C.COLORS.unstable, jackpot: C.COLORS.jackpot,
};

export function heatColor(kind) { return MARK_COLORS[kind] || C.COLORS.capture; }

// heatGlyph — микро-глиф метки курса в точке (x,y) (§12.10); размер ≈0.22·cell,
// толщина штриха ≥1.4 px. pulse — «дыхание» метки (reduced-motion → статично).
export function heatGlyph(ctx, x, y, cell, kind, pulse) {
    const a = pulse == null ? 1 : pulse;
    const col = heatColor(kind);
    const r = Math.max(3, cell * 0.22);
    ctx.save();
    ctx.strokeStyle = hA(col, 0.9 * a);
    ctx.fillStyle = hA(col, 0.9 * a);
    ctx.lineWidth = Math.max(1.4, cell * 0.045);
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    if (kind === 'mud' || kind === 'wall') {
        for (const oy of [-r * 0.35, r * 0.35]) {
            ctx.beginPath();
            ctx.moveTo(x - r, y + oy);
            ctx.quadraticCurveTo(x - r * 0.5, y + oy - r * 0.5, x, y + oy);
            ctx.quadraticCurveTo(x + r * 0.5, y + oy + r * 0.5, x + r, y + oy);
            ctx.stroke();
        }
    } else if (kind === 'gate') {
        diamondPath(ctx, x, y, r); ctx.stroke();
    } else if (kind === 'gate_twice') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.35, y - r); ctx.lineTo(x - r * 0.35, y + r);
        ctx.moveTo(x + r * 0.35, y - r); ctx.lineTo(x + r * 0.35, y + r);
        ctx.stroke();
    } else if (kind === 'bridge') {
        ctx.setLineDash([2.5, 2.5]);
        ctx.beginPath(); ctx.moveTo(x - r, y); ctx.lineTo(x + r, y); ctx.stroke();
        ctx.setLineDash([]);
    } else if (kind === 'bridge_twice') {
        ctx.setLineDash([2.5, 2.5]);
        ctx.beginPath();
        ctx.moveTo(x - r, y - r * 0.4); ctx.lineTo(x + r, y - r * 0.4);
        ctx.moveTo(x - r, y + r * 0.4); ctx.lineTo(x + r, y + r * 0.4);
        ctx.stroke();
        ctx.setLineDash([]);
    } else if (kind === 'current_against') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.7, y - r * 0.6); ctx.lineTo(x, y); ctx.lineTo(x - r * 0.7, y + r * 0.6);
        ctx.stroke();
        ctx.beginPath(); ctx.moveTo(x + r * 0.9, y - r * 0.9); ctx.lineTo(x - r * 0.1, y + r * 0.1); ctx.stroke();
    } else if (kind === 'current_along') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.7, y - r * 0.6); ctx.lineTo(x, y); ctx.lineTo(x - r * 0.7, y + r * 0.6);
        ctx.stroke();
    } else if (kind === 'dead_end') {
        ctx.fillRect(x - r, y - r * 0.3, r * 2, r * 0.6);
    } else if (kind === 'turn') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.6, y + r * 0.5);
        ctx.lineTo(x - r * 0.6, y - r * 0.3);
        ctx.lineTo(x + r * 0.5, y - r * 0.3);
        ctx.stroke();
    } else if (kind === 'hazard') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.5, y - r); ctx.lineTo(x + r * 0.2, y - r * 0.1);
        ctx.lineTo(x - r * 0.2, y + r * 0.1); ctx.lineTo(x + r * 0.5, y + r);
        ctx.stroke();
    } else if (kind === 'jackpot') {
        for (const [dx, dy] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
            ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + dx * r, y + dy * r); ctx.stroke();
        }
    }
    ctx.restore();
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

// ---- Диспетчер мини-фигур легенды (§4.9): id из LEGEND_ITEMS → фигура ----

export function drawLegendFigure(ctx, id, r) {
    const cx = r.x + r.w / 2;
    const cy = r.y + r.h / 2;
    switch (id) {
        case 'start': figStart(ctx, r); break;
        case 'beacon': figBeacon(ctx, r); break;
        case 'finish': figStar(ctx, r); break;
        case 'lane': figLane(ctx, r); break;
        case 'mud': figMud(ctx, r); break;
        case 'wall': figWall(ctx, r); break;
        case 'bottleneck': figBottleneck(ctx, r); break;
        case 'current': figChevron(ctx, r, 0, 0.9); break;
        case 'gate': figGate(ctx, r); break;
        case 'bridge': figBridge(ctx, r); break;
        case 'dead_end': figDeadEnd(ctx, r, null); break;
        case 'sector': figSector(ctx, r, 1, null); break;
        case 'sig_quiet': figSector(ctx, r, 0, null); break;
        case 'sig_mid': figSector(ctx, r, 1, null); break;
        case 'sig_loud': figSector(ctx, r, 2, null); break;
        case 'mark_resistance': heatGlyph(ctx, cx, cy, r.w, 'mud', 1); break;
        case 'mark_turn': heatGlyph(ctx, cx, cy, r.w, 'turn', 1); break;
        case 'mark_dead_end': heatGlyph(ctx, cx, cy, r.w, 'dead_end', 1); break;
        case 'mark_against': heatGlyph(ctx, cx, cy, r.w, 'current_against', 1); break;
        case 'mark_along': heatGlyph(ctx, cx, cy, r.w, 'current_along', 1); break;
        case 'mark_hazard': heatGlyph(ctx, cx, cy, r.w, 'hazard', 1); break;
        case 'mark_jackpot': heatGlyph(ctx, cx, cy, r.w, 'jackpot', 1); break;
        case 'mark_gate_twice': heatGlyph(ctx, cx, cy, r.w, 'gate_twice', 1); break;
        case 'mark_bridge_twice': heatGlyph(ctx, cx, cy, r.w, 'bridge_twice', 1); break;
        default: break;
    }
}
