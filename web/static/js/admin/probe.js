// web/static/js/admin/probe.js
// Проба смертности поселений: запуск, прогресс, три режима видения.
import { fetchWithAuth } from './auth.js';
import { showLoader } from '../loader.js';
import { notifyError } from '../ui/toast.js';
import { drawBars, drawLines } from './chart.js';

// Крутилки: ключ параметра → id input в разметке.
const DIALS = [
    ['p_base', 'pPBase'],
    ['k_base', 'pKBase'],
    ['alpha', 'pAlpha'],
    ['n_crit_mode', 'pNCritMode'],
    ['n_crit', 'pNCrit'],
    ['n_dead', 'pNDead'],
    ['t0', 'pT0'],
    ['hg_mode', 'pHgMode'],
    ['t_min', 'pTMin'],
    ['t_max', 'pTMax'],
    ['atmo_penalty', 'pAtmo0', 'pAtmo1', 'pAtmo2'],
    ['water_threshold', 'pWaterThr'],
    ['water_penalty', 'pWaterPen'],
];

const PALETTE = ['#60a5fa', '#f87171', '#34d399', '#fbbf24', '#a78bfa', '#22d3ee'];
const CLASS_COLORS = ['#60a5fa', '#34d399', '#f87171', '#a78bfa', '#fbbf24'];

let currentResultId = null;
let compareResultId = null;
let modeData = null;   // stats-ответ текущего результата
let currentMode = 'mass';
let liveState = null;  // {resultId, curves, horizon, cursor, speed, playing, timer}
let presetList = [];   // id → пресет из /admin/probe/presets

export function initProbe() {
    loadPresets();
    loadResults();
}

function presetName(id) {
    const p = presetList.find(x => x.id === id);
    return p ? p.name : id;
}

// ==================== ПРЕСЕТЫ И КРУТИЛКИ ====================

export async function loadPresets() {
    try {
        const res = await fetchWithAuth('/admin/probe/presets');
        presetList = await res.json();
        const sel = document.getElementById('probePreset');
        sel.innerHTML = '<option value="">— Пользовательский —</option>' +
            presetList.map(p => `<option value="${p.id}">${p.name}</option>`).join('');
        sel.addEventListener('change', () => applyPresetToDials(presetList));
    } catch (e) {
        notifyError('Не удалось загрузить пресеты: ' + e.message);
    }
}

function applyPresetToDials(presets) {
    const id = document.getElementById('probePreset').value;
    const p = presets.find(x => x.id === id);
    if (!p) return;
    const params = p.params;
    for (const row of DIALS) {
        const key = row[0];
        if (row[1] === 'pAtmo0') {
            const arr = params.atmo_penalty || [];
            document.getElementById('pAtmo0').value = arr[0] ?? '';
            document.getElementById('pAtmo1').value = arr[1] ?? '';
            document.getElementById('pAtmo2').value = arr[2] ?? '';
            continue;
        }
        const el = document.getElementById(row[1]);
        if (el && params[key] !== undefined) el.value = params[key];
    }
}

// ==================== ЗАПУСК ====================

export async function runProbe() {
    const body = {
        preset_id: document.getElementById('probePreset').value,
        overrides: collectOverrides(),
        seed: num('probeSeed'),
        sample_size: num('probeSample'),
        horizon_days: num('probeHorizon'),
    };
    const res = await fetchWithAuth('/admin/probe/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
    });
    if (res.status === 409) {
        notifyError('Прогон уже идёт');
        return;
    }
    if (!res.ok) {
        notifyError('Не удалось запустить прогон');
        return;
    }
    document.getElementById('probeProgress').style.display = 'block';
    pollProbe();
}

function collectOverrides() {
    const ov = {};
    for (const row of DIALS) {
        if (row[1] === 'pAtmo0') {
            const a = [num('pAtmo0'), num('pAtmo1'), num('pAtmo2')];
            if (a.some(v => v !== null)) ov.atmo_penalty = a;
            continue;
        }
        const el = document.getElementById(row[1]);
        if (!el) continue;
        const v = el.type === 'select-one' ? el.value : parseFloat(el.value);
        if (el.type === 'select-one' && !v) continue;
        if (el.type !== 'select-one' && !isFinite(v)) continue;
        ov[row[0]] = v;
    }
    return ov;
}

