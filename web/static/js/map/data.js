// web/static/js/map/data.js
import { state, elements } from './config.js';
import { draw, setShipIcon, setShipColor, galaxyRadiusFromRegions, updateFitZoom } from './map_render.js';
import { setShipOptions } from './ship_sprites.js';
import { filterState } from '../filters.js';
// Модалка системы (спека 99.2.30 §6.8): автооткрытие по прибытии композитного
// маршрута — только на странице карты (data.js — модуль карты). modalState —
// для проверки «модалка уже открыта на системе прибытия».
import { modalState } from '../modal/state.js';
import { openSystemModal, closeModal, refreshPlanets } from '../modal/index.js';
import { notifyInfo } from '../ui/toast.js';
import { startFlightHum, playArrival } from '../ui/sound.js';
// Подпись баланса в шапке карты (идея 2026-09-23 §5.1): единый форматтер
// «… Cr» из модуля денег — второй форматтер не заводим.
import { moneyLabel } from '../dashboard/money.js';

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

// loadAccountBalance — баланс игрока в шапку карты (идея 2026-09-23 §5.1).
// Источник — отдельный GET /me/money (эндпоинт §3.2/О-д4 уже есть; контракт
// /me не трогаем). Показываем только свой счёт (канон 14_money.md §14.4).
// Тихий сбой: без баланса карта работает, в шапке остаётся «—».
async function loadAccountBalance(token) {
    const el = document.getElementById('accountBalance');
    if (!el) return;
    try {
        const res = await fetch('/me/money', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (res.status === 401 || res.status === 403) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) return;
        const data = await res.json();
        el.textContent = moneyLabel(data && data.balance);
    } catch (e) {
        console.warn('loadAccountBalance error:', e);
    }
}

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
        // Пакман (2026-09-20): снапшот сервера НЕ пересобирается на каждый
        // батч — полный ответ вычитаем по eatenIds, НО «долетающие» звёзды
        // (pendingEatenIds: пакман ещё летит к ним) остаются видимыми, чтобы
        // лопнуть в момент его прибытия (не «сами» и не заранее).
        const pacmanEaten = (state.pacman && state.pacman.eatenIds) || new Set();
        const pacmanPending = (state.pacman && state.pacman.pendingEatenIds) || new Set();
        let clustersArr = Array.isArray(clusters) ? clusters : [];
        if (pacmanEaten.size > 0) {
            clustersArr = clustersArr.filter(c => !(c.cnt === 1 && c.sid &&
                pacmanEaten.has(c.sid) && !pacmanPending.has(c.sid)));
        }
        state.clusters = clustersArr;
        lastFetchedBounds = bounds;

        for (const c of state.clusters) {
            if (c.cnt === 1 && c.sid) {
                if (pacmanEaten.has(c.sid) && !pacmanPending.has(c.sid)) continue; // съедено и лопнуло
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
        // Кнопка «В админку» (пожелание 2026-09-19): видна только админским
        // ролям; обычным игрокам не показывается вовсе.
        const adminNavBtn = document.getElementById('adminNavBtn');
        if (adminNavBtn) {
            adminNavBtn.style.display = (user.role === 'admin' || user.role === 'skycomposer') ? '' : 'none';
        }
        // Радиус радара (спека 77a §4.2): для отрисовки границы видимости.
        if (typeof user.radar_radius === 'number') state.radarRadius = user.radar_radius;
        // Двигатель (спека 91a §6.1): без установленного двигателя полёт
        // невозможен — блокируем кнопки полёта в UI (сервер валидирует тоже).
        // Валидный двигатель — предмет в слоте engine типа 'engine' из каталога.
        const shipCatalog = Array.isArray(user.ship_catalog) ? user.ship_catalog : [];
        const engineId = user.equipment && user.equipment.engine;
        const engineItem = engineId ? shipCatalog.find(i => i.id === engineId) : null;
        state.hasEngine = !!(engineItem && engineItem.type === 'engine');
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

        // Баланс в шапку карты (идея 2026-09-23 §5.1) — без await, чтобы
        // загрузка карты не блокировалась вторым запросом.
        loadAccountBalance(token);

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
        // Спека 99.2.30 §6.8 (триггер B): возврат/рефреш — первый loadUserData.
        // Распознавание автостарта композитного маршрута: маркер compositeRoute
        // совпал с current_world_id (автостарт идёт или завершён) → автооткрытие
        // модалки; маркер есть, но полёта к нему нет и мир не совпал → маркер
        // устарел (игрок явно ушёл от маршрута).
        if (!currentWorldIdLoaded && user.current_world_id) {
            const marker = sessionStorage.getItem('compositeRoute');
            if (marker) {
                const enRoute = user.flight && user.flight.to === marker;
                if (enRoute) {
                    // Ещё летим к системе маршрута — маркер живёт (автооткрытие по прибытии).
                } else if (marker === user.current_world_id) {
                    checkCompositeArrival(user);
                } else {
                    sessionStorage.removeItem('compositeRoute');
                }
            }
        }
        currentWorldIdLoaded = true;
        return user;
    } catch (e) {
        console.warn('loadUserData error:', e);
    }
}

// ==================== КОМПОЗИТНЫЙ МАРШРУТ: АВТООТКРЫТИЕ ПО ПРИБЫТИИ (спека 99.2.30 §6.8) ====================

