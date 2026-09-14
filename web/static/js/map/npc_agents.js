// web/static/js/map/npc_agents.js
// NPC-агенты на карте (спека 20a.1 §7): периодический опрос
// /api/npc/positions (O(1) из PositionCache менеджера, согласованно с тиком
// 5с), отрисовка иконок (ромб, отличимый от звёзд), плавное движение летящих
// между сэмплами (линейная интерполяция, лаг в один опрос), видимость — когда
// видны имена звёзд (nameDisplayThreshold), клик → мини-панель, WS-тост
// npc_arrivals_batch + рефреш.
import { state } from './config.js';
import { isFiniteNumber } from './utils.js';
import { CONFIG } from '../config.js';
import { notifyInfo } from '../ui/toast.js';
import { handleUnauthorized } from './data.js';
import { getShipSprite } from './ship_render.js';
import { focusAgent } from './navigation.js';
// draw — циклический импорт map_render.js (map_render импортирует
// drawNPCAgents из этого модуля): ES-модули допускают цикл, доступ к draw
// только в рантайме (loadNPCPositions), после инициализации обоих модулей.
import { draw } from './map_render.js';

const { map: mapCfg } = CONFIG;

// Период опроса позиций — согласованно с npcTickInterval (5с, спека §7).
const POSITIONS_POLL_MS = 5000;

// lastNpcPos — прошлые позиции агентов для поворота спрайта по вектору
// движения (спека 99.2.15 §5.2; в позициях курса нет — берём из дельт).
const lastNpcPos = new Map(); // id -> {x, y, angle}

let npcLoopStarted = false;

// Снимки позиций для плавного движения летящих агентов (спека 20a.1 §7):
// интерполяция между прошлым и текущим сэмплом. prevNpcById — прошлый снимок
// (Map id -> p), curNpcAt/prevNpcAt — времена получения. state.npcPositions —
// текущий снимок (контракт, читается drawNPCAgents/findNPCAgentAt).
let prevNpcById = null; // Map(id -> p) | null — прошлый снимок
let curNpcById = null;  // Map(id -> p) | null — текущий снимок
let curNpcAt = 0;       // время получения текущего снимка
let prevNpcAt = 0;      // время получения предыдущего снимка
let npcAnimFrame = 0;   // id активного rAF-цикла плавного движения (0 — нет)

// ==================== ЗАГРУЗКА ПОЗИЦИЙ ====================

