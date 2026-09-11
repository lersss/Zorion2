// web/static/js/modal/minimap.js
import { modalState } from './state.js';
import { computeLayout, getOrbitRadius, getPlanetAngle } from './layout.js';

export function drawMiniMap(ctx, cx, cy, finalStarRadius, step, maxOrbit, planets, width, height) {
    const miniSize = 120;
    const miniX = width - miniSize - 20;
    const miniY = height - miniSize - 20;

    // Фон
    ctx.save();
    ctx.fillStyle = 'rgba(0,0,0,0.6)';
    ctx.strokeStyle = '#444';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.roundRect(miniX, miniY, miniSize, miniSize, 8);
    ctx.fill();
    ctx.stroke();
    ctx.restore();

    const layout = computeLayout(planets, modalState.starRadius, width, height);
    const timeMs = performance.now() - (modalState.animStart || performance.now());

    const systemRadius = Math.max(
        finalStarRadius * 1.8,
        finalStarRadius * 1.8 + (maxOrbit + 1) * step * 1.1
    );

    const padding = 0.9;
    const miniScale = (miniSize * padding) / (systemRadius * 2);
    const centerX = miniX + miniSize / 2;
    const centerY = miniY + miniSize / 2;

    // Рисуем звезду
    ctx.save();
    ctx.beginPath();
    ctx.arc(centerX, centerY, Math.max(2, finalStarRadius * miniScale), 0, 2 * Math.PI);
    ctx.fillStyle = '#fff4a3';
    ctx.fill();
    ctx.restore();

    // Рисуем планеты (с анимацией)
    if (planets && planets.length > 0) {
        planets.forEach((p, idx) => {
            const orbitRadius = getOrbitRadius(layout, p, idx);
            const angle = getPlanetAngle(p, orbitRadius, idx, timeMs);
            const px = centerX + orbitRadius * Math.cos(angle) * miniScale;
            const py = centerY + orbitRadius * Math.sin(angle) * miniScale;
            ctx.save();
            ctx.beginPath();
            ctx.arc(px, py, Math.max(1.5, 3 * miniScale), 0, 2 * Math.PI);
            ctx.fillStyle = '#6fcf97';
            ctx.fill();
            ctx.restore();
        });
    }

    // РАМКА ВИДИМОЙ ОБЛАСТИ
    const worldCenterX = (width / 2 - modalState.offsetX) / modalState.zoom;
    const worldCenterY = (height / 2 - modalState.offsetY) / modalState.zoom;

    const viewWidth = (width / modalState.zoom) * miniScale;
    const viewHeight = (height / modalState.zoom) * miniScale;
    const viewX = centerX + (worldCenterX - cx) * miniScale - viewWidth / 2;
    const viewY = centerY + (worldCenterY - cy) * miniScale - viewHeight / 2;

    ctx.save();
    ctx.beginPath();
    ctx.roundRect(miniX, miniY, miniSize, miniSize, 8);
    ctx.clip();
    ctx.strokeStyle = 'rgba(255,255,255,0.5)';
    ctx.lineWidth = 1;
    ctx.setLineDash([2, 3]);
    ctx.strokeRect(viewX, viewY, viewWidth, viewHeight);
    ctx.restore();
}