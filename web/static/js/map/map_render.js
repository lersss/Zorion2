// web/static/js/map/map_render.js
import { state, elements } from './config.js';
import { isFiniteNumber, worldToCanvas, getStarColor, getStarShade } from './utils.js';
import { CONFIG } from '../config.js';
import { drawNPCAgents } from './npc_agents.js';
import { recolorShipSprite } from './ship_sprites.js';
import { drawStarfield, initStarfield } from './starfield.js';

const { map: mapCfg } = CONFIG;

// Спрайт корабля (спека 61b §6.1): цельный PNG из /me (state.userShipIcon,
// уже смаппленное) + цвет перекраски (state.userShipColor) →
// recolorShipSprite(icon, color). Спрайт грузится асинхронно — до загрузки
// рисуется фолбэк-примитив (И4).

// setShipIcon — мост из data.js: PNG-имя спрайта из /me (уже смаппленное).
export function setShipIcon(fileName) {
    if (!fileName || typeof fileName !== 'string') return;
    state.userShipIcon = fileName;
}

// setShipColor — цвет перекраски спрайта из /me (NULL = «Оригинал»).
export function setShipColor(color) {
    state.userShipColor = color || null;
}

// Размеры звёзд по спектральному классу — вынесено из циклов.
export const SPECTRAL_SIZE = {
    'O': 21, 'B': 19.5, 'A': 18,
    'F': 16.5, 'G': 15,
    'K': 12, 'M': 9,
    'L': 7.5, 'T': 6,
    'Y': 5.4,
};

// ==================== RESIZE ====================

export function resizeCanvas() {
    const wrapper = document.getElementById('canvas-wrapper');
    state.canvasWidth = wrapper.clientWidth;
    state.canvasHeight = wrapper.clientHeight;
    elements.canvas.width = state.canvasWidth;
    elements.canvas.height = state.canvasHeight;
    initStarfield(); // фон «звёздное небо»: тайл при загрузке и при ресайзе (спека 30c.1)
    updateFitZoom();
    draw();
}

// galaxyRadiusFromRegions — радиус галактики из центров регионов
// (самый дальний центр + запас 2%).
export function galaxyRadiusFromRegions(regions) {
    let maxR = 1;
    for (const r of regions) {
        const d = Math.hypot(r.x, r.y);
        if (d > maxR) maxR = d;
    }
    return maxR * 1.02;
}

// updateFitZoom — ставит minZoom так, чтобы вся галактика помещалась в экран.
// Вызывается после загрузки регионов и при ресайзе.
export function updateFitZoom() {
    if (!state.galaxyRadius) return;
    const minDim = Math.min(state.canvasWidth, state.canvasHeight);
    const fit = (minDim * 0.95) / (2 * state.galaxyRadius);
    state.minZoom = Math.max(fit, 0.00001);
    // Если текущий масштаб ниже нового минимума (например, из сохранённого вьюпорта) — поднимаем.
    if (state.scale < state.minZoom) {
        state.scale = state.minZoom;
    }
}

// ==================== РАЗМЕР КЛАСТЕРА НА ЭКРАНЕ ====================

// Используется и в рендере, и в hover-детекции (из events.js).
export function clusterScreenRadius(c) {
    if (c.cnt === 1) {
        const baseSize = SPECTRAL_SIZE[c.sspec] || 12;
        const hash = hashString(c.sid || '');
        const variation = 0.9 + (hash % 20) / 100;
        return Math.max(mapCfg.minRadius, baseSize * variation * state.scale);
    }
    return Math.max(10, Math.min(30, 8 + Math.log2(c.cnt) * 3));
}

// clusterDotRadius — радиус точки-звезды кластера: растёт с числом звёзд,
// но не превращается в пузырь (потолок ~5px).
function clusterDotRadius(count) {
    return Math.max(1.2, Math.min(5, 1.2 + Math.log2(count) * 0.4));
}

// ==================== DRAW ====================

