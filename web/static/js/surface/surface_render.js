// web/static/js/surface/surface_render.js
// Отрисовка прогулки (спека 2026-09-21 §7.2): параллакс-небо (только из sky
// пакета), дальний рельеф, основной рельеф/пещеры (чанки кэшируются), декор,
// жизнь, игрок, HUD (в surface_ui.js). Canvas 2D. Погода — surface_weather.js.
import { CHUNK, CHUNK_RADIUS, COLORS, FLOAT_SPAN, FLOAT_GAP, ZOOM, SHIP_DECOR_SIZE, SHIP_DECOR_X, SHIP_HOVER_BOTTOM, SHIP_BOB_AMP, SHIP_BOB_PERIOD_MS } from './surface_config.js';
import { shade, rgba, horizonHeight } from './surface_world.js';
import { drawDecorPrim } from './surface_decor.js';
import { planetTexture } from './surface_net.js';
import { recolorShipSprite, shipDrawTransform } from '../map/ship_sprites.js';

// 700 → 1200 (Э3, 2026-09-23): рецепт гор (scale 0.55, crest+spike+step+fan)
// поднимает профиль до ~550 px над рест-линией, а полоса парящей породы
// (FLOAT_SPAN) требует ещё 260 px над рельефом — при 700 земля уходила за верх
// канваса (твёрдая, но не нарисованная, §6 п.6). Рост ВЫСОТЫ канваса не нужен:
// память = (CHUNK+1)·CHUNK_HEIGHT·SS²·4 — от margin НЕ зависит, меняется лишь
// topY. Низ растра при этом опускается до baseY − margin + CHUNK_HEIGHT = 800;
// максимум рельефа всех биомов (fallback + рецепты) = 544 — запас 256 px.
export const CHUNK_TOP_MARGIN = 1200;
export const CHUNK_HEIGHT = 1700;

// Глубинный градиент (объём) привязан к РЕСТ-линии мира (baseY − DEPTH_TOP_MARGIN),
// а не к верху растра: рост CHUNK_TOP_MARGIN вверх не должен менять затемнение
// мира (иначе весь кадр уходит в тень). Мировой диапазон [-400, 1300] — как до Э3;
// общий проход и запечённый в чанк градиент считаются от одного якоря и совпадают.
const DEPTH_TOP_MARGIN = 700;

// Суперсэмплинг растра чанка под зум (идея 2026-09-22 §8.2): рендер в
// повышенном разрешении, отрисовка — в логическом размере. Иначе 1:1-растр
// растягивается зумом (×ZOOM·DPR) и рельеф блочный. Множитель ЦЕЛЫЙ (≈ZOOM·DPR):
// при дробном соседние 1-px колонки дают AA-стыки (source-over не суммирует
// полупрозрачные кромки до 1) — по рельефу проступает частая сетка. Потолок
// CHUNK_SS_MAX — память растёт как SS² (компромисс память↔резкость, см. отчёт).
const CHUNK_SS_MAX = 2;

function chunkSupersample() {
    const dpr = (typeof window !== 'undefined' && window.devicePixelRatio) || 1;
    return Math.max(1, Math.min(CHUNK_SS_MAX, Math.round(ZOOM * dpr)));
}

// Полоса парящей породы (float-формации): шаг сканирования по y (px) и сдвиг
// проб внутрь от границ полосы. Границы (FLOAT_GAP/FLOAT_SPAN) обрезают твёрдую
// область, давая у кромок субпиксельные твёрдые полоски: у самой границы
// твёрдых сэмплов сетки нет, хотя физика там твёрда. Проба, смещённая на
// FLOAT_EPS внутрь, их ловит. Растр должен быть надмножеством твёрдой области
// (идея 2026-09-21 §2.1) — см. fillFloatRun.
const FLOAT_STEP_Y = 1;
const FLOAT_EPS = 0.001;
const FLOAT_BLEED = 1;

