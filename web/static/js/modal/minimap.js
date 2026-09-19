// web/static/js/modal/minimap.js
import { modalState } from './state.js';
import { computeLayout, getPlanetPose } from './layout.js';

const MINI_SIZE = 120;

// formatAU — читаемое разделение: ≥ 100 а.е. — целое («342 а.е.»), иначе
// два знака («0.45 а.е.»).
function formatAU(au) {
    if (typeof au !== 'number' || !isFinite(au) || au <= 0) return '';
    return au >= 100 ? Math.round(au) + ' а.е.' : au.toFixed(2) + ' а.е.';
}

// starAu — расстояние компаньона от барицентра в а.е. (51a): честное sepAU;
// фолбэк старых позиций без sepAU (close35a / старые wide) — расстояние от
// барицентра в px, переведённое по шкале канваса (1 а.е. = finalStarRadius px).
function starAu(s, layout) {
    if (typeof s.sepAU === 'number' && s.sepAU > 0) return s.sepAU;
    return Math.hypot(s.x - layout.cx, s.y - layout.cy) / layout.finalStarRadius;
}

// miniObjects — объекты миникарты в линейной шкале (51a итер. 5): миниатюра
// канваса. Единый источник для отрисовки и хит-теста клика, чтобы координаты
// не расходились. world — мировая позиция объекта (для фокуса по клику).
export function miniObjects(planets, width, height) {
    const layout = computeLayout(planets, modalState.starRadius, width, height);
    // Глобальные часы (51a): фаза планет не сбрасывается при переоткрытии модалки.
    const timeMs = performance.now();
    const miniX = width - MINI_SIZE - 20;
    const miniY = height - MINI_SIZE - 20;
    const centerX = miniX + MINI_SIZE / 2;
    const centerY = miniY + MINI_SIZE / 2;
    const miniRadius = MINI_SIZE / 2 - 3;

    // Масштаб: systemRadius покрывает внешнюю планету (индексная шкала канваса),
    // планеты ≤ 0.9·MINI_SIZE/2 = 54px < miniRadius — маркеры звёзд всегда дальше.
    const systemRadius = Math.max(
        layout.finalStarRadius * 1.8,
        layout.finalStarRadius * 1.8 + (layout.maxOrbit + 1) * layout.step * 1.1
    );
    const miniScale = (MINI_SIZE * 0.9) / (systemRadius * 2);

    const objects = [];
    const stars = layout.stars.concat(layout.minimapStars);
    stars.forEach(s => {
        const sx = centerX + (s.x - layout.cx) * miniScale;
        const sy = centerY + (s.y - layout.cy) * miniScale;
        if (s.kind === 'main') {
            objects.push({
                kind: 'main', x: centerX, y: centerY, radius: 3, color: s.color,
                world: { x: layout.mainX, y: layout.mainY },
            });
        } else if (Math.hypot(sx - centerX, sy - centerY) <= miniRadius) {
            // Звезда внутри круга (close) — обычная точка по честной позиции.
            objects.push({
                kind: 'companion', x: sx, y: sy, radius: 2, color: s.color,
                world: { x: s.x, y: s.y },
            });
        } else {
            // Звезда за краем: маркер на границе круга + подпись расстояния
            // (оба кликабельны, мир-координата — честная позиция звезды).
            const angle = Math.atan2(sy - centerY, sx - centerX);
            const mx = centerX + Math.cos(angle) * miniRadius;
            const my = centerY + Math.sin(angle) * miniRadius;
            objects.push({
                kind: 'companion', x: mx, y: my, radius: 2, color: s.color,
                world: { x: s.x, y: s.y },
            });
            objects.push({
                kind: 'companion-label', x: mx - Math.cos(angle) * 17, y: my - Math.sin(angle) * 17,
                radius: 0, color: s.color,
                world: { x: s.x, y: s.y },
                label: formatAU(starAu(s, layout)),
            });
        }
    });

    if (planets) {
        planets.forEach((p, idx) => {
            const pose = getPlanetPose(layout, p, idx, timeMs);
            objects.push({
                kind: 'planet', index: idx,
                x: centerX + (pose.x - layout.cx) * miniScale,
                y: centerY + (pose.y - layout.cy) * miniScale,
                radius: 2, color: '#6fcf97',
                world: { x: pose.x, y: pose.y },
            });
        });
    }

    return { objects, miniX, miniY, miniSize: MINI_SIZE, layout, miniScale, centerX, centerY, miniRadius };
}

export function drawMiniMap(ctx, planets, width, height) {
    const { objects, miniX, miniY, miniSize, layout, miniScale, centerX, centerY } = miniObjects(planets, width, height);

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

    // Объекты в линейной шкале (51a итер. 5): главная — центр, планеты и
    // close-компаньоны — по канвасным позициям; звёзды за краем — маркер на
    // границе круга + подпись расстояния. Контент с clip — подписи не вылезают.
    ctx.save();
    ctx.beginPath();
    ctx.roundRect(miniX, miniY, miniSize, miniSize, 8);
    ctx.clip();
    objects.forEach(o => {
        if (o.label) {
            ctx.save();
            ctx.font = '9px system-ui';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillStyle = '#aab';
            ctx.fillText(o.label, o.x, o.y);
            ctx.restore();
        } else {
            ctx.save();
            ctx.beginPath();
            ctx.arc(o.x, o.y, o.radius, 0, 2 * Math.PI);
            ctx.fillStyle = o.color;
            ctx.fill();
            ctx.restore();
        }
    });
    ctx.restore();

    // ---- РАМКА ВИДИМОЙ ОБЛАСТИ (51a итер. 5, линейная шкала) ----
    // Центр viewport в мировых координатах → мини-координаты; размер — канвас
    // в мировых, умноженный на miniScale. В линейной шкале рамка честная.
    // Слежение камеры (запрос создателя 99.2.27): followOffset сдвигает вид —
    // рамка учитывает его, иначе во время полёта указывала бы не туда.
    const worldCenterX = (modalState.canvasWidth / 2 - modalState.offsetX - (modalState.followOffsetX || 0)) / modalState.zoom;
    const worldCenterY = (modalState.canvasHeight / 2 - modalState.offsetY - (modalState.followOffsetY || 0)) / modalState.zoom;
    const viewWidth = (modalState.canvasWidth / modalState.zoom) * miniScale;
    const viewHeight = (modalState.canvasHeight / modalState.zoom) * miniScale;
    const viewX = centerX + (worldCenterX - layout.cx) * miniScale - viewWidth / 2;
    const viewY = centerY + (worldCenterY - layout.cy) * miniScale - viewHeight / 2;

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
