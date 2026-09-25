// web/static/js/surface/surface_render.js
// Отрисовка прогулки (спека 2026-09-21 §7.2): параллакс-небо (только из sky
// пакета), дальний рельеф, основной рельеф/пещеры (чанки кэшируются), декор,
// жизнь, игрок, HUD (в surface_ui.js). Canvas 2D. Погода — surface_weather.js.
import { CHUNK, CHUNK_RADIUS, CHUNK_TOP_MARGIN, CHUNK_HEIGHT, COLORS, FLOAT_SPAN, FLOAT_GAP, ZOOM, PLAYER_H, SHIP_DECOR_SIZE, SHIP_DECOR_X, SHIP_HOVER_BOTTOM, SHIP_BOB_AMP, SHIP_BOB_PERIOD_MS, LIQUID_DEFAULTS } from './surface_config.js';
import { shade, shadeHex, parseHex, rgba, horizonHeight, liquidWave } from './surface_world.js';
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
// Геометрия растра — единый источник в `surface_config.js` (skyTop и растр чанка
// читают одни числа, §2.1/§2.3). Реэкспорт сохраняет прежних потребителей
// (tools/e2e/surface-*.js импортируют CHUNK_TOP_MARGIN/CHUNK_HEIGHT отсюда).
export { CHUNK_TOP_MARGIN, CHUNK_HEIGHT };

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

// mixHex — линейная смесь двух hex-цветов (t=0 → a, t=1 → b).
function mixHex(a, b, t) {
    const ca = parseHex(a), cb = parseHex(b);
    const f = (x, y) => Math.round(x + (y - x) * t);
    const to = (v) => f(v).toString(16).padStart(2, '0');
    return '#' + to(ca.r) + to(cb.r) + to(ca.g) + to(cb.g) + to(ca.b) + to(cb.b);
}

// drawLiquidBody — тело жидкости и ледовая корка в канвас чанка (ЧК6 §5.1/§5.2).
// Тело `[liquidLevel, liquidLevel+depth]` полупрозрачно; глубже — плотнее (цвет
// тянется к `fog.color`, α к `fog.alpha`) — так читается глубина. Корка underIce
// — непрозрачная плита `[lv−iceH, lv]` над зеркалом (вне полыней), часть слоя
// земли. Глубинный градиент ниже тонирует и воду (source-atop).
function drawLiquidBody(ctx, world, baseX, topY) {
    const L = world.liquid;
    if (!L) return;
    const fogColor = L.fog ? L.fog.color : '#0b2036';
    const fogAlpha = L.fog ? L.fog.alpha : L.surface.alpha;
    const scale = Math.max(1, LIQUID_DEFAULTS.depthScale);
    for (let lx = 0; lx <= CHUNK; lx++) {
        const c = world._liquidCol(baseX + lx);
        if (!c) continue;
        const frac = Math.min(1, c.depth / scale);
        const col = mixHex(L.color, fogColor, frac);
        const alpha = Math.max(0, Math.min(1, L.surface.alpha + (fogAlpha - L.surface.alpha) * frac));
        const y0 = Math.max(0, Math.floor(c.lv - topY));
        const y1 = Math.min(CHUNK_HEIGHT - 1, Math.floor(c.lv + c.depth - topY));
        if (y1 < y0) continue;
        ctx.fillStyle = rgba(col, alpha);
        ctx.fillRect(lx, y0, 1, y1 - y0 + 1);
    }
    if (world._liquidCrust) {
        const ice = L.ice || (world.palette && world.palette.ice) || '#a9c6dc';
        ctx.fillStyle = ice;
        for (let lx = 0; lx <= CHUNK; lx++) {
            const top = world.crustTop(baseX + lx);
            if (top === null) continue;
            const c = world._liquidCol(baseX + lx);
            if (!c) continue;
            const y0 = Math.max(0, Math.floor(top - topY));
            const y1 = Math.min(CHUNK_HEIGHT - 1, Math.floor(c.lv - topY));
            if (y1 >= y0) ctx.fillRect(lx, y0, 1, y1 - y0 + 1);
        }
    }
}

