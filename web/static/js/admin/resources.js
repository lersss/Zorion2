// web/static/js/admin/resources.js
// Вкладка «Ресурсы» (спека 94a, read-only): каталог универсального слоя
// (20 ресурсов) + покрытие рас (для каждой оси с весом ≥ 10 — ресурсы,
// попадающие в окно расы). Данные — GET /admin/resources; фильтры каталога
// (категория/тип/поиск) и покрытия (раса/ось) работают вместе (И).
import { fetchWithAuth } from './auth.js';
import {
    AXIS_KEYS, drawRadar, renderBars, renderMatrix, renderThermo,
    renderLabels, renderStrip, stripLegend, k2c, shortLabel,
} from './realResources.js';

// Читаемые названия 10 осей свойств (09_resources §9.1.2).
const AXIS_NAMES = {
    hardness: 'твёрдость',
    elasticity: 'эластичность',
    conductivity: 'проводимость',
    density: 'плотность',
    energy_density: 'энергоёмкость',
    biocompatibility: 'биосовместимость',
    radioactivity: 'радиоактивность',
    toxicity: 'токсичность',
    flammability: 'горючесть',
    chemical_activity: 'химическая активность',
};

// Читаемые названия consumption-осей (хемотипов, спека 94a §3.1).
const CHEMOTYPE_NAMES = {
    'ВОД': 'вода',
    'АММ': 'аммиак',
    'МЕТ': 'метан',
    'CO2': 'углекислота',
    'СЕР': 'сера',
    'РАС': 'рассолы',
    'СКФ': 'сверхкритические флюиды',
    'КРЕ': 'кремний',
    'ОРГ': 'органика',
    'ГРД': 'градиенты',
    'ВГЕ': 'водород',
    'ПЫЛ': 'пыль',
    'ИЗЛ': 'излучение',
};

// Читаемые названия 6 категорий (categories.go).
const CATEGORY_NAMES = {
    water: 'Вода/жидкости',
    mineral: 'Минералы',
    gas: 'Газы',
    fuel: 'Топливо/редокс',
    organic: 'Органика',
    rare: 'Редкие',
};

let resourcesBound = false;
let resourcesData = null;   // кэш ответа /admin/resources
let resourceNames = {};     // id ресурса → имя (для колонки «Ресурсы» в покрытии)
let resourceRaces = {};     // id ресурса → [имя расы] (кто потребляет)

// Состояние подвкладки «Реальные вещества» (идея 2026-09-18 §4б, этап 2).
let realMode = 'single';    // 'single' | 'all' | 'compare' | 'list'
let realSelectedId = null;  // выбранное вещество (режимы «Один ресурс»/«Все форматы»)
let realFormat = '1';       // выбранный формат (1–6)
let realWindow = 'goods';   // окна: 'off' | 'goods' (демо) | 'races' (хемотипы)
let realWindowPick = 'all'; // конкретное окно для показа: 'all' | id окна
let realSortAxis = 'name';  // столбец сортировки списка: 'name' | ось | 'phase'
let realSortDir = 'asc';    // 'asc' | 'desc'
let realCompareA = null;    // первое вещество сравнения
let realCompareB = null;    // второе вещество сравнения

// initResources — ленивая инициализация при активации вкладки (как initBalancer).
export function initResources() {
    if (resourcesBound) return;
    resourcesBound = true;
    bindFilters();
    bindRealFilters();
    switchResSubTab();
    loadResources();
}

// ---------- ПОДВКЛАДКИ РЕСУРСОВ (идея 2026-09-18 §4б) ----------

// switchResSubTab — переключение подвкладки «Базовый слой» / «Реальные
// вещества» (кнопки, как в генерации 71a; класс res-subtab-btn — свой,
// чтобы не конфликтовать с gen-subtab-btn). Без аргумента — восстановление
// сохранённой из localStorage.
export function switchResSubTab(name) {
    const target = name || localStorage.getItem('adminResSubTab') || 'base';
    document.querySelectorAll('#tab-resources .res-sub').forEach(d => {
        d.style.display = d.id === 'resSub-' + target ? 'block' : 'none';
    });
    document.querySelectorAll('#tab-resources .res-subtab-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.resSub === target);
    });
    localStorage.setItem('adminResSubTab', target);
}

// bindRealFilters — обработчики подвкладки «Реальные вещества».
function bindRealFilters() {
    const bind = (id, evt, fn) => {
        const el = document.getElementById(id);
        if (el) el.addEventListener(evt, fn);
    };
    bind('realFamilyFilter', 'change', () => { renderRealList(); saveRealState(); });
    bind('realFormatSelect', 'change', () => {
        realFormat = document.getElementById('realFormatSelect').value;
        renderRealDetail();
        saveRealState();
    });
    bind('realWindowSel', 'change', () => {
        realWindow = document.getElementById('realWindowSel').value;
        fillRealWindowPick();
        syncWindowSelects();
        renderRealDetail();
        saveRealState();
    });
    bind('realAllWindowSel', 'change', () => {
        realWindow = document.getElementById('realAllWindowSel').value;
        fillRealWindowPick();
        syncWindowSelects();
        renderRealAllFormats();
        saveRealState();
    });
    bind('realCompareA', 'change', () => {
        realCompareA = document.getElementById('realCompareA').value;
        renderRealCompare();
        saveRealState();
    });
    bind('realCompareB', 'change', () => {
        realCompareB = document.getElementById('realCompareB').value;
        renderRealCompare();
        saveRealState();
    });
    bind('realCompareFormat', 'change', () => { renderRealCompare(); saveRealState(); });
    bind('realCompareWindowSel', 'change', () => {
        realWindow = document.getElementById('realCompareWindowSel').value;
        fillRealWindowPick();
        syncWindowSelects();
        renderRealCompare();
        saveRealState();
    });
    bind('realListFamily', 'change', () => { renderRealStripList(); saveRealState(); });
    bind('realListWindowSel', 'change', () => {
        realWindow = document.getElementById('realListWindowSel').value;
        fillRealWindowPick();
        syncWindowSelects();
        renderRealStripList();
        saveRealState();
    });
    bind('realAxisTop', 'change', () => { renderRealTop5(); saveRealState(); });
    bind('realWindowPickSel', 'change', () => {
        realWindowPick = document.getElementById('realWindowPickSel').value;
        saveRealState();
        renderRealMode();
    });
}

