// web/static/js/belt/belt_render.js
// Отрисовка сцены добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §9.1/§9.2), вид сверху. Слои
// снизу вверх: звёздное поле → далёкая пыль → мелкие обломки (параллакс 0.6)
// → крупные жилы (1.0) → частицы руды → корабль → эффекты (луч бурения,
// «+X т»). Ассеты астероидов — подэтап 3c; здесь плейсхолдеры (фигуры).
import * as C from './belt_config.js';
import { shipDrawTransform } from '../map/ship_sprites.js';

function toScreen(wx, wy, cam, vw, vh) {
    return { x: (wx - cam.x) * C.WORLD_SCALE + vw / 2, y: (wy - cam.y) * C.WORLD_SCALE + vh / 2 };
}

// drawScene — полный кадр сцены. opts: { drilling, target, depleted,
// remainingLevel, shipSprite, shipOrient, now }.
export function drawScene(ctx, world, cam, vw, vh, opts) {
    ctx.fillStyle = C.COLORS.bg;
    ctx.fillRect(0, 0, vw, vh);

    drawStars(ctx, world.stars, cam, vw, vh, opts.now);
    drawDust(ctx, world.dust, cam, vw, vh);
    drawFieldEdge(ctx, cam, vw, vh);

    // Мелкие обломки (декор) — параллакс 0.6.
    for (const a of world.asteroids) {
        if (a.vein) continue;
        drawAsteroid(ctx, a, cam, vw, vh, opts, C.PARALLAX_DECOR);
    }
    // Крупные жилы — игровое поле (параллакс 1.0).
    for (const a of world.asteroids) {
        if (!a.vein) continue;
        drawAsteroid(ctx, a, cam, vw, vh, opts, 1);
    }

    drawParticles(ctx, world.particles, cam, vw, vh);

    if (opts.drilling && opts.target) drawBeam(ctx, world.ship, opts.target, cam, vw, vh);
    drawShip(ctx, world.ship, cam, vw, vh, opts);
    drawFloaters(ctx, world.floaters, cam, vw, vh);
}

function drawStars(ctx, stars, cam, vw, vh, now) {
    const T = C.STAR_TILE;
    const ox = (((-cam.x * C.PARALLAX_STARS) % T) + T) % T;
    const oy = (((-cam.y * C.PARALLAX_STARS) % T) + T) % T;
    const gxMax = Math.ceil(vw / T);
    const gyMax = Math.ceil(vh / T);
    for (let gx = -1; gx <= gxMax; gx++) {
        for (let gy = -1; gy <= gyMax; gy++) {
            const bx = gx * T + ox;
            const by = gy * T + oy;
            for (const st of stars) {
                const sx = bx + st.x;
                const sy = by + st.y;
                if (sx < -4 || sx > vw + 4 || sy < -4 || sy > vh + 4) continue;
                const tw = 0.75 + 0.25 * Math.sin(now * 0.001 * st.tw + st.ph);
                ctx.globalAlpha = Math.max(0, st.a * tw);
                ctx.fillStyle = st.c;
                ctx.fillRect(sx, sy, st.s, st.s);
            }
        }
    }
    ctx.globalAlpha = 1;
}

function drawDust(ctx, dust, cam, vw, vh) {
    for (const d of dust) {
        const p = toScreen(d.x - cam.x * (C.PARALLAX_DUST - 1), d.y - cam.y * (C.PARALLAX_DUST - 1), cam, vw, vh);
        const rad = d.r * C.WORLD_SCALE;
        if (p.x < -rad || p.x > vw + rad || p.y < -rad || p.y > vh + rad) continue;
        const g = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, rad);
        g.addColorStop(0, 'rgba(120,130,160,' + d.a + ')');
        g.addColorStop(1, 'rgba(120,130,160,0)');
        ctx.fillStyle = g;
        ctx.fillRect(p.x - rad, p.y - rad, rad * 2, rad * 2);
    }
}