export function draw() {
    const {
        canvasWidth, canvasHeight, currentWorldId, hoveredWorldId,
        isFlying, flyStartTime, flyDuration, flyFrom, flyTo,
        scale, offsetX, offsetY, clusters, focusWorldId
    } = state;
    const { ctx, statusBar } = elements;

    ctx.clearRect(0, 0, canvasWidth, canvasHeight);

    drawStarfield(ctx, canvasWidth, canvasHeight, scale, offsetX, offsetY, Date.now());

// --- Регионы галактики: на малом зуме вместо кружков с количеством ---
    // Уровни детализации по зумам:
    //  scale < regionNamesZoom            — регионы с названиями, без звёзд;
    //  regionNamesZoom..regionDisplayThreshold — названия/цвета плавно затухают от
    //      полной прозрачности у minZoom до 0 к появлению названий звёзд (0.5);
    //  scale >= regionDisplayThreshold    — только звёзды.
    // Раньше окно затухания было 0.02..0.05 (при fit-зуме мин. — ~0.05), и регионы
    // исчезали заведомо раньше названий миров, оставляя огромный «немой» диапазон.
    const regionThreshold = mapCfg.regionDisplayThreshold;
    const namesZoom = mapCfg.regionNamesZoom;
    let regionAlpha = 0;
    let drawStars = false;
    if (scale < namesZoom) {
        regionAlpha = 1;
    } else if (scale < regionThreshold) {
        const k = (scale - namesZoom) / (regionThreshold - namesZoom);
        regionAlpha = 1 - k;
        drawStars = true;
    } else {
        drawStars = true;
    }
    if (regionAlpha > 0) {
        drawRegions(ctx, canvasWidth, canvasHeight, scale, offsetX, offsetY, regionAlpha, true);
    }

    // --- Отрисовка кластеров ---
    const visibleClusters = clusters || [];
    const singles = [];

    for (const c of visibleClusters) {
        const px = c.x * scale + offsetX;
        const py = c.y * scale + offsetY;

        // Пропускаем то, что вне экрана
        if (!isFiniteNumber(px) || !isFiniteNumber(py)) continue;
        if (px < -50 || py < -50 || px > canvasWidth + 50 || py > canvasHeight + 50) continue;

        if (!drawStars) {
            // На малом зуме (чистые регионы) звёзды не рисуем.
            continue;
        }

        if (c.cnt === 1) {
            drawSingleStar(ctx, c, px, py, scale, currentWorldId, hoveredWorldId, focusWorldId);
            singles.push({ c, x: px, y: py });
        } else {
            drawCluster(ctx, c, px, py);
        }
    }

    // --- Подписи одиночных миров ---
    // Показываем при достаточном зуме ИЛИ когда объектов мало (место есть).
    if (scale > mapCfg.nameDisplayThreshold || singles.length <= mapCfg.nameAlwaysShowLimit) {
        drawNames(ctx, singles, scale);
    }

    // --- Имя звезды под курсором (даже когда общие названия скрыты) ---
    drawHoveredStarName(ctx, scale, offsetX, offsetY);

    // --- NPC-агенты (спека 20a.1 §7): иконки поверх звёзд ---
    drawNPCAgents(ctx, canvasWidth, canvasHeight);

    // --- Анимация полёта ---
    if (isFlying && flyFrom && flyTo) {
        drawFlight(ctx, scale, flyFrom, flyTo, flyStartTime, flyDuration);
    }

    updateStatusBar(statusBar, visibleClusters, isFlying, flyStartTime, flyDuration);

    // FPS-плашка рисуется ПОСЛЕ всей отрисовки: любой вызов draw() (полёт,
    // события мыши, отдельный rAF-цикл NPC-агентов на близком зуме) чистит
    // канвас целиком — рисуем плашку последней, чтобы её не стирало.
    drawFpsOverlay(ctx, canvasWidth, canvasHeight);
}

// ==================== ОТРИСОВКА ЭЛЕМЕНТОВ ====================

