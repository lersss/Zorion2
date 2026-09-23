// web/static/js/modal/events.js
import { modalState, flightModeForSystem } from './state.js';
import { drawSystem, MIN_STAR_PX, orbitalPoint } from './modal_render.js';
import { computeLayout, getOrbitRadius, getPlanetAngle, planetRadius, planetOrbitCenter, beltRing, beltNearestAngle, setBeltArrivalAngle } from './layout.js';
import { miniObjects } from './minimap.js';
import { closeModal } from './index.js';
import { getSpectralInfo, exoticStarInfo, formatStellarMass, formatAU } from './panel.js';
import { starTypeLabel } from './utils.js';
import { notifyError } from '../ui/toast.js';
import { biomeIconHtml, prettyName } from './tabs.js';
// Реестр слоёв (спека 2026-09-23 §5): контекстные меню модалки — полоса menu
// (маршрутизируемые), тултип звезды — пассивный слой той же полосы. Закрытие —
// только через handle.close() (иначе в стеке остаётся мёртвая запись).
// openAlert (полоса alert, диалог поверх модалки) — из ui/alert.js.
import { openLayer } from '../ui/layers.js';
import { openAlert } from '../ui/alert.js';

export function initEvents(canvas, spectralClass, planets, starRadius, starColor, width, height) {
    const dpr = window.devicePixelRatio || 1;

    function getAnimTime() {
        // Глобальные часы (51a): фаза планет не сбрасывается при переоткрытии модалки.
        return performance.now();
    }

    function getPlanetWorldPos(p, idx) {
        const layout = computeLayout(planets, starRadius, width, height);
        const orbitRadius = getOrbitRadius(layout, p, idx);
        const angle = getPlanetAngle(p, orbitRadius, idx, getAnimTime());
        const radius = planetRadius(p.size);
        const center = planetOrbitCenter(layout, p);
        const px = center.x + orbitRadius * Math.cos(angle);
        const py = center.y + orbitRadius * Math.sin(angle);
        return { px, py, radius, finalStarRadius: layout.finalStarRadius };
    }

    function hitTest(worldX, worldY) {
        const layout = computeLayout(planets, starRadius, width, height);
        // Звезда — по позиции главной (в тесной паре она смещена от барицентра).
        // Радиус — как у рендера (70a): с экранным минимумом MIN_STAR_PX, иначе
        // на низком зуме hit-область (s.radius+8)·zoom ≈ 0.04px не ловит клик.
        const distToStar = Math.hypot(worldX - layout.mainX, worldY - layout.mainY);
        if (distToStar < Math.max(layout.finalStarRadius, MIN_STAR_PX / modalState.zoom) + 8) return 'star';

        // Компаньоны/внешние компаньоны (35b): по честным координатам layout.stars
        // (stars[0] — главная, уже проверена выше).
        for (let i = 1; i < layout.stars.length; i++) {
            const s = layout.stars[i];
            if (Math.hypot(worldX - s.x, worldY - s.y) < Math.max(s.radius, MIN_STAR_PX / modalState.zoom) + 8) {
                return { type: 'star', starIndex: i };
            }
        }

        for (let idx = 0; idx < planets.length; idx++) {
            const { px, py, radius } = getPlanetWorldPos(planets[idx], idx);
            if (Math.hypot(worldX - px, worldY - py) < radius + 6) {
                return { type: 'planet', index: idx };
            }
        }

        // Пояса (ТЗ §9.4): точка в полосе кольца. Планеты/звёзды проверены
        // раньше — клик по планете поверх кольца = планета (приоритет). Та же
        // геометрия, что у отрисовки (beltRing, §9.4 — «кликается там, где
        // нарисовано»).
        const belts = modalState.belts || [];
        for (const b of belts) {
            const g = beltRing(layout, b);
            if (!isFinite(g.radius) || g.radius <= 0) continue;
            const d = Math.hypot(worldX - g.cx, worldY - g.cy);
            const tol = Math.max(g.half, 4 / modalState.zoom);
            if (Math.abs(d - g.radius) <= tol) return { type: 'belt', id: b.id };
        }
        return null;
    }

    function toWorld(e) {
        const rect = canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (canvas.width / rect.width) / dpr;
        const mouseY = (e.clientY - rect.top) * (canvas.height / rect.height) / dpr;
        // Слежение камеры (запрос создателя 99.2.27): followOffset сдвигает
        // отрисовку — хит-тест обязан учитывать его, иначе клики/ховер/ПКМ
        // во время полёта попадали бы мимо объектов.
        const worldX = (mouseX - modalState.offsetX - (modalState.followOffsetX || 0)) / modalState.zoom;
        const worldY = (mouseY - modalState.offsetY - (modalState.followOffsetY || 0)) / modalState.zoom;
        return { worldX, worldY };
    }

    // ---- HOVER ----
    canvas.addEventListener('mousemove', (e) => {
        const { worldX, worldY } = toWorld(e);
        const hit = hitTest(worldX, worldY);

        if (hit === 'star') {
            if (modalState.hoveredObject !== 'star') {
                modalState.hoveredObject = 'star';
                canvas.style.cursor = 'pointer';
            }
        } else if (hit && hit.type === 'star') {
            if (!modalState.hoveredObject || modalState.hoveredObject.type !== 'star' || modalState.hoveredObject.starIndex !== hit.starIndex) {
                modalState.hoveredObject = hit;
                canvas.style.cursor = 'pointer';
            }
        } else if (hit && hit.type === 'planet') {
            if (!modalState.hoveredObject || modalState.hoveredObject.type !== 'planet' || modalState.hoveredObject.index !== hit.index) {
                modalState.hoveredObject = hit;
                canvas.style.cursor = 'pointer';
            }
        } else if (hit && hit.type === 'belt') {
            // Кольцо пояса читается как интерактив (подсветка в modal_render +
            // cursor: pointer, ТЗ §9.4).
            if (!modalState.hoveredObject || modalState.hoveredObject.type !== 'belt' || modalState.hoveredObject.id !== hit.id) {
                modalState.hoveredObject = hit;
                canvas.style.cursor = 'pointer';
            }
        } else {
            if (modalState.hoveredObject !== null) {
                modalState.hoveredObject = null;
                canvas.style.cursor = modalState.isDragging ? 'grabbing' : 'default';
            }
        }

        // Тултип звезды (70a): у курсора при ховере на звезду (кроме драга),
        // при уходе с объекта/канваса — прячем.
        if (!modalState.isDragging && (hit === 'star' || (hit && hit.type === 'star'))) {
            showStarTooltip(e, hit);
        } else {
            hideStarTooltip();
        }
    });

    canvas.addEventListener('mouseleave', () => {
        if (modalState.hoveredObject !== null) {
            modalState.hoveredObject = null;
            canvas.style.cursor = modalState.isDragging ? 'grabbing' : 'default';
        }
        hideStarTooltip();
    });

    // ---- WHEEL ZOOM ----
    canvas.addEventListener('wheel', (e) => {
        e.preventDefault();
        const rect = canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (canvas.width / rect.width) / dpr;
        const mouseY = (e.clientY - rect.top) * (canvas.height / rect.height) / dpr;

        // Адаптивный шаг зума (51a): на обзоре (zoom < 0.05) отдаление/приближение
        // быстрее — от 1 до ~0.0006 (Ugvol, внешняя на 8226 а.е.) ~30 прокруток;
        // на обычных зумах (zoom ≥ 0.3) поведение как раньше.
        let delta;
        if (modalState.zoom < 0.05) {
            delta = e.deltaY > 0 ? 0.65 : 1.5;
        } else if (modalState.zoom < 0.3) {
            delta = e.deltaY > 0 ? 0.8 : 1.25;
        } else {
            delta = e.deltaY > 0 ? 0.9 : 1.1;
        }

        // Динамический минимум зума (51a): от самой дальней звезды системы,
        // чтобы все звёзды (компаньоны/внешние кратных) влезали в кадр.
        const layout = computeLayout(planets, starRadius, width, height);
        const minZoom = layout.maxStarDistPx > 0
            ? Math.max(0.0003, Math.min(0.02, modalState.canvasWidth / (2 * layout.maxStarDistPx)))
            : 0.02;
        const newZoom = Math.min(Math.max(modalState.zoom * delta, minZoom), 5);

        const worldX = (mouseX - modalState.offsetX - (modalState.followOffsetX || 0)) / modalState.zoom;
        const worldY = (mouseY - modalState.offsetY - (modalState.followOffsetY || 0)) / modalState.zoom;
        modalState.zoom = newZoom;
        // Ручной зум: если камера удерживалась на объекте прибытия
        // (arrivalObject) — освобождаем: followOffset к 0 (вид системы) ДО
        // пересчёта offsetX/Y. Иначе offsetX/Y считались с учётом followOffset,
        // а он обнуляется в updateCameraFollow → вид «висит» в смещённой точке,
        // объекты пропадают из кадра (запрос создателя «после зума пропадают
        // все объекты»). Во время полёта (arrivalObject нет) followOffset —
        // позиция слежения, не трогаем (зум вокруг видимого центра).
        if (modalState.arrivalObject) {
            modalState.arrivalObject = null;
            modalState.followOffsetX = 0;
            modalState.followOffsetY = 0;
        }
        modalState.followExact = false;
        // Пользователь зумил — следующий клик «Найти меня» центрирует.
        modalState.followDirty = true;
        // Слежение камеры: followOffset отдельно — offsetX считаем с его учётом,
        // чтобы точка под курсором не уезжала.
        modalState.offsetX = mouseX - worldX * modalState.zoom - (modalState.followOffsetX || 0);
        modalState.offsetY = mouseY - worldY * modalState.zoom - (modalState.followOffsetY || 0);
    }, { passive: false });

    // ---- DRAG ----
    canvas.addEventListener('mousedown', (e) => {
        modalState.isDragging = true;
        modalState.dragMoved = false;
        modalState.dragStartX = e.clientX;
        modalState.dragStartY = e.clientY;
        modalState.dragStartOffsetX = modalState.offsetX;
        modalState.dragStartOffsetY = modalState.offsetY;
        canvas.style.cursor = 'grabbing';
        hideStarTooltip();
    });

    window.addEventListener('mousemove', (e) => {
        if (modalState.isDragging) {
            const dx = (e.clientX - modalState.dragStartX) / dpr;
            const dy = (e.clientY - modalState.dragStartY) / dpr;
            modalState.offsetX = modalState.dragStartOffsetX + dx;
            modalState.offsetY = modalState.dragStartOffsetY + dy;
            // Ручной пан: слежение возвращается к смещению к цели (не точный
            // центр после «Найти меня»); центрирование по прибытии снимается —
            // followOffset к 0 (вид системы), иначе вид «висит» в смещённой
            // точке (объекты пропадают, запрос создателя).
            if (modalState.arrivalObject) {
                modalState.arrivalObject = null;
                modalState.followOffsetX = 0;
                modalState.followOffsetY = 0;
            }
            modalState.followExact = false;
            // Пользователь панировал — следующий клик «Найти меня» центрирует
            // (а не выключает слежение).
            modalState.followDirty = true;
            if (Math.hypot(dx, dy) > 3) {
                modalState.dragMoved = true;
            }
        }
    });

    window.addEventListener('mouseup', () => {
        if (modalState.isDragging) {
            modalState.isDragging = false;
            modalState.suppressNextClick = modalState.dragMoved;
            canvas.style.cursor = modalState.hoveredObject ? 'pointer' : 'default';
        }
    });

    // ---- CLICK ----
    canvas.addEventListener('click', (e) => {
        // Клик после реального драга (перемещение > 3px) — не считается.
        if (modalState.suppressNextClick) {
            modalState.suppressNextClick = false;
            return;
        }
        const rect = canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (canvas.width / rect.width) / dpr;
        const mouseY = (e.clientY - rect.top) * (canvas.height / rect.height) / dpr;

        // Клик по миникарте (51a): ближайший объект — фокус на него; мимо
        // объектов — игнор. Как клик по канвасу не обрабатывается. Размеры —
        // свежие из modalState (после ресайза окна замыкание width/height устарело).
        const mini = miniObjects(planets, modalState.canvasWidth, modalState.canvasHeight);
        if (mouseX >= mini.miniX && mouseX <= mini.miniX + mini.miniSize &&
            mouseY >= mini.miniY && mouseY <= mini.miniY + mini.miniSize) {
            let best = null;
            let bestDist = 14;
            mini.objects.forEach(o => {
                const d = Math.hypot(mouseX - o.x, mouseY - o.y);
                if (d < bestDist) {
                    bestDist = d;
                    best = o;
                }
            });
            if (best) {
                // Клик по миникарте — явный фокус: сбрасываем слежение камеры
                // (иначе followOffset сдвинул бы вид от выбранного объекта).
                modalState.followOffsetX = 0;
                modalState.followOffsetY = 0;
                modalState.followExact = false;
                modalState.arrivalObject = null;
                // Пользователь выбрал объект — следующий клик «Найти меня» центрирует.
                modalState.followDirty = true;
                modalState.offsetX = modalState.canvasWidth / 2 - best.world.x * modalState.zoom;
                modalState.offsetY = modalState.canvasHeight / 2 - best.world.y * modalState.zoom;
                drawSystem(canvas, spectralClass, planets, starRadius, starColor, modalState.canvasWidth, modalState.canvasHeight);
            }
            return;
        }

        const { worldX, worldY } = toWorld(e);
        const hit = hitTest(worldX, worldY);

        if (hit === 'star' || (hit && hit.type === 'star')) {
            modalState.selectedPlanetIndex = null;
            modalState.selectedObject = { type: 'star' };
            if (typeof window.showStarCard === 'function') {
                window.showStarCard();
            }
            return;
        }

        if (hit && hit.type === 'planet') {
            modalState.selectedPlanetIndex = hit.index;
            modalState.selectedObject = { type: 'planet', index: hit.index };
            if (typeof window.updateRightPanel === 'function') {
                window.updateRightPanel(hit.index);
            }
        } else {
            if (modalState.selectedPlanetIndex !== null || modalState.selectedObject !== null) {
                modalState.selectedPlanetIndex = null;
                modalState.selectedObject = null;
                if (typeof window.updateRightPanel === 'function') {
                    window.updateRightPanel(null);
                }
            }
        }
    });

    // ---- КОНТЕКСТНОЕ МЕНЮ (ПКМ) ----
    canvas.addEventListener('contextmenu', (e) => {
        e.preventDefault();
        const { worldX, worldY } = toWorld(e);
        const hit = hitTest(worldX, worldY);
        if (hit === 'star') {
            showStarMenu(e.clientX, e.clientY);
        } else if (hit && hit.type === 'star') {
            // Компаньон/внешний компаньон (спека 99.2.27 §5.2, решение создателя
            // 2026-09-20): ПКМ по компаньону → «Лететь».
            showCompanionMenu(e.clientX, e.clientY, hit.starIndex);
        } else if (hit && hit.type === 'planet') {
            // Планета (претензия создателя «как на карте»): ПКМ → «Лететь».
            showPlanetMenu(e.clientX, e.clientY, hit.index);
        } else if (hit && hit.type === 'belt') {
            // Пояс (ТЗ §9.4): ПКМ → «Лететь»/«Добывать». Точка клика (worldX/worldY)
            // едет в меню: «Лететь» летит в место клика (правка создателя 2026-09-23).
            showBeltMenu(e.clientX, e.clientY, hit.id, worldX, worldY);
        } else {
            hideStarMenu();
        }
    });

}

