// web/static/js/admin/hypothesis.js
//
// Вкладка «Проверка гипотез»: выбор гипотезы, крутилки (как в поселениях) —
// планеты у звезды, население — и «тонкая настройка»: поля planet.data как
// строки «поле → значение». Варьируемая ось гипотезы — тоже поле (первая
// строка, помечена «варьируемая ось»); добавить её повторно нельзя. На сервер
// уходит готовый TwinSpec: шаблон + оверрайды (ось + добавленные поля).
import { fetchWithAuth } from './auth.js';
import { pollJob, pollIntervals } from './poll.js';
import { getSettlementFields } from './generation.js';
import { HYPOTHESIS_PRESETS } from './hypothesisPresets.js';

// currentPreset — выбранный пресет (для сборки spec).
let currentPreset = null;

// bindFieldsLoaded — перерисовка формы, когда подгрузился реестр полей
// (оси/baked-поля рисуются по нему; до загрузки они пусты).
let fieldsBound = false;
function bindFieldsLoaded() {
    if (fieldsBound) return;
    fieldsBound = true;
    document.addEventListener('settlement-fields-loaded', () => {
        if (document.getElementById('hypothesisSelect')) renderHypothesisForm();
    });
}

// populateHypothesisPresets — наполняет селект и рисует форму.
export function populateHypothesisPresets() {
    bindFieldsLoaded();
    const sel = document.getElementById('hypothesisSelect');
    sel.innerHTML = HYPOTHESIS_PRESETS.map(p =>
        `<option value="${p.id}">${p.name}</option>`).join('');
    renderHypothesisForm();
}

// renderHypothesisForm — карточки групп: ось, планеты, население, и
// сворачиваемая «тонкая настройка» (остальные поля, закрыта по умолчанию).
export function renderHypothesisForm() {
    const sel = document.getElementById('hypothesisSelect');
    const preset = HYPOTHESIS_PRESETS.find(p => p.id === sel.value);
    if (!preset) return;
    currentPreset = preset;

    document.getElementById('hypothesisInfo').textContent = preset.hypothesis;

    const box = document.getElementById('hypothesisGroups');
    box.innerHTML = preset.groups.map((g, i) => `
        <div class="hyp-group">
            <div class="hyp-group-name">${g.name}</div>

            <div id="hypAxis_${i}"></div>

            <div class="hyp-line">
                <label>Планет у звезды
                    <input type="number" id="hypPPW_${i}" value="${g.planetsPerWorld}" min="1">
                </label>
            </div>

            <div class="hyp-line">
                <label>Поселений на планету
                    <input type="number" id="hypSPP_${i}" value="${g.settlementsPerPlanet ?? 1}" min="1">
                </label>
            </div>

            <div class="hyp-line">
                <label>Население</label>
                <div class="seg seg-sm" id="hypPopSeg_${i}">
                    <button type="button" class="seg-btn ${g.population.kind === 'random' ? 'active' : ''}"
                            data-kind="random" onclick="setHypPopKind(${i}, 'random')">Случайное</button>
                    <button type="button" class="seg-btn ${g.population.kind === 'fixed' ? 'active' : ''}"
                            data-kind="fixed" onclick="setHypPopKind(${i}, 'fixed')">Фиксированное</button>
                </div>
                <div class="pop-line" id="hypPopInputs_${i}"></div>
            </div>

            <div id="hypComps_${i}"></div>

            <details class="hyp-fine">
                <summary>Другие поля · числовые и строковые</summary>
                <div class="hyp-line" style="margin-top:8px;">
                    <button type="button" class="btn secondary" onclick="addHypField(${i})">＋ Добавить поле</button>
                    <span class="hyp-fine-hint">для тех, кто хочет больше контроля</span>
                </div>
                <div id="hypFields_${i}"></div>
            </details>
        </div>`).join('');

    preset.groups.forEach((g, i) => {
        renderHypPopInputs(i, g.population);
        renderAxisFieldRow(preset, g, i);
        renderCompositions(preset, g, i);
        renderAllPlanetFields(preset, g, i);
    });
}

