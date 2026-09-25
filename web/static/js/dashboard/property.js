// web/static/js/dashboard/property.js
// Вкладка «🏠 Собственность» дашборда (спека
// 2026-09-26-собственность-игрока-в-дашборде §6/§7): список единиц
// собственности игрока — поселения и строения с владельцем-игроком.
//
// Данные — единственный источник GET /me/property (§5.1): {items[]} с полем
// kind (открытый список), серверными подписями name/subtitle, адресом
// (world_id/planet_id/world_name/planet_name) и режимом знания
// knowledge.mode/at/fresh (§4.3). Живых чисел планеты в списке НЕТ (решение
// В2) — они в попапе, который сам решает, что показать по знанию.
//
// «На карте» — переход на карту с авто-открытием попапа (вариант A, §3/§7.2):
// URL строит propertyMapUrl. «🚀 Лететь» (ЧК2, §8) — тем же стыком с &fly=1;
// карта по флагу доводит корабль существующими обёртками полёта. Гейт по
// двигателю — hasEngineFromMe по /me (§8.4); без двигателя кнопка неактивна.
//
// ЧК3 (§9): строка — компактная двухстрочная (стили в dashboard.css, классы
// property-*), статус маршрута в строке берётся из того же /me (flight,
// current_position, pending_destination) чистыми хелперами routeMatchForItem/
// routeStatusForItem — новой сущности нет.
//
// Даты — только через gameDate (общий игровой календарь: UTC, год +1000);
// строки — через escapeHtml (stored XSS из серверных имён). Модуль
// Node-безопасен: DOM/localStorage/fetch трогаются только внутри initProperty
// (тест — web/frontend_property_test.go).

import { gameDate } from '../game_date.js';
import { escapeHtml } from './cargo.js';

// ROUTE_ARRIVED_TEXT — чип-вспышка по прилёте (§9.3): короткая отметка на
// строке-цели, НЕ постоянная метка «ты здесь».
export const ROUTE_ARRIVED_TEXT = '🚀 Прибыли';

// knowledgeBadge — плашка знания строки (§4.3/§6.4): текст по режиму + класс
// стилизации (цвета — dashboard.css). presence — «● Свежие данные» без даты
// (at = null); snapshot — «📷 Данные на <дата>», при fresh=false + « · устарело»;
// scan — «🔍 Данные сканера на <дата>» (+« · устарело» при fresh=false, §9.4);
// none — «Нет данных о планете». Семантика повторяет резолв видимости
// (источник истины — он, не копия).
export function knowledgeBadge(knowledge) {
    const k = knowledge || {};
    if (k.mode === 'presence') {
        return { text: '● Свежие данные', cls: 'property-badge-ok' };
    }
    if (k.mode === 'snapshot') {
        const stale = k.fresh === false;
        return {
            text: '📷 Данные на ' + gameDate(k.at) + (stale ? ' · устарело' : ''),
            cls: stale ? 'property-badge-stale' : 'property-badge-memory',
        };
    }
    if (k.mode === 'scan') {
        const stale = k.fresh === false;
        return {
            text: '🔍 Данные сканера на ' + gameDate(k.at) + (stale ? ' · устарело' : ''),
            cls: stale ? 'property-badge-stale' : 'property-badge-memory',
        };
    }
    return { text: 'Нет данных о планете', cls: 'property-badge-none' };
}

// propertyMapUrl — URL стыка «дашборд → карта» (§7.2): путь «На карте»
// /map?system=..&planet=..&wname=..; opts.fly добавляет &fly=1 (ЧК2, §8).
// Значения — из ответа API, кодируются encodeURIComponent.
export function propertyMapUrl(item, opts) {
    const it = item || {};
    let url = '/map?system=' + encodeURIComponent(it.world_id == null ? '' : it.world_id)
        + '&planet=' + encodeURIComponent(it.planet_id == null ? '' : it.planet_id)
        + '&wname=' + encodeURIComponent(it.world_name == null ? '' : it.world_name);
    if (opts && opts.fly) url += '&fly=1';
    return url;
}

