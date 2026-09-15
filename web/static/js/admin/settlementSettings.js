// web/static/js/admin/settlementSettings.js
// Настройки населения (спека 99.2.16 §5.1): карточка «Настройки населения»
// на вкладке «Основное» — СПЖ (лет) и коэффициент рождаемости k, подпись
// нетто-темпа на лету, PATCH /admin/settlement-settings. Значения грузятся
// при каждом открытии вкладки (могут меняться другой вкладкой/агентом).
import { fetchWithAuth } from './auth.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

// updateSettlementNetto — подпись «Нетто-темп» на лету (oninput):
// (1−k)/СПЖ_лет × 100, знак перевёрнут для показа (минус = рост, спека §3.6).
export function updateSettlementNetto() {
    const label = document.getElementById('settlementNettoLabel');
    const leInput = document.getElementById('settlementLifeExpectancy');
    const kInput = document.getElementById('settlementBirthRate');
    if (!label || !leInput || !kInput) return;
    const years = parseFloat(leInput.value);
    const k = parseFloat(kInput.value);
    if (!isFinite(years) || !isFinite(k) || years <= 0) {
        label.textContent = 'Нетто-темп: —';
        return;
    }
    const netto = (1 - k) / years * 100; // минус = рост
    const abs = Math.abs(netto);
    const text = netto < 0
        ? `Нетто-темп: <b>+${abs.toFixed(1)}%/год (рост)</b>`
        : netto > 0
            ? `Нетто-темп: <b>−${abs.toFixed(1)}%/год (убыль)</b>`
            : 'Нетто-темп: <b>0%/год (равновесие)</b>';
    label.innerHTML = text;
}

// initSettlementSettings — загрузка текущих значений при открытии вкладки
// «Основное» (вызывается из tabs.js на каждую активацию).
export async function initSettlementSettings() {
    try {
        const res = await fetchWithAuth('/admin/settlement-settings');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const s = await res.json();
        const setVal = (id, v) => {
            const el = document.getElementById(id);
            if (el) el.value = v;
        };
        setVal('settlementLifeExpectancy', s.life_expectancy_years);
        setVal('settlementBirthRate', s.birth_rate_coefficient);
        updateSettlementNetto();
    } catch (e) {
        console.error('initSettlementSettings error:', e);
    }
}

// saveSettlementSettings — PATCH /admin/settlement-settings; ошибка валидации
// → тост с текстом с сервера, успех → тост «Сохранено».
export async function saveSettlementSettings() {
    const years = parseFloat(document.getElementById('settlementLifeExpectancy').value);
    const k = parseFloat(document.getElementById('settlementBirthRate').value);
    const body = {};
    if (isFinite(years)) body.life_expectancy_years = years;
    if (isFinite(k)) body.birth_rate_coefficient = k;
    if (Object.keys(body).length === 0) {
        notifyError('Введите корректные значения');
        return;
    }

    try {
        const res = await fetchWithAuth('/admin/settlement-settings', {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сохранить настройки');
            return;
        }
        notifySuccess('Сохранено');
        initSettlementSettings();
    } catch (e) {
        console.error('saveSettlementSettings error:', e);
        notifyError('Ошибка сохранения настроек');
    }
}