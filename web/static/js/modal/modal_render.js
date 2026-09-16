// web/static/js/modal/modal_render.js
import { modalState } from './state.js';
import { drawMiniMap } from './minimap.js';
import { getPlanetTexture } from './textures.js';
import { computeLayout, getOrbitRadius, getPlanetPose, getPlanetSize, planetOrbitCenter } from './layout.js';

// Минимальный экранный радиус звезды (51a): на отдалённом зуме (0.02–0.3)
// звезда не сжимается ниже ~4px на экране и остаётся яркой читаемой точкой.
// Экспорт (70a): hit-тест в events.js использует тот же радиус, что рендер.
export const MIN_STAR_PX = 4;

export async function drawSystem(canvas, spectralClass, planets, starRadius, starColor, width, height) {
    const dpr = window.devicePixelRatio || 1;
    canvas.width = width * dpr;
    canvas.height = height * dpr;
    canvas.style.width = width + 'px';
    canvas.style.height = height + 'px';

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    const layout = computeLayout(planets, starRadius, width, height);
    const { mainX, mainY, finalStarRadius, sizeMultiplier, stars } = layout;
    // Глобальные часы (51a): фаза планет не сбрасывается при переоткрытии модалки.
    const timeMs = performance.now();

    // Компактные остатки (ЧД/нейтронная/WD, 40a): свечение главной звезды
    // гасим — точка без ореола; обычные звёзды и протозвезда — как есть.
    const compactRemnant = ['black_hole', 'neutron', 'white_dwarf'].includes(modalState.starType);

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
            ctx.shadowBlur = compactRemnant ? 0 : 40;
        } else {
            ctx.shadowColor = s.color;
            ctx.shadowBlur = 25;
        }
        ctx.beginPath();
        // Минимальный радиус в мировых координатах: на отдалении звезда не
        // сжимается ниже MIN_STAR_PX экранных пикселей (51a).
        ctx.arc(s.x, s.y, Math.max(s.radius, MIN_STAR_PX / modalState.zoom), 0, 2 * Math.PI);
        ctx.fillStyle = s.color;
        ctx.fill();
        ctx.restore();
    });

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

        // ---- ПОДСВЕТКА ПРИ ХОВЕРЕ: ПЛАНЕТА ----
        if (modalState.hoveredObject && modalState.hoveredObject.type === 'planet') {
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

    // ---- ПОДСВЕТКА ПРИ ХОВЕРЕ: ЗВЁЗДЫ (70a) ----
    // Вне условия по планетам: в системе без планет подсветка звёзд тоже рисуется.
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
    } else if (modalState.hoveredObject && modalState.hoveredObject.type === 'star') {
        // Компаньон/внешний компаньон (70a): подсветка по честной позиции
        // из layout.stars.
        const s = layout.stars[modalState.hoveredObject.starIndex];
        if (s) {
            ctx.save();
            ctx.shadowColor = 'rgba(255,255,255,0.3)';
            ctx.shadowBlur = 25;
            ctx.beginPath();
            ctx.arc(s.x, s.y, s.radius + 4, 0, 2 * Math.PI);
            ctx.fillStyle = 'rgba(255,255,255,0.15)';
            ctx.fill();
            ctx.strokeStyle = 'rgba(255,255,255,0.7)';
            ctx.lineWidth = 2;
            ctx.stroke();
            ctx.restore();
        }
    }

    ctx.restore(); // сброс трансформации

    // ---- МИНИ-КАРТА (поверх всего) ----
    drawMiniMap(ctx, planets, width, height);
}
