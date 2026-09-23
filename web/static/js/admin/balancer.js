// web/static/js/admin/balancer.js
// Вкладка «Балансировка» (спека 99.2.17 §7): загрузка кривой/эталонов/
// серверный sample, drag узлов и bend-ромбов, Shift+клик — добавить узел,
// ПКМ — удалить, зум колесом по Y + пан пустого места, «По узлам»,
// лог/линейная, стрелочки ▲/▼ за краем (клик — центрирование), кнопки
// Сохранить/Отмена/Сброс, тултипы, информационная панель.
// Масштабирование Y — чисто клиентская механика (§7 п.3): клиент хранит
// view; после пана по X запрашивает повторный sample (клиент НЕ дублирует
// evaluateCurve на постоянных данных — только на live-preview при drag).
import { fetchWithAuth } from './auth.js';
import { notifyError, notifySuccess } from '../ui/toast.js';
import { render, hitTest, dataToScreen, screenToData, evaluateCurveClient, fmtR } from './balancerCanvas.js';
import { gameDateUtc } from '../game_date.js';

// Диапазоны X компонент (для начального обзора и клампа X при drag).
// Единицы: жара и холод — °C (решение создателя 2026-09-15), гравитация — g,
// радиация — rad, голод — нагрузка (сило-часы, §7.1).
const COMPONENTS = {
    heat: { xMin: 30, xMax: 4000, unit: '°C' },
    cold: { xMin: -273.15, xMax: 14.85, unit: '°C' },
    gravity: { xMin: 0, xMax: 10, unit: 'g' },
    radiation: { xMin: 0, xMax: 100, unit: 'rad' },
    hunger: { xMin: 0, xMax: 8760, unit: 'нагрузка' },
};

// RACE_COMPONENTS — компоненты расового балансировщика (§7.2): голод —
// глобальная, в расовом режиме недоступен.
const RACE_COMPONENTS = ['heat', 'cold', 'gravity', 'radiation'];

// EFFECT_COMPONENTS — компоненты-эффекты: только у них порог = нулевой
// префикс кривой (§4.4/§7.1) → маркер порога на графике.
const EFFECT_COMPONENTS = ['hunger'];

// X_LABELS — подпись единиц оси X внизу графика.
const X_LABELS = {
    heat: 'жара, °C',
    cold: 'холод, °C',
    gravity: 'гравитация, g',
    radiation: 'радиация, rad',
    hunger: 'нагрузка, сило-ч',
};

// Ось Y — в процентах (R×100, решение создателя 2026-09-15): видимый
// диапазон хранится в %, кламп [−100, +100]. Y_MIN — низ лог-шкалы:
// R = 1e-14 доли = 1e-12%. Данные (nodes/sampled/etalons) остаются в долях R.
const Y_MIN = 1e-12;
const Y_MAX = 100;
const SAMPLE_N = 250;

let canvas = null;
let bound = false;
let dirty = false;

const state = {
    component: 'heat',
    raceID: '',          // '' = Люди (глобальная); иначе — раса (99.2.23 §4.3)
    raceMeta: null,      // мета расы: {reproduction, card_hash, factory_hash, stale, active_equals_factory, birth_rate_coefficient, natural_rate_per_sec}
    factoryNodes: [],    // factory-кривая текущей компоненты (оверлей пунктиром)
    factoryBends: [],
    showFactory: false,  // переключатель «показать заводскую»
    nodes: [],
    bends: [],
    baseNodes: [],
    baseBends: [],
    etalons: [],
    sampled: [],
    xs: [],
    presets: [],       // пресеты текущей компоненты (итерация 7): [{name, updated_at}]
    presetActive: '',  // имя активного пресета
    recovery: null,    // скаляр recovery эффект-компоненты (hunger, §7.1)
    view: { logY: true, yMin: Y_MIN, yMax: Y_MAX, xMin: 30, xMax: 4000 },
};

// drag — текущее перетаскивание (null, node, bend, pan).
let drag = null;

// dragInfo — живая подпись при drag (UX-правка 2026-09-15): значения видны
// ВО ВРЕМЯ движения узла/ромбика. Заполняется в updateDragInfo (вызывается
// из redraw), рисуется плашкой в canvas; null вне drag.
let dragInfo = null;

// sampleTimer — debounce повторного sample после пана по X.
let sampleTimer = null;

export function initBalancer() {
    canvas = document.getElementById('balancerCanvas');
    if (!canvas) return;
    if (bound) return;
    bound = true;

    bindScale();
    bindButtons();
    bindPresets();
    bindRaceControls();
    bindCanvas();
    document.getElementById('balancerComponent').addEventListener('change', onComponentChange);
    document.getElementById('balancerRace').addEventListener('change', onRaceChange);
    document.getElementById('balancerGoSettlement').addEventListener('click', (e) => {
        e.preventDefault();
        // Настройки населения — на вкладке «Основное» (tab-main).
        activateMainTab();
    });

    loadRaceList();
    loadComponent();
}

