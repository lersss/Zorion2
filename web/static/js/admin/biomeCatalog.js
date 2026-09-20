// web/static/js/admin/biomeCatalog.js
// Справочник биомов (99.2.28 §22, UI-спека 99.2.28-ui): карточка на вкладке
// «Основное» — подразделы Биомы / Типы недр / Типы планет / Полосы климатов /
// Несовместимость / Параметры. Правка — роль admin; чтение — admin +
// skycomposer (read-only: поля disabled, кнопки скрыты, значок 🔒).
// Сохранение — PATCH /admin/biome-catalog (справочник) + PATCH
// /admin/planet-archetypes (полосы): сервер пишет файл атомарно (tmp+rename)
// и делает hot-reload store. Правки применяются к новым мирам (бэкфилл
// запрещён, инвариант 17).
import { fetchWithAuth, getAdminRole } from './auth.js';
import { notifyError, notifySuccess, notifyInfo } from '../ui/toast.js';

// ==================== КОНСТАНТЫ (перечни справочника) ====================

const BANDS = ['э', 'ж', 'у', 'х'];
const BAND_NAMES = { э: 'экстремальный', ж: 'жаркий', у: 'умеренный', х: 'холодный' };

// Признаки атмосферы по составу (приложение §0) — единый источник
// мультивыбора в формах биома/типа недр (UI-спека §3).
const ATMOSPHERE_FEATURES = [
    { id: 'no_toxic', hint: 'нет токсичных газов выше порога' },
    { id: 'toxic', hint: 'любой токсичный газ выше порога' },
    { id: 'acidic', hint: 'SO₂+H₂S+HCl+HF > 0.1%' },
    { id: 'methane', hint: 'CH₄ > 1%' },
    { id: 'nitrogen', hint: 'N₂ > 50%' },
    { id: 'co2', hint: 'CO₂ > 50%' },
    { id: 'oxygen_free', hint: 'O₂ < 1%' },
    { id: 'dense', hint: 'P > 1 атм' },
];

const CATEGORIES = ['литосфера', 'вода', 'биосфера', 'вулканизм', 'крио', 'экзотика'];
const SUBTERRAIN_CATEGORIES = ['породы', 'вулканические', 'осадочные', 'биогенные', 'водные', 'ледяные', 'рудные', 'радиоактивные', 'карстовые'];
const LIQUIDS = ['', 'вода', 'метан', 'аммиак', 'co2'];
const LIQUID_NAMES = { '': '— нет —', 'вода': 'вода', 'метан': 'метан', 'аммиак': 'аммиак', 'co2': 'CO₂' };
const VOLCANISM = ['', 'any', 'hot', 'magma'];
const VOLCANISM_NAMES = { '': '— не зависит —', 'any': 'требует V>0', 'hot': 'V и T≥500 (лава)', 'magma': 'V и T≥1200 (магма)' };
const TYPE_TAGS = ['вулканический', 'биосферный', 'водный', 'ледяной', 'пустынный', 'стеклянный', 'металлический', 'каменный'];
const PREDICATE_TYPES = ['settleable', 'life', 'radioactive_core', 'dominant_form', 'share_of', 'tag_sum', 'temperature', 'water_percent', 'any_of', 'and', 'or'];
const PREDICATE_NAMES = {
    settleable: 'пригодна для людей', life: 'есть жизнь', radioactive_core: 'радиоактивное ядро',
    dominant_form: 'доминирующая форма', share_of: 'доля формы', tag_sum: 'сумма тега',
    temperature: 'температура', water_percent: 'вода %', any_of: 'любая из форм',
    and: 'И (все)', or: 'ИЛИ (любое)',
};

// ==================== СОСТОЯНИЕ ====================

let state = null; // { catalog, archetypes, dirty, dirtyArchetypes, activeSub, selectedId, search, filter, sort, sortDir }
let bound = false;

function isAdmin() { return getAdminRole() === 'admin'; }

// ==================== ИНИЦИАЛИЗАЦИЯ (ленивая, из tabs.js) ====================

// initBiomeCatalog — загрузка справочника + полос при активации вкладки
// «Основное» (ленивая: первый раз — fetch, повторные активации — только
// перерисовка, чтобы не сбрасывать несохранённые правки). force=true —
// принудительная перезагрузка (кнопка «⟳ Повторить»).
export async function initBiomeCatalog(force) {
    if (!bound) {
        bound = true;
        bindCardButtons();
    }
    if (state && !force) {
        renderCard();
        return;
    }
    try {
        const [catRes, archRes] = await Promise.all([
            fetchWithAuth('/admin/biome-catalog'),
            fetchWithAuth('/admin/planet-archetypes'),
        ]);
        if (!catRes.ok) throw new Error('HTTP ' + catRes.status);
        const catalog = await catRes.json();
        const archetypes = (archRes.ok && (await archRes.json())) || { climates: [] };
        state = {
            catalog, archetypes,
            dirty: false, dirtyArchetypes: false,
            activeSub: 'biomes', selectedId: null,
            search: '', filter: '', sort: 'name', sortDir: 'asc',
        };
        renderCard();
    } catch (e) {
        console.error('initBiomeCatalog error:', e);
        const content = document.getElementById('biomeCatalogContent');
        if (content) {
            content.innerHTML = '<p class="hint">Ошибка загрузки справочника.</p>' +
                '<button class="btn secondary" onclick="initBiomeCatalog(true)">⟳ Повторить</button>';
        }
    }
}

// bindCardButtons — обработчики кнопок карточки (Сохранить/Сбросить/Перегенерировать).
function bindCardButtons() {
    const saveBtn = document.getElementById('biomeCatalogSaveBtn');
    if (saveBtn) saveBtn.addEventListener('click', saveBiomeCatalog);
    const resetBtn = document.getElementById('biomeCatalogResetBtn');
    if (resetBtn) resetBtn.addEventListener('click', resetBiomeCatalog);
    const regenBtn = document.getElementById('biomeCatalogRegenBtn');
    if (regenBtn) regenBtn.addEventListener('click', () => {
        const btn = document.querySelector('.tab-btn[data-tab="tab-generation"]');
        if (btn) btn.click();
    });
}

// ==================== РЕНДЕР КАРТОЧКИ ====================

// renderCard — перерисовка активного подраздела + индикатор + права.
function renderCard() {
    if (!state) return;
    const sub = document.getElementById('biomeCatalogSub');
    if (sub) sub.textContent = '· ' + subName(state.activeSub);
    document.querySelectorAll('#biomeCatalogSeg .seg-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.bcSub === state.activeSub);
    });
    const content = document.getElementById('biomeCatalogContent');
    if (!content) return;
    switch (state.activeSub) {
        case 'biomes': renderBiomes(content); break;
        case 'subterrain': renderSubterrain(content); break;
        case 'planetTypes': renderPlanetTypes(content); break;
        case 'bands': renderBands(content); break;
        case 'compat': renderCompatibility(content); break;
        case 'params': renderParams(content); break;
    }
    updateStatus();
    updateRoleUI();
}

