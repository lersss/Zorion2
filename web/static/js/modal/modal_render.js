// web/static/js/modal/modal_render.js
import { modalState } from './state.js';
import { drawMiniMap } from './minimap.js';
import { getPlanetTexture } from './textures.js';
import { computeLayout, getOrbitRadius, getPlanetPose, planetRadius, planetOrbitCenter, beltRing, beltPoint } from './layout.js';
import { fnv1a } from './utils.js';
// Спрайт корабля игрока для маркера «я здесь»/корабля в полёте (спека 99.2.27
// §5.8/§5.11): ship_sprites.js — автономный модуль (не импортирует map/config.js,
// не требует canvas карты), перекраска gCO='hue' + восстановление альфы.
import { recolorShipSprite, shipOrientFor, shipDrawTransform } from '../map/ship_sprites.js';
// Звёздный фон (ТЗ @uidesigner, слой 0): starfield.js — автономный модуль
// (offscreen-тайл + fillRect за кадр, без map/config.js), read-only импорт.
import { drawStarfield, initStarfield } from '../map/starfield.js';
// Рецептура ядра звёзд (правка 2026-09-22 «мишень»): общая чистая coreStops из
// star_presets.js — формула живёт там, у модалки своя отрисовка (импорт
// star_render.js сюда запрещён: тянет map/config.js и ломает Node-граф).
import { coreStops } from '../map/star_presets.js';

// Минимальный экранный радиус звезды (51a): на отдалённом зуме (0.02–0.3)
// звезда не сжимается ниже ~4px на экране и остаётся яркой читаемой точкой.
// Экспорт (70a): hit-тест в events.js использует тот же радиус, что рендер.
export const MIN_STAR_PX = 4;

// Корабль в схеме — крошечное мировое тело (спека 2026-09-22 §4.3/И-В4):
// радиус SHIP_WORLD_R мировой px, экранный пол SHIP_MIN_PX — «виден на зуме».
// Свои/чужие/NPC — один радиус тела; кольцо «я здесь» и бейджи — экранный UI.
export const SHIP_WORLD_R = 1.0;
const SHIP_MIN_PX = 2;

// shipWorldSize — диаметр корабля в мировых px: 2·SHIP_WORLD_R с экранным полом
// SHIP_MIN_PX (пол 2 px — как у звезды, экранный, не кламп размера).
function shipWorldSize() {
    return Math.max(2 * SHIP_WORLD_R, SHIP_MIN_PX / modalState.zoom);
}

// FX_PX — экранный масштаб полётных эффектов (трасса, частицы, вспышки, свечение,
// пламя): остаются экранно-константными и читаемыми. Привязка эффектов к
// крошечному телу — отдельная техправка спеки (этап 2/3); тело рисуется
// shipWorldSize(), эффекты — FX_PX/zoom.
const FX_PX = 16;

// ==================== ЗВЁЗДНЫЙ ФОН (слой 0, ТЗ @uidesigner) ====================

// starfieldInited — module-level guard: initStarfield генерирует тайлы один раз
// (повторный вызов пересоздал бы их впустую; карта уже могла инициализировать).
let starfieldInited = false;

function ensureStarfield() {
    if (starfieldInited) return;
    starfieldInited = true;
    initStarfield();
}

// Стабилизация аргумента offset для starfield (запрос создателя «параллакс
// работает раз из 5»): непрерывное смещение от базы, не зависящее от
// zoom-пересчёта. При зуме база перепривязывается (аргумент = 0 — фон стоит,
// перепривязан к текущему виду); при пане/слежении аргумент растёт от базы
// плавно (фон дрейфует). Без этого zoom-пересчёт offsetX/Y (под курсор)
// попадал в аргумент скачком, а starfield-детект смены scale (1.5 + zoom)
// сбрасывал bgX=0 каждый кадр при непрерывном зуме → фон не накапливал.
let sfBaseX = 0;
let sfBaseY = 0;
let sfPrevZoom = null;

// ==================== ПУЛ ЧАСТИЦ (module-scope, ≤24, ТЗ @uidesigner) ====================

const MAX_PARTICLES = 24;
const particles = [];

// spawnTrailParticles — спавн частиц на хвосте корабля (позади, с разбросом).
// 80% — цвет корабля, 20% — белый; размеры 1.2–2.8/zoom (экранные).
function spawnTrailParticles(x, y, angle, count, color) {
    for (let i = 0; i < count; i++) {
        if (particles.length >= MAX_PARTICLES) particles.shift();
        const back = (0.3 + Math.random() * 0.5) * (110 / modalState.zoom);
        const px = x - Math.cos(angle) * back + (Math.random() - 0.5) * 6 / modalState.zoom;
        const py = y - Math.sin(angle) * back + (Math.random() - 0.5) * 6 / modalState.zoom;
        const speed = (0.05 + Math.random() * 0.1) / modalState.zoom;
        particles.push({
            x: px, y: py,
            vx: -Math.cos(angle) * speed + (Math.random() - 0.5) * 0.02 / modalState.zoom,
            vy: -Math.sin(angle) * speed + (Math.random() - 0.5) * 0.02 / modalState.zoom,
            born: performance.now(),
            life: 400 + Math.random() * 500, // 0.4–0.9 с
            color: Math.random() < 0.8 ? color : '#ffffff',
            size: (1.2 + Math.random() * 1.6) / modalState.zoom,
        });
    }
}

// updateAndDrawParticles — движение + отрисовка пула ('lighter', затухание).
function updateAndDrawParticles(ctx, now) {
    if (particles.length === 0) return;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    for (let i = particles.length - 1; i >= 0; i--) {
        const p = particles[i];
        const age = now - p.born;
        if (age >= p.life) {
            particles.splice(i, 1);
            continue;
        }
        p.x += p.vx;
        p.y += p.vy;
        const t = 1 - age / p.life;
        ctx.globalAlpha = t * 0.8;
        ctx.fillStyle = p.color;
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.size, 0, 2 * Math.PI);
        ctx.fill();
    }
    ctx.restore();
}

// ==================== ХЕЛПЕРЫ ====================

