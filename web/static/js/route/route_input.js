// web/static/js/route/route_input.js
// Ввод мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §3.1): Pointer Events, тач и мышь
// равнозначны, `touch-action:none` на канвасе (CSS). Первая точка обязана
// попасть в радиус СТАРТА; первая вершина притягивается к СТАРТУ, последняя —
// к ФИНИШУ; координаты клипаются к [0,1]². Потолок точек — MAX_POINTS.
import * as C from './route_config.js';

// MIN_STEP — минимальный шаг между вершинами (§3.1, осн. §6.6): тап по месту
// последней вершины (в т.ч. по СТАРТУ сразу после pointerdown) не дублирует её.
const MIN_STEP = 0.005;

function clamp01(v) {
    return v < 0 ? 0 : v > 1 ? 1 : v;
}

// toField — экранная точка → нормализованное поле [0,1]², клип по квадрату.
function toField(ev, view, rect) {
    return {
        x: clamp01((ev.clientX - rect.left - view.x0) / view.size),
        y: clamp01((ev.clientY - rect.top - view.y0) / view.size),
    };
}

// addVertex — вершина при отпускании: в радиусе ФИНИША притягивается ровно к нему.
function addVertex(state, p, handlers) {
    if (state.path.length >= C.MAX_POINTS) {
        handlers.onTooMany();
        return;
    }
    let v = { x: p.x, y: p.y };
    if (C.dist(p, state.field.finish) <= C.ENDPOINT_R) {
        v = { x: state.field.finish.x, y: state.field.finish.y };
    }
    const last = state.path[state.path.length - 1];
    if (last && C.dist(last, v) < MIN_STEP) return;
    state.path.push(v);
}

// bindPointer — обработчики канваса. handlers: { onChange, onStartFail,
// onTooMany, isFrozen }. onChange зовётся на фиксацию вершины/старт/отмену.
export function bindPointer(canvas, state, viewFn, handlers) {
    const rect = () => canvas.getBoundingClientRect();
    const frozen = () => !!(handlers.isFrozen && handlers.isFrozen());

    canvas.addEventListener('pointerdown', (e) => {
        if (frozen()) return;
        e.preventDefault();
        const p = toField(e, viewFn(), rect());
        if (state.path.length === 0) {
            if (C.dist(p, state.field.start) > C.ENDPOINT_R) {
                handlers.onStartFail();
                return;
            }
            state.path.push({ x: state.field.start.x, y: state.field.start.y });
        }
        state.drag = p;
        state.dragging = true;
        try { canvas.setPointerCapture(e.pointerId); } catch (err) { /* без захвата */ }
        handlers.onChange();
    });

    canvas.addEventListener('pointermove', (e) => {
        if (!state.dragging || frozen()) return;
        state.drag = toField(e, viewFn(), rect());
    });

    canvas.addEventListener('pointerup', (e) => {
        if (!state.dragging || frozen()) return;
        const p = toField(e, viewFn(), rect());
        state.dragging = false;
        state.drag = null;
        addVertex(state, p, handlers);
        handlers.onChange();
    });

    canvas.addEventListener('pointercancel', () => {
        state.dragging = false;
        state.drag = null;
    });
}