// syncWindowSelects — синхронизация селектов окон всех режимов с realWindow.
function syncWindowSelects() {
    ['realWindowSel', 'realAllWindowSel', 'realCompareWindowSel', 'realListWindowSel'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.value = realWindow;
    });
}

// ---------- Состояние подвкладки в localStorage (F5 сохраняет положение) ----------

const REAL_STATE_KEY = 'adminRealState';

// saveRealState — сохранить состояние подвкладки «Реальные вещества»
// (режим, формат, окна, семейство, выбранное вещество, сравнение, топ-5).
function saveRealState() {
    const st = {
        mode: realMode,
        format: realFormat,
        compareFormat: document.getElementById('realCompareFormat')?.value || realFormat,
        window: realWindow,
        windowPick: realWindowPick,
        family: document.getElementById('realFamilyFilter')?.value || '',
        selected: realSelectedId,
        compareA: realCompareA,
        compareB: realCompareB,
        listFamily: document.getElementById('realListFamily')?.value || '',
        axisTop: document.getElementById('realAxisTop')?.value || '',
        sortAxis: realSortAxis,
        sortDir: realSortDir,
    };
    try { localStorage.setItem(REAL_STATE_KEY, JSON.stringify(st)); } catch (e) { /* переполнение — не критично */ }
}

// restoreRealState — применить сохранённое состояние (вызывается после
// заполнения селектов, перед рендером режима). Валидирует id веществ.
function restoreRealState() {
    let st = null;
    try { st = JSON.parse(localStorage.getItem(REAL_STATE_KEY) || 'null'); } catch (e) { st = null; }
    if (!st) return;
    if (st.mode && ['single', 'all', 'compare', 'list'].includes(st.mode)) realMode = st.mode;
    if (st.format) realFormat = st.format;
    if (st.window && ['off', 'goods', 'races'].includes(st.window)) realWindow = st.window;
    if (st.windowPick && ['all', ...(activeWindows() || []).map(w => w.id)].includes(st.windowPick)) realWindowPick = st.windowPick;
    if (st.sortAxis && ['name', 'phase', ...AXIS_KEYS].includes(st.sortAxis)) realSortAxis = st.sortAxis;
    if (st.sortDir && ['asc', 'desc'].includes(st.sortDir)) realSortDir = st.sortDir;
    if (st.family) setSelValue('realFamilyFilter', st.family);
    if (st.listFamily) setSelValue('realListFamily', st.listFamily);
    if (st.axisTop) setSelValue('realAxisTop', st.axisTop);
    setSelValue('realFormatSelect', realFormat);
    setSelValue('realCompareFormat', st.compareFormat || realFormat);
    setSelValue('realWindowPickSel', realWindowPick);
    if (st.selected && resourcesData.real.some(r => r.id === st.selected)) realSelectedId = st.selected;
    if (st.compareA && resourcesData.real.some(r => r.id === st.compareA)) realCompareA = st.compareA;
    if (st.compareB && resourcesData.real.some(r => r.id === st.compareB)) realCompareB = st.compareB;
    setSelValue('realCompareA', realCompareA);
    setSelValue('realCompareB', realCompareB);
}

// setSelValue — установить значение селекта (если option существует).
function setSelValue(id, v) {
    const el = document.getElementById(id);
    if (el && v !== null && v !== undefined && v !== '') {
        if (el.querySelector(`option[value="${v}"]`)) el.value = v;
    }
}

// bindFilters — обработчики фильтров (навешиваются один раз).
function bindFilters() {
    const bind = (id, evt, fn) => {
        const el = document.getElementById(id);
        if (el) el.addEventListener(evt, fn);
    };
    bind('resourcesCatFilter', 'change', renderCatalog);
    bind('resourcesTypeFilter', 'change', renderCatalog);
    bind('resourcesSearch', 'input', renderCatalog);
    bind('resourcesRaceFilter', 'change', renderCoverage);
    bind('resourcesRaceFilter', 'change', renderT1);
    bind('resourcesAxisFilter', 'change', renderCoverage);
}

// loadResources — загрузка и рендер (кнопка «Обновить» + активация вкладки).
export async function loadResources() {
    const body = document.getElementById('resourcesBody');
    const racesBody = document.getElementById('resourcesRacesBody');
    if (!body || !racesBody) return;
    body.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#94a3b8;">Загрузка...</td></tr>';
    racesBody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:#94a3b8;">Загрузка...</td></tr>';
    try {
        const res = await fetchWithAuth('/admin/resources');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        resourcesData = await res.json();
        resourceNames = {};
        resourceRaces = {};
        resourcesData.resources.forEach(r => { resourceNames[r.id] = r.name; });
        // Обратный индекс: ресурс → какие расы его потребляют (через coverage).
        resourcesData.races.forEach(rc => {
            const raceLabel = rc.name || rc.race_id;
            Object.values(rc.coverage || {}).forEach(ids => {
                ids.forEach(id => {
                    if (!resourceRaces[id]) resourceRaces[id] = [];
                    if (!resourceRaces[id].includes(raceLabel)) resourceRaces[id].push(raceLabel);
                });
            });
        });
        fillRaceSelect();
        fillAxisSelect();
        renderAll();
        renderReal();
    } catch (e) {
        body.innerHTML = `<tr><td colspan="6" style="text-align:center;color:#f87171;">❌ Ошибка загрузки: ${e.message}</td></tr>`;
        racesBody.innerHTML = '';
        const summary = document.getElementById('resourcesSummary');
        if (summary) summary.textContent = '';
    }
}

// ==================== Рендер ====================

