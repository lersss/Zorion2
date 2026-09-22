// web/static/js/modal/textures.js
import { modalState } from './state.js';

const textureCache = new Map();

// getPlanetTexture — текстура планеты (спека 2026-09-20 §6.1 + дельта
// 2026-09-21 §6.1): авторизованный /api/planet-image (planet_id + size, JWT),
// fetch+blob. Ключ кэша — (id, size, own/known/unknown): прокси трёх режимов
// (own = full — своя система, known = honest — есть Knowledge, unknown = stub);
// сервер по-прежнему решает режим — клиентский ключ лишь не держит устаревший
// PNG после скана чужой планеты. 401 → reject без редиректа (фолбэк-круг на
// канвасе, §6.1).
export function getPlanetTexture(planet, size) {
    // own — своя система (in_own_system с сервера, баг 2026-09-22): режим full
    // решает сервер по current_world_id == мира планеты (не по my_position).
    const own = !!modalState.inOwnSystem;
    const known = !!planet.knowledge;
    const key = `planet_${planet.id}_${size}_${own ? 'own' : (known ? 'known' : 'unknown')}`;
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