// setHypPopKind — переключение стратегии населения группы.
window.setHypPopKind = (i, kind) => {
    document.querySelectorAll(`#hypPopSeg_${i} .seg-btn`).forEach(b =>
        b.classList.toggle('active', b.dataset.kind === kind));
    renderHypPopInputs(i, { kind });
};

// renderHypPopInputs — поля населения группы (min/max или фиксированное).
function renderHypPopInputs(i, pop) {
    const kind = pop.kind || 'random';
    const box = document.getElementById(`hypPopInputs_${i}`);
    if (kind === 'fixed') {
        box.innerHTML = `<label>Число жителей
            <input type="text" inputmode="numeric" id="hypPopFixed_${i}" value="${fmtDigits(pop.fixed ?? 100000000)}">
        </label>`;
        return;
    }
    box.innerHTML = `
        <label>Мин <input type="text" inputmode="numeric" id="hypPopMin_${i}" value="${fmtDigits(pop.min ?? 10000)}"></label>
        <label>Макс <input type="text" inputmode="numeric" id="hypPopMax_${i}" value="${fmtDigits(pop.max ?? 300000000)}"></label>`;
}

// ==================== ПОЛЯ (ТОНКАЯ НАСТРОЙКА) ====================

// axisKey — ключ варьируемой оси (текущий выбор в дроп-дауне, или null).
function axisKey() {
    const sel = document.querySelector('.hyp-axis-select');
    return sel ? sel.value : null;
}

// existingFieldKeys — ключи полей, уже присутствующих в группе (ось + все
// поля шаблона + добавленные) — чтобы «добавить поле» не создавало дублей.
function existingFieldKeys(i) {
    const keys = new Set();
    document.querySelectorAll(`#hypAxis_${i} .hyp-field-row, #hypFields_${i} .hyp-field-row`).forEach(row => {
        const sel = row.querySelector('.hyp-field-select') || row.querySelector('.hyp-axis-select');
        if (sel && sel.value) keys.add(sel.value);
        const label = row.querySelector('.hyp-field-map-label');
        if (label) keys.add(label.textContent);
    });
    return keys;
}

// fieldOptionsHTML — опции селекта полей, исключая уже занятые ключи.
function fieldOptionsHTML(i) {
    const exclude = new Set([...(axisKey() ? [axisKey()] : []), ...existingFieldKeys(i)]);
    return getSettlementFields()
        .filter(f => !exclude.has(f.key))
        .map(f => `<option value="${f.key}">${f.label}</option>`).join('');
}

// hypFieldRowHTML — строка «поле → значение» (селект поля + ввод + копия).
function hypFieldRowHTML(i) {
    return `
        <div class="hyp-field-row">
            <select class="hyp-field-select" onchange="renderHypFieldValue(${i}, this)">
                <option value="">— поле —</option>
                ${fieldOptionsHTML(i)}
            </select>
            <span class="hyp-field-value"></span>
            <button type="button" class="btn hyp-field-copy" onclick="copyHypField(this)" title="Скопировать поле в другие группы">⇄</button>
            <button type="button" class="btn danger hyp-field-del" onclick="this.parentElement.remove()" title="Убрать поле">✕</button>
        </div>`;
}

// addHypField — добавляет строку «поле → значение» для переопределения data.
window.addHypField = (i) => {
    document.getElementById(`hypFields_${i}`).insertAdjacentHTML('beforeend', hypFieldRowHTML(i));
};

// renderAxisFieldRow — варьируемая ось гипотезы: дроп-даун всех полей
// (можно сменить руками, синхронизируется в группе-близнеце) + значение.
function renderAxisFieldRow(preset, g, i) {
    const a = preset.axis;
    if (!a) return; // ось не задана — варьируется только стартовое население
    const options = getSettlementFields().map(f => `<option value="${f.key}">${f.label}</option>`).join('');

    const box = document.getElementById(`hypAxis_${i}`);
    box.innerHTML = `
        <div class="hyp-field-row hyp-axis-row">
            <select class="hyp-axis-select" onchange="setHypAxis(${i}, this)">
                <option value="">— поле —</option>
                ${options}
            </select>
            <span class="hyp-field-value"></span>
            <span class="hyp-axis-tag">варьируемая ось</span>
        </div>`;

    const row = box.firstElementChild;
    row.querySelector('.hyp-axis-select').value = a.key;
    renderHypFieldValue(i, row.querySelector('.hyp-axis-select'));

    let val = g.axisValue;
    if (a.key === 'temperature' && typeof val === 'number') {
        val = Math.round((val - 273) * 10) / 10; // K → °C (в UI температура в °C)
    }
    writeHypFieldValue(row, val);
}

