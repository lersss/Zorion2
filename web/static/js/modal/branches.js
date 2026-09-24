// web/static/js/modal/branches.js
// Ветки поселения в карточке планеты (спека 2026-09-22-поселение-ветка-
// буферы-переработка §6): блок на каждую ветку — имя рецепта/выхода, сложность,
// выход (товар → количество) виден всем, кто видит детали поселения; вход —
// только админ. Плюс админ-формы «создать ветку» и «добавить во вход».
//
// Чистые функции без DOM/сети на верхнем уровне: модуль тянется в import-граф
// админки (modal/tabs.js) и исполняется в Node (web/frontend_branches_test.go).

import { readUnitScale, storedToDisplay, unitScaleDef } from '../unit_scale.js';

// formatAmount — количество: до 2 знаков, без хвостовых нулей (дробные батчи
// §4.2). Нечисло → '—'.
function formatAmount(n) {
    if (typeof n !== 'number' || isNaN(n)) return '—';
    return String(Math.round(n * 100) / 100);
}

// trimZeros — снимает хвостовые нули дробной части ('0.250' → '0.25').
function trimZeros(s) {
    return s.indexOf('.') >= 0 ? s.replace(/\.?0+$/, '') : s;
}

// groupThousands — разделитель разрядов пробелом ('1300' → '1 300').
function groupThousands(s) {
    const parts = s.split('.');
    parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
    return parts.join('.');
}

// formatCount — абсолютное число (ед/сутки или люди): разряды через пробел и
// разумное число знаков (крупные — целые/1 знак, малые не теряются). Нечисло → '—'.
function formatCount(n) {
    if (typeof n !== 'number' || !isFinite(n)) return '—';
    if (n === 0) return '0';
    const sign = n < 0 ? '-' : '';
    const abs = Math.abs(n);
    let body;
    if (abs >= 100) {
        body = trimZeros(abs.toFixed(0));
    } else if (abs >= 1) {
        body = trimZeros(abs.toFixed(1));
    } else {
        // < 1: держим значащие цифры, чтобы не потерять порядок (0.00000065).
        const k = Math.min(12, Math.max(2, -Math.floor(Math.log10(abs)) + 2));
        body = trimZeros(abs.toFixed(k));
    }
    if (body === '' || Number(body) === 0) return n.toExponential(2);
    return sign + groupThousands(body);
}

// formatPercent — доля 0..1 в проценты с разумным округлением ('0.25' → '25%').
function formatPercent(share) {
    if (typeof share !== 'number' || !isFinite(share)) return '—';
    return trimZeros((share * 100).toFixed(1)) + '%';
}

// currentUnitScaleKey — выбранный масштаб единицы (ключ gs_unitScale); нет
// storage (Node) → дефолт «на млрд». Читается один раз на рендер.
function currentUnitScaleKey() {
    return readUnitScale(typeof localStorage !== 'undefined' ? localStorage : null);
}

// bufferRowHtml — строка буфера «имя → количество».
function bufferRowHtml(entry) {
    const name = entry && entry.good_name ? entry.good_name : '#' + (entry && entry.good_id);
    return `<div style="color:#ccc;">${name}: <strong>${formatAmount(entry && entry.amount)}</strong></div>`;
}

// branchArithmeticRowsHtml — строки витрины арифметики ветки (спека
// 2026-09-23-стадии-поселения §8.2/§11.3): число скорости пары «тип × рецепт»
// в выбранном масштабе (rate хранится как «ед/сутки/млрд» — §2.1), «забираем»
// по компонентам рецепта (абсолютные ед/сутки на текущем населении), доля
// добора из залежей (проценты) и пометка «рецепт не в наборе стадии».
function branchArithmeticRowsHtml(branch, unitScaleKey) {
    let html = '';
    if (branch.rate != null) {
        const shown = formatCount(storedToDisplay(branch.rate, unitScaleKey));
        html += `<div style="color:#888; font-size:0.85rem;">скорость (число пары): <strong>${shown}</strong> ${unitScaleDef(unitScaleKey).unit}</div>`;
    }
    const take = Array.isArray(branch.take) ? branch.take : [];
    if (take.length > 0) {
        const items = take.map(t => `${t.good_name || ('#' + t.good_id)} ×${formatCount(t.per_day)}`).join(' · ');
        html += `<div style="color:#ccc; font-size:0.85rem;">забираем: <strong>${items}</strong></div>`;
    }
    if (branch.deposit_share > 0) {
        html += `<div style="color:#888; font-size:0.85rem;">из залежи: <strong>${formatPercent(branch.deposit_share)}</strong></div>`;
    }
    if (branch.not_in_stage_set) {
        html += `<div style="color:#facc15; font-size:0.85rem;">рецепт не в наборе стадии (не производит)</div>`;
    }
    return html;
}