// getChunkCanvas — лениво отрисованный чанк (кэш). Рельеф + пещеры.
export function getChunkCanvas(world, index) {
    const ss = chunkSupersample();
    // Сброс кэша при смене SS (DPR/зум) И идентичности рецепта (_viewKey, §2.5):
    // правка рецепта вида (relief/версия) иначе показала бы старый растр чанка.
    if (!world._chunkCache || world._chunkSS !== ss || world._chunkViewKey !== world._viewKey) {
        world._chunkCache = new Map();
        world._chunkSS = ss;
        world._chunkViewKey = world._viewKey;
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

    // Профиль и интервалы — один раз на колонку (иначе terrainHeight считается
    // несколько раз: главный проход, float, маска пещеры). `columnSpans` — единый
    // источник верха твёрдого (§2.3); colTh — кэш terrainHeight для маски.
    const colSpans = new Array(CHUNK + 1);
    const colTh = new Array(CHUNK + 2);
    const colForms = new Array(CHUNK + 2);
    const colCracks = new Array(CHUNK + 2);
    for (let i = 0; i <= CHUNK + 1; i++) {
        colTh[i] = world.terrainHeight(baseX + i);
        colForms[i] = world.formSpans(baseX + i, colTh[i]);
        colCracks[i] = world.crackSpans(baseX + i, colTh[i]);
    }
    for (let i = 0; i <= CHUNK; i++) colSpans[i] = world.columnSpans(baseX + i);
    // solidAtCol — то же поле `solidAt`, но по предвычисленным профилю И интервалам
    // форм колонки (без повторного terrainHeight на каждую пробу маски). Порядок
    // форм `(base ∨ add) ∧ ¬sub` тот же, что в `solidAt` (§2.2): без него маска
    // пещеры красила «воздухом» аддитивную плиту ниже terrainHeight (S1-bis, §2.1).
    const solidAtCol = (xi, yy) => {
        // Корка underIce — часть эффективного поля (ЧК6 §3.3): маска пещер не
        // красит «воздухом» ледовую плиту. Вне underIce — no-op.
        if (world.crustAt(baseX + xi, yy)) return true;
        const fs = colForms[xi];
        if (fs) for (const s of fs.sub) if (yy >= s.top && yy <= s.bottom) return false;
        if (yy >= colTh[xi]) {
            if (!world.caveAt(baseX + xi, yy, colTh[xi])) return true;
            if (fs) for (const a of fs.add) if (yy >= a.top && yy <= a.bottom) return true;
            return false;
        }
        if (fs) for (const a of fs.add) if (yy >= a.top && yy <= a.bottom) return true;
        return world._baseSolidAt(baseX + xi, yy, colTh[xi]);
    };

    // Главный проход — твёрдое тело столбца из интервалов columnSpans (§2.3):
    // база (интервал [terrainHeight, +∞)) плюс косметический верх surfaceY
    // (viewOnly-рябь, §2.1). Для столбца без форм/пещер это один fillRect;
    // второго пути («залить эвристикой от surfaceY вниз») нет — верх твёрдого
    // берётся из поля.
    ctx.fillStyle = rock;
    for (let lx = 0; lx <= CHUNK; lx++) {
        const wx = baseX + lx;
        const spans = colSpans[lx];
        const baseTop = spans[spans.length - 1].top; // база — последний интервал
        // Растр — надмножество твёрдой области: верх = min(видовой верх, верх базы).
        const y0 = Math.floor(Math.min(world.surfaceY(wx), baseTop) - topY);
        if (y0 >= CHUNK_HEIGHT) continue;
        ctx.fillRect(lx, Math.max(0, y0), 1, CHUNK_HEIGHT - Math.max(0, y0));
    }

    // 2D-формы (Э5.2, §2.3): задетые формой столбцы — шаг 1 px и заливка ровно
    // твёрдого `solidAt` (аддитивные формы красятся, вычитающие — «воздух»).
    // Столбцы без форм не задеваются — нулевая регрессия ЧК0–ЧК5 (быстрый путь
    // главного прохода выше). Растр — надмножество твёрдого; поэтому аддитивная
    // заливка берёт пиксели, ПЕРЕСЕКАЮЩИЕ интервал (floor/floor), а вычитающая
    // («воздух») — только пиксели, ЦЕЛИКОМ лежащие в интервале (ceil/ceil−1):
    // воздух не рисуется там, где solidAt=true (консервативность, S1-bis).
    for (let lx = 0; lx <= CHUNK; lx++) {
        const fs = colForms[lx];
        if (!fs) continue;
        if (fs.add.length) {
            ctx.fillStyle = rock;
            for (const a of fs.add) {
                const ly0 = Math.max(0, Math.floor(a.top - topY));
                const ly1 = Math.min(CHUNK_HEIGHT - 1, Math.floor(a.bottom - topY));
                if (ly1 >= ly0) ctx.fillRect(lx, ly0, 1, ly1 - ly0 + 1);
            }
        }
        if (fs.sub.length) {
            ctx.fillStyle = COLORS.cave;
            for (const s of fs.sub) {
                const ly0 = Math.max(0, Math.ceil(s.top - topY));
                const ly1 = Math.min(CHUNK_HEIGHT - 1, Math.ceil(s.bottom - topY) - 1);
                if (ly1 >= ly0) ctx.fillRect(lx, ly0, 1, ly1 - ly0 + 1);
            }
        }
    }

    // Косметические трещины crack2d (решение гейта 2026-09-25, дефект D1): тёмный
    // клин поверх рельефа, физику НЕ трогает (вне равенства по твёрдому телу,
    // §2.1) — красится после твёрдых интервалов и до пещерной маски. Цвет —
    // тёмный тон палитры (`palette.dark`), НЕ COLORS.cave: пещерный цвет обязан
    // остаться признаком вычитания твёрдого, иначе проверка «пещерный цвет на
    // твёрдом» ложно сработала бы на косметике.
    ctx.fillStyle = world.hasView ? world.palette.dark : shade(world.color, 0.4);
    for (let lx = 0; lx <= CHUNK; lx++) {
        const cs = colCracks[lx];
        if (!cs) continue;
        for (const ck of cs) {
            const ly0 = Math.max(0, Math.floor(ck.top - topY));
            const ly1 = Math.min(CHUNK_HEIGHT - 1, Math.floor(ck.bottom - topY));
            if (ly1 >= ly0) ctx.fillRect(lx, ly0, 1, ly1 - ly0 + 1);
        }
    }

    // Пещерная маска — ЧАСТЬ ТВЁРДОГО ТЕЛА, не косметика (S1-bis): isCave
    // вычитает твёрдое из solidAt, маска — проявление этого вычитания. Растр
    // консервативен: «воздух» красится только там, где solidAt=false. 3×3-блок
    // допустим, только если ВЕСЬ блок воздух; на границе пещеры — шаг 1 px
    // (твёрдая точка у границы не перекрывается пещерой = «невидимая стена»).
    ctx.fillStyle = COLORS.cave;
    for (let lx = 0; lx < CHUNK; lx += 3) {
        const wx = baseX + lx;
        const th = colTh[lx];
        for (let ly = 0; ly < CHUNK_HEIGHT; ly += 3) {
            const wy = topY + ly;
            if (wy < th + 6) continue;      // корка поверхности держит игрока
            if (!world.caveAt(wx, wy, th)) continue;
            let allAir = true;
            for (let dx = 0; dx < 3 && allAir; dx++) {
                for (let dy = 0; dy < 3; dy++) {
                    if (solidAtCol(lx + dx, wy + dy)) { allAir = false; break; }
                }
            }
            if (allAir) { ctx.fillRect(lx, ly, 3, 3); continue; }
            // Граница пещеры — шаг 1 px: красим ровно воздушные точки блока.
            for (let dx = 0; dx < 3; dx++) {
                for (let dy = 0; dy < 3; dy++) {
                    if (!solidAtCol(lx + dx, wy + dy)) ctx.fillRect(lx + dx, ly + dy, 1, 1);
                }
            }
        }
    }

    // Парящая порода (float) — частный случай интервалов columnSpans (Э5.1):
    // интервал [th−FLOAT_SPAN, th−FLOAT_GAP] очерчивает полосу, твёрдость внутри
    // решает `solidAt` (шум). Растр — НАДМНОЖЕСТВО твёрдой области: шаг по x —
    // 1 px, по y — FLOAT_STEP_Y с запасом FLOAT_BLEED (прежние 3×3 оставляли
    // непокрашенную кромку до 3 px — игрок упирался в невидимую стену, идея §2.1).
    ctx.fillStyle = rock;
    for (let lx = 0; lx <= CHUNK; lx++) {
        const wx = baseX + lx;
        for (const sp of colSpans[lx]) {
            if (sp.bottom === Infinity) continue; // базу красит главный проход
            // Сэмплы полосы снизу вверх: сетка FLOAT_STEP_Y + проба у верхней
            // границы (сетка туда не попадает из-за сдвига FLOAT_EPS).
            const ys = [];
            for (let wy = sp.bottom - FLOAT_EPS; wy >= sp.top + FLOAT_EPS; wy -= FLOAT_STEP_Y) ys.push(wy);
            ys.push(sp.top + FLOAT_EPS);
            let yTop = null, yBottom = null;
            for (const wy of ys) {
                // Полоса `float` — база: 2D-формы красит отдельный проход выше,
                // здесь они не нужны, а их расчёт удорожал бы скан (§2.4).
                if (world.baseSolid(wx, wy)) {
                    if (yTop === null) yBottom = wy;
                    yTop = wy;
                } else if (yTop !== null) {
                    fillFloatRun(ctx, lx, yTop, yBottom, topY);
                    yTop = null;
                }
            }
            if (yTop !== null) fillFloatRun(ctx, lx, yTop, yBottom, topY);
        }
    }

    // Слой жидкости (ЧК6 §5.1): тело жидкости запекается в канвас земли — ПОСЛЕ
    // твёрдого тела, ДО глубинного градиента (градиент тонирует и воду). Отдельного
    // `drawLiquidBack` нет; анимируется только фронтальное зеркало (`drawLiquidFront`).
    drawLiquidBody(ctx, world, baseX, topY);

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
        // Кромка — по верху твёрдого: столбцы с формами (грот/чаша/вал) берут
        // `floorY` (пол/вал), иначе кромка висела бы на уровне исходной земли
        // поперёк выреза. Столбцы без форм — прежний `surfaceY` (нулевая регрессия).
        const ey = colForms[lx] ? world.floorY(wx) : world.surfaceY(wx);
        const ly = Math.floor(ey - topY);
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
    // Вытеснение кэша: держим только чанки видимого окна (± запас). Канвас ~7 МБ
    // при SS=2, без вытеснения память растёт с пройденным путём (хвост эпика).
    if (world._chunkCache && world._chunkCache.size) {
        const keep = radius + 1;
        for (const key of world._chunkCache.keys()) {
            if (key < centerChunk - keep || key > centerChunk + keep) world._chunkCache.delete(key);
        }
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

// shipDrawSize — мировой размер спрайта корабля на высадке (решение создателя
// 2026-09-25): ship_scale пакета (размер в ростах человека) × рост игрока
// PLAYER_H. Пакет без ship_scale (старый/старый сервер) → прежняя константа
// SHIP_DECOR_SIZE: сдача аддитивная, старое поведение 1:1.
export function shipDrawSize(pkg) {
    const scale = pkg ? Number(pkg.ship_scale) : 0;
    if (scale > 0) return scale * PLAYER_H;
    return SHIP_DECOR_SIZE;
}

// drawShip — корабль игрока парит над точкой высадки (ЧК-ship, идея
// 2026-09-23 §5; решение создателя 2026-09-23 — «подвесить его… без лестницы»):
// статичная декорация (не интерактивна, ничего не персистит). Низ спрайта — на
// SHIP_HOVER_BOTTOM над землёй, лёгкое вертикальное покачивание от времени
// кадра (физику игрока и ввод не трогает). Размер — shipDrawSize(pkg) (ship_scale
// × рост игрока, дефолт корабля 12 ростов; фолбэк — SHIP_DECOR_SIZE). Поза —
// каноническая пара (angle, flip) из пакета (на прогулке /me не зовём): курса
// нет → shipDrawTransform(0, orient). Спрайт перекрашивается recolorShipSprite
// (кэш); фолбэк И4: не загружен/имя неизвестно → null, ничего не рисуем
// (отрисовка не ломается). Рисуется до игрока — тот поверх.
export function drawShip(ctx, world, camera, vw, vh, pkg, timeMs) {
    if (!pkg || !pkg.ship_icon) return;
    const sprite = recolorShipSprite(pkg.ship_icon, pkg.ship_color);
    if (!sprite) return;
    const size = shipDrawSize(pkg);
    // Якорь по X — стартовая колонка мира (ЧК6.2 §6.2): корабль стоит у спавна,
    // который теперь выбирается по §6.2, а не всегда x=0.
    const wx = (Number.isFinite(world.spawnX) ? world.spawnX : 0) + SHIP_DECOR_X;
    const x = wx - camera.x + vw / 2;
    // Якорь — `floorY` (§5 п.12): корабль стоит/парит над ПОЛОМ (землёй), не на
    // плите-своде и не на terrainHeight (в гроте terrainHeight игнорирует формы).
    const gy = world.floorY(wx) - camera.y + vh / 2;
    const bob = Math.sin((timeMs || 0) * 2 * Math.PI / SHIP_BOB_PERIOD_MS) * SHIP_BOB_AMP;
    const orient = { angle: Number(pkg.ship_angle) || 0, flip: !!pkg.ship_flip };
    const t = shipDrawTransform(0, orient);
    ctx.save();
    // Центр спрайта = низ над землёй (SHIP_HOVER_BOTTOM) + половина размера + покачивание.
    ctx.translate(x, gy - SHIP_HOVER_BOTTOM - size / 2 + bob);
    ctx.rotate(t.rotate);
    ctx.scale(t.scaleX, t.scaleY);
    ctx.drawImage(sprite, -size / 2, -size / 2, size, size);
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
            // Якорь — `floorY` (§5 п.12): фауна на полу грота, не на потолке.
            const sy = world.floorY(x) - camera.y + vh / 2;
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

// ==================== СЛОЙ ЖИДКОСТИ — ФРОНТ (ЧК6 §5.1/§5.2) ====================

// liquidGroups — непрерывные группы точек зеркала (разрыв на сухих колонках и
// над коркой underIce, где зеркало гасится, N7). Точка: {x, y, depth} в экранных
// координатах. Волна — `liquidWave` (тот же примитив `wave`, §5.2).
function liquidGroups(world, camera, vw, vh, t) {
    const L = world.liquid;
    const left = camera.x - vw / 2, right = camera.x + vw / 2;
    const x0 = Math.floor(left) - 1, x1 = Math.ceil(right) + 1;
    const groups = [];
    let g = null;
    for (let wx = x0; wx <= x1; wx++) {
        const c = world._liquidCol(wx);
        const on = c && !world.crustAt(wx, c.lv);
        if (!on) { if (g) { groups.push(g); g = null; } continue; }
        const wy = c.lv + liquidWave(world.seed, L.surface.wave, wx, t);
        const p = { x: wx - camera.x + vw / 2, y: wy - camera.y + vh / 2, depth: c.depth };
        if (!g) { g = [p]; groups.push(g); } else g.push(p);
    }
    return groups;
}

// drawUnderwater — обзор под водой (§5.3): полноэкранная подкраска fog.color +
// мягкая виньетка, включается только когда ГОЛОВА в жидкости (`liquidAt(x, headY+1)`),
// так «лежание на зеркале» не меняет обзор/воздух. Рисуется внутри drawLiquidFront
// (последним), нового покадрового прохода нет. День/ночь — ×lightMul.
function drawUnderwater(ctx, world, player, vw, vh, lightMul) {
    const L = world.liquid;
    const headY = player.y - player.h / 2;
    if (world.liquidAt(player.x, headY + 1) === '') return;
    const lv = world.liquidLevel(player.x);
    const d = Math.max(0, headY - lv);
    const scale = Math.max(1, LIQUID_DEFAULTS.depthScale);
    const fogA = L.fog ? L.fog.alpha : LIQUID_DEFAULTS.fogAlpha;
    let a = 0.2 + (fogA * 0.8 - 0.2) * Math.min(1, d / scale);
    a = Math.max(0, Math.min(1, a * lightMul));
    if (a <= 0) return;
    const color = L.fog ? L.fog.color : '#123a55';
    ctx.save();
    ctx.fillStyle = rgba(color, a);
    ctx.fillRect(0, 0, vw, vh);
    // Виньетка (мягкая, без «слепоты»): α от 0 в центре до ≈0.25 по краям.
    const g = ctx.createRadialGradient(vw / 2, vh / 2, Math.min(vw, vh) * 0.25, vw / 2, vh / 2, Math.max(vw, vh) * 0.75);
    g.addColorStop(0, rgba(color, 0));
    g.addColorStop(1, rgba(color, Math.min(0.25, a + 0.06)));
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, vw, vh);
    ctx.restore();
}

// drawLiquidFront — фронтальный проход жидкости (§5.1): подводная пелена
// (утопление декора, N2) → зеркало/волна → блик → кромка-пена → обзор под водой
// (§5.3). После игрока, до передних слоёв погоды. Пелена рисуется ВСЕГДА и не
// зависит от положения игрока; над коркой underIce зеркало/блик/пена гасятся (N7)
// — видны только в полыньях. `player` — для подводной подкраски (ЧК6.2).
export function drawLiquidFront(ctx, world, camera, vw, vh, env, player) {
    const L = world.liquid;
    if (!L) return;
    const lightMul = env ? env.lightMul() : 1;
    const left = camera.x - vw / 2, right = camera.x + vw / 2;
    const x0 = Math.floor(left) - 2, x1 = Math.ceil(right) + 2;
    // 1) Подводная пелена (N2): fog.color, α = fog.alpha·0.35 — по телу воды
    //    [liquidLevel, bedY]; тонирует декор ниже ватерлинии, независимо от игрока.
    //    Модулируется слоем суток (§10: блик и пелена читают lightMul).
    if (L.fog) {
        ctx.save();
        ctx.fillStyle = rgba(L.fog.color, Math.max(0, Math.min(1, L.fog.alpha * 0.35 * lightMul)));
        for (let wx = x0; wx <= x1; wx++) {
            const c = world._liquidCol(wx);
            if (!c) continue;
            const y0 = c.lv - camera.y + vh / 2;
            const y1 = c.lv + c.depth - camera.y + vh / 2;
            ctx.fillRect(wx - camera.x + vw / 2, y0, 1, Math.max(1, y1 - y0));
        }
        ctx.restore();
    }
    // 2) Зеркало/блик/кромка — только там, где зеркало (вне корки). Групп нет
    //    (видны только сухие колонки) — слои пропускаем, подкраска §5.3 уместна.
    const t = (typeof performance !== 'undefined' ? performance.now() : 0) / 1000;
    const groups = liquidGroups(world, camera, vw, vh, t);
    if (groups.length) {
        ctx.save();
        ctx.lineJoin = 'round';
        // Зеркало: линия уровня с волной, темнеет вместе с миром.
        ctx.lineWidth = 2;
        ctx.strokeStyle = rgba(L.color, Math.max(0, Math.min(1, 0.85 * lightMul)));
        for (const grp of groups) {
            if (grp.length < 2) continue;
            ctx.beginPath();
            ctx.moveTo(grp[0].x, grp[0].y);
            for (let i = 1; i < grp.length; i++) ctx.lineTo(grp[i].x, grp[i].y);
            ctx.stroke();
        }
        // Блик: яркая линия по зеркалу, гаснет ночью (§5.2).
        ctx.lineWidth = 1;
        ctx.strokeStyle = rgba(shadeHex(L.color, 1.5), Math.max(0, Math.min(1, 0.55 * lightMul)));
        for (const grp of groups) {
            if (grp.length < 2) continue;
            ctx.beginPath();
            ctx.moveTo(grp[0].x, grp[0].y - 1);
            for (let i = 1; i < grp.length; i++) ctx.lineTo(grp[i].x, grp[i].y - 1);
            ctx.stroke();
        }
        // Кромка-пена у пологого берега: там, где колонка затоплена и глубина мала.
        if (L.surface.foam > 0) {
            ctx.fillStyle = rgba(shadeHex(L.color, 1.7), Math.max(0, Math.min(0.8, 0.55 * L.surface.foam * (0.4 + 0.6 * lightMul))));
            const band = Math.max(1, 2 + 4 * L.surface.foam);
            for (const grp of groups) {
                for (const p of grp) {
                    if (p.depth < 30) ctx.fillRect(p.x, p.y, 1, Math.max(1, band * (1 - p.depth / 30)));
                }
            }
        }
        ctx.restore();
    }
    // 3) Обзор под водой (§5.3) — последним в проходе.
    if (player) drawUnderwater(ctx, world, player, vw, vh, lightMul);
}

// drawLiquidEmissive — свечение жидкости (лава/планктон, §5.2/§5.4) в финальном
// эмиссивном порядке (после переднего ночного тинта): самосветящаяся линия зеркала
// + мягкий ореол. Лава светится всегда; планктон — сильнее ночью.
export function drawLiquidEmissive(ctx, world, camera, vw, vh, env) {
    const L = world.liquid;
    if (!L || !L.glow) return;
    const nightFactor = env ? env.nightFactor() : 1;
    const boost = L.medium === 'лава' ? 1 : (0.4 + 0.6 * nightFactor);
    const t = (typeof performance !== 'undefined' ? performance.now() : 0) / 1000;
    const groups = liquidGroups(world, camera, vw, vh, t);
    if (!groups.length) return;
    ctx.save();
    ctx.lineJoin = 'round';
    ctx.globalCompositeOperation = 'lighter';
    const widths = [7, 3.5, 1.5];
    const alphas = [0.10, 0.22, 0.5];
    for (let k = 0; k < widths.length; k++) {
        ctx.lineWidth = widths[k];
        ctx.strokeStyle = rgba(L.glow, Math.max(0, Math.min(1, alphas[k] * boost)));
        for (const grp of groups) {
            if (grp.length < 2) continue;
            ctx.beginPath();
            ctx.moveTo(grp[0].x, grp[0].y);
            for (let i = 1; i < grp.length; i++) ctx.lineTo(grp[i].x, grp[i].y);
            ctx.stroke();
        }
    }
    ctx.restore();
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
