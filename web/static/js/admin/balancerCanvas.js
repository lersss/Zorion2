// web/static/js/admin/balancerCanvas.js
// Canvas-рендер вкладки «Балансировка» (спека 99.2.17 §7): кривая (ломанная
// по серверным точкам sample), узлы-ручки, bend-ромбики ◆ на середине
// сегментов, эталонные крестики ×, стрелочки ▲/▼ за краем видимой области.
// Преобразование данных→экран (лог/линейная × масштаб/сдвиг) — в одном месте
// (dataToScreen/screenToData): узлы, кривая, крестики и стрелочки рисуются
// через него, артефактов рассинхрона нет. evaluateCurveClient/bendTransformClient
// дублируют серверную математику ТОЛЬКО для live-preview при drag (§7 п.4);
// при сохранении (PUT) сервер пересчитывает по-настоящему.

export function decadeFloor(x) {
    return Math.pow(10, Math.floor(Math.log10(x)));
}

export function decadeCeil(x) {
    return Math.pow(10, Math.ceil(Math.log10(x)));
}

// Клиентский дубликат bendTransform (§2) — для live-preview.
export function bendTransformClient(t, k) {
    if (Math.abs(k) < 1e-10) return t;
    return (Math.exp(k * t) - 1) / (Math.exp(k) - 1);
}

// Клиентский дубликат evaluateCurve (§2/§5) — для live-preview при drag.
export function evaluateCurveClient(nodes, bends, x) {
    if (x <= nodes[0].x) return nodes[0].y;
    if (x >= nodes[nodes.length - 1].x) return nodes[nodes.length - 1].y;
    for (let i = 0; i < bends.length; i++) {
        if (x <= nodes[i + 1].x) {
            const t = (x - nodes[i].x) / (nodes[i + 1].x - nodes[i].x);
            return nodes[i].y + (nodes[i + 1].y - nodes[i].y) * bendTransformClient(t, bends[i]);
        }
    }
    return nodes[nodes.length - 1].y;
}

const PAD = { top: 12, right: 12, bottom: 24, left: 8 };

const COLORS = {
    curve: '#4a9eff',
    node: '#fbbf24',
    bend: '#a78bfa',
    etalon: '#f87171',
    threshold: '#f59e0b',
    zero: '#334155',
    axis: '#64748b',
    grid: 'rgba(100, 116, 139, 0.22)',
};

export const HIT_NODE_R = 10;
export const HIT_BEND_R = 8;

function plotSize(canvas) {
    return {
        w: canvas.width - PAD.left - PAD.right,
        h: canvas.height - PAD.top - PAD.bottom,
    };
}

// dataToScreen — данные (x, y) → экранные px по текущему view (лог/линейная).
// Y — в ПРОЦЕНТАХ (R×100, решение создателя 2026-09-15): view.yMin/yMax — %,
// точки данных конвертятся ×100 в местах вызовов (данные остаются в долях R).
export function dataToScreen(canvas, view, x, yPct) {
    const s = plotSize(canvas);
    const sx = PAD.left + (x - view.xMin) / (view.xMax - view.xMin) * s.w;
    let yy = yPct;
    if (view.logY && yPct <= 0) yy = view.yMin; // ровный ноль — нижняя кромка
    let frac;
    if (view.logY) {
        const lyMin = Math.log10(view.yMin);
        const lyMax = Math.log10(view.yMax);
        frac = (Math.log10(yy) - lyMin) / (lyMax - lyMin);
    } else {
        frac = (yy - view.yMin) / (view.yMax - view.yMin);
    }
    return { x: sx, y: PAD.top + (1 - frac) * s.h };
}

// screenToData — экранные px → данные (обратно dataToScreen). Y — в %.
export function screenToData(canvas, view, sx, sy) {
    const s = plotSize(canvas);
    const x = view.xMin + (sx - PAD.left) / s.w * (view.xMax - view.xMin);
    let y;
    if (view.logY) {
        const lyMin = Math.log10(view.yMin);
        const lyMax = Math.log10(view.yMax);
        const frac = 1 - (sy - PAD.top) / s.h;
        y = Math.pow(10, lyMin + frac * (lyMax - lyMin));
    } else {
        const frac = 1 - (sy - PAD.top) / s.h;
        y = view.yMin + frac * (view.yMax - view.yMin);
    }
    return { x, y };
}

// midY — значение кривой на середине сегмента i (позиция bend-ромбика).
function midY(nodes, bends, i) {
    const xm = (nodes[i].x + nodes[i + 1].x) / 2;
    return evaluateCurveClient(nodes, bends, xm);
}

