// web/static/js/belt/belt_render.js
// Отрисовка сцены добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §9.1/§9.2), вид сверху. Слои
// снизу вверх: звёздное поле → далёкая пыль (0.4) → мелкие обломки (0.6)
// → крупные жилы (1.0) → частицы руды → луч бурения/прицел → корабль →
// эффекты («+X т»). Ассеты астероидов — подэтап 3c; здесь плейсхолдеры (фигуры).
import * as C from './belt_config.js';
import { shipDrawTransform } from '../map/ship_sprites.js';

// Кэш Image спрайтов/паттернов (арт-ТЗ §4.3, прелоад): имя → Image. Ошибка
// загрузки/отсутствие файла → null, рисующий код берёт фолбэк-многоугольник.
const spriteCache = new Map();

// getSprite — Image из кэша; null, если файл не загрузился (фолбэк §4.5).
export function getSprite(name) {
    if (!name) return null;
    const img = spriteCache.get(name);
    if (!img || !img.complete || img.naturalWidth === 0) return null;
    return img;
}

// preloadSprites — прогрев кэша до старта сцены (§4.3): без «мигания» первых
// кадров подменой многоугольников. Возвращает Promise, который резолвится,
// когда все файлы либо загружены, либо провалились (ошибка → фолбэк).
export function preloadSprites() {
    const names = C.ROCK_SPRITES.concat(C.DEBRIS_SPRITES, C.VEIN_SPRITES,
        C.ICE_SPRITES, C.ICE_DEBRIS_SPRITES, C.ICE_VEIN_SPRITES);
    const jobs = names.map((name) => new Promise((resolve) => {
        if (spriteCache.has(name)) { resolve(); return; }
        const img = new Image();
        img.onload = () => resolve();
        img.onerror = () => resolve(); // нет файла — фолбэк, сцена не падает
        img.src = C.SPRITE_BASE + name + '.png';
        spriteCache.set(name, img);
    }));
    return Promise.all(jobs);
}

// tintCache — затемнённые копии спрайтов (истощение, арт-ТЗ §4.3): тонировка
// делается в offscreen-канвасе, где есть только спрайт, — иначе source-atop
// заливает фон сцены и вокруг камня виден квадрат. Ключ "name|color|tone".
// color — цвет тонировки: тёмный (истощение железа) или холодный (лёд, 4b).
const tintCache = new Map();

function getTintedSprite(name, tone, color) {
    const img = getSprite(name);
    if (!img) return null;
    const tint = color || C.COLORS.asteroidDark;
    const key = name + '|' + tint + '|' + tone.toFixed(2);
    let canvas = tintCache.get(key);
    if (canvas) return canvas;
    canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const c = canvas.getContext('2d');
    c.drawImage(img, 0, 0);
    c.globalCompositeOperation = 'source-atop';
    c.globalAlpha = tone;
    c.fillStyle = tint;
    c.fillRect(0, 0, canvas.width, canvas.height);
    tintCache.set(key, canvas);
    return canvas;
}

function toScreen(wx, wy, cam, vw, vh) {
    return { x: (wx - cam.x) * C.WORLD_SCALE + vw / 2, y: (wy - cam.y) * C.WORLD_SCALE + vh / 2 };
}

