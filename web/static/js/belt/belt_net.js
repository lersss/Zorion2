// web/static/js/belt/belt_net.js
// Сеть мини-игры добычи в поясе (осн. спека §5.3–§5.4): enter/collect/leave.
// Клиент не присылает позицию и не распоряжается трюмом — только событие
// сбора (сервер клампит) и команды входа/выхода. Токен — как у карты
// (localStorage/sessionStorage 'token').
function token() {
    return localStorage.getItem('token') || sessionStorage.getItem('token');
}

async function post(path, body) {
    let res;
    try {
        res = await fetch(path, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token(),
            },
            body: body ? JSON.stringify(body) : undefined,
        });
    } catch (e) {
        // Сеть недоступна — status 0 (экран ошибки даёт «Повторить», §5.3).
        return { ok: false, status: 0, error: 'Не удалось связаться с сервером.' };
    }
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (e) { data = { error: text }; }
    if (!res.ok) {
        return { ok: false, status: res.status, error: (data && data.error) || text || 'Ошибка сервера' };
    }
    return { ok: true, status: res.status, data };
}

// enter — вход в заход (идемпотентно: повторный вызов возвращает ту же сессию,
// §6.2 п.2). Пакет захода (§7) — единственный вход клиентского генератора.
export function enter(beltId) {
    return post('/api/belt/mine/enter', { belt_id: beltId });
}

// collect — событие сбора: amount — сколько клиент «набрал» за интервал (§5.3);
// resource — какой ресурс бурят: 'iron' | 'ice' (спека 2026-09-24 §2.4 R1).
// Ресурс называет клиент; опущенный resource сервер читает как железо (v1).
export function collect(amount, resource) {
    const body = { amount };
    if (resource) body.resource = resource;
    return post('/api/belt/mine/collect', body);
}

// leave — «вернуться на карту»: буфер захода в трюм, позиция → orbit/belt.
export function leave() {
    return post('/api/belt/mine/leave');
}
