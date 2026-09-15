// web/static/js/modal/modal_render.js
import { modalState } from './state.js';
import { drawMiniMap } from './minimap.js';
import { getPlanetTexture } from './textures.js';
import { computeLayout, getOrbitRadius, getPlanetPose, getPlanetSize, planetOrbitCenter } from './layout.js';

// formatAU — читаемое разделение: ≥ 100 а.е. — целое («342 а.е.»), иначе
// два знака («0.45 а.е.»).
function formatAU(au) {
    if (typeof au !== 'number' || !isFinite(au) || au <= 0) return '';
    return au >= 100 ? Math.round(au) + ' а.е.' : au.toFixed(2) + ' а.е.';
}

export async function drawSystem(canvas, spectralClass, planets, starRadius, starColor, width, height) {
    const dpr = window.devicePixelRatio || 1;
    canvas.width = width * dpr;
    canvas.height = height * dpr;
    canvas.style.width = width + 'px';
    canvas.style.height = height + 'px';

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    const layout = computeLayout(planets, starRadius, width, height);
    const { cx, cy, mainX, mainY, finalStarRadius, step, maxOrbit, sizeMultiplier, stars } = layout;
    const timeMs = performance.now() - (modalState.animStart || performance.now());

    ctx.save();
    ctx.translate(modalState.offsetX, modalState.offsetY);
    ctx.scale(modalState.zoom, modalState.zoom);

    // ---- СЛОЙ 1: ОРБИТЫ ----
    // P-планеты — вокруг барицентра (cx, cy), S — вокруг главной (mainX, mainY)
    // (35b §6.1).
    if (planets && planets.length > 0) {
        planets.forEach((p, idx) => {
            const orbitRadius = getOrbitRadius(layout, p, idx);
            const center = planetOrbitCenter(layout, p);
            ctx.save();
            ctx.strokeStyle = '#444';
            ctx.lineWidth = 1;
            ctx.setLineDash([3, 6]);
            ctx.beginPath();
            ctx.arc(center.x, center.y, orbitRadius, 0, 2 * Math.PI);
            ctx.stroke();
            ctx.restore();
        });
    }

    // ---- СЛОЙ 2: ЗВЁЗДЫ (главная + компаньоны по честной геометрии, 35b §6.2) ----
    stars.forEach(s => {
        ctx.save();
        if (s.kind === 'main') {
            ctx.shadowColor = s.color;
            ctx.shadowBlur = 40;
        } else {
            ctx.globalAlpha = 0.85;
            ctx.shadowColor = s.color;
            ctx.shadowBlur = 18;
        }
        ctx.beginPath();
        ctx.arc(s.x, s.y, s.radius, 0, 2 * Math.PI);
        ctx.fillStyle = s.color;
        ctx.fill();
        ctx.restore();
    });

    // Подпись реального расстояния wide-компаньона вне честного масштаба
    // (35b §6.2, решение №3б): «компаньон: 342 а.е.»; у кратных — и внешние.
    const labelled = stars.filter(s => s.kind !== 'main' && s.atEdge && s.sepAU > 0);
    if (labelled.length > 0) {
        ctx.save();
        ctx.font = '11px system-ui';
        ctx.textAlign = 'center';
        labelled.forEach(s => {
            const label = (s.kind === 'companion' ? 'компаньон: ' : 'внешний: ') + formatAU(s.sepAU);
            // Подпись — с внутренней стороны от края кадра: у нижнего компаньона
            // сверху, у верхнего — снизу (иначе уходит за край канваса).
            const belowCenter = s.y > cy;
            const ly = belowCenter ? s.y - s.radius - 8 : s.y + s.radius + 14;
            ctx.fillStyle = 'rgba(0,0,0,0.6)';
            ctx.fillText(label, s.x, ly + 1);
            ctx.fillStyle = '#aab';
            ctx.fillText(label, s.x, ly);
        });
        ctx.restore();
    }

    // ---- СЛОЙ 3: ПЛАНЕТЫ (АСИНХРОННАЯ ЗАГРУЗКА ТЕКСТУР) ----
    if (planets && planets.length > 0) {
        const loadPromises = planets.map(async (p, idx) => {
            let texture = null;
            try {
                texture = await getPlanetTexture(p, spectralClass, sizeMultiplier);
            } catch (e) {
                console.warn('Failed to load texture for planet', p.id, e);
            }

            const pose = getPlanetPose(layout, p, idx, timeMs);
            const drawRadius = (10 + (p.size || 10) * 0.6) * sizeMultiplier;
            return { x: pose.x, y: pose.y, radius: drawRadius, texture, idx };
        });

        const loaded = await Promise.all(loadPromises);

        loaded.forEach(({ x, y, radius, texture, idx }) => {
            if (texture && texture instanceof HTMLImageElement && texture.complete && texture.naturalWidth > 0) {
                ctx.save();
                ctx.shadowColor = 'rgba(255,255,255,0.1)';
                ctx.shadowBlur = 8;
                ctx.drawImage(texture, x - radius, y - radius, radius * 2, radius * 2);
                ctx.restore();
            } else {
                const p = planets[idx];
                let color = '#aaa';
                const type = (p.type || '').toLowerCase();
                if (type.includes('газовый') || type === 'gas_giant') color = '#e8a87c';
                else if (type.includes('землеподобная') || type === 'terran') color = '#6fcf97';
                else if (type.includes('пустынная') || type === 'desert') color = '#d4a373';
                else if (type.includes('ледяная') || type === 'ice') color = '#a8d8ea';
                else if (type.includes('вулканическая') || type === 'volcanic') color = '#e74c3c';
                else if (type.includes('океаническая') || type === 'ocean') color = '#3498db';
                ctx.save();
                ctx.shadowColor = color;
                ctx.shadowBlur = 15;
                ctx.beginPath();
                ctx.arc(x, y, radius, 0, 2 * Math.PI);
                ctx.fillStyle = color;
                ctx.fill();
                ctx.restore();
            }
        });

        // ---- ПОДСВЕТКА ПРИ ХОВЕРЕ ----
        if (modalState.hoveredObject === 'star') {
            ctx.save();
            ctx.shadowColor = 'rgba(255,255,255,0.3)';
            ctx.shadowBlur = 25;
            ctx.beginPath();
            ctx.arc(mainX, mainY, finalStarRadius + 4, 0, 2 * Math.PI);
            ctx.fillStyle = 'rgba(255,255,255,0.15)';
            ctx.fill();
            ctx.strokeStyle = 'rgba(255,255,255,0.7)';
            ctx.lineWidth = 2;
            ctx.stroke();
            ctx.restore();
        } else if (modalState.hoveredObject && modalState.hoveredObject.type === 'planet') {
            const idx = modalState.hoveredObject.index;
            const p = loaded[idx];
            if (p) {
                ctx.save();
                ctx.shadowColor = 'rgba(255,255,255,0.3)';
                ctx.shadowBlur = 20;
                ctx.beginPath();
                ctx.arc(p.x, p.y, p.radius + 3, 0, 2 * Math.PI);
                ctx.fillStyle = 'rgba(255,255,255,0.15)';
                ctx.fill();
                ctx.strokeStyle = 'rgba(255,255,255,0.6)';
                ctx.lineWidth = 2;
                ctx.stroke();
                ctx.restore();
            }
        }

        // ---- ПОДСВЕТКА ВЫБРАННОЙ ПЛАНЕТЫ ----
        if (modalState.selectedPlanetIndex !== null) {
            const p = loaded[modalState.selectedPlanetIndex];
            if (p) {
                ctx.save();
                ctx.shadowColor = 'rgba(255,215,0,0.5)';
                ctx.shadowBlur = 30;
                ctx.beginPath();
                ctx.arc(p.x, p.y, p.radius + 5, 0, 2 * Math.PI);
                ctx.strokeStyle = 'rgba(255,215,0,0.8)';
                ctx.lineWidth = 3;
                ctx.stroke();
                ctx.restore();
            }
        }
    }

    ctx.restore(); // сброс трансформации

    // ---- МИНИ-КАРТА (поверх всего) ----
    drawMiniMap(ctx, cx, cy, finalStarRadius, step, maxOrbit, planets, width, height);
}