// web/static/js/route/route_figures_landmarks.js
// Ориентиры доски — кодовые фигуры Старта, маяка и Цели (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §4.9/§12.2). Вынесено из
// route_figures.js без изменения поведения; легенда — не спрайт.
import * as C from './route_config.js';
import { hA, radial, ring } from './route_figures_prims.js';

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
