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
// «Посмотреть» — переход на карту с авто-открытием попапа (вариант A, §3/§7.2):
// URL строит propertyMapUrl. «Перелететь» (ЧК2, §8) — тем же стыком с &fly=1;
// карта по флагу доводит корабль существующими обёртками полёта. Гейт по
// двигателю — hasEngineFromMe по /me (§8.4); без двигателя кнопка неактивна.
//
// Даты — только через gameDate (общий игровой календарь: UTC, год +1000);
// строки — через escapeHtml (stored XSS из серверных имён). Модуль
// Node-безопасен: DOM/localStorage/fetch трогаются только внутри initProperty
// (тест — web/frontend_property_test.go).

import { gameDate } from '../game_date.js';
import { escapeHtml } from './cargo.js';

// BADGE_STYLE — инлайновые стили плашки знания: index.html не растёт
// (решение §6.1 — только 2 строки разметки), точные пиксели — @uidesigner.
const BADGE_STYLE = {
    'property-badge-ok': 'background:rgba(74,222,128,0.12); border:1px solid rgba(74,222,128,0.4); color:#4ade80;',
    'property-badge-memory': 'background:rgba(148,163,184,0.10); border:1px solid rgba(148,163,184,0.35); color:#94a3b8;',
    'property-badge-stale': 'background:rgba(251,191,36,0.12); border:1px solid rgba(251,191,36,0.4); color:#fbbf24;',
    'property-badge-none': 'background:rgba(148,163,184,0.08); border:1px dashed rgba(148,163,184,0.3); color:#94a3b8;',
};

// knowledgeBadge — плашка знания строки (§4.3/§6.4): текст по режиму + класс
// стилизации. presence — «● Свежие данные» без даты (at = null); snapshot —
// «📷 Данные на <дата>», при fresh=false + « · устарело»; scan — «🔍 Данные
// сканера на <дата>»; none — «Нет данных». Семантика повторяет резолв
// видимости (источник истины — он, не копия).
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
        return { text: '🔍 Данные сканера на ' + gameDate(k.at), cls: 'property-badge-memory' };
    }
    return { text: 'Нет данных', cls: 'property-badge-none' };
}

// propertyMapUrl — URL стыка «дашборд → карта» (§7.2): путь «Посмотреть»
// /map?system=..&planet=..&wname=..; opts.fly добавляет &fly=1 (задел ЧК2, §8).
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
// → true: как карта, по умолчанию двигатель есть (сервер всё равно провалидирует)
// — кнопка активна, страница не ломается (§8.4).
export function hasEngineFromMe(me) {
    if (!me) return true;
    const catalog = Array.isArray(me.ship_catalog) ? me.ship_catalog : [];
    const engineId = me.equipment && me.equipment.engine;
    const engineItem = engineId ? catalog.find(i => i.id === engineId) : null;
    return !!(engineItem && engineItem.type === 'engine');
}

