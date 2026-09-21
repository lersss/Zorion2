// web/static/js/main.js
import { state, elements } from './map/config.js';
import { resizeCanvas } from './map/map_render.js';
import { handleCanvasClick, initFlyBtn, initPanZoom, initHover, initContextMenu } from './map/events.js';
import { animationLoop } from './map/animation.js';
import { centerOnAgent } from './map/navigation.js';
import { showFlightPanel } from './map/flight.js';
import { applyFiltersFromUI, resetFilters, filterState } from './filters.js';
import { loadClusters, loadUserData, startPlayerPositionsLoop } from './map/data.js';
import { draw } from './map/map_render.js';
import { radarBoundaryVariant, setRadarBoundaryVariant } from './map/map_render.js';
import { setRedrawCallback, preloadShipSprites } from './map/ship_sprites.js';
import { showTextLoader } from './loader.js';
import { notifyError } from './ui/toast.js';
import { initSoundToggle, startFlightHum, activateSound } from './ui/sound.js';
import { initEntitySearch } from './search.js';
import { formatZoom } from './map/utils.js';
import { startNPCLoop, initNPCSearch } from './map/npc_agents.js';

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

    // 1. Пользователь (current_world_id, ship_icon/ship_color, реестр спрайтов).
    await loadUserData();
    // Прогрев кэша перекраски 21×9 (уточнение 61b 2026-09-16): после /me
    // известен порядок реестра; прогрев идёт по мере onload спрайтов,
    // старт карты не блокирует.
    preloadShipSprites();

    // 2. Восстановление вьюпорта.
    // Полёт, восстановленный из /me (идея 42a), имеет приоритет над
    // сохранённым вьюпортом: камера возвращается к кораблю в текущей точке
    // пути, а не к старой точке из sessionStorage.
    if (state.isFlying && state.flyFrom && state.flyTo) {
        showFlightPanel(state.flyFrom.name, state.flyTo.name);
        // Полёт восстановлен после F5 (user.flight в data.js) — поднимаем гул,
        // иначе «летим, но тихо» (ловушка 6). Стартовый сигнал не играем:
        // это не новый запуск.
        startFlightHum();
        // Идея 42a: если до рефреша слежение было включено — восстанавливаем.
        // Класс active кнопке добавит animationLoop на первом кадре (animation.js).
        if (sessionStorage.getItem('followShip') === '1') state.followShip = true;
        centerOnAgent();
    } else {
        const restored = restoreViewport();
        if (!restored) {
            // Первый заход — центрируемся на игроке, если знаем мир
            if (state.currentWorldId) {
                centerOnAgent();
            }
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
    // Во время полёта (идея 42a) камерой владеет полётная логика
    // (centerOnAgent + maybeReloadClusters) — фоллбэк не трогаем, иначе
    // после рефреша в полёте над пустой областью камера ушла бы с корабля.
    if (!state.isFlying && (!state.clusters || state.clusters.length === 0)) {
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
    // Явная активация звука на странице карты (флаг звукового модуля): только
    // здесь, поэтому админка (общий ui/toast.js) молчит. Все play-функции до
    // активации — no-op.
    activateSound();
    elements.canvas.addEventListener('click', handleCanvasClick);
    window.addEventListener('resize', resizeCanvas);
    // Перерисовка карты после асинхронной загрузки спрайтов кораблей
    // (спека 61b §5.6: onload спрайта → scheduleRedraw → draw).
    setRedrawCallback(draw);
    initFlyBtn();
    initPanZoom();
    initHover();
    initContextMenu();
    initEntitySearch();
    // Кнопка вкл/выкл звука в шапке карты (отражает состояние, переключает его).
    initSoundToggle();

    // --- NPC-агенты на карте (спека 20a.1 §7): опрос позиций + WS ---
    startNPCLoop();

    // --- Чужие игроки в радиусе радара (спека 77a §5.3): опрос позиций ---
    startPlayerPositionsLoop();

    // --- Поиск агента на карте (спека 26a.1 §6.2): поле в #zoom-controls ---
    initNPCSearch();

    // --- Тестовая переключалка границы видимости (спека 77a §9.1) ---
    const boundarySelect = document.getElementById('radarBoundarySelect');
    if (boundarySelect) {
        boundarySelect.value = String(radarBoundaryVariant());
        boundarySelect.addEventListener('change', () => {
            setRadarBoundaryVariant(parseInt(boundarySelect.value, 10) || 1);
        });
    }

    // --- Переход в админку (пожелание 2026-09-19): кнопка видна только
    // админским ролям (показ — в data.js по роли из /me); игровой токен
    // копируется в админский ключ — тот же JWT, что и у админки. ---
    const adminNavBtn = document.getElementById('adminNavBtn');
    if (adminNavBtn) {
        adminNavBtn.addEventListener('click', () => {
            const t = localStorage.getItem('token');
            if (t) localStorage.setItem('adminToken', t);
            window.location.href = '/admin';
        });
    }

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