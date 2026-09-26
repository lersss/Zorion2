// web/static/js/route/route_render.js
// Отрисовка мини-игры «Прокладка маршрута» на доске v9 «Планшет» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.4). Слои: фон (код) → доска n×n
// 1:1 (клетки/объекты/секторы — код) → путь и метки курса → спрайты ядра сцены
// (Цель, маяки, Старт). Спрайт НЕ несёт игровой истины: цены/τ/содержимое —
// только код. reduced-motion → без пульсов/дрейфа.
import * as C from './route_config.js';
import { cellCenter } from './route_board.js';
import { drawResultFx } from './route_effects.js';
import { getSprite, tintedSprite, bloomSprite } from './route_sprites.js';
import {
    TAU, hA, radial, ring, roundRect, heatGlyph, contentGlyph,
    figWall, figMud, figLane, figWarm, figBottleneck, figGate, figBridge, figChevron,
    figDeadEnd, sectorDensity, sectorRim, sectorRimDash, beaconRings, beaconSquare, startTriangle,
} from './route_figures.js';
import { shipDrawTransform } from '../map/ship_sprites.js';
import { CONFIG } from '../config.js';

// Цвет/оттенок звезды — из map/utils.js (формулу не дублируем). map/config.js
// читает DOM на верхнем уровне, а на route его нет, — создаём скрытый шов-канвас
// перед динамическим импортом (маршрут карту не рисует).
let getStarShade = null;
try {
    if (!document.getElementById('mapCanvas')) {
        const shim = document.createElement('canvas');
        shim.id = 'mapCanvas'; shim.style.display = 'none';
        document.documentElement.appendChild(shim);
    }
    ({ getStarShade } = await import('../map/utils.js'));
} catch (e) { getStarShade = null; }

const starColor = (s) => {
    if (getStarShade) return getStarShade(s.spectral_class, s.temperature, s.star_type);
    const c = CONFIG.map.starColors;
    return (s.star_type && c[s.star_type]) || c[s.spectral_class] || c.default;
};

// initBackground — детерминированный от seed фон (звёзды/пыль/яркие/туманности).
export function initBackground(st) {
    const rng = C.mulberry32(st.seed || 1);
    const pick = () => C.COLORS.nebula[Math.floor(rng() * C.COLORS.nebula.length)];
    st.bg = {
        stars: Array.from({ length: 170 }, () => ({ x: rng(), y: rng(), s: rng() < 0.85 ? 1 : 1.6, a: 0.25 + rng() * 0.6,
            ph: rng() * TAU, tw: 0.6 + rng() * 1.4, par: 0.1 + rng() * 0.25, c: rng() < 0.15 ? '#aac7ff' : '#e2e8f0' })),
        dust: Array.from({ length: 12 }, () => ({ x: rng(), y: rng(), r: 0.06 + rng() * 0.14, a: 0.02 + rng() * 0.05 })),
        bright: Array.from({ length: 5 }, () => ({ x: 0.1 + rng() * 0.8, y: 0.1 + rng() * 0.8 })),
        neb: [0, 1].map((i) => ({ nx: i ? 0.45 + rng() * 0.4 : 0.15 + rng() * 0.3, ny: i ? 0.5 + rng() * 0.4 : 0.12 + rng() * 0.3,
            s: 0.8 + rng() * 0.6, a: (i ? 0.15 : 0.22) + rng() * 0.1, c: pick(), ph: rng() * TAU, rot: (rng() - 0.5) * 0.6 })),
    };
}

// prepareBoard — индексы доски (Set/Map) и плоские списки объектов: строятся
// один раз на offer, рендер/ввод к ним только читаются.
export function prepareBoard(st) {
    const b = st.board || {};
    const idx = {
        wall: new Set(b.wall || []),
        lane: new Set(b.lane || []),
        mud: new Set(b.mud || []),
        gate: new Set(b.gate || []),
        bridge: new Set(b.bridge || []),
        deadEnd: new Set(b.dead_end || []),
        bottleneck: new Set(b.bottleneck || []),
        beacons: new Set(b.beacons || []),
        current: new Map(),
        sectorOf: new Map(),
        visible: b.visible || [],
    };
    for (const c of b.current || []) idx.current.set(c.cell, c.dir);
    (b.sectors || []).forEach((s, i) => {
        for (const c of s.cells || []) if (!idx.sectorOf.has(c)) idx.sectorOf.set(c, i);
    });
    st.boardIndex = idx;
}

