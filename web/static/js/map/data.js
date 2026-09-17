// web/static/js/map/data.js
import { state, elements } from './config.js';
import { draw, setShipIcon, setShipColor, galaxyRadiusFromRegions, updateFitZoom } from './map_render.js';
import { setShipOptions } from './ship_sprites.js';
import { filterState } from '../filters.js';

// Размер ячейки кластеризации на экране, в пикселях.
export const CLUSTER_CELL_PX = 40;

// Задержка перед перезапросом после zoom/pan.
const RELOAD_DEBOUNCE_MS = 180;

// Минимальный интервал между полётными перезапросами кластеров (B17):
// во время полёта рендер-цикл крутится каждый кадр, без гарда 600 мс
// запросы шли бы до ~5/с на игрока.
const FLIGHT_RELOAD_INTERVAL_MS = 600;

// Порог «край вьюпорта вышел за загруженную область»: 25% размера вьюпорта
// по оси — рендер-цикл запрашивает новый район, когда с края открылось
// заметное поле.
const FLIGHT_RELOAD_MARGIN_RATIO = 0.25;

let loadingData = false;
let pendingReload = false;
let reloadTimer = null;
let currentWorldIdLoaded = false;
let lastFlightReloadAt = 0;

// Вейтеры, ждущие освобождения loadingData (фикс 33b): ветка прибытия
// дожидается конца полётного перезапроса, чтобы loadClusters не потерялся.
const loadIdleWaiters = [];

// lastFetchedBounds — границы вьюпорта, для которых загружены кластеры.
// Полётный цикл сравнивает с ними текущий вьюпорт, чтобы не дёргать API
// на каждом кадре (B17).
let lastFetchedBounds = null;

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

// maybeReloadClusters — полётный перезапрос кластеров (B17): рендер-цикл во
// время полёта двигает вьюпорт, но мышиные события (и scheduleReload) не
// приходят, поэтому звёзды за краем не появлялись. Вызывает loadClusters
// НАПРЯМУЮ (мимо общего reloadTimer), чтобы не отодвигать дебаунс релоадов
// от пана/зума игрока. Ограничено сверху: 25% края + интервалом 600 мс.
// Во время полёта запрашивает и сравнивает по одной расширенной зоне
// (B26): текущий вьюпорт + полоса впереди по курсу ~1.5 с полёта
// (скорость = dist(flyFrom, flyTo) / flyDuration), чтобы следующая зона
// была загружена до прилёта, а не «под носом».
export function maybeReloadClusters() {
    if (loadingData) return;
    const bounds = getFlightBounds();
    const w = bounds.xMax - bounds.xMin;
    const h = bounds.yMax - bounds.yMin;
    const need = !lastFetchedBounds ||
        bounds.xMin < lastFetchedBounds.xMin - FLIGHT_RELOAD_MARGIN_RATIO * w ||
        bounds.xMax > lastFetchedBounds.xMax + FLIGHT_RELOAD_MARGIN_RATIO * w ||
        bounds.yMin < lastFetchedBounds.yMin - FLIGHT_RELOAD_MARGIN_RATIO * h ||
        bounds.yMax > lastFetchedBounds.yMax + FLIGHT_RELOAD_MARGIN_RATIO * h;
    if (!need) return;
    const now = Date.now();
    if (now - lastFlightReloadAt < FLIGHT_RELOAD_INTERVAL_MS) return;
    lastFlightReloadAt = now;
    loadClusters(bounds).catch(err => console.error('maybeReloadClusters:', err));
}

// resetFlightReloadTimer — сброс полётного таймера перезапросов (фикс 33c):
// между полётами lastFlightReloadAt должен обнуляться, иначе новый полёт,
// начатый в пределах FLIGHT_RELOAD_INTERVAL_MS после предыдущего, пропустит
// первую полётную подгрузку кластеров (гард в maybeReloadClusters).
export function resetFlightReloadTimer() {
    lastFlightReloadAt = 0;
}

// ==================== ЗАГРУЗКА КЛАСТЕРОВ ====================

// notifyLoadIdle — резолвит всех вейтеров waitForLoadIdle после
// освобождения loadingData (фикс 33b).
function notifyLoadIdle() {
    while (loadIdleWaiters.length) {
        loadIdleWaiters.pop()();
    }
}

