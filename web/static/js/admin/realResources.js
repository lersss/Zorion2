// web/static/js/admin/realResources.js
// 6 графических форматов профиля ресурса для подвкладки «Реальные вещества»
// (идея 2026-09-18 §4б, этап 2): 1 радар-паутина (canvas), 2 бары,
// 3 тепловая матрица, 4 термометры с окнами, 5 ярлыки-градации (§9.1.5),
// 6 профиль-полоска. Чистые рендеры — состояние (режим/формат/окна)
// живёт в resources.js. Тёмная тема: фон #0f172a, сетка rgba(100,116,139,0.25).

// AXIS_KEYS — ключи JSON-полей 10 осей (фиксированный порядок, как в
// layer.go: твёрдость, эластичность, проводимость, плотность, энергоёмкость,
// биосовместимость, радиоактивность, токсичность, горючесть, хим. активность).
export const AXIS_KEYS = [
    'hardness', 'elasticity', 'conductivity', 'density', 'energy_density',
    'biocompatibility', 'radioactivity', 'toxicity', 'flammability', 'chemical_activity',
];

// SHORT_LABELS — короткие метки осей для тесных форматов (радар, бары,
// матрица, термометры, легенда полосок): полные имена не влезают без обрезки.
// Полные имена — AXIS_NAMES в resources.js (карточка/ярлыки/итог-строки).
const SHORT_LABELS = {
    hardness: 'твёрдость',
    elasticity: 'эластичн.',
    conductivity: 'проводим.',
    density: 'плотность',
    energy_density: 'энергоёмк.',
    biocompatibility: 'биосовмест.',
    radioactivity: 'радиоакт.',
    toxicity: 'токсичн.',
    flammability: 'горючесть',
    chemical_activity: 'хим. активн.',
};

// shortLabel — короткая метка оси (тесные форматы).
export function shortLabel(key) { return SHORT_LABELS[key] || key; }

// k2c — кельвины → °C. Интерфейс показывает ТОЛЬКО °C (решение создателя
// 2026-09-18); кельвины — внутренняя единица модели (данные/API/формулы).
export function k2c(k) { return Math.round(k - 273.15); }

// ==================== Общие хелперы ====================

// axisIntervals — интервалы окон по оси (из набора окон).
function axisIntervals(windows, axisKey) {
    const ivs = [];
    for (const w of windows || []) {
        const iv = w.axes[axisKey];
        if (iv) ivs.push(iv);
    }
    return ivs;
}

// windowColor — цвет окна: зелёный = товар, голубой = раса.
function windowColor(windowType) {
    return windowType === 'races' ? '#38bdf8' : '#22c55e';
}

// resultLine — итог-строка окон. single=true (выбрано одно окно) — детально
// с причиной промаха; single=false («все») — компактно «Броня ✗ · Топливо ✗…»
// без шума деталей (каша из 13 строк нечитаема). Температуры — в °C.
function resultLine(results, labels, single) {
    if (!results || !results.length) return '';
    const parts = results.map(r => {
        if (r.ok) return `<span style="color:#22c55e;">${r.name} ✓</span>`;
        if (!single) return `<span style="color:#f87171;">${r.name} ✗</span>`;
        const m = r.misses[0];
        const label = m.axis === 't_melt' ? 'T_melt' : m.axis === 't_boil' ? 'T_boil' : (labels[m.axis] || m.axis);
        const isT = m.axis === 't_melt' || m.axis === 't_boil';
        const v = isT ? k2c(m.v) : m.v;
        const lim = isT ? k2c(m.dir === 'lo' ? m.iv.lo : m.iv.hi) : (m.dir === 'lo' ? m.iv.lo : m.iv.hi);
        const unit = isT ? ' °C' : '';
        return `<span style="color:#f87171;">${r.name} ✗ (${label} ${v}${unit} ${m.dir === 'lo' ? '<' : '>'} ${lim}${unit})</span>`;
    });
    return `<div style="margin-top:8px; font-size:1.05rem; display:flex; flex-wrap:wrap; gap:2px 14px;">${parts.join('')}</div>`;
}

