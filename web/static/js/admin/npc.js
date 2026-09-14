// web/static/js/admin/npc.js
// Вкладка «NPC» (спека 20a.1 §6): таблица агентов (имя, статус, миры,
// last_observed_at, уведомления), создание/удаление, настройки менеджера
// (speed_factor / tick / batch). Ручки — этап 4 (admin_npc.go).
import { fetchWithAuth } from './auth.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

const REFRESH_MS = 10000; // авторефреш списка агентов (полёты идут постоянно)

let npcBound = false;

function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
}

function fmtDate(v) {
    if (!v) return '—';
    const d = new Date(v);
    return isNaN(d.getTime()) ? '—' : d.toLocaleString('ru-RU');
}

function fmtWorld(id) {
    return id ? esc(id.slice(0, 8) + '…') : '—';
}

const STATUS_LABEL = { idle: '🟢 idle', flying: '🔵 flying', observing: '🟡 observing' };

// ==================== СПИСОК АГЕНТОВ ====================

export async function loadNPC() {
    const tbody = document.getElementById('npcBody');
    if (!tbody) return;
    try {
        const res = await fetchWithAuth('/admin/npc');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const agents = Array.isArray(data.agents) ? data.agents : [];

        tbody.innerHTML = '';
        if (agents.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#94a3b8;">Агентов нет — создайте первого.</td></tr>';
            return;
        }
        for (const a of agents) {
            const row = document.createElement('tr');
            row.innerHTML = `
                <td>${esc(a.name)}</td>
                <td>${STATUS_LABEL[a.status] || esc(a.status)}</td>
                <td title="${esc(a.current_world_id || '')}">${fmtWorld(a.current_world_id)}</td>
                <td title="${esc(a.target_world_id || '')}">${fmtWorld(a.target_world_id)}</td>
                <td>${fmtDate(a.last_observed_at)}</td>
                <td><input type="checkbox" ${a.notify_enabled ? 'checked' : ''}></td>
                <td><button class="btn-small danger">Удалить</button></td>`;
            const chk = row.querySelector('input[type=checkbox]');
            chk.addEventListener('change', () => toggleNotify(a.id, chk.checked));
            row.querySelector('button').addEventListener('click', () => deleteAgent(a.id, a.name));
            tbody.appendChild(row);
        }
    } catch (e) {
        console.error('loadNPC error:', e);
        tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#f87171;">Ошибка загрузки</td></tr>';
    }
}

// ==================== НАСТРОЙКИ ====================

export async function loadNPCSettings() {
    try {
        const res = await fetchWithAuth('/admin/npc/settings');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const s = await res.json();
        const setVal = (id, v) => {
            const el = document.getElementById(id);
            if (el) el.value = v;
        };
        setVal('npcSpeedFactor', s.speed_factor);
        setVal('npcTickInterval', s.tick_interval_sec);
        setVal('npcBatchSize', s.batch_size);
    } catch (e) {
        console.error('loadNPCSettings error:', e);
    }
}

export async function saveNPCSettings() {
    const speed = parseFloat(document.getElementById('npcSpeedFactor').value);
    const tick = parseFloat(document.getElementById('npcTickInterval').value);
    const batch = parseInt(document.getElementById('npcBatchSize').value, 10);

    const body = {};
    if (isFinite(speed) && speed > 0) body.speed_factor = speed;
    if (isFinite(tick) && tick >= 1) body.tick_interval_sec = tick;
    if (Number.isInteger(batch) && batch >= 1) body.batch_size = batch;
    if (Object.keys(body).length === 0) {
        notifyError('Введите корректные значения настроек');
        return;
    }

    try {
        const res = await fetchWithAuth('/admin/npc/settings', {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сохранить настройки');
            return;
        }
        notifySuccess('Настройки NPC сохранены');
        loadNPCSettings();
    } catch (e) {
        console.error('saveNPCSettings error:', e);
        notifyError('Ошибка сохранения настроек');
    }
}

// ==================== СОЗДАНИЕ / УДАЛЕНИЕ ====================

export async function createAgent() {
    const name = document.getElementById('npcAgentName').value.trim();
    const startWorld = document.getElementById('npcStartWorld').value.trim();
    const resultEl = document.getElementById('npcCreateResult');

    if (!name) {
        resultEl.textContent = '❌ Имя обязательно';
        return;
    }
    const body = { name };
    if (startWorld) body.start_world_id = startWorld;

    try {
        const res = await fetchWithAuth('/admin/npc', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            resultEl.textContent = '❌ ' + (err.error || 'Не удалось создать агента');
            return;
        }
        resultEl.textContent = '✅ Агент создан';
        document.getElementById('npcAgentName').value = '';
        document.getElementById('npcStartWorld').value = '';
        notifySuccess('NPC-агент создан');
        loadNPC();
    } catch (e) {
        console.error('createAgent error:', e);
        resultEl.textContent = '❌ Ошибка соединения';
    }
}

export async function toggleNotify(id, enabled) {
    try {
        const res = await fetchWithAuth('/admin/npc/' + encodeURIComponent(id), {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ notify_enabled: enabled }),
        });
        if (!res.ok) {
            notifyError('Не удалось переключить уведомления');
            loadNPC(); // вернуть чекбокс к серверному состоянию
        }
    } catch (e) {
        console.error('toggleNotify error:', e);
        notifyError('Ошибка переключения уведомлений');
        loadNPC();
    }
}

export async function deleteAgent(id, name) {
    if (!confirm(`Удалить агента «${name}»?`)) return;
    try {
        const res = await fetchWithAuth('/admin/npc/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось удалить агента');
            return;
        }
        notifySuccess('Агент удалён');
        loadNPC();
    } catch (e) {
        console.error('deleteAgent error:', e);
        notifyError('Ошибка удаления агента');
    }
}

// ==================== ИНИЦИАЛИЗАЦИЯ ====================

// initNPC — ленивая загрузка вкладки (вызывается из tabs.js при активации).
export function initNPC() {
    if (npcBound) return;
    npcBound = true;
    loadNPC();
    loadNPCSettings();
    setInterval(loadNPC, REFRESH_MS);
}