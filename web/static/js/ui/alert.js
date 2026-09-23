// web/static/js/ui/alert.js
// Диалоги поверх контекста (полоса alert спеки 2026-09-23 §2–§4): ошибка,
// предупреждение, подтверждение. Строятся на реестре слоёв (ui/layers.js):
// z-порядок, Esc, клик-вне, ловушка фокуса и возврат фокуса — реестра; здесь
// только содержимое диалога и результат выбора.
//
// Node-безопасность: DOM — только внутри функций (модуль попадает в
// import-граф админки через modal/events.js, см. web/frontend_test.go).
import { openLayer } from './layers.js';
import { playSound } from './sound.js';

const STYLE_ID = 'ui-alert-style';

// Счётчик для уникальных id заголовка/текста (aria-labelledby/describedby):
// несколько алертов подряд не должны ссылаться на один и тот же узел.
let dialogSeq = 0;

// openAlert — ошибка/предупреждение поверх всего (полоса alert): модалка под
// ним остаётся открытой. Кнопка по умолчанию — «Понятно». Возвращает
// Promise<actionId> (null, если закрыли без выбора: Esc).
export function openAlert(opts) {
    const o = opts || {};
    const actions = Array.isArray(o.actions) && o.actions.length
        ? o.actions
        : [{ id: 'ok', label: 'Понятно', primary: true }];
    // Звук ошибки (решение менеджера 2026-09-23, §3.5): играет только там, где
    // звук активирован (карта) — playSound до activateSound — no-op.
    playSound('ui_error');
    return openDialog({
        title: o.title || '',
        text: o.text || '',
        buttons: actions.map(a => ({ label: a.label || a.id, primary: !!a.primary, value: a.id })),
        dismissValue: null,
    });
}

// openConfirm — подтверждение действия: Promise<boolean>.
export function openConfirm(opts) {
    const o = opts || {};
    return openDialog({
        title: o.title || '',
        text: o.text || '',
        buttons: [
            { label: o.confirmLabel || 'ОК', primary: true, value: true },
            { label: o.cancelLabel || 'Отмена', value: false },
        ],
        dismissValue: false,
    });
}

function openDialog(cfg) {
    return new Promise((resolve) => {
        if (typeof document === 'undefined') {
            resolve(cfg.dismissValue);
            return;
        }
        ensureStyle();

        const overlay = document.createElement('div');
        overlay.className = 'ui-alert-overlay';
        overlay.dataset.layer = 'alert';

        const dialog = document.createElement('div');
        dialog.className = 'ui-alert';

        // Уникальные id и связка aria-labelledby/describedby (a11y содержимого
        // диалога). role="dialog"/aria-modal/tabindex ставит реестр — общее
        // правило слоя с trapFocus (§3.2), диалог его не дублирует.
        const uid = 'ui-alert-' + (++dialogSeq);
        const titleId = uid + '-title';
        const textId = uid + '-text';
        if (cfg.title) {
            const title = document.createElement('div');
            title.className = 'ui-alert-title';
            title.id = titleId;
            title.textContent = cfg.title;
            dialog.appendChild(title);
            dialog.setAttribute('aria-labelledby', titleId);
        }
        const text = document.createElement('div');
        text.className = 'ui-alert-text';
        text.id = textId;
        text.textContent = cfg.text || '';
        dialog.appendChild(text);
        dialog.setAttribute('aria-describedby', textId);

        const actions = document.createElement('div');
        actions.className = 'ui-alert-actions';
        cfg.buttons.forEach((b) => {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'ui-alert-btn' + (b.primary ? ' primary' : '');
            btn.textContent = b.label;
            btn.addEventListener('click', () => layer.close(b.value));
            actions.appendChild(btn);
        });
        dialog.appendChild(actions);
        overlay.appendChild(dialog);
        document.body.appendChild(overlay);

        const layer = openLayer(overlay, {
            level: 'alert',
            contentEl: dialog,
            closeOnEsc: true,
            closeOnOutside: false,
            onClose: (result) => {
                if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
                const chosen = cfg.buttons.find(b => b.value === result);
                resolve(chosen ? chosen.value : cfg.dismissValue);
            },
        });
    });
}

function ensureStyle() {
    if (typeof document === 'undefined') return;
    if (document.getElementById(STYLE_ID)) return;
    const style = document.createElement('style');
    style.id = STYLE_ID;
    style.textContent = `
        .ui-alert-overlay {
            position: fixed;
            inset: 0;
            background: rgba(0,0,0,0.55);
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .ui-alert {
            background: #1e293b;
            border: 1px solid #334155;
            border-radius: 12px;
            box-shadow: 0 12px 32px rgba(0,0,0,0.55);
            padding: 18px 20px;
            min-width: 280px;
            max-width: 440px;
            color: #e2e8f0;
            font: 0.9rem/1.45 system-ui, sans-serif;
        }
        .ui-alert-title {
            font-size: 1.05rem;
            font-weight: 600;
            color: #f1f5f9;
            margin-bottom: 8px;
        }
        .ui-alert-text {
            color: #cbd5e1;
            margin-bottom: 14px;
            white-space: pre-line;
        }
        .ui-alert-actions {
            display: flex;
            justify-content: flex-end;
            gap: 8px;
        }
        .ui-alert-btn {
            background: #0f172a;
            color: #e2e8f0;
            border: 1px solid #334155;
            border-radius: 8px;
            padding: 8px 14px;
            font-size: 0.9rem;
            cursor: pointer;
        }
        .ui-alert-btn:hover { border-color: #475569; }
        .ui-alert-btn.primary { background: #2563eb; border-color: #2563eb; color: #fff; }
        .ui-alert-btn.primary:hover { background: #1d4ed8; }
    `;
    document.head.appendChild(style);
}
