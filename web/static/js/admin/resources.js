// web/static/js/admin/resources.js
// Вкладка «Ресурсы» (спека 94a, read-only): каталог универсального слоя
// (20 ресурсов) + покрытие рас (для каждой оси с весом ≥ 10 — ресурсы,
// попадающие в окно расы). Данные — GET /admin/resources; фильтры каталога
// (категория/тип/поиск) и покрытия (раса/ось) работают вместе (И).
import { fetchWithAuth } from './auth.js';

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

// initResources — ленивая инициализация при активации вкладки (как initBalancer).
export function initResources() {
    if (resourcesBound) return;
    resourcesBound = true;
    bindFilters();
    loadResources();
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
        resourcesData.resources.forEach(r => { resourceNames[r.id] = r.name; });
        fillRaceSelect();
        fillAxisSelect();
        renderAll();
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
        return `<tr>
            <td>${i + 1}</td>
            <td>${r.name}${flags.length ? ` <span class="hint" style="font-size:0.75rem;">(${flags.join(', ')})</span>` : ''}</td>
            <td>${r.category_icon} ${CATEGORY_NAMES[r.category] || r.category}</td>
            <td data-id="${r.id}" onclick="toggleAxesCell(this)" style="cursor:pointer; border-bottom: 1px dotted rgba(147,197,253,0.5);">${topAxes(r, 4)} <span style="font-size:0.7rem;color:#93c5fd;">▾</span></td>
            <td>${r.t_melt} / ${r.t_boil}</td>
            <td>${r.bridge ? 'мостовой' : 'ядерный'}${closes ? ` · кормит: ${closes}` : ''}</td>
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