function drawSingleStar(ctx, c, x, y, scale, currentWorldId, hoveredWorldId, focusWorldId) {
    const radius = clusterScreenRadius(c);
    const color = getStarShade(c.sspec || 'G', c.stemp, c.stype);

    ctx.globalAlpha = 1;
    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    ctx.fillStyle = color;
    ctx.fill();
    ctx.strokeStyle = '#0f172a';
    ctx.lineWidth = 1;
    ctx.stroke();

    // Точки-компаньоны двойных/кратных систем (35a): binary — одна точка,
    // multiple — две, single — ничего. Цвет точки — по спектру компаньона
    // (35b §6.4): binary — mods.companion, multiple — внутренний + внешний
    // (extra_companions[*].spectral_class). Данные — smods из кластера
    // (filter_worlds_handler, 35b §2.3).
    const companionAngles = c.systype === 'binary' ? [Math.PI / 4] :
                            c.systype === 'multiple' ? [Math.PI / 6, Math.PI * 5 / 6] : [];
    if (companionAngles.length > 0) {
        const mods = c.smods || {};
        const extra = Array.isArray(mods.extra_companions) ? mods.extra_companions : [];
        const companionSpecs = c.systype === 'multiple'
            ? [mods.companion, extra[0] && extra[0].spectral_class]
            : [mods.companion];
        const dist = Math.max(3, radius * 1.5);
        const cr = radius * 0.45;
        ctx.globalAlpha = 0.6;
        companionAngles.forEach((angleRad, i) => {
            const dotColor = companionSpecs[i] ? getStarColor(companionSpecs[i], 'star') : color;
            ctx.beginPath();
            ctx.arc(x + Math.cos(angleRad) * dist, y + Math.sin(angleRad) * dist, cr, 0, Math.PI * 2);
            ctx.fillStyle = dotColor;
            ctx.fill();
        });
        ctx.globalAlpha = 1;
    }

    if (c.sid === focusWorldId) {
        ctx.save();
        ctx.strokeStyle = 'rgba(74, 222, 128, 0.9)';
        ctx.lineWidth = 2;
        ctx.setLineDash([6, 4]);
        ctx.beginPath();
        ctx.arc(x, y, radius + 7, 0, Math.PI * 2);
        ctx.stroke();
        ctx.setLineDash([]);
        ctx.shadowColor = 'rgba(74, 222, 128, 0.5)';
        ctx.shadowBlur = 14;
        ctx.beginPath();
        ctx.arc(x, y, radius + 7, 0, Math.PI * 2);
        ctx.stroke();
        ctx.restore();
    }

    if (c.sid === currentWorldId) {
        try {
            const glow = ctx.createRadialGradient(x, y, Math.max(0, radius - 2), x, y, radius + 10);
            glow.addColorStop(0, 'rgba(251,191,36,0.3)');
            glow.addColorStop(1, 'rgba(251,191,36,0)');
            ctx.fillStyle = glow;
            ctx.beginPath();
            ctx.arc(x, y, radius + 10, 0, Math.PI * 2);
            ctx.fill();
        } catch (e) {
            ctx.beginPath();
            ctx.arc(x, y, radius + 6, 0, Math.PI * 2);
            ctx.strokeStyle = '#fbbf24';
            ctx.lineWidth = 2;
            ctx.stroke();
        }
    }

    if (c.sid === hoveredWorldId) {
        ctx.save();
        ctx.shadowColor = 'rgba(255,255,255,0.3)';
        ctx.shadowBlur = 12;
        ctx.beginPath();
        ctx.arc(x, y, radius + 3, 0, Math.PI * 2);
        ctx.fillStyle = 'rgba(255,255,255,0.15)';
        ctx.fill();
        ctx.strokeStyle = 'rgba(255,255,255,0.6)';
        ctx.lineWidth = 2;
        ctx.stroke();
        ctx.restore();
    }
}

function drawCluster(ctx, c, x, y) {
    const count = c.cnt;
    const color = getStarShade(c.sspec || 'G', c.stemp, c.stype);
    const radius = clusterDotRadius(count);

    // Свечение: по базовому hex класса (градация не нужна на ореоле),
    // крупные кластеры — яркие «звёзды» с ореолом.
    const glow = radius * 2.5;
    const g = ctx.createRadialGradient(x, y, 0, x, y, glow);
    g.addColorStop(0, hexToRgba(getStarColor(c.sspec || 'G', c.stype), 0.35));
    g.addColorStop(1, hexToRgba(getStarColor(c.sspec || 'G', c.stype), 0));
    ctx.beginPath();
    ctx.arc(x, y, glow, 0, Math.PI * 2);
    ctx.fillStyle = g;
    ctx.fill();

    // Ядро звезды + белый блик в центре.
    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    ctx.fillStyle = color;
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x, y, Math.max(0.6, radius * 0.4), 0, Math.PI * 2);
    ctx.fillStyle = 'rgba(255,255,255,0.75)';
    ctx.fill();
}