function renderAll() {
    renderSummary();
    renderCatalog();
    renderCoverage();
    renderT1();
}

// renderSummary — сводка: «ресурсов: 20 · рас: 50 · дыр: 0».
function renderSummary() {
    const summary = document.getElementById('resourcesSummary');
    if (!summary || !resourcesData) return;
    const gaps = resourcesData.gaps || [];
    summary.textContent = `ресурсов: ${resourcesData.resources.length} · рас: ${resourcesData.races.length} · дыр: ${gaps.length}`;
}

// renderCatalog — каталог с фильтрами (категория И тип И поиск).
function renderCatalog() {
    const body = document.getElementById('resourcesBody');
    if (!body || !resourcesData) return;
    const cat = document.getElementById('resourcesCatFilter').value;
    const type = document.getElementById('resourcesTypeFilter').value;
    const search = document.getElementById('resourcesSearch').value.trim().toLowerCase();

    const filtered = resourcesData.resources.filter(r => {
        if (cat && r.category !== cat) return false;
        if (type === 'nuclear' && r.bridge) return false;
        if (type === 'bridge' && !r.bridge) return false;
        if (search && !r.name.toLowerCase().includes(search)) return false;
        return true;
    });

    if (filtered.length === 0) {
        body.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#94a3b8;">Ничего не найдено по фильтру</td></tr>';
        return;
    }
    body.innerHTML = filtered.map((r, i) => {
        const flags = [];
        if (r.sublimating) flags.push('сублимирующий');
        if (r.supercritical) flags.push('сверхкритический');
        const closes = (r.closes || []).map(a => CHEMOTYPE_NAMES[a] || a).join(', ');
        const races = resourceRaces[r.id] || [];
        return `<tr>
            <td>${i + 1}</td>
            <td>${r.name}${flags.length ? ` <span class="hint" style="font-size:0.75rem;">(${flags.join(', ')})</span>` : ''}</td>
            <td>${r.category_icon} ${CATEGORY_NAMES[r.category] || r.category}</td>
            <td data-id="${r.id}" onclick="toggleAxesCell(this)" style="cursor:pointer; border-bottom: 1px dotted rgba(147,197,253,0.5);">${topAxes(r, 4)} <span style="font-size:0.7rem;color:#93c5fd;">▾</span></td>
            <td>${k2c(r.t_melt)} / ${k2c(r.t_boil)} °C</td>
            <td>${r.bridge ? 'мостовой' : 'ядерный'}${closes ? ` · кормит: ${closes}` : ''}
                <span data-id="${r.id}" onclick="toggleRacesCell(this)" style="cursor:pointer; color:#93c5fd; border-bottom: 1px dotted rgba(147,197,253,0.5); margin-left:6px; white-space:nowrap;">едят: ${races.length} ▾</span></td>
        </tr>`;
    }).join('');
}

// renderCoverage — покрытие рас с фильтрами (раса И ось) + счётчик строк.
function renderCoverage() {
    const racesBody = document.getElementById('resourcesRacesBody');
    if (!racesBody || !resourcesData) return;
    const raceFilter = document.getElementById('resourcesRaceFilter').value;
    const axisFilter = document.getElementById('resourcesAxisFilter').value;

    let rows = '';
    let shown = 0;
    resourcesData.races.forEach(rc => {
        // Оси — в каноническом порядке хемотипов (не по коду).
        const axes = Object.keys(CHEMOTYPE_NAMES).filter(a => a in rc.coverage);
        let nameShown = false;
        if (axes.length === 0) {
            if (!raceFilter || raceFilter === rc.race_id) {
                if (!axisFilter) {
                    shown++;
                    rows += `<tr><td>${rc.name || rc.race_id}</td><td colspan="4" style="color:#94a3b8;">нет осей с весом ≥ 10</td></tr>`;
                }
            }
            return;
        }
        axes.forEach((axis, idx) => {
            if (raceFilter && raceFilter !== rc.race_id) return;
            if (axisFilter && axisFilter !== axis) return;
            shown++;
            const ids = rc.coverage[axis] || [];
            const names = ids.map(id => resourceNames[id] || id).join(', ');
            rows += `<tr>
                <td>${nameShown ? '' : (rc.name || rc.race_id)}</td>
                <td>${chemotypeLabel(axis)}</td>
                <td>${rc.consumption[axis]}</td>
                <td>${names || '—'}</td>
                <td>${ids.length > 0 ? '✅' : '❌'}</td>
            </tr>`;
            nameShown = true;
        });
    });
    racesBody.innerHTML = rows || '<tr><td colspan="5" style="text-align:center;color:#94a3b8;">Ничего не найдено по фильтру</td></tr>';

    const countEl = document.getElementById('resourcesCoverageCount');
    if (countEl) countEl.textContent = `показано ${shown} из ${coverageTotalRows()}`;
}

// coverageTotalRows — полное число строк покрытия (все расы, все оси).
function coverageTotalRows() {
    let n = 0;
    resourcesData.races.forEach(rc => {
        const axes = Object.keys(CHEMOTYPE_NAMES).filter(a => a in rc.coverage);
        n += axes.length === 0 ? 1 : axes.length;
    });
    return n;
}

// ==================== Хелперы ====================

// topAxes — n осей с наибольшими значениями (компактный профиль).
function topAxes(r, n) {
    return Object.keys(AXIS_NAMES)
        .map(k => ({ name: AXIS_NAMES[k], value: r[k] }))
        .sort((a, b) => b.value - a.value)
        .slice(0, n)
        .map(e => `${e.name} ${e.value}`)
        .join(' · ');
}

// fullProfile — полный профиль всех 10 осей (для разворота ячейки по клику).
function fullProfile(r) {
    return Object.keys(AXIS_NAMES)
        .map(k => `${AXIS_NAMES[k]} ${r[k]}`)
        .join(' · ');
}

