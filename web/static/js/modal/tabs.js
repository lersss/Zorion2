// web/static/js/modal/tabs.js
import { populationAt, planetPopulationAt } from './extrapolate.js';
import { modalState } from './state.js';
import { getPlanetTexture } from './textures.js';

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
function prettyName(name) {
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
function biomeIconHtml(form) {
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
    const onThisOrbit = myPos && myPos.status === 'orbit' &&
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

    // Тип (название в шапке карточки)
    html += `<p style="margin:4px 0;"><strong>Тип:</strong> ${planet.type || '—'}</p>`;

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
            </div>`;
    });
    return html;
}

// ---------- ЗАГЛУШКИ ----------

function renderFactionsStub() {
    return `<p style="color: #666; text-align: center; padding: 20px 0;">
        🏛️ Данные о фракциях будут доступны позже<br>
        <span style="font-size: 0.85rem;">(после реализации генерации фракций)</span>
    </p>`;
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
        case 'settlements':
            container.innerHTML = renderSettlements(planet);
            break;
        case 'factions':
            container.innerHTML = renderFactionsStub();
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