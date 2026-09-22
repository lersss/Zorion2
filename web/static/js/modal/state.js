// web/static/js/modal/state.js
export const modalState = {
    zoom: 1,
    offsetX: 0,
    offsetY: 0,
    // Слежение камеры за кораблём в полёте (запрос создателя 99.2.27):
    // отдельное смещение отрисовки (не трогает offsetX/Y — drag/zoom/миникарта
    // работают как раньше). Lerp-ом к цели во время полёта, к 0 по прибытии.
    followOffsetX: 0,
    followOffsetY: 0,
    // followEnabled — режим слежения (паттерн 42a + решение менеджера 99.2.27):
    // по умолчанию вкл при полёте (авто-слежение); кнопка «Найти меня»
    // переключает вкл/выкл; подсветка кнопки = слежение активно.
    followEnabled: true,
    // followExact — точный центр на корабль (после клика «Найти меня»): без
    // смещения к цели; сбрасывается при ручном пане/зуме пользователя.
    followExact: false,
    // followDirty — пользователь панировал/зумил (запрос создателя «слежение
    // невозможно отключить»): dead-zone держит корабль в зоне ±25% — эвристика
    // «корабль у центра» (offCenter) почти всегда false → выключение не
    // срабатывало. Теперь: клик при followDirty → центр на корабль; клик без
    // followDirty (пользователь не трогал вид) → выключить слежение.
    followDirty: false,
// arrivalObject — объект прибытия (запрос создателя 99.2.27): после прилёта
// камера мягко центрирует на него и стоит (планета медленно орбитирует —
// камера отслеживает); ручной пан/зум сбрасывает — камера свободна.
arrivalObject: null,
    isDragging: false,
    dragStartX: 0,
    dragStartY: 0,
    dragStartOffsetX: 0,
    dragStartOffsetY: 0,
    hoveredObject: null,
    selectedObject: null,      // { type: 'star' } | { type: 'planet', index }
    selectedPlanetIndex: null,
    // Выбранный спутник (баг 2026-09-21: карточка спутника сбрасывалась на
    // карточку планеты при фоновом обновлении). Хранит { planetId, satelliteId }
    // — по id, чтобы пережить перечитку данных (refreshPlanets). null — карточка
    // спутника закрыта (показана вкладка карточки планеты).
    selectedSatellite: null,
    canvasWidth: 0,
    canvasHeight: 0,
    starRadius: 60,
    starColor: '#fff4a3',
    systemType: 'single',      // single/binary/multiple (35a)
    binaryType: '',            // wide/close (35a)
    companion: '',             // спектр компаньона (35b §6.6)
    companionColor: '',        // цвет компаньона по спектру (35a)
    companionMass: null,       // масса компаньона M☉ (35b)
    companionTemp: null,       // температура компаньона K (35b)
    companionSepAU: null,      // разделение пары а.е. (35b)
    extraCompanions: [],       // внешние компаньоны кратных (35b)
    planets: [],
    // Пояса малых тел системы (спека поясов этап 2 §4.1): belts из ответа
    // /api/worlds/{id}/planets. Для player — только visible=true, состав при
    // знании; для admin — целиком. Пусто — «Поясов нет».
    belts: [],
    restricted: false,         // модалка без деталей системы (403, спека 77a §5.5/И11): звезда открыта, планеты — заглушка
    canvas: null,
    canvasWrapper: null,
    spectralClass: 'G',
    starType: 'star',            // star/white_dwarf/neutron/black_hole/protostar (99.2.4 §2)
    worldId: null,
    worldName: '',
    worldTemperature: 0,
    stellarMass: null,       // масса звезды в M☉ (29a §4м); null — не задана
    stellarMods: null,       // модификаторы звезды (41a): {subtype, disk_state, ...} — для ветки «аккреция» ЧД
    worldAge: null,          // возраст системы в млрд лет (41a); null — нет данных (старые миры/обычные звёзды)
    authToken: null, // токен открытия модалки (админка); refreshPlanets использует его
    // Роль игрока из /me (спека 2026-09-20 §6.2): admin/skycomposer видят
    // «Вид с орбиты» всегда (гейт 2); null — роль ещё не загружена.
    role: null,
    hasEngine: true, // установлен ли двигатель игрока (спека 91a §6.1): без него
                     // «Перелететь» из модалки блокируется; true = админка/не загружено
    // Внутрисистемная позиция игрока (спека 99.2.27 §4.4): my_position из
    // /api/worlds/{id}/planets; null = игрок не в этой системе. При активном
    // внутрисистемном полёте — {status:'in_flight', from, to, start_time, arrive_at}.
    myPosition: null,
    // Явный признак «своя система» из ответа /api/worlds/{id}/planets
    // (in_own_system, баг 2026-09-22): current_world_id == worldId. НЕ выводить
    // «свою систему» из myPosition != null — позиция пуста в окне прибытия/
    // межзвёздного полёта, и кнопка полёта ошибочно становилась композитной.
    inOwnSystem: false,
    companionId: null,   // синтетический id компаньона (companion:<world>, §3.1)
    systemPlayers: [],   // чужие игроки в этой системе (опрос 5 с, §5.4): {id, username, status, object_type, object_id, ...}
    shipIcon: '',        // спрайт игрока для маркера «я здесь»/корабля в полёте (спека 99.2.27 §5.8/§5.11)
    shipColor: null,     // цвет перекраски спрайта (NULL = «Оригинал»)
    // Активный межзвёздный полёт из /me (запрос создателя): my_position = null
    // вне системы — надёжный признак; кнопка «Найти меня» в модалке при
    // межзвёздном полёте закрывает модалку и ведёт себя как кнопка карты.
    interstellarFlight: null,
    _rafId: null,
    _playersTimer: null, // id setInterval опроса чужих игроков в системе (спека 99.2.27 §5.4)
    dragMoved: false,
    suppressNextClick: false,
    activeTab: 'general', // текущая вкладка карточки планеты, чтобы «Обновить» не сбрасывал на «Общее»
    autoRefreshTimer: null, // id setInterval автообновления карточки планеты (отладка, admin.html)
    previousPopulation: {},  // planetId -> население на прошлый refresh, для стрелочки тренда
    previousSettlementPop: {}  // settlementId -> население на прошлый refresh, вкладка «Поселения»
};