// branchBlockHtml — блок одной ветки. Input показывается только админу
// (isAdmin): входной буфер видит только админ (§6). Выход — «пол» по качеству
// (поле качества не заводим, §1) — отдельной пометки в блоке нет.
export function branchBlockHtml(branch, isAdmin) {
    if (!branch) return '';
    const unitScaleKey = currentUnitScaleKey();
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
    html += branchArithmeticRowsHtml(branch, unitScaleKey);
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

// settlementArithmeticHtml — блок арифметики поселения на текущем населении
// (спека 2026-09-23-стадии-поселения §8.2/§11.3): по позициям производим /
// потребляем / сверх (отрицательный net → «дефицит <|net|>»), ед/сутки —
// абсолютные, с разделителями разрядов. Масштаб к ним НЕ применяется. Пустой /
// отсутствующий блок → пустая строка (у player при выключенной настройке и в
// снимке сервер его чистит).
export function settlementArithmeticHtml(settlement) {
    const list = settlement && Array.isArray(settlement.arithmetic) ? settlement.arithmetic : [];
    if (list.length === 0) return '';
    let html = `<div style="color:#888; font-size:0.9rem; text-transform:uppercase; margin-top:8px;">Арифметика (на текущем населении, ед/сутки)</div>`;
    list.forEach(a => {
        const net = a.net_per_day;
        let netHtml;
        if (typeof net === 'number' && net < 0) {
            netHtml = `дефицит <strong>${formatCount(Math.abs(net))}</strong>`;
        } else if (typeof net === 'number') {
            netHtml = `сверх <strong>+${formatCount(net)}</strong>`;
        } else {
            netHtml = `сверх <strong>—</strong>`;
        }
        html += `<div style="color:#ccc; font-size:0.9rem;">${a.position}: производим <strong>${formatCount(a.produced_per_day)}</strong> · потребляем <strong>${formatCount(a.consumed_per_day)}</strong> · ${netHtml}</div>`;
    });
    return html;
}

// stageRowHtml — строка ступени поселения (спека 2026-09-23-стадии-поселения
// §11.3): имя типа (стадия не тайна — отдаётся всегда) + пороги. Пороги и
// близость — люди, абсолютные, с разделителями разрядов (масштаб НЕ
// применяется). Порог входа — только если объявлен (>0); порог выхода >0 — как
// есть, иначе «не читается (пол)»; при входе следующей ступени — близость «до
// следующей» (next_enter − население). Нет имени типа → пустая строка (в снимке
// тип не замораживается).
export function stageRowHtml(settlement) {
    const typeName = settlement && settlement.type_name;
    if (!typeName) return '';
    const stage = settlement.stage;
    let line = `Стадия: <strong>${typeName}</strong>`;
    if (stage) {
        if (stage.enter > 0) line += ` · порог входа ${formatCount(stage.enter)}`;
        line += stage.exit > 0 ? ` · порог выхода ${formatCount(stage.exit)}` : ' · порог выхода не читается (пол)';
        if (stage.next_enter > 0) {
            line += ` · до следующей ступени ${formatCount(stage.next_enter - (settlement.population || 0))}`;
        }
    }
    return `<div>${line}</div>`;
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

// effectsBlockHtml — блок действующих эффектов поселения (§6): админу — как
// раньше (тип/источник, нагрузка, порог, состояние, сила/w + форма ручной
// нагрузки); игроку (спека 2026-09-23-орбита-планеты-присутствие-и-снимок §5.4/
// §6.2 п.2) — player-safe DTO {name, impact, state} без нагрузки/порога/силы.
// Пустой список игроку не показываем (блока нет).
export function effectsBlockHtml(effects, isAdmin, settlementID) {
    const list = Array.isArray(effects) ? effects : [];
    if (!isAdmin) {
        if (list.length === 0) return '';
        let playerHtml = `<div style="color:#888; font-size:0.9rem; text-transform:uppercase; margin-top:8px;">Эффекты (${list.length})</div>`;
        list.forEach(e => { playerHtml += playerEffectRowHtml(e); });
        return playerHtml;
    }
    let html = `<div style="color:#888; font-size:0.9rem; text-transform:uppercase; margin-top:8px;">Эффекты (${list.length})</div>`;
    if (list.length === 0) {
        html += `<div style="color:#666; font-size:0.85rem;">Эффектов нет</div>`;
    }
    list.forEach(e => { html += effectRowHtml(e); });
    html += adminEffectLoadFormHtml(settlementID);
    return html;
}

// EFFECT_IMPACT_LABELS — словарь подписей вида воздействия (effect_types.impact,
// §5.4): единственное место перевода ключа в человекочитаемый текст; неизвестный
// ключ показывается как есть (тот же паттерн, что BUILDING_TYPE_LABELS в tabs.js).
const EFFECT_IMPACT_LABELS = {
    population_rate: 'влияет на население'
};

// playerEffectRowHtml — строка эффекта игроку (§5.4): имя, вид воздействия,
// состояние («действует»/«не действует»). Нагрузки/порога/кривой/силы R(load)
// игроку не отдаём и не рисуем (§15).
function playerEffectRowHtml(e) {
    const name = (e && e.name) ? e.name : 'Эффект';
    const impactKey = e && e.impact;
    const impact = impactKey ? (EFFECT_IMPACT_LABELS[impactKey] || impactKey) : '';
    const impactLine = impact ? ` · <span style="color:#94a3b8;">${impact}</span>` : '';
    const state = (e && e.state === 'active') ? 'действует' : 'не действует';
    return `
        <div style="margin:6px 0; padding:8px; background:#14142a; border-radius:4px;">
            <div><strong>${name}</strong>${impactLine}</div>
            <div style="color:#ccc; font-size:0.9rem;">состояние: <strong>${state}</strong></div>
        </div>`;
}
