// web/static/js/admin/tests.js
//
// Автоматические тесты: /admin/tests — запуск и показ результатов.
// Результат рисуется человеческим языком, а не сырым выводом go test.

import { fetchWithAuth } from './auth.js';
import { showLoader } from '../loader.js';

const API_URL = '/admin/tests';

// ---------- ПУБЛИЧНЫЙ API ----------

// runTests(refresh) — показать последние (false) или запустить заново (true).
export async function runTests(refresh) {
    const container = document.getElementById('tests-content');
    if (!container) {
        console.error('tests: контейнер #tests-content не найден');
        return;
    }

    let stop = null;
    if (refresh) {
        stop = showLoader(container, 'Запускаем go test — это может занять минуту...');
    }
    if (!refresh && container.dataset.loaded) {
        return;
    }

    try {
        const url = refresh ? `${API_URL}?refresh=1` : API_URL;
        const res = await fetchWithAuth(url);
        if (!res.ok) {
            throw new Error(`HTTP ${res.status}: ${res.statusText}`);
        }
        const result = await res.json();
        if (stop) stop();
        container.dataset.loaded = '1';
        renderTests(container, result);
    } catch (err) {
        console.error('tests error:', err);
        if (stop) stop();
        showError(container, err.message);
    }
}

// ---------- РЕНДЕР ----------

function renderTests(container, result) {
    container.innerHTML = '';
    setTestsAvailable(result.available !== false);
    container.appendChild(buildSummary(result));
    container.appendChild(buildMessage(result));

    if (result.packages && result.packages.length > 0) {
        container.appendChild(buildPackagesTable(result));
    }
}

// setTestsAvailable — кнопки запуска бессмысленны там, где нет исходников
// (прод): при available=false отключаем их, чтобы не дёргать бесполезный прогон.
function setTestsAvailable(available) {
    ['runTestsBtn', 'lastTestsBtn'].forEach(id => {
        const btn = document.getElementById(id);
        if (btn) btn.disabled = !available;
    });
}

function buildSummary(result) {
    const div = document.createElement('div');
    div.style.cssText = `
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        gap: 12px;
        margin-bottom: 16px;
    `;

    const ok = result.status !== 'fail' && result.status !== 'error';
    const statusText = result.status === 'success' ? 'Все зелёные'
        : result.status === 'fail' ? 'Есть падения'
        : 'Ошибка запуска';

    const cells = [
        { label: 'Итог', value: statusText, color: ok ? '#4ade80' : '#ef4444' },
        { label: 'Пакетов', value: fmtPackages(result) },
        { label: 'Тестов', value: fmtTests(result) },
        {
            label: 'Время',
            value: result.duration_ms != null ? fmtDuration(result.duration_ms) : '—'
        },
    ];

    cells.forEach(c => {
        const cell = document.createElement('div');
        cell.style.cssText = `
            padding: 14px;
            background: #12121f;
            border-radius: 10px;
            text-align: center;
        `;
        cell.innerHTML = `
            <div style="font-size: 0.75rem; color: #888; margin-bottom: 4px;">${c.label}</div>
            <div style="font-size: 1.4rem; font-weight: bold; color: ${c.color || '#e0e0e0'};">${c.value}</div>
        `;
        div.appendChild(cell);
    });

    return div;
}

function buildMessage(result) {
    const wrapper = document.createElement('div');
    wrapper.style.cssText = 'margin-bottom: 16px; line-height: 1.5;';

    if (result.status === 'error') {
        wrapper.innerHTML = `
            <div style="padding:14px; background:#2a1212; border:1px solid #7f1d1d;
                        border-radius:10px; color:#fca5a5;">
                ⚠️ ${escapeHtml(result.message || 'Не удалось запустить тесты.')}
            </div>`;
        return wrapper;
    }

    if (result.status === 'success') {
        const skipped = result.tests_skipped > 0
            ? `, ${result.tests_skipped} пропущено` : '';
        const noTestsPkgs = result.packages_skip > 0
            ? `, пакетов без тестов: ${result.packages_skip}` : '';
        const msg = result.packages_total > 0
            ? `${pluralize(result.tests_total, 'тест', 'теста', 'тестов')} в `
                + `${pluralize(result.packages_ok, 'пакете', 'пакетах', 'пакетах')} прошли без падений `
                + `(за ${fmtDuration(result.duration_ms)})${noTestsPkgs}.`
            : 'Тестов нет — нечего показать.';
        wrapper.innerHTML = `
            <div style="padding:14px; background:#0f2a17; border:1px solid #14532d;
                        border-radius:10px; color:#86efac;">
                ✅ ${msg}${skipped}
            </div>`;
        return wrapper;
    }

    // fail
    const failed = `<b style="color:#fca5a5;">${result.tests_failed}</b>`;
    const pkgFail = result.packages_total - result.packages_ok - result.packages_skip;
    const parts = [`Упало тестов: ${failed}`];
    if (pkgFail > 0) {
        parts.push(`упало пакетов: <b style="color:#fca5a5;">${pkgFail}</b>`);
    }
    wrapper.innerHTML = `
        <div style="padding:14px; background:#2a1212; border:1px solid #7f1d1d;
                    border-radius:10px; color:#fecaca;">
            ❌ ${parts.join(', ')}. Подробности по пакетам ниже.
        </div>`;
    return wrapper;
}

