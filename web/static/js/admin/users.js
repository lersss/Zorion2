// web/static/js/admin/users.js
// Раздел «Пользователи» (спека 99.2.14 §6, §8): список с поиском/фильтром/
// пагинацией, профиль, создание, смена роли, сброс пароля, удаление.
// Доступен только skycomposer — вкладку скрывает main.js по роли из /me.
import { fetchWithAuth } from './auth.js';
import { notifyError, notifySuccess } from '../ui/toast.js';
import { gameDate } from '../game_date.js';

let currentPage = 1;
let profileId = null;

// esc — экранирование пользовательских данных перед вставкой в innerHTML.
function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
}

// ==================== СПИСОК ====================

export async function loadUsers(page) {
    if (page) currentPage = page;
    const query = document.getElementById('usersSearchInput').value.trim();
    const role = document.getElementById('usersRoleFilter').value;
    const tbody = document.getElementById('usersBody');
    if (!tbody) return;

    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#94a3b8;">Загрузка...</td></tr>';
    try {
        const url = `/admin/users?query=${encodeURIComponent(query)}&role=${encodeURIComponent(role)}&page=${currentPage}&limit=20`;
        const res = await fetchWithAuth(url);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();

        tbody.innerHTML = '';
        const users = Array.isArray(data.users) ? data.users : [];
        if (users.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#94a3b8;">Нет пользователей</td></tr>';
        } else {
            users.forEach(u => {
                const row = document.createElement('tr');
                row.innerHTML = `
                    <td>${esc(u.id)}</td>
                    <td>${esc(u.username)}</td>
                    <td>${u.email ? esc(u.email) : '—'}</td>
                    <td>${esc(u.role)}</td>
                    <td>${gameDate(u.created_at)}</td>
                    <td>${gameDate(u.updated_at)}</td>
                    <td><button class="btn-small">Профиль</button></td>`;
                row.querySelector('button').addEventListener('click', () => openUserProfile(u.id));
                tbody.appendChild(row);
            });
        }

        const pagination = document.getElementById('usersPagination');
        pagination.innerHTML = '';
        const total = data.total || 0;
        if (total > 0) {
            const totalPages = Math.max(1, Math.ceil(total / data.limit));
            const prev = document.createElement('button');
            prev.textContent = '◀';
            prev.addEventListener('click', () => loadUsers(Math.max(1, currentPage - 1)));
            pagination.appendChild(prev);

            const span = document.createElement('span');
            span.textContent = `Страница ${currentPage} из ${totalPages}`;
            pagination.appendChild(span);

            const next = document.createElement('button');
            next.textContent = '▶';
            next.addEventListener('click', () => loadUsers(Math.min(totalPages, currentPage + 1)));
            pagination.appendChild(next);
        }
    } catch (e) {
        console.error(e);
        tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#f87171;">Ошибка загрузки</td></tr>';
    }
}

// ==================== СОЗДАНИЕ ====================

export function toggleCreateUserForm() {
    const card = document.getElementById('createUserCard');
    const visible = card.style.display !== 'none';
    card.style.display = visible ? 'none' : 'block';
    if (!visible) {
        document.getElementById('newUserUsername').value = '';
        document.getElementById('newUserPassword').value = '';
        document.getElementById('newUserEmail').value = '';
        document.getElementById('newUserRole').value = 'player';
        document.getElementById('createUserResult').textContent = '';
    }
}

export async function createUser() {
    const username = document.getElementById('newUserUsername').value.trim();
    const password = document.getElementById('newUserPassword').value;
    const email = document.getElementById('newUserEmail').value.trim();
    const role = document.getElementById('newUserRole').value;
    const resultEl = document.getElementById('createUserResult');

    if (!username || !password) {
        resultEl.textContent = '❌ Логин и пароль обязательны';
        return;
    }
    try {
        const res = await fetchWithAuth('/admin/users', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password, email, role })
        });
        if (res.ok) {
            resultEl.textContent = '✅ Пользователь создан';
            toggleCreateUserForm();
            notifySuccess('Пользователь создан');
            loadUsers(1);
        } else {
            const err = await res.json().catch(() => ({}));
            resultEl.textContent = '❌ ' + (err.error || 'Ошибка сервера');
        }
    } catch (e) {
        resultEl.textContent = '❌ ' + e.message;
    }
}

// ==================== ПРОФИЛЬ ====================