function drawNames(ctx, singles, scale) {
    // Подписи масштабируются с зумом: растут при приближении, сжимаются при
    // отдалении (fontSize = nameFontSize × scale). Раньше жёсткий минимум 18px
    // держал размер постоянным в широком диапазоне зума. Пороги:
    // nameMinFontSize — читаемый минимум, nameMaxFontSize — чтобы на большом
    // зуме название не лезло за пределы экрана.
    const fontSize = Math.round(Math.min(
        mapCfg.nameMaxFontSize,
        Math.max(mapCfg.nameMinFontSize, mapCfg.nameFontSize * scale),
    ));
    ctx.fillStyle = '#94a3b8';
    ctx.font = `${fontSize}px system-ui`;
    ctx.textAlign = 'center';

    for (const s of singles) {
        // Название hover-звезды рисуем отдельно (пилюлей над ней).
        if (s.c.sid === state.hoveredWorldId) continue;
        const radius = clusterScreenRadius(s.c);
        // Названия чистые, без суффиксов типа («дв.», «нейтр.») — тип виден
        // цветом (экзотика — свой цвет) и в модалке (решение создателя).
        ctx.fillText(s.c.sname || '—', s.x, s.y + radius + fontSize);
    }
}

// drawHoveredStarName — рисует имя звезды под курсором над ней.
// Работает на любом зуме (даже когда общие названия скрыты): берёт звезду
// из state.clusters и рисует подпись-«пилюлю» выше точки.
function drawHoveredStarName(ctx, scale, offsetX, offsetY) {
    const hoveredId = state.hoveredWorldId;
    if (!hoveredId) return;

    const clusters = state.clusters || [];
    let c = null;
    for (const cl of clusters) {
        if (cl.cnt === 1 && cl.sid === hoveredId) { c = cl; break; }
    }
    if (!c) return;

    const x = c.x * scale + offsetX;
    const y = c.y * scale + offsetY;
    if (!isFiniteNumber(x) || !isFiniteNumber(y)) return;

    const name = c.sname || '—';
    const r = clusterScreenRadius(c);
    const fontSize = Math.max(11, Math.round(mapCfg.nameFontSize + 2));
    ctx.font = `600 ${fontSize}px system-ui`;
    const w = Math.max(40, ctx.measureText(name).width + 16);
    const h = fontSize + 8;
    const top = y - r - 6 - h;

    ctx.fillStyle = 'rgba(10,15,32,0.88)';
    roundRectPath(ctx, x - w / 2, top, w, h, 6);
    ctx.fill();
    ctx.strokeStyle = 'rgba(148,163,184,0.55)';
    ctx.lineWidth = 1;
    roundRectPath(ctx, x - w / 2, top, w, h, 6);
    ctx.stroke();

    ctx.fillStyle = '#e2e8f0';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(name, x, top + h / 2);
    ctx.textBaseline = 'alphabetic';
}

// roundRectPath — скруглённый прямоугольник (путь для fill/stroke).
function roundRectPath(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
}

// ==================== РЕГИОНЫ (малый зум) ====================

// Кэш ячеек Вороного: пересчитываются только при изменении списка регионов.
let voronoiCells = null;
let voronoiKey = '';

