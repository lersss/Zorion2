// web/static/js/ui/toast.js
// Красивые попап-уведомления вместо стандартных alert().
import { playToast } from './sound.js';

let toastStyleAdded = false;

function ensureStyle() {
    if (toastStyleAdded) return;
    toastStyleAdded = true;
    const style = document.createElement('style');
    style.id = 'toast-style';
    style.textContent = `
        .toast-container {
            position: fixed;
            top: 16px;
            right: 16px;
            z-index: 10000;
            display: flex;
            flex-direction: column;
            gap: 10px;
            pointer-events: none;
            max-width: 360px;
        }
        .toast {
            pointer-events: auto;
            display: flex;
            align-items: flex-start;
            gap: 10px;
            padding: 12px 14px;
            border-radius: 10px;
            background: #1e293b;
            border: 1px solid #334155;
            box-shadow: 0 8px 24px rgba(0,0,0,0.5);
            color: #e0e0e0;
            font: 0.85rem/1.4 system-ui, sans-serif;
            animation: toastIn 0.25s ease;
            max-width: 360px;
        }
        .toast.hide { animation: toastOut 0.25s ease forwards; }
        .toast-icon {
            flex-shrink: 0;
            width: 24px;
            height: 24px;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.8rem;
        }
        .toast-error { border-color: rgba(248,113,113,0.5); }
        .toast-error .toast-icon { background: rgba(248,113,113,0.2); }
        .toast-success { border-color: rgba(74,222,128,0.5); }
        .toast-success .toast-icon { background: rgba(74,222,128,0.2); }
        .toast-info { border-color: rgba(96,165,250,0.5); }
        .toast-info .toast-icon { background: rgba(96,165,250,0.2); }
        .toast-msg { flex: 1; word-break: break-word; }
        .toast-close {
            flex-shrink: 0;
            background: none;
            border: none;
            color: #64748b;
            font-size: 0.9rem;
            cursor: pointer;
            padding: 0 2px;
            line-height: 1;
        }
        .toast-close:hover { color: #e0e0e0; }
        @keyframes toastIn {
            from { opacity: 0; transform: translateX(30px); }
            to { opacity: 1; transform: translateX(0); }
        }
        @keyframes toastOut {
            from { opacity: 1; transform: translateX(0); }
            to { opacity: 0; transform: translateX(30px); }
        }
    `;
    document.head.appendChild(style);
}

function ensureContainer() {
    let container = document.getElementById('toast-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'toast-container';
        container.className = 'toast-container';
        document.body.appendChild(container);
    }
    return container;
}

const ICONS = {
    error: '✕',
    success: '✓',
    info: 'ℹ'
};

function showToast(message, type = 'error', duration = 5000) {
    ensureStyle();
    const container = ensureContainer();

    // Звук тоста: только ошибка/успех (решение создателя В2=А); info — молча.
    if (type === 'error' || type === 'success') playToast(type);

    const el = document.createElement('div');
    el.className = 'toast toast-' + type;
    el.innerHTML = `
        <div class="toast-icon">${ICONS[type] || ICONS.info}</div>
        <div class="toast-msg"></div>
        <button class="toast-close" aria-label="Закрыть">✕</button>
    `;
    el.querySelector('.toast-msg').textContent = message;
    el.querySelector('.toast-close').addEventListener('click', () => dismiss(el));
    container.appendChild(el);

    let timer = null;
    function dismiss(t) {
        if (timer) { clearTimeout(timer); timer = null; }
        if (!t || !t.parentNode) return;
        t.classList.add('hide');
        setTimeout(() => { if (t.parentNode) t.parentNode.removeChild(t); }, 250);
    }
    el.dismiss = dismiss;

    if (duration > 0) timer = setTimeout(() => dismiss(el), duration);
    return el;
}

export function notifyError(message, duration = 5000) {
    return showToast(message, 'error', duration);
}

export function notifySuccess(message, duration = 4000) {
    return showToast(message, 'success', duration);
}

export function notifyInfo(message, duration = 4000) {
    return showToast(message, 'info', duration);
}

export function dismissToast(el) {
    if (el && el.dismiss) el.dismiss(el);
}

// Глобальный fallback — для инлайн-скриптов без import.
window.notifyError = notifyError;
window.notifySuccess = notifySuccess;
window.notifyInfo = notifyInfo;