// fillFloatRun — закраска одной твёрдой полосы колонки. По x — запас ±1 px
// (твёрдая точка с дробным x покрывается и соседней колонкой), по y — запас
// FLOAT_BLEED (перекрывает дискретизацию шага FLOAT_STEP_Y). Так растр —
// надмножество твёрдой области: «что твёрдо, то видно» (идея 2026-09-21 §2.1).
function fillFloatRun(ctx, lx, yTop, yBottom, topY) {
    const y0 = Math.max(0, Math.floor(yTop - topY) - FLOAT_BLEED);
    const y1 = Math.min(CHUNK_HEIGHT - 1, Math.floor(yBottom - topY) + FLOAT_BLEED);
    if (y1 < y0) return;
    ctx.fillRect(lx - 1, y0, 3, y1 - y0 + 1);
}

// Снеговая линия (§4.3): локальный максимум профиля в окне SNOW_W, линия на
// `snowLine` px НИЖЕ него; на теневой стороне линия ниже (больше снега).
// `snowLine` — число рецепта, а числа рецепта дорелейные (их умножает
// relief.scale, как `amp` слоёв) → линия масштабируется тем же множителем,
// иначе при смене `scale` снег накрывал бы весь склон. Абсолютной шкалы высот
// в прогулке нет — линия относительная (осознанное отклонение §11). Окно
// пересекает границы чанков: viewHeight — чистая функция мира, поэтому расчёт
// корректен и в запечённом канвасе чанка.
const SNOW_W = 700;   // полуокно локального максимума (px)
const SNOW_CS = 24;   // шаг грубой выборки профиля (px)

function drawSnowBand(ctx, world, baseX, topY) {
    const line = world.snowLine * world.reliefScale;
    if (!line) return;
    const lo = baseX - SNOW_W;
    const n = Math.ceil((CHUNK + 2 * SNOW_W) / SNOW_CS) + 2;
    const prof = new Array(n);
    for (let j = 0; j < n; j++) prof[j] = world.viewHeight(lo + j * SNOW_CS);
    const k = Math.ceil(SNOW_W / SNOW_CS);
    ctx.fillStyle = world.palette.snow;
    for (let lx = 0; lx <= CHUNK; lx++) {
        const wx = baseX + lx;
        const j0 = Math.round((wx - lo) / SNOW_CS);
        let m = prof[j0];
        for (let j = Math.max(0, j0 - k); j <= Math.min(n - 1, j0 + k); j++) if (prof[j] < m) m = prof[j];
        const y = world.surfaceY(wx);
        // Теневая сторона — склон, поднимающийся вправо (условная ориентация: оси
        // солнца в прогулке нет). §11: «южный склон поднимает снег на 300–800 м» →
        // на освещённом склоне линия выше (снега меньше), на теневом — ниже
        // (снега больше): px-ниже-максимума делится на snowLineShadow (§4.3).
        const shadow = prof[Math.min(n - 1, j0 + 1)] < prof[Math.max(0, j0 - 1)];
        const lineEff = shadow ? line / Math.max(0.2, world.snowLineShadow) : line;
        const top = m + lineEff;          // y растёт вниз: линия ниже максимума
        if (y > top) continue;            // ниже линии — снега нет
        const depth = Math.min(60, 4 + (top - y) * 0.5);
        ctx.fillRect(lx, Math.floor(y - topY), 1, depth);
    }
}

