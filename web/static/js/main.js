// web/static/js/main.js
import { state, elements } from './map/config.js';
import { resizeCanvas } from './map/map_render.js';
import { handleCanvasClick, initFlyBtn, initPanZoom, initHover, initContextMenu } from './map/events.js';
import { animationLoop } from './map/animation.js';
import { centerOnAgent } from './map/navigation.js';
import { applyFiltersFromUI, resetFilters, filterState } from './filters.js';
import { loadClusters, loadUserData } from './map/data.js';
import { draw } from './map/map_render.js';
import { showTextLoader } from './loader.js';
import { notifyError } from './ui/toast.js';
import { initEntitySearch } from './search.js';
import { formatZoom } from './map/utils.js';
import { startNPCLoop } from './map/npc_agents.js';

// --- Восстановление вьюпорта из sessionStorage ---
function restoreViewport() {
    try {
        const saved = sessionStorage.getItem('viewport');
        if (saved) {
            const vp = JSON.parse(saved);
            state.offsetX = vp.offsetX || 0;
            state.offsetY = vp.offsetY || 0;
            state.scale = vp.scale || 1;
            if (elements.zoomInfo) {
                elements.zoomInfo.textContent = formatZoom(state.scale, state.minZoom);
            }
            return true;
        }
    } catch (e) { /* ignore */ }
    return false;
}

// --- Инициализация карты ---
async function initMap() {
    resizeCanvas();

    // 1. Пользователь (current_world_id)
    await loadUserData();

    // 2. Восстановление вьюпорта
    const restored = restoreViewport();
    if (!restored) {
        // Первый заход — центрируемся на игроке, если знаем мир
        if (state.currentWorldId) {
            centerOnAgent();
        }
    }

    // 3. Первая загрузка кластеров
    if (elements.loading) elements.loading.style.display = 'block';
    let stopStatus = showTextLoader(elements.statusBar, '⏳ Загрузка карты...');

    await loadClusters();

    if (elements.loading) elements.loading.style.display = 'none';
    stopStatus();

    // Вьюпорт из sessionStorage мог указывать на пустоту (например, после
    // перегенерации вселенной координаты сменились). Если в кадре нет ни
    // одного мира — центрируемся на галактику и перезаписываем вьюпорт.
    if (!state.clusters || state.clusters.length === 0) {
        if (state.galaxyRadius) {
            state.scale = state.minZoom || 0.001;
            state.offsetX = state.canvasWidth / 2;
            state.offsetY = state.canvasHeight / 2;
            sessionStorage.setItem('viewport', JSON.stringify({
                offsetX: state.offsetX, offsetY: state.offsetY, scale: state.scale,
            }));
            if (elements.zoomInfo) {
                elements.zoomInfo.textContent = formatZoom(state.scale, state.minZoom);
            }
            loadClusters();
        } else if (state.currentWorldId) {
            centerOnAgent();
        }
    }
}

function init() {
    elements.canvas.addEventListener('click', handleCanvasClick);
    window.addEventListener('resize', resizeCanvas);
    initFlyBtn();
    initPanZoom();
    initHover();
    initContextMenu();
    initEntitySearch();

    // --- NPC-агенты на карте (спека 20a.1 §7): опрос позиций + WS ---
    startNPCLoop();

    // --- Автоматическое применение фильтров ---
    const filterInputs = document.querySelectorAll('#filters-bar input, #filters-bar select');
    let timeoutId = null;

    async function applyFilters() {
        applyFiltersFromUI();

        const spinner = document.getElementById('filter-spinner');
        const countEl = document.getElementById('filter-count');
        let filterStop = null;
        if (spinner) {
            spinner.classList.add('visible');
            const filterStart = Date.now();
            spinner.title = '0 с';
            filterStop = setInterval(() => {
                spinner.title = Math.round((Date.now() - filterStart) / 1000) + ' с';
            }, 250);
        }
        if (countEl) countEl.textContent = '...';

        try {
            await loadClusters();
            if (countEl) {
                const activeCount = [
                    filterState.hasPlanets,
                    filterState.hasLife,
                    filterState.hasHabitable,
                    filterState.planetType,
                    filterState.resourceCategory,
                ].filter(Boolean).length;
                countEl.textContent = activeCount > 0 ? `(${activeCount})` : '';
            }
        } catch (e) {
            console.error('Filter apply error:', e);
            if (countEl) countEl.textContent = '❌';
        } finally {
            if (filterStop) clearInterval(filterStop);
            if (spinner) spinner.classList.remove('visible');
        }
    }

    filterInputs.forEach(el => {
        el.addEventListener('change', () => {
            clearTimeout(timeoutId);
            timeoutId = setTimeout(applyFilters, 300);
        });
    });

    const resetBtn = document.getElementById('reset-filters');
    if (resetBtn) {
        resetBtn.addEventListener('click', async () => {
            resetFilters();
            document.getElementById('filter-has-planets').checked = false;
            document.getElementById('filter-life').checked = false;
            document.getElementById('filter-habitable').checked = false;
            document.getElementById('filter-planet-type').value = '';
            document.getElementById('filter-resource').value = '';

            const spinner = document.getElementById('filter-spinner');
            const countEl = document.getElementById('filter-count');
            let filterStop = null;
            if (spinner) {
                spinner.classList.add('visible');
                const filterStart = Date.now();
                spinner.title = '0 с';
                filterStop = setInterval(() => {
                    spinner.title = Math.round((Date.now() - filterStart) / 1000) + ' с';
                }, 250);
            }
            if (countEl) countEl.textContent = '...';

            try {
                await loadClusters();
                if (countEl) countEl.textContent = '';
            } catch (e) {
                console.error('Reset filter error:', e);
                if (countEl) countEl.textContent = '❌';
            } finally {
                if (filterStop) clearInterval(filterStop);
                if (spinner) spinner.classList.remove('visible');
            }
        });
    }

    // --- Первая загрузка + запуск цикла анимации ---
    initMap().then(() => {
        animationLoop();
    }).catch(err => {
        console.error('initMap error:', err);
        if (elements.statusBar) elements.statusBar.textContent = '❌ Ошибка загрузки';
    });
}

init();