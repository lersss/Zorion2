// web/static/js/modal/textures.js
const textureCache = new Map();

export function getPlanetTexture(planet, spectralClass, sizeMultiplier) {
    const key = `planet_${planet.id || planet.orbit_index}_${spectralClass}`;
    if (textureCache.has(key)) {
        return textureCache.get(key);
    }

    const seed = planet.id ? hashStringToNumber(planet.id) : Date.now() + planet.orbit_index;
    const climateId = getClimateId(planet);
    // Увеличиваем радиус для лучшего качества
    const radius = Math.min(40, 20 + (planet.size || 10) * 1.5);

    const url = `/api/planet-image?seed=${seed}&starType=${spectralClass}&climateId=${climateId}&radius=${radius}`;

    // Создаём промис, который загружает изображение через fetch
    const promise = new Promise((resolve, reject) => {
        const img = new Image();
        img.crossOrigin = 'anonymous';
        img.onload = () => {
            resolve(img);
        };
        img.onerror = () => {
            reject(new Error('Failed to load planet texture'));
        };
        img.src = url;
    });

    textureCache.set(key, promise);
    return promise;
}

export function clearTextureCache() {
    textureCache.clear();
}

// Вспомогательные функции
function hashStringToNumber(str) {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
        const char = str.charCodeAt(i);
        hash = ((hash << 5) - hash) + char;
        hash |= 0;
    }
    return Math.abs(hash);
}

function getClimateId(planet) {
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