// drawFieldEdge — тонкое кольцо мягкой границы пятна (навигационный ориентир).
function drawFieldEdge(ctx, cam, vw, vh) {
    const p = toScreen(0, 0, cam, vw, vh);
    ctx.save();
    ctx.strokeStyle = C.COLORS.edge;
    ctx.lineWidth = C.EDGE_SOFT * C.WORLD_SCALE * 0.5;
    ctx.beginPath();
    ctx.arc(p.x, p.y, C.FIELD_RADIUS * C.WORLD_SCALE, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
}

// drawAsteroid — плейсхолдер астероида: неровный многоугольник + блеск жилы.
// parallax — множитель слоя (декор 0.6, жилы 1.0).
function drawAsteroid(ctx, a, cam, vw, vh, opts, parallax) {
    const p = toScreen(a.x - cam.x * (parallax - 1), a.y - cam.y * (parallax - 1), cam, vw, vh);
    const rad = a.r * C.WORLD_SCALE;
    if (p.x < -rad * 2 || p.x > vw + rad * 2 || p.y < -rad * 2 || p.y > vh + rad * 2) return;

    const depleted = !!opts.depleted;
    const dim = a.drill; // визуальное истощение конкретной жилы

    ctx.save();
    ctx.translate(p.x, p.y);
    ctx.rotate(a.rot);
    ctx.beginPath();
    const n = a.shape.length;
    for (let i = 0; i < n; i++) {
        const ang = (i / n) * Math.PI * 2;
        const rr = rad * a.shape[i];
        const x = Math.cos(ang) * rr;
        const y = Math.sin(ang) * rr;
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.fillStyle = depleted ? C.COLORS.asteroidDark : (a.vein ? C.COLORS.asteroidVein : C.COLORS.asteroid);
    if (dim > 0 && !depleted) ctx.globalAlpha = Math.max(0.55, 1 - dim * 0.4);
    ctx.fill();
    ctx.globalAlpha = 1;
    ctx.lineWidth = 2;
    ctx.strokeStyle = C.COLORS.asteroidEdge;
    ctx.stroke();

    // Блеск руды на жилах — гаснет при выработке/локальном истощении.
    if (a.vein && !depleted) {
        const glintA = Math.max(0, 1 - dim);
        for (const g of a.glints) {
            ctx.globalAlpha = glintA * (0.5 + 0.5 * Math.sin(opts.now * 0.002 + g.x));
            ctx.fillStyle = C.COLORS.glint;
            ctx.beginPath();
            ctx.arc(g.x, g.y, g.r, 0, Math.PI * 2);
            ctx.fill();
        }
        ctx.globalAlpha = 1;
    }
    ctx.restore();
}

function drawParticles(ctx, parts, cam, vw, vh) {
    ctx.fillStyle = C.COLORS.particle;
    for (const p of parts) {
        const s = toScreen(p.x, p.y, cam, vw, vh);
        ctx.globalAlpha = Math.max(0, p.life / p.max);
        ctx.fillRect(s.x - p.r / 2, s.y - p.r / 2, p.r, p.r);
    }
    ctx.globalAlpha = 1;
}

function drawBeam(ctx, ship, target, cam, vw, vh) {
    const a = toScreen(ship.x, ship.y, cam, vw, vh);
    const b = toScreen(target.x, target.y, cam, vw, vh);
    ctx.save();
    ctx.strokeStyle = C.COLORS.beam;
    ctx.globalAlpha = 0.55 + 0.25 * Math.sin(Date.now() * 0.02);
    ctx.lineWidth = 2;
    ctx.setLineDash([6, 6]);
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();
    ctx.restore();
}

// drawShip — корабль игрока: спрайт из реестра (с Angle/Flip, спека
// 2026-09-21) или фолбэк-треугольник, если спрайт не загрузился (И4).
function drawShip(ctx, ship, cam, vw, vh, opts) {
    const p = toScreen(ship.x, ship.y, cam, vw, vh);
    const size = C.SHIP_SIZE;
    const t = shipDrawTransform(ship.heading, opts.shipOrient || { angle: 0, flip: false });
    ctx.save();
    ctx.translate(p.x, p.y);
    if (opts.shipSprite) {
        ctx.rotate(t.rotate);
        ctx.scale(t.scaleX, t.scaleY);
        ctx.drawImage(opts.shipSprite, -size / 2, -size / 2, size, size);
    } else {
        ctx.rotate(ship.heading);
        ctx.beginPath();
        ctx.moveTo(size * 0.5, 0);
        ctx.lineTo(-size * 0.35, size * 0.32);
        ctx.lineTo(-size * 0.2, 0);
        ctx.lineTo(-size * 0.35, -size * 0.32);
        ctx.closePath();
        ctx.fillStyle = C.COLORS.shipFallback;
        ctx.fill();
        ctx.lineWidth = 1.5;
        ctx.strokeStyle = C.COLORS.shipAccent;
        ctx.stroke();
    }
    ctx.restore();

    // Сопло/тяга — процедурно (§9.1 п.4).
    if (ship.thrusting || ship.braking) {
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(ship.heading);
        ctx.globalAlpha = ship.braking ? 0.35 : 0.6;
        ctx.fillStyle = C.COLORS.shipAccent;
        const flame = 0.6 + 0.15 * Math.sin(Date.now() * 0.03);
        ctx.beginPath();
        ctx.moveTo(-size * 0.3, 0);
        ctx.lineTo(-size * flame, size * 0.14);
        ctx.lineTo(-size * flame, -size * 0.14);
        ctx.closePath();
        ctx.fill();
        ctx.restore();
    }
}

function drawFloaters(ctx, floaters, cam, vw, vh) {
    ctx.save();
    ctx.font = '13px "Segoe UI", Roboto, system-ui, sans-serif';
    ctx.textAlign = 'center';
    for (const f of floaters) {
        const p = toScreen(f.x, f.y, cam, vw, vh);
        ctx.globalAlpha = Math.max(0, f.life / f.max);
        ctx.fillStyle = C.COLORS.glint;
        ctx.fillText(f.text, p.x, p.y);
    }
    ctx.restore();
    ctx.globalAlpha = 1;
}
