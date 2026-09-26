// web/static/js/route/route_ui.js
// DOM/HUD и оверлеи мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §2–§4): паспорт/режим/легенда,
// карточка сектора с разведкой (§4.3), тост, оверлеи причины (§3) и результата
// (§4.7). Числа механики сюда не попадают — исключение только результат после
// отправки (bonus со знаком, §4.7): до отправки чисел/вердикта/оптимума нет.
import * as C from './route_config.js';

const $ = (id) => document.getElementById(id);

export const toMap = () => { window.location.href = '/map'; };

export const showLoading = (t) => { $('loading-text').textContent = t || 'Загрузка…'; $('loading').style.display = 'flex'; };
export const hideLoading = () => { $('loading').style.display = 'none'; };
export const showHud = () => { $('hud').style.display = 'flex'; };
export const hideHud = () => { $('hud').style.display = 'none'; };
export const showError = (m) => { $('error-text').textContent = m; $('error').style.display = 'flex'; };
export const hideError = () => { $('error').style.display = 'none'; };

let toastTimer = null;
// showToast — тост низ-центр на 3 с (§3): сырой текст ответа сервера не подставляем.
export function showToast(msg) {
    const el = $('toast');
    if (!el) return;
    el.textContent = msg;
    el.style.display = 'block';
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.style.display = 'none'; }, 3000);
}

let cooldownTimer = null;
// showReason — оверлей-причина (§3): тексты из REASON_OVERLAY + локальный досчёт
// отката `mm:ss` от cooldown_remaining_s.
export function showReason(reason, cooldownS) {
    const info = C.REASON_OVERLAY[reason] || { title: 'Мини-игра недоступна', note: '' };
    $('reason-title').textContent = info.title;
    $('reason-note').textContent = info.note || '';
    const timer = $('reason-timer');
    clearInterval(cooldownTimer);
    const left0 = Number(cooldownS);
    if (reason === 'cooldown' && isFinite(left0)) {
        const until = Date.now() + left0 * 1000;
        const tick = () => {
            const left = Math.max(0, Math.round((until - Date.now()) / 1000));
            timer.textContent = 'Откат ' + Math.floor(left / 60) + ':' + String(left % 60).padStart(2, '0');
            if (left <= 0) clearInterval(cooldownTimer);
        };
        timer.style.display = ''; tick();
        cooldownTimer = setInterval(tick, 1000);
    } else {
        timer.style.display = 'none';
    }
    $('reason').style.display = 'flex';
}

export function buildPassport(state) {
    $('passport-chips').innerHTML = C.passportChips(state.passport, state.board);
}

export function buildMode(state) {
    const el = $('mode-chip');
    if (!el) return;
    if (!state.mode) { el.style.display = 'none'; return; }
    el.textContent = 'Режим: ' + C.modeLabel(state.mode);
    el.style.display = '';
}

// ---- Карточка сектора (§4.3): σ, окружение, риск, «Разведать» / «Уже вскрыт» ----

function revealedContent(state, si) {
    const r = (state.revealed || []).find((x) => x.sector === si);
    return r ? r.content : null;
}

// renderSectorCard — карточка выбранного сектора; скрыта, если сектор не выбран.
// handlers: { onScan(), onClose() }.
export function renderSectorCard(state, handlers) {
    const el = $('sector-card');
    if (!el) return;
    const si = state.selectedSector;
    const sectors = (state.board && state.board.sectors) || [];
    const sec = (si == null) ? null : sectors[si];
    if (!sec) { el.style.display = 'none'; el.innerHTML = ''; return; }
    const content = revealedContent(state, si);
    let actions;
    if (content) {
        actions = '<button class="hud-btn" data-act="close">Закрыть</button>';
    } else {
        const busy = !!state.scanBusy;
        const can = state.pingsLeft > 0 && !busy;
        actions = '<button class="hud-btn" data-act="close">Закрыть</button>' +
            '<button class="hud-btn primary" data-act="scan"' + (can ? '' : ' disabled') + '>' +
            (busy ? 'Разведываю…' : 'Разведать') + '</button>';
    }
    const title = content
        ? C.contentLabel(content)
        : 'σ: ' + C.sigLabel(sec.sig) + ' · рядом: ' + C.surroundLabel(sec.surround);
    el.innerHTML =
        '<div class="sector-head">Сектор ' + (si + 1) + ' из ' + sectors.length +
        (content ? ' · Уже вскрыт' : '') + '</div>' +
        '<div class="sector-title">' + title + '</div>' +
        (content ? '' : '<p class="sector-risk">Риск помехи: ' + C.riskBySig(sec.sig) + '</p>') +
        (content ? '' : '<p class="sector-risk">Импульсов: ' + state.pingsLeft + ' · разведка стоит 1 импульс</p>') +
        '<div class="sector-actions">' + actions + '</div>';
    el.style.display = '';
    el.onclick = (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;
        const act = btn.getAttribute('data-act');
        if (act === 'scan' && handlers && handlers.onScan) handlers.onScan();
        else if (act === 'close' && handlers && handlers.onClose) handlers.onClose();
    };
}

// showResult — оверлей результата (§4.7): bonus СО ЗНАКОМ (в т. ч. отрицательный),
// процент, новый остаток, качественные слова. #result-breakdown остаётся скрытым.
export function showResult(bonus, remainingS) {
    const b = Number(bonus) || 0;
    const neg = b < 0;
    $('result-title').textContent = neg ? 'Перелёт стал длиннее' : 'Ускорение принято';
    $('result-class').textContent = C.bonusWord(b);
    $('result-bonus').textContent = 'Скорость перелёта ' + (neg ? '−' : '+') +
        Math.round(Math.abs(b) * 100) + ' %' + (neg ? ' (перелёт удлинился)' : '');
    $('result-remaining').textContent = 'Осталось ~' + C.remainWord(remainingS);
    const panel = $('result').querySelector('.panel');
    if (panel) panel.classList.toggle('result-negative', neg);
    $('result-breakdown').hidden = true; // разбор по 16 факторам — отдельный чекпоинт
    $('result').style.display = 'flex';
}
