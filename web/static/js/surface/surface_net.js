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
export function land(planetId) {
    return post('/api/surface/land', { planet_id: planetId });
}

// leave — «вызов корабля» и смерть: возврат на орбиту планеты.
export function leave() {
    return post('/api/surface/leave');
}