// hexToRgba — '#aabbcc' + alpha → 'rgba(r,g,b,alpha)'. Фолбэк — #fde68a.
function hexToRgba(hex, alpha) {
    let h = String(hex || '').replace('#', '');
    if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
    const n = parseInt(h, 16);
    if (isNaN(n)) return `rgba(253,230,138,${alpha})`;
    const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
    return `rgba(${r},${g},${b},${alpha})`;
}

// drawModalStar — «яркая точка + мягкое свечение» (рецептура карты §1/§4,
// правка 2026-09-22 «звёзды-мишень»): ореол rh = 1.6·rBase (мягче первой орбиты
// 1.8·rBase) и ядро rBase двумя радиальными градиентами. Числа ядра — общая
// coreStops(sspec); цвет — палитра модалки (s.color). Плоский круг + shadowBlur
// убраны.
function drawModalStar(ctx, x, y, rBase, color, sspec) {
    const rh = rBase * 1.6;
    const halo = ctx.createRadialGradient(x, y, 0, x, y, rh);
    halo.addColorStop(0, hexToRgba(color, 0.42));
    halo.addColorStop(0.20, hexToRgba(color, 0.20));
    halo.addColorStop(0.55, hexToRgba(color, 0.07));
    halo.addColorStop(1, hexToRgba(color, 0));
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(x, y, rh, 0, 2 * Math.PI);
    ctx.fill();

    const cs = coreStops(sspec);
    const core = ctx.createRadialGradient(x, y, 0, x, y, rBase);
    core.addColorStop(0, `rgba(255,255,255,${cs.a1})`);
    core.addColorStop(cs.r1, `rgba(255,255,255,${cs.a2})`);
    core.addColorStop(cs.r2, hexToRgba(color, cs.ac));
    core.addColorStop(1, hexToRgba(color, 0));
    ctx.fillStyle = core;
    ctx.beginPath();
    ctx.arc(x, y, rBase, 0, 2 * Math.PI);
    ctx.fill();
}

// roundRectPath — скруглённый прямоугольник (путь для fill/stroke).
function roundRectPath(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
}

