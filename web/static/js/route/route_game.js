// web/static/js/route/route_game.js
// Точка входа страницы мини-игры «Прокладка маршрута» на доске v9 «Планшет»
// (спека 2026-09-25-маршрут-мини-игра-интерфейс.md §7.7): boot → offer → доска/
// HUD → ввод по клеткам → гейт «Проложить». Отправка (boost), разведка (scan) и
// результат — подэтап C2; здесь только доска, ввод, путь и гейт. Точный прогноз
// (ETA/секунды/q/bonus) до отправки нигде не показывается (решение 14);
// температура — только °C. Сеть: offer + /me.
import * as C from './route_config.js';
import { beaconsOnPath, stepHeat, rebuildPath } from './route_board.js';
import { chosenSprites, preloadSprites } from './route_sprites.js';
import { drawScene, initBackground, prepareBoard } from './route_render.js';
import { bindPointer } from './route_input.js';
import { activateSound, playSound } from '../ui/sound.js';
import { setShipOptions, recolorShipSprite, shipOrientFor } from '../map/ship_sprites.js';

const $ = (id) => document.getElementById(id);
const num = (v) => (typeof v === 'number' && isFinite(v) ? v : 0);

const state = {
    fingerprint: '', board: null, boardIndex: null, passport: null, mode: '',
    path: [], waypoints: [], dragging: false, drag: null,
    captured: new Set(), flash: new Map(), heat: [],
    revealed: [], pingsLeft: 0,
    remainingS: 0, remainingAt: 0, minBoostS: 0,
    submitting: false, finished: false,
    shipSprite: null, shipName: '', shipColor: null, shipOrient: { angle: 0, flip: false },
    bg: null, chosen: {}, seed: 0, reduced: false, view: null,
    message: '', cooldownTimer: null, tickTimer: null,
};

