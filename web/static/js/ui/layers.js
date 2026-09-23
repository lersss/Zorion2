// web/static/js/ui/layers.js
// Единый реестр слоёв интерфейса (спека 2026-09-23 «Слои интерфейса и стек
// попапов» §1–§3): владелец z-index, стека попапов, маршрутизации Esc/клик-вне,
// ловушки фокуса и ref-count блокировки прокрутки.
//
// Полосы (bands) вместо россыпи чисел: modal 1000 → menu 1100 → banner 1200 →
// alert 2000 → system 9000 → toast 10000. Внутри полосы шаг 10 по порядку
// открытия. Верхний слой — последний открытый; Esc и клик-вне адресуются
// ТОЛЬКО к верхнему маршрутизируемому (пассивные — тосты/баннеры/тултипы —
// получают z, но вне маршрутизации).
//
// Здесь только реестр (слои, стек, маршрутизация, фокус, блокировка прокрутки).
// Готовые диалоги полосы alert (`openAlert`/`openConfirm`, свои разметка и
// стили) — в `ui/alert.js`: он использует реестр, не наоборот (без цикла).
//
// Node-безопасность: DOM/fetch — только внутри функций (модуль попадает в
// import-граф админки через ui/toast.js, см. web/frontend_test.go).

export const LEVELS = {
    modal: 1000,
    menu: 1100,
    banner: 1200,
    alert: 2000,
    system: 9000,
    toast: 10000,
};

const STEP = 10;
// Порядок полос — для closeToLayer/closeAll по уровню.
const BANDS = ['modal', 'menu', 'banner', 'alert', 'system', 'toast'];

// stack — открытые слои в порядке открытия (последний — верхний).
const stack = [];

let scrollLocks = 0;      // ref-count блокировки прокрутки
let savedOverflow = null; // overflow документа до первой блокировки
let listenersOn = false;

function hasLevel(level) {
    return Object.prototype.hasOwnProperty.call(LEVELS, level);
}

function bandIndex(level) {
    const i = BANDS.indexOf(level);
    return i < 0 ? BANDS.indexOf('modal') : i;
}

function bandCount(level) {
    let n = 0;
    for (const h of stack) if (h.level === level) n++;
    return n;
}

// ==================== СТЕК ====================

// openLayer — открыть слой. el — уже смонтированный в DOM корень попапа
// (получает z-index полосы и служит границей клика-вне). contentEl — содержимое
// слоя, если корень является подложкой (модалка/алерт): клик по подложке
// считается кликом вне содержимого. Возвращает handle.
export function openLayer(el, opts) {
    if (!el) return null;
    const o = opts || {};
    const level = hasLevel(o.level) ? o.level : 'modal';
    const passive = o.passive !== undefined ? !!o.passive : (level === 'toast' || level === 'banner');

    const handle = {
        el,
        contentEl: o.contentEl || el,
        level,
        passive,
        closeOnEsc: o.closeOnEsc !== undefined ? !!o.closeOnEsc : (level !== 'toast' && level !== 'banner'),
        closeOnOutside: o.closeOnOutside !== undefined ? !!o.closeOnOutside : (level === 'menu' || level === 'modal'),
        trapFocus: o.trapFocus !== undefined ? !!o.trapFocus
            : (!passive && (level === 'modal' || level === 'alert' || level === 'system')),
        lockScroll: !!o.lockScroll,
        onClose: typeof o.onClose === 'function' ? o.onClose : null,
        result: undefined,
        closed: false,
        prevFocus: null,
        z: 0,
    };
    handle.z = LEVELS[level] + STEP * bandCount(level);
    el.style.zIndex = String(handle.z);
    stack.push(handle);
    ensureListeners();
    markLayerA11y(handle, o.label);
    if (handle.trapFocus) enterFocus(handle);
    if (handle.lockScroll) acquireScrollLock();

    handle.close = (result) => closeHandle(handle, result);
    return handle;
}

// closeTop — закрыть только верхний маршрутизируемый слой (Esc/клик-вне).
export function closeTop(reason) {
    const top = topLayer();
    if (!top) return false;
    closeHandle(top, reason);
    return true;
}

// closeToLayer — закрыть всё выше уровня/слоя; inclusive:true — и сам уровень.
// Пассивные (тосты/баннеры/тултипы) не гасятся (F2, §2): их снимает свой
// жизненный цикл или явный handle.close() — иначе закрытие модалки глушило бы
// баннер пакмана/координатный тултип.
export function closeToLayer(target, opts) {
    const inclusive = !!(opts && opts.inclusive);
    let from = -1;
    if (typeof target === 'string') {
        const bi = bandIndex(target);
        for (let i = 0; i < stack.length; i++) {
            if (stack[i].passive) continue;
            const b = bandIndex(stack[i].level);
            if (b > bi || (inclusive && b === bi)) { from = i; break; }
        }
    } else if (target) {
        const i = stack.indexOf(target);
        from = i < 0 ? -1 : (inclusive ? i : i + 1);
    }
    if (from < 0) return;
    const targets = stack.slice(from).filter(h => !h.passive);
    for (let i = targets.length - 1; i >= 0; i--) closeHandle(targets[i], 'closeToLayer');
}

// closeAll — закрыть всё (level — только указанную полосу). Без аргумента
// закрываются ТОЛЬКО маршрутизируемые (§2): видимые тосты/баннеры не гасим.
export function closeAll(level) {
    const targets = level ? stack.filter(h => h.level === level) : stack.filter(h => !h.passive);
    for (let i = targets.length - 1; i >= 0; i--) closeHandle(targets[i], 'closeAll');
}

