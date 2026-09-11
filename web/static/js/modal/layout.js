// web/static/js/modal/layout.js
// Единый расчёт геометрии системы: орбиты и позиции планет во времени.
// Используется рендером, событиями (ховер/клик) и мини-картой, чтобы
// координаты всегда совпадали, а планеты вращались вокруг звезды синхронно.

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

    return { cx, cy, finalStarRadius, step, maxOrbit, sizeMultiplier };
}

export function getOrbitRadius(layout, p, idx) {
    const randomOffset = (idx * 1.7) % 0.2 - 0.1;
    return layout.finalStarRadius * 1.8 + (p.orbit_index + 1) * layout.step * (1 + randomOffset);
}

// Угол планеты на орбите в момент времени timeMs (мс от начала анимации).
export function getPlanetAngle(p, orbitRadius, idx, timeMs) {
    // Период оборота зависит от радиуса: внутренние планеты быстрее.
    const periodMs = (25 + orbitRadius * 0.35) * 1000;
    const speed = (2 * Math.PI) / periodMs;
    const base = (idx * 1.3 + 0.7) % (2 * Math.PI);
    return base + speed * timeMs;
}

export function getPlanetPose(layout, p, idx, timeMs) {
    const orbitRadius = getOrbitRadius(layout, p, idx);
    const angle = getPlanetAngle(p, orbitRadius, idx, timeMs);
    return {
        orbitRadius,
        angle,
        x: layout.cx + orbitRadius * Math.cos(angle),
        y: layout.cy + orbitRadius * Math.sin(angle)
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