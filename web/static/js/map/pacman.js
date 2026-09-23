// web/static/js/map/pacman.js
// Пакман — событие вселенной (спека 2026-09-20 §6): жёлтый пакман с ртом
// летит по карте и ест миры; баннер «Галактику пожирает Пакман! N / M»,
// HUD-счётчик, вспышки поедания, тосты. Состояние — state.pacman; рендер —
// drawPacman (вызывается из draw() в map_render.js); диспетчер WS —
// handlePacmanMessage (вызывается из npc_agents.js onmessage).
import { state } from './config.js';
import { isFiniteNumber } from './utils.js';
import { notifyInfo, notifyError } from '../ui/toast.js';
import { scheduleReload, loadUserData } from './data.js';
// Реестр слоёв (спека 2026-09-23 §5): баннер пакмана и оверлей «Галактика
// пуста» — пассивные слои полосы banner (z от реестра, маршрутизация мимо).
import { openLayer } from '../ui/layers.js';
// draw — циклический импорт map_render.js (map_render импортирует drawPacman
// из этого модуля, как npc_agents.js): ES-модули допускают цикл, доступ к
// draw только в рантайме (pacmanAnimTick), после инициализации обоих модулей.
import { draw } from './map_render.js';

// Экранный размер пакмана (спека §8): постоянный экранный размер, виден
// на любом зуме (как стрелка игрока). 34 px — создатель уменьшил в 3 раза
// от 100 px (2026-09-20: «сделаем меньше в 3 раза»); узнаваемость держат
// пульсирующий нимб, белые глаза и обводка.
const PACMAN_SCREEN_PX = 34;
// Частота анимации рта (~1.5 Гц).
const PACMAN_MOUTH_HZ = 1.5;
// Время жизни частиц поедания (~500 мс).
const PACMAN_PARTICLE_MS = 500;
// Длина следа (последние ~10 позиций).
const PACMAN_TRAIL_LEN = 10;

// state.pacman — состояние пакмана (спека §6.1).
state.pacman = {
    active: false, x: 0, y: 0, prevX: 0, prevY: 0, targetX: 0, targetY: 0,
    movedAt: 0, eatenTotal: 0, total: 0, status: null,
    follow: false, // «Следить за Пакманом»: камера держит его в центре (2026-09-20)
    eatenIds: new Set(),      // все съеденные (сервер удалил)
    pendingEaten: [],         // съеденные, но ещё «долетают» (видимы до лопания)
    pendingEatenIds: new Set(), // id из pendingEaten (для фильтра перезапроса)
    lastPositionAt: 0, lastPositionInterval: 1000, // темп позиций → maxDelay лопания
};

// Частицы поедания: {x, y, vx, vy, bornAt} в экранных координатах.
let pacmanParticles = [];
// След: последние позиции в мировых координатах.
let pacmanTrail = [];
// DOM-элементы баннера/оверлея пустой галактики (создаются лениво).
// Оба — пассивные слои полосы banner (спека 2026-09-23 §5): z от реестра,
// поведение прежнее (авто по событию WS), Esc/клик-вне их не касаются и они
// не «съедают» Esc у модалки.
let pacmanBannerEl = null;
let pacmanBannerHandle = null;
let pacmanEmptyEl = null;
let pacmanEmptyHandle = null;
// rAF-цикл перерисовки (анимация рта/движения при активном пакмане; без него
// карта перерисовывается ~3 раз/с по событиям — рот «замирал», 2026-09-20).
let pacmanAnimFrame = 0;
// Кнопка «👾 Следить» в шапке карты (map.html #pacmanFollowBtn).
let pacmanFollowBtn = null;

// handlePacmanMessage — диспетчер WS-событий пакмана (спека §5.2).
export function handlePacmanMessage(data) {
    switch (data.type) {
        case 'pacman_start':
            onPacmanStart(data);
            break;
        case 'pacman_position':
            onPacmanPosition(data);
            break;
        case 'pacman_eaten':
            onPacmanEaten(data);
            break;
        case 'pacman_end':
            onPacmanEnd(data);
            break;
        case 'pacman_notice':
            onPacmanNotice(data);
            break;
    }
}

