// web/static/js/modal/market.js
// Вкладка «Магазин» карточки планеты (спека
// 2026-09-24-магазин-модулей-локальный-рынок §10): витрина локального рынка
// (раздел «Модули») + выбор универсального слота с трейд-ином + покупка
// (POST /api/planets/{id}/market/buy). Витрина читается лениво при открытии
// вкладки (GET /api/planets/{id}/market, §7.1); текущее оборудование, каталог
// и число слотов — из /me (паттерн раздела «Корабль», web/index.html).
//
// Чистые функции (marketHtml и помощники) без DOM/сети на верхнем уровне:
// модуль тянется в import-граф админки (modal/tabs.js) и исполняется в Node
// (web/frontend_test.go, TestAdminFrontendLoadsInNode) — document/fetch/window
// только внутри функций.
import { modalState } from './state.js';
import { escapeHtml } from './contracts.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

// MODULE_TYPE_LABELS — подписи типов модуля: единственное место, где ключ
// (equipment.type) превращается в человекочитаемое имя. Неизвестный — как есть.
const MODULE_TYPE_LABELS = {
    cargo: 'грузовой',
    engine: 'двигатель',
    radar: 'радар',
    scanner: 'сканер'
};

// moduleTypeLabel — подпись типа модуля по ключу.
export function moduleTypeLabel(type) {
    if (!type) return '—';
    return MODULE_TYPE_LABELS[type] || type;
}

// moduleParamsText — параметры модуля читаемо (§10): радиус/ёмкость/скорость/
// глубина. Зеркало slotDetail раздела «Корабль» (web/index.html) — тот же
// формат; числа только с сервера (params предмета), не хардкод.
export function moduleParamsText(item) {
    const p = (item && item.params) || {};
    switch (item && item.type) {
        case 'radar':
            return 'Радиус: ' + (typeof p.radius === 'number' ? p.radius : '—') + ' px';
        case 'scanner': {
            const depth = p.depth === 'surface' ? 'поверхность' : (p.depth || '—');
            return 'Глубина: ' + depth + (p.settlements ? ' · Поселения: да' : '');
        }
        case 'engine':
            return 'Скорость: ' + (typeof p.speed_factor === 'number' ? p.speed_factor : '—') + ' сек/px';
        case 'cargo':
            return 'Ёмкость: +' + (typeof p.capacity === 'number' ? p.capacity : '—') + ' т';
        default:
            return '';
    }
}

// universalSlotKeys — ключи универсальных слотов по числу из формы модели
// (конвенция трюма §20.2: universal / universal2 / universal3). Число — не
// хардкод: count из ship_model.slots.universal.
export function universalSlotKeys(count) {
    const n = Number(count) > 0 ? Math.floor(Number(count)) : 0;
    const keys = [];
    for (let i = 1; i <= n; i++) keys.push(i === 1 ? 'universal' : 'universal' + i);
    return keys;
}

// offerTradein — выкуп старого модуля слота: floor(цена_старого / 2), цена
// старого — из витрины по item.id (§7.2 п.5). Модуля нет в витрине → 0
// (§18 п.4). oldItemId пуст (пустой слот) → 0.
export function offerTradein(oldItemId, offers) {
    if (!oldItemId) return 0;
    const list = Array.isArray(offers) ? offers : [];
    const o = list.find(x => x && x.item && x.item.id === oldItemId);
    if (!o || typeof o.price !== 'number') return 0;
    return Math.floor(o.price / 2);
}

// finalPrice — итоговая цена покупки: max(0, price − tradein) (§7.2 п.5;
// выкуп не даёт денег сверх цены — И-М3).
export function finalPrice(price, tradein) {
    const p = typeof price === 'number' ? price : 0;
    const t = typeof tradein === 'number' ? tradein : 0;
    return Math.max(0, p - t);
}

// universalSlotCount — число универсальных слотов из /me
// (ship_model.slots.universal; для starter — 3).
function universalSlotCount(me) {
    const slots = me && me.ship_model && me.ship_model.slots;
    const n = slots ? Number(slots.universal) : 0;
    return Number.isFinite(n) && n > 0 ? n : 0;
}

// catalogById — индекс каталога /me.ship_catalog по id.
function catalogById(me) {
    const map = {};
    const list = me && Array.isArray(me.ship_catalog) ? me.ship_catalog : [];
    list.forEach(i => { if (i && i.id) map[i.id] = i; });
    return map;
}