// drawRegions — рисует регионы как ячейки Вороного: каждый регион покрывает
// территорию, ближе к его центру, чем к любому другому. Ячейки не пересекаются
// и замощают всю галактику (нет ни дыр, ни перекрытий).
// alpha — множитель прозрачности (0..1) для плавного затухания у порога зума;
// showNames — рисовать ли названия (убираются, когда регионов на экране мало).
function drawRegions(ctx, canvasWidth, canvasHeight, scale, offsetX, offsetY, alpha = 1, showNames = true) {
    const regions = state.regions || [];
    if (regions.length < 2) return;

    const cells = ensureVoronoi(regions);
    // Названия регионов масштабируются с зумом: на фите галактики (state.minZoom)
    // они равны regionFontSize, дальше растут пропорционально приближению, пока
    // не упрутся в regionMaxFontSize (на большом зуме регионы почти исчезли).
    const regionRefZoom = state.minZoom || mapCfg.regionNamesZoom;
    const fontSize = Math.round(Math.min(
        mapCfg.regionMaxFontSize,
        Math.max(mapCfg.regionFontSize, mapCfg.regionFontSize * (scale / regionRefZoom)),
    ));
    const labels = [];

    for (const cell of cells) {
        const poly = cell.poly;
        if (poly.length < 3) continue;

        // Быстрый cull: bounding box ячейки на экране.
        let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
        for (const v of poly) {
            const sx = v.x * scale + offsetX;
            const sy = v.y * scale + offsetY;
            if (sx < minX) minX = sx;
            if (sx > maxX) maxX = sx;
            if (sy < minY) minY = sy;
            if (sy > maxY) maxY = sy;
        }
        if (maxX < -50 || maxY < -50 || minX > canvasWidth + 50 || minY > canvasHeight + 50) continue;

        // Заливка ячейки.
        ctx.beginPath();
        for (let j = 0; j < poly.length; j++) {
            const sx = poly[j].x * scale + offsetX;
            const sy = poly[j].y * scale + offsetY;
            if (j === 0) ctx.moveTo(sx, sy);
            else ctx.lineTo(sx, sy);
        }
        ctx.closePath();
        ctx.fillStyle = hexToRgba(cell.color, 0.20 * alpha);
        ctx.fill();
        ctx.strokeStyle = hexToRgba(cell.color, 0.6 * alpha);
        ctx.lineWidth = 2;
        ctx.stroke();

        if (!showNames) continue;

        // Подпись собираем отдельно — чтобы избежать наложений названий.
        // Якорь — геометрический центр ячейки (центроид), а не центр кластера:
        // у краевых регионов (обрезанных границей галактики и соседями) они
        // расходятся, название с центроида сидит в середине территории (81a).
        const c = polygonCentroid(poly);
        const lx = c.x * scale + offsetX;
        const ly = c.y * scale + offsetY;
        if (!isFiniteNumber(lx) || !isFiniteNumber(ly)) continue;
        const cellSize = Math.hypot(maxX - minX, maxY - minY);
        labels.push({ name: cell.name, profile: cell.profile || '', race: cell.race || '', x: lx, y: ly, size: cellSize });
    }

    if (showNames) drawRegionLabels(ctx, labels, alpha, fontSize);
}

// drawRegionLabels — размещает названия регионов без наложений:
// крупные ячейки получают приоритет, пересекающиеся подписи пропускаются.
// Отладочно (59a): под названием — вторая строка с типом профиля региона
// (мельче, приглушённый жёлтый); отладочно (расы): третья строка с именем
// доминантной расы территории (мельче, приглушённый индиго). В финале убрать
// вместе с профилем — профиль и раса не публикуются как ярлыки (спека §11.7 /
// GDD §2.6.1). Вторая и третья строки включены в rect проверки наложений,
// чтобы не наезжать на соседние подписи.
function drawRegionLabels(ctx, labels, alpha, fontSize) {
    if (labels.length === 0) return;

    labels.sort((a, b) => b.size - a.size);
    const placed = [];
    ctx.font = `600 ${fontSize}px system-ui`;
    ctx.textAlign = 'center';
    ctx.fillStyle = `rgba(226,232,240,${0.9 * alpha})`;

    for (const lb of labels) {
        const hasProfile = !!lb.profile;
        const hasRace = !!lb.race;
        const subFontSize = Math.max(9, Math.round(fontSize * 0.6));
        const nameW = lb.name.length * fontSize * 0.62 + 8;
        const profileW = hasProfile ? lb.profile.length * subFontSize * 0.62 + 8 : 0;
        const raceW = hasRace ? lb.race.length * subFontSize * 0.62 + 8 : 0;
        const w = Math.max(nameW, profileW, raceW);
        const profileH = hasProfile ? subFontSize + 8 : 0;
        const raceH = hasRace ? subFontSize + 8 : 0;
        const h = fontSize + 6 + profileH + raceH;
        const rect = { x: lb.x - w / 2, y: lb.y - 6, w, h };

        let ok = true;
        for (const p of placed) {
            if (rect.x < p.x + p.w && rect.x + rect.w > p.x &&
                rect.y < p.y + p.h && rect.y + rect.h > p.y) {
                ok = false;
                break;
            }
        }
        if (!ok) continue;

        placed.push(rect);
        ctx.fillText(lb.name, lb.x, lb.y + fontSize);
        let lineY = lb.y + fontSize;
        // Отладочно (59a): тип профиля региона второй строкой, мельче.
        if (hasProfile) {
            ctx.font = `500 ${subFontSize}px system-ui`;
            ctx.fillStyle = `rgba(250,204,21,${0.75 * alpha})`; // приглушённый жёлтый
            lineY += subFontSize + 2;
            ctx.fillText(lb.profile, lb.x, lineY);
            ctx.font = `600 ${fontSize}px system-ui`;
            ctx.fillStyle = `rgba(226,232,240,${0.9 * alpha})`;
        }
        // Отладочно (расы): имя доминантной расы территории третьей строкой.
        if (hasRace) {
            ctx.font = `500 ${subFontSize}px system-ui`;
            ctx.fillStyle = `rgba(129,140,248,${0.75 * alpha})`; // приглушённый индиго
            lineY += subFontSize + 2;
            ctx.fillText(lb.race, lb.x, lineY);
            ctx.font = `600 ${fontSize}px system-ui`;
            ctx.fillStyle = `rgba(226,232,240,${0.9 * alpha})`;
        }
    }
}

