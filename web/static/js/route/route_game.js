// web/static/js/route/route_game.js
// Точка входа страницы мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md): boot → offer → поле/HUD → ввод →
// «Проложить» → результат. Точный прогноз (ETA/секунды/q/bonus) до отправки
// нигде не показывается (решение 14); температура — только °C. Сеть: offer/boost.
import * as C from './route_config.js';
import { chosenSprites, preloadSprites } from './route_sprites.js';
import { drawScene, initBackground } from './route_render.js';
import { bindPointer } from './route_input.js';
import { activateSound, playSound } from '../ui/sound.js';
import { setShipOptions, recolorShipSprite, shipOrientFor } from '../map/ship_sprites.js';

const $ = (id) => document.getElementById(id);
const num = (v) => (typeof v === 'number' && isFinite(v) ? v : 0);
const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g,
    (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
let toastTimer = null;

const state = {
    fingerprint: '', field: null, passport: null,
    path: [], drag: null, dragging: false,
    captured: new Set(), fixedCaptured: new Set(), falseHit: new Set(), flash: new Map(),
    remainingS: 0, remainingAt: 0, minBoostS: 0,
    submitting: false, finished: false, boost: null,
    shipSprite: null, shipName: '', shipColor: null, shipOrient: { angle: 0, flip: false },
    bg: null, chosen: {}, seed: 0, reduced: false, view: null,
    zoneHit: new Set(), cooldownTimer: null, tickTimer: null, message: '', lastLen: 0,
};

function prefersReduced() {
    try { return window.matchMedia('(prefers-reduced-motion: reduce)').matches; }
    catch (e) { return false; }
}

function notify(msg) {
    const el = $('toast');
    if (!el) return;
    el.textContent = msg;
    el.style.display = 'block';
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.style.display = 'none'; }, 3000);
}

// ---- Переходы/оверлеи ----
const toMap = () => { window.location.href = '/map'; };
const showLoading = (t) => { $('loading-text').textContent = t || 'Загрузка…'; $('loading').style.display = 'flex'; };
const hideLoading = () => { $('loading').style.display = 'none'; };
const showHud = () => { $('hud').style.display = 'flex'; };
const hideHud = () => { $('hud').style.display = 'none'; };
const showError = (m) => { $('error-text').textContent = m; $('error').style.display = 'flex'; };
const hideError = () => { $('error').style.display = 'none'; };

function showReason(reason, cooldownS) {
    const info = C.REASON_OVERLAY[reason] || { title: 'Мини-игра недоступна', note: '' };
    $('reason-title').textContent = info.title;
    $('reason-note').textContent = info.note || '';
    const timer = $('reason-timer');
    clearInterval(state.cooldownTimer);
    if (reason === 'cooldown' && cooldownS != null) {
        const until = Date.now() + num(cooldownS) * 1000;
        const tick = () => {
            const left = Math.max(0, Math.round((until - Date.now()) / 1000));
            timer.textContent = 'Откат ' + Math.floor(left / 60) + ':' + String(left % 60).padStart(2, '0');
            if (left <= 0) clearInterval(state.cooldownTimer);
        };
        timer.style.display = ''; tick();
        state.cooldownTimer = setInterval(tick, 1000);
    } else {
        timer.style.display = 'none';
    }
    $('reason').style.display = 'flex';
}

function showResult(d) {
    $('result-class').textContent = C.qualityClassWord(d.quality);
    $('result-bonus').textContent = 'Скорость перелёта +' + Math.round(num(d.bonus) * 100) + ' %';
    $('result-remaining').textContent = 'Осталось ~' + C.remainWord(num(d.remaining_s));
    $('result').style.display = 'flex';
}

// ---- Сеть ----
function token() { return localStorage.getItem('token') || sessionStorage.getItem('token'); }
async function request(path, opts) {
    try {
        const res = await fetch(path, opts);
        const text = await res.text();
        let data = null;
        try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
        return { ok: res.ok, status: res.status, data, error: (data && data.error) || text || '' };
    } catch (e) { return { ok: false, status: 0, data: null, error: 'Не удалось связаться с сервером' }; }
}
const getOffer = () => request('/api/accelerator/offer', { headers: { Authorization: 'Bearer ' + token() } });
const postBoost = (body) => request('/api/accelerator/boost', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token() },
    body: JSON.stringify(body),
});

