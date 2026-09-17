// web/static/js/modal/index.js
import { modalState, resetState } from './state.js';
import { drawSystem } from './modal_render.js';
import { initEvents } from './events.js';
import { clearTextureCache } from './textures.js';
import { getStarColor, getStarSize, starTypeLabel, systemTypeLabel, starModsBadges } from './utils.js';
import { renderRightPanel } from './panel.js';
import { notifyError } from '../ui/toast.js';
import { repaintPopulationNumbers } from './extrapolate.js';
import { record } from '../dashboard/journal.js';

// handleUnauthorized — локальная копия map/data.js: чистит игровой токен и
// редиректит на логин. Не импортируем из ../map/ — тот тянет map/config.js,
// который при загрузке требует #mapCanvas (его нет в админке) и роняет весь
// import-граф. Единственное использование здесь — обработка 401 (и отсутствия
// токена); 403 «вне зоны видимости» разлогин не вызывает (спека 77a §5.5/И11).
function handleUnauthorized() {
    localStorage.removeItem('token');
    if (window.location.pathname !== '/login-page') {
        window.location.href = '/login-page';
    }
}

// openSystemModal — открывает модалку системы по ID мира.
// focusOpts: { planetId?, satelliteId? } — после открытия выбирает объект
// (планету или спутник) в правой панели.
// authToken — необязательный токен для авторизации (админка «Миры»: там
// используется adminToken, а игрового 'token' может не быть). undefined =
// обычный игровой токен из localStorage.
// starInfo — открытая информация о звезде из кластера карты (спека 77a §5.5):
// { stype, stemp, systype, smods, x, y }. Используется при 403 (система вне
// радиуса/знания): модалка открывается с карточкой звезды и заглушкой вместо
// планет — вид звезды открыт везде, детали системы закрыты (И11).
export function openSystemModal(worldId, worldName, spectralClass, focusOpts, authToken, starInfo) {
    const token = authToken || localStorage.getItem('token');
    if (!token) {
        handleUnauthorized();
        return;
    }

    // Сохраняем токен в состоянии: refreshPlanets (кнопка «Обновить») и
    // события используют его же, а не игровой localStorage (в админке его нет).
    modalState.authToken = token;

    // Двигатель игрока (спека 91a §6.1): без него «Перелететь» из модалки
    // блокируется. starInfo приходит с карты (state.hasEngine); админка/
    // неизвестно — true (админ летает всегда, сервер валидирует тоже).
    modalState.hasEngine = starInfo ? !!starInfo.hasEngine : true;

    fetch(`/api/worlds/${worldId}/planets`, {
        headers: { 'Authorization': 'Bearer ' + token }
    })
    .then(response => {
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return;
            }
            if (response.status === 403) {
                // Детали системы закрыты вне радиуса/знания (спека 77a §5.5/И11):
                // звезда открыта — модалка с карточкой звезды и заглушкой вместо
                // планет. starInfo — открытая информация из кластера карты.
                const data = {
                    planets: [],
                    restricted: true,
                    star_type: starInfo && starInfo.stype,
                    system_type: starInfo && starInfo.systype,
                    stellar_mods: starInfo && starInfo.smods,
                    temperature: starInfo && starInfo.stemp,
                    coord_x: starInfo && starInfo.x,
                    coord_y: starInfo && starInfo.y,
                };
                renderModal(worldId, worldName, spectralClass, data);
                return;
            }
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return response.json();
    })
    .then(data => {
        // Guard: 401/403-ветки выше уже обработали ответ и вернули undefined —
        // цепочку дальше не ведём (иначе второй renderModal затёр бы
        // restricted-карточку, а record упал бы на data.star_type).
        if (!data) return;
        console.log('Planets data:', data);
        renderModal(worldId, worldName || data.world_name, spectralClass || data.spectral_class, data);
        // Хук журнала (спека 86a §4.3): аддитивная запись встреченного при
        // открытии модалки системы — мир, экзотическая звезда, типы планет,
        // расы поселений (race_id пусто = люди — журнал не пишет, §5.1.3).
        // Ничего не блокирует и не перехватывает (И1).
        record({
            worldId,
            starType: data.star_type,
            planetTypes: (data.planets || []).map(p => p.type).filter(Boolean),
            raceIds: (data.planets || []).flatMap(p =>
                (p.settlements || []).map(s => s.race_id)
            ),
        });
        if (focusOpts) {
            if (focusOpts.satelliteId) {
                selectSatelliteInModal(focusOpts.satelliteId);
            } else if (focusOpts.planetId) {
                selectPlanetInModal(focusOpts.planetId);
            }
        }
    })
    .catch(error => {
        console.error('Error loading planets:', error);
        notifyError('Ошибка загрузки планет: ' + error.message);
    });
}