// heatColor — цвет ячейки матрицы по шкале 0–100: синий→зелёный→жёлтый→красный.
function heatColor(v) {
    const stops = [[0, [59, 130, 246]], [50, [34, 197, 94]], [75, [234, 179, 8]], [100, [239, 68, 68]]];
    let a = stops[0], b = stops[stops.length - 1];
    for (let i = 0; i < stops.length - 1; i++) {
        if (v >= stops[i][0] && v <= stops[i + 1][0]) { a = stops[i]; b = stops[i + 1]; break; }
    }
    const t = (b[0] === a[0]) ? 0 : (v - a[0]) / (b[0] - a[0]);
    const c = a[1].map((x, i) => Math.round(x + (b[1][i] - x) * t));
    return `rgb(${c[0]}, ${c[1]}, ${c[2]})`;
}

// tPos — позиция температуры на лог-шкале 10–6000 K (в %).
function tPos(t) {
    const lo = Math.log10(10), hi = Math.log10(6000);
    return (Math.log10(Math.max(t, 10)) - lo) / (hi - lo) * 100;
}

// ==================== Формат 1: радар-паутина (canvas) ====================

// drawRadar — паутина 10 осей: кольца сетки, оси ≥ 70 ярче/жирнее, тень под
// полигоном, в центре чип фазы при 293 K + T + флаг, окна — дуга-сектор на
// луче, провал окна — красная точка на вершине.
// Зум: canvas.dataset.zoom (1 = полный диапазон 0–100; >1 приближает низкие
// значения). Колесо мыши меняет, двойной клик сбрасывает (навешивается в
// resources.js renderFormat). Подписи — короткие метки (shortLabel).
// ctx: { labels, color, windows, windowType, phase }.
export function drawRadar(canvas, res, ctx) {
    const { labels, color, windows, windowType, phase } = ctx;
    const c = canvas.getContext('2d');
    const w = canvas.width, h = canvas.height;
    const cx = w / 2, cy = h / 2;
    const R = Math.min(w, h) / 2 - 48;
    const n = AXIS_KEYS.length;
    const zoom = parseFloat(canvas.dataset.zoom || '1');
    const scale = 100 / zoom; // максимальное отображаемое значение
    const sv = v => R * Math.min(1, Math.max(0, v / scale)); // значение → радиус
    c.clearRect(0, 0, w, h);

    // Кольца сетки — на значениях ≤ scale (иначе вне поля зрения).
    for (const val of [25, 50, 75, 100]) {
        if (val > scale + 0.01) continue;
        const frac = val / scale;
        c.beginPath();
        for (let i = 0; i <= n; i++) {
            const ang = -Math.PI / 2 + i * 2 * Math.PI / n;
            const x = cx + R * frac * Math.cos(ang);
            const y = cy + R * frac * Math.sin(ang);
            if (i === 0) c.moveTo(x, y); else c.lineTo(x, y);
        }
        c.strokeStyle = 'rgba(100, 116, 139, 0.25)';
        c.lineWidth = 1;
        c.stroke();
    }
    // Граница масштаба (кольцо на scale).
    c.beginPath();
    for (let i = 0; i <= n; i++) {
        const ang = -Math.PI / 2 + i * 2 * Math.PI / n;
        const x = cx + R * Math.cos(ang);
        const y = cy + R * Math.sin(ang);
        if (i === 0) c.moveTo(x, y); else c.lineTo(x, y);
    }
    c.strokeStyle = 'rgba(100, 116, 139, 0.5)';
    c.lineWidth = 1;
    c.stroke();
    // Подпись масштаба (видна при зуме).
    if (zoom > 1.01) {
        c.textAlign = 'left';
        c.textBaseline = 'top';
        c.fillStyle = '#64748b';
        c.font = '11px system-ui';
        c.fillText(`0–${Math.round(scale)}`, 8, 8);
    }
    // Лучи (ось со значением ≥ 70 — ярче и жирнее).
    for (let i = 0; i < n; i++) {
        const ang = -Math.PI / 2 + i * 2 * Math.PI / n;
        const v = res[AXIS_KEYS[i]] || 0;
        c.beginPath();
        c.moveTo(cx, cy);
        c.lineTo(cx + R * Math.cos(ang), cy + R * Math.sin(ang));
        c.strokeStyle = v >= 70 ? 'rgba(147, 197, 253, 0.7)' : 'rgba(100, 116, 139, 0.25)';
        c.lineWidth = v >= 70 ? 2 : 1;
        c.stroke();
    }
    // Окна: дуга-сектор интервала на луче.
    if (windows) {
        const wc = windowColor(windowType);
        for (let i = 0; i < n; i++) {
            const ang = -Math.PI / 2 + i * 2 * Math.PI / n;
            const ivs = axisIntervals(windows, AXIS_KEYS[i]);
            for (const iv of ivs) {
                const r0 = sv(iv.lo), r1 = sv(iv.hi);
                c.beginPath();
                c.arc(cx, cy, r1, ang - 0.07, ang + 0.07);
                c.arc(cx, cy, r0, ang + 0.07, ang - 0.07, true);
                c.closePath();
                c.fillStyle = wc + '55';
                c.fill();
            }
        }
    }
    // Полигон профиля с тенью.
    c.save();
    c.shadowColor = color;
    c.shadowBlur = 14;
    c.beginPath();
    for (let i = 0; i < n; i++) {
        const ang = -Math.PI / 2 + i * 2 * Math.PI / n;
        const v = res[AXIS_KEYS[i]] || 0;
        const x = cx + sv(v) * Math.cos(ang);
        const y = cy + sv(v) * Math.sin(ang);
        if (i === 0) c.moveTo(x, y); else c.lineTo(x, y);
    }
    c.closePath();
    c.fillStyle = color + '2e';
    c.fill();
    c.strokeStyle = color;
    c.lineWidth = 2;
    c.stroke();
    c.restore();

    // Провал окна — красная точка на вершине.
    if (windows) {
        for (let i = 0; i < n; i++) {
            const key = AXIS_KEYS[i];
            const v = res[key] || 0;
            const ivs = axisIntervals(windows, key);
            if (ivs.length === 0) continue;
            const inAny = ivs.some(iv => v >= iv.lo && v <= iv.hi);
            if (!inAny) {
                const ang = -Math.PI / 2 + i * 2 * Math.PI / n;
                const x = cx + sv(v) * Math.cos(ang);
                const y = cy + sv(v) * Math.sin(ang);
                c.fillStyle = '#ef4444';
                c.beginPath();
                c.arc(x, y, 4, 0, Math.PI * 2);
                c.fill();
            }
        }
    }

    // Центр: чип фазы + T + флаг.
    c.textAlign = 'center';
    c.fillStyle = '#e0e0e0';
    c.font = 'bold 14px system-ui';
    c.fillText(phase, cx, cy - 2);
    c.fillStyle = '#94a3b8';
    c.font = '12px system-ui';
    c.fillText(`${k2c(res.t_melt)}/${k2c(res.t_boil)} °C`, cx, cy + 14);
    if (res.sublimating) {
        c.fillStyle = '#f87171';
        c.font = '12px system-ui';
        c.fillText('сублимирует', cx, cy + 28);
    }

    // Подписи осей (≥ 70 — ярче и жирнее; короткие метки, отступ 28px).
    // Умное позиционирование: если подпись не влезает в canvas по горизонтали
    // (малые размеры «Все форматы»), сдвигаем её внутрь — без обрезки.
    for (let i = 0; i < n; i++) {
        const ang = -Math.PI / 2 + i * 2 * Math.PI / n;
        const v = res[AXIS_KEYS[i]] || 0;
        const label = shortLabel(AXIS_KEYS[i]);
        const lx = cx + (R + 28) * Math.cos(ang);
        const ly = cy + (R + 28) * Math.sin(ang);
        c.textAlign = Math.abs(Math.cos(ang)) < 0.3 ? 'center' : (Math.cos(ang) > 0 ? 'left' : 'right');
        c.textBaseline = Math.abs(Math.sin(ang)) < 0.3 ? 'middle' : (Math.sin(ang) > 0 ? 'top' : 'bottom');
        c.font = v >= 70 ? 'bold 13px system-ui' : '13px system-ui';
        c.fillStyle = v >= 70 ? '#93c5fd' : '#94a3b8';
        const tw = c.measureText(label).width;
        let tx = lx;
        if (Math.abs(Math.cos(ang)) >= 0.3) {
            if (Math.cos(ang) > 0) tx = Math.min(lx, w - tw - 6);   // правая: не за правый край
            else tx = Math.max(lx, tw + 6);                          // левая: не за левый край
        }
        c.fillText(label, tx, ly);
    }
}