// waitForLoadIdle — ожидание освобождения loadingData (фикс 33b): ветка
// прибытия ждёт конец полётного перезапроса, чтобы loadClusters() не увидел
// loadingData = true и не потерял загрузку зоны прибытия через pendingReload.
// timeoutMs — защита от зависшего fetch (в коде нет таймаутов fetch).
export function waitForLoadIdle(timeoutMs = 2000) {
    if (!loadingData) return Promise.resolve();
    return new Promise(resolve => {
        const timer = setTimeout(() => resolve(), timeoutMs);
        loadIdleWaiters.push(() => {
            clearTimeout(timer);
            resolve();
        });
    });
}

export async function loadClusters(boundsOverride = null) {
    if (loadingData) {
        pendingReload = true;
        return;
    }
    loadingData = true;

    try {
        const token = localStorage.getItem('token');
        if (!token) throw new Error('No token');

        const bounds = boundsOverride || getViewportBounds();
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
        lastFetchedBounds = bounds;

        for (const c of state.clusters) {
            if (c.cnt === 1 && c.sid) {
                if (!state.worlds.some(w => w.id === c.sid)) {
                    state.worlds.push({
                        id: c.sid,
                        name: c.sname || '—',
                        // Без фолбека на 'G': у экзотики sspec пустой (NULL),
                        // цвет берётся по star_type (баг #1).
                        spectral_class: c.sspec || '',
                        star_type: c.stype || 'star',
                        system_type: c.systype || 'single',
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
        notifyLoadIdle();
        if (pendingReload) {
            pendingReload = false;
            scheduleReload();
        }
    }

    // Хвостовая перерисовка после загрузки. На полётном перезапросе
    // (maybeReloadClusters → boundsOverride задан) её НЕ делаем: полётный
    // rAF-цикл сам рисует кадр, а draw() из сетевого колбэка вне цикла даёт
    // лишнюю/нестабильную перерисовку. Вне полёта — как раньше.
    if (!boundsOverride) {
        draw();
    }
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
        if (state.regions.length >= 1) {
            state.galaxyRadius = galaxyRadiusFromRegions(state.regions);
            // Во время полёта зум не трогаем: updateFitZoom() посреди пути
            // мог скачком сдвинуть state.scale (перезапрос кластеров).
            // Вне полёта работает как раньше.
            if (!state.isFlying) {
                updateFitZoom();
            }
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

// getFlightBounds — вьюпорт для полётного перезапроса (B26): объединение
// текущего вьюпорта и смещённого вперёд по курсу flyFrom → flyTo на ~1.5 с
// полёта (min/max по осям) — экран не пустеет, а полоса впереди по курсу
// загружается до прилёта. Вне полёта (вызова нет — функция полётная) —
// обычный вьюпорт без смещения.
function getFlightBounds() {
    const bounds = getViewportBounds();
    if (!state.isFlying || !state.flyFrom || !state.flyTo || !(state.flyDuration > 0)) {
        return bounds;
    }
    // Курс и скорость — от стартовой точки сегмента (61a): при редиректе
    // это точка P маршрута, а не мир отправления.
    const sx = (typeof state.flyStartX === 'number') ? state.flyStartX : state.flyFrom.coord_x;
    const sy = (typeof state.flyStartY === 'number') ? state.flyStartY : state.flyFrom.coord_y;
    const dx = state.flyTo.coord_x - sx;
    const dy = state.flyTo.coord_y - sy;
    const speed = Math.hypot(dx, dy) / state.flyDuration;
    const shift = speed * 1.5;
    const dist = Math.hypot(dx, dy) || 1;
    const shiftX = (dx / dist) * shift;
    const shiftY = (dy / dist) * shift;
    return {
        xMin: Math.min(bounds.xMin, bounds.xMin + shiftX),
        xMax: Math.max(bounds.xMax, bounds.xMax + shiftX),
        yMin: Math.min(bounds.yMin, bounds.yMin + shiftY),
        yMax: Math.max(bounds.yMax, bounds.yMax + shiftY),
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

// Период опроса позиций чужих игроков (спека 77a §5.3): согласованно с
// полётным циклом карты.
const PLAYER_POSITIONS_POLL_MS = 5000;

let playerPositionsLoopStarted = false;

// loadPlayerPositions — позиции чужих игроков в радиусе радара (спека 77a
// §5.3). Сервер отдаёт только игроков в радиусе (И1) — клиент рисует как есть.
export async function loadPlayerPositions() {
    const token = localStorage.getItem('token');
    if (!token) return;
    try {
        const res = await fetch('/api/players/positions', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        state.playerPositions = Array.isArray(data.players) ? data.players : [];
        draw();
    } catch (err) {
        // Тихий сбой: карта работает и без чужих игроков.
        console.warn('loadPlayerPositions error:', err);
    }
}

// startPlayerPositionsLoop — периодический опрос позиций чужих игроков.
export function startPlayerPositionsLoop() {
    if (playerPositionsLoopStarted) return;
    playerPositionsLoopStarted = true;
    loadPlayerPositions();
    setInterval(loadPlayerPositions, PLAYER_POSITIONS_POLL_MS);
}

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

        if (user.id) state.userId = user.id;
        if (user.role) state.userRole = user.role;
        // Радиус радара (спека 77a §4.2): для отрисовки границы видимости.
        if (typeof user.radar_radius === 'number') state.radarRadius = user.radar_radius;
        if (user.ship_icon) {
            setShipIcon(user.ship_icon);
        }
        // Цвет перекраски (NULL = «Оригинал») и порядок реестра спрайтов
        // для spriteForAgent (спека 61b §6.1/§5.6).
        setShipColor(user.ship_color);
        setShipOptions(user.ship_options);

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
                // Явно возвращаем префикс «Текущий мир:» — до этого шапка
                // могла показывать «В полёте:» (61a).
                const prefix = document.getElementById('currentWorldPrefix');
                if (prefix) prefix.textContent = 'Текущий мир:';
                const label = document.getElementById('currentWorldName');
                if (label) label.textContent = world.name;
            }
        }

        // Восстановление полёта после рефреша (идея 42a): сервер помнит
        // полёт в travel.Manager, /me отдаёт его в user.flight. Восстанавливаем
        // state.* — панель (main.js) и animationLoop подхватят как обычный полёт.
        if (user.flight) {
            const [fromWorld, toWorld] = await Promise.all([
                fetchWorldByID(user.flight.from, token),
                fetchWorldByID(user.flight.to, token),
            ]);
            if (fromWorld && toWorld) {
                if (!state.worlds.find(w => w.id === fromWorld.id)) state.worlds.push(fromWorld);
                if (!state.worlds.find(w => w.id === toWorld.id)) state.worlds.push(toWorld);
                state.flyFrom = fromWorld;
                state.flyTo = toWorld;
                state.flyStartTime = user.flight.start_time;
                state.flyDuration = user.flight.duration;
                // Стартовая точка сегмента (61a); фолбэк на fromWorld, если
                // start_x/start_y нет (старый сервер/полёт без редиректа).
                state.flyStartX = (typeof user.flight.start_x === 'number') ? user.flight.start_x : fromWorld.coord_x;
                state.flyStartY = (typeof user.flight.start_y === 'number') ? user.flight.start_y : fromWorld.coord_y;
                state.isFlying = true;
                // Шапка: «В полёте: From → To» (61a). Напрямую, без импорта
                // из flight.js — чтобы не плодить циклы между модулями.
                const prefix = document.getElementById('currentWorldPrefix');
                if (prefix) prefix.textContent = 'В полёте:';
                const label = document.getElementById('currentWorldName');
                if (label) label.textContent = fromWorld.name + ' → ' + toWorld.name;
            } else {
                // Мир from/to не загрузился (удалён при перегенерации) —
                // полёт не восстанавливаем, оставляем как было.
                console.warn('loadUserData: не удалось восстановить полёт — миры from/to не загрузились');
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
        if (res.status === 401) {
            handleUnauthorized();
            return null;
        }
        if (res.status === 403) {
            // Детали системы закрыты вне радиуса/знания (спека 77a §5.5/И11):
            // /worlds/{id} за-радарной звезды — 403. Не разлогиниваем —
            // возвращаем null (вызывающий обрабатывает: кэш/фолбэк).
            console.warn('fetchWorldByID 403 (вне зоны видимости) for', id);
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
            // Без фолбека на 'G': у экзотики спектр NULL (баг #1).
            spectral_class: w.spectral_class || '',
            star_type: w.star_type || 'star',
            system_type: w.system_type || 'single',
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