// refreshPlanets — перечитывает планеты текущей открытой системы (без
// пересоздания модалки, без сброса вкладки/камеры). Используется кнопкой
// «Обновить» в карточке планеты: пересчёт населения от среды происходит на
// сервере при каждом чтении (docs/gamedesign/18a_population_death.md), эта
// функция просто вытягивает уже пересчитанный результат.
export function refreshPlanets() {
    const worldId = modalState.worldId;
    const token = modalState.authToken || localStorage.getItem('token');
    if (!token || !worldId) return;

    fetch(`/api/worlds/${worldId}/planets`, {
        headers: { 'Authorization': 'Bearer ' + token }
    })
    .then(response => {
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return response.json();
    })
    .then(data => {
        // Запоминаем прежнее население по id планеты — стрелочка тренда
        // в panel.js (populationTrendArrow) сравнивает с этим на следующий рендер.
        const previous = modalState.previousPopulation || {};
        (modalState.planets || []).forEach(p => { previous[p.id] = p.population; });
        modalState.previousPopulation = previous;

        // То же для поселений — стрелочка тренда во вкладке «Поселения»
        // (settlementTrendArrow, tabs.js).
        const previousSettlements = modalState.previousSettlementPop || {};
        (modalState.planets || []).forEach(p => {
            (Array.isArray(p.settlements) ? p.settlements : []).forEach(s => {
                if (s && typeof s.population === 'number') previousSettlements[s.id] = s.population;
            });
        });
        modalState.previousSettlementPop = previousSettlements;

        modalState.planets = Array.isArray(data && data.planets) ? data.planets : [];
        const idx = modalState.selectedPlanetIndex;
        if (idx !== null && idx !== undefined && modalState.planets[idx]) {
            renderRightPanel(modalState.planets, idx);
        } else {
            renderRightPanel(modalState.planets, null);
        }
    })
    .catch(error => {
        console.error('Error refreshing planets:', error);
        notifyError('Ошибка обновления: ' + error.message);
    });
}

// selectPlanetInModal — выбирает планету по id в правой панели модалки.
export function selectPlanetInModal(planetId) {
    const planets = modalState.planets || [];
    const idx = planets.findIndex(p => p.id === planetId);
    if (idx < 0) return;

    modalState.selectedPlanetIndex = idx;
    renderRightPanel(planets, idx);
    drawSystem(
        modalState.canvas, modalState.spectralClass, planets,
        modalState.starRadius, modalState.starColor,
        modalState.canvasWidth, modalState.canvasHeight
    );
}

// selectSatelliteInModal — выбирает спутник по id: открывает карточку
// родительской планеты и программно кликает по спутнику во вкладке «Общее».
export function selectSatelliteInModal(satelliteId) {
    const planets = modalState.planets || [];
    const planetIdx = planets.findIndex(p =>
        Array.isArray(p.satellites) && p.satellites.some(s => s.id === satelliteId)
    );
    if (planetIdx < 0) return;
    const planet = planets[planetIdx];
    const satIdx = planet.satellites.findIndex(s => s.id === satelliteId);
    if (satIdx < 0) return;

    modalState.selectedPlanetIndex = planetIdx;
    renderRightPanel(planets, planetIdx);
    drawSystem(
        modalState.canvas, modalState.spectralClass, planets,
        modalState.starRadius, modalState.starColor,
        modalState.canvasWidth, modalState.canvasHeight
    );

    const tabContent = document.getElementById('tab-content');
    const li = tabContent && tabContent.querySelector(`[data-sat-idx="${satIdx}"]`);
    if (li) li.click();
}

// ==================== МОДАЛКА ====================