// drawScene — кадр: фон → виньетка → доска.
export function drawScene(ctx, st, view, vw, vh, now) {
    ctx.fillStyle = C.COLORS.bg;
    ctx.fillRect(0, 0, vw, vh);
    if (st.bg) drawBackground(ctx, st.bg, st.chosen, st.reduced, vw, vh, now);
    drawVignette(ctx, vw, vh);
    if (st.board) drawBoard(ctx, st, view, now);
}

function drawBackground(ctx, b, chosen, reduced, vw, vh, now) {
    const t = reduced ? 0 : now;
    const neb = getSprite(chosen && chosen.nebula_bg);
    for (const n of b.neb) {
        const x = n.nx * vw + (reduced ? 0 : Math.sin(t * 0.00003 + n.ph) * 14);
        const y = n.ny * vh + (reduced ? 0 : Math.cos(t * 0.000025 + n.ph) * 12);
        const r = Math.max(vw, vh) * n.s * 0.7;
        ctx.save();
        ctx.globalCompositeOperation = 'lighter';
        ctx.globalAlpha = n.a;
        if (neb) { ctx.translate(x, y); ctx.rotate(n.rot); ctx.drawImage(tintedSprite(neb, n.c), -r, -r, r * 2, r * 2); }
        else radial(ctx, x, y, r, n.c, 0.5);
        ctx.restore();
    }
    for (const s of b.stars) {
        ctx.globalAlpha = s.a * (0.75 + 0.25 * Math.sin(t * 0.001 * s.tw + s.ph));
        ctx.fillStyle = s.c;
        ctx.fillRect(s.x * vw + (reduced ? 0 : Math.sin(t * 0.000004 * s.tw + s.ph) * 3 * s.par), s.y * vh, s.s, s.s);
    }
    ctx.globalAlpha = 1;
    for (const d of b.dust) radial(ctx, d.x * vw, d.y * vh, d.r * Math.max(vw, vh), '#7882a0', d.a);
    ctx.fillStyle = '#e2e8f0'; ctx.globalAlpha = 0.85;
    ctx.strokeStyle = 'rgba(226,232,240,0.3)'; ctx.lineWidth = 1;
    for (const s of b.bright) {
        const x = s.x * vw;
        const y = s.y * vh;
        ctx.fillRect(x - 1, y - 1, 2.4, 2.4);
        ctx.beginPath(); ctx.moveTo(x - 9, y); ctx.lineTo(x + 9, y);
        ctx.moveTo(x, y - 9); ctx.lineTo(x, y + 9); ctx.stroke();
    }
    ctx.globalAlpha = 1;
}

