// web/static/js/admin/tabs.js
import { loadPlanetStats } from './stats.js';
import { runAudit } from './audit.js';
import { bindTestsButtons } from './tests.js';

let auditBound = false;

export function initTabs() {
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', function () {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            this.classList.add('active');

            const tabId = this.dataset.tab;
            document.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('active'));
            const pane = document.getElementById(tabId);
            if (pane) pane.classList.add('active');

            // Действия при активации вкладки
            if (tabId === 'tab-stats') {
                loadPlanetStats();
            }
            if (tabId === 'tab-audit') {
                bindAuditButton();
            }
            if (tabId === 'tab-tests') {
                bindTestsButtons();
            }
        });
    });
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