// activateMainTab — переключение на вкладку «Основное» (настройки населения).
function activateMainTab() {
    document.querySelectorAll('.tab-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.tab === 'tab-main'));
    document.querySelectorAll('.tab-pane').forEach(p =>
        p.classList.toggle('active', p.id === 'tab-main'));
    localStorage.setItem('adminActiveTab', 'tab-main');
}

// ==================== Загрузка данных ====================

async function loadComponent() {
    state.component = document.getElementById('balancerComponent').value;
    // Голод — глобальная компонента (§7.2): в расовом режиме недоступен —
    // не уходим в расовый эндпоинт, возвращаемся на «жару».
    if (state.raceID && !RACE_COMPONENTS.includes(state.component)) {
        state.component = 'heat';
        document.getElementById('balancerComponent').value = 'heat';
    }
    const r = COMPONENTS[state.component];
    state.view.xMin = r.xMin;
    state.view.xMax = r.xMax;
    // Линейная шкала — по умолчанию; Y авто-фокусируется на узлы кривой
    // (fitViewToNodes) после загрузки, НЕ от нижней границы лог-шкалы.
    state.view.logY = false;
    state.view.yMin = Y_MIN;
    state.view.yMax = Y_MAX;
    setScaleUI();
    if (state.raceID) {
        // Раса (99.2.23 §4.3): active-кривая + мета + factory-оверлей;
        // эталоны и пресеты — слой глобального балансировщика, для рас не в скоупе.
        await Promise.all([loadRaceCurve(), loadRaceFactory()]);
        state.etalons = [];
        state.recovery = null;
    } else {
        await Promise.all([loadCurve(), loadEtalons(), loadPresets()]);
    }
    updateRaceUI();
    dirty = false;
    fitViewToNodes();
    sampleVisible();
}

// ==================== Раса (99.2.23 §4.3) ====================

// loadRaceList — GET /admin/race-balancer/status → селектор расы
// («Люди (глобальная)» + 50 рас из каталога).
async function loadRaceList() {
    const sel = document.getElementById('balancerRace');
    if (!sel) return;
    try {
        const res = await fetchWithAuth('/admin/race-balancer/status');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const s = await res.json();
        for (const r of (s.races || [])) {
            const opt = document.createElement('option');
            opt.value = r.race_id;
            opt.textContent = r.name;
            sel.appendChild(opt);
        }
    } catch (e) {
        console.error('loadRaceList:', e);
    }
}

// onRaceChange — смена расы: перезагрузка кривой (active) + factory-оверлей.
function onRaceChange() {
    state.raceID = document.getElementById('balancerRace').value;
    state.showFactory = false;
    const ov = document.getElementById('balancerFactoryOverlay');
    if (ov) ov.checked = false;
    loadComponent();
}

// loadRaceCurve — GET /admin/race-balancer/curve?race_id=&component=:
// active-кривая + мета (reproduction, stale, active==factory, ручки расчёта).
async function loadRaceCurve() {
    try {
        const res = await fetchWithAuth(`/admin/race-balancer/curve?race_id=${state.raceID}&component=${state.component}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const c = await res.json();
        state.nodes = c.nodes;
        state.bends = c.bends;
        state.baseNodes = c.nodes.map(n => ({ ...n }));
        state.baseBends = c.bends.slice();
        state.raceMeta = c;
        const repro = document.getElementById('balancerReproduction');
        if (repro) repro.value = c.reproduction;
        updateReproCalc();
        renderRaceStale();
    } catch (e) {
        console.error('loadRaceCurve:', e);
        notifyError('Не удалось загрузить кривую расы');
    }
}

// loadRaceFactory — GET /admin/race-balancer/factory?race_id=&component=:
// заводская кривая для оверлея (пунктир).
async function loadRaceFactory() {
    try {
        const res = await fetchWithAuth(`/admin/race-balancer/factory?race_id=${state.raceID}&component=${state.component}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const f = await res.json();
        state.factoryNodes = f.nodes || [];
        state.factoryBends = f.bends || [];
    } catch (e) {
        console.error('loadRaceFactory:', e);
        state.factoryNodes = [];
        state.factoryBends = [];
    }
}

// updateRaceUI — видимость расовых контролов: reproduction + кнопки +
// оверлей — для расы; пресеты — только для «Люди (глобальная)».
function updateRaceUI() {
    const isRace = !!state.raceID;
    const controls = document.getElementById('balancerRaceControls');
    const presetsRow = document.getElementById('balancerPresetSelect').closest('.compact-row');
    const resetBtn = document.getElementById('balancerResetBtn');
    if (controls) controls.style.display = isRace ? 'flex' : 'none';
    if (presetsRow) presetsRow.style.display = isRace ? 'none' : 'flex';
    if (resetBtn) resetBtn.style.display = isRace ? 'none' : 'inline-block';
    // Голод — только глобальный режим (§7.2): при выбранной расе опция выключена.
    const compSel = document.getElementById('balancerComponent');
    if (compSel) {
        const hungerOpt = compSel.querySelector('option[value="hunger"]');
        if (hungerOpt) hungerOpt.disabled = isRace;
    }
    updateRecoveryUI();
    if (!isRace) renderRaceStale();
}

// updateRecoveryUI — поле скаляра recovery видно только для эффект-компоненты
// в глобальном режиме (у расы голода нет, §7.1/§7.2).
function updateRecoveryUI() {
    const row = document.getElementById('balancerRecoveryRow');
    if (!row) return;
    const show = !state.raceID && state.component === 'hunger';
    row.style.display = show ? 'block' : 'none';
}

// setRecoveryInput — выставить значение поля recovery из store.
function setRecoveryInput(v) {
    const inp = document.getElementById('balancerRecovery');
    if (inp && v != null) inp.value = v;
}

// renderRaceStale — плашка «заводские настройки устарели» (card_hash ≠ хэшу
// текущей карточки): жёлтая плашка + кнопка «Обновить заводские».
function renderRaceStale() {
    const el = document.getElementById('balancerRaceStale');
    if (!el) return;
    if (state.raceID && state.raceMeta && state.raceMeta.stale) {
        el.style.display = 'block';
        el.innerHTML = '⚠️ Заводские настройки устарели: карточка расы менялась после генерации. ' +
            '<button class="btn secondary" style="margin-left:8px;" onclick="window.balancerGenerateFactory()">Обновить заводские</button>';
    } else {
        el.style.display = 'none';
        el.innerHTML = '';
    }
}

// updateReproCalc — живой расчёт «рост в оптимуме при k=2: ×N/год» под полем
// reproduction: нетто = reproduction·(1−k)·R_ест; рост = (1−нетто)^год − 1.
function updateReproCalc() {
    const el = document.getElementById('balancerReproCalc');
    if (!el) return;
    const input = document.getElementById('balancerReproduction');
    const m = state.raceMeta;
    if (!input || !m) { el.textContent = ''; return; }
    const repro = parseFloat(input.value);
    if (!(repro > 0)) { el.textContent = 'reproduction > 0'; return; }
    const netto = repro * (1 - m.birth_rate_coefficient) * m.natural_rate_per_sec;
    const growth = Math.pow(1 - netto, 365 * 24 * 3600) - 1;
    if (Math.abs(growth) < 1e-6) {
        el.textContent = 'рост в оптимуме: ~0 (статика)';
    } else if (growth > 0) {
        el.textContent = `рост в оптимуме: ×${(1 + growth).toFixed(2)}/год (+${(growth * 100).toFixed(1)}%/год)`;
    } else {
        el.textContent = `убыль в оптимуме: −${(Math.abs(growth) * 100).toFixed(1)}%/год`;
    }
}

// bindRaceControls — кнопки расового режима: reproduction (живой расчёт +
// сохранение по change), «Сгенерировать из карточки», «Вернуть заводские»,
// оверлей заводской.
function bindRaceControls() {
    const repro = document.getElementById('balancerReproduction');
    if (repro) {
        repro.addEventListener('input', updateReproCalc);
        repro.addEventListener('change', saveRaceReproduction);
    }
    document.getElementById('balancerRaceGenerateBtn').addEventListener('click', generateRaceFactory);
    document.getElementById('balancerRaceResetFactoryBtn').addEventListener('click', resetRaceToFactory);
    document.getElementById('balancerFactoryOverlay').addEventListener('change', (e) => {
        state.showFactory = e.target.checked;
        redraw();
    });
}

// generateRaceFactory — «Сгенерировать из карточки»: factory пересчитан из
// текущей карточки; active НЕ трогается; card_hash обновляется.
window.balancerGenerateFactory = generateRaceFactory;
async function generateRaceFactory() {
    if (!confirm('Заводские настройки будут пересчитаны из карточки; текущая (настроенная) кривая не изменится. Продолжить?')) return;
    try {
        const res = await fetchWithAuth(`/admin/race-balancer/generate?race_id=${state.raceID}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: '{}',
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось перегенерировать заводские');
            return;
        }
        notifySuccess('Заводские настройки пересчитаны из карточки');
        await loadRaceFactory();
        await loadRaceCurve();
        sampleVisible();
    } catch (e) {
        console.error('generateRaceFactory:', e);
        notifyError('Ошибка перегенерации заводских');
    }
}

// resetRaceToFactory — «Вернуть заводские»: active = factory (deep copy).
async function resetRaceToFactory() {
    if (!confirm('Текущая кривая будет заменена заводской. Продолжить?')) return;
    try {
        const res = await fetchWithAuth(`/admin/race-balancer/reset-factory?race_id=${state.raceID}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: '{}',
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось вернуть заводские');
            return;
        }
        notifySuccess('Кривая возвращена на заводскую');
        dirty = false;
        await loadRaceCurve();
        sampleVisible();
    } catch (e) {
        console.error('resetRaceToFactory:', e);
        notifyError('Ошибка возврата заводских');
    }
}

// saveRaceReproduction — PUT /admin/race-balancer/reproduction?race_id=:
// сохранение множителя размножения (валидация > 0).
async function saveRaceReproduction() {
    const input = document.getElementById('balancerReproduction');
    const repro = parseFloat(input.value);
    if (!(repro > 0)) {
        notifyError('reproduction должен быть > 0');
        return;
    }
    try {
        const res = await fetchWithAuth(`/admin/race-balancer/reproduction?race_id=${state.raceID}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ reproduction: repro }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сохранить reproduction');
            return;
        }
        notifySuccess('Множитель размножения сохранён');
        await loadRaceCurve();
    } catch (e) {
        console.error('saveRaceReproduction:', e);
        notifyError('Ошибка сохранения reproduction');
    }
}

async function loadCurve() {
    try {
        const res = await fetchWithAuth(`/admin/balancer/curve?component=${state.component}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const c = await res.json();
        state.nodes = c.nodes;
        state.bends = c.bends;
        state.baseNodes = c.nodes.map(n => ({ ...n }));
        state.baseBends = c.bends.slice();
        // Скаляр recovery (только эффект-компонента, §7.1): отсутствует у прочих.
        state.recovery = (c.recovery != null) ? c.recovery : null;
        setRecoveryInput(state.recovery);
        updateRecoveryUI();
    } catch (e) {
        console.error('loadCurve:', e);
        notifyError('Не удалось загрузить кривую');
    }
}

async function loadEtalons() {
    try {
        const res = await fetchWithAuth(`/admin/balancer/etalons?component=${state.component}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const e = await res.json();
        state.etalons = e.etalons;
    } catch (err) {
        console.error('loadEtalons:', err);
        state.etalons = [];
    }
    updateInfo();
}

// ==================== Пресеты (итерация 7, спека §7 п.8) ====================

// loadPresets — GET /admin/balancer/presets?component=… → список + активный.
async function loadPresets() {
    try {
        const res = await fetchWithAuth(`/admin/balancer/presets?component=${state.component}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const p = await res.json();
        state.presets = p.presets;
        state.presetActive = p.active;
    } catch (e) {
        console.error('loadPresets:', e);
        state.presets = [];
        state.presetActive = '';
    }
    renderPresetSelect();
}

// renderPresetSelect — перезаполнение дропдауна пресетов; default —
// без кнопки «Удалить» (заводской, удаляется только reset-default).
function renderPresetSelect() {
    const sel = document.getElementById('balancerPresetSelect');
    if (!sel) return;
    sel.innerHTML = '';
    for (const p of state.presets) {
        const opt = document.createElement('option');
        opt.value = p.name;
        const date = p.updated_at ? gameDateUtc(p.updated_at) : '';
        opt.title = `обновлён: ${date}`;
        opt.textContent = p.name === state.presetActive ? `${p.name} (активный)` : p.name;
        sel.appendChild(opt);
    }
    if (!state.presets.length) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = '— нет пресетов —';
        sel.appendChild(opt);
    }
    const del = document.getElementById('balancerPresetDeleteBtn');
    if (del) {
        const isDefault = sel.value === 'default';
        del.disabled = isDefault;
        del.title = isDefault ? 'default — заводской; используйте «Вернуть заводской»' : '';
    }
}

// bindPresets — кнопки панели пресетов.
function bindPresets() {
    document.getElementById('balancerPresetSelect').addEventListener('change', renderPresetSelect);
    document.getElementById('balancerPresetApplyBtn').addEventListener('click', applyPreset);
    document.getElementById('balancerPresetDeleteBtn').addEventListener('click', deletePreset);
    document.getElementById('balancerPresetSaveBtn').addEventListener('click', savePresetAs);
    document.getElementById('balancerPresetResetDefaultBtn').addEventListener('click', resetDefaultPreset);
}

// savePresetAs — «Сохранить как…»: сначала PUT текущих экранных узлов
// (пресет сохраняет ТО, ЧТО НА ЭКРАНЕ, включая несохранённые правки),
// затем POST presets — «сохранил = сразу применил» (спек §7 п.8).
async function savePresetAs() {
    const sel = document.getElementById('balancerPresetSelect');
    let name = sel && sel.value ? sel.value : '';
    const input = prompt('Имя пресета (до 32 символов):', name);
    if (input === null) return;
    name = input.trim();
    if (!name) {
        notifyError('Имя пресета не может быть пустым');
        return;
    }
    const exists = state.presets.some(p => p.name === name);
    if (exists && !confirm(`Пресет «${name}» уже есть — перезаписать?`)) return;

    // Экран → store: PUT тех же данных (несохранённые правки попадут в пресет).
    if (dirty) {
        const ok = await putCurrentCurve();
        if (!ok) return;
    }
    try {
        const res = await fetchWithAuth('/admin/balancer/presets', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ component: state.component, name }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сохранить пресет');
            return;
        }
        notifySuccess(`Пресет «${name}» сохранён и применён`);
        await loadPresets();
    } catch (e) {
        console.error('savePresetAs:', e);
        notifyError('Ошибка сохранения пресета');
    }
}

// curvePayload — тело PUT: кривая + (для глобального голода) скаляр recovery
// (§7.1). Для рас скаляр не отправляется.
function curvePayload() {
    const payload = { component: state.component, nodes: state.nodes, bends: state.bends };
    if (!state.raceID && state.component === 'hunger') {
        const inp = document.getElementById('balancerRecovery');
        const rec = inp ? parseFloat(inp.value) : NaN;
        if (Number.isFinite(rec) && rec >= 0) payload.recovery = rec;
    }
    return payload;
}

// putCurrentCurve — PUT текущих экранных узлов/bends (вынесено из saveCurve
// для повторного использования). Для расы — PUT /admin/race-balancer/curve
// (active-кривая); для «Люди» — глобальный store 99.2.17. true при успехе.
async function putCurrentCurve() {
    try {
        const url = state.raceID
            ? `/admin/race-balancer/curve?race_id=${state.raceID}&component=${state.component}`
            : '/admin/balancer/curve';
        const res = await fetchWithAuth(url, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(curvePayload()),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сохранить кривую');
            return false;
        }
        const c = await res.json();
        state.nodes = c.nodes;
        state.bends = c.bends;
        state.baseNodes = c.nodes.map(n => ({ ...n }));
        state.baseBends = c.bends.slice();
        if (state.raceID && c.reproduction != null) {
            state.raceMeta = c;
            const repro = document.getElementById('balancerReproduction');
            if (repro) repro.value = c.reproduction;
            updateReproCalc();
            renderRaceStale();
        }
        if (!state.raceID && c.recovery != null) {
            state.recovery = c.recovery;
            setRecoveryInput(c.recovery);
        }
        dirty = false;
        return true;
    } catch (e) {
        console.error('putCurrentCurve:', e);
        notifyError('Ошибка сохранения кривой');
        return false;
    }
}

// applyPreset — «Применить»: POST presets/apply → кривая перерисовывается.
async function applyPreset() {
    const sel = document.getElementById('balancerPresetSelect');
    const name = sel ? sel.value : '';
    if (!name) return;
    try {
        const res = await fetchWithAuth('/admin/balancer/presets/apply', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ component: state.component, name }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось применить пресет');
            return;
        }
        notifySuccess(`Пресет «${name}» применён`);
        dirty = false;
        await loadCurve();
        await loadPresets();
        sampleVisible();
    } catch (e) {
        console.error('applyPreset:', e);
        notifyError('Ошибка применения пресета');
    }
}

// deletePreset — «Удалить»: DELETE (default — кнопка неактивна).
async function deletePreset() {
    const sel = document.getElementById('balancerPresetSelect');
    const name = sel ? sel.value : '';
    if (!name || name === 'default') return;
    if (!confirm(`Удалить пресет «${name}»?`)) return;
    try {
        const res = await fetchWithAuth('/admin/balancer/presets', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ component: state.component, name }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось удалить пресет');
            return;
        }
        notifySuccess(`Пресет «${name}» удалён`);
        await loadPresets();
    } catch (e) {
        console.error('deletePreset:', e);
        notifyError('Ошибка удаления пресета');
    }
}

