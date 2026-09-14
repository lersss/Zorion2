// web/static/js/map/npc_agents.js
// NPC-агенты на карте (спека 20a.1 §7): периодический опрос
// /api/npc/positions (O(1) из PositionCache менеджера, согласованно с тиком
// 5с), отрисовка иконок (ромб, отличимый от звёзд), видимость — когда
// видны имена звёзд (nameDisplayThreshold), клик → мини-панель, WS-тост
// npc_arrivals_batch + рефреш.
import { state } from './config.js';
import { isFiniteNumber } from './utils.js';
import { CONFIG } from '../config.js';
import { notifyInfo } from '../ui/toast.js';
import { handleUnauthorized } from './data.js';
import { getShipSprite } from './ship_render.js';
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
        state.npcPositions = Array.isArray(data.positions) ? data.positions : [];
        draw();
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

// ==================== ОТРИСОВКА ====================

// npcScreenRadius — размер иконки на экране (растёт с зумом).
function npcScreenRadius() {
    return Math.max(3.5, 4.5 * state.scale);
}

// npcAngle — курс агента: вектор движения между последними опросами позиций
// (идл-агент не движется — курс сохраняется).
function npcAngle(p) {
    const last = lastNpcPos.get(p.id);
    let angle = last ? last.angle : 0;
    if (last && (p.x !== last.x || p.y !== last.y)) {
        angle = Math.atan2(p.y - last.y, p.x - last.x);
    }
    lastNpcPos.set(p.id, { x: p.x, y: p.y, angle });
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

    // Имя при достаточном зуме — как у звёзд (drawNames).
    const fontSize = Math.round(Math.min(
        mapCfg.nameMaxFontSize,
        Math.max(mapCfg.nameMinFontSize, mapCfg.nameFontSize * scale),
    ));
    ctx.font = `${fontSize}px system-ui`;
    ctx.textAlign = 'center';

    for (const p of positions) {
        const px = p.x * scale + offsetX;
        const py = p.y * scale + offsetY;
        if (!isFiniteNumber(px) || !isFiniteNumber(py)) continue;
        // Экранный cull как у звёзд.
        if (px < -50 || py < -50 || px > canvasWidth + 50 || py > canvasHeight + 50) continue;

        const sprite = getShipSprite(p.id);
        if (sprite && sprite.complete && sprite.naturalWidth > 0) {
            ctx.save();
            ctx.translate(px, py);
            ctx.rotate(npcAngle(p));
            ctx.drawImage(sprite, -size, -size, size * 2, size * 2);
            ctx.restore();
        } else {
            // Фолбэк-ромб (И4): каталог пуст / спрайт ещё грузится.
            npcAngle(p);
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

        ctx.fillStyle = '#f1f5f9';
        ctx.fillText(p.name || '—', px, py + size + fontSize);
    }
}

// ==================== КЛИК: МИНИ-ПАНЕЛЬ ====================

// findNPCAgentAt — агент под курсором (экранные координаты).
export function findNPCAgentAt(mouseX, mouseY) {
    if (state.scale <= mapCfg.nameDisplayThreshold) return null;
    const positions = state.npcPositions || [];
    const size = npcScreenRadius();
    const hitRadius = Math.max(size + 4, mapCfg.minDistForClick / 2);
    let best = null;
    let bestDist = Infinity;
    for (const p of positions) {
        const px = p.x * state.scale + state.offsetX;
        const py = p.y * state.scale + state.offsetY;
        if (!isFiniteNumber(px) || !isFiniteNumber(py)) continue;
        const dist = Math.hypot(mouseX - px, mouseY - py);
        if (dist < hitRadius && dist < bestDist) {
            bestDist = dist;
            best = p;
        }
    }
    return best;
}

// showNPCPanel — мини-панель агента (имя, статус, текущий/целевой мир).
export function showNPCPanel(agent, screenX, screenY) {
    hideNPCPanel();
    const panel = document.createElement('div');
    panel.id = 'npc-agent-panel';
    panel.style.cssText = `
        position: fixed;
        left: ${Math.min(screenX, window.innerWidth - 260)}px;
        top: ${Math.max(8, screenY - 10)}px;
        background: #1a1a2e;
        border: 1px solid #334155;
        border-radius: 10px;
        box-shadow: 0 8px 24px rgba(0,0,0,0.5);
        padding: 10px 12px;
        min-width: 220px;
        z-index: 1100;
        font-size: 0.85rem;
        color: #e0e0e0;
        user-select: none;
    `;
    const short = id => id ? id.slice(0, 8) + '…' : '—';
    panel.innerHTML = `
        <div style="font-weight:600;margin-bottom:6px;">👁️ ${escHtml(agent.name || '—')}</div>
        <div>Статус: <b>${escHtml(agent.status || '—')}</b></div>
        <div>Текущий мир: <span title="${escHtml(agent.current_world_id || '')}">${short(agent.current_world_id)}</span></div>
        <div>Целевой мир: <span title="${escHtml(agent.target_world_id || '')}">${agent.target_world_id ? short(agent.target_world_id) : '—'}</span></div>
    `;
    document.body.appendChild(panel);
}

export function hideNPCPanel() {
    const panel = document.getElementById('npc-agent-panel');
    if (panel) panel.remove();
}

function escHtml(s) {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
}