// topLayer — верхний маршрутизируемый слой (пассивные пропускаются).
export function topLayer() {
    for (let i = stack.length - 1; i >= 0; i--) {
        if (!stack[i].passive) return stack[i];
    }
    return null;
}

export function hasOpen() {
    return stack.length > 0;
}

// ==================== ЗАКРЫТИЕ СЛОЯ ====================

// closeHandle — идемпотентное закрытие: снять из стека, вернуть фокус, вызвать
// onClose ровно один раз (onClose может ре-энтри вызывать closeToLayer —
// запись из стека уже снята, цикла нет).
function closeHandle(handle, result) {
    if (!handle || handle.closed) return;
    handle.closed = true;
    handle.result = result;
    const i = stack.indexOf(handle);
    if (i >= 0) stack.splice(i, 1);
    if (handle.lockScroll) releaseScrollLock();
    if (handle.prevFocus) restoreFocus(handle);
    if (handle.onClose) {
        try {
            handle.onClose(result);
        } catch (e) {
            console.error('layers: onClose', e);
        }
    }
}

// ==================== МАРШРУТИЗАЦИЯ ====================

function ensureListeners() {
    if (listenersOn || typeof document === 'undefined') return;
    listenersOn = true;
    document.addEventListener('keydown', onKeydown, true);
    document.addEventListener('pointerdown', onPointerDown, true);
    if (typeof window !== 'undefined' && window.addEventListener) {
        window.addEventListener('pagehide', releaseAllScrollLocks);
    }
}

function onKeydown(e) {
    const top = topLayer();
    if (!top) return;
    if (e.key === 'Escape') {
        if (!top.closeOnEsc) return;
        // Гасим событие, чтобы чужие/старые слушатели не закрыли нижний слой.
        e.preventDefault();
        e.stopPropagation();
        closeHandle(top, 'esc');
        return;
    }
    if (e.key === 'Tab' && top.trapFocus) trapTab(e, top);
}

function onPointerDown(e) {
    const top = topLayer();
    if (!top || !top.closeOnOutside) return;
    const content = top.contentEl || top.el;
    if (content && content.contains && content.contains(e.target)) return;
    closeHandle(top, 'outside');
}

// trapTab — циклический Tab внутри верхнего диалога.
function trapTab(e, layer) {
    const list = focusables(layer.contentEl || layer.el);
    if (!list.length) {
        e.preventDefault();
        return;
    }
    const idx = list.indexOf(document.activeElement);
    if (e.shiftKey) {
        if (idx <= 0) { e.preventDefault(); list[list.length - 1].focus(); }
    } else if (idx === -1 || idx === list.length - 1) {
        e.preventDefault();
        list[0].focus();
    }
}

const FOCUSABLE_SEL = 'a[href], button:not([disabled]), input:not([disabled]), ' +
    'select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusables(root) {
    if (!root || typeof root.querySelectorAll !== 'function') return [];
    return Array.prototype.slice.call(root.querySelectorAll(FOCUSABLE_SEL));
}

// markLayerA11y — семантика слоя: trapFocus-слой — диалог (role/aria-modal/
// tabindex, §3.2), маршрутизируемый слой полосы menu — role="menu".
function markLayerA11y(handle, label) {
    const target = handle.contentEl || handle.el;
    if (!target || !target.setAttribute) return;
    if (handle.trapFocus) {
        target.setAttribute('role', 'dialog');
        target.setAttribute('aria-modal', 'true');
        if (!target.hasAttribute('tabindex') && !(target.querySelector && target.querySelector('[autofocus]'))) {
            target.setAttribute('tabindex', '-1');
        }
    } else if (!handle.passive && handle.level === 'menu') {
        target.setAttribute('role', 'menu');
    }
    if (label && target.setAttribute) target.setAttribute('aria-label', label);
}

function enterFocus(handle) {
    if (typeof document === 'undefined') return;
    handle.prevFocus = document.activeElement || null;
    const root = handle.contentEl || handle.el;
    if (!root || typeof root.focus !== 'function') return;
    const auto = (root.querySelector && root.querySelector('[autofocus]')) || null;
    const target = auto || focusables(root)[0] || root;
    if (target && typeof target.focus === 'function') {
        try { target.focus(); } catch (e) { /* узел без фокуса — не блокируем */ }
    }
}

function restoreFocus(handle) {
    const prev = handle.prevFocus;
    if (!prev || typeof prev.focus !== 'function') return;
    if (prev.isConnected === false) return;
    try { prev.focus(); } catch (e) { /* узел потерял фокус — не блокируем */ }
}

// ==================== БЛОКИРОВКА ПРОКРУТКИ ====================

function acquireScrollLock() {
    if (typeof document === 'undefined') return;
    scrollLocks++;
    if (scrollLocks !== 1) return;
    savedOverflow = document.documentElement.style.overflow;
    document.documentElement.style.overflow = 'hidden';
}

function releaseScrollLock() {
    if (scrollLocks > 0) scrollLocks--;
    if (scrollLocks === 0) releaseAllScrollLocks();
}

// releaseAllScrollLocks — страховка навигации (§3.4): стек умирает вместе со
// страницей, а стиль документа надо вернуть.
function releaseAllScrollLocks() {
    scrollLocks = 0;
    if (typeof document === 'undefined') return;
    document.documentElement.style.overflow = savedOverflow || '';
    savedOverflow = null;
}