// hasEngineFromMe — есть ли установленный двигатель по ответу /me (то же
// правило, что state.hasEngine карты, map/data.js): валидный предмет в слоте
// engine типа 'engine' из каталога ship_catalog. me == null (/me не загрузился)
// → true: как карта, по умолчанию двигатель есть (сервер всё равно
// провалидирует) — кнопка активна, страница не ломается (§8.4).
export function hasEngineFromMe(me) {
    if (!me) return true;
    const catalog = Array.isArray(me.ship_catalog) ? me.ship_catalog : [];
    const engineId = me.equipment && me.equipment.engine;
    const engineItem = engineId ? catalog.find(i => i.id === engineId) : null;
    return !!(engineItem && engineItem.type === 'engine');
}

// validMs — пригодная метка времени (UnixMilli): конечное число > 0. Битое/
// отсутствующее значение → null (чип «в пути» без времени, §9.3).
function validMs(v) {
    return (typeof v === 'number' && isFinite(v) && v > 0) ? v : null;
}

// routeMatchForItem — сопоставление строки с активным полётом (§9.3): intra —
// current_position.status === 'in_flight', to_type === 'planet',
// to_id === item.planet_id (arriveAt — arrive_at); composite — flight.to ===
// item.world_id, pending_destination.object_type === 'planet' и
// pending_destination.object_id === item.planet_id (arriveAt —
// start_time + duration). Чистый межзвёздный к звезде (без намерения на
// планету) и чужие строки — null. Возвращает {mode, arriveAt} | null;
// arriveAt — UnixMilli или null (битая метка).
export function routeMatchForItem(me, item) {
    const m = me || {};
    const it = item || {};
    const pos = m.current_position || {};
    if (it.planet_id != null && pos.status === 'in_flight' &&
        pos.to_type === 'planet' && pos.to_id === it.planet_id) {
        return { mode: 'intra', arriveAt: validMs(pos.arrive_at) };
    }
    const fl = m.flight;
    const pending = m.pending_destination;
    if (fl && it.world_id != null && it.planet_id != null &&
        fl.to === it.world_id && pending && pending.object_type === 'planet' &&
        pending.object_id === it.planet_id) {
        const start = validMs(fl.start_time);
        const dur = (typeof fl.duration === 'number' && isFinite(fl.duration) && fl.duration >= 0) ? fl.duration : null;
        return { mode: 'composite', arriveAt: (start !== null && dur !== null) ? start + dur * 1000 : null };
    }
    return null;
}

// routeStatusForItem — чип «🚀 В пути к <планета>» для строки (§9.3): время
// прилёта показываем, если метка пригодна (битая/отсутствующая — без времени).
// Нет активного полёта к строке → null (пустого чипа не рисуем).
export function routeStatusForItem(me, item) {
    const match = routeMatchForItem(me, item);
    if (!match) return null;
    const it = item || {};
    let text = '🚀 В пути к ' + ((it.planet_name == null || it.planet_name === '') ? '—' : it.planet_name);
    if (match.arriveAt !== null) {
        const when = gameDate(match.arriveAt);
        if (when !== '—') text += ' · ' + when;
    }
    return { text: text, cls: 'property-route-chip' };
}

// planArrival — чистое решение о ближайшем прилёте (§9.3): {planetId, arriveAt,
// past, key} | null, где key = «planetId:arriveAt» — признак потреблённого
// прилёта. Прилёт в прошлом (arriveAt <= now; рассинхрон/восстановление) при
// уже потреблённом key → null: один раз отработали и повторно не планируем
// (иначе цикл «прибыли → load() → /me» каждые ~250 мс). handled — множество
// таких key. Таймеры/DOM — снаружи, в initProperty.
export function planArrival(items, me, handled, now) {
    if (!Array.isArray(items)) return null;
    const t = (typeof now === 'number') ? now : Date.now();
    const handledSet = (handled && typeof handled.has === 'function') ? handled : new Set();
    let soonest = null;
    for (const it of items) {
        const match = routeMatchForItem(me, it);
        if (!match || match.arriveAt === null) continue;
        if (soonest === null || match.arriveAt < soonest.arriveAt) {
            soonest = { planetId: it.planet_id, arriveAt: match.arriveAt };
        }
    }
    if (soonest === null) return null;
    const key = soonest.planetId + ':' + soonest.arriveAt;
    const past = soonest.arriveAt <= t;
    if (past && handledSet.has(key)) return null;
    return { planetId: soonest.planetId, arriveAt: soonest.arriveAt, past: past, key: key };
}

