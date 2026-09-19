// web/static/js/auth.js
// Ядро авторизации (спека переноса-студии-товаров-iterC §8.1): id-независимые
// примитивы — токен/роль//me//login/401. Логика — один раз; адаптеры админки
// (admin/auth.js) и студии (studio/auth.js) делегируют сюда, поведение 1:1
// (консолидация, не изменение). На верхнем уровне — только константы и
// localStorage (DOM/toast не нужны — frontend_test.go грузит граф в Node).
const TOKEN_KEY = 'adminToken';
const ADMIN_ROLES = ['admin', 'skycomposer'];

let adminToken = localStorage.getItem(TOKEN_KEY) || '';

export function getToken() { return adminToken; }
export function setToken(t) { adminToken = t; localStorage.setItem(TOKEN_KEY, t); }
export function clearToken() { adminToken = ''; localStorage.removeItem(TOKEN_KEY); }
export function isAdminRole(role) { return ADMIN_ROLES.includes(role); }

// validateToken — GET /me с Bearer → {ok, role} (без UI).
export async function validateToken() {
    if (!adminToken) return { ok: false, role: null };
    try {
        const res = await fetch('/me', { headers: { Authorization: 'Bearer ' + adminToken } });
        if (!res.ok) return { ok: false, role: null };
        const me = await res.json();
        return { ok: true, role: me.role };
    } catch (e) {
        return { ok: false, role: null };
    }
}

// login — POST /login → {ok, token, role, username, error} (без UI).
export async function login(username, password) {
    try {
        const res = await fetch('/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        });
        if (!res.ok) return { ok: false, error: 'Неверный логин или пароль' };
        const data = await res.json();
        if (!isAdminRole(data.user.role)) {
            return { ok: false, error: 'Недостаточно прав: нужна роль администратора' };
        }
        return { ok: true, token: data.token, role: data.user.role, username: data.user.username };
    } catch (e) {
        return { ok: false, error: 'Ошибка соединения с сервером' };
    }
}

// fetchWithAuth — fetch с Bearer; 401 → clearToken() + onUnauthorized() +
// throw 'Unauthorized' (адаптер решает, что показать — оверлей/тост).
export async function fetchWithAuth(url, options = {}, onUnauthorized) {
    const headers = options.headers || {};
    headers['Authorization'] = 'Bearer ' + adminToken;
    const res = await fetch(url, { ...options, headers });
    if (res.status === 401) {
        clearToken();
        if (onUnauthorized) onUnauthorized();
        throw new Error('Unauthorized');
    }
    return res;
}