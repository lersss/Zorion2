// web/static/js/admin/audit.js
//
// Аудит планет: запрос /admin/audit, рендер результата.
// Использует fetchWithAuth из auth.js — единый способ авторизации в админке.

import { fetchWithAuth } from './auth.js';
import { showLoader } from '../loader.js';

const API_URL = '/admin/audit';

// ---------- СОСТОЯНИЕ ----------
let lastResult = null;
let showAllSamples = false;

// ---------- ПУБЛИЧНЫЙ API ----------

// runAudit — запускает аудит и рендерит результат.
export async function runAudit() {
    const container = document.getElementById('audit-content');
    if (!container) {
        console.error('audit: контейнер #audit-content не найден');
        return;
    }

    const stop = showLoader(container, 'Проверяем планеты...');

    try {
        const res = await fetchWithAuth(API_URL);

        if (!res.ok) {
            throw new Error(`HTTP ${res.status}: ${res.statusText}`);
        }

        const result = await res.json();
        lastResult = result;
        showAllSamples = false;
        stop();
        renderAudit(container, result);
    } catch (err) {
        console.error('audit error:', err);
        stop(`
            <div style="padding: 20px; color: #e74c3c;">
                ❌ Ошибка аудита: ${err.message}
            </div>
        `);
    }
}

// ---------- РЕНДЕР ----------

function renderAudit(container, result) {
    container.innerHTML = '';

    // Сводка
    container.appendChild(buildSummary(result));

    // Если аномалий нет — показываем зелёное сообщение и выходим
    if (result.total_issues === 0) {
        const noIssues = document.createElement('div');
        noIssues.style.cssText = `
            text-align: center;
            padding: 40px;
            color: #4ade80;
            font-size: 1.1rem;
        `;
        noIssues.textContent = '✅ Противоречий не найдено';
        container.appendChild(noIssues);
        return;
    }

    // Таблица кодов проблем
    container.appendChild(buildCodesTable(result));

    // Кнопка "показать все примеры" (если примеров > 50)
    if (result.sample_issues && result.sample_issues.length > 50) {
        const toggleBtn = document.createElement('button');
        toggleBtn.textContent = showAllSamples
            ? '▲ Показать первые 50'
            : `▼ Показать все (${result.sample_issues.length})`;
        toggleBtn.style.cssText = `
            margin: 12px 0;
            padding: 6px 14px;
            background: #2a2a4a;
            color: #e0e0e0;
            border: none;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.85rem;
        `;
        toggleBtn.addEventListener('click', () => {
            showAllSamples = !showAllSamples;
            renderAudit(container, lastResult);
        });
        container.appendChild(toggleBtn);
    }

    // Таблица примеров
    container.appendChild(buildSamplesTable(result));
}

// ---------- СВОДКА ----------

function buildSummary(result) {
    const div = document.createElement('div');
    div.style.cssText = `
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        gap: 12px;
        margin-bottom: 20px;
    `;

    const cells = [
        { label: 'Всего планет', value: result.total_entities },
        {
            label: 'С проблемами',
            value: result.entities_with_issues,
            color: result.entities_with_issues > 0 ? '#f59e0b' : '#4ade80'
        },
        {
            label: 'Всего проблем',
            value: result.total_issues,
            color: result.total_issues > 0 ? '#ef4444' : '#4ade80'
        },
        { label: 'Время', value: result.duration_ms + ' мс' },
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

// ---------- ТАБЛИЦА КОДОВ ----------

function buildCodesTable(result) {
    const wrapper = document.createElement('div');
    wrapper.style.cssText = 'margin-bottom: 24px;';

    if (!result.issues_by_code || Object.keys(result.issues_by_code).length === 0) {
        return wrapper;
    }

    const title = document.createElement('h3');
    title.textContent = 'По типам проблем';
    title.style.cssText = 'margin: 0 0 10px 0; font-size: 1rem; color: #aaa;';
    wrapper.appendChild(title);

    const entries = Object.entries(result.issues_by_code)
        .sort((a, b) => b[1] - a[1]);

    const table = document.createElement('table');
    table.style.cssText = `
        width: 100%;
        border-collapse: collapse;
        font-size: 0.85rem;
    `;
    table.innerHTML = `
        <thead>
            <tr>
                <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Код</th>
                <th style="text-align:right; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Кол-во</th>
                <th style="text-align:right; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Уровень</th>
            </tr>
        </thead>
        <tbody></tbody>
    `;

    const tbody = table.querySelector('tbody');
    entries.forEach(([code, count]) => {
        const severity = guessSeverity(code, result);
        const color = severity === 'high' ? '#ef4444'
            : severity === 'medium' ? '#f59e0b'
            : '#94a3b8';
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; font-family: monospace; color: #cbd5e1;">${code}</td>
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; text-align:right; font-weight: bold;">${count}</td>
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; text-align:right; color:${color};">${severity}</td>
        `;
        tbody.appendChild(tr);
    });

    wrapper.appendChild(table);
    return wrapper;
}

// guessSeverity — уровень из Sample Issues (там есть), иначе эвристика.
function guessSeverity(code, result) {
    if (result.sample_issues) {
        const sample = result.sample_issues.find(i => i.code === code);
        if (sample) return sample.severity;
    }
    if (code.includes('nan') || code.includes('negative') || code.includes('zero')
        || code.includes('sum_not_100') || code.includes('flag_mismatch')) {
        return 'high';
    }
    if (code.includes('without') || code.includes('in_cold') || code.includes('in_heat')) {
        return 'high';
    }
    return 'medium';
}

// ---------- ТАБЛИЦА ПРИМЕРОВ ----------

function buildSamplesTable(result) {
    const wrapper = document.createElement('div');

    if (!result.sample_issues || result.sample_issues.length === 0) {
        return wrapper;
    }

    const title = document.createElement('h3');
    title.textContent = 'Примеры проблем';
    title.style.cssText = 'margin: 0 0 10px 0; font-size: 1rem; color: #aaa;';
    wrapper.appendChild(title);

    if (result.truncated) {
        const note = document.createElement('div');
        note.textContent = `⚠️ Показано ${result.sample_issues.length} из ${result.total_issues}`;
        note.style.cssText = 'font-size: 0.75rem; color: #f59e0b; margin-bottom: 8px;';
        wrapper.appendChild(note);
    }

    const limit = showAllSamples ? result.sample_issues.length : 50;
    const samples = result.sample_issues.slice(0, limit);

    const table = document.createElement('table');
    table.style.cssText = `
        width: 100%;
        border-collapse: collapse;
        font-size: 0.8rem;
    `;
    table.innerHTML = `
        <thead>
            <tr>
                <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Планета</th>
                <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Проблема</th>
                <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding: 6px 4px;">Описание</th>
            </tr>
        </thead>
        <tbody></tbody>
    `;

    const tbody = table.querySelector('tbody');
    samples.forEach(issue => {
        const color = issue.severity === 'high' ? '#ef4444'
            : issue.severity === 'medium' ? '#f59e0b'
            : '#94a3b8';
        const name = issue.entity_name || issue.entity_id;
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e;">${name}</td>
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; font-family: monospace; color:${color};">${issue.code}</td>
            <td style="padding: 5px 4px; border-bottom:1px solid #1a1a2e; color: #cbd5e1;">${issue.description}</td>
        `;
        tbody.appendChild(tr);
    });

    wrapper.appendChild(table);
    return wrapper;
}