function subName(sub) {
    return { biomes: 'Биомы', subterrain: 'Типы недр', planetTypes: 'Типы планет', bands: 'Полосы климатов', compat: 'Несовместимость', params: 'Параметры' }[sub] || sub;
}

// updateRoleUI — read-only для skycomposer: скрыть кнопки, показать 🔒.
function updateRoleUI() {
    const admin = isAdmin();
    const saveBtn = document.getElementById('biomeCatalogSaveBtn');
    const resetBtn = document.getElementById('biomeCatalogResetBtn');
    if (saveBtn) saveBtn.style.display = admin ? '' : 'none';
    if (resetBtn) resetBtn.style.display = admin ? '' : 'none';
    const lock = document.getElementById('biomeCatalogLock');
    if (lock) lock.style.display = admin ? 'none' : '';
}

// updateStatus — индикатор «изменено/сохранено».
function updateStatus() {
    const el = document.getElementById('biomeCatalogStatus');
    if (!el) return;
    const dirty = state.dirty || state.dirtyArchetypes;
    el.textContent = dirty ? '⚠️ изменено' : '✓ сохранено';
    el.style.color = dirty ? '#fbbf24' : '#4ade80';
}

// markDirty — любая правка данных → индикатор «изменено».
function markDirty(which) {
    if (which === 'archetypes') state.dirtyArchetypes = true;
    else state.dirty = true;
    updateStatus();
}

// switchBiomeSub — переключение подраздела (onclick из HTML).
export function switchBiomeSub(sub) {
    if (!state) return;
    state.activeSub = sub;
    state.selectedId = null;
    renderCard();
}

// ==================== ХЕЛПЕРЫ РЕНДЕРА ====================

// el — создание элемента с классом и текстом.
function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
}

// chipSelect — мультивыбор чипами (паттерн .chip из admin.css).
// options: [{id, label}], selected: Set/массив, onChange(новый массив).
function chipSelect(container, options, selected, onChange, disabled) {
    const sel = new Set(selected || []);
    const wrap = el('div', 'chip-row');
    options.forEach(opt => {
        const chip = el('label', 'chip');
        const cb = document.createElement('input');
        cb.type = 'checkbox';
        cb.checked = sel.has(opt.id);
        cb.disabled = !!disabled;
        cb.addEventListener('change', () => {
            if (cb.checked) sel.add(opt.id); else sel.delete(opt.id);
            onChange(Array.from(sel));
        });
        chip.appendChild(cb);
        chip.appendChild(document.createTextNode(opt.label));
        if (opt.hint) chip.title = opt.hint;
        wrap.appendChild(chip);
    });
    container.appendChild(wrap);
}

// numInput — числовое поле с подписью.
function numInput(container, label, value, onChange, opts = {}) {
    const div = el('div');
    const lab = el('label', '', label);
    const input = document.createElement('input');
    input.type = 'number';
    input.value = value ?? '';
    input.step = opts.step || 'any';
    input.min = opts.min ?? '';
    input.max = opts.max ?? '';
    input.disabled = !!opts.disabled;
    input.style.width = opts.width || '80px';
    input.addEventListener('input', () => {
        const v = input.value === '' ? 0 : parseFloat(input.value);
        onChange(isFinite(v) ? v : 0);
    });
    div.appendChild(lab);
    div.appendChild(input);
    container.appendChild(div);
}

// boolInput — чекбокс.
function boolInput(container, label, value, onChange, disabled) {
    const lab = el('label', 'pop-line');
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = !!value;
    cb.disabled = !!disabled;
    cb.addEventListener('change', () => onChange(cb.checked));
    lab.appendChild(cb);
    lab.appendChild(document.createTextNode(label));
    container.appendChild(lab);
}

// selectInput — выпадающий список.
function selectInput(container, label, value, options, onChange, disabled) {
    const div = el('div');
    const lab = el('label', '', label);
    const sel = document.createElement('select');
    options.forEach(o => {
        const opt = document.createElement('option');
        opt.value = o.value;
        opt.textContent = o.label;
        sel.appendChild(opt);
    });
    sel.value = value ?? '';
    sel.disabled = !!disabled;
    sel.addEventListener('change', () => onChange(sel.value));
    div.appendChild(lab);
    div.appendChild(sel);
    container.appendChild(div);
}

// textInput — текстовое поле.
function textInput(container, label, value, onChange, opts = {}) {
    const div = el('div');
    const lab = el('label', '', label);
    const input = document.createElement('input');
    input.type = 'text';
    input.value = value ?? '';
    input.disabled = !!opts.disabled;
    input.style.width = opts.width || '140px';
    input.addEventListener('input', () => onChange(input.value));
    div.appendChild(lab);
    div.appendChild(input);
    container.appendChild(div);
}

// fieldsetBlock — группа полей редактора (UI-спека §6: компактные подблоки).
function fieldsetBlock(title) {
    const fs = document.createElement('fieldset');
    fs.style.cssText = 'border:1px solid #2a2a4a;border-radius:8px;padding:10px 12px;margin:0 0 10px 0;';
    const legend = el('legend', '', title);
    legend.style.cssText = 'color:#94a3b8;font-size:0.8rem;padding:0 6px;';
    fs.appendChild(legend);
    return fs;
}

// ==================== ПОДРАЗДЕЛ: БИОМЫ ====================

