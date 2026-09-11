// web/static/js/map/animation.js
import { state, elements } from './config.js';
import { draw } from './map_render.js';
import { loadClusters, loadUserData } from './data.js';
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
            // Перезагружаем данные (могли прилететь в другую область)
            loadUserData(true).then(() => {
                if (state.currentWorldId) {
                    centerOnAgent();
                }
                return loadClusters();
            }).catch(err => console.error('Arrival reload error:', err));
        } else {
            updateFlightPanel();
            if (state.followShip) {
                centerOnAgent();
                const centerBtn = document.getElementById('centerBtn');
                if (centerBtn) centerBtn.classList.add('active');
            }
            draw();
        }
    }
    requestAnimationFrame(animationLoop);
}