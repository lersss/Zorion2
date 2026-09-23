// web/static/js/modal/index.js
import { modalState, resetState } from './state.js';
import { drawSystem, objectCanvasPos, orbitalPoint } from './modal_render.js';
import { computeLayout, getOrbitRadius } from './layout.js';
import { initEvents } from './events.js';
import { clearTextureCache } from './textures.js';
import { getStarColor, getStarSize, starTypeLabel, systemTypeLabel, starModsBadges } from './utils.js';
import { renderRightPanel } from './panel.js';
import { cleanupOrbitView } from './tabs.js';
import { notifyError } from '../ui/toast.js';
// Реестр слоёв (спека 2026-09-23 §5): модалка — слой полосы modal; Esc и
// клик-вне адресуются реестром только к верхнему слою, локальных слушателей нет.
import { openLayer, closeToLayer, closeAll } from '../ui/layers.js';
import { playSound, startFlightHum, stopFlightHum, playArrival } from '../ui/sound.js';
import { repaintPopulationNumbers } from './extrapolate.js';
import { record } from '../dashboard/journal.js';
// Колбэк перерисовки спрайтов (запрос создателя 99.2.27): при асинхронной
// загрузке спрайта игрока (img.onload → scheduleRedraw) перерисовывается
// модалка, а не карта. ship_sprites.js — автономный модуль.
import { setRedrawCallback, getRedrawCallback } from '../map/ship_sprites.js';

// prevRedrawCallback — колбэк карты/дашборда, сохранённый при открытии модалки
// (восстанавливается в closeModal, чтобы не сломать карту).
let prevRedrawCallback = null;

// modalLayerHandle — слой модалки в реестре слоёв (ui/layers.js); modalTornDown
// — защита от повторного входа в уборку (спека 2026-09-23 §5).
let modalLayerHandle = null;
let modalTornDown = false;

// Синхронный источник полёта карты (спека 99.2.30 §6.4/§6.9): mapState.isFlying
// — фолбэк для клика 🎯 до резолва /me (modalState.interstellarFlight ещё null).
// Динамический импорт map/config.js только на странице карты (в админке
// import-граф карты не тянется — фолбэк просто закрыть модалку). Прогрев при
// загрузке модуля: к моменту клика импорт уже резолвлен (карта грузит модалку
// статически, модуль в кэше).
let mapStateRef = null;
if (document.getElementById('mapCanvas')) {
    import('../map/config.js').then(m => { mapStateRef = m.state; }).catch(() => {});
}

