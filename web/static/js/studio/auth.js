// web/static/js/studio/auth.js
// Адаптер авторизации студии (спека переноса-студии-товаров-iterC §8.3):
// заменяет инлайн-копию ~80 строк в studio.html; логика — в ядре ../auth.js.
// window.*-функции для инлайн-обработчиков studio.html (onclick/onkeydown);
// bootstrap — сам модуль (выполняется после парсинга документа — start/
// showReport из классического скрипта уже определены).
import {
    getToken, setToken, clearToken, isAdminRole,
    validateToken, login, fetchWithAuth
} from '../auth.js';

function showStudioLogin(msg) {
    document.getElementById("studioLoginError").textContent = msg || "";
    document.getElementById("studioLoginOverlay").style.display = "flex";
    document.getElementById("studioLoginPassword").value = "";
    setTimeout(() => document.getElementById("studioLoginUsername").focus(), 0);
}
function hideStudioLogin() { document.getElementById("studioLoginOverlay").style.display = "none"; }

// ensureStudioAuth — проверка токена и роли на старте (паттерн ensureAdminAuth):
// нет токена / 401 / роль player → оверлей логина. Сервер всё равно проверяет
// роль на каждой /studio/api/* ручке — это UX, не защита.
window.ensureStudioAuth = async function () {
    if (!getToken()) { showStudioLogin(); return false; }
    const v = await validateToken();
    if (!v.ok) {
        clearToken();
        showStudioLogin();
        return false;
    }
    if (!isAdminRole(v.role)) {
        showStudioLogin("Недостаточно прав: нужна роль администратора");
        return false;
    }
    hideStudioLogin();
    return true;
};

// studioLogin — вход из оверлея (паттерн adminLogin).
window.studioLogin = async function () {
    const username = document.getElementById("studioLoginUsername").value.trim();
    const password = document.getElementById("studioLoginPassword").value;
    if (!username || !password) { document.getElementById("studioLoginError").textContent = "Введите логин и пароль"; return; }
    document.getElementById("studioLoginError").textContent = "";
    const r = await login(username, password);
    if (!r.ok) { document.getElementById("studioLoginError").textContent = r.error; return; }
    setToken(r.token);
    hideStudioLogin();
    window.start();
};

// studioFetch — fetch с Bearer (паттерн fetchWithAuth); 401 — сброс токена,
// оверлей + тост «Сессия истекла».
window.studioFetch = function (url, options) {
    return fetchWithAuth(url, options, () => {
        showStudioLogin();
        window.showReport(["Сессия истекла — войдите снова"]);
    });
};

// bootstrap: авторизация → start() = fetchState + интервал (перенос строки
// studio.html:1653; модуль выполняется после парсинга документа).
window.ensureStudioAuth().then(ok => { if (ok) window.start(); });