function prefersReduced() {
    try { return window.matchMedia('(prefers-reduced-motion: reduce)').matches; }
    catch (e) { return false; }
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

async function loadShip() {
    const res = await request('/me', { headers: { Authorization: 'Bearer ' + token() } });
    if (!res.ok || !res.data) return;
    setShipOptions(res.data.ship_options);
    state.shipName = res.data.ship_icon || '';
    state.shipColor = res.data.ship_color;
    state.shipOrient = shipOrientFor(state.shipName);
    state.shipSprite = recolorShipSprite(state.shipName, state.shipColor);
}

// ---- Логика доски/гейта (§4.4/§4.5) ----
function remaining(now) {
    return Math.max(0, state.remainingS - (now - state.remainingAt) / 1000);
}
function beaconsTotal() { return (state.board && state.board.beacons ? state.board.beacons.length : 0); }
function missingBeacons() { return Math.max(0, beaconsTotal() - state.captured.size); }

function gateReady(now) {
    const b = state.board;
    if (!b || state.submitting || state.finished) return false;
    const path = state.path;
    if (!path.length) return false;
    if (path[0] !== b.start) return false;
    if (path[path.length - 1] !== b.finish) return false;
    if (state.captured.size !== beaconsTotal()) return false;
    return remaining(now) >= state.minBoostS;
}

// statusText — первая невыполненная причина (§4.5).
function statusText(now) {
    const b = state.board;
    if (!b) return '';
    const path = state.path;
    if (!path.length) return 'Ведите путь от СТАРТА к звёзде-ФИНИШУ';
    if (path[0] !== b.start) return 'Путь должен начинаться от СТАРТА';
    const miss = missingBeacons();
    if (miss > 0) return 'Не хватает ' + miss + ' маяков';
    if (path[path.length - 1] !== b.finish) return 'Путь не доходит до ФИНИШа';
    if (remaining(now) < state.minBoostS) return 'Перелёт уже завершается — ускорить не получится';
    return 'Путь готов';
}

// refresh — пересчёт захваченных маяков, «жара» трассы, счётчиков, гейта, статуса.
function refresh() {
    if (!state.board) return;
    if (state.path.length) state.message = '';
    const onPath = beaconsOnPath(state.board, state.path);
    const now = performance.now();
    for (const c of onPath) {
        if (!state.captured.has(c)) { state.flash.set(c, now); playSound('ui_select'); }
    }
    state.captured = new Set(onPath);
    state.heat = stepHeat(state.board, state.path, state.revealed);
    updateHud();
}

// pingsDots — импульсы точками (осталось). Знаменателя offer не даёт.
function pingsDots() { return '●'.repeat(Math.max(0, state.pingsLeft)); }

function updateHud() {
    const now = performance.now();
    $('status-line').textContent = state.message || statusText(now);
    $('beacon-count').textContent = 'Маяки ' + state.captured.size + '/' + beaconsTotal();
    $('pings-count').textContent = 'Импульсы ' + (pingsDots() || '—');
    if (!state.submitting) {
        $('boost').textContent = 'Проложить';
        $('boost').disabled = !gateReady(now);
    }
}

// ---- HUD паспорта/режима/легенды ----
function buildPassport() {
    $('passport-chips').innerHTML = C.passportChips(state.passport, state.board);
}

function buildMode() {
    const el = $('mode-chip');
    if (!el) return;
    if (!state.mode) { el.style.display = 'none'; return; }
    el.textContent = 'Режим: ' + C.modeLabel(state.mode);
    el.style.display = '';
}

function buildLegend() {
    const el = $('legend');
    if (!el) return;
    el.innerHTML = C.LEGEND_ITEMS.map((it) =>
        '<span class="legend-item"><span class="legend-swatch" style="background:' + it.color +
        ';"></span>' + it.label + '</span>').join('');
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
    if (!state.board) return;
    if (!state.shipSprite && state.shipName) state.shipSprite = recolorShipSprite(state.shipName, state.shipColor);
    const canvas = $('route-canvas');
    const ctx = canvas.getContext('2d');
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const dpr = canvas.width / vw || 1;
    state.view = C.computeView(vw, vh, state.board.n);
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
    $('legend-toggle').onclick = () => {
        const el = $('legend');
        const open = el.style.display !== 'none';
        el.style.display = open ? 'none' : 'grid';
        $('legend-toggle').textContent = open ? 'Легенда ⏷' : 'Легенда ⏶';
    };
    $('reset').onclick = () => {
        if (state.submitting || state.finished) return;
        state.path = []; state.waypoints = []; state.drag = null; state.dragging = false;
        state.message = '';
        refresh();
    };
    $('undo').onclick = () => {
        if (state.submitting || state.finished || !state.waypoints.length) return;
        state.waypoints.pop(); state.dragging = false; state.drag = null;
        state.path = rebuildPath(state.board, state.waypoints);
        refresh();
    };
    // «Проложить» (отправка) — подэтап C2; в C1 кнопка только отражает гейт.
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
    buildLegend();
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
    const board = d.board || null;
    Object.assign(state, {
        fingerprint: d.fingerprint || '', board, passport: d.passport || null,
        mode: (board && board.mode) || '',
        seed: board ? num(C.hashSeed(d.fingerprint || '')) : 0,
        remainingS: num(d.remaining_s), remainingAt: performance.now(),
        minBoostS: num(d.min_remaining_boost_s),
        revealed: Array.isArray(d.revealed) ? d.revealed : [],
        pingsLeft: num(d.pings_left),
        path: [], waypoints: [], drag: null, dragging: false,
        captured: new Set(), flash: new Map(), heat: [],
        message: '', finished: false, submitting: false,
    });
    initBackground(state);
    prepareBoard(state);
    await preloadSprites(state.passport && state.passport.to && state.passport.to.star_type);
    state.chosen = chosenSprites();
    const canvas = $('route-canvas');
    resize(canvas);
    state.view = C.computeView(window.innerWidth, window.innerHeight, board ? board.n : 0);
    window.addEventListener('resize', () => resize(canvas));
    bindPointer(canvas, state, () => state.view, {
        onChange: refresh,
        onStartFail: () => { state.message = 'Начните путь от СТАРТА'; updateHud(); },
        isFrozen: () => state.submitting || state.finished,
    });
    hideLoading();
    buildMode();
    buildPassport();
    showHud();
    refresh();
    clearInterval(state.tickTimer);
    state.tickTimer = setInterval(() => { updateHud(); });
    requestAnimationFrame(frame);
}

bindUi();
boot();