// getChunkCanvas — лениво отрисованный чанк (кэш). Рельеф + пещеры.
export function getChunkCanvas(world, index) {
    const ss = chunkSupersample();
    // Смена DPR/зума → растр чанка в прежнем разрешении невалиден: сброс кэша.
    if (!world._chunkCache || world._chunkSS !== ss) {
        world._chunkCache = new Map();
        world._chunkSS = ss;
    }
    const cached = world._chunkCache.get(index);
    if (cached) return cached;

    const canvas = document.createElement('canvas');
    // +1 колонка (CHUNK..CHUNK+1): реальные данные следующей мировой колонки —
    // перекрывает 1-px шов на стыке чанков при апскейле зума (билинейная выборка
    // края канваса иначе даёт полупрозрачную полосу). Канвас несёт ТОЛЬКО
    // непрозрачное (рельеф/пещеры/парящая порода), а глубинный градиент и светлая
    // кромка поверхности вынесены в общий проход drawTerrain: полупрозрачный
    // градиент внутри канваса на стыке чанков композитился дважды и давал тёмную
    // вертикальную полосу каждые CHUNK·ZOOM px. Растр — в SS раз больше
    // логического размера; рисование идёт в логических координатах.
    canvas.width = Math.round((CHUNK + 1) * ss);
    canvas.height = Math.round(CHUNK_HEIGHT * ss);
    const ctx = canvas.getContext('2d');
    // Точные коэффициенты (с учётом округления) — растр покрыт целиком, без
    // прозрачной кромки (иначе вернулся бы шов).
    ctx.scale(canvas.width / (CHUNK + 1), canvas.height / CHUNK_HEIGHT);
    const topY = world.baseY - CHUNK_TOP_MARGIN;
    const baseX = index * CHUNK;
    // Цвет земли: с рецептом — палитра биома (§3.4, base — «земля»), иначе
    // прежний затемнённый biome_color (фолбэк 1:1).
    const rock = world.hasView ? world.palette.base : shade(world.color, 0.62);

    ctx.fillStyle = rock;
    for (let lx = 0; lx <= CHUNK; lx++) {
        const wx = baseX + lx;
        // Верх отрисовки — surfaceY (физический профиль ∪ viewOnly-рябь, §3.1/§6 п.4):
        // растр — надмножество твёрдой области, физика по terrainHeight.
        const th = world.surfaceY(wx);
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

    // Парящие камни/арки (float-формации): красим там, где физика (isSolid)
    // считает породу твёрдой (идея 2026-09-21 §2.1). Растр — НАДМНОЖЕСТВО
    // твёрдой области: шаг по x — 1 px (прежние 3 px блоками 3×3 оставляли
    // непокрашенной левую кромку твёрдой области до 3 px — игрок упирался в
    // невидимую стену), по y — FLOAT_STEP_Y с запасом FLOAT_BLEED. Высота
    // полосы (FLOAT_GAP..FLOAT_SPAN) — те же константы, что у физики.
    ctx.fillStyle = rock;
    for (let lx = 0; lx <= CHUNK; lx++) {
        const wx = baseX + lx;
        const th = world.terrainHeight(wx);
        // Сэмплы полосы сверху вниз: сетка FLOAT_STEP_Y + проба у нижней
        // границы (сетка туда не попадает из-за сдвига FLOAT_EPS).
        const ys = [];
        for (let wy = th - FLOAT_GAP - FLOAT_EPS; wy >= th - FLOAT_SPAN + FLOAT_EPS; wy -= FLOAT_STEP_Y) ys.push(wy);
        ys.push(th - FLOAT_SPAN + FLOAT_EPS);
        let yTop = null, yBottom = null;
        for (const wy of ys) {
            if (world.isSolid(wx, wy)) {
                if (yTop === null) yBottom = wy;
                yTop = wy;
            } else if (yTop !== null) {
                fillFloatRun(ctx, lx, yTop, yBottom, topY);
                yTop = null;
            }
        }
        if (yTop !== null) fillFloatRun(ctx, lx, yTop, yBottom, topY);
    }

    // Глубинный градиент (объём) — source-atop: ложится ТОЛЬКО на уже нарисованное
    // (рельеф/пещеры/парящая порода), небо остаётся прозрачным. Так канвас чанка
    // непрозрачен лишь под рельефом → перекрытие на стыке не даёт двойного
    // композита полупрозрачного слоя (тёмных полос). Небо/дальний план тем же
    // мировым градиентом темнит общий проход drawTerrain (до блитов).
    const gradTop = (world.baseY - DEPTH_TOP_MARGIN) - topY;
    const grad = ctx.createLinearGradient(0, gradTop, 0, gradTop + CHUNK_HEIGHT);
    grad.addColorStop(0, 'rgba(0,0,0,0)');
    grad.addColorStop(1, 'rgba(0,0,0,0.72)');
    ctx.globalCompositeOperation = 'source-atop';
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, CHUNK + 1, CHUNK_HEIGHT);
    ctx.globalCompositeOperation = 'source-over';

    // Кромка поверхности — светлее (палитра: light; фолбэк — shade). Непрозрачна,
    // печётся в канвас чанка (в кэш): каждый кадр её рисовать не нужно.
    ctx.fillStyle = world.hasView ? world.palette.light : shade(world.color, 1.15);
    for (let lx = 0; lx <= CHUNK; lx++) {
        const wx = baseX + lx;
        const ly = Math.floor(world.surfaceY(wx) - topY);
        if (ly >= 0 && ly < CHUNK_HEIGHT) ctx.fillRect(lx, ly, 1, 2);
    }

    // Снеговые шапки (горы, §4.3) — поверх кромки, в тот же запечённый канвас.
    drawSnowBand(ctx, world, baseX, topY);

    const result = { canvas, topY };
    world._chunkCache.set(index, result);
    return result;
}

