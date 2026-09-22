// web/static/js/modal/tabs.js
import { populationAt, planetPopulationAt } from './extrapolate.js';
import { modalState } from './state.js';
import { getPlanetTexture } from './textures.js';
import { groupDeposits } from './deposits.js';
import { branchesBlockHtml } from './branches.js';
import { boardHtml, canPublishHere, publishFormHtml, escapeHtml } from './contracts.js';
import { notifyError, notifySuccess } from '../ui/toast.js';

// ---------- ИСТОРИЯ ФОРМИРОВАНИЯ (Ф4, спека 2026-09-22-облако-этап-2 §6.3) ----------

// Флаг показа значка истории формирования. Решение создателя 2026-09-22
// («давай попробуем значок, но чет сомневаюсь. Посмотрим»): показ обратим —
// false гасит значок одним флагом, без правок бэкенда/API; текст описаний от
// значка не зависит (каналы независимы).
const SHOW_FORMATION_HISTORY_BADGE = true;

// Подписи типов formation_history для тултипа значка.
const FORMATION_HISTORY_LABELS = {
    formed_early: 'сформировалась рано',
    formed_late: 'сформировалась поздно',
    migrated: 'мигрировала',
    ice_lost: 'потеряла лёд',
    stripped_embryo: 'сорванный эмбрион',
};

// formationHistoryBadge — компактная метка истории формирования (непустой
// formation_history и знание планеты: сервер скрывает маркер без знания).
function formationHistoryBadge(planet) {
    if (!SHOW_FORMATION_HISTORY_BADGE) return '';
    const hist = planet && planet.formation_history;
    if (!Array.isArray(hist) || hist.length === 0) return '';
    const labels = hist.map(e => FORMATION_HISTORY_LABELS[e.type] || e.type);
    const tooltip = labels.join(', ');
    return `<span title="${tooltip}" style="display:inline-block; margin-left:8px; padding:1px 8px; border-radius:10px; background:rgba(251,191,36,0.15); border:1px solid rgba(251,191,36,0.4); color:#fbbf24; font-size:0.8rem;">✦ история формирования</span>`;
}

// ---------- УТИЛИТЫ ----------

// K → °C с округлением
function kelvinToCelsius(k) {
    if (typeof k !== 'number' || isNaN(k)) return '—';
    return (k - 273.15).toFixed(1);
}

// Форматирование числа: 1234567 → "1 234 567"
function formatNumber(n) {
    if (typeof n !== 'number' || isNaN(n)) return '—';
    return n.toLocaleString('ru-RU');
}

// Обрезка длинного списка форм: {form: percent} → [[form, percent], ...]
function topEntries(map, limit) {
    if (!map || typeof map !== 'object') return [];
    const entries = Object.entries(map).filter(([, v]) => v > 0.01);
    entries.sort((a, b) => b[1] - a[1]);
    return entries.slice(0, limit);
}

// Красивое имя: пустая_порода → пустая порода
export function prettyName(name) {
    if (typeof name !== 'string') return name;
    return name.split('_').join(' ');
}

// capitalize — первая буква заглавная ('belrano' → 'Belrano')
function capitalize(s) {
    if (!s) return s || '';
    return s.charAt(0).toUpperCase() + s.slice(1);
}

// Иконки по форме/типу
const FORM_ICONS = {
    'горы': '🪨', 'пески_пустыни': '🏜️', 'кратеры': '⚫',
    'стеклянные_поля': '🔮', 'металлические_поля': '⚙️',
    'лавовые_поля': '🌋', 'вулканические_поля': '🗻',
    'ледники': '❄️', 'мёрзлые_газы': '💠',
    'океаны': '🌊', 'озёра_реки': '💧',
    'луга_степи': '🌾', 'леса': '🌲', 'джунгли': '🌴',
    'болота': '🌿', 'коралловые_рифы': '🐠',
    'реголит': '🪨', 'ледяная_кора': '🧊',
    'криовулканы': '🌋', 'тектонические_разломы': '〰️',
    'гейзерные_поля': '💨'
};

const SUBTERRAIN_ICONS = {
    'пустая_порода': '⬛', 'магматические_породы': '🔥',
    'метаморфические_породы': '🪨', 'осадочные_породы': '🟫',
    'рудные_жилы': '⚒️', 'редкоземельные_жилы': '💎',
    'радиоактивные_зоны': '☢️', 'угольные_пласты': '🖤',
    'нефтяные_карманы': '🛢️', 'газовые_карманы': '💨',
    'подземные_воды': '💧', 'подземные_льды': '❄️',
    'магматические_камеры': '🌋', 'кристаллические_жилы': '💠',
    'соляные_купола': '🧂', 'пещерные_системы': '🕳️',
    'металлические_ядра': '⚙️'
};

// Цвета для полосок композиции
const FORM_COLORS = {
    'горы': '#8b7355', 'пески_пустыни': '#d4a373', 'кратеры': '#4a4a4a',
    'стеклянные_поля': '#a8d8ea', 'металлические_поля': '#9e9e9e',
    'лавовые_поля': '#e74c3c', 'вулканические_поля': '#c0392b',
    'ледники': '#d0e8f2', 'мёрзлые_газы': '#b0d4e3',
    'океаны': '#3498db', 'озёра_реки': '#5dade2',
    'луга_степи': '#a3c644', 'леса': '#27ae60', 'джунгли': '#16a085',
    'болота': '#556b2f', 'коралловые_рифы': '#e91e63',
    'реголит': '#8a7a6a', 'ледяная_кора': '#d0e4f0',
    'криовулканы': '#5d9fb5', 'тектонические_разломы': '#6b7280',
    'гейзерные_поля': '#a5d8e8'
};

const SUBTERRAIN_COLORS = {
    'пустая_порода': '#3a3a3a', 'магматические_породы': '#c0392b',
    'метаморфические_породы': '#7f8c8d', 'осадочные_породы': '#8d6e63',
    'рудные_жилы': '#95a5a6', 'редкоземельные_жилы': '#9b59b6',
    'радиоактивные_зоны': '#f1c40f', 'угольные_пласты': '#2c3e50',
    'нефтяные_карманы': '#34495e', 'газовые_карманы': '#7fb3d5',
    'подземные_воды': '#3498db', 'подземные_льды': '#aed6f1',
    'магматические_камеры': '#e74c3c', 'кристаллические_жилы': '#8e44ad',
    'соляные_купола': '#ecf0f1', 'пещерные_системы': '#2c2c2c',
    'металлические_ядра': '#7f8c8d'
};

// ---------- РЕНДЕР КОМПОЗИЦИИ ----------

// biomeIconHtml — иконка биома: PNG из /static/sprites/biomes/<id>.png
// (пачка 1, 23 шт, спека 2026-09-20 §6.3), если файл есть; onerror →
// эмодзи FORM_ICONS[form] / «•» (34 биома без иконок — фолбэк, М1).
// Список id не хардкодим — фолбэк по ошибке загрузки.
export function biomeIconHtml(form) {
    const fallback = FORM_ICONS[form] || '•';
    return `<img src="/static/sprites/biomes/${encodeURIComponent(form)}.png" alt=""
        data-biome-fallback="${fallback}"
        style="width:22px; height:22px; vertical-align:middle; margin-right:4px;">`;
}

