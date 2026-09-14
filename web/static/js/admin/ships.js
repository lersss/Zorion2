// web/static/js/admin/ships.js
// Вкладка «Корабли» (спека 99.2.15 §6): «теневой» генератор деталей —
// таблица по категориям, генерация 1/10/перегенерация/удаление и
// предпросмотр-гейт целостности сборки И9:
//  1. деталь отдельно, крупно;
//  2. сетка ≥ 12 случайных кораблей в ≥ 6 цветах палитры, каждый в двух
//     масштабах — детальный 200×200 и игровой (~34 px, полёт shipSize*3.2);
//  3. стрип по категориям: все формы категории в сборке на трёх корпусах
//     (нейтральный + min + max габарита);
//  4. сэмплер worst-case: max-нос/крылья/двигатели/хвост × min-корпус.
// Данные стрипа и worst-case приходят с сервера (admin_ship_parts.go):
// семантика параметров генератора живёт на бэкенде.
import { fetchWithAuth } from './auth.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

const CATEGORY_LABEL = {
    hull: 'Корпус', nose: 'Нос', wings: 'Крылья', engine: 'Двигатели', tail: 'Хвост',
};

// Цвет «одиночной» детали крупно и общий цвет стрипа — нейтральный серый
// (палитра — свойство схемы, деталь сама по себе бесцветна, И1).
const PREVIEW_COLOR = '#94a3b8';

const WORST_LABEL = {
    nose_max_hull_min: 'max-нос × min-корпус',
    wings_max_hull_min: 'max-крылья × min-корпус',
    engine_max_hull_min: 'max-двигатели × min-корпус',
    tail_max_hull_min: 'max-хвост × min-корпус',
};

let shipsBound = false;
let selectedPartId = null;
let state = {
    parts: [],
    byId: new Map(),
    byCategory: new Map(),
    palette: [],
    layerOrder: [],
    hulls: {},        // {neutral, min, max} — корпуса стрипа (с сервера)
    neutral: {},      // нейтральные придатки для стрипа (с сервера)
    worstCases: [],   // сборки у границ габаритов (с сервера)
};

function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
}

function fmtParams(params) {
    if (!params || Object.keys(params).length === 0) return '—';
    try { return JSON.stringify(params); } catch (e) { return '—'; }
}

// ==================== ЗАГРУЗКА ====================

export async function loadShips() {
    try {
        const res = await fetchWithAuth('/admin/ship-parts');
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось загрузить каталог деталей');
            return;
        }
        const data = await res.json();
        state.parts = Array.isArray(data.parts) ? data.parts : [];
        state.palette = Array.isArray(data.palette) ? data.palette : [];
        state.layerOrder = Array.isArray(data.layerOrder) ? data.layerOrder : [];
        state.hulls = data.hulls || {};
        state.neutral = data.neutral_parts || {};
        state.worstCases = Array.isArray(data.worst_cases) ? data.worst_cases : [];

        state.byId = new Map();
        state.byCategory = new Map();
        for (const p of state.parts) {
            state.byId.set(p.id, p);
            if (!state.byCategory.has(p.category)) state.byCategory.set(p.category, []);
            state.byCategory.get(p.category).push(p);
        }
        if (selectedPartId && !state.byId.has(selectedPartId)) selectedPartId = null;

        render();
    } catch (e) {
        console.error('loadShips error:', e);
        notifyError('Ошибка соединения при загрузке деталей');
    }
}

// ==================== ГЕНЕРАЦИЯ / УДАЛЕНИЕ ====================

// generateShips — «Сгенерировать 1» / «Сгенерировать 10» для категории.
export async function generateShips(category, count) {
    try {
        const res = await fetchWithAuth('/admin/ship-parts/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ category, count }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось сгенерировать детали');
            return;
        }
        notifySuccess(`Сгенерировано: ${CATEGORY_LABEL[category]} +${count}`);
        loadShips();
    } catch (e) {
        console.error('generateShips error:', e);
        notifyError('Ошибка соединения при генерации');
    }
}

// regenerateCategory — перегенерация категории (10 форм, ON CONFLICT).
export async function regenerateCategory(category) {
    if (!confirm(`Перегенерировать категорию «${CATEGORY_LABEL[category] || category}»? Старые формы заменятся на 10 новых.`)) return;
    try {
        const res = await fetchWithAuth('/admin/ship-parts/regenerate-category', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ category }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось перегенерировать категорию');
            return;
        }
        notifySuccess(`Категория «${CATEGORY_LABEL[category] || category}» перегенерирована (10 форм)`);
        loadShips();
    } catch (e) {
        console.error('regenerateCategory error:', e);
        notifyError('Ошибка соединения при перегенерации');
    }
}

