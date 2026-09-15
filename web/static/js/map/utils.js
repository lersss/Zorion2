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

// getStarColor — цвет звезды. Экзотика (star_type ≠ star) — свой цвет
// (99.2.4 §8), ветка по star_type раньше ветки по спектральному классу;
// «прочая экзотика» (сверхгиганты, star_type='star') — по классу O–A.
export function getStarColor(spectralClass, starType) {
    const colors = CONFIG.map.starColors;
    if (starType && colors[starType]) return colors[starType];
    return colors[spectralClass] || colors.default;
}

// getStarShade — цвет звезды с градацией по светимости: внутри спектрального
// класса положение по температуре даёт лёгкий сдвиг светимости (положение в
// диапазоне starTempRanges → ±половина spread по L в HSL). Звёзды одного
// класса перестают быть одинаковыми. Без температуры — базовый цвет класса.
// Экзотика — фиксированный цвет: градация не нужна (у ЧД T=0, «тёмная»).
export function getStarShade(spectralClass, temperature, starType) {
    if (starType && CONFIG.map.starColors[starType]) {
        return getStarColor(spectralClass, starType);
    }
    const ranges = CONFIG.map.starTempRanges;
    const rng = ranges && ranges[spectralClass];
    if (!rng || typeof temperature !== 'number' || !isFinite(temperature)) {
        return getStarColor(spectralClass, starType);
    }
    const span = rng[1] - rng[0];
    if (!(span > 0)) return getStarColor(spectralClass, starType);
    const p = Math.min(Math.max((temperature - rng[0]) / span, 0), 1);
    const { h, s, l } = hexToHsl(getStarColor(spectralClass, starType));
    const spread = 0.16; // небольшой разбег по светимости внутри класса
    const nl = Math.min(Math.max(l + (p - 0.5) * spread, 0.10), 0.90);
    return `hsl(${Math.round(h)}, ${(s * 100).toFixed(1)}%, ${(nl * 100).toFixed(1)}%)`;
}

// hexToHsl — '#aabbcc' → { h, s, l } (h в градусах, s/l в 0..1).
function hexToHsl(hex) {
    let hh = String(hex || '').replace('#', '');
    if (hh.length === 3) hh = hh[0] + hh[0] + hh[1] + hh[1] + hh[2] + hh[2];
    const n = parseInt(hh, 16);
    if (isNaN(n)) return { h: 260, s: 0.75, l: 0.5 };
    const r = ((n >> 16) & 255) / 255;
    const g = ((n >> 8) & 255) / 255;
    const b = (n & 255) / 255;
    const max = Math.max(r, g, b);
    const min = Math.min(r, g, b);
    const l = (max + min) / 2;
    let h = 0;
    let s = 0;
    if (max !== min) {
        const d = max - min;
        s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
        switch (max) {
            case r: h = (g - b) / d + (g < b ? 6 : 0); break;
            case g: h = (b - r) / d + 2; break;
            default: h = (r - g) / d + 4;
        }
        h *= 60;
    }
    return { h, s, l };
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