// web/static/js/modal/panel.js
import { modalState } from './state.js';
import { drawSystem } from './modal_render.js';
import { renderTabContent } from './tabs.js';
import { planetPopulationAt } from './extrapolate.js';

// Перевод Кельвинов в Цельсии (для таблицы планет). Экспорт — для
// энциклопедии (86a §5.1.2: окна рас в °C).
export function kelvinToCelsius(k) {
    if (typeof k !== 'number' || isNaN(k)) return '—';
    return (k - 273.15).toFixed(1);
}

// capitalize — первая буква заглавная ('belrano' → 'Belrano')
function capitalize(s) {
    if (!s) return s || '';
    return s.charAt(0).toUpperCase() + s.slice(1);
}

// formatStellarMass — масса звезды в M☉ (29a §4м): < 10 — 1 десятичный знак,
// ≥ 10 — целое; null/не число — «—». Экспорт — для тултипа звезды (70a).
export function formatStellarMass(m) {
    if (typeof m !== 'number' || !isFinite(m) || m <= 0) return '—';
    return (m >= 10 ? Math.round(m) : m.toFixed(1)) + ' M☉';
}

// formatAU — разделение пары: ≥ 100 а.е. — целое, иначе два знака.
// Экспорт — для тултипа звезды (70a).
export function formatAU(au) {
    if (typeof au !== 'number' || !isFinite(au) || au <= 0) return '—';
    return au >= 100 ? Math.round(au) : au.toFixed(2);
}

// exoticStarInfo — честные значения карточки экзотики (41a §5.1): таблица по
// типу объекта; числа — из принятой спеки 99.2.4 §5.1. Ветка «аккреция» для ЧД
// по stellar_mods (subtype/disk_state = accretion, решение §8.1 вариант б):
// T диска — статический диапазон 10⁵–10⁷ K, сама ЧД не излучает.
// Возвращает { temperature, color, radius, luminosity, age } — строки; для
// не-экзотики — null (карточка идёт по getSpectralInfo). Экспорт — для
// тултипа звезды (70a).
export function exoticStarInfo() {
    const starType = modalState.starType;
    if (!starType || starType === 'star') return null;

    const mods = modalState.stellarMods || {};
    const accretion = mods.subtype === 'accretion' || mods.disk_state === 'accretion';
    const temp = modalState.worldTemperature;
    const mass = modalState.stellarMass;
    const age = modalState.worldAge;

    const info = { temperature: '—', color: '—', radius: '—', luminosity: '—', age: '—' };

    switch (starType) {
        case 'black_hole':
            // Сама ЧД не излучает; аккреционный диск — голубовато-белый,
            // диапазон 10⁵–10⁷ K / 10²–10³ L☉ (§5.1). T=0 в данных не показываем.
            info.temperature = accretion ? 'диск: 10⁵–10⁷ K' : 'нет фотосферы (не излучает)';
            info.color = accretion ? 'голубовато-белый (диск)' : 'чёрный (тень)';
            info.luminosity = accretion ? 'диск: 10²–10³ L☉' : 'нет (не излучает)';
            // Горизонт событий: 2.95 × M/M☉ км (шварцшильдовский радиус, §5.2).
            if (typeof mass === 'number' && isFinite(mass) && mass > 0) {
                info.radius = 'горизонт событий ≈ ' + Math.round(2.95 * mass) + ' км';
            }
            break;
        case 'neutron':
            info.temperature = (typeof temp === 'number' && temp > 0) ? temp.toLocaleString('ru-RU') + ' K' : '—';
            info.color = 'голубой/белый';
            info.radius = '≈10–15 км';
            info.luminosity = '0.01–1 L☉';
            break;
        case 'white_dwarf':
            info.temperature = (typeof temp === 'number' && temp > 0) ? temp.toLocaleString('ru-RU') + ' K' : '—';
            info.color = 'белый/серебристый';
            info.radius = '≈0.01 R☉ (≈7000 км)';
            info.luminosity = '10⁻²–10⁻⁴ L☉';
            break;
        case 'protostar':
            info.temperature = (typeof temp === 'number' && temp > 0) ? temp.toLocaleString('ru-RU') + ' K' : '—';
            info.color = 'красно-оранжевый';
            info.radius = 'порядка R☉ и больше (сжимается)';
            info.luminosity = '1–10² L☉';
            break;
        default:
            return info; // неизвестный экзотический тип — «—» по всем полям, без падения карточки
    }

    // Возраст (§5.2): ≥ 0.1 млрд — «N млрд лет»; < 0.1 (протозвезда) —
    // «молодая: ≈N млн лет»; нет данных (старые миры) — «—».
    if (typeof age === 'number') {
        info.age = age >= 0.1
            ? age.toFixed(1) + ' млрд лет'
            : 'молодая: ≈' + Math.round(age * 1000) + ' млн лет';
    }
    return info;
}

