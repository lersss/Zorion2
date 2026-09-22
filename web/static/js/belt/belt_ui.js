// web/static/js/belt/belt_ui.js
// HUD, экраны и локальные тосты страницы добычи в поясе (UI-спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча-ui §4–§6). Все значения — из
// пакета захода (§7 осн. спеки) и ответов collect; клиент их не пересчитывает.
// Тост — локальный #toast (низ-центр, 3 с), как на surface.html; глобальный
// ui/toast.js здесь не используется (§6.1).
import { cargoMassLabel, cargoPercent, cargoNum } from '../dashboard/cargo.js';

const $ = (id) => document.getElementById(id);

// ---- Цвета чипов (§4.3) — существующие константы проекта ----
const RESERVE = {
    'полный': { text: 'Запас пояса: полный', color: '#4ade80' },
    'истощается': { text: 'Запас пояса: истощается', color: '#facc15' },
    'выработан': { text: 'Пояс выработан', color: '#ef4444' },
};
const RESERVE_NODATA = { text: 'Запас пояса: нет данных', color: '#64748b' };
const BELT_CLASS = {
    'богатый': { text: 'Пояс: богатый', color: '#4ade80' },
    'средний': { text: 'Пояс: средний', color: '#facc15' },
    'бедный': { text: 'Пояс: бедный', color: '#f97316' },
};
const CLASS_NODATA = { text: 'Пояс: нет данных', color: '#64748b' };

const KIND_LABELS = {
    asteroid: 'пояс астероидов',
    kuiper: 'пояс Койпера',
    debris: 'обломочный пояс',
    dust_ring: 'пылевое кольцо',
    oort: 'облако Оорта',
};

let leaveHandler = null;
let lastHint = null;
let toastTimer = null;

// ==================== ПРЕЛОАДЕР / HUD ====================

export function showLoading(text) {
    const el = $('loading-text');
    if (el) el.textContent = text || 'Загрузка…';
    const o = $('loading');
    if (o) o.style.display = 'flex';
}
export function hideLoading() {
    const o = $('loading');
    if (o) o.style.display = 'none';
}

export function showHUD() {
    const h = $('hud');
    if (h) h.style.display = 'flex';
}
export function hideHUD() {
    const h = $('hud');
    if (h) h.style.display = 'none';
}

// setBeltName — имя пояса и его тип в .hud-center (§4.2 п.4).
export function setBeltName(name, kind) {
    const el = $('hud-belt');
    if (!el) return;
    const label = KIND_LABELS[kind] || kind || 'пояс';
    el.textContent = name ? ('Пояс ' + name) : label;
}

// setChip — чип уровня запаса/класса: текст и цвета (border/color) на rgba-фоне.
function setChip(el, info) {
    if (!el) return;
    el.textContent = info.text;
    el.style.color = info.color;
    el.style.borderColor = info.color;
}

// updateHUD — буфер захода, трюм (полоса/подпись/бейдж «Полон»), чипы запаса и
// класса (§4.2/§4.3). Данные — серверные (пакет/collect), клиент не считает.
export function updateHUD(s) {
    const minedEl = $('hud-mined');
    if (minedEl) minedEl.textContent = cargoNum(s.mined) + ' т';

    // Шкала трюма = ЗАНЯТОЕ место: груз в трюме + буфер захода (mined), не более
    // total. Буфер захода тоже занимает место (спека трюма §9.1, осн. §5.3.2):
    // во время добычи шкала растёт вместе с буфером, а не стоит на нуле.
    const total = typeof s.total === 'number' && isFinite(s.total) ? s.total : 0;
    let used = (typeof s.used === 'number' && isFinite(s.used) ? s.used : 0)
        + (typeof s.mined === 'number' && isFinite(s.mined) ? s.mined : 0);
    if (used > total) used = total;
    // «Полон» — согласовано со шкалой: занятое место достигло total
    // (совпадает с серверным full = «свободно − буфер ≤ 0», осн. §5.3.2).
    const full = total > 0 && used >= total - 1e-9;

    const fill = $('cargo-fill');
    const text = $('cargo-text');
    const badge = $('cargo-full-badge');
    if (fill) {
        fill.style.width = cargoPercent(used, total) + '%';
        fill.style.background = full ? '#ef4444' : '#3b82f6';
    }
    if (text) text.textContent = cargoMassLabel(used, total);
    if (badge) badge.style.display = full ? 'inline-block' : 'none';

    setChip($('hud-reserve'), RESERVE[s.remainingLevel] || RESERVE_NODATA);
    setChip($('hud-belt-class'), BELT_CLASS[s.beltClass] || CLASS_NODATA);
}

