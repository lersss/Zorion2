// web/static/js/modal/state.js
export const modalState = {
    zoom: 1,
    offsetX: 0,
    offsetY: 0,
    isDragging: false,
    dragStartX: 0,
    dragStartY: 0,
    dragStartOffsetX: 0,
    dragStartOffsetY: 0,
    hoveredObject: null,
    selectedObject: null,      // { type: 'star' } | { type: 'planet', index }
    selectedPlanetIndex: null,
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
    _rafId: null,
    dragMoved: false,
    suppressNextClick: false,
    activeTab: 'general', // текущая вкладка карточки планеты, чтобы «Обновить» не сбрасывал на «Общее»
    autoRefreshTimer: null, // id setInterval автообновления карточки планеты (отладка, admin.html)
    previousPopulation: {},  // planetId -> население на прошлый refresh, для стрелочки тренда
    previousSettlementPop: {}  // settlementId -> население на прошлый refresh, вкладка «Поселения»
};

export function resetState() {
    modalState.zoom = 1;
    modalState.offsetX = 0;
    modalState.offsetY = 0;
    modalState.isDragging = false;
    modalState.hoveredObject = null;
    modalState.selectedObject = null;
    modalState.selectedPlanetIndex = null;
    modalState.dragMoved = false;
    modalState.suppressNextClick = false;
    modalState.activeTab = 'general';
    modalState.authToken = null;
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
}