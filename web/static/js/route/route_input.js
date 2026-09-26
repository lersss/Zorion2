// web/static/js/route/route_input.js
// Ввод мини-игры «Прокладка маршрута» на доске v9 (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §4.1/§7.6): Pointer Events по
// клеткам, тач и мышь равнозначны (`touch-action:none` на канвасе — CSS).
// Палец задаёт waypoints-клетки; цепочка между ними достраивается 4-связно
// (rebuildPath). Первый тап обязан попасть в клетку СТАРТА.
import { cellAtPoint, rebuildPath } from './route_board.js';

// addWaypoint — добавить клетку-waypoint, если она не совпадает с последней;
// пересобрать цепочку пути. Возвращает true, если путь изменился.
function addWaypoint(state, cell) {
    const wp = state.waypoints;
    if (wp.length && wp[wp.length - 1] === cell) return false;
    wp.push(cell);
    state.path = rebuildPath(state.board, wp);
    return true;
}

// bindPointer — обработчики канваса. handlers: { onChange, onStartFail,
// isFrozen }. onChange зовётся на каждое изменение пути.
export function bindPointer(canvas, state, viewFn, handlers) {
    const frozen = () => !!(handlers && handlers.isFrozen && handlers.isFrozen());
    const rect = () => canvas.getBoundingClientRect();
    const cellAt = (e) => {
        const r = rect();
        return cellAtPoint(viewFn(), e.clientX - r.left, e.clientY - r.top);
    };
    const pointAt = (e) => {
        const r = rect();
        return { x: e.clientX - r.left, y: e.clientY - r.top };
    };

    canvas.addEventListener('pointerdown', (e) => {
        if (frozen()) return;
        const cell = cellAt(e);
        if (cell < 0) return; // вне доски — клип
        e.preventDefault();
        if (!state.waypoints.length) {
            if (cell !== state.board.start) { handlers.onStartFail(); return; }
            addWaypoint(state, cell);
        } else {
            addWaypoint(state, cell);
        }
        state.dragging = true;
        state.drag = pointAt(e);
        try { canvas.setPointerCapture(e.pointerId); } catch (err) { /* без захвата */ }
        handlers.onChange();
    });

    canvas.addEventListener('pointermove', (e) => {
        if (!state.dragging || frozen()) return;
        const cell = cellAt(e);
        state.drag = pointAt(e);
        if (cell < 0) return;
        if (addWaypoint(state, cell)) handlers.onChange();
    });

    canvas.addEventListener('pointerup', (e) => {
        if (!state.dragging || frozen()) return;
        const cell = cellAt(e);
        if (cell >= 0 && addWaypoint(state, cell)) handlers.onChange();
        state.dragging = false;
        state.drag = null;
        handlers.onChange();
    });

    canvas.addEventListener('pointercancel', () => {
        state.dragging = false;
        state.drag = null;
    });
}