// ==================== Формат 2: горизонтальные бары ====================

// renderBars — 10 строк: полоса 0–100 с делениями, заливка до значения +
// число справа; ось ≥ 70 — ярче; окно — закрашенная зона-скобка на треке,
// маркер в зоне = пройдено (зелёный), иначе красный; футер T/фаза; итог окон.
export function renderBars(res, ctx) {
    const { labels, color, windows, windowType, phase, results } = ctx;
    const wc = windowColor(windowType);
    const rows = AXIS_KEYS.map(key => {
        const v = res[key] || 0;
        const ivs = axisIntervals(windows, key);
        const inAny = ivs.some(iv => v >= iv.lo && v <= iv.hi);
        const zones = ivs.map(iv =>
            `<div style="position:absolute; left:${iv.lo}%; width:${iv.hi - iv.lo}%; top:0; bottom:0; background:${wc}33; border-left:1px solid ${wc}; border-right:1px solid ${wc};"></div>`).join('');
        const marker = ivs.length > 0
            ? `<div style="position:absolute; left:${v}%; top:-2px; width:2px; height:18px; background:${inAny ? '#22c55e' : '#ef4444'};"></div>`
            : '';
        return `<div style="display:flex; align-items:center; gap:8px; font-size:1.15rem;">
            <span style="width:150px; text-align:right; color:${v >= 70 ? '#93c5fd' : '#94a3b8'}; font-weight:${v >= 70 ? '600' : '400'};">${shortLabel(key)}</span>
            <div style="flex:1; position:relative; height:26px; background:#1e293b; border-radius:6px;">
                ${zones}
                <div style="position:absolute; left:0; top:0; bottom:0; width:${v}%; background:${v >= 70 ? color : color + '88'}; border-radius:6px;"></div>
                ${marker}
            </div>
            <span style="width:48px; text-align:right; color:#cbd5e1;">${v}</span>
        </div>`;
    }).join('');
    const footer = `<div style="margin-top:8px; font-size:1.05rem; color:#94a3b8;">T_melt ${k2c(res.t_melt)} °C · T_boil ${k2c(res.t_boil)} °C · при 20 °C — <b style="color:#e0e0e0;">${phase}</b>${res.sublimating ? ' <span style="color:#f87171;">(сублимирует)</span>' : ''}</div>`;
    return `<div style="display:flex; flex-direction:column; gap:6px;">${rows}</div>${footer}${resultLine(results, labels, ctx.single)}`;
}