// slotCellHtml — ячейка выбора универсального слота (§10): занятая показывает
// модуль и выкуп «выкуп старого: −1 500 Cr», пустая — «без выкупа». Выбранная
// подсвечена. Клик вешает bindMarket (data-market-slot).
function slotCellHtml(key, me, offers, selected) {
    const equipment = (me && me.equipment) || {};
    const oldId = equipment[key];
    const item = oldId ? catalogById(me)[oldId] : null;
    const tradein = offerTradein(oldId, offers);
    const body = item
        ? `<div style="font-weight:600;">${escapeHtml(item.name || oldId)}</div>
            <div style="color:#94a3b8; font-size:0.8rem;">${escapeHtml(moduleTypeLabel(item.type))}</div>
            <div style="color:#fde68a; font-size:0.85rem; margin-top:2px;">выкуп старого: −${tradein} Cr</div>`
        : `<div style="color:#64748b;">пусто</div>
            <div style="color:#64748b; font-size:0.85rem; margin-top:2px;">без выкупа</div>`;
    return `<div data-market-slot="${escapeHtml(key)}" style="flex:1; min-width:120px; padding:8px 10px; border:1px solid ${selected ? '#4a9eff' : '#2a2a4a'}; background:${selected ? '#232345' : '#1a1a2e'}; border-radius:6px; cursor:pointer;">
        <div style="color:#888; font-size:0.8rem; text-transform:uppercase;">${escapeHtml(key)}</div>
        ${body}
    </div>`;
}

// offerCardHtml — карточка предложения (§10): имя, тип, параметры, цена и
// итог с учётом выкупа выбранного слота; кнопка «Купить» (data-market-buy).
// При can_trade == false кнопка неактивна (гейт присутствия, §10).
function offerCardHtml(offer, me, offers, selectedSlot, canTrade) {
    const item = offer.item || {};
    const oldId = selectedSlot ? ((me && me.equipment) || {})[selectedSlot] : null;
    const tradein = offerTradein(oldId, offers);
    const total = finalPrice(offer.price, tradein);
    const disabled = canTrade ? '' : ' disabled';
    const disabledStyle = canTrade ? 'cursor:pointer;' : 'cursor:not-allowed; opacity:0.4;';
    const tradeinLine = tradein > 0
        ? ` <span style="color:#94a3b8; font-size:0.85rem;">− выкуп ${tradein} Cr</span>`
        : '';
    return `
        <div style="margin:6px 0; padding:10px; background:#1a1a2e; border-radius:4px;">
            <div style="display:flex; justify-content:space-between; align-items:flex-start; gap:8px;">
                <div>
                    <div><strong>${escapeHtml(item.name || ('#' + offer.id))}</strong> <span style="color:#888; font-size:0.85rem;">${escapeHtml(moduleTypeLabel(item.type))}</span></div>
                    <div style="color:#94a3b8; font-size:0.85rem; margin-top:4px;">${escapeHtml(moduleParamsText(item))}</div>
                    <div style="color:#fde68a; margin-top:4px;">${offer.price} Cr${tradeinLine}</div>
                    <div style="color:#e0e0e0; margin-top:2px;">Итого: <strong>${total} Cr</strong></div>
                </div>
                <button data-market-buy="${escapeHtml(String(offer.id))}"${disabled} style="background:#2a2a4a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; white-space:nowrap; ${disabledStyle}">Купить</button>
            </div>
        </div>`;
}

// marketHtml — содержимое вкладки (§10): раздел «Модули» (карточки предложений)
// + выбор слота + подпись гейта. Числа — с сервера (offer.price, item.params);
// баланс в витрине не дублируется (он в шапке). Пусто (offers: []) —
// «На планете нет рынка».
export function marketHtml(market, me, selectedSlot) {
    const offers = (market && Array.isArray(market.offers)) ? market.offers : [];
    const canTrade = !!(market && market.can_trade === true);
    if (offers.length === 0) {
        return `<p style="color:#666; text-align:center; padding:20px 0;">На планете нет рынка</p>`;
    }
    const keys = universalSlotKeys(universalSlotCount(me));
    const sel = (selectedSlot && keys.indexOf(selectedSlot) !== -1) ? selectedSlot : (keys[0] || null);

    let html = `<p style="color:#888; font-size:0.9rem; text-transform:uppercase;">Модули</p>`;
    offers.forEach(o => { html += offerCardHtml(o, me, offers, sel, canTrade); });

    if (keys.length > 0) {
        html += `<p style="color:#888; font-size:0.9rem; text-transform:uppercase; margin-top:12px;">Слот установки</p>`;
        html += `<div style="display:flex; gap:8px; flex-wrap:wrap;">`;
        keys.forEach(k => { html += slotCellHtml(k, me, offers, k === sel); });
        html += `</div>`;
    }

    if (!canTrade) {
        html += `<p style="color:#fbbf24; font-size:0.85rem; margin-top:10px;">Магазин доступен только с орбиты планеты</p>`;
    }
    return html;
}

// renderMarket — контейнер вкладки до загрузки (паттерн доски контрактов):
// initMarket подгружает витрину и заменяет содержимое.
export function renderMarket(planet) {
    return `<div data-market><p style="color:#666; text-align:center; padding:12px 0;">Загрузка…</p></div>`;
}

// authToken — токен игрока (паттерн modal/*.js: токен модалки, фолбэк localStorage).
function authToken() {
    return modalState.authToken || localStorage.getItem('token');
}

// errorMessage — текст ошибки сервера: writeJSONError отдаёт {"error": "..."}
// (auth_handlers.go), показываем человеческую строку, а не сырой JSON.
function errorMessage(text, status) {
    try {
        const j = JSON.parse(text);
        if (j && j.error) return j.error;
    } catch (e) { /* не JSON — отдаём как есть */ }
    return text || ('Ошибка ' + status);
}