function renderBiomes(content) {
    content.innerHTML = '';
    const admin = isAdmin();

    // Панель списка: поиск/фильтр/сортировка + таблица.
    const listPanel = el('div');
    listPanel.style.cssText = 'flex:0 0 42%;min-width:320px;';

    const toolbar = el('div', 'search-form');
    const search = document.createElement('input');
    search.type = 'text';
    search.placeholder = 'Поиск по id/имени...';
    search.value = state.search;
    search.addEventListener('input', () => { state.search = search.value; renderBiomes(content); });
    toolbar.appendChild(search);

    const filter = document.createElement('select');
    filter.innerHTML = '<option value="">Все категории</option>' +
        CATEGORIES.map(c => `<option value="${c}">${c}</option>`).join('');
    filter.value = state.filter;
    filter.addEventListener('change', () => { state.filter = filter.value; renderBiomes(content); });
    toolbar.appendChild(filter);

    const sort = document.createElement('select');
    sort.innerHTML = '<option value="name">по имени</option><option value="weight_base">по весу</option><option value="albedo">по альбедо</option>';
    sort.value = state.sort;
    sort.addEventListener('change', () => { state.sort = sort.value; renderBiomes(content); });
    toolbar.appendChild(sort);

    if (admin) {
        const addBtn = el('button', 'btn', '＋ Новый биом');
        addBtn.addEventListener('click', () => {
            state.selectedId = '__new__';
            renderBiomes(content);
        });
        toolbar.appendChild(addBtn);
    }
    listPanel.appendChild(toolbar);

    const list = filteredBiomes();
    if (list.length === 0) {
        listPanel.appendChild(el('p', 'hint', state.search || state.filter
            ? `Ничего не найдено по «${state.search || state.filter}»`
            : 'Биомов пока нет — добавьте первый'));
    } else {
        const table = document.createElement('table');
        const thead = document.createElement('thead');
        thead.innerHTML = '<tr><th>id</th><th>Имя</th><th>Категория</th><th>Полосы</th><th>w</th></tr>';
        table.appendChild(thead);
        const tbody = document.createElement('tbody');
        list.forEach(b => {
            const row = document.createElement('tr');
            row.style.cursor = 'pointer';
            row.style.background = state.selectedId === b.id ? '#1e3a8a' : '';
            row.addEventListener('click', () => {
                state.selectedId = b.id;
                renderBiomes(content);
            });
            const idCell = el('td', '', b.id);
            idCell.style.fontFamily = 'monospace';
            row.appendChild(idCell);
            row.appendChild(el('td', '', b.name || ''));
            row.appendChild(el('td', '', b.category || ''));
            row.appendChild(el('td', '', (b.bands || []).join('')));
            row.appendChild(el('td', '', String(b.weight_base ?? '')));
            tbody.appendChild(row);
        });
        table.appendChild(tbody);
        listPanel.appendChild(table);
    }

    // Панель редактора.
    const editorPanel = el('div');
    editorPanel.style.cssText = 'flex:1 1 58%;min-width:380px;';
    if (state.selectedId === null) {
        editorPanel.appendChild(el('p', 'hint', 'Выберите биом из списка'));
    } else if (state.selectedId === '__new__') {
        const b = { id: '', name: '', category: 'литосфера', description: '', bands: ['у'], type_tags: [], weight_base: 1, albedo: 0.2 };
        state.catalog.biomes.push(b);
        state.selectedId = '';
        renderBiomeEditor(editorPanel, b, true);
    } else {
        const b = state.catalog.biomes.find(x => x.id === state.selectedId);
        if (b) renderBiomeEditor(editorPanel, b, false);
    }

    const wrap = el('div');
    wrap.style.cssText = 'display:flex;gap:16px;align-items:flex-start;';
    wrap.appendChild(listPanel);
    wrap.appendChild(editorPanel);
    content.appendChild(wrap);
}

// filteredBiomes — поиск/фильтр/сортировка списка биомов.
function filteredBiomes() {
    let list = state.catalog.biomes.slice();
    if (state.search) {
        const q = state.search.toLowerCase();
        list = list.filter(b => (b.id || '').toLowerCase().includes(q) || (b.name || '').toLowerCase().includes(q));
    }
    if (state.filter) list = list.filter(b => b.category === state.filter);
    const dir = state.sortDir === 'desc' ? -1 : 1;
    list.sort((a, b) => {
        const va = a[state.sort] ?? '', vb = b[state.sort] ?? '';
        if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * dir;
        return String(va).localeCompare(String(vb), 'ru') * dir;
    });
    return list;
}

// renderBiomeEditor — полная форма биома (UI-спека §6, поля §22.1).
function renderBiomeEditor(panel, b, isNew) {
    const admin = isAdmin();
    const disabled = !admin;
    const upd = () => markDirty('catalog');

    const head = el('div');
    head.style.cssText = 'display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:8px;';
    head.appendChild(el('strong', '', isNew ? 'Новый биом' : (b.id || '')));
    if (b.category) head.appendChild(el('span', 'chip', b.category));
    panel.appendChild(head);
    panel.appendChild(el('p', 'hint',
        'Изменения применяются к новым мирам; бэкфилл запрещён (99.2.28 §16).'));

    // Основное.
    const main = fieldsetBlock('Основное');
    const row1 = el('div', 'compact-row');
    textInput(row1, 'id (🔒 не редактируется)', b.id, v => { b.id = v; upd(); }, { disabled: !isNew, width: '160px' });
    textInput(row1, 'Отображаемое имя', b.name, v => { b.name = v; upd(); }, { disabled, width: '160px' });
    selectInput(row1, 'Категория', b.category, CATEGORIES.map(c => ({ value: c, label: c })), v => { b.category = v; upd(); }, disabled);
    // Цвет поверхности (спека 2026-09-20 §4.2): необязательный hex #RRGGBB —
    // переопределяет базу категории в картинке планеты. Валидация — на
    // сервере (инвариант 17, PATCH /admin/biome-catalog → 422 при битом hex).
    textInput(row1, 'Цвет (#RRGGBB)', b.color, v => { b.color = v; upd(); }, { disabled, width: '100px' });
    main.appendChild(row1);
    const descRow = el('div');
    const descLab = el('label', '', 'Описание');
    const desc = document.createElement('textarea');
    desc.rows = 2;
    desc.value = b.description || '';
    desc.disabled = disabled;
    desc.style.cssText = 'width:100%;background:#0f172a;border:1px solid #334155;color:#e0e0e0;border-radius:4px;padding:4px 8px;font-size:0.85rem;';
    desc.addEventListener('input', () => { b.description = desc.value; upd(); });
    descRow.appendChild(descLab);
    descRow.appendChild(desc);
    main.appendChild(descRow);
    panel.appendChild(main);

    // Условия появления.
    const cond = fieldsetBlock('Условия появления');
    const row2 = el('div', 'compact-row');
    numInput(row2, 'T_min (K)', b.t_min, v => { b.t_min = v; upd(); }, { disabled });
    numInput(row2, 'T_max (K)', b.t_max, v => { b.t_max = v; upd(); }, { disabled });
    numInput(row2, 'P_min (атм)', b.p_min, v => { b.p_min = v; upd(); }, { disabled });
    numInput(row2, 'P_max (атм)', b.p_max, v => { b.p_max = v; upd(); }, { disabled });
    numInput(row2, 'вода_min (%)', b.water_min, v => { b.water_min = v; upd(); }, { disabled });
    numInput(row2, 'вода_max (%)', b.water_max, v => { b.water_max = v; upd(); }, { disabled });
    numInput(row2, 'iron_min', b.iron_min, v => { b.iron_min = v; upd(); }, { disabled, step: '0.05', min: 0, max: 1 });
    numInput(row2, 'ice_min', b.ice_min, v => { b.ice_min = v; upd(); }, { disabled, step: '0.05', min: 0, max: 1 });
    numInput(row2, 'rock_min', b.rock_min, v => { b.rock_min = v; upd(); }, { disabled, step: '0.05', min: 0, max: 1 });
    numInput(row2, 'g_min', b.g_min, v => { b.g_min = v; upd(); }, { disabled });
    numInput(row2, 'g_max', b.g_max, v => { b.g_max = v; upd(); }, { disabled });
    cond.appendChild(row2);
    panel.appendChild(cond);

    // Признаки и гейты.
    const gates = fieldsetBlock('Признаки и гейты');
    const row3 = el('div', 'compact-row');
    boolInput(row3, 'needs_light (фотосинтез)', b.needs_light !== false, v => { b.needs_light = v; upd(); }, disabled);
    selectInput(row3, 'Жидкость', b.liquid_medium || '', LIQUIDS.map(l => ({ value: l, label: LIQUID_NAMES[l] })), v => { b.liquid_medium = v || ''; upd(); }, disabled);
    selectInput(row3, 'Вулканизм', b.volcanism || '', VOLCANISM.map(v => ({ value: v, label: VOLCANISM_NAMES[v] })), v => { b.volcanism = v || ''; upd(); }, disabled);
    gates.appendChild(row3);
    const row4 = el('div', 'compact-row');
    boolInput(row4, 'requires_radiation', b.requires_radiation, v => { b.requires_radiation = v; upd(); }, disabled);
    boolInput(row4, 'requires_tidal_lock', b.requires_tidal_lock, v => { b.requires_tidal_lock = v; upd(); }, disabled);
    boolInput(row4, 'requires_life', b.requires_life, v => { b.requires_life = v; upd(); }, disabled);
    boolInput(row4, 'requires_liquid_water', b.requires_liquid_water, v => { b.requires_liquid_water = v; upd(); }, disabled);
    gates.appendChild(row4);
    const atmWrap = el('div');
    atmWrap.appendChild(el('label', '', 'Признаки атмосферы (перечень §0, AND)'));
    chipSelect(atmWrap, ATMOSPHERE_FEATURES.map(f => ({ id: f.id, label: f.id, hint: f.hint })),
        b.atmosphere_ok || [], v => { b.atmosphere_ok = v; upd(); }, disabled);
    gates.appendChild(atmWrap);
    const bandsWrap = el('div');
    bandsWrap.appendChild(el('label', '', 'Полосы (whitelist, э/ж/у/х)'));
    chipSelect(bandsWrap, BANDS.map(x => ({ id: x, label: x + ' ' + BAND_NAMES[x] })),
        b.bands || [], v => { b.bands = v; upd(); }, disabled);
    gates.appendChild(bandsWrap);
    const tagsWrap = el('div');
    tagsWrap.appendChild(el('label', '', 'Теги типа планеты'));
    chipSelect(tagsWrap, TYPE_TAGS.map(x => ({ id: x, label: x })),
        b.type_tags || [], v => { b.type_tags = v; upd(); }, disabled);
    gates.appendChild(tagsWrap);
    const row5 = el('div', 'compact-row');
    numInput(row5, 'weight_base (0–10)', b.weight_base, v => { b.weight_base = v; upd(); }, { disabled, step: '0.1', min: 0, max: 10 });
    numInput(row5, 'albedo (0–1)', b.albedo, v => { b.albedo = v; upd(); }, { disabled, step: '0.05', min: 0, max: 1 });
    gates.appendChild(row5);
    panel.appendChild(gates);

    if (admin && !isNew) {
        const delBtn = el('button', 'btn danger', '🗑️ Удалить биом');
        delBtn.style.marginTop = '8px';
        delBtn.addEventListener('click', () => {
            const used = confirm(`Удалить биом «${b.id}»? Он используется планетами — планеты останутся без биома (бэкфилл запрещён).`);
            if (!used) return;
            state.catalog.biomes = state.catalog.biomes.filter(x => x.id !== b.id);
            state.selectedId = null;
            markDirty('catalog');
            renderCard();
        });
        panel.appendChild(delBtn);
    }
}

