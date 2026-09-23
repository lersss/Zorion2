// web/static/js/belt/belt_main.js
// Точка входа страницы добычи в поясе (UI-спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча-ui §4–§6): заход (enter) → сцена
// (Canvas 2D, вид сверху, инерционный полёт, бурение) → «Вернуться» (leave).
// Вход: /belt.html?belt=<uuid> (кнопка «⛏ Добывать» в строке пояса).
import * as C from './belt_config.js';
import { BeltWorld } from './belt_world.js';
import { drawScene } from './belt_render.js';
import { preloadSprites } from './belt_render.js';
import { enter, collect, leave } from './belt_net.js';
import { bindInput, drillHeld } from './belt_input.js';
import * as ui from './belt_ui.js';
import { activateSound, playSound } from '../ui/sound.js';
import { setShipOptions, recolorShipSprite, shipOrientFor } from '../map/ship_sprites.js';
import { cargoNum } from '../dashboard/cargo.js';

const state = {
    pkg: null,
    world: null,
    camera: { x: 0, y: 0 },
    input: { forward: false, back: false, left: false, right: false, brake: false, space: false, mouseDown: false, aim: null, aimWorld: null },
    running: false,
    leaving: false,
    offline: false,
    full: false,
    depleted: false,
    mined: 0,
    cargo: { used: 0, total: 0, free: 0 },
    rateCap: 0,
    remainingLevel: '',
    beltClass: '',
    drilling: false,
    lastTarget: null,
    pending: 0,
    lastTime: 0,
    blockedUntil: 0,
    shipSprite: null,
    shipSpriteName: '',
    shipColor: null,
    worldId: '',
    shipOrient: { angle: 0, flip: false },
    collectTimer: null,
    toastFullShown: false,
};

const num = (v) => (typeof v === 'number' && isFinite(v) ? v : 0);

function resize(canvas) {
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(window.innerWidth * dpr);
    canvas.height = Math.round(window.innerHeight * dpr);
    canvas.style.width = window.innerWidth + 'px';
    canvas.style.height = window.innerHeight + 'px';
}

// ---- Спрайт корабля игрока (реестр /me, спека 2026-09-21 §6.2) ----
async function loadShipSprite() {
    const t = localStorage.getItem('token') || sessionStorage.getItem('token');
    if (!t) return;
    try {
        const res = await fetch('/me', { headers: { 'Authorization': 'Bearer ' + t } });
        if (!res.ok) return;
        const me = await res.json();
        setShipOptions(me.ship_options);
        state.shipSpriteName = me.ship_icon || '';
        state.shipColor = me.ship_color;
        // Система игрока — для метки «вернуться к системе» при выходе из пояса
        // (решение создателя 2026-09-23): игрок остаётся на орбите пояса этой
        // системы, карта по метке откроет её попап.
        state.worldId = me.current_world_id || '';
        state.shipOrient = shipOrientFor(state.shipSpriteName);
        state.shipSprite = recolorShipSprite(state.shipSpriteName, state.shipColor);
    } catch (e) { /* фолбэк-треугольник */ }
}

// ---- HUD ----
// isCargoFull — «трюм полон» по согласованному условию: груз + буфер ≥ total.
// Совпадает с серверным full = «свободно − буфер ≤ 0» (осн. §5.3.2/§7), поэтому
// бейдж «Полон» и блокировка сбора совпадают с заполненной шкалой трюма.
function isCargoFull() {
    const total = num(state.cargo.total);
    return total > 0 && (num(state.cargo.used) + num(state.mined)) >= total - 1e-9;
}

function refreshHUD() {
    ui.updateHUD({
        mined: state.mined,
        used: num(state.cargo.used),
        total: num(state.cargo.total),
        remainingLevel: state.remainingLevel,
        beltClass: state.beltClass,
    });
}

function hintHtml(hit, inRange, offBelt) {
    if (state.full) {
        return 'Трюм полон — добыча недоступна. '
            + '<button id="hint-leave" type="button" style="margin-left:8px; background:#2a2a4a; border:none; color:#fde68a; padding:3px 10px; border-radius:4px; cursor:pointer; font-size:0.8rem;">Вернуться</button>';
    }
    if (state.depleted) return 'Пояс выработан — добывать больше нечего.';
    if (state.offline) return 'Нет связи — добыча приостановлена';
    if (state.drilling) return '⛏ Добыча идёт…';
    if (inRange) return 'ЛКМ или [Space] — добыча';
    if (hit) return 'Подлетите ближе';
    if (offBelt) return 'Пояс позади — развернитесь к полосе';
    return 'Наведите нос на жилу';
}

