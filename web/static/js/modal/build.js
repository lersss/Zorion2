// web/static/js/modal/build.js
// Панель «Построить структуру» карточки планеты (спека
// 2026-09-24-постройка-структур-на-планете §5.2/§11, ред. 3): админ прямо из
// игры ставит любую структуру из дерева производителей на планету. Данные
// формы — GET /admin/planets/{id}/build-options, поиск владельца-игрока/агента
// — GET /admin/owner-candidates, создание — POST /admin/planets/{id}/structures.
// После успеха карточка обновляется ответом (buildings/settlements), панель
// остаётся открытой, строка результата несёт ФАКТИЧЕСКУЮ ступень (из свежего
// блока settlements[] по created.id — owner-проход может её пересчитать).
//
// Чистые функции без DOM/сети на верхнем уровне: модуль тянется в import-граф
// админки (modal/panel.js → index.js) и исполняется в Node
// (web/frontend_build_test.go).
import { modalState } from './state.js';
import { escapeHtml, safeCssColor } from './contracts.js';

// isAdmin — роль из /me (как tabs.js isAdmin): инструмент только для
// admin/skycomposer (Р1/§11).
function isAdmin() {
    return modalState.role === 'admin' || modalState.role === 'skycomposer';
}

// OWNER_TYPE_LABELS — подписи классов владельца (§11). Пункта «без владельца»
// нет — владелец обязателен (Р13).
const OWNER_TYPE_LABELS = { faction: 'Фракция', player: 'Игрок', agent: 'Агент' };

// buildButtonHtml — кнопка «Построить» в шапке карточки (Р12); пусто не-админу.
export function buildButtonHtml() {
    if (!isAdmin()) return '';
    return `<button id="build-structure-btn" title="Поставить структуру на планету (админ)" style="background:#2a2a4a; border:1px solid #facc15; color:#facc15; padding:6px 14px; border-radius:4px; cursor:pointer; font-size:0.95rem;">⚙ Построить</button>`;
}

// buildPanelContainerHtml — контейнер формы между шапкой и рядом вкладок.
export function buildPanelContainerHtml() {
    if (!isAdmin()) return '';
    return `<div id="build-panel" data-build-panel style="display:none;"></div>`;
}

// typeOptionsHtml — опции селекта типов: группы по class_name, серверный
// порядок сохраняем (клиент не пересортировывает, §11). У live=false —
// честная пометка «производство ещё не реализовано» (§7).
export function typeOptionsHtml(types) {
    let html = '<option value="">— выберите тип —</option>';
    let groupId = null;
    (types || []).forEach(t => {
        if (t.class_id !== groupId) {
            if (groupId !== null) html += '</optgroup>';
            html += `<optgroup label="${escapeHtml(t.class_name || 'Прочее')}">`;
            groupId = t.class_id;
        }
        const suffix = t.live ? '' : ' — производство ещё не реализовано';
        html += `<option value="${escapeHtml(t.id)}">${escapeHtml(t.name)}${suffix}</option>`;
    });
    if (groupId !== null) html += '</optgroup>';
    return html;
}

// ownerListHtml — список кандидатов владельца: строка с цветной точкой (если
// есть color — фракции) и подписью (фракция: «Люди — Корпорация»;
// игрок/агент: «имя — игрок/агент»). Выбранная строка подсвечена.
export function ownerListHtml(items, selectedId) {
    if (!items.length) return '<div style="color:#666; font-size:0.85rem;">Ничего нет</div>';
    return items.map(it => {
        const dot = it.color
            ? `<span style="display:inline-block; width:10px; height:10px; border-radius:50%; background:${safeCssColor(it.color)}; margin-right:6px;"></span>`
            : '';
        const sub = it.subtitle ? ' — ' + escapeHtml(it.subtitle) : '';
        const bg = it.id === selectedId ? 'background:#2a2a4a;' : '';
        return `<div data-owner-pick="${escapeHtml(it.id)}" data-owner-name="${escapeHtml(it.name)}" style="cursor:pointer; padding:4px 6px; border-radius:4px; ${bg}">${dot}${escapeHtml(it.name)}${sub}</div>`;
    }).join('');
}

