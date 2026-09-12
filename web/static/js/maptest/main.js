// web/static/js/maptest/main.js
// Maptest — инструмент: галактика + playback кривых смертности поселений.

const CLASS_COLORS = {
    'землеподобные': '#34d399',
    'ледяные': '#60a5fa',
    'вулканические': '#f87171',
    'газовые': '#a78bfa',
    'прочие': '#fbbf24',
};
const DEAD_COLOR = '#1e293b';
const SPECTRAL = { O: '#9db4ff', B: '#aac4ff', A: '#cad8ff', F: '#f8f7ff', G: '#fff4e8', K: '#ffd2a1', M: '#ffcc6f' };

let rows = [];
let horizon = 7300;
let cursor = 0;
let speed = 1;
let playing = false;
let timer = null;
let scale = 1, ox = 0, oy = 0;
let dragging = false, lastX = 0, lastY = 0;
let presetNames = {};

let canvas = null;
let ctx = null;

function authHeaders() {
    return { 'X-Admin-Password': localStorage.getItem('adminPassword') || '' };
}

export async function initMaptest() {
    canvas = document.getElementById('mtCanvas');
    ctx = canvas.getContext('2d');
    window.mtLoadResult = mtLoadResult;
    window.mtReload = mtReload;
    window.mtFitAll = mtFitAll;
    window.mtRedraw = mtRedraw;
    window.mtToggle = mtToggle;
    window.mtSpeed = mtSpeed;
    window.mtStep = mtStep;
    window.mtSeek = mtSeek;
    if (!localStorage.getItem('adminPassword')) {
        document.getElementById('mtCount').textContent = '⚠ Задай пароль в админке (вкладка «Основное»).';
        return;
    }
    bindCanvas();
    window.addEventListener('resize', () => { fitAll(); redraw(); });
    await loadResults();
}

export function mtRedraw() { redraw(); }

async function loadResults() {
    try {
        const pres = await fetch('/admin/probe/presets', { headers: authHeaders() });
        if (pres.ok) {
            const plist = await pres.json();
            presetNames = Object.fromEntries(plist.map(p => [p.id, p.name]));
        }
        const res = await fetch('/admin/probe/results', { headers: authHeaders() });
        if (res.status === 401) {
            document.getElementById('mtCount').textContent = '⚠ Неверный пароль.';
            return;
        }
        const list = await res.json();
        const sel = document.getElementById('mtResult');
        sel.innerHTML = '<option value="">— выбери прогон —</option>' + list.map(it =>
            `<option value="${it.id}">${presetNames[it.preset_id] || it.preset_id} · медиана ${fmt(it.median_days)} сут</option>`).join('');
        if (list.length) {
            sel.value = list[0].id;
            await mtLoadResult();
        } else {
            document.getElementById('mtCount').textContent = 'Прогонов нет.';
        }
    } catch (e) {
        document.getElementById('mtCount').textContent = '❌ ' + e.message;
    }
}

export async function mtLoadResult() {
    const id = document.getElementById('mtResult').value;
    if (!id) return;
    stop();
    document.getElementById('mtCount').textContent = 'Загрузка...';
    try {
        const res = await fetch(`/admin/probe/playback?file=${encodeURIComponent(id)}`, { headers: authHeaders() });
        if (!res.ok) {
            document.getElementById('mtCount').textContent = '❌ Ошибка загрузки: ' + res.status;
            return;
        }
        const data = await res.json();
        rows = data.rows || [];
        horizon = data.horizon_days || 7300;
        document.getElementById('mtSlider').max = Math.round(horizon);
        document.getElementById('mtSlider').value = 0;
        cursor = 0;
        fitAll();
        document.getElementById('mtCount').textContent = `${rows.length} миров`;
        redraw();
    } catch (e) {
        document.getElementById('mtCount').textContent = '❌ ' + e.message;
    }
}

export function mtReload() { mtLoadResult(); }

export function mtFitAll() {
    fitAll();
    redraw();
}

