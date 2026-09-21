// web/static/js/map/events.js
import { state, elements } from './config.js';
import { isFiniteNumber, formatZoom } from './utils.js';
import { draw, clusterScreenRadius } from './map_render.js';
import { starHitRadius } from './star_render.js';
import { scheduleReload } from './data.js';
import { centerOnAgent } from './navigation.js';
import { CONFIG } from '../config.js';
import { openSystemModal } from '../modal/index.js';
import { startFlight } from './flight.js';
import { notifyError } from '../ui/toast.js';
import { findNPCAgentAt, showNPCTooltip, hideNPCTooltip, clearNPCHighlight } from './npc_agents.js';

const { map: mapCfg } = CONFIG;

// ==================== СОХРАНЕНИЕ VIEWPORT ====================

function saveViewport() {
    try {
        sessionStorage.setItem('viewport', JSON.stringify({
            offsetX: state.offsetX,
            offsetY: state.offsetY,
            scale: state.scale
        }));
    } catch (e) { /* ignore */ }
}

// ==================== ПОИСК КЛАСТЕРА ПОД КУРСОРОМ ====================

function findClusterAt(mouseX, mouseY) {
    const clusters = state.clusters || [];
    let found = null;
    let bestDist = Infinity;

    for (const c of clusters) {
        const px = c.x * state.scale + state.offsetX;
        const py = c.y * state.scale + state.offsetY;
        if (!isFiniteNumber(px) || !isFiniteNumber(py)) continue;

        // Зона клика/наведения — по ВИДИМОМУ размеру звезды (ореол с потолком),
        // а не по растущему линейно clusterScreenRadius (решение создателя
        // 2026-09-22, идея «внешний вид звёзд» §Решения по гейту п.2).
        const radius = starHitRadius(c, clusterScreenRadius(c));
        const dist = Math.hypot(mouseX - px, mouseY - py);
        const hitRadius = Math.max(radius + 3, mapCfg.minDistForClick);

        if (dist < hitRadius && dist < bestDist) {
            bestDist = dist;
            found = { cluster: c, screenX: px, screenY: py };
        }
    }
    return found;
}

// ==================== HOVER ====================

export function initHover() {
    elements.canvas.addEventListener('mousemove', (e) => {
        const rect = elements.canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (elements.canvas.width / rect.width);
        const mouseY = (e.clientY - rect.top) * (elements.canvas.height / rect.height);

        // Тултип агента по наведению (спека 26a.1, правка создателя): иконка
        // агента поверх звёзд — приоритет над тултипом мира. Увёл мышь с
        // иконки — тултип скрыт (наведение ≠ клик; pointer-events: none —
        // клики/драг карты не перехватываются).
        const agent = findNPCAgentAt(mouseX, mouseY);
        if (agent) {
            showNPCTooltip(agent, e.clientX, e.clientY);
            if (state.hoveredWorldId !== null) {
                state.hoveredWorldId = null;
                elements.tooltip.classList.remove('active');
                draw(); // убрать hover-кольцо звезды под тултипом агента
            }
            return;
        }
        hideNPCTooltip();

        const hit = findClusterAt(mouseX, mouseY);
        // Hover-подсветка — только для одиночной звезды (cnt===1): у кластера
        // нет данных отдельной звезды. Вид звезды открыт везде (спека 77a §5.5).
        const newHoveredId = hit && hit.cluster.cnt === 1 ? hit.cluster.sid : null;

        if (state.hoveredWorldId !== newHoveredId) {
            state.hoveredWorldId = newHoveredId;
            elements.canvas.style.cursor = hit ? 'pointer' : 'crosshair';
            showTooltip(hit ? hit.cluster : null);
            draw();
        } else if (hit) {
            elements.canvas.style.cursor = 'pointer';
        }
    });

    elements.canvas.addEventListener('mouseleave', () => {
        hideNPCTooltip();
        if (state.hoveredWorldId !== null) {
            state.hoveredWorldId = null;
            elements.canvas.style.cursor = 'crosshair';
            showTooltip(null);
            draw();
        }
    });
}