// drawScene — полный кадр сцены. opts: { drilling, target, depleted,
// shipSprite, shipOrient, now, offBelt }.
export function drawScene(ctx, world, cam, vw, vh, opts) {
    ctx.fillStyle = C.COLORS.bg;
    ctx.fillRect(0, 0, vw, vh);

    drawStars(ctx, world.stars, cam, vw, vh, opts.now);
    drawDust(ctx, world.dust, cam, vw, vh);

    // Мелкие обломки (декор) — параллакс 0.6.
    for (const a of world.debris) {
        drawAsteroid(ctx, a, cam, vw, vh, opts, C.PARALLAX_DECOR);
    }
    // Крупные жилы — игровое поле (параллакс 1.0).
    for (const a of world.asteroids) {
        drawAsteroid(ctx, a, cam, vw, vh, opts, 1);
    }

    drawParticles(ctx, world.particles, cam, vw, vh);

    if (opts.drilling && opts.target) drawBeam(ctx, world.ship, opts.target, cam, vw, vh, opts.target.res === 'ice');
    drawAim(ctx, world.ship, cam, vw, vh);
    drawShip(ctx, world.ship, cam, vw, vh, opts);
    if (opts.offBelt) drawOffBelt(ctx, world.ship, cam, vw, vh);
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

// drawAsteroid — тело: спрайт-камень (арт-ТЗ §4.3) или фолбэк-многоугольник,
// если файл не загрузился. parallax — множитель слоя (декор 0.6, жилы 1.0).
// Наведённая цель подсвечивается контуром accent-цвета (§4.6 UI-спеки).
function drawAsteroid(ctx, a, cam, vw, vh, opts, parallax) {
    const p = toScreen(a.x - cam.x * (parallax - 1), a.y - cam.y * (parallax - 1), cam, vw, vh);
    const rad = a.r * C.WORLD_SCALE;
    if (p.x < -rad * 2 || p.x > vw + rad * 2 || p.y < -rad * 2 || p.y > vh + rad * 2) return;

    // Ресурс тела: 'ice' | 'iron' (лёд — реальный ледяной ассет, 4c).
    const isIce = a.res === 'ice';
    // Жила выработанного ресурса (гашение, §8.1) либо весь пояс выработан.
    const resDepleted = !!(a.vein && opts.depletedRes && opts.depletedRes[a.res]);
    const depleted = !!opts.depleted || resDepleted;
    const dim = a.drill; // визуальное истощение конкретной жилы
    // Реестр: жила → ICE_SPRITES/ROCK_SPRITES, обломок → ICE_DEBRIS_SPRITES/
    // DEBRIS_SPRITES. iceAsset=true, когда рисуем реальный ледяной ассет — тогда
    // плейсхолдер-тинт НЕ применяем (иначе двойная синяя заливка, §10.9).
    let rockName, iceAsset;
    if (a.vein) {
        iceAsset = isIce && C.ICE_SPRITES.length > 0;
        rockName = iceAsset ? C.ICE_SPRITES[a.sprite % C.ICE_SPRITES.length] : C.ROCK_SPRITES[a.sprite];
    } else {
        iceAsset = isIce && C.ICE_DEBRIS_SPRITES.length > 0;
        rockName = iceAsset ? C.ICE_DEBRIS_SPRITES[a.sprite % C.ICE_DEBRIS_SPRITES.length] : C.DEBRIS_SPRITES[a.sprite];
    }
    const rock = getSprite(rockName);
    // Фолбэк-лёд (нет ice_*.png): старый rock_*/debris_* + холодный тинт.
    const iceTint = isIce && !iceAsset;

    ctx.save();
    ctx.translate(p.x, p.y);
    ctx.rotate(a.rot);

    if (rock) {
        // Спрайт: масштаб = 2r / SPRITE_LONG_SIDE (визуальный размер ≈ 2r,
        // отдельного «визуального радиуса» нет — §4.5).
        const scale = (rad * 2) / C.SPRITE_LONG_SIDE;
        const size = C.SPRITE_LONG_SIDE * scale;
        // Истощение: помимо гашения руды — тёмная/обесцвеченная тонировка камня
        // (offscreen-копия, чтобы не залить фон сцены). Холодный тинт — ТОЛЬКО
        // фолбэк-льду без ассета (iceTint); реальный ледяной спрайт не тонируем.
        let tone = depleted ? 0.55 : Math.min(0.45, dim * 0.45);
        if (iceTint && !depleted) tone = Math.max(tone, 0.30);
        const tintColor = iceTint ? C.COLORS.shipAccent : C.COLORS.asteroidDark;
        const img = tone > 0.01 ? (getTintedSprite(rockName, tone, tintColor) || rock) : rock;
        ctx.drawImage(img, -size / 2, -size / 2, size, size);
    } else {
        // Фолбэк-многоугольник (инвариант §4.5): нет файла/ошибка загрузки.
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
        ctx.fillStyle = depleted ? C.COLORS.asteroidDark
            : (isIce ? C.COLORS.iceRock : (a.vein ? C.COLORS.asteroidVein : C.COLORS.asteroid));
        if (dim > 0 && !depleted) ctx.globalAlpha = Math.max(0.55, 1 - dim * 0.4);
        else if (isIce && !depleted) ctx.globalAlpha = 0.9;
        ctx.fill();
        ctx.globalAlpha = 1;
        ctx.lineWidth = 2;
        ctx.strokeStyle = isIce ? C.COLORS.iceGlint : C.COLORS.asteroidEdge;
        ctx.stroke();
    }
    ctx.restore();

    // Слой руды (арт-ТЗ §4.3): аддитивно поверх камня, поворот a.rot+veinRot,
    // alpha ∝ veinRich·(1−drill); при выработанном поясе не рисуется.
    if (a.vein && !depleted) {
        // Ледяной блеск — из ледяного набора (холодный), иначе каменный (§10.9).
        const veinName = (isIce && C.ICE_VEIN_SPRITES.length)
            ? C.ICE_VEIN_SPRITES[a.veinPattern % C.ICE_VEIN_SPRITES.length]
            : C.VEIN_SPRITES[a.veinPattern];
        const vein = getSprite(veinName);
        const veinA = a.veinRich * Math.max(0, 1 - dim);
        if (vein && veinA > 0.01) {
            const vs = (rad * 2) / C.SPRITE_LONG_SIDE * a.veinScale;
            const vSize = C.SPRITE_LONG_SIDE * vs;
            ctx.save();
            ctx.translate(p.x, p.y);
            ctx.rotate(a.rot + a.veinRot);
            ctx.globalCompositeOperation = 'lighter';
            ctx.globalAlpha = veinA;
            ctx.drawImage(vein, -vSize / 2, -vSize / 2, vSize, vSize);
            ctx.restore();
        }
    }

    // rim-light (арт-ТЗ §4.3): радиальный градиент в ЭКРАННОМ пространстве,
    // постоянное направление (сторона звезды), не вращается с телом. Свет
    // обрезается по альфе спрайта (offscreen + destination-in) — иначе вокруг
    // камня виден мягкий ореол поверх фона. Фолбэк-многоугольник — clip по
    // его контуру (свет только на теле).
    const lx = opts.lightX || 0;
    const ly = opts.lightY || 0;
    if (rock) {
        const scale = (rad * 2) / C.SPRITE_LONG_SIDE;
        const size = C.SPRITE_LONG_SIDE * scale;
        const buf = document.createElement('canvas');
        buf.width = Math.max(1, Math.ceil(size));
        buf.height = Math.max(1, Math.ceil(size));
        const bctx = buf.getContext('2d');
        const cx = buf.width / 2;
        const cy = buf.height / 2;
        const g = bctx.createRadialGradient(
            cx + lx * rad, cy + ly * rad, rad * 0.15,
            cx, cy, rad * 1.05);
        g.addColorStop(0, 'rgba(200,210,230,0.28)');
        g.addColorStop(0.55, 'rgba(160,175,205,0.10)');
        g.addColorStop(1, 'rgba(120,130,160,0)');
        bctx.fillStyle = g;
        bctx.fillRect(0, 0, buf.width, buf.height);
        // Обрезка по альфе спрайта: свет остаётся только на камне.
        bctx.globalCompositeOperation = 'destination-in';
        bctx.drawImage(rock, 0, 0, buf.width, buf.height);
        bctx.globalCompositeOperation = 'source-over';
        ctx.save();
        ctx.globalCompositeOperation = 'lighter';
        ctx.drawImage(buf, -size / 2, -size / 2, size, size);
        ctx.restore();
    } else {
        // Фолбэк: свет по контуру многоугольника (без ореола на фоне).
        ctx.save();
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
        ctx.clip();
        const g = ctx.createRadialGradient(
            lx * rad, ly * rad, rad * 0.15,
            0, 0, rad * 1.05);
        g.addColorStop(0, 'rgba(200,210,230,0.28)');
        g.addColorStop(0.55, 'rgba(160,175,205,0.10)');
        g.addColorStop(1, 'rgba(120,130,160,0)');
        ctx.globalCompositeOperation = 'lighter';
        ctx.fillStyle = g;
        ctx.fillRect(-rad * 1.1, -rad * 1.1, rad * 2.2, rad * 2.2);
        ctx.restore();
    }

    // Блеск руды на жилах — искры поверх руды, гаснут при выработке/истощении.
    if (a.vein && !depleted) {
        const glintA = Math.max(0, 1 - dim);
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(a.rot);
        for (const g of a.glints) {
            ctx.globalAlpha = glintA * (0.5 + 0.5 * Math.sin(opts.now * 0.002 + g.x));
            ctx.fillStyle = isIce ? C.COLORS.iceGlint : C.COLORS.glint;
            ctx.beginPath();
            ctx.arc(g.x, g.y, g.r, 0, Math.PI * 2);
            ctx.fill();
        }
        ctx.restore();
        ctx.globalAlpha = 1;
    }

    // Подсветка наведённой жилы (§4.6): контур цели — сильнее, чем у прочих.
    if (opts.target === a && !depleted) {
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.globalAlpha = 0.9;
        ctx.lineWidth = 2.5;
        ctx.strokeStyle = isIce ? C.COLORS.iceGlint : C.COLORS.glint;
        ctx.beginPath();
        ctx.arc(0, 0, rad, 0, Math.PI * 2);
        ctx.stroke();
        ctx.restore();
        ctx.globalAlpha = 1;
    }
}

function drawParticles(ctx, parts, cam, vw, vh) {
    for (const p of parts) {
        const s = toScreen(p.x, p.y, cam, vw, vh);
        ctx.globalAlpha = Math.max(0, p.life / p.max);
        ctx.fillStyle = p.c || C.COLORS.particle;
        ctx.fillRect(s.x - p.r / 2, s.y - p.r / 2, p.r, p.r);
    }
    ctx.globalAlpha = 1;
}

function drawBeam(ctx, ship, target, cam, vw, vh, isIce) {
    const a = toScreen(ship.x, ship.y, cam, vw, vh);
    const b = toScreen(target.x, target.y, cam, vw, vh);
    ctx.save();
    ctx.strokeStyle = isIce ? C.COLORS.iceGlint : C.COLORS.beam;
    ctx.globalAlpha = 0.55 + 0.25 * Math.sin(Date.now() * 0.02);
    ctx.lineWidth = 2;
    ctx.setLineDash([6, 6]);
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();
    ctx.restore();
}

// drawAim — лёгкий луч прицела от носа по heading (§4.6): показывает, куда
// смотрит нос при плавном довороте. Новых цветов не вводим (accent).
function drawAim(ctx, ship, cam, vw, vh) {
    const p = toScreen(ship.x, ship.y, cam, vw, vh);
    const len = 260;
    ctx.save();
    ctx.globalAlpha = 0.18;
    ctx.strokeStyle = C.COLORS.glint;
    ctx.lineWidth = 1.5;
    ctx.setLineDash([4, 8]);
    ctx.beginPath();
    ctx.moveTo(p.x, p.y);
    ctx.lineTo(p.x + Math.cos(ship.heading) * len, p.y + Math.sin(ship.heading) * len);
    ctx.stroke();
    ctx.restore();
}

// drawOffBelt — индикатор «пояс позади» (§4.6, опция): когда игрок ушёл
// поперёк за полосу, тонкая стрелка показывает сторону пояса (к оси Y).
function drawOffBelt(ctx, ship, cam, vw, vh) {
    const p = toScreen(ship.x, ship.y, cam, vw, vh);
    const dir = ship.y > 0 ? -1 : 1; // к оси пояса
    ctx.save();
    ctx.translate(p.x, p.y);
    ctx.globalAlpha = 0.5;
    ctx.strokeStyle = C.COLORS.shipAccent;
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(0, dir * 54);
    ctx.lineTo(-9, dir * 36);
    ctx.moveTo(0, dir * 54);
    ctx.lineTo(9, dir * 36);
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
