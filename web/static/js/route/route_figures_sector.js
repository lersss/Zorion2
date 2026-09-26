// web/static/js/route/route_figures_sector.js
// Фигуры секторов и глифы содержимого доски v9 «Планшет» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §12.2/§12.9). Вынесено из
// route_figures.js без изменения поведения.
import * as C from './route_config.js';
import { hA, diamondPath } from './route_figures_prims.js';

export function sectorDensity(sig) { return [0.20, 0.32, 0.46][sig] || 0.3; }

// sectorRim — цвет каймы сектора по содержимому (§12.9): опасность трап/сбой
// красная, находки золотые, нейтральные мираже/вакуум стальные.
export function sectorRim(content) {
    if (content === 'trap' || content === 'unstable') return C.COLORS.unstable;
    if (content === 'jackpot' || content === 'lure') return C.COLORS.jackpot;
    if (content === 'decoy') return C.COLORS.decoy;
    if (content === 'empty') return C.COLORS.empty;
    return C.COLORS.sector;
}

// sectorRimDash — пунктир каймы (пульс у Сбоя, пунктир у Зова/Миража), §12.9.
export function sectorRimDash(content) {
    if (content === 'unstable') return [3, 2];
    if (content === 'lure' || content === 'decoy') return [4, 3];
    return null;
}

// sigTexture — σ-текстура сектора (§12.2): 3 / 5 дуг, у Гулкого — 8 хаотичных штрихов.
function sigTexture(ctx, r, sig) {
    const n = [3, 5, 8][sig] || 3;
    const s = Math.min(r.w, r.h);
    ctx.save();
    ctx.strokeStyle = hA(C.COLORS.sector, 0.5);
    ctx.lineWidth = 1;
    if (sig >= 2) {
        for (let k = 0; k < n; k++) {
            const x0 = r.x + ((k * 13 + 5) % Math.max(4, r.w - 6)) + 3;
            const y0 = r.y + ((k * 7 + 3) % Math.max(4, r.h - 6)) + 3;
            ctx.beginPath();
            ctx.moveTo(x0, y0);
            ctx.lineTo(x0 + (k % 2 ? 7 : -6), y0 + (k % 3 ? 5 : -4));
            ctx.stroke();
        }
    } else {
        for (let k = 0; k < n; k++) {
            ctx.beginPath();
            ctx.arc(r.x + r.w / 2, r.y + r.h / 2, s * (0.18 + k * 0.1), Math.PI * 0.15, Math.PI * 0.85);
            ctx.stroke();
        }
    }
    ctx.restore();
}

// figSector — одна клетка сектора-облака: заливка по σ + σ-текстура + кайма и
// центральный глиф по содержимому (глиф — только после вскрытия, §12.9).
export function figSector(ctx, r, sig, content) {
    ctx.save();
    ctx.fillStyle = C.COLORS.sector;
    ctx.globalAlpha = sectorDensity(sig);
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.restore();
    sigTexture(ctx, r, sig);
    ctx.save();
    ctx.strokeStyle = hA(sectorRim(content), content ? 0.9 : 0.45);
    ctx.lineWidth = content ? 1.6 : 1;
    const dash = sectorRimDash(content);
    if (dash) ctx.setLineDash(dash);
    ctx.strokeRect(r.x + 0.5, r.y + 0.5, r.w - 1, r.h - 1);
    ctx.setLineDash([]);
    ctx.restore();
    if (content) contentGlyph(ctx, r.x + r.w / 2, r.y + r.h / 2, r.w, content, 1);
}

// ---- Глиф содержимого сектора (§12.9): свой силуэт у каждого содержимого ----

export function contentGlyph(ctx, x, y, cell, content, pulse) {
    const a = pulse == null ? 1 : pulse;
    const r = Math.max(3, cell * 0.22);
    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    if (content === 'jackpot') {
        ctx.strokeStyle = hA(C.COLORS.jackpot, 0.95 * a);
        ctx.lineWidth = Math.max(1.6, cell * 0.06);
        for (const [dx, dy] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
            ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + dx * r, y + dy * r); ctx.stroke();
        }
    } else if (content === 'lure') {
        ctx.strokeStyle = hA(C.COLORS.jackpot, 0.95 * a);
        ctx.lineWidth = Math.max(1.4, cell * 0.05);
        diamondPath(ctx, x, y, r); ctx.stroke();
        diamondPath(ctx, x, y, r * 0.55); ctx.stroke();
    } else if (content === 'trap') {
        ctx.strokeStyle = hA(C.COLORS.unstable, 0.95 * a);
        ctx.lineWidth = Math.max(1.6, cell * 0.06);
        ctx.beginPath();
        ctx.moveTo(x - r, y - r); ctx.lineTo(x + r, y + r);
        ctx.moveTo(x + r, y - r); ctx.lineTo(x - r, y + r);
        ctx.stroke();
    } else if (content === 'decoy') {
        ctx.strokeStyle = hA(C.COLORS.decoy, 0.9 * a);
        ctx.lineWidth = Math.max(1.4, cell * 0.05);
        ctx.beginPath(); ctx.arc(x, y, r, Math.PI * 0.25, Math.PI * 1.75); ctx.stroke();
    } else if (content === 'unstable') {
        ctx.strokeStyle = hA(C.COLORS.unstable, 0.95 * a);
        ctx.lineWidth = Math.max(1.6, cell * 0.06);
        ctx.beginPath();
        ctx.moveTo(x - r * 0.5, y - r); ctx.lineTo(x + r * 0.2, y - r * 0.1);
        ctx.lineTo(x - r * 0.2, y + r * 0.1); ctx.lineTo(x + r * 0.5, y + r);
        ctx.stroke();
    }
    ctx.restore();
}
