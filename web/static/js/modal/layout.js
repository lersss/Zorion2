// web/static/js/modal/layout.js
// Единый расчёт геометрии системы: орбиты и позиции планет во времени.
// Используется рендером, событиями (ховер/клик) и мини-картой, чтобы
// координаты всегда совпадали, а планеты вращались синхронно.
//
// Двойные/кратные (35b §6.1): барицентр пары — в центре кадра (cx, cy);
// тесная пара — главная смещена на d₁ = a·m₂/(m₁+m₂), компаньон — на
// d₂ = a·m₁/(m₁+m₂). Wide (51a): главная в центре кадра, компаньон честно
// на полном расстоянии a вне кадра (canvas обрежет). Звёзды статичны.
// Массы старых миров (NULL) → фолбэк 0.5/0.5 (§2.4). minimapStars — только
// фолбэк старых wide-миров без sepAU (честной позиции нет).

import { modalState } from './state.js';
import { getStarColor, getStarSize, fnv1a } from './utils.js';

// Параметры компаньона из состояния модалки (с фолбэками §2.4).
function companionParams() {
    const isBinary = modalState.systemType === 'binary' || modalState.systemType === 'multiple';
    const m1 = (typeof modalState.stellarMass === 'number' && modalState.stellarMass > 0)
        ? modalState.stellarMass : 0.5;
    const m2 = (typeof modalState.companionMass === 'number' && modalState.companionMass > 0)
        ? modalState.companionMass : 0.5;
    const sepAU = (typeof modalState.companionSepAU === 'number' && modalState.companionSepAU > 0)
        ? modalState.companionSepAU : 0;
    return { isBinary, m1, m2, sepAU };
}

// starColorOf — цвет звезды по спектру (компаньон) или по состоянию.
function starColorOf(spec) {
    if (spec) return getStarColor(spec, 'star');
    return modalState.companionColor || modalState.starColor;
}

// starSizeOf — размер звезды по спектру (для компаньона; главная — modalState.starRadius).
// Кламп под кадр снят: размер — от класса (спека §4.1/И-В1).
function starSizeOf(spec) {
    if (spec) return getStarSize(spec, 'star');
    return modalState.companionColor ? 0.5 * modalState.starRadius : modalState.starRadius;
}

// planetRadius — единый радиус планеты (спека §4.2): мировые px схемы от
// реального радиуса в R⊕. h = size^0.6, R_px = clamp(10·h, 5, 43); потолок 43 —
// страховка (макс. гигант 11.2 → 42.6), пол 5 — при size < 0.315. Битый/
// отсутствующий size → Земля (10 px), не NaN.
export function planetRadius(size) {
    const s = (typeof size === 'number' && isFinite(size) && size > 0) ? size : 1;
    const r = 10 * Math.pow(s, 0.6);
    return Math.max(5, Math.min(43, r));
}

// largestPlanetRadius — радиус крупнейшей планеты набора (0, если планет нет).
function largestPlanetRadius(planets) {
    return (planets || []).reduce((m, p) => Math.max(m, planetRadius(p && p.size)), 0);
}

// starFloorRadius — пол звезды (спека §4.1/§4.3): звезда не меньше
// 1.15·радиуса крупнейшей планеты своей подсистемы. Планеты компаньона/
// внешнего компонента в данных текущего мира не выделены (orbit_center —
// только main/barycenter, 35b; подсистемы компонентов — отдельные миры-
// записи, развилка Ф1 ещё не решена), поэтому пол для них считается по
// крупнейшей планете мира: консервативно — ни один звёздный компонент не
// мельче любой планеты системы.
function starFloorRadius(baseRadius, largestPlanetR) {
    return Math.max(baseRadius, 1.15 * largestPlanetR);
}