// resetDefaultPreset — «Вернуть заводской»: POST presets/reset-default.
async function resetDefaultPreset() {
    if (!confirm('Кривая и пресет default будут заменены кодовыми дефолтами. Продолжить?')) return;
    try {
        const res = await fetchWithAuth('/admin/balancer/presets/reset-default', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ component: state.component }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось вернуть заводской пресет');
            return;
        }
        notifySuccess('Кривая возвращена на заводской дефолт');
        dirty = false;
        await loadCurve();
        await loadPresets();
        sampleVisible();
    } catch (e) {
        console.error('resetDefaultPreset:', e);
        notifyError('Ошибка возврата заводского пресета');
    }
}

// xsGrid — равномерная сетка X по видимому диапазону для sample.
function xsGrid() {
    const xs = [];
    for (let i = 0; i < SAMPLE_N; i++) {
        xs.push(state.view.xMin + (state.view.xMax - state.view.xMin) * i / (SAMPLE_N - 1));
    }
    return xs;
}

async function sampleVisible() {
    const xs = xsGrid();
    state.xs = xs;
    try {
        const q = state.raceID ? `?race_id=${state.raceID}` : '';
        const res = await fetchWithAuth(`/admin/balancer/curve/sample${q}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ component: state.component, xs }),
        });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const s = await res.json();
        state.sampled = s.points;
    } catch (e) {
        console.error('sample:', e);
    }
    redraw();
}

function redraw() {
    if (!canvas) return;
    updateDragInfo();
    render(canvas, state.view, {
        nodes: state.nodes,
        bends: state.bends,
        sampled: state.sampled,
        etalons: state.etalons,
        xs: state.xs,
        dirty: dirty,
        dragInfo: dragInfo,
        xLabel: X_LABELS[state.component],
        threshold: EFFECT_COMPONENTS.includes(state.component) ? zeroPrefix(state.nodes) : null,
        factoryNodes: state.showFactory ? state.factoryNodes : [],
        factoryBends: state.showFactory ? state.factoryBends : [],
    });
    updateSaveLabel();
    updateInfo();
}

function updateSaveLabel() {
    const btn = document.getElementById('balancerSaveBtn');
    if (btn) btn.textContent = dirty ? 'Сохранить*' : 'Сохранить';
}

// ==================== Кнопки и шкала ====================

function bindScale() {
    const seg = document.getElementById('balancerScaleSeg');
    seg.querySelectorAll('.seg-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            state.view.logY = btn.dataset.scale === 'log';
            // Лог-шкала работает только для положительных: если автофокус
            // увёл yMin ≤ 0 (линейный), в лог-режиме прижимаем к низу.
            if (state.view.logY && state.view.yMin <= 0) state.view.yMin = Y_MIN;
            setScaleUI();
            redraw();
        });
    });
}

function setScaleUI() {
    const seg = document.getElementById('balancerScaleSeg');
    seg.querySelectorAll('.seg-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.scale === (state.view.logY ? 'log' : 'linear')));
}

function bindButtons() {
    document.getElementById('balancerFitBtn').addEventListener('click', fitToNodes);
    document.getElementById('balancerResetViewBtn').addEventListener('click', resetView);
    document.getElementById('balancerSaveBtn').addEventListener('click', saveCurve);
    document.getElementById('balancerCancelBtn').addEventListener('click', cancelCurve);
    document.getElementById('balancerResetBtn').addEventListener('click', resetCurve);
}

// fitViewToNodes — авто-фокус оси Y (в %) на узлы кривой с запасом ±20%
// (1 деление, если все узлы равны), кламп [−100, +100]. Чисто клиентская
// арифметика (решение создателя 2026-09-15: линейная сфокусирована на
// границах данных, не от нижней границы лог-шкалы).
function fitViewToNodes() {
    if (!state.nodes.length) return;
    let mn = Infinity, mx = -Infinity;
    for (const n of state.nodes) {
        if (n.y < mn) mn = n.y;
        if (n.y > mx) mx = n.y;
    }
    let lo = mn * 100;
    let hi = mx * 100;
    let pad = (hi - lo) * 0.2;
    if (pad <= 0) pad = 1;
    lo -= pad;
    hi += pad;
    if (lo < -100) lo = -100;
    if (hi > 100) hi = 100;
    if (lo >= hi) { lo = -100; hi = 100; }
    state.view.yMin = lo;
    state.view.yMax = hi;
}

// fitToNodes — кнопка «По узлам»: авто-фокус + перерисовка.
function fitToNodes() {
    fitViewToNodes();
    redraw();
}

// resetView — кнопка «Сброс вида»: вернуть начальный обзор — полный
// X-диапазон компоненты + авто-фокус Y на узлы (как при загрузке
// компоненты). Переключатель лог/линейная НЕ трогаем — сбрасывается
// только масштаб. X изменился → повторный sample.
function resetView() {
    const r = COMPONENTS[state.component];
    state.view.xMin = r.xMin;
    state.view.xMax = r.xMax;
    fitViewToNodes(); // Y — авто-фокус на текущие узлы (кламп [−100, 100] %)
    requestSample();
    redraw();
}

async function saveCurve() {
    try {
        const url = state.raceID
            ? `/admin/race-balancer/curve?race_id=${state.raceID}&component=${state.component}`
            : '/admin/balancer/curve';
        const res = await fetchWithAuth(url, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(curvePayload()),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сохранить кривую');
            return;
        }
        const c = await res.json();
        state.nodes = c.nodes;
        state.bends = c.bends;
        state.baseNodes = c.nodes.map(n => ({ ...n }));
        state.baseBends = c.bends.slice();
        if (state.raceID && c.reproduction != null) {
            state.raceMeta = c;
            const repro = document.getElementById('balancerReproduction');
            if (repro) repro.value = c.reproduction;
            updateReproCalc();
            renderRaceStale();
        }
        if (!state.raceID && c.recovery != null) {
            state.recovery = c.recovery;
            setRecoveryInput(c.recovery);
        }
        dirty = false;
        notifySuccess(state.raceID ? 'Кривая расы сохранена' : 'Кривая сохранена');
        sampleVisible();
    } catch (e) {
        console.error('saveCurve:', e);
        notifyError('Ошибка сохранения кривой');
    }
}

async function cancelCurve() {
    // Отмена = GET curve + перерисовка (выброс локальных правок).
    if (state.raceID) {
        await loadRaceCurve();
    } else {
        await loadCurve();
    }
    dirty = false;
    sampleVisible();
    redraw();
}

async function resetCurve() {
    if (state.raceID) {
        // Раса: «Вернуть заводские» (active = factory) — отдельная кнопка.
        resetRaceToFactory();
        return;
    }
    if (!confirm(`Сбросить кривую «${state.component}» на дефолты?`)) return;
    try {
        const res = await fetchWithAuth('/admin/balancer/curve/reset', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ component: state.component }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сбросить кривую');
            return;
        }
        const c = await res.json();
        state.nodes = c.nodes;
        state.bends = c.bends;
        state.baseNodes = c.nodes.map(n => ({ ...n }));
        state.baseBends = c.bends.slice();
        state.recovery = (c.recovery != null) ? c.recovery : null;
        setRecoveryInput(state.recovery);
        updateRecoveryUI();
        dirty = false;
        notifySuccess('Кривая сброшена на дефолты');
        fitViewToNodes(); // авто-фокус на дефолтные узлы (решение 2026-09-15)
        sampleVisible();
    } catch (e) {
        console.error('resetCurve:', e);
        notifyError('Ошибка сброса кривой');
    }
}

// ==================== События мыши ====================

function canvasCoords(e) {
    const rect = canvas.getBoundingClientRect();
    return {
        x: (e.clientX - rect.left) * canvas.width / rect.width,
        y: (e.clientY - rect.top) * canvas.height / rect.height,
    };
}

function bindCanvas() {
    canvas.addEventListener('mousedown', onMouseDown);
    canvas.addEventListener('mousemove', onMouseMove);
    canvas.addEventListener('mouseup', onMouseUp);
    canvas.addEventListener('mouseleave', () => { drag = null; redraw(); });
    canvas.addEventListener('wheel', onWheel, { passive: false });
    canvas.addEventListener('contextmenu', onContextMenu);
    canvas.addEventListener('click', onClick);
}

function onMouseDown(e) {
    const p = canvasCoords(e);
    const hit = hitTest(canvas, state.view, {
        nodes: state.nodes, bends: state.bends, etalons: state.etalons,
    }, p.x, p.y);

    if (e.button === 0) {
        if (hit && hit.type === 'node') {
            drag = { kind: 'node', index: hit.index, last: p };
        } else if (hit && hit.type === 'bend') {
            drag = { kind: 'bend', index: hit.index, last: p };
        } else if (hit && hit.type === 'indicator') {
            // Прыжок к объекту: центрируем Y на целевом объекте.
            centerOnIndicator(hit);
            return;
        } else if (hit && hit.type === 'etalon') {
            // Эталон не draggable — ничего.
            drag = null;
        } else {
            // Пан пустого места.
            const d = screenToData(canvas, state.view, p.x, p.y);
            drag = { kind: 'pan', last: p, dataStart: d, viewStart: { ...state.view } };
        }
        if (drag) canvas.style.cursor = 'grabbing';
    } else if (e.button === 2) {
        // ПКМ — удаление узла (в onContextMenu).
        return;
    }
}

function onMouseMove(e) {
    const p = canvasCoords(e);
    if (!drag) {
        updateTooltip(p);
        return;
    }
    if (drag.kind === 'pan') {
        const d = screenToData(canvas, state.view, p.x, p.y);
        const v = drag.viewStart;
        // Сдвиг окна: захваченная точка данных следует за курсором.
        state.view.xMin = v.xMin - (d.x - drag.dataStart.x);
        state.view.xMax = v.xMax - (d.x - drag.dataStart.x);
        if (state.view.logY) {
            const logShift = Math.log10(drag.dataStart.y) - Math.log10(Math.max(d.y, Y_MIN));
            state.view.yMin = clampY(Math.pow(10, Math.log10(v.yMin) + logShift));
            state.view.yMax = clampY(Math.pow(10, Math.log10(v.yMax) + logShift));
        } else {
            state.view.yMin = clampY(v.yMin - (d.y - drag.dataStart.y));
            state.view.yMax = clampY(v.yMax - (d.y - drag.dataStart.y));
        }
        // X изменился → повторный sample (кривая пересчитывается серверно).
        requestSample();
        redraw();
        return;
    }
    if (drag.kind === 'node') {
        const d = screenToData(canvas, state.view, p.x, p.y);
        // screenToData возвращает y в % — узел двигается в долях R (÷100).
        moveNode(drag.index, d.x, d.y / 100);
        dirty = true;
        redraw();
    } else if (drag.kind === 'bend') {
        const d = screenToData(canvas, state.view, p.x, p.y);
        moveBend(drag.index, d.y / 100);
        dirty = true;
        redraw();
    }
    drag.last = p;
}

function onMouseUp(e) {
    if (drag && drag.kind === 'node') {
        magnetToEtalon(drag.index);
    }
    drag = null;
    canvas.style.cursor = '';
    redraw();
}

// moveNode — перемещение узла (X кламп к соседям/диапазону, Y к [0, 0.999)).
function moveNode(i, x, y) {
    const r = COMPONENTS[state.component];
    const prevX = i > 0 ? state.nodes[i - 1].x + 0.0001 : r.xMin;
    const nextX = i < state.nodes.length - 1 ? state.nodes[i + 1].x - 0.0001 : r.xMax;
    state.nodes[i].x = clamp(x, prevX, nextX);
    state.nodes[i].y = clamp(y, 0, 0.9989);
}

// moveBend — вертикальный drag ромбика: m → k = 2·ln(1/m − 1), кламп ±10 (§7 п.5).
function moveBend(i, yUser) {
    const y0 = state.nodes[i].y;
    const y1 = state.nodes[i + 1].y;
    if (Math.abs(y1 - y0) < 1e-12) return; // плоский сегмент — изгиб не определён
    let m = (yUser - y0) / (y1 - y0);
    if (m <= 0.01) m = 0.01;
    if (m >= 0.99) m = 0.99;
    let k = 2 * Math.log(1 / m - 1);
    if (k > 10) k = 10;
    if (k < -10) k = -10;
    state.bends[i] = k;
}

// magnetToEtalon — магнит эталона (3 px экранных): снаппится только y.
function magnetToEtalon(i) {
    const node = state.nodes[i];
    const nsp = dataToScreen(canvas, state.view, node.x, node.y * 100);
    for (const e of state.etalons) {
        const esp = dataToScreen(canvas, state.view, e.x, e.y * 100);
        if (Math.hypot(nsp.x - esp.x, nsp.y - esp.y) <= 3) {
            if (Math.abs(node.y - e.y) > 1e-12) {
                node.y = e.y;
                notifySuccess(`Прилипло к эталону: ${e.label}`);
            }
            break;
        }
    }
}

// onClick — Shift+клик: добавить узел (x = клик, y = текущее значение кривой).
function onClick(e) {
    if (!e.shiftKey) return;
    const p = canvasCoords(e);
    const d = screenToData(canvas, state.view, p.x, p.y);
    const r = COMPONENTS[state.component];
    if (d.x < r.xMin || d.x > r.xMax) {
        notifyError('Новый узел вне диапазона компоненты');
        return;
    }
    if (state.nodes.length >= 16) {
        notifyError('Максимум 16 узлов на компоненту');
        return;
    }
    const y = evaluateCurveClient(state.nodes, state.bends, d.x);
    // Вставка в сортированную позицию.
    let idx = state.nodes.length;
    for (let i = 0; i < state.nodes.length; i++) {
        if (d.x < state.nodes[i].x) { idx = i; break; }
    }
    state.nodes.splice(idx, 0, { x: d.x, y: Math.max(0, Math.min(0.9989, y)) });
    state.bends.splice(Math.max(0, idx - 1), 0, 0);
    dirty = true;
    redraw();
}

// onContextMenu — ПКМ по узлу: удалить (минимум 3).
function onContextMenu(e) {
    e.preventDefault();
    const p = canvasCoords(e);
    const hit = hitTest(canvas, state.view, {
        nodes: state.nodes, bends: state.bends, etalons: state.etalons,
    }, p.x, p.y);
    if (!hit || hit.type !== 'node') return;
    if (state.nodes.length <= 3) {
        notifyError('Минимум 3 узла — удалить нельзя');
        return;
    }
    state.nodes.splice(hit.index, 1);
    state.bends.splice(hit.index, 1); // сегмент после удалённого узла
    dirty = true;
    redraw();
}

// onWheel — зум по Y (в %) вокруг позиции курсора; кламп [−100, 100].
function onWheel(e) {
    e.preventDefault();
    const p = canvasCoords(e);
    const d = screenToData(canvas, state.view, p.x, p.y);
    const factor = e.deltaY < 0 ? 0.8 : 1.25; // вверх — приближение
    if (state.view.logY) {
        const yc = Math.log10(Math.max(d.y, Y_MIN));
        const lyMin = Math.log10(state.view.yMin);
        const lyMax = Math.log10(state.view.yMax);
        let lo = yc - (yc - lyMin) * factor;
        let hi = yc + (lyMax - yc) * factor;
        if (hi - lo < 1e-12) return;
        state.view.yMin = clampY(Math.pow(10, lo));
        state.view.yMax = clampY(Math.pow(10, hi));
    } else {
        const yc = d.y;
        let lo = yc - (yc - state.view.yMin) * factor;
        let hi = yc + (state.view.yMax - yc) * factor;
        if (hi - lo < 1e-9) return;
        state.view.yMin = clampY(lo);
        state.view.yMax = clampY(hi);
    }
    // X-зум синхронно с Y (решение создателя 2026-09-15): тот же factor,
    // центр — позиция курсора (d.x). Кламп к диапазону компоненты (пан
    // может увести окно за границы — зум не должен) + защита от схлопывания.
    const r = COMPONENTS[state.component];
    const minW = (r.xMax - r.xMin) * 1e-4;
    const xLo = Math.max(d.x - (d.x - state.view.xMin) * factor, r.xMin);
    const xHi = Math.min(d.x + (state.view.xMax - d.x) * factor, r.xMax);
    if (xHi - xLo >= minW && (xLo !== state.view.xMin || xHi !== state.view.xMax)) {
        state.view.xMin = xLo;
        state.view.xMax = xHi;
        requestSample(); // X изменился → серверная оцифровка на новом окне
    }
    redraw();
}

// centerOnIndicator — прыжок к объекту за краем: центрируем Y (в %) на нём.
function centerOnIndicator(hit) {
    const obj = hit.kind === 'node' ? state.nodes[hit.index] : state.etalons[hit.index];
    if (!obj) return;
    const target = obj.y * 100; // данные в долях → ось в %
    if (state.view.logY) {
        const spanDec = Math.log10(state.view.yMax) - Math.log10(state.view.yMin);
        let c = Math.log10(Math.max(target, Y_MIN));
        state.view.yMin = clampY(Math.pow(10, c - spanDec / 2));
        state.view.yMax = clampY(Math.pow(10, c + spanDec / 2));
    } else {
        const span = state.view.yMax - state.view.yMin;
        state.view.yMin = clampY(target - span / 2);
        state.view.yMax = clampY(target + span / 2);
    }
    redraw();
}

function requestSample() {
    clearTimeout(sampleTimer);
    sampleTimer = setTimeout(sampleVisible, 150);
}

// ==================== Тултип и панель ====================

function updateTooltip(p) {
    const hit = hitTest(canvas, state.view, {
        nodes: state.nodes, bends: state.bends, etalons: state.etalons,
    }, p.x, p.y);
    if (!hit) {
        canvas.title = '';
        return;
    }
    if (hit.type === 'node') {
        const n = state.nodes[hit.index];
        canvas.title = `X = ${fmtX(n.x)}, R = ${fmtR(n.y * 100)}, t₅₀ = ${tTime(n.y)}`;
    } else if (hit.type === 'bend') {
        canvas.title = `изгиб k = ${state.bends[hit.index].toFixed(2)}`;
    } else if (hit.type === 'etalon') {
        const e = state.etalons[hit.index];
        canvas.title = `эталон: ${e.label} (R = ${fmtR(e.y * 100)})`;
    } else if (hit.type === 'indicator') {
        const obj = hit.kind === 'node' ? state.nodes[hit.index] : state.etalons[hit.index];
        const edge = hit.dir === 'down' ? 'верхним' : 'нижним';
        const what = hit.kind === 'node' ? `узел ${fmtX(obj.x)}` : `эталон ${obj.label || ''}`;
        canvas.title = `${what}, R = ${fmtR(obj.y * 100)} — за ${edge} краем`;
    }
}

// updateDragInfo — живая подпись при drag: актуальные значения видны во
// время движения (нативный тултип на время drag не обновляется — ранний
// return в onMouseMove, поэтому плашка рисуется в canvas). Форматы те же,
// что в updateTooltip (fmtX/fmtR/tTime — не дублируем).
function updateDragInfo() {
    if (!drag) { dragInfo = null; return; }
    if (drag.kind === 'node') {
        const n = state.nodes[drag.index];
        dragInfo = { kind: 'node', text: `X = ${fmtX(n.x)}, R = ${fmtR(n.y * 100)}, t₅₀ = ${tTime(n.y)}` };
    } else if (drag.kind === 'bend') {
        dragInfo = { kind: 'bend', text: `изгиб k = ${state.bends[drag.index].toFixed(2)}` };
    } else {
        dragInfo = null;
    }
}

// updateInfo — информационная панель: для каждого эталона R_кривая/R_маркер,
// t₅₀ и t₁₀₀₀ (guard: R ≤ 0 → «не вымирает», R ≥ 1 → «мгновенно»).
function updateInfo() {
    const el = document.getElementById('balancerInfo');
    if (!el) return;
    if (!state.etalons.length || !state.nodes.length) {
        el.innerHTML = '';
        return;
    }
    const rows = state.etalons.map(e => {
        const rc = evaluateCurveClient(state.nodes, state.bends, e.x);
        const ratio = e.y > 0 ? rc / e.y : null;
        return `<div style="margin-bottom:4px;">` +
            `<span style="color:#f87171;">×</span> <b>${e.label}</b> (${fmtX(e.x)}): ` +
            `R_кривая = ${fmtR(rc * 100)}, R_маркер = ${fmtR(e.y * 100)}` +
            (ratio !== null ? `, отношение = ${ratio.toFixed(2)}` : '') +
            ` · t₅₀ = ${tTime(rc)}, t₁₀₀₀ = ${tTime(rc, 1000)}` +
            `</div>`;
    });
    el.innerHTML = `<b>Эталоны (цели):</b><br>${rows.join('')}`;
}

// tTime — время вымирания (сек) для p0: ln(p0)/|ln(1−R)|. R ≤ 0 → «не
// вымирает», R ≥ 1 → «мгновенно».
function tTime(r, p0 = 2) {
    if (r <= 0) return 'не вымирает';
    if (r >= 1) return 'мгновенно';
    return fmtDur(Math.log(p0) / Math.abs(Math.log(1 - r)));
}

function fmtDur(sec) {
    if (sec < 90) return `${Math.round(sec)} сек`;
    if (sec < 3600) return `${(sec / 60).toFixed(1)} мин`;
    if (sec < 86400) return `${(sec / 3600).toFixed(1)} ч`;
    return `${(sec / 86400).toFixed(1)} сут`;
}

function fmtX(x) {
    return Math.abs(x) >= 1000 ? x.toFixed(0) : String(Number(x.toPrecision(4)));
}

function clamp(v, lo, hi) {
    return Math.max(lo, Math.min(hi, v));
}

// clampY — кламп видимого диапазона Y (в %): [−100, +100] (решение 2026-09-15).
function clampY(v) {
    return clamp(v, -100, 100);
}

// zeroPrefix — порог включения (§4.4): X последнего подряд идущего узла
// кривой с y = 0 (нулевой префикс); нулевого префикса нет → 0.
function zeroPrefix(nodes) {
    let t = 0;
    for (const n of (nodes || [])) {
        if (n.y !== 0) break;
        t = n.x;
    }
    return t;
}

// onComponentChange — смена компоненты (из HTML change).
function onComponentChange() {
    loadComponent();
}