function drawVignette(ctx, vw, vh) {
    const g = ctx.createRadialGradient(vw / 2, vh / 2, Math.min(vw, vh) * 0.3, vw / 2, vh / 2, Math.max(vw, vh) * 0.75);
    g.addColorStop(0, 'rgba(0,0,0,0)'); g.addColorStop(1, 'rgba(0,0,0,0.45)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, vw, vh);
}

function cellRect(view, cell) {
    const cs = view.size / view.n;
    const i = cell % view.n;
    const j = (cell / view.n) | 0;
    return { x: view.x0 + i * cs, y: view.y0 + j * cs, w: cs, h: cs };
}

// ---- Доска ----

function drawBoard(ctx, st, view, now) {
    const b = st.board;
    const idx = st.boardIndex;
    const { x0, y0, size } = view;
    ctx.save();
    ctx.beginPath(); ctx.rect(x0, y0, size, size); ctx.clip();
    ctx.fillStyle = C.COLORS.boardBg;
    ctx.fillRect(x0, y0, size, size);
    drawCells(ctx, view, idx);
    drawSectors(ctx, st, view, now);
    drawCurrents(ctx, view, idx, now, st.reduced);
    drawGates(ctx, view, idx);
    drawBridges(ctx, view, idx);
    drawDeadEnds(ctx, st, view, idx);
    drawGrid(ctx, view);
    drawPath(ctx, st, view);
    drawHeat(ctx, st, view, now);
    drawHighlight(ctx, st, view, now);
    drawResultFx(ctx, st, view, now);
    drawFinish(ctx, st, view, now);
    drawStart(ctx, st, view);
    drawBeacons(ctx, st, view, now);
    ctx.restore();
    ctx.strokeStyle = C.COLORS.border; ctx.lineWidth = 1.5;
    ctx.strokeRect(x0 + 0.75, y0 + 0.75, size - 1.5, size - 1.5);
}

// drawCells — видимая фактура клеток: Мгла/Помехи (visible > 1) тёплые плотные,
// Трасса (visible < 1) холодное светлое. Чисел цены в UI нет.
function drawCells(ctx, view, idx) {
    const n = view.n;
    const total = n * n;
    for (let c = 0; c < total; c++) {
        const r = cellRect(view, c);
        if (idx.wall.has(c)) { figWall(ctx, r); continue; }
        const v = idx.visible[c];
        if (idx.mud.has(c)) { figMud(ctx, r); continue; }
        if (idx.lane.has(c) || (v < 1 && v > 0)) { figLane(ctx, r); continue; }
        if (v > 1) figWarm(ctx, r);
    }
    // Разрывы — светлая щель в помехах
    for (const c of idx.bottleneck) figBottleneck(ctx, cellRect(view, c));
}

// drawCurrents — шевроны течения по dir (0:+i,1:−i,2:+j,3:−j).
function drawCurrents(ctx, view, idx, now, reduced) {
    for (const [cell, dir] of idx.current) {
        const ph = reduced ? 0 : Math.sin(now * 0.003 + cell) * 0.5 + 0.5;
        figChevron(ctx, cellRect(view, cell), dir, 0.35 + 0.4 * ph);
    }
}

function drawGates(ctx, view, idx) {
    for (const c of idx.gate) figGate(ctx, cellRect(view, c));
}

function drawBridges(ctx, view, idx) {
    for (const c of idx.bridge) figBridge(ctx, cellRect(view, c));
}

// drawDeadEnds — Обрывы: кайма + «стоп»-маркер; спрайт false_signal —
// маркер сломанного маяка у Обрыва/Миража (решение создателя 2026-09-26).
function drawDeadEnds(ctx, st, view, idx) {
    const img = getSprite(st.chosen && st.chosen.false_signal);
    for (const c of idx.deadEnd) figDeadEnd(ctx, cellRect(view, c), img);
}

function drawGrid(ctx, view) {
    const n = view.n;
    const cs = view.size / n;
    ctx.strokeStyle = C.COLORS.grid;
    ctx.lineWidth = 1;
    for (let i = 1; i < n; i++) {
        const g = cs * i;
        ctx.beginPath(); ctx.moveTo(view.x0 + g, view.y0); ctx.lineTo(view.x0 + g, view.y0 + view.size); ctx.stroke();
        ctx.beginPath(); ctx.moveTo(view.x0, view.y0 + g); ctx.lineTo(view.x0 + view.size, view.y0 + g); ctx.stroke();
    }
}

// ---- Секторы ----

function drawSectors(ctx, st, view, now) {
    const b = st.board;
    const rx = new Map();
    for (const r of st.revealed || []) rx.set(r.sector, r.content);
    const img = getSprite(st.chosen && st.chosen.hazard_cloud);
    (b.sectors || []).forEach((sec, si) => {
        const cells = sec.cells || [];
        if (!cells.length) return;
        let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity, cx = 0, cy = 0;
        ctx.save();
        ctx.beginPath();
        for (const c of cells) {
            const r = cellRect(view, c);
            ctx.rect(r.x, r.y, r.w, r.h);
            minX = Math.min(minX, r.x); minY = Math.min(minY, r.y);
            maxX = Math.max(maxX, r.x + r.w); maxY = Math.max(maxY, r.y + r.h);
            cx += r.x + r.w / 2; cy += r.y + r.h / 2;
        }
        ctx.clip();
        const dens = sectorDensity(sec.sig);
        const content = rx.get(si);
        ctx.globalAlpha = dens;
        if (img) {
            cx /= cells.length; cy /= cells.length;
            ctx.translate(cx, cy); ctx.rotate(st.reduced ? 0 : now * 0.0002 * (1 + si)); ctx.translate(-cx, -cy);
            ctx.drawImage(tintedSprite(img, C.COLORS.sector), minX, minY, maxX - minX, maxY - minY);
        } else {
            ctx.fillStyle = C.COLORS.sector;
            for (const c of cells) { const r = cellRect(view, c); ctx.fillRect(r.x, r.y, r.w, r.h); }
        }
        ctx.restore();
        // Кайма сектора: при вскрытии — по содержимому (опасность красная,
        // находки золотые, нейтральные стальные), иначе нейтральная (σ плотностью).
        const rim = sectorRim(content);
        const rimDash = sectorRimDash(content);
        ctx.save();
        ctx.strokeStyle = hA(rim, content ? 0.9 : 0.45);
        ctx.lineWidth = content ? 1.6 : 1;
        if (rimDash) ctx.setLineDash(rimDash);
        for (const c of cells) { const r = cellRect(view, c); ctx.strokeRect(r.x + 0.5, r.y + 0.5, r.w - 1, r.h - 1); }
        ctx.setLineDash([]);
        ctx.restore();
        // Центральный глиф содержимого (§12.9) — только после вскрытия.
        let cxm = 0, cym = 0;
        for (const c of cells) { const r = cellRect(view, c); cxm += r.x + r.w / 2; cym += r.y + r.h / 2; }
        cxm /= cells.length; cym /= cells.length;
        const cellSize = view.size / view.n;
        if (content) {
            const pulse = st.reduced ? 1 : 0.7 + 0.3 * Math.sin(now * 0.004 + si);
            contentGlyph(ctx, cxm, cym, cellSize, content, pulse);
        }
        // Выбранный сектор (§4.3): пунктирная кайма + клетки подсвечены.
        if (st.selectedSector === si) {
            ctx.save();
            ctx.strokeStyle = hA('#e2e8f0', 0.95);
            ctx.lineWidth = 2;
            ctx.setLineDash([5, 4]);
            for (const c of cells) { const r = cellRect(view, c); ctx.strokeRect(r.x + 1.5, r.y + 1.5, r.w - 3, r.h - 3); }
            ctx.restore();
        }
        drawSectorLabel(ctx, view, cells, sec, content, content ? cellSize * 0.62 : 0);
    });
}

function drawSectorLabel(ctx, view, cells, sec, content, dy) {
    let cx = 0, cy = 0;
    for (const c of cells) { const r = cellRect(view, c); cx += r.x + r.w / 2; cy += r.y + r.h / 2; }
    cx /= cells.length; cy = cy / cells.length + (dy || 0);
    const text = content ? C.contentLabel(content) : 'σ: ' + C.sigLabel(sec.sig);
    ctx.save();
    ctx.font = '600 11px "Segoe UI", Roboto, system-ui, sans-serif';
    try { ctx.letterSpacing = '0.02em'; } catch (e) { /* не поддержано */ }
    const w = ctx.measureText(text).width + 12;
    const h = 18;
    ctx.fillStyle = hA('#0f172a', 0.82);
    roundRect(ctx, cx - w / 2, cy - h / 2, w, h, 6);
    ctx.fill();
    ctx.strokeStyle = hA(sectorRim(content), 0.9);
    ctx.lineWidth = 1;
    ctx.stroke();
    ctx.fillStyle = '#e2e8f0';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(text, cx, cy + 0.5);
    ctx.restore();
}

// ---- Путь и метки курса ----

function drawPath(ctx, st, view) {
    const path = st.path || [];
    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    if (path.length) {
        // После отправки тон пути — по знаку bonus (§4.7): зелёный/янтарный.
        ctx.strokeStyle = st.resultFx
            ? (st.resultFx.bonus < 0 ? C.COLORS.warning : C.COLORS.success)
            : C.COLORS.path;
        ctx.lineWidth = Math.max(3, view.size / view.n * 0.22);
        ctx.beginPath();
        for (let k = 0; k < path.length; k++) {
            const p = cellCenter(view, path[k]);
            if (k === 0) ctx.moveTo(p.x, p.y);
            else ctx.lineTo(p.x, p.y);
        }
        ctx.stroke();
    }
    // бледная «резинка» от конца пути к пальцу
    if (st.dragging && st.drag && path.length) {
        const a = cellCenter(view, path[path.length - 1]);
        ctx.strokeStyle = C.COLORS.pathFree;
        ctx.lineWidth = Math.max(2, view.size / view.n * 0.12);
        ctx.setLineDash([6, 6]);
        ctx.beginPath(); ctx.moveTo(a.x, a.y); ctx.lineTo(st.drag.x, st.drag.y); ctx.stroke();
        ctx.setLineDash([]);
        ctx.fillStyle = C.COLORS.path;
        ctx.beginPath(); ctx.arc(st.drag.x, st.drag.y, 3, 0, TAU); ctx.fill();
    }
    ctx.restore();
}

// drawHighlight — кайма клеток строки разбора результата (§4.7.1): тап по
// строке подсвечивает связанные клетки цветом фактора; повторный тап/тап по
// фону снимает. Пульсация — «дыхание»; reduced-motion → статично.
function drawHighlight(ctx, st, view, now) {
    const h = st.highlight;
    if (!h || !h.cells || !h.cells.size) return;
    const pulse = st.reduced ? 0.9 : 0.55 + 0.45 * Math.sin(now * 0.005);
    const lw = Math.max(2, view.size / view.n * 0.09);
    ctx.save();
    ctx.strokeStyle = hA(h.color || C.COLORS.capture, 0.35 + 0.6 * pulse);
    ctx.lineWidth = lw;
    ctx.lineJoin = 'round';
    for (const c of h.cells) {
        const r = cellRect(view, c);
        ctx.strokeRect(r.x + lw / 2, r.y + lw / 2, r.w - lw, r.h - lw);
    }
    ctx.restore();
}

// drawHeat — микро-глифы меток курса (§4.6/§12.10): без тотала, шкалы и чисел.
function drawHeat(ctx, st, view, now) {
    const heat = st.heat || [];
    if (!heat.length) return;
    const byCell = new Map();
    for (const m of heat) {
        if (!byCell.has(m.cell)) byCell.set(m.cell, []);
        byCell.get(m.cell).push(m.kind);
    }
    const cs = view.size / view.n;
    for (const [cell, kinds] of byCell) {
        const r = cellRect(view, cell);
        kinds.forEach((kind, k) => {
            const pulse = st.reduced ? 0.85 : 0.6 + 0.4 * Math.sin(now * 0.004 + cell + k);
            heatGlyph(ctx, r.x + r.w * (0.22 + 0.22 * (k % 4)), r.y + r.h * 0.22, cs, kind, pulse);
        });
    }
}

// ---- Спрайты ядра сцены ----

function drawFinish(ctx, st, view, now) {
    const f = cellCenter(view, st.board.finish);
    const to = (st.passport && st.passport.to) || {};
    const black = to.star_type === 'black_hole';
    const base = view.size / view.n;
    const r = base * 0.9 * (st.reduced ? 1 : 1 + 0.06 * Math.sin(now * 0.0015));
    const img = getSprite(st.chosen && (black ? st.chosen.black_hole : st.chosen.star_core));
    if (black) {
        radial(ctx, f.x, f.y, r * 1.5, C.COLORS.voidHalo, 0.7);
        if (img) {
            bloomSprite(ctx, img, f.x, f.y, r * 1.8, 0);
            ctx.fillStyle = '#02030a';
            ctx.beginPath(); ctx.arc(f.x, f.y, r * 0.36, 0, TAU); ctx.fill();
            ctx.globalAlpha = 1;
        } else ring(ctx, f.x, f.y, r * 0.8, C.COLORS.warmHalo, 1, 2);
        return;
    }
    const col = starColor(to);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    radial(ctx, f.x, f.y, r * 1.5, col, 0.5);
    if (img) bloomSprite(ctx, img, f.x, f.y, r * 1.8, 0.25);
    ctx.restore();
    if (!img) radial(ctx, f.x, f.y, r * 0.9, col, 1);
}

function drawStart(ctx, st, view) {
    const s = cellCenter(view, st.board.start);
    const base = view.size / view.n;
    const r = base * 0.7;
    if (st.shipSprite) {
        const tf = shipDrawTransform(0, st.shipOrient);
        const sz = r * 1.7;
        ctx.save();
        ctx.translate(s.x, s.y); ctx.rotate(tf.rotate); ctx.scale(tf.scaleX, tf.scaleY);
        ctx.drawImage(st.shipSprite, -sz / 2, -sz / 2, sz, sz);
        ctx.restore();
    } else {
        startTriangle(ctx, s.x, s.y, r, C.COLORS.cold);
    }
    ring(ctx, s.x, s.y, base * 0.46, C.COLORS.cold, 0.6, 1.2);
}

function drawBeacons(ctx, st, view, now) {
    const b = st.board;
    const img = getSprite(st.chosen && st.chosen.beacon);
    const base = view.size / view.n;
    for (const c of b.beacons || []) {
        const p = cellCenter(view, c);
        const captured = st.captured.has(c);
        const sz = base * 0.8;
        const pulse = st.reduced ? 0.6 : (now % 1600) / 1600;
        beaconRings(ctx, p.x, p.y, base, pulse, captured);
        if (captured) drawCaptureFlash(ctx, p, base * 0.42, st.flash.get(c), now);
        if (img) {
            ctx.save(); ctx.globalCompositeOperation = 'lighter'; bloomSprite(ctx, img, p.x, p.y, sz, captured ? 0.35 : 0.2); ctx.restore();
        } else {
            beaconSquare(ctx, p.x, p.y, captured);
        }
    }
}

// drawCaptureFlash — вспышка захвата маяка 180 мс + кольцо-разлёт 400 мс.
function drawCaptureFlash(ctx, p, r, flash, now) {
    if (flash == null) return;
    const dt = now - flash;
    if (dt < 180) radial(ctx, p.x, p.y, r * 1.8, C.COLORS.captureSoft, 0.8 * (1 - dt / 180));
    else if (dt < 580) ring(ctx, p.x, p.y, r * (1 + (dt - 180) / 400 * 1.2), C.COLORS.captureSoft, 0.7 * (1 - (dt - 180) / 400), 2);
}
