// web/static/js/admin/generation.js
import { fetchWithAuth } from './auth.js';
import { loadStats } from './stats.js';
import { loadWorlds } from './worlds.js';
import { pollJob, pollIntervals } from './poll.js';
import { notifyError, notifyInfo } from '../ui/toast.js';
import { SETTLEMENT_PRESETS } from './settlementPresets.js';

// Пресеты генерации вселенной (проверены: 100k миров, 200 кластеров).
const UNIVERSE_PRESETS = {
    dense: {   // плотная галактика
        worlds: 100000, clusters: 200, mapSize: 40000, minDist: 120,
        clusterRadius: 2500, clusterSpacing: 5000, outlierPercent: 20
    },
    sparse: {  // просторная: большая галактика, миры дальше друг от друга
        worlds: 100000, clusters: 200, mapSize: 50000, minDist: 150,
        clusterRadius: 2500, clusterSpacing: 5000, outlierPercent: 20
    },
    compact: { // тесная: компактная галактика, миры близко, больше выбросов
        worlds: 100000, clusters: 200, mapSize: 36000, minDist: 100,
        clusterRadius: 1600, clusterSpacing: 3200, outlierPercent: 25
    },
    prototype: { // тестовая галактика: один мир для прототипа поселения
        worlds: 1, clusters: 1, mapSize: 800, minDist: 100,
        clusterRadius: 80, clusterSpacing: 160, outlierPercent: 0
    },
    swarm: {   // рой: компактная группа мелких миров
        worlds: 300, clusters: 6, mapSize: 3000, minDist: 120,
        clusterRadius: 620, clusterSpacing: 1300, outlierPercent: 15, shape: 'blob'
    },
    frontier: {   // пограничье: просторная галактика, много выбросов
        worlds: 1000, clusters: 8, mapSize: 6500, minDist: 180,
        clusterRadius: 1480, clusterSpacing: 3000, outlierPercent: 30, shape: 'blob'
    },
    constellation: {   // созвездие: много кластеров, круглая форма
        worlds: 3000, clusters: 40, mapSize: 8000, minDist: 100,
        clusterRadius: 650, clusterSpacing: 1350, outlierPercent: 10, shape: 'circle'
    },
    crossroads: {   // перекрёсток: средняя галактика, много миров
        worlds: 10000, clusters: 30, mapSize: 13000, minDist: 110,
        clusterRadius: 1470, clusterSpacing: 3000, outlierPercent: 20, shape: 'blob'
    },
    archipelago: {   // архипелаг: крупные острова-кластеры, круглая форма
        worlds: 20000, clusters: 12, mapSize: 21000, minDist: 130,
        clusterRadius: 3950, clusterSpacing: 8000, outlierPercent: 15, shape: 'circle'
    },
    outback: {   // глубинка: много кластеров, много выбросов
        worlds: 30000, clusters: 80, mapSize: 34000, minDist: 160,
        clusterRadius: 2250, clusterSpacing: 4600, outlierPercent: 30, shape: 'blob'
    },
    agglomeration: {   // скопление: очень много кластеров
        worlds: 50000, clusters: 150, mapSize: 30000, minDist: 110,
        clusterRadius: 1540, clusterSpacing: 3100, outlierPercent: 12, shape: 'blob'
    },
    fararm: {   // дальняя рука: огромная галактика, круглая форма
        worlds: 75000, clusters: 40, mapSize: 47000, minDist: 150,
        clusterRadius: 4800, clusterSpacing: 9700, outlierPercent: 18, shape: 'circle'
    },
    metropolis: {   // метрополия: максимум миров, много кластеров
        worlds: 100000, clusters: 300, mapSize: 42000, minDist: 115,
        clusterRadius: 1580, clusterSpacing: 3200, outlierPercent: 15, shape: 'blob'
    },
    outskirts: {   // окраина: огромная галактика, много выбросов
        worlds: 100000, clusters: 60, mapSize: 50000, minDist: 140,
        clusterRadius: 3760, clusterSpacing: 7600, outlierPercent: 35, shape: 'blob'
    },
};

// applyPreset — заполняет поля формы из выбранного пресета.
export function applyPreset() {
    const p = UNIVERSE_PRESETS[document.getElementById('genPreset').value];
    if (!p) return;
    document.getElementById('genWorlds').value = p.worlds;
    document.getElementById('genClusters').value = p.clusters;
    document.getElementById('genMapSize').value = p.mapSize;
    document.getElementById('genMinDist').value = p.minDist;
    document.getElementById('genClusterRadius').value = p.clusterRadius;
    document.getElementById('genClusterSpacing').value = p.clusterSpacing;
    document.getElementById('genOutlierPercent').value = p.outlierPercent;
    document.getElementById('genShape').value = p.shape || 'blob';
}

