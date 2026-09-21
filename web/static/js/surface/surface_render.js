// web/static/js/surface/surface_render.js
// Отрисовка прогулки (спека 2026-09-21 §7.2): параллакс-небо (только из sky
// пакета), дальний рельеф, основной рельеф/пещеры (чанки кэшируются), декор,
// жизнь, игрок, частицы погоды, HUD (в surface_ui.js). Canvas 2D.
import { CHUNK, CHUNK_RADIUS, COLORS } from './surface_config.js';
import { shade, rgba } from './surface_world.js';

const CHUNK_TOP_MARGIN = 620;
const CHUNK_HEIGHT = 1700;

// getChunkCanvas — лениво отрисованный чанк (кэш). Рельеф + пещеры.
export function getChunkCanvas(world, index) {
    if (!world._chunkCache) world._chunkCache = new Map();
    const cached = world._chunkCache.get(index);
    if (cached) return cached;

    const canvas = document.createElement('canvas');
    canvas.width = CHUNK;
    canvas.height = CHUNK_HEIGHT;
    const ctx = canvas.getContext('2d');
    const topY = world.baseY - CHUNK_TOP_MARGIN;
    const baseX = index * CHUNK;
    const rock = shade(world.color, 0.62);

    ctx.fillStyle = rock;
    for (let lx = 0; lx < CHUNK; lx++) {
        const wx = baseX + lx;
        const th = world.terrainHeight(wx);
        const y0 = Math.floor(th - topY);
        if (y0 >= CHUNK_HEIGHT) continue;
        ctx.fillRect(lx, Math.max(0, y0), 1, CHUNK_HEIGHT - Math.max(0, y0));
    }

    // Пещеры — грубая маска (шаг 3 px, иначе дорого).
    ctx.fillStyle = COLORS.cave;
    for (let lx = 0; lx < CHUNK; lx += 3) {
        const wx = baseX + lx;
        const th = world.terrainHeight(wx);
        for (let ly = 0; ly < CHUNK_HEIGHT; ly += 3) {
            const wy = topY + ly;
            if (wy < th + 6) continue;
            if (world.isCave(wx, wy)) ctx.fillRect(lx, ly, 3, 3);
        }
    }

    // Глубинный градиент (объём).
    const grad = ctx.createLinearGradient(0, 0, 0, CHUNK_HEIGHT);
    grad.addColorStop(0, 'rgba(0,0,0,0)');
    grad.addColorStop(1, 'rgba(0,0,0,0.72)');
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, CHUNK, CHUNK_HEIGHT);

    // Кромка поверхности — светлее.
    ctx.fillStyle = shade(world.color, 1.15);
    for (let lx = 0; lx < CHUNK; lx++) {
        const wx = baseX + lx;
        const ly = Math.floor(world.terrainHeight(wx) - topY);
        if (ly >= 0 && ly < CHUNK_HEIGHT) ctx.fillRect(lx, ly, 1, 2);
    }

    const result = { canvas, topY };
    world._chunkCache.set(index, result);
    return result;
}

// drawSky — параллакс-небо из пакета (§7.1, В2): светило + тела системы.
export function drawSky(ctx, vw, vh, sky, camera, timeMs) {
    const grad = ctx.createLinearGradient(0, 0, 0, vh);
    grad.addColorStop(0, COLORS.skyTop);
    grad.addColorStop(1, COLORS.skyBottom);
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, vw, vh);

    if (!sky) return;
    const star = sky.star || {};
    // Светило: слабый параллакс (0.05) + ореол.
    const sx = vw * 0.72 - camera.x * 0.05;
    const sy = vh * 0.20 - camera.y * 0.03;
    const r = Math.max(28, vh * 0.06);
    const halo = ctx.createRadialGradient(sx, sy, r * 0.2, sx, sy, r * 2.4);
    halo.addColorStop(0, star.color || '#ffd700');
    halo.addColorStop(0.35, rgba(star.color || '#ffd700', 0.35));
    halo.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(sx, sy, r * 2.4, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = star.color || '#ffd700';
    ctx.beginPath();
    ctx.arc(sx, sy, r, 0, Math.PI * 2);
    ctx.fill();

    // Тела системы: параллакс 0.12–0.3, высота height из пакета.
    (sky.bodies || []).forEach((b, i) => {
        const depth = 0.10 + 0.16 * (i / Math.max(1, sky.bodies.length - 1 || 1));
        const bx = vw * 0.5 + (i - 1) * vw * 0.22 - camera.x * depth;
        const by = vh * (0.10 + 0.5 * (b.height || 0.3)) - camera.y * 0.04;
        const size = Math.max(6, vh * 0.05 * (b.size_hint || 0.3) * 3);
        const g = ctx.createRadialGradient(bx - size * 0.3, by - size * 0.3, size * 0.1, bx, by, size);
        g.addColorStop(0, '#ffffff');
        g.addColorStop(0.25, b.color || '#8a7a6a');
        g.addColorStop(1, shade(b.color || '#8a7a6a', 0.35));
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.arc(bx, by, size, 0, Math.PI * 2);
        ctx.fill();
    });
}