// showTooltip — заполняет и показывает тултип для одиночного мира, скрывает иначе.
function showTooltip(cluster) {
    if (!cluster || cluster.cnt !== 1) {
        elements.tooltip.classList.remove('active');
        return;
    }
    const name = cluster.sname || '—';
    const spec = cluster.sspec || 'G';
    const level = cluster.level !== undefined && cluster.level !== null ? cluster.level : '—';

    elements.tooltipName.textContent = name;
    // Тип человекочитаемо: обычная звезда — спектральный класс («Тип: G»),
    // экзотика — русское название типа («Тип: чёрная дыра», не raw stype).
    elements.tooltipType.textContent = 'Тип: ' + tooltipStarType(cluster.stype, spec);
    elements.tooltipLevel.textContent = 'Уровень: ' + level;
    elements.tooltipFlyBtn.dataset.worldId = cluster.sid;
    elements.tooltipFlyBtn.style.display = '';
    elements.tooltip.classList.add('active');
}

// tooltipStarType — человекочитаемый тип звезды для тултипа (99.2.4 §2, §8).
// stype === 'star' (или пусто) — спектральный класс; экзотика — русское имя.
const STAR_TYPE_LABELS = {
    black_hole: 'чёрная дыра',
    neutron: 'нейтронная звезда',
    white_dwarf: 'белый карлик',
    protostar: 'протозвезда',
};

function tooltipStarType(stype, spec) {
    if (!stype || stype === 'star') return spec;
    return STAR_TYPE_LABELS[stype] || stype;
}

// ==================== CLICK ====================

export function handleCanvasClick(e) {
    if (state.isDragging) return;
    if (state.dragStartX !== undefined && state.dragStartY !== undefined) {
        const dx = e.clientX - state.dragStartX;
        const dy = e.clientY - state.dragStartY;
        if (Math.hypot(dx, dy) > 5) return;
    }

    const rect = elements.canvas.getBoundingClientRect();
    const mouseX = (e.clientX - rect.left) * (elements.canvas.width / rect.width);
    const mouseY = (e.clientY - rect.top) * (elements.canvas.height / rect.height);

    // Данные агента — тултип по наведению (initHover), клик по иконке
    // проходит к звёздам (модалка мира под агентом).
    const hit = findClusterAt(mouseX, mouseY);

    if (!hit) {
        elements.tooltip.classList.remove('active');
        state.selectedWorldId = null;
        clearNPCHighlight(); // клик по пустому месту сбрасывает подсветку агента (спека 26a.1 §6.2)
        return;
    }

    const c = hit.cluster;

    if (c.cnt === 1) {
        // Одиночный мир — открываем модалку. sspec без фолбека на 'G':
        // у экзотики он пустой (NULL), фолбек врал бы «Жёлтый карлик» (баг #1).
        // starInfo — открытая информация о звезде (спека 77a §5.5): при 403
        // (система вне радиуса/знания) модалка откроется с карточкой звезды.
        if (typeof openSystemModal === 'function') {
            openSystemModal(c.sid, c.sname || '—', c.sspec || '', null, null, {
                stype: c.stype,
                stemp: c.stemp,
                systype: c.systype,
                smods: c.smods,
                x: c.x,
                y: c.y,
                hasEngine: state.hasEngine, // спека 91a §6.1: блок «Перелететь» в модалке
                // Спрайт игрока для маркера «я здесь»/корабля в полёте (спека
                // 99.2.27 §5.8/§5.11): та же иконка/цвет, что на карте.
                shipIcon: state.userShipIcon,
                shipColor: state.userShipColor,
            });
        }
        elements.tooltip.classList.remove('active');
        state.selectedWorldId = c.sid;
    } else {
        // Кластер — зуммируем к его центру
        const targetScale = Math.min(state.scale * 2, mapCfg.maxZoom);
        const worldX = c.x;
        const worldY = c.y;
        state.scale = targetScale;
        state.offsetX = state.canvasWidth / 2 - worldX * targetScale;
        state.offsetY = state.canvasHeight / 2 - worldY * targetScale;
        if (elements.zoomInfo) {
            elements.zoomInfo.textContent = Math.round(targetScale * 100) + '%';
        }
        draw();
        saveViewport();
        scheduleReload();
    }
}

// ==================== КНОПКА "ЛЕТЕТЬ" И КОНТЕКСТНОЕ МЕНЮ ====================

export function initFlyBtn() {
    elements.tooltipFlyBtn.addEventListener('click', async function (e) {
        e.stopPropagation();
        // Без двигателя полёт невозможен (спека 91a §6.1): блокируем с
        // подсказкой; сервер валидирует тоже (админ/skycomposer — исключение).
        if (!state.hasEngine) {
            notifyError('Двигатель не установлен — полёт невозможен');
            return;
        }
        const worldId = this.dataset.worldId;
        const token = localStorage.getItem('token');
        await startFlight(worldId, token);
    });
}