function num(id) {
    const el = document.getElementById(id);
    if (!el) return null;
    const v = parseFloat(el.value);
    return isFinite(v) ? v : null;
}

function pollProbe() {
    const interval = setInterval(async () => {
        try {
            const res = await fetchWithAuth('/admin/generate-status?job=probe_run');
            const data = await res.json();
            const progress = data.processed || 0;
            const total = data.total || 0;
            document.getElementById('probeProgressText').textContent =
                total > 0 ? `${progress} из ${total}` : 'Подготовка...';
            document.getElementById('probeProgressBar').value = total > 0 ? progress / total * 100 : 0;
            document.getElementById('probeCancelBtn').style.display =
                data.status === 'running' ? 'inline-block' : 'none';

            if (data.status === 'done') {
                clearInterval(interval);
                document.getElementById('probeProgress').style.display = 'none';
                document.getElementById('probeResult').textContent = '✅ Прогон завершён';
                loadResults();
            } else if (data.status === 'error' || data.status === 'canceled') {
                clearInterval(interval);
                document.getElementById('probeProgress').style.display = 'none';
                document.getElementById('probeResult').textContent =
                    (data.status === 'error' ? '❌ Ошибка: ' + (data.error || 'неизвестная') : '⏹️ Остановлено');
            }
        } catch (e) {
            clearInterval(interval);
            document.getElementById('probeProgress').style.display = 'none';
        }
    }, 1500);
}

export async function cancelProbe() {
    try {
        await fetchWithAuth('/admin/generate-cancel?job=probe_run', { method: 'POST' });
    } catch (e) { /* проигнорировано */ }
}

// ==================== РЕЗУЛЬТАТЫ ====================

export async function loadResults() {
    const container = document.getElementById('probeResultsList');
    try {
        const res = await fetchWithAuth('/admin/probe/results');
        const list = await res.json();
        if (!list.length) {
            container.innerHTML = '<p style="color:#94a3b8;">Прогонов ещё нет.</p>';
            return;
        }
container.innerHTML = '<table class="stats-table"><thead><tr>' +
            '<th>ID</th><th>Пресет</th><th>Создан</th><th>Поселений</th>' +
            '<th>Медиана, сут</th><th>Вымерли</th><th></th></tr></thead>' +
            '<tbody>' + list.map(it => `<tr>
                <td>${it.id}</td>
                <td>${presetName(it.preset_id)}</td>
                <td>${it.created_at}</td>
                <td>${it.total}</td>
                <td>${fmtNum(it.median_days)}</td>
                <td>${(it.extinct_fraction * 100).toFixed(1)}%</td>
                <td><button class="btn btn-small" onclick="showProbeResult('${it.id}')">Показать</button></td>
            </tr>`).join('') + '</tbody></table>';

        // Селект «Сравнить с» — все прогоны.
        const cmp = document.getElementById('probeCompare');
        if (cmp) {
            cmp.innerHTML = '<option value="">— нет —</option>' +
                list.map(it => `<option value="${it.id}">${presetName(it.preset_id)} (${fmtNum(it.median_days)} сут)</option>`).join('');
        }
    } catch (e) {
        container.innerHTML = `<p style="color:#f87171;">❌ ${e.message}</p>`;
    }
}

export async function showProbeResult(id) {
    stopLive();
    currentResultId = id;
    compareResultId = null;
    document.getElementById('probeCompare').value = '';
    renderResult(currentResultId);
}

export async function compareProbe(id) {
    compareResultId = id || null;
    renderMode();
}

async function renderResult(id) {
    const container = document.getElementById('probeView');
    const stop = showLoader(container, 'Загрузка результата...');
    try {
        const res = await fetchWithAuth(`/admin/probe/results/${id}?stats=1`);
        if (!res.ok) throw new Error('результат не найден');
        modeData = await res.json();
        stop();
        currentMode = 'mass';
        container.innerHTML = '';
        container.appendChild(buildModeBar());
        const view = document.createElement('div');
        view.id = 'probeModeView';
        container.appendChild(view);
        renderMode();
    } catch (e) {
        stop(`<p style="color:#f87171;">❌ Ошибка загрузки: ${e.message}</p>`);
    }
}