export async function openUserProfile(id) {
    profileId = id;
    const card = document.getElementById('userProfileCard');
    card.style.display = 'block';
    card.innerHTML = '<p style="color:#94a3b8;">Загрузка профиля...</p>';
    try {
        const res = await fetchWithAuth('/admin/users/' + encodeURIComponent(id));
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const u = await res.json();
        card.innerHTML = `
            <h2>👤 ${esc(u.username)}</h2>
            <div style="display:grid; grid-template-columns:1fr 1fr; gap:6px 20px; margin:12px 0; font-size:0.9rem;">
                <div><span style="color:#94a3b8;">ID:</span> ${esc(u.id)}</div>
                <div><span style="color:#94a3b8;">Роль:</span> ${esc(u.role)}</div>
                <div><span style="color:#94a3b8;">Email:</span> ${u.email ? esc(u.email) : '—'}</div>
                <div><span style="color:#94a3b8;">Иконка корабля:</span> ${esc(u.ship_icon)}</div>
                <div><span style="color:#94a3b8;">Текущий мир:</span> ${u.current_world_name ? esc(u.current_world_name) : '—'}</div>
                <div><span style="color:#94a3b8;">agent_id:</span> ${u.agent_id ? esc(u.agent_id) : '—'}</div>
                <div><span style="color:#94a3b8;">Создан:</span> ${gameDate(u.created_at)}</div>
                <div><span style="color:#94a3b8;">Обновлён:</span> ${gameDate(u.updated_at)}</div>
            </div>
            <div style="display:flex; flex-wrap:wrap; gap:10px; align-items:center; border-top:1px solid #334155; padding-top:12px;">
                <label style="color:#94a3b8; font-size:0.85rem;">Роль:</label>
                <select id="profileRoleSelect">
                    <option value="player" ${u.role === 'player' ? 'selected' : ''}>player</option>
                    <option value="admin" ${u.role === 'admin' ? 'selected' : ''}>admin</option>
                    <option value="skycomposer" ${u.role === 'skycomposer' ? 'selected' : ''}>skycomposer</option>
                </select>
                <button class="btn-small" onclick="changeUserRole()">Сменить роль</button>
                <input type="password" id="profileNewPassword" placeholder="Новый пароль" style="width:150px;">
                <button class="btn-small" onclick="resetUserPassword()">Сбросить пароль</button>
                <button class="btn-small" onclick="deleteUser()">Удалить</button>
            </div>
            <div id="profileResult" class="result"></div>`;
    } catch (e) {
        console.error(e);
        card.innerHTML = '<p style="color:#f87171;">Ошибка загрузки профиля</p>';
    }
}

export async function changeUserRole() {
    if (!profileId) return;
    const role = document.getElementById('profileRoleSelect').value;
    const resultEl = document.getElementById('profileResult');
    try {
        const res = await fetchWithAuth(`/admin/users/${profileId}/role`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ role })
        });
        if (res.ok) {
            notifySuccess('Роль обновлена');
            openUserProfile(profileId);
            loadUsers(currentPage);
        } else {
            const err = await res.json().catch(() => ({}));
            resultEl.textContent = '❌ ' + (err.error || 'Ошибка сервера');
        }
    } catch (e) {
        resultEl.textContent = '❌ ' + e.message;
    }
}

export async function resetUserPassword() {
    if (!profileId) return;
    const password = document.getElementById('profileNewPassword').value;
    const resultEl = document.getElementById('profileResult');
    if (!password) {
        resultEl.textContent = '❌ Введите новый пароль';
        return;
    }
    try {
        const res = await fetchWithAuth(`/admin/users/${profileId}/reset-password`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password })
        });
        if (res.ok) {
            resultEl.textContent = '✅ Пароль сброшен';
            document.getElementById('profileNewPassword').value = '';
            notifySuccess('Пароль сброшен');
        } else {
            const err = await res.json().catch(() => ({}));
            resultEl.textContent = '❌ ' + (err.error || 'Ошибка сервера');
        }
    } catch (e) {
        resultEl.textContent = '❌ ' + e.message;
    }
}

export async function deleteUser() {
    if (!profileId) return;
    if (!confirm('Удалить пользователя? Действие необратимо.')) return;
    const resultEl = document.getElementById('profileResult');
    try {
        const res = await fetchWithAuth(`/admin/users/${profileId}`, { method: 'DELETE' });
        if (res.ok || res.status === 204) {
            notifySuccess('Пользователь удалён');
            document.getElementById('userProfileCard').style.display = 'none';
            profileId = null;
            loadUsers(currentPage);
        } else {
            const err = await res.json().catch(() => ({}));
            resultEl.textContent = '❌ ' + (err.error || 'Ошибка сервера');
        }
    } catch (e) {
        resultEl.textContent = '❌ ' + e.message;
    }
}