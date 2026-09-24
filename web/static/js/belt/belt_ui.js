// web/static/js/belt/belt_ui.js
// HUD, экраны и локальные тосты страницы добычи в поясе (UI-спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча-ui §4–§6). Все значения — из
// пакета захода (§7 осн. спеки) и ответов collect; клиент их не пересчитывает.
// Тост — локальный #toast (низ-центр, 3 с), как на surface.html; глобальный
// ui/toast.js здесь не используется (§6.1).
import { cargoMassLabel, cargoPercent, cargoNum } from '../dashboard/cargo.js';

const $ = (id) => document.getElementById(id);

// ---- Цвета чипов (§4.3, дельта §15.3) — существующие константы проекта ----
// «выработан» — по ресурсу (не «Пояс выработан»: это глобальное состояние §15.5).
const RESERVE = {
    'полный': { text: 'Запас железа: полный', color: '#4ade80' },
    'истощается': { text: 'Запас железа: истощается', color: '#facc15' },
    'выработан': { text: 'Запас железа: выработан', color: '#ef4444' },
};
const RESERVE_NODATA = { text: 'Запас железа: нет данных', color: '#64748b' };
const RESERVE_ICE = {
    'полный': { text: 'Запас льда: полный', color: '#4ade80' },
    'истощается': { text: 'Запас льда: истощается', color: '#facc15' },
    'выработан': { text: 'Запас льда: выработан', color: '#ef4444' },
};
const BELT_CLASS = {
    'богатый': { text: 'Пояс: богатый', color: '#4ade80' },
    'средний': { text: 'Пояс: средний', color: '#facc15' },
    'бедный': { text: 'Пояс: бедный', color: '#f97316' },
};
const CLASS_NODATA = { text: 'Пояс: нет данных', color: '#64748b' };
const ICE_CLASS = {
    'богатый': { text: 'Лёд: богатый', color: '#4ade80' },
    'средний': { text: 'Лёд: средний', color: '#facc15' },
    'бедный': { text: 'Лёд: бедный', color: '#f97316' },
};

const num = (v) => (typeof v === 'number' && isFinite(v) ? v : 0);

const KIND_LABELS = {
    asteroid: 'пояс астероидов',
    kuiper: 'пояс астероидов',
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

// updateHUD — буферы захода (железо + вода), трюм (одна шкала по СУММЕ обоих
// буферов и груза, дельта §15.2), чипы запаса/класса по ресурсу (§15.3).
// Ледяной UI скрыт целиком, если льда нет (§15.4); при фолбэке железный UI =
// ровно v1. Данные — серверные (пакет/collect), клиент не считает.
export function updateHUD(s) {
    const minedIron = num(s.mined && s.mined.iron);
    const minedIce = num(s.mined && s.mined.ice);
    // Ледяная строка буфера видна при наличии льда; исключение — mined_ice > 0
    // (защита от потери груза, §15.4), даже если ресурса нет.
    const showIce = !!s.iceAvailable || minedIce > 0;

    const ironEl = $('hud-mined-iron');
    if (ironEl) ironEl.textContent = 'Железо Fe: ' + cargoNum(minedIron) + ' т';
    const iceEl = $('hud-mined-ice');
    if (iceEl) {
        iceEl.style.display = showIce ? '' : 'none';
        if (showIce) iceEl.textContent = 'Вода неочищенная: ' + cargoNum(minedIce) + ' т';
    }

    // Шкала трюма = ЗАНЯТОЕ место: груз в трюме + ОБА буфера захода, не более
    // total (масса общая, §15.1/§15.2). Буферы тоже занимают место.
    const total = num(s.total);
    let used = num(s.used) + minedIron + minedIce;
    if (used > total) used = total;
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

    // Ледяные чипы: строка скрыта целиком, если льда нет (§15.3). Внутри —
    // «нет данных» для льда НЕ показываем (двусмысленно, §15.3): чип без
    // данных просто не рисуется.
    const iceChips = $('hud-chips-ice');
    if (iceChips) iceChips.style.display = s.iceAvailable ? '' : 'none';
    const resIce = $('hud-reserve-ice');
    if (resIce) {
        const info = RESERVE_ICE[s.remainingLevelIce];
        resIce.style.display = info ? '' : 'none';
        if (info) setChip(resIce, info);
    }
    const clsIce = $('hud-ice-class');
    if (clsIce) {
        const info = ICE_CLASS[s.iceClass];
        clsIce.style.display = info ? '' : 'none';
        if (info) setChip(clsIce, info);
    }
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
    // onReturn (решение создателя 2026-09-23): «Вернуться» ведёт на карту с
    // меткой системы — карта откроет попап. Не задан — обычный переход на /map.
    if (map) {
        map.onclick = (opts && typeof opts.onReturn === 'function')
            ? opts.onReturn
            : () => { window.location.href = '/map'; };
    }
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
    b.textContent = busy ? 'Выходим…' : 'Вернуться';
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