// regionAtPoint — имя региона, в котором лежит точка (мировые координаты).
function regionAtPoint(regions, x, y) {
    if (!regions || regions.length < 2) return null;
    const cells = ensureVoronoi(regions);
    for (const cell of cells) {
        if (pointInPolygon(x, y, cell.poly)) return cell.name;
    }
    return null;
}

// pointInPolygon — принадлежность точки полигону (ray casting).
function pointInPolygon(x, y, poly) {
    let inside = false;
    for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
        const xi = poly[i].x, yi = poly[i].y;
        const xj = poly[j].x, yj = poly[j].y;
        if (((yi > y) !== (yj > y)) && (x < (xj - xi) * (y - yi) / (yj - yi) + xi)) {
            inside = !inside;
        }
    }
    return inside;
}

// polygonCentroid — геометрический центр (центроид) полигона по площади.
// Ячейки Вороного выпуклые → центроид всегда внутри территории. Вырожденный
// полигон (нулевая площадь) — падаем на первую вершину (страховка).
function polygonCentroid(poly) {
    let a = 0, cx = 0, cy = 0;
    for (let i = 0; i < poly.length; i++) {
        const j = (i + 1) % poly.length;
        const xi = poly[i].x, yi = poly[i].y;
        const xj = poly[j].x, yj = poly[j].y;
        const cross = xi * yj - xj * yi;
        a += cross;
        cx += (xi + xj) * cross;
        cy += (yi + yj) * cross;
    }
    a *= 0.5;
    if (Math.abs(a) < 1e-9) {
        return { x: poly[0].x, y: poly[0].y };
    }
    return { x: cx / (6 * a), y: cy / (6 * a) };
}

// ensureVoronoi — строит ячейки Вороного, только если список регионов изменился.
function ensureVoronoi(regions) {
    const key = regions.map(r => r.id).join(',');
    if (voronoiKey === key && voronoiCells) return voronoiCells;
    voronoiCells = buildVoronoi(regions);
    voronoiKey = key;
    return voronoiCells;
}

// buildVoronoi — диаграмма Вороного по центрам регионов.
// Ячейка сайта = пересечение полуплоскостей всех остальных сайтов
// (точки, которым этот сайт ближе любого соседа), обрезанное по
// окружности галактики.
function buildVoronoi(regions) {
    // Окружность галактики: радиус по самому дальнему центру + запас.
    let maxR = 1;
    for (const r of regions) {
        const d = Math.hypot(r.x, r.y);
        if (d > maxR) maxR = d;
    }
    const galaxyR = maxR * 1.02;
    const bbox = [];
    const N = 64;
    for (let k = 0; k < N; k++) {
        const a = (2 * Math.PI * k) / N;
        bbox.push({ x: galaxyR * Math.cos(a), y: galaxyR * Math.sin(a) });
    }

    const cells = [];
    for (let i = 0; i < regions.length; i++) {
        const s = regions[i];
        let poly = bbox.map(p => ({ x: p.x, y: p.y }));

        for (let j = 0; j < regions.length; j++) {
            if (i === j) continue;
            const t = regions[j];
            const dx = t.x - s.x;
            const dy = t.y - s.y;
            const c = (t.x * t.x + t.y * t.y - s.x * s.x - s.y * s.y) / 2;
            poly = clipHalfPlane(poly, dx, dy, c);
            if (poly.length < 3) break;
        }

        if (poly.length >= 3) {
            cells.push({ id: s.id, name: s.name, x: s.x, y: s.y, color: s.color, profile: s.profile || '', race: s.race_name || '', poly });
        }
    }
    return cells;
}

