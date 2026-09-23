// web/static/js/dashboard/money.js
// Деньги игрока в интерфейсе (идея 2026-09-23 «Деньги на счёте в шапке и
// история» §5.1–§5.4): строка «Счёт: … Cr» в шапке дашборда и карты + вкладка
// «💳 Счёт» с балансом и историей операций.
//
// Данные — единственный источник GET /me/money (§3.2/О-д4): {balance,
// withdrawable, operations[]}. Показываем ТОЛЬКО свой счёт (owner_id из JWT,
// канон 14_money.md §14.4); корзину «заработанное» (withdrawable) не выносим.
//
// moneyLabel — единственная точка форматирования подписи «… Cr» для обеих
// шапок (дашборд и карта импортируют её оттуда же; второй форматтер не
// заводим). moneyDate — обёртка над gameDate (общий игровой календарь: UTC,
// год +1000) — второго форматирования дат нет.
//
// Модуль Node-безопасен: DOM/localStorage/fetch трогаются только внутри
// initMoney (тест — web/frontend_money_test.go).

import { gameDate } from '../game_date.js';
import { escapeHtml } from './cargo.js';

// MONEY_KIND_LABELS — человекочитаемые подписи типов операций (§5.2).
// kind — открытый список: неизвестное значение даёт «Операция по счёту».
const MONEY_KIND_LABELS = {
    admin_seed: 'Стартовый капитал',
    escrow_lock: 'Залог по контракту',
    escrow_release: 'Выплата по контракту',
    escrow_return: 'Возврат залога',
    contract_work_earn: 'Заработок по подряду',
};

// moneyAmount — целое с разделителем тысяч (ru-RU, неразрывный пробел);
// нечисло → «—» (битые данные не рисуем как NaN, приём из cargoNum).
function moneyAmount(n) {
    if (typeof n !== 'number' || !isFinite(n)) return '—';
    return Number(n).toLocaleString('ru-RU');
}

// moneyLabel — подпись баланса «10 000 Cr» (сумма + « Cr» через пробел).
// Баланс 0 показываем как «0 Cr», не прочерк (§5.3).
export function moneyLabel(n) {
    const amount = moneyAmount(n);
    return amount === '—' ? '—' : amount + ' Cr';
}

// moneySign — сумма операции со знаком: `delta > 0` → «+10 000 Cr»,
// `delta < 0` → «−300 Cr» (типографский минус U+2212). Нечисло → «—».
export function moneySign(delta) {
    if (typeof delta !== 'number' || !isFinite(delta)) return '—';
    const sign = delta < 0 ? '\u2212' : '+';
    return sign + Math.abs(delta).toLocaleString('ru-RU') + ' Cr';
}

// moneyKindLabel — понятная подпись типа операции; неизвестный/пустой kind
// не ломает UI (§5.2, открытый список).
export function moneyKindLabel(kind) {
    return MONEY_KIND_LABELS[kind] || 'Операция по счёту';
}

// moneyDate — дата операции по общему игровому календарю (импорт gameDate;
// UTC, год +1000, формат «23.09.3026, 14:05»).
export function moneyDate(iso) {
    return gameDate(iso);
}

// moneyRowHtml — строка истории: слева тип + дата, справа сумма со знаком.
// Сумма окрашивается gain/loss (§5.2).
export function moneyRowHtml(op) {
    const kind = escapeHtml(moneyKindLabel(op && op.kind));
    const date = escapeHtml(moneyDate(op && op.occurred_at));
    const delta = op && op.delta;
    const sign = moneySign(delta);
    const cls = (typeof delta === 'number' && delta < 0) ? 'money-loss' : 'money-gain';
    return '<div class="money-row">'
        + '<div class="money-row-info">'
        + '<span class="money-row-kind">' + kind + '</span>'
        + '<span class="money-row-meta">' + date + '</span>'
        + '</div>'
        + '<span class="money-row-amount ' + cls + '">' + sign + '</span>'
        + '</div>';
}

// moneyItemsHtml — список строк; пусто/не-массив → пустое состояние (§5.3).
export function moneyItemsHtml(ops) {
    if (!Array.isArray(ops) || ops.length === 0) {
        return '<div class="money-empty">Операций пока нет.</div>';
    }
    return ops.map(moneyRowHtml).join('');
}

// initMoney — связывает шапку и карточку «Счёт» с API: один GET /me/money,
// баланс в шапку и в сводку, история в список. 401/403 — чистим токен и
// уходим на вход; прочие ошибки не ломают страницу (§5.3). DOM — лениво
// (Node-безопасно).
export function initMoney() {
    const doc = globalThis.document;
    if (!doc) return;
    const headerEl = doc.getElementById('accountBalance');
    const summaryEl = doc.getElementById('money-balance');
    const historyEl = doc.getElementById('money-history');
    const statusEl = doc.getElementById('money-status');
    if (!headerEl && !summaryEl && !historyEl) return; // блока нет на этой странице

    const token = () => {
        const ls = globalThis.localStorage;
        return ls ? ls.getItem('token') : null;
    };
    const setStatus = (text) => { if (statusEl) statusEl.textContent = text; };
    const redirectToLogin = () => {
        const ls = globalThis.localStorage;
        if (ls) ls.removeItem('token');
        if (globalThis.location) globalThis.location.href = '/login-page';
    };

    async function load() {
        const t = token();
        if (!t) return;
        try {
            const res = await fetch('/me/money', { headers: { 'Authorization': 'Bearer ' + t } });
            if (res.status === 401 || res.status === 403) { redirectToLogin(); return; }
            if (!res.ok) throw new Error('Не удалось загрузить счёт');
            const data = await res.json();
            const label = moneyLabel(data && data.balance);
            if (headerEl) headerEl.textContent = label;
            if (summaryEl) summaryEl.textContent = label;
            if (historyEl) historyEl.innerHTML = moneyItemsHtml(data && data.operations);
            setStatus('');
        } catch (e) {
            // Страница живёт: шапка остаётся «—», в карточке — сообщение (§5.3).
            setStatus('❌ Не удалось загрузить счёт');
        }
    }

    load();
}
