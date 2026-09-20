// web/static/js/modal/textures.js
import { modalState } from './state.js';

const textureCache = new Map();

// getPlanetTexture — текстура планеты (спека 2026-09-20 §6.1): авторизованный
// /api/planet-image (planet_id + size, JWT), fetch+blob. Ключ кэша — (id, size,
// own/foreign): режим картинки (честная/заглушка) решает сервер, клиент не
// знает его заранее — own/foreign — прокси режима (своя система = честная).
// 401 → reject без редиректа (фолбэк-круг на канвасе, §6.1).
export function getPlanetTexture(planet, size) {
    const own = !!modalState.myPosition;
    const key = `planet_${planet.id}_${size}_${own ? 'own' : 'foreign'}`;
    if (textureCache.has(key)) {
        return textureCache.get(key);
    }

    const token = modalState.authToken || localStorage.getItem('token');
    const url = `/api/planet-image?planet_id=${encodeURIComponent(planet.id)}&size=${size}`;

    const promise = fetch(url, { headers: { 'Authorization': 'Bearer ' + token } })
        .then(res => {
            if (res.status === 401) {
                throw new Error('unauthorized');
            }
            if (!res.ok) {
                throw new Error('HTTP ' + res.status);
            }
            return res.blob();
        })
        .then(blob => new Promise((resolve, reject) => {
            const objectUrl = URL.createObjectURL(blob);
            const img = new Image();
            img.onload = () => {
                // URL можно revoke сразу после декодирования — изображение
                // остаётся в памяти браузера (orbit view не держит URL).
                URL.revokeObjectURL(objectUrl);
                resolve(img);
            };
            img.onerror = () => {
                URL.revokeObjectURL(objectUrl);
                reject(new Error('Failed to load planet texture'));
            };
            img.src = objectUrl;
        }));

    textureCache.set(key, promise);
    return promise;
}

export function clearTextureCache() {
    textureCache.clear();
}