function drawPolyline(ctx, canvas, view, points) {
    if (!points || points.length < 2) return;
    ctx.beginPath();
    let started = false;
    for (const p of points) {
        // Точки данных — в долях R → конверсия ×100 (ось Y в процентах).
        const sp = dataToScreen(canvas, view, p.x, p.y * 100);
        if (sp.y < -2000 || sp.y > canvas.height + 2000) {
            started = false; // разрыв при вылете за край (лог-шкала)
            continue;
        }
        if (!started) {
            ctx.moveTo(sp.x, sp.y);
            started = true;
        } else {
            ctx.lineTo(sp.x, sp.y);
        }
    }
    ctx.stroke();
}

// render — полная отрисовка холста по текущему состоянию.
// state: { nodes, bends, sampled, etalons, xs (сетка для preview), dirty,
//          factoryNodes, factoryBends (оверлей заводской, 99.2.23 §4.3) }
export function render(canvas, view, state) {
    const ctx = canvas.getContext('2d');
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // Ровный ноль (пунктир) — нижняя кромка в обеих шкалах.
    const zeroTop = dataToScreen(canvas, view, view.xMin, view.yMin).y;
    ctx.strokeStyle = COLORS.zero;
    ctx.setLineDash([4, 4]);
    ctx.beginPath();
    ctx.moveTo(PAD.left, zeroTop);
    ctx.lineTo(canvas.width - PAD.right, zeroTop);
    ctx.stroke();
    ctx.setLineDash([]);

    // Маркер порога включения эффект-компоненты (§4.4/§7.1): вертикальная
    // пунктирная линия в X порога (нулевой префикс кривой). Рисуется только
    // для эффект-компонент (state.threshold != null) и если порог в видимой
    // области.
    if (state.threshold != null && state.threshold > view.xMin && state.threshold < view.xMax) {
        const sp = dataToScreen(canvas, view, state.threshold, view.yMin);
        ctx.strokeStyle = COLORS.threshold;
        ctx.lineWidth = 1;
        ctx.setLineDash([3, 3]);
        ctx.beginPath();
        ctx.moveTo(sp.x, PAD.top);
        ctx.lineTo(sp.x, canvas.height - PAD.bottom);
        ctx.stroke();
        ctx.setLineDash([]);
    }

    // Заводская кривая (оверлей, 99.2.23 §4.3): пунктирная линия factory
    // поверх active (в цвет компоненты, полупрозрачно) — видно расхождение
    // ручной правки от заводской. Оцифровка — клиентским evaluateCurveClient
    // по узлам factory (серверный sample для factory не нужен: узлы уже
    // загружены GET factory; математика та же, что у live-preview).
    if (state.factoryNodes && state.factoryNodes.length >= 2 && state.xs && state.xs.length >= 2) {
        const pts = state.xs.map(x => ({ x, y: evaluateCurveClient(state.factoryNodes, state.factoryBends || [], x) }));
        ctx.strokeStyle = COLORS.curve;
        ctx.globalAlpha = 0.45;
        ctx.lineWidth = 1.5;
        ctx.setLineDash([2, 4]);
        drawPolyline(ctx, canvas, view, pts);
        ctx.setLineDash([]);
        ctx.globalAlpha = 1;
    }

    // Кривая: серверные точки sample (сплошная, если чисто; пунктиром, если dirty).
    ctx.lineWidth = 2;
    ctx.strokeStyle = COLORS.curve;
    if (state.dirty) ctx.setLineDash([6, 4]);
    drawPolyline(ctx, canvas, view, state.sampled);
    ctx.setLineDash([]);

    // Preview текущей (правленой) кривой — сплошная поверх пунктира.
    if (state.dirty && state.xs && state.nodes.length >= 2) {
        const pts = state.xs.map(x => ({ x, y: evaluateCurveClient(state.nodes, state.bends, x) }));
        ctx.strokeStyle = COLORS.curve;
        ctx.lineWidth = 2;
        drawPolyline(ctx, canvas, view, pts);
    }

    // Эталонные крестики ×.
    ctx.lineWidth = 1.5;
    for (const e of state.etalons || []) {
        const sp = dataToScreen(canvas, view, e.x, e.y * 100);
        if (sp.y >= -20 && sp.y <= canvas.height + 20) {
            ctx.strokeStyle = COLORS.etalon;
            ctx.beginPath();
            ctx.moveTo(sp.x - 5, sp.y - 5);
            ctx.lineTo(sp.x + 5, sp.y + 5);
            ctx.moveTo(sp.x + 5, sp.y - 5);
            ctx.lineTo(sp.x - 5, sp.y + 5);
            ctx.stroke();
        }
    }

    // Bend-ромбики ◆ на середине сегментов (на текущей кривой).
    for (let i = 0; i < state.bends.length; i++) {
        const xm = (state.nodes[i].x + state.nodes[i + 1].x) / 2;
        const ym = midY(state.nodes, state.bends, i);
        const sp = dataToScreen(canvas, view, xm, ym * 100);
        if (sp.y < -20 || sp.y > canvas.height + 20) continue;
        ctx.fillStyle = COLORS.bend;
        ctx.beginPath();
        ctx.moveTo(sp.x, sp.y - HIT_BEND_R);
        ctx.lineTo(sp.x + HIT_BEND_R, sp.y);
        ctx.lineTo(sp.x, sp.y + HIT_BEND_R);
        ctx.lineTo(sp.x - HIT_BEND_R, sp.y);
        ctx.closePath();
        ctx.fill();
    }

    // Узлы-ручки.
    for (let i = 0; i < state.nodes.length; i++) {
        const sp = dataToScreen(canvas, view, state.nodes[i].x, state.nodes[i].y * 100);
        if (sp.y < -20 || sp.y > canvas.height + 20) continue;
        ctx.fillStyle = COLORS.node;
        ctx.beginPath();
        ctx.arc(sp.x, sp.y, HIT_NODE_R, 0, Math.PI * 2);
        ctx.fill();
        ctx.strokeStyle = '#0f172a';
        ctx.lineWidth = 2;
        ctx.stroke();
    }

    // Стрелочки ▲/▼ за краем видимой области (узлы и эталоны).
    ctx.lineWidth = 2;
    for (let i = 0; i < state.nodes.length; i++) {
        const n = state.nodes[i];
        if (n.y > view.yMax) drawArrow(ctx, canvas, view, n.x, 'down', COLORS.node);
        else if (n.y < view.yMin) drawArrow(ctx, canvas, view, n.x, 'up', COLORS.node);
    }
    for (const e of state.etalons || []) {
        if (e.y > view.yMax) drawArrow(ctx, canvas, view, e.x, 'down', COLORS.etalon);
        else if (e.y < view.yMin) drawArrow(ctx, canvas, view, e.x, 'up', COLORS.etalon);
    }

    // Шкала и сетка осей (UX-правка 2026-09-15): тики с подписями на обеих
    // осях, без e-нотации; Y — слева, X — внизу, не пересекаются.
    drawYScale(ctx, canvas, view);
    drawXScale(ctx, canvas, view);

    // Метка оси R (слева сверху, в свободной зоне над областью).
    ctx.fillStyle = COLORS.axis;
    ctx.font = '10px system-ui';
    ctx.fillText('R', 2, PAD.top - 6);

    // Подпись единиц оси X (слева снизу, отдельной строкой от тиков).
    ctx.font = '11px system-ui';
    if (state.xLabel) ctx.fillText(state.xLabel, 2, canvas.height - 14);

    // Плавающая подпись при drag (живые значения, UX-правка 2026-09-15):
    // плашка в правом верхнем углу канваса — не «прилипает» к курсору
    // (не мигает, не перекрывает узел), обновляется на каждом move.
    if (state.dragInfo && state.dragInfo.text) {
        const pad = 8;
        ctx.font = '12px system-ui';
        const tw = ctx.measureText(state.dragInfo.text).width;
        const bx = canvas.width - tw - pad * 2 - 4;
        const by = 3;
        ctx.fillStyle = 'rgba(15, 23, 42, 0.92)';
        ctx.fillRect(bx, by, tw + pad * 2, 22);
        ctx.strokeStyle = COLORS.axis;
        ctx.lineWidth = 1;
        ctx.strokeRect(bx, by, tw + pad * 2, 22);
        ctx.fillStyle = state.dragInfo.kind === 'bend' ? COLORS.bend : COLORS.node;
        ctx.fillText(state.dragInfo.text, bx + pad, by + 15);
    }
}