// ==================== ПОДРАЗДЕЛ: ТИПЫ НЕДР ====================

function renderSubterrain(content) {
    content.innerHTML = '';
    const admin = isAdmin();

    const listPanel = el('div');
    listPanel.style.cssText = 'flex:0 0 42%;min-width:320px;';

    const toolbar = el('div', 'search-form');
    const search = document.createElement('input');
    search.type = 'text';
    search.placeholder = 'Поиск по id/имени...';
    search.value = state.search;
    search.addEventListener('input', () => { state.search = search.value; renderSubterrain(content); });
    toolbar.appendChild(search);
    if (admin) {
        const addBtn = el('button', 'btn', '＋ Новый тип');
        addBtn.addEventListener('click', () => {
            state.selectedId = '__new__';
            renderSubterrain(content);
        });
        toolbar.appendChild(addBtn);
    }
    listPanel.appendChild(toolbar);

    const q = state.search.toLowerCase();
    const list = state.catalog.subterrain_types.filter(s =>
        !q || (s.id || '').toLowerCase().includes(q) || (s.name || '').toLowerCase().includes(q));
    if (list.length === 0) {
        listPanel.appendChild(el('p', 'hint', 'Ничего не найдено'));
    } else {
        const table = document.createElement('table');
        table.innerHTML = '<thead><tr><th>id</th><th>Категория</th><th>Полосы</th><th>w</th></tr></thead>';
        const tbody = document.createElement('tbody');
        list.forEach(s => {
            const row = document.createElement('tr');
            row.style.cursor = 'pointer';
            row.style.background = state.selectedId === s.id ? '#1e3a8a' : '';
            row.addEventListener('click', () => { state.selectedId = s.id; renderSubterrain(content); });
            const idCell = el('td', '', s.id);
            idCell.style.fontFamily = 'monospace';
            row.appendChild(idCell);
            row.appendChild(el('td', '', s.category || ''));
            row.appendChild(el('td', '', (s.bands || []).join('')));
            row.appendChild(el('td', '', String(s.weight_base ?? '')));
            tbody.appendChild(row);
        });
        table.appendChild(tbody);
        listPanel.appendChild(table);
    }

    const editorPanel = el('div');
    editorPanel.style.cssText = 'flex:1 1 58%;min-width:380px;';
    if (state.selectedId === null) {
        editorPanel.appendChild(el('p', 'hint', 'Выберите тип недр из списка'));
    } else if (state.selectedId === '__new__') {
        const s = { id: '', name: '', category: 'породы', bands: ['у'], weight_base: 1, conditions: {} };
        state.catalog.subterrain_types.push(s);
        state.selectedId = '';
        renderSubterrainEditor(editorPanel, s, true);
    } else {
        const s = state.catalog.subterrain_types.find(x => x.id === state.selectedId);
        if (s) renderSubterrainEditor(editorPanel, s, false);
    }

    const wrap = el('div');
    wrap.style.cssText = 'display:flex;gap:16px;align-items:flex-start;';
    wrap.appendChild(listPanel);
    wrap.appendChild(editorPanel);
    content.appendChild(wrap);
}

