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
            <div style="color:#888; font-size:0.85rem; margin-top:6px; text-transform:uppercase;">Выход · остаток (осадок)</div>`;
    const output = branch.output || [];
    if (output.length === 0) {
        html += `<div style="color:#666;">— пусто —</div>`;
    } else {
        output.forEach(e => { html += bufferRowHtml(e); });
    }
    // Петля потребления (итерация 4 §6): «сделано» и «съедено» за ПОСЛЕДНИЙ
    // проход (не накопительный счётчик) + текущая скорость поедания, ед/сек.
    // Значения — скаляры; 0 приходит отсутствием поля (omitempty), поэтому
    // строку «за проход» показываем всегда.
    html += `<div style="color:#888; font-size:0.85rem; margin-top:6px;">за проход: сделано <strong>${formatAmount(branch.produced || 0)}</strong> · съедено <strong>${formatAmount(branch.eaten || 0)}</strong></div>`;
    html += `<div style="color:#888; font-size:0.85rem;">скорость поедания: <strong>${formatAmount(branch.eaten_rate || 0)}</strong> ед/сек</div>`;
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

// ---------- ЭФФЕКТЫ ПОСЕЛЕНИЯ (спека 2026-09-22-эффекты-снабжения §6) ----------

// formatForce — сила эффекта R (доля/сек): малые значения без потери смысла
// (обычный formatAmount округлил бы 1e-7 до «0»).
function formatForce(n) {
    if (typeof n !== 'number' || isNaN(n)) return '—';
    if (n === 0) return '0';
    if (Math.abs(n) >= 0.01) return String(Math.round(n * 100) / 100);
    return n.toExponential(2);
}

// effectRowHtml — строка одного действующего эффекта (витрина админа, §6):
// тип/источник (позиция корзины), нагрузка, порог, состояние (включён/снят —
// производное load ≥ порог), текущая сила R и сила условия w.
function effectRowHtml(e) {
    const type = e && e.effect_type_id != null ? ('тип #' + e.effect_type_id) : 'эффект';
    const curve = (e && e.curve) ? (' (' + e.curve + ')') : '';
    const src = (e && e.source_position) ? e.source_position : '—';
    const state = (e && e.enabled) ? 'включён' : 'снят';
    return `
        <div style="margin:6px 0; padding:8px; background:#14142a; border-radius:4px;">
            <div><strong>${type}</strong>${curve} · источник: <strong>${src}</strong></div>
            <div style="color:#ccc; font-size:0.9rem;">нагрузка: <strong>${formatAmount(e && e.load)}</strong> · порог: <strong>${formatAmount(e && e.threshold)}</strong> · состояние: <strong>${state}</strong></div>
            <div style="color:#888; font-size:0.85rem;">сила: <strong>${formatForce(e && e.rate)}</strong> · w: <strong>${formatAmount(e && e.w)}</strong></div>
        </div>`;
}

// adminEffectLoadFormHtml — админ-форма «задать нагрузку вручную» (§6, F9):
// effect_type_id + load → POST /admin/settlements/{id}/effects.
export function adminEffectLoadFormHtml(settlementID) {
    return `
        <div data-effect-load-form="${settlementID || ''}" style="margin-top:8px; padding-top:8px; border-top:1px solid #2a2a4a;">
            <div style="color:#facc15; font-size:0.85rem;">⚙ админ — задать нагрузку вручную (сило-ч, песочница)</div>
            <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-top:6px;">
                <input data-effect-load-type type="number" min="1" placeholder="effect_type_id"
                    style="width:130px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <input data-effect-load-value type="number" step="any" min="0" placeholder="нагрузка"
                    style="width:120px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <button data-effect-load-set style="background:#2a2a4a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; cursor:pointer;">Задать нагрузку</button>
            </div>
        </div>`;
}

// effectsBlockHtml — блок действующих эффектов поселения (§6): виден ТОЛЬКО
// админу (игроку детали поселения не отдаются, stripPlanetDetails). На эффект:
// тип/источник, нагрузка, порог, состояние, сила/w; внизу — админ-форма
// ручного задания нагрузки.
export function effectsBlockHtml(effects, isAdmin, settlementID) {
    if (!isAdmin) return '';
    const list = Array.isArray(effects) ? effects : [];
    let html = `<div style="color:#888; font-size:0.9rem; text-transform:uppercase; margin-top:8px;">Эффекты (${list.length})</div>`;
    if (list.length === 0) {
        html += `<div style="color:#666; font-size:0.85rem;">Эффектов нет</div>`;
    }
    list.forEach(e => { html += effectRowHtml(e); });
    html += adminEffectLoadFormHtml(settlementID);
    return html;
}