// propertyRowHtml — строка списка, компактная двухстрочная (§9.2): ярус 1 —
// иконка+имя+тип слева, действия справа; ярус 2 — адрес «world › planet»,
// плашка знания и (если есть) статус маршрута. Иконка: поселение 🏘, строение
// 🏗. Кнопки «На карте» (акцент) и «🚀 Лететь» (тише); без двигателя — неактивный
// span с тултипом (§8.4). opts: {hasEngine, me, arrived}; hasEngine по умолчанию
// true (как с двигателем), arrived — множество planet_id со вспышкой «прибыли».
// Стили — dashboard.css (классы property-*), инлайновых нет.
export function propertyRowHtml(item, opts) {
    const it = item || {};
    const o = opts || {};
    const hasEngine = o.hasEngine !== false;
    const icon = it.kind === 'building' ? '🏗' : '🏘';
    const badge = knowledgeBadge(it.knowledge);
    const address = (it.world_name || '—') + ' › ' + (it.planet_name || '—');
    const flyBtn = hasEngine
        ? '<a class="btn-secondary property-fly-btn" href="' + escapeHtml(propertyMapUrl(it, { fly: true })) + '">🚀 Лететь</a>'
        : '<span class="btn-secondary property-fly-btn is-disabled" title="Двигатель не установлен — полёт невозможен">🚀 Лететь</span>';
    let routeHtml = '';
    if (o.arrived && typeof o.arrived.has === 'function' && o.arrived.has(it.planet_id)) {
        routeHtml = '<span class="property-route-chip property-route-arrived">' + ROUTE_ARRIVED_TEXT + '</span>';
    } else {
        const status = routeStatusForItem(o.me, it);
        if (status) routeHtml = '<span class="' + status.cls + '">' + escapeHtml(status.text) + '</span>';
    }
    return '<div class="property-row">'
        + '<div class="property-row-head">'
        + '<span class="property-row-name"><span class="property-row-icon">' + icon + '</span> ' + escapeHtml(it.name)
        + ' <span class="property-row-subtitle">· ' + escapeHtml(it.subtitle) + '</span></span>'
        + '<div class="property-row-actions">'
        + '<a class="btn-secondary property-view-btn" href="' + escapeHtml(propertyMapUrl(it)) + '">На карте</a>'
        + flyBtn
        + '</div>'
        + '</div>'
        + '<div class="property-row-meta">'
        + '<span class="property-row-address">' + escapeHtml(address) + '</span>'
        + '<span class="property-badge ' + badge.cls + '">' + escapeHtml(badge.text) + '</span>'
        + routeHtml
        + '</div>'
        + '</div>';
}

// propertyItemsHtml — список строк; пусто/не-массив → пустое состояние (§6.3).
// opts прокидывается в строки (гейт двигателя §8.4, статус маршрута §9.3).
export function propertyItemsHtml(items, opts) {
    if (!Array.isArray(items) || items.length === 0) {
        return '<div class="property-empty">У вас пока нет собственности. Здесь появятся ваши поселения и строения.</div>';
    }
    return items.map(it => propertyRowHtml(it, opts)).join('');
}

