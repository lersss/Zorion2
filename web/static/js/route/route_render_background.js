// web/static/js/route/route_render_background.js
// Фон сцены мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.4). Вынесено из route_render.js
// без изменения поведения.
import { radial } from './route_figures.js';
import { getSprite, tintedSprite } from './route_sprites.js';

export function drawBackground(ctx, b, chosen, reduced, vw, vh, now) {
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

export function drawVignette(ctx, vw, vh) {
    const g = ctx.createRadialGradient(vw / 2, vh / 2, Math.min(vw, vh) * 0.3, vw / 2, vh / 2, Math.max(vw, vh) * 0.75);
    g.addColorStop(0, 'rgba(0,0,0,0)'); g.addColorStop(1, 'rgba(0,0,0,0.45)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, vw, vh);
}
