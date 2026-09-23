// web/static/js/search.js
import { state, elements } from './map/config.js';
import { draw } from './map/map_render.js';
import { loadClusters, handleUnauthorized } from './map/data.js';
import { openSystemModal } from './modal/index.js';
import { notifyError } from './ui/toast.js';
import { formatZoom } from './map/utils.js';
// Реестр слоёв (спека 2026-09-23 §5/§6.2): дропдаун результатов — слой полосы
// menu (иначе при открытой модалке уходил под неё, аудит §0); Esc/клик-вне —
// реестр, у дропдауна своих слушателей нет.
import { openLayer } from './ui/layers.js';

const SEARCH_DEBOUNCE_MS = 250;
const MAX_RESULTS = 20;
const FOCUS_ZOOM = 6;

// dropdownHandle — слой дропдауна #entity-search-results (открыт, пока список виден).
let dropdownHandle = null;

// initEntitySearch — включает поиск по точному имени объекта в фильтрах-баре.
export function initEntitySearch() {
    const input = document.getElementById('entity-search');
    if (!input) return;
    const dropdown = document.getElementById('entity-search-results');
    if (!dropdown) return;

    let timer = null;

    input.addEventListener('input', () => {
        clearTimeout(timer);
        timer = setTimeout(() => runSearch(input, dropdown), SEARCH_DEBOUNCE_MS);
    });

    input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            clearTimeout(timer);
            runSearch(input, dropdown);
        }
        // Esc закрывает дропдаун через реестр слоёв (свой обработчик снят,
        // спека §6.2) — здесь остаётся только Enter.
    });

    // Клик вне поля — очистка подсветки найденного мира. Закрытие дропдауна
    // делает реестр (слой полосы menu, closeOnOutside), а не этот слушатель:
    // двойное закрытие и «Esc убил модалку под меню» не возвращаем (§6.3).
    document.addEventListener('click', (e) => {
        if (e.target.closest('.filter-search')) return;
        clearFocus();
    });
}

// ==================== ПОИСК ====================

async function runSearch(input, dropdown) {
    const q = input.value.trim();
    hideDropdown(dropdown);
    if (!q) return;

    const token = localStorage.getItem('token');
    if (!token) {
        handleUnauthorized();
        return;
    }

    try {
        const res = await fetch(
            `/api/entities/search?q=${encodeURIComponent(q)}&limit=${MAX_RESULTS}`,
            { headers: { 'Authorization': 'Bearer ' + token } }
        );
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`);

        const data = await res.json();
        renderDropdown(dropdown, Array.isArray(data.results) ? data.results : []);
    } catch (err) {
        console.error('Entity search error:', err);
        notifyError('Ошибка поиска: ' + err.message);
    }
}

function renderDropdown(dropdown, results) {
    dropdown.innerHTML = '';

    if (results.length === 0) {
        dropdown.textContent = 'Ничего не найдено';
        dropdown.classList.add('empty-msg');
    } else {
        dropdown.classList.remove('empty-msg');
        results.forEach(res => {
            const item = document.createElement('button');
            item.type = 'button';
            item.className = 'entity-search-item';
            item.innerHTML =
                `<span>${kindIcon(res.kind)}</span>` +
                `<span>${escapeHtml(res.name)}</span>` +
                `<span class="entity-search-kind">${kindLabel(res.kind)}</span>`;
            item.addEventListener('mousedown', (e) => {
                e.preventDefault();
                hideDropdown(dropdown);
                focusOnResult(res);
            });
            dropdown.appendChild(item);
        });
    }

    dropdown.hidden = false;
    // Дропдаун — слой полосы menu (спека §6.2): выходит НАД модалкой (1000) и
    // закрывается реестром (Esc/клик-вне) или явным handle.close().
    if (dropdownHandle) { const h = dropdownHandle; dropdownHandle = null; h.close(); }
    dropdownHandle = openLayer(dropdown, {
        level: 'menu',
        closeOnEsc: true,
        closeOnOutside: true,
        onClose: () => { dropdownHandle = null; dropdown.hidden = true; },
    });
}

// hideDropdown — закрытие через слой реестра (без мёртвой записи в стеке);
// фолбэк — спрятать узел без слоя (элемент статичный, не удаляется).
function hideDropdown(dropdown) {
    const el = dropdown || document.getElementById('entity-search-results');
    if (dropdownHandle && (!el || dropdownHandle.el === el)) { dropdownHandle.close(); return; }
    if (el) el.hidden = true;
}

// ==================== ФОКУС ====================

function focusOnResult(res) {
    if (!res || typeof res.coord_x !== 'number' || typeof res.coord_y !== 'number') return;

    focusOnStar(res);

    if (res.kind === 'planet') {
        openSystemModal(res.world_id, res.world_name, res.spectral, { planetId: res.id }, null, {
            // Спрайт игрока для маркера «я здесь» (спека 99.2.27 §5.11):
            // та же иконка/цвет, что на карте (state из map/config.js).
            shipIcon: state.userShipIcon,
            shipColor: state.userShipColor,
        });
    } else if (res.kind === 'satellite') {
        openSystemModal(res.world_id, res.world_name, res.spectral, {
            planetId: res.planet_id,
            satelliteId: res.id,
        }, null, {
            shipIcon: state.userShipIcon,
            shipColor: state.userShipColor,
        });
    }
}

// focusOnStar — центрирует карту на мире объекта и рисует подсветку.
function focusOnStar(res) {
    const { coord_x: x, coord_y: y } = res;

    // Мир в кэш, чтобы рисовался точкой и был кликабелен.
    if (res.world_id && !state.worlds.some(w => w.id === res.world_id)) {
        state.worlds.push({
            id: res.world_id,
            name: res.world_name || res.name,
            // Без фолбека на 'G': у экзотики спектр NULL (баг #1).
            spectral_class: res.spectral || '',
            coord_x: x,
            coord_y: y,
        });
    }

    state.scale = Math.max(state.scale, FOCUS_ZOOM);
    state.offsetX = state.canvasWidth / 2 - x * state.scale;
    state.offsetY = state.canvasHeight / 2 - y * state.scale;
    state.focusWorldId = res.world_id || null;
    state.followShip = false;
    sessionStorage.removeItem('followShip');

    if (elements.zoomInfo) {
        elements.zoomInfo.textContent = formatZoom(state.scale, state.minZoom);
    }

    draw();
    loadClusters().catch(err => console.error('focus reload:', err));
}

// clearFocus — снимает подсветку найденного мира.
export function clearFocus() {
    if (state.focusWorldId) {
        state.focusWorldId = null;
        draw();
    }
}

// ==================== УТИЛИТЫ ====================

function kindLabel(kind) {
    switch (kind) {
        case 'world': return 'звезда';
        case 'planet': return 'планета';
        case 'satellite': return 'спутник';
        default: return 'объект';
    }
}

function kindIcon(kind) {
    switch (kind) {
        case 'world': return '⭐';
        case 'planet': return '🪐';
        case 'satellite': return '🌙';
        default: return '•';
    }
}

function escapeHtml(s) {
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}