function drawArrow(ctx, canvas, view, x, dir, color) {
    const s = plotSize(canvas);
    const sx = PAD.left + (x - view.xMin) / (view.xMax - view.xMin) * s.w;
    const sy = dir === 'down' ? PAD.top + 5 : canvas.height - PAD.bottom - 5;
    ctx.strokeStyle = color;
    ctx.fillStyle = color;
    ctx.beginPath();
    if (dir === 'down') {
        ctx.moveTo(sx - 5, sy - 4);
        ctx.lineTo(sx + 5, sy - 4);
        ctx.lineTo(sx, sy + 4);
    } else {
        ctx.moveTo(sx - 5, sy + 4);
        ctx.lineTo(sx + 5, sy + 4);
        ctx.lineTo(sx, sy - 4);
    }
    ctx.closePath();
    ctx.fill();
}

// hitTest — что под курсором (mx, my — экранные px холста).
// → {type:'node', index} | {type:'bend', index} | {type:'etalon', index} |
//   {type:'indicator', kind:'node'|'etalon', index, dir} | null
export function hitTest(canvas, view, state, mx, my) {
    // Узлы (r=10).
    for (let i = 0; i < state.nodes.length; i++) {
        const sp = dataToScreen(canvas, view, state.nodes[i].x, state.nodes[i].y * 100);
        if (dist(mx, my, sp) <= HIT_NODE_R) return { type: 'node', index: i };
    }
    // Bend-ромбики (r=8).
    for (let i = 0; i < state.bends.length; i++) {
        const xm = (state.nodes[i].x + state.nodes[i + 1].x) / 2;
        const ym = midY(state.nodes, state.bends, i);
        const sp = dataToScreen(canvas, view, xm, ym * 100);
        if (dist(mx, my, sp) <= HIT_BEND_R) return { type: 'bend', index: i };
    }
    // Эталоны (магнит ~3 px на отпускание — в balancer.js; хит крестика здесь).
    for (let i = 0; i < (state.etalons || []).length; i++) {
        const sp = dataToScreen(canvas, view, state.etalons[i].x, state.etalons[i].y * 100);
        if (dist(mx, my, sp) <= 6) return { type: 'etalon', index: i };
    }
    // Стрелочки за краем.
    for (let i = 0; i < state.nodes.length; i++) {
        const n = state.nodes[i];
        const dir = n.y > view.yMax ? 'down' : n.y < view.yMin ? 'up' : null;
        if (dir) {
            const sp = arrowScreen(canvas, view, n.x, dir);
            if (dist(mx, my, sp) <= 8) return { type: 'indicator', kind: 'node', index: i, dir };
        }
    }
    for (let i = 0; i < (state.etalons || []).length; i++) {
        const e = state.etalons[i];
        const dir = e.y > view.yMax ? 'down' : e.y < view.yMin ? 'up' : null;
        if (dir) {
            const sp = arrowScreen(canvas, view, e.x, dir);
            if (dist(mx, my, sp) <= 8) return { type: 'indicator', kind: 'etalon', index: i, dir };
        }
    }
    return null;
}