// toggleAxesCell — клик по ячейке «Ключевые оси»: топ-4 ↔ полный профиль.
export function toggleAxesCell(td) {
    const r = resourcesData.resources.find(x => x.id === td.dataset.id);
    if (!r) return;
    const expanded = td.dataset.expanded === '1';
    td.dataset.expanded = expanded ? '0' : '1';
    const body = expanded ? topAxes(r, 4) : fullProfile(r);
    td.innerHTML = `${body} <span style="font-size:0.7rem;color:#93c5fd;">${expanded ? '▾' : '▴'}</span>`;
    setWrap(td, !expanded);
}

// toggleRacesCell — клик по «едят: N»: список рас ↔ счётчик.
export function toggleRacesCell(td) {
    const expanded = td.dataset.expanded === '1';
    td.dataset.expanded = expanded ? '0' : '1';
    const races = resourceRaces[td.dataset.id] || [];
    if (expanded) {
        td.innerHTML = `едят: ${races.length} <span style="font-size:0.7rem;">▾</span>`;
    } else {
        td.innerHTML = `${races.join(', ')} <span style="font-size:0.7rem;">▴</span>`;
    }
    setWrap(td, !expanded);
}

// setWrap — при раскрытии разрешить перенос текста, чтобы длинный список
// не растягивал таблицу за пределы экрана (горизонтальная прокрутка).
function setWrap(td, on) {
    if (on) {
        td.style.whiteSpace = 'normal';
        td.style.overflowWrap = 'anywhere';
    } else {
        td.style.whiteSpace = '';
        td.style.overflowWrap = '';
    }
}

// chemotypeLabel — читаемое имя consumption-оси («ВОД → вода»).
function chemotypeLabel(axis) {
    return `${axis} → ${CHEMOTYPE_NAMES[axis] || axis}`;
}

// fillRaceSelect — селект расы (50 био-рас из JSON).
function fillRaceSelect() {
    const sel = document.getElementById('resourcesRaceFilter');
    if (!sel) return;
    sel.innerHTML = '<option value="">Все расы</option>' +
        resourcesData.races.map(rc => `<option value="${rc.race_id}">${rc.name || rc.race_id}</option>`).join('');
}

// fillAxisSelect — селект оси (13 хемотипов, читаемые).
function fillAxisSelect() {
    const sel = document.getElementById('resourcesAxisFilter');
    if (!sel) return;
    sel.innerHTML = '<option value="">Все оси</option>' +
        Object.keys(CHEMOTYPE_NAMES).map(a => `<option value="${a}">${chemotypeLabel(a)}</option>`).join('');
}

// ==================== Корзина T1 ====================

// renderT1 — «Корзина T1 выбранной расы»: еда (свои ресурсы) + теневой
// товар (из не-своих ресурсов). Спека 94a §7: корзина T1 = ресурсы без
// переработки (только добычи) + простенький производимый товар теневым
// механизмом для роста.
function renderT1() {
    const block = document.getElementById('resourcesT1Block');
    if (!block || !resourcesData) return;
    const raceFilter = document.getElementById('resourcesRaceFilter').value;
    if (!raceFilter) {
        block.innerHTML = '<span style="color:#94a3b8;">Выберите расу в фильтре выше — покажу, что ей нужно на 1 тире.</span>';
        return;
    }
    const rc = resourcesData.races.find(x => x.race_id === raceFilter);
    if (!rc) { block.innerHTML = ''; return; }
    const label = rc.name || rc.race_id;

    // Свои ресурсы: всё, что закрывает оси расы (из coverage).
    const ownIds = new Set();
    Object.values(rc.coverage || {}).forEach(ids => ids.forEach(id => ownIds.add(id)));
    const own = resourcesData.resources.filter(r => ownIds.has(r.id));
    const ownNames = own.map(r => r.name);

    // Не-свои: каталог минус свои.
    const foreign = resourcesData.resources.filter(r => !ownIds.has(r.id));

    // Теневой товар T1: по типам — лучший не-свой по релевантной оси.
    const t1 = [
        { name: 'Топливо', axis: 'energy_density', alt: 'flammability' },
        { name: 'Химикаты', axis: 'chemical_activity' },
        { name: 'Конструкционные', axis: 'hardness' },
        { name: 'Товары быта', axis: 'elasticity', any: true },
    ].map(t => {
        const pool = t.any ? foreign.slice(0, 3) : [bestByAxis(foreign, t.axis)];
        const list = pool.map(r => r.name).filter(Boolean).join(', ');
        return `<b>${t.name}:</b> ${list || '—'}`;
    }).join('<br>');

    block.innerHTML = `
        <div style="background:#16162a; border:1px solid #2a2a4a; border-radius:8px; padding:10px 14px;">
            <b>${label}</b> — корзина T1:
            <div style="margin-top:6px;"><b>Еда (ресурсы диеты):</b> ${ownNames.join(', ') || '—'}</div>
            <div style="margin-top:4px;"><b>Теневой товар (из не-своих ресурсов):</b><br>${t1}</div>
        </div>`;
}

// bestByAxis — не-свой ресурс с максимальным значением оси (для теневого товара).
function bestByAxis(list, axis) {
    if (!list.length) return null;
    return list.reduce((a, b) => (b[axis] || 0) > (a[axis] || 0) ? b : a);
}

// ==================== Реальные вещества (идея 2026-09-18 §4б, этап 2) ====================

// REAL_FORMATS — 6 форматов отображения профиля (спеки @designer).
const REAL_FORMATS = [
    ['1', 'Радар-паутина'],
    ['2', 'Горизонтальные бары'],
    ['3', 'Тепловая матрица'],
    ['4', 'Термометры с окнами'],
    ['5', 'Ярлыки-градации'],
    ['6', 'Профиль-полоска'],
];

