// web/static/js/route/route_render_core.js
// Спрайты ядра сцены мини-игры «Прокладка маршрута»: Цель, маяки, Старт (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.4). Вынесено из route_render.js
// без изменения поведения.
import * as C from './route_config.js';
import { cellCenter } from './route_board.js';
import { getSprite, bloomSprite } from './route_sprites.js';
import { TAU, radial, ring, startTriangle, beaconRings, beaconSquare } from './route_figures.js';
import { shipDrawTransform } from '../map/ship_sprites.js';
import { CONFIG } from '../config.js';

// Цвет/оттенок звезды — из map/utils.js (формулу не дублируем). map/config.js
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

const starColor = (s) => {
    if (getStarShade) return getStarShade(s.spectral_class, s.temperature, s.star_type);
    const c = CONFIG.map.starColors;
    return (s.star_type && c[s.star_type]) || c[s.spectral_class] || c.default;
};

// ---- Спрайты ядра сцены ----

export function drawFinish(ctx, st, view, now) {
    const f = cellCenter(view, st.board.finish);
    const to = (st.passport && st.passport.to) || {};
    const black = to.star_type === 'black_hole';
    const base = view.size / view.n;
    const r = base * 0.9 * (st.reduced ? 1 : 1 + 0.06 * Math.sin(now * 0.0015));
    const img = getSprite(st.chosen && (black ? st.chosen.black_hole : st.chosen.star_core));
    if (black) {
        radial(ctx, f.x, f.y, r * 1.5, C.COLORS.voidHalo, 0.7);
        if (img) {
            bloomSprite(ctx, img, f.x, f.y, r * 1.8, 0);
            ctx.fillStyle = '#02030a';
            ctx.beginPath(); ctx.arc(f.x, f.y, r * 0.36, 0, TAU); ctx.fill();
            ctx.globalAlpha = 1;
        } else ring(ctx, f.x, f.y, r * 0.8, C.COLORS.warmHalo, 1, 2);
        return;
    }
    const col = starColor(to);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    radial(ctx, f.x, f.y, r * 1.5, col, 0.5);
    if (img) bloomSprite(ctx, img, f.x, f.y, r * 1.8, 0.25);
    ctx.restore();
    if (!img) radial(ctx, f.x, f.y, r * 0.9, col, 1);
}

export function drawStart(ctx, st, view) {
    const s = cellCenter(view, st.board.start);
    const base = view.size / view.n;
    const r = base * 0.7;
    if (st.shipSprite) {
        const tf = shipDrawTransform(0, st.shipOrient);
        const sz = r * 1.7;
        ctx.save();
        ctx.translate(s.x, s.y); ctx.rotate(tf.rotate); ctx.scale(tf.scaleX, tf.scaleY);
        ctx.drawImage(st.shipSprite, -sz / 2, -sz / 2, sz, sz);
        ctx.restore();
    } else {
        startTriangle(ctx, s.x, s.y, r, C.COLORS.cold);
    }
    ring(ctx, s.x, s.y, base * 0.46, C.COLORS.cold, 0.6, 1.2);
}

export function drawBeacons(ctx, st, view, now) {
    const b = st.board;
    const img = getSprite(st.chosen && st.chosen.beacon);
    const base = view.size / view.n;
    for (const c of b.beacons || []) {
        const p = cellCenter(view, c);
        const captured = st.captured.has(c);
        const sz = base * 0.8;
        const pulse = st.reduced ? 0.6 : (now % 1600) / 1600;
        beaconRings(ctx, p.x, p.y, base, pulse, captured);
        if (captured) drawCaptureFlash(ctx, p, base * 0.42, st.flash.get(c), now);
        if (img) {
            ctx.save(); ctx.globalCompositeOperation = 'lighter'; bloomSprite(ctx, img, p.x, p.y, sz, captured ? 0.35 : 0.2); ctx.restore();
        } else {
            beaconSquare(ctx, p.x, p.y, captured);
        }
    }
}

// drawCaptureFlash — вспышка захвата маяка 180 мс + кольцо-разлёт 400 мс.
function drawCaptureFlash(ctx, p, r, flash, now) {
    if (flash == null) return;
    const dt = now - flash;
    if (dt < 180) radial(ctx, p.x, p.y, r * 1.8, C.COLORS.captureSoft, 0.8 * (1 - dt / 180));
    else if (dt < 580) ring(ctx, p.x, p.y, r * (1 + (dt - 180) / 400 * 1.2), C.COLORS.captureSoft, 0.7 * (1 - (dt - 180) / 400), 2);
}
