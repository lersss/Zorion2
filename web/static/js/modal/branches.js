// web/static/js/modal/branches.js
// Ветки поселения в карточке планеты (спека 2026-09-22-поселение-ветка-
// буферы-переработка §6): блок на каждую ветку — имя рецепта/выхода, сложность,
// выход (товар → количество) виден всем, кто видит детали поселения; вход —
// только админ. Плюс админ-формы «создать ветку» и «добавить во вход».
//
// Чистые функции без DOM/сети на верхнем уровне: модуль тянется в import-граф
// админки (modal/tabs.js) и исполняется в Node (web/frontend_branches_test.go).

// formatAmount — количество: до 2 знаков, без хвостовых нулей (дробные батчи
// §4.2). Нечисло → '—'.
function formatAmount(n) {
    if (typeof n !== 'number' || isNaN(n)) return '—';
    return String(Math.round(n * 100) / 100);
}

// bufferRowHtml — строка буфера «имя → количество».
function bufferRowHtml(entry) {
    const name = entry && entry.good_name ? entry.good_name : '#' + (entry && entry.good_id);
    return `<div style="color:#ccc;">${name}: <strong>${formatAmount(entry && entry.amount)}</strong></div>`;
}

// branchBlockHtml — блок одной ветки. Input показывается только админу
// (isAdmin): входной буфер видит только админ (§6). Выход — «пол» по качеству
// (поле качества не заводим, §1) — отдельной пометки в блоке нет.
export function branchBlockHtml(branch, isAdmin) {
    if (!branch) return '';
    const title = branch.recipe_name || ('рецепт #' + branch.recipe_id);
    const complexity = branch.complexity ? ` · сложность ${branch.complexity}` : '';
    let html = `
        <div style="margin:8px 0; padding:10px; background:#14142a; border-radius:4px;">
            <div><strong>${title}</strong><span style="color:#888;">${complexity}</span></div>
            <div style="color:#888; font-size:0.85rem; margin-top:6px; text-transform:uppercase;">Выход</div>`;
    const output = branch.output || [];
    if (output.length === 0) {
        html += `<div style="color:#666;">— пусто —</div>`;
    } else {
        output.forEach(e => { html += bufferRowHtml(e); });
    }
    if (isAdmin) {
        html += `<div style="color:#facc15; font-size:0.85rem; margin-top:6px; text-transform:uppercase;">Вход (виден только админу)</div>`;
        const input = branch.input || [];
        if (input.length === 0) {
            html += `<div style="color:#666;">— пусто —</div>`;
        } else {
            input.forEach(e => { html += bufferRowHtml(e); });
        }
    }
    html += `</div>`;
    return html;
}

// branchesBlockHtml — блок всех веток поселения (плюс админ-формы).
export function branchesBlockHtml(branches, isAdmin, settlementID) {
    const list = Array.isArray(branches) ? branches : [];
    if (list.length === 0 && !isAdmin) return '';
    let html = `<div style="color:#888; font-size:0.9rem; text-transform:uppercase; margin-top:8px;">Ветки (${list.length})</div>`;
    if (list.length === 0) {
        html += `<div style="color:#666; font-size:0.85rem;">Веток нет</div>`;
    }
    list.forEach(b => { html += branchBlockHtml(b, isAdmin); });
    if (isAdmin) {
        html += adminCreateBranchFormHtml(settlementID);
        list.forEach(b => { html += adminBranchInputFormHtml(b.id); });
    }
    return html;
}

// adminCreateBranchFormHtml — форма «создать ветку» (§6): рецепт вводится
// recipe_id «руками» (О2); подсказка про пилот «Пища» (рецепт 69).
export function adminCreateBranchFormHtml(settlementID) {
    return `
        <div data-branch-create-form="${settlementID || ''}" style="margin-top:8px; padding-top:8px; border-top:1px solid #2a2a4a;">
            <div style="color:#facc15; font-size:0.85rem;">⚙ админ — создать ветку (рецепт id; пилот «Пища» = 69)</div>
            <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-top:6px;">
                <input data-branch-recipe type="number" min="1" placeholder="recipe_id"
                    style="width:120px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <button data-branch-create style="background:#2a2a4a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; cursor:pointer;">Создать ветку</button>
            </div>
        </div>`;
}

// adminBranchInputFormHtml — форма «добавить во вход» для конкретной ветки (§6):
// выбор ресурса из /studio/api/resources + количество.
export function adminBranchInputFormHtml(branchID) {
    return `
        <div data-branch-input-form="${branchID}" style="margin-top:6px; display:flex; flex-wrap:wrap; gap:6px; align-items:center;">
            <select data-branch-input-good style="background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <option value="">Загрузка…</option>
            </select>
            <input data-branch-input-amount type="number" step="1" min="1" placeholder="количество"
                style="width:110px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
            <button data-branch-input-add style="background:#2a2a4a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; cursor:pointer;">Добавить во вход</button>
        </div>`;
}