// GOODS_WINDOWS — демо-окна товаров (§5.2.2; реестр не зафиксирован — витрина).
const GOODS_WINDOWS = [
    { id: 'armor', name: 'Броня', axes: { hardness: { lo: 70, hi: 100 }, density: { lo: 0, hi: 50 }, elasticity: { lo: 40, hi: 100 } } },
    { id: 'fuel', name: 'Топливо', axes: { energy_density: { lo: 70, hi: 100 }, flammability: { lo: 70, hi: 100 } } },
    { id: 'construct', name: 'Конструкционные', axes: { hardness: { lo: 70, hi: 100 } }, t_melt: { lo: 1500, hi: 100000 } },
    { id: 'meds', name: 'Медикаменты', axes: { biocompatibility: { lo: 70, hi: 100 }, toxicity: { lo: 0, hi: 30 } } },
];

// RUS_AXIS_TO_KEY — русские имена осей (Go, chemotypes.go) → ключи JSON-полей.
const RUS_AXIS_TO_KEY = {
    'твёрдость': 'hardness',
    'эластичность': 'elasticity',
    'проводимость': 'conductivity',
    'плотность': 'density',
    'энергоёмкость': 'energy_density',
    'биосовместимость': 'biocompatibility',
    'радиоактивность': 'radioactivity',
    'токсичность': 'toxicity',
    'горючесть': 'flammability',
    'химическая активность': 'chemical_activity',
};

// fillRealWindowPick — селект «Окно»: 'все' + конкретные окна текущего набора
// (товары 4 или хемотипы 13). При realWindow='off' — селект недоступен.
// Показ окна по одному делает окна читаемыми (каша из всех сразу — нет).
function fillRealWindowPick() {
    const sel = document.getElementById('realWindowPickSel');
    if (!sel) return;
    const all = activeWindows() || [];
    if (all.length === 0) {
        sel.disabled = true;
        sel.innerHTML = '<option value="all">все</option>';
        realWindowPick = 'all';
        return;
    }
    sel.disabled = false;
    sel.innerHTML = '<option value="all">все</option>' +
        all.map(w => `<option value="${w.id}">${w.name}</option>`).join('');
    if (realWindowPick !== 'all' && !all.some(w => w.id === realWindowPick)) realWindowPick = 'all';
    sel.value = realWindowPick;
}

// renderReal — вся подвкладка «Реальные вещества» (сводка + селекты + режим).
function renderReal() {
    renderRealSummary();
    fillRealFamilySelect();
    fillRealAxisSelect();
    fillRealFormatSelect();
    fillRealCompareSelects();
    fillRealListFamilySelect();
    fillRealWindowPick();
    restoreRealState();   // применить сохранённое (режим/формат/окна/выборы)
    syncWindowSelects();
    switchRealMode(realMode);   // показывает нужный div режима + рендерит
}

// renderRealSummary — живая сводка: «111 веществ · различимы по осям: 86 ·
// с учётом T: 107 · коллизий: 43 пары» (числа считает сервер, distance.go).
function renderRealSummary() {
    const el = document.getElementById('realSummary');
    if (!el || !resourcesData) return;
    const s = resourcesData.real_summary;
    if (!s) return;
    el.textContent = `${s.total} веществ · различимы по осям: ${s.by_axes} · с учётом T: ${s.with_t} · коллизий: ${s.collisions} пар`;
}

// fillRealFamilySelect — селект семейства (режим «Один ресурс»).
function fillRealFamilySelect() {
    const sel = document.getElementById('realFamilyFilter');
    if (!sel || !resourcesData) return;
    sel.innerHTML = '<option value="">Все семейства</option>' +
        (resourcesData.families || []).map(f => `<option value="${f}">${f}</option>`).join('');
}

// fillRealAxisSelect — селект оси для топ-5 (режим «Список»).
function fillRealAxisSelect() {
    const sel = document.getElementById('realAxisTop');
    if (!sel || !resourcesData) return;
    sel.innerHTML = Object.keys(AXIS_NAMES).map(k => `<option value="${k}">${AXIS_NAMES[k]}</option>`).join('');
}

// fillRealFormatSelect — селекты формата (режимы «Один ресурс» и «Сравнить»).
function fillRealFormatSelect() {
    const sel = document.getElementById('realFormatSelect');
    if (sel) sel.innerHTML = REAL_FORMATS.map(([v, n]) => `<option value="${v}">${n}</option>`).join('');
    const cmp = document.getElementById('realCompareFormat');
    if (cmp) cmp.innerHTML = REAL_FORMATS.map(([v, n]) => `<option value="${v}">${n}</option>`).join('');
}

// fillRealCompareSelects — селекты двух веществ (режим «Сравнить»).
function fillRealCompareSelects() {
    const a = document.getElementById('realCompareA');
    const b = document.getElementById('realCompareB');
    if (!a || !b || !resourcesData) return;
    const opts = resourcesData.real.map(r => `<option value="${r.id}">${r.name}</option>`).join('');
    a.innerHTML = opts;
    b.innerHTML = opts;
    if (!realCompareA && resourcesData.real.length) realCompareA = resourcesData.real[0].id;
    if (!realCompareB && resourcesData.real.length > 1) realCompareB = resourcesData.real[1].id;
    a.value = realCompareA;
    b.value = realCompareB;
}

// fillRealListFamilySelect — селект семейства (режим «Список»).
function fillRealListFamilySelect() {
    const sel = document.getElementById('realListFamily');
    if (!sel || !resourcesData) return;
    sel.innerHTML = '<option value="">Все семейства</option>' +
        (resourcesData.families || []).map(f => `<option value="${f}">${f}</option>`).join('');
}

// switchRealMode — переключение режима просмотра (кнопки, как в генерации 71a).
export function switchRealMode(mode) {
    realMode = mode;
    document.querySelectorAll('#tab-resources .real-mode').forEach(d => {
        d.style.display = d.id === 'realMode-' + mode ? 'block' : 'none';
    });
    document.querySelectorAll('#tab-resources .real-mode-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.realMode === mode);
    });
    renderRealMode();
    saveRealState();
}

