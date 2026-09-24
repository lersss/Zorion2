// web/static/js/surface/surface_decor.js
// Библиотека декора прогулки (спека 2026-09-23 §3.2): 15 базовых примитивов
// (+ лиана и сухая котловина-примета). Каждая функция — чистая отрисовка от
// готовых параметров записи (h/w разрешены детерминированно в surface_world.js);
// Math.random нет. Цвета — из палитры биома (world.palette, §3.4), не литералы.
import { rgba } from './surface_world.js';

// ==================== 15 БАЗОВЫХ ПРИМИТИВОВ ====================

function drawTree(ctx, pal, d, x, gy) {
    const h = d.h;
    const tw = Math.max(2, h * 0.08);
    ctx.fillStyle = pal.trunk;
    ctx.fillRect(x - tw / 2, gy - h, tw, h);
    if (d.buttress) {
        const bw = h * 0.30, bh = h * 0.14;
        ctx.beginPath();
        ctx.moveTo(x - tw / 2, gy - bh); ctx.lineTo(x - bw, gy); ctx.lineTo(x - tw / 2, gy);
        ctx.closePath(); ctx.fill();
        ctx.beginPath();
        ctx.moveTo(x + tw / 2, gy - bh); ctx.lineTo(x + bw, gy); ctx.lineTo(x + tw / 2, gy);
        ctx.closePath(); ctx.fill();
    }
    const crown = d.crown || 2;
    const cr = h * 0.34;
    ctx.fillStyle = pal.accent;
    for (let i = 0; i < crown; i++) {
        const off = (i - (crown - 1) / 2) * cr * 0.8;
        ctx.beginPath();
        ctx.arc(x + off, gy - h - cr * 0.3, cr * (i % 2 ? 1.0 : 0.8), 0, Math.PI * 2);
        ctx.fill();
    }
    if (d.hang) {
        const len = d.hang.len;
        ctx.strokeStyle = pal.moss;
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(x, gy - h * 0.7);
        ctx.quadraticCurveTo(x + 4, gy - h * 0.4 + len * 0.5, x + 1, gy - h * 0.4 + len);
        ctx.stroke();
    }
}

function drawConifer(ctx, pal, d, x, gy) {
    const h = d.h;
    const tiers = d.tiers || 3;
    ctx.fillStyle = pal.trunk;
    ctx.fillRect(x - 1.5, gy - h * 0.25, 3, h * 0.25);
    ctx.fillStyle = pal.moss;
    const w = h * 0.42;
    for (let i = 0; i < tiers; i++) {
        const t = i / tiers;
        const yTop = gy - h + t * h * 0.75;
        const yBot = yTop + h * 0.42;
        const half = w * (0.35 + 0.65 * (1 - t));
        ctx.beginPath();
        ctx.moveTo(x, yTop); ctx.lineTo(x + half, yBot); ctx.lineTo(x - half, yBot);
        ctx.closePath(); ctx.fill();
    }
}

function drawPalm(ctx, pal, d, x, gy) {
    const h = d.h;
    ctx.strokeStyle = pal.trunk;
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(x, gy);
    ctx.quadraticCurveTo(x + h * 0.12, gy - h * 0.6, x + h * 0.06, gy - h);
    ctx.stroke();
    const topX = x + h * 0.06, topY = gy - h;
    const fronds = d.fronds || 6;
    ctx.strokeStyle = pal.accent;
    ctx.lineWidth = 2;
    for (let i = 0; i < fronds; i++) {
        const a = Math.PI * (0.15 + 0.7 * (i / Math.max(1, fronds - 1)));
        ctx.beginPath();
        ctx.moveTo(topX, topY);
        ctx.quadraticCurveTo(topX + Math.cos(a) * h * 0.3, topY - Math.sin(a) * h * 0.28,
            topX + Math.cos(a) * h * 0.42, topY - Math.sin(a) * h * 0.1);
        ctx.stroke();
    }
}

function drawMushroom(ctx, pal, d, x, gy) {
    const h = d.h, r = d.capR || h * 0.5;
    ctx.fillStyle = pal.trunk;
    ctx.fillRect(x - 1.5, gy - h, 3, h);
    ctx.fillStyle = pal.glow;
    ctx.beginPath();
    ctx.ellipse(x, gy - h, r, r * 0.6, 0, Math.PI, 0);
    ctx.fill();
}