// clipHalfPlane — отсечение полигона полуплоскостью dx*x + dy*y <= c
// (алгоритм Сазерленда—Ходжмана).
function clipHalfPlane(poly, dx, dy, c) {
    const out = [];
    for (let i = 0; i < poly.length; i++) {
        const cur = poly[i];
        const prev = poly[(i + poly.length - 1) % poly.length];
        const curIn = dx * cur.x + dy * cur.y <= c;
        const prevIn = dx * prev.x + dy * prev.y <= c;

        if (curIn) {
            if (!prevIn) out.push(halfPlaneIntersect(prev, cur, dx, dy, c));
            out.push(cur);
        } else if (prevIn) {
            out.push(halfPlaneIntersect(prev, cur, dx, dy, c));
        }
    }
    return out;
}

// halfPlaneIntersect — точка пересечения отрезка [a, b] с линией dx*x + dy*y = c.
function halfPlaneIntersect(a, b, dx, dy, c) {
    const ad = dx * a.x + dy * a.y - c;
    const bd = dx * b.x + dy * b.y - c;
    const t = ad / (ad - bd);
    return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
}

// hexToRgba — '#aabbcc' + alpha → 'rgba(r,g,b,alpha)'.
function hexToRgba(hex, alpha) {
    let h = String(hex || '').replace('#', '');
    if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
    const n = parseInt(h, 16);
    if (isNaN(n)) return `rgba(124,108,255,${alpha})`;
    const r = (n >> 16) & 255;
    const g = (n >> 8) & 255;
    const b = n & 255;
    return `rgba(${r},${g},${b},${alpha})`;
}

// ==================== ПОЛЁТ ====================

function drawFlight(ctx, scale, flyFrom, flyTo, flyStartTime, flyDuration) {
    const elapsed = (Date.now() - flyStartTime) / 1000;
    const progress = Math.min(elapsed / flyDuration, 1);
    // Стартовая точка сегмента (61a): при редиректе — точка P маршрута,
    // не координаты мира отправления. Фолбэк на flyFrom, если не задана.
    const fromPos = worldToCanvas({
        coord_x: (typeof state.flyStartX === 'number') ? state.flyStartX : flyFrom.coord_x,
        coord_y: (typeof state.flyStartY === 'number') ? state.flyStartY : flyFrom.coord_y,
    });
    const toPos = worldToCanvas(flyTo);
    if (!isFiniteNumber(fromPos.x) || !isFiniteNumber(fromPos.y)) return;
    if (!isFiniteNumber(toPos.x) || !isFiniteNumber(toPos.y)) return;

    const x = fromPos.x + (toPos.x - fromPos.x) * progress;
    const y = fromPos.y + (toPos.y - fromPos.y) * progress;

    const angle = Math.atan2(toPos.y - fromPos.y, toPos.x - fromPos.x);

    // Пунктирная линия — от носа корабля к цели, а не от звезды.
    const shipSize = mapCfg.shipSize * scale;
    const noseOffset = shipSize * 1.1;
    const noseX = x + Math.cos(angle) * noseOffset;
    const noseY = y + Math.sin(angle) * noseOffset;
    ctx.beginPath();
    ctx.moveTo(noseX, noseY);
    ctx.lineTo(toPos.x, toPos.y);
    ctx.strokeStyle = 'rgba(251,191,36,0.45)';
    ctx.lineWidth = 2;
    ctx.setLineDash([6, 5]);
    ctx.stroke();
    ctx.setLineDash([]);

    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(angle);

    // Анимированное пламя двигателя.
    const flicker = 0.75 + 0.25 * Math.sin(elapsed * 25);
    const flameLen = shipSize * 1.1 * flicker;
    const flameGrad = ctx.createLinearGradient(-shipSize * 0.8, 0, -shipSize * 0.8 - flameLen, 0);
    flameGrad.addColorStop(0, 'rgba(147,197,253,0.9)');
    flameGrad.addColorStop(0.35, 'rgba(59,130,246,0.6)');
    flameGrad.addColorStop(1, 'rgba(59,130,246,0)');
    ctx.beginPath();
    ctx.moveTo(-shipSize * 0.8, -shipSize * 0.28);
    ctx.lineTo(-shipSize * 0.8 - flameLen, 0);
    ctx.lineTo(-shipSize * 0.8, shipSize * 0.28);
    ctx.closePath();
    ctx.fillStyle = flameGrad;
    ctx.fill();

    // Спрайт корабля: цельный PNG игрока (спека 61b §6.1) с перекраской;
    // спрайт не загружен/имя неизвестно → примитив-фолбэк (И4).
    const sprite = recolorShipSprite(state.userShipIcon, state.userShipColor);
    if (sprite) {
        const w = shipSize * 3.2;
        const h = shipSize * 3.2;
        ctx.drawImage(sprite, -w / 2, -h / 2, w, h);
    } else {
        const bodyGrad = ctx.createLinearGradient(0, -shipSize * 0.55, 0, shipSize * 0.55);
        bodyGrad.addColorStop(0, '#bfdbfe');
        bodyGrad.addColorStop(0.5, '#3b82f6');
        bodyGrad.addColorStop(1, '#1d4ed8');
        ctx.beginPath();
        ctx.moveTo(shipSize * 1.05, 0);
        ctx.lineTo(-shipSize * 0.6, -shipSize * 0.55);
        ctx.lineTo(-shipSize * 0.35, 0);
        ctx.lineTo(-shipSize * 0.6, shipSize * 0.55);
        ctx.closePath();
        ctx.fillStyle = bodyGrad;
        ctx.fill();
        ctx.strokeStyle = '#0f172a';
        ctx.lineWidth = 1;
        ctx.stroke();
    }

    ctx.restore();
}