// renderRightPanel — рисует правую панель модалки:
// список планет системы (selectedIndex === null/undefined)
// или карточку выбранной планеты.
export function renderRightPanel(planets, selectedIndex) {
    const panel = document.getElementById('right-panel');
    if (!panel) return;

    if (selectedIndex === null || selectedIndex === undefined) {
        renderPlanetsList();
    } else {
        renderCard(panel, planets, selectedIndex);
    }
}

// Ключ из web/static/js/admin/main.js (переключатель в админке) — держать
// строку синхронной при переименовании.
const AUTO_REFRESH_PLANET_KEY = 'debugAutoRefreshPlanet';

// stopAutoRefresh — гасит таймер автообновления карточки планеты (отладка).
// Вызывается при уходе с карточки конкретной планеты (в список планет) или
// при закрытии модалки — вне карточки планеты обновлять нечего.
function stopAutoRefresh() {
    if (modalState.autoRefreshTimer !== null) {
        clearInterval(modalState.autoRefreshTimer);
        modalState.autoRefreshTimer = null;
    }
}

// syncAutoRefreshTimer — запускает таймер, если включён переключатель в
// админке и он ещё не запущен. Не привязан к конкретной планете: каждый тик
// вызывает refreshPlanets(), которая обновляет то, что выбрано в модалке
// на момент тика, — переключение между планетами внутри модалки не требует
// перезапуска таймера.
function syncAutoRefreshTimer() {
    const enabled = localStorage.getItem(AUTO_REFRESH_PLANET_KEY) === '1';
    if (!enabled) {
        stopAutoRefresh();
        return;
    }
    if (modalState.autoRefreshTimer !== null) return;
    modalState.autoRefreshTimer = setInterval(() => {
        import('./index.js').then(mod => mod.refreshPlanets());
    }, 3000);
}

// renderPlanetsList — список планет системы в правой панели (70a): вид по
// умолчанию при открытии модалки и при снятии выделения планеты. Информация
// о звезде вынесена в тултип при наведении на звезду на канвасе (events.js).
export function renderPlanetsList() {
    stopAutoRefresh();
    const panel = document.getElementById('right-panel');
    if (!panel) return;

    const planets = (modalState.planets || []).slice();

    // Модалка без деталей системы (403, спека 77a §5.5/И11): звезда открыта,
    // планеты/поселения — честная заглушка вместо списка (стиль как
    // «Нет данных — купить отчёт» в tabs.js).
    if (modalState.restricted) {
        panel.innerHTML = `
            <div style="background:#0d0d1a; border-radius:8px; padding:10px;">
                <h4 style="margin:0 0 8px 0; font-size:1rem; color:#aaa;">Планеты</h4>
                <p style="margin:8px 0; padding:8px 10px; background:rgba(148,163,184,0.08); border:1px dashed rgba(148,163,184,0.3); border-radius:8px; color:#94a3b8; font-size:0.85rem;">
                    Система вне зоны видимости: детали (планеты, поселения) недоступны — долетите или купите отчёт
                </p>
            </div>
        `;
        return;
    }

    panel.innerHTML = `
        <div style="background:#0d0d1a; border-radius:8px; padding:10px;">
            <h4 style="margin:0 0 8px 0; font-size:1rem; color:#aaa;">Планеты (${planets.length})</h4>
            ${planetsTable(planets)}
        </div>
    `;

    // Кликабельные строки планет — обработчики вешаем после вставки.
    panel.querySelectorAll('tr[data-index]').forEach(tr => {
        tr.addEventListener('mouseenter', () => { tr.style.background = '#1f1f3a'; });
        tr.addEventListener('mouseleave', () => { tr.style.background = 'transparent'; });
    });
}

