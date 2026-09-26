// web/static/js/route/route_figures_field.js
// Фигуры поля доски v9 «Планшет» (спека 2026-09-25-маршрут-мини-игра-интерфейс.md
// §12.2/§12.13). Вынесено из route_figures.js без изменения поведения.
import * as C from './route_config.js';
import { hA, radial, lensLine, hatch, diamondPath } from './route_figures_prims.js';

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
