// web/static/js/route/route_render_geom.js
// Геометрия доски мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.4). Вынесено из route_render.js
// без изменения поведения.

// cellRect — прямоугольник клетки индекса cell на доске n×n.
export function cellRect(view, cell) {
    const cs = view.size / view.n;
    const i = cell % view.n;
    const j = (cell / view.n) | 0;
    return { x: view.x0 + i * cs, y: view.y0 + j * cs, w: cs, h: cs };
}