// ==================== СТАТУС-БАР ====================

function updateStatusBar(statusBar, clusters, isFlying, flyStartTime, flyDuration) {
    if (isFlying) {
        const elapsed = (Date.now() - flyStartTime) / 1000;
        const remaining = Math.max(0, flyDuration - elapsed);
        statusBar.textContent = `🚀 В полёте... осталось ${Math.ceil(remaining)} сек.`;
        return;
    }

    const totalClusters = clusters.length;
    let totalWorlds = 0;
    let singles = 0;
    for (const c of clusters) {
        totalWorlds += c.cnt;
        if (c.cnt === 1) singles++;
    }
    let text = `${totalWorlds} миров в кадре (${singles} одиночных, ${totalClusters - singles} кластеров)`;

    // Имя региона под центром экрана — видно даже когда подписи пропали.
    const cx = (state.canvasWidth / 2 - state.offsetX) / state.scale;
    const cy = (state.canvasHeight / 2 - state.offsetY) / state.scale;
    const regionName = regionAtPoint(state.regions, cx, cy);
    if (regionName) {
        text += ` · Регион: ${regionName}`;
    }
    statusBar.textContent = text;
}

// ==================== УТИЛИТЫ ====================

export function hashString(s) {
    let hash = 0;
    for (let i = 0; i < s.length; i++) {
        hash = (hash * 31 + s.charCodeAt(i)) & 0xFFFFFFFF;
    }
    return hash;
}

// ==================== FPS-СЧЁТЧИК ====================
// Только для admin/skycomposer (инструмент создателя, идея 28a). EMA-сглаживание
// по времени между кадрами; рисование — в конце draw() (см. вызов выше), чтобы
// плашку не стирал clearRect ни одного из источников перерисовки. Для player —
// мгновенный return: ноль вычислений и отрисовки.
const FPS_ROLES = ['admin', 'skycomposer'];
const FPS_EMA_ALPHA = 0.1;
let fpsEMA = 60;
let fpsLastFrameAt = 0;

export function updateFpsCounter() {
    if (!FPS_ROLES.includes(state.userRole)) return;
    const now = performance.now();
    if (fpsLastFrameAt > 0) {
        const dt = now - fpsLastFrameAt;
        if (dt > 0) {
            const inst = 1000 / dt;
            fpsEMA = fpsEMA * (1 - FPS_EMA_ALPHA) + inst * FPS_EMA_ALPHA;
        }
    }
    fpsLastFrameAt = now;
}

// drawFpsOverlay — маленькая полупрозрачная плашка «N FPS» в левом нижнем углу.
function drawFpsOverlay(ctx, canvasWidth, canvasHeight) {
    if (!FPS_ROLES.includes(state.userRole)) return;
    const text = Math.round(fpsEMA) + ' FPS';
    const fontSize = 12;
    const pad = 8;
    ctx.font = `600 ${fontSize}px system-ui`;
    const w = ctx.measureText(text).width + 14;
    const h = fontSize + 10;
    const x = pad;
    const y = canvasHeight - h - pad;

    ctx.fillStyle = 'rgba(10,15,32,0.55)';
    ctx.fillRect(x, y, w, h);
    ctx.fillStyle = 'rgba(226,232,240,0.7)';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillText(text, x + 7, y + h / 2);
    ctx.textBaseline = 'alphabetic';
}