// ==================== Формат 3: тепловая матрица ====================

// renderMatrix — сетка 2×5, ячейка = ось (подпись + число + заливка по шкале
// 0–100 синий→зелёный→жёлтый→красный); окна — рамка ячейки зелёная/красная;
// футер T/фаза; строка-итог окон.
export function renderMatrix(res, ctx) {
    const { labels, windows, windowType, phase, results } = ctx;
    const cells = AXIS_KEYS.map(key => {
        const v = res[key] || 0;
        const ivs = axisIntervals(windows, key);
        const inAny = ivs.some(iv => v >= iv.lo && v <= iv.hi);
        const frame = ivs.length > 0 ? (inAny ? '2px solid #22c55e' : '2px solid #ef4444') : '1px solid #2a2a4a';
        return `<div style="background:${heatColor(v)}; border:${frame}; border-radius:8px; padding:12px 10px; text-align:center;">
            <div style="font-size:1.05rem; color:#0f172a; font-weight:600; white-space:nowrap;">${shortLabel(key)}</div>
            <div style="font-size:1.6rem; color:#0f172a; font-weight:700;">${v}</div>
        </div>`;
    }).join('');
    const footer = `<div style="margin-top:8px; font-size:1.05rem; color:#94a3b8;">T_melt ${k2c(res.t_melt)} °C · T_boil ${k2c(res.t_boil)} °C · при 20 °C — <b style="color:#e0e0e0;">${phase}</b>${res.sublimating ? ' <span style="color:#f87171;">(сублимирует)</span>' : ''}</div>`;
    return `<div style="display:grid; grid-template-columns:repeat(5, 1fr); gap:8px;">${cells}</div>${footer}${resultLine(results, labels, ctx.single)}`;
}