async function loadShip() {
    const res = await request('/me', { headers: { Authorization: 'Bearer ' + token() } });
    if (!res.ok || !res.data) return;
    setShipOptions(res.data.ship_options);
    state.shipName = res.data.ship_icon || '';
    state.shipColor = res.data.ship_color;
    state.shipOrient = shipOrientFor(state.shipName);
    state.shipSprite = recolorShipSprite(state.shipName, state.shipColor);
}

// ---- Захват узлов/зон ----
function pathWithFree() {
    const pts = state.path.slice();
    if (state.dragging && state.drag) pts.push(state.drag);
    return pts;
}

function refresh() {
    if (!state.field) return;
    if (state.path.length) state.message = '';
    const pts = pathWithFree();
    const display = C.nodeSet(state.field, pts, 'beacon');
    for (const i of display) {
        if (!state.captured.has(i)) { state.flash.set(i, performance.now()); playSound('ui_select'); }
    }
    state.captured = display;
    state.fixedCaptured = C.nodeSet(state.field, state.path, 'beacon');
    const fset = C.nodeSet(state.field, pts, 'false_signal');
    for (const i of fset) {
        if (!state.falseHit.has(i)) { state.falseHit.add(i); notify('Это ложный сигнал'); }
    }
    const zset = C.zoneSet(state.field, pts);
    for (const i of zset) if (!state.zoneHit.has(i)) playSound('ui_error');
    state.zoneHit = zset;
    if (state.path.length > state.lastLen) playSound('ui_open');
    state.lastLen = state.path.length;
    updateHud();
}

function updateHud() {
    const now = performance.now();
    $('status-line').textContent = state.message || C.statusText(state, now);
    $('beacon-count').textContent = 'Маяки ' + state.fixedCaptured.size + '/' + C.beaconsTotal(state.field);
    if (!state.submitting) {
        $('boost').textContent = 'Проложить';
        $('boost').disabled = !C.gateReady(state, now);
    }
}

// ---- HUD паспорта ----
const chip = (t) => '<span class="hud-chip">' + esc(t) + '</span>';
function buildPassport() {
    const p = state.passport;
    const nodes = state.field.nodes || [];
    const falseN = nodes.filter((n) => n.type === 'false_signal').length;
    const out = [];
    if (p && p.from && p.to) {
        out.push(chip('🛰 ' + C.distanceWord(p.dist)));
        out.push(chip(C.starClassWord(p.from) + ' → ' + C.starClassWord(p.to) + ' · ' + C.systemWord(p.to)));
        out.push(chip(C.tempWord(p.from.temperature) + ' → ' + C.tempWord(p.to.temperature)));
    }
    out.push(chip('Маяки ' + C.beaconsTotal(state.field) + ' · Ложные ' + falseN + ' · Зоны ' + (state.field.zones || []).length));
    if (p && p.destination_belts && p.destination_belts.length) out.push(chip('Пояс: ' + C.beltWord(p.destination_belts[0])));
    $('passport-chips').innerHTML = out.join('');
}

// ---- Отправка ----
// toastForRefusal — человеческий текст отказа boost. Для refusal-тела (есть
// reason) сырой текст ответа не показываем; для cooldown добавляем таймер mm:ss.
function cooldownClock(s) {
    const left = Math.max(0, Math.round(num(s)));
    return Math.floor(left / 60) + ':' + String(left % 60).padStart(2, '0');
}
function toastForRefusal(reason, data, fallback) {
    if (!reason) return fallback || 'Не удалось ускорить';
    const base = C.REASON_TOAST[reason] || 'Не удалось ускорить';
    if (reason === 'cooldown' && data && data.cooldown_remaining_s != null) {
        return base + ' · Откат ' + cooldownClock(data.cooldown_remaining_s);
    }
    return base;
}
function setBusy(busy) {
    state.submitting = busy;
    $('boost').textContent = busy ? 'Прокладываю…' : 'Проложить';
    $('boost').disabled = busy || !C.gateReady(state, performance.now());
    $('undo').disabled = busy || state.finished;
    $('reset').disabled = busy || state.finished;
}
async function submit() {
    if (!C.gateReady(state, performance.now())) return;
    setBusy(true);
    const res = await postBoost({ fingerprint: state.fingerprint, path: state.path });
    if (res.status === 401) { window.location.href = '/login-page'; return; }
    if (res.ok) {
        playSound('ui_success');
        state.finished = true;
        state.boost = { t0: performance.now(), dur: 1000 };
        setBusy(false);
        showResult(res.data || {});
        return;
    }
    setBusy(false);
    const reason = res.data && res.data.reason;
    if (reason === 'already_active' || reason === 'no_flight') { showReason(reason, null); return; }
    playSound('ui_error');
    notify(toastForRefusal(reason, res.data, res.error));
    refresh();
}