// ---- Кадр ----
function frame(now) {
    if (!state.running) return;
    const canvas = document.getElementById('belt-canvas');
    const ctx = canvas.getContext('2d');
    const dt = Math.min(0.05, (now - state.lastTime) / 1000 || 0);
    state.lastTime = now;
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const dpr = vw > 0 ? canvas.width / vw : 1;

    const paused = ui.isPaused();
    // Курсор (экранные координаты) → мировая точка прицела: нос доворачивается
    // к ней (§5.1). Камера сглажена, берём её текущее положение.
    const aim = state.input.aim;
    if (aim) {
        state.input.aimWorld = {
            x: state.camera.x + (aim.x - vw / 2) / C.WORLD_SCALE,
            y: state.camera.y + (aim.y - vh / 2) / C.WORLD_SCALE,
        };
    }
    let hit = null;
    let inRange = false;
    let target = null;
    if (!paused && !state.leaving) {
        state.world.update(dt, state.input);
        // Цель бурения — жила под лучом носа (не «ближайшая»); удержание ЛКМ/Space
        // бурит только наведённую цель, потеря наведения прекращает бурение (§5.2).
        hit = state.world.rayVein();
        inRange = !!hit && state.world.distToSurface(hit) <= C.EXTRACT_RADIUS;
        target = inRange ? hit : null;
        const canDrill = !!target && drillHeld(state.input)
            && !state.full && !state.depleted && now >= state.blockedUntil;
        if (state.world.collided) state.blockedUntil = now + C.COLLISION_BLOCK_MS;
        state.drilling = canDrill;
        if (canDrill) {
            state.pending += state.rateCap * dt;
            state.world.drill(target, dt);
            state.lastTarget = target;
        }
    } else {
        state.drilling = false;
    }
    const offBelt = Math.abs(state.world.ship.y) > C.BELT_HALF_WIDTH;

    state.camera.x += (state.world.ship.x - state.camera.x) * C.CAMERA_LERP;
    state.camera.y += (state.world.ship.y - state.camera.y) * C.CAMERA_LERP;

    if (!state.shipSprite && state.shipSpriteName) {
        state.shipSprite = recolorShipSprite(state.shipSpriteName, state.shipColor);
    }

    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    drawScene(ctx, state.world, state.camera, vw, vh, {
        drilling: state.drilling,
        target,
        depleted: state.depleted,
        shipSprite: state.shipSprite,
        shipOrient: state.shipOrient,
        offBelt,
        now,
        // rim-light: постоянное направление (сторона звезды), в экранном
        // пространстве — не вращается вместе с телом (арт-ТЗ §4.3).
        lightX: -0.55,
        lightY: -0.55,
    });

    refreshHUD();
    ui.setHint(hintHtml(hit, inRange, offBelt));
    ui.updateControls(state.input);
    requestAnimationFrame(frame);
}

// ---- Сбор (сервер — касса, §5.3) ----
function setOffline(v) {
    if (state.offline === v) return;
    state.offline = v;
    if (v) ui.showPill();
    else ui.hidePill();
}

async function collectTick() {
    if (!state.running || state.leaving || ui.isPaused()) return;
    if (state.full || state.depleted) return;
    const amount = state.pending;
    if (amount <= 0) return;
    state.pending = 0;
    const res = await collect(amount);
    if (res.status === 401) { window.location.href = '/login-page'; return; }
    if (!res.ok) {
        // Сеть/5xx — ненавязчивый пилл; буфер не растёт (сервер — касса, §5.4).
        state.pending += amount;
        setOffline(true);
        return;
    }
    setOffline(false);
    const d = res.data || {};
    state.mined = num(d.mined);
    state.remainingLevel = d.remaining_level || state.remainingLevel;
    state.cargo.free = num(d.cargo_free);
    state.cargo.used = Math.max(0, num(state.cargo.total) - state.cargo.free);
    state.full = !!d.full || isCargoFull();
    if (state.remainingLevel === 'выработан' && !state.depleted) {
        state.depleted = true;
        ui.notify('Пояс выработан');
    }
    if (state.full && !state.toastFullShown) {
        state.toastFullShown = true;
        ui.notify('Трюм полон — выгрузите груз на корабле');
    }
    if (num(d.granted) > 0) {
        const at = state.lastTarget || state.world.ship;
        state.world.addFloater('+' + cargoNum(num(d.granted)) + ' т', at.x, at.y - 24);
    }
}

// ---- Выход ----
// returnToMap — уход на карту с меткой «вернуться к системе пояса» (решение
// создателя 2026-09-23): карта при загрузке откроет попап системы игрока (он
// остался на орбите пояса). Метку ставим, только если знаем систему; обычный
// заход на карту (без метки) попап не открывает. Редиректы на /login-page (401)
// идут мимо — метку не ставят.
async function returnToMap() {
    let worldId = state.worldId;
    if (!worldId) {
        // loadShipSprite ещё не успел (быстрый выход / экран ошибки) — берём /me.
        try {
            const t = localStorage.getItem('token') || sessionStorage.getItem('token');
            if (t) {
                const res = await fetch('/me', { headers: { 'Authorization': 'Bearer ' + t } });
                if (res.ok) {
                    const me = await res.json();
                    worldId = me.current_world_id || '';
                }
            }
        } catch (e) { /* метку не ставим — обычный заход на карту */ }
    }
    if (worldId) sessionStorage.setItem('beltReturn', worldId);
    window.location.href = '/map';
}

