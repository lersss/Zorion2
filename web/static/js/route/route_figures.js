// web/static/js/route/route_figures.js
// Общие примитивы отрисовки фигур доски v9 «Планшет» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.4/§4.9). Их используют и доска
// (route_render), и попап-легенда (route_legend): мини-фигура в легенде — это
// та же фигура, нарисованная тем же кодом, а не спрайт и не цветной свотч.
// Спрайт НЕ несёт игровой истины: цены/τ/содержимое — только код.
//
// Фигуры разнесены по логике (примитивы, поле, секторы, ориентиры, метки); этот
// файл — точка входа и диспетчер легенды, реэкспорт сохраняет прежние импорты
// (`import { figWall, heatGlyph, ... } from './route_figures.js'`).
export * from './route_figures_prims.js';
export * from './route_figures_field.js';
export * from './route_figures_sector.js';
export * from './route_figures_landmarks.js';
export * from './route_figures_marks.js';

import { figStart, figBeacon, figStar } from './route_figures_landmarks.js';
import {
    figLane, figMud, figWall, figBottleneck, figChevron, figGate, figBridge, figDeadEnd,
} from './route_figures_field.js';
import { figSector } from './route_figures_sector.js';
import { heatGlyph } from './route_figures_marks.js';

// ---- Диспетчер мини-фигур легенды (§4.9): id из LEGEND_ITEMS → фигура ----

export function drawLegendFigure(ctx, id, r) {
    const cx = r.x + r.w / 2;
    const cy = r.y + r.h / 2;
    switch (id) {
        case 'start': figStart(ctx, r); break;
        case 'beacon': figBeacon(ctx, r); break;
        case 'finish': figStar(ctx, r); break;
        case 'lane': figLane(ctx, r); break;
        case 'mud': figMud(ctx, r); break;
        case 'wall': figWall(ctx, r); break;
        case 'bottleneck': figBottleneck(ctx, r); break;
        case 'current': figChevron(ctx, r, 0, 0.9); break;
        case 'gate': figGate(ctx, r); break;
        case 'bridge': figBridge(ctx, r); break;
        case 'dead_end': figDeadEnd(ctx, r, null); break;
        case 'sector': figSector(ctx, r, 1, null); break;
        case 'sig_quiet': figSector(ctx, r, 0, null); break;
        case 'sig_mid': figSector(ctx, r, 1, null); break;
        case 'sig_loud': figSector(ctx, r, 2, null); break;
        case 'mark_resistance': heatGlyph(ctx, cx, cy, r.w, 'mud', 1); break;
        case 'mark_turn': heatGlyph(ctx, cx, cy, r.w, 'turn', 1); break;
        case 'mark_dead_end': heatGlyph(ctx, cx, cy, r.w, 'dead_end', 1); break;
        case 'mark_against': heatGlyph(ctx, cx, cy, r.w, 'current_against', 1); break;
        case 'mark_along': heatGlyph(ctx, cx, cy, r.w, 'current_along', 1); break;
        case 'mark_hazard': heatGlyph(ctx, cx, cy, r.w, 'hazard', 1); break;
        case 'mark_jackpot': heatGlyph(ctx, cx, cy, r.w, 'jackpot', 1); break;
        case 'mark_gate_twice': heatGlyph(ctx, cx, cy, r.w, 'gate_twice', 1); break;
        case 'mark_bridge_twice': heatGlyph(ctx, cx, cy, r.w, 'bridge_twice', 1); break;
        default: break;
    }
}
