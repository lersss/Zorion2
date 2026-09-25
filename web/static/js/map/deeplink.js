// web/static/js/map/deeplink.js
// Стык «дашборд → карта» (спека 2026-09-26-собственность-игрока-в-дашборде
// §3/§7.2, вариант A): вкладка «Собственность» ведёт на
// /map?system=<world_id>&planet=<planet_id>&wname=<world_name>, карта на старте
// открывает попап системы с фокусом на планете. Явный deeplink важнее неявных
// авто-открывателей (прибытие compositeRoute / возврат beltReturn) — при
// наличии intent'а они подавляются, а оба маркера чистятся (С3). Флаг читает
// map/data.js через shouldSuppressAutoOpen().
//
// Порядок в initMap (web/static/js/main.js): parseIntent → loadUserData →
// loadClusters → handleDashboardDeepLink (§7.2).
//
// Модуль намеренно БЕЗ статических импортов: тяжёлый граф карты/модалки
// (config.js берёт #mapCanvas на верхнем уровне) подключается динамически
// внутри handleDashboardDeepLink. Так чистые хелперы (parseIntentSearch,
// worldFromCache, starInfoFromWorld) тестируются в Node без DOM
// (web/frontend_deeplink_test.go), а data.js может импортировать
// shouldSuppressAutoOpen без цикла.

// pendingIntent — разобранный переход дашборда, живёт между parseIntent() и
// handleDashboardDeepLink() (последний его потребляет). null — обычная карта.
let pendingIntent = null;

// suppressAutoOpen — при наличии intent'а отложенные авто-открыватели
// подавляются (С3), пока handleDashboardDeepLink не потребит intent.
let suppressAutoOpen = false;

// parseIntentSearch — чистый разбор строки запроса: нет system — null (обычная
// карта). planet/wname необязательны; fly=1 — задел ЧК2 («Перелететь»): после
// открытия попапа довести корабль до planetId существующей обёрткой (§8.2).
export function parseIntentSearch(search) {
    const params = new URLSearchParams(search || '');
    const worldId = params.get('system');
    if (!worldId) return null;
    return {
        worldId: worldId,
        planetId: params.get('planet') || null,
        worldName: params.get('wname') || '',
        fly: params.get('fly') === '1',
    };
}

// worldFromCache — мир по id из кэша карты (state.worlds), иначе null.
export function worldFromCache(worlds, worldId) {
    if (!Array.isArray(worlds)) return null;
    return worlds.find(w => w && w.id === worldId) || null;
}

// starInfoFromWorld — starInfo попапа из мира кэша/ответа /worlds/{id} (§7.2):
// stype/stemp/systype/smods/x/y + флаги карты (hasEngine/shipIcon/shipColor, как
// в map/events.js). Полей stemp/smods в кэше карты может не быть — попап честно
// покажет, что есть (при 403 — карточка звезды из доступного).
export function starInfoFromWorld(world, mapState) {
    if (!world) return null;
    const s = mapState || {};
    return {
        stype: world.star_type,
        stemp: world.temperature,
        systype: world.system_type,
        smods: world.stellar_mods,
        x: world.coord_x,
        y: world.coord_y,
        hasEngine: s.hasEngine,
        shipIcon: s.userShipIcon,
        shipColor: s.userShipColor,
    };
}

// parseIntent — разбор location.search ДО loadUserData. Нет system → no-op.
// При наличии intent'а выставляется флаг подавления (С3) — data.js пропустит
// авто-открыватели и вычистит маркеры.
export function parseIntent() {
    const loc = globalThis.location;
    if (!loc) return null;
    pendingIntent = parseIntentSearch(loc.search);
    suppressAutoOpen = !!pendingIntent;
    return pendingIntent;
}

// shouldSuppressAutoOpen — флаг подавления для map/data.js (С3).
export function shouldSuppressAutoOpen() {
    return suppressAutoOpen;
}