// drawFarRelief — дальний силуэт рельефа (параллакс 0.35).
export function drawFarRelief(ctx, world, camera, vw, vh) {
    ctx.fillStyle = 'rgba(8,12,22,0.85)';
    ctx.beginPath();
    ctx.moveTo(0, vh);
    for (let sx = 0; sx <= vw; sx += 12) {
        const wx = camera.x * 0.35 + (sx - vw / 2);
        const h = world.baseY - 220 - (world.terrainHeight(wx * 1.4) - world.baseY) * 0.5;
        const y = h - camera.y * 0.35 + vh * 0.35;
        ctx.lineTo(sx, y);
    }
    ctx.lineTo(vw, vh);
    ctx.closePath();
    ctx.fill();
}

// drawTerrain — основной рельеф из кэшированных чанков.
export function drawTerrain(ctx, world, camera, vw, vh) {
    const centerChunk = Math.floor(camera.x / CHUNK);
    for (let i = centerChunk - CHUNK_RADIUS; i <= centerChunk + CHUNK_RADIUS; i++) {
        const { canvas, topY } = getChunkCanvas(world, i);
        const sx = i * CHUNK - camera.x + vw / 2;
        const sy = topY - camera.y + vh / 2;
        ctx.drawImage(canvas, sx, sy);
    }
}

// drawDecor — растительность/лишайники/камни (step 4) + редкие находки.
export function drawDecor(ctx, world, camera, vw, vh) {
    const left = camera.x - vw / 2;
    for (let sx = 0; sx <= vw; sx += 4) {
        const wx = left + sx;
        const d = world.decorAt(wx);
        if (!d) continue;
        const gy = world.terrainHeight(wx) - camera.y + vh / 2;
        const x = sx;
        if (d.kind === 'tree') {
            ctx.strokeStyle = shade(world.color, 0.5);
            ctx.lineWidth = 3;
            ctx.beginPath();
            ctx.moveTo(x, gy);
            ctx.lineTo(x, gy - d.h);
            ctx.stroke();
            ctx.fillStyle = world.life ? shade(world.color, 1.5) : shade(world.color, 0.7);
            ctx.beginPath();
            ctx.arc(x, gy - d.h, d.h * 0.45, 0, Math.PI * 2);
            ctx.fill();
        } else if (d.kind === 'plant') {
            ctx.strokeStyle = world.life ? shade(world.color, 1.7) : shade(world.color, 0.8);
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(x, gy);
            ctx.quadraticCurveTo(x + 4, gy - d.h * 0.6, x + 2, gy - d.h);
            ctx.stroke();
        } else if (d.kind === 'lichen') {
            ctx.fillStyle = rgba(world.color, 0.5);
            ctx.fillRect(x, gy - d.h, 5, d.h);
        } else {
            ctx.fillStyle = shade(world.color, 0.45);
            ctx.beginPath();
            ctx.arc(x, gy - 3, d.h * 0.4, 0, Math.PI * 2);
            ctx.fill();
        }
    }

    // Редкие декорации-находки (любопытство §9).
    const start = Math.floor(left / 200) * 200;
    for (let wx = start; wx <= left + vw + 200; wx += 200) {
        const r = world.rareDecorAt(wx);
        if (!r) continue;
        const x = r.x - camera.x + vw / 2;
        const gy = world.terrainHeight(r.x) - camera.y + vh / 2;
        ctx.save();
        ctx.fillStyle = r.kind === 'кристалл' ? '#9ae6ff' : r.kind === 'обломок' ? '#b0b7c3' : '#e3d1a0';
        ctx.beginPath();
        ctx.moveTo(x, gy - 16);
        ctx.lineTo(x + 8, gy - 4);
        ctx.lineTo(x + 4, gy);
        ctx.lineTo(x - 4, gy);
        ctx.lineTo(x - 8, gy - 4);
        ctx.closePath();
        ctx.fill();
        ctx.restore();
    }
}