// Рисует полоску + полный список форм. Без обрезки и кнопки «показать все»
// (69a: в блоках Поверхность/Недра остаток был 1–2 строки, кнопка бесполезна).
// pngIcons — true для поверхности (биомы, PNG-иконки §6.3); недры — эмодзи.
function renderComposition(composition, icons, colors, pngIcons) {
    if (!composition || Object.keys(composition).length === 0) {
        return '<p style="color:#666; margin: 4px 0;">— нет данных —</p>';
    }

    const allEntries = Object.entries(composition).filter(([, v]) => v > 0.01);
    allEntries.sort((a, b) => b[1] - a[1]);

    // Полоска
    let bar = '<div style="display:flex; height:10px; border-radius:5px; overflow:hidden; margin: 6px 0;">';
    allEntries.forEach(([form, pct]) => {
        const color = colors[form] || '#555';
        bar += `<div style="width:${pct}%; background:${color};" title="${prettyName(form)} ${pct.toFixed(1)}%"></div>`;
    });
    bar += '</div>';

    // Список
    const renderRow = ([form, pct]) => {
        const icon = pngIcons ? biomeIconHtml(form) : (icons[form] || '•');
        return `<li style="margin: 2px 0; display:flex; justify-content:space-between;">
            <span>${icon} ${prettyName(form)}</span>
            <span style="color:#888;">${pct.toFixed(1)}%</span>
        </li>`;
    };

    let list = `<ul style="list-style:none; padding: 0; margin: 4px 0;">`;
    allEntries.forEach(e => { list += renderRow(e); });
    list += '</ul>';

    return bar + list;
}

// ---------- ВИД С ОРБИТЫ (спека 2026-09-20 §6.2) ----------

// orbitViewState — состояние загрузки большой картинки: {status} —
// 'loading' | 'ok' | 'error'. Object URL revoke делает textures.js после
// декодирования; cleanupOrbitView сбрасывает состояние при закрытии модалки
// и перед новой загрузкой.
let orbitViewState = null;

// isAdmin — роль из /me (спека §6.2): admin/skycomposer видят всё (гейт 2).
function isAdmin() {
    return modalState.role === 'admin' || modalState.role === 'skycomposer';
}

// orbitViewHtml — контейнер «Вид с орбиты» (заполняется renderOrbitView
// после вставки в DOM: асинхронная загрузка картинки). Показывается, когда
// игрок на орбите этой планеты (99.2.27 §4.4/§5.11) или роль админ.
function orbitViewHtml(planet) {
    const myPos = modalState.myPosition;
    // orbit | surface этой планеты (спека 2026-09-21 §7.6 п.5): вид с орбиты
    // доступен и с поверхности планеты (игрок всё ещё в системе).
    const onThisOrbit = myPos && (myPos.status === 'orbit' || myPos.status === 'surface') &&
        myPos.object_type === 'planet' && myPos.object_id === planet.id;
    if (!onThisOrbit && !isAdmin()) return '';
    return `
        <div class="orbit-view" data-orbit-view>
            <div style="color:#888; font-size:0.9rem; text-transform:uppercase; margin:8px 0 4px 0;">Вид с орбиты</div>
            <div class="orbit-view-frame" style="width:288px; max-width:100%; border-radius:12px; overflow:hidden; background:#0d0d1a; border:1px solid #2a2a4a;">
                <div class="orbit-view-body" style="display:flex; align-items:center; justify-content:center; min-height:180px; color:#94a3b8; font-size:0.85rem;">Загрузка…</div>
            </div>
        </div>
    `;
}

// renderOrbitView — загрузка большой картинки (size=big, 512px PNG,
// CSS-масштаб): состояния Загрузка… / Успех / Не удалось загрузить вид.
function renderOrbitView(planet, container) {
    const wrap = container.querySelector('[data-orbit-view]');
    if (!wrap) return;
    const body = wrap.querySelector('.orbit-view-body');
    if (!body) return;

    cleanupOrbitView();
    orbitViewState = { status: 'loading' };

    getPlanetTexture(planet, 'big')
        .then(img => {
            if (!body.isConnected) return; // карточка перерисована
            body.innerHTML = '';
            body.appendChild(img);
            img.style.width = '100%';
            img.style.height = 'auto';
            img.style.display = 'block';
            orbitViewState = { status: 'ok' };
        })
        .catch(() => {
            if (!body.isConnected) return;
            body.innerHTML = '<span>Не удалось загрузить вид</span>';
            orbitViewState = { status: 'error' };
        });
}

// cleanupOrbitView — сброс состояния большой картинки (спека §6.2).
// Вызывается при закрытии модалки (index.js closeModal) и перед новой
// загрузкой. Object URL revoke делает textures.js после декодирования.
export function cleanupOrbitView() {
    orbitViewState = null;
}

// ---------- ОБЩАЯ ВКЛАДКА ----------