// renderRealMode — рендер активного режима.
function renderRealMode() {
    if (realMode === 'single') { renderRealList(); renderRealDetail(); }
    else if (realMode === 'all') { renderRealAllFormats(); }
    else if (realMode === 'compare') { renderRealCompare(); }
    else if (realMode === 'list') { renderRealStripList(); renderRealTop5(); }
}

// ==================== Окна ====================

// chemotypeWindows — окна «расы» из шаблонов хемотипов (сервер, chemotypes.go):
// каждая consumption-ось = окно-набор (оси + T_melt/T_boil).
function chemotypeWindows() {
    return (resourcesData.chemotypes || []).map(ct => {
        const axes = {};
        (ct.windows || []).forEach(w => {
            const key = RUS_AXIS_TO_KEY[w.axis];
            if (key) axes[key] = { lo: w.lo, hi: w.hi };
        });
        const item = { id: ct.axis, name: ct.axis, axes };
        if (ct.t_melt) item.t_melt = ct.t_melt;
        if (ct.t_boil) item.t_boil = ct.t_boil;
        return item;
    });
}

// checkWindow — проверка ресурса по окну (оси + T): список промахов.
function checkWindow(res, win) {
    const misses = [];
    const check = (axis, v, iv) => {
        if (v < iv.lo) misses.push({ axis, v, iv, dir: 'lo' });
        else if (v > iv.hi) misses.push({ axis, v, iv, dir: 'hi' });
    };
    for (const [axis, iv] of Object.entries(win.axes || {})) check(axis, res[axis] || 0, iv);
    if (win.t_melt) check('t_melt', res.t_melt, win.t_melt);
    if (win.t_boil) check('t_boil', res.t_boil, win.t_boil);
    return { ok: misses.length === 0, misses };
}

// windowResults — итог по каждому окну набора.
function windowResults(res, windows) {
    return (windows || []).map(w => {
        const r = checkWindow(res, w);
        return { name: w.name, ok: r.ok, misses: r.misses };
    });
}

// activeWindows — набор окон по realWindow ('off' → null).
function activeWindows() {
    if (realWindow === 'goods') return GOODS_WINDOWS;
    if (realWindow === 'races') return chemotypeWindows();
    return null;
}

// buildCtx — контекст рендера формата (окна + итог + фаза). Логика окон:
// «все» — интервалы НЕ рисуются (чистый профиль), внизу компактная сводка
// ✓/✗ по всем окнам; конкретное окно (realWindowPick) — рисуются интервалы
// только его, внизу одна детальная итог-строка. Так окна читаемы.
function buildCtx(res, color) {
    const all = activeWindows();                       // полный набор окон или null
    const picked = all && realWindowPick !== 'all'
        ? all.filter(w => w.id === realWindowPick) : null;   // одно выбранное окно
    return {
        labels: AXIS_NAMES,
        color,
        windows: picked,                               // интервалы для рисования (null при «все»)
        results: picked ? windowResults(res, picked) : (all ? windowResults(res, all) : null),
        windowType: realWindow,
        phase: phaseAt293(res),
        single: !!picked,                              // одно окно → детальная итог-строка
    };
}

// realRadarMax — максимум радара по режиму: «Один ресурс» — крупно,
// «Сравнить» — два рядом, «Все форматы» — чтобы сетка влезла на экран.
function realRadarMax() {
    if (realMode === 'all') return 440;
    if (realMode === 'compare') return 600;
    return 800;
}

// renderFormat — рендер формата в контейнер. Окна берутся из ctx (buildCtx):
// «все» — интервалы не рисуются (компактная сводка), конкретное окно —
// интервалы только его.
function renderFormat(container, fmt, res, ctx) {
    if (fmt === '1') {
        // Адаптивный размер радара: по доступной ширине, но не больше
        // максимума режима (не экономим место, но всё на один экран).
        const avail = container.clientWidth || 560;
        const size = Math.min(realRadarMax(), Math.max(360, avail - 8));
        container.innerHTML = `<canvas width="${size}" height="${size}" style="max-width:100%;"></canvas>`;
        const cv = container.firstChild;
        cv.title = 'Колесо мыши — зум (низкие значения), двойной клик — сброс';
        cv.addEventListener('wheel', (e) => {
            e.preventDefault();
            const z = parseFloat(cv.dataset.zoom || '1');
            const nz = Math.min(8, Math.max(1, e.deltaY < 0 ? z * 1.25 : z / 1.25));
            cv.dataset.zoom = String(nz);
            drawRadar(cv, res, ctx);
        });
        cv.addEventListener('dblclick', () => { cv.dataset.zoom = '1'; drawRadar(cv, res, ctx); });
        drawRadar(cv, res, ctx);
    } else if (fmt === '2') container.innerHTML = renderBars(res, ctx);
    else if (fmt === '3') container.innerHTML = renderMatrix(res, ctx);
    else if (fmt === '4') container.innerHTML = renderThermo(res, ctx);
    else if (fmt === '5') container.innerHTML = renderLabels(res, ctx);
    else if (fmt === '6') container.innerHTML = renderStrip(res, ctx);
}

// windowHint — подпись источника окон (демо-товары / хемотипы).
function windowHint() {
    if (realWindow === 'goods') return '<span style="font-size:0.95rem; color:#64748b;">демо-окна товаров §5.2.2, реестр не зафиксирован</span>';
    if (realWindow === 'races') return '<span style="font-size:0.95rem; color:#64748b;">окна хемотипов (спека 94a §3.1)</span>';
    return '';
}

// ==================== Режим 1: Один ресурс ====================