// ---- ТУЛТИП ЗВЕЗДЫ (70a) ----
// starTooltipEl — тултип информации о звезде при наведении на канвасе модалки.
// Создаётся лениво один раз; pointer-events: none — не перехватывает клики.
// Прячется при уходе мыши с объекта/канваса и при драге; удаляется в closeModal.
// Пассивный слой полосы menu (спека §1/§2): получает z, но вне Esc/клик-вне.
let starTooltipEl = null;
let starTooltipHandle = null;

function getStarTooltipEl() {
    // isConnected: closeModal удаляет элемент из DOM — при повторном открытии
    // модалки пересоздаём (иначе ссылка ведёт на отсоединённый узел). Заодно
    // снимаем «мёртвую» запись прежнего слоя: тултип пассивный, реестр его по
    // закрытию модалки не гасит (F2) — иначе новый тултип не зарегистрируется.
    if (!starTooltipEl || !starTooltipEl.isConnected) {
        if (starTooltipHandle) starTooltipHandle.close();
        starTooltipEl = document.createElement('div');
        starTooltipEl.id = 'star-tooltip';
        starTooltipEl.style.cssText = `
            position: fixed;
            pointer-events: none;
            background: #1a1a2e;
            border: 1px solid #334155;
            border-radius: 8px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.5);
            padding: 8px 10px;
            font-size: 0.9rem;
            color: #e0e0e0;
            user-select: none;
            max-width: 260px;
            display: none;
        `;
        document.body.appendChild(starTooltipEl);
    }
    return starTooltipEl;
}

