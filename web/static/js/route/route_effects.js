// web/static/js/route/route_effects.js
// Эффекты кадра мини-игры «Прокладка маршрута» (§4.7): световой фронт вдоль пути
// после отправки — тон по знаку bonus (зелёный «ускорение» / янтарный
// «торможение»), на завершении пробега вспышка на Цели. Число bonus в сцене
// не показывается (он только в оверлее). Вынесено из route_render.js, чтобы не
// растить его.
import * as C from './route_config.js';
import { cellCenter } from './route_board.js';

const TAU = Math.PI * 2;
const hA = C.hexA;

function radial(ctx, x, y, r, color, a) {
    const g = ctx.createRadialGradient(x, y, 0, x, y, r);
    g.addColorStop(0, hA(color, a)); g.addColorStop(1, hA(color, 0));
    ctx.fillStyle = g;
    ctx.fillRect(x - r, y - r, r * 2, r * 2);
}

// drawResultFx — световой фронт вдоль пути после отправки (§4.7).
export function drawResultFx(ctx, st, view, now) {
    const fx = st.resultFx;
    const path = st.path || [];
    if (!fx || path.length < 2) return;
    const pts = path.map((c) => cellCenter(view, c));
    const segs = [];
    let total = 0;
    for (let k = 1; k < pts.length; k++) {
        const d = Math.hypot(pts[k].x - pts[k - 1].x, pts[k].y - pts[k - 1].y);
        segs.push(d); total += d;
    }
    const t = st.reduced ? 1 : Math.min(1, (now - fx.at) / 900);
    const col = fx.bonus < 0 ? C.COLORS.warning : C.COLORS.success;
    const p = pointAlong(pts, segs, total, t * total);
    const cell = view.size / view.n;
    radial(ctx, p.x, p.y, cell * 0.9, col, 0.75);
    ctx.fillStyle = hA(col, 0.95);
    ctx.beginPath(); ctx.arc(p.x, p.y, Math.max(2.5, cell * 0.14), 0, TAU); ctx.fill();
    if (t >= 1) {
        const f = cellCenter(view, st.board.finish);
        radial(ctx, f.x, f.y, cell * (0.9 + 0.5 * Math.sin(now * 0.006)), col, 0.5);
    }
}

// pointAlong — точка на ломаной пути на расстоянии target от начала.
function pointAlong(pts, segs, total, target) {
    if (total <= 0) return pts[0];
    let acc = 0;
    for (let k = 1; k < pts.length; k++) {
        const s = segs[k - 1];
        if (acc + s >= target) {
            const f = s > 0 ? (target - acc) / s : 0;
            return {
                x: pts[k - 1].x + (pts[k].x - pts[k - 1].x) * f,
                y: pts[k - 1].y + (pts[k].y - pts[k - 1].y) * f,
            };
        }
        acc += s;
    }
    return pts[pts.length - 1];
}