// flightModeForSystem — режим полёта в модалке: 'intra' — своя система
// (внутрисистемный полёт, кнопка «🚀 Лететь»), 'composite' — чужая система или
// активный межзвёздный полёт (композитный «🚀 Лететь · через систему»). «Своя
// система» — ЯВНЫЙ флаг сервера inOwnSystem (current_world_id == worldId, баг
// 2026-09-22), а не косвенный myPosition != null: позиция пуста в окне
// прибытия/межзвёздного полёта, и кнопка ошибочно становилась композитной
// (сервер → 400 «Already in this world»). Межзвёздный полёт в своей системе —
// композитный: это редирект /travel (спека 99.2.30 §3.4/§3.5).
export function flightModeForSystem() {
    return (modalState.inOwnSystem && !modalState.interstellarFlight) ? 'intra' : 'composite';
}

export function resetState() {
    modalState.zoom = 1;
    modalState.offsetX = 0;
    modalState.offsetY = 0;
    modalState.followOffsetX = 0;
    modalState.followOffsetY = 0;
    modalState.followEnabled = true;
    modalState.followExact = false;
    modalState.followDirty = false;
    modalState.arrivalObject = null;
    modalState.isDragging = false;
    modalState.hoveredObject = null;
    modalState.selectedObject = null;
    modalState.selectedPlanetIndex = null;
    modalState.selectedSatellite = null;
    modalState.dragMoved = false;
    modalState.suppressNextClick = false;
    modalState.activeTab = 'general';
    modalState.authToken = null;
    modalState.role = null;
    modalState.restricted = false;
    modalState.belts = [];
    modalState.myPosition = null;
    modalState.inOwnSystem = false;
    modalState.companionId = null;
    modalState.systemPlayers = [];
    modalState.shipIcon = '';
    modalState.shipColor = null;
    modalState.interstellarFlight = null;
    modalState.previousPopulation = {};
    modalState.previousSettlementPop = {};
    if (modalState._rafId !== null) {
        cancelAnimationFrame(modalState._rafId);
        modalState._rafId = null;
    }
    if (modalState.autoRefreshTimer !== null) {
        clearInterval(modalState.autoRefreshTimer);
        modalState.autoRefreshTimer = null;
    }
    if (modalState._playersTimer !== null) {
        clearInterval(modalState._playersTimer);
        modalState._playersTimer = null;
    }
}