// ==================== Формат 4: термометры с зонами и окнами ====================

// renderThermo — главный носитель окон: по каждой оси шкала 0–100 с тремя
// зонами (низ/сред/высок), светящийся маркер значения, поверх — закрашенный
// интервал окна (зелёный = товар, голубой = раса); отдельный термометр T:
// зоны фаз твёрдое/жидкое/газ, маркер 293 K, поверх — T-окна хемотипов;
// итог-строка.
export function renderThermo(res, ctx) {
    const { labels, windows, windowType, phase, results } = ctx;
    const wc = windowColor(windowType);
    const rows = AXIS_KEYS.map(key => {
        const v = res[key] || 0;
        const ivs = axisIntervals(windows, key);
        const zones = `
            <div style="position:absolute; left:0; width:33.3%; top:0; bottom:0; background:rgba(100,116,139,0.12);"></div>
            <div style="position:absolute; left:66.6%; width:33.4%; top:0; bottom:0; background:rgba(100,116,139,0.12);"></div>`;
        const winZones = ivs.map(iv =>
            `<div style="position:absolute; left:${iv.lo}%; width:${iv.hi - iv.lo}%; top:0; bottom:0; background:${wc}44; border-left:1px solid ${wc}; border-right:1px solid ${wc};"></div>`).join('');
        const marker = `<div style="position:absolute; left:${v}%; top:-3px; width:2px; height:20px; background:#f1f5f9; box-shadow:0 0 6px #f1f5f9;"></div>`;
        return `<div style="display:flex; align-items:center; gap:8px; font-size:1.15rem;">
            <span style="width:150px; text-align:right; color:#94a3b8;">${shortLabel(key)}</span>
            <div style="flex:1; position:relative; height:26px; background:#0f172a; border:1px solid #2a2a4a; border-radius:6px;">
                ${zones}${winZones}${marker}
            </div>
            <span style="width:48px; text-align:right; color:#cbd5e1;">${v}</span>
        </div>`;
    }).join('');
    const tThermo = renderTThermo(res, ctx);
    return `<div style="display:flex; flex-direction:column; gap:6px;">${rows}</div>${tThermo}${resultLine(results, labels, ctx.single)}`;
}

