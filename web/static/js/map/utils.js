import { state } from './config.js';
import { CONFIG } from '../config.js';

export function isFiniteNumber(v) {
    return typeof v === 'number' && isFinite(v);
}

export function worldToCanvas(world) {
    return {
        x: world.coord_x * state.scale + state.offsetX,
        y: world.coord_y * state.scale + state.offsetY
    };
}

export function getStarColor(spectralClass) {
    const colors = CONFIG.map.starColors;
    return colors[spectralClass] || colors.default;
}

// formatZoom — читаемое значение зума. Никогда не «0%»:
// на масштабе «вся галактика» — подпись «Галактика», иначе минимум 1%.
export function formatZoom(scale, minZoom) {
    if (minZoom && scale <= minZoom) return 'Галактика';
    return Math.max(1, Math.round(scale * 100)) + '%';
}

// для обратной совместимости (пока не удалим старый код)
export function getColorByType(type) {
    return CONFIG.map.starColors.default;
}