// renderSubterrainEditor — редактор типа недр (условия — данные, §8).
function renderSubterrainEditor(panel, s, isNew) {
    const admin = isAdmin();
    const disabled = !admin;
    const upd = () => markDirty('catalog');
    const c = s.conditions || (s.conditions = {});

    const head = el('div');
    head.style.cssText = 'display:flex;align-items:center;gap:8px;margin-bottom:8px;';
    head.appendChild(el('strong', '', isNew ? 'Новый тип недр' : (s.id || '')));
    panel.appendChild(head);
    panel.appendChild(el('p', 'hint', 'Условия появления — данные справочника: админка правит то, что реально влияет на генерацию.'));

    const main = fieldsetBlock('Основное');
    const row1 = el('div', 'compact-row');
    textInput(row1, 'id (🔒 не редактируется)', s.id, v => { s.id = v; upd(); }, { disabled: !isNew, width: '160px' });
    textInput(row1, 'Имя', s.name, v => { s.name = v; upd(); }, { disabled, width: '160px' });
    selectInput(row1, 'Категория', s.category, SUBTERRAIN_CATEGORIES.map(x => ({ value: x, label: x })), v => { s.category = v; upd(); }, disabled);
    numInput(row1, 'weight_base', s.weight_base, v => { s.weight_base = v; upd(); }, { disabled, step: '0.1', min: 0 });
    main.appendChild(row1);
    const bandsWrap = el('div');
    bandsWrap.appendChild(el('label', '', 'Полосы (whitelist, э/ж/у/х)'));
    chipSelect(bandsWrap, BANDS.map(x => ({ id: x, label: x + ' ' + BAND_NAMES[x] })),
        s.bands || [], v => { s.bands = v; upd(); }, disabled);
    main.appendChild(bandsWrap);
    panel.appendChild(main);

    const cond = fieldsetBlock('Условия появления (гейты)');
    const row2 = el('div', 'compact-row');
    numInput(row2, 't_max (K)', c.t_max, v => { c.t_max = v; upd(); }, { disabled });
    numInput(row2, 'water_min (%)', c.water_min, v => { c.water_min = v; upd(); }, { disabled });
    numInput(row2, 'v_min', c.v_min, v => { c.v_min = v; upd(); }, { disabled });
    numInput(row2, 'v_max', c.v_max, v => { c.v_max = v; upd(); }, { disabled });
    numInput(row2, 'iron_min', c.iron_min, v => { c.iron_min = v; upd(); }, { disabled, step: '0.05', min: 0, max: 1 });
    numInput(row2, 'ice_min', c.ice_min, v => { c.ice_min = v; upd(); }, { disabled, step: '0.05', min: 0, max: 1 });
    numInput(row2, 'rock_min', c.rock_min, v => { c.rock_min = v; upd(); }, { disabled, step: '0.05', min: 0, max: 1 });
    numInput(row2, 'p_min (атм)', c.p_min, v => { c.p_min = v; upd(); }, { disabled });
    numInput(row2, 'radioactivity_min', c.radioactivity_min, v => { c.radioactivity_min = v; upd(); }, { disabled });
    cond.appendChild(row2);
    const row3 = el('div', 'compact-row');
    boolInput(row3, 'flag_required (жидкая вода)', c.flag_required, v => { c.flag_required = v; upd(); }, disabled);
    boolInput(row3, 'life_required', c.life_required, v => { c.life_required = v; upd(); }, disabled);
    boolInput(row3, 'draft_oceans (океаны в draft)', c.draft_oceans, v => { c.draft_oceans = v; upd(); }, disabled);
    cond.appendChild(row3);
    panel.appendChild(cond);

    if (admin && !isNew) {
        const delBtn = el('button', 'btn danger', '🗑️ Удалить тип');
        delBtn.style.marginTop = '8px';
        delBtn.addEventListener('click', () => {
            if (!confirm(`Удалить тип недр «${s.id}»?`)) return;
            state.catalog.subterrain_types = state.catalog.subterrain_types.filter(x => x.id !== s.id);
            state.selectedId = null;
            markDirty('catalog');
            renderCard();
        });
        panel.appendChild(delBtn);
    }
}

// ==================== ПОДРАЗДЕЛ: ТИПЫ ПЛАНЕТ ====================

function renderPlanetTypes(content) {
    content.innerHTML = '';
    const admin = isAdmin();

    const panel = el('div');
    panel.appendChild(el('p', 'hint',
        'Порядок правил сверху вниз: первое сработавшее решит тип (99.2.28 §11). Перетаскивайте строки (⠿).'));

    const list = el('div');
    list.style.cssText = 'display:flex;flex-direction:column;gap:8px;';
    state.catalog.planet_types.forEach((rule, idx) => {
        const row = el('div');
        row.draggable = admin;
        row.style.cssText = 'display:flex;align-items:center;gap:10px;background:#1a1a2e;border:1px solid #2a2a4a;border-radius:8px;padding:8px 12px;cursor:grab;';
        row.addEventListener('dragstart', e => {
            e.dataTransfer.setData('text/plain', String(idx));
            row.style.opacity = '0.5';
        });
        row.addEventListener('dragend', () => { row.style.opacity = ''; });
        row.addEventListener('dragover', e => e.preventDefault());
        row.addEventListener('drop', e => {
            e.preventDefault();
            const from = parseInt(e.dataTransfer.getData('text/plain'), 10);
            if (from === idx) return;
            const [moved] = state.catalog.planet_types.splice(from, 1);
            state.catalog.planet_types.splice(idx, 0, moved);
            markDirty('catalog');
            renderPlanetTypes(content);
        });

        row.appendChild(el('span', '', '⠿'));
        const idCell = el('strong', '', rule.id);
        idCell.style.fontFamily = 'monospace';
        row.appendChild(idCell);
        const summary = el('span', 'hint', predicateSummary(rule.predicates));
        summary.style.cssText = 'flex:1;margin:0;';
        row.appendChild(summary);
        if (admin) {
            const editBtn = el('button', 'btn-small', '✎');
            editBtn.addEventListener('click', () => {
                state.selectedId = state.selectedId === rule.id ? null : rule.id;
                renderPlanetTypes(content);
            });
            row.appendChild(editBtn);
            const delBtn = el('button', 'btn-small danger', '✕');
            delBtn.addEventListener('click', () => {
                if (!confirm(`Удалить правило «${rule.id}»?`)) return;
                state.catalog.planet_types = state.catalog.planet_types.filter(r => r.id !== rule.id);
                markDirty('catalog');
                renderPlanetTypes(content);
            });
            row.appendChild(delBtn);
        }
        list.appendChild(row);

        // Редактор правила (под строкой).
        if (state.selectedId === rule.id && admin) {
            const editor = el('div');
            editor.style.cssText = 'background:#0f172a;border:1px solid #334155;border-radius:8px;padding:10px 12px;';
            renderRuleEditor(editor, rule);
            list.appendChild(editor);
        }
    });
    panel.appendChild(list);

    // Фолбэк-тип.
    const fbRow = el('div', 'compact-row');
    fbRow.style.marginTop = '12px';
    textInput(fbRow, 'Фолбэк-тип (когда ни одно правило не сработало)', state.catalog.fallback_type,
        v => { state.catalog.fallback_type = v; markDirty('catalog'); }, { width: '200px', disabled: !admin });
    panel.appendChild(fbRow);

    if (admin) {
        const addBtn = el('button', 'btn', '＋ Новое правило');
        addBtn.style.marginTop = '10px';
        addBtn.addEventListener('click', () => {
            state.catalog.planet_types.push({ id: 'новый_тип', predicates: [{ type: 'settleable' }] });
            state.selectedId = 'новый_тип';
            markDirty('catalog');
            renderPlanetTypes(content);
        });
        panel.appendChild(addBtn);
    }

    content.appendChild(panel);
}

