// web/static/js/route/route_game.js
// Точка входа страницы мини-игры «Прокладка маршрута» на доске v9 «Планшет»
// (спека 2026-09-25-маршрут-мини-игра-интерфейс.md §7.7): boot → offer →
// доска/HUD → ввод по клеткам → разведка импульсом (scan) → гейт «Проложить» →
// двухтапное подтверждение → boost → результат (в т. ч. отрицательный). Точный
// прогноз (ETA/секунды/q/bonus) до отправки нигде не показывается (решение 14);
// температура — только °C. DOM/оверлеи — в route_ui.js.
import * as C from './route_config.js';
import { beaconsOnPath, stepHeat, rebuildPath } from './route_board.js';
import { chosenSprites, preloadSprites } from './route_sprites.js';
import { drawScene, initBackground, prepareBoard } from './route_render.js';
import { bindPointer } from './route_input.js';
import * as UI from './route_ui.js';
import { initLegend, isLegendOpen } from './route_legend.js';
import { activateSound, playSound } from '../ui/sound.js';
import { setShipOptions, recolorShipSprite, shipOrientFor } from '../map/ship_sprites.js';

const $ = (id) => document.getElementById(id);
const num = (v) => (typeof v === 'number' && isFinite(v) ? v : 0);

const state = {
    fingerprint: '', board: null, boardIndex: null, passport: null, mode: '',
    path: [], waypoints: [], dragging: false, drag: null,
    dragStart: -1, dragMoved: false,
    captured: new Set(), flash: new Map(), heat: [],
    revealed: [], pingsLeft: 0, selectedSector: null, scanBusy: false,
    remainingS: 0, remainingAt: 0, minBoostS: 0,
    submitting: false, finished: false, confirming: false, confirmTimer: null, resultFx: null,
    shipSprite: null, shipName: '', shipColor: null, shipOrient: { angle: 0, flip: false },
    bg: null, chosen: {}, seed: 0, reduced: false, view: null,
    message: '', tickTimer: null,
};

function prefersReduced() {
    try { return window.matchMedia('(prefers-reduced-motion: reduce)').matches; }
    catch (e) { return false; }
}

// ---- Сеть ----
function token() { return localStorage.getItem('token') || sessionStorage.getItem('token'); }
const authHeaders = () => ({ Authorization: 'Bearer ' + token() });
const authJson = () => ({ Authorization: 'Bearer ' + token(), 'Content-Type': 'application/json' });

async function request(path, opts) {
    try {
        const res = await fetch(path, opts);
        const text = await res.text();
        let data = null;
        try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
        return { ok: res.ok, status: res.status, data, error: (data && data.error) || text || '' };
    } catch (e) { return { ok: false, status: 0, data: null, error: 'Не удалось связаться с сервером' }; }
}
const getOffer = () => request('/api/accelerator/offer', { headers: authHeaders() });

async function loadShip() {
    const res = await request('/me', { headers: authHeaders() });
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
    UI.renderSectorCard(state, sectorHandlers);
}

// pingsDots — импульсы точками (осталось). Знаменателя offer не даёт.
function pingsDots() { return '●'.repeat(Math.max(0, state.pingsLeft)); }

function updateHud() {
    const now = performance.now();
    $('status-line').textContent = state.message || statusText(now);
    $('beacon-count').textContent = 'Маяки ' + state.captured.size + '/' + beaconsTotal();
    $('pings-count').textContent = 'Импульсы ' + (pingsDots() || '—');
    const btn = $('boost');
    if (state.submitting) {
        // Отправка (§3/§7.3): «Прокладываю…» + блокировка кнопки; поле/HUD
        // заморожены через isFrozen().
        btn.textContent = 'Прокладываю…';
        btn.classList.remove('confirm');
        btn.disabled = true;
    } else {
        btn.textContent = state.confirming ? 'Проложить?' : 'Проложить';
        btn.classList.toggle('confirm', state.confirming);
        btn.disabled = !gateReady(now);
    }
}

// ---- Разведка/отправка (§4.3/§4.4/§4.7) ----

// handleFailure — единая развязка отказов (§8): оверлей-причина для гейтов
// доступности, тост — для прочих кодов/сети. Сырой текст ответа не показываем.
function handleFailure(res, fallback) {
    const d = res.data || {};
    const reason = d.reason || '';
    if (reason && C.REASON_OVERLAY[reason]) { UI.showReason(reason, d.cooldown_remaining_s); return; }
    UI.showToast((reason && C.REASON_TOAST[reason]) || fallback);
}

async function scan() {
    if (state.scanBusy || state.submitting || state.finished) return;
    const si = state.selectedSector;
    if (si == null) return;
    if ((state.revealed || []).some((r) => r.sector === si)) return; // уже вскрыт — импульс бережём
    if (state.pingsLeft <= 0) { UI.showToast(C.REASON_TOAST.no_pings); return; }
    state.scanBusy = true;
    UI.renderSectorCard(state, sectorHandlers);
    const res = await request('/api/accelerator/scan', {
        method: 'POST',
        headers: authJson(),
        body: JSON.stringify({ fingerprint: state.fingerprint, sector: si }),
    });
    state.scanBusy = false;
    if (res.status === 401) { window.location.href = '/login-page'; return; }
    if (res.ok && res.data) {
        if (Array.isArray(res.data.revealed)) state.revealed = res.data.revealed;
        if (typeof res.data.pings_left === 'number') state.pingsLeft = res.data.pings_left;
        playSound('ui_open');
        state.heat = stepHeat(state.board, state.path, state.revealed);
    } else {
        handleFailure(res, 'Не удалось разведать');
    }
    updateHud();
    UI.renderSectorCard(state, sectorHandlers);
}

