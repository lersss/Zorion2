// web/static/js/admin/chart.js
// Минимальные canvas-чарты для админки: столбики и линии.
// Никаких зависимостей — чистый 2D.

const FONT = '11px system-ui, sans-serif';

export function setupCanvas(canvas, width, height) {
    const dpr = window.devicePixelRatio || 1;
    canvas.width = width * dpr;
    canvas.height = height * dpr;
    canvas.style.width = width + 'px';
    canvas.style.height = height + 'px';
    const ctx = canvas.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, width, height);
    return ctx;
}

// drawBars — серия(и) столбиков. series: [{color, bins: [{lo, hi, count}]}]
export function drawBars(canvas, series, opts = {}) {
    const width = opts.width || 700;
    const height = opts.height || 260;
    const ctx = setupCanvas(canvas, width, height);
    const pad = { l: 46, r: 8, t: 10, b: 26 };
    const plotW = width - pad.l - pad.r;
    const plotH = height - pad.t - pad.b;

    const maxCount = Math.max(1, ...series.flatMap(s => s.bins.map(b => b.count)));
    const n = series[0].bins.length;
    const bw = plotW / n;
    const perSeries = bw / series.length;

    series.forEach((s, si) => {
        ctx.globalAlpha = series.length > 1 ? 0.55 : 1;
        ctx.fillStyle = s.color || '#60a5fa';
        s.bins.forEach((b, i) => {
            const h = plotH * (b.count / maxCount);
            const x = pad.l + i * bw + si * perSeries + 1;
            ctx.fillRect(x, pad.t + plotH - h, perSeries - 2, h);
        });
    });
    ctx.globalAlpha = 1;

    drawAxes(ctx, pad, plotW, plotH, {
        xTicks: xTicksForBins(series[0].bins, bw, pad.l, plotW),
        yMax: maxCount,
    });
    ctx.fillStyle = '#94a3b8';
    ctx.font = FONT;
    ctx.fillText('время жизни, сутки', pad.l + plotW / 2 - 55, height - 6);
    ctx.fillText('поселений', 8, pad.t + plotH / 2);
}

// drawLines — серия(и) линий. points: [{x, y}], x и y нормализуются по всем сериям.
export function drawLines(canvas, series, opts = {}) {
    const width = opts.width || 700;
    const height = opts.height || 260;
    const ctx = setupCanvas(canvas, width, height);
    const pad = { l: 46, r: 8, t: 10, b: 26 };
    const plotW = width - pad.l - pad.r;
    const plotH = height - pad.t - pad.b;

    let xMin = Infinity, xMax = -Infinity, yMin = Infinity, yMax = -Infinity;
    for (const s of series) {
        for (const p of s.points) {
            if (p.x < xMin) xMin = p.x;
            if (p.x > xMax) xMax = p.x;
            if (p.y < yMin) yMin = p.y;
            if (p.y > yMax) yMax = p.y;
        }
    }
    if (xMin === Infinity) { xMin = 0; xMax = 1; }
    if (yMin === Infinity) { yMin = 0; yMax = 1; }
    if (xMax === xMin) xMax = xMin + 1;
    if (yMax === yMin) yMax = yMin + 1;

    const px = x => pad.l + ((x - xMin) / (xMax - xMin)) * plotW;
    const py = y => pad.t + plotH - ((y - yMin) / (yMax - yMin)) * plotH;

    series.forEach(s => {
        ctx.strokeStyle = s.color || '#60a5fa';
        ctx.lineWidth = 2;
        ctx.beginPath();
        s.points.forEach((p, i) => {
            const X = px(p.x), Y = py(p.y);
            if (i === 0) ctx.moveTo(X, Y);
            else ctx.lineTo(X, Y);
        });
        ctx.stroke();
    });

    drawAxes(ctx, pad, plotW, plotH, {
        xTicks: ticks(5, xMin, xMax, pad.l, plotW, v => fmt(v)),
        yTicks: ticks(4, yMin, yMax, pad.t, plotH, v => v.toFixed(1)),
    });

    if (opts.markerX !== undefined && opts.markerX >= xMin && opts.markerX <= xMax) {
        const X = px(opts.markerX);
        ctx.strokeStyle = '#fbbf24';
        ctx.setLineDash([4, 4]);
        ctx.beginPath();
        ctx.moveTo(X, pad.t);
        ctx.lineTo(X, pad.t + plotH);
        ctx.stroke();
        ctx.setLineDash([]);
    }

    ctx.fillStyle = '#94a3b8';
    ctx.font = FONT;
    ctx.fillText('сутки', pad.l + plotW / 2 - 15, height - 6);
    ctx.fillText('доля живых', 8, pad.t + plotH / 2);
}

function drawAxes(ctx, pad, plotW, plotH, opts) {
    ctx.strokeStyle = '#334155';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(pad.l, pad.t);
    ctx.lineTo(pad.l, pad.t + plotH);
    ctx.lineTo(pad.l + plotW, pad.t + plotH);
    ctx.stroke();

    ctx.fillStyle = '#94a3b8';
    ctx.font = FONT;
    ctx.textAlign = 'center';
    for (const t of (opts.xTicks || [])) {
        ctx.fillText(t.label, t.x, pad.t + plotH + 14);
    }
    ctx.textAlign = 'right';
    for (const t of (opts.yTicks || [])) {
        ctx.fillText(t.label, pad.l - 6, t.y + 4);
    }
    ctx.textAlign = 'start';
}

function xTicksForBins(bins, bw, padL, plotW) {
    const step = Math.max(1, Math.floor(bins.length / 5));
    const out = [];
    for (let i = 0; i < bins.length; i += step) {
        out.push({ x: padL + i * bw + bw / 2, label: fmt(bins[i].lo) });
    }
    return out;
}

function ticks(count, min, max, origin, span, fmtFn) {
    const out = [];
    for (let i = 0; i <= count; i++) {
        const f = i / count;
        out.push({
            x: origin + f * span,
            y: origin + f * span,
            label: fmtFn(min + (max - min) * f),
        });
    }
    return out;
}

function fmt(v) {
    if (v >= 1000000) return (v / 1000000).toFixed(1) + 'M';
    if (v >= 10000) return Math.round(v / 1000) + 'k';
    if (v >= 1000) return (v / 1000).toFixed(1) + 'k';
    if (Number.isInteger(v)) return String(v);
    return v.toFixed(1);
}