function onPacmanStart(data) {
    const p = state.pacman;
    p.active = true;
    p.total = data.total || 0;
    p.eatenTotal = 0;
    p.status = 'running';
    p.x = p.prevX = p.targetX = 0;
    p.y = p.prevY = p.targetY = 0;
    p.movedAt = 0;
    p.eatenIds = new Set();
    p.pendingEaten = [];
    p.pendingEatenIds = new Set();
    p.lastPositionAt = 0;
    p.lastPositionInterval = 1000;
    pacmanTrail = [];
    pacmanParticles = [];
    showPacmanBanner();
    ensurePacmanAnimLoop();
    notifyInfo(`⚠️ Галактику пожирает Пакман! (${p.total} миров)`);
}

// activatePacman — «просыпание» от любого события пакмана (спека §6.1,
// уточнение реализации): поздний клиент не получает pacman_start (шлётся
// 1 раз при старте) — активируемся от position/eaten. Вызывается только
// при переходе false→true: баннер не пересоздаётся на каждое событие.
function activatePacman() {
    const p = state.pacman;
    p.active = true;
    p.status = 'running';
    if (!p.eatenIds) p.eatenIds = new Set();
    showPacmanBanner();
    ensurePacmanAnimLoop();
}

function onPacmanPosition(data) {
    const p = state.pacman;
    if (!p.active) {
        activatePacman();
        // Поздний клиент: первая позиция — без интерполяции от (0,0),
        // пакман появляется сразу в текущей точке (следующее событие
        // интерполирует от неё).
        p.prevX = p.x = data.x;
        p.prevY = p.y = data.y;
        p.movedAt = 0;
        return;
    }
    // Темп позиций → maxDelay лопания звезды (ждём пакмана, но не вечно).
    const now = Date.now();
    if (p.lastPositionAt > 0) {
        const iv = now - p.lastPositionAt;
        if (iv > 0 && iv < 30000) p.lastPositionInterval = iv;
    }
    p.lastPositionAt = now;
    if (p.movedAt > 0) {
        p.prevX = p.x;
        p.prevY = p.y;
    }
    p.targetX = data.x;
    p.targetY = data.y;
    p.movedAt = now;
}

function onPacmanEaten(data) {
    const p = state.pacman;
    if (!p.active) {
        activatePacman();
        p.total = data.total || 0;
    }
    p.eatenTotal = data.eaten_total || 0;
    p.total = data.total || p.total;
    // Съеденные миры НЕ лопаются сразу (2026-09-20, создатель: «не лопать,
    // пока он до звезды не долетит»): звезда видна, пока пакман летит к ней,
    // и лопается в момент прибытия (processPendingEaten в rAF). В eatenIds
    // кладём сразу (сервер удалил), а из отрисовки убираем при лопании.
    const eatenNow = data.worlds || [];
    for (const w of eatenNow) {
        p.eatenIds.add(w.id);
        if (!p.pendingEatenIds.has(w.id)) {
            p.pendingEaten.push({ id: w.id, x: w.x, y: w.y, since: Date.now() });
            p.pendingEatenIds.add(w.id);
        }
    }
    // Debounced перезапрос видимой области — новые кластеры от свежего
    // снапшота (полный снапшот: съеденные, но ещё «долетающие», остаются).
    scheduleReload();
    updatePacmanBanner();
}