// propertyRowHtml — строка списка (§6.4): иконка по kind (🏠 поселение /
// 🏗 строение), имя, подпись (subtitle), адрес «world › planet», плашка знания,
// «Посмотреть» (ЧК1) и «Перелететь» (ЧК2, §8). Кнопка «Перелететь» — стык с
// &fly=1; без двигателя — неактивный span с тултипом (§8.4). opts необязателен:
// по умолчанию кнопка активна (как с двигателем).
export function propertyRowHtml(item, opts) {
    const it = item || {};
    const hasEngine = !(opts && opts.hasEngine === false);
    const icon = it.kind === 'building' ? '🏗' : '🏠';
    const badge = knowledgeBadge(it.knowledge);
    const address = (it.world_name || '—') + ' › ' + (it.planet_name || '—');
    const flyBtn = hasEngine
        ? '<a class="btn-secondary property-fly-btn" style="white-space:nowrap;" href="' + escapeHtml(propertyMapUrl(it, { fly: true })) + '">Перелететь</a>'
        : '<span class="btn-secondary property-fly-btn is-disabled" style="white-space:nowrap; opacity:0.5; cursor:not-allowed;" title="Двигатель не установлен — полёт невозможен">Перелететь</span>';
    return '<div class="property-row" style="display:flex; justify-content:space-between; align-items:center; gap:12px; padding:8px 0; border-bottom:1px solid #1f2a3a;">'
        + '<div class="property-row-info" style="display:flex; flex-direction:column; gap:3px; min-width:0;">'
        + '<span class="property-row-name" style="font-weight:600;">' + icon + ' ' + escapeHtml(it.name)
        + ' <span class="property-row-subtitle" style="color:#94a3b8; font-weight:400; font-size:0.8rem;">' + escapeHtml(it.subtitle) + '</span></span>'
        + '<span class="property-row-address" style="color:#94a3b8; font-size:0.8rem;">' + escapeHtml(address) + '</span>'
        + '<span class="property-badge ' + badge.cls + '" style="align-self:flex-start; padding:2px 8px; border-radius:8px; font-size:0.78rem; ' + BADGE_STYLE[badge.cls] + '">' + escapeHtml(badge.text) + '</span>'
        + '</div>'
        + '<div class="property-row-actions" style="display:flex; align-items:center; gap:8px; white-space:nowrap;">'
        + '<a class="btn-secondary property-view-btn" style="white-space:nowrap;" href="' + escapeHtml(propertyMapUrl(it)) + '">Посмотреть</a>'
        + flyBtn
        + '</div>'
        + '</div>';
}

// propertyItemsHtml — список строк; пусто/не-массив → пустое состояние (§6.3).
// opts прокидывается в строки (гейт двигателя, §8.4).
export function propertyItemsHtml(items, opts) {
    if (!Array.isArray(items) || items.length === 0) {
        return '<div class="property-empty" style="color:#94a3b8; padding:6px 0;">У вас пока нет собственности. Здесь появятся ваши поселения и строения.</div>';
    }
    return items.map(it => propertyRowHtml(it, opts)).join('');
}

// initProperty — связывает вкладку с API (§6.2): загрузка при открытии вкладки
// (клик по .tab-btn[data-tab="tab-property"]) и один раз при старте, если вкладка
// активна из localStorage; повтор — по visibilitychange (игрок вернулся в окно).
// Один in-flight запрос: повторный клик не плодит запросы. 401/403 — чистим
// токен и уходим на вход; прочие ошибки — текст в панели, страница жива. DOM —
// лениво (Node-безопасно).
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
    // Признак двигателя из /me (§8.4): один раз за жизнь вкладки, повторные
    // загрузки (клик/visibilitychange) его не перезапрашивают (кэш). null — ещё
    // не узнали; engineLoading — общий in-flight, чтобы параллельные загрузки не
    // плодили /me.
    let engineFlag = null;
    let engineLoading = null;

    async function loadEngine() {
        if (engineFlag !== null) return engineFlag;
        if (engineLoading) return engineLoading;
        const t = token();
        if (!t) return true;
        engineLoading = (async () => {
            try {
                const res = await fetch('/me', { headers: { 'Authorization': 'Bearer ' + t } });
                if (res.status === 401 || res.status === 403) { redirectToLogin(); return true; }
                if (!res.ok) return true; // /me не загрузился — считаем двигатель есть
                engineFlag = hasEngineFromMe(await res.json());
                return engineFlag;
            } catch (e) {
                return true;
            } finally {
                engineLoading = null;
            }
        })();
        return engineLoading;
    }

    async function load() {
        if (inFlight) return;
        const t = token();
        if (!t) return;
        inFlight = true;
        try {
            const hasEngine = await loadEngine();
            const res = await fetch('/me/property', { headers: { 'Authorization': 'Bearer ' + t } });
            if (res.status === 401 || res.status === 403) { redirectToLogin(); return; }
            if (!res.ok) throw new Error('Не удалось загрузить собственность');
            const data = await res.json();
            pane.innerHTML = propertyItemsHtml(data && data.items, { hasEngine: hasEngine });
        } catch (e) {
            // Страница живёт: в панели — сообщение об ошибке (§6.2).
            pane.innerHTML = '<div class="property-empty" style="color:#94a3b8; padding:6px 0;">❌ ' + escapeHtml(e.message) + '</div>';
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
