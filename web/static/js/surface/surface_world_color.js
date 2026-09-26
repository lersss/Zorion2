// resolvePalette — палитра вида (§3.4): обязателен base, остальные ключи
// выводятся из base; без рецепта — от пакетного biome_color. Значения — hex
// (shadeHex), чтобы работали rgba()/shade().
export function resolvePalette(view, color) {
    const src = (view && view.palette) || {};
    const base = src.base || color;
    return {
        base,
        dark: src.dark || shadeHex(base, 0.62),
        light: src.light || shadeHex(base, 1.15),
        accent: src.accent || shadeHex(base, 0.85),
        rock: src.rock || shadeHex(base, 0.55),
        trunk: src.trunk || shadeHex(base, 0.5),
        moss: src.moss || src.accent || shadeHex(base, 1.3),
        snow: src.snow || '#eef3f7',
        snowShade: src.snowShade || '#b3c6d6',
        ice: src.ice || '#a9c6dc',
        haze: src.haze || null,
        glow: src.glow || src.accent || shadeHex(base, 1.4),
        ember: src.ember || src.accent || base,
    };
}

// Утилиты цвета (палитра биома → оттенки рельефа).
export function parseHex(hex) {
    const h = hex && hex[0] === '#' ? hex.slice(1) : '8a7a6a';
    return {
        r: parseInt(h.slice(0, 2), 16) || 0,
        g: parseInt(h.slice(2, 4), 16) || 0,
        b: parseInt(h.slice(4, 6), 16) || 0,
    };
}

export function shade(hex, factor) {
    const c = parseHex(hex);
    const f = (v) => Math.max(0, Math.min(255, Math.round(v * factor)));
    return `rgb(${f(c.r)},${f(c.g)},${f(c.b)})`;
}

// shadeHex — как shade(), но возвращает hex: производные ключи палитры (§3.4)
// должны оставаться hex, иначе rgba()/shade() по ним ломаются.
export function shadeHex(hex, factor) {
    const c = parseHex(hex);
    const f = (v) => Math.max(0, Math.min(255, Math.round(v * factor)));
    const to = (v) => f(v).toString(16).padStart(2, '0');
    return '#' + to(c.r) + to(c.g) + to(c.b);
}

export function rgba(hex, alpha) {
    const c = parseHex(hex);
    return `rgba(${c.r},${c.g},${c.b},${alpha})`;
}