// renderTThermo — термометр T (лог-шкала 10–6000 K): зоны фаз ресурса
// (твёрдое/жидкое/газ), маркер 293 K, поверх — T-окна хемотипов.
function renderTThermo(res, ctx) {
    const { windows, windowType } = ctx;
    const wc = windowColor(windowType);
    const tMelt = res.t_melt, tBoil = res.t_boil;
    const zones = `
        <div style="position:absolute; left:0; width:${tPos(tMelt)}%; top:0; bottom:0; background:rgba(100,116,139,0.15);"></div>
        <div style="position:absolute; left:${tPos(tMelt)}%; width:${tPos(tBoil) - tPos(tMelt)}%; top:0; bottom:0; background:rgba(59,130,246,0.2);"></div>
        <div style="position:absolute; left:${tPos(tBoil)}%; width:${100 - tPos(tBoil)}%; top:0; bottom:0; background:rgba(239,68,68,0.15);"></div>`;
    const winZones = [];
    for (const w of windows || []) {
        if (w.t_melt) winZones.push(
            `<div style="position:absolute; left:${tPos(w.t_melt.lo)}%; width:${tPos(w.t_melt.hi) - tPos(w.t_melt.lo)}%; top:0; bottom:0; background:${wc}55; border-left:1px solid ${wc}; border-right:1px solid ${wc};"></div>`);
        if (w.t_boil) winZones.push(
            `<div style="position:absolute; left:${tPos(w.t_boil.lo)}%; width:${tPos(w.t_boil.hi) - tPos(w.t_boil.lo)}%; top:0; bottom:0; background:${wc}44; border-left:1px dashed ${wc}; border-right:1px dashed ${wc};"></div>`);
    }
    const marker = `<div style="position:absolute; left:${tPos(293)}%; top:-3px; width:2px; height:26px; background:#fbbf24; box-shadow:0 0 6px #fbbf24;"></div>`;
    return `<div style="display:flex; align-items:center; gap:8px; font-size:1.15rem; margin-top:8px;">
        <span style="width:150px; text-align:right; color:#94a3b8;">T (лог), °C</span>
        <div style="flex:1; position:relative; height:26px; background:#0f172a; border:1px solid #2a2a4a; border-radius:6px;">
            ${zones}${winZones.join('')}${marker}
        </div>
        <span style="width:48px; text-align:right; color:#fbbf24;">20 °C</span>
    </div>`;
}

// ==================== Формат 5: ярлыки-градации (§9.1.5) ====================

// GRADATIONS — 5 градаций на ось (свои слова), радиоактивность — 3
// (<20 фон / 20–60 опасное / >60 оружейное, решение §4б).
const GRADATIONS = {
    hardness: [[0, 'очень мягкое'], [20, 'мягкое'], [40, 'средней твёрдости'], [60, 'твёрдое'], [80, 'очень твёрдое']],
    elasticity: [[0, 'хрупкое'], [20, 'малоподатливое'], [40, 'упругое'], [60, 'эластичное'], [80, 'очень эластичное']],
    conductivity: [[0, 'изолятор'], [20, 'слабый проводник'], [40, 'проводник'], [60, 'хороший проводник'], [80, 'сверхпроводник']],
    density: [[0, 'очень лёгкое'], [20, 'лёгкое'], [40, 'средней плотности'], [60, 'плотное'], [80, 'очень плотное']],
    energy_density: [[0, 'инертное'], [20, 'низкая ёмкость'], [40, 'средняя ёмкость'], [60, 'высокая ёмкость'], [80, 'топливо']],
    biocompatibility: [[0, 'враждебно жизни'], [20, 'несовместимо'], [40, 'нейтрально'], [60, 'совместимо'], [80, 'биосовместимо']],
    radioactivity: [[0, 'фон'], [20, 'опасное'], [60, 'оружейное']],
    toxicity: [[0, 'безопасное'], [20, 'малотоксичное'], [40, 'умеренно токсичное'], [60, 'токсичное'], [80, 'смертельно токсичное']],
    flammability: [[0, 'не горит'], [20, 'трудновоспламеняемое'], [40, 'горючее'], [60, 'легко воспламеняемое'], [80, 'взрывоопасное']],
    chemical_activity: [[0, 'инертное'], [20, 'малоактивное'], [40, 'активное'], [60, 'очень активное'], [80, 'агрессивное']],
};

// gradation — ярлык + интервал градации для значения оси.
function gradation(key, v) {
    const g = GRADATIONS[key];
    for (let i = 0; i < g.length; i++) {
        const next = i === g.length - 1 ? Infinity : g[i + 1][0];
        if (v >= g[i][0] && v < next) {
            return { label: g[i][1], lo: g[i][0], hi: next === Infinity ? 100 : next };
        }
    }
    const last = g[g.length - 1];
    return { label: last[1], lo: last[0], hi: 100 };
}

// tMeltLabel/tBoilLabel — температурные ярлыки без кельвинов (§9.1.5).
function tMeltLabel(t) {
    if (t < 300) return 'легкоплавкое';
    if (t < 1000) return 'плавкое';
    if (t < 2000) return 'тугоплавкое';
    return 'сверхтугоплавкое';
}
function tBoilLabel(t) {
    if (t < 400) return 'летучее';
    if (t < 1500) return 'среднекипящее';
    if (t < 3000) return 'высококипящее';
    return 'сверхвысококипящее';
}

