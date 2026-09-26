// web/static/js/route/route_input.js
// Ввод мини-игры «Прокладка маршрута» на доске v9 (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §4.1/§4.3/§7.6): Pointer Events по
// клеткам, тач и мышь равнозначны (`touch-action:none` на канвасе — CSS).
// Палец ведёт waypoints-клетки; цепочка между ними достраивается 4-связно
// (rebuildPath). Первый тап обязан попасть в клетку СТАРТА.
//
// Одиночный тап (без протяжки) по клетке сектора открывает карточку сектора
// (§4.3); протяжка через сектор — обычная прокладка пути.
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
// onSelectSector(cell, sector), isFrozen }. onChange зовётся на каждое
// изменение пути (путь/захват/гейт пересчитываются в route_game.refresh).
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
    const sectorAt = (cell) => {
        const idx = state.boardIndex && state.boardIndex.sectorOf;
        return (cell >= 0 && idx) ? idx.get(cell) : undefined;
    };

    canvas.addEventListener('pointerdown', (e) => {
        if (frozen()) return;
        const cell = cellAt(e);
        if (cell < 0) return; // вне доски — клип
        e.preventDefault();
        state.dragStart = cell;
        state.dragMoved = false;
        state.dragging = true;
        state.drag = pointAt(e);
        try { canvas.setPointerCapture(e.pointerId); } catch (err) { /* без захвата */ }
    });

    canvas.addEventListener('pointermove', (e) => {
        if (!state.dragging || frozen()) return;
        const cell = cellAt(e);
        state.drag = pointAt(e);
        if (cell < 0) return;
        if (cell !== state.dragStart) state.dragMoved = true;
        if (!state.dragMoved) return; // палец ещё на исходной клетке
        if (!state.waypoints.length) {
            if (state.dragStart !== state.board.start) return; // путь обязан начинаться от СТАРТА
            addWaypoint(state, state.board.start);
        }
        if (addWaypoint(state, cell)) handlers.onChange();
    });

    canvas.addEventListener('pointerup', (e) => {
        if (frozen()) return;
        const cell = cellAt(e);
        const moved = state.dragMoved;
        const startCell = state.dragStart;
        state.dragging = false;
        state.drag = null;
        state.dragStart = -1;
        state.dragMoved = false;

        if (!moved) {
            // одиночный тап: сектор → карточка; иначе — waypoint/старт.
            const si = sectorAt(cell);
            if (si != null) {
                if (handlers.onSelectSector) handlers.onSelectSector(cell, si);
                return;
            }
            if (handlers.onSelectSector) handlers.onSelectSector(cell, null);
            if (!state.waypoints.length) {
                if (cell !== state.board.start) { handlers.onStartFail(); return; }
                addWaypoint(state, cell);
            } else {
                addWaypoint(state, cell);
            }
            handlers.onChange();
            return;
        }

        if (!state.waypoints.length) {
            if (startCell !== state.board.start) { handlers.onStartFail(); return; }
            addWaypoint(state, startCell);
        }
        if (cell >= 0 && addWaypoint(state, cell)) handlers.onChange();
        else handlers.onChange();
    });

    canvas.addEventListener('pointercancel', () => {
        state.dragging = false;
        state.drag = null;
        state.dragStart = -1;
        state.dragMoved = false;
    });
}