// checkCompositeArrival — распознавание автостарта композитного маршрута (M5):
// три серверных признака в порядке — (1) pending_destination в /me и
// pending_destination.world_id == current_world_id (намерение пережило
// клиентский таймер — гонка §6.3); (2) current_position.status == 'in_flight'
// (автостарт уже запущен сервером); (3) сессионный маркер compositeRoute ==
// world_id (намерение уже исполнено — автостарт идёт или завершён, а клиент
// вернулся на карту с задержкой). Автооткрытие модалки системы прибытия:
// открыта другая система — закрыть; открыта система прибытия — только
// refreshPlanets(). Фокус по current_position.to_type/to_id (планета →
// planetId; спутник → satelliteId); позиция уже orbit — фокус на объекте
// позиции. Тост «🚀 Прибыли в систему <X>» (notifyInfo, ~4 с). Маркер
// стирается после потребления. Гонка таймеров (клиентский раньше серверного
// onArrival): автооткрытие по признаку 1 — повторный /me-чек через ~1 с,
// одноразово, конвергентно (полоса появится → refreshPlanets()).
export async function checkCompositeArrival(user) {
    const worldId = user && user.current_world_id;
    if (!worldId) return;
    const marker = sessionStorage.getItem('compositeRoute');
    const pending = user.pending_destination;
    const pos = user.current_position;
    const sign1 = !!(pending && pending.world_id === worldId);
    const sign2 = !!(pos && pos.status === 'in_flight');
    const sign3 = marker === worldId;
    if (!sign1 && !sign2 && !sign3) return;

    // Гул полёта через сегменты (99.2.30/99.2.27): маршрут ещё продолжается
    // (намерение — sign1, или внутрисистемный сегмент уже идёт — sign2) — гул
    // держим (непрерывность через стык, «прибытие» не сигналим); распознан
    // только по маркеру (sign3, намерение исполнено, позиция orbit/surface) —
    // конец маршрута: глушим гул и сигналим прибытие один раз. Тост notifyInfo
    // молчит (решение В2=А) — дубля звука прибытия нет.
    if (sign1 || sign2) {
        startFlightHum();
    } else {
        playArrival();
    }

    // Маркер потреблён (автооткрытие) — стираем.
    if (sign3) sessionStorage.removeItem('compositeRoute');

    // Имя мира прибытия — для тоста и модалки (кэш карты, фолбэк — запрос).
    let world = state.worlds.find(w => w.id === worldId);
    if (!world) {
        const token = localStorage.getItem('token');
        world = await fetchWorldByID(worldId, token);
        if (world) state.worlds.push(world);
    }
    const worldName = world ? world.name : '—';

    // Фокус по current_position.to_type/to_id; позиция уже orbit (успел
    // долететь за время отсутствия) — фокус на объекте позиции.
    // Пояс (спека поясов этап 2 §7.2): канвас-координат нет — модалка
    // открывается, фокус не ставится (бейдж «вы в поясе» — в секции).
    const focusOpts = {};
    if (pos && pos.status === 'in_flight') {
        if (pos.to_type === 'planet') focusOpts.planetId = pos.to_id;
        else if (pos.to_type === 'satellite') focusOpts.satelliteId = pos.to_id;
    } else if (pos && (pos.status === 'orbit' || pos.status === 'surface')) {
        // surface: фокус на планете прогулки (спека 2026-09-21 §7.6 п.5).
        if (pos.object_type === 'planet') focusOpts.planetId = pos.object_id;
        else if (pos.object_type === 'satellite') focusOpts.satelliteId = pos.object_id;
    }

    // Модалка уже открыта на системе прибытия — не переоткрывать, только
    // refreshPlanets() (модалка подхватит полосу из my_position).
    if (document.getElementById('system-modal-overlay') && modalState.worldId === worldId) {
        refreshPlanets();
        notifyInfo('🚀 Прибыли в систему ' + worldName);
        // Гонка таймеров: автооткрытие по признаку 1 (позиция ещё не in_flight) —
        // повторный /me-чек через ~1 с, одноразово, конвергентно.
        if (sign1 && !sign2) scheduleCompositeArrivalRecheck();
        return;
    }
    // Модалка открыта на другой системе — закрыть (игрок физически в системе
    // прибытия, «камера там, где игрок», решение 1).
    if (document.getElementById('system-modal-overlay')) {
        closeModal();
    }
    openSystemModal(worldId, worldName, world ? world.spectral_class : '', focusOpts, null, {
        hasEngine: state.hasEngine,
        shipIcon: state.userShipIcon,
        shipColor: state.userShipColor,
    });
    notifyInfo('🚀 Прибыли в систему ' + worldName);
    // Гонка таймеров — повторный /me-чек (см. выше).
    if (sign1 && !sign2) scheduleCompositeArrivalRecheck();
}

// scheduleCompositeArrivalRecheck — повторный /me-чек через ~1 с (спека 99.2.30
// §6.8): автооткрытие по признаку 1 (намерение есть, позиция ещё не in_flight) —
// модалка подхватит старт своим refreshPlanets(); если через ~1 с полоса не
// появилась (позиция всё ещё не in_flight) — повторный чек, при in_flight →
// refreshPlanets(). Одноразово, конвергентно: крайнее окно захлопывается за
// ≤ 2 циклов.
function scheduleCompositeArrivalRecheck() {
    setTimeout(async () => {
        try {
            const token = localStorage.getItem('token');
            if (!token) return;
            const res = await fetch('/me', { headers: { 'Authorization': 'Bearer ' + token } });
            if (!res.ok) return;
            const me = await res.json();
            if (me.current_position && me.current_position.status === 'in_flight' &&
                document.getElementById('system-modal-overlay')) {
                refreshPlanets();
            }
        } catch (e) {
            // Тихий сбой: модалка подхватит старт своим refreshPlanets().
        }
    }, 1000);
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