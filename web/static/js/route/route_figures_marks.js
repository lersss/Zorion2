// web/static/js/route/route_figures_marks.js
// Микро-глифы меток курса и глифы строк разбора (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §12.10/§12.14). Вынесено из
// route_figures.js без изменения поведения.
import * as C from './route_config.js';
import { TAU, hA, diamondPath } from './route_figures_prims.js';
import { sectorRim, contentGlyph } from './route_figures_sector.js';

const MARK_COLORS = {
    mud: C.COLORS.heatCost, wall: C.COLORS.heatCost,
    gate: C.COLORS.gate, gate_twice: C.COLORS.gate,
    bridge: C.COLORS.bridge, bridge_twice: C.COLORS.bridge,
    current_against: C.COLORS.unstable, current_along: C.COLORS.current,
    dead_end: C.COLORS.deadEnd, turn: C.COLORS.captureSoft,
    hazard: C.COLORS.unstable, jackpot: C.COLORS.jackpot,
};

export function heatColor(kind) { return MARK_COLORS[kind] || C.COLORS.capture; }

// heatGlyph — микро-глиф метки курса в точке (x,y) (§12.10); размер ≈0.22·cell,
// толщина штриха ≥1.4 px. pulse — «дыхание» метки (reduced-motion → статично).
export function heatGlyph(ctx, x, y, cell, kind, pulse) {
    const a = pulse == null ? 1 : pulse;
    const col = heatColor(kind);
    const r = Math.max(3, cell * 0.22);
    ctx.save();
    ctx.strokeStyle = hA(col, 0.9 * a);
    ctx.fillStyle = hA(col, 0.9 * a);
    ctx.lineWidth = Math.max(1.4, cell * 0.045);
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    if (kind === 'mud' || kind === 'wall') {
        for (const oy of [-r * 0.35, r * 0.35]) {
            ctx.beginPath();
            ctx.moveTo(x - r, y + oy);
            ctx.quadraticCurveTo(x - r * 0.5, y + oy - r * 0.5, x, y + oy);
            ctx.quadraticCurveTo(x + r * 0.5, y + oy + r * 0.5, x + r, y + oy);
            ctx.stroke();
        }
    } else if (kind === 'gate') {
        diamondPath(ctx, x, y, r); ctx.stroke();
    } else if (kind === 'gate_twice') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.35, y - r); ctx.lineTo(x - r * 0.35, y + r);
        ctx.moveTo(x + r * 0.35, y - r); ctx.lineTo(x + r * 0.35, y + r);
        ctx.stroke();
    } else if (kind === 'bridge') {
        ctx.setLineDash([2.5, 2.5]);
        ctx.beginPath(); ctx.moveTo(x - r, y); ctx.lineTo(x + r, y); ctx.stroke();
        ctx.setLineDash([]);
    } else if (kind === 'bridge_twice') {
        ctx.setLineDash([2.5, 2.5]);
        ctx.beginPath();
        ctx.moveTo(x - r, y - r * 0.4); ctx.lineTo(x + r, y - r * 0.4);
        ctx.moveTo(x - r, y + r * 0.4); ctx.lineTo(x + r, y + r * 0.4);
        ctx.stroke();
        ctx.setLineDash([]);
    } else if (kind === 'current_against') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.7, y - r * 0.6); ctx.lineTo(x, y); ctx.lineTo(x - r * 0.7, y + r * 0.6);
        ctx.stroke();
        ctx.beginPath(); ctx.moveTo(x + r * 0.9, y - r * 0.9); ctx.lineTo(x - r * 0.1, y + r * 0.1); ctx.stroke();
    } else if (kind === 'current_along') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.7, y - r * 0.6); ctx.lineTo(x, y); ctx.lineTo(x - r * 0.7, y + r * 0.6);
        ctx.stroke();
    } else if (kind === 'dead_end') {
        ctx.fillRect(x - r, y - r * 0.3, r * 2, r * 0.6);
    } else if (kind === 'turn') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.6, y + r * 0.5);
        ctx.lineTo(x - r * 0.6, y - r * 0.3);
        ctx.lineTo(x + r * 0.5, y - r * 0.3);
        ctx.stroke();
    } else if (kind === 'hazard') {
        ctx.beginPath();
        ctx.moveTo(x - r * 0.5, y - r); ctx.lineTo(x + r * 0.2, y - r * 0.1);
        ctx.lineTo(x - r * 0.2, y + r * 0.1); ctx.lineTo(x + r * 0.5, y + r);
        ctx.stroke();
    } else if (kind === 'jackpot') {
        for (const [dx, dy] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
            ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + dx * r, y + dy * r); ctx.stroke();
        }
    }
    ctx.restore();
}