export async function generateUniverse() {
    const worlds = parseInt(document.getElementById('genWorlds').value);
    const clusters = parseInt(document.getElementById('genClusters').value);
    const mapSize = parseInt(document.getElementById('genMapSize').value);
    const minDist = parseInt(document.getElementById('genMinDist').value);
    const clusterRadius = parseInt(document.getElementById('genClusterRadius').value);
    const clusterSpacing = parseInt(document.getElementById('genClusterSpacing').value);
    const outlierPercent = parseInt(document.getElementById('genOutlierPercent').value);
    const shape = document.getElementById('genShape').value;
    if (isNaN(worlds) || isNaN(clusters) || worlds <= 0 || clusters <= 0 || isNaN(mapSize) || mapSize <= 0 || isNaN(minDist) || minDist <= 0 || isNaN(clusterRadius) || clusterRadius <= 0 || isNaN(clusterSpacing) || clusterSpacing <= 0 || isNaN(outlierPercent) || outlierPercent < 0) {
        document.getElementById('genResult').textContent = '❌ Введите корректные числа';
        return;
    }
    if (!confirm(`Сгенерировать ${worlds} миров в ${clusters} кластерах?`)) return;

    document.getElementById('genResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('genProgress').style.display = 'block';
    document.getElementById('genProgressBar').value = 0;
    document.getElementById('genProgressText').textContent = '0 из ' + worlds;
    document.getElementById('cancelUniverseBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                world_count: worlds,
                cluster_count: clusters,
                map_size: mapSize,
                min_dist: minDist,
                cluster_radius: clusterRadius,
                cluster_spacing: clusterSpacing,
                outlier_percent: outlierPercent,
                shape: shape
            })
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('genResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('genProgress').style.display = 'none';
            document.getElementById('cancelUniverseBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['universe']) clearInterval(pollIntervals['universe']);
        pollIntervals['universe'] = setInterval(() => pollJob('generate_universe', 'genProgress', 'genResult', 'cancelUniverseBtn'), 1500);
    } catch (e) {
        document.getElementById('genResult').textContent = '❌ ' + e.message;
        document.getElementById('genProgress').style.display = 'none';
        document.getElementById('cancelUniverseBtn').style.display = 'none';
    }
}

export async function generatePlanets() {
    const preset = document.getElementById('planetPreset').value;
    if (preset === 'prototype') {
        await generatePrototypePlanet();
        return;
    }
    if (!confirm('Сгенерировать планеты для всех миров?')) return;
    document.getElementById('planetResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('planetProgress').style.display = 'block';
    document.getElementById('planetProgressBar').value = 0;
    document.getElementById('planetProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelPlanetsBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-planets', { method: 'POST' });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('planetResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('planetProgress').style.display = 'none';
            document.getElementById('cancelPlanetsBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['planets']) clearInterval(pollIntervals['planets']);
        pollIntervals['planets'] = setInterval(() => pollJob('generate_planets', 'planetProgress', 'planetResult', 'cancelPlanetsBtn'), 1500);
    } catch (e) {
        document.getElementById('planetResult').textContent = '❌ ' + e.message;
        document.getElementById('planetProgress').style.display = 'none';
        document.getElementById('cancelPlanetsBtn').style.display = 'none';
    }
}

export async function generateFactions() {
    if (!confirm('Сгенерировать фракции для всех обитаемых планет?')) return;
    document.getElementById('factionResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('factionProgress').style.display = 'block';
    document.getElementById('factionProgressBar').value = 0;
    document.getElementById('factionProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelFactionsBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-factions', { method: 'POST' });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('factionResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('factionProgress').style.display = 'none';
            document.getElementById('cancelFactionsBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['factions']) clearInterval(pollIntervals['factions']);
        pollIntervals['factions'] = setInterval(() => pollJob('generate_factions', 'factionProgress', 'factionResult', 'cancelFactionsBtn'), 1500);
    } catch (e) {
        document.getElementById('factionResult').textContent = '❌ ' + e.message;
        document.getElementById('factionProgress').style.display = 'none';
        document.getElementById('cancelFactionsBtn').style.display = 'none';
    }
}

export async function generateResources() {
    if (!confirm('Сгенерировать ресурсы для всех планет?')) return;
    document.getElementById('resourceResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('resourceProgress').style.display = 'block';
    document.getElementById('resourceProgressBar').value = 0;
    document.getElementById('resourceProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelResourcesBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-resources', { method: 'POST' });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('resourceResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('resourceProgress').style.display = 'none';
            document.getElementById('cancelResourcesBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['resources']) clearInterval(pollIntervals['resources']);
        pollIntervals['resources'] = setInterval(() => pollJob('generate_resources', 'resourceProgress', 'resourceResult', 'cancelResourcesBtn'), 1500);
    } catch (e) {
        document.getElementById('resourceResult').textContent = '❌ ' + e.message;
        document.getElementById('resourceProgress').style.display = 'none';
        document.getElementById('cancelResourcesBtn').style.display = 'none';
    }
}

// generatePrototypePlanet — тестовая галактика: землеподобная планета
// с поселением 1 уровня (~10 человек) для первого мира.
async function generatePrototypePlanet() {
    if (!confirm('Сгенерировать землеподобную планету с поселением (прототип)?')) return;
    document.getElementById('planetResult').textContent = '⏳ Генерация...';
    try {
        const res = await fetchWithAuth('/admin/generate-prototype-planet', { method: 'POST' });
        const text = await res.text();
        if (!res.ok) {
            document.getElementById('planetResult').textContent = '❌ Ошибка: ' + text;
            return;
        }
        const data = JSON.parse(text);
        document.getElementById('planetResult').textContent =
            `✅ Мир «${data.world_name}» → планета «${data.planet_name}» с поселением (${data.population} чел.)`;
    } catch (e) {
        document.getElementById('planetResult').textContent = '❌ ' + e.message;
    }
}

// ---------- ГЕНЕРАЦИЯ ПОСЕЛЕНИЙ (модель) ----------

// settlementFields — реестр полей planet.data (грузится при инициализации).
// Формат элемента: {key, label, type, unit, min, max, values}.
let settlementFields = [];

// getSettlementFields — реестр полей planet.data для форм «тонкой настройки»
// (близнецы в гипотезах используют тот же реестр, что правила поселений).
export function getSettlementFields() {
    return settlementFields;
}

// settleMode и settlePopKind — текущий выбор сегментов.
let settleMode = 'complex';
let settlePopKind = 'random';
// settlementDirty — форма поселений менялась руками; перед перезаписью
// пресетом это требует подтверждения.
let settlementDirty = false;
// lastPresetID — последний успешно применённый пресет (для возврата селекта).
let lastPresetID = null;

// loadSettlementFields — подгружает реестр полей с сервера для формы правил.
export async function loadSettlementFields() {
    bindSettlementDirty();
    try {
        const res = await fetchWithAuth('/admin/settlement-fields');
        // Пустой пароль (ещё не введён в админку) даёт 401 спокойно, на нём
        // не ругаемся — после ввода пароля функцию вызовут ещё раз.
        if (res.status === 401) return;
        if (!res.ok) {
            notifyError('Не удалось загрузить поля: HTTP ' + res.status);
            return;
        }
        settlementFields = await res.json();
        // Событие для вкладки «Гипотезы»: форма близнецов рисует оси/baked-поля
        // по реестру — до загрузки они пусты.
        document.dispatchEvent(new CustomEvent('settlement-fields-loaded'));
        populateSettlementPresets();
        applySettlementPreset(DEFAULT_PRESET_ID);
        renderSettlementModel();
    } catch (e) {
        if (e.message === 'Unauthorized') return;
        notifyError('Не удалось загрузить поля: ' + e.message);
    }
}

// DEFAULT_PRESET_ID — пресет, который применяется по умолчанию при загрузке.
const DEFAULT_PRESET_ID = 'greenbelt';

// bindSettlementDirty — помечает форму как изменённую руками при любом
// пользовательском изменении контролов поселений (пресет-селект исключён).
let settlementDirtyBound = false;
function bindSettlementDirty() {
    if (settlementDirtyBound) return;
    settlementDirtyBound = true;
    const mark = e => {
        const t = e.target;
        if (!t || !t.closest) return;
        // Смена самого пресета — не ручная правка формы.
        if (t.id === 'settlePreset') return;
        if (t.closest('#settleRulesBlock, #settleSimpleBlock, #settlePopInputs, #settleHistory, #settleHypothesis')) {
            settlementDirty = true;
        }
    };
    document.addEventListener('input', mark);
    document.addEventListener('change', mark);
}

// populateSettlementPresets — наполняет выпадающий список пресетов.
function populateSettlementPresets() {
    const sel = document.getElementById('settlePreset');
    sel.innerHTML = SETTLEMENT_PRESETS.map(p =>
        `<option value="${p.id}">${p.name}</option>`).join('');
}

// applySettlementPreset — заполняет форму (правила, шанс, население, историю)
// из выбранного пресета. Вызывается при смене пресета в селекте и при загрузке.
export function applySettlementPreset() {
    const sel = document.getElementById('settlePreset');
    const preset = SETTLEMENT_PRESETS.find(p => p.id === sel.value);
    if (!preset) return;

    // Вручную правленная форма — спросить перед перезаписью, иначе молча.
    if (settlementDirty) {
        const ok = confirm(`Форма изменена вручную. Применить пресет «${preset.name}» и перезаписать?`);
        if (!ok) {
            if (lastPresetID) sel.value = lastPresetID;
            return;
        }
    }

    lastPresetID = preset.id;
    settlementDirty = false;

    // Режим — всегда сложная модель с правилами.
    setSettleMode('complex');

    // Шанс в процентах.
    document.getElementById('settleChanceRange').value = preset.chance;
    document.getElementById('settleChance').value = preset.chance;

    // Население: переключаем стратегию, затем заполняем конкретные поля.
    setSettlePopKind(preset.population.kind);
    if (preset.population.kind === 'fixed') {
        document.getElementById('settlePopFixedValue').value = fmtDigits(preset.population.fixed);
    } else {
        document.getElementById('settlePopMin').value = fmtDigits(preset.population.min);
        document.getElementById('settlePopMax').value = fmtDigits(preset.population.max);
    }

    // Правила: пересобираем список с нуля.
    const list = document.getElementById('settleRulesList');
    list.innerHTML = '';
    preset.rules.forEach(def => list.appendChild(addRuleRow(def)));

    // История заселения и гипотеза дельты в текстовые поля.
    document.getElementById('settleHistory').value = preset.history;
    if (preset.hypothesis) {
        document.getElementById('settleHypothesis').value = preset.hypothesis;
    }

    // Описание принципа под селектом.
    renderSettlementPresetInfo(preset);
}

// renderSettlementPresetInfo — подсказка с геймдизайнерским принципом пресета.
function renderSettlementPresetInfo(preset) {
    const box = document.getElementById('settlePresetInfo');
    if (preset && preset.principle) {
        box.textContent = preset.principle;
    } else {
        box.textContent = '';
    }
}

// setSettleMode — переключение режима модели (simple / complex).
export function setSettleMode(mode) {
    settleMode = mode;
    document.querySelectorAll('#settleModeSeg .seg-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.mode === mode));
    renderSettlementModel();
}

// setSettlePopKind — переключение стратегии населения (random / fixed).
export function setSettlePopKind(kind) {
    settlePopKind = kind;
    document.querySelectorAll('#settlePopSeg .seg-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.kind === kind));
    renderSettlementModel();
}

// addRuleRow — строка правила с уже заполненными значениями (для пресетов).
function addRuleRow(def = {}) {
    const row = document.createElement('div');
    row.className = 'settle-rule-row';
    const options = settlementFields.map(f => {
        const unit = f.unit ? ' · ' + f.unit : '';
        return `<option value="${f.key}">${f.label}${unit}</option>`;
    }).join('');
    row.innerHTML = `
        <select class="settle-rule-field">
            <option value="">— поле —</option>
            ${options}
        </select>
        <div class="settle-rule-inputs"></div>
        <button class="btn danger settle-rule-del" onclick="this.parentElement.remove()" title="Удалить правило">✕</button>`;
    const field = row.querySelector('.settle-rule-field');
    field.value = def.field || '';
    field.addEventListener('change', () => renderSettlementRuleInputs(row, field.value));
    if (def.field) {
        renderSettlementRuleInputs(row, def.field);
        fillRuleValues(row, def);
    }
    return row;
}

// fillRuleValues — проставляет значения в только что отрисованные поля правила.
function fillRuleValues(row, def) {
    const spec = fieldSpec(def.field);
    if (!spec) return;
    if (spec.type === 'bool') {
        const seg = row.querySelector('.seg');
        const want = def.is === false ? 'false' : 'true';
        seg.querySelectorAll('.seg-btn').forEach(b =>
            b.classList.toggle('active', b.dataset.val === want));
    } else if (spec.type === 'number') {
        const min = row.querySelector('.settle-rule-min');
        const max = row.querySelector('.settle-rule-max');
        if (min && def.min != null) min.value = fmtDigits(def.min);
        if (max && def.max != null) max.value = fmtDigits(def.max);
    } else if (spec.type === 'string') {
        const chips = row.querySelectorAll('.chip input');
        const allowed = def.notIn && def.notIn.length
            ? (spec.values || []).filter(v => !def.notIn.includes(v))
            : (def.in || spec.values || []);
        chips.forEach(inp => { inp.checked = allowed.includes(inp.value); });
    }
}

// fieldSpec — описание поля по ключу.
function fieldSpec(key) {
    return settlementFields.find(f => f.key === key);
}

// renderSettlementModel — показывает/скрывает блоки и рисует инпуты населения.
export function renderSettlementModel() {
    document.getElementById('settleRulesBlock').style.display = settleMode === 'complex' ? 'block' : 'none';
    document.getElementById('settleSimpleBlock').style.display = settleMode === 'simple' ? 'block' : 'none';
    renderSettlementPopulation();
}

// renderSettlementPopulation — инпуты диапазона / фиксированного значения.
function renderSettlementPopulation() {
    const box = document.getElementById('settlePopInputs');
    if (settlePopKind === 'fixed') {
        box.innerHTML = `<div class="pop-line">
            <label>Число жителей
                <input type="text" inputmode="numeric" id="settlePopFixedValue" value="100 000">
            </label>
        </div>`;
    } else {
        box.innerHTML = `<div class="pop-line">
            <label>Мин
                <input type="text" inputmode="numeric" id="settlePopMin" value="100 000">
            </label>
            <label>Макс
                <input type="text" inputmode="numeric" id="settlePopMax" value="1 000 000 000">
            </label>
        </div>`;
    }
}

// fmtDigits — разряды через пробел (только целая часть).
function fmtDigits(v) {
    if (v == null) return '';
    const s = String(v);
    const neg = s.startsWith('-');
    const body = neg ? s.slice(1) : s;
    const [int, ...rest] = body.split('.');
    const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
    return (neg ? '-' : '') + grouped + (rest.length ? '.' + rest.join('.') : '');
}

// parseDigits — читает число из поля с пробелами в разрядах (запятая → точка).
function parseDigits(s) {
    if (s == null) return NaN;
    return parseFloat(String(s).replace(/\s/g, '').replace(',', '.'));
}

// addSettlementRule — добавляет строку правила «поле → условие» в список.
export function addSettlementRule() {
    const list = document.getElementById('settleRulesList');
    list.appendChild(addRuleRow());
}

// renderSettlementRuleInputs — рисует условие правила по типу поля:
// number → min/max (от/до, подсказка диапазона генерации),
// string → чипы значений (равно выбранным),
// bool → сегмент «равно true / false».
function renderSettlementRuleInputs(row, key) {
    const spec = fieldSpec(key);
    const box = row.querySelector('.settle-rule-inputs');
    if (!spec) { box.innerHTML = ''; return; }

    if (spec.type === 'bool') {
        box.innerHTML = `<div class="seg seg-sm">
            <button type="button" class="seg-btn" data-val="true">равно true</button>
            <button type="button" class="seg-btn" data-val="false">равно false</button>
        </div>`;
        box.querySelector('.seg').addEventListener('click', e => {
            const btn = e.target.closest('.seg-btn');
            if (!btn) return;
            box.querySelectorAll('.seg-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
        });
        return;
    }

    if (spec.type === 'number') {
        const lo = spec.min != null ? fmtDigits(spec.min) : '—';
        const hi = spec.max != null ? fmtDigits(spec.max) : '—';
        const range = (spec.min != null || spec.max != null)
            ? `Генерация: ${lo}…${hi}${spec.unit ? ' ' + spec.unit : ''}`
            : '';
        box.innerHTML = `<div class="num-range">
            <label>от <input type="text" inputmode="decimal" class="settle-rule-min" placeholder="любое"></label>
            <label>до <input type="text" inputmode="decimal" class="settle-rule-max" placeholder="любое"></label>
            ${spec.unit ? `<span class="unit">${spec.unit}</span>` : ''}
        </div>`;
        if (range) {
            const tip = document.createElement('div');
            tip.className = 'rule-hint';
            tip.textContent = range;
            box.appendChild(tip);
        }
        return;
    }

    // string: чипы допустимых значений с галочками.
    const vals = spec.values || [];
    box.innerHTML = `<div class="chip-wrap">
        <span class="chip-label">равно:</span>
        <div class="chip-row">${vals.map(v =>
            `<label class="chip"><input type="checkbox" value="${v}"><span>${v}</span></label>`
        ).join('')}</div>
    </div>`;
}

// buildSettlementModel — собирает объект модели из формы.
function buildSettlementModel() {
    const population = settlePopKind === 'fixed'
        ? { kind: 'fixed', fixed: parseInt(parseDigits(document.getElementById('settlePopFixedValue').value)) }
        : {
            kind: 'random',
            min: parseInt(parseDigits(document.getElementById('settlePopMin').value)),
            max: parseInt(parseDigits(document.getElementById('settlePopMax').value)),
        };

    const rules = [];
    if (settleMode === 'complex') {
        document.querySelectorAll('#settleRulesList .settle-rule-row').forEach(row => {
            const key = row.querySelector('.settle-rule-field').value;
            const spec = fieldSpec(key);
            if (!key || !spec) return;
            const rule = { field: key };
            if (spec.type === 'bool') {
                const active = row.querySelector('.seg-btn.active');
                if (active) rule.is = active.dataset.val === 'true';
            } else if (spec.type === 'number') {
                const minV = parseDigits(row.querySelector('.settle-rule-min').value);
                const maxV = parseDigits(row.querySelector('.settle-rule-max').value);
                // Температура в форме в °C, в данных планет — в Кельвинах.
                if (key === 'temperature') {
                    if (!isNaN(minV)) rule.min = minV + 273;
                    if (!isNaN(maxV)) rule.max = maxV + 273;
                } else {
                    if (!isNaN(minV)) rule.min = minV;
                    if (!isNaN(maxV)) rule.max = maxV;
                }
            } else if (spec.type === 'string') {
                const vals = Array.from(row.querySelectorAll('.chip input:checked'))
                    .map(o => o.value);
                if (vals.length) rule.in = vals;
            }
            if (rule.min != null || rule.max != null || rule.in || rule.is != null) {
                rules.push(rule);
            }
        });
    }

    return {
        mode: settleMode,
        // В UI шанс вводится в процентах (0–100), в модели — доля 0–1.
        chance: parseFloat(document.getElementById('settleChance').value) / 100,
        population,
        rules,
    };
}

export async function generateSettlements() {
    if (!confirm('Сгенерировать поселения по модели?')) return;

    const body = buildSettlementModel();

    document.getElementById('settlementResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('settlementProgress').style.display = 'block';
    document.getElementById('settlementProgressBar').value = 0;
    document.getElementById('settlementProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelSettlementsBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-settlements', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('settlementResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('settlementProgress').style.display = 'none';
            document.getElementById('cancelSettlementsBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['settlements']) clearInterval(pollIntervals['settlements']);
        pollIntervals['settlements'] = setInterval(() => pollJob('generate_settlements', 'settlementProgress', 'settlementResult', 'cancelSettlementsBtn'), 1500);
    } catch (e) {
        document.getElementById('settlementResult').textContent = '❌ ' + e.message;
        document.getElementById('settlementProgress').style.display = 'none';
        document.getElementById('cancelSettlementsBtn').style.display = 'none';
    }
}

// generateRaceSettlements — отдельный проход: поселения рас (спека 99.2.21 §7).
// Доминанта кластера + подселение соседней расы на выбросах; крутилка —
// шанс заселения соседней расы (0–100%).
export async function generateRaceSettlements() {
    if (!confirm('Сгенерировать поселения рас?')) return;

    const neighborChance = parseFloat(document.getElementById('raceNeighborChance').value) / 100;
    if (isNaN(neighborChance) || neighborChance < 0 || neighborChance > 1) {
        document.getElementById('raceSettlementResult').textContent = '❌ Шанс: 0–100%';
        return;
    }

    document.getElementById('raceSettlementResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('raceSettlementProgress').style.display = 'block';
    document.getElementById('raceSettlementProgressBar').value = 0;
    document.getElementById('raceSettlementProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelRaceSettlementsBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-race-settlements', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ neighbor_chance: neighborChance }),
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('raceSettlementResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('raceSettlementProgress').style.display = 'none';
            document.getElementById('cancelRaceSettlementsBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['race_settlements']) clearInterval(pollIntervals['race_settlements']);
        pollIntervals['race_settlements'] = setInterval(() => pollJob('generate_race_settlements', 'raceSettlementProgress', 'raceSettlementResult', 'cancelRaceSettlementsBtn'), 1500);
    } catch (e) {
        document.getElementById('raceSettlementResult').textContent = '❌ ' + e.message;
        document.getElementById('raceSettlementProgress').style.display = 'none';
        document.getElementById('cancelRaceSettlementsBtn').style.display = 'none';
    }
}

export async function cancelGeneration(jobType) {
    if (!confirm(`Остановить генерацию?`)) return;
    try {
        const res = await fetchWithAuth(`/admin/generate-cancel?job=${jobType}`, { method: 'POST' });
        if (res.ok) {
            notifyInfo('Остановка запрошена');
            const btnId = jobType === 'generate_universe' ? 'cancelUniverseBtn' :
                          jobType === 'generate_planets' ? 'cancelPlanetsBtn' :
                          jobType === 'generate_factions' ? 'cancelFactionsBtn' :
                          jobType === 'generate_settlements' ? 'cancelSettlementsBtn' :
                          jobType === 'generate_race_settlements' ? 'cancelRaceSettlementsBtn' :
                          jobType === 'hypothesis' ? 'cancelHypothesisBtn' :
                          'cancelResourcesBtn';
            document.getElementById(btnId).style.display = 'none';
        } else {
            const text = await res.text();
            notifyError('Ошибка: ' + text);
        }
    } catch (e) {
        notifyError('Ошибка: ' + e.message);
    }
}

export async function clearSettlements() {
    if (!confirm('Удалить ВСЕ поселения? Планеты не пострадают.')) return;
    const box = document.getElementById('clearSettlementsResult');
    box.textContent = '⏳ Очистка...';
    try {
        const res = await fetchWithAuth('/admin/clear-settlements', { method: 'POST' });
        const text = await res.text();
        if (!res.ok) {
            box.textContent = '❌ Ошибка: ' + text;
            return;
        }
        const data = JSON.parse(text);
        box.textContent = '✅ Удалено поселений: ' + data.deleted;
    } catch (e) {
        box.textContent = '❌ ' + e.message;
    }
}

export async function clearUniverse() {
    if (!confirm('Удалить ВСЕ миры?')) return;
    try {
        const res = await fetchWithAuth('/admin/clear', { method: 'POST', headers: { 'Content-Type': 'application/json' } });
        if (res.ok) {
            document.getElementById('clearResult').textContent = '✅ Вселенная очищена';
            loadStats();
            loadWorlds(1);
        } else {
            const text = await res.text();
            document.getElementById('clearResult').textContent = '❌ Ошибка: ' + text;
        }
    } catch (e) {
        document.getElementById('clearResult').textContent = '❌ ' + e.message;
    }
}

// ---------- ПОДВКЛАДКИ ГЕНЕРАЦИИ (99.2.3 §2) ----------

// switchGenSubTab — переключение подвкладки раздела «Генерация».
// Сохраняется в localStorage (tabs.js хранит подвкладку тоже).
export function switchGenSubTab() {
    const sel = document.getElementById('genSubTab');
    const name = sel ? sel.value : 'stars';
    document.querySelectorAll('#tab-generation .gen-sub').forEach(d => {
        d.style.display = d.id === 'genSub-' + name ? 'block' : 'none';
    });
    if (name === 'stars') loadGenConfig();
    localStorage.setItem('adminGenSubTab', name);
}

// applyGenSubTab — применяет сохранённую подвкладку (вызывается из tabs.js
// при активации вкладки «Генерация»).
export function applyGenSubTab() {
    const saved = localStorage.getItem('adminGenSubTab') || 'stars';
    const sel = document.getElementById('genSubTab');
    if (sel) sel.value = saved;
    switchGenSubTab();
}

// showTab — переход на другую вкладку админки (кнопки-заглушки подвкладок).
export function showTab(tabId) {
    const btn = document.querySelector(`.tab-btn[data-tab="${tabId}"]`);
    if (btn) btn.click();
}

// ---------- КОНФИГ ГЕНЕРАЦИИ ЗВЁЗД (99.2.3 §4) ----------

// SPECTRAL_CLASSES — порядок спектральных классов O–Y для форм весов.
const SPECTRAL_CLASSES = ['O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y'];

// SYSTEM_TYPES — порядок типов систем/объектов для форм весов (99.2.4 §4.1).
const SYSTEM_TYPES = [
    ['single', 'Одиночная'], ['binary', 'Двойная'], ['multiple', 'Кратная (3+)'],
    ['black_hole', 'Чёрная дыра'], ['neutron', 'Нейтронная'],
    ['white_dwarf', 'Белый карлик'], ['protostar', 'Протозвезда'], ['exotic', 'Прочая экзотика'],
];

// PLANET_MEAN_KEYS — все редактируемые значения таблицы средних (99.2.3 §4.3).
const PLANET_MEAN_KEYS = [
    'O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y',
    'black_hole', 'neutron', 'white_dwarf', 'protostar', 'exotic',
    'binary_wide_factor', 'binary_close_mean', 'multiple_factor',
];

// MASS_KEYS — типы с диапазоном массы (29a §4м): спектральные классы + экзотика.
const MASS_KEYS = [
    'O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y',
    'black_hole', 'neutron', 'white_dwarf', 'protostar', 'exotic',
];

// mrId — id поля диапазона массы ('black_hole', 'min' → 'mrBlackHole_min').
function mrId(key, bound) {
    const camel = key.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
    return 'mr' + camel[0].toUpperCase() + camel.slice(1) + '_' + bound;
}

// stId — id поля веса типа системы ('black_hole' → 'stBlackHole').
// snake_case → camelCase как в mrId/pmId: иначе 'black_hole' ищет
// несуществующий 'stBlack_hole' (баг 39c: вес ЧД/белого карлика не
// заполнялся/не сохранялся).
function stId(key) {
    const camel = key.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
    return 'st' + camel[0].toUpperCase() + camel.slice(1);
}

// pmId — id поля среднего ('black_hole' → 'pmBlackHole', 'O' → 'pmO').
function pmId(key) {
    const camel = key.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
    return 'pm' + camel[0].toUpperCase() + camel.slice(1);
}

// recalcGenWeights — автоподсчёт процентов (вес/сумма×100) для обоих блоков
// весов: спектральные — от суммы обычных звёзд, типы — от суммы всех систем.
export function recalcGenWeights() {
    let specSum = 0;
    const specVals = {};
    SPECTRAL_CLASSES.forEach(cls => {
        const el = document.getElementById('sw' + cls);
        const v = el ? (parseFloat(el.value) || 0) : 0;
        specVals[cls] = v;
        specSum += v;
    });
    SPECTRAL_CLASSES.forEach(cls => {
        const pct = specSum > 0 ? (specVals[cls] / specSum * 100) : 0;
        const el = document.getElementById('sw' + cls + '_pct');
        if (el) el.textContent = pct.toFixed(1) + '%';
    });
    const specSumEl = document.getElementById('spectralWeightsSum');
    if (specSumEl) specSumEl.textContent = 'Сумма: ' + specSum.toFixed(1);

    let sysSum = 0;
    const sysVals = {};
    SYSTEM_TYPES.forEach(([key]) => {
        const el = document.getElementById(stId(key));
        const v = el ? (parseFloat(el.value) || 0) : 0;
        sysVals[key] = v;
        sysSum += v;
    });
    SYSTEM_TYPES.forEach(([key]) => {
        const pct = sysSum > 0 ? (sysVals[key] / sysSum * 100) : 0;
        const el = document.getElementById(stId(key) + '_pct');
        if (el) el.textContent = pct.toFixed(1) + '%';
    });
    const sysSumEl = document.getElementById('systemTypeWeightsSum');
    if (sysSumEl) sysSumEl.textContent = 'Сумма: ' + sysSum.toFixed(1);
}

// genConfigLoaded — защита от повторной загрузки конфига на каждый клик.
let genConfigLoaded = false;

// loadGenConfig — GET /admin/generation/config → заполняет формы весов и средних.
export async function loadGenConfig() {
    const box = document.getElementById('genConfigResult');
    try {
        const res = await fetchWithAuth('/admin/generation/config');
        if (!res.ok) {
            if (box) box.textContent = '❌ Не удалось загрузить конфиг: HTTP ' + res.status;
            return;
        }
        const cfg = await res.json();
        const spec = (cfg.star_weights && cfg.star_weights.spectral) || {};
        SPECTRAL_CLASSES.forEach(cls => {
            const el = document.getElementById('sw' + cls);
            if (el && spec[cls] != null) el.value = spec[cls];
        });
        const sys = (cfg.star_weights && cfg.star_weights.system_types) || {};
        SYSTEM_TYPES.forEach(([key]) => {
            const el = document.getElementById(stId(key));
            if (el && sys[key] != null) el.value = sys[key];
        });
        const pm = cfg.planet_means || {};
        PLANET_MEAN_KEYS.forEach(key => {
            const el = document.getElementById(pmId(key));
            if (el && pm[key] != null) el.value = pm[key];
        });
        // Диапазоны массы (29a §4м): {min, max} по типу.
        const mr = cfg.stellar_mass_ranges || {};
        MASS_KEYS.forEach(key => {
            const r = mr[key];
            if (!r) return;
            const elMin = document.getElementById(mrId(key, 'min'));
            const elMax = document.getElementById(mrId(key, 'max'));
            if (elMin && r.min != null) elMin.value = r.min;
            if (elMax && r.max != null) elMax.value = r.max;
        });
        recalcGenWeights();
        genConfigLoaded = true;
        if (box) box.textContent = '✅ Конфиг загружен (дефолты или сохранённый)';
    } catch (e) {
        if (box) box.textContent = '❌ ' + e.message;
    }
}

// saveGenConfig — PUT /admin/generation/config с валидацией на сервере
// (веса: 409 при нулевой сумме; mean: 422 при > 8 — не клампится, 99.2.3 §4.5).
export async function saveGenConfig() {
    const box = document.getElementById('genConfigResult');
    const spectral = {};
    SPECTRAL_CLASSES.forEach(cls => {
        const el = document.getElementById('sw' + cls);
        spectral[cls] = el ? (parseFloat(el.value) || 0) : 0;
    });
    const system_types = {};
    SYSTEM_TYPES.forEach(([key]) => {
        const el = document.getElementById(stId(key));
        system_types[key] = el ? (parseFloat(el.value) || 0) : 0;
    });
    const planet_means = {};
    PLANET_MEAN_KEYS.forEach(key => {
        const el = document.getElementById(pmId(key));
        planet_means[key] = el ? (parseFloat(el.value) || 0) : 0;
    });
    // Диапазоны массы (29a §4м): {min, max} по типу.
    const stellar_mass_ranges = {};
    MASS_KEYS.forEach(key => {
        const elMin = document.getElementById(mrId(key, 'min'));
        const elMax = document.getElementById(mrId(key, 'max'));
        if (elMin && elMax) {
            const minV = parseFloat(elMin.value);
            const maxV = parseFloat(elMax.value);
            if (!isNaN(minV) && !isNaN(maxV)) {
                stellar_mass_ranges[key] = { min: minV, max: maxV };
            }
        }
    });

    const body = JSON.stringify({ star_weights: { spectral, system_types }, planet_means, stellar_mass_ranges });
    try {
        const res = await fetchWithAuth('/admin/generation/config', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body,
        });
        if (!res.ok) {
            const text = await res.text();
            if (box) box.textContent = '❌ ' + (text || ('HTTP ' + res.status));
            return;
        }
        genConfigLoaded = true;
        if (box) box.textContent = '✅ Конфиг сохранён (применится при следующей генерации)';
    } catch (e) {
        if (box) box.textContent = '❌ ' + e.message;
    }
}

// ---------- ПЕРЕСЧЁТ ПЛАНЕТ (99.2.3 §5) ----------

// regeneratePlanets — POST /admin/regenerate-planets: ручной пересчёт планет
// выбранных миров по равномерному счёту (0–8), без весов и средних.
export async function regeneratePlanets() {
    const minP = parseInt(document.getElementById('regMinPlanets').value);
    const maxP = parseInt(document.getElementById('regMaxPlanets').value);
    if (isNaN(minP) || isNaN(maxP) || minP < 0 || maxP > 8 || minP > maxP) {
        document.getElementById('regenerateResult').textContent = '❌ Мин/макс планет: 0 ≤ мин ≤ макс ≤ 8';
        return;
    }
    const body = {
        min_planets: minP,
        max_planets: maxP,
        include_normal: document.getElementById('regIncludeNormal').checked,
        include_binary: document.getElementById('regIncludeBinary').checked,
        include_exotic: document.getElementById('regIncludeExotic').checked,
    };
    if (!body.include_normal && !body.include_binary && !body.include_exotic) {
        document.getElementById('regenerateResult').textContent = '❌ Выберите хотя бы одну категорию миров';
        return;
    }
    if (!confirm('Пересчитать планеты? Планеты выбранных миров будут перезаписаны.')) return;

    document.getElementById('regenerateResult').textContent = '⏳ Пересчёт запущен...';
    document.getElementById('regenerateProgress').style.display = 'block';
    document.getElementById('regenerateProgressBar').value = 0;
    document.getElementById('regenerateProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelRegenerateBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/regenerate-planets', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('regenerateResult').textContent = '❌ ' + text;
            document.getElementById('regenerateProgress').style.display = 'none';
            document.getElementById('cancelRegenerateBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['regenerate']) clearInterval(pollIntervals['regenerate']);
        pollIntervals['regenerate'] = setInterval(() => pollJob('regenerate_planets', 'regenerateProgress', 'regenerateResult', 'cancelRegenerateBtn'), 1500);
    } catch (e) {
        document.getElementById('regenerateResult').textContent = '❌ ' + e.message;
        document.getElementById('regenerateProgress').style.display = 'none';
        document.getElementById('cancelRegenerateBtn').style.display = 'none';
    }
}