// renderRealList — компактный список веществ семейства (без скролл-ленты:
// семейство обычно ≤ 20 строк). Клик по имени — выбор вещества.
function renderRealList() {
    const el = document.getElementById('realList');
    if (!el || !resourcesData) return;
    const fam = document.getElementById('realFamilyFilter').value;
    const list = resourcesData.real.filter(r => !fam || r.family === fam);
    if (list.length === 0) {
        el.innerHTML = '<span style="color:#94a3b8;">В семействе нет веществ</span>';
        return;
    }
    el.innerHTML = `<table class="stats-table">
        <thead><tr>
            <th data-tip="Клик по имени — профиль в выбранном формате">Вещество</th>
            <th>Категория</th>
            <th data-tip="3 оси с наибольшими значениями">Ключевые оси</th>
            <th data-tip="Агрегатное состояние при 20 °C (§9.1.2)">Фаза при 20 °C</th>
        </tr></thead>
        <tbody>${list.map(r => {
            const sel = r.id === realSelectedId ? ' style="background:#1e293b;"' : '';
            return `<tr${sel}>
                <td style="cursor:pointer;" onclick="selectReal('${r.id}')">${r.name}${r.sublimating ? ' <span class="hint" style="font-size:0.75rem;">(сублимирует)</span>' : ''}</td>
                <td>${r.category_icon} ${CATEGORY_NAMES[r.category] || r.category}</td>
                <td>${topAxes(r, 3)}</td>
                <td>${phaseAt293(r)}</td>
            </tr>`;
        }).join('')}</tbody>
    </table>`;
}

// selectReal — выбор вещества (режимы «Один ресурс» и «Все форматы»).
export function selectReal(id) {
    realSelectedId = id;
    renderRealList();
    renderRealDetail();
    if (realMode === 'all') renderRealAllFormats();
    saveRealState();
}

// renderRealDetail — выбранный формат + карточка (режим «Один ресурс»).
function renderRealDetail() {
    const el = document.getElementById('realDetail');
    if (!el || !resourcesData) return;
    if (!realSelectedId) {
        el.innerHTML = '<span style="color:#94a3b8;">Выберите вещество слева</span>';
        return;
    }
    const a = resourcesData.real.find(r => r.id === realSelectedId);
    if (!a) { el.innerHTML = ''; return; }
    const ctx = buildCtx(a, '#4a9eff');
    el.innerHTML = `<div style="background:#16162a; border:1px solid #2a2a4a; border-radius:8px; padding:14px 18px;">
        <div style="font-weight:600; font-size:1.1rem; color:#4a9eff; margin-bottom:8px;">${a.name}</div>
        <div id="realFormatBody"></div>
        ${windowHint()}
        ${cardBlock(a, '#4a9eff')}
    </div>`;
    renderFormat(document.getElementById('realFormatBody'), realFormat, a, ctx);
}

// cardBlock — карточка вещества: семейство, категория, T, фаза при 293 K.
function cardBlock(r, color) {
    const flags = [];
    if (r.sublimating) flags.push('сублимирующий');
    return `<div style="margin-top:8px; border-top:1px solid #2a2a4a; padding-top:8px;">
        <div style="font-size:1.05rem; line-height:1.7; color:#cbd5e1;">
            <div>Семейство: <b>${r.family}</b></div>
            <div>Категория: ${r.category_icon} ${CATEGORY_NAMES[r.category] || r.category}</div>
            <div>T_melt: <b>${k2c(r.t_melt)} °C</b> · T_boil: <b>${k2c(r.t_boil)} °C</b></div>
            <div>Фаза при 20 °C: <b>${phaseAt293(r)}</b>${flags.length ? ` <span class="hint">(${flags.join(', ')})</span>` : ''}</div>
        </div>
    </div>`;
}

// phaseAt293 — агрегатное состояние при 293 K (правило §9.1.2:
// T < T_melt → твёрдое, T_melt ≤ T < T_boil → жидкое, ≥ T_boil → газ;
// сублимирующие — жидкой фазы нет).
function phaseAt293(r) {
    if (r.sublimating) return 293 < r.t_melt ? 'твёрдое' : 'газ';
    if (293 < r.t_melt) return 'твёрдое';
    if (293 < r.t_boil) return 'жидкое';
    return 'газ';
}

// ==================== Режим 2: Все форматы ====================

// renderRealAllFormats — сетка всех 6 форматов выбранного ресурса
// (цель — создатель выбирает лучший вид).
function renderRealAllFormats() {
    const el = document.getElementById('realAllFormats');
    if (!el || !resourcesData) return;
    if (!realSelectedId) {
        el.innerHTML = '<span style="color:#94a3b8;">Сначала выберите вещество в режиме «Один ресурс»</span>';
        return;
    }
    const a = resourcesData.real.find(r => r.id === realSelectedId);
    if (!a) { el.innerHTML = ''; return; }
    const ctx = buildCtx(a, '#4a9eff');
    el.innerHTML = `<div style="font-weight:600; color:#4a9eff; margin-bottom:6px;">${a.name} — все 6 форматов</div>
        ${windowHint()}
        <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(480px, 1fr)); gap:12px; margin-top:8px;">
            ${REAL_FORMATS.map(([f, n]) => `<div style="background:#16162a; border:1px solid #2a2a4a; border-radius:8px; padding:10px 14px;">
                <div style="font-size:1rem; color:#94a3b8; margin-bottom:6px;">${n}</div>
                <div id="realAllFmt-${f}"></div>
            </div>`).join('')}
        </div>`;
    REAL_FORMATS.forEach(([f]) => renderFormat(document.getElementById('realAllFmt-' + f), f, a, ctx));
}

// ==================== Режим 3: Сравнить ====================

