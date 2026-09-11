// web/static/js/admin/stats.js
import { fetchWithAuth } from './auth.js';
import { showLoader } from '../loader.js';

// Перевод Кельвинов в Цельсии
function kelvinToCelsius(k) {
    if (typeof k !== 'number' || isNaN(k)) return '—';
    return (k - 273.15).toFixed(1);
}

export async function loadStats() {
    try {
        const res = await fetchWithAuth('/admin/stats');
        const data = await res.json();
        document.getElementById('statsWorlds').textContent = data.worlds || 0;
        document.getElementById('statsPlanets').textContent = data.planets || 0;
    } catch (e) { console.error(e); }
}

export async function loadPlanetStats(refresh = false) {
    const container = document.getElementById('planetStatsContainer');
    const stop = showLoader(container, 'Загрузка статистики...');
    try {
        const url = refresh ? '/admin/stats/planets?refresh=1' : '/admin/stats/planets';
        const res = await fetchWithAuth(url);
        if (!res.ok) throw new Error('Failed to fetch stats');
        const stats = await res.json();
        stop();
        renderPlanetStats(stats, container);
    } catch (e) {
        stop(`<p style="color:#f87171;">❌ Ошибка загрузки: ${e.message}</p>`);
    }
}

export function renderPlanetStats(stats, container) {
    let html = '';

    // Общая сводка
    html += `<div class="stat-grid">`;
    html += `<div class="stat-card"><strong>Всего миров:</strong> ${stats.total_worlds}</div>`;
    html += `<div class="stat-card"><strong>Миров с планетами:</strong> ${stats.worlds_with_planets}</div>`;
    html += `<div class="stat-card"><strong>Всего планет:</strong> ${stats.total_planets}</div>`;
    html += `<div class="stat-card"><strong>Обитаемых:</strong> ${stats.habitable_count}</div>`;
    html += `<div class="stat-card"><strong>С жизнью:</strong> ${stats.life_count}</div>`;
    html += `<div class="stat-card"><strong>Средний размер:</strong> ${stats.avg_size.toFixed(2)}</div>`;
    html += `<div class="stat-card"><strong>Средняя масса:</strong> ${stats.avg_mass.toFixed(2)}</div>`;
    html += `<div class="stat-card"><strong>Средняя температура:</strong> ${kelvinToCelsius(stats.avg_temp)} °C</div>`;
    html += `<div class="stat-card"><strong>Средняя вода:</strong> ${stats.avg_water.toFixed(1)}%</div>`;
    html += `<div class="stat-card"><strong>Среднее население:</strong> ${stats.avg_population.toLocaleString()}</div>`;
    html += `</div>`;

    // Геймдизайнерские типы
    html += `<h3 style="margin-top:20px;">Геймдизайнерские типы планет</h3>`;
    html += `<table class="stats-table" id="gd-table">
        <thead><tr>
            <th class="sortable" data-sort="gdtype" data-order="asc">Тип</th>
            <th class="sortable" data-sort="gdcount" data-order="asc">Кол-во</th>
        </tr></thead>
        <tbody id="gd-body"></tbody>
    </table>`;

    // Типы поверхностей
    html += `<h3 style="margin-top:20px;">Распределение по типам поверхностей</h3>`;
    html += `<table class="stats-table" id="type-table">
        <thead><tr>
            <th class="sortable" data-sort="type" data-order="asc">Тип</th>
            <th class="sortable" data-sort="count" data-order="asc">Кол-во</th>
        </tr></thead>
        <tbody id="type-body"></tbody>
    </table>`;

    // Гидросферы
    html += `<h3 style="margin-top:20px;">Гидросферы</h3>`;
    html += `<table class="stats-table" id="hydro-table">
        <thead><tr>
            <th class="sortable" data-sort="hydro" data-order="asc">Тип</th>
            <th class="sortable" data-sort="hydrocount" data-order="asc">Кол-во</th>
        </tr></thead>
        <tbody id="hydro-body"></tbody>
    </table>`;

    // Атмосферы
    html += `<h3 style="margin-top:20px;">Атмосферы</h3>`;
    html += `<table class="stats-table" id="atmo-table">
        <thead><tr>
            <th class="sortable" data-sort="atmo" data-order="asc">Тип</th>
            <th class="sortable" data-sort="atmocount" data-order="asc">Кол-во</th>
        </tr></thead>
        <tbody id="atmo-body"></tbody>
    </table>`;

    // Биосферы
    html += `<h3 style="margin-top:20px;">Биосферы</h3>`;
    html += `<table class="stats-table" id="bio-table">
        <thead><tr>
            <th class="sortable" data-sort="bio" data-order="asc">Тип</th>
            <th class="sortable" data-sort="biocount" data-order="asc">Кол-во</th>
        </tr></thead>
        <tbody id="bio-body"></tbody>
    </table>`;

    // По спектральным классам
    html += `<h3 style="margin-top:20px;">По спектральным классам</h3>`;
    html += `<table class="stats-table" id="spectral-table">
        <thead><tr>
            <th class="sortable" data-sort="spectral" data-order="asc">Спектр</th>
            <th class="sortable" data-sort="types" data-order="asc">Типы (кол-во)</th>
            <th class="sortable" data-sort="total" data-order="asc">Всего</th>
        </tr></thead>
        <tbody id="spectral-body"></tbody>
    </table>`;

    // Аномалии
    html += `<h3 style="margin-top:20px;">⚠️ Аномалии</h3>`;
    if (stats.anomalies && stats.anomalies.length > 0) {
        html += `<ul>`;
        stats.anomalies.forEach(a => {
            const severityClass = a.severity === 'high' ? 'anomaly-high' : a.severity === 'medium' ? 'anomaly-medium' : 'anomaly-low';
            html += `<li class="${severityClass}">${a.description}: ${a.value.toFixed(1)} (ожидалось ~${a.expected.toFixed(1)})</li>`;
        });
        html += `</ul>`;
    } else {
        html += `<p style="color:#6fcf97;">✅ Аномалий не обнаружено</p>`;
    }

    container.innerHTML = html;

    // --- Заполнение таблиц ---
    fillTable('gd-body', stats.game_design_types);
    fillTable('type-body', stats.planets_by_type);
    fillTable('hydro-body', stats.hydrosphereCount);
    fillTable('atmo-body', stats.atmosphereCount);
    fillTable('bio-body', stats.biosphereCount);

    // Спектральные классы
    const spectralBody = document.getElementById('spectral-body');
    let spectralData = Object.entries(stats.planets_by_spectral || {}).map(([spec, types]) => {
        const total = Object.values(types).reduce((sum, v) => sum + v, 0);
        const typesStr = Object.entries(types).map(([t, c]) => `${t}: ${c}`).join(', ');
        return { spec, typesStr, total };
    });
    spectralData.sort((a, b) => b.total - a.total);
    spectralBody.innerHTML = spectralData.map(d => `<tr><td>${d.spec}</td><td>${d.typesStr}</td><td>${d.total}</td></tr>`).join('');

    // Сортировка
    addSorting('gd-table', 'gd-body', { gdtype: 'text', gdcount: 'number' });
    addSorting('type-table', 'type-body', { type: 'text', count: 'number' });
    addSorting('hydro-table', 'hydro-body', { hydro: 'text', hydrocount: 'number' });
    addSorting('atmo-table', 'atmo-body', { atmo: 'text', atmocount: 'number' });
    addSorting('bio-table', 'bio-body', { bio: 'text', biocount: 'number' });
    addSorting('spectral-table', 'spectral-body', { spectral: 'text', types: 'text', total: 'number' });
}