async function callShip() {
    if (state.leaving) return;
    state.leaving = true;
    playSound('ui_select');
    ui.setLeaveBusy(true);
    ui.notify('Возвращаюсь…');
    const res = await leave();
    if (res.status === 401) { window.location.href = '/login-page'; return; }
    if (!res.ok) {
        state.leaving = false;
        ui.setLeaveBusy(false);
        playSound('ui_error');
        ui.notify('Не удалось выйти из пояса: ' + res.error);
        return;
    }
    playSound('ui_success');
    state.running = false;
    clearInterval(state.collectTimer);
    returnToMap();
}

// ---- Ввод ----
function onEscape() {
    if (state.leaving) return;
    if (ui.isPaused()) ui.hidePause();
    else ui.showPause(() => ui.hidePause(), callShip);
}

// ---- Экран ошибки входа (§5.3) ----
function enterErrorMessage(res) {
    if (res.status === 0 || res.status >= 500) {
        return { text: 'Не удалось связаться с сервером.', retry: true };
    }
    const msg = (res.error || '').trim();
    if (msg.indexOf('выработан') >= 0) return { text: 'Пояс выработан — добывать здесь больше нечего.' };
    if (msg.indexOf('Трюм полон') >= 0) return { text: 'Трюм полон — сначала освободите место на корабле.' };
    if (msg.indexOf('Сначала долетите') >= 0) return { text: 'Сначала долетите до пояса.' };
    if (msg.indexOf('нет данных') >= 0) return { text: 'Нет данных о запасе пояса — добыча здесь недоступна.' };
    if (msg.indexOf('Пояс не найден') >= 0) return { text: 'Пояс не найден.' };
    return { text: msg || 'Заход в пояс невозможен.' };
}

function showEnterError(res) {
    ui.hideLoading();
    const info = enterErrorMessage(res);
    playSound('ui_error');
    // «Вернуться» с экрана ошибки тоже ведёт на карту с меткой системы (решение
    // создателя 2026-09-23): попап откроется, если метка совпадёт с current_world_id.
    ui.showError(info.text, { retry: info.retry ? boot : null, onReturn: returnToMap });
}

// ---- Запуск ----
async function boot() {
    activateSound();
    const params = new URLSearchParams(window.location.search);
    const beltID = params.get('belt');
    if (!beltID) { window.location.href = '/map'; return; }
    ui.showLoading('Заход в пояс…');
    loadShipSprite();
    // Прелоад спрайтов/паттернов до старта сцены (арт-ТЗ §4.3): первый кадр
    // уже со спрайтами, без подмены многоугольников. Ошибка файла → фолбэк.
    await preloadSprites();
    const res = await enter(beltID);
    if (!res.ok) {
        if (res.status === 401) { window.location.href = '/login-page'; return; }
        showEnterError(res);
        return;
    }
    const pkg = res.data || {};
    state.pkg = pkg;
    state.mined = num(pkg.mined);
    // Защитный дефолт: неполный пакет не должен ронять кадр (нет limits/rate_cap).
    state.rateCap = num(pkg.limits && pkg.limits.rate_cap);
    state.cargo = {
        used: num(pkg.cargo && pkg.cargo.used),
        total: num(pkg.cargo && pkg.cargo.total),
        free: num(pkg.cargo && pkg.cargo.free),
    };
    state.remainingLevel = pkg.remaining_level || '';
    state.beltClass = pkg.belt_class || '';
    state.full = isCargoFull();
    state.depleted = state.remainingLevel === 'выработан';
    state.toastFullShown = state.full;
    state.world = new BeltWorld(num(pkg.seed));
    window.__beltWorld = state.world; // e2e-снимки (tools/e2e/belt-sprites-shot.js)
    state.camera.x = 0;
    state.camera.y = 0;

    const canvas = document.getElementById('belt-canvas');
    resize(canvas);
    window.addEventListener('resize', () => resize(canvas));
    bindInput(state.input, { onEscape, canvas });
    ui.onLeave(callShip);

    ui.hideLoading();
    ui.showHUD();
    ui.setBeltName(pkg.belt_name, pkg.belt_kind);
    refreshHUD();

    if (state.mined > 0) {
        ui.notify('Сессия добычи возобновлена: в буфере ' + cargoNum(state.mined) + ' т');
    }

    state.running = true;
    state.lastTime = performance.now();
    requestAnimationFrame(frame);
    state.collectTimer = setInterval(collectTick, C.COLLECT_INTERVAL_MS);
}

boot();
