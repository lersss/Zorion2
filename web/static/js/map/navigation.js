// web/static/js/map/navigation.js
import { state, elements } from './config.js';
import { draw } from './map_render.js';

// centerOnAgent — центрирует карту на текущем мире игрока.
// Мир должен быть в state.worlds (его кладёт туда loadUserData
// через /worlds/{id}). Если не найден — молча выходим.
export function centerOnAgent() {
    if (!state.currentWorldId) {
        console.warn('centerOnAgent: currentWorldId не задан');
        return;
    }

    // Если игрок в полёте — центрируем на текущей позиции по пути
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
            elements.zoomInfo.textContent = Math.round(state.scale * 100) + '%';
        }
        draw();
        return;
    }

    const world = (state.worlds || []).find(w => w.id === state.currentWorldId);
    if (!world) {
        console.warn('centerOnAgent: мир', state.currentWorldId, 'не найден в кэше');
        return;
    }

    if (typeof world.coord_x !== 'number' || typeof world.coord_y !== 'number') {
        console.warn('centerOnAgent: у мира нет координат');
        return;
    }

    const targetScale = Math.max(state.scale, 1.0);
    state.offsetX = state.canvasWidth / 2 - world.coord_x * targetScale;
    state.offsetY = state.canvasHeight / 2 - world.coord_y * targetScale;
    state.scale = targetScale;

    if (elements.zoomInfo) {
        elements.zoomInfo.textContent = Math.round(state.scale * 100) + '%';
    }
    draw();
}