// isModalReadyForFly — готов ли попап к запуску маршрута (§8.2): modalState уже
// знает запрошенный мир, массив планет заполнен (renderModal выполнен) и /me
// отработал (meLoaded — interstellarFlight заполняется асинхронно из /me;
// без него flightModeForSystem() мог бы ошибочно выбрать 'intra'). Чистый
// предикат — тестируется в Node без DOM/таймеров (frontend_deeplink_test.go).
export function isModalReadyForFly(modalState, worldId) {
    return !!modalState && modalState.worldId === worldId && Array.isArray(modalState.planets)
        && modalState.meLoaded === true;
}

// planetInModalList — есть ли целевая планета в загруженном списке попапа (§8.2):
// при ограниченной/пустой системе (403-карточка звезды, planets === []) маршрут
// не запускаем — попап уже честно показывает, что данных нет, а flyToPlanet дал
// бы вводящий в заблуждение тост «планета не найдена». Чистый предикат (Node-тест).
export function planetInModalList(planets, planetId) {
    return Array.isArray(planets) && planets.some(p => p && p.id === planetId);
}

// waitForModalReady — дождаться готовности попапа после openSystemModal (планеты
// и /me грузятся асинхронно): опрос modalState шагом 150 мс, общий таймаут 5 с
// (§8.2). Таймаут — просто false (ничего не делаем, без зависания). Тяжёлый граф
// модалки подключается динамически — модуль Node-безопасен (см. шапку).
async function waitForModalReady(worldId) {
    const { modalState } = await import('../modal/state.js');
    if (isModalReadyForFly(modalState, worldId)) return true;
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline) {
        await new Promise(resolve => setTimeout(resolve, 150));
        if (isModalReadyForFly(modalState, worldId)) return true;
    }
    return false;
}

// handleDashboardDeepLink — вызывается из initMap ПОСЛЕ loadClusters (§7.2).
// Разрешение мира (С2, только реально доступное): мир в кэше карты → попап
// сразу; иначе fetchWorldByID — 200 → попап с тем, что есть; null (403/нет) →
// попап НЕ открываем, тост «<wname>: система вне зоны видимости — данных нет».
// intent потребляется, URL затирается (F5 не откроет попап заново).
export async function handleDashboardDeepLink() {
    const current = pendingIntent;
    pendingIntent = null;
    suppressAutoOpen = false;
    if (!current) return;

    // Тяжёлый граф — только на живой карте (модуль Node-безопасен, см. шапку).
    const { state } = await import('./config.js');
    const { openSystemModal } = await import('../modal/index.js');

    const focusOpts = current.planetId ? { planetId: current.planetId } : {};
    let world = worldFromCache(state.worlds, current.worldId);
    if (!world) {
        const { fetchWorldByID } = await import('./data.js');
        const token = globalThis.localStorage ? globalThis.localStorage.getItem('token') : null;
        world = await fetchWorldByID(current.worldId, token);
    }

    if (world) {
        openSystemModal(current.worldId, world.name || '—', world.spectral_class || '', focusOpts, null,
            starInfoFromWorld(world, state));
        // ЧК2 (§8.2): &fly=1 — довести до планеты существующей обёрткой, когда
        // попап заполнит modalState (планеты + /me). fly без planetId — обычное
        // открытие; таймаут ожидания — тихо ничего (без зависания). Целевой
        // планеты нет в списке (ограниченная система/битая цель) — тихо ничего:
        // попап сам честно показывает, что данных нет.
        if (current.fly && current.planetId) {
            if (await waitForModalReady(current.worldId)) {
                const { modalState } = await import('../modal/state.js');
                if (planetInModalList(modalState.planets, current.planetId)) {
                    const { flyToPlanet } = await import('../modal/events.js');
                    await flyToPlanet(current.planetId);
                }
            }
        }
    } else {
        const { notifyError } = await import('../ui/toast.js');
        notifyError((current.worldName || current.worldId) + ': система вне зоны видимости — данных нет');
    }

    const hist = globalThis.history;
    if (hist && hist.replaceState) hist.replaceState({}, '', '/map');
}
