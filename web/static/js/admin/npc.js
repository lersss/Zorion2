// web/static/js/admin/npc.js
// Вкладка «NPC» (спека 20a.1 §6; спека 26a.1 §9): таблица агентов
// (постранично, keyset-курсор — «Показать ещё»), создание/удаление,
// массовая генерация (асинхронный джоб + pollJob), метрики поведения
// (опрос 5 с, подсветка лагов), настройки менеджера (speed/tick/batch +
// глобальный рубильник пушей).
import { fetchWithAuth } from './auth.js';
import { notifyError, notifySuccess } from '../ui/toast.js';
import { pollJob, pollIntervals } from './poll.js';

const REFRESH_MS = 10000;      // авторефреш списка агентов (полёты идут постоянно)
const METRICS_MS = 5000;       // опрос метрик (спека 26a.1 §9)
const NPC_PAGE_LIMIT = 200;    // страница списка (спека 26a.1 §5.2, default 200)
const BULK_CONFIRM_THRESHOLD = 1000; // confirm при больших N (спека §9)

let npcBound = false;
let npcNextCursor = null; // keyset-курсор следующей страницы (null — конец)
let npcShown = 0;         // показано строк на экране

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

// ==================== СПИСОК АГЕНТОВ (пагинация, спека 26a.1 §5.4) ====================

// renderNPCRow — одна строка таблицы агента (имя, статус, миры, наблюдение,
// уведомления, удаление).
function renderNPCRow(a) {
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
    return row;
}

// updateMoreUI — счётчик «показано N» и видимость кнопки «Показать ещё».
function updateMoreUI() {
    const moreBtn = document.getElementById('npcMoreBtn');
    const countEl = document.getElementById('npcShownCount');
    if (moreBtn) moreBtn.style.display = npcNextCursor ? 'inline-block' : 'none';
    if (countEl) countEl.textContent = npcShown > 0 ? `Показано: ${npcShown}` : '';
}

