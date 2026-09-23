// web/static/js/unit_scale.js
// Единая точка конверсии масштаба единицы темпа (задача «переключатель
// масштаба единицы», спека 2026-09-23-стадии-поселения §2.1). Единица модели и
// хранимая в БД — одна: «ед/сутки/млрд». Переключатель влияет ТОЛЬКО на показ
// и ввод: значение на сервер всегда уходит в хранимой единице. Множители
// «хранимое → отображаемое» живут ровно здесь (одна точка на JS; Go-дубль —
// settlement.UnitScale в internal/economy/settlement/units.go).
//
// Чистый модуль без DOM/сети на верхнем уровне: тянется из студии как глобал
// globalThis.UnitScale (классический <script> студии не умеет import) и из
// карточки поселения (import), исполняется в Node (web/frontend_unit_scale_test.go).

export const UNIT_SCALE_KEY = 'gs_unitScale';
export const DEFAULT_UNIT_SCALE = 'billion';

// UNIT_SCALE_LIST — масштабы в порядке показа; mul — множитель «хранимое (на
// млрд) → отображаемое» (billion = 1 — хранимая единица).
export const UNIT_SCALE_LIST = [
    { key: 'person',  mul: 1e-9, label: 'на 1 чел', unit: 'ед/сутки / 1 чел' },
    { key: 'kilo',    mul: 1e-6, label: 'на 1 000', unit: 'ед/сутки / 1 000' },
    { key: 'mega',    mul: 1e-3, label: 'на 10⁶',   unit: 'ед/сутки / 10⁶' },
    { key: 'billion', mul: 1,    label: 'на млрд',  unit: 'ед/сутки / 10⁹' }
];

// normNum — снимает мусор плавающей точки от умножения/деления на степень
// десяти (0.6 / 1e-9 → 600000000, а не 599999999.99999994), не теряя порядок
// малых значений (toPrecision, а не округление до знаков).
function normNum(v) {
    if (typeof v !== 'number' || !isFinite(v)) return v;
    return Number(v.toPrecision(12));
}

// unitScaleDef — описание масштаба; неизвестный ключ (в т.ч. пустой) — дефолт.
export function unitScaleDef(key) {
    return UNIT_SCALE_LIST.find(s => s.key === key) ||
        UNIT_SCALE_LIST.find(s => s.key === DEFAULT_UNIT_SCALE);
}

// unitScaleMul — множитель «хранимое → отображаемое».
export function unitScaleMul(key) {
    return unitScaleDef(key).mul;
}

// storedToDisplay — показ: «ед/сутки/млрд» → значение в масштабе key.
export function storedToDisplay(stored, key) {
    return normNum(stored * unitScaleMul(key));
}

// displayToStored — ввод: значение в масштабе key → «ед/сутки/млрд».
export function displayToStored(display, key) {
    return normNum(display / unitScaleMul(key));
}

// readUnitScale — выбранный масштаб из общего localStorage (ключ gs_unitScale);
// нет ключа/неизвестное значение/нет storage → дефолт «на млрд».
export function readUnitScale(storage) {
    let v = null;
    try { v = storage ? storage.getItem(UNIT_SCALE_KEY) : null; } catch (e) { v = null; }
    return unitScaleDef(v).key;
}

// Глобал для классического скрипта студии (ES-import ему недоступен).
if (typeof globalThis !== 'undefined') {
    globalThis.UnitScale = {
        UNIT_SCALE_KEY, DEFAULT_UNIT_SCALE, UNIT_SCALE_LIST,
        unitScaleDef, unitScaleMul, storedToDisplay, displayToStored, readUnitScale
    };
}
