// web/static/js/map/animation.js
import { state, elements } from './config.js';
import { draw, updateFpsCounter } from './map_render.js';
import { loadClusters, loadUserData, maybeReloadClusters, waitForLoadIdle } from './data.js';
import { centerOnAgent } from './navigation.js';
import { updateFlightPanel, hideFlightPanel } from './flight.js';

export function animationLoop() {
    if (state.isFlying) {
        const elapsed = (Date.now() - state.flyStartTime) / 1000;
        if (elapsed >= state.flyDuration) {
            state.isFlying = false;
            state.followShip = false;
            const centerBtn = document.getElementById('centerBtn');
            if (centerBtn) centerBtn.classList.remove('active');
            hideFlightPanel();
            elements.statusBar.textContent = '✅ Прибытие!';
            // Перезагружаем данные (могли прилететь в другую область).
            // Сначала центр, потом загрузка: центр ставит камеру на точку
            // прибытия, loadClusters грузит звёзды уже вокруг неё (иначе —
            // пустой экран до следующего перезапроса).
            loadUserData(true).then(async () => {
                if (state.currentWorldId) {
                    centerOnAgent();
                }
                // Фикс 33b: ждём, пока полётный перезапрос кластеров доедет, —
                // иначе loadClusters видит loadingData = true, ставит
                // pendingReload и теряет загрузку зоны прибытия (пустой кадр
                // до дебаунса +180 мс). Порядок «центр → загрузка» (фикс 37a)
                // сохраняется.
                await waitForLoadIdle();
                return loadClusters();
            }).catch(err => console.error('Arrival reload error:', err));
        } else {
            updateFlightPanel();
            if (state.followShip) {
                centerOnAgent();
                const centerBtn = document.getElementById('centerBtn');
                if (centerBtn) centerBtn.classList.add('active');
            } else {
                // centerOnAgent при слежении сам рисует кадр — второй draw()
                // ниже дал бы двойную перерисовку (FPS падает вдвое).
                draw();
            }
            // B17: вьюпорт движется, а мышиных событий нет — подгружаем
            // кластеры под новый район напрямую (с ограничением частоты).
            maybeReloadClusters();
        }
    }
    // EMA-обновление FPS-счётчика каждый кадр (дешёвое, без рисования).
    // Рисование плашки — в конце draw() (map_render.js): там она не затирается
    // ни одним источником перерисовки (полёт, мышь, rAF-цикл NPC на близком зуме).
    updateFpsCounter();
    requestAnimationFrame(animationLoop);
}