// hideStarTooltip — скрытие через реестр (без «призрачной» записи в стеке);
// собственное display:none остаётся фолбэком для узла без слоя.
function hideStarTooltip() {
    if (starTooltipHandle) { starTooltipHandle.close(); return; }
    if (starTooltipEl) starTooltipEl.style.display = 'none';
}

// showStarTooltip — показывает тултип у курсора. hit: 'star' (главная) или
// {type:'star', starIndex} (компаньон/внешний из layout.stars).
function showStarTooltip(e, hit) {
    const el = getStarTooltipEl();
    if (!starTooltipHandle) {
        starTooltipHandle = openLayer(el, {
            level: 'menu',
            passive: true,
            closeOnEsc: false,
            closeOnOutside: false,
            trapFocus: false,
            onClose: () => {
                starTooltipHandle = null;
                if (starTooltipEl) starTooltipEl.style.display = 'none';
            },
        });
    }
    el.innerHTML = buildStarTooltipHtml(hit);
    el.style.display = 'block';
    // Смещение от курсора (14px); у правого края — влево, чтобы не уходить за экран.
    const left = e.clientX + 14 + el.offsetWidth > window.innerWidth
        ? e.clientX - el.offsetWidth - 14
        : e.clientX + 14;
    el.style.left = Math.max(4, left) + 'px';
    // Клампинг по нижнему краю (70a): тултип не вылезает за вьюпорт.
    const top = Math.max(4, e.clientY - 10);
    el.style.top = Math.max(4, Math.min(top, window.innerHeight - el.offsetHeight - 4)) + 'px';
}

// tooltipRowsHtml — строки «ключ: значение» компактного тултипа.
function tooltipRowsHtml(rows) {
    return rows.map(([k, v]) =>
        `<div style="line-height:1.5;"><span style="color:#888;">${k}:</span> ${v}</div>`
    ).join('');
}

// buildStarTooltipHtml — содержимое тултипа (70a): вся информация из карточки
// звезды без количества планет. Главная — по modalState; компаньон/внешний —
// по kind звезды в layout.stars (данные из modalState.companion* /
// extraCompanions).
function buildStarTooltipHtml(hit) {
    const exotic = modalState.starType && modalState.starType !== 'star';
    const spec = modalState.spectralClass || '';
    const temp = modalState.worldTemperature;

    if (hit === 'star') {
        const specInfo = exotic ? null : getSpectralInfo(spec);
        const exoticInfo = exotic ? exoticStarInfo() : null;
        const typeText = exotic ? (starTypeLabel(modalState.starType) || modalState.starType) : specInfo.type;
        const tempText = exotic
            ? exoticInfo.temperature
            : (temp ? (temp - 273.15).toFixed(0) + ' °C' + ' (' + temp.toFixed(0) + ' K)' : '—');
        let html = tooltipRowsHtml([
            ['Тип', typeText],
            ['Температура', tempText],
            ['Масса', formatStellarMass(modalState.stellarMass)],
            ['Цвет', exotic ? exoticInfo.color : specInfo.color],
            ['Радиус', exotic ? exoticInfo.radius : specInfo.radius],
            ['Светимость', exotic ? exoticInfo.luminosity : specInfo.luminosity],
            [exotic ? 'Возраст' : 'Срок жизни', exotic ? exoticInfo.age : specInfo.age],
        ]);
        if (!exotic && specInfo.description) {
            html += `<div style="margin-top:6px; color:#888; font-size:0.85rem;">${specInfo.description}</div>`;
        }
        return html;
    }

    // Компаньон/внешний компаньон: источник данных по kind звезды в layout.
    const layout = computeLayout(modalState.planets, modalState.starRadius, modalState.canvasWidth, modalState.canvasHeight);
    const star = layout.stars[hit.starIndex];
    if (!star) return '';
    if (star.kind === 'companion') {
        const info = getSpectralInfo(modalState.companion || '');
        return tooltipRowsHtml([
            ['Тип', modalState.companion ? info.type : 'Компаньон'],
            ['Температура', typeof modalState.companionTemp === 'number' ? modalState.companionTemp.toFixed(0) + ' K' : '—'],
            ['Масса', formatStellarMass(modalState.companionMass)],
            ['До доминантной', typeof modalState.companionSepAU === 'number' ? formatAU(modalState.companionSepAU) + ' а.е.' : '—'],
            ['Цвет', info.color],
            ['Радиус', info.radius],
            ['Светимость', info.luminosity],
        ]);
    }
    // kind === 'extra': внешний компаньон кратной (35b §6.2) — stars[2..]
    // соответствуют extraCompanions по порядку.
    const ec = (modalState.extraCompanions || [])[hit.starIndex - 2];
    if (!ec) return '';
    const info = getSpectralInfo(ec.spectral_class || '');
    return tooltipRowsHtml([
        ['Спектр', ec.spectral_class || '—'],
        ['Температура', typeof ec.temp === 'number' ? ec.temp.toFixed(0) + ' K' : '—'],
        ['До доминантной', typeof ec.sep_au === 'number' ? formatAU(ec.sep_au) + ' а.е.' : '—'],
        ['Цвет', info.color],
        ['Радиус', info.radius],
        ['Светимость', info.luminosity],
    ]);
}

