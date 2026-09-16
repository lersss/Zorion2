// web/static/js/modal/events.js
import { modalState } from './state.js';
import { drawSystem } from './modal_render.js';
import { computeLayout, getOrbitRadius, getPlanetAngle, getPlanetSize, planetOrbitCenter } from './layout.js';
import { miniObjects } from './minimap.js';
import { closeModal } from './index.js';

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
        const radius = getPlanetSize(p, layout.sizeMultiplier);
        const center = planetOrbitCenter(layout, p);
        const px = center.x + orbitRadius * Math.cos(angle);
        const py = center.y + orbitRadius * Math.sin(angle);
        return { px, py, radius, finalStarRadius: layout.finalStarRadius };
    }

    function hitTest(worldX, worldY) {
        const layout = computeLayout(planets, starRadius, width, height);
        // Звезда — по позиции главной (в тесной паре она смещена от барицентра).
        const distToStar = Math.hypot(worldX - layout.mainX, worldY - layout.mainY);
        if (distToStar < layout.finalStarRadius + 8) return 'star';

        for (let idx = 0; idx < planets.length; idx++) {
            const { px, py, radius } = getPlanetWorldPos(planets[idx], idx);
            if (Math.hypot(worldX - px, worldY - py) < radius + 6) {
                return { type: 'planet', index: idx };
            }
        }
        return null;
    }

    function toWorld(e) {
        const rect = canvas.getBoundingClientRect();
        const mouseX = (e.clientX - rect.left) * (canvas.width / rect.width) / dpr;
        const mouseY = (e.clientY - rect.top) * (canvas.height / rect.height) / dpr;
        const worldX = (mouseX - modalState.offsetX) / modalState.zoom;
        const worldY = (mouseY - modalState.offsetY) / modalState.zoom;
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
        } else if (hit && hit.type === 'planet') {
            if (!modalState.hoveredObject || modalState.hoveredObject.type !== 'planet' || modalState.hoveredObject.index !== hit.index) {
                modalState.hoveredObject = hit;
                canvas.style.cursor = 'pointer';
            }
        } else {
            if (modalState.hoveredObject !== null) {
                modalState.hoveredObject = null;
                canvas.style.cursor = modalState.isDragging ? 'grabbing' : 'default';
            }
        }
    });

    canvas.addEventListener('mouseleave', () => {
        if (modalState.hoveredObject !== null) {
            modalState.hoveredObject = null;
            canvas.style.cursor = modalState.isDragging ? 'grabbing' : 'default';
        }
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

        const worldX = (mouseX - modalState.offsetX) / modalState.zoom;
        const worldY = (mouseY - modalState.offsetY) / modalState.zoom;
        modalState.zoom = newZoom;
        modalState.offsetX = mouseX - worldX * modalState.zoom;
        modalState.offsetY = mouseY - worldY * modalState.zoom;
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
    });

    window.addEventListener('mousemove', (e) => {
        if (modalState.isDragging) {
            const dx = (e.clientX - modalState.dragStartX) / dpr;
            const dy = (e.clientY - modalState.dragStartY) / dpr;
            modalState.offsetX = modalState.dragStartOffsetX + dx;
            modalState.offsetY = modalState.dragStartOffsetY + dy;
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
                modalState.offsetX = modalState.canvasWidth / 2 - best.world.x * modalState.zoom;
                modalState.offsetY = modalState.canvasHeight / 2 - best.world.y * modalState.zoom;
                drawSystem(canvas, spectralClass, planets, starRadius, starColor, modalState.canvasWidth, modalState.canvasHeight);
            }
            return;
        }

        const { worldX, worldY } = toWorld(e);
        const hit = hitTest(worldX, worldY);

        if (hit === 'star') {
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
        } else {
            hideStarMenu();
        }
    });

    // Левая кнопка вне меню — скрывает. ПКМ по контекстному меню не должен закрывать его.
    document.addEventListener('mousedown', (e) => {
        if (e.button !== 2 && !e.target.closest('#star-context-menu')) hideStarMenu();
    });
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
        z-index: 1100;
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

    // «Перелететь» живёт только в игровой карте: из админки (authToken задан)
    // полёт невозможен — #mapCanvas там нет, не показываем нерабочий пункт.
    if (!modalState.authToken) {
        const btn = document.createElement('div');
        btn.style.cssText = `
            padding: 8px 10px;
            cursor: pointer;
            border-radius: 6px;
            display: flex;
            align-items: center;
            gap: 8px;
        `;
        btn.innerHTML = `🚀 <span>Перелететь</span>`;
        btn.addEventListener('mouseenter', () => { btn.style.background = '#2a2a44'; });
        btn.addEventListener('mouseleave', () => { btn.style.background = 'none'; });
        btn.addEventListener('click', async () => {
            hideStarMenu();
            await startTravelToStar();
        });
        menu.appendChild(btn);
    }

    document.body.appendChild(menu);
}

function hideStarMenu() {
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
    const { startFlight } = await import('../map/flight.js');
    const ok = await startFlight(worldId, token);
    if (ok) {
        // Модалка закрывается — панель перелёта видна на карте в шапке.
        closeModal();
    }
}