function buildPackagesTable(result) {
    const wrapper = document.createElement('div');

    const title = document.createElement('h3');
    title.textContent = 'По пакетам';
    title.style.cssText = 'margin: 0 0 10px 0; font-size: 1rem; color: #aaa;';
    wrapper.appendChild(title);

    const table = document.createElement('table');
    table.style.cssText = `
        width: 100%;
        border-collapse: collapse;
        font-size: 0.85rem;
    `;
    table.innerHTML = `
        <thead>
            <tr>
                <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Пакет</th>
                <th style="text-align:center; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Статус</th>
                <th style="text-align:right; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Тесты</th>
                <th style="text-align:right; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Время</th>
            </tr>
        </thead>
        <tbody></tbody>
    `;
    const tbody = table.querySelector('tbody');

    result.packages.forEach(pkg => {
        const statusCell = pkg.status === 'ok'
            ? '<span style="color:#4ade80;">✓ зелёный</span>'
            : pkg.status === 'skip'
            ? '<span style="color:#94a3b8;">skip</span>'
            : '<span style="color:#ef4444;">✗ упал</span>';

        const failedText = pkg.failed > 0
            ? `${pkg.tests - pkg.failed}/${pkg.tests}`
            : String(pkg.tests);

        const meta = pkg.failed > 0
            ? `, из них упало <b style="color:#ef4444;">${pkg.failed}</b>`
            : '';

        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e;">
                <span title="${escapeHtml(pkg.path)}" style="color:#e2e8f0;">${escapeHtml(pkg.name)}</span>
            </td>
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; text-align:center;">${statusCell}</td>
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; text-align:right;">${failedText}${meta}</td>
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; text-align:right;">${fmtDuration(pkg.duration_ms)}</td>
        `;
        tbody.appendChild(tr);

        if (pkg.failures && pkg.failures.length > 0) {
            pkg.failures.forEach(f => {
                const ftr = document.createElement('tr');
                ftr.innerHTML = `
                    <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; padding-left:24px;
                               font-family: monospace; font-size:0.8rem; color:#e2e8f0;">↳ ${escapeHtml(f.test)}</td>
                    <td colspan="2" style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; color:#fca5a5; font-size:0.8rem;">
                        ${escapeHtml(f.message)}
                    </td>
                    <td></td>
                `;
                tbody.appendChild(ftr);
            });
        }
    });

    wrapper.appendChild(table);
    return wrapper;
}

// ---------- ПОМОЩНИКИ ----------

function fmtPackages(result) {
    return `${result.packages_ok} из ${result.packages_total}`;
}

function fmtTests(result) {
    if (result.tests_total === 0) return '—';
    return `${result.tests_total} тестов`;
}

function fmtDuration(ms) {
    if (ms == null || isNaN(ms)) return '—';
    if (ms < 1000) return `${Math.round(ms)} мс`;
    return `${(ms / 1000).toFixed(2)} с`;
}

// pluralize(5, 'тест', 'теста', 'тестов') → '5 тестов'
function pluralize(n, one, few, many) {
    const d = n % 10;
    const m = n % 100;
    if (d === 1 && m !== 11) return `${n} ${one}`;
    if (d >= 2 && d <= 4 && (m < 12 || m > 14)) return `${n} ${few}`;
    return `${n} ${many}`;
}

function escapeHtml(s) {
    return String(s == null ? '' : s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function showError(container, message) {
    container.innerHTML = `
        <div style="padding: 20px; color: #e74c3c;">
            ❌ Ошибка загрузки тестов: ${escapeHtml(message)}
        </div>`;
}

export function bindTestsButtons() {
    const runBtn = document.getElementById('runTestsBtn');
    if (runBtn && !runBtn.dataset.bound) {
        runBtn.addEventListener('click', () => runTests(true));
        runBtn.dataset.bound = '1';
    }
    const lastBtn = document.getElementById('lastTestsBtn');
    if (lastBtn && !lastBtn.dataset.bound) {
        lastBtn.addEventListener('click', () => runTests(false));
        lastBtn.dataset.bound = '1';
    }
}