// getSpectralInfo — справочник по спектральному классу для карточки звезды.
// Экспорт — для тултипа звезды (70a) и энциклопедии (86a §5.2).
// temp — диапазон температуры по классу (стандартная астрофизика, спека 86a
// §5.2.2; предположение дизайнера, эталон не задан — гейт §12.5).
export function getSpectralInfo(spec) {
    const map = {
        'O': { type: 'Голубой гигант', color: 'Голубой', radius: '16–25 R☉', luminosity: 'Высокая', age: 'Короткий (до 10 млн лет)', description: 'Очень горячие и яркие звёзды, живут недолго.', temp: '30 000–60 000 K' },
        'B': { type: 'Голубо-белый гигант', color: 'Голубо-белый', radius: '5–14 R☉', luminosity: 'Высокая', age: 'Короткий (50 млн лет)', description: 'Яркие массивные звёзды с сильным излучением.', temp: '10 000–30 000 K' },
        'A': { type: 'Белый', color: 'Белый', radius: '1.4–5 R☉', luminosity: 'Средняя', age: 'Обычная (до 1 млрд лет)', description: 'Белые звёзды, похожие на Сириус.', temp: '7 500–10 000 K' },
        'F': { type: 'Жёлто-белый', color: 'Жёлто-белый', radius: '1.1–2 R☉', luminosity: 'Средняя', age: 'Обычная (2–4 млрд лет)', description: 'Тёплые звёзды, чуть горячее Солнца.', temp: '6 000–7 500 K' },
        'G': { type: 'Жёлтый карлик', color: 'Жёлтый', radius: '0.9–1.2 R☉', luminosity: 'Обычная', age: 'Долгая (до 10 млрд лет)', description: 'Звёзды солнечного типа, стабильные и долгоживущие.', temp: '5 200–6 000 K' },
        'K': { type: 'Оранжевый карлик', color: 'Оранжевый', radius: '0.6–0.9 R☉', luminosity: 'Пониженная', age: 'Очень долгая (до 30 млрд лет)', description: 'Долгоживущие оранжевые звёзды, часто с пригодными для жизни зонами.', temp: '3 700–5 200 K' },
        'M': { type: 'Красный карлик', color: 'Красный', radius: '0.1–0.6 R☉', luminosity: 'Низкая', age: 'Чрезвычайно долгая (триллионы лет)', description: 'Самые распространённые звёзды галактики.', temp: '2 400–3 700 K' },
        'L': { type: 'Коричневый карлик', color: 'Красно-коричневый', radius: '0.05–0.1 R☉', luminosity: 'Очень низкая', age: 'Долгая', description: 'Недостаточно массивна для термоядерного синтеза водорода.', temp: '1 300–2 400 K' },
        'T': { type: 'Коричневый карлик (метановый)', color: 'Тёмно-красный', radius: '0.04–0.08 R☉', luminosity: 'Очень низкая', age: 'Долгая', description: 'Холодные коричневые карлики с метановой атмосферой.', temp: '700–1 300 K' },
        'Y': { type: 'Холодный коричневый карлик', color: 'Красновато-чёрный', radius: '0.02–0.05 R☉', luminosity: 'Минимальная', age: 'Долгая', description: 'Самые холодные карлики, едва теплее Юпитера.', temp: '< 700 K' }
    };
    return map[spec] || { type: 'Неизвестно', color: '—', radius: '—', luminosity: '—', age: '—', description: 'Данные отсутствуют.', temp: '—' };
}