// setHypAxis — смена варьируемой оси: синхронизирует дроп-дауны во всех
// группах (близнецы меняют одно и то же поле) и перерисовывает ввод значения.
window.setHypAxis = (i, sel) => {
    const key = sel.value;
    document.querySelectorAll('[id^="hypAxis_"]').forEach(box => {
        const j = parseInt(box.id.replace('hypAxis_', ''));
        const s = box.querySelector('.hyp-axis-select');
        if (!s) return;
        s.value = key;
        renderHypFieldValue(j, s);
    });
};

// compLabels — читаемые названия композиций.
const compLabels = { surface_composition: 'Поверхность', subterrain_composition: 'Недра' };

// SURFACE_FORMS / SUBTERRAIN_FORMS — зеркало composition_forms.go (16 / 17).
const SURFACE_FORMS = [
    'горы', 'пески_пустыни', 'кратеры', 'стеклянные_поля', 'металлические_поля',
    'лавовые_поля', 'вулканические_поля', 'ледники', 'мёрзлые_газы', 'океаны',
    'озёра_реки', 'луга_степи', 'леса', 'джунгли', 'болота', 'коралловые_рифы',
];
const SUBTERRAIN_FORMS = [
    'пустая_порода', 'магматические_породы', 'метаморфические_породы', 'осадочные_породы',
    'рудные_жилы', 'редкоземельные_жилы', 'радиоактивные_зоны', 'угольные_пласты',
    'нефтяные_карманы', 'газовые_карманы', 'подземные_воды', 'подземные_льды',
    'магматические_камеры', 'кристаллические_жилы', 'соляные_купола', 'пещерные_системы',
    'металлические_ядра',
];

// compFormsForKey — список форм по ключу композиции.
function compFormsForKey(key) {
    return key === 'subterrain_composition' ? SUBTERRAIN_FORMS : SURFACE_FORMS;
}

// compPairHTML — строка «форма → процент» редактора композиции (форма — селект).
function compPairHTML(form, pct, forms) {
    const options = forms.map(f =>
        `<option value="${f}" ${f === form ? 'selected' : ''}>${f}</option>`).join('');
    return `
        <div class="hyp-comp-pair">
            <select class="hyp-comp-form">${options}</select>
            <input type="number" class="hyp-comp-pct" value="${pct}" min="0" max="100" step="1">
            <span class="unit">%</span>
            <button type="button" class="btn danger hyp-field-del" onclick="this.parentElement.remove()" title="Убрать">✕</button>
        </div>`;
}

// addCompPair — добавляет строку формы в редактор композиции.
window.addCompPair = (btn) => {
    const key = btn.closest('.hyp-comp-pairs').dataset.compKey;
    btn.insertAdjacentHTML('beforebegin', compPairHTML('', '', compFormsForKey(key)));
};

// renderCompositions — композиции (поверхность/недра) как редакторы
// «форма → процент» в карточке группы (паритет обеих колонок).
function renderCompositions(preset, g, i) {
    const box = document.getElementById(`hypComps_${i}`);
    const effective = { ...(preset.base || {}), ...(g.overrides || {}) };

    for (const key of ['surface_composition', 'subterrain_composition']) {
        const value = effective[key];
        if (!value) continue;
        const pairs = Object.entries(value)
            .map(([form, pct]) => compPairHTML(form, pct, compFormsForKey(key))).join('');
        box.insertAdjacentHTML('beforeend', `
            <div class="hyp-comp">
                <div class="hyp-comp-title">${compLabels[key] || key}</div>
                <div class="hyp-comp-pairs" data-comp-key="${key}">
                    ${pairs}
                    <button type="button" class="btn secondary hyp-comp-add" onclick="addCompPair(this)">＋</button>
                </div>
            </div>`);
    }
}