// ownerSearchMessageHtml — текст-заглушка списка владельца при поиске (§11):
// запрос короче минимума (q ≥ 2) — подсказка; пустой результат — «Ничего не
// найдено»; есть строки — пусто (рисует ownerListHtml). Чистая функция — под
// node-тест.
export function ownerSearchMessageHtml(q, itemCount) {
    if (String(q == null ? '' : q).trim().length < 2) {
        return '<div style="color:#64748b; font-size:0.85rem;">Введите минимум 2 символа</div>';
    }
    if (!itemCount) {
        return '<div style="color:#64748b; font-size:0.85rem;">Ничего не найдено</div>';
    }
    return '';
}

// formHtml — разметка панели формы: тип / владелец (селект класса + список или
// поиск) / население (только для поселения) / хинт расы / кнопка и статус.
export function formHtml(options) {
    const ownerType = (options.default_owner && options.default_owner.owner_type) || 'faction';
    const raceHint = options.default_race
        ? `Раса: ${escapeHtml(options.default_race.race_name || options.default_race.race_id)} (по региону планеты)`
        : 'Раса: регион без расы — люди';
    return `
        <div style="margin:8px 0 0 0; padding:10px; background:#0d0d1a; border:1px solid #2a2a4a; border-radius:8px;">
            <div style="display:flex; justify-content:space-between; align-items:center;">
                <div style="color:#facc15; font-size:0.9rem;">⚙ Построить структуру (админ)</div>
                <button id="build-close-btn" title="Закрыть панель" style="background:none; border:none; color:#888; cursor:pointer; font-size:1rem;">✕</button>
            </div>
            <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-top:8px;">
                <label style="color:#888; font-size:0.85rem;">Тип структуры</label>
                <select id="build-type" style="flex:1; min-width:220px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                    ${typeOptionsHtml(options.types)}
                </select>
            </div>
            <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-top:6px;">
                <label style="color:#888; font-size:0.85rem;">Владелец</label>
                <select id="build-owner-type" style="background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                    ${['faction', 'player', 'agent'].map(k => `<option value="${k}"${k === ownerType ? ' selected' : ''}>${OWNER_TYPE_LABELS[k]}</option>`).join('')}
                </select>
            </div>
            <div id="build-owner-faction" style="max-height:160px; overflow:auto; margin-top:4px;"></div>
            <div id="build-owner-search-wrap" style="display:none; margin-top:4px;">
                <input id="build-owner-search" type="text" placeholder="поиск — минимум 2 символа"
                    style="width:100%; box-sizing:border-box; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <div id="build-owner-results" style="max-height:160px; overflow:auto; margin-top:4px;"></div>
            </div>
            <div id="build-population-wrap" style="display:none; margin-top:6px;">
                <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center;">
                    <label style="color:#888; font-size:0.85rem;">Население</label>
                    <input id="build-population" type="number" min="1" step="1"
                        style="width:140px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                </div>
                <div style="color:#64748b; font-size:0.8rem; margin-top:2px;">Пусто — подставится порог входа ступени (иначе 1000)</div>
            </div>
            <div style="color:#94a3b8; font-size:0.85rem; margin-top:6px;">${raceHint}</div>
            <div style="display:flex; gap:8px; align-items:center; margin-top:8px;">
                <button id="build-submit-btn" style="background:#2a2a4a; border:1px solid #facc15; color:#facc15; padding:6px 14px; border-radius:4px; cursor:pointer;">Построить</button>
                <span id="build-status" style="font-size:0.85rem; color:#94a3b8;"></span>
            </div>
        </div>`;
}

// markInvalid/clearInvalid — подсветка невалидного поля до отправки (§11).
function markInvalid(el) { if (el) el.style.borderColor = '#f87171'; }
function clearInvalid(el) { if (el) el.style.borderColor = '#2a2a4a'; }

// setStatus — строка статуса/ошибок панели; ok=true — зелёный результат.
function setStatus(el, text, ok) {
    if (!el) return;
    el.textContent = text;
    el.style.color = ok ? '#86efac' : '#f87171';
}