// initProperty — связывает вкладку с API (§6.2): загрузка при открытии вкладки
// (клик по .tab-btn[data-tab="tab-property"]) и один раз при старте, если вкладка
// активна из localStorage; повтор — по visibilitychange (игрок вернулся в окно).
// Один in-flight запрос: повторный клик не плодит запросы. 401/403 — чистим
// токен и уходим на вход; прочие ошибки — текст в панели, страница жива. Статус
// маршрута пересчитывается на каждой загрузке из того же /me (§9.3): пока полёт
// активен — чип «в пути» (+ время прилёта), по прилёте — вспышка «прибыли» ~4 с.
// DOM — лениво (Node-безопасно).
export function initProperty() {
    const doc = globalThis.document;
    if (!doc) return;
    const pane = doc.getElementById('tab-property');
    if (!pane) return; // блока нет на этой странице
    const tabBtn = doc.querySelector('.tab-btn[data-tab="tab-property"]');

    const token = () => {
        const ls = globalThis.localStorage;
        return ls ? ls.getItem('token') : null;
    };
    const redirectToLogin = () => {
        const ls = globalThis.localStorage;
        if (ls) ls.removeItem('token');
        if (globalThis.location) globalThis.location.href = '/login-page';
    };

    let inFlight = false;
    let meLoading = null;
    // Последняя отрисовка — чтобы вспышка «прибыли» перерисовывалась без запроса.
    let lastItems = null;
    let lastMe = null;
    let lastHasEngine = true;
    // planet_id строк со вспышкой «прибыли» и её таймеры (гаснет ~4 с, §9.3).
    const arrived = new Set();
    const arrivedTimers = new Map();
    // Потреблённые прилёты (key «planetId:arriveAt») — чтобы прилёт в прошлом
    // отработал один раз и не планировался по кругу (шторм load()/me).
    const handledArrivals = new Set();
    let arrivalTimer = null;

    function render() {
        pane.innerHTML = propertyItemsHtml(lastItems, {
            hasEngine: lastHasEngine, me: lastMe, arrived: arrived,
        });
    }

    function clearArrivalTimer() {
        if (arrivalTimer !== null) { clearTimeout(arrivalTimer); arrivalTimer = null; }
    }

    function showArrived(planetId) {
        if (planetId == null) return;
        arrived.add(planetId);
        render();
        const prev = arrivedTimers.get(planetId);
        if (prev) clearTimeout(prev);
        arrivedTimers.set(planetId, setTimeout(() => {
            arrivedTimers.delete(planetId);
            arrived.delete(planetId);
            render();
        }, 4000));
    }

    // scheduleArrival — таймер до ближайшего прилёта (§9.3): по срабатыванию
    // показываем вспышку «прибыли» на строке-цели и перечитываем /me. Прилёт в
    // прошлом (восстановление/рассинхрон) отрабатываем один раз, key запоминаем
    // и повторно не планируем — цикла нет. Битой метки нет — строке статус «в
    // пути» без времени, прилёт таймером не ловим (planArrival → null).
    function scheduleArrival(items, me) {
        clearArrivalTimer();
        const plan = planArrival(items, me, handledArrivals, Date.now());
        if (!plan) return;
        if (plan.past) {
            handledArrivals.add(plan.key);
            showArrived(plan.planetId);
            // load() нельзя звать синхронно: мы внутри его же try (inFlight=true).
            setTimeout(() => { load(); }, 0);
            return;
        }
        const delay = plan.arriveAt - Date.now() + 250;
        arrivalTimer = setTimeout(() => {
            arrivalTimer = null;
            handledArrivals.add(plan.key);
            showArrived(plan.planetId);
            load();
        }, delay);
    }

    // fetchMe — свежий /me на каждую загрузку (§9.3: пересчёт статуса); общий
    // in-flight, чтобы параллельные загрузки не плодили запрос. /me не загрузился
    // (сеть) → null, страница живёт (двигатель считается есть).
    async function fetchMe() {
        const t = token();
        if (!t) return null;
        if (meLoading) return meLoading;
        meLoading = (async () => {
            try {
                const res = await fetch('/me', { headers: { 'Authorization': 'Bearer ' + t } });
                if (res.status === 401 || res.status === 403) { redirectToLogin(); return null; }
                if (!res.ok) return null;
                return await res.json();
            } catch (e) {
                return null;
            } finally {
                meLoading = null;
            }
        })();
        return meLoading;
    }

    async function load() {
        if (inFlight) return;
        const t = token();
        if (!t) return;
        inFlight = true;
        clearArrivalTimer();
        try {
            const me = await fetchMe();
            const hasEngine = hasEngineFromMe(me);
            const res = await fetch('/me/property', { headers: { 'Authorization': 'Bearer ' + t } });
            if (res.status === 401 || res.status === 403) { redirectToLogin(); return; }
            if (!res.ok) throw new Error('Не удалось загрузить собственность. Обновите вкладку.');
            const data = await res.json();
            lastItems = data && data.items;
            lastMe = me;
            lastHasEngine = hasEngine;
            render();
            scheduleArrival(lastItems, me);
        } catch (e) {
            // Страница живёт: в панели — сообщение об ошибке (§6.2).
            pane.innerHTML = '<div class="property-empty">❌ ' + escapeHtml(e.message) + '</div>';
        } finally {
            inFlight = false;
        }
    }

    if (tabBtn) {
        tabBtn.addEventListener('click', () => { load(); });
    }
    const ls = globalThis.localStorage;
    if (ls && ls.getItem('dashboardActiveTab') === 'tab-property') load();
    doc.addEventListener('visibilitychange', () => {
        if (doc.visibilityState === 'visible' && pane.classList.contains('active')) load();
    });
}
