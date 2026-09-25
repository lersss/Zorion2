// web/static/js/admin/accelerator.js
// Кнопка «Сбросить мой таймер» на вкладке «Основное» (идея ускорителя §13):
// админский сброс СВОЕГО состояния ускорителя (откат + признак «ускорение
// действует») для наигрыша мини-игры без ожидания отката. Доступно обеим
// админ-ролям — сервер закрыт auth.AdminAuth (admin + skycomposer).
// Node-безопасен: ни document/fetch на верхнем уровне (frontend_test.go).
import { fetchWithAuth } from './auth.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

export async function resetAcceleratorSelf() {
    const ok = confirm('Сбросить таймер ускорителя на своём аккаунте? Снимутся откат и действующее ускорение.');
    if (!ok) return;

    const resultEl = document.getElementById('acceleratorResetStatus');
    if (resultEl) resultEl.textContent = '';
    try {
        const res = await fetchWithAuth('/admin/accelerator/reset-self', { method: 'POST' });
        if (res.ok) {
            if (resultEl) resultEl.textContent = '✅ Таймер сброшен';
            notifySuccess('Таймер ускорителя сброшен');
        } else {
            const err = await res.json().catch(() => ({}));
            const msg = err.error || 'Ошибка сервера';
            if (resultEl) resultEl.textContent = '❌ ' + msg;
            notifyError(msg);
        }
    } catch (e) {
        if (resultEl) resultEl.textContent = '❌ ' + e.message;
        notifyError(e.message);
    }
}