function showStarMenu(x, y) {
    hideStarMenu();

    const menu = document.createElement('div');
    menu.id = 'star-context-menu';
    menu.style.cssText = `
        position: fixed;
        left: ${x}px;
        top: ${y}px;
        background: #1a1a2e;
        border: 1px solid #334155;
        border-radius: 8px;
        box-shadow: 0 8px 24px rgba(0,0,0,0.5);
        padding: 4px;
        min-width: 160px;
        font-size: 0.9rem;
        color: #e0e0e0;
    `;

    const title = document.createElement('div');
    title.style.cssText = `
        padding: 6px 10px;
        font-size: 0.75rem;
        color: #888;
        border-bottom: 1px solid #2a2a44;
        margin-bottom: 4px;
    `;
    title.textContent = modalState.worldName || 'Система';
    menu.appendChild(title);

    // Внутрисистемный полёт доступен только в своей системе (спека 99.2.27
    // §5.1): явный флаг сервера (flightModeForSystem), а не my_position != null
    // (баг 2026-09-22: в окне прибытия/межзвёздного полёта позиция пуста). В
    // своей системе «Лететь» из модалки убирается (М-6, осознанно) — вместо
    // него «Лететь».
    if (flightModeForSystem() === 'intra') {
        // ПКМ-пункт скрывается, если игрок уже на орбите звезды (§5.6) или
        // летит ОТ этой звезды (запрос создателя «глупый тост»: цель == from
        // полёта — полёт к ней запрещён, отмены в UI нет — пункт не показываем).
        const pos = modalState.myPosition;
        const onStarOrbit = !!pos && pos.status === 'orbit' && pos.object_type === 'star' && pos.object_id === modalState.worldId;
        const flyingFromStar = !!pos && pos.status === 'in_flight' && pos.from_type === 'star' && pos.from_id === modalState.worldId;
        if (!onStarOrbit && !flyingFromStar) {
            const btn = document.createElement('div');
            btn.style.cssText = `
                padding: 8px 10px;
                cursor: pointer;
                border-radius: 6px;
                display: flex;
                align-items: center;
                gap: 8px;
            `;
            btn.innerHTML = `🚀 <span>Лететь</span>`;
            btn.addEventListener('mouseenter', () => { btn.style.background = '#2a2a44'; });
            btn.addEventListener('mouseleave', () => { btn.style.background = 'none'; });
            btn.addEventListener('click', async () => {
                hideStarMenu();
                if (!modalState.hasEngine) {
                    notifyError('Двигатель не установлен — полёт невозможен');
                    return;
                }
                await startIntraFlight('star', modalState.worldId);
            });
            menu.appendChild(btn);
        }
    } else if (!modalState.authToken) {
        // Чужая система / 403: существующий «Лететь» (межзвёздный, §5.5).
        const btn = document.createElement('div');
        btn.style.cssText = `
            padding: 8px 10px;
            cursor: pointer;
            border-radius: 6px;
            display: flex;
            align-items: center;
            gap: 8px;
        `;
        btn.innerHTML = `🚀 <span>Лететь</span>`;
        btn.addEventListener('mouseenter', () => { btn.style.background = '#2a2a44'; });
        btn.addEventListener('mouseleave', () => { btn.style.background = 'none'; });
        btn.addEventListener('click', async () => {
            hideStarMenu();
            // Без двигателя полёт невозможен (спека 91a §6.1): блокируем с
            // подсказкой; сервер валидирует тоже (админ/skycomposer — исключение).
            if (!modalState.hasEngine) {
                notifyError('Двигатель не установлен — полёт невозможен');
                return;
            }
            await startTravelToStar();
        });
        menu.appendChild(btn);
    }

    openStarMenu(menu);
}

// showCompanionMenu — ПКМ по компаньону/внешнему компаньону (спека 99.2.27
// §5.2 + 99.2.30 §6.1, решение создателя 2026-09-21): пункт «🚀 Лететь».
// Своя система (my_position != null) — внутрисистемный полёт (startIntraFlight,
// цель — синтетический id companion:<world> / extra:<world>:<i>); чужая система
// (my_position == null, компаньон виден на канвасе) — композитный маршрут
// (startCompositeFlight('companion', targetId), модалка закрывается, карта ведёт
// корабль). В своей системе пункт скрыт если: уже на орбите этого компаньона,
// летим ОТ него, активный межзвёздный (иначе старт даст 400 «Вы в межзвёздном
// полёте»). Без двигателя — пункт есть, клик → тост (как ПКМ звезды/планеты).
function showCompanionMenu(x, y, starIndex) {
    hideStarMenu();

    const layout = computeLayout(modalState.planets, modalState.starRadius, modalState.canvasWidth, modalState.canvasHeight);
    const star = layout.stars[starIndex];
    if (!star) return;

    // Синтетический id цели: companion:<world> (главный компаньон) или
    // extra:<world>:<i> (внешний компаньон кратной, stars[2..] ↔ extraCompanions).
    let targetId = null;
    let label = 'компаньон';
    if (star.kind === 'companion') {
        targetId = modalState.companionId;
        label = modalState.companion ? 'компаньон ' + modalState.companion : 'компаньон';
    } else if (star.kind === 'extra') {
        const i = starIndex - 2;
        const ec = (modalState.extraCompanions || [])[i];
        targetId = 'extra:' + modalState.worldId + ':' + i;
        label = ec && ec.spectral_class ? 'внешний компаньон ' + ec.spectral_class : 'внешний компаньон';
    }
    if (!targetId) return;

    // Своя система — внутрисистемный полёт (как раньше); чужая — композитный
    // маршрут (спека 99.2.30 §6.1, решение создателя 2026-09-21). «Своя система»
    // — явный флаг сервера, а не my_position != null (баг 2026-09-22).
    const myPos = modalState.myPosition;
    const intra = flightModeForSystem() === 'intra';
    if (myPos) {
        // Скрыт, если игрок уже на орбите этого компаньона (§5.6) или летит ОТ
        // него (запрос создателя «глупый тост»: цель == from полёта).
        if (myPos.status === 'orbit' && myPos.object_type === 'star' && myPos.object_id === targetId) return;
        if (myPos.status === 'in_flight' && myPos.from_type === 'star' && myPos.from_id === targetId) return;
        // Своя система: при активном межзвёздном полёте внутрисистемный старт
        // даст 400 «Вы в межзвёздном полёте» (спека 99.2.30 §6.2) — не показываем.
        if (modalState.interstellarFlight) return;
    }

    const menu = document.createElement('div');
    menu.id = 'star-context-menu';
    menu.style.cssText = `
        position: fixed;
        left: ${x}px;
        top: ${y}px;
        background: #1a1a2e;
        border: 1px solid #334155;
        border-radius: 8px;
        box-shadow: 0 8px 24px rgba(0,0,0,0.5);
        padding: 4px;
        min-width: 180px;
        font-size: 0.9rem;
        color: #e0e0e0;
    `;

    const title = document.createElement('div');
    title.style.cssText = `
        padding: 6px 10px;
        font-size: 0.75rem;
        color: #888;
        border-bottom: 1px solid #2a2a44;
        margin-bottom: 4px;
    `;
    title.textContent = label;
    menu.appendChild(title);

    const btn = document.createElement('div');
    btn.style.cssText = `
        padding: 8px 10px;
        cursor: pointer;
        border-radius: 6px;
        display: flex;
        align-items: center;
        gap: 8px;
    `;
    btn.innerHTML = `🚀 <span>Лететь</span>`;
    btn.addEventListener('mouseenter', () => { btn.style.background = '#2a2a44'; });
    btn.addEventListener('mouseleave', () => { btn.style.background = 'none'; });
    btn.addEventListener('click', async () => {
        hideStarMenu();
        if (!modalState.hasEngine) {
            notifyError('Двигатель не установлен — полёт невозможен');
            return;
        }
        if (intra) {
            await startIntraFlight('star', targetId);
        } else {
            await startCompositeFlight('companion', targetId);
        }
    });
    menu.appendChild(btn);

    openStarMenu(menu);
}