function arrowScreen(canvas, view, x, dir) {
    const s = plotSize(canvas);
    const sx = PAD.left + (x - view.xMin) / (view.xMax - view.xMin) * s.w;
    const sy = dir === 'down' ? PAD.top + 5 : canvas.height - PAD.bottom - 5;
    return { x: sx, y: sy };
}

// dist — евклидово расстояние.
function dist(x1, y1, p) {
    return Math.hypot(x1 - p.x, y1 - p.y);
}

// ==================== Форматтеры подписей (без e-нотации) ====================

// sup — юникод-надстрочные цифры степени: sup(-14) → «⁻¹⁴», sup(3) → «³».
const SUP_DIGITS = ['⁰', '¹', '²', '³', '⁴', '⁵', '⁶', '⁷', '⁸', '⁹'];

export function sup(n) {
    const neg = n < 0 ? '⁻' : '';
    const s = String(Math.abs(n)).split('').map(d => SUP_DIGITS[Number(d)]).join('');
    return neg + s;
}

// fmtPow10 — подпись целой декады: 10⁰ → «1», 10¹ → «10», 10² → «100»,
// 10⁻¹² → «10⁻¹²» (юникод-степени, без e-нотации).
function fmtPow10(d) {
    if (d === 0) return '1';
    if (d === 1) return '10';
    if (d === 2) return '100';
    return '10' + sup(d);
}

// fmtR — читаемое значение R в ПРОЦЕНТАХ (R×100, решение создателя 2026-09-15):
// ≥ 0.1% — обычная дробь «9.48%»; меньше — юникод-степень «5.94·10⁻⁷%»
// (R = 5.94e-9 доли → 5.94e-7%); 0 → «0%». Без e-нотации.
export function fmtR(pct) {
    if (pct === 0) return '0%';
    if (pct >= 0.1) return String(Number(pct.toFixed(2))) + '%';
    let exp = Math.floor(Math.log10(pct));
    let mant = pct / Math.pow(10, exp);
    let m = Number(mant.toPrecision(3));
    if (m >= 10) { m /= 10; exp += 1; }
    if (m === 1) return fmtPow10(exp) + '%';
    return `${m}·10${sup(exp)}%`;
}