// initBuildPanel — включает панель на карточке планеты: кнопка «Построить»
// раскрывает/сворачивает контейнер, форма грузится лениво при первом открытии.
// switchTab(planet-карточки) вызывает panel.js — после успеха переключаем на
// «Поселения»/«Строения» по created.kind (панель остаётся открытой).
export function initBuildPanel(planet, panel, switchTab) {
    if (!isAdmin()) return;
    const btn = panel.querySelector('#build-structure-btn');
    const container = panel.querySelector('[data-build-panel]');
    if (!btn || !container) return;

    const state = {
        loaded: false, loading: false,
        typesById: new Map(), ownerType: 'faction', ownerId: '', ownerName: '',
        searchTimer: null, resultsItems: [],
    };
    const token = () => modalState.authToken || localStorage.getItem('token');

    btn.addEventListener('click', () => {
        if (container.style.display !== 'none') { container.style.display = 'none'; return; }
        container.style.display = 'block';
        if (!state.loaded && !state.loading) loadOptions();
    });

    function loadOptions() {
        state.loading = true;
        container.innerHTML = '<div style="color:#94a3b8; font-size:0.85rem; padding:6px 0;">Загрузка данных формы…</div>';
        fetch('/admin/planets/' + encodeURIComponent(planet.id) + '/build-options', {
            headers: { 'Authorization': 'Bearer ' + token() }
        }).then(res => {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
        }).then(options => {
            state.loading = false;
            state.loaded = true;
            state.typesById = new Map((options.types || []).map(t => [String(t.id), t]));
            const def = options.default_owner;
            state.ownerType = (def && def.owner_type) || 'faction';
            state.ownerId = (def && def.owner_id) || '';
            state.ownerName = (def && def.name) || '';
            renderForm(options);
        }).catch(e => {
            state.loading = false;
            container.innerHTML = `<div style="color:#f87171; font-size:0.85rem; padding:6px 0;">Не удалось загрузить форму: ${escapeHtml(e.message)}</div>`;
        });
    }

    function renderForm(options) {
        container.innerHTML = formHtml(options);
        const typeSel = container.querySelector('#build-type');
        const ownerTypeSel = container.querySelector('#build-owner-type');
        const factionWrap = container.querySelector('#build-owner-faction');
        const searchWrap = container.querySelector('#build-owner-search-wrap');
        const searchInput = container.querySelector('#build-owner-search');
        const resultsWrap = container.querySelector('#build-owner-results');
        const popWrap = container.querySelector('#build-population-wrap');
        const popInput = container.querySelector('#build-population');
        const statusEl = container.querySelector('#build-status');

        const closeBtn = container.querySelector('#build-close-btn');
        if (closeBtn) closeBtn.addEventListener('click', () => { container.style.display = 'none'; });

        // ---- тип структуры: поле населения видно и предзаполнено только для поселения ----
        function applyType() {
            const t = state.typesById.get(typeSel.value);
            const isSettlement = !!t && t.target === 'settlement';
            popWrap.style.display = isSettlement ? 'block' : 'none';
            if (isSettlement) {
                const enter = t.stage && typeof t.stage.enter === 'number' ? t.stage.enter : 0;
                popInput.value = enter >= 1 ? String(Math.trunc(enter)) : String(options.population_fallback || 1000);
            }
        }
        typeSel.addEventListener('change', () => { clearInvalid(typeSel); applyType(); });
        applyType();

        // ---- владелец: класс → список фракций целиком или поиск игрока/агента ----
        const factionItems = () => (options.factions || []).map(f => ({
            id: f.id, name: f.name, subtitle: f.type, color: f.color
        }));
        function renderOwnerList(wrap, items) {
            wrap.innerHTML = ownerListHtml(items, state.ownerId);
            wrap.querySelectorAll('[data-owner-pick]').forEach(row => {
                row.addEventListener('click', () => {
                    state.ownerId = row.dataset.ownerPick;
                    state.ownerName = row.dataset.ownerName;
                    if (state.ownerType === 'faction') renderOwnerList(factionWrap, factionItems());
                    else renderOwnerList(resultsWrap, state.resultsItems);
                });
            });
        }
        function refreshOwnerUi() {
            clearInvalid(factionWrap);
            clearInvalid(searchInput);
            const isFaction = state.ownerType === 'faction';
            factionWrap.style.display = isFaction ? 'block' : 'none';
            searchWrap.style.display = isFaction ? 'none' : 'block';
            if (isFaction) {
                renderOwnerList(factionWrap, factionItems());
            } else {
                resultsWrap.innerHTML = ownerSearchMessageHtml('', 0);
            }
        }
        ownerTypeSel.addEventListener('change', () => {
            state.ownerType = ownerTypeSel.value;
            state.ownerId = '';
            state.ownerName = '';
            state.resultsItems = [];
            if (searchInput) searchInput.value = '';
            refreshOwnerUi();
        });
        refreshOwnerUi();

        // ---- поиск игрока/агента: debounce ~250 мс, минимум 2 символа ----
        async function runOwnerSearch() {
            const q = (searchInput.value || '').trim();
            if (q.length < 2) {
                state.resultsItems = [];
                resultsWrap.innerHTML = ownerSearchMessageHtml(q, 0);
                return;
            }
            try {
                const res = await fetch('/admin/owner-candidates?type=' + encodeURIComponent(state.ownerType) +
                    '&q=' + encodeURIComponent(q), { headers: { 'Authorization': 'Bearer ' + token() } });
                if (!res.ok) throw new Error('HTTP ' + res.status);
                const data = await res.json();
                state.resultsItems = (Array.isArray(data.items) ? data.items : []).map(i => ({
                    id: i.id, name: i.name, subtitle: i.subtitle
                }));
                if (state.resultsItems.length === 0) {
                    resultsWrap.innerHTML = ownerSearchMessageHtml(q, 0);
                    return;
                }
                renderOwnerList(resultsWrap, state.resultsItems);
            } catch (e) {
                resultsWrap.innerHTML = `<div style="color:#f87171; font-size:0.85rem;">Ошибка поиска: ${escapeHtml(e.message)}</div>`;
            }
        }
        if (searchInput) {
            searchInput.addEventListener('input', () => {
                if (state.searchTimer) clearTimeout(state.searchTimer);
                state.searchTimer = setTimeout(runOwnerSearch, 250);
            });
        }

        // ---- отправка: клиентская проверка до запроса, ошибки — текстом ----
        const submitBtn = container.querySelector('#build-submit-btn');
        if (submitBtn) submitBtn.addEventListener('click', submit);

        async function submit() {
            const typeId = Number(typeSel.value);
            if (!typeId) { markInvalid(typeSel); setStatus(statusEl, 'Выберите тип структуры'); return; }
            clearInvalid(typeSel);
            if (!state.ownerId) {
                markInvalid(state.ownerType === 'faction' ? factionWrap : searchInput);
                setStatus(statusEl, 'Выберите владельца');
                return;
            }
            const t = state.typesById.get(String(typeId));
            const body = { producer_type_id: typeId, owner_type: state.ownerType, owner_id: state.ownerId };
            if (t && t.target === 'settlement' && popInput.value !== '') {
                const pop = Number(popInput.value);
                if (!Number.isFinite(pop) || pop < 1) {
                    markInvalid(popInput);
                    setStatus(statusEl, 'Население должно быть больше нуля');
                    return;
                }
                clearInvalid(popInput);
                body.population = Math.trunc(pop);
            }

            setStatus(statusEl, 'Построение…');
            // Блокируем кнопку на время запроса: иначе двойной клик создаст две
            // структуры. Возвращаем после ответа (finally — на всех ветках).
            submitBtn.disabled = true;
            try {
                let res;
                try {
                    res = await fetch('/admin/planets/' + encodeURIComponent(planet.id) + '/structures', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token() },
                        body: JSON.stringify(body)
                    });
                } catch (e) {
                    setStatus(statusEl, 'Ошибка сети: ' + e.message);
                    return;
                }
                if (!res.ok) {
                    if (res.status === 422) setStatus(statusEl, (await res.text()).trim() || 'Не удалось построить');
                    else if (res.status === 409) setStatus(statusEl, 'Идёт обслуживание вселенной — попробуйте позже');
                    else if (res.status === 403) setStatus(statusEl, 'Недостаточно прав');
                    else setStatus(statusEl, 'Ошибка ' + res.status + ': ' + (await res.text()).trim());
                    return;
                }

                const data = await res.json();
                const created = data.created || {};
                planet.buildings = Array.isArray(data.buildings) ? data.buildings : [];
                planet.settlements = Array.isArray(data.settlements) ? data.settlements : [];

                // Фактическая ступень (§5.1/M6): owner-проход мог её пересчитать —
                // берём из свежего блока поселений по created.id, не из created.type_name.
                const fresh = planet.settlements.find(s => s.id === created.id);
                const actualStage = fresh && fresh.type_name ? fresh.type_name : created.type_name;
                let line = 'Построено: ' + (actualStage || '—') + ' · владелец ' + (created.owner_name || state.ownerName || '—');
                if (fresh && fresh.type_name && created.type_name && fresh.type_name !== created.type_name) {
                    line += ` — ступень пересчитана по населению (запрошен был ${created.type_name})`;
                }
                setStatus(statusEl, line, true);
                if (typeof switchTab === 'function') {
                    switchTab(created.kind === 'settlement' ? 'settlements' : 'structures');
                }
            } finally {
                submitBtn.disabled = false;
            }
        }
    }
}