export async function deleteShipPart(id) {
    if (!confirm('Удалить деталь?')) return;
    try {
        const res = await fetchWithAuth('/admin/ship-parts/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            notifyError(err.error || 'Не удалось удалить деталь');
            return;
        }
        notifySuccess('Деталь удалена');
        loadShips();
    } catch (e) {
        console.error('deleteShipPart error:', e);
        notifyError('Ошибка соединения при удалении');
    }
}

// ==================== КОМПОЗИЦИЯ SVG ====================

// composeSVG — собранный корабль (спека §5.1): порядок слоёв из layerOrder,
// цвет схемы — в style="color:" корневого <svg> (инвариант И1: акценты с
// явным fill цвет схемы не трогает). sizePx — ширина/высота на экране
// (viewBox 200×200 растягивается векторно).
function composeSVG(parts, color, sizePx) {
    const frags = [];
    for (const cat of state.layerOrder) {
        const p = state.byId.get(parts[cat]);
        frags.push(`<g>${p ? p.svg : ''}</g>`);
    }
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="${sizePx}" height="${sizePx}" style="color:${color}">${frags.join('')}</svg>`;
}

// singlePartSVG — деталь отдельно, крупно (гейт И9 п.1).
function singlePartSVG(p, sizePx) {
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="${sizePx}" height="${sizePx}" style="color:${PREVIEW_COLOR}"><g>${p.svg}</g></svg>`;
}

// shuffle — перемешивание Фишера—Йетса (для цветов сетки).
function shuffle(arr) {
    const a = [...arr];
    for (let i = a.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        [a[i], a[j]] = [a[j], a[i]];
    }
    return a;
}

// randomShips — ≥ 12 случайных собранных кораблей в ≥ 6 цветах палитры
// (гейт И9 п.2): случайная деталь каждой категории, цвет — из палитры.
function randomShips() {
    const cats = state.layerOrder.filter(c => (state.byCategory.get(c) || []).length > 0);
    if (cats.length === 0) return [];
    const colors = shuffle(state.palette).slice(0, Math.min(8, state.palette.length));
    const N = Math.max(12, 16);
    const ships = [];
    for (let i = 0; i < N; i++) {
        const parts = {};
        for (const cat of cats) {
            const list = state.byCategory.get(cat);
            parts[cat] = list[Math.floor(Math.random() * list.length)].id;
        }
        ships.push({ parts, color: colors[i % colors.length] });
    }
    return ships;
}

// stripRows — стрип по категориям (гейт И9 п.4): каждая форма категории в
// сборке на трёх корпусах (нейтральный + min + max габарита); для самого
// корпуса — формы на нейтральных придатках.
function stripRows() {
    const rows = [];
    for (const cat of state.layerOrder) {
        const list = state.byCategory.get(cat) || [];
        if (list.length === 0) continue;
        if (cat === 'hull') {
            rows.push({
                label: 'Корпус · формы',
                ships: list.map(p => ({ parts: { ...state.neutral, hull: p.id }, color: PREVIEW_COLOR })),
            });
            continue;
        }
        const hullVariants = [
            ['neutral', 'корпус нейтральный'],
            ['min', 'корпус min'],
            ['max', 'корпус max'],
        ];
        for (const [hkey, hlabel] of hullVariants) {
            const hullId = state.hulls[hkey];
            if (!hullId) continue;
            rows.push({
                label: `${CATEGORY_LABEL[cat]} · ${hlabel}`,
                ships: list.map(p => ({ parts: { ...state.neutral, hull: hullId, [cat]: p.id }, color: PREVIEW_COLOR })),
            });
        }
    }
    return rows;
}

// ==================== РЕНДЕР ====================

function render() {
    renderCategories();
    renderDetail();
    renderPreview();
}

function renderCategories() {
    const root = document.getElementById('shipsCategories');
    if (!root) return;
    if (state.parts.length === 0) {
        root.innerHTML = `<p class="hint">Каталог пуст. Сгенерируйте первые детали: кнопки «＋1»/«＋10» под заголовками категорий.</p>`;
        return;
    }
    let html = '';
    for (const cat of state.layerOrder) {
        const list = state.byCategory.get(cat) || [];
        html += `
            <div style="margin-top:14px;">
                <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">
                    <strong>${CATEGORY_LABEL[cat] || cat}</strong>
                    <span class="hint">${list.length} форм</span>
                    <button class="btn-small" onclick="generateShips('${cat}', 1)">＋1</button>
                    <button class="btn-small" onclick="generateShips('${cat}', 10)">＋10</button>
                    <button class="btn-small secondary" onclick="regenerateCategory('${cat}')">↻ Перегенерировать</button>
                </div>
                <div class="ship-tiles">
                    ${list.map(p => `
                        <div class="ship-tile" data-id="${esc(p.id)}" title="${esc(p.id)}">
                            ${singlePartSVG(p, 80)}
                            <div class="tile-name">${esc(p.name)}</div>
                            <div class="tile-id">${esc(fmtParams(p.params))}</div>
                        </div>`).join('')}
                </div>
            </div>`;
    }
    root.innerHTML = html;
    // Выбор детали — делегирование клика по плиткам.
    root.querySelectorAll('.ship-tile').forEach(tile => {
        tile.addEventListener('click', () => selectShipPart(tile.dataset.id));
    });
}

