// web/static/js/surface/surface_net.js
// Сеть прогулки (спека 2026-09-21 §6): land/leave. Клиент урон НЕ присылает
// (§8.7) — только два перехода. Токен — как у карты (localStorage 'token').
function token() {
    return localStorage.getItem('token') || sessionStorage.getItem('token');
}

async function post(path, body) {
    const res = await fetch(path, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + token(),
        },
        body: body ? JSON.stringify(body) : undefined,
    });
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (e) { data = { error: text }; }
    if (!res.ok) {
        return { ok: false, status: res.status, error: (data && data.error) || text || 'Ошибка сервера' };
    }
    return { ok: true, status: res.status, data };
}

// land — высадка (идемпотентно: повторный вызов возвращает тот же биом).
// biome — необязательный админский выбор биома (идея 2026-09-21); пусто —
// не передаём (сервер бросает жребий ∝ share, §5.1).
export function land(planetId, biome) {
    const body = { planet_id: planetId };
    if (biome) body.biome = biome;
    return post('/api/surface/land', body);
}

// leave — «вызов корабля» и смерть: возврат на орбиту планеты.
export function leave() {
    return post('/api/surface/leave');
}

// Текстуры тел неба (идея 2026-09-22 §8.4) — авторизованный /api/planet-image
// (как modal/textures.js): fetch + Bearer, blob → Image. Кэш по planet_id,
// промис резолвится в Image или null (401/ошибка) — null даёт фолбэк-диск.
const textureCache = new Map();

export function planetTexture(planetId) {
    if (!planetId) return Promise.resolve(null);
    if (textureCache.has(planetId)) return textureCache.get(planetId);
    const url = `/api/planet-image?planet_id=${encodeURIComponent(planetId)}&size=small`;
    const promise = fetch(url, { headers: { 'Authorization': 'Bearer ' + token() } })
        .then((res) => {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.blob();
        })
        .then((blob) => new Promise((resolve, reject) => {
            const objectUrl = URL.createObjectURL(blob);
            const img = new Image();
            img.onload = () => { URL.revokeObjectURL(objectUrl); resolve(img); };
            img.onerror = () => { URL.revokeObjectURL(objectUrl); reject(new Error('texture')); };
            img.src = objectUrl;
        }))
        .catch(() => null);
    textureCache.set(planetId, promise);
    return promise;
}