function renderGeneral(planet) {
    let html = '';

    // Большая картинка «Вид с орбиты» (спека 2026-09-20 §6.2): вверху
    // карточки, над блоком «Тип»; заполняется renderOrbitView после вставки.
    html += orbitViewHtml(planet);

    // Тип (название в шапке карточки) + значок истории формирования (Ф4).
    html += `<p style="margin:4px 0;"><strong>Тип:</strong> ${planet.type || '—'}${formationHistoryBadge(planet)}</p>`;

    // Знание о планете (спека 77a §6.2): для player без знания сервер скрывает
    // детали (поверхность/недра/атмосфера/поселения) — заглушка «нет данных —
    // купить отчёт» вместо них. У admin/skycomposer детали на месте (И7).
    const hasDetails = planet.surface_composition || planet.atmosphere || planet.core || planet.subterrain_composition;
    if (!planet.knowledge && !hasDetails) {
        html += `<p style="margin:8px 0; padding:8px 10px; background:rgba(148,163,184,0.08); border:1px dashed rgba(148,163,184,0.3); border-radius:8px; color:#94a3b8; font-size:0.85rem;">
            Нет данных — купить отчёт
        </p>`;
    }

    // Поверхность: у player — из знания сканера (с датой актуальности, И8);
    // у admin — из тела планеты.
    const surfaceDominant = planet.knowledge ? planet.knowledge.surface_dominant : planet.surface_dominant;
    if (surfaceDominant) {
        html += `<p style="margin:4px 0;"><strong>Доминирует:</strong> ${surfaceDominant}</p>`;
    }

    // Физика
    html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Физика</p>`;
    html += `<p style="margin:4px 0;"><strong>Масса:</strong> ${planet.mass ? planet.mass.toFixed(2) + ' M⊕' : '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Размер:</strong> ${planet.size ? planet.size.toFixed(2) + ' R⊕' : '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Плотность:</strong> ${planet.density ? planet.density.toFixed(2) : '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Температура:</strong> ${kelvinToCelsius(planet.temperature)} °C (${planet.temperature ? planet.temperature.toFixed(0) : '—'} K)</p>`;
    html += `<p style="margin:4px 0;"><strong>Расстояние до звезды:</strong> ${planet.orbit_radius_au ? planet.orbit_radius_au.toFixed(1) + ' а.е.' : '—'}</p>`;

    // Атмосфера, биосфера
    html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Среда</p>`;
    html += `<p style="margin:4px 0;"><strong>Атмосфера:</strong> ${planet.atmosphere || '—'}</p>`;
    if (planet.atmosphere_data && planet.atmosphere_data.pressure_atm != null) {
        html += `<p style="margin:4px 0;"><strong>Давление:</strong> ${planet.atmosphere_data.pressure_atm.toFixed(2)} атм</p>`;
    }
    if (planet.hydrosphere) {
        html += `<p style="margin:4px 0;"><strong>Гидросфера:</strong> ${planet.hydrosphere}</p>`;
    }
    if (planet.biosphere) {
        html += `<p style="margin:4px 0;"><strong>Биосфера:</strong> ${planet.biosphere}</p>`;
    }
    if (planet.archetype || planet.climate) {
        html += `<p style="margin:4px 0;"><strong>Архетип:</strong> ${planet.archetype || planet.climate}</p>`;
    }
    html += `<p style="margin:4px 0;"><strong>Вода:</strong> ${planet.water_percent ? planet.water_percent.toFixed(1) + '%' : '—'}</p>`;

    // Ядро
    if (planet.core) {
        const c = planet.core;
        html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Ядро</p>`;
        html += `<p style="margin:4px 0;"><strong>Тип:</strong> ${c.type || '—'}</p>`;
        html += `<p style="margin:4px 0;"><strong>Доля массы:</strong> ${c.mass_percent ? c.mass_percent.toFixed(1) + '%' : '—'}</p>`;
        html += `<p style="margin:4px 0;"><strong>Активность:</strong> ${c.activity ? c.activity.toFixed(1) : '—'}/100</p>`;
        html += `<p style="margin:4px 0;"><strong>Радиоактивность:</strong> ${c.radioactivity ? c.radioactivity.toFixed(1) : '—'}/100</p>`;
        html += `<p style="margin:4px 0;"><strong>Возраст:</strong> ${c.age ? c.age.toFixed(2) + ' млрд лет' : '—'}</p>`;
    }

    // Поверхность (у player — из знания сканера, спека 77a §6.2)
    html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Поверхность</p>`;
    const surfaceComp = planet.knowledge && planet.knowledge.surface_composition
        ? planet.knowledge.surface_composition : planet.surface_composition;
    html += renderComposition(surfaceComp, FORM_ICONS, FORM_COLORS, true);

    // Недра
    html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Недра</p>`;
    html += renderComposition(planet.subterrain_composition, SUBTERRAIN_ICONS, SUBTERRAIN_COLORS, false);

    // Жизнь
    html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Жизнь</p>`;
    html += `<p style="margin:4px 0;"><strong>Обитаемость:</strong> ${planet.habitable ? '✅ Да' : '— Нет'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Жизнь:</strong> ${planet.life ? '✅ Да' : '— Нет'}</p>`;
    const populationNow = planetPopulationAt(planet, Date.now());
    if (populationNow > 0) {
        html += `<p style="margin:4px 0;"><strong>Население:</strong> <span data-pop-planet>${formatNumber(populationNow)}</span></p>`;
    }

    // Описание
    if (planet.description) {
        html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Описание</p>`;
        html += `<p style="margin:4px 0; font-style:italic; color:#bbb;">${planet.description}</p>`;
    }

    // Спутники (для газовых гигантов)
    if (planet.satellites && planet.satellites.length > 0) {
        html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Спутники (${planet.satellites.length}) — клик для деталей</p>`;
        html += '<ul style="list-style:none; padding:0; margin:4px 0;">';
        planet.satellites.forEach((sat, si) => {
            const lifeIcon = sat.life ? ' 🔴' : ' ⚪';
            html += `<li data-sat-idx="${si}" style="margin: 4px 0; padding: 6px; background:#1a1a2e; border-radius:4px; cursor:pointer;" title="Открыть карточку спутника">
                <div><strong>${capitalize(sat.name)}</strong>${lifeIcon}</div>
                <div style="color:#888; font-size:0.9rem;">
                    ${sat.temperature ? kelvinToCelsius(sat.temperature) + ' °C' : '—'} · 
                    ${sat.water_percent ? sat.water_percent.toFixed(0) + '% воды' : '—'}
                </div>
            </li>`;
        });
        html += '</ul>';
    }

    return html;
}

// ---------- КАРТОЧКА СПУТНИКА ----------

// compositeSatTooltip — тултип композитной кнопки спутника (спека 99.2.30
// §6.10). Локальная копия тултипа композитной кнопки планеты: статический
// импорт из panel.js дал бы цикл модулей (panel.js импортирует tabs.js).
function compositeSatTooltip() {
    const flight = modalState.interstellarFlight;
    if (flight) {
        if (flight.to === modalState.worldId) {
            return 'Вы уже летите к этой системе — маршрут дополнится полётом к спутнику';
        }
        const name = modalState.interstellarFlightName;
        return 'Маршрут развернётся: полёт к ' + (name || 'другой системе') + ' и до орбиты спутника';
    }
    return 'Перелёт к системе и полёт до орбиты спутника — в 2 этапа';
}

// renderSatelliteCard — карточка спутника по клику из списка планеты.
// Кнопка «назад» возвращает к общей вкладке планеты.
// Экспорт — panel.js восстанавливает карточку при перерисовке панели
// (refreshPlanets, /me): выбор хранится в modalState.selectedSatellite.
export function renderSatelliteCard(planet, sat, container) {
    if (!sat) return;

    // Внутрисистемная позиция (спека 99.2.27 §5.11): бейдж «● Вы на орбите»
    // у спутника (позиция = спутник → маркер у родительской планеты, М-4).
    const myPos = modalState.myPosition;
    const onThisOrbit = myPos && myPos.status === 'orbit' &&
        myPos.object_type === 'satellite' && myPos.object_id === sat.id;
    const orbitBadge = onThisOrbit
        ? `<span style="background:rgba(74,222,128,0.15); border:1px solid rgba(74,222,128,0.4); color:#4ade80; border-radius:10px; padding:2px 8px; font-size:0.8rem; margin-left:8px;">● Вы на орбите</span>`
        : '';

    // Кнопка полёта (спека 99.2.27 §5.5 + 99.2.30 §6.1/§6.5): своя система —
    // внутрисистемная «🚀 Лететь»; чужая (my_position == null, планеты видны)
    // — композитная «🚀 Лететь · через систему». Две кнопки никогда не видны
    // одновременно (§6.5).
    const satFlyBtnHtml = myPos
        ? `<button data-sat-fly style="background:#2a2a4a; border:none; color:#fde68a; padding:6px 14px; border-radius:4px; cursor:pointer; font-size:0.95rem;">🚀 Лететь</button>`
        : `<button data-sat-composite-fly title="${compositeSatTooltip()}" style="background:#2a2a4a; border:none; color:#fde68a; padding:6px 14px; border-radius:4px; cursor:pointer; font-size:0.95rem;">🚀 Лететь · через систему</button>`;

    let html = `
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:10px;">
            <h4 style="margin:0; font-size:1.1rem;">${capitalize(sat.name)}${orbitBadge}</h4>
            <div style="display:flex; gap:8px;">
                ${satFlyBtnHtml}
                <button data-sat-back style="background:#2a2a4a; border:none; color:#aaa; padding:6px 14px; border-radius:4px; cursor:pointer; font-size:0.95rem;">← К планете ${planet.name ? capitalize(planet.name) : ''}</button>
            </div>
        </div>
    `;

    html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Физика</p>`;
    html += `<p style="margin:4px 0;"><strong>Орбита:</strong> ${sat.orbit_index ?? '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Размер:</strong> ${sat.size ? sat.size.toFixed(2) + ' R⊕' : '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Масса:</strong> ${sat.mass ? sat.mass.toFixed(2) + ' M⊕' : '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Температура:</strong> ${kelvinToCelsius(sat.temperature)} °C (${sat.temperature ? sat.temperature.toFixed(0) : '—'} K)</p>`;
    html += `<p style="margin:4px 0;"><strong>Вода:</strong> ${sat.water_percent ? sat.water_percent.toFixed(1) + '%' : '—'}</p>`;

    html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Среда</p>`;
    html += `<p style="margin:4px 0;"><strong>Атмосфера:</strong> ${sat.atmosphere || '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Биосфера:</strong> ${sat.biosphere || '—'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Обитаемость:</strong> ${sat.habitable ? '✅ Да' : '— Нет'}</p>`;
    html += `<p style="margin:4px 0;"><strong>Жизнь:</strong> ${sat.life ? '✅ Да' : '— Нет'}</p>`;

    if (sat.surface_composition && Object.keys(sat.surface_composition).length) {
        html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Поверхность</p>`;
        html += renderComposition(sat.surface_composition, FORM_ICONS, FORM_COLORS, true);
    }

    if (sat.subterrain_composition && Object.keys(sat.subterrain_composition).length) {
        html += `<p style="margin:8px 0 4px 0; color:#888; font-size:0.9rem; text-transform:uppercase;">Недра</p>`;
        html += renderComposition(sat.subterrain_composition, SUBTERRAIN_ICONS, SUBTERRAIN_COLORS, false);
    }

    if (sat.description) {
        html += `<p style="margin:4px 0; font-style:italic; color:#bbb;">${formatNumberSafe(sat.description)}</p>`;
    }

    container.innerHTML = html;

    const backBtn = container.querySelector('[data-sat-back]');
    if (backBtn) {
        backBtn.addEventListener('click', () => {
            // «← К планете» — сбрасываем выбор спутника (баг 2026-09-21),
            // иначе при следующей перерисовке панели карточка спутника
            // «воскреснет».
            modalState.selectedSatellite = null;
            renderTabContent('general', planet, container);
        });
    }

    // Кнопка «🚀 Лететь» (спека 99.2.27 §5.5): внутрисистемный полёт на орбиту
    // спутника. Доступна только в своей системе; disabled при: нет двигателя,
    // цель == текущая позиция, цель == активный полёт, летим ОТ этого спутника
    // (запрос создателя «глупый тост»), активный межзвёздный полёт (спека
    // 99.2.30 §6.2, мелкое 4 — иначе старт даст 400 «Вы в межзвёздном полёте»).
    const flyBtn = container.querySelector('[data-sat-fly]');
    if (flyBtn) {
        const myPos = modalState.myPosition;
        const disabled = !myPos || !modalState.hasEngine || !!modalState.interstellarFlight ||
            (myPos.status === 'orbit' && myPos.object_type === 'satellite' && myPos.object_id === sat.id) ||
            (myPos.status === 'in_flight' && myPos.to_type === 'satellite' && myPos.to_id === sat.id) ||
            (myPos.status === 'in_flight' && myPos.from_type === 'satellite' && myPos.from_id === sat.id);
        if (disabled) {
            flyBtn.disabled = true;
            flyBtn.style.opacity = '0.4';
            flyBtn.style.cursor = 'not-allowed';
            if (!modalState.hasEngine) flyBtn.title = 'Двигатель не установлен';
            else if (modalState.interstellarFlight) flyBtn.title = 'Вы в межзвёздном полёте — дождитесь прибытия';
            else if (!myPos) flyBtn.title = 'Внутрисистемный полёт — только в своей системе';
            else flyBtn.title = 'Вы уже на орбите этого объекта';
        } else {
            flyBtn.addEventListener('click', () => {
                import('./events.js').then(m => m.startIntraFlight('satellite', sat.id));
            });
        }
        // Асинхронный фолбэк (спека 99.2.30 §6.6): клик до резолва /me —
        // синхронный mapState.isFlying (динамический импорт только на странице
        // карты; в админке import-граф карты не тянется).
        if (!modalState.interstellarFlight && document.getElementById('mapCanvas')) {
            import('../map/config.js').then(m => {
                if (m.state.isFlying && flyBtn && !flyBtn.disabled) {
                    flyBtn.disabled = true;
                    flyBtn.style.opacity = '0.4';
                    flyBtn.style.cursor = 'not-allowed';
                    flyBtn.title = 'Вы в межзвёздном полёте — дождитесь прибытия';
                }
            }).catch(() => {});
        }
    }

    // Композитная кнопка спутника «🚀 Лететь · через систему» (спека 99.2.30
    // §6.1/§6.6): чужая система — полёт к спутнику через систему (2 сегмента).
    // Disabled без двигателя (91a); НЕ блокируется при активном межзвёздном
    // (это /travel 202/редирект, работает, §3.4/§3.5).
    const compositeBtn = container.querySelector('[data-sat-composite-fly]');
    if (compositeBtn) {
        if (!modalState.hasEngine) {
            compositeBtn.disabled = true;
            compositeBtn.style.opacity = '0.4';
            compositeBtn.style.cursor = 'not-allowed';
            compositeBtn.title = 'Двигатель не установлен — полёт невозможен';
        } else {
            compositeBtn.addEventListener('click', async () => {
                // Двойной клик: пока запрос /travel в полёте — кнопка disabled
                // (защита от дублей старта, §6.6).
                compositeBtn.disabled = true;
                compositeBtn.style.opacity = '0.4';
                compositeBtn.style.cursor = 'not-allowed';
                const ok = await import('./events.js').then(m => m.startCompositeFlight('satellite', sat.id));
                if (!ok) {
                    // Ошибка старта: модалка остаётся открытой, тост с текстом
                    // сервера, ничего не закрываем (§6.7 п.5) — кнопка снова активна.
                    compositeBtn.disabled = false;
                    compositeBtn.style.opacity = '';
                    compositeBtn.style.cursor = '';
                }
            });
        }
    }
}

// formatNumberSafe — безопасное отображение строки/числа
function formatNumberSafe(v) {
    if (typeof v === 'number') return formatNumber(v);
    return v ? String(v) : '';
}

// ---------- РЕСУРСЫ ----------

function renderResources(planet) {
    if (!planet.resources || typeof planet.resources !== 'object') {
        return '<p style="color: #666;">Нет данных о ресурсах</p>';
    }

    const categories = Object.keys(planet.resources);
    if (categories.length === 0) {
        return '<p style="color: #666;">Нет данных о ресурсах</p>';
    }

    let html = '<p><strong>Ресурсы:</strong></p><ul style="list-style: none; padding: 0;">';
    categories.forEach(cat => {
        const val = planet.resources[cat];
        if (typeof val === 'number') {
            const percent = (val * 100).toFixed(0);
            html += `<li style="margin-bottom: 6px;">
                <span style="display: inline-block; width: 80px;">${prettyName(cat)}:</span>
                <div style="display: inline-block; width: 100px; height: 8px; background: #333; border-radius: 4px; overflow: hidden; vertical-align: middle;">
                    <div style="width: ${percent}%; height: 100%; background: #4a9eff; border-radius: 4px;"></div>
                </div>
                <span style="margin-left: 8px; font-size: 0.85rem; color: #888;">${percent}%</span>
            </li>`;
        }
    });
    html += '</ul>';
    return html;
}

// ---------- ПОСЕЛЕНИЯ ----------

// settlementTrendArrow — ↓/↑/— рядом с населением поселения. Направление из
// ТЕКУЩИХ данных объекта (99.2.12): признак снижения (r_per_sec > 0 — жара,
// или lambda_per_hour > 0 — холод/гравитация/радиация) → ↓ сразу; дельта
// двух серверных снапшотов (modalState.previousSettlementPop, refreshPlanets)
// — путь для роста и равновесия (рост реализован рождаемостью, 99.2.16).
// При конфликте живой сигнал снижения приоритетен (не врём в сторону роста).
function settlementTrendArrow(s) {
    const declining = (typeof s.r_per_sec === 'number' && s.r_per_sec > 0) ||
        (typeof s.lambda_per_hour === 'number' && s.lambda_per_hour > 0);
    if (declining) return ' <span style="color:#f66;" title="Население убывает">↓</span>';
    const prev = modalState.previousSettlementPop[s.id];
    if (typeof prev !== 'number' || typeof s.population !== 'number') return '';
    if (s.population < prev) return ' <span style="color:#f66;" title="Население убывает">↓</span>';
    if (s.population > prev) return ' <span style="color:#6f6;" title="Население растёт">↑</span>';
    return ' <span style="color:#888;" title="Без изменений">—</span>';
}

// Код причины «Вымерло» → человеческий текст (18b_settlement_log.md, §«Причина»)
const EXTINCT_CAUSE_TEXT = {
    'heat': 'Экстремальная жара',
    'cold': 'Сильный холод',
    'gravity_high': 'Высокая гравитация',
    'gravity_low': 'Низкая гравитация',
    'radiation': 'Радиоактивный фон',
    'natural': 'Естественная убыль'
};

// settlementLogRows — строки лога поселения «Вымерло · дата · причина»,
// последние 3 записи, сортировка по дате убывающая. При 0 записей — пусто
// (блок скрыт). Запись появляется только после серверного синка, отдавшего
// log (extrapolate.js запись не рисует — 18b §«UI»).
function settlementLogRows(s) {
    if (!s.log || !Array.isArray(s.log) || s.log.length === 0) return '';
    const rows = s.log
        .slice()
        .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
        .slice(0, 3);
    let html = `<div style="color:#888; font-size:0.9rem; text-transform:uppercase; margin-top:8px;">Лог</div>`;
    rows.forEach(e => {
        const when = e.occurred_at ? new Date(e.occurred_at).toLocaleString('ru-RU') : '—';
        const cause = EXTINCT_CAUSE_TEXT[e.cause] || e.cause || '';
        html += `<div style="color:#ccc; margin-top:4px;">💀 Вымерло · ${when} · ${cause}</div>`;
    });
    return html;
}

function renderSettlements(planet) {
    // Для player без знания сканера сервер скрывает поселения (спека 77a
    // §6.2): «нет данных — купить отчёт» вместо «нет поселений» (последнее —
    // само по себе знание). У admin/skycomposer settlements на месте (И7).
    if (!planet.knowledge && !planet.settlements) {
        return `<p style="color: #666; text-align: center; padding: 20px 0;">Нет данных — купить отчёт</p>`;
    }
    // Player со знанием сканера: детали поселений скрыты (население/раса —
    // платные отчёты), видно только наличие + число (спека 77a §6.2).
    if (planet.knowledge && !planet.settlements) {
        const n = planet.knowledge.settlements_count || 0;
        if (n <= 0) {
            return `<p style="color: #666; text-align: center; padding: 20px 0;">🏙️ На планете нет поселений</p>`;
        }
        return `<p style="color:#888; font-size:0.9rem; text-transform:uppercase;">Поселения (${n})</p>
            <p style="color:#94a3b8; font-size:0.85rem; padding: 8px 0;">Детали поселений — купить отчёт</p>`;
    }

    const list = planet.settlements;
    if (!list || list.length === 0) {
        return `<p style="color: #666; text-align: center; padding: 20px 0;">🏙️ На планете нет поселений</p>`;
    }

    let html = `<p style="color:#888; font-size:0.9rem; text-transform:uppercase;">Поселения (${list.length})</p>`;
    list.forEach((s, i) => {
        html += `
            <div style="margin: 6px 0; padding: 10px; background:#1a1a2e; border-radius:4px;">
                <div style="display:flex; justify-content:space-between; align-items:center;">
                    <strong>Поселение ${i + 1}</strong>
                </div>
                <div style="color:#ccc; margin-top:6px;">
                    <div>Раса: <strong>${s.race_name || 'Люди'}</strong></div>
                    <div>Население: <strong id="pop-${s.id}">${formatNumber(populationAt(s, Date.now()))}</strong>${settlementTrendArrow(s)}</div>
                    <div>Стабильность: <strong>${populationAt(s, Date.now()) === 0 ? '—' : (s.stability != null ? s.stability + '%' : '—')}</strong></div>
                </div>
                ${settlementLogRows(s)}
                ${branchesBlockHtml(s.branches, isAdmin(), s.id)}
            </div>`;
    });
    return html;
}

// ---------- ВЕТКИ ПОСЕЛЕНИЯ (спека 2026-09-22-поселение-ветка-буферы-переработка §6) ----------

// applyBranchesToSettlement — замена блока веток поселения ответом ручки
// (контракт §5: клиент заменяет блок целиком).
function applyBranchesToSettlement(planet, settlementID, branches) {
    const list = planet && planet.settlements ? planet.settlements : [];
    const s = list.find(x => x.id === settlementID);
    if (s) s.branches = Array.isArray(branches) ? branches : [];
}

// initBranchesAdmin — вешает админ-формы веток после вставки вкладки в DOM:
// «создать ветку» (recipe_id) и «добавить во вход» (ресурс из
// /studio/api/resources + количество). Ответ ручек (§5) заменяет блок веток.
function initBranchesAdmin(planet, container) {
    if (!isAdmin()) return;

    container.querySelectorAll('[data-branch-create]').forEach(btn => {
        const form = btn.closest('[data-branch-create-form]');
        if (!form) return;
        const settlementID = form.dataset.branchCreateForm;
        btn.addEventListener('click', async () => {
            const inp = form.querySelector('[data-branch-recipe]');
            const recipeID = inp ? Number(inp.value) : 0;
            if (!settlementID || !recipeID) return;
            const token = modalState.authToken || localStorage.getItem('token');
            try {
                const res = await fetch('/admin/settlements/' + encodeURIComponent(settlementID) + '/branches', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
                    body: JSON.stringify({ recipe_id: recipeID })
                });
                if (!res.ok) { console.warn('ветка: HTTP ' + res.status + ' ' + await res.text()); return; }
                const data = await res.json();
                applyBranchesToSettlement(planet, data.settlement_id, data.branches);
                renderTabContent('settlements', planet, container);
            } catch (e) {
                console.warn('ветка: ' + e.message);
            }
        });
    });

    container.querySelectorAll('[data-branch-input-add]').forEach(btn => {
        const form = btn.closest('[data-branch-input-form]');
        if (!form) return;
        const branchID = form.dataset.branchInputForm;
        const sel = form.querySelector('[data-branch-input-good]');
        loadDepositResources().then(list => {
            if (!sel || !sel.isConnected) return;
            sel.innerHTML = list.length
                ? list.map(r => `<option value="${r.id}">${r.name}</option>`).join('')
                : '<option value="">ресурсов нет</option>';
        }).catch(() => {
            if (sel && sel.isConnected) sel.innerHTML = '<option value="">ресурсы недоступны</option>';
        });
        btn.addEventListener('click', async () => {
            const amountEl = form.querySelector('[data-branch-input-amount]');
            if (!branchID || !sel || !sel.value || !amountEl || !Number(amountEl.value)) return;
            const token = modalState.authToken || localStorage.getItem('token');
            try {
                const res = await fetch('/admin/branches/' + encodeURIComponent(branchID) + '/input', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
                    body: JSON.stringify({ good_id: Number(sel.value), amount: Number(amountEl.value) })
                });
                if (!res.ok) { console.warn('ветка вход: HTTP ' + res.status + ' ' + await res.text()); return; }
                const data = await res.json();
                applyBranchesToSettlement(planet, data.settlement_id, data.branches);
                renderTabContent('settlements', planet, container);
            } catch (e) {
                console.warn('ветка вход: ' + e.message);
            }
        });
    });
}

// ---------- ФРАКЦИИ (спека 2026-09-21-фабрики-релиз-2-столицы-фракций §4) ----------
// BUILDING_TYPE_LABELS — словарь подписей типов строений: единственное место,
// где ключ (buildings.building_type) превращается в человекочитаемое имя.
// Неизвестный ключ показывается как есть (выдуманных имён не вводим, §6).
const BUILDING_TYPE_LABELS = {
    capital: 'Столица'
};

// buildingTypeLabel — подпись типа строения по ключу.
function buildingTypeLabel(type) {
    if (!type) return '—';
    return BUILDING_TYPE_LABELS[type] || type;
}

// factionCapitalHtml — инлайновый раскрывающийся блок деталей столицы
// (клик по карточке фракции, §4.3): без новых окон и серверных вызовов.
function factionCapitalHtml(faction, planet) {
    return `
        <div data-faction-details="${faction.id}" style="display:none; margin-top:8px; padding-top:8px; border-top:1px solid #2a2a4a;">
            <div style="color:#888; font-size:0.9rem; text-transform:uppercase;">Столица (строение)</div>
            <div style="margin-top:4px;">Тип: Другое — ничего не производит</div>
            <div>Владелец: ${faction.name || '—'} (${faction.type || '—'})</div>
            <div>Планета: ${planet.name ? capitalize(planet.name) : '—'} — родная планета фракции</div>
            <div style="color:#94a3b8; font-size:0.85rem; margin-top:6px;">Управление появится позже: сейчас видно только владельца</div>
        </div>`;
}

// renderFactions — вкладка «Фракции» карточки планеты: фракции, для которых
// планета родная (factions.homeworld_id), и строка столицы у владельца-фракции.
// Сила (strength) не показывается — генератор пишет заглушку 1 (§4.2).
function renderFactions(planet) {
    // Player без знания о планете сервер фракции/строения не отдаёт (§5):
    // пустое состояние как у поселений — «нет данных — купить отчёт»
    // (у admin/skycomposer знание не требуется, И7).
    if (!isAdmin() && !planet.knowledge) {
        return `<p style="color: #666; text-align: center; padding: 20px 0;">Нет данных — купить отчёт</p>`;
    }

    const factions = planet.factions || [];
    if (factions.length === 0) {
        // Снимок знания фракций не содержит и может быть устаревшим (§4.4):
        // отсутствие фракций в отчёте — не факт «фракций нет».
        return `<p style="color: #666; text-align: center; padding: 20px 0;">В отчёте сканера фракции не значились</p>`;
    }

    const buildings = planet.buildings || [];
    let html = `<p style="color:#888; font-size:0.9rem; text-transform:uppercase;">Фракции (${factions.length})</p>`;
    html += `<p style="color:#94a3b8; font-size:0.85rem; margin:4px 0 8px 0;">родная планета — эта</p>`;

    factions.forEach(f => {
        const color = f.color || '#888';
        // Столица фракции: buildings, где владелец — эта фракция (§4.2).
        const capital = buildings.find(b =>
            b.building_type === 'capital' && b.owner_type === 'faction' && b.owner_id === f.id);
        html += `
            <div style="margin:6px 0; padding:10px; background:#1a1a2e; border-radius:4px;">
                <div data-faction-toggle="${f.id}" style="cursor:pointer;">
                    <div><span style="display:inline-block; width:10px; height:10px; border-radius:50%; background:${color}; margin-right:6px;"></span><strong>${f.name || '—'}</strong></div>
                    <div style="color:#ccc; margin-top:4px;">${f.type || '—'}</div>
                    <div style="color:#888; font-size:0.85rem; margin-top:4px;">${f.description || ''}</div>
                    ${capital ? `<div style="color:#fde68a; margin-top:6px;">🏛 ${buildingTypeLabel(capital.building_type)} · Другое (ничего не производит)
                        <span style="color:#94a3b8; font-size:0.8rem;"> ▸ нажмите, чтобы раскрыть</span></div>` : ''}
                </div>
                ${capital ? factionCapitalHtml(f, planet) : ''}
            </div>`;
    });
    return html;
}

// ---------- ЗАЛЕЖИ (спека 2026-09-22-поселение-добыча-сырья-биома-ленивый-буфер §5.2) ----------

// depositResourcesCache — список ресурсов каталога (kind='resource') для
// админ-формы «добавить залежь»: один запрос на сессию модалки.
let depositResourcesCache = null;

// loadDepositResources — ресурсы каталога через студийный справочник
// (/studio/api/resources): good_id из формы валидируется сервером как
// kind='resource' (§6). Токен модалки — админский (карточку админа открывает
// admin/worlds.js с getAdminToken()).
async function loadDepositResources() {
    if (depositResourcesCache) return depositResourcesCache;
    const token = modalState.authToken || localStorage.getItem('token');
    const res = await fetch('/studio/api/resources', {
        headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!res.ok) throw new Error('HTTP ' + res.status);
    const list = await res.json();
    depositResourcesCache = Array.isArray(list) ? list : [];
    return depositResourcesCache;
}

// wealthRangeLabel — диапазон богатства пятен одного ресурса: 0.20–0.80;
// одно значение — «0.50»; нет чисел — «—».
function wealthRangeLabel(g) {
    if (g.wealthMin === null) return '—';
    if (g.wealthMin === g.wealthMax) return g.wealthMin.toFixed(2);
    return g.wealthMin.toFixed(2) + '–' + g.wealthMax.toFixed(2);
}

// adminDepositFormHtml — админ-инструмент «добавить залежь вручную» (§6):
// ресурс каталога (select) + опциональные богатство/запас; stratum итерации 1
// — только surface. Результат/ошибки — в строке статуса под формой.
function adminDepositFormHtml() {
    return `
        <div style="margin-top:16px; padding-top:10px; border-top:1px solid #2a2a4a;">
            <div style="color:#facc15; font-size:0.85rem;">⚙ админ — добавить залежь (surface)</div>
            <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-top:6px;">
                <select id="deposit-good-select" style="background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                    <option value="">Загрузка…</option>
                </select>
                <input id="deposit-wealth" type="number" step="0.01" min="0" max="1" placeholder="богатство 0–1"
                    style="width:120px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <input id="deposit-amount" type="number" step="1" min="0" placeholder="запас"
                    style="width:90px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <button id="deposit-add-btn" style="background:#2a2a4a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; cursor:pointer;">Добавить</button>
            </div>
            <div id="deposit-add-status" style="font-size:0.85rem; margin-top:6px; color:#94a3b8;"></div>
        </div>`;
}

// renderDeposits — вкладка «Залежи» карточки планеты: пятна, сведённые по
// ресурсу (число пятен, суммарный запас, диапазон богатства; T13). Сервер
// фильтрует залежи по знанию (§5.1): player без знания — «нет данных».
function renderDeposits(planet) {
    if (!isAdmin() && !planet.knowledge) {
        return `<p style="color: #666; text-align: center; padding: 20px 0;">Нет данных — купить отчёт</p>`;
    }

    const deposits = Array.isArray(planet.deposits) ? planet.deposits : [];
    let html = '';
    if (deposits.length === 0) {
        html += `<p style="color: #666; text-align: center; padding: 12px 0;">Залежей нет</p>`;
    } else {
        html += `<p style="color:#888; font-size:0.9rem; text-transform:uppercase;">Залежи (${deposits.length})</p>`;
        groupDeposits(deposits).forEach(g => {
            // «Выработано» — админская индикация (спека итерации 3 §6/п.26):
            // выработанные залежи (amount = 0) игроку не приходят, поэтому
            // счётчик ненулевой только у админа.
            const depleted = g.depleted > 0 ? ` (выработано: ${g.depleted})` : '';
            html += `
                <div style="margin:6px 0; padding:10px; background:#1a1a2e; border-radius:4px;">
                    <div><strong>${g.good_name || ('#' + g.good_id)}</strong></div>
                    <div style="color:#ccc; margin-top:4px;">Пятен: <strong>${g.count}</strong>${depleted}</div>
                    <div style="color:#ccc;">Запас: <strong>${formatNumber(g.amount)}</strong></div>
                    <div style="color:#ccc;">Богатство: <strong>${wealthRangeLabel(g)}</strong></div>
                </div>`;
        });
    }

    // Инструмент песочницы — только админу (§6). Залежи видны «там же», где
    // кнопка: результат добавления сразу обновляет блок.
    if (isAdmin()) html += adminDepositFormHtml();
    return html;
}

// initDepositsAdmin — наполняет список ресурсов и вешает кнопку добавления
// (вызывается после вставки вкладки в DOM).
function initDepositsAdmin(planet, container) {
    const btn = container.querySelector('#deposit-add-btn');
    if (!btn) return;
    const sel = container.querySelector('#deposit-good-select');
    const statusEl = container.querySelector('#deposit-add-status');

    loadDepositResources().then(list => {
        if (!sel || !sel.isConnected) return; // вкладка перерисована
        sel.innerHTML = list.length
            ? list.map(r => `<option value="${r.id}">${r.name}</option>`).join('')
            : '<option value="">ресурсов нет</option>';
    }).catch(() => {
        if (sel && sel.isConnected) sel.innerHTML = '<option value="">ресурсы недоступны</option>';
    });

    btn.addEventListener('click', () => submitAddDeposit(planet, container, statusEl));
}

// submitAddDeposit — POST /admin/planets/{id}/deposits (§6): успех заменяет
// блок deposits ответом (формат карточки §5.2); ошибки 404/409/422 — текстом.
async function submitAddDeposit(planet, container, statusEl) {
    const sel = container.querySelector('#deposit-good-select');
    const wealthEl = container.querySelector('#deposit-wealth');
    const amountEl = container.querySelector('#deposit-amount');
    if (!sel || !sel.value) {
        if (statusEl) statusEl.textContent = 'Выберите ресурс';
        return;
    }
    const body = { good_id: Number(sel.value) };
    if (wealthEl && wealthEl.value !== '') body.wealth = Number(wealthEl.value);
    if (amountEl && amountEl.value !== '') body.amount = Number(amountEl.value);

    const token = modalState.authToken || localStorage.getItem('token');
    if (statusEl) statusEl.textContent = 'Добавление…';
    try {
        const res = await fetch('/admin/planets/' + encodeURIComponent(planet.id) + '/deposits', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
            body: JSON.stringify(body)
        });
        const text = await res.text();
        if (!res.ok) {
            if (statusEl) statusEl.textContent = 'Ошибка ' + res.status + ': ' + text;
            return;
        }
        const data = JSON.parse(text);
        planet.deposits = Array.isArray(data.deposits) ? data.deposits : [];
        // Полная перерисовка вкладки: блок показывает новое состояние, форма
        // остаётся (список ресурсов берётся из кэша).
        renderTabContent('deposits', planet, container);
    } catch (e) {
        if (statusEl) statusEl.textContent = 'Ошибка: ' + e.message;
    }
}

// ---------- КОНТРАКТЫ (спека 2026-09-22-контракт-перелёт-и-доска §2–§3) ----------

// contractsGateOpen — пройден ли гейт знания планеты для доски (§2.2):
// player без знания доски не видит (сервер отдаёт 403). Один источник для
// renderContracts и initContracts — иначе вкладка рисует «нет данных», а
// initContracts всё равно дёргает сервер и тостит 403.
function contractsGateOpen(planet) {
    return isAdmin() || !!planet.knowledge;
}

// renderContracts — вкладка «Задания/Контракты» карточки планеты (§2.1):
// доска планеты (тип, заголовок, цена, «осталось N», требования, автор) и
// форма «Опубликовать» — только когда игрок стоит на этой планете (§3).
// Доска гейтится знанием планеты сервером (§2.2): player без знания — «нет
// данных». Балансы и «кто взял» не показываются (§2.2).
function renderContracts(planet) {
    if (!contractsGateOpen(planet)) {
        return `<p style="color: #666; text-align: center; padding: 20px 0;">Нет данных — купить отчёт</p>`;
    }
    let html = `<div data-contract-board><p style="color:#666; text-align:center; padding:12px 0;">Загрузка…</p></div>`;
    if (canPublishHere(modalState.myPosition, planet.id)) {
        html += publishFormHtml();
    } else {
        html += `<p style="color:#64748b; font-size:0.85rem; margin-top:12px;">Опубликовать контракт можно только с планеты, где вы находитесь</p>`;
    }
    return html;
}

// loadContracts — GET /api/planets/{id}/contracts (§2.3): доска планеты.
// Ошибки (403 «планета не известна», 404) — тостом, доска остаётся пустой.
// Доска всегда читается заново при открытии вкладки (свежесть важнее кэша).
// Обработчики «Взять» вешаются ЗДЕСЬ, после заполнения доски: строки
// появляются асинхронно, к моменту initContracts их ещё нет (иначе кнопка
// мертва — блокирующий дефект ревью).
async function loadContracts(planet, container) {
    const token = modalState.authToken || localStorage.getItem('token');
    let contracts = [];
    try {
        const res = await fetch('/api/planets/' + encodeURIComponent(planet.id) + '/contracts', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
        if (!res.ok) {
            notifyError('Не удалось загрузить контракты: ' + (await res.text()));
        } else {
            const data = await res.json();
            contracts = Array.isArray(data.contracts) ? data.contracts : [];
        }
    } catch (e) {
        notifyError('Не удалось загрузить контракты: ' + e.message);
    }
    const board = container.querySelector('[data-contract-board]');
    if (!board || !board.isConnected) return;
    board.innerHTML = boardHtml(contracts, Date.now());
    board.querySelectorAll('[data-contract-take]').forEach(btn => {
        btn.addEventListener('click', () => takeContract(planet, container, btn.dataset.contractTake, btn));
    });
}

// initContracts — грузит доску, наполняет список систем-назначений и вешает
// обработчик публикации (вызывается после вставки вкладки в DOM). Гейт знания
// не пройден — сервер не дёргаем (иначе 403-тост на пустой вкладке).
function initContracts(planet, container) {
    if (!contractsGateOpen(planet)) return;

    loadContracts(planet, container);

    const publishBtn = container.querySelector('[data-contract-publish]');
    if (publishBtn) {
        loadDestWorlds(container);
        publishBtn.addEventListener('click', () => publishContract(planet, container));
    }
}

// loadDestWorlds — список систем для выбора назначения перелёта (§3):
// GET /worlds (видимость решает сервер, 77a). Текущая система исключается.
async function loadDestWorlds(container) {
    const sel = container.querySelector('[data-contract-dest-world]');
    if (!sel) return;
    const token = modalState.authToken || localStorage.getItem('token');
    try {
        const res = await fetch('/worlds', { headers: { 'Authorization': 'Bearer ' + token } });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const worlds = await res.json();
        if (!sel.isConnected) return;
        const list = (Array.isArray(worlds) ? worlds : []).filter(w => w.id !== modalState.worldId);
        sel.innerHTML = '<option value="">система-назначение…</option>' +
            list.map(w => `<option value="${escapeHtml(w.id)}">${escapeHtml(w.name || w.id)}</option>`).join('');
    } catch (e) {
        if (sel.isConnected) sel.innerHTML = '<option value="">системы недоступны</option>';
    }
}

// takeContract — POST /api/contracts/take (§2.3): успех/ошибка тостом,
// доска обновляется после действия. Ошибка сервера (напр. «двигатель
// недостаточно быстр») показывается как есть.
async function takeContract(planet, container, contractID, btn) {
    const token = modalState.authToken || localStorage.getItem('token');
    if (btn) btn.disabled = true;
    try {
        const res = await fetch('/api/contracts/take', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
            body: JSON.stringify({ contract_id: contractID })
        });
        const text = await res.text();
        if (!res.ok) {
            notifyError(text || ('Ошибка ' + res.status));
        } else {
            notifySuccess('Контракт взят');
        }
    } catch (e) {
        notifyError('Не удалось взять контракт: ' + e.message);
    }
    // Доска обновляется после действия — только если вкладка ещё открыта
    // (иначе перезапишем контент другой вкладки).
    if (container.isConnected && modalState.activeTab === 'contracts') {
        renderTabContent('contracts', planet, container);
    }
}

// publishContract — POST /api/contracts (§3): публикация игроком с планеты,
// где он стоит. Тип «перелёт»: from_world_id — текущая система игрока,
// dest_world_id — выбранная система-назначение. Результат — тостом.
async function publishContract(planet, container) {
    const titleEl = container.querySelector('[data-contract-title]');
    const rewardEl = container.querySelector('[data-contract-reward]');
    const destEl = container.querySelector('[data-contract-dest-world]');
    const statusEl = container.querySelector('[data-contract-publish-status]');
    const title = titleEl ? titleEl.value.trim() : '';
    const reward = rewardEl ? Number(rewardEl.value) : 0;
    const destWorld = destEl ? destEl.value : '';
    if (!title) { if (statusEl) statusEl.textContent = 'Укажите заголовок'; return; }
    if (!reward || reward <= 0) { if (statusEl) statusEl.textContent = 'Укажите цену больше нуля'; return; }
    if (!destWorld) { if (statusEl) statusEl.textContent = 'Выберите систему-назначение'; return; }
    if (!modalState.worldId) { if (statusEl) statusEl.textContent = 'Не удалось определить текущую систему'; return; }

    const token = modalState.authToken || localStorage.getItem('token');
    if (statusEl) statusEl.textContent = 'Публикация…';
    try {
        const res = await fetch('/api/contracts', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
            body: JSON.stringify({
                planet_id: planet.id,
                type: 'travel',
                title: title,
                reward: reward,
                payload: { from_world_id: modalState.worldId, dest_world_id: destWorld, dest_planet_id: null }
            })
        });
        const text = await res.text();
        if (!res.ok) {
            if (statusEl) statusEl.textContent = 'Ошибка ' + res.status + ': ' + text;
            notifyError(text || ('Ошибка ' + res.status));
            return;
        }
        notifySuccess('Контракт опубликован');
    } catch (e) {
        if (statusEl) statusEl.textContent = 'Ошибка: ' + e.message;
        notifyError('Не удалось опубликовать контракт: ' + e.message);
        return;
    }
    if (container.isConnected && modalState.activeTab === 'contracts') {
        renderTabContent('contracts', planet, container);
    }
}

// ---------- ГЛАВНЫЙ ЭКСПОРТ ----------

// renderTabContent — рендерит контент вкладки в container.
export function renderTabContent(tab, planet, container) {
    switch (tab) {
        case 'general':
            container.innerHTML = renderGeneral(planet);
            // Большая картинка «Вид с орбиты» (спека 2026-09-20 §6.2):
            // асинхронная загрузка после вставки контейнера в DOM.
            renderOrbitView(planet, container);
            break;
        case 'resources':
            container.innerHTML = renderResources(planet);
            break;
        case 'deposits':
            container.innerHTML = renderDeposits(planet);
            initDepositsAdmin(planet, container);
            break;
        case 'settlements':
            container.innerHTML = renderSettlements(planet);
            initBranchesAdmin(planet, container);
            break;
        case 'factions':
            container.innerHTML = renderFactions(planet);
            break;
        case 'contracts':
            container.innerHTML = renderContracts(planet);
            initContracts(planet, container);
            break;
        default:
            container.innerHTML = '<p style="color: #666;">Неизвестная вкладка</p>';
    }

    // Иконки биомов (спека 2026-09-20 §6.3): PNG-иконка не загрузилась
    // (файла нет) → фолбэк эмодзи/«•» (onerror-биндинг, список id не
    // хардкодим — фолбэк по ошибке загрузки).
    container.querySelectorAll('img[data-biome-fallback]').forEach(img => {
        img.addEventListener('error', () => {
            img.outerHTML = img.dataset.biomeFallback;
        });
    });

    // Клик по спутнику — карточка спутника
    container.querySelectorAll('[data-sat-idx]').forEach(li => {
        li.addEventListener('click', () => {
            const idx = parseInt(li.dataset.satIdx, 10);
            const sat = planet.satellites && planet.satellites[idx];
            // Запоминаем выбор в состоянии (баг 2026-09-21): по id планеты и
            // спутника, а не по индексу — переживает перечитку данных.
            modalState.selectedSatellite = sat
                ? { planetId: planet.id, satelliteId: sat.id }
                : null;
            renderSatelliteCard(planet, sat, container);
        });
    });

    // Клик по фракции — инлайновый тоггл блока деталей столицы (§4.3):
    // повторный клик сворачивает, серверных вызовов нет (данные уже приехали).
    container.querySelectorAll('[data-faction-toggle]').forEach(el => {
        el.addEventListener('click', () => {
            const details = container.querySelector(`[data-faction-details="${el.dataset.factionToggle}"]`);
            if (details) details.style.display = details.style.display === 'none' ? 'block' : 'none';
        });
    });
}

// Стиль «Вид с орбиты» (спека 2026-09-20 §6.2): рамка 288px, на узких
// экранах (<900px) — 192px.
if (!document.getElementById('orbit-view-style')) {
    const style = document.createElement('style');
    style.id = 'orbit-view-style';
    style.textContent = `
        @media (max-width: 900px) {
            .orbit-view-frame { width: 192px !important; }
        }
    `;
    document.head.appendChild(style);
}