export async function loadNPCPositions() {
    const token = localStorage.getItem('token');
    if (!token) return;
    try {
        const res = await fetch('/api/npc/positions', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const positions = Array.isArray(data.positions) ? data.positions : [];
        // Пара снимков для плавного движения (спека 20a.1 §7): текущий уходит
        // в «прошлый», новый кладём с временем получения. Первый снимок — без
        // прошлого: летящие рисуются от текущей позиции (t=0, ТЗ §2).
        prevNpcById = curNpcById;
        prevNpcAt = curNpcAt;
        curNpcById = new Map(positions.map(p => [p.id, p]));
        curNpcAt = Date.now();
        state.npcPositions = positions;
        draw();
        ensureNpcAnimLoop();
    } catch (err) {
        // Тихий сбой: карта работает и без агентов; лог — в консоль.
        console.warn('loadNPCPositions error:', err);
    }
}

// startNPCLoop — периодический опрос позиций (раз в 5с) + WS-уведомления.
export function startNPCLoop() {
    if (npcLoopStarted) return;
    npcLoopStarted = true;
    loadNPCPositions();
    setInterval(loadNPCPositions, POSITIONS_POLL_MS);
    connectNPCSocket();
}

// ==================== WS: npc_arrivals_batch ====================

// connectNPCSocket — WebSocket к /ws: тост о прибытиях агентов + рефреш
// иконок (спека §5: broadcast всем подключённым). Ролей пока нет.
//
// Токен — в query (?token=JWT): браузерный WebSocket API не умеет кастомные
// заголовки, сервер принимает query-токен только для пути /ws
// (internal/auth/middleware.go). WS-URL не попадает в адресную строку,
// историю браузера и referer.
//
// Реконнект ограничен: WS-тост — некритичная функция (карта работает и без
// него), поэтому при закрытии (протухший токен → 401, рестарт сервера)
// пробуем несколько раз и останавливаемся — без вечной реконнект-петли.
const WS_RECONNECT_ATTEMPTS = 5;
const WS_RECONNECT_MS = 10000;

let wsReconnectTimer = null;
let wsReconnectAttempts = 0;

function connectNPCSocket() {
    const token = localStorage.getItem('token');
    if (!token) return; // не авторизован — сокет не стартуем (тихий отказ)
    const proto = location.protocol === 'https:' ? 'wss://' : 'ws://';
    let ws;
    try {
        ws = new WebSocket(proto + location.host + '/ws?token=' + encodeURIComponent(token));
    } catch (e) {
        return;
    }
    ws.onopen = () => {
        wsReconnectAttempts = 0; // соединение живое — сбрасываем счётчик попыток
    };
    ws.onmessage = (e) => {
        let data;
        try {
            data = JSON.parse(e.data);
        } catch (err) {
            return;
        }
        if (data.type !== 'npc_arrivals_batch') return;
        const n = (data.arrivals || []).length;
        const extra = data.total > 0 ? ` (ещё ${data.total})` : '';
        notifyInfo(`👁️ NPC-агент${n > 1 ? 'ы' : ''} прибыл${extra}`);
        loadNPCPositions();
    };
    ws.onclose = () => {
        wsReconnectAttempts++;
        if (wsReconnectAttempts > WS_RECONNECT_ATTEMPTS) return; // тихий отказ
        if (wsReconnectTimer) clearTimeout(wsReconnectTimer);
        wsReconnectTimer = setTimeout(connectNPCSocket, WS_RECONNECT_MS);
    };
}

// ==================== ПЛАВНОЕ ДВИЖЕНИЕ МЕЖДУ СЭМПЛАМИ ====================

// npcRenderPos — позиция агента на кадр: летящий — линейная интерполяция
// между прошлым и текущим сэмплом (спека 20a.1 §7 «при полёте иконка движется
// между сэмплами»), t = (now - t2)/(t2 - t1), зажат в [0,1]. Сегмент x1→x2
// играется после прихода x2 (лаг в один опрос: пара известна только когда оба
// сэмпла получены). t=0 — сразу после прихода x2 (рисуем от старого, ТЗ §2);
// t=1 — сегмент завершён, держим новый снимок до следующего опроса.
// Idle/нет прошлого сэмпла — позиция текущего снимка как есть.
function npcRenderPos(p) {
    if (p.status !== 'flying' || !prevNpcById || !prevNpcAt) return p;
    const prev = prevNpcById.get(p.id);
    if (!prev) return p; // агент только появился — без прошлого сэмпла
    const dt = curNpcAt - prevNpcAt;
    if (dt <= 0) return p;
    const t = Math.min(Math.max((Date.now() - curNpcAt) / dt, 0), 1);
    return {
        x: prev.x + (p.x - prev.x) * t,
        y: prev.y + (p.y - prev.y) * t,
    };
}

// npcNeedsAnim — есть ли летящий агент с незавершённой интерполяцией (t<1),
// чья интерполированная позиция попадает в текущий вьюпорт (запас 50px, как
// cull в drawNPCAgents). Пока есть — цикл перерисовки жив; иначе — стоп:
// полный draw() (Вороной, кластеры, звёзды) не крутится впустую (ТЗ §3).
function npcNeedsAnim() {
    if (!prevNpcById || !curNpcById) return false;
    if (state.scale <= mapCfg.nameDisplayThreshold) return false; // агенты не видны
    const dt = curNpcAt - prevNpcAt;
    if (dt <= 0) return false;
    if (Date.now() - curNpcAt >= dt) return false; // все сегменты завершены
    // Вьюпорт: позиция из npcRenderPos (та же, что рисуется) — без рассинхрона
    // «цикл жив, а агент уже уехал за экран».
    const { scale, offsetX, offsetY, canvasWidth, canvasHeight } = state;
    const positions = state.npcPositions || [];
    for (const p of positions) {
        if (p.status !== 'flying') continue;
        const pos = npcRenderPos(p);
        const px = pos.x * scale + offsetX;
        const py = pos.y * scale + offsetY;
        if (!isFiniteNumber(px) || !isFiniteNumber(py)) continue;
        if (px < -50 || py < -50 || px > canvasWidth + 50 || py > canvasHeight + 50) continue;
        return true;
    }
    return false;
}

// ensureNpcAnimLoop — поднять цикл перерисовки, если есть что анимировать.
// Вызывается после опроса и из drawNPCAgents (зум мог скрыть агентов — цикл
// сам останавливается в npcNeedsAnim, а при приближении поднимается снова).
function ensureNpcAnimLoop() {
    if (npcAnimFrame) return; // цикл уже крутится
    if (!npcNeedsAnim()) return;
    npcAnimFrame = requestAnimationFrame(npcAnimTick);
}

// npcAnimTick — кадр плавного движения. Отдельный лёгкий цикл (одна
// перерисовка на кадр, паттерн scheduleRedraw), не конфликтует с animationLoop
// из animation.js (тот — только полёт игрока): здесь рисуем только пока идёт
// интерполяция агентов, и пропускаем кадр при state.isFlying — полёт игрока
// уже перерисовывает карту каждый кадр, дублировать не нужно.
// npcAnimFrame держим ненулевым до конца тика: draw() внутри может снова
// дойти до ensureNpcAnimLoop (drawNPCAgents) — защита от лавины rAF.
function npcAnimTick() {
    if (!npcNeedsAnim()) {
        npcAnimFrame = 0; // нечего интерполировать — стоп до следующего опроса
        return;
    }
    if (!state.isFlying) draw();
    npcAnimFrame = requestAnimationFrame(npcAnimTick);
}

// ==================== ОТРИСОВКА ====================

// npcScreenRadius — размер иконки на экране (растёт с зумом).
function npcScreenRadius() {
    return Math.max(3.5, 4.5 * state.scale);
}

// npcAngle — курс агента: вектор движения между последними опросами позиций
// (идл-агент не движется — курс сохраняется). Позиции — интерполированные
// (npcRenderPos): угол от интерполированной позиции согласован с тем, где
// иконка видна на кадре (ТЗ §4, lastNpcPos хранит прошлые позиции).
function npcAngle(p, pos) {
    const last = lastNpcPos.get(p.id);
    let angle = last ? last.angle : 0;
    if (last && (pos.x !== last.x || pos.y !== last.y)) {
        angle = Math.atan2(pos.y - last.y, pos.x - last.x);
    }
    lastNpcPos.set(p.id, { x: pos.x, y: pos.y, angle });
    return angle;
}

// drawNPCAgents — иконки агентов. Вызывается из draw() в map_render.js.
// Видимость: когда видны имена звёзд (тот же порог nameDisplayThreshold,
// спека §7: на галактическом обзоре агенты скрыты). Вместо ромба — мини-спрайт
// схемы агента (assemblyFromSeed(id), спека 99.2.15 §5.2), повёрнутый по
// вектору движения; каталог пуст/спрайт не загружен — фолбэк-ромб (И4).
export function drawNPCAgents(ctx, canvasWidth, canvasHeight) {
    if (state.scale <= mapCfg.nameDisplayThreshold) return;
    const positions = state.npcPositions || [];
    if (positions.length === 0) return;

    const { scale, offsetX, offsetY } = state;
    const size = npcScreenRadius();

    for (const p of positions) {
        const pos = npcRenderPos(p);
        const px = pos.x * scale + offsetX;
        const py = pos.y * scale + offsetY;
        if (!isFiniteNumber(px) || !isFiniteNumber(py)) continue;
        // Экранный cull как у звёзд.
        if (px < -50 || py < -50 || px > canvasWidth + 50 || py > canvasHeight + 50) continue;

        const sprite = getShipSprite(p.id);
        if (sprite && sprite.complete && sprite.naturalWidth > 0) {
            ctx.save();
            ctx.translate(px, py);
            ctx.rotate(npcAngle(p, pos));
            ctx.drawImage(sprite, -size, -size, size * 2, size * 2);
            ctx.restore();
        } else {
            // Фолбэк-ромб (И4): каталог пуст / спрайт ещё грузится.
            npcAngle(p, pos);
            ctx.beginPath();
            ctx.moveTo(px, py - size);
            ctx.lineTo(px + size, py);
            ctx.lineTo(px, py + size);
            ctx.lineTo(px - size, py);
            ctx.closePath();
            ctx.fillStyle = '#f472b6';
            ctx.fill();
            ctx.strokeStyle = '#0f172a';
            ctx.lineWidth = 1;
            ctx.stroke();
        }

        // Подсветка найденного поиском агента (спека 26a.1 §6.2):
        // двойной ореол вокруг иконки; сброс — новый поиск/клик по пустому месту.
        if (state.highlightedNpcId && p.id === state.highlightedNpcId) {
            ctx.beginPath();
            ctx.arc(px, py, size * 1.9, 0, Math.PI * 2);
            ctx.strokeStyle = '#fbbf24';
            ctx.lineWidth = 2.5;
            ctx.stroke();
            ctx.beginPath();
            ctx.arc(px, py, size * 2.7, 0, Math.PI * 2);
            ctx.strokeStyle = 'rgba(251,191,36,0.35)';
            ctx.lineWidth = 1.5;
            ctx.stroke();
        }
    }

    // После кадра — поднять цикл перерисовки, если интерполяция ещё идёт
    // (зум мог скрыть агентов: цикл сам остановится, при приближении оживёт).
    ensureNpcAnimLoop();
}

// ==================== ТУЛТИП ПРИ НАВЕДЕНИИ (rollover) ====================

// npcTooltipEl — переиспользуемый тултип агента (создаётся один раз при
// первом наведении). pointer-events: none — не перехватывает клики/драг
// карты (наведение ≠ клик). Позиция обновляется по mousemove без
// пересоздания DOM (тултип не должен дёргаться на частых событиях).
let npcTooltipEl = null;

// showNPCTooltip — показать данные агента (имя, статус, текущий/целевой
// мир) рядом с курсором. Вызывается из initHover (events.js) при наведении
// на иконку; hideNPCTooltip — при уводе мыши.
export function showNPCTooltip(agent, screenX, screenY) {
    if (!npcTooltipEl) {
        npcTooltipEl = document.createElement('div');
        npcTooltipEl.id = 'npc-tooltip';
        npcTooltipEl.style.cssText = `
            position: fixed;
            pointer-events: none;
            z-index: 1100;
            background: #1a1a2e;
            border: 1px solid #334155;
            border-radius: 8px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.5);
            padding: 8px 10px;
            max-width: 260px;
            font-size: 0.8rem;
            color: #e0e0e0;
            user-select: none;
        `;
        document.body.appendChild(npcTooltipEl);
    }
    const short = id => id ? id.slice(0, 8) + '…' : '—';
    npcTooltipEl.innerHTML = `
        <div style="font-weight:600;margin-bottom:4px;">👁️ ${escHtml(agent.name || '—')}</div>
        <div>Статус: <b>${escHtml(agent.status || '—')}</b></div>
        <div>Текущий мир: <span title="${escHtml(agent.current_world_id || '')}">${short(agent.current_world_id)}</span></div>
        <div>Целевой мир: <span title="${escHtml(agent.target_world_id || '')}">${agent.target_world_id ? short(agent.target_world_id) : '—'}</span></div>
    `;
    // Смещение от курсора (12px); у правого края — влево, чтобы не уходить за экран.
    const left = screenX + 14 + npcTooltipEl.offsetWidth > window.innerWidth
        ? screenX - npcTooltipEl.offsetWidth - 14
        : screenX + 14;
    npcTooltipEl.style.left = Math.max(4, left) + 'px';
    npcTooltipEl.style.top = Math.max(4, screenY - 10) + 'px';
}

export function hideNPCTooltip() {
    if (npcTooltipEl) npcTooltipEl.remove();
    npcTooltipEl = null;
}

// findNPCAgentAt — агент под курсором (экранные координаты).
export function findNPCAgentAt(mouseX, mouseY) {
    if (state.scale <= mapCfg.nameDisplayThreshold) return null;
    const positions = state.npcPositions || [];
    const size = npcScreenRadius();
    const hitRadius = Math.max(size + 4, mapCfg.minDistForClick / 2);
    let best = null;
    let bestDist = Infinity;
    for (const p of positions) {
        const pos = npcRenderPos(p);
        const px = pos.x * state.scale + state.offsetX;
        const py = pos.y * state.scale + state.offsetY;
        if (!isFiniteNumber(px) || !isFiniteNumber(py)) continue;
        const dist = Math.hypot(mouseX - px, mouseY - py);
        if (dist < hitRadius && dist < bestDist) {
            bestDist = dist;
            best = p;
        }
    }
    return best;
}

// ==================== ПОИСК АГЕНТА НА КАРТЕ (спека 26a.1 §6.2) ====================

// clearNPCHighlight — сброс подсветки (новый поиск / клик по пустому месту).
export function clearNPCHighlight() {
    if (state.highlightedNpcId) {
        state.highlightedNpcId = null;
        draw();
    }
}

// initNPCSearch — поле + кнопка «🔍 Найти агента» в #zoom-controls
// (web/map.html): 0 → тост «Не найден»; 1 → центр + подсветка; >1 →
// мини-список (до 10, имя + мир), клик по пункту → центр + подсветка.
export function initNPCSearch() {
    const input = document.getElementById('npcSearchInput');
    const btn = document.getElementById('npcSearchBtn');
    if (!input || !btn) return;

    const run = () => searchNPCAgent(input.value.trim());
    btn.addEventListener('click', run);
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') run(); });
    // Клик вне поля/списка — скрыть список результатов.
    document.addEventListener('click', (e) => {
        if (!e.target.closest('#npc-search-results') &&
            e.target !== input && e.target !== btn) {
            hideNPCSearchResults();
        }
    });
}

async function searchNPCAgent(q) {
    clearNPCHighlight();
    hideNPCSearchResults();
    if (!q) {
        notifyInfo('Введите имя или id агента');
        return;
    }
    const token = localStorage.getItem('token');
    if (!token) return;
    try {
        const res = await fetch('/api/npc/search?q=' + encodeURIComponent(q), {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const results = Array.isArray(data.results) ? data.results : [];
        if (results.length === 0) {
            notifyInfo('Агент не найден');
            return;
        }
        if (results.length === 1) {
            focusSearchResult(results[0]);
            return;
        }
        showNPCSearchResults(results);
    } catch (e) {
        console.error('searchNPCAgent error:', e);
        notifyInfo('Ошибка поиска агента');
    }
}

// focusSearchResult — центр + подсветка. Позиции нет (агент не в снапшоте,
// x/y = null) — тост, без подсветки (спека §6.1: клиент центрирует только
// при наличии позиции).
function focusSearchResult(r) {
    if (!focusAgent(r.id)) {
        notifyInfo(`Агент «${r.name}» найден, но позиция ещё неизвестна`);
    }
}

// showNPCSearchResults — мини-список при >1 совпадении (до 10, имя + мир),
// выпадает под полем поиска; клик по пункту → центр + подсветка.
function showNPCSearchResults(results) {
    const input = document.getElementById('npcSearchInput');
    if (!input) return;
    const rect = input.getBoundingClientRect();

    const box = document.createElement('div');
    box.id = 'npc-search-results';
    box.style.cssText = `
        position: fixed;
        left: ${rect.left}px;
        top: ${rect.bottom + 4}px;
        min-width: ${Math.max(rect.width, 220)}px;
        max-height: 260px;
        overflow-y: auto;
        background: #1a1a2e;
        border: 1px solid #334155;
        border-radius: 8px;
        box-shadow: 0 8px 24px rgba(0,0,0,0.5);
        z-index: 1200;
        font-size: 0.85rem;
        color: #e0e0e0;
        user-select: none;
    `;
    const max = 10;
    for (const r of results.slice(0, max)) {
        const item = document.createElement('div');
        item.style.cssText = `
            padding: 8px 10px;
            cursor: pointer;
            border-bottom: 1px solid #2a2a44;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        `;
        item.textContent = `${r.name} · ${r.status || '—'}`;
        item.addEventListener('mouseenter', () => { item.style.background = '#2a2a44'; });
        item.addEventListener('mouseleave', () => { item.style.background = 'none'; });
        item.addEventListener('click', () => {
            focusSearchResult(r);
            hideNPCSearchResults();
        });
        box.appendChild(item);
    }
    if (results.length > max) {
        const more = document.createElement('div');
        more.style.cssText = 'padding: 6px 10px; color: #94a3b8; font-size: 0.8rem;';
        more.textContent = `и ещё ${results.length - max}…`;
        box.appendChild(more);
    }
    document.body.appendChild(box);
}

function hideNPCSearchResults() {
    const el = document.getElementById('npc-search-results');
    if (el) el.remove();
}

function escHtml(s) {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
}