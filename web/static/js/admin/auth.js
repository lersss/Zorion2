// web/static/js/admin/auth.js
// Авторизация админки по ролевому JWT (спека 99.2.14 §3–4).
// X-Admin-Password снят; вход — обычный POST /login; токен хранится
// в localStorage['adminToken'] (не конфликтует с игровым 'token').
import { notifyError, notifySuccess } from '../ui/toast.js';

const TOKEN_KEY = 'adminToken';
const ADMIN_ROLES = ['admin', 'skycomposer'];

let adminToken = localStorage.getItem(TOKEN_KEY) || '';
let adminRole = null;
let afterLoginCallback = null;

export function setAfterLogin(fn) { afterLoginCallback = fn; }
export function getAdminRole() { return adminRole; }
export function getAdminToken() { return adminToken; }

// ensureAdminAuth — проверка токена и роли на старте. Нет валидного токена
// или роль player — показываем оверлей логина и возвращаем false.
// Сервер всё равно проверяет роль на каждой /admin/* ручке — это UX, не защита.
export async function ensureAdminAuth() {
    if (!adminToken) {
        showLoginOverlay();
        return false;
    }
    try {
        const res = await fetch('/me', { headers: { Authorization: 'Bearer ' + adminToken } });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const me = await res.json();
        if (!ADMIN_ROLES.includes(me.role)) {
            throw new Error('no admin role');
        }
        adminRole = me.role;
        hideLoginOverlay();
        return true;
    } catch (e) {
        adminToken = '';
        localStorage.removeItem(TOKEN_KEY);
        showLoginOverlay();
        return false;
    }
}

// adminLogin — вход по обычному логину/паролю.
export async function adminLogin() {
    const username = document.getElementById('adminLoginUsername').value.trim();
    const password = document.getElementById('adminLoginPassword').value;
    const errEl = document.getElementById('adminLoginError');
    if (!username || !password) {
        errEl.textContent = 'Введите логин и пароль';
        return;
    }
    errEl.textContent = '';
    try {
        const res = await fetch('/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        });
        if (!res.ok) {
            errEl.textContent = 'Неверный логин или пароль';
            return;
        }
        const data = await res.json();
        if (!ADMIN_ROLES.includes(data.user.role)) {
            errEl.textContent = 'Недостаточно прав: нужна роль администратора';
            return;
        }
        adminToken = data.token;
        adminRole = data.user.role;
        localStorage.setItem(TOKEN_KEY, adminToken);
        hideLoginOverlay();
        notifySuccess('Добро пожаловать, ' + data.user.username);
        if (afterLoginCallback) afterLoginCallback();
    } catch (e) {
        errEl.textContent = 'Ошибка соединения с сервером';
    }
}

// adminLogout — выход из админки (токен только админский, игровой не трогаем).
export function adminLogout() {
    adminToken = '';
    adminRole = null;
    localStorage.removeItem(TOKEN_KEY);
    showLoginOverlay();
}

// fetchWithAuth — единый способ авторизации в админке.
// На 401 (протух/невалиден токен) — снова показать логин.
export async function fetchWithAuth(url, options = {}) {
    const headers = options.headers || {};
    headers['Authorization'] = 'Bearer ' + adminToken;
    const res = await fetch(url, { ...options, headers });
    if (res.status === 401) {
        adminToken = '';
        localStorage.removeItem(TOKEN_KEY);
        showLoginOverlay();
        notifyError('Сессия истекла — войдите снова');
        throw new Error('Unauthorized');
    }
    return res;
}

function showLoginOverlay() {
    const el = document.getElementById('adminLoginOverlay');
    if (el) el.style.display = 'flex';
}

function hideLoginOverlay() {
    const el = document.getElementById('adminLoginOverlay');
    if (el) el.style.display = 'none';
}