// showPlanetMenu — ПКМ по планете на канвасе (спека 99.2.27 §5.5 + 99.2.30
// §6.1, решение создателя 2026-09-21 «как на карте»): пункт «🚀 Лететь» —
// своя система (my_position != null) — внутрисистемный полёт на орбиту планеты
// (POST /api/intrasystem-flight); чужая система (my_position == null, планеты
// видны — не 403) — композитный маршрут (startCompositeFlight, модалка
// закрывается, карта ведёт корабль). Пункт скрыт если: уже на орбите этой
// планеты, цель = активный полёт, летим ОТ неё, активный межзвёздный в своей
// системе (иначе старт даст 400 «Вы в межзвёздном полёте»). Без двигателя —
// пункт есть, клик → тост (как ПКМ звезды).
function showPlanetMenu(x, y, planetIndex) {
    hideStarMenu(); // скрыть предыдущее меню сразу (паттерн showStarMenu)
    const planet = (modalState.planets || [])[planetIndex];
    if (!planet) return;

    const myPos = modalState.myPosition;
    // «Своя система» — явный флаг сервера (flightModeForSystem); при
    // межзвёздном полёте в своей системе это композитный редирект, не intra
    // (баг 2026-09-22: не выводить из my_position != null).
    const intra = flightModeForSystem() === 'intra';
    let onThisOrbit = false;
    let onThisSurface = false;
    if (myPos) {
        // Цель = активный полёт / летим ОТ неё (запрос создателя «глупый тост»:
        // цель == from полёта) / межзвёздный в своей системе — меню не создаём.
        if (myPos.status === 'in_flight' && myPos.to_type === 'planet' && myPos.to_id === planet.id) return;
        if (myPos.status === 'in_flight' && myPos.from_type === 'planet' && myPos.from_id === planet.id) return;
        if (modalState.interstellarFlight) return;
        // Своя орбита/поверхность этой планеты (спека 2026-09-21 §5.2): меню
        // создаём ради пункта «Высадиться»/«Вы на поверхности» — ранний return
        // ровно в валидном состоянии снят.
        onThisOrbit = myPos.status === 'orbit' && myPos.object_type === 'planet' && myPos.object_id === planet.id;
        onThisSurface = myPos.status === 'surface' && myPos.object_type === 'planet' && myPos.object_id === planet.id;
    }

    const menu = document.createElement('div');
    menu.id = 'star-context-menu';
    menu.style.cssText = `
        position: fixed;
        left: ${x}px;
        top: ${y}px;
        background: #1a1a2e;
        border: 1px solid #334155;
        border-radius: 8px;
        box-shadow: 0 8px 24px rgba(0,0,0,0.5);
        padding: 4px;
        min-width: 160px;
        font-size: 0.9rem;
        color: #e0e0e0;
    `;

    const title = document.createElement('div');
    title.style.cssText = `
        padding: 6px 10px;
        font-size: 0.75rem;
        color: #888;
        border-bottom: 1px solid #2a2a44;
        margin-bottom: 4px;
    `;
    title.textContent = planet.name || 'Планета';
    menu.appendChild(title);

    // menuItem — единый стиль строки контекстного меню.
    const menuItem = (html) => {
        const el = document.createElement('div');
        el.style.cssText = `
        padding: 8px 10px;
        cursor: pointer;
        border-radius: 6px;
        display: flex;
        align-items: center;
        gap: 8px;
    `;
        el.innerHTML = html;
        el.addEventListener('mouseenter', () => { el.style.background = '#2a2a44'; });
        el.addEventListener('mouseleave', () => { el.style.background = 'none'; });
        return el;
    };

    // «Лететь» — скрыт, когда уже на орбите/поверхности этой планеты.
    if (!onThisOrbit && !onThisSurface) {
        const btn = menuItem(`🚀 <span>Лететь</span>`);
        btn.addEventListener('click', async () => {
            hideStarMenu();
            // Без двигателя полёт невозможен (спека 91a §6.1): блокируем с
            // подсказкой; сервер валидирует тоже (админ/skycomposer — исключение).
            if (!modalState.hasEngine) {
                notifyError('Двигатель не установлен — полёт невозможен');
                return;
            }
            if (intra) {
                await startIntraFlight('planet', planet.id);
            } else {
                await startCompositeFlight('planet', planet.id);
            }
        });
        menu.appendChild(btn);
    }

    // «Высадиться» (спека 2026-09-21 §5.2 + идея 2026-09-21): только с орбиты
    // этой планеты. Газовый гигант — disabled (клиент biomes не читает; §5.4).
    // Админу — раскрытие выбора биома планеты (аккордеон; роль гейтится
    // ПЕРВОЙ — у player biomes вырезаны сервером, 77a И1).
    if (onThisOrbit || onThisSurface) {
        if (onThisSurface) {
            // «Вы на поверхности» — осмысленный пункт (идея 2026-09-21 §3 п.1а):
            // клик открывает прогулку заново (позиция уже surface — land
            // идемпотентно вернёт тот же биом).
            const surfaceItem = menuItem(`🚶 <span>Вы на поверхности</span>`);
            surfaceItem.addEventListener('click', () => {
                hideStarMenu();
                window.location.href = '/surface.html?planet=' + encodeURIComponent(planet.id);
            });
            menu.appendChild(surfaceItem);
        } else if (planet.is_gas_giant || planet.is_mini_neptune) {
            // Класс без поверхности: газовый гигант и мини-нептун несут только
            // оболочку — биомов/недр нет (спека 2026-09-23 §8.3: мини-нептун
            // ведёт себя как гигант). Высадка disabled; сервер отвечает отказом.
            const surfaceless = planet.is_mini_neptune ? 'мини-нептун' : 'газовый гигант';
            const landItem = menuItem(`🚶 <span style="color:#64748b;">Высадиться</span>
               <span title="Высадка недоступна: у планеты нет поверхности"
                     style="color:#64748b; font-size:0.75rem; margin-left:auto;">${surfaceless}</span>`);
            landItem.style.cursor = 'default';
            landItem.addEventListener('click', () => notifyError(
                planet.is_mini_neptune ? 'Мини-нептун — высадка невозможна' : 'Газовый гигант — высадка невозможна'));
            menu.appendChild(landItem);
        } else {
            appendLandItem(menu, planet);
        }
    }

    openStarMenu(menu);
}