function drawCactus(ctx, pal, d, x, gy) {
    const h = d.h;
    const w = Math.max(3, h * 0.22);
    ctx.fillStyle = pal.moss;
    ctx.fillRect(x - w / 2, gy - h, w, h);
    const arms = d.arms ?? 2; // силуэт кактуса: без ветвей колонна не читается
    for (let i = 0; i < arms; i++) {
        const ay = gy - h * (0.45 + 0.25 * i);
        const dir = i % 2 ? 1 : -1;
        ctx.fillRect(x, ay, dir * w * 1.2, w * 0.7);
        ctx.fillRect(x + dir * w * 1.2 - w * 0.35, ay - h * 0.25, w * 0.7, h * 0.25);
    }
}

function drawBush(ctx, pal, d, x, gy) {
    const r = d.h * 0.6;
    ctx.fillStyle = pal.moss;
    for (let i = -1; i <= 1; i++) {
        ctx.beginPath();
        ctx.arc(x + i * r * 0.6, gy - r * 0.5, r * (i === 0 ? 0.8 : 0.6), 0, Math.PI * 2);
        ctx.fill();
    }
}

function drawFern(ctx, pal, d, x, gy) {
    const h = d.h;
    const fronds = d.fronds || 4;
    ctx.strokeStyle = pal.moss;
    ctx.lineWidth = 1.5;
    for (let i = 0; i < fronds; i++) {
        const dir = i % 2 ? 1 : -1;
        const spread = h * (0.3 + 0.25 * Math.floor(i / 2));
        ctx.beginPath();
        ctx.moveTo(x, gy);
        ctx.quadraticCurveTo(x + dir * spread * 0.5, gy - h * 0.7, x + dir * spread, gy - h * 0.4);
        ctx.stroke();
    }
}

function drawGrass(ctx, pal, d, x, gy) {
    const h = d.h;
    const blades = d.blades || 5;
    ctx.strokeStyle = pal.accent;
    ctx.lineWidth = 1;
    for (let i = 0; i < blades; i++) {
        const dir = (i % 2 ? 1 : -1) * (0.3 + 0.7 * (i / blades));
        ctx.beginPath();
        ctx.moveTo(x, gy);
        ctx.quadraticCurveTo(x + dir * 3, gy - h * 0.6, x + dir * h * 0.3, gy - h);
        ctx.stroke();
    }
}

function drawLichen(ctx, pal, d, x, gy) {
    ctx.fillStyle = rgba(pal.moss, 0.5);
    ctx.fillRect(x - d.w / 2, gy - d.h, d.w, d.h);
}

function drawCrystal(ctx, pal, d, x, gy) {
    const h = d.h, w = h * 0.5;
    // glow (спека 2026-09-23 §4.7.12): false — матовый кристалл (обсидиан, В3);
    // true/undefined — светится (совместимость со сданными, §4.7.13).
    ctx.fillStyle = d.glow === false ? pal.rock : pal.glow;
    ctx.beginPath();
    ctx.moveTo(x, gy - h);
    ctx.lineTo(x + w, gy - h * 0.35);
    ctx.lineTo(x + w * 0.5, gy);
    ctx.lineTo(x - w * 0.5, gy);
    ctx.lineTo(x - w, gy - h * 0.35);
    ctx.closePath();
    ctx.fill();
    ctx.strokeStyle = pal.light;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x, gy - h); ctx.lineTo(x, gy);
    ctx.stroke();
}

function drawGrowth(ctx, pal, d, x, gy) {
    const r = d.r || d.h * 0.5;
    ctx.fillStyle = pal.accent;
    ctx.beginPath();
    ctx.arc(x, gy - r * 0.5, r, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + r * 0.6, gy - r * 0.3, r * 0.6, 0, Math.PI * 2);
    ctx.fill();
}

function drawRock(ctx, pal, d, x, gy) {
    const h = d.h, w = d.w || h * 1.2;
    ctx.fillStyle = pal.rock;
    ctx.beginPath();
    ctx.moveTo(x - w / 2, gy);
    ctx.lineTo(x - w * 0.3, gy - h);
    ctx.lineTo(x + w * 0.2, gy - h * 0.9);
    ctx.lineTo(x + w / 2, gy);
    ctx.closePath();
    ctx.fill();
}

