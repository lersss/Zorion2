// web/static/js/route/route_legend.js
// Попап-легенда мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §4.9): открывается кнопкой «Легенда»,
// содержит азбуку доски ЧЕТЫРЬМЯ СВЁРНУТЫМИ группами — Ориентиры / Поле / Секторы /
// Метки курса. Каждая позиция нарисована мини-фигурой тем же кодом, что доска
// (route_figures.drawLegendFigure), + название + строка смысла. Путь/предпросмотр и
// вскрытое содержимое в легенду не входят (§4.9) — название находки в карточке
// сектора. Статичная справка: не раскрывает состояние партии, чисел нет.
// Закрытие: «✕» / тап по фону / Esc; звук открытия/закрытия не играется.
import * as C from './route_config.js';
import { drawLegendFigure } from './route_figures.js';

const $ = (id) => document.getElementById(id);
const TILE = 48;
let open = false;

export function isLegendOpen() { return open; }

// makeFigure — мини-фигура позиции на отдельном canvas (DPR ≤2, подложка #0b1220).
function makeFigure(id) {
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const cv = document.createElement('canvas');
    cv.className = 'legend-fig';
    cv.width = Math.round(TILE * dpr);
    cv.height = Math.round(TILE * dpr);
    cv.style.width = TILE + 'px';
    cv.style.height = TILE + 'px';
    const ctx = cv.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, TILE, TILE);
    drawLegendFigure(ctx, id, { x: 1, y: 1, w: TILE - 2, h: TILE - 2 });
    return cv;
}

function legendText(it) {
    const text = document.createElement('div');
    text.className = 'legend-text';
    const name = document.createElement('div');
    name.className = 'legend-name';
    name.textContent = it.name;
    const meaning = document.createElement('div');
    meaning.className = 'legend-meaning';
    meaning.textContent = it.meaning;
    text.appendChild(name);
    text.appendChild(meaning);
    return text;
}

// sigRow — ряд гулкости: три фигуры (Тихий / Ровный / Гулкий) в одной строке (§4.9/§12.11).
function sigRow() {
    const row = document.createElement('div');
    row.className = 'legend-sig-row';
    for (const [id, cap] of [['sig_quiet', C.SIG_LABELS[0]], ['sig_mid', C.SIG_LABELS[1]], ['sig_loud', C.SIG_LABELS[2]]]) {
        const cell = document.createElement('div');
        cell.className = 'legend-sig-cell';
        cell.appendChild(makeFigure(id));
        const label = document.createElement('span');
        label.className = 'legend-sig-cap';
        label.textContent = cap;
        cell.appendChild(label);
        row.appendChild(cell);
    }
    return row;
}

function buildRow(it) {
    const row = document.createElement('div');
    row.className = 'legend-row';
    row.appendChild(it.sigRow ? sigRow() : makeFigure(it.id));
    row.appendChild(legendText(it));
    return row;
}

function toggleGroup(title, body) {
    const openNow = title.getAttribute('aria-expanded') === 'true';
    title.setAttribute('aria-expanded', openNow ? 'false' : 'true');
    body.hidden = openNow;
}

// buildLegend — дом попапа: четыре свёрнутые группы азбуки из LEGEND_ITEMS (§4.9).
export function buildLegend() {
    const list = $('legend-list');
    if (!list) return;
    list.innerHTML = '';
    for (const g of C.LEGEND_GROUPS) {
        const items = C.LEGEND_ITEMS.filter((it) => it.group === g.id);
        if (!items.length) continue;
        const sec = document.createElement('section');
        sec.className = 'legend-group';

        const title = document.createElement('button');
        title.type = 'button';
        title.className = 'legend-group-title';
        title.setAttribute('aria-expanded', 'false');
        title.appendChild(document.createTextNode(g.title));
        const chevron = document.createElement('span');
        chevron.className = 'legend-chevron';
        chevron.setAttribute('aria-hidden', 'true');
        title.appendChild(chevron);
        sec.appendChild(title);

        const body = document.createElement('div');
        body.className = 'legend-group-body';
        body.hidden = true;
        for (const it of items) body.appendChild(buildRow(it));
        sec.appendChild(body);

        title.addEventListener('click', () => toggleGroup(title, body));
        list.appendChild(sec);
    }
}

export function openLegend() {
    if (open) return;
    const el = $('legend-popup');
    if (!el) return;
    el.style.display = 'flex';
    open = true;
    const close = $('legend-close');
    if (close) close.focus();
}

export function closeLegend() {
    if (!open) return;
    const el = $('legend-popup');
    if (el) el.style.display = 'none';
    open = false;
    const toggle = $('legend-toggle');
    if (toggle) toggle.focus();
}

// initLegend — дом + обработчики (кнопка/«✕»/фон/Esc). Зовётся один раз.
export function initLegend() {
    buildLegend();
    const toggle = $('legend-toggle');
    if (toggle) toggle.onclick = openLegend;
    const close = $('legend-close');
    if (close) close.onclick = closeLegend;
    const el = $('legend-popup');
    if (el) el.addEventListener('pointerdown', (e) => { if (e.target === el) closeLegend(); });
    document.addEventListener('keydown', (e) => {
        if (open && e.key === 'Escape') { e.preventDefault(); closeLegend(); }
    });
}