// renderAllPlanetFields — числовые/строковые поля планеты группы (шаблон base
// + baked оверрайды поверх) в «других полях». Ось и композиции показаны
// отдельно и сюда не дублируются; поля вне реестра (description и т.п.) не
// показываются. Вложенные объекты (core) раскрываются в dot-ключи
// (core.radioactivity и т.п.) — как их знает реестр полей.
function renderAllPlanetFields(preset, g, i) {
    const box = document.getElementById(`hypFields_${i}`);
    const axis = axisKey();
    const effective = { ...(preset.base || {}), ...(g.overrides || {}) };

    const entries = [];
    for (const [key, value] of Object.entries(effective)) {
        if (key === 'core' && value && typeof value === 'object') {
            for (const [sub, subVal] of Object.entries(value)) {
                const dot = `${key}.${sub}`;
                if (dot === axis) continue;
                entries.push([dot, subVal]);
            }
            continue;
        }
        if (key === axis) continue;
        entries.push([key, value]);
    }

    for (const [key, value] of entries) {
        const spec = getSettlementFields().find(f => f.key === key);
        if (!spec) continue; // композиции — в renderCompositions; description — вне тонкой настройки

        box.insertAdjacentHTML('beforeend', hypFieldRowHTML(i));
        const row = box.lastElementChild;
        row.querySelector('.hyp-field-select').value = key;
        renderHypFieldValue(i, row.querySelector('.hyp-field-select'));
        let val = value;
        if (spec.type === 'number' && key === 'temperature' && typeof value === 'number') {
            val = Math.round((value - 273) * 10) / 10; // K → °C
        }
        writeHypFieldValue(row, val);
    }
}

// renderHypFieldValue — рисует ввод значения по типу выбранного поля.
window.renderHypFieldValue = (i, sel) => {
    const spec = getSettlementFields().find(f => f.key === sel.value);
    const box = sel.parentElement.querySelector('.hyp-field-value');
    if (!spec) {
        box.innerHTML = '';
        return;
    }
    if (spec.type === 'bool') {
        box.innerHTML = `<label><input type="checkbox" class="hyp-field-input"> да</label>`;
    } else if (spec.type === 'string') {
        const opts = (spec.values || []).length
            ? spec.values.map(v => `<option value="${v}">${v}</option>`).join('')
            : '';
        box.innerHTML = `<select class="hyp-field-input"><option value="">— значение —</option>${opts}</select>`;
    } else {
        const unit = spec.unit ? ` <span class="unit">${spec.unit}</span>` : '';
        // step="any" обязателен: браузерный step=1 отклонил бы дробные значения
        // (density 0.077, gravity 0.29, mass 15.9). min/max + тултип — рамки
        // реестра (температура в °C — как и рамки).
        let attrs = 'step="any"';
        if (spec.min != null && spec.max != null) {
            attrs += ` min="${spec.min}" max="${spec.max}" title="Допустимо: ${spec.min}…${spec.max}${spec.unit ? ' ' + spec.unit : ''}"`;
        }
        box.innerHTML = `<input type="number" ${attrs} class="hyp-field-input" placeholder="значение">${unit}`;
    }
};

// readHypFieldValue — сырое значение из ввода строки поля (display-форма,
// температура в °C — копируется как есть, без конверсии).
function readHypFieldValue(row) {
    const input = row.querySelector('.hyp-field-input');
    if (!input) return null;
    if (input.type === 'checkbox') return input.checked;
    return input.value;
}

// writeHypFieldValue — проставляет значение в ввод строки поля.
function writeHypFieldValue(row, value) {
    const input = row.querySelector('.hyp-field-input');
    if (!input) return;
    if (input.type === 'checkbox') {
        input.checked = !!value;
    } else {
        input.value = value;
    }
}