function buildModeBar() {
    const bar = document.createElement('div');
    bar.style.cssText = 'display:flex; gap:8px; margin-top:12px; flex-wrap:wrap; align-items:center;';
    bar.innerHTML = `
        <button class="btn mode-btn" data-mode="mass" onclick="setProbeMode('mass')">📊 Массовый</button>
        <button class="btn mode-btn" data-mode="group" onclick="setProbeMode('group')">🗂 Групповой</button>
        <button class="btn mode-btn" data-mode="live" onclick="setProbeMode('live')">▶ Живой</button>`;
    return bar;
}

export function setProbeMode(mode) {
    currentMode = mode;
    document.querySelectorAll('.mode-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.mode === mode));
    if (mode !== 'live' && liveState && liveState.timer) {
        clearInterval(liveState.timer);
        liveState.timer = null;
        liveState.playing = false;
    }
    renderMode();
}

async function renderMode() {
    const view = document.getElementById('probeModeView');
    if (!view || !modeData) return;
    if (currentMode === 'mass') {
        view.innerHTML = '';
        view.appendChild(buildScalars(modeData));
        view.appendChild(buildMassCharts(modeData));
    } else if (currentMode === 'group') {
        view.innerHTML = '';
        view.appendChild(buildGroupView(modeData));
    } else {
        await renderLive(view);
    }
}

// ==================== РЕЖИМ 1. МАССОВЫЙ ====================

function buildScalars(data) {
    const s = data.stats;
    const wrap = document.createElement('div');
    wrap.className = 'stat-grid';
    const cards = [
        ['Поселений', data.total],
        ['Медиана, сут', fmtNum(s.median_days)],
        ['Q25, сут', fmtNum(s.q25_days)],
        ['Q75, сут', fmtNum(s.q75_days)],
        ['Min, сут', fmtNum(s.min_days)],
        ['Max, сут', fmtNum(s.max_days)],
        ['Вымерли за прогон', (s.extinct_fraction * 100).toFixed(1) + '%'],
        ['Пресет', presetName(data.preset_id)],
    ];
    wrap.innerHTML = cards.map(c => `<div class="stat-card"><strong>${c[0]}:</strong> ${c[1]}</div>`).join('');
    return wrap;
}

function buildMassCharts(data) {
    const wrap = document.createElement('div');
    const hCanvas = document.createElement('canvas');
    wrap.appendChild(label('Гистограмма времени жизни'));
    wrap.appendChild(hCanvas);
    const sCanvas = document.createElement('canvas');
    wrap.appendChild(label('Кривая выживаемости'));
    wrap.appendChild(sCanvas);
    drawHistogramView(hCanvas, data);
    drawSurvivalView(sCanvas, data);
    return wrap;
}

function drawHistogramView(canvas, data) {
    const series = [{ color: PALETTE[0], bins: data.stats.histogram }];
    if (compareResultId) {
        fetch(`/admin/probe/results/${compareResultId}?stats=1`, {
            headers: { 'X-Admin-Password': localStorage.getItem('adminPassword') || '' },
        }).then(r => r.json()).then(cmp => {
            series.push({ color: PALETTE[1], bins: cmp.stats.histogram });
            drawBars(canvas, series);
        }).catch(() => drawBars(canvas, series));
        return;
    }
    drawBars(canvas, series);
}

function drawSurvivalView(canvas, data) {
    const series = [{ color: PALETTE[0], points: data.stats.survival.map(p => ({ x: p.t, y: p.alive })) }];
    if (compareResultId) {
        fetch(`/admin/probe/results/${compareResultId}?stats=1`, {
            headers: { 'X-Admin-Password': localStorage.getItem('adminPassword') || '' },
        }).then(r => r.json()).then(cmp => {
            series.push({ color: PALETTE[1], points: cmp.stats.survival.map(p => ({ x: p.t, y: p.alive })) });
            drawLines(canvas, series);
        }).catch(() => drawLines(canvas, series));
        return;
    }
    drawLines(canvas, series);
}

// ==================== РЕЖИМ 2. ГРУППОВОЙ ====================

function buildGroupView(data) {
    const wrap = document.createElement('div');
    const groups = data.stats.groups || {};
    const entries = Object.entries(groups).sort((a, b) => b[1].count - a[1].count);

    if (!entries.length) {
        wrap.innerHTML = '<p style="color:#94a3b8;">Групп нет.</p>';
        return wrap;
    }

    entries.forEach(([cls, g], i) => {
        wrap.appendChild(label(`${cls} — ${g.count} поселений`));
        const cards = document.createElement('div');
        cards.className = 'stat-grid';
        cards.innerHTML = [
            ['Медиана', fmtNum(g.median_days)],
            ['Q25', fmtNum(g.q25_days)],
            ['Q75', fmtNum(g.q75_days)],
            ['Min', fmtNum(g.min_days)],
            ['Max', fmtNum(g.max_days)],
            ['Вымерли', (g.extinct_fraction * 100).toFixed(1) + '%'],
        ].map(c => `<div class="stat-card"><strong>${c[0]}:</strong> ${c[1]}</div>`).join('');
        wrap.appendChild(cards);

        const canvas = document.createElement('canvas');
        canvas.style.marginTop = '4px';
        wrap.appendChild(canvas);
        drawBars(canvas, [{ color: CLASS_COLORS[i % CLASS_COLORS.length], bins: g.histogram }], { height: 180 });
    });
    return wrap;
}

// ==================== РЕЖИМ 3. ЖИВОЙ ====================

async function renderLive(view) {
    if (!liveState || liveState.resultId !== currentResultId) {
        try {
            const res = await fetchWithAuth(`/admin/probe/results/${currentResultId}?curves=3`);
            const data = await res.json();
            liveState = {
                resultId: currentResultId,
                curves: data.curves || [],
                horizon: data.horizon_days || 7300,
                cursor: 0,
                speed: 1,
                playing: false,
                timer: null,
            };
        } catch (e) {
            view.innerHTML = `<p style="color:#f87171;">❌ Ошибка загрузки кривых: ${e.message}</p>`;
            return;
        }
    }

    view.innerHTML = '';
    view.appendChild(buildLiveControls());
    view.appendChild(buildLiveCards());
    const canvas = document.createElement('canvas');
    canvas.style.marginTop = '12px';
    view.appendChild(canvas);
    view.__liveCanvas = canvas;
    drawLiveFrame();
}

function buildLiveControls() {
    const st = liveState;
    const wrap = document.createElement('div');
    wrap.style.cssText = 'display:flex; gap:8px; align-items:center; margin-top:12px; flex-wrap:wrap;';
    wrap.innerHTML = `
        <button class="btn btn-small" onclick="liveSeek(0)">⏮</button>
        <button class="btn btn-small" onclick="liveStep(-1)">◀</button>
        <button class="btn btn-small" id="livePlayBtn" onclick="liveToggle()">▶ Играть</button>
        <button class="btn btn-small" onclick="liveStep(1)">▶</button>
        <select id="liveSpeed" onchange="liveSpeed(this.value)" style="width:80px;">
            <option value="1">×1</option>
            <option value="10">×10</option>
            <option value="100">×100</option>
        </select>
        <input type="range" id="liveSlider" min="0" max="${Math.round(st.horizon)}" value="0"
            style="flex:1; min-width:160px;" oninput="liveSeek(parseFloat(this.value))">
        <span id="liveTime" style="color:#94a3b8; font-size:0.85rem; white-space:nowrap;">0 сут</span>`;
    return wrap;
}

function buildLiveCards() {
    const wrap = document.createElement('div');
    wrap.id = 'liveCards';
    wrap.style.cssText = 'display:flex; gap:10px; flex-wrap:wrap; margin-top:12px;';
    return wrap;
}

function liveP(curve, t) {
    if (t < curve.t0) return curve.p0;
    const tt = t - curve.t0;
    let p;
    if (curve.alpha > 0) {
        const frac = 1 - tt / curve.t;
        if (frac <= 0) p = 0;
        else p = curve.p0 * Math.pow(frac, 1 / curve.alpha);
    } else {
        p = curve.p0 * Math.exp(-curve.k * tt);
    }
    return p <= curve.n_dead ? 0 : p;
}

function drawLiveFrame() {
    const view = document.getElementById('probeModeView');
    if (!view) return;
    const canvas = view.__liveCanvas;
    const st = liveState;
    if (!canvas || !st) return;

    const series = st.curves.map((c, i) => ({
        color: PALETTE[i % PALETTE.length],
        points: liveCurvePoints(c, st.horizon),
    }));
    drawLines(canvas, series, { markerX: st.cursor });

    const timeEl = document.getElementById('liveTime');
    if (timeEl) timeEl.textContent = `${Math.round(st.cursor)} сут`;
    const slider = document.getElementById('liveSlider');
    if (slider) slider.value = Math.round(st.cursor);

    const cards = document.getElementById('liveCards');
    if (cards) {
        cards.innerHTML = st.curves.map((c, i) => {
            const alive = st.cursor < c.lifetime;
            const pop = liveP(c, st.cursor);
            return `<div class="stat-card" style="min-width:200px; border-left:3px solid ${PALETTE[i % PALETTE.length]};">
                <div style="font-weight:600;">${esc(c.planet_name)}</div>
                <div style="color:#94a3b8; font-size:0.8rem;">${esc(c.class)} · пригодность=${c.h_planet.toFixed(2)}</div>
                <div style="margin-top:6px;">Стартовое: ${fmtNum(c.p0)}</div>
                <div>время жизни: ${fmtNum(c.lifetime)} сут</div>
                <div>сейчас: <strong>${fmtNum(pop)}</strong> (${alive ? '<span style="color:#34d399;">живо</span>' : '<span style="color:#f87171;">вымерло</span>'})</div>
            </div>`;
        }).join('');
    }
}

function liveCurvePoints(c, horizon) {
    const points = [];
    const n = 200;
    for (let i = 0; i <= n; i++) {
        const t = horizon * i / n;
        points.push({ x: t, y: liveP(c, t) });
    }
    return points;
}

export function liveToggle() {
    const st = liveState;
    if (!st) return;
    st.playing = !st.playing;
    const btn = document.getElementById('livePlayBtn');
    if (btn) btn.textContent = st.playing ? '⏸ Пауза' : '▶ Играть';
    if (st.playing) {
        st.timer = setInterval(() => {
            st.cursor += st.speed;
            if (st.cursor >= st.horizon) { st.cursor = st.horizon; liveToggle(); return; }
            drawLiveFrame();
        }, 50);
    } else if (st.timer) {
        clearInterval(st.timer);
        st.timer = null;
    }
}

export function liveSpeed(v) {
    if (liveState) liveState.speed = parseFloat(v);
}

export function liveStep(dir) {
    const st = liveState;
    if (!st) return;
    st.cursor = Math.max(0, Math.min(st.horizon, st.cursor + st.speed * dir * 50));
    drawLiveFrame();
}

export function liveSeek(t) {
    const st = liveState;
    if (!st) return;
    st.cursor = Math.max(0, Math.min(st.horizon, t));
    drawLiveFrame();
}

function stopLive() {
    if (liveState) {
        if (liveState.timer) clearInterval(liveState.timer);
        liveState = null;
    }
}

// ==================== ПОМОЩНИКИ ====================

function label(text) {
    const el = document.createElement('div');
    el.style.cssText = 'color:#94a3b8; font-size:0.85rem; margin-top:16px;';
    el.textContent = text;
    return el;
}

function fmtNum(v) {
    if (!isFinite(v)) return '—';
    if (v >= 1000) return v.toLocaleString('ru-RU', { maximumFractionDigits: 1 });
    return v.toFixed(1);
}

function esc(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
}