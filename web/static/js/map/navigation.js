// web/static/js/map/navigation.js
import { state, elements } from './config.js';
import { draw } from './map_render.js';
import { formatZoom } from './utils.js';
import { notifyInfo } from '../ui/toast.js';
import { CONFIG } from '../config.js';

// centerOnAgent — центрирует карту на текущем мире игрока.
// Мир должен быть в state.worlds (его кладёт туда loadUserData
// через /worlds/{id}). Если мир не задан или не найден в кэше —
// фоллбэк на ближайшую загруженную звезду (B27), чтобы кнопка не молчала.
export function centerOnAgent() {
    // Если игрок в полёте — центрируем на текущей позиции по пути.
    // Ветка не зависит от currentWorldId: у нового игрока мир может быть NULL,
    // но полёт уже идёт и корабль есть.
    if (state.isFlying && state.flyFrom && state.flyTo && state.flyStartTime !== undefined && state.flyDuration > 0) {
        const elapsed = (Date.now() - state.flyStartTime) / 1000;
        const progress = Math.min(elapsed / state.flyDuration, 1);
        const fromX = state.flyFrom.coord_x;
        const fromY = state.flyFrom.coord_y;
        const toX = state.flyTo.coord_x;
        const toY = state.flyTo.coord_y;

        const worldX = fromX + (toX - fromX) * progress;
        const worldY = fromY + (toY - fromY) * progress;

        const targetScale = Math.max(state.scale, 1.0);
        state.offsetX = state.canvasWidth / 2 - worldX * targetScale;
        state.offsetY = state.canvasHeight / 2 - worldY * targetScale;
        state.scale = targetScale;

        if (elements.zoomInfo) {
            elements.zoomInfo.textContent = formatZoom(state.scale, state.minZoom || CONFIG.map.minZoom);
        }
        draw();
        return;
    }

    // Мир игрока: задан и найден в кэше — центрируем на нём; иначе
    // (мир не задан или не в кэше) — фоллбэк на ближайшую звезду.
    let world = null;
    if (state.currentWorldId) {
        world = (state.worlds || []).find(w => w.id === state.currentWorldId);
        if (world && (typeof world.coord_x !== 'number' || typeof world.coord_y !== 'number')) {
            console.warn('centerOnAgent: у мира нет координат');
            return;
        }
    }

    if (world) {
        centerViewport(world.coord_x, world.coord_y);
        return;
    }

    centerOnNearestStar();
}

// centerViewport — ставит центр экрана в мировую точку (x, y),
// сохраняя текущий зум (минимум 1.0), и перерисовывает карту.
function centerViewport(x, y) {
    const targetScale = Math.max(state.scale, 1.0);
    state.offsetX = state.canvasWidth / 2 - x * targetScale;
    state.offsetY = state.canvasHeight / 2 - y * targetScale;
    state.scale = targetScale;

    if (elements.zoomInfo) {
        elements.zoomInfo.textContent = formatZoom(state.scale, state.minZoom || CONFIG.map.minZoom);
    }
    draw();
}

// centerOnNearestStar — фоллбэк «Найти меня»: центрирует вьюпорт на
// ближайшей к центру экрана загруженной звезде (state.worlds и позиции
// кластеров state.clusters, x/y — реальная звезда). Звёзд нет вовсе — тост.
function centerOnNearestStar() {
    const worldX = (state.canvasWidth / 2 - state.offsetX) / state.scale;
    const worldY = (state.canvasHeight / 2 - state.offsetY) / state.scale;

    const candidates = [];
    for (const w of (state.worlds || [])) {
        if (typeof w.coord_x === 'number' && typeof w.coord_y === 'number') {
            candidates.push({ x: w.coord_x, y: w.coord_y });
        }
    }
    for (const c of (state.clusters || [])) {
        candidates.push({ x: c.x, y: c.y });
    }

    let best = null;
    let bestDist = Infinity;
    for (const s of candidates) {
        const dx = s.x - worldX;
        const dy = s.y - worldY;
        const d = dx * dx + dy * dy;
        if (d < bestDist) {
            bestDist = d;
            best = s;
        }
    }

    if (!best) {
        notifyInfo('Звёзды ещё не загружены — подвинь карту и попробуй снова');
        return;
    }

    centerViewport(best.x, best.y);
}