// loadMarket — GET /api/planets/{id}/market (§7.1). Возвращает {status, market}:
// status нужен, чтобы отличить 404 «Нет данных» (режим none) от успеха.
async function loadMarket(planetID) {
    const res = await fetch('/api/planets/' + encodeURIComponent(planetID) + '/market', {
        headers: { 'Authorization': 'Bearer ' + authToken() }
    });
    if (!res.ok) return { status: res.status, market: null };
    return { status: res.status, market: await res.json() };
}

// loadMe — оборудование/каталог/слоты игрока (паттерн раздела «Корабль»):
// ship_model.slots + equipment + ship_catalog.
async function loadMe() {
    const res = await fetch('/me', { headers: { 'Authorization': 'Bearer ' + authToken() } });
    if (!res.ok) return null;
    return await res.json();
}

// buyOffer — POST /api/planets/{id}/market/buy (§7.2): тело {offer_id, slot}.
// Лок кнопки на время запроса (двойной клик не создаёт вторую покупку, §10).
// Успех — локальное обновление ячейки слота (инвентаря нет: ключ
// перезаписывается, §7.2 п.7) и перерисовка витрины; ошибки — тостом.
async function buyOffer(container, planet, market, me, slot, offerID, btn) {
    if (!btn || btn.disabled) return;
    btn.disabled = true;
    btn.style.opacity = '0.5';
    btn.style.cursor = 'wait';
    try {
        const res = await fetch('/api/planets/' + encodeURIComponent(planet.id) + '/market/buy', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + authToken() },
            body: JSON.stringify({ offer_id: Number(offerID), slot: slot })
        });
        const text = await res.text();
        if (!res.ok) {
            notifyError(errorMessage(text, res.status));
            btn.disabled = false;
            btn.style.opacity = '';
            btn.style.cursor = 'pointer';
            return;
        }
        let data = {};
        try { data = JSON.parse(text); } catch (e) { data = {}; }
        if (me) {
            if (!me.equipment) me.equipment = {};
            me.equipment[slot] = data.item_id || null;
        }
        const bought = data.item_id ? ': ' + data.item_id : '';
        const paid = (typeof data.amount === 'number') ? ' за ' + data.amount + ' Cr' : '';
        notifySuccess('Куплено' + bought + paid);
        if (container.isConnected && modalState.activeTab === 'market') {
            renderInto(container, planet, market, me, slot);
        }
    } catch (e) {
        notifyError('Не удалось купить: ' + e.message);
        btn.disabled = false;
        btn.style.opacity = '';
        btn.style.cursor = 'pointer';
    }
}

// bindMarket — обработчики после вставки: выбор слота (пересчёт выкупа и
// итога) и кнопки «Купить».
function bindMarket(container, planet, market, me, selectedSlot) {
    const keys = universalSlotKeys(universalSlotCount(me));
    const sel = (selectedSlot && keys.indexOf(selectedSlot) !== -1) ? selectedSlot : (keys[0] || null);

    container.querySelectorAll('[data-market-slot]').forEach(cell => {
        cell.addEventListener('click', () => {
            renderInto(container, planet, market, me, cell.dataset.marketSlot);
        });
    });

    container.querySelectorAll('[data-market-buy]').forEach(btn => {
        btn.addEventListener('click', () => {
            buyOffer(container, planet, market, me, sel, btn.dataset.marketBuy, btn);
        });
    });
}

// renderInto — перерисовка содержимого витрины с текущим выбором слота.
function renderInto(container, planet, market, me, selectedSlot) {
    container.innerHTML = marketHtml(market, me, selectedSlot);
    bindMarket(container, planet, market, me, selectedSlot);
}

// initMarket — ленивая загрузка вкладки (вызывается после вставки в DOM):
// витрина + /me, затем рендер. 404 (режим none, §7.1) — заглушка «Нет данных»
// (вкладка не должна была появиться — одна ветка видимости с роутом, §10).
export async function initMarket(planet, container) {
    let result;
    try {
        result = await loadMarket(planet.id);
    } catch (e) {
        notifyError('Не удалось загрузить магазин: ' + e.message);
        if (container.isConnected) {
            container.innerHTML = `<p style="color:#666; text-align:center; padding:20px 0;">Не удалось загрузить магазин</p>`;
        }
        return;
    }
    if (!container.isConnected) return;
    if (result.status === 404) {
        container.innerHTML = `<p style="color:#666; text-align:center; padding:20px 0;">Нет данных — купить отчёт</p>`;
        return;
    }
    if (!result.market) {
        container.innerHTML = `<p style="color:#666; text-align:center; padding:20px 0;">Не удалось загрузить магазин</p>`;
        return;
    }
    let me = null;
    try { me = await loadMe(); } catch (e) { me = null; }
    if (!container.isConnected) return;
    renderInto(container, planet, result.market, me, null);
}
