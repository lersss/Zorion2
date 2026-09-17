// web/static/js/dashboard/encyclopedia.js
// Энциклопедия дашборда (спека 86a §5): 6 разделов + статистика «открыто X
// из Y» (§4.2). Источники: /api/encyclopedia/races (сервер), getSpectralInfo/
// exoticStarReference (panel.js — единый справочник 41a), PLANET_TYPE_INFO
// (фронт-реестр §5.3.1), журнал (journal.js). Открытие — смешанное (§6):
// звёзды/планеты — базовое сразу; расы — силуэты «—» до встречи (решение
// гейта 1, §12); экзотика и карточки типов планет — по первой встрече.

import { getSpectralInfo, exoticStarReference } from '../modal/panel.js';
import { starTypeLabel } from '../modal/utils.js';
import { PLANET_TYPE_INFO, PLANET_TYPE_ORDER } from './planet_types.js';
import { get, EXOTIC_STAR_TYPES } from './journal.js';
import { renderRacesSection } from './races_section.js';

// Знаменатели «открыто X из Y» (спека 86a §4.2): константы фронт-реестров.
const RACES_TOTAL = 60; // каталог config/races.json
const EXOTIC_TOTAL = EXOTIC_STAR_TYPES.length; // 4
const PLANETS_TOTAL = PLANET_TYPE_ORDER.length; // 9

// SPECTRAL_CLASSES — порядок 10 классов O–Y (getSpectralInfo).
const SPECTRAL_CLASSES = ['O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y'];

// ==================== СТАТИСТИКА (§4.2) ====================

// renderStats — «Изучено систем», «Полётов», «открыто X из Y» по разделам.
// Заготовки (ресурсы/корабли/лор) в «открыто X из Y» не участвуют.
export function renderStats(container, journal) {
    const openRaces = new Set([...(journal.metRaces || []), 'humans']).size; // люди — по умолчанию
    const rows = [
        ['Изучено систем', String((journal.visitedWorlds || []).length)],
        ['Полётов', String(journal.flights || 0)],
        ['Расы', `${openRaces} из ${RACES_TOTAL}`],
        ['Экзотические звёзды', `${(journal.seenStarTypes || []).length} из ${EXOTIC_TOTAL}`],
        ['Типы планет', `${(journal.seenPlanetTypes || []).length} из ${PLANETS_TOTAL}`],
        ['Ресурсы', 'заготовка'],
        ['Корабли', 'заготовка'],
        ['Лор', 'заготовка'],
    ];
    container.innerHTML = `
        <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(160px, 1fr)); gap:10px;">
            ${rows.map(([label, value]) => `
                <div style="background:#1e293b; border:1px solid #334155; border-radius:10px; padding:10px; text-align:center;">
                    <div style="font-size:1.3rem; font-weight:600;">${value}</div>
                    <div style="font-size:0.75rem; color:#94a3b8; margin-top:2px;">${label}</div>
                </div>`).join('')}
        </div>`;
}

// ==================== РАЗДЕЛЫ ====================

// renderEncyclopedia — 6 разделов: расы, звёзды, планеты, ресурсы, корабли, лор.
export function renderEncyclopedia(container, racesData, journal) {
    container.innerHTML = `
        <div class="enc-section">
            <h3>👽 Расы <span style="font-size:0.75rem; color:#94a3b8;">(${new Set([...(journal.metRaces || []), 'humans']).size} из ${RACES_TOTAL})</span></h3>
            <div id="enc-races"></div>
        </div>
        <div class="enc-section">
            <h3>⭐ Звёзды</h3>
            <div id="enc-stars"></div>
        </div>
        <div class="enc-section">
            <h3>🪐 Планеты <span style="font-size:0.75rem; color:#94a3b8;">(${(journal.seenPlanetTypes || []).length} из ${PLANETS_TOTAL})</span></h3>
            <div id="enc-planets"></div>
        </div>
        <div class="enc-section">
            <h3>🧪 Ресурсы / сырьё</h3>
            <div id="enc-resources"></div>
        </div>
        <div class="enc-section">
            <h3>🚀 Корабли / оборудование</h3>
            <div id="enc-ships"></div>
        </div>
        <div class="enc-section">
            <h3>📜 Лор-термины / существа</h3>
            <div id="enc-lore"></div>
        </div>`;

    renderRacesSection(container.querySelector('#enc-races'), racesData, journal);
    renderStars(container.querySelector('#enc-stars'), journal);
    renderPlanets(container.querySelector('#enc-planets'), journal);
    renderResourcesStub(container.querySelector('#enc-resources'));
    renderShipsStub(container.querySelector('#enc-ships'));
    renderLoreStub(container.querySelector('#enc-lore'));
}

