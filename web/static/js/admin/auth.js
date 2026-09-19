// web/static/js/admin/auth.js
// Тонкий адаптер авторизации админки (спека переноса-студии-товаров-iterC
// §8.2): те же экспорты и поведение, логика — в ядре ../auth.js. 13 файлов
// админки импортируют ./auth.js (12 — fetchWithAuth/getAdminToken, main.js —
// ensureAdminAuth и др.) — контракт не меняется.
// X-Admin-Password снят; вход — обычный POST /login; токен хранится
// в localStorage['adminToken'] (не конфликтует с игровым 'token').
import { notifyError, notifySuccess } from '../ui/toast.js';
import {
    getToken, setToken, clearToken, isAdminRole,
    validateToken, login, fetchWithAuth as coreFetchWithAuth
} from '../auth.js';

let adminRole = null;
let afterLoginCallback = null;

export function setAfterLogin(fn) { afterLoginCallback = fn; }
export function getAdminRole() { return adminRole; }
export function getAdminToken() { return getToken(); }

// ensureAdminAuth — проверка токена и роли на старте. Нет валидного токена
// или роль player — показываем оверлей логина и возвращаем false.
// Сервер всё равно проверяет роль на каждой /admin/* ручке — это UX, не защита.
export async function ensureAdminAuth() {
    if (!getToken()) {
        showLoginOverlay();
        return false;
    }
    const v = await validateToken();
    if (!v.ok || !isAdminRole(v.role)) {
        clearToken();
        showLoginOverlay();
        return false;
    }
    adminRole = v.role;
    hideLoginOverlay();
    return true;
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
    const r = await login(username, password);
    if (!r.ok) {
        errEl.textContent = r.error;
        return;
    }
    setToken(r.token);
    adminRole = r.role;
    hideLoginOverlay();
    notifySuccess('Добро пожаловать, ' + r.username);
    if (afterLoginCallback) afterLoginCallback();
}

// adminLogout — выход из админки (токен только админский, игровой не трогаем).
export function adminLogout() {
    clearToken();
    adminRole = null;
    showLoginOverlay();
}

// fetchWithAuth — единый способ авторизации в админке.
// На 401 (протух/невалиден токен) — снова показать логин.
export async function fetchWithAuth(url, options = {}) {
    return coreFetchWithAuth(url, options, () => {
        showLoginOverlay();
        notifyError('Сессия истекла — войдите снова');
    });
}

function showLoginOverlay() {
    const el = document.getElementById('adminLoginOverlay');
    if (el) el.style.display = 'flex';
}

function hideLoginOverlay() {
    const el = document.getElementById('adminLoginOverlay');
    if (el) el.style.display = 'none';
}