// ==================== КОНТЕКСТНОЕ МЕНЮ (ПКМ по миру на карте) ====================

// coordsTooltipEl — тултип координат точки (ПКМ по пустому месту).
// Создаётся лениво один раз; pointer-events: none — не перехватывает клики.
// Минимальное время жизни 3 с (правка создателя 2026-09-16): после показа
// тултип не исчезает, даже если мышь ушла, — прячется по таймеру, а не по
// mousemove. Жёсткое скрытие (ЛКМ, ПКМ по звезде) — сразу, hideCoordsTooltip.
const COORDS_TOOLTIP_MIN_MS = 3000;
let coordsTooltipEl = null;
let coordsShownAt = 0;
let coordsHideTimer = null;

function showCoordsTooltip(screenX, screenY, worldX, worldY) {
    if (coordsHideTimer) { clearTimeout(coordsHideTimer); coordsHideTimer = null; }
    if (!coordsTooltipEl) {
        coordsTooltipEl = document.createElement('div');
        coordsTooltipEl.id = 'coords-tooltip';
        coordsTooltipEl.style.cssText = `
            position: fixed;
            pointer-events: none;
            z-index: 1100;
            background: #1a1a2e;
            border: 1px solid #334155;
            border-radius: 8px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.5);
            padding: 8px 10px;
            font-size: 0.85rem;
            color: #e0e0e0;
            user-select: none;
            white-space: nowrap;
        `;
        document.body.appendChild(coordsTooltipEl);
    }
    coordsTooltipEl.textContent = `Координаты: (${worldX}; ${worldY})`;
    // Смещение от курсора (14px); у правого края — влево, чтобы не уходить за экран.
    const left = screenX + 14 + coordsTooltipEl.offsetWidth > window.innerWidth
        ? screenX - coordsTooltipEl.offsetWidth - 14
        : screenX + 14;
    coordsTooltipEl.style.left = Math.max(4, left) + 'px';
    coordsTooltipEl.style.top = Math.max(4, screenY - 10) + 'px';
    coordsShownAt = Date.now();
}

function hideCoordsTooltip() {
    if (coordsHideTimer) { clearTimeout(coordsHideTimer); coordsHideTimer = null; }
    if (coordsTooltipEl) coordsTooltipEl.remove();
    coordsTooltipEl = null;
}

// hideCoordsTooltipGuarded — «мягкое» скрытие (мышь ушла/двинулась): не
// раньше минимального времени жизни; позже — как обычное скрытие.
function hideCoordsTooltipGuarded() {
    if (!coordsTooltipEl || coordsHideTimer) return;
    const remaining = COORDS_TOOLTIP_MIN_MS - (Date.now() - coordsShownAt);
    if (remaining > 0) {
        coordsHideTimer = setTimeout(() => {
            coordsHideTimer = null;
            if (coordsTooltipEl) hideCoordsTooltip();
        }, remaining);
        return;
    }
    hideCoordsTooltip();
}

export function initContextMenu() {
    // ПКМ (нажатие) по пустому месту — тултип координат сразу, на нажатии,
    // а не на отпускании (contextmenu) — правка создателя 2026-09-16.
    elements.canvas.addEventListener('mousedown', (e) => {
        if (e.button !== 2) return;
        const rect = elements.canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (elements.canvas.width / rect.width);
        const mouseY = (e.clientY - rect.top) * (elements.canvas.height / rect.height);

        const hit = findClusterAt(mouseX, mouseY);
        if (hit && hit.cluster.cnt === 1) return; // звезда — меню мира на contextmenu
        hideWorldMenu();
        const worldX = Math.round((mouseX - state.offsetX) / state.scale);
        const worldY = Math.round((mouseY - state.offsetY) / state.scale);
        showCoordsTooltip(e.clientX, e.clientY, worldX, worldY);
    });

    // ПКМ по канвасу: браузерное меню подавляем всегда; по звезде — меню мира.
    elements.canvas.addEventListener('contextmenu', (e) => {
        e.preventDefault();
        const rect = elements.canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (elements.canvas.width / rect.width);
        const mouseY = (e.clientY - rect.top) * (elements.canvas.height / rect.height);

        const hit = findClusterAt(mouseX, mouseY);
        if (hit && hit.cluster.cnt === 1) {
            hideCoordsTooltip();
            showWorldMenu(e.clientX, e.clientY, hit.cluster.sid, hit.cluster.sname || 'Мир');
        } else {
            hideWorldMenu();
            // Страховка: тултип обычно показан уже по mousedown; если нажатие
            // не дошло до канваса (наведено на тултип звезды) — показываем здесь.
            if (!coordsTooltipEl) {
                const worldX = Math.round((mouseX - state.offsetX) / state.scale);
                const worldY = Math.round((mouseY - state.offsetY) / state.scale);
                showCoordsTooltip(e.clientX, e.clientY, worldX, worldY);
            }
        }
    });

    // Мышь двинулась/ушла — «мягкое» скрытие: не раньше 3 с жизни тултипа.
    elements.canvas.addEventListener('mousemove', () => hideCoordsTooltipGuarded());
    elements.canvas.addEventListener('mouseleave', () => hideCoordsTooltipGuarded());

    // ПКМ по тултипу: активный тултип перехватывает pointer-events,
    // без этого браузерное меню откроется вместо нашего.
    elements.tooltip.addEventListener('contextmenu', (e) => {
        const worldId = elements.tooltipFlyBtn.dataset.worldId;
        if (!worldId) return;
        e.preventDefault();
        hideCoordsTooltip(); // жёстко: ПКМ по тултипу звезды — новое действие (меню мира)
        showWorldMenu(e.clientX, e.clientY, worldId, elements.tooltipName.textContent || 'Мир');
    });

    // Левая кнопка вне меню — скрывает меню мира и тултип координат
    // (жёстко: ЛКМ — новое действие). ПКМ — отдаём канвасу.
    document.addEventListener('mousedown', (e) => {
        if (e.button !== 2 && !e.target.closest('#map-context-menu')) hideWorldMenu();
        if (e.button !== 2) hideCoordsTooltip();
    });
}

