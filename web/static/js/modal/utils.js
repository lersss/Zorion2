// utils.js
export function hashStringToNumber(str) {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
        const char = str.charCodeAt(i);
        hash = ((hash << 5) - hash) + char;
        hash |= 0;
    }
    return Math.abs(hash);
}

export function getClimateId(planet) {
    const type = (planet.type || '').toLowerCase();
    const temp = planet.temperature || 0;
    const water = planet.water_percent || 0;

    if (type.includes('вулканическая') || type.includes('лавовая')) return 'экстремальный';
    if (type.includes('пустынная') && temp > 300) return 'жаркий';
    if (type.includes('ледяная') || temp < 200) return 'холодный';
    if (type.includes('океаническая') || water > 60) return 'умеренный';
    if (temp > 350) return 'жаркий';
    if (temp > 200) return 'умеренный';
    if (temp > 100) return 'холодный';
    return 'изменчивый';
}

export function getStarColor(spectralClass) {
    const colors = {
        'O': '#9bb0ff', 'B': '#aabfff', 'A': '#cad7ff',
        'F': '#f8f7ff', 'G': '#fff4a3', 'K': '#ffd2a1',
        'M': '#ffb47c', 'L': '#ff8c5a', 'T': '#d95c14',
        'Y': '#9e4b2c'
    };
    return colors[spectralClass] || '#ffffff';
}

export function getStarSize(spectralClass) {
    const sizes = {
        'O': 120, 'B': 105, 'A': 90,
        'F': 75, 'G': 60,
        'K': 48, 'M': 36,
        'L': 30, 'T': 24,
        'Y': 18
    };
    return sizes[spectralClass] || 60;
}