// ==================== ЗВЁЗДЫ (§5.2) ====================

// renderStars — 10 спектральных классов сразу (полный справочник) + 4
// экзотических типа по первой встрече (journal.seenStarTypes).
function renderStars(container, journal) {
    const seen = new Set(journal.seenStarTypes || []);
    const rows = SPECTRAL_CLASSES.map(spec => {
        const info = getSpectralInfo(spec);
        return `<tr>
            <td><strong>${spec}</strong></td>
            <td>${info.type}</td>
            <td>${info.color}</td>
            <td>${info.temp || '—'}</td>
            <td>${info.radius}</td>
            <td>${info.luminosity}</td>
            <td>${info.age}</td>
            <td style="font-size:0.8rem; color:#94a3b8;">${info.description}</td>
        </tr>`;
    }).join('');

    const exoticCards = EXOTIC_STAR_TYPES.map(type => {
        if (!seen.has(type)) {
            return `<div style="background:#1e293b; border:1px dashed #334155; border-radius:10px; padding:10px; color:#64748b; font-size:0.8rem; text-align:center;">
                Экзотический объект не встречался</div>`;
        }
        const ref = exoticStarReference(type) || {};
        return `<div style="background:#1e293b; border:1px solid #334155; border-radius:10px; padding:10px; font-size:0.85rem;">
            <strong>${starTypeLabel(type) || type}</strong>
            <div style="color:#94a3b8; margin-top:4px;">T: ${ref.temperature || '—'}</div>
            <div style="color:#94a3b8;">Цвет: ${ref.color || '—'}</div>
            <div style="color:#94a3b8;">Радиус: ${ref.radius || '—'}</div>
            <div style="color:#94a3b8;">Светимость: ${ref.luminosity || '—'}</div>
        </div>`;
    }).join('');

    container.innerHTML = `
        <div style="font-size:0.8rem; color:#94a3b8; margin-bottom:6px;">Спектральные классы — общий справочник (виден сразу)</div>
        <div style="overflow-x:auto;">
            <table style="width:100%; border-collapse:collapse; font-size:0.85rem;">
                <thead><tr style="color:#94a3b8; text-align:left;">
                    <th style="padding:4px;">Класс</th><th style="padding:4px;">Тип</th><th style="padding:4px;">Цвет</th>
                    <th style="padding:4px;">T</th><th style="padding:4px;">Радиус</th><th style="padding:4px;">Светимость</th>
                    <th style="padding:4px;">Срок жизни</th><th style="padding:4px;">Описание</th>
                </tr></thead>
                <tbody>${rows}</tbody>
            </table>
        </div>
        <div style="font-size:0.8rem; color:#94a3b8; margin:10px 0 6px;">Экзотические объекты — открываются первой встречей</div>
        <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(200px, 1fr)); gap:8px;">${exoticCards}</div>`;
}

// ==================== ПЛАНЕТЫ (§5.3) ====================