// Текстуры тел неба (идея 2026-09-22 §8.4): planet_id → HTMLImageElement.
// Грузится один раз на id (авторизованный /api/planet-image, surface_net.js);
// пока грузится/при ошибке (401, нет картинки) — фолбэк-диск. Спутники и
// компаньоны id не имеют — всегда диск.
const skyTextures = new Map();

function skyBodyTexture(planetId) {
    if (!planetId) return null;
    if (skyTextures.has(planetId)) return skyTextures.get(planetId);
    skyTextures.set(planetId, null);
    planetTexture(planetId).then((img) => { if (img) skyTextures.set(planetId, img); });
    return null;
}

function rgbCss(c) { return `rgb(${c.r},${c.g},${c.b})`; }

// drawSun — светило с приглушённым ореолом (идея §8.4). Без env alpha = 1 и
// прежние параметры (ореол ×1.9; §6.2 направления):
function drawSun(ctx, x, y, r, color, alpha, haloMul, haloAlpha) {
    ctx.save();
    ctx.globalAlpha = Math.max(0, Math.min(1, alpha));
    const halo = ctx.createRadialGradient(x, y, r * 0.2, x, y, r * haloMul);
    halo.addColorStop(0, color);
    halo.addColorStop(0.4, rgba(color, haloAlpha));
    halo.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(x, y, r * haloMul, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(x, y, r, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// drawSky — параллакс-небо из пакета (§7.1, В2): светило + тела системы.
// env (необязательный, §6.3 п.4 спеки): палитра неба по таймлайну суток и дуга
// светила. Без env — прежний вид (обратная совместимость, T9).
export function drawSky(ctx, vw, vh, sky, camera, timeMs, env) {
    const sc = env ? env.skyColors() : null;
    const grad = ctx.createLinearGradient(0, 0, 0, vh);
    grad.addColorStop(0, sc ? rgbCss(sc.top) : COLORS.skyTop);
    grad.addColorStop(1, sc ? rgbCss(sc.bottom) : COLORS.skyBottom);
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, vw, vh);

    if (!sky) return;
    const star = sky.star || {};
    const starColor = star.color || '#ffd700';
    if (env) {
        // Дуга светила (§6.2): вне окна — за горизонтом (null); alpha 1 днём / 0 в ночи.
        const pose = env.sunPose(vw, vh, camera);
        if (pose && pose.alpha > 0.001) drawSun(ctx, pose.x, pose.y, pose.r, starColor, pose.alpha, pose.haloMul, pose.haloAlpha);
    } else {
        // Без env — прежний вид: светило в vw·0.72/vh·0.20, параллакс 0.05.
        drawSun(ctx, vw * 0.72 - camera.x * 0.05, vh * 0.20 - camera.y * 0.03,
            Math.max(16, vh * 0.04), starColor, 1, 1.9, 0.16);
    }

    // Тела системы: параллакс 0.10–0.26, высота height из пакета. Мельче и
    // тусклее прежнего, разнесены по высоте (идея §8.4 — «не навязчивые»).
    (sky.bodies || []).forEach((b, i) => {
        const depth = 0.10 + 0.16 * (i / Math.max(1, sky.bodies.length - 1 || 1));
        const bx = vw * 0.5 + (i - 1) * vw * 0.22 - camera.x * depth;
        const by = vh * (0.06 + 0.6 * (b.height || 0.3)) - camera.y * 0.04;
        const size = Math.max(4, Math.min(vh * 0.06, vh * 0.03 * (b.size_hint || 0.3) * 2.4));
        ctx.save();
        ctx.globalAlpha = 0.55;
        const img = b.planet_id ? skyBodyTexture(b.planet_id) : null;
        if (img) {
            // Реальная текстура планеты, вписана в круг.
            ctx.beginPath();
            ctx.arc(bx, by, size, 0, Math.PI * 2);
            ctx.clip();
            ctx.drawImage(img, bx - size, by - size, size * 2, size * 2);
        } else {
            const g = ctx.createRadialGradient(bx - size * 0.3, by - size * 0.3, size * 0.1, bx, by, size);
            g.addColorStop(0, '#ffffff');
            g.addColorStop(0.25, b.color || '#8a7a6a');
            g.addColorStop(1, shade(b.color || '#8a7a6a', 0.35));
            ctx.fillStyle = g;
            ctx.beginPath();
            ctx.arc(bx, by, size, 0, Math.PI * 2);
            ctx.fill();
        }
        ctx.restore();
    });
}

// FAR_STEP — фиксированный шаг выборки дальнего плана в мировых координатах
// (не по экрану): силуэт считается на одной и той же мировой решётке, поэтому
// при сдвиге камеры уезжает цельно, а не перерисовывается каждый кадр (§2.2).
export const FAR_STEP = 24;

// farReliefProfile — точки дальнего силуэта: мировая решётка FAR_STEP в
// «дальнем» мире (параллакс 0.35), экранная координата выводится из камеры.
export function farReliefProfile(world, camera, vw) {
    const camFar = camera.x * 0.35;
    const half = vw / 2;
    const start = Math.floor((camFar - half) / FAR_STEP) * FAR_STEP;
    const end = camFar + half;
    const pts = [];
    for (let wx = start; wx <= end; wx += FAR_STEP) {
        pts.push({ wx, sx: wx - camFar + half, y: world.farHeight(wx) });
    }
    return pts;
}

// drawFarRelief — дальний силуэт рельефа (параллакс 0.35).
export function drawFarRelief(ctx, world, camera, vw, vh) {
    const pts = farReliefProfile(world, camera, vw);
    if (!pts.length) return;
    const yOff = -camera.y * 0.35 + vh * 0.35;
    ctx.fillStyle = 'rgba(8,12,22,0.85)';
    ctx.beginPath();
    ctx.moveTo(0, vh);
    ctx.lineTo(0, pts[0].y + yOff);
    for (const p of pts) ctx.lineTo(p.sx, p.y + yOff);
    ctx.lineTo(vw, pts[pts.length - 1].y + yOff);
    ctx.lineTo(vw, vh);
    ctx.closePath();
    ctx.fill();
}

// horizonFill — цвет заливки яруса из палитры биома (§3.6 правило 2): fill —
// ключ палитры (dark/base/…); неизвестный/пустой — dark, затем base.
function horizonFill(palette, key) {
    const k = (typeof key === 'string' && key) ? key : 'dark';
    return palette[k] || palette.dark || palette.base;
}

// horizonProfile — точки силуэта одного яруса (§3.6): фиксированная мировая
// решётка step (не по экрану — силуэт «уезжает цельно», §2.2), параллакс p,
// низкочастотный профиль примитива. Экранная координата непрерывна от камеры.
export function horizonProfile(world, camera, vw, vh, layer) {
    const p = Math.max(0.36, Math.min(0.99, Number(layer.parallax) || 0.5));
    const step = Math.max(4, Number(layer.step) || FAR_STEP);
    const prof = layer.profile || {};
    const camP = camera.x * p;
    const half = vw / 2;
    const start = Math.floor((camP - half) / step) * step;
    const end = camP + half;
    // Базисная линия: дальний пояс (меньший параллакс) выше, ближний — ниже.
    // Смещение подобрано так, чтобы пояс читался над линией земли (у спавна она
    // на ~vh·0.6): дальний ~0.45vh, ближний ~0.51vh.
    const baseY = world.baseY - 230 + (p - 0.5) * 470;
    const yOff = -camera.y * p + vh * 0.5;
    const pts = [];
    for (let wx = start; wx <= end; wx += step) {
        pts.push({ wx, sx: wx - camP + half, y: baseY + horizonHeight(prof.prim, prof, wx, world.seed) + yOff });
    }
    return pts;
}

// drawHorizon — пояса параллакса между farRelief и drawEnvironmentMid (§3.6,
// §6 п.1). Ярусы декоративны: физика (terrainHeight/isSolid) их не знает (§6
// п.3). Заливка — из палитры биома (fill: dark/base), дымка смешивает силуэт с
// текущим небом — согласована с суточным циклом (ночью ярус не светлее неба,
// §3.6 правило 3). Нет рецепта / нет horizon — не рисуется (фолбэк 1:1).
export function drawHorizon(ctx, world, camera, vw, vh, env) {
    const layers = world.horizon;
    if (!layers || !layers.length) return;
    const sc = (env && typeof env.skyColors === 'function') ? env.skyColors() : null;
    const hazeCss = (a) => sc
        ? `rgba(${sc.bottom.r},${sc.bottom.g},${sc.bottom.b},${a})`
        : rgba(world.palette.base, a);
    for (const layer of layers) {
        const pts = horizonProfile(world, camera, vw, vh, layer);
        if (pts.length < 2) continue;
        const haze = Math.max(0, Math.min(1, Number(layer.haze) || 0));
        ctx.beginPath();
        ctx.moveTo(0, vh);
        ctx.lineTo(0, pts[0].y);
        for (const pt of pts) ctx.lineTo(pt.sx, pt.y);
        ctx.lineTo(vw, pts[pts.length - 1].y);
        ctx.lineTo(vw, vh);
        ctx.closePath();
        ctx.fillStyle = horizonFill(world.palette, layer.fill);
        ctx.fill();
        if (haze > 0) { ctx.fillStyle = hazeCss(haze); ctx.fill(); }
    }
}

// viewChunkRadius — сколько чанков влево/вправо покрывает экран при масштабе
// ZOOM: видимая ширина мира = vw / ZOOM. Не константа CHUNK_RADIUS (идея
// 2026-09-22 §8.3): иначе на широких экранах за краями чанков земли нет, а
// декор есть — деревья висят в пустоте. CHUNK_RADIUS остаётся нижней границей
// (в памяти ±3 чанка, §7.2).
function viewChunkRadius(vw) {
    return Math.max(CHUNK_RADIUS, Math.ceil(vw / (2 * ZOOM) / CHUNK) + 1);
}

// drawDepthGradient — глубинный градиент (объём) для неба/дальнего плана/ярусов:
// ОДИН проход на весь экран, ДО блитов чанков (рельеф перекроет его и получит
// затемнение из канваса чанка, source-atop). Градиент мировой (от topY до
// topY+CHUNK_HEIGHT) — общий проход и запечённый в чанк профиль совпадают, стык
// «небо↔рельеф» непрерывен. Раньше слой рисовался внутри канваса каждого чанка и
// на стыке (перекрытие CHUNK+1) композитился дважды — тёмная вертикальная полоса
// каждые CHUNK·ZOOM px.
function drawDepthGradient(ctx, world, camera, vw, vh) {
    const topY = world.baseY - DEPTH_TOP_MARGIN;
    const y0 = topY - camera.y + vh / 2;
    const grad = ctx.createLinearGradient(0, y0, 0, y0 + CHUNK_HEIGHT);
    grad.addColorStop(0, 'rgba(0,0,0,0)');
    grad.addColorStop(1, 'rgba(0,0,0,0.72)');
    ctx.fillStyle = grad;
    ctx.fillRect(0, y0, vw, CHUNK_HEIGHT);
}

// drawTerrain — основной рельеф из кэшированных чанков. Глубинный градиент —
// один общий проход ДО блитов (темнит небо/дальний план/ярусы/погоду, нарисованные
// раньше); рельеф тем же мировым градиентом запечён в канвас чанка (source-atop).
// Кромка поверхности тоже запечена в чанк. Полос на стыках нет: канвас чанка
// непрозрачен только под рельефом, перекрытие CHUNK+1 безопасно.
export function drawTerrain(ctx, world, camera, vw, vh) {
    drawDepthGradient(ctx, world, camera, vw, vh);
    const centerChunk = Math.floor(camera.x / CHUNK);
    const radius = viewChunkRadius(vw);
    for (let i = centerChunk - radius; i <= centerChunk + radius; i++) {
        const { canvas, topY } = getChunkCanvas(world, i);
        const sx = i * CHUNK - camera.x + vw / 2;
        const sy = topY - camera.y + vh / 2;
        // Растр чанка суперсэмплен — рисуем в логическом размере: апскейла нет.
        ctx.drawImage(canvas, sx, sy, CHUNK + 1, CHUNK_HEIGHT);
    }
}

// visibleDecor — декор кадра перебором МИРОВЫХ колонок (идея 2026-09-21 §2.2):
// набор и позиции зависят только от мира и камеры, не от субпиксельной фазы.
// Шаг — 1 мировая колонка (`decorAt` — дешёвый хеш), запас по краям — под
// крону/стебли у границы кадра.
export function visibleDecor(world, camera, vw) {
    const left = camera.x - vw / 2;
    const margin = 8;
    const out = [];
    for (let wx = Math.floor(left) - margin; wx <= Math.ceil(camera.x + vw / 2) + margin; wx++) {
        const d = world.decorAt(wx);
        if (!d) continue;
        out.push({ wx, x: wx - camera.x + vw / 2, ...d });
    }
    return out;
}

// drawDecor — растительность/лишайники/камни + редкие находки.
export function drawDecor(ctx, world, camera, vw, vh) {
    for (const d of visibleDecor(world, camera, vw)) {
        const x = d.x;
        const gy = world.terrainHeight(d.wx) - camera.y + vh / 2;
        // Рецепт вида (§3.2): отрисовка по примитиву.
        if (d.prim) { drawDecorPrim(ctx, world, d, x, gy); continue; }
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
    const left = camera.x - vw / 2;
    const start = Math.floor(left / 200) * 200;
    for (let wx = start; wx <= left + vw + 200; wx += 200) {
        const r = world.rareDecorAt(wx);
        if (!r) continue;
        const x = r.x - camera.x + vw / 2;
        const gy = world.terrainHeight(r.x) - camera.y + vh / 2;
        // Примета биома по рецепту (landmark.prim, §3.3) — библиотека декора.
        if (r.prim) { drawDecorPrim(ctx, world, { prim: r.prim, h: 20 }, x, gy); continue; }
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

// drawShip — корабль игрока парит над точкой высадки (ЧК-ship, идея
// 2026-09-23 §5; решение создателя 2026-09-23 — «подвесить его… без лестницы»):
// статичная декорация (не интерактивна, ничего не персистит). Низ спрайта — на
// SHIP_HOVER_BOTTOM над землёй, лёгкое вертикальное покачивание от времени
// кадра (физику игрока и ввод не трогает). Поза — каноническая пара
// (angle, flip) из пакета (на прогулке /me не зовём): курса нет →
// shipDrawTransform(0, orient). Спрайт перекрашивается recolorShipSprite (кэш);
// фолбэк И4: не загружен/имя неизвестно → null, ничего не рисуем (отрисовка не
// ломается). Рисуется до игрока — тот поверх.
export function drawShip(ctx, world, camera, vw, vh, pkg, timeMs) {
    if (!pkg || !pkg.ship_icon) return;
    const sprite = recolorShipSprite(pkg.ship_icon, pkg.ship_color);
    if (!sprite) return;
    const wx = SHIP_DECOR_X;
    const x = wx - camera.x + vw / 2;
    const gy = world.terrainHeight(wx) - camera.y + vh / 2;
    const bob = Math.sin((timeMs || 0) * 2 * Math.PI / SHIP_BOB_PERIOD_MS) * SHIP_BOB_AMP;
    const orient = { angle: Number(pkg.ship_angle) || 0, flip: !!pkg.ship_flip };
    const t = shipDrawTransform(0, orient);
    ctx.save();
    // Центр спрайта = низ над землёй (SHIP_HOVER_BOTTOM) + половина размера + покачивание.
    ctx.translate(x, gy - SHIP_HOVER_BOTTOM - SHIP_DECOR_SIZE / 2 + bob);
    ctx.rotate(t.rotate);
    ctx.scale(t.scaleX, t.scaleY);
    ctx.drawImage(sprite, -SHIP_DECOR_SIZE / 2, -SHIP_DECOR_SIZE / 2, SHIP_DECOR_SIZE, SHIP_DECOR_SIZE);
    ctx.restore();
}

// drawCreatures — животные (не бой, §7.3): пасётся/убегает/подходит/стайка/детёныш.
export function drawCreatures(ctx, world, camera, vw, vh, player, timeMs) {
    const centerChunk = Math.floor(camera.x / CHUNK);
    const radius = viewChunkRadius(vw);
    for (let i = centerChunk - radius; i <= centerChunk + radius; i++) {
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