// drawCreatures — животные (не бой, §7.3): пасётся/убегает/подходит/стайка/детёныш.
export function drawCreatures(ctx, world, camera, vw, vh, player, timeMs) {
    const centerChunk = Math.floor(camera.x / CHUNK);
    for (let i = centerChunk - CHUNK_RADIUS; i <= centerChunk + CHUNK_RADIUS; i++) {
        for (const c of world.creaturesFor(i)) {
            let x = c.x;
            let flip = 1;
            const dist = player ? player.x - x : 0;
            // Поведение: реакция на игрока (без урона).
            if (c.behavior === 'flee' && Math.abs(dist) < 120) {
                x += Math.sign(-dist) * 40;
                flip = dist > 0 ? -1 : 1;
            } else if (c.behavior === 'approach' && Math.abs(dist) < 180) {
                x += Math.sign(dist) * 24;
                flip = dist > 0 ? 1 : -1;
            } else if (c.behavior === 'graze') {
                x += Math.sin(timeMs * 0.001 + c.phase) * 10;
            } else if (c.behavior === 'herd') {
                x += Math.sin(timeMs * 0.0007 + c.phase) * 30;
            }
            const sx = x - camera.x + vw / 2;
            if (sx < -40 || sx > vw + 40) continue;
            const sy = world.terrainHeight(x) - camera.y + vh / 2;
            const size = c.behavior === 'juvenile' ? c.size * 0.6 : c.size;
            const body = `hsl(${Math.floor(c.hue * 360)},45%,${world.life ? 55 : 40}%)`;
            ctx.fillStyle = body;
            ctx.beginPath();
            ctx.ellipse(sx, sy - size * 0.5, size, size * 0.62, 0, 0, Math.PI * 2);
            ctx.fill();
            // Голова в сторону движения.
            ctx.beginPath();
            ctx.arc(sx + flip * size * 0.9, sy - size * 0.7, size * 0.35, 0, Math.PI * 2);
            ctx.fill();
        }
    }
}

// drawPlayer — игрок (простой силуэт в скафандре).
export function drawPlayer(ctx, player, camera, vw, vh, timeMs) {
    const x = player.x - camera.x + vw / 2;
    const y = player.y - camera.y + vh / 2;
    const bob = player.onGround ? Math.sin(timeMs * 0.01) * Math.min(1.5, Math.abs(player.vx) * 0.05) : 0;
    const w = player.w;
    const h = player.h;
    ctx.save();
    ctx.translate(x, y + bob);
    // Пульс-кольцо «я здесь».
    ctx.strokeStyle = 'rgba(56,189,248,0.28)';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(0, 0, h * 0.8 + Math.sin(timeMs * 0.004) * 2, 0, Math.PI * 2);
    ctx.stroke();
    // Скафандр.
    ctx.fillStyle = COLORS.player;
    ctx.fillRect(-w / 2, -h / 2, w, h);
    // Визор.
    ctx.fillStyle = COLORS.playerAccent;
    ctx.fillRect(-w / 2 + 2, -h / 2 + 3, w - 4, 6);
    ctx.restore();
}

// drawParticles — частицы погоды (визуально, без урона §7.2).
export function drawWeather(ctx, weather, vw, vh, timeMs) {
    const id = weather && weather.particles;
    if (!id || id === 'none') return;
    ctx.save();
    if (id === 'fog') {
        ctx.fillStyle = 'rgba(200,210,220,0.18)';
        for (let i = 0; i < 3; i++) {
            const y = vh * (0.3 + i * 0.2) + Math.sin(timeMs * 0.0005 + i) * 10;
            ctx.fillRect(0, y, vw, 60);
        }
    } else if (id === 'aurora') {
        const g = ctx.createLinearGradient(0, 0, 0, vh * 0.5);
        g.addColorStop(0, 'rgba(80,220,180,0.22)');
        g.addColorStop(1, 'rgba(80,220,180,0)');
        ctx.fillStyle = g;
        ctx.fillRect(0, 0, vw, vh * 0.5);
    } else {
        const color = id === 'snow' ? 'rgba(240,248,255,0.8)'
            : id === 'rain' ? 'rgba(140,200,255,0.55)'
                : 'rgba(190,170,140,0.5)';
        ctx.strokeStyle = color;
        ctx.fillStyle = color;
        const n = id === 'dust' ? 120 : 90;
        for (let i = 0; i < n; i++) {
            const seed = (i * 9301 + 49297) % 233280;
            const rnd = seed / 233280;
            const speed = id === 'rain' ? 700 : id === 'snow' ? 60 : 240;
            const x = (rnd * vw + (id === 'dust' ? timeMs * 0.1 : 0)) % vw;
            const y = (rnd * vh + timeMs * 0.001 * speed) % vh;
            if (id === 'rain') {
                ctx.beginPath();
                ctx.moveTo(x, y);
                ctx.lineTo(x - 2, y + 10);
                ctx.stroke();
            } else {
                ctx.fillRect(x, y, id === 'dust' ? 2 : 3, id === 'dust' ? 2 : 3);
            }
        }
    }
    ctx.restore();
}