// renderPlanets — 9 типов сразу (имя, иконка, однострочник); карточка типа —
// по первому посещению (journal.seenPlanetTypes).
function renderPlanets(container, journal) {
    const seen = new Set(journal.seenPlanetTypes || []);
    const cards = PLANET_TYPE_ORDER.map(type => {
        const info = PLANET_TYPE_INFO[type];
        const opened = seen.has(type);
        return `
            <div style="background:#1e293b; border:1px solid ${opened ? '#3b82f6' : '#334155'}; border-radius:10px; padding:10px;">
                <div style="display:flex; align-items:center; gap:8px;">
                    <span style="font-size:1.2rem;">${info.icon}</span>
                    <strong style="font-size:0.95rem;">${type}</strong>
                    ${opened ? '<span style="font-size:0.7rem; color:#60a5fa;">открыт</span>' : ''}
                </div>
                <div style="font-size:0.8rem; color:#94a3b8; margin-top:4px;">${info.desc}</div>
                ${opened && info.detail ? `<div style="font-size:0.8rem; color:#cbd5e1; margin-top:6px; border-top:1px solid #334155; padding-top:6px;">${info.detail}</div>` : ''}
            </div>`;
    }).join('');
    container.innerHTML = `
        <div style="font-size:0.8rem; color:#94a3b8; margin-bottom:6px;">Типы планет — общий реестр; карточка типа открывается первым посещением</div>
        <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(220px, 1fr)); gap:8px;">${cards}</div>`;
}

// ==================== ЗАГОТОВКИ (§5.4–§5.6) ====================

// stub — каркас раздела-заготовки: реестр будущего содержимого + пометка.
function stub(items, note) {
    return `
        <div style="font-size:0.8rem; color:#94a3b8; margin-bottom:6px;">${note}</div>
        <div style="display:flex; flex-wrap:wrap; gap:6px;">
            ${items.map(i => `<span style="background:#1e293b; border:1px solid #334155; border-radius:10px; padding:4px 10px; font-size:0.8rem; color:#cbd5e1;">${i}</span>`).join('')}
        </div>`;
}

// Ресурсы/сырьё (§5.4): 6 категорий веществ (09_resources §9.1.3) + 99.2.5.
function renderResourcesStub(container) {
    container.innerHTML = stub(
        ['🪨 Минералы', '🌿 Органика', '⭐ Редкие', '🔥 Топливо', '💧 Вода', '💨 Газы'],
        'Вещества и свойства появятся с ресурсной моделью (99.2.5).'
    );
}

// Корабли/оборудование (§5.5): 3 типа слотов (77a §2.3) + пометка про 77a.
function renderShipsStub(container) {
    container.innerHTML = stub(
        ['📡 Радар', '🔭 Сканер', '⚙️ Двигатель'],
        'Модель корабля и оборудование появятся с 77a.'
    );
}

// Лор-термины/существа (§5.6): бестиарий (76a.1) + словарь терминов.
function renderLoreStub(container) {
    container.innerHTML = stub(
        ['Бестиарий (76a.1)', 'Словарь терминов: отчёт, сертификат, присутствие, фронтир, сад, раса-дом, выброс'],
        'Наполнится с бестиарием (76a.1).'
    );
}

// ==================== ЗАГРУЗКА ====================

// loadEncyclopedia — читает журнал, тянет /api/encyclopedia/races и рисует
// статистику + разделы. Возвращает Promise (для обработки 401 на странице).
export function loadEncyclopedia() {
    const journal = get();
    const statsEl = document.getElementById('stats');
    const encEl = document.getElementById('encyclopedia');
    if (statsEl) renderStats(statsEl, journal);
    if (encEl) encEl.innerHTML = '<div style="color:#94a3b8;">Загрузка энциклопедии…</div>';

    const token = localStorage.getItem('token');
    if (!token) return Promise.resolve();
    return fetch('/api/encyclopedia/races', {
        headers: { 'Authorization': 'Bearer ' + token }
    })
    .then(res => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
    })
    .then(data => {
        if (encEl) renderEncyclopedia(encEl, data && data.races, get());
    })
    .catch(e => {
        console.error('Ошибка загрузки энциклопедии:', e);
        if (encEl) encEl.innerHTML = '<div style="color:#f66;">Не удалось загрузить энциклопедию.</div>';
    });
}