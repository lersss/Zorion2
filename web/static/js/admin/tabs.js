// web/static/js/admin/tabs.js
import { loadPlanetStats } from './stats.js';
import { runAudit } from './audit.js';
import { bindTestsButtons } from './tests.js';
import { loadUsers } from './users.js';
import { initNPC } from './npc.js';
import { initShips } from './ships.js';

// Активная вкладка сохраняется между обновлениями страницы.
const STORAGE_KEY = 'adminActiveTab';

let auditBound = false;
let usersBound = false;

export function initTabs() {
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', function () {
            activateTab(this.dataset.tab);
        });
    });

    const saved = localStorage.getItem(STORAGE_KEY);
    activateTab(saved || 'tab-main');
}

// activateTab — переключение вкладки + ленивая инициализация + сохранение.
function activateTab(tabId) {
    if (!document.getElementById(tabId)) {
        tabId = 'tab-main';
    }
    document.querySelectorAll('.tab-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.tab === tabId));
    document.querySelectorAll('.tab-pane').forEach(p =>
        p.classList.toggle('active', p.id === tabId));
    localStorage.setItem(STORAGE_KEY, tabId);

    if (tabId === 'tab-stats') {
        loadPlanetStats();
    }
    if (tabId === 'tab-audit') {
        bindAuditButton();
    }
    if (tabId === 'tab-tests') {
        bindTestsButtons();
    }
    if (tabId === 'tab-users') {
        bindUsersTab();
    }
    if (tabId === 'tab-npc') {
        initNPC();
    }
    if (tabId === 'tab-ships') {
        initShips();
    }
}

// bindUsersTab — ленивая загрузка списка пользователей (вкладка «Пользователи»).
function bindUsersTab() {
    if (usersBound) return;
    loadUsers(1);
    usersBound = true;
}

// bindAuditButton — вешает обработчик на кнопку "Запустить аудит".
// Защищено флагом auditBound, чтобы не навесить несколько раз.
function bindAuditButton() {
    if (auditBound) return;
    const btn = document.getElementById('runAuditBtn');
    if (!btn) return;
    btn.addEventListener('click', () => {
        runAudit();
    });
    auditBound = true;
}