export function computeLayout(planets, starRadius, width, height) {
    const cx = width / 2;
    const cy = height / 2;
    const maxRadius = Math.min(width, height) * 0.4;

    // Пол звезды (спека §4.1, решение создателя 2026-09-22): звезда не меньше
    // 1.15·радиуса крупнейшей планеты своей системы. Кламп под кадр снят —
    // кадр окно, не рамка (И-В1/И-В2). Пол ≤ 49 (гигант 11.2 → 42.6), M (54)
    // и выше не затронуты, порядок классов сохраняется. Экзотика (WD/ЧД/НЗ/
    // протозвезда) компактна — пол к ней НЕ применяется (компактность — суть
    // класса, §4.1). Пол применяется к каждой звезде, включая компаньонов и
    // внешние компоненты (§4.3).
    const largestPlanetR = largestPlanetRadius(planets);
    const mainExotic = !!(modalState.starType && modalState.starType !== 'star');
    const finalStarRadius = mainExotic ? starRadius : starFloorRadius(starRadius, largestPlanetR);

    const planetCount = planets ? planets.length : 0;
    let orbitSpacingMultiplier = 1;
    if (planetCount > 8 && planetCount <= 12) {
        orbitSpacingMultiplier = 0.9;
    } else if (planetCount > 12) {
        orbitSpacingMultiplier = 0.8;
    }

    const maxOrbit = planets ? planets.reduce((max, p) => Math.max(max, p.orbit_index), 0) : 0;
    // availableRadius без вычитания звезды (спека §5.2): большая звезда больше
    // не «съедает» орбиты — клиренс starR·1.8 уходит в getOrbitRadius.
    const availableRadius = maxRadius;
    const step = (maxOrbit > 0)
        ? (availableRadius / (maxOrbit + 1)) * orbitSpacingMultiplier
        : availableRadius / 3;

    // ---- ПАРА ЗВЁЗД (35b §6.1) ----
    // Главная звезда: по умолчанию в барицентре (одиночные — барицентр и есть
    // центр кадра). Для честной геометрии тесной пары смещается на d₁.
    let mainX = cx;
    let mainY = cy;
    const stars = [{ kind: 'main', x: cx, y: cy, radius: finalStarRadius, color: modalState.starColor, sspec: modalState.spectralClass }];
    // minimapStars — только фолбэк старых wide-миров без sepAU (51a): честной
    // позиции у них нет, миникарта рисует их по старой позиции.
    const minimapStars = [];

    const { isBinary, m1, m2, sepAU } = companionParams();
    // 35a-паритет: компаньон рисуется для любой binary/multiple (цвет фолбечится
    // на главный, если спектра нет).
    const hasCompanion = isBinary;
    const frameEdge = maxRadius - finalStarRadius * 1.2;

    if (hasCompanion && sepAU > 0) {
        // Честная барицентричная геометрия. Масштаб: 1 а.е. = finalStarRadius —
        // тесная пара (0.05–0.9 а.е.) рисуется рядом со звездой, wide (100+ а.е.)
        // — за пределами кадра (гибрид-шкала, canvas обрежет).
        const honest = sepAU * finalStarRadius;
        const d1 = honest * m2 / (m1 + m2);
        const d2 = honest * m1 / (m1 + m2);

        const compSpec = modalState.companion;
        const compColor = starColorOf(compSpec);
        const compRadius = starFloorRadius(starSizeOf(compSpec), largestPlanetR);

        if (d2 > frameEdge) {
            // Wide (51a): главная в центре кадра (стартовый вид не уезжает),
            // компаньон честно на полном расстоянии honest справа (статика).
            mainX = cx;
            mainY = cy;
            stars[0] = { kind: 'main', x: mainX, y: mainY, radius: finalStarRadius, color: modalState.starColor, sspec: modalState.spectralClass };
            stars.push({
                kind: 'companion', x: cx + honest, y: cy,
                radius: compRadius, color: compColor, sspec: compSpec,
                sepAU: sepAU, atEdge: false,
            });
        } else {
            // Честная геометрия тесной пары: главная слева от барицентра,
            // компаньон справа (статика).
            mainX = cx - d1;
            mainY = cy;
            stars[0] = { kind: 'main', x: mainX, y: mainY, radius: finalStarRadius, color: modalState.starColor, sspec: modalState.spectralClass };
            stars.push({
                kind: 'companion', x: cx + d2, y: cy,
                radius: compRadius, color: compColor, sspec: compSpec,
                sepAU: sepAU, atEdge: false,
            });
        }

        // Внешние компаньоны кратных (35b §6.2): честно от пары по углу (51a);
        // без sep_au — только на миникарте (фолбэк старых миров).
        (modalState.extraCompanions || []).forEach((ec, i) => {
            const spec = ec && ec.spectral_class;
            const color = spec ? getStarColor(spec, 'star') : compColor;
            const radius = spec ? starFloorRadius(getStarSize(spec, 'star'), largestPlanetR) : compRadius;
            const angle = (i + 1) * (Math.PI / 3);
            const ecSepAU = (ec && typeof ec.sep_au === 'number') ? ec.sep_au : 0;
            if (ecSepAU > 0) {
                const r = ecSepAU * finalStarRadius;
                stars.push({
                    kind: 'extra',
                    x: cx + Math.cos(angle) * r,
                    y: cy + Math.sin(angle) * r,
                    radius, color, sspec: spec,
                    sepAU: ecSepAU, atEdge: false,
                });
            } else {
                minimapStars.push({
                    kind: 'extra',
                    x: cx + Math.cos(angle) * frameEdge,
                    y: cy + Math.sin(angle) * frameEdge,
                    radius, color,
                    sepAU: 0, atEdge: true,
                });
            }
        });
    } else if (hasCompanion) {
        // Старые миры без companion_sep_au (35a-отрисовка, фолбэк §2.4):
        // close — компаньон вплотную справа; wide/кратные — только на миникарте (51a).
        const compSpec = modalState.companion;
        const compColor = starColorOf(compSpec);
        const isMultiple = modalState.systemType === 'multiple';
        const close35a = modalState.binaryType === 'close' && !isMultiple;
        if (close35a) {
            stars.push({
                kind: 'companion',
                x: cx + finalStarRadius * 0.8, y: cy,
                radius: starFloorRadius(finalStarRadius * 0.5, largestPlanetR), color: compColor, sspec: compSpec,
                sepAU: 0, atEdge: false,
            });
        } else {
            const orbitR = Math.max(12, finalStarRadius * 1.8);
            const cr = finalStarRadius * 0.45;
            const baseAngle = 0;
            const angles = isMultiple
                ? [baseAngle, baseAngle + Math.PI / 3]
                : [baseAngle];
            angles.forEach((a, i) => {
                minimapStars.push({
                    kind: i === 0 ? 'companion' : 'extra',
                    x: cx + Math.cos(a) * orbitR,
                    y: cy + Math.sin(a) * orbitR,
                    radius: cr, color: compColor,
                    sepAU: 0, atEdge: false,
                });
            });
        }
    }

    // Максимальное расстояние не-главных звёзд от барицентра (51a): для
    // динамического минимума зума, чтобы все звёзды влезали в кадр.
    let maxStarDistPx = 0;
    stars.forEach(s => {
        if (s.kind !== 'main') {
            maxStarDistPx = Math.max(maxStarDistPx, Math.hypot(s.x - cx, s.y - cy));
        }
    });
    minimapStars.forEach(s => {
        maxStarDistPx = Math.max(maxStarDistPx, Math.hypot(s.x - cx, s.y - cy));
    });

    return { cx, cy, mainX, mainY, finalStarRadius, step, maxOrbit, stars, minimapStars, frameEdge, maxStarDistPx };
}