// copyHypField — копирует поле (ключ + значение) из этой строки в остальные
// группы: есть у соседа такое поле — обновить значение, нет — добавить.
window.copyHypField = (btn) => {
    const row = btn.closest('.hyp-field-row');
    const box = row.parentElement;
    const fromI = parseInt(box.id.replace('hypFields_', ''));
    const sel = row.querySelector('.hyp-field-select');
    const key = sel.value;
    if (!key) return;
    const value = readHypFieldValue(row);
    if (value === null) return;

    document.querySelectorAll('[id^="hypFields_"]').forEach(target => {
        const toI = parseInt(target.id.replace('hypFields_', ''));
        if (toI === fromI) return;

        let existing = null;
        target.querySelectorAll('.hyp-field-row').forEach(r => {
            if (r.querySelector('.hyp-field-select').value === key) existing = r;
        });

        if (existing) {
            writeHypFieldValue(existing, value);
            return;
        }
        target.insertAdjacentHTML('beforeend', hypFieldRowHTML(toI));
        const added = target.lastElementChild;
        added.querySelector('.hyp-field-select').value = key;
        renderHypFieldValue(toI, added.querySelector('.hyp-field-select'));
        writeHypFieldValue(added, value);
    });
};

// collectHypOverrides — собирает все строки полей группы (ось + поля шаблона +
// добавленные + композиции) в overrides. Температура в данных — K (+273).
function collectHypOverrides(i) {
    const overrides = {};
    const fields = getSettlementFields();

    // Композиции (редактор «форма → процент»).
    document.querySelectorAll(`#hypComps_${i} .hyp-comp`).forEach(comp => {
        const key = comp.querySelector('.hyp-comp-pairs').dataset.compKey;
        const map = {};
        comp.querySelectorAll('.hyp-comp-pair').forEach(pair => {
            const form = pair.querySelector('.hyp-comp-form').value.trim();
            const pct = parseFloat(pair.querySelector('.hyp-comp-pct').value);
            if (form && !isNaN(pct)) map[form] = pct;
        });
        overrides[key] = map;
    });

    // Ось + числовые/строковые поля. Dot-ключи (core.radioactivity) собираются
    // как вложенные объекты: { core: { radioactivity: X } }.
    document.querySelectorAll(`#hypAxis_${i} .hyp-field-row, #hypFields_${i} .hyp-field-row`).forEach(row => {
        const sel = row.querySelector('.hyp-field-select') || row.querySelector('.hyp-axis-select');
        const key = sel && sel.value;
        if (!key) return;
        const spec = fields.find(f => f.key === key);
        const input = row.querySelector('.hyp-field-input');
        if (!input) return;
        let val;
        if (spec.type === 'bool') {
            val = input.checked;
        } else if (spec.type === 'string') {
            if (!input.value) return;
            val = input.value;
        } else {
            const v = parseFloat(input.value);
            if (isNaN(v)) return;
            val = key === 'temperature' ? v + 273 : v;
        }
        if (key.includes('.')) {
            const parts = key.split('.');
            let obj = overrides;
            for (let i = 0; i < parts.length - 1; i++) {
                if (typeof obj[parts[i]] !== 'object' || obj[parts[i]] === null) obj[parts[i]] = {};
                obj = obj[parts[i]];
            }
            obj[parts[parts.length - 1]] = val;
        } else {
            overrides[key] = val;
        }
    });
    return overrides;
}