// showBeltMenu — ПКМ по поясу (ТЗ §9.4): пункты «🚀 Лететь» и «⛏ Добывать» в
// стиле showPlanetMenu. Состояния согласованы с таблицей «Объекты» (бывшие
// кнопки строки пояса): «Лететь» — своя система → внутрисистемный полёт,
// чужая → композитный маршрут; скрыт, если уже в этом поясе / цель или
// отправление активного полёта / активный межзвёздный (иначе старт даст 400).
// «Добывать» — активен только в этом поясе, не выработан, нет активного полёта;
// иначе disabled (как у кнопки). Экспорт — для ПКМ по строке пояса (panel.js).
// worldX/worldY — канвасные координаты клика по кольцу (contextmenu): «Лететь»
// летит в место клика (правка создателя 2026-09-23). ПКМ по СТРОКЕ списка их не
// передаёт — точка прибытия = ближайшая к кораблю (фолбэк, старт без точки).
export function showBeltMenu(x, y, beltId, worldX, worldY) {
    hideStarMenu(); // скрыть предыдущее меню сразу (паттерн showStarMenu)
    const belt = (modalState.belts || []).find(b => b.id === beltId);
    if (!belt) return;
    const clickPoint = (isFinite(worldX) && isFinite(worldY)) ? { x: worldX, y: worldY } : null;

    const myPos = modalState.myPosition;
    // «Своя система» — явный флаг сервера (flightModeForSystem), не my_position.
    const intra = flightModeForSystem() === 'intra';

    const inThisBelt = !!myPos && (myPos.status === 'orbit' || myPos.status === 'mining') &&
        myPos.object_type === 'belt' && myPos.object_id === belt.id;
    const flyingTo = !!myPos && myPos.status === 'in_flight' &&
        myPos.to_type === 'belt' && myPos.to_id === belt.id;
    const flyingFrom = !!myPos && myPos.status === 'in_flight' &&
        myPos.from_type === 'belt' && myPos.from_id === belt.id;
    const inFlight = !!modalState.interstellarFlight || (!!myPos && myPos.status === 'in_flight');

    const menu = document.createElement('div');
    menu.id = 'star-context-menu';
    menu.style.cssText = `
        position: fixed;
        left: ${x}px;
        top: ${y}px;
        background: #1a1a2e;
        border: 1px solid #334155;
        border-radius: 8px;
        box-shadow: 0 8px 24px rgba(0,0,0,0.5);
        padding: 4px;
        min-width: 180px;
        font-size: 0.9rem;
        color: #e0e0e0;
    `;

    const title = document.createElement('div');
    title.style.cssText = `
        padding: 6px 10px;
        font-size: 0.75rem;
        color: #888;
        border-bottom: 1px solid #2a2a44;
        margin-bottom: 4px;
    `;
    title.textContent = belt.name || 'Пояс';
    menu.appendChild(title);

    // menuItem — единый стиль строки контекстного меню (как showPlanetMenu).
    const menuItem = (html) => {
        const el = document.createElement('div');
        el.style.cssText = `
        padding: 8px 10px;
        cursor: pointer;
        border-radius: 6px;
        display: flex;
        align-items: center;
        gap: 8px;
    `;
        el.innerHTML = html;
        el.addEventListener('mouseenter', () => { el.style.background = '#2a2a44'; });
        el.addEventListener('mouseleave', () => { el.style.background = 'none'; });
        return el;
    };

    // «Лететь» — скрыт, когда уже в поясе / цель или отправление полёта /
    // активный межзвёздный. Без двигателя — пункт есть, клик → тост.
    if (!inThisBelt && !flyingTo && !flyingFrom && !modalState.interstellarFlight) {
        const btn = menuItem(`🚀 <span>Лететь</span>`);
        btn.addEventListener('click', async () => {
            hideStarMenu();
            if (!modalState.hasEngine) {
                notifyError('Двигатель не установлен — полёт невозможен');
                return;
            }
            if (intra) {
                // clickPoint = место клика по кольцу (null — ПКМ по строке списка:
                // цель станет ближайшей к кораблю точкой, фолбэк внутри старта).
                await startIntraFlight('belt', belt.id, clickPoint);
            } else {
                // Композитный маршрут: точку прибытия считает сервер (§6) —
                // клик ему не передаём.
                await startCompositeFlight('belt', belt.id);
            }
        });
        menu.appendChild(btn);
    }

    // «Добывать» — точка входа в мини-игру (двигатель не требуется). Активна,
    // когда игрок в этом поясе, запас не выработан и нет активного полёта.
    // Финальный ответ по клику — read-only вердикт + алерт поверх модалки
    // (спека 2026-09-23 §4.2): отказ больше не уводит с карты на belt.html.
    const level = belt.remaining_level || '';
    let mineTitle = '';
    if (inFlight) mineTitle = 'Вы в полёте — дождитесь прибытия';
    else if (!inThisBelt) mineTitle = 'Сначала долетите до пояса';
    else if (level === 'выработан') mineTitle = 'Пояс выработан';
    const mineBtn = menuItem(`⛏ <span>Добывать</span>`);
    if (mineTitle) {
        mineBtn.style.cursor = 'not-allowed';
        mineBtn.style.opacity = '0.4';
        mineBtn.title = mineTitle;
    } else {
        if (myPos && myPos.status === 'mining') {
            mineBtn.innerHTML = `⛏ <span>Продолжить добычу</span>`;
        }
        mineBtn.addEventListener('click', async () => {
            hideStarMenu();
            // «Выработан» сюда не доходит — пункт disabled (`mineTitle` выше,
            // UI-спека §2.2 п.3); мёртвая ветка с алертом убрана 2026-09-23.
            // «⛏ Продолжить добычу» (status='mining') — прямая навигация без
            // пречека (§4.2 п.4): заход уже открыт, состояние не меняем.
            if (myPos && myPos.status === 'mining') {
                window.location.href = '/belt.html?belt=' + encodeURIComponent(belt.id);
                return;
            }
            const verdict = await beltMineVerdict(belt);
            if (!verdict.ok) {
                // «Нельзя» → алерт полосы alert (2000) ПОВЕРХ модалки: игрок
                // остаётся на карте, модалка открыта, enter не вызывается.
                openAlert({ title: verdict.title, text: verdict.text });
                return;
            }
            window.location.href = '/belt.html?belt=' + encodeURIComponent(belt.id);
        });
    }
    menu.appendChild(mineBtn);

    openStarMenu(menu);
}

