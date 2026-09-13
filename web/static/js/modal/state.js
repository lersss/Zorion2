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
    planets: [],
    canvas: null,
    canvasWrapper: null,
    spectralClass: 'G',
    worldId: null,
    worldName: '',
    worldTemperature: 0,
    worldCoordX: 0,
    worldCoordY: 0,
    animStart: 0,
    _rafId: null,
    dragMoved: false,
    suppressNextClick: false,
    activeTab: 'general', // текущая вкладка карточки планеты, чтобы «Обновить» не сбрасывал на «Общее»
    autoRefreshTimer: null, // id setInterval автообновления карточки планеты (отладка, admin.html)
    previousPopulation: {}  // planetId -> население на прошлый refresh, для стрелочки тренда
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
    modalState.previousPopulation = {};
    if (modalState._rafId !== null) {
        cancelAnimationFrame(modalState._rafId);
        modalState._rafId = null;
    }
    if (modalState.autoRefreshTimer !== null) {
        clearInterval(modalState.autoRefreshTimer);
        modalState.autoRefreshTimer = null;
    }
}