// factorColor — цвет каймы подсветки клеток разбора (§4.7.1): тот же цвет, что
// у глифа фактора. Секторные факторы — цвет содержимого (sectorRim), прочие —
// цвет метки курса (heatColor); новые события — мягкий акцент.
export function factorColor(code) {
    const g = C.FACTOR_GLYPHS[code];
    if (!g) return C.COLORS.capture;
    if (g.via === 'heat') return heatColor(g.kind);
    if (g.via === 'content') return sectorRim(g.kind);
    return code === 'ping_wasted' ? C.COLORS.empty : C.COLORS.captureSoft;
}

// factorGlyph — глиф строки разбора (§4.7.1): переиспользует метку курса
// (heatGlyph) или содержимое сектора (contentGlyph); события revisit/overshoot/
// ping_wasted рисуются собственным кодом — на доске их нет (§12.14).
export function factorGlyph(ctx, x, y, cell, code) {
    const g = C.FACTOR_GLYPHS[code];
    if (!g) return;
    if (g.via === 'heat') { heatGlyph(ctx, x, y, cell, g.kind, 1); return; }
    if (g.via === 'content') { contentGlyph(ctx, x, y, cell, g.kind, 1); return; }
    const r = Math.max(3, cell * 0.22);
    const s = Math.max(1.4, cell * 0.045);
    const d = Math.max(1.2, r * 0.24);
    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    if (code === 'revisit') {
        // Петля (§12.14): кольцо-возврат 300° с разрывом 60° сверху + наконечник + точка-ось.
        ctx.strokeStyle = hA(C.COLORS.captureSoft, 0.9);
        ctx.fillStyle = hA(C.COLORS.captureSoft, 0.9);
        ctx.lineWidth = s;
        const th0 = Math.PI * 1.67;
        const th1 = Math.PI * 3.33;
        ctx.beginPath(); ctx.arc(x, y, r, th0, th1); ctx.stroke();
        const ex = x + r * Math.cos(th1);
        const ey = y + r * Math.sin(th1);
        const back = Math.atan2(-Math.cos(th1), Math.sin(th1)); // обратная касательная
        for (const da of [-Math.PI / 6, Math.PI / 6]) {
            ctx.beginPath();
            ctx.moveTo(ex, ey);
            ctx.lineTo(ex + r * 0.55 * Math.cos(back + da), ey + r * 0.55 * Math.sin(back + da));
            ctx.stroke();
        }
        ctx.beginPath(); ctx.arc(x, y, d, 0, TAU); ctx.fill();
    } else if (code === 'overshoot') {
        // Перелёт Цели (§12.14): вытянутая U-шпилька с разворотом 180° + наконечник влево + точка-цель.
        ctx.strokeStyle = hA(C.COLORS.captureSoft, 0.9);
        ctx.fillStyle = hA(C.COLORS.captureSoft, 0.9);
        ctx.lineWidth = s;
        ctx.beginPath();
        ctx.moveTo(x - r * 0.45, y - r * 0.5);
        ctx.lineTo(x + r * 0.55, y - r * 0.5);
        ctx.stroke();
        ctx.beginPath();
        ctx.arc(x + r * 0.55, y, r * 0.5, -Math.PI / 2, Math.PI / 2);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(x + r * 0.55, y + r * 0.5);
        ctx.lineTo(x - r * 0.06, y + r * 0.5);
        ctx.moveTo(x - r * 0.06, y + r * 0.16);
        ctx.lineTo(x - r * 0.48, y + r * 0.5);
        ctx.lineTo(x - r * 0.06, y + r * 0.84);
        ctx.stroke();
        ctx.beginPath(); ctx.arc(x - r * 0.78, y, d, 0, TAU); ctx.fill();
    } else if (code === 'ping_wasted') {
        // Зонд впустую (§12.14): точка-источник + две пунктирные вложенные дуги.
        ctx.strokeStyle = hA(C.COLORS.empty, 0.9);
        ctx.fillStyle = hA(C.COLORS.empty, 0.9);
        ctx.lineWidth = s;
        ctx.beginPath(); ctx.arc(x, y, d, 0, TAU); ctx.fill();
        const a70 = Math.PI * 70 / 180;
        const a40 = Math.PI * 40 / 180;
        ctx.setLineDash([2.5, 2.5]);
        ctx.beginPath(); ctx.arc(x, y, r * 0.65, -a70, a70); ctx.stroke();
        ctx.strokeStyle = hA(C.COLORS.empty, 0.5);
        ctx.setLineDash([2.5, 3.5]);
        ctx.beginPath(); ctx.arc(x, y, r * 1.05, -a40, a40); ctx.stroke();
        ctx.setLineDash([]);
    }
    ctx.restore();
}