// beltMineVerdict — read-only вердикт «можно ли зайти в пояс» (спека §4.2),
// БЕЗ вызова POST /api/belt/mine/enter (он не read-only: пишет
// users.current_position и лениво инициализирует запас пояса). Данные уже
// есть на карте: modalState.myPosition (в этом ли поясе), modalState.belts[]
// (remaining_level/belt_class под гейтом знания), GET /api/cargo (свободный
// трюм). Приоритет причин: не в поясе → выработан → нет данных → полон трюм.
async function beltMineVerdict(belt) {
    const myPos = modalState.myPosition;
    const inThisBelt = !!myPos && (myPos.status === 'orbit' || myPos.status === 'mining') &&
        myPos.object_type === 'belt' && myPos.object_id === belt.id;
    if (!inThisBelt) {
        return { ok: false, title: 'Сначала долетите до пояса', text: 'Подлетите к поясу и попробуйте снова.' };
    }
    const level = belt.remaining_level || '';
    if (level === 'выработан') {
        return { ok: false, title: 'Пояс выработан', text: 'Добывать здесь больше нечего.' };
    }
    // Пустой `remaining_level` — НЕ запрет захода: запас инициализируется ЛЕНИВО на
    // первом enter (осн. §6.3/§8.4). После перегенерации галактики у всех поясов
    // «нет данных» — заход обязан работать (баг 2026-09-23: клиент блокировал
    // кнопку, хотя сервер инициализирует запас сам). Случай «состав без железа»
    // знает только сервер — его покажет экран ошибки `belt.html` (третий путь).
    if (await cargoIsFull()) {
        return {
            ok: false,
            title: 'Трюм полон',
            text: 'Нет свободного места в трюме — добывать некуда. Выгрузите груз на дашборде (раздел «Корабль», блок «Трюм») и возвращайтесь.',
        };
    }
    return { ok: true };
}

// cargoIsFull — трюм без свободного места по GET /api/cargo (лимит массы
// used/total считает сервер, спека трюма §9.1). Сеть/ответ недоступны (null) —
// вердикт «трюм» пропускаем: из-за сети игрока не блокируем, остальные
// проверки работают (§4.2).
async function cargoIsFull() {
    const token = modalState.authToken || localStorage.getItem('token');
    if (!token) return null;
    try {
        const res = await fetch('/api/cargo', { headers: { 'Authorization': 'Bearer ' + token } });
        if (!res.ok) return null;
        const data = await res.json();
        const mass = (data && data.limits && data.limits.mass) || {};
        const used = Number(mass.used);
        const total = Number(mass.total);
        if (!isFinite(used) || !isFinite(total)) return null;
        return total - used <= 0;
    } catch (e) {
        return null;
    }
}

// isAdminRole — роль из /me (как tabs.js isAdmin): админский инструмент.
function isAdminRole() {
    return modalState.role === 'admin' || modalState.role === 'skycomposer';
}

// clampMenuToScreen — прижимает контекстное меню к экрану (приём тултипа
// звезды): после раскрытия аккордеона меню может вылезти за край.
function clampMenuToScreen(menu) {
    const r = menu.getBoundingClientRect();
    let left = r.left, top = r.top;
    if (r.right > window.innerWidth - 4) left -= r.right - (window.innerWidth - 4);
    if (r.bottom > window.innerHeight - 4) top -= r.bottom - (window.innerHeight - 4);
    if (left < 4) left = 4;
    if (top < 4) top = 4;
    menu.style.left = left + 'px';
    menu.style.top = top + 'px';
}

// landBiomeRow — строка списка биомов: иконка + имя слева, доля справа.
function landBiomeRow(label, iconHtml, title, rightText, onClick) {
    const row = document.createElement('div');
    row.style.cssText = 'display:flex; align-items:center; gap:6px; padding:5px 6px; border-radius:6px; cursor:pointer;';
    if (title) row.title = title;
    row.innerHTML = `${iconHtml || ''}<span>${label}</span>` +
        (rightText ? `<span style="color:#888; margin-left:auto;">${rightText}</span>` : '');
    row.addEventListener('mouseenter', () => { row.style.background = '#2a2a44'; });
    row.addEventListener('mouseleave', () => { row.style.background = 'none'; });
    row.addEventListener('click', onClick);
    return row;
}

// appendLandItem — пункт «🚶 Высадиться» (идея 2026-09-21 §3): у player/без
// биомов — обычный пункт («биом ?» / «биомов нет»), у админа — тумблер
// «⚙ выбор биома ▸» с раскрытым списком биомов планеты по убыванию доли.
// Клик по биому → /surface.html?planet=<id>&biome=<form>; «Случайно» → без biome.
function appendLandItem(menu, planet) {
    const admin = isAdminRole();
    const biomes = admin
        ? (planet.biomes || []).filter((b) => b && b.share > 0).sort((a, b) => b.share - a.share)
        : [];
    const hasBiomes = biomes.length > 0;

    const landItem = document.createElement('div');
    landItem.style.cssText = `
        padding: 8px 10px;
        cursor: pointer;
        border-radius: 6px;
        display: flex;
        align-items: center;
        gap: 8px;
    `;
    landItem.addEventListener('mouseenter', () => { landItem.style.background = '#2a2a44'; });
    landItem.addEventListener('mouseleave', () => { landItem.style.background = 'none'; });
    landItem.innerHTML = `🚶 <span>Высадиться</span>` + (hasBiomes
        ? `<span id="land-biome-toggle" style="color:#64748b; font-size:0.75rem; margin-left:auto;">⚙ выбор биома ▸</span>`
        : `<span title="Высадка: биом случаен — чем больше доля, тем вероятнее"
                 style="color:#64748b; font-size:0.75rem; margin-left:auto;">${admin ? 'биомов нет' : 'биом ?'}</span>`);
    menu.appendChild(landItem);

    const toSurface = (biome) => {
        hideStarMenu();
        let url = '/surface.html?planet=' + encodeURIComponent(planet.id);
        if (biome) url += '&biome=' + encodeURIComponent(biome);
        window.location.href = url;
    };

    if (!hasBiomes) {
        landItem.addEventListener('click', () => toSurface(''));
        return;
    }

    // Раскрытый блок: заголовок с чипом «⚙ админ», «Случайно» + биомы планеты.
    const block = document.createElement('div');
    block.style.cssText = 'display:none; padding:6px 10px 8px 10px; border-top:1px solid #2a2a44; margin-top:2px;';

    const header = document.createElement('div');
    header.style.cssText = 'display:flex; align-items:center; gap:6px; margin-bottom:6px;';
    header.innerHTML = `<span style="color:#888; font-size:0.75rem;">Биом планеты</span>
        <span style="background:rgba(250,204,21,0.12); border:1px solid rgba(250,204,21,0.35);
                     color:#facc15; border-radius:10px; padding:1px 7px; font-size:0.7rem;">⚙ админ</span>`;
    block.appendChild(header);

    const list = document.createElement('div');
    list.style.cssText = 'max-height:240px; overflow-y:auto;';
    list.appendChild(landBiomeRow('Случайно — как у игрока', '', 'Биом выпадет по долям — как у обычного игрока', '', () => toSurface('')));
    biomes.forEach((b) => {
        list.appendChild(landBiomeRow(prettyName(b.form), biomeIconHtml(b.form), '',
            b.share.toFixed(1) + ' %', () => toSurface(b.form)));
    });
    block.appendChild(list);
    menu.appendChild(block);

    let expanded = false;
    landItem.addEventListener('click', () => {
        expanded = !expanded;
        block.style.display = expanded ? 'block' : 'none';
        const toggle = landItem.lastElementChild;
        if (toggle) toggle.textContent = expanded ? '⚙ выбор биома ▾' : '⚙ выбор биома ▸';
        if (expanded) clampMenuToScreen(menu);
    });
}