function renderModal(worldId, worldName, spectralClass, data) {
    if (document.getElementById('system-modal-overlay')) return;

    resetState();

    const planets = Array.isArray(data && data.planets) ? data.planets : [];
    const starType = (data && data.star_type) || 'star';
    const starColor = getStarColor(spectralClass, starType);
    const starRadius = getStarSize(spectralClass, starType);

    // Оверлей
    const overlay = document.createElement('div');
    overlay.id = 'system-modal-overlay';
    overlay.style.cssText = `
        position: fixed;
        top: 0; left: 0; width: 100%; height: 100%;
        background: rgba(0,0,0,0.7);
        z-index: 1000;
        display: flex;
        justify-content: center;
        align-items: center;
        backdrop-filter: blur(4px);
        animation: fadeIn 0.2s ease;
    `;

    // Модалка
    const modal = document.createElement('div');
    modal.style.cssText = `
        background: #1a1a2e;
        color: #e0e0e0;
        border-radius: 16px;
        padding: 20px;
        width: 80vw;
        height: 80vh;
        display: flex;
        flex-direction: column;
        box-shadow: 0 8px 32px rgba(0,0,0,0.5);
        position: relative;
    `;

    // Заголовок
    const header = document.createElement('div');
    header.style.cssText = `
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: 12px;
    `;
    const title = document.createElement('h2');
    // Координаты мира — рядом с названием (49b, решение создателя 2026-09-16):
    // спектральный класс из шапки убран, формат «Nemurzan (99; -1775)» — целые
    // (дробь отброшена), без слова «координаты». Тип объекта (экзотика) виден
    // в чипах шапки и в карточке звезды.
    const coordX = (data && typeof data.coord_x === 'number') ? data.coord_x : null;
    const coordY = (data && typeof data.coord_y === 'number') ? data.coord_y : null;
    const titleText = `${worldName} (${coordX !== null ? Math.trunc(coordX) : '—'}, ${coordY !== null ? Math.trunc(coordY) : '—'})`;
    title.textContent = titleText;
    // Название — единым читаемым цветом (не цветом звезды): у ЧД/нейтронной
    // цвет объекта тёмный (#2a1a4a) и текст на фоне модалки нечитаем (40b).
    title.style.cssText = `
        margin: 0;
        font-size: 1.5rem;
        color: #ececec;
    `;
    const closeBtn = document.createElement('button');
    closeBtn.innerHTML = '✕';
    closeBtn.style.cssText = `
        background: none;
        border: none;
        color: #aaa;
        font-size: 1.8rem;
        cursor: pointer;
        padding: 0 8px;
    `;
    closeBtn.onclick = () => closeModal();
    header.appendChild(title);
    header.appendChild(closeBtn);
    modal.appendChild(header);

    // Блок «Тип системы» + «Тип объекта» + модификаторы (99.2.4 §8).
    const infoLine = document.createElement('div');
    infoLine.style.cssText = `
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
        margin-bottom: 10px;
        font-size: 0.8rem;
    `;
    const chips = [];
    const sysLabel = systemTypeLabel(data && data.system_type);
    if (sysLabel) chips.push(`тип системы: ${sysLabel}`);
    if (starType && starType !== 'star') {
        chips.push(`тип объекта: ${starTypeLabel(starType) || starType}`);
    }
    starModsBadges(data && data.stellar_mods).forEach(b => chips.push(b));
    chips.forEach(text => {
        const chip = document.createElement('span');
        chip.textContent = text;
        chip.style.cssText = `
            background: rgba(148,163,184,0.12);
            border: 1px solid rgba(148,163,184,0.25);
            border-radius: 10px;
            padding: 2px 8px;
            color: #cbd5e1;
        `;
        infoLine.appendChild(chip);
    });
    if (infoLine.children.length) modal.appendChild(infoLine);

    // Контент
    const content = document.createElement('div');
    content.style.cssText = `
        display: flex;
        flex: 1;
        gap: 20px;
        min-height: 0;
    `;

    // Canvas
    const canvasWrapper = document.createElement('div');
    canvasWrapper.style.cssText = `
        flex: 1;
        min-width: 0;
        background: #0d0d1a;
        border-radius: 12px;
        position: relative;
        overflow: hidden;
        cursor: grab;
    `;
    const canvas = document.createElement('canvas');
    canvas.id = 'system-canvas';
    canvas.style.cssText = `
        width: 100%;
        height: 100%;
        display: block;
    `;
    canvasWrapper.appendChild(canvas);
    content.appendChild(canvasWrapper);

    // Правая панель
    const rightPanel = document.createElement('div');
    rightPanel.id = 'right-panel';
    rightPanel.style.cssText = `
        flex: 1;
        background: #12121f;
        border-radius: 12px;
        padding: 12px;
        overflow-y: auto;
        min-width: 200px;
        max-height: 100%;
    `;
    content.appendChild(rightPanel);

    modal.appendChild(content);
    overlay.appendChild(modal);
    document.body.appendChild(overlay);

    // Закрытие по клику на оверлей
    overlay.addEventListener('click', (e) => {
        if (e.target === overlay) closeModal();
    });

    // ESC
    function handleKeydown(e) {
        if (e.key === 'Escape') closeModal();
    }
    document.addEventListener('keydown', handleKeydown);

    // Размеры
    const rect = canvasWrapper.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;

    modalState.canvasWidth = width;
    modalState.canvasHeight = height;
    modalState.starRadius = starRadius;
    modalState.starColor = starColor;
    modalState.systemType = (data && data.system_type) || 'single';
    const mods = (data && data.stellar_mods) || {};
    modalState.stellarMods = mods;
    modalState.worldAge = (typeof data.age === 'number') ? data.age : null;
    modalState.binaryType = mods.binary_type || '';
    modalState.companion = mods.companion || '';
    modalState.companionColor = getStarColor(mods.companion, 'star');
    // Параметры компаньона (35b §6.6): масса/температура/разделение пары,
    // внешние компаньоны кратных. Старые миры — поля отсутствуют → null/[].
    modalState.companionMass = (typeof mods.companion_mass === 'number') ? mods.companion_mass : null;
    modalState.companionTemp = (typeof mods.companion_temp === 'number') ? mods.companion_temp : null;
    modalState.companionSepAU = (typeof mods.companion_sep_au === 'number') ? mods.companion_sep_au : null;
    modalState.extraCompanions = Array.isArray(mods.extra_companions) ? mods.extra_companions : [];
    modalState.canvas = canvas;
    modalState.canvasWrapper = canvasWrapper;
    modalState.spectralClass = spectralClass;
    modalState.starType = starType;
    modalState.worldId = worldId;
    modalState.worldName = worldName;
    modalState.worldTemperature = (data && data.temperature) || 0;
    modalState.stellarMass = (data && data.stellar_mass) || null;
    modalState.selectedPlanetIndex = null;
    modalState.selectedObject = null;
    modalState.planets = planets;
    modalState.restricted = !!data.restricted;

    clearTextureCache();

    renderRightPanel(planets, null);

    initEvents(canvas, spectralClass, planets, starRadius, starColor, width, height);

    // Глобальная функция для events.js (клик по планете)
    window.updateRightPanel = (selectedIndex) => {
        renderRightPanel(planets, selectedIndex);
    };

    // Глобальная функция для events.js (клик по звезде): карточка звезды
    // убрана (70a) — клик возвращает панель к списку планет.
    window.showStarCard = () => {
        renderRightPanel(planets, null);
    };

    // Клик по строке таблицы
    rightPanel.addEventListener('click', (e) => {
        const row = e.target.closest('tr');
        if (row && row.dataset.index !== undefined) {
            const idx = parseInt(row.dataset.index);
            if (!isNaN(idx) && idx >= 0 && idx < planets.length) {
                if (modalState.selectedPlanetIndex !== idx) {
                    modalState.selectedPlanetIndex = idx;
                    renderRightPanel(planets, idx);
                    drawSystem(canvas, spectralClass, planets, starRadius, starColor, width, height);
                }
            }
        }
    });

    // Resize
    const resizeObserver = new ResizeObserver(() => {
        const newRect = canvasWrapper.getBoundingClientRect();
        modalState.canvasWidth = newRect.width;
        modalState.canvasHeight = newRect.height;
        drawSystem(canvas, spectralClass, planets, starRadius, starColor, newRect.width, newRect.height);
    });
    resizeObserver.observe(canvasWrapper);

    modalState._escListener = handleKeydown;

    // ---- АНИМАЦИЯ: rAF-цикл вращения планет ----
    // Попутно — косметическая тень населения (extrapolate.js): раз в секунду
    // перерисовываем числа от локального счёта, чтобы между синками с сервером
    // население «жило». rAF в фоновой вкладке замирает — первый кадр после
    // возврата сразу даёт свежее число без запроса.
    let lastPopRepaint = 0;
    function tick() {
        if (!document.getElementById('system-modal-overlay')) return;
        drawSystem(canvas, spectralClass, planets, starRadius, starColor, modalState.canvasWidth, modalState.canvasHeight);
        const popIdx = modalState.selectedPlanetIndex;
        if (popIdx !== null && popIdx !== undefined && modalState.planets[popIdx]) {
            const frameMs = performance.now();
            if (frameMs - lastPopRepaint >= 1000) {
                lastPopRepaint = frameMs;
                repaintPopulationNumbers(modalState.planets[popIdx]);
            }
        }
        modalState._rafId = requestAnimationFrame(tick);
    }
    modalState._rafId = requestAnimationFrame(tick);
}

// ==================== ЗАКРЫТИЕ ====================

export function closeModal() {
    const overlay = document.getElementById('system-modal-overlay');
    if (overlay) overlay.remove();
    resetState();
    if (modalState._escListener) {
        document.removeEventListener('keydown', modalState._escListener);
        delete modalState._escListener;
    }
    const ctxMenu = document.getElementById('star-context-menu');
    if (ctxMenu) ctxMenu.remove();
    const starTooltip = document.getElementById('star-tooltip');
    if (starTooltip) starTooltip.remove();
    delete window.updateRightPanel;
    delete window.showStarCard;
}

// ==================== СТИЛЬ АНИМАЦИИ ====================

if (!document.getElementById('modal-fade-style')) {
    const style = document.createElement('style');
    style.id = 'modal-fade-style';
    style.textContent = `
        @keyframes fadeIn {
            from { opacity: 0; }
            to { opacity: 1; }
        }
    `;
    document.head.appendChild(style);
}

// Экспорт в window
window.openSystemModal = openSystemModal;