// predicateSummary — краткое описание предикатов правила.
function predicateSummary(preds) {
    if (!preds || preds.length === 0) return '—';
    return preds.map(p => {
        switch (p.type) {
            case 'settleable': return 'пригодна';
            case 'life': return 'жизнь';
            case 'radioactive_core': return 'рад. ядро';
            case 'dominant_form': return `доминанта ${p.form}`;
            case 'share_of': return `${p.form} ≥ ${p.min}%`;
            case 'tag_sum': return `тег ${p.tag} ≥ ${p.min}%`;
            case 'temperature': return p.lt ? `T < ${p.lt}` : `T > ${p.gt}`;
            case 'water_percent': return p.lt ? `вода < ${p.lt}` : `вода > ${p.gt}`;
            case 'any_of': return `любая из [${(p.forms || []).join(', ')}] ≥ ${p.min}%`;
            case 'and': return `И(${(p.rules || []).length})`;
            case 'or': return `ИЛИ(${(p.rules || []).length})`;
            default: return p.type;
        }
    }).join(' · ');
}

// renderRuleEditor — редактор правила типа (предикаты, рекурсивно для and/or).
function renderRuleEditor(container, rule) {
    const upd = () => markDirty('catalog');
    const idRow = el('div', 'compact-row');
    textInput(idRow, 'id правила', rule.id, v => { rule.id = v; upd(); }, { width: '160px' });
    container.appendChild(idRow);

    const predsWrap = el('div');
    predsWrap.appendChild(el('label', '', 'Предикаты (все должны выполниться, AND)'));
    renderPredicateList(predsWrap, rule.predicates, upd);
    container.appendChild(predsWrap);

    const addBtn = el('button', 'btn-small', '＋ предикат');
    addBtn.style.marginTop = '6px';
    addBtn.addEventListener('click', () => {
        rule.predicates.push({ type: 'settleable' });
        upd();
        renderRuleEditor(container, rule);
    });
    container.appendChild(addBtn);
}

// renderPredicateList — список предикатов (рекурсия для and/or, глубина ≤ 3).
function renderPredicateList(container, preds, upd, depth = 0) {
    if (depth > 3) return;
    preds.forEach((p, idx) => {
        const row = el('div');
        row.style.cssText = 'display:flex;gap:6px;align-items:center;flex-wrap:wrap;margin:4px 0;padding-left:' + (depth * 14) + 'px;';

        const sel = document.createElement('select');
        PREDICATE_TYPES.forEach(t => {
            const opt = document.createElement('option');
            opt.value = t;
            opt.textContent = PREDICATE_NAMES[t];
            sel.appendChild(opt);
        });
        sel.value = p.type;
        sel.addEventListener('change', () => {
            p.type = sel.value;
            if (sel.value === 'and' || sel.value === 'or') p.rules = p.rules || [{ type: 'settleable' }];
            upd();
            renderPredicateList(container, preds, upd, depth);
        });
        row.appendChild(sel);

        if (p.type === 'dominant_form' || p.type === 'share_of') {
            const formSel = document.createElement('select');
            state.catalog.biomes.forEach(b => {
                const opt = document.createElement('option');
                opt.value = b.id;
                opt.textContent = b.id;
                formSel.appendChild(opt);
            });
            formSel.value = p.form || '';
            formSel.addEventListener('change', () => { p.form = formSel.value; upd(); });
            row.appendChild(formSel);
        }
        if (p.type === 'tag_sum') {
            const tagSel = document.createElement('select');
            TYPE_TAGS.forEach(t => {
                const opt = document.createElement('option');
                opt.value = t;
                opt.textContent = t;
                tagSel.appendChild(opt);
            });
            tagSel.value = p.tag || '';
            tagSel.addEventListener('change', () => { p.tag = tagSel.value; upd(); });
            row.appendChild(tagSel);
        }
        if (p.type === 'any_of') {
            const formsSel = document.createElement('select');
            formsSel.multiple = true;
            formsSel.size = 4;
            state.catalog.biomes.forEach(b => {
                const opt = document.createElement('option');
                opt.value = b.id;
                opt.textContent = b.id;
                formsSel.appendChild(opt);
            });
            (p.forms || []).forEach(f => {
                [...formsSel.options].forEach(o => { if (o.value === f) o.selected = true; });
            });
            formsSel.addEventListener('change', () => {
                p.forms = [...formsSel.selectedOptions].map(o => o.value);
                upd();
            });
            row.appendChild(formsSel);
        }
        if (p.type === 'share_of' || p.type === 'tag_sum' || p.type === 'any_of') {
            const min = document.createElement('input');
            min.type = 'number';
            min.step = 'any';
            min.value = p.min ?? '';
            min.style.width = '70px';
            min.addEventListener('input', () => { p.min = parseFloat(min.value) || 0; upd(); });
            row.appendChild(min);
        }
        if (p.type === 'temperature' || p.type === 'water_percent') {
            const lt = document.createElement('input');
            lt.type = 'number';
            lt.step = 'any';
            lt.placeholder = 'lt';
            lt.value = p.lt ?? '';
            lt.style.width = '70px';
            lt.addEventListener('input', () => { p.lt = parseFloat(lt.value) || 0; upd(); });
            row.appendChild(lt);
            const gt = document.createElement('input');
            gt.type = 'number';
            gt.step = 'any';
            gt.placeholder = 'gt';
            gt.value = p.gt ?? '';
            gt.style.width = '70px';
            gt.addEventListener('input', () => { p.gt = parseFloat(gt.value) || 0; upd(); });
            row.appendChild(gt);
        }
        if (p.type === 'and' || p.type === 'or') {
            const nested = el('div');
            nested.style.cssText = 'flex-basis:100%;';
            renderPredicateList(nested, p.rules || (p.rules = [{ type: 'settleable' }]), upd, depth + 1);
            const addNested = el('button', 'btn-small', '＋');
            addNested.addEventListener('click', () => {
                p.rules.push({ type: 'settleable' });
                upd();
                renderPredicateList(container, preds, upd, depth);
            });
            nested.appendChild(addNested);
            row.appendChild(nested);
        }

        const del = el('button', 'btn-small danger', '✕');
        del.addEventListener('click', () => {
            preds.splice(idx, 1);
            upd();
            renderPredicateList(container, preds, upd, depth);
        });
        row.appendChild(del);
        container.appendChild(row);
    });
}