function showWorldMenu(x, y, worldId, name) {
    hideWorldMenu();

    const menu = document.createElement('div');
    menu.id = 'map-context-menu';
    menu.style.cssText = `
        position: fixed;
        left: ${x}px;
        top: ${y}px;
        background: #1a1a2e;
        border: 1px solid #334155;
        border-radius: 8px;
        box-shadow: 0 8px 24px rgba(0,0,0,0.5);
        padding: 4px;
        min-width: 180px;
        z-index: 1100;
        font-size: 0.9rem;
        color: #e0e0e0;
        user-select: none;
    `;

    const title = document.createElement('div');
    title.style.cssText = `
        padding: 6px 10px;
        font-size: 0.75rem;
        color: #888;
        border-bottom: 1px solid #2a2a44;
        margin-bottom: 4px;
    `;
    title.textContent = name;
    menu.appendChild(title);

    const flyBtn = document.createElement('div');
    flyBtn.style.cssText = `
        padding: 8px 10px;
        cursor: pointer;
        border-radius: 6px;
        display: flex;
        align-items: center;
        gap: 8px;
    `;
    flyBtn.innerHTML = `🚀 <span>Лететь</span>`;
    flyBtn.addEventListener('mouseenter', () => { flyBtn.style.background = '#2a2a44'; });
    flyBtn.addEventListener('mouseleave', () => { flyBtn.style.background = 'none'; });
    flyBtn.addEventListener('click', async () => {
        hideWorldMenu();
        // Без двигателя полёт невозможен (спека 91a §6.1): блокируем с
        // подсказкой; сервер валидирует тоже (админ/skycomposer — исключение).
        if (!state.hasEngine) {
            notifyError('Двигатель не установлен — полёт невозможен');
            return;
        }
        const token = localStorage.getItem('token');
        await startFlight(worldId, token);
        draw();
    });
    menu.appendChild(flyBtn);

    document.body.appendChild(menu);
}

function hideWorldMenu() {
    const menu = document.getElementById('map-context-menu');
    if (menu) menu.remove();
}

// ==================== PAN / ZOOM ====================