// mulberry32 — детерминированный PRNG (как в belt_world.js): форма камней пояса
// без Math.random (инвариант ТЗ §9.6).
function mulberry32(seed) {
    let a = seed >>> 0;
    return function () {
        a |= 0; a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
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
    const { mainX, mainY, finalStarRadius, stars } = layout;
    // Глобальные часы (51a): фаза планет не сбрасывается при переоткрытии модалки.
    const timeMs = performance.now();

    // ---- СЛОЙ 0: ЗВЁЗДНЫЙ ФОН (ТЗ @uidesigner) ----
    // До ctx.save()/translate — starfield рисует в экранных координатах.
    // scale = 1.5 + zoom — фон виден всегда (альфа-рамп FADE_IN=1.0) и детект
    // смены масштаба в starfield работает (при зуме фон стоит).
    // Аргумент offset — СТАБИЛИЗИРОВАННЫЙ (запрос создателя «параллакс раз из
    // 5»): (offsetX + followOffsetX − sfBase)·4 — непрерывное смещение от базы,
    // перепривязанной при зуме. При пане/слежении растёт плавно (фон дрейфует),
    // при зуме — 0 (фон стоит, перепривязан). zoom-пересчёт offsetX/Y не
    // попадает в аргумент скачком.
    ensureStarfield();
    const sfOffsetX = modalState.offsetX + (modalState.followOffsetX || 0);
    const sfOffsetY = modalState.offsetY + (modalState.followOffsetY || 0);
    if (modalState.zoom !== sfPrevZoom) {
        sfPrevZoom = modalState.zoom;
        sfBaseX = sfOffsetX;
        sfBaseY = sfOffsetY;
    }
    drawStarfield(ctx, width, height, 1.5 + modalState.zoom,
        (sfOffsetX - sfBaseX) * 4,
        (sfOffsetY - sfBaseY) * 4,
        timeMs);

    // Компактные остатки (ЧД/нейтронная/WD, 40a): свечение главной звезды
    // гасим — точка без ореола; обычные звёзды и протозвезда — как есть.
    const compactRemnant = ['black_hole', 'neutron', 'white_dwarf'].includes(modalState.starType);

    ctx.save();
    // Слежение камеры (запрос создателя 99.2.27): followOffset — отдельное
    // смещение отрисовки, не трогает offsetX/Y (drag/zoom/миникарта как раньше).
    ctx.translate(modalState.offsetX + (modalState.followOffsetX || 0), modalState.offsetY + (modalState.followOffsetY || 0));
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

    // ---- СЛОЙ 1.5: ПОЯСА МАЛЫХ ТЕЛ (ТЗ §9, вариант C «россыпь камней») ----
    // Кольцо пояса встаёт между орбитами и звёздами (инвариант порядка слоёв
    // §9.6); прочие слои не переставляются.
    drawBelts(ctx, layout);

    // ---- СЛОЙ 2: ЗВЁЗДЫ (главная + компаньоны по честной геометрии, 35b §6.2) ----
    // Вид — «яркая точка + мягкое свечение» (рецептура карты §1/§4, правка
    // 2026-09-22 «мишень»): плоский круг + shadowBlur читались «просто кружком».
    // Экзотика главной (starType !== 'star') — вне правки: прежний вид (рецептуру
    // drawExotic карты сюда не переносим).
    stars.forEach(s => {
        // Минимальный радиус в мировых координатах: на отдалении звезда не
        // сжимается ниже MIN_STAR_PX экранных пикселей (51a).
        const rBase = Math.max(s.radius, MIN_STAR_PX / modalState.zoom);
        if (s.kind === 'main' && modalState.starType && modalState.starType !== 'star') {
            ctx.save();
            ctx.shadowColor = s.color;
            ctx.shadowBlur = compactRemnant ? 0 : 40;
            ctx.beginPath();
            ctx.arc(s.x, s.y, rBase, 0, 2 * Math.PI);
            ctx.fillStyle = s.color;
            ctx.fill();
            ctx.restore();
            return;
        }
        drawModalStar(ctx, s.x, s.y, rBase, s.color, s.sspec);
    });

    // ---- СЛОЙ 3: ПЛАНЕТЫ (АСИНХРОННАЯ ЗАГРУЗКА ТЕКСТУР) ----
    if (planets && planets.length > 0) {
        const loadPromises = planets.map(async (p, idx) => {
            let texture = null;
            try {
                texture = await getPlanetTexture(p, 'small');
            } catch (e) {
                console.warn('Failed to load texture for planet', p.id, e);
            }

            const pose = getPlanetPose(layout, p, idx, timeMs);
            const drawRadius = planetRadius(p.size);
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

    // ---- МАРКЕР «Я ЗДЕСЬ» / КОРАБЛЬ В ПОЛЁТЕ (спека 99.2.27 §5.8/§5.11) ----
    drawMyPosition(ctx, layout, planets, timeMs);

    // ---- ЧУЖИЕ ИГРОКИ НА КАНВАСЕ (слой 7, ТЗ @uidesigner) ----
    drawForeignPlayers(ctx, layout, planets, timeMs);

    ctx.restore(); // сброс трансформации

    // ---- МИНИ-КАРТА (поверх всего) ----
    drawMiniMap(ctx, planets, width, height);
}

// objectCanvasPos — позиция объекта системы на канвасе (спека 99.2.27 §5.8):
// звезда — layout.mainX/Y; компаньон — layout.stars[i]; планета — её орбита;
// спутник — позиция родительской планеты (спутники на канвасе не рисуются, v1).
// Экспорт — для слежения камеры за кораблём (index.js updateCameraFollow).
export function objectCanvasPos(layout, planets, objType, objId, timeMs) {
    if (objType === 'star') {
        if (objId === modalState.worldId) return { x: layout.mainX, y: layout.mainY };
        if (objId === modalState.companionId) {
            const s = layout.stars[1];
            if (s) return { x: s.x, y: s.y };
            return { x: layout.mainX, y: layout.mainY };
        }
        const m = /^extra:(.+):(\d+)$/.exec(objId || '');
        if (m) {
            // stars = [main, companion, extra0, extra1…]: внешний компаньон i
            // живёт в stars[i + 2] (ревью 99.2.27: off-by-one, +1 рисовал у соседа).
            const idx = parseInt(m[2], 10) + 2;
            const s = layout.stars[idx];
            if (s) return { x: s.x, y: s.y };
        }
        return { x: layout.mainX, y: layout.mainY };
    }
    if (objType === 'planet') {
        const idx = (planets || []).findIndex(p => p.id === objId);
        if (idx >= 0) {
            const pose = getPlanetPose(layout, planets[idx], idx, timeMs);
            return { x: pose.x, y: pose.y };
        }
    }
    if (objType === 'satellite') {
        const idx = (planets || []).findIndex(p =>
            (p.satellites || []).some(s => s.id === objId)
        );
        if (idx >= 0) {
            const pose = getPlanetPose(layout, planets[idx], idx, timeMs);
            return { x: pose.x, y: pose.y };
        }
    }
    // Пояс малых тел (ТЗ §9.3): каноническая точка — середина кольца на
    // детерминированном азимуте. Без геометрии пояса — прежний фолбэк (центр).
    if (objType === 'belt') {
        const b = (modalState.belts || []).find(x => x.id === objId);
        if (b) return beltPoint(layout, b);
    }
    return { x: layout.mainX, y: layout.mainY };
}

// orbitalPoint — орбитальная точка объекта (запрос создателя «корабль прыгает»):
// точка, где будет маркер «я здесь». Для star (главная/компаньон) — центр +
// смещение вправо (finalStarRadius + shipSize·0.5), как drawMyPosition; для
// planet/satellite — позиция объекта как есть. Полёт к звезде долетает до
// орбиты (у края звезды), а не в центр — финальная позиция совпадает с
// маркером, без прыжка в последнем кадре. Экспорт — для слежения камеры
// (index.js updateCameraFollow/centerOnPlayer).
export function orbitalPoint(layout, planets, objType, objId, timeMs) {
    const p = objectCanvasPos(layout, planets, objType, objId, timeMs);
    if (objType === 'star') {
        // Смещение у звезды — полкорпуса крошечного тела (этап 1, §4.3): та же
        // величина, что в drawMyPosition, иначе корабль «прыгает» в последнем
        // кадре полёта (финальная позиция ≠ маркер «я здесь»).
        const shipSize = shipWorldSize();
        return { x: p.x + layout.finalStarRadius + shipSize * 0.5, y: p.y };
    }
    return p;
}

// drawMyPosition — маркер «я здесь» (спека 99.2.27 §5.11): спрайт корабля
// игрока + зелёное пульс-кольцо (#4ade80). Подпись «вы» убрана (запрос
// создателя). Позиция = спутник → маркер у родительской планеты (М-4).
// У звезды — смещение вправо, чтобы корабль не сливался с ней. Во время
// полёта статичный маркер не рисуется — только движущийся корабль (§5.8).
function drawMyPosition(ctx, layout, planets, timeMs) {
    const pos = modalState.myPosition;
    if (!pos) return;
    if (pos.status === 'in_flight') {
        drawIntraFlightShip(ctx, layout, planets, timeMs, pos);
        return;
    }
    // surface — маркер «я здесь» у планеты (спека 2026-09-21 §7.6 п.5);
    // mining — игрок добывает в поясе (ТЗ §9.3 п.4): маркер в точке пояса,
    // янтарное кольцо.
    if (pos.status !== 'orbit' && pos.status !== 'surface' && pos.status !== 'mining') return;

    const p = objectCanvasPos(layout, planets, pos.object_type, pos.object_id, timeMs);
    // Тело — крошечное мировое (SHIP_WORLD_R, экранный пол 2 px); кольцо
    // «я здесь» — экранный UI (FX_PX/zoom), не размер тела (И-В4).
    const bodySize = shipWorldSize();
    const fxSize = FX_PX / modalState.zoom;

    // Смещение у звезды (§5.11): корабль справа от звезды, не поверх неё.
    let drawX = p.x;
    let drawY = p.y;
    if (pos.object_type === 'star') {
        drawX = p.x + layout.finalStarRadius + bodySize * 0.5;
    }

    // Пульс-кольцо: радиус fxSize·(1.25+0.25·sin), альфа 0.30 (UI). Цвет —
    // orbit зелёное #4ade80 (как у планет), mining янтарное #fde68a (как бейдж
    // «⛏ Добываете» в таблице, ТЗ §9.3 п.4).
    const ringR = fxSize * (1.25 + 0.25 * Math.sin(timeMs * 0.004));
    ctx.save();
    ctx.strokeStyle = pos.status === 'mining' ? 'rgba(253,230,138,0.30)' : 'rgba(74,222,128,0.30)';
    ctx.lineWidth = 1.5 / modalState.zoom;
    ctx.beginPath();
    ctx.arc(drawX, drawY, ringR, 0, 2 * Math.PI);
    ctx.stroke();
    ctx.restore();

    const sprite = recolorShipSprite(modalState.shipIcon, modalState.shipColor);
    if (sprite) {
        // Маркер «я здесь»: курса нет — каноническая поза пары (A, F), без
        // антипереворота (§6.2/§6.4).
        const t = shipDrawTransform(0, shipOrientFor(modalState.shipIcon));
        ctx.save();
        ctx.translate(drawX, drawY);
        ctx.rotate(t.rotate);
        ctx.scale(t.scaleX, t.scaleY);
        ctx.drawImage(sprite, -bodySize / 2, -bodySize / 2, bodySize, bodySize);
        ctx.restore();
    } else {
        // Фолбэк-ромб (И4): спрайт не загружен/имя неизвестно.
        ctx.save();
        ctx.beginPath();
        ctx.moveTo(drawX, drawY - bodySize / 2);
        ctx.lineTo(drawX + bodySize / 2, drawY);
        ctx.lineTo(drawX, drawY + bodySize / 2);
        ctx.lineTo(drawX - bodySize / 2, drawY);
        ctx.closePath();
        ctx.fillStyle = '#4ade80';
        ctx.fill();
        ctx.strokeStyle = '#0f172a';
        ctx.lineWidth = 1;
        ctx.stroke();
        ctx.restore();
    }
}

// ==================== ТРЁХФАЗНЫЙ ПОЛЁТ (ТЗ @uidesigner) ====================

// drawTrail — светящаяся трасса-хвост: 2 прохода (ядро 2/zoom + ореол 7/zoom),
// 'lighter', градиент от хвоста (альфа 0.15/0.04) к кораблю (0.85/0.18).
function drawTrail(ctx, x, y, angle, trailLen, color) {
    const tailX = x - Math.cos(angle) * trailLen;
    const tailY = y - Math.sin(angle) * trailLen;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.lineCap = 'round';
    const coreGrad = ctx.createLinearGradient(tailX, tailY, x, y);
    coreGrad.addColorStop(0, hexToRgba(color, 0.15));
    coreGrad.addColorStop(1, hexToRgba(color, 0.85));
    ctx.strokeStyle = coreGrad;
    ctx.lineWidth = 2 / modalState.zoom;
    ctx.beginPath();
    ctx.moveTo(tailX, tailY);
    ctx.lineTo(x, y);
    ctx.stroke();
    const haloGrad = ctx.createLinearGradient(tailX, tailY, x, y);
    haloGrad.addColorStop(0, hexToRgba(color, 0.04));
    haloGrad.addColorStop(1, hexToRgba(color, 0.18));
    ctx.strokeStyle = haloGrad;
    ctx.lineWidth = 7 / modalState.zoom;
    ctx.beginPath();
    ctx.moveTo(tailX, tailY);
    ctx.lineTo(x, y);
    ctx.stroke();
    ctx.restore();
}

// drawGhostRoute — тусклый призрак маршрута до цели (dash [3,8], альфа 0.14).
function drawGhostRoute(ctx, x, y, toX, toY) {
    ctx.save();
    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.lineTo(toX, toY);
    ctx.strokeStyle = 'rgba(253,230,138,0.14)';
    ctx.lineWidth = 1.5 / modalState.zoom;
    ctx.setLineDash([3 / modalState.zoom, 8 / modalState.zoom]);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();
}

// drawSpeedLines — 3 штриха за кормой в warp-фазе (крейсер), 'lighter'.
function drawSpeedLines(ctx, x, y, angle, shipSize, timeMs) {
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.strokeStyle = 'rgba(253,230,138,0.35)';
    ctx.lineWidth = 1 / modalState.zoom;
    for (let i = -1; i <= 1; i++) {
        const off = i * shipSize * 0.35;
        const px = x - Math.cos(angle) * shipSize * 0.6 + Math.cos(angle + Math.PI / 2) * off;
        const py = y - Math.sin(angle) * shipSize * 0.6 + Math.sin(angle + Math.PI / 2) * off;
        const len = shipSize * (1.2 + 0.6 * Math.sin(timeMs * 0.02 + i * 2));
        ctx.beginPath();
        ctx.moveTo(px, py);
        ctx.lineTo(px - Math.cos(angle) * len, py - Math.sin(angle) * len);
        ctx.stroke();
    }
    ctx.restore();
}

// drawPulseRings — вспышка старта/прибытия: 2 пульс-кольца (радиусы 0→3·shipSize
// и 0→4.5·shipSize, альфа 0.7→0), 'lighter'. ageMs — время с момента события.
function drawPulseRings(ctx, x, y, shipSize, ageMs, color) {
    const t = Math.min(1, ageMs / 1300);
    const alpha = 0.7 * (1 - t);
    if (alpha <= 0) return;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.strokeStyle = hexToRgba(color, alpha);
    ctx.lineWidth = 2 / modalState.zoom;
    ctx.beginPath();
    ctx.arc(x, y, 3 * shipSize * t, 0, 2 * Math.PI);
    ctx.stroke();
    ctx.strokeStyle = hexToRgba(color, alpha * 0.6);
    ctx.lineWidth = 1.5 / modalState.zoom;
    ctx.beginPath();
    ctx.arc(x, y, 4.5 * shipSize * t, 0, 2 * Math.PI);
    ctx.stroke();
    ctx.restore();
}

// drawTargetBreath — «вдох» цели при торможении: пульс-кольцо у цели.
function drawTargetBreath(ctx, toX, toY, shipSize, t, color) {
    ctx.save();
    ctx.strokeStyle = hexToRgba(color, 0.3 * t);
    ctx.lineWidth = 1.5 / modalState.zoom;
    ctx.beginPath();
    ctx.arc(toX, toY, shipSize * (1 + t * 0.8), 0, 2 * Math.PI);
    ctx.stroke();
    ctx.restore();
}

// drawShipGlow — свечение корабля: ореол radialGradient 1.6·shipSize.
function drawShipGlow(ctx, x, y, shipSize, color) {
    const glowR = shipSize * 1.6;
    const glow = ctx.createRadialGradient(x, y, 0, x, y, glowR);
    glow.addColorStop(0, hexToRgba(color, 0.35));
    glow.addColorStop(1, hexToRgba(color, 0));
    ctx.save();
    ctx.fillStyle = glow;
    ctx.beginPath();
    ctx.arc(x, y, glowR, 0, 2 * Math.PI);
    ctx.fill();
    ctx.restore();
}

// drawEngineFlame — пульс двигателя: пламя за кормой, sin-мерцание, 'lighter'.
function drawEngineFlame(ctx, x, y, angle, shipSize, color, timeMs) {
    const flicker = 0.7 + 0.3 * Math.sin(timeMs * 0.03);
    const flameLen = shipSize * 0.9 * flicker;
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(angle);
    ctx.globalCompositeOperation = 'lighter';
    const flameGrad = ctx.createLinearGradient(-shipSize * 0.5, 0, -shipSize * 0.5 - flameLen, 0);
    flameGrad.addColorStop(0, hexToRgba(color, 0.8));
    flameGrad.addColorStop(1, hexToRgba(color, 0));
    ctx.fillStyle = flameGrad;
    ctx.beginPath();
    ctx.moveTo(-shipSize * 0.5, -shipSize * 0.22);
    ctx.lineTo(-shipSize * 0.5 - flameLen, 0);
    ctx.lineTo(-shipSize * 0.5, shipSize * 0.22);
    ctx.closePath();
    ctx.fill();
    ctx.restore();
}

// drawIntraFlightShip — трёхфазный полёт (ТЗ @uidesigner): разгон p<0.12
// (вспышка старта, трейл 55px, частицы 6/кадр, корабль ×1.15), крейсер
// 0.12–0.88 (трейл 110px, speed-lines, свечение max, частицы 3/кадр),
// торможение p>0.88 (трейл сжимается, вспышка прибытия, «вдох» цели).
// Старый пунктир from→to удалён; вместо него — светящаяся трасса-хвост +
// тусклый призрак маршрута до цели. Эффекты — в слое между планетами и
// кораблём (сначала эффекты, потом спрайт). Guard dist<1 (спутник→своя
// планета): без трассы/частиц, только кольца + парение.
function drawIntraFlightShip(ctx, layout, planets, timeMs, pos) {
    // Орбитальные точки (запрос создателя «корабль прыгает»): цель к звезде —
    // орбита (смещение от центра, как маркер «я здесь»), не центр. from — тоже
    // орбитальная точка (корабль стартует с места маркера).
    const from = orbitalPoint(layout, planets, pos.from_type, pos.from_id, timeMs);
    const to = orbitalPoint(layout, planets, pos.to_type, pos.to_id, timeMs);
    const total = (pos.arrive_at || 0) - (pos.start_time || 0);
    const progress = total > 0 ? Math.min(1, Math.max(0, (Date.now() - pos.start_time) / total)) : 1;
    const now = Date.now();
    const dist = Math.hypot(to.x - from.x, to.y - from.y);

    const x = from.x + (to.x - from.x) * progress;
    const y = from.y + (to.y - from.y) * progress;
    // NaN-гвард (запрос создателя «чёрный экран»): битые from/to/времена не
    // должны уводить отрисовку в пустоту (NaN в translate рисует ничего).
    if (!isFinite(x) || !isFinite(y)) return;
    const angle = Math.atan2(to.y - from.y, to.x - from.x);

    // Эффекты полёта — экранный масштаб FX_PX/zoom (остаются читаемыми);
    // тело корабля — крошечное мировое (§4.3) и рисуется bodySize.
    const shipSize = FX_PX / modalState.zoom;
    const bodySize = shipWorldSize();
    const color = modalState.shipColor || '#fde68a';

    // Фазы.
    const accel = progress < 0.12;
    const cruise = progress >= 0.12 && progress <= 0.88;
    const brake = progress > 0.88;
    const isHover = dist < 1; // спутник → своя планета: парение без трассы

    // Длина трейла по фазе (экранные px → мировые /zoom).
    let trailLen;
    if (accel) trailLen = 55 / modalState.zoom;
    else if (cruise) trailLen = 110 / modalState.zoom;
    else trailLen = (110 / modalState.zoom) * (1 - (progress - 0.88) / 0.12); // сжимается

    // ---- ЭФФЕКТЫ (слой между планетами и кораблём) ----
    if (!isHover) {
        drawGhostRoute(ctx, x, y, to.x, to.y);
        drawTrail(ctx, x, y, angle, trailLen, color);
        // Частицы: 6/кадр (разгон), 3/кадр (крейсер); прогресс ≥ 1 — не спавнить.
        if (progress < 1) {
            spawnTrailParticles(x, y, angle, accel ? 6 : 3, color);
        }
        if (cruise) drawSpeedLines(ctx, x, y, angle, shipSize, timeMs);
    }
    updateAndDrawParticles(ctx, performance.now());

    // Вспышки старта/прибытия (кольца) — и при парении тоже.
    const startAge = now - (pos.start_time || 0);
    if (startAge >= 0 && startAge < 1300) {
        drawPulseRings(ctx, x, y, shipSize, startAge, color);
    }
    const arriveAge = (pos.arrive_at || 0) - now;
    if (arriveAge >= 0 && arriveAge < 1100) {
        drawPulseRings(ctx, x, y, shipSize, arriveAge, color);
    }
    // «Вдох» цели при торможении.
    if (brake) {
        drawTargetBreath(ctx, to.x, to.y, shipSize, (progress - 0.88) / 0.12, color);
    }

    // Парение при dist<1: sin(timeMs·0.003)·3/zoom.
    const hoverY = isHover ? Math.sin(timeMs * 0.003) * 3 / modalState.zoom : 0;
    const drawX = x;
    const drawY = y + hoverY;

    // Свечение корабля: ореол radialGradient 1.6·shipSize.
    drawShipGlow(ctx, drawX, drawY, shipSize, color);

    // Корабль: спрайт (или треугольник-фолбэк И4), ×1.15 при разгоне.
    // shadowBlur 18 (в warp — 26), /zoom — экранный размер свечения.
    const shipScale = accel ? 1.15 : 1;
    // Полный трансформ отрисовки (спека §6.2/§6.4): rotate = H + V·A,
    // антипереворот scaleY = V по КУРСУ H; масштаб разгона домножается.
    const t = shipDrawTransform(angle, shipOrientFor(modalState.shipIcon));
    ctx.save();
    ctx.translate(drawX, drawY);
    ctx.rotate(t.rotate);
    ctx.scale(t.scaleX * shipScale, t.scaleY * shipScale);
    ctx.shadowColor = hexToRgba(color, 0.8);
    ctx.shadowBlur = (cruise ? 26 : 18) / modalState.zoom;
    const sprite = recolorShipSprite(modalState.shipIcon, modalState.shipColor);
    if (sprite) {
        ctx.drawImage(sprite, -bodySize / 2, -bodySize / 2, bodySize, bodySize);
    } else {
        ctx.beginPath();
        ctx.moveTo(bodySize * 0.55, 0);
        ctx.lineTo(-bodySize * 0.4, -bodySize * 0.45);
        ctx.lineTo(-bodySize * 0.2, 0);
        ctx.lineTo(-bodySize * 0.4, bodySize * 0.45);
        ctx.closePath();
        ctx.fillStyle = '#4ade80';
        ctx.fill();
        ctx.strokeStyle = '#0f172a';
        ctx.lineWidth = 1;
        ctx.stroke();
    }
    ctx.restore();

    // Пламя двигателя (пульс) — поверх корабля.
    drawEngineFlame(ctx, drawX, drawY, angle, shipSize, color, timeMs);
}

// ==================== ЧУЖИЕ ИГРОКИ (слой 7, ТЗ @uidesigner) ====================

// drawForeignPlayers — чужие игроки на канвасе: из modalState.systemPlayers
// (стоящие; летящие отфильтрованы в loadSystemPlayers). Позиция — объект
// системы, смещение по кругу (FNV-1a(id)%360), размер 0.75·shipSize, спрайт +
// подложка-круг. Свой vs чужой: свой крупнее + зелёное кольцо + «вы» (рисует
// drawMyPosition), чужой мельче без подписи. Бейдж скопления N≥2 над объектом.
// Маркеры НЕ интерактивны (не перехватывают клики/ховер).
function drawForeignPlayers(ctx, layout, planets, timeMs) {
    const players = modalState.systemPlayers || [];
    if (players.length === 0) return;
    // Тело чужого — тот же крошечный радиус, что у своих (SHIP_WORLD_R, §4.3);
    // подложка-круг и развод по кругу — экранный UI (FX_PX/zoom), знак «чужой».
    const shipSize = FX_PX / modalState.zoom;
    const bodySize = shipWorldSize();
    const foreignSize = shipSize * 0.75;

    // Группировка по объекту для бейджа N≥2.
    const groups = new Map();
    for (const p of players) {
        const key = p.object_type + ':' + p.object_id;
        if (!groups.has(key)) groups.set(key, []);
        groups.get(key).push(p);
    }

    for (const p of players) {
        const obj = objectCanvasPos(layout, planets, p.object_type, p.object_id, timeMs);
        // Смещение по кругу (FNV-1a(id)%360), чтобы корабли не сливались.
        const ang = (fnv1a(p.id || '') % 360) * Math.PI / 180;
        const offR = shipSize * 0.9;
        const px = obj.x + Math.cos(ang) * offR;
        const py = obj.y + Math.sin(ang) * offR;

        // Подложка-круг.
        ctx.save();
        ctx.beginPath();
        ctx.arc(px, py, foreignSize * 0.75, 0, 2 * Math.PI);
        ctx.fillStyle = 'rgba(0,0,0,0.25)';
        ctx.fill();
        ctx.strokeStyle = 'rgba(255,255,255,0.08)';
        ctx.lineWidth = 1 / modalState.zoom;
        ctx.stroke();
        ctx.restore();

        // Спрайт (фолбэк-ромб И4); чужой стоящий игрок: курса нет — каноническая
        // поза пары (A, F), без антипереворота (§6.2/§6.4).
        const sprite = recolorShipSprite(p.ship_icon, p.ship_color);
        if (sprite) {
            const t = shipDrawTransform(0, shipOrientFor(p.ship_icon));
            ctx.save();
            ctx.translate(px, py);
            ctx.rotate(t.rotate);
            ctx.scale(t.scaleX, t.scaleY);
            ctx.drawImage(sprite, -bodySize / 2, -bodySize / 2, bodySize, bodySize);
            ctx.restore();
        } else {
            ctx.save();
            ctx.beginPath();
            ctx.moveTo(px, py - bodySize / 2);
            ctx.lineTo(px + bodySize / 2, py);
            ctx.lineTo(px, py + bodySize / 2);
            ctx.lineTo(px - bodySize / 2, py);
            ctx.closePath();
            ctx.fillStyle = '#38bdf8';
            ctx.fill();
            ctx.strokeStyle = '#0f172a';
            ctx.lineWidth = 1;
            ctx.stroke();
            ctx.restore();
        }
    }

    // Бейдж скопления N≥2 над объектом (кружок 18/zoom, обводка #fde68a).
    for (const group of groups.values()) {
        if (group.length < 2) continue;
        const first = group[0];
        const obj = objectCanvasPos(layout, planets, first.object_type, first.object_id, timeMs);
        const badgeR = 18 / modalState.zoom;
        ctx.save();
        ctx.beginPath();
        ctx.arc(obj.x, obj.y - badgeR * 1.2, badgeR, 0, 2 * Math.PI);
        ctx.fillStyle = 'rgba(30,30,50,0.85)';
        ctx.fill();
        ctx.strokeStyle = '#fde68a';
        ctx.lineWidth = 1.5 / modalState.zoom;
        ctx.stroke();
        ctx.fillStyle = '#fff';
        ctx.font = `bold ${10 / modalState.zoom}px system-ui`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(String(group.length), obj.x, obj.y - badgeR * 1.2);
        ctx.textBaseline = 'alphabetic';
        ctx.restore();
    }
}

// ==================== ПОЯСА МАЛЫХ ТЕЛ (ТЗ §9, вариант C) ====================

// Палитра камня — существующая (belt_render.js COLORS, ТЗ §9.0): новых цветов
// не вводим.
const BELT_STONE_COLORS = ['#6b7280', '#8b93a1', '#454c58', '#3b414b'];
// Тона камней при наведении — подсветка россыпи вместо подложки-annulus.
const BELT_STONE_HOVER_COLORS = ['#94a3b8', '#a8b3c2', '#7c8798', '#8b93a1'];
// Выработанный пояс: светлых тонов нет (ТЗ §5) — только тёмная часть палитры.
const BELT_STONE_DEPLETED_COLORS = ['#6b7280', '#454c58', '#3b414b'];
// Веса тонов (та же палитра, новых цветов нет): светлые реже — светотень
// (светлый край) читается на тёмном теле, россыпь не сливается в светлую кашу.
const BELT_STONE_TONE_WEIGHTS = [0.32, 0.14, 0.30, 0.24];
// Светотень (ТЗ §4): свет со стороны звезды — светлый край на внутренней
// стороне камня, тёмный кант на внешней. Оба тона — из существующей палитры.
const BELT_STONE_LIGHT = '#8b93a1';
const BELT_STONE_SHADOW = '#3b414b';
// Пороги светотени в ЭКРАННЫХ px камня: мельче — только тон (иначе грязь).
const BELT_LIGHT_MIN_PX = 2.5;
const BELT_SHADOW_MIN_PX = 4;

// beltStoneCount — число камней россыпи пояса: от ПРОТЯЖЁННОСТИ (width_au —
// видимая геометрия карточки). Класс/богатство (belt_class, remaining_level) НЕ
// используются: они скрыты до скана, а вид пояса не должен меняться после скана
// (утечка знания; решение приёмной 2026-09-23). Внутри полосы точное число — из
// сида fnv1a(belt.id) (детерминизм §9.6). Выработанный пояс — вдвое реже, но не
// меньше 2 камней (иначе пояс исчезнет со схемы, а ПКМ по нему нужен).
export function beltStoneCount(belt) {
    const w = Number(belt && belt.width_au);
    let min, max;
    if (!isFinite(w) || w <= 0) { min = 10; max = 16; } // нет данных — «средняя» полоса
    else if (w < 0.5) { min = 6; max = 10; }
    else if (w < 2) { min = 10; max = 16; }
    else if (w <= 8) { min = 16; max = 22; }
    else { min = 22; max = 28; }
    const rng = mulberry32(fnv1a(String((belt && belt.id) || '')));
    const count = min + Math.floor(rng() * (max - min + 1));
    if (belt && belt.remaining_level === 'выработан') {
        return Math.max(2, Math.round(count * 0.5));
    }
    return count;
}

// drawBelts — слой кольца поясов (вариант C «россыпь камней», ТЗ §9.1/§9.2).
// Геометрия — из beltRing (один источник с хит-тестом, §9.4); цвета —
// существующая палитра.
function drawBelts(ctx, layout) {
    const belts = modalState.belts || [];
    if (belts.length === 0) return;
    belts.forEach(b => {
        const g = beltRing(layout, b);
        if (!isFinite(g.radius) || g.radius <= 0 || !isFinite(g.half) || g.half <= 0) return;
        const depleted = b.remaining_level === 'выработан';
        const hovered = !!modalState.hoveredObject &&
            modalState.hoveredObject.type === 'belt' && modalState.hoveredObject.id === b.id;

        // Подложки-annulus нет (правки создателя 2026-09-23: сначала «не нужна эта
        // обводка», затем «полупрозрачное кольцо так и осталось») — пояс на схеме
        // рисуется ТОЛЬКО россыпью камней. Геометрия beltRing остаётся источником
        // для хит-теста/ПКМ (невидимая зона клика, §9.4); интерактив (ховер) —
        // подсветкой камней, а не подложкой.
        drawBeltStones(ctx, b, g, depleted, hovered);
    });
}

// drawBeltStones — россыпь камней пояса. Число камней — beltStoneCount (от
// протяжённости пояса); раскладка: 2–6 куч по 3–6 камней с пустыми
// промежутками между ними + 0–2 одиночки (ТЗ визуального дизайна 2026-09-23 §2);
// размеры: ~20–25 % «якорных» (×1.0–1.5 от base), остальные мелкие (×0.5–0.85) —
// разброс вместо «одинаковых» (§1); крупные рисуются поверх мелких (§5).
// Детерминизм от belt.id (mulberry32), без Math.random (инвариант §9.6).
// Светотень — ОТДЕЛЬНЫМ проходом после тел (§4): переживёт подмену камня на
// спрайт (drawBeltStone остаётся единой точкой подмены, §9.5).
function drawBeltStones(ctx, belt, g, depleted, hovered) {
    const rng = mulberry32(fnv1a(String(belt.id)));
    const count = beltStoneCount(belt);
    const base = Math.max(g.half * 0.45, 1.5 / modalState.zoom); // экранный пол
    const tones = hovered
        ? BELT_STONE_HOVER_COLORS
        : (depleted ? BELT_STONE_DEPLETED_COLORS : BELT_STONE_COLORS);

    // Кучи и одиночки (§2): count камней = кучи (по 3–6) + 0–2 одиночки.
    const nClusters = Math.max(2, Math.min(6, Math.round(count / 5)));
    const singles = Math.min(Math.floor(rng() * 3), Math.max(0, count - nClusters * 3));
    const inClusters = count - singles;
    const perCluster = [];
    for (let i = 0; i < nClusters; i++) {
        perCluster.push(Math.floor(inClusters / nClusters) + (i < inClusters % nClusters ? 1 : 0));
    }

    const stones = [];
    // sizeFor — «якорные» камни (×1.0–1.5) против мелких (×0.5–0.85) (§1).
    const sizeFor = () => (rng() < 0.22 ? base * (1.0 + rng() * 0.5) : base * (0.5 + rng() * 0.35));
    // Тон с весами (светлые реже) — по тому же rng (детерминизм §9.6).
    const toneFor = () => {
        let r = rng();
        for (let i = 0; i < tones.length; i++) {
            r -= BELT_STONE_TONE_WEIGHTS[i] || 0;
            if (r <= 0) return tones[i];
        }
        return tones[tones.length - 1];
    };
    const pushStone = (angle, size, radialJitter) => {
        const rr = g.radius + (rng() - 0.5) * radialJitter;
        stones.push({
            x: g.cx + Math.cos(angle) * rr,
            y: g.cy + Math.sin(angle) * rr,
            size,
            rot: rng() * 2 * Math.PI,
            verts: 7 + Math.floor(rng() * 5),                        // 7..11 вершин (§3)
            spread: size >= base ? [0.60, 1.0] : [0.80, 1.0],        // у крупных разброс сильнее (§3)
            aspect: rng() < 0.3 ? 1.3 + rng() * 0.3 : 1,             // ~30 % вытянутые (§3)
            tone: toneFor(),
        });
    };
    for (let c = 0; c < nClusters; c++) {
        const a0 = rng() * 2 * Math.PI;
        const n = perCluster[c];
        // Куча плотная, но камни НЕ сливаются: дуга ≈ 2.1 радиуса на камень
        // (≈1 поперечник + малый зазор); поперёк кольца разброс малый — куча
        // читается сгустком, между кучами остаются пустые промежутки (§2).
        const span = (n * base * 2.1) / Math.max(1, g.radius);
        for (let i = 0; i < n; i++) {
            const t = n === 1 ? 0.5 : i / (n - 1);
            pushStone(a0 + (t - 0.5) * span, sizeFor(), Math.min(g.half, base * 1.6));
        }
    }
    for (let i = 0; i < singles; i++) pushStone(rng() * 2 * Math.PI, sizeFor(), g.half);

    stones.sort((p, q) => p.size - q.size); // крупные поверх мелких (§5)

    ctx.save();
    if (depleted) ctx.globalAlpha = 0.4; // выработан — приглушено (§9.1)
    for (const s of stones) drawBeltStone(ctx, s);
    // Светотень (§4): у выработанного светлых тонов/блика нет — проход пропущен.
    if (!depleted) {
        for (const s of stones) drawBeltStoneLight(ctx, s, g);
    }
    ctx.restore();
}

// beltStonePath — путь камня (общий для тела и светотени): процедурный
// многоугольник в локальной системе (translate/rotate/scale(aspect)); координаты
// запекаются в путь, transform снимается сразу. Форма — от позиции/размера камня
// (детерминизм, без Math.random).
function beltStonePath(ctx, s) {
    const rng = mulberry32(((Math.floor(s.x * 7) ^ Math.floor(s.y * 13) ^ Math.floor(s.size * 31)) >>> 0));
    ctx.save();
    ctx.translate(s.x, s.y);
    ctx.rotate(s.rot);
    ctx.scale(s.aspect, 1);
    ctx.beginPath();
    for (let i = 0; i < s.verts; i++) {
        const a = (i / s.verts) * 2 * Math.PI;
        const r = s.size * (s.spread[0] + rng() * (s.spread[1] - s.spread[0]));
        const px = Math.cos(a) * r;
        const py = Math.sin(a) * r;
        if (i === 0) ctx.moveTo(px, py);
        else ctx.lineTo(px, py);
    }
    ctx.closePath();
    ctx.restore();
}

// drawBeltStone — тело камня (заливка тоном). ЕДИНАЯ точка будущей подмены на
// спрайт (drawImage из web/static/sprites/belt/, направление 2 §1): геометрия
// кольца, точка пояса и хит-тест при этом не трогаются (§9.5), а светотень —
// отдельный проход (drawBeltStoneLight), поэтому переживёт подмену. Обводки нет
// (правка создателя 2026-09-23).
function drawBeltStone(ctx, s) {
    beltStonePath(ctx, s);
    ctx.fillStyle = s.tone;
    ctx.fill();
}

// drawBeltStoneLight — светотень камня (§4): звезда — центр кольца, свет
// радиально наружу ⇒ светлый край (BELT_STONE_LIGHT) на ВНУТРЕННЕЙ стороне
// камня, тёмный кант (BELT_STONE_SHADOW) — на внешней. Свет ЭКРАННЫЙ: считается
// от направления «центр кольца → камень» и не зависит от rot камня. Пороги по
// экранному размеру: < 2.5 px — только тон, ≥ 4 px — ещё и тёмный кант.
function drawBeltStoneLight(ctx, s, g) {
    const screenPx = s.size * modalState.zoom;
    if (screenPx < BELT_LIGHT_MIN_PX) return;
    let ux = s.x - g.cx;
    let uy = s.y - g.cy;
    const len = Math.hypot(ux, uy);
    if (!isFinite(len) || len === 0) return;
    ux /= len;
    uy /= len;
    const perpX = -uy;
    const perpY = ux;
    const big = s.size * 4; // заведомо больше камня

    // Полуплоскость от линии через точку (cut вдоль u): innerSide — сторона к
    // звезде (dot ≤ cut), иначе внешняя (dot ≥ cut). Обрезается путём камня.
    const halfPlane = (cut, color, innerSide) => {
        const sgn = innerSide ? -1 : 1;
        const bx = s.x + ux * cut;
        const by = s.y + uy * cut;
        const farX = ux * sgn * 2 * big;
        const farY = uy * sgn * 2 * big;
        ctx.beginPath();
        ctx.moveTo(bx + perpX * big, by + perpY * big);
        ctx.lineTo(bx - perpX * big, by - perpY * big);
        ctx.lineTo(bx - perpX * big + farX, by - perpY * big + farY);
        ctx.lineTo(bx + perpX * big + farX, by + perpY * big + farY);
        ctx.closePath();
        ctx.fillStyle = color;
        ctx.fill();
    };

    ctx.save();
    beltStonePath(ctx, s);
    ctx.clip();
    halfPlane(-s.size * 0.25, BELT_STONE_LIGHT, true);
    if (screenPx >= BELT_SHADOW_MIN_PX) halfPlane(s.size * 0.6, BELT_STONE_SHADOW, false);
    ctx.restore();
}