function fitAll() {
    if (!rows.length) return;
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    for (const w of rows) {
        if (w.x < minX) minX = w.x;
        if (w.x > maxX) maxX = w.x;
        if (w.y < minY) minY = w.y;
        if (w.y > maxY) maxY = w.y;
    }
    const cw = canvas.clientWidth || 800;
    const ch = canvas.clientHeight || 500;
    const spanX = maxX - minX || 1;
    const spanY = maxY - minY || 1;
    scale = Math.min(cw / spanX, ch / spanY) * 0.95;
    ox = cw / 2 - (minX + maxX) / 2 * scale;
    oy = ch / 2 - (minY + maxY) / 2 * scale;
}

// ==================== RENDER ====================

function redraw() {
    if (!ctx || !canvas) return;
    const cw = canvas.clientWidth || 800;
    const ch = canvas.clientHeight || 500;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = cw * dpr;
    canvas.height = ch * dpr;
    canvas.style.width = cw + 'px';
    canvas.style.height = ch + 'px';
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = '#0a0f1e';
    ctx.fillRect(0, 0, cw, ch);

    const habitableOnly = document.getElementById('mtHabitable').checked;
    const classColor = document.getElementById('mtClassColor').checked;

    ctx.fillStyle = DEAD_COLOR;
    for (const w of rows) {
        const px = w.x * scale + ox;
        const py = w.y * scale + oy;
        if (px < -4 || py < -4 || px > cw + 4 || py > ch + 4) continue;
        const alive = cursor < w.lifetime;
        if (habitableOnly && !alive) continue;
        ctx.fillStyle = alive
            ? (classColor ? (CLASS_COLORS[w.class] || '#fbbf24') : (SPECTRAL[w.spectral] || '#fff4e8'))
            : DEAD_COLOR;
        ctx.fillRect(px - 1, py - 1, 3, 3);
    }

    document.getElementById('mtTime').textContent = Math.round(cursor) + ' сут';
    document.getElementById('mtSlider').value = Math.round(cursor);
}

// ==================== PLAYBACK ====================

export function mtToggle() {
    playing = !playing;
    document.getElementById('mtPlayBtn').textContent = playing ? '⏸ Пауза' : '▶ Играть';
    if (playing) {
        timer = setInterval(() => {
            cursor += speed;
            if (cursor >= horizon) { cursor = horizon; mtToggle(); return; }
            redraw();
        }, 50);
    } else if (timer) {
        clearInterval(timer);
        timer = null;
    }
}

export function mtSpeed(v) {
    speed = parseFloat(v);
}

export function mtStep(dir) {
    cursor = Math.max(0, Math.min(horizon, cursor + speed * dir * 50));
    redraw();
}

export function mtSeek(t) {
    cursor = Math.max(0, Math.min(horizon, t));
    redraw();
}

function stop() {
    playing = false;
    if (timer) { clearInterval(timer); timer = null; }
    const btn = document.getElementById('mtPlayBtn');
    if (btn) btn.textContent = '▶ Играть';
}

// ==================== PAN / ZOOM ====================

function bindCanvas() {
    canvas.addEventListener('wheel', e => {
        e.preventDefault();
        const factor = e.deltaY < 0 ? 1.2 : 1 / 1.2;
        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;
        ox = mx - (mx - ox) * factor;
        oy = my - (my - oy) * factor;
        scale *= factor;
        redraw();
    }, { passive: false });

    canvas.addEventListener('mousedown', e => {
        dragging = true;
        lastX = e.clientX;
        lastY = e.clientY;
        canvas.classList.add('dragging');
    });
    window.addEventListener('mousemove', e => {
        if (!dragging) return;
        ox += e.clientX - lastX;
        oy += e.clientY - lastY;
        lastX = e.clientX;
        lastY = e.clientY;
        redraw();
    });
    window.addEventListener('mouseup', () => {
        dragging = false;
        canvas.classList.remove('dragging');
    });
}

function fmt(v) {
    if (!isFinite(v)) return '—';
    if (v >= 1000) return v.toLocaleString('ru-RU', { maximumFractionDigits: 0 });
    return v.toFixed(1);
}