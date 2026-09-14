// web/static/js/admin/worlds.js
import { fetchWithAuth, getAdminToken } from './auth.js';
import { openSystemModal } from '../modal/index.js';
import { loadStats } from './stats.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

let currentPage = 1, currentLimit = 50, currentSearch = '';
let currentSort = '', currentOrder = 'asc';

// trendArrow — ↓/↑/— для итога по галактике (зеркало populationTrendArrow
// в modal/panel.js: рост в модели пока не даёт ни одна компонента).
function trendArrow(trend) {
    if (trend === 'decline') return ' <span style="color:#f66;" title="Население убывает">↓</span>';
    if (trend === 'growth') return ' <span style="color:#6f6;" title="Население растёт">↑</span>';
    return ' <span style="color:#888;" title="Без изменений">—</span>';
}

export async function loadWorlds(page) {
    if (page) currentPage = page;
    currentLimit = parseInt(document.getElementById('limitSelect').value, 10) || 50;
    currentSearch = document.getElementById('searchInput').value;

    const tbody = document.getElementById('worldsBody');
    if (!tbody) return;

    // Лоадер со счётчиком — строка таблицы на всю ширину.
    const start = Date.now();
    const loaderRow = document.createElement('tr');
    loaderRow.innerHTML = `
        <td colspan="6" style="text-align:center; color:#94a3b8; padding:32px;">
            <div style="display:flex; flex-direction:column; align-items:center; gap:8px;">
                <div class="zorion-loader-spinner" style="width:28px; height:28px; border:3px solid rgba(74,158,255,0.25); border-top-color:#4a9eff; border-radius:50%; animation:zorionSpin 0.8s linear infinite;"></div>
                <div>
                    <span>Загрузка миров...</span>
                    <span class="loader-time" style="font-variant-numeric:tabular-nums; color:#7a8aa0; margin-left:8px;">0 с</span>
                </div>
            </div>
        </td>`;
    tbody.innerHTML = '';
    tbody.appendChild(loaderRow);

    const timeEl = loaderRow.querySelector('.loader-time');
    const timer = setInterval(() => {
        timeEl.textContent = Math.round((Date.now() - start) / 1000) + ' с';
    }, 250);

    try {
        const url = `/admin/worlds?page=${currentPage}&limit=${currentLimit}&search=${encodeURIComponent(currentSearch)}&sort=${currentSort}&order=${currentOrder}`;
        const res = await fetchWithAuth(url);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();

        const tbody = document.getElementById('worldsBody');
        clearInterval(timer);
        tbody.innerHTML = '';

        // Итог по галактике с трендом.
        const popEl = document.getElementById('galaxyPopulation');
        if (popEl) popEl.textContent = (data.galaxy_population ?? 0).toLocaleString('ru-RU');
        const trendEl = document.getElementById('galaxyTrendArrow');
        if (trendEl) trendEl.innerHTML = trendArrow(data.galaxy_trend);

        // Защита от отсутствия data.data
        const worlds = Array.isArray(data?.data) ? data.data : [];

        if (worlds.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#94a3b8;">Нет миров</td></tr>';
        } else {
            worlds.forEach(w => {
                const row = document.createElement('tr');

                // Клик по строке — открыть модалку системы (схема + планеты).
                // Пустой population_trend (старый формат ответа) — без тренда.
                row.style.cursor = 'pointer';
                row.title = 'Открыть систему ' + (w.name || '');
                row.addEventListener('click', () => {
                    openSystemModal(w.id, w.name, w.spectral_class, null, getAdminToken());
                });

                // ID
                const idCell = document.createElement('td');
                idCell.textContent = w.id;
                row.appendChild(idCell);

                // Имя
                const nameCell = document.createElement('td');
                nameCell.textContent = w.name;
                row.appendChild(nameCell);

                // Координата X
                const xCell = document.createElement('td');
                xCell.textContent = w.coord_x;
                row.appendChild(xCell);

                // Координата Y
                const yCell = document.createElement('td');
                yCell.textContent = w.coord_y;
                row.appendChild(yCell);

                // Население (живое, на «сейчас») + тренд мира.
                const popCell = document.createElement('td');
                popCell.style.fontVariantNumeric = 'tabular-nums';
                popCell.style.whiteSpace = 'nowrap';
                popCell.innerHTML = (w.population ?? 0).toLocaleString('ru-RU') + trendArrow(w.population_trend);
                row.appendChild(popCell);

                // Кнопка удаления
                const btnCell = document.createElement('td');
                const btn = document.createElement('button');
                btn.className = 'btn-small';
                btn.textContent = 'Удалить';
                btn.addEventListener('click', (e) => {
                    // Клик по кнопке — не клик по строке (модалку не открываем).
                    e.stopPropagation();
                    deleteWorld(w.id);
                });
                btnCell.appendChild(btn);
                row.appendChild(btnCell);

                tbody.appendChild(row);
            });
        }

        // Пагинация
        const pagination = document.getElementById('paginationControls');
        pagination.innerHTML = '';
        const total = data?.total ?? 0;
        if (total > 0) {
            const totalPages = Math.ceil(total / data.limit);
            const prev = document.createElement('button');
            prev.textContent = '◀';
            prev.addEventListener('click', () => loadWorlds(Math.max(1, currentPage - 1)));
            pagination.appendChild(prev);

            const span = document.createElement('span');
            span.textContent = `Страница ${currentPage} из ${totalPages}`;
            pagination.appendChild(span);

            const next = document.createElement('button');
            next.textContent = '▶';
            next.addEventListener('click', () => loadWorlds(Math.min(totalPages, currentPage + 1)));
            pagination.appendChild(next);
        }
    } catch (e) {
        clearInterval(timer);
        console.error(e);
        document.getElementById('worldsBody').innerHTML = '<tr><td colspan="6" style="text-align:center;color:#f87171;">Ошибка загрузки</td></tr>';
    }
}

