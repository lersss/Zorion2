// web/static/js/search.js
import { state, elements } from './map/config.js';
import { draw } from './map/map_render.js';
import { loadClusters, handleUnauthorized } from './map/data.js';
import { openSystemModal } from './modal/index.js';
import { notifyError } from './ui/toast.js';
import { formatZoom } from './map/utils.js';

const SEARCH_DEBOUNCE_MS = 250;
const MAX_RESULTS = 20;
const FOCUS_ZOOM = 6;

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
        if (e.key === 'Escape') {
            hideDropdown(dropdown);
            input.blur();
        }
    });

    // Закрытие по клику вне поля + очистка подсветки фокуса.
    document.addEventListener('click', (e) => {
        if (e.target.closest('.filter-search')) return;
        hideDropdown(dropdown);
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
}

function hideDropdown(dropdown) {
    const el = dropdown || document.getElementById('entity-search-results');
    if (el) el.hidden = true;
}

// ==================== ФОКУС ====================

function focusOnResult(res) {
    if (!res || typeof res.coord_x !== 'number' || typeof res.coord_y !== 'number') return;

    focusOnStar(res);

    if (res.kind === 'planet') {
        openSystemModal(res.world_id, res.world_name, res.spectral, { planetId: res.id });
    } else if (res.kind === 'satellite') {
        openSystemModal(res.world_id, res.world_name, res.spectral, {
            planetId: res.planet_id,
            satelliteId: res.id,
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
            spectral_class: res.spectral || 'G',
            coord_x: x,
            coord_y: y,
        });
    }

    state.scale = Math.max(state.scale, FOCUS_ZOOM);
    state.offsetX = state.canvasWidth / 2 - x * state.scale;
    state.offsetY = state.canvasHeight / 2 - y * state.scale;
    state.focusWorldId = res.world_id || null;
    state.followShip = false;

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