function drawBone(ctx, pal, d, x, gy) {
    const h = d.h;
    ctx.strokeStyle = pal.light;
    ctx.lineWidth = 2;
    if (d.kind === 'shell') {
        ctx.beginPath();
        ctx.ellipse(x, gy - h * 0.4, h * 0.6, h * 0.4, 0, Math.PI, 0);
        ctx.stroke();
        return;
    }
    ctx.beginPath();
    ctx.moveTo(x - h * 0.5, gy); ctx.lineTo(x, gy - h * 0.6); ctx.lineTo(x + h * 0.5, gy);
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(x - h * 0.3, gy - h * 0.25); ctx.lineTo(x + h * 0.3, gy - h * 0.25);
    ctx.stroke();
}

function drawDebris(ctx, pal, d, x, gy) {
    const h = d.h, w = d.w || h;
    ctx.fillStyle = pal.rock;
    ctx.beginPath();
    ctx.moveTo(x - w * 0.5, gy);
    ctx.lineTo(x - w * 0.2, gy - h);
    ctx.lineTo(x + w * 0.5, gy - h * 0.3);
    ctx.lineTo(x + w * 0.2, gy);
    ctx.closePath();
    ctx.fill();
}

function drawVent(ctx, pal, d, x, gy) {
    const h = d.h, w = d.w || h * 1.4;
    ctx.fillStyle = pal.rock;
    ctx.beginPath();
    ctx.moveTo(x - w / 2, gy);
    ctx.lineTo(x - w * 0.12, gy - h);
    ctx.lineTo(x + w * 0.12, gy - h);
    ctx.lineTo(x + w / 2, gy);
    ctx.closePath();
    ctx.fill();
    if (d.plume) {
        ctx.fillStyle = rgba(pal.light, 0.35);
        ctx.beginPath();
        ctx.ellipse(x, gy - h - 4, w * 0.25, h * 0.3, 0, 0, Math.PI * 2);
        ctx.fill();
    }
}

// ==================== HANG И ПРИМЕТЫ ====================

// liana — свисающая лиана (привязка hang, §3.2): самостоятельная запись или
// деталь дерева (тогда рисуется в drawTree по d.hang).
function drawLiana(ctx, pal, d, x, gy) {
    const len = d.len || d.h || 30;
    ctx.strokeStyle = pal.moss;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(x, gy - len);
    ctx.quadraticCurveTo(x + 4, gy - len * 0.5, x, gy);
    ctx.stroke();
}

// oasis_dry — сухая котловина-примета (§4.1, вода — ЧК6): обводка впадины +
// пара валунов. Детерминирована участком rareDecorAt (не каждый кадр).
function drawOasisDry(ctx, pal, d, x, gy) {
    ctx.strokeStyle = rgba(pal.dark, 0.7);
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.ellipse(x, gy + 2, 26, 6, 0, 0, Math.PI * 2);
    ctx.stroke();
    ctx.fillStyle = pal.rock;
    ctx.beginPath();
    ctx.arc(x - 10, gy - 4, 4, 0, Math.PI * 2);
    ctx.arc(x + 12, gy - 3, 3, 0, Math.PI * 2);
    ctx.fill();
}

// ==================== ДИСПЕТЧЕР ====================

const PRIMS = {
    tree: drawTree, conifer: drawConifer, palm: drawPalm, mushroom: drawMushroom,
    cactus: drawCactus, bush: drawBush, fern: drawFern, grass: drawGrass,
    lichen: drawLichen, crystal: drawCrystal, growth: drawGrowth, rock: drawRock,
    bone: drawBone, debris: drawDebris, vent: drawVent, liana: drawLiana,
    oasis_dry: drawOasisDry,
};

// drawDecorPrim — отрисовка записи декора по prim (§3.2). Неизвестный prim
// пропускается (forward-compat: новый примитив не ломает старый клиент).
export function drawDecorPrim(ctx, world, d, x, gy) {
    const fn = PRIMS[d.prim];
    if (!fn) return;
    fn(ctx, world.palette, d, x, gy);
}