// renderLabels — игровой вид: прилагательные + интервалы по каждой оси,
// температурные ярлыки, строка «при 293 K», строка пригодности по окнам.
export function renderLabels(res, ctx) {
    const { labels, results, phase } = ctx;
    const rows = AXIS_KEYS.map(key => {
        const v = res[key] || 0;
        const g = gradation(key, v);
        return `<div style="display:flex; gap:8px; font-size:1.15rem; line-height:1.75;">
            <span style="width:180px; color:#94a3b8;">${labels[key]}</span>
            <span style="color:#e0e0e0;"><b>${g.label}</b></span>
            <span style="color:#64748b;">[${g.lo}–${g.hi}]</span>
        </div>`;
    }).join('');
    const tLine = `<div style="display:flex; gap:8px; font-size:1.15rem; margin-top:6px; line-height:1.75;">
        <span style="width:180px; color:#94a3b8;">T_melt</span>
        <span style="color:#e0e0e0;"><b>${tMeltLabel(res.t_melt)}</b></span>
        <span style="color:#64748b;">[${k2c(res.t_melt)} °C]</span>
        <span style="width:180px; color:#94a3b8; margin-left:14px;">T_boil</span>
        <span style="color:#e0e0e0;"><b>${tBoilLabel(res.t_boil)}</b></span>
        <span style="color:#64748b;">[${k2c(res.t_boil)} °C]</span>
    </div>`;
    const phaseLine = `<div style="font-size:1.15rem; margin-top:6px; color:#94a3b8;">при 20 °C — <b style="color:#e0e0e0;">${phase}</b>${res.sublimating ? ' <span style="color:#f87171;">(сублимирует)</span>' : ''}</div>`;
    const suitLine = results && results.length
        ? `<div style="font-size:1.15rem; margin-top:6px;">Годится: ${results.map(r => `${r.name} ${r.ok ? '✓' : '✗'}`).join(', ')}</div>`
        : '';
    return `${rows}${tLine}${phaseLine}${suitLine}`;
}

// ==================== Формат 6: профиль-полоска ====================

// renderStrip — 1 строка = 10 сегментов (заливка = значение/100), чип фазы
// справа, T в тултипе; рамка строки по итогу окна (если окна включены).
export function renderStrip(res, ctx) {
    const { labels, color, windows, results, phase } = ctx;
    const segs = AXIS_KEYS.map(key => {
        const v = res[key] || 0;
        return `<div style="flex:1; height:22px; background:#1e293b; border-radius:3px; overflow:hidden;" title="${labels[key]}: ${v}">
            <div style="width:${v}%; height:100%; background:${color};"></div>
        </div>`;
    }).join('');
    const frame = windows && results && results.length
        ? (results.every(r => r.ok) ? '1px solid #22c55e' : '1px solid #ef4444')
        : '1px solid #2a2a4a';
    return `<div style="display:flex; align-items:center; gap:10px; padding:6px 10px; border:${frame}; border-radius:6px; background:#16162a;">
        <span style="width:220px; font-size:1.05rem; color:#e0e0e0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;" title="${res.name}">${res.name}</span>
        <div style="flex:1; display:flex; gap:3px;">${segs}</div>
        <span style="width:90px; text-align:right; font-size:1rem; color:#94a3b8;" title="T_melt ${k2c(res.t_melt)} °C · T_boil ${k2c(res.t_boil)} °C">${phase}</span>
    </div>`;
}

// stripLegend — легенда-подпись осей под полосками (одна на всех; короткие метки).
export function stripLegend(labels) {
    return `<div style="display:flex; gap:3px; margin-top:8px; font-size:1rem; color:#64748b;">
        ${AXIS_KEYS.map(k => `<div style="flex:1; text-align:center;">${shortLabel(k)}</div>`).join('')}
    </div>`;
}