export function initPanZoom() {
    elements.canvas.addEventListener('mousedown', (e) => {
        // Драг — только левой кнопкой: ПКМ занят меню мира и тултипом
        // координат (идея 47a), драг правой был дублем левой.
        if (e.target === elements.canvas && e.button === 0) {
            state.isDragging = true;
            state.dragStartX = e.clientX;
            state.dragStartY = e.clientY;
            state.dragStartOffsetX = state.offsetX;
            state.dragStartOffsetY = state.offsetY;
            elements.canvas.style.cursor = 'grabbing';
            stopFollow();
        }
    });

    window.addEventListener('mousemove', (e) => {
        if (state.isDragging) {
            const dx = e.clientX - state.dragStartX;
            const dy = e.clientY - state.dragStartY;
            state.offsetX = state.dragStartOffsetX + dx;
            state.offsetY = state.dragStartOffsetY + dy;
            draw();
        }
    });

    window.addEventListener('mouseup', () => {
        if (state.isDragging) {
            state.isDragging = false;
            if (state.hoveredWorldId === null) {
                elements.canvas.style.cursor = 'crosshair';
            } else {
                elements.canvas.style.cursor = 'pointer';
            }
            saveViewport();
            scheduleReload();
        }
    });

    elements.canvas.addEventListener('wheel', (e) => {
        e.preventDefault();
        stopFollow();
        const rect = elements.canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (elements.canvas.width / rect.width);
        const mouseY = (e.clientY - rect.top) * (elements.canvas.height / rect.height);

        const minZ = state.minZoom || mapCfg.minZoom;
        let delta = e.deltaY > 0 ? mapCfg.wheelSensitivity : 1 / mapCfg.wheelSensitivity;
        if (e.deltaY > 0 && state.scale > minZ) {
            // Ускорение отдаления при приближении к «Галактика»:
            // чем ближе к минимуму, тем крупнее шаг, чтобы не крутить колесо десятки раз.
            const ratio = state.scale / minZ;
            if (ratio < 60) {
                const t = Math.max(0, 1 - ratio / 60);
                delta = Math.pow(mapCfg.wheelSensitivity, 1 + 2.5 * t);
            }
        }
        const newScale = Math.min(Math.max(state.scale * delta, minZ), mapCfg.maxZoom);
        if (newScale === state.scale) return;

        const worldX = (mouseX - state.offsetX) / state.scale;
        const worldY = (mouseY - state.offsetY) / state.scale;
        state.scale = newScale;
        state.offsetX = mouseX - worldX * state.scale;
        state.offsetY = mouseY - worldY * state.scale;

        if (elements.zoomInfo) elements.zoomInfo.textContent = formatZoom(state.scale, minZ);
        draw();
        saveViewport();
        scheduleReload();
    }, { passive: false });

    document.getElementById('zoomInBtn').addEventListener('click', () => {
        const centerX = state.canvasWidth / 2;
        const centerY = state.canvasHeight / 2;
        const worldX = (centerX - state.offsetX) / state.scale;
        const worldY = (centerY - state.offsetY) / state.scale;
        state.scale = Math.min(state.scale * mapCfg.zoomStep, mapCfg.maxZoom);
        state.offsetX = centerX - worldX * state.scale;
        state.offsetY = centerY - worldY * state.scale;
        if (elements.zoomInfo) elements.zoomInfo.textContent = formatZoom(state.scale, state.minZoom || mapCfg.minZoom);
        draw();
        saveViewport();
        scheduleReload();
    });

    document.getElementById('zoomOutBtn').addEventListener('click', () => {
        const centerX = state.canvasWidth / 2;
        const centerY = state.canvasHeight / 2;
        const worldX = (centerX - state.offsetX) / state.scale;
        const worldY = (centerY - state.offsetY) / state.scale;
        const minZ = state.minZoom || mapCfg.minZoom;
        // Быстрый скачок к «Галактика», когда уже близко к минимуму.
        let next = state.scale / mapCfg.zoomStep;
        if (next < minZ * 4) next = minZ;
        state.scale = Math.max(next, minZ);
        state.offsetX = centerX - worldX * state.scale;
        state.offsetY = centerY - worldY * state.scale;
        if (elements.zoomInfo) elements.zoomInfo.textContent = formatZoom(state.scale, minZ);
        draw();
        saveViewport();
        scheduleReload();
    });

    document.getElementById('centerBtn').addEventListener('click', () => {
        state.followShip = true;
        // Идея 42a: след слежения пишем только в полёте (вне полёта флаг
        // бесполезен — иначе при следующем полёте + рефреше слежение
        // включилось бы без нажатия). Вне полёта — зачищаем след.
        if (state.isFlying) {
            sessionStorage.setItem('followShip', '1');
        } else {
            sessionStorage.removeItem('followShip');
        }
        centerOnAgent();
        setCenterBtnActive(state.isFlying);
    });

    // Любое взаимодействие с картой (панорамирование/зум) отключает слежение
    function stopFollow() {
        if (state.followShip) {
            state.followShip = false;
            sessionStorage.removeItem('followShip');
            setCenterBtnActive(false);
        }
    }

    // Визуально отмечает активную кнопку «Найти меня»
    function setCenterBtnActive(active) {
        const btn = document.getElementById('centerBtn');
        if (btn) btn.classList.toggle('active', active);
    }
}