// exoticStarReference — статический справочник экзотических объектов (41a
// §5.1): цвет/радиус/светимость/температура-описание без привязки к
// конкретной системе (в отличие от exoticStarInfo, где часть полей — из
// modalState). Экспорт — для энциклопедии (86a §5.2: карточки экзотики).
export function exoticStarReference(starType) {
    const ref = {
        'black_hole': {
            temperature: 'нет фотосферы (не излучает); аккреционный диск — 10⁵–10⁷ K',
            color: 'чёрный (тень); диск — голубовато-белый',
            radius: 'горизонт событий ≈ 2.95 × M/M☉ км',
            luminosity: 'нет (не излучает); диск — 10²–10³ L☉',
        },
        'neutron': {
            temperature: 'фактическая (см. систему)',
            color: 'голубой/белый',
            radius: '≈10–15 км',
            luminosity: '0.01–1 L☉',
        },
        'white_dwarf': {
            temperature: 'фактическая (см. систему)',
            color: 'белый/серебристый',
            radius: '≈0.01 R☉ (≈7000 км)',
            luminosity: '10⁻²–10⁻⁴ L☉',
        },
        'protostar': {
            temperature: 'фактическая (см. систему)',
            color: 'красно-оранжевый',
            radius: 'порядка R☉ и больше (сжимается)',
            luminosity: '1–10² L☉',
        },
    };
    return ref[starType] || null;
}

// ---------- СПИСОК ПЛАНЕТ (правая панель) ----------

// planetsTable — компактная таблица планет для правой панели.
// Строки data-index — по ним работает общий клик в index.js.
function planetsTable(planets) {
    if (!planets || planets.length === 0) {
        return '<div style="color:#666; font-size:0.9rem;">Нет планет</div>';
    }
    let html = `
        <table style="width:100%; border-collapse: collapse; font-size: 0.9rem;">
            <thead>
                <tr>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding:2px 4px;">Планета</th>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding:2px 4px;">Тип</th>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding:2px 4px;">Раз.</th>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333; padding:2px 4px;">T</th>
                </tr>
            </thead>
            <tbody>
    `;
    planets.forEach((p, idx) => {
        // Населённые планеты отмечаем домиком (пожелание создателя 2026-09-17).
        const inhabited = planetPopulationAt(p, Date.now()) > 0;
        const nameCell = inhabited
            ? `<span title="Населена">🏠 ${capitalize(p.name || (idx + 1))}</span>`
            : `${capitalize(p.name || (idx + 1))}`;
        html += `
            <tr data-index="${idx}" style="border-bottom: 1px solid #1a1a2e; cursor: pointer;">
                <td style="padding:2px 4px;">${nameCell}</td>
                <td style="padding:2px 4px;">${p.type || '?'}</td>
                <td style="padding:2px 4px;">${p.size ? p.size.toFixed(1) : '-'}</td>
                <td style="padding:2px 4px;">${p.temperature ? kelvinToCelsius(p.temperature) + '°' : '-'}</td>
            </tr>
        `;
    });
    return html + '</tbody></table>';
}

// ---------- КАРТОЧКА ПЛАНЕТЫ ----------

// populationTrendArrow — ↓/↑/— рядом с числом населения. Направление из
// ТЕКУЩИХ данных объекта, а не только из дельты серверных снапшотов
// (99.2.12): признак снижения (любое поселение с r_per_sec > 0 — жара,
// или lambda_per_hour > 0 — холод/гравитация/радиация) → ↓ сразу при
// открытии карточки; дельта двух снапшотов (modalState.previousPopulation,
// refreshPlanets) — запасной вариант для роста и равновесия. При конфликте
// живой сигнал снижения приоритетен (99.2.16: рост есть — рождаемость; живой
// сигнал показывает только убыль, рост читается дельтой снапшотов).
function populationTrendArrow(planet) {
    if (planet.settlements && planet.settlements.length) {
        const declining = planet.settlements.some(s => s && (
            (typeof s.r_per_sec === 'number' && s.r_per_sec > 0) ||
            (typeof s.lambda_per_hour === 'number' && s.lambda_per_hour > 0)
        ));
        if (declining) return ' <span style="color:#f66;" title="Население убывает">↓</span>';
    }
    const prev = modalState.previousPopulation[planet.id];
    if (typeof prev !== 'number' || typeof planet.population !== 'number') return '';
    if (planet.population < prev) return ' <span style="color:#f66;" title="Население убывает">↓</span>';
    if (planet.population > prev) return ' <span style="color:#6f6;" title="Население растёт">↑</span>';
    return ' <span style="color:#888;" title="Без изменений">—</span>';
}