// ==================== ПОДРАЗДЕЛ: ПОЛОСЫ КЛИМАТОВ ====================

function renderBands(content) {
    content.innerHTML = '';
    const admin = isAdmin();
    const disabled = !admin;
    const upd = () => markDirty('archetypes');

    content.appendChild(el('p', 'hint',
        'Веса base_surface/base_subterrain — для альбедо-прокси слоя 5 и предварительных недр ядра (99.2.28 §15.2). ' +
        'Whitelist биомов здесь НЕ хранится — он в bands биома. Порядок полос — перетаскиванием (⠿).'));

    const list = el('div');
    list.style.cssText = 'display:flex;flex-direction:column;gap:10px;';
    state.archetypes.climates.forEach((c, idx) => {
        const card = el('div');
        card.draggable = admin;
        card.style.cssText = 'background:#1a1a2e;border:1px solid #2a2a4a;border-radius:8px;padding:10px 14px;cursor:grab;';
        card.addEventListener('dragstart', e => {
            e.dataTransfer.setData('text/plain', String(idx));
            card.style.opacity = '0.5';
        });
        card.addEventListener('dragend', () => { card.style.opacity = ''; });
        card.addEventListener('dragover', e => e.preventDefault());
        card.addEventListener('drop', e => {
            e.preventDefault();
            const from = parseInt(e.dataTransfer.getData('text/plain'), 10);
            if (from === idx) return;
            const [moved] = state.archetypes.climates.splice(from, 1);
            state.archetypes.climates.splice(idx, 0, moved);
            upd();
            renderBands(content);
        });

        const head = el('div');
        head.style.cssText = 'display:flex;align-items:center;gap:8px;margin-bottom:6px;';
        head.appendChild(el('span', '', '⠿'));
        head.appendChild(el('strong', '', c.id + ' (' + (c.name || '') + ')'));
        list.appendChild(card);

        const body = el('div');
        body.style.cssText = 'display:flex;gap:16px;flex-wrap:wrap;';
        c.base_surface = c.base_surface || {};
        c.base_subterrain = c.base_subterrain || {};
        body.appendChild(weightMapEditor('base_surface (веса биомов)', c.base_surface, upd, disabled, 'surface'));
        body.appendChild(weightMapEditor('base_subterrain (веса недр)', c.base_subterrain, upd, disabled, 'subterrain'));
        const listsWrap = el('div');
        listsWrap.style.cssText = 'flex:1;min-width:220px;';
        listsWrap.appendChild(allowedListEditor('Гидросферы', c.allowed_hydrospheres, v => { c.allowed_hydrospheres = v; upd(); }, disabled));
        listsWrap.appendChild(allowedListEditor('Атмосферы', c.allowed_atmospheres, v => { c.allowed_atmospheres = v; upd(); }, disabled));
        listsWrap.appendChild(allowedListEditor('Биосферы', c.allowed_biospheres, v => { c.allowed_biospheres = v; upd(); }, disabled));
        body.appendChild(listsWrap);
        card.appendChild(body);
    });
    content.appendChild(list);
}

// weightMapEditor — редактор map «форма → вес» (строки + добавление).
function weightMapEditor(title, map, upd, disabled, kind) {
    const wrap = el('div');
    wrap.style.cssText = 'flex:1;min-width:240px;';
    wrap.appendChild(el('label', '', title));
    const rows = el('div');
    rows.style.cssText = 'display:flex;flex-direction:column;gap:4px;';
    const renderRows = () => {
        rows.innerHTML = '';
        Object.entries(map || {}).forEach(([form, w]) => {
            const row = el('div');
            row.style.cssText = 'display:flex;gap:6px;align-items:center;';
            const formSel = document.createElement('select');
            const options = kind === 'surface' ? state.catalog.biomes : state.catalog.subterrain_types;
            options.forEach(o => {
                const opt = document.createElement('option');
                opt.value = o.id;
                opt.textContent = o.id;
                formSel.appendChild(opt);
            });
            formSel.value = form;
            formSel.disabled = disabled;
            formSel.addEventListener('change', () => {
                const v = map[form];
                delete map[form];
                map[formSel.value] = v;
                upd();
                renderRows();
            });
            row.appendChild(formSel);
            const wIn = document.createElement('input');
            wIn.type = 'number';
            wIn.step = '0.01';
            wIn.min = '0';
            wIn.value = w;
            wIn.disabled = disabled;
            wIn.style.width = '70px';
            wIn.addEventListener('input', () => { map[form] = parseFloat(wIn.value) || 0; upd(); });
            row.appendChild(wIn);
            if (!disabled) {
                const del = el('button', 'btn-small danger', '✕');
                del.addEventListener('click', () => {
                    delete map[form];
                    upd();
                    renderRows();
                });
                row.appendChild(del);
            }
            rows.appendChild(row);
        });
    };
    renderRows();
    wrap.appendChild(rows);
    if (!disabled) {
        const addBtn = el('button', 'btn-small', '＋ вес');
        addBtn.style.marginTop = '4px';
        addBtn.addEventListener('click', () => {
            const options = kind === 'surface' ? state.catalog.biomes : state.catalog.subterrain_types;
            const first = options.find(o => !(o.id in (map || {})));
            if (!first) { notifyInfo('Все формы уже в списке'); return; }
            map[first.id] = 0.1;
            upd();
            renderRows();
        });
        wrap.appendChild(addBtn);
    }
    return wrap;
}

// allowedListEditor — редактор allowed-списка (чипы + добавление своего).
function allowedListEditor(title, list, onChange, disabled) {
    const wrap = el('div');
    wrap.style.cssText = 'margin-bottom:8px;';
    wrap.appendChild(el('label', '', title));
    const chips = el('div', 'chip-row');
    (list || []).forEach(v => {
        const chip = el('span', 'chip');
        chip.textContent = v;
        if (!disabled) {
            chip.style.cursor = 'pointer';
            chip.title = 'Убрать';
            chip.addEventListener('click', () => {
                onChange((list || []).filter(x => x !== v));
            });
        }
        chips.appendChild(chip);
    });
    wrap.appendChild(chips);
    if (!disabled) {
        const addRow = el('div');
        addRow.style.cssText = 'display:flex;gap:6px;margin-top:4px;';
        const input = document.createElement('input');
        input.type = 'text';
        input.placeholder = 'новое значение';
        input.style.cssText = 'background:#0f172a;border:1px solid #334155;color:#e0e0e0;border-radius:4px;padding:2px 8px;font-size:0.8rem;width:140px;';
        const addBtn = el('button', 'btn-small', '＋');
        addBtn.addEventListener('click', () => {
            const v = input.value.trim();
            if (!v) return;
            onChange([...(list || []), v]);
            input.value = '';
        });
        addRow.appendChild(input);
        addRow.appendChild(addBtn);
        wrap.appendChild(addRow);
    }
    return wrap;
}