// planetOrbitCenter — центр вращения планеты (35b §6.1): P-планеты вокруг
// барицентра, S — вокруг главной. Старые миры без orbit_center — «main» (§2.4).
export function planetOrbitCenter(layout, p) {
    if (p && p.orbit_center === 'barycenter') {
        return { x: layout.cx, y: layout.cy };
    }
    return { x: layout.mainX, y: layout.mainY };
}

export function getOrbitRadius(layout, p, idx) {
    const randomOffset = (idx * 1.7) % 0.2 - 0.1;
    return layout.finalStarRadius * 1.8 + (p.orbit_index + 1) * layout.step * (1 + randomOffset);
}

// Угол планеты на орбите в момент времени timeMs (мс, глобальные часы).
// Периоды — косметика (35b §6.1): S — визуальная формула 25 + r·0.35 с,
// замедлена ×10 (51a); P — по Кеплеру III от массы пары (√((M₁+M₂)/M₁))
// с бо́льшим визуальным периодом (орбита 3a далеко, абсолют честного
// времени не моделируем).
export function getPlanetAngle(p, orbitRadius, idx, timeMs) {
    let periodMs = (25 + orbitRadius * 0.35) * 1000 * 10;
    if (p && p.orbit_center === 'barycenter') {
        const { m1, m2 } = companionParams();
        periodMs *= Math.sqrt((m1 + m2) / m1);
    }
    const speed = (2 * Math.PI) / periodMs;
    const base = (idx * 1.3 + 0.7) % (2 * Math.PI);
    return base + speed * timeMs;
}

export function getPlanetPose(layout, p, idx, timeMs) {
    const orbitRadius = getOrbitRadius(layout, p, idx);
    const angle = getPlanetAngle(p, orbitRadius, idx, timeMs);
    const center = planetOrbitCenter(layout, p);
    return {
        orbitRadius,
        angle,
        x: center.x + orbitRadius * Math.cos(angle),
        y: center.y + orbitRadius * Math.sin(angle)
    };
}

export function getAnimTime() {
    // Время анимации относительно начала сессии модалки.
    return 0;
}

// ==================== ПОЯСА МАЛЫХ ТЕЛ (ТЗ §9 «Пояс на схеме системы») ====================

// beltNoOrbitRank — порядковый номер пояса без orbit_index среди таких же поясов
// по возрастанию radius_au (0, 1, …): Койпера ближе Оорта. Нужен, чтобы пояса
// без якорной орбиты вставали за внешней орбитой по порядку, а не в одну точку.
function beltNoOrbitRank(belt) {
    const belts = (modalState.belts || []).filter(b => b && typeof b.orbit_index !== 'number');
    belts.sort((a, b) => (Number(a.radius_au) || 0) - (Number(b.radius_au) || 0));
    const i = belts.findIndex(b => b.id === (belt && belt.id));
    return i >= 0 ? i : 0;
}