// fmtPlain — обычное число без e-нотации для тиков осей.
function fmtPlain(v) {
    if (v === 0) return '0';
    const abs = Math.abs(v);
    if (abs >= 1000) return String(Math.round(v));
    if (abs >= 100) return String(Number(v.toFixed(1)));
    if (abs >= 1) return String(Number(v.toPrecision(4)));
    return String(Number(v.toPrecision(3)));
}

// niceStep — «красивый» шаг 1/2/5·10^n для равномерных тиков.
function niceStep(raw) {
    const pow = Math.pow(10, Math.floor(Math.log10(raw)));
    const norm = raw / pow;
    const candidates = [1, 2, 5, 10];
    let best = candidates[0];
    let bestDiff = Infinity;
    for (const c of candidates) {
        const diff = Math.abs(norm - c);
        if (diff < bestDiff) { bestDiff = diff; best = c; }
    }
    return best * pow;
}

// ==================== Шкалы осей ====================

function drawHGridLine(ctx, canvas, y) {
    ctx.strokeStyle = COLORS.grid;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(PAD.left, y);
    ctx.lineTo(canvas.width - PAD.right, y);
    ctx.stroke();
}

function drawVGridLine(ctx, canvas, x) {
    ctx.strokeStyle = COLORS.grid;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x, PAD.top);
    ctx.lineTo(x, canvas.height - PAD.bottom);
    ctx.stroke();
}

// yLabel — подпись Y слева от области (не пересекается с X-подписями внизу).
function yLabel(ctx, canvas, y, text) {
    ctx.fillStyle = COLORS.axis;
    ctx.font = '11px system-ui';
    ctx.fillText(text, 2, y + 4);
}

// drawYScale — ось R (в %): лог — тики на каждой декаде («10⁻¹²%», «1%»,
// «10%», «100%»); линейная — 5-7 равномерных делений с красивым шагом.
// Если в видимой области нет ни одной целой декады (зум внутри декады) —
// подписи границ.
function drawYScale(ctx, canvas, view) {
    if (view.logY) {
        const lo = Math.ceil(Math.log10(view.yMin));
        const hi = Math.floor(Math.log10(view.yMax));
        if (lo <= hi) {
            for (let d = lo; d <= hi; d++) {
                const sp = dataToScreen(canvas, view, view.xMin, Math.pow(10, d));
                drawHGridLine(ctx, canvas, sp.y);
                yLabel(ctx, canvas, sp.y, fmtPow10(d) + '%');
            }
        } else {
            const sp0 = dataToScreen(canvas, view, view.xMin, view.yMin);
            const sp1 = dataToScreen(canvas, view, view.xMin, view.yMax);
            yLabel(ctx, canvas, sp1.y, fmtR(view.yMax));
            yLabel(ctx, canvas, sp0.y, fmtR(view.yMin));
        }
        return;
    }
    const step = niceStep((view.yMax - view.yMin) / 6);
    for (let v = Math.ceil(view.yMin / step) * step; v <= view.yMax; v += step) {
        const sp = dataToScreen(canvas, view, view.xMin, v);
        drawHGridLine(ctx, canvas, sp.y);
        yLabel(ctx, canvas, sp.y, fmtPlain(v));
    }
}

// drawXScale — ось X: 5-8 равномерных делений с красивым шагом, вертикальные
// тики-линии, подписи под осью (не заезжают за края области).
function drawXScale(ctx, canvas, view) {
    const step = niceStep((view.xMax - view.xMin) / 6);
    const start = Math.ceil(view.xMin / step) * step;
    for (let v = start; v <= view.xMax; v += step) {
        const sp = dataToScreen(canvas, view, v, view.yMin);
        drawVGridLine(ctx, canvas, sp.x);
        ctx.fillStyle = COLORS.axis;
        ctx.font = '11px system-ui';
        const text = fmtPlain(v);
        const tw = ctx.measureText(text).width;
        let tx = sp.x - tw / 2;
        if (tx < PAD.left) tx = PAD.left;
        if (tx + tw > canvas.width - PAD.right) tx = canvas.width - PAD.right - tw;
        ctx.fillText(text, tx, canvas.height - 6);
    }
}