// ==================== ПОДСКАЗКА У ЦЕЛИ (§4.4) ====================

// setHint — единственная динамическая строка HUD. html перестраивается только
// при изменении (кнопка выхода в подсказке не пересоздаётся каждый кадр).
export function setHint(html) {
    const el = $('hud-hint');
    if (!el || html === lastHint) return;
    lastHint = html;
    el.innerHTML = html;
    const b = el.querySelector('#hint-leave');
    if (b && leaveHandler) b.addEventListener('click', leaveHandler);
}

// ==================== ЛЕГЕНДА УПРАВЛЕНИЯ (§4.2 п.7) ====================

// updateControls — подсветка активных чипов легенды «двух стиков»: W/S тяга,
// A/D стрейф, мышь нос (курсор ведёт нос — input.aim), ЛКМ/Space добыча,
// Shift тормоз.
export function updateControls(input) {
    const map = {
        forward: input.forward, back: input.back,
        left: input.left, right: input.right,
        brake: input.brake, space: input.space,
        lmb: input.mouseDown, mouse: !!input.aim,
    };
    for (const key in map) {
        const els = document.querySelectorAll('.ctrl-key[data-key="' + key + '"]');
        for (const el of els) el.classList.toggle('active', !!map[key]);
    }
}

// ==================== ПАУЗА ====================

export function showPause(onResume, onLeave) {
    const r = $('pause-resume');
    const l = $('pause-leave');
    if (r) r.onclick = onResume;
    if (l) l.onclick = onLeave;
    const o = $('pause');
    if (o) o.style.display = 'flex';
}
export function hidePause() {
    const o = $('pause');
    if (o) o.style.display = 'none';
}
export function isPaused() {
    const o = $('pause');
    return !!o && o.style.display === 'flex';
}

// ==================== ЭКРАН ОШИБКИ (§5.3) ====================

export function showError(message, opts) {
    const t = $('error-text');
    if (t) t.textContent = message || 'Не удалось открыть заход.';
    const retry = $('error-retry');
    if (retry) {
        if (opts && typeof opts.retry === 'function') {
            retry.style.display = '';
            retry.onclick = opts.retry;
        } else {
            retry.style.display = 'none';
        }
    }
    const map = $('error-map');
    if (map) map.onclick = () => { window.location.href = '/map'; };
    const o = $('error');
    if (o) o.style.display = 'flex';
}
export function hideError() {
    const o = $('error');
    if (o) o.style.display = 'none';
}

// ==================== ПИЛЛ «НЕТ СВЯЗИ» (§5.4) ====================

export function showPill() {
    const p = $('offline-pill');
    if (p) p.classList.add('show');
}
export function hidePill() {
    const p = $('offline-pill');
    if (p) p.classList.remove('show');
}

// ==================== КНОПКА ВЫХОДА (§4.2 п.6) ====================

export function onLeave(fn) {
    leaveHandler = fn;
    const b = $('belt-leave-btn');
    if (b) b.onclick = fn;
}

export function setLeaveBusy(busy) {
    const b = $('belt-leave-btn');
    if (!b) return;
    b.disabled = !!busy;
    b.textContent = busy ? 'Выходим…' : 'Вернуться на карту';
}

// ==================== ТОСТ (§6.1) ====================

export function notify(msg) {
    const el = $('toast');
    if (!el) return;
    el.textContent = msg;
    el.style.display = 'block';
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.style.display = 'none'; }, 3000);
}