// ==================== ПОДРАЗДЕЛ: НЕСОВМЕСТИМОСТЬ ====================

function renderCompatibility(content) {
    content.innerHTML = '';
    content.appendChild(el('p', 'hint',
        'Авто-правила от жидкости (99.2.28 §5.6): разные жидкие среды на одной поверхности запрещены — read-only, ' +
        'чтобы не ломать консистентность каскада. Ручные пары — существующая матрица (БД), редактируется через ' +
        'POST /admin/compatibility (в справочник не включена — второй источник истины не создаём).'));

    // Авто-правила: пары биомов с разными liquid_medium из {вода, метан, аммиак, co2}.
    const auto = el('div');
    auto.appendChild(el('strong', '', 'Авто-правила от жидкости (read-only)'));
    const autoPairs = [];
    const byLiquid = {};
    state.catalog.biomes.forEach(b => {
        if (!b.liquid_medium) return;
        (byLiquid[b.liquid_medium] = byLiquid[b.liquid_medium] || []).push(b.id);
    });
    const liquids = Object.keys(byLiquid);
    for (let i = 0; i < liquids.length; i++) {
        for (let j = i + 1; j < liquids.length; j++) {
            byLiquid[liquids[i]].forEach(a => byLiquid[liquids[j]].forEach(b => autoPairs.push([a, b])));
        }
    }
    if (autoPairs.length === 0) {
        auto.appendChild(el('p', 'hint', 'Жидких сред в справочнике нет — авто-правил нет.'));
    } else {
        const table = document.createElement('table');
        table.innerHTML = '<thead><tr><th>Биом A</th><th>Биом B</th></tr></thead>';
        const tbody = document.createElement('tbody');
        autoPairs.forEach(([a, b]) => {
            const row = document.createElement('tr');
            row.appendChild(el('td', '', a));
            row.appendChild(el('td', '', b));
            tbody.appendChild(row);
        });
        table.appendChild(tbody);
        auto.appendChild(table);
    }
    content.appendChild(auto);

    // Ручные пары из существующей матрицы (read-only).
    const manual = el('div');
    manual.style.marginTop = '14px';
    manual.appendChild(el('strong', '', 'Ручные пары матрицы (read-only)'));
    const manualHint = el('p', 'hint', 'Загрузка…');
    manual.appendChild(manualHint);
    content.appendChild(manual);

    fetchWithAuth('/admin/compatibility?category=surface')
        .then(res => res.ok ? res.json() : null)
        .then(data => {
            if (!data) { manualHint.textContent = 'Матрица недоступна.'; return; }
            const pairs = [];
            Object.entries(data.forbid || {}).forEach(([a, bs]) => bs.forEach(b => pairs.push([a, b])));
            if (pairs.length === 0) {
                manualHint.textContent = 'Ручных пар нет.';
                return;
            }
            manualHint.textContent = '';
            const table = document.createElement('table');
            table.innerHTML = '<thead><tr><th>Биом A</th><th>Биом B</th></tr></thead>';
            const tbody = document.createElement('tbody');
            pairs.forEach(([a, b]) => {
                const row = document.createElement('tr');
                row.appendChild(el('td', '', a));
                row.appendChild(el('td', '', b));
                tbody.appendChild(row);
            });
            table.appendChild(tbody);
            manual.appendChild(table);
        })
        .catch(() => { manualHint.textContent = 'Матрица недоступна.'; });
}

// ==================== ПОДРАЗДЕЛ: ПАРАМЕТРЫ ====================

function renderParams(content) {
    content.innerHTML = '';
    const admin = isAdmin();
    const disabled = !admin;
    const upd = () => markDirty('catalog');

    content.appendChild(el('p', 'hint',
        'Пороги токсичности атмосферы по газам (99.2.28 §6.2): любой газ выше порога → признак toxic. ' +
        'CH₄-порог 1% (не 5%) — генерация даёт CH₄ строго <5% (находка @critic №3).'));

    const th = state.catalog.params.toxic_thresholds || (state.catalog.params.toxic_thresholds = {});
    const rows = el('div', 'compact-row');
    Object.entries(th).forEach(([gas, val]) => {
        numInput(rows, gas + ' (%)', val, v => { th[gas] = v; upd(); }, { disabled, step: '0.1', min: 0 });
    });
    content.appendChild(rows);
}

// ==================== СОХРАНЕНИЕ / СБРОС ====================

// saveBiomeCatalog — PATCH справочника (+ полос, если изменены).
export async function saveBiomeCatalog() {
    if (!state) return;
    if (!isAdmin()) { notifyError('Недостаточно прав'); return; }
    if (!state.dirty && !state.dirtyArchetypes) { notifyInfo('Нет изменений'); return; }

    try {
        if (state.dirty) {
            const res = await fetchWithAuth('/admin/biome-catalog', {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    biomes: state.catalog.biomes,
                    subterrain_types: state.catalog.subterrain_types,
                    planet_types: state.catalog.planet_types,
                    fallback_type: state.catalog.fallback_type,
                    params: state.catalog.params,
                }),
            });
            if (!res.ok) {
                const err = await res.json().catch(() => ({}));
                notifyError(err.error || 'Не удалось сохранить справочник');
                return;
            }
            state.dirty = false;
        }
        if (state.dirtyArchetypes) {
            const res = await fetchWithAuth('/admin/planet-archetypes', {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ climates: state.archetypes.climates }),
            });
            if (!res.ok) {
                const err = await res.json().catch(() => ({}));
                notifyError(err.error || 'Не удалось сохранить полосы климатов');
                return;
            }
            state.dirtyArchetypes = false;
        }
        notifySuccess('Сохранено — применяется к новым мирам');
        updateStatus();
    } catch (e) {
        console.error('saveBiomeCatalog error:', e);
        notifyError('Ошибка сохранения справочника');
    }
}

// resetBiomeCatalog — сброс к заводскому сиду (подтверждение).
export async function resetBiomeCatalog() {
    if (!state) return;
    if (!isAdmin()) { notifyError('Недостаточно прав'); return; }
    if (!confirm('Сбросить справочник биомов к заводскому сиду? Все правки будут потеряны.')) return;
    try {
        const res = await fetchWithAuth('/admin/biome-catalog/reset', { method: 'POST' });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сбросить справочник');
            return;
        }
        notifySuccess('Справочник сброшен к заводскому');
        await initBiomeCatalog();
    } catch (e) {
        console.error('resetBiomeCatalog error:', e);
        notifyError('Ошибка сброса справочника');
    }
}