// collectHypRangeErrors — жёсткая проверка числовых значений групп против
// рамок реестра (температура в форме — °C, как и рамки реестра). Возвращает
// перечень «группа/поле/значение/рамка»; пусто — всё в рамках. Ошибка, а не
// кламп: инструмент — проверка рамок, молчаливое исправление скрыло бы факт.
function collectHypRangeErrors() {
    const errors = [];
    const fields = getSettlementFields();
    currentPreset.groups.forEach((g, i) => {
        const groupName = g.name || `группа ${i + 1}`;
        document.querySelectorAll(`#hypAxis_${i} .hyp-field-row, #hypFields_${i} .hyp-field-row`).forEach(row => {
            const sel = row.querySelector('.hyp-field-select') || row.querySelector('.hyp-axis-select');
            const key = sel && sel.value;
            if (!key) return;
            const spec = fields.find(f => f.key === key);
            if (!spec || spec.type !== 'number' || spec.min == null || spec.max == null) return;
            const input = row.querySelector('.hyp-field-input');
            if (!input || input.value === '') return;
            const val = parseFloat(input.value);
            if (isNaN(val)) return;
            if (val < spec.min || val > spec.max) {
                errors.push(`${groupName}/${key}: ${val} вне рамок [${spec.min}, ${spec.max}]${spec.unit ? ' ' + spec.unit : ''}`);
            }
        });
    });
    return errors;
}

// buildTwinSpec — собирает TwinSpec из формы.
function buildTwinSpec() {
    const groups = currentPreset.groups.map((g, i) => {
        const kind = document.querySelector(`#hypPopSeg_${i} .seg-btn.active`).dataset.kind;
        const population = kind === 'fixed'
            ? { kind: 'fixed', fixed: parseInt(parseDigits(document.getElementById(`hypPopFixed_${i}`).value)) }
            : {
                kind: 'random',
                min: parseInt(parseDigits(document.getElementById(`hypPopMin_${i}`).value)),
                max: parseInt(parseDigits(document.getElementById(`hypPopMax_${i}`).value)),
            };

        return {
            id: g.id,
            name: g.name,
            overrides: collectHypOverrides(i),
            planets_per_world: parseInt(document.getElementById(`hypPPW_${i}`).value),
            settlement: {
                // Шанс в близнецах не настраивается — берём из пресета.
                chance: (g.chance ?? 100) / 100,
                settlements_per_planet: parseInt(document.getElementById(`hypSPP_${i}`).value),
                population,
            },
        };
    });
    return { id: currentPreset.id, base: currentPreset.base, groups };
}

// runHypothesis — запуск эксперимента выбранной гипотезы.
export async function runHypothesis() {
    const preset = currentPreset;
    if (!preset) return;

    if (!confirm(`Запустить эксперимент «${preset.name}»?\n\nВселенная будет перетёрта (миры, планеты, поселения).`)) return;

    // Жёсткая проверка рамок: значения вне рамок реестра — ошибка, POST не
    // уходит (например, температура −50000 °C блокируется здесь и на сервере).
    const rangeErrors = collectHypRangeErrors();
    if (rangeErrors.length) {
        document.getElementById('hypothesisResult').textContent = '❌ Значения вне рамок генератора:\n' + rangeErrors.join('\n');
        document.getElementById('hypothesisProgress').style.display = 'none';
        return;
    }

    const spec = buildTwinSpec();

    document.getElementById('hypothesisResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('hypothesisProgress').style.display = 'block';
    document.getElementById('hypothesisProgressBar').value = 0;
    document.getElementById('hypothesisProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelHypothesisBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/hypothesis/run', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(spec),
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('hypothesisResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('hypothesisProgress').style.display = 'none';
            document.getElementById('cancelHypothesisBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['hypothesis']) clearInterval(pollIntervals['hypothesis']);
        pollIntervals['hypothesis'] = setInterval(
            () => pollJob('hypothesis', 'hypothesisProgress', 'hypothesisResult', 'cancelHypothesisBtn'), 1500);
    } catch (e) {
        document.getElementById('hypothesisResult').textContent = '❌ ' + e.message;
        document.getElementById('hypothesisProgress').style.display = 'none';
        document.getElementById('cancelHypothesisBtn').style.display = 'none';
    }
}

// fmtDigits / parseDigits — разряды через пробел (как в поселениях).
function fmtDigits(v) {
    if (v == null) return '';
    const s = String(v);
    const [int, ...rest] = s.split('.');
    const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
    return grouped + (rest.length ? '.' + rest.join('.') : '');
}

function parseDigits(s) {
    if (s == null) return NaN;
    return parseFloat(String(s).replace(/\s/g, '').replace(',', '.'));
}