// handleUnauthorized — локальная копия map/data.js: чистит игровой токен и
// редиректит на логин. Не импортируем из ../map/ — тот тянет map/config.js,
// который при загрузке требует #mapCanvas (его нет в админке) и роняет весь
// import-граф. Единственное использование здесь — обработка 401 (и отсутствия
// токена); 403 «вне зоны видимости» разлогин не вызывает (спека 77a §5.5/И11).
function handleUnauthorized() {
    localStorage.removeItem('token');
    if (window.location.pathname !== '/login-page') {
        // Стек слоёв закрываем перед уходом (спека 2026-09-23 §3.4) — страница
        // всё равно перезагрузится, но блокировка прокрутки снимется сразу.
        closeAll();
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

    // Звук открытия окна системы (ui_open). Только на карте: до activateSound
    // (карта) playSound — no-op, поэтому админка молчит.
    playSound('ui_open');

    // Сохраняем токен в состоянии: refreshPlanets (кнопка «Обновить») и
    // события используют его же, а не игровой localStorage (в админке его нет).
    modalState.authToken = token;

    // Двигатель игрока (спека 91a §6.1): без него «Перелететь» из модалки
    // блокируется. starInfo приходит с карты (state.hasEngine); админка/
    // неизвестно — true (админ летает всегда, сервер валидирует тоже).
    modalState.hasEngine = starInfo ? !!starInfo.hasEngine : true;
    // Спрайт игрока для маркера «я здесь»/корабля в полёте (спека 99.2.27
    // §5.8/§5.11): та же иконка/цвет, что на карте; админка/неизвестно —
    // пусто (фолбэк-примитив, И4).
    modalState.shipIcon = (starInfo && starInfo.shipIcon) || '';
    modalState.shipColor = (starInfo && starInfo.shipColor) || null;

    // Гарантированный спрайт (запрос создателя 99.2.27): /me — надёжный источник
    // (auth_handlers /me ВСЕГДА отдаёт валидный ship_icon через ResolveShipIcon:
    // реестр→как есть, неизвестное/пустое→DefaultHumanShip).
    // Запрашиваем ВСЕГДА (не только при отсутствии starInfo) — перекрывает
    // starInfo, если карта отдала устаревшее/пустое; работает из любого входа
    // (карта/поиск/админка). rAF-тик перерисовывает каждый кадр — достаточно
    // заполнить modalState.
    fetch('/me', { headers: { 'Authorization': 'Bearer ' + token } })
        .then(res => {
            if (res.status === 401) {
                handleUnauthorized();
                return null;
            }
            if (!res.ok) return null;
            return res.json();
        })
        .then(me => {
            if (!me) return;
            modalState.shipIcon = me.ship_icon || '';
            modalState.shipColor = me.ship_color || null;
            // Межзвёздный полёт (запрос создателя): my_position = null вне
            // системы — надёжный признак; кнопка «Найти меня» в модалке при
            // межзвёздном полёте закроет её и поведёт как кнопка карты.
            modalState.interstellarFlight = (me && me.flight) || null;
            // Имя системы-цели межзвёздного полёта (спека 99.2.30 §6.10):
            // для тултипа композитной кнопки «Маршрут развернётся: полёт к
            // <система>…». Источник — кэш миров карты (state.worlds / flyTo);
            // динамический импорт только на странице карты.
            modalState.interstellarFlightName = null;
            if (modalState.interstellarFlight && document.getElementById('mapCanvas')) {
                import('../map/config.js').then(m => {
                    const flight = modalState.interstellarFlight;
                    if (!flight) return;
                    const w = m.state.worlds.find(x => x.id === flight.to) || m.state.flyTo;
                    if (w && w.name) {
                        modalState.interstellarFlightName = w.name;
                    }
                }).catch(() => {});
            }
            // Роль (спека 2026-09-20 §6.2): admin/skycomposer видят «Вид с
            // орбиты» всегда. /me асинхронный — если модалка уже открыта и
            // планета выбрана, перерисовываем карточку (гвард: роль пришла
            // после рендера — иначе блок не появится до следующего refresh).
            // То же для межзвёздного полёта (спека 99.2.30 §6.2): тултип
            // композитной кнопки спутника зависит от цели полёта.
            modalState.role = me.role || null;
            if ((modalState.role || modalState.interstellarFlight) && document.getElementById('system-modal-overlay') &&
                modalState.selectedPlanetIndex !== null &&
                modalState.planets && modalState.planets[modalState.selectedPlanetIndex]) {
                renderRightPanel(modalState.planets, modalState.selectedPlanetIndex);
            }
        })
        .catch(() => { /* тихий сбой: остаётся фолбэк-примитив (И4) */ });

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
        // Пояса (спека поясов этап 2 §4.1): состав может раскрыться после
        // скана/присутствия — обновляем вместе с планетами.
        modalState.belts = Array.isArray(data && data.belts) ? data.belts : [];
        // Внутрисистемная позиция (спека 99.2.27 §5.3): по прибытии полёта
        // my_position переходит in_flight → orbit — маркер «я здесь» переезжает
        // на цель, полоса полёта уходит.
        const wasInFlight = modalState.myPosition && modalState.myPosition.status === 'in_flight';
        modalState.myPosition = (data && data.my_position) || null;
        // Явный признак «своя система» (баг 2026-09-22) — обновляем вместе с
        // позицией: current_world_id == worldId, независимо от my_position.
        modalState.inOwnSystem = !!(data && data.in_own_system);
        // Прибытие (запрос создателя 99.2.27): камера мягко центрирует на объект
        // прибытия (позиция orbit = цель полёта), а не «вся система целиком».
        if (wasInFlight && modalState.myPosition && modalState.myPosition.status === 'orbit') {
            modalState.arrivalObject = {
                type: modalState.myPosition.object_type,
                id: modalState.myPosition.object_id,
            };
            // Прибытие внутрисистемного сегмента — глушим гул полёта и даём один
            // сигнал прибытия (конец композитного маршрута, ловушка 5; тост
            // автооткрытия notifyInfo молчит — В2=А, дубля звука нет).
            playArrival();
        }
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

    // Спрайт игрока (запрос создателя 99.2.27): openSystemModal мог установить
    // его из starInfo или /me — resetState его сбрасывает (иначе фолбэк-ромб
    // навсегда: /me-фетч для карты не запускался, т.к. starInfo имел иконку).
    // Роль (спека 2026-09-20 §6.2) — та же гонка: /me мог вернуться ДО
    // renderModal, resetState сбросил бы её — сохраняем и восстанавливаем.
    const shipIcon = modalState.shipIcon;
    const shipColor = modalState.shipColor;
    const role = modalState.role;
    // Межзвёздный полёт (баг 2026-09-24): /me асинхронный и мог вернуться ДО
    // renderModal — resetState сбрасывал interstellarFlight, режим модалки
    // становился 'intra', «Лететь» уходил во внутрисистемный старт → сервер
    // отвечал 400 «Вы в межзвёздном полёте» (пояс/планета в своей системе
    // становились недостижимы из полёта). Та же гонка, что у role/shipIcon —
    // сохраняем и восстанавливаем (имя цели — для тултипа композитной кнопки).
    const interstellarFlight = modalState.interstellarFlight;
    const interstellarFlightName = modalState.interstellarFlightName;
    resetState();
    modalState.shipIcon = shipIcon;
    modalState.shipColor = shipColor;
    modalState.role = role;
    modalState.interstellarFlight = interstellarFlight;
    modalState.interstellarFlightName = interstellarFlightName;

    const planets = Array.isArray(data && data.planets) ? data.planets : [];
    const starType = (data && data.star_type) || 'star';
    const starColor = getStarColor(spectralClass, starType);
    const starRadius = getStarSize(spectralClass, starType);

    // Оверлей
    const overlay = document.createElement('div');
    overlay.id = 'system-modal-overlay';
    // z-index не задаём: его выдаёт реестр слоёв (openLayer, полоса modal).
    overlay.style.cssText = `
        position: fixed;
        top: 0; left: 0; width: 100%; height: 100%;
        background: rgba(0,0,0,0.7);
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
    starModsBadges(data && data.stellar_mods, data && data.belts).forEach(b => chips.push(b));
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

    // Полоса внутрисистемного полёта (спека 99.2.27 §5.7, ТЗ @uidesigner):
    // между блоком чипов и контентом. Видна только при активном полёте
    // (my_position.status === 'in_flight'); таймер — в rAF-тике ниже.
    // Стили/анимации — в <style>-блоке #intra-flight-style (см. низ файла).
    const flightStrip = document.createElement('div');
    flightStrip.id = 'intra-flight-strip';
    flightStrip.innerHTML = `
        <span class="intra-flight-icon">🚀</span>
        <span class="intra-flight-route">
            <span class="intra-flight-from"></span>
            <span class="intra-flight-arrow">→</span>
            <span class="intra-flight-to"></span>
        </span>
        <span class="intra-flight-time"></span>
        <div class="intra-flight-track"><div class="intra-flight-bar"></div></div>
    `;
    modal.appendChild(flightStrip);

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

    // Кнопка «Найти меня» (запрос создателя 99.2.27, как на карте): DOM-элемент
    // поверх canvasWrapper (pointer-events только на себе — клики/хит-тесты
    // канваса не ломаются). Клик — центр на позицию игрока + переключение
    // слежения (centerOnPlayer); подсветка .active = слежение активно.
    const findMeBtn = document.createElement('button');
    findMeBtn.id = 'modal-find-me-btn';
    findMeBtn.className = 'modal-find-me-btn';
    findMeBtn.title = 'Найти меня';
    findMeBtn.textContent = '🎯';
    findMeBtn.addEventListener('click', centerOnPlayer);
    canvasWrapper.appendChild(findMeBtn);

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

    // Модалка — слой полосы modal (спека 2026-09-23 §5): Esc и клик-вне
    // (клик по подложке = вне содержимого) маршрутизирует реестр; закрытие
    // приходит в onClose → teardownModal (та же уборка, что была в closeModal).
    modalTornDown = false;
    modalLayerHandle = openLayer(overlay, {
        level: 'modal',
        contentEl: modal,
        label: titleText,
        onClose: () => { modalLayerHandle = null; teardownModal(); },
    });

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
    // Внешние компаньоны кратных (35b): из stellar_mods; id (синтетический,
    // спека 99.2.27 §3.1) — из ответа API (data.extra_companions, если есть).
    modalState.extraCompanions = Array.isArray(data.extra_companions)
        ? data.extra_companions
        : (Array.isArray(mods.extra_companions) ? mods.extra_companions : []);
    // Внутрисистемная позиция игрока (спека 99.2.27 §4.4): my_position —
    // только если игрок в этой системе; при активном полёте — status=in_flight
    // (модалка при открытии сразу показывает полосу полёта, 42a-паттерн).
    modalState.myPosition = data.my_position || null;
    // Явный признак «своя система» (баг 2026-09-22): current_world_id == worldId
    // из ответа системы. Кнопка полёта/пометка своей системы — по нему, а не по
    // наличию my_position (позиция пуста в окне прибытия/межзвёздного полёта).
    modalState.inOwnSystem = !!data.in_own_system;
    modalState.companionId = data.companion_id || null;
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
    // Пояса малых тел (спека поясов этап 2 §4.1): belts из ответа модалки.
    modalState.belts = Array.isArray(data && data.belts) ? data.belts : [];
    modalState.restricted = !!data.restricted;

    // Стартовый зум z0 по «интересной зоне» (спека §5.2/§6.3, Ф4): в кадре —
    // звезда + внутренняя орбита, не вся система (внешние орбиты честно за
    // кадром, И-В2). z0 = clamp(0.45·min(w,h)/R_zone, 0.30, 1.0), где
    // R_zone = max(2·R_звезды, мин. орбита планеты). При z0<1 барицентр (cx,cy)
    // держим в центре кадра смещением (иначе звезда уезжает вверх-влево).
    {
        const startLayout = computeLayout(planets, starRadius, width, height);
        const orbits = planets.map((p, i) => getOrbitRadius(startLayout, p, i));
        const minOrbit = orbits.length ? Math.min.apply(null, orbits) : 0;
        const rZone = Math.max(2 * startLayout.finalStarRadius, minOrbit);
        if (rZone > 0) {
            const z0 = Math.min(Math.max(0.45 * Math.min(width, height) / rZone, 0.30), 1.0);
            modalState.zoom = z0;
            modalState.offsetX = width / 2 * (1 - z0);
            modalState.offsetY = height / 2 * (1 - z0);
        }
    }

    // Колбэк перерисовки спрайтов (запрос создателя 99.2.27): пока модалка
    // открыта, асинхронная загрузка спрайта игрока перерисовывает модалку
    // (иначе остаётся фолбэк-треугольник). Сохраняем колбэк карты и ставим
    // свой; при закрытии — восстанавливаем (closeModal).
    prevRedrawCallback = getRedrawCallback();
    setRedrawCallback(() => {
        if (!document.getElementById('system-modal-overlay')) return; // guard: модалка закрыта
        drawSystem(
            modalState.canvas, modalState.spectralClass, modalState.planets,
            modalState.starRadius, modalState.starColor,
            modalState.canvasWidth, modalState.canvasHeight
        );
    });

    clearTextureCache();

    renderRightPanel(planets, null);

    initEvents(canvas, spectralClass, planets, starRadius, starColor, width, height);

    // Глобальная функция для events.js (клик по планете)
    window.updateRightPanel = (selectedIndex) => {
        // Выбор планеты кликом по канвасу — звук ui_select (только карта).
        if (selectedIndex !== null && selectedIndex !== undefined) playSound('ui_select');
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
                    // Выбор планеты кликом по строке — звук ui_select (только карта).
                    playSound('ui_select');
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
        // z0<1 держит барицентр в центре смещением; при ресайзе без ручного
        // пана/зума пересчитываем offset, иначе звезда уезжает от центра.
        if (!modalState.followDirty) {
            modalState.offsetX = newRect.width / 2 * (1 - modalState.zoom);
            modalState.offsetY = newRect.height / 2 * (1 - modalState.zoom);
        }
        drawSystem(canvas, spectralClass, planets, starRadius, starColor, newRect.width, newRect.height);
    });
    resizeObserver.observe(canvasWrapper);

    // ---- АНИМАЦИЯ: rAF-цикл вращения планет ----
    // Попутно — косметическая тень населения (extrapolate.js): раз в секунду
    // перерисовываем числа от локального счёта, чтобы между синками с сервером
    // население «жило». rAF в фоновой вкладке замирает — первый кадр после
    // возврата сразу даёт свежее число без запроса.
    let lastPopRepaint = 0;
    let lastArrivalCheck = 0;
    function tick() {
        if (!document.getElementById('system-modal-overlay')) return;
        // Слежение камеры за кораблём в полёте (запрос создателя 99.2.27):
        // до отрисовки — камера lerp-ом держит корабль в кадре; по прибытии
        // плавно возвращается к виду всей системы.
        updateCameraFollow();
        // Подсветка кнопки «Найти меня» (слежение активно) — каждый кадр
        // (classList.toggle с тем же значением — no-op, дёшево).
        updateFindMeButton();
        drawSystem(canvas, spectralClass, planets, starRadius, starColor, modalState.canvasWidth, modalState.canvasHeight);
        const popIdx = modalState.selectedPlanetIndex;
        if (popIdx !== null && popIdx !== undefined && modalState.planets[popIdx]) {
            const frameMs = performance.now();
            if (frameMs - lastPopRepaint >= 1000) {
                lastPopRepaint = frameMs;
                repaintPopulationNumbers(modalState.planets[popIdx]);
            }
        }
        // Полоса внутрисистемного полёта (спека 99.2.27 §5.7): прогресс и
        // обратный отсчёт от my_position (in_flight); по прибытии (клиентский
        // таймер) — повторный запрос планет: my_position переходит в orbit,
        // маркер «я здесь» переезжает на цель (§5.3/§5.10).
        updateFlightStrip();
        const nowMs = performance.now();
        if (nowMs - lastArrivalCheck >= 1000) {
            lastArrivalCheck = nowMs;
            const pos = modalState.myPosition;
            if (pos && pos.status === 'in_flight' && pos.arrive_at && Date.now() >= pos.arrive_at) {
                refreshPlanets();
            }
        }
        modalState._rafId = requestAnimationFrame(tick);
    }
    modalState._rafId = requestAnimationFrame(tick);

    // Опрос чужих игроков в системе (спека 99.2.27 §5.4): каждые 5 с, только
    // когда модалка открыта на свою систему (my_position != null). Маркеры
    // «кто у какого объекта» + бейдж скопления N≥2 (§5.12).
    if (modalState.myPosition) {
        loadSystemPlayers();
        modalState._playersTimer = setInterval(loadSystemPlayers, 5000);
    }
}

// updateCameraFollow — слежение камеры за кораблём в полёте (запрос создателя
// 99.2.27): dead-zone — камера жёстко держит корабль в центральной зоне
// (центр ±25% канваса; followExact — ровно в центре), точная коррекция без
// инерции (корабль не может улететь из кадра при любой скорости). По
// прибытии/отмене/выключенном слежении — быстрый возврат к 0 (вид системы);
// после прилёта — мягкий центр на объект прибытия (arrivalObject). Ручной
// пан/зум сбрасывает arrivalObject/followExact — камера свободна.
//
// «Корабль улетает» (запрос создателя): lerp 0.1/кадр отставал при высокой
// экранной скорости (близкий зум/дальняя цель), кламп ±40% упирался. Кламп
// убран — система может временно выходить из кадра (нормально для слежения);
// чёрного экрана нет: во время полёта корабль виден, после прилёта камера
// центрирует на объект прибытия.
function updateCameraFollow() {
    const pos = modalState.myPosition;
    if (!pos || pos.status !== 'in_flight' || !modalState.followEnabled) {
        // Прибытие (запрос создателя 99.2.27): мягкий центр на объект прибытия
        // (~0.5 с, lerp 0.2/кадр), камера стоит на нём (планета медленно
        // орбитирует — отслеживаем). Ручной пан/зум сбрасывает arrivalObject —
        // камера свободна (возврат к виду системы).
        if (modalState.arrivalObject && pos && pos.status === 'orbit') {
            const planets = modalState.planets || [];
            const layout = computeLayout(planets, modalState.starRadius, modalState.canvasWidth, modalState.canvasHeight);
            // Орбитальная точка (запрос создателя «корабль прыгает»): для star —
            // смещение от центра (где маркер «я здесь»), камера центрирует на маркер.
            const target = orbitalPoint(layout, planets,
                modalState.arrivalObject.type, modalState.arrivalObject.id, performance.now());
            if (isFinite(target.x) && isFinite(target.y)) {
                const targetFollowX = modalState.canvasWidth / 2 - target.x * modalState.zoom - modalState.offsetX;
                const targetFollowY = modalState.canvasHeight / 2 - target.y * modalState.zoom - modalState.offsetY;
                modalState.followOffsetX += (targetFollowX - modalState.followOffsetX) * 0.2;
                modalState.followOffsetY += (targetFollowY - modalState.followOffsetY) * 0.2;
            }
            return;
        }
        // По прибытии/отмене/выключенном слежении: быстрый возврат к виду системы.
        modalState.followOffsetX += (0 - modalState.followOffsetX) * 0.6;
        modalState.followOffsetY += (0 - modalState.followOffsetY) * 0.6;
        if (Math.abs(modalState.followOffsetX) < 0.5 && Math.abs(modalState.followOffsetY) < 0.5) {
            modalState.followOffsetX = 0;
            modalState.followOffsetY = 0;
        }
        return;
    }
    const planets = modalState.planets || [];
    const layout = computeLayout(planets, modalState.starRadius, modalState.canvasWidth, modalState.canvasHeight);
    const timeMs = performance.now();
    // Орбитальные точки (запрос создателя «корабль прыгает»): цель к звезде —
    // орбита (смещение от центра, как маркер «я здесь»), не центр — камера
    // ведёт к орбите, финальная позиция совпадает с маркером.
    const from = orbitalPoint(layout, planets, pos.from_type, pos.from_id, timeMs);
    const to = orbitalPoint(layout, planets, pos.to_type, pos.to_id, timeMs);
    const total = (pos.arrive_at || 0) - (pos.start_time || 0);
    const progress = total > 0 ? Math.min(1, Math.max(0, (Date.now() - pos.start_time) / total)) : 1;
    const shipX = from.x + (to.x - from.x) * progress;
    const shipY = from.y + (to.y - from.y) * progress;
    // NaN-гвард: битые from/to/времена не должны уводить камеру в пустоту.
    if (!isFinite(shipX) || !isFinite(shipY)) return;

    // Жёсткое слежение (запрос создателя «корабль улетает»): dead-zone —
    // камера держит корабль в центральной зоне (центр ±10% канваса — запрос
    // создателя «корабль прижимается к кромке»: было ±25%, корабль уходил к
    // краю; при followExact — ровно в центре). ТОЧНАЯ коррекция без инерции:
    // если корабль вышел из зоны — followOffset сдвигается ровно настолько,
    // чтобы вернуть его к краю зоны (корабль не может улететь из кадра при
    // любой скорости — lerp 0.1/кадр отставал, кламп ±40% упирался).
    // Кламп убран: система может временно выходить из кадра (нормально для
    // слежения); после прилёта камера центрирует на объект прибытия
    // (arrivalObject) — чёрного экрана нет.
    const shipScreenX = shipX * modalState.zoom + modalState.offsetX + modalState.followOffsetX;
    const shipScreenY = shipY * modalState.zoom + modalState.offsetY + modalState.followOffsetY;
    const zoneX = modalState.followExact ? 0 : modalState.canvasWidth * 0.1;
    const zoneY = modalState.followExact ? 0 : modalState.canvasHeight * 0.1;
    const relX = shipScreenX - modalState.canvasWidth / 2;
    const relY = shipScreenY - modalState.canvasHeight / 2;
    if (relX > zoneX) modalState.followOffsetX -= (relX - zoneX);
    else if (relX < -zoneX) modalState.followOffsetX -= (relX + zoneX);
    if (relY > zoneY) modalState.followOffsetY -= (relY - zoneY);
    else if (relY < -zoneY) modalState.followOffsetY -= (relY + zoneY);
}

// centerOnPlayer — кнопка «Найти меня» (запрос создателя 99.2.27, паттерн 42a
// + решение менеджера): во время полёта переключает режим слежения вкл/выкл.
//   - слежение вкл (по умолчанию) + вид смещён (пользователь панировал) →
//     центр на корабль (точный, followExact), слежение остаётся;
//   - слежение вкл + вид уже центрирован → выключить (камера свободна,
//     возврат к виду системы, подсветка снята);
//   - слежение выкл → включить + центр на корабль (подсветка).
// Вне полёта (orbit): клик — центр на позицию игрока, слежение не включается
// (42a), подсветки нет.
function centerOnPlayer() {
    // Межзвёздный полёт (спека 99.2.30 §6.4/§6.9): кнопка «Найти меня» в
    // модалке закрывает модалку и ведёт себя как кнопка карты — центр на
    // корабле по пути + слежение (centerOnAgent карты). Условие —
    // modalState.interstellarFlight (из /me) ИЛИ синхронный mapState.isFlying
    // (клик до резолва /me — фикс §6.4). Кнопки карты нет (админка/нет карты)
    // — просто закрыть модалку, ничего не падает.
    if (modalState.interstellarFlight || (mapStateRef && mapStateRef.isFlying)) {
        closeModal();
        const centerBtn = document.getElementById('centerBtn');
        if (centerBtn) centerBtn.click();
        return;
    }
    const pos = modalState.myPosition;
    if (!pos) return;
    const planets = modalState.planets || [];
    const layout = computeLayout(planets, modalState.starRadius, modalState.canvasWidth, modalState.canvasHeight);
    const timeMs = performance.now();
    let target;
    if (pos.status === 'in_flight') {
        // Орбитальные точки (запрос создателя «корабль прыгает»): цель к звезде —
        // орбита, не центр — «Найти меня» ведёт к той же точке, где маркер.
        const from = orbitalPoint(layout, planets, pos.from_type, pos.from_id, timeMs);
        const to = orbitalPoint(layout, planets, pos.to_type, pos.to_id, timeMs);
        const total = (pos.arrive_at || 0) - (pos.start_time || 0);
        const progress = total > 0 ? Math.min(1, Math.max(0, (Date.now() - pos.start_time) / total)) : 1;
        target = { x: from.x + (to.x - from.x) * progress, y: from.y + (to.y - from.y) * progress };
    } else if (pos.status === 'orbit' || pos.status === 'surface' || pos.status === 'mining') {
        // surface — «Найти меня» ведёт к планете прогулки (спека 2026-09-21 §7.6 п.5);
        // mining — к точке пояса добычи (ТЗ §9: пояс на схеме).
        target = orbitalPoint(layout, planets, pos.object_type, pos.object_id, timeMs);
    } else {
        return;
    }
    if (!isFinite(target.x) || !isFinite(target.y)) return;
    // Явный центр пользователя — снимаем центрирование по прибытии.
    modalState.arrivalObject = null;

    if (pos.status === 'in_flight') {
        // Запрос создателя «слежение невозможно отключить»: dead-zone держит
        // корабль в зоне ±25% — эвристика «корабль у центра» (offCenter) почти
        // всегда false → выключение не срабатывало. Теперь: клик при
        // followDirty (пользователь панировал/зумил) → центр на корабль;
        // клик без followDirty (вид не трогали) → выключить слежение.
        if (modalState.followEnabled && !modalState.followDirty) {
            // Вид не смещён пользователем — выключить слежение (камера свободна).
            modalState.followEnabled = false;
            modalState.followExact = false;
            modalState.followOffsetX = 0;
            modalState.followOffsetY = 0;
        } else {
            // Включить/перецентрировать на корабль (точный центр).
            modalState.followEnabled = true;
            modalState.followExact = true;
            modalState.followOffsetX = 0;
            modalState.followOffsetY = 0;
            modalState.offsetX = modalState.canvasWidth / 2 - target.x * modalState.zoom;
            modalState.offsetY = modalState.canvasHeight / 2 - target.y * modalState.zoom;
            modalState.followDirty = false;
        }
    } else {
        // Вне полёта: центр на позицию игрока, слежение не включается (42a).
        modalState.followEnabled = false;
        modalState.followExact = false;
        modalState.followOffsetX = 0;
        modalState.followOffsetY = 0;
        modalState.offsetX = modalState.canvasWidth / 2 - target.x * modalState.zoom;
        modalState.offsetY = modalState.canvasHeight / 2 - target.y * modalState.zoom;
        modalState.followDirty = false;
    }
    updateFindMeButton();
}

// updateFindMeButton — подсветка кнопки «Найти меня» (паттерн карты,
// events.js:538): активна, когда слежение включено и идёт полёт.
function updateFindMeButton() {
    const btn = document.getElementById('modal-find-me-btn');
    if (!btn) return;
    const active = modalState.followEnabled &&
        modalState.myPosition && modalState.myPosition.status === 'in_flight';
    btn.classList.toggle('active', active);
}

// objectLabel — имя объекта системы для полосы полёта (спека §5.7): планета/
// спутник — имя, звезда — имя системы, компаньон — «компаньон X».
function objectLabel(objType, objId) {
    const planets = modalState.planets || [];
    if (objType === 'planet') {
        const p = planets.find(x => x.id === objId);
        return p ? p.name : objId;
    }
    if (objType === 'satellite') {
        for (const p of planets) {
            const s = (p.satellites || []).find(x => x.id === objId);
            if (s) return s.name;
        }
        return objId;
    }
    if (objType === 'belt') {
        // Пояс — цель полёта (спека поясов этап 2 §7.2): имя из modalState.belts.
        const b = (modalState.belts || []).find(x => x.id === objId);
        return b ? b.name : objId;
    }
    if (objType === 'star') {
        if (objId === modalState.worldId) return modalState.worldName || 'звезда';
        if (objId === modalState.companionId) return 'компаньон';
        const m = /^extra:(.+):(\d+)$/.exec(objId || '');
        if (m) {
            const ec = (modalState.extraCompanions || [])[parseInt(m[2], 10)];
            return ec && ec.spectral_class ? 'внешний компаньон ' + ec.spectral_class : 'внешний компаньон';
        }
        return objId;
    }
    return objId;
}

// updateFlightStrip — полоса полёта (ТЗ @uidesigner): «🚀 Полёт: от → к ·
// осталось N сек» + прогресс-бар с шиммером. Скрыта вне полёта. Вызывается
// из rAF-тика. Логика (прогресс/остаток) не меняется — только разметка.
function updateFlightStrip() {
    const strip = document.getElementById('intra-flight-strip');
    if (!strip) return;
    const pos = modalState.myPosition;
    if (!pos || pos.status !== 'in_flight') {
        strip.style.display = 'none';
        return;
    }
    // Внутрисистемный сегмент (99.2.27) — держим гул полёта: на стыке
    // композитного маршрута он уже звучит (непрерывность), у одиночного
    // внутрисистемного полёта поднимаем здесь (идемпотентно).
    startFlightHum();
    strip.style.display = 'flex';
    const now = Date.now();
    const total = (pos.arrive_at || 0) - (pos.start_time || 0);
    const remaining = Math.max(0, (pos.arrive_at || 0) - now);
    const progress = total > 0 ? Math.min(1, Math.max(0, (now - pos.start_time) / total)) : 1;

    const fromEl = strip.querySelector('.intra-flight-from');
    const toEl = strip.querySelector('.intra-flight-to');
    if (fromEl) fromEl.textContent = objectLabel(pos.from_type, pos.from_id);
    if (toEl) toEl.textContent = objectLabel(pos.to_type, pos.to_id);

    // <2 с — «Заход на орбиту…» + fly-arrive (box-shadow пульс).
    const timeEl = strip.querySelector('.intra-flight-time');
    if (timeEl) {
        if (remaining < 2000) {
            timeEl.textContent = '· Заход на орбиту…';
            strip.classList.add('fly-arrive');
        } else {
            timeEl.textContent = `· осталось ${Math.ceil(remaining / 1000)} сек`;
            strip.classList.remove('fly-arrive');
        }
    }

    // Прогресс-бар: последние 10% — fly-final (opacity пульс).
    const bar = strip.querySelector('.intra-flight-bar');
    if (bar) {
        bar.style.width = Math.round(progress * 100) + '%';
        bar.classList.toggle('fly-final', progress > 0.9);
    }
}

// loadSystemPlayers — чужие игроки в этой системе (спека §5.4): фильтр
// /api/players/positions по object_type/object_id системы. Чужие внутрисистемно
// летящие в модалке НЕ показываются (v1) — только стоящие.
async function loadSystemPlayers() {
    const token = modalState.authToken || localStorage.getItem('token');
    if (!token) return;
    try {
        const res = await fetch('/api/players/positions', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (!res.ok) return;
        const data = await res.json();
        const all = Array.isArray(data.players) ? data.players : [];
        const planets = modalState.planets || [];
        modalState.systemPlayers = all.filter(p => {
            if (!p.object_type || !p.object_id) return false;
            if (p.object_type === 'star') {
                return p.object_id === modalState.worldId || p.object_id === modalState.companionId ||
                    (typeof p.object_id === 'string' && p.object_id.startsWith('extra:' + modalState.worldId + ':'));
            }
            if (p.object_type === 'planet') {
                return planets.some(x => x.id === p.object_id);
            }
            if (p.object_type === 'satellite') {
                return planets.some(x => (x.satellites || []).some(s => s.id === p.object_id));
            }
            return false;
        });
    } catch (e) {
        // Тихий сбой: модалка работает и без чужих игроков.
    }
}

// ==================== ЗАКРЫТИЕ ====================

// closeModal — единственная точка закрытия модалки (кнопка ✕, полёты,
// карта). Если слой открыт в реестре — закрываем через него (реестр вызовет
// onClose → teardownModal); иначе убираем напрямую (повторный вызов).
export function closeModal() {
    if (modalLayerHandle) { modalLayerHandle.close(); return; }
    teardownModal();
}

// teardownModal — уборка модалки (тело прежнего closeModal). Идемпотентна:
// вызывается из onClose слоя и напрямую; повторный вход — no-op.
function teardownModal() {
    if (modalTornDown) return;
    modalTornDown = true;
    modalLayerHandle = null;
    // Слои выше модалки (меню, алерт) закрываем через реестр ДО снятия DOM —
    // их onClose сам убирает свои узлы (без призрачных записей в стеке, §5).
    closeToLayer('modal', { inclusive: true });
    const overlay = document.getElementById('system-modal-overlay');
    if (overlay) overlay.remove();
    // Гул: закрытие модалки на внутрисистемном полёте глушит гул (полоса
    // сегмента больше не видна). В межзвёздном полёте гул принадлежит карте
    // (flight.js/animation.js) — здесь не трогаем.
    if (!(mapStateRef && mapStateRef.isFlying)) stopFlightHum();
    // Большая картинка «Вид с орбиты» (спека 2026-09-20 §6.2): сброс
    // состояния при закрытии модалки (revoke object URL, если есть).
    cleanupOrbitView();
    resetState();
    // Восстанавливаем колбэк перерисовки карты/дашборда (запрос создателя
    // 99.2.27): модалка закрыта — её колбэк больше не нужен.
    setRedrawCallback(prevRedrawCallback);
    prevRedrawCallback = null;
    // Меню/тултип модалки уже сняты реестром (closeToLayer выше) — это
    // страховка от «висящих» узлов, если слой закрыли в обход реестра.
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

// Стиль полосы внутрисистемного полёта (ТЗ @uidesigner): градиент контейнера,
// рамка + свечение, 🚀 fly-bounce, «→» fly-pulse, шиммер-блик бара, финальные
// 10% fly-final, «Заход на орбиту…» fly-arrive. Один блок, guard по id.
if (!document.getElementById('intra-flight-style')) {
    const style = document.createElement('style');
    style.id = 'intra-flight-style';
    style.textContent = `
        #intra-flight-strip {
            display: none;
            align-items: center;
            gap: 10px;
            margin-bottom: 10px;
            padding: 8px 12px;
            background: linear-gradient(90deg, rgba(251,191,36,0.10), rgba(251,191,36,0.04));
            border: 1px solid rgba(251,191,36,0.45);
            border-radius: 10px;
            font-size: 0.9rem;
            color: #fde68a;
            box-shadow: 0 0 12px rgba(251,191,36,0.15);
        }
        .intra-flight-icon {
            display: inline-block;
            animation: fly-bounce 1.2s ease-in-out infinite;
            text-shadow: 0 0 8px rgba(251,191,36,0.6);
        }
        .intra-flight-arrow {
            display: inline-block;
            animation: fly-pulse 1.2s ease-in-out infinite;
        }
        .intra-flight-track {
            flex: 1;
            height: 6px;
            background: #2a2a44;
            border-radius: 3px;
            overflow: hidden;
            position: relative;
        }
        .intra-flight-bar {
            width: 0%;
            height: 100%;
            background: linear-gradient(90deg, #fbbf24, #fde68a);
            border-radius: 3px;
            box-shadow: 0 0 8px rgba(251,191,36,0.5);
            position: relative;
            overflow: hidden;
        }
        .intra-flight-bar::after {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            width: 40%;
            height: 100%;
            background: linear-gradient(90deg, transparent, rgba(255,255,255,0.5), transparent);
            animation: fly-shine 1.6s linear infinite;
        }
        .intra-flight-bar.fly-final {
            animation: fly-final 0.8s ease-in-out infinite;
        }
        #intra-flight-strip.fly-arrive {
            animation: fly-arrive 0.6s ease-in-out infinite;
        }
        @keyframes fly-bounce {
            0%, 100% { transform: translateY(0); }
            50% { transform: translateY(-3px); }
        }
        @keyframes fly-pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.4; }
        }
        @keyframes fly-shine {
            0% { transform: translateX(-100%); }
            100% { transform: translateX(300%); }
        }
        @keyframes fly-final {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        @keyframes fly-arrive {
            0%, 100% { box-shadow: 0 0 8px rgba(251,191,36,0.2); }
            50% { box-shadow: 0 0 18px rgba(251,191,36,0.6); }
        }
    `;
    document.head.appendChild(style);
}

// Стиль кнопки «Найти меня» (запрос создателя 99.2.27, как на карте):
// круглая, тёмная; .active — слежение активно (подсветка, паттерн карты
// events.js:538). Один блок, guard по id.
if (!document.getElementById('modal-find-me-style')) {
    const style = document.createElement('style');
    style.id = 'modal-find-me-style';
    style.textContent = `
        .modal-find-me-btn {
            position: absolute;
            top: 8px;
            right: 8px;
            z-index: 10;
            background: #1e293b;
            border: 1px solid #334155;
            color: #fff;
            width: 2.2rem;
            height: 2.2rem;
            border-radius: 50%;
            font-size: 1rem;
            cursor: pointer;
            transition: 0.2s;
        }
        .modal-find-me-btn:hover { background: #334155; }
        .modal-find-me-btn.active {
            background: #2563eb;
            border-color: #3b82f6;
            box-shadow: 0 0 0 2px rgba(59,130,246,0.4);
        }
        .modal-find-me-btn.active:hover { background: #2563eb; }
    `;
    document.head.appendChild(style);
}

// Экспорт в window
window.openSystemModal = openSystemModal;