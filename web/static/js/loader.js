// web/static/js/loader.js
// Единый лоадер: спиннер + текст + счётчик прошедших секунд.
//
// Использование:
//   import { showLoader } from '../loader.js';
//   const stop = showLoader(container, 'Загрузка статистики...');
//   ... await fetch(...) ...
//   stop('✅ Готово');
// stop() останавливает таймер и, если передан текст, подставляет его.

const LOADER_HTML = (message) => `
    <div class="zorion-loader" style="display:flex; flex-direction:column; align-items:center; justify-content:center; padding:32px; color:#94a3b8;">
        <div class="zorion-loader-spinner" style="width:28px; height:28px; border:3px solid rgba(74,158,255,0.25); border-top-color:#4a9eff; border-radius:50%; animation:zorionSpin 0.8s linear infinite;"></div>
        <div style="margin-top:12px; font-size:0.9rem; display:flex; align-items:center; gap:8px;">
            <span class="zorion-loader-text">${message}</span>
            <span class="zorion-loader-time" style="font-variant-numeric: tabular-nums; color:#7a8aa0;">0 с</span>
        </div>
    </div>`;

let styleInjected = false;

export function showLoader(container, message) {
    // Инжектим анимацию и стили один раз на страницу.
    if (!styleInjected) {
        const style = document.createElement('style');
        style.textContent = `
            @keyframes zorionSpin { to { transform: rotate(360deg); } }
            .zorion-loader-spinner { flex: none; }
        `;
        document.head.appendChild(style);
        styleInjected = true;
    }

    if (container) {
        container.innerHTML = LOADER_HTML(message);
    }

    const start = Date.now();
    const timeEl = container ? container.querySelector('.zorion-loader-time') : null;
    const timer = setInterval(() => {
        if (timeEl) {
            timeEl.textContent = Math.round((Date.now() - start) / 1000) + ' с';
        }
    }, 250);

    // stop(message?) — финальное сообщение вместо счётчика, таймер останавливается.
    return function stop(finalMessage, keepSpinner = false) {
        clearInterval(timer);
        if (!container) return;
        if (finalMessage) {
            container.innerHTML = finalMessage;
        } else if (timeEl) {
            timeEl.textContent = Math.round((Date.now() - start) / 1000) + ' с';
            if (keepSpinner === false) {
                const spinner = container.querySelector('.zorion-loader-spinner');
                if (spinner) spinner.style.visibility = 'hidden';
            }
        }
    };
}

// showTextLoader — для текстовых статусов (status-bar), без смены контейнера.
// Возвращает функцию финализации, которая подставляет ${elapsed} с.
export function showTextLoader(el, message) {
    if (!el) return () => {};
    const start = Date.now();
    el.textContent = `${message} 0 с`;
    const timeEl = el;
    const timer = setInterval(() => {
        timeEl.textContent = `${message} ${Math.round((Date.now() - start) / 1000)} с`;
    }, 250);
    return function stop(finalMessage) {
        clearInterval(timer);
        if (!el) return;
        if (finalMessage) {
            el.textContent = finalMessage;
        } else {
            el.textContent = `⏳ ${Math.round((Date.now() - start) / 1000)} с`;
        }
    };
}