// startIntraFlight — старт внутрисистемного полёта (спека 99.2.27 §4.1):
// POST /api/intrasystem-flight. Модалка НЕ закрывается (суть пожелания):
// полоса полёта появляется из ответа (my_position = in_flight).
// clickPoint — необязательная точка клика по кольцу пояса (канвасные world-
// координаты, ПКМ на канвасе): цель полёта к поясу = место клика (правка
// создателя 2026-09-23). Другие цели и «Лететь» из списка её не передают.
// Экспорт — для кнопки «Лететь» в карточках (panel.js, динамический импорт).
export async function startIntraFlight(objectType, objectId, clickPoint) {
    const token = modalState.authToken || localStorage.getItem('token');
    if (!token) return;
    // Позиция ДО старта — для цели полёта к поясу (правка создателя 2026-09-23:
    // «лететь к ближайшей» точке кольца, а не через полкарты к канонической).
    const before = modalState.myPosition;
    try {
        const res = await fetch('/api/intrasystem-flight', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token
            },
            body: JSON.stringify({ object_type: objectType, object_id: objectId })
        });
        const text = await res.text();
        if (!res.ok) {
            let msg = text;
            try { msg = JSON.parse(text).error || text; } catch (e) { /* plain text */ }
            notifyError('Ошибка: ' + msg);
            return;
        }
        const data = JSON.parse(text);
        // Пояс: цель полёта — место клика по кольцу, а без точки клика (ПКМ по
        // строке списка) — ближайшая к кораблю точка осевой линии на момент
        // старта (запоминаем ДО смены позиции). Точку берёт beltPoint — один
        // источник для полёта, маркера, камеры и чужих игроков (§9.3 п.2).
        if (objectType === 'belt') storeBeltArrivalAngle(objectId, before, clickPoint);
        // Позиция = полёт (решение создателя): модалка сразу показывает полосу.
        // Новый полёт — центрирование по прибытии снимается (слежение берёт верх).
        modalState.arrivalObject = null;
        modalState.myPosition = {
            status: 'in_flight',
            from_type: data.from_type,
            from_id: data.from_id,
            to_type: data.to_type,
            to_id: data.to_id,
            start_time: data.start_time,
            arrive_at: data.arrive_at,
        };
    } catch (e) {
        notifyError('Ошибка: ' + e.message);
    }
}

// starMenuHandle — слой контекстного меню модалки (#star-context-menu) в
// реестре слоёв; меню в DOM без handle не остаётся (hideStarMenu).
let starMenuHandle = null;

// openStarMenu — смонтировать меню и зарегистрировать его слоем полосы menu
// (спека 2026-09-23 §5): меню над модалкой, Esc/клик-вне закрывают именно его.
function openStarMenu(menu) {
    if (starMenuHandle) { const h = starMenuHandle; starMenuHandle = null; h.close(); }
    document.body.appendChild(menu);
    starMenuHandle = openLayer(menu, {
        level: 'menu',
        onClose: () => {
            starMenuHandle = null;
            if (menu.parentNode) menu.remove();
        },
    });
}

// hideStarMenu — закрытие через слой реестра (вызывается в начале каждого
// show…Menu и по выбору пункта); фолбэк — убрать узел без слоя.
function hideStarMenu() {
    if (starMenuHandle) { starMenuHandle.close(); return; }
    const menu = document.getElementById('star-context-menu');
    if (menu) menu.remove();
}

async function startTravelToStar() {
    const worldId = modalState.worldId;
    // Модалку может открыть админка, где игрового токена нет: сначала
    // берём токен, под которым открыта модалка, затем игровой. Импорт
    // динамический: map/ тянется только по клику «Перелететь» и не роняет
    // админку при загрузке (map/config.js требует canvas карты).
    const token = modalState.authToken || localStorage.getItem('token');
    const { startFlight, returnToMapInFlight } = await import('../map/flight.js');
    const ok = await startFlight(worldId, token);
    if (ok) {
        // Спека 99.2.30 §6.8: успешный старт обычного «Перелететь» стирает
        // сессионный маркер композитного маршрута (игрок явно ушёл от
        // маршрута, M1).
        sessionStorage.removeItem('compositeRoute');
        // Модалка закрывается — панель перелёта видна на карте в шапке.
        closeModal();
        // Спека 99.2.30 §6.7: «лететь из модалки = камера карты ведёт корабль»
        // (гейт создателя, идея §8 п.7в) — слежение как кнопка «Найти меня».
        returnToMapInFlight();
    }
}

// storeBeltArrivalAngle — зафиксировать азимут прибытия к поясу. Источник точки
// (правка создателя 2026-09-23 «летим на место клика»): clickPoint — проекция
// клика по кольцу на осевую линию (ровно под курсором); без него (ПКМ по строке
// списка «Объекты») — направление от центра кольца на корабль (позиция до старта
// полёта), т.е. ближайшая точка осевой линии (§9.3). Позиции нет (окно
// прибытия/межзвёздный полёт) или она уже в этом поясе — не трогаем: beltPoint
// оставит канонический beltAngle (маркер не исчезает, §9.3 п.2).
function storeBeltArrivalAngle(beltId, beforePos, clickPoint) {
    const belt = (modalState.belts || []).find(b => b.id === beltId);
    if (!belt) return;
    const layout = computeLayout(
        modalState.planets, modalState.starRadius,
        modalState.canvasWidth, modalState.canvasHeight
    );
    // Место клика — корабль-ориентир не нужен: точка прибытия ровно под курсором
    // (beltNearestAngle — направление от центра кольца на точку).
    if (clickPoint && isFinite(clickPoint.x) && isFinite(clickPoint.y)) {
        const angle = beltNearestAngle(layout, belt, clickPoint.x, clickPoint.y);
        if (angle !== null) setBeltArrivalAngle(belt, angle);
        return;
    }
    if (!beforePos) return;
    if (beforePos.status === 'in_flight') return;
    if (beforePos.object_type === 'belt' && beforePos.object_id === beltId) return;
    const ship = orbitalPoint(
        layout, modalState.planets || [],
        beforePos.object_type, beforePos.object_id, performance.now()
    );
    const angle = beltNearestAngle(layout, belt, ship.x, ship.y);
    if (angle !== null) setBeltArrivalAngle(belt, angle);
}

// startCompositeFlight — старт композитного маршрута (спека 99.2.30 §6.1/§6.7):
// POST /travel с телом {world_id, destination: {object_type, object_id}} —
// расширение startFlight. Успех → модалка закрывается, карта показывает
// межзвёздный полёт со слежением; сессионный маркер compositeRoute пишется
// (автооткрытие модалки по прибытии, §6.8). Ошибка → модалка остаётся
// открытой, тост с текстом сервера (§6.7 п.5). Экспорт — для композитной
// кнопки в карточках (panel.js/tabs.js, динамический импорт).
export async function startCompositeFlight(objectType, objectId) {
    const worldId = modalState.worldId;
    const token = modalState.authToken || localStorage.getItem('token');
    const { startFlight, returnToMapInFlight } = await import('../map/flight.js');
    const ok = await startFlight(worldId, token, { object_type: objectType, object_id: objectId });
    if (ok) {
        // Сессионный маркер композитного маршрута (§6.8): пишется при успешном
        // композитном старте (вместе со слежением); стирается при автооткрытии,
        // при успешном «Перелететь» (M1), если current_world_id не совпал.
        sessionStorage.setItem('compositeRoute', worldId);
        // Модалка закрывается — карта показывает межзвёздный полёт (полоса,
        // камера со слежением — существующее, 61a/42a).
        closeModal();
        // Слежение после старта из модалки (§6.7): followShip + centerOnAgent
        // + подсветка centerBtn (общий хелпер «вернулся на карту в полёте»).
        returnToMapInFlight();
    }
    return ok;
}
