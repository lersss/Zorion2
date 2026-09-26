// web/static/js/route/route_geometry.js
// Геометрия и утилиты страницы мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §6). Вынесено из route_config.js
// без изменения поведения.

// computeView — квадрат 1:1 (letterbox) в свободной области между HUD.
// Верх (паспорт+чип режима+статус) и низ (строка обратной связи/легенда +
// панель действий + «Проложить») зарезервированы; доска 1:1 по §6.
// n — из board.n (не хардкодить): рендер/ввод читают view.n.
export function computeView(vw, vh, n) {
    const top = 118;
    const bottom = 232;
    const avail = Math.max(120, vh - top - bottom);
    const size = Math.max(120, Math.min(vw - 24, avail));
    return { x0: (vw - size) / 2, y0: top + Math.max(0, (avail - size) / 2), size, n: n || 0 };
}

// hexA — '#rrggbb' → 'rgba(r,g,b,a)'.
export function hexA(hex, a) {
    const h = String(hex || '#ffffff').replace('#', '');
    const full = h.length === 3 ? h[0] + h[0] + h[1] + h[1] + h[2] + h[2] : h;
    const n = parseInt(full, 16);
    return 'rgba(' + ((n >> 16) & 255) + ',' + ((n >> 8) & 255) + ',' + (n & 255) + ',' + a + ')';
}

// mulberry32 — детерминированный RNG фона от seed.
export function mulberry32(seed) {
    let s = (seed >>> 0) || 1;
    return function () {
        s = (s + 0x6d2b79f5) | 0;
        let t = Math.imul(s ^ (s >>> 15), 1 | s);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

// hashSeed — числовой seed фона из fingerprint сегмента (косметика, детерминизм
// фона от сегмента; игровой истины не несёт).
export function hashSeed(str) {
    let h = 2166136261;
    const s = String(str || '');
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
    return h >>> 0;
}