// processPendingEaten — в rAF-цикле: «долетающие» звёзды лопаются, когда
// пакман добрался (в радиусе рта) ИЛИ истёк maxDelay (быстрая скорость —
// пакман «проскакивает», не ждём вечно).
function processPendingEaten() {
    const p = state.pacman;
    if (!p.active || p.pendingEaten.length === 0) return;
    const now = Date.now();
    const interval = Math.max(100, Math.min(5000, p.lastPositionInterval || 1000));
    const maxDelay = interval * 2 + 500;
    const reach = Math.max(PACMAN_SCREEN_PX / (state.scale || 1), 4); // рот в мировых ед.
    const remaining = [];
    for (const e of p.pendingEaten) {
        const dist = Math.hypot(p.x - e.x, p.y - e.y);
        if (dist < reach || now - e.since > maxDelay) {
            popEatenStar(e);
        } else {
            remaining.push(e);
        }
    }
    p.pendingEaten = remaining;
}

// popEatenStar — звезда лопнула: удалить из отрисовки + частицы.
function popEatenStar(e) {
    const p = state.pacman;
    state.worlds = (state.worlds || []).filter(w => w.id !== e.id);
    state.clusters = (state.clusters || []).filter(c => !(c.cnt === 1 && c.sid === e.id));
    p.pendingEatenIds.delete(e.id);
    spawnPacmanParticles(e.x, e.y);
}

function onPacmanEnd(data) {
    const p = state.pacman;
    p.active = false;
    p.status = data.status || 'done';
    p.follow = false;
    // Остаток «долетающих» лопается сразу (пакман кончил — ждать нечего).
    for (const e of p.pendingEaten) popEatenStar(e);
    p.pendingEaten = [];
    p.pendingEatenIds = new Set();
    hidePacmanBanner();
    stopPacmanAnimLoop();
    if (p.status === 'done') {
        notifyInfo(`👾 Пакман съел ${p.eatenTotal} миров`);
    } else if (p.status === 'canceled') {
        notifyInfo(`⏹️ Пакман остановлен (съедено ${p.eatenTotal} из ${p.total})`);
    } else {
        notifyError('Ошибка пакмана');
    }
}

function onPacmanNotice(data) {
    notifyInfo(data.message || '');
    if (data.message && data.message.indexOf('Ваш мир съеден') !== -1) {
        // Состояние «без мира» (77a): перезагрузка /me → карта без деталей.
        loadUserData(true);
    }
}

// ==================== БАННЕР И ОВЕРЛЕЙ ====================

function showPacmanBanner() {
    if (!pacmanBannerEl) {
        pacmanBannerEl = document.createElement('div');
        pacmanBannerEl.id = 'pacman-banner';
        // z-index не задаём: его выдаёт реестр (пассивный слой полосы banner).
        pacmanBannerEl.style.cssText = `
            position: fixed;
            top: 12px;
            left: 50%;
            transform: translateX(-50%);
            background: rgba(10,15,32,0.92);
            border: 1px solid #facc15;
            border-radius: 10px;
            padding: 8px 16px;
            font-size: 0.95rem;
            color: #facc15;
            text-align: center;
            box-shadow: 0 8px 24px rgba(0,0,0,0.5);
            pointer-events: none;
            user-select: none;
        `;
        pacmanBannerEl.innerHTML = `
            <div>⚠️ Галактику пожирает Пакман!</div>
            <div id="pacman-banner-counter">👾 0 / 0</div>
            <progress id="pacman-banner-bar" value="0" max="100" style="width:220px;height:6px;"></progress>
        `;
        document.body.appendChild(pacmanBannerEl);
    }
    if (!pacmanBannerHandle) {
        pacmanBannerHandle = openLayer(pacmanBannerEl, {
            level: 'banner',
            passive: true,
            closeOnEsc: false,
            closeOnOutside: false,
            trapFocus: false,
            onClose: () => {
                pacmanBannerHandle = null;
                if (pacmanBannerEl) pacmanBannerEl.style.display = 'none';
            },
        });
    }
    pacmanBannerEl.style.display = 'block';
    updatePacmanBanner();
    showPacmanFollowButton();
}

