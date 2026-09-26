// web/static/js/route/route_figures_prims.js
// Общие примитивы отрисовки фигур доски v9 «Планшет» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.4). Вынесено из route_figures.js
// без изменения поведения; фигуры доски/секторов/меток — в соседних файлах.
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

// lensLine — продольная сужающаяся линия (линза) от x0 к x1 в y, макс. полуширина hw.
export function lensLine(ctx, x0, x1, y, hw, steps) {
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
export function hatch(ctx, r, angDeg, step, dash, phase) {
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

// diamondPath — ромб как путь (вызывающий сам решает fill/stroke).
export function diamondPath(ctx, x, y, r) {
    ctx.beginPath();
    ctx.moveTo(x, y - r); ctx.lineTo(x + r, y); ctx.lineTo(x, y + r); ctx.lineTo(x - r, y);
    ctx.closePath();
}