// ---- Кадр/окно ----
function resize(canvas) {
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    canvas.width = Math.round(window.innerWidth * dpr);
    canvas.height = Math.round(window.innerHeight * dpr);
    canvas.style.width = window.innerWidth + 'px';
    canvas.style.height = window.innerHeight + 'px';
}
function frame(now) {
    if (!state.field) return;
    if (!state.shipSprite && state.shipName) state.shipSprite = recolorShipSprite(state.shipName, state.shipColor);
    const canvas = $('route-canvas');
    const ctx = canvas.getContext('2d');
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const dpr = canvas.width / vw || 1;
    state.view = C.computeView(vw, vh);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    drawScene(ctx, state, state.view, vw, vh, now);
    requestAnimationFrame(frame);
}

// ---- UI-события ----
function bindUi() {
    $('passport-toggle').onclick = () => {
        const el = $('passport');
        el.classList.toggle('open');
        $('passport-toggle').textContent = el.classList.contains('open') ? 'Паспорт ⏶' : 'Паспорт ⏷';
    };
    $('to-map').onclick = toMap;
    $('reset').onclick = () => {
        if (state.submitting || state.finished) return;
        state.path = []; state.drag = null; state.dragging = false;
        state.falseHit.clear(); state.zoneHit.clear(); state.message = '';
        refresh();
    };
    $('undo').onclick = () => {
        if (state.submitting || state.finished || !state.path.length) return;
        state.path.pop(); state.dragging = false; state.drag = null;
        refresh();
    };
    $('boost').onclick = () => { if (!state.submitting) submit(); };
    $('error-retry').onclick = () => { hideError(); boot(); };
    $('error-map').onclick = toMap;
    $('reason-map').onclick = toMap;
    $('result-map').onclick = toMap;
}

// ---- Запуск ----
async function boot() {
    activateSound();
    state.reduced = prefersReduced();
    hideError();
    hideHud();
    showLoading('Готовим маршрут…');
    loadShip();
    const res = await getOffer();
    if (res.status === 401) { window.location.href = '/login-page'; return; }
    if (res.status === 409) {
        hideLoading();
        showReason((res.data && res.data.reason) || 'no_flight', res.data && res.data.cooldown_remaining_s);
        return;
    }
    if (!res.ok) {
        hideLoading();
        showError(res.status === 0 || res.status >= 500
            ? 'Не удалось связаться с сервером'
            : (res.error || 'Не удалось загрузить маршрут'));
        return;
    }
    const d = res.data || {};
    Object.assign(state, {
        fingerprint: d.fingerprint || '', passport: d.passport || null, field: d.field || null,
        seed: d.field ? num(d.field.seed) : 0, remainingS: num(d.remaining_s), remainingAt: performance.now(),
        minBoostS: num(d.min_remaining_boost_s), path: [], drag: null, dragging: false,
        captured: new Set(), fixedCaptured: new Set(), falseHit: new Set(), zoneHit: new Set(),
        flash: new Map(), boost: null, finished: false, submitting: false,
    });
    initBackground(state);
    await preloadSprites(state.passport && state.passport.to && state.passport.to.star_type);
    state.chosen = chosenSprites();
    const canvas = $('route-canvas');
    resize(canvas);
    window.addEventListener('resize', () => resize(canvas));
    bindPointer(canvas, state, () => state.view || C.computeView(window.innerWidth, window.innerHeight), {
        onChange: refresh,
        onStartFail: () => { state.message = 'Начните путь от СТАРТА'; updateHud(); },
        onTooMany: () => notify(C.REASON_TOAST.too_many_points),
        isFrozen: () => state.submitting || state.finished,
    });
    hideLoading();
    buildPassport();
    showHud();
    refresh();
    clearInterval(state.tickTimer);
    state.tickTimer = setInterval(refresh, 500);
    requestAnimationFrame(frame);
}

bindUi();
boot();
