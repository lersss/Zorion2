// web/static/js/modal/minimap.js
import { modalState } from './state.js';
import { computeLayout, getOrbitRadius, getPlanetAngle, planetOrbitCenter } from './layout.js';

// clampToMini — прижимает точку к краю мини-карты (35b §6.5, требование
// создателя №3б): wide-компаньон вне честного масштаба рисуется у края
// круга мини-карты, не перекрывая планеты (они внутри ~54 px от центра).
function clampToMini(x, y, r, centerX, centerY, maxR) {
    const dx = x - centerX;
    const dy = y - centerY;
    const d = Math.hypot(dx, dy);
    if (d + r <= maxR) return { x, y };
    const k = (maxR - r) / d;
    return { x: centerX + dx * k, y: centerY + dy * k };
}

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

    // Звёзды (35b §6.5): главная и компаньоны из того же computeLayout, что и
    // канвас; wide-компаньон вне масштаба — у края круга мини-карты, цвет по
    // спектру. Подпись расстояния на масштабе 120 px не выводится (нечитаема)
    // — расстояние читается в модалке (§6.2) и карточке звезды (§6.3).
    const mainStar = layout.stars.find(s => s.kind === 'main') || { x: cx, y: cy, radius: finalStarRadius };
    const miniEdgeR = miniSize / 2 - 3;

    ctx.save();
    layout.stars.forEach(s => {
        let sx = centerX + (s.x - cx) * miniScale;
        let sy = centerY + (s.y - cy) * miniScale;
        const sr = Math.max(1.5, s.radius * miniScale);
        if (s.kind !== 'main') {
            const clamped = clampToMini(sx, sy, sr, centerX, centerY, miniEdgeR);
            sx = clamped.x;
            sy = clamped.y;
        }
        ctx.beginPath();
        ctx.arc(sx, sy, sr, 0, 2 * Math.PI);
        ctx.fillStyle = s.kind === 'main' ? '#fff4a3' : s.color;
        ctx.fill();
    });
    ctx.restore();

    // Планеты (с анимацией): P — вокруг барицентра, S — вокруг главной.
    if (planets && planets.length > 0) {
        planets.forEach((p, idx) => {
            const orbitRadius = getOrbitRadius(layout, p, idx);
            const angle = getPlanetAngle(p, orbitRadius, idx, timeMs);
            const center = planetOrbitCenter(layout, p);
            const cxp = centerX + (center.x - cx) * miniScale;
            const cyp = centerY + (center.y - cy) * miniScale;
            const px = cxp + orbitRadius * Math.cos(angle) * miniScale;
            const py = cyp + orbitRadius * Math.sin(angle) * miniScale;
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