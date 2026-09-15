// web/static/js/modal/layout.js
// Единый расчёт геометрии системы: орбиты и позиции планет во времени.
// Используется рендером, событиями (ховер/клик) и мини-картой, чтобы
// координаты всегда совпадали, а планеты вращались синхронно.
//
// Двойные/кратные (35b §6.1): барицентр пары — в центре кадра (cx, cy);
// главная смещена на d₁ = a·m₂/(m₁+m₂), компаньон — на d₂ = a·m₁/(m₁+m₂).
// Массы старых миров (NULL) → фолбэк 0.5/0.5 (§2.4). Wide-компаньон вне
// честного масштаба кадра — на краю кадра (гибрид-шкала, решение №3б).

import { modalState } from './state.js';
import { getStarColor, getStarSize } from './utils.js';

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
function starSizeOf(spec, maxStarRadius) {
    if (spec) return Math.min(getStarSize(spec, 'star'), maxStarRadius);
    return Math.min(modalState.companionColor ? 0.5 * modalState.starRadius : modalState.starRadius, maxStarRadius);
}

export function computeLayout(planets, starRadius, width, height) {
    const cx = width / 2;
    const cy = height / 2;
    const maxRadius = Math.min(width, height) * 0.4;
    const maxStarRadius = maxRadius * 0.25;
    const finalStarRadius = Math.min(starRadius, maxStarRadius);

    const planetCount = planets ? planets.length : 0;
    let sizeMultiplier = 1;
    let orbitSpacingMultiplier = 1;
    if (planetCount > 8 && planetCount <= 12) {
        sizeMultiplier = 0.85;
        orbitSpacingMultiplier = 0.9;
    } else if (planetCount > 12) {
        sizeMultiplier = 0.7;
        orbitSpacingMultiplier = 0.8;
    }

    const maxOrbit = planets ? planets.reduce((max, p) => Math.max(max, p.orbit_index), 0) : 0;
    const availableRadius = maxRadius - finalStarRadius * 1.8;
    const step = (maxOrbit > 0)
        ? (availableRadius / (maxOrbit + 1)) * orbitSpacingMultiplier
        : availableRadius / 3;

    // ---- ПАРА ЗВЁЗД (35b §6.1) ----
    // Главная звезда: по умолчанию в барицентре (одиночные — барицентр и есть
    // центр кадра). Для честной геометрии тесной пары смещается на d₁.
    let mainX = cx;
    let mainY = cy;
    const stars = [{ kind: 'main', x: cx, y: cy, radius: finalStarRadius, color: modalState.starColor }];

    const { isBinary, m1, m2, sepAU } = companionParams();
    // 35a-паритет: компаньон рисуется для любой binary/multiple (цвет фолбечится
    // на главный, если спектра нет).
    const hasCompanion = isBinary;
    const frameEdge = maxRadius - finalStarRadius * 1.2;

    if (hasCompanion && sepAU > 0) {
        // Честная барицентричная геометрия. Масштаб: 1 а.е. = finalStarRadius —
        // тесная пара (0.05–0.9 а.е.) рисуется рядом со звездой, wide (100+ а.е.)
        // — за пределами кадра (гибрид-шкала).
        const honest = sepAU * finalStarRadius;
        const d1 = honest * m2 / (m1 + m2);
        const d2 = honest * m1 / (m1 + m2);

        const compSpec = modalState.companion;
        const compColor = starColorOf(compSpec);
        const compRadius = starSizeOf(compSpec, maxStarRadius);

        if (d2 <= frameEdge) {
            // Честная геометрия: главная слева от барицентра, компаньон справа.
            mainX = cx - d1;
            mainY = cy;
            stars[0] = { kind: 'main', x: mainX, y: mainY, radius: finalStarRadius, color: modalState.starColor };
            stars.push({
                kind: 'companion', x: cx + d2, y: cy,
                radius: compRadius, color: compColor,
                sepAU: sepAU, atEdge: false,
            });
        } else {
            // Гибрид-шкала: главная в центре, компаньон на краю кадра
            // в направлении орбитального угла (решение №3б).
            const angle = (performance.now() * 0.0001) % (2 * Math.PI);
            stars.push({
                kind: 'companion',
                x: cx + Math.cos(angle) * frameEdge,
                y: cy + Math.sin(angle) * frameEdge,
                radius: compRadius, color: compColor,
                sepAU: sepAU, atEdge: true,
            });
        }

        // Внешние компаньоны кратных (35b §6.2): тоже на краю кадра.
        (modalState.extraCompanions || []).forEach((ec, i) => {
            const spec = ec && ec.spectral_class;
            const color = spec ? getStarColor(spec, 'star') : compColor;
            const radius = spec ? Math.min(getStarSize(spec, 'star'), maxStarRadius) : compRadius;
            const angle = (performance.now() * 0.0001 + (i + 1) * (Math.PI / 3)) % (2 * Math.PI);
            stars.push({
                kind: 'extra',
                x: cx + Math.cos(angle) * frameEdge,
                y: cy + Math.sin(angle) * frameEdge,
                radius, color,
                sepAU: (ec && typeof ec.sep_au === 'number') ? ec.sep_au : 0,
                atEdge: true,
            });
        });
    } else if (hasCompanion) {
        // Старые миры без companion_sep_au (35a-отрисовка, фолбэк §2.4):
        // close — компаньон вплотную справа; wide/кратные — на медленной орбите.
        const compSpec = modalState.companion;
        const compColor = starColorOf(compSpec);
        const isMultiple = modalState.systemType === 'multiple';
        const close35a = modalState.binaryType === 'close' && !isMultiple;
        if (close35a) {
            stars.push({
                kind: 'companion',
                x: cx + finalStarRadius * 0.8, y: cy,
                radius: finalStarRadius * 0.5, color: compColor,
                sepAU: 0, atEdge: false,
            });
        } else {
            const orbitR = Math.max(12, finalStarRadius * 1.8);
            const cr = finalStarRadius * 0.45;
            const baseAngle = (performance.now() * 0.0002) % (2 * Math.PI);
            const angles = isMultiple
                ? [baseAngle, baseAngle + Math.PI / 3]
                : [baseAngle];
            angles.forEach((a, i) => {
                stars.push({
                    kind: i === 0 ? 'companion' : 'extra',
                    x: cx + Math.cos(a) * orbitR,
                    y: cy + Math.sin(a) * orbitR,
                    radius: cr, color: compColor,
                    sepAU: 0, atEdge: false,
                });
            });
        }
    }

    return { cx, cy, mainX, mainY, finalStarRadius, step, maxOrbit, sizeMultiplier, stars, frameEdge };
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

// Угол планеты на орбите в момент времени timeMs (мс от начала анимации).
// Периоды — косметика (35b §6.1): S — визуальная формула 25 + r·0.35 с;
// P — по Кеплеру III от массы пары (√((M₁+M₂)/M₁)) с бо́льшим визуальным
// периодом (орбита 3a далеко, абсолют честного времени не моделируем).
export function getPlanetAngle(p, orbitRadius, idx, timeMs) {
    let periodMs = (25 + orbitRadius * 0.35) * 1000;
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

// Размер планет на экране (общий для рендера и ховера).
export function getPlanetSize(p, sizeMultiplier) {
    const type = (p.type || '').toLowerCase();
    if (type.includes('газовый') || type === 'gas_giant') return 22 * sizeMultiplier;
    if (type.includes('землеподобная') || type === 'terran' || type === 'earthlike') return 12 * sizeMultiplier;
    if (type.includes('пустынная') || type === 'desert') return 10 * sizeMultiplier;
    if (type.includes('ледяная') || type === 'ice') return 10 * sizeMultiplier;
    if (type.includes('вулканическая') || type === 'volcanic') return 10 * sizeMultiplier;
    if (type.includes('океаническая') || type === 'ocean') return 12 * sizeMultiplier;
    return 8 * sizeMultiplier;
}

export function getAnimTime() {
    // Время анимации относительно начала сессии модалки.
    return 0;
}