// loadNPC — первая страница (свежие сверху). Авторефреш сбрасывает к первой
// странице — вкладка показывает «сейчас», накопленные страницы перерисовываются
// (спека 26a.1 §5.4).
export async function loadNPC() {
    const tbody = document.getElementById('npcBody');
    if (!tbody) return;
    try {
        const res = await fetchWithAuth(`/admin/npc?limit=${NPC_PAGE_LIMIT}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const agents = Array.isArray(data.agents) ? data.agents : [];
        npcNextCursor = data.next_cursor || null;
        npcShown = agents.length;

        tbody.innerHTML = '';
        if (agents.length === 0) {
            npcNextCursor = null;
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#94a3b8;">Агентов нет — создайте первого.</td></tr>';
        } else {
            for (const a of agents) tbody.appendChild(renderNPCRow(a));
        }
        updateMoreUI();
    } catch (e) {
        console.error('loadNPC error:', e);
        tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#f87171;">Ошибка загрузки</td></tr>';
    }
}

// loadMoreNPC — «Показать ещё»: аппендит следующую страницу к DOM.
export async function loadMoreNPC() {
    const tbody = document.getElementById('npcBody');
    if (!tbody || !npcNextCursor) return;
    try {
        const res = await fetchWithAuth(`/admin/npc?limit=${NPC_PAGE_LIMIT}&cursor=${encodeURIComponent(npcNextCursor)}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const agents = Array.isArray(data.agents) ? data.agents : [];
        npcNextCursor = data.next_cursor || null;
        npcShown += agents.length;
        for (const a of agents) tbody.appendChild(renderNPCRow(a));
        updateMoreUI();
    } catch (e) {
        console.error('loadMoreNPC error:', e);
        notifyError('Не удалось загрузить следующую страницу');
    }
}

// ==================== МЕТРИКИ (спека 26a.1 §8–9) ====================

// loadNPCMetrics — опрос /admin/npc/metrics: строка показателей + подсветка
// лагов (overdue_arrivals > 0 и full_batches > 0 — жёлтый/красный).
export async function loadNPCMetrics() {
    const el = document.getElementById('npcMetricsRow');
    if (!el) return;
    try {
        const res = await fetchWithAuth('/admin/npc/metrics');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const m = await res.json();
        const sched = m.scheduler || {};
        const bulk = m.last_bulk;
        const overdue = m.overdue_arrivals || 0;
        const fullBatches = sched.full_batches || 0;

        let bulkText = '—';
        if (bulk && bulk.count > 0) {
            bulkText = `${bulk.count} за ${(bulk.duration_ms / 1000).toFixed(1)} с`;
        }
        let tickText = (sched.last_tick_ms !== undefined && sched.last_tick_ms !== null)
            ? `${Math.round(sched.last_tick_ms)} мс` : '—';

        el.innerHTML = `
            Агентов: <b>${m.agents_total ?? '—'}</b> ·
            В полёте: <b>${m.agents_flying ?? '—'}</b> ·
            idle: <b>${m.agents_idle ?? '—'}</b> ·
            Очередь на тик: <b class="${overdue > 0 ? 'warn-text' : ''}">${overdue}</b> ·
            Тик: ${tickText} ·
            Полных батчей: <b class="${fullBatches > 0 ? 'warn-text' : ''}">${fullBatches}</b> ·
            Последняя пачка: ${bulkText}`;
    } catch (e) {
        console.error('loadNPCMetrics error:', e);
        el.textContent = 'Ошибка загрузки метрик';
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
        const chk = document.getElementById('npcNotifyGlobal');
        if (chk) chk.checked = !!s.notify_enabled_global;
    } catch (e) {
        console.error('loadNPCSettings error:', e);
    }
}

export async function saveNPCSettings() {
    const speed = parseFloat(document.getElementById('npcSpeedFactor').value);
    const tick = parseFloat(document.getElementById('npcTickInterval').value);
    const batch = parseInt(document.getElementById('npcBatchSize').value, 10);
    const globalNotify = document.getElementById('npcNotifyGlobal');

    const body = {};
    if (isFinite(speed) && speed > 0) body.speed_factor = speed;
    if (isFinite(tick) && tick >= 1) body.tick_interval_sec = tick;
    if (Number.isInteger(batch) && batch >= 1) body.batch_size = batch;
    if (globalNotify) body.notify_enabled_global = globalNotify.checked;
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

// ==================== МАССОВАЯ ГЕНЕРАЦИЯ (спека 26a.1 §4, §9) ====================

// generateBulk — «Сгенерировать N агентов»: confirm при больших N, POST →
// 202 → pollJob('generate_npc', ...); кнопка disabled во время джоба.
export async function generateBulk() {
    const count = parseInt(document.getElementById('npcBulkCount').value, 10);
    if (!Number.isInteger(count) || count < 1) {
        notifyError('Введите количество ≥ 1');
        return;
    }
    if (count >= BULK_CONFIRM_THRESHOLD &&
        !confirm(`Создать ${count} агентов? Операция необратима, пачка пишется одной транзакцией.`)) {
        return;
    }

    const btn = document.getElementById('npcBulkBtn');
    const progress = document.getElementById('npcBulkProgress');
    btn.disabled = true;
    try {
        const res = await fetchWithAuth('/admin/npc/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ count }),
        });
        if (res.status === 409) {
            notifyError('Генерация уже идёт');
            btn.disabled = false;
            return;
        }
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось запустить генерацию');
            btn.disabled = false;
            return;
        }

        progress.style.display = 'block';
        document.getElementById('npcBulkProgressBar').value = 0;
        document.getElementById('npcBulkProgressText').textContent = 'Подготовка...';
        document.getElementById('npcBulkResult').textContent = '';
        pollIntervals['generate_npc'] = setInterval(
            () => pollJob('generate_npc', 'npcBulkProgress', 'npcBulkResult', ''), 1500);
    } catch (e) {
        console.error('generateBulk error:', e);
        btn.disabled = false;
        notifyError('Ошибка запуска генерации');
    }
}

// ==================== МАССОВОЕ УДАЛЕНИЕ (правка 2026-09-15) ====================

// clearAllAgents — «Удалить всех агентов»: confirm с актуальным количеством
// (свежий запрос метрик — могло измениться с последнего опроса), POST →
// {deleted: N}. 409 — джоб генерации идёт, ждём.
export async function clearAllAgents() {
    let count = null;
    try {
        const m = await fetchWithAuth('/admin/npc/metrics');
        if (m.ok) {
            const d = await m.json();
            if (typeof d.agents_total === 'number') count = d.agents_total;
        }
    } catch (e) {
        console.error('clearAllAgents: metrics error:', e);
    }
    const label = count !== null
        ? `Удалить всех агентов (${count})? Это необратимо.`
        : 'Удалить всех агентов? Это необратимо.';
    if (!confirm(label)) return;

    try {
        const res = await fetchWithAuth('/admin/npc/clear', { method: 'POST' });
        if (res.status === 409) {
            notifyError('Генерация уже идёт — дождитесь завершения');
            return;
        }
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось удалить агентов');
            return;
        }
        const d = await res.json();
        notifySuccess(`Удалено: ${d.deleted}`);
        loadNPC();
        loadNPCMetrics();
    } catch (e) {
        console.error('clearAllAgents error:', e);
        notifyError('Ошибка удаления агентов');
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
    loadNPCMetrics();
    setInterval(loadNPC, REFRESH_MS);
    setInterval(loadNPCMetrics, METRICS_MS);
}