function updatePacmanBanner() {
    if (!pacmanBannerEl) return;
    const p = state.pacman;
    const counter = document.getElementById('pacman-banner-counter');
    if (counter) {
        // total неизвестен до первого pacman_eaten (поздний клиент) — «? / ?».
        counter.textContent = p.total > 0 ? `👾 ${p.eatenTotal} / ${p.total}` : '👾 ? / ?';
    }
    const bar = document.getElementById('pacman-banner-bar');
    if (bar && p.total > 0) bar.value = p.eatenTotal / p.total * 100;
}

// hidePacmanBanner — скрытие через слой реестра (без мёртвой записи в стеке);
// фолбэк — погасить узел без слоя (элемент переиспользуется, не удаляется).
function hidePacmanBanner() {
    if (pacmanBannerHandle) pacmanBannerHandle.close();
    else if (pacmanBannerEl) pacmanBannerEl.style.display = 'none';
    hidePacmanFollowButton();
}

// updateEmptyGalaxyOverlay — лёгкий оверлей «Галактика пуста — ждём
// генерации» (спека §6.3): 0 кластеров + нет текущего мира. Вызывается из
// drawPacman (дешёвая проверка, DOM-операции только при смене состояния).
function updateEmptyGalaxyOverlay() {
    const empty = !state.pacman.active &&
        (state.clusters || []).length === 0 &&
        !state.currentWorldId;
    if (empty && !pacmanEmptyEl) {
        pacmanEmptyEl = document.createElement('div');
        pacmanEmptyEl.id = 'pacman-empty-overlay';
        // z-index не задаём: его выдаёт реестр (пассивный слой полосы banner).
        pacmanEmptyEl.style.cssText = `
            position: fixed;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            background: rgba(10,15,32,0.85);
            border: 1px solid #334155;
            border-radius: 12px;
            padding: 16px 24px;
            font-size: 1.05rem;
            color: #e2e8f0;
            text-align: center;
            pointer-events: none;
            user-select: none;
        `;
        pacmanEmptyEl.textContent = '🌌 Галактика пуста — ждём генерации';
        document.body.appendChild(pacmanEmptyEl);
        pacmanEmptyHandle = openLayer(pacmanEmptyEl, {
            level: 'banner',
            passive: true,
            closeOnEsc: false,
            closeOnOutside: false,
            trapFocus: false,
            onClose: () => {
                pacmanEmptyHandle = null;
                if (pacmanEmptyEl) { pacmanEmptyEl.remove(); pacmanEmptyEl = null; }
            },
        });
    } else if (!empty && pacmanEmptyEl) {
        // Скрытие — только через handle реестра (§5, «призрачные слои»).
        if (pacmanEmptyHandle) pacmanEmptyHandle.close();
        else { pacmanEmptyEl.remove(); pacmanEmptyEl = null; }
    }
}

// ==================== ЧАСТИЦЫ ПОЕДАНИЯ ====================

function spawnPacmanParticles(x, y) {
    const px = x * state.scale + state.offsetX;
    const py = y * state.scale + state.offsetY;
    if (!isFiniteNumber(px) || !isFiniteNumber(py)) return;
    for (let i = 0; i < 10; i++) {
        const a = Math.random() * Math.PI * 2;
        const speed = 30 + Math.random() * 60;
        pacmanParticles.push({
            x: px, y: py,
            vx: Math.cos(a) * speed,
            vy: Math.sin(a) * speed,
            bornAt: Date.now(),
        });
    }
}

function drawPacmanParticles(ctx) {
    const now = Date.now();
    pacmanParticles = pacmanParticles.filter(pt => now - pt.bornAt < PACMAN_PARTICLE_MS);
    for (const pt of pacmanParticles) {
        const age = (now - pt.bornAt) / PACMAN_PARTICLE_MS;
        const px = pt.x + pt.vx * age;
        const py = pt.y + pt.vy * age;
        ctx.beginPath();
        ctx.arc(px, py, Math.max(1, 3 * (1 - age)), 0, Math.PI * 2);
        ctx.fillStyle = `rgba(250,204,21,${0.8 * (1 - age)})`;
        ctx.fill();
    }
}

