// web/static/js/map/flight.js
// Общая логика перелёта + панель в шапке карты.
// Используется и картой (tooltip/контекстное меню), и модалкой системы (ПКМ по звезде).

import { state, elements } from './config.js';
import { draw } from './map_render.js';
import { fetchWorldByID } from './data.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

// ensureWorld — берёт мир из кэша или подгружает по ID (даже вне текущего кадра).
async function ensureWorld(id, token) {
    let w = state.worlds.find(x => x.id === id);
    if (!w) {
        w = await fetchWorldByID(id, token);
        if (w) state.worlds.push(w);
    }
    return w;
}

// ==================== ПАНЕЛЬ ПЕРЕЛЁТА ====================

export function showFlightPanel(fromName, toName) {
    const panel = document.getElementById('flight-panel');
    if (panel) panel.classList.add('flying');
    const from = document.getElementById('flightFrom');
    const to = document.getElementById('flightTo');
    if (from) from.textContent = fromName || '—';
    if (to) to.textContent = toName || '—';
    updateFlightPanel();
}

export function updateFlightPanel() {
    const bar = document.getElementById('flightBar');
    const time = document.getElementById('flightTime');
    const from = document.getElementById('flightFrom');
    const to = document.getElementById('flightTo');
    if (!bar || !time) return;
    if (state.isFlying) {
        const elapsed = (Date.now() - state.flyStartTime) / 1000;
        const p = Math.min(Math.max(elapsed / (state.flyDuration || 1), 0), 1);
        bar.style.width = Math.round(p * 100) + '%';
        const remaining = Math.max(0, state.flyDuration - elapsed);
        time.textContent = `${Math.ceil(remaining)} сек`;
        if (from) from.textContent = state.flyFrom ? state.flyFrom.name || '—' : '—';
        if (to) to.textContent = state.flyTo ? state.flyTo.name || '—' : '—';
    } else {
        bar.style.width = '0%';
        time.textContent = '—';
        if (from) from.textContent = '—';
        if (to) to.textContent = '—';
    }
}

export function hideFlightPanel() {
    // Панель скрывается, когда полёт завершён.
    const panel = document.getElementById('flight-panel');
    if (panel) panel.classList.remove('flying');
    updateFlightPanel();
}

// setCurrentWorldLabel — показывает имя текущего мира в шапке.
export function setCurrentWorldLabel(name) {
    const label = document.getElementById('currentWorldName');
    if (label) label.textContent = name || '—';
}

// ==================== НАЧАЛО ПЕРЕЛЁТА ====================

// startFlight — отправляет /travel, подгружает миры, запускает полёт и показывает панель.
// Возвращает true при успехе, false при ошибке.
export async function startFlight(worldId, token) {
    if (state.isFlying) {
        notifyError('Уже в полёте');
        return false;
    }
    if (!worldId) {
        notifyError('Не выбран мир назначения');
        return false;
    }
    if (!token) {
        notifyError('Не авторизован');
        return false;
    }
    try {
        const res = await fetch('/travel', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token
            },
            body: JSON.stringify({ world_id: worldId })
        });
        const text = await res.text();
        if (!res.ok) {
            notifyError('Ошибка: ' + text);
            return false;
        }
        const data = JSON.parse(text);

        const [fromWorld, toWorld] = await Promise.all([
            ensureWorld(data.from, token),
            ensureWorld(data.to, token),
        ]);
        if (!fromWorld || !toWorld) {
            notifyError('Не удалось загрузить данные миров для перелёта');
            return false;
        }

        state.flyFrom = fromWorld;
        state.flyTo = toWorld;
        state.flyDuration = data.duration;
        state.flyStartTime = Date.now();
        state.isFlying = true;
        notifySuccess('Полёт начат: ' + fromWorld.name + ' → ' + toWorld.name);

        elements.tooltip.classList.remove('active');
        showFlightPanel(fromWorld.name, toWorld.name);
        setCurrentWorldLabel(fromWorld.name);
        draw();
        return true;
    } catch (e) {
        notifyError('Ошибка: ' + e.message);
        return false;
    }
}