// renderRealCompare — два ресурса рядом (синий/красный), выбранный формат.
function renderRealCompare() {
    const el = document.getElementById('realCompareDetail');
    if (!el || !resourcesData) return;
    if (!realCompareA || !realCompareB) {
        el.innerHTML = '<span style="color:#94a3b8;">Выберите два вещества</span>';
        return;
    }
    const a = resourcesData.real.find(r => r.id === realCompareA);
    const b = resourcesData.real.find(r => r.id === realCompareB);
    if (!a || !b) { el.innerHTML = ''; return; }
    const fmt = document.getElementById('realCompareFormat').value;
    const ctxA = buildCtx(a, '#4a9eff');
    const ctxB = buildCtx(b, '#f87171');
    el.innerHTML = `<div style="display:flex; gap:16px; flex-wrap:wrap; align-items:flex-start;">
        <div style="flex:1; min-width:420px; background:#16162a; border:1px solid #2a2a4a; border-radius:8px; padding:14px 18px;">
            <div style="font-weight:600; font-size:1.1rem; color:#4a9eff; margin-bottom:8px;">${a.name}</div>
            <div id="realCmpA"></div>
        </div>
        <div style="flex:1; min-width:420px; background:#16162a; border:1px solid #2a2a4a; border-radius:8px; padding:14px 18px;">
            <div style="font-weight:600; font-size:1.1rem; color:#f87171; margin-bottom:8px;">${b.name}</div>
            <div id="realCmpB"></div>
        </div>
    </div>
    ${windowHint()}`;
    renderFormat(document.getElementById('realCmpA'), fmt, a, ctxA);
    renderFormat(document.getElementById('realCmpB'), fmt, b, ctxB);
}

// ==================== Режим 4: Список ====================

// phaseOrder — порядок фаз для сортировки (твёрдое → жидкое → газ).
function phaseOrder(r) {
    const p = phaseAt293(r);
    return p === 'твёрдое' ? 0 : p === 'жидкое' ? 1 : 2;
}

// sortReal — клик по заголовку списка: сортировка по столбцу (повторный
// клик — разворот направления).
export function sortReal(axis) {
    if (realSortAxis === axis) realSortDir = realSortDir === 'asc' ? 'desc' : 'asc';
    else { realSortAxis = axis; realSortDir = axis === 'name' ? 'asc' : 'desc'; }
    saveRealState();
    renderRealStripList();
}

// renderRealStripList — режим «Список»: таблица со ЗАКРЕПЛЁННОЙ шапкой
// (sticky): Вещество | 10 осей (мини-бары) | Фаза. Клик по заголовку —
// сортировка по столбцу. Названия осей — в шапке, не под полосками.
function renderRealStripList() {
    const el = document.getElementById('realStripList');
    if (!el || !resourcesData) return;
    const fam = document.getElementById('realListFamily').value;
    const list = resourcesData.real.filter(r => !fam || r.family === fam);
    if (list.length === 0) {
        el.innerHTML = '<span style="color:#94a3b8;">Нет веществ</span>';
        return;
    }
    const dir = realSortDir === 'desc' ? -1 : 1;
    const sorted = list.slice().sort((a, b) => {
        if (realSortAxis === 'name') return a.name.localeCompare(b.name, 'ru') * dir;
        if (realSortAxis === 'phase') return (phaseOrder(a) - phaseOrder(b)) * dir;
        return ((a[realSortAxis] || 0) - (b[realSortAxis] || 0)) * dir;
    });
    const arrow = axis => axis === realSortAxis ? (realSortDir === 'asc' ? ' ▲' : ' ▼') : '';
    const th = (axis, label) =>
        `<th onclick="sortReal('${axis}')" style="cursor:pointer; white-space:nowrap; user-select:none; background:#16162a;" title="Сортировка по ${label}">${label}${arrow(axis)}</th>`;
    const rows = sorted.map(r => {
        const segs = AXIS_KEYS.map(key => {
            const v = r[key] || 0;
            return `<td style="padding:2px 3px;"><div style="position:relative; height:20px; background:#1e293b; border-radius:3px; overflow:hidden;" title="${AXIS_NAMES[key]}: ${v}">
                <div style="width:${v}%; height:100%; background:#4a9eff;"></div></div></td>`;
        }).join('');
        return `<tr>
            <td style="white-space:nowrap; overflow:hidden; text-overflow:ellipsis; max-width:200px; font-size:1rem;">${r.name}${r.sublimating ? ' <span class="hint" style="font-size:0.8rem;">(субл.)</span>' : ''}</td>
            ${segs}
            <td style="font-size:1rem;">${phaseAt293(r)}</td>
        </tr>`;
    }).join('');
    el.innerHTML = `<div style="font-size:1rem; color:#94a3b8; margin-bottom:6px;">${sorted.length} веществ · клик по заголовку — сортировка ${windowHint()}</div>
        <div style="overflow:auto; max-height:calc(100vh - 330px);">
        <table class="stats-table" style="width:100%; border-collapse:collapse;">
            <thead style="position:sticky; top:0; z-index:2;">
                <tr>${th('name', 'Вещество')}${AXIS_KEYS.map(k => th(k, shortLabel(k))).join('')}${th('phase', 'Фаза')}</tr>
            </thead>
            <tbody>${rows}</tbody>
        </table>
        </div>`;
}

// renderRealTop5 — топ-5 веществ по выбранной оси с полосками («вау»-момент
// узнаваемости: алмаз — твёрдость 100, фтор — токсичность 100).
function renderRealTop5() {
    const el = document.getElementById('realTop5');
    if (!el || !resourcesData) return;
    const axis = document.getElementById('realAxisTop').value;
    if (!axis) return;
    const top = resourcesData.real.slice()
        .sort((a, b) => (b[axis] || 0) - (a[axis] || 0))
        .slice(0, 5);
    const max = top[0] ? top[0][axis] : 0;
    el.innerHTML = `<div style="background:#16162a; border:1px solid #2a2a4a; border-radius:8px; padding:10px 14px;">
        <b>Топ-5 по оси «${AXIS_NAMES[axis]}»:</b>
        <div style="margin-top:6px; display:flex; flex-direction:column; gap:4px;">
            ${top.map(r => {
                const w = max ? Math.round(r[axis] / max * 100) : 0;
                return `<div style="display:flex; align-items:center; gap:8px; font-size:1.1rem;">
                    <span style="width:240px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${r.name}</span>
                    <div style="flex:1; background:#1e293b; border-radius:4px; height:22px;">
                        <div style="width:${w}%; background:#4a9eff; border-radius:4px; height:22px;"></div>
                    </div>
                    <span style="width:48px; text-align:right; color:#93c5fd;">${r[axis]}</span>
                </div>`;
            }).join('')}
        </div>
    </div>`;
}