function renderDetail() {
    const root = document.getElementById('shipsDetail');
    if (!root) return;
    if (!selectedPartId) {
        root.innerHTML = `<p class="hint">Выберите деталь в списке выше — она покажется здесь крупно (проверка формы и акцентов).</p>`;
        return;
    }
    const p = state.byId.get(selectedPartId);
    if (!p) {
        root.innerHTML = `<p class="hint">Деталь не найдена.</p>`;
        return;
    }
    root.innerHTML = `
        <div style="display:flex;gap:16px;align-items:center;flex-wrap:wrap;">
            <div style="background:#0f172a;border:1px solid #334155;border-radius:10px;padding:12px;">
                ${singlePartSVG(p, 260)}
            </div>
            <div style="max-width:420px;">
                <div style="font-weight:600;">${esc(p.name)}</div>
                <div class="hint" style="word-break:break-all;">id: ${esc(p.id)}</div>
                <div class="hint">категория: ${CATEGORY_LABEL[p.category] || p.category}</div>
                <div class="hint" style="margin-top:6px;">параметры: ${esc(fmtParams(p.params))}</div>
                <div class="hint">создана: ${esc(p.created_at || '—')}</div>
                <button class="btn-small danger" style="margin-top:10px;" onclick="deleteShipPart('${esc(p.id)}')">🗑 Удалить</button>
            </div>
        </div>`;
}

function renderPreview() {
    const root = document.getElementById('shipsPreview');
    if (!root) return;
    const cats = state.layerOrder.filter(c => (state.byCategory.get(c) || []).length > 0);
    if (cats.length < 5) {
        root.innerHTML = `<p class="hint">Каталог неполный (${cats.length}/5 категорий). Сгенерируйте детали — предпросмотр соберётся автоматически.</p>`;
        return;
    }

    const grid = randomShips();
    const strips = stripRows();
    const worst = state.worstCases;

    let html = '';

    // 1. Сетка случайных кораблей (п.2–3): детальный 200×200 + игровой масштаб.
    html += `<h3 style="margin-top:6px;">Сетка случайных кораблей (${grid.length}, ≥ 6 цветов)</h3>
        <div class="ship-grid">
            ${grid.map(s => `
                <div class="ship-cell">
                    <div class="ship-two">
                        <div>${composeSVG(s.parts, s.color, 200)}</div>
                        <div title="Масштаб карты (полёт shipSize*3.2 ≈ 34 px)">${composeSVG(s.parts, s.color, 34)}</div>
                    </div>
                    <div class="cell-cap">цвет ${esc(s.color)}</div>
                </div>`).join('')}
        </div>`;

    // 2. Стрип по категориям (п.4): все формы на трёх корпусах.
    html += `<h3 style="margin-top:18px;">Стрип по категориям (формы × корпусы нейтральный/min/max)</h3>
        <div style="font-size:0.75rem;color:#94a3b8;margin-bottom:4px;">Горизонтальная прокрутка — каждая форма категории в сборке, чтобы увидеть «битую» деталь отдельно.</div>`;
    for (const row of strips) {
        html += `<div class="strip-row">
            <div class="strip-label">${esc(row.label)}</div>
            <div class="strip-ships">
                ${row.ships.map(s => `<div class="strip-ship">${composeSVG(s.parts, s.color, 110)}</div>`).join('')}
            </div>
        </div>`;
    }

    // 3. Сэмплер крайних сочетаний (п.5): max-придаток × min-корпус.
    html += `<h3 style="margin-top:18px;">Сэмплер крайних сочетаний (worst-case, границы §3.1)</h3>`;
    if (worst.length === 0) {
        html += `<p class="hint">Нет данных (нужны корпуса и придатки в каталоге).</p>`;
    } else {
        for (let i = 0; i < worst.length; i++) {
            const w = worst[i];
            const color = state.palette[i % state.palette.length] || PREVIEW_COLOR;
            html += `<div class="wc-row">
                <div class="wc-label">${esc(WORST_LABEL[w.key] || w.key)}</div>
                <div>${composeSVG(w.parts, color, 200)}</div>
                <div title="Масштаб карты">${composeSVG(w.parts, color, 34)}</div>
            </div>`;
        }
    }

    root.innerHTML = html;
}

// selectShipPart — выбор детали для крупного показа (гейт И9 п.1).
export function selectShipPart(id) {
    selectedPartId = id || null;
    renderDetail();
}

// initShips — ленивая загрузка вкладки (вызывается из tabs.js при активации).
export function initShips() {
    if (shipsBound) return;
    shipsBound = true;
    loadShips();
}