// formatPopulation — "1 234 567"
function formatPopulation(n) {
    return n.toLocaleString('ru-RU');
}

function renderCard(panel, planets, selectedIndex) {
    const planet = planets[selectedIndex];
    if (!planet) {
        renderPlanetsList();
        return;
    }

    // Внутрисистемная позиция игрока (спека 99.2.27 §5.11): бейдж «● Вы на
    // орбите» в карточке объекта + строка «Корабли на орбите: N» (§5.12).
    const myPos = modalState.myPosition;
    const onThisOrbit = myPos && myPos.status === 'orbit' &&
        myPos.object_type === 'planet' && myPos.object_id === planet.id;
    const orbitBadge = onThisOrbit
        ? `<span style="background:rgba(74,222,128,0.15); border:1px solid rgba(74,222,128,0.4); color:#4ade80; border-radius:10px; padding:2px 8px; font-size:0.8rem; margin-left:8px;">● Вы на орбите</span>`
        : '';
    const shipsHere = (modalState.systemPlayers || []).filter(p =>
        p.object_type === 'planet' && p.object_id === planet.id
    ).length;
    const shipsLine = shipsHere > 0
        ? `<div style="margin:4px 0; color:#94a3b8; font-size:0.85rem;">Корабли на орбите: ${shipsHere}</div>`
        : '';

    panel.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
            <h3 style="margin: 0; font-size: 1.2rem; color: #aaa;">${capitalize(planet.name) || `Планета #${selectedIndex + 1}`}${planet.population ? ` (<span id="planet-pop-num">${formatPopulation(planetPopulationAt(planet, Date.now()))}</span>${populationTrendArrow(planet)})` : ''}${orbitBadge}</h3>
            <div style="display:flex; gap:8px;">
                <button id="intra-fly-btn" title="Внутрисистемный полёт на орбиту планеты (спека 99.2.27)" style="background: #2a2a4a; border: none; color: #fde68a; padding: 6px 14px; border-radius: 4px; cursor: pointer; font-size: 0.95rem;">🚀 Лететь</button>
                <button id="refresh-planet-btn" title="Пересчитать население от среды и перезагрузить данные" style="background: #2a2a4a; border: none; color: #aaa; padding: 6px 14px; border-radius: 4px; cursor: pointer; font-size: 0.95rem;">🔄 Обновить</button>
                <button id="back-to-list-btn" style="background: #2a2a4a; border: none; color: #aaa; padding: 6px 14px; border-radius: 4px; cursor: pointer; font-size: 0.95rem;">← Назад</button>
            </div>
        </div>
        ${shipsLine}
        <div style="display: flex; gap: 8px; margin-bottom: 12px; border-bottom: 1px solid #333; padding-bottom: 8px;">
            <button class="tab-btn" data-tab="general" style="background: none; border: none; color: #888; padding: 4px 12px; cursor: pointer; font-size: 0.95rem; border-radius: 4px;">Общее</button>
            <button class="tab-btn" data-tab="resources" style="background: none; border: none; color: #888; padding: 4px 12px; cursor: pointer; font-size: 0.95rem; border-radius: 4px;">Ресурсы</button>
            <button class="tab-btn" data-tab="settlements" style="background: none; border: none; color: #888; padding: 4px 12px; cursor: pointer; font-size: 0.95rem; border-radius: 4px;">Поселения</button>
            <button class="tab-btn" data-tab="factions" style="background: none; border: none; color: #888; padding: 4px 12px; cursor: pointer; font-size: 0.95rem; border-radius: 4px;">Фракции</button>
        </div>
        <div id="tab-content" style="font-size: 1rem; line-height: 1.7;"></div>
    `;

    const tabBtns = panel.querySelectorAll('.tab-btn');
    const tabContent = panel.querySelector('#tab-content');

    function switchTab(tab) {
        modalState.activeTab = tab;
        tabBtns.forEach(btn => {
            btn.style.color = btn.dataset.tab === tab ? '#fff' : '#888';
            btn.style.background = btn.dataset.tab === tab ? '#2a2a4a' : 'none';
        });
        renderTabContent(tab, planet, tabContent);
    }

    tabBtns.forEach(btn => {
        btn.addEventListener('click', () => switchTab(btn.dataset.tab));
    });

    // Открытие карточки — с «Общее»; обновление данных (см. refreshPlanets в
    // index.js) не трогает вкладку, на которой стоял игрок.
    switchTab(modalState.activeTab || 'general');

    // Обновить — перечитывает планеты мира заново, без пересоздания модалки
    // и без сброса текущей вкладки. Пересчёт населения от среды происходит
    // на сервере при каждом чтении (18a_population_death.md), кнопка просто
    // вытягивает свежий результат. Динамический import вместо прямого —
    // index.js импортирует panel.js, статический импорт обратно дал бы цикл
    // модулей.
    const refreshBtn = panel.querySelector('#refresh-planet-btn');
    if (refreshBtn) {
        refreshBtn.addEventListener('click', () => {
            import('./index.js').then(mod => mod.refreshPlanets());
        });
    }

    // Кнопка «🚀 Лететь» (спека 99.2.27 §5.5/§5.6): внутрисистемный полёт на
    // орбиту планеты. Доступна только в своей системе (my_position != null);
    // disabled при: нет двигателя (91a), цель == текущая позиция, цель ==
    // активный полёт, летим ОТ этой планеты (запрос создателя «глупый тост»).
    // Модалка не закрывается — полоса полёта в тике.
    const flyBtn = panel.querySelector('#intra-fly-btn');
    if (flyBtn) {
        const myPos = modalState.myPosition;
        const disabled = !myPos || !modalState.hasEngine ||
            (myPos.status === 'orbit' && myPos.object_type === 'planet' && myPos.object_id === planet.id) ||
            (myPos.status === 'in_flight' && myPos.to_type === 'planet' && myPos.to_id === planet.id) ||
            (myPos.status === 'in_flight' && myPos.from_type === 'planet' && myPos.from_id === planet.id);
        if (disabled) {
            flyBtn.disabled = true;
            flyBtn.style.opacity = '0.4';
            flyBtn.style.cursor = 'not-allowed';
            if (!modalState.hasEngine) flyBtn.title = 'Двигатель не установлен';
            else if (!myPos) flyBtn.title = 'Внутрисистемный полёт — только в своей системе';
            else flyBtn.title = 'Вы уже на орбите этого объекта';
        } else {
            flyBtn.addEventListener('click', () => {
                import('./events.js').then(m => m.startIntraFlight('planet', planet.id));
            });
        }
    }

    syncAutoRefreshTimer();

    const backBtn = panel.querySelector('#back-to-list-btn');
    if (backBtn) {
        backBtn.addEventListener('click', () => {
            modalState.selectedPlanetIndex = null;
            modalState.selectedObject = null;
            renderRightPanel(planets, null);
            const canvas = document.getElementById('system-canvas');
            if (canvas) {
                drawSystem(
                    canvas,
                    modalState.spectralClass,
                    planets,
                    modalState.starRadius,
                    modalState.starColor,
                    modalState.canvasWidth,
                    modalState.canvasHeight
                );
            }
        });
    }
}