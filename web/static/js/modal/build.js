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

// getBuildState — состояние панели «Построить» по id планеты. Живёт в
// modalState.buildForms и переживает перерисовку карточки (автообновление,
// «Обновить», /me): открыта/закрыта, загруженные опции (без повторного
// запроса), выбранный тип/владелец/население, результаты поиска, статус,
// подсветка невалидных полей и скролл списков владельца.
function getBuildState(planetId) {
    if (!modalState.buildForms) modalState.buildForms = {};
    let st = modalState.buildForms[planetId];
    if (!st) {
        st = {
            open: false,
            loading: false,
            options: null,
            ownerType: 'faction',
            ownerId: '',
            ownerName: '',
            searchQuery: '',
            searchTimer: null,
            resultsItems: [],
            typeId: '',
            population: '',
            statusText: '',
            statusColor: '',
            invalid: {},
            scrollFaction: 0,
            scrollResults: 0,
        };
        modalState.buildForms[planetId] = st;
    }
    return st;
}

// loadingHtml — заглушка «Загрузка данных формы…» в контейнере панели.
function loadingHtml() {
    return '<div style="color:#94a3b8; font-size:0.85rem; padding:6px 0;">Загрузка данных формы…</div>';
}

// initBuildPanel — включает панель на карточке планеты: кнопка «Построить»
// раскрывает/сворачивает контейнер, форма грузится лениво при первом открытии.
// switchTab(planet-карточки) вызывает panel.js — после успеха переключаем на
// «Поселения»/«Строения» по created.kind (панель остаётся открытой).
// Состояние формы — в modalState.buildForms (getBuildState): перерисовка панели
// (баг 2026-09-25) не сбрасывает форму и не перезапрашивает опции.
export function initBuildPanel(planet, panel, switchTab) {
    if (!isAdmin()) return;
    const btn = panel.querySelector('#build-structure-btn');
    const container = panel.querySelector('[data-build-panel]');
    if (!btn || !container) return;

    const st = getBuildState(planet.id);
    const token = () => modalState.authToken || localStorage.getItem('token');

    // Восстановление после перерисовки: открытая панель остаётся раскрытой,
    // форма — из сохранённых опций (без повторного запроса).
    container.style.display = st.open ? 'block' : 'none';
    if (st.open && st.options) renderForm(st.options);
    else if (st.open && st.loading) container.innerHTML = loadingHtml();

    btn.addEventListener('click', () => {
        if (container.style.display !== 'none') {
            container.style.display = 'none';
            st.open = false;
            return;
        }
        container.style.display = 'block';
        st.open = true;
        if (!st.options && !st.loading) loadOptions();
    });

    function loadOptions() {
        st.loading = true;
        container.innerHTML = loadingHtml();
        fetch('/admin/planets/' + encodeURIComponent(planet.id) + '/build-options', {
            headers: { 'Authorization': 'Bearer ' + token() }
        }).then(res => {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
        }).then(options => {
            st.loading = false;
            if (!options || typeof options !== 'object') throw new Error('пустой ответ');
            st.options = options;
            // Префилл владельца — только если игрок ещё не выбирал (память пуста).
            if (!st.ownerId && !st.ownerName) {
                const def = options.default_owner;
                st.ownerType = (def && def.owner_type) || 'faction';
                st.ownerId = (def && def.owner_id) || '';
                st.ownerName = (def && def.name) || '';
            }
            // Гвард карточки-сироты (ср. tabs.js:1038, market.js:295): если
            // панель перерисовали, пока грузились опции, в отсоединённый
            // контейнер не пишем — кэш опций сохранён, форма отрисуется при
            // следующей инициализации панели (авто/ручное обновление).
            if (!container.isConnected) return;
            renderForm(options);
        }).catch(e => {
            st.loading = false;
            if (!container.isConnected) return;
            container.innerHTML = `<div style="color:#f87171; font-size:0.85rem; padding:6px 0;">Не удалось загрузить форму: ${escapeHtml(e.message)}</div>`;
        });
    }

    function renderForm(options) {
        const typesById = new Map((options.types || []).map(t => [String(t.id), t]));
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

        // Восстановление сохранённого выбора до навешивания обработчиков.
        if (typeSel) typeSel.value = st.typeId || '';
        if (ownerTypeSel) ownerTypeSel.value = st.ownerType || 'faction';
        if (searchInput) searchInput.value = st.searchQuery || '';

        const closeBtn = container.querySelector('#build-close-btn');
        if (closeBtn) closeBtn.addEventListener('click', () => {
            container.style.display = 'none';
            st.open = false;
        });

        // ---- тип структуры: поле населения видно и предзаполнено только для поселения ----
        function applyType(keepPopulation) {
            const t = typesById.get(typeSel.value);
            const isSettlement = !!t && t.target === 'settlement';
            popWrap.style.display = isSettlement ? 'block' : 'none';
            if (isSettlement) {
                if (keepPopulation && st.population !== '') {
                    popInput.value = st.population;
                } else {
                    const enter = t.stage && typeof t.stage.enter === 'number' ? t.stage.enter : 0;
                    popInput.value = enter >= 1 ? String(Math.trunc(enter)) : String(options.population_fallback || 1000);
                    st.population = popInput.value;
                }
            }
        }
        typeSel.addEventListener('change', () => {
            clearInvalid(typeSel);
            st.invalid.type = false;
            st.typeId = typeSel.value;
            st.population = ''; // смена типа — население по умолчанию нового типа
            applyType(false);
        });
        if (popInput) popInput.addEventListener('input', () => {
            st.population = popInput.value;
            clearInvalid(popInput);
            st.invalid.population = false;
        });
        applyType(true);

        // ---- владелец: класс → список фракций целиком или поиск игрока/агента ----
        const factionItems = () => (options.factions || []).map(f => ({
            id: f.id, name: f.name, subtitle: f.type, color: f.color
        }));
        function renderOwnerList(wrap, items) {
            wrap.innerHTML = ownerListHtml(items, st.ownerId);
            wrap.querySelectorAll('[data-owner-pick]').forEach(row => {
                row.addEventListener('click', () => {
                    st.ownerId = row.dataset.ownerPick;
                    st.ownerName = row.dataset.ownerName;
                    st.invalid.owner = false;
                    if (st.ownerType === 'faction') renderOwnerList(factionWrap, factionItems());
                    else renderOwnerList(resultsWrap, st.resultsItems);
                });
            });
        }
        function renderResults() {
            if (st.resultsItems && st.resultsItems.length) renderOwnerList(resultsWrap, st.resultsItems);
            else resultsWrap.innerHTML = ownerSearchMessageHtml(st.searchQuery, 0);
        }
        function refreshOwnerUi() {
            clearInvalid(factionWrap);
            clearInvalid(searchInput);
            const isFaction = st.ownerType === 'faction';
            factionWrap.style.display = isFaction ? 'block' : 'none';
            searchWrap.style.display = isFaction ? 'none' : 'block';
            if (isFaction) renderOwnerList(factionWrap, factionItems());
            else renderResults();
        }
        ownerTypeSel.addEventListener('change', () => {
            st.ownerType = ownerTypeSel.value;
            st.ownerId = '';
            st.ownerName = '';
            st.resultsItems = [];
            st.searchQuery = '';
            st.invalid.owner = false;
            if (searchInput) searchInput.value = '';
            refreshOwnerUi();
        });
        refreshOwnerUi();
        // Скролл списков владельца переживает перерисовку: пишем позицию на
        // прокрутке и возвращаем после восстановления списка.
        if (factionWrap) {
            factionWrap.addEventListener('scroll', () => { st.scrollFaction = factionWrap.scrollTop; });
            factionWrap.scrollTop = st.scrollFaction || 0;
        }
        if (resultsWrap) {
            resultsWrap.addEventListener('scroll', () => { st.scrollResults = resultsWrap.scrollTop; });
            resultsWrap.scrollTop = st.scrollResults || 0;
        }

        // ---- поиск игрока/агента: debounce ~250 мс, минимум 2 символа ----
        async function runOwnerSearch() {
            const q = (searchInput.value || '').trim();
            if (q.length < 2) {
                st.resultsItems = [];
                resultsWrap.innerHTML = ownerSearchMessageHtml(q, 0);
                return;
            }
            try {
                const res = await fetch('/admin/owner-candidates?type=' + encodeURIComponent(st.ownerType) +
                    '&q=' + encodeURIComponent(q), { headers: { 'Authorization': 'Bearer ' + token() } });
                if (!res.ok) throw new Error('HTTP ' + res.status);
                const data = await res.json();
                st.resultsItems = (Array.isArray(data.items) ? data.items : []).map(i => ({
                    id: i.id, name: i.name, subtitle: i.subtitle
                }));
                if (!resultsWrap.isConnected) return;
                if (st.resultsItems.length === 0) {
                    resultsWrap.innerHTML = ownerSearchMessageHtml(q, 0);
                    return;
                }
                renderOwnerList(resultsWrap, st.resultsItems);
            } catch (e) {
                if (!resultsWrap.isConnected) return;
                resultsWrap.innerHTML = `<div style="color:#f87171; font-size:0.85rem;">Ошибка поиска: ${escapeHtml(e.message)}</div>`;
            }
        }
        if (searchInput) {
            searchInput.addEventListener('input', () => {
                st.searchQuery = searchInput.value || '';
                if (st.searchTimer) clearTimeout(st.searchTimer);
                st.searchTimer = setTimeout(runOwnerSearch, 250);
            });
        }

        // ---- отправка: клиентская проверка до запроса, ошибки — текстом ----
        // setStatusSt — статус в DOM + память (переживает перерисовку).
        function setStatusSt(text, ok) {
            setStatus(statusEl, text, ok);
            st.statusText = text;
            st.statusColor = ok ? '#86efac' : '#f87171';
        }
        const submitBtn = container.querySelector('#build-submit-btn');
        if (submitBtn) submitBtn.addEventListener('click', submit);

        async function submit() {
            const typeId = Number(typeSel.value);
            if (!typeId) { markInvalid(typeSel); st.invalid.type = true; setStatusSt('Выберите тип структуры'); return; }
            clearInvalid(typeSel); st.invalid.type = false;
            if (!st.ownerId) {
                markInvalid(st.ownerType === 'faction' ? factionWrap : searchInput);
                st.invalid.owner = true;
                setStatusSt('Выберите владельца');
                return;
            }
            clearInvalid(st.ownerType === 'faction' ? factionWrap : searchInput);
            st.invalid.owner = false;
            const t = typesById.get(String(typeId));
            const body = { producer_type_id: typeId, owner_type: st.ownerType, owner_id: st.ownerId };
            if (t && t.target === 'settlement' && popInput.value !== '') {
                const pop = Number(popInput.value);
                if (!Number.isFinite(pop) || pop < 1) {
                    markInvalid(popInput);
                    st.invalid.population = true;
                    setStatusSt('Население должно быть больше нуля');
                    return;
                }
                clearInvalid(popInput);
                st.invalid.population = false;
                body.population = Math.trunc(pop);
            }

            setStatusSt('Построение…');
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
                    setStatusSt('Ошибка сети: ' + e.message);
                    return;
                }
                if (!res.ok) {
                    if (res.status === 422) setStatusSt((await res.text()).trim() || 'Не удалось построить');
                    else if (res.status === 409) setStatusSt('Идёт обслуживание вселенной — попробуйте позже');
                    else if (res.status === 403) setStatusSt('Недостаточно прав');
                    else setStatusSt('Ошибка ' + res.status + ': ' + (await res.text()).trim());
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
                let line = 'Построено: ' + (actualStage || '—') + ' · владелец ' + (created.owner_name || st.ownerName || '—');
                if (fresh && fresh.type_name && created.type_name && fresh.type_name !== created.type_name) {
                    line += ` — ступень пересчитана по населению (запрошен был ${created.type_name})`;
                }
                setStatusSt(line, true);
                if (typeof switchTab === 'function') {
                    switchTab(created.kind === 'settlement' ? 'settlements' : 'structures');
                }
            } finally {
                submitBtn.disabled = false;
            }
        }

        // ---- восстановление статуса и подсветки невалидных полей ----
        if (st.statusText) {
            statusEl.textContent = st.statusText;
            statusEl.style.color = st.statusColor || '#94a3b8';
        }
        if (st.invalid.type) markInvalid(typeSel);
        if (st.invalid.owner) markInvalid(st.ownerType === 'faction' ? factionWrap : searchInput);
        if (st.invalid.population) markInvalid(popInput);
    }
}