function fillTable(bodyId, data) {
    const tbody = document.getElementById(bodyId);
    if (!tbody) return;
    let entries = Object.entries(data || {}).map(([key, val]) => ({ key, val }));
    entries.sort((a, b) => b.val - a.val);
    tbody.innerHTML = entries.map(d => `<tr><td>${d.key}</td><td>${d.val}</td></tr>`).join('');
}

function addSorting(tableId, bodyId, sortKeyMap) {
    const table = document.getElementById(tableId);
    if (!table) return;
    const headers = table.querySelectorAll('.sortable');
    headers.forEach(th => {
        th.addEventListener('click', function() {
            const sortKey = this.dataset.sort;
            const currentOrder = this.dataset.order;
            const newOrder = currentOrder === 'asc' ? 'desc' : 'asc';
            this.dataset.order = newOrder;
            const tbody = document.getElementById(bodyId);
            const rows = Array.from(tbody.querySelectorAll('tr'));
            rows.sort((a, b) => {
                let valA, valB;
                if (sortKeyMap[sortKey]) {
                    const key = sortKeyMap[sortKey];
                    valA = key === 'text' ? a.cells[0].textContent : parseInt(a.cells[1].textContent);
                    valB = key === 'text' ? b.cells[0].textContent : parseInt(b.cells[1].textContent);
                } else {
                    valA = a.cells[0].textContent;
                    valB = b.cells[0].textContent;
                }
                if (typeof valA === 'string') {
                    return newOrder === 'asc' ? valA.localeCompare(valB) : valB.localeCompare(valA);
                } else {
                    return newOrder === 'asc' ? valA - valB : valB - valA;
                }
            });
            rows.forEach(row => tbody.appendChild(row));
            table.querySelectorAll('.sortable').forEach(th => {
                th.textContent = th.textContent.replace(/ [▲▼]/, '');
            });
            this.textContent += newOrder === 'asc' ? ' ▲' : ' ▼';
        });
    });
}