// beltMidRadius — радиус осевой линии пояса в шкале схемы (мировые px). Пояс с
// orbit_index садится на орбиту этого номера ровно как планета (та же формула,
// что getOrbitRadius, без per-планетного разброса idx); пояс без orbit_index
// (Койпера/Оорта) — за внешней орбитой по порядку radius_au (И-В2: честно
// возможно за кадром, §9.0).
export function beltMidRadius(layout, belt) {
    if (belt && typeof belt.orbit_index === 'number') {
        return layout.finalStarRadius * 1.8 + (belt.orbit_index + 1) * layout.step;
    }
    return layout.finalStarRadius * 1.8 + (layout.maxOrbit + 2 + beltNoOrbitRank(belt)) * layout.step;
}

// beltRing — единая геометрия кольца пояса (осевая линия + полутолщина) для
// отрисовки и хит-теста (§9.4: один источник, прецедент planetRadius). Полутолщина
// = mid·width_au/radius_au, ограниченная экранным полом (тонкое кольцо не
// исчезает) и потолком 0.5·step (Койпера/Оорта не съедают кадр); при radius_au ≤ 0
// / нет данных — дефолт 5 % радиуса. Центр — главная звезда (околозвёздный пояс).
export function beltRing(layout, belt) {
    const radius = beltMidRadius(layout, belt);
    const rAU = Number(belt && belt.radius_au);
    const wAU = Number(belt && belt.width_au);
    let half = (isFinite(rAU) && rAU > 0 && isFinite(wAU) && wAU > 0)
        ? radius * (wAU / rAU)
        : radius * 0.05;
    // clamp(half, 3px_экран/zoom, 0.5·step): экранный пол применяется последним —
    // при малом зуме тонкое кольцо не исчезает (§9.4).
    const floor = 3 / modalState.zoom;
    const cap = 0.5 * layout.step;
    half = Math.max(floor, Math.min(half, cap));
    return { cx: layout.mainX, cy: layout.mainY, radius, half };
}

// beltAngle — канонический азимут точки пояса (рад): hash(belt.id) → 0..360°
// (§9.3), не зависит от времени кадра. Разные пояса — разный азимут.
export function beltAngle(belt) {
    return ((fnv1a(String((belt && belt.id) || '')) % 360) * Math.PI) / 180;
}

// beltArrivalAngles — азимут прибытия к поясу (правка создателя 2026-09-23):
// ближайшая точка осевой линии кольца к кораблю на момент старта полёта
// («не лететь через полкарты к чужой точке кольца»). Живёт в модуле — переживает
// закрытие/переоткрытие модалки (иначе маркер «я здесь» прыгал бы на канонический
// азимут); после перезагрузки страницы (F5) карта пуста — beltPoint берёт
// канонический beltAngle, и маркер не исчезает (§9.3 п.2).
const beltArrivalAngles = new Map();

// setBeltArrivalAngle — запомнить азимут прибытия к поясу (рад).
export function setBeltArrivalAngle(belt, angle) {
    const id = String((belt && belt.id) || '');
    if (!id || !isFinite(angle)) return;
    beltArrivalAngles.set(id, angle);
}

// beltNearestAngle — азимут ближайшей точки осевой линии кольца к точке (x, y)
// в канвасных координатах: направление от центра кольца на точку (§9.3: точка —
// на осевой линии). Совпадает с центром (dx=dy=0) — null (нет направления).
export function beltNearestAngle(layout, belt, x, y) {
    const g = beltRing(layout, belt);
    const dx = x - g.cx;
    const dy = y - g.cy;
    if (!isFinite(dx) || !isFinite(dy) || (dx === 0 && dy === 0)) return null;
    return Math.atan2(dy, dx);
}

// beltPoint — точка пояса (середина кольца, осевая линия): ближайшая к кораблю
// на момент старта полёта (beltArrivalAngles), иначе каноническая (beltAngle).
// Один источник для маркера «я здесь», начала/конца полёта, слежения камеры и
// чужих игроков в поясе (§9.3 п.1–2) — финальный кадр полёта совпадает с
// маркером без «прыжка».
export function beltPoint(layout, belt) {
    const g = beltRing(layout, belt);
    const stored = beltArrivalAngles.get(String((belt && belt.id) || ''));
    const a = isFinite(stored) ? stored : beltAngle(belt);
    return { x: g.cx + g.radius * Math.cos(a), y: g.cy + g.radius * Math.sin(a) };
}
