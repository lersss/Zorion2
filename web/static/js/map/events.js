// web/static/js/map/events.js
import { state, elements } from './config.js';
import { isFiniteNumber } from './utils.js';
import { draw, clusterScreenRadius } from './map_render.js';
import { scheduleReload } from './data.js';
import { centerOnAgent } from './navigation.js';
import { CONFIG } from '../config.js';
import { openSystemModal } from '../modal/index.js';
import { startFlight } from './flight.js';

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

        const radius = clusterScreenRadius(c);
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

        const hit = findClusterAt(mouseX, mouseY);
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
    elements.tooltipType.textContent = 'Тип: ' + (cluster.stype || spec);
    elements.tooltipLevel.textContent = 'Уровень: ' + level;
    elements.tooltipFlyBtn.dataset.worldId = cluster.sid;
    elements.tooltip.classList.add('active');
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

    const hit = findClusterAt(mouseX, mouseY);

    if (!hit) {
        elements.tooltip.classList.remove('active');
        state.selectedWorldId = null;
        return;
    }

    const c = hit.cluster;

    if (c.cnt === 1) {
        // Одиночный мир — открываем модалку
        if (typeof openSystemModal === 'function') {
            openSystemModal(c.sid, c.sname || '—', c.sspec || 'G');
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
        const worldId = this.dataset.worldId;
        const token = localStorage.getItem('token');
        await startFlight(worldId, token);
    });
}

// ==================== КОНТЕКСТНОЕ МЕНЮ (ПКМ по миру на карте) ====================

export function initContextMenu() {
    // ПКМ по канвасу — по миру.
    elements.canvas.addEventListener('contextmenu', (e) => {
        const rect = elements.canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (elements.canvas.width / rect.width);
        const mouseY = (e.clientY - rect.top) * (elements.canvas.height / rect.height);

        const hit = findClusterAt(mouseX, mouseY);
        if (hit && hit.cluster.cnt === 1) {
            e.preventDefault();
            showWorldMenu(e.clientX, e.clientY, hit.cluster.sid, hit.cluster.sname || 'Мир');
        } else {
            hideWorldMenu();
        }
    });

    // ПКМ по тултипу: активный тултип перехватывает pointer-events,
    // без этого браузерное меню откроется вместо нашего.
    elements.tooltip.addEventListener('contextmenu', (e) => {
        const worldId = elements.tooltipFlyBtn.dataset.worldId;
        if (!worldId) return;
        e.preventDefault();
        showWorldMenu(e.clientX, e.clientY, worldId, elements.tooltipName.textContent || 'Мир');
    });

    // Левая кнопка вне меню — скрывает. ПКМ — отдаём канвасу/тултипу.
    document.addEventListener('mousedown', (e) => {
        if (e.button !== 2 && !e.target.closest('#map-context-menu')) hideWorldMenu();
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
    flyBtn.innerHTML = `🚀 <span>Перелететь</span>`;
    flyBtn.addEventListener('mouseenter', () => { flyBtn.style.background = '#2a2a44'; });
    flyBtn.addEventListener('mouseleave', () => { flyBtn.style.background = 'none'; });
    flyBtn.addEventListener('click', async () => {
        hideWorldMenu();
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
        if (e.target === elements.canvas) {
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

        const delta = e.deltaY > 0 ? mapCfg.wheelSensitivity : 1 / mapCfg.wheelSensitivity;
        const newScale = Math.min(Math.max(state.scale * delta, mapCfg.minZoom), mapCfg.maxZoom);
        if (newScale === state.scale) return;

        const worldX = (mouseX - state.offsetX) / state.scale;
        const worldY = (mouseY - state.offsetY) / state.scale;
        state.scale = newScale;
        state.offsetX = mouseX - worldX * state.scale;
        state.offsetY = mouseY - worldY * state.scale;

        if (elements.zoomInfo) elements.zoomInfo.textContent = Math.round(state.scale * 100) + '%';
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
        if (elements.zoomInfo) elements.zoomInfo.textContent = Math.round(state.scale * 100) + '%';
        draw();
        saveViewport();
        scheduleReload();
    });

    document.getElementById('zoomOutBtn').addEventListener('click', () => {
        const centerX = state.canvasWidth / 2;
        const centerY = state.canvasHeight / 2;
        const worldX = (centerX - state.offsetX) / state.scale;
        const worldY = (centerY - state.offsetY) / state.scale;
        state.scale = Math.max(state.scale / mapCfg.zoomStep, mapCfg.minZoom);
        state.offsetX = centerX - worldX * state.scale;
        state.offsetY = centerY - worldY * state.scale;
        if (elements.zoomInfo) elements.zoomInfo.textContent = Math.round(state.scale * 100) + '%';
        draw();
        saveViewport();
        scheduleReload();
    });

    document.getElementById('centerBtn').addEventListener('click', () => {
        state.followShip = true;
        centerOnAgent();
        setCenterBtnActive(state.isFlying);
    });

    // Любое взаимодействие с картой (панорамирование/зум) отключает слежение
    function stopFollow() {
        if (state.followShip) {
            state.followShip = false;
            setCenterBtnActive(false);
        }
    }

    // Визуально отмечает активную кнопку «Найти меня»
    function setCenterBtnActive(active) {
        const btn = document.getElementById('centerBtn');
        if (btn) btn.classList.toggle('active', active);
    }
}