// onBoostClick — двухтапное подтверждение (§4.4): «Проложить?» на 3 с, затем
// повторный тап отправляет; истечение — обратно «Проложить».
function onBoostClick() {
    if (state.submitting || state.finished) return;
    if (!gateReady(performance.now())) return;
    if (!state.confirming) {
        state.confirming = true;
        clearTimeout(state.confirmTimer);
        state.confirmTimer = setTimeout(() => { state.confirming = false; updateHud(); }, 3000);
        updateHud();
        return;
    }
    clearTimeout(state.confirmTimer);
    state.confirming = false;
    submit();
}

async function submit() {
    if (state.submitting || state.finished) return;
    state.submitting = true;
    updateHud();
    UI.renderSectorCard(state, sectorHandlers);
    const res = await request('/api/accelerator/boost', {
        method: 'POST',
        headers: authJson(),
        body: JSON.stringify({ fingerprint: state.fingerprint, path: state.path }),
    });
    if (res.status === 401) { window.location.href = '/login-page'; return; }
    state.submitting = false;
    if (res.ok && res.data) {
        state.finished = true;
        const bonus = num(res.data.bonus);
        state.resultFx = { at: performance.now(), bonus };
        playSound(bonus < 0 ? 'ui_error' : 'ui_success');
        updateHud();
        UI.showResult(bonus, num(res.data.remaining_s));
        return;
    }
    // Отказ после отправки — возврат в «готовое», путь сохраняем (§3).
    handleFailure(res, 'Не удалось проложить');
    updateHud();
    UI.renderSectorCard(state, sectorHandlers);
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
const sectorHandlers = {
    onScan: () => { scan(); },
    onClose: () => { state.selectedSector = null; UI.renderSectorCard(state, sectorHandlers); },
};

function selectSector(cell, si) {
    state.selectedSector = (si == null ? null : si);
    UI.renderSectorCard(state, sectorHandlers);
}

function bindUi() {
    $('passport-toggle').onclick = () => {
        const el = $('passport');
        el.classList.toggle('open');
        $('passport-toggle').textContent = el.classList.contains('open') ? 'Паспорт ⏶' : 'Паспорт ⏷';
    };
    $('to-map').onclick = UI.toMap;
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
    $('boost').onclick = onBoostClick;
    $('error-retry').onclick = () => { UI.hideError(); boot(); };
    $('error-map').onclick = UI.toMap;
    $('reason-map').onclick = UI.toMap;
    $('result-map').onclick = UI.toMap;
    initLegend();
}

// ---- Запуск ----
async function boot() {
    activateSound();
    state.reduced = prefersReduced();
    UI.hideError();
    UI.hideHud();
    UI.showLoading('Готовим маршрут…');
    loadShip();
    const res = await getOffer();
    if (res.status === 401) { window.location.href = '/login-page'; return; }
    if (res.status === 409) {
        UI.hideLoading();
        UI.showReason((res.data && res.data.reason) || 'no_flight', res.data && res.data.cooldown_remaining_s);
        return;
    }
    if (!res.ok) {
        UI.hideLoading();
        UI.showError(res.status === 0 || res.status >= 500
            ? 'Не удалось связаться с сервером'
            : (res.error || 'Не удалось загрузить маршрут'));
        return;
    }
    const d = res.data || {};
    const board = d.board || null;
    clearTimeout(state.confirmTimer);
    Object.assign(state, {
        fingerprint: d.fingerprint || '', board, passport: d.passport || null,
        mode: (board && board.mode) || '',
        seed: board ? num(C.hashSeed(d.fingerprint || '')) : 0,
        remainingS: num(d.remaining_s), remainingAt: performance.now(),
        minBoostS: num(d.min_remaining_boost_s),
        revealed: Array.isArray(d.revealed) ? d.revealed : [],
        pingsLeft: num(d.pings_left),
        path: [], waypoints: [], drag: null, dragging: false,
        dragStart: -1, dragMoved: false,
        captured: new Set(), flash: new Map(), heat: [],
        selectedSector: null, scanBusy: false,
        message: '', finished: false, submitting: false, confirming: false, resultFx: null,
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
        onSelectSector: selectSector,
        isFrozen: () => state.submitting || state.finished || state.scanBusy || isLegendOpen(),
    });
    UI.hideLoading();
    UI.buildMode(state);
    UI.buildPassport(state);
    UI.renderSectorCard(state, sectorHandlers);
    UI.showHud();
    refresh();
    clearInterval(state.tickTimer);
    state.tickTimer = setInterval(() => { updateHud(); });
    requestAnimationFrame(frame);
}

bindUi();
boot();
