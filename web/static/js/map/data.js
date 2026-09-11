// web/static/js/map/data.js
import { state, elements } from './config.js';
import { draw, setShipIcon, galaxyRadiusFromRegions, updateFitZoom } from './map_render.js';
import { filterState } from '../filters.js';

// Размер ячейки кластеризации на экране, в пикселях.
export const CLUSTER_CELL_PX = 40;

// Задержка перед перезапросом после zoom/pan.
const RELOAD_DEBOUNCE_MS = 180;

let loadingData = false;
let pendingReload = false;
let reloadTimer = null;
let currentWorldIdLoaded = false;

// ==================== АВТОРИЗАЦИЯ ====================

// handleUnauthorized — универсальный обработчик 401/403.
// Чистит токен и редиректит на логин (защита от зацикливания).
export function handleUnauthorized() {
    localStorage.removeItem('token');
    if (window.location.pathname !== '/login-page') {
        window.location.href = '/login-page';
    }
}

// ==================== DEBOUNCE ====================

export function scheduleReload() {
    if (reloadTimer) clearTimeout(reloadTimer);
    reloadTimer = setTimeout(() => {
        reloadTimer = null;
        loadClusters().catch(err => console.error('scheduleReload:', err));
    }, RELOAD_DEBOUNCE_MS);
}

// ==================== ЗАГРУЗКА КЛАСТЕРОВ ====================

export async function loadClusters() {
    if (loadingData) {
        pendingReload = true;
        return;
    }
    loadingData = true;

    try {
        const token = localStorage.getItem('token');
        if (!token) throw new Error('No token');

        const bounds = getViewportBounds();
        const cell = getCellSize();
        const url = buildUrl(bounds, cell);

        const res = await fetch(url, {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`);

        const clusters = await res.json();
        state.clusters = Array.isArray(clusters) ? clusters : [];

        for (const c of state.clusters) {
            if (c.cnt === 1 && c.sid) {
                if (!state.worlds.some(w => w.id === c.sid)) {
                    state.worlds.push({
                        id: c.sid,
                        name: c.sname || '—',
                        spectral_class: c.sspec || 'G',
                        coord_x: c.x,
                        coord_y: c.y,
                    });
                }
            }
        }

        if (elements.loading) elements.loading.style.display = 'none';

        await loadRegions();
    } catch (err) {
        console.error('loadClusters error:', err);
        if (elements.statusBar) elements.statusBar.textContent = '❌ Ошибка: ' + err.message;
    } finally {
        loadingData = false;
        if (pendingReload) {
            pendingReload = false;
            scheduleReload();
        }
    }

    draw();
}

// loadRegions — подгружает все регионы галактики.
// Нужны все центры: диаграмма Вороного на фронте зависит от соседей.
// Запрос дешёвый (регионов сотни).
async function loadRegions() {
    try {
        const token = localStorage.getItem('token');
        if (!token) throw new Error('No token');

        const res = await fetch('/api/regions', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`);

        const regions = await res.json();
        state.regions = Array.isArray(regions) ? regions : [];
        if (state.regions.length >= 2) {
            state.galaxyRadius = galaxyRadiusFromRegions(state.regions);
            updateFitZoom();
        }
    } catch (err) {
        console.error('loadRegions error:', err);
    }
}

// ==================== ГРАНИЦЫ VIEWPORT ====================

function getViewportBounds() {
    const invScale = 1 / state.scale;
    return {
        xMin: -state.offsetX * invScale,
        xMax: (state.canvasWidth - state.offsetX) * invScale,
        yMin: -state.offsetY * invScale,
        yMax: (state.canvasHeight - state.offsetY) * invScale,
    };
}

function getCellSize() {
    return CLUSTER_CELL_PX / state.scale;
}

// ==================== URL ====================

function buildUrl(bounds, cell) {
    const params = new URLSearchParams();
    params.set('x_min', bounds.xMin.toFixed(3));
    params.set('x_max', bounds.xMax.toFixed(3));
    params.set('y_min', bounds.yMin.toFixed(3));
    params.set('y_max', bounds.yMax.toFixed(3));
    params.set('cell', cell.toFixed(3));

    if (filterState.hasPlanets) params.set('has_planets', 'true');
    if (filterState.hasLife) params.set('has_life', 'true');
    if (filterState.hasHabitable) params.set('has_habitable', 'true');
    if (filterState.planetType) params.set('planet_type', filterState.planetType);
    if (filterState.resourceCategory) params.set('resource_category', filterState.resourceCategory);

    return '/api/worlds/filter?' + params.toString();
}

// ==================== ПОЛЬЗОВАТЕЛЬ ====================

export async function loadUserData(force = false) {
    if (currentWorldIdLoaded && !force) return;
    try {
        const token = localStorage.getItem('token');
        if (!token) {
            handleUnauthorized();
            return;
        }
        const res = await fetch('/me', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) {
            console.warn('loadUserData: HTTP', res.status);
            return;
        }
        const user = await res.json();

        if (user.ship_icon) {
            setShipIcon(user.ship_icon);
        }

        if (user.current_world_id) {
            state.currentWorldId = user.current_world_id;

            let world = state.worlds.find(w => w.id === user.current_world_id);
            if (!world) {
                world = await fetchWorldByID(user.current_world_id, token);
                if (world) {
                    state.worlds.push(world);
                }
            }

            if (world) {
                const label = document.getElementById('currentWorldName');
                if (label) label.textContent = world.name;
            }
        }
        currentWorldIdLoaded = true;
    } catch (e) {
        console.warn('loadUserData error:', e);
    }
}

// fetchWorldByID — загружает один мир по ID через /worlds/{id}.
export async function fetchWorldByID(id, token) {
    try {
        const res = await fetch('/worlds/' + encodeURIComponent(id), {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return null;
        }
        if (!res.ok) {
            console.warn('fetchWorldByID HTTP', res.status, 'for', id);
            return null;
        }
        const payload = await res.json();
        const w = payload.world || payload;
        if (!w || !w.id) return null;
        return {
            id: w.id,
            name: w.name || '—',
            spectral_class: w.spectral_class || 'G',
            coord_x: w.coord_x,
            coord_y: w.coord_y,
        };
    } catch (e) {
        console.warn('fetchWorldByID error:', e);
        return null;
    }
}

// ==================== ОБРАТНАЯ СОВМЕСТИМОСТЬ ====================

export function loadData() {
    return loadClusters();
}

export function filterWorlds(worlds) {
    return worlds || [];
}