// initWorldsSorting — клик по заголовку «Население»: первое нажатие — по
// возрастанию, следующие переключают asc/desc (как в stats.js), затем
// перезагрузка первой страницы.
export function initWorldsSorting() {
    const th = document.getElementById('sort-population-th');
    if (!th) return;
    th.addEventListener('click', () => {
        if (currentSort !== 'population') {
            currentSort = 'population';
            currentOrder = 'asc';
        } else {
            currentOrder = currentOrder === 'asc' ? 'desc' : 'asc';
        }
        const base = 'Население';
        th.textContent = base + (currentOrder === 'asc' ? ' ▲' : ' ▼');
        loadWorlds(1);
    });
}

export async function deleteWorld(id) {
    if (!confirm('Удалить мир?')) return;
    try {
        const res = await fetchWithAuth('/admin/worlds/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id })
        });
        if (res.ok) {
            notifySuccess('Мир удалён');
            loadStats();
            loadWorlds(currentPage);
        } else {
            const text = await res.text();
            notifyError('Ошибка: ' + text);
        }
    } catch (e) {
        notifyError('Ошибка: ' + e.message);
    }
}

export async function createWorld() {
    const name = document.getElementById('worldName').value.trim();
    const x = parseFloat(document.getElementById('coordX').value);
    const y = parseFloat(document.getElementById('coordY').value);
    const resultEl = document.getElementById('createResult');

    if (!name || isNaN(x) || isNaN(y)) {
        resultEl.textContent = '❌ Заполните все поля';
        return;
    }

    try {
        const res = await fetchWithAuth('/admin/worlds/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, coord_x: x, coord_y: y })
        });

        if (res.ok) {
            resultEl.textContent = '✅ Мир создан';
            loadStats();
            loadWorlds(1);
        } else {
            const text = await res.text();
            resultEl.textContent = '❌ ' + text;
        }
    } catch (e) {
        resultEl.textContent = '❌ ' + e.message;
    }
}