// ==================== РЕНДЕР (спека §6.2) ====================

// drawPacman — огромный жёлтый круг с ртом, свечение, след. Вызывается из
// draw() в map_render.js поверх кластеров/звёзд, под баннером. Не
// перехватывает клики (hit-тест не добавляется — пакман не интерактивен).
export function drawPacman(ctx, canvasWidth, canvasHeight) {
    const p = state.pacman;

    // Позиция: интерполяция от prev к target по времени (как NPC-сэмплы).
    let x = p.x, y = p.y;
    if (p.active && p.movedAt > 0 && (p.prevX !== p.targetX || p.prevY !== p.targetY)) {
        const t = Math.min(Math.max((Date.now() - p.movedAt) / 1000, 0), 1);
        x = p.prevX + (p.targetX - p.prevX) * t;
        y = p.prevY + (p.targetY - p.prevY) * t;
        p.x = x;
        p.y = y;
    }

    const px = x * state.scale + state.offsetX;
    const py = y * state.scale + state.offsetY;
    if (!isFiniteNumber(px) || !isFiniteNumber(py)) {
        updateEmptyGalaxyOverlay();
        return;
    }

    if (p.active) {
        // След: последние позиции с убывающей альфой.
        pacmanTrail.push({ x, y });
        if (pacmanTrail.length > PACMAN_TRAIL_LEN) pacmanTrail.shift();
        for (let i = 0; i < pacmanTrail.length - 1; i++) {
            const tr = pacmanTrail[i];
            const trx = tr.x * state.scale + state.offsetX;
            const try_ = tr.y * state.scale + state.offsetY;
            const alpha = ((i + 1) / pacmanTrail.length) * 0.35;
            ctx.beginPath();
            ctx.arc(trx, try_, PACMAN_SCREEN_PX * 0.5 * (i / pacmanTrail.length), 0, Math.PI * 2);
            ctx.fillStyle = `rgba(250,204,21,${alpha})`;
            ctx.fill();
        }

        // Свечение: пульсирующий радиальный нимб (привлекает внимание на
        // дальних зумах, где 100-px круг теряется среди плотных звёзд).
        const pulse = 0.5 + 0.18 * Math.sin(Date.now() / 1000 * Math.PI * 2 * 2);
        const glow = ctx.createRadialGradient(px, py, PACMAN_SCREEN_PX * 0.3, px, py, PACMAN_SCREEN_PX * 2.2);
        glow.addColorStop(0, `rgba(250,204,21,${pulse})`);
        glow.addColorStop(0.5, 'rgba(250,204,21,0.12)');
        glow.addColorStop(1, 'rgba(250,204,21,0)');
        ctx.beginPath();
        ctx.arc(px, py, PACMAN_SCREEN_PX * 2.2, 0, Math.PI * 2);
        ctx.fillStyle = glow;
        ctx.fill();

        // Рот: тёмный клин, анимация открытия-закрытия ~1.5 Гц, ориентация —
        // по направлению движения (prev → target). Сектор рта реально
        // увеличивается-уменьшается: от 0° (закрыт) до ~57° (0.5 рад) —
        // «паку-паку» как у классического Пакмана (2026-09-20).
        const angle = Math.atan2(p.targetY - p.prevY, p.targetX - p.prevX);
        // Рот открывается от ~11° (запас, чтобы клин не слипался в точку)
        // до ~69° — классическая «паку-паку» (2026-09-20).
        const mouth = 0.10 + 0.50 * Math.abs(Math.sin(Date.now() / 1000 * Math.PI * 2 * PACMAN_MOUTH_HZ));
        // Заливка сектора-клина: путь ИЗ ЦЕНТРА (moveTo), иначе closePath
        // соединяет концы дуги хордой — «отрезанный край» вместо клина.
        // Классический Пакман: БЕЗ глаз и БЕЗ обводки (решение создателя
        // 2026-09-20) — только жёлтый круг с ртом; заметность дают нимб,
        // след и пульсация.
        ctx.beginPath();
        ctx.moveTo(px, py);
        ctx.arc(px, py, PACMAN_SCREEN_PX, angle + mouth, angle + Math.PI * 2 - mouth);
        ctx.closePath();
        ctx.fillStyle = '#facc15';
        ctx.fill();
    }

    drawPacmanParticles(ctx);
    updateEmptyGalaxyOverlay();
}

// ==================== rAF-ЦИКЛ И СЛЕЖЕНИЕ (2026-09-20) ====================

// ensurePacmanAnimLoop — непрерывная перерисовка, пока пакман активен:
// без неё карта рисуется ~3 раз/с по событиям (мышь, поллинг) — анимация
// рта и интерполяция движения «замирали» (создатель: «рот не открывает,
// ~3 фпс»). Паттерн ensureNpcAnimLoop из npc_agents.js.
function ensurePacmanAnimLoop() {
    if (pacmanAnimFrame) return; // цикл уже крутится
    if (!state.pacman.active) return;
    pacmanAnimFrame = requestAnimationFrame(pacmanAnimTick);
}

function stopPacmanAnimLoop() {
    if (pacmanAnimFrame) {
        cancelAnimationFrame(pacmanAnimFrame);
        pacmanAnimFrame = 0;
    }
}

// pacmanAnimTick — кадр: если включено слежение — камера держит пакмана в
// центре; перерисовка полная (как npcAnimTick: пропускаем кадр при полёте
// игрока — тот уже перерисовывает карту).
function pacmanAnimTick() {
    const p = state.pacman;
    if (!p.active) {
        pacmanAnimFrame = 0;
        return;
    }
    // «Долетающие» звёзды лопаются, когда пакман добрался до них.
    processPendingEaten();
    if (p.follow) {
        // Центрируем камеру на текущей (интерполированной) позиции пакмана.
        state.offsetX = state.canvasWidth / 2 - p.x * state.scale;
        state.offsetY = state.canvasHeight / 2 - p.y * state.scale;
        if (typeof window.pacmanPersistViewport === 'function') {
            try { window.pacmanPersistViewport(); } catch (e) { /* не критично */ }
        }
    }
    if (!state.isFlying) draw();
    pacmanAnimFrame = requestAnimationFrame(pacmanAnimTick);
}

// initPacmanFollowButton — кнопка «👾 Следить» в шапке карты (#pacmanFollowBtn,
// map.html): показывается при активном пакмане, клик переключает слежение
// (камера следует за пакманом). Подсветка кнопки — class .active (CSS).
export function initPacmanFollowButton() {
    if (pacmanFollowBtn) return;
    const btn = document.getElementById('pacmanFollowBtn');
    if (!btn) return;
    pacmanFollowBtn = btn;
    btn.addEventListener('click', () => {
        const p = state.pacman;
        if (!p.active) return;
        p.follow = !p.follow;
        btn.classList.toggle('active', p.follow);
        btn.textContent = p.follow ? '⏹️ Не следить' : '👾 Следить за Пакманом';
        ensurePacmanAnimLoop();
    });
}

// showPacmanBanner — также показывает кнопку «Следить» (событие активно).
function showPacmanFollowButton() {
    if (!pacmanFollowBtn) initPacmanFollowButton();
    if (!pacmanFollowBtn) return;
    pacmanFollowBtn.style.display = '';
    pacmanFollowBtn.classList.remove('active');
    pacmanFollowBtn.textContent = '👾 Следить за Пакманом';
}

function hidePacmanFollowButton() {
    if (pacmanFollowBtn) {
        pacmanFollowBtn.style.display = 'none';
        pacmanFollowBtn.classList.remove('active');
    }
}