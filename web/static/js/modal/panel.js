// web/static/js/modal/panel.js
import { modalState } from './state.js';
import { drawSystem } from './modal_render.js';
import { renderTabContent } from './tabs.js';
import { planetPopulationAt } from './extrapolate.js';

// Перевод Кельвинов в Цельсии (для таблицы планет)
function kelvinToCelsius(k) {
    if (typeof k !== 'number' || isNaN(k)) return '—';
    return (k - 273.15).toFixed(1);
}

// capitalize — первая буква заглавная ('belrano' → 'Belrano')
function capitalize(s) {
    if (!s) return s || '';
    return s.charAt(0).toUpperCase() + s.slice(1);
}

// renderRightPanel — рисует правую панель модалки:
// карточку звезды со списком планет (selectedIndex === null/undefined)
// или карточку выбранной планеты.
export function renderRightPanel(planets, selectedIndex) {
    const panel = document.getElementById('right-panel');
    if (!panel) return;

    if (selectedIndex === null || selectedIndex === undefined) {
        renderStarCard();
    } else {
        renderCard(panel, planets, selectedIndex);
    }
}

// Ключ из web/static/js/admin/main.js (переключатель в админке) — держать
// строку синхронной при переименовании.
const AUTO_REFRESH_PLANET_KEY = 'debugAutoRefreshPlanet';

// stopAutoRefresh — гасит таймер автообновления карточки планеты (отладка).
// Вызывается при уходе с карточки конкретной планеты (в карточку звезды) или
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

// renderStarCard — рисует карточку звезды в правой панели: инфо по звезде
// и рядом компактный список планет системы. Это вид системы по умолчанию
// (при открытии и при снятии выделения планеты).
export function renderStarCard() {
    stopAutoRefresh();
    const panel = document.getElementById('right-panel');
    if (!panel) return;

    const name = modalState.worldName || 'Звезда';
    const spec = modalState.spectralClass || 'G';
    const color = modalState.starColor || '#fff4a3';
    const planets = (modalState.planets || []).slice();

    const temp = modalState.worldTemperature;
    const coordX = modalState.worldCoordX;
    const coordY = modalState.worldCoordY;

    const specInfo = getSpectralInfo(spec);

    panel.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
            <h3 style="margin: 0; font-size: 1.35rem; color: ${color}; cursor: default;">${capitalize(name)} <span style="font-size:0.9rem; color:#888; font-weight:normal;">(${spec})</span></h3>
            <span style="background:#2a2a4a; color:#aaa; padding:4px 12px; border-radius:12px; font-size:0.9rem;">Звезда ${spec}</span>
        </div>
        <div style="display:flex; flex-wrap:wrap; gap:12px; align-items:flex-start;">
            <div style="flex:1; min-width:170px;">
                <div style="display:flex; align-items:center; gap:12px; margin-bottom:12px; padding:10px; background:#0d0d1a; border-radius:8px;">
                    <div style="width:44px; height:44px; border-radius:50%; background: radial-gradient(circle at 35% 35%, #fff, ${color}); box-shadow:0 0 18px ${color};"></div>
                    <div>
                        <div style="font-size:1.05rem; color:#cbd5e1;"><strong>Спектральный класс:</strong> ${spec}</div>
                        <div style="font-size:1.05rem; color:#88b0e0;"><strong>Тип:</strong> ${specInfo.type}</div>
                        <div style="font-size:1.05rem; color:#cbd5e1;"><strong>Планет в системе:</strong> ${planets.length}</div>
                    </div>
                </div>
                <div style="font-size:1rem; line-height:1.7;">
                    <p style="margin:4px 0;"><strong>Температура:</strong> ${temp ? (temp - 273.15).toFixed(0) + ' °C' + ' (' + temp.toFixed(0) + ' K)' : '—'}</p>
                    <p style="margin:4px 0;"><strong>Цвет:</strong> ${specInfo.color}</p>
                    <p style="margin:4px 0;"><strong>Относительный радиус:</strong> ${specInfo.radius}</p>
                    <p style="margin:4px 0;"><strong>Светимость:</strong> ${specInfo.luminosity}</p>
                    <p style="margin:4px 0;"><strong>Координаты:</strong> (${coordX ? coordX.toFixed(2) : '—'}; ${coordY ? coordY.toFixed(2) : '—'})</p>
                    <p style="margin:4px 0;"><strong>Возраст:</strong> ${specInfo.age}</p>
                    <p style="margin:8px 0; color:#888; font-size:0.95rem;">${specInfo.description}</p>
                    <p style="margin:6px 0; color:#666;">🔄 Кликните по планете (в списке или на канвасе), чтобы открыть её характеристики. Клик по пустому месту — к звезде.</p>
                </div>
            </div>
            <div style="flex:1; min-width:170px; background:#0d0d1a; border-radius:8px; padding:10px;">
                <h4 style="margin:0 0 8px 0; font-size:1rem; color:#aaa;">Планеты (${planets.length})</h4>
                ${planetsTable(planets)}
            </div>
        </div>
    `;

    // Кликабельные строки планет — обработчики вешаем после вставки.
    panel.querySelectorAll('tr[data-index]').forEach(tr => {
        tr.addEventListener('mouseenter', () => { tr.style.background = '#1f1f3a'; });
        tr.addEventListener('mouseleave', () => { tr.style.background = 'transparent'; });
    });
}

// getSpectralInfo — справочник по спектральному классу для карточки звезды.
function getSpectralInfo(spec) {
    const map = {
        'O': { type: 'Голубой гигант', color: 'Голубой', radius: '16–25 R☉', luminosity: 'Высокая', age: 'Короткий (до 10 млн лет)', description: 'Очень горячие и яркие звёзды, живут недолго.' },
        'B': { type: 'Голубо-белый гигант', color: 'Голубо-белый', radius: '5–14 R☉', luminosity: 'Высокая', age: 'Короткий (50 млн лет)', description: 'Яркие массивные звёзды с сильным излучением.' },
        'A': { type: 'Белый', color: 'Белый', radius: '1.4–5 R☉', luminosity: 'Средняя', age: 'Обычная (до 1 млрд лет)', description: 'Белые звёзды, похожие на Сириус.' },
        'F': { type: 'Жёлто-белый', color: 'Жёлто-белый', radius: '1.1–2 R☉', luminosity: 'Средняя', age: 'Обычная (2–4 млрд лет)', description: 'Тёплые звёзды, чуть горячее Солнца.' },
        'G': { type: 'Жёлтый карлик', color: 'Жёлтый', radius: '0.9–1.2 R☉', luminosity: 'Обычная', age: 'Долгая (до 10 млрд лет)', description: 'Звёзды солнечного типа, стабильные и долгоживущие.' },
        'K': { type: 'Оранжевый карлик', color: 'Оранжевый', radius: '0.6–0.9 R☉', luminosity: 'Пониженная', age: 'Очень долгая (до 30 млрд лет)', description: 'Долгоживущие оранжевые звёзды, часто с пригодными для жизни зонами.' },
        'M': { type: 'Красный карлик', color: 'Красный', radius: '0.1–0.6 R☉', luminosity: 'Низкая', age: 'Чрезвычайно долгая (триллионы лет)', description: 'Самые распространённые звёзды галактики.' },
        'L': { type: 'Коричневый карлик', color: 'Красно-коричневый', radius: '0.05–0.1 R☉', luminosity: 'Очень низкая', age: 'Долгая', description: 'Недостаточно массивна для термоядерного синтеза водорода.' },
        'T': { type: 'Коричневый карлик (метановый)', color: 'Тёмно-красный', radius: '0.04–0.08 R☉', luminosity: 'Очень низкая', age: 'Долгая', description: 'Холодные коричневые карлики с метановой атмосферой.' },
        'Y': { type: 'Холодный коричневый карлик', color: 'Красновато-чёрный', radius: '0.02–0.05 R☉', luminosity: 'Минимальная', age: 'Долгая', description: 'Самые холодные карлики, едва теплее Юпитера.' }
    };
    return map[spec] || { type: 'Неизвестно', color: '—', radius: '—', luminosity: '—', age: '—', description: 'Данные отсутствуют.' };
}

// ---------- СПИСОК ПЛАНЕТ (в карточке звезды) ----------

// planetsTable — компактная таблица планет для карточки звезды.
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
        html += `
            <tr data-index="${idx}" style="border-bottom: 1px solid #1a1a2e; cursor: pointer;">
                <td style="padding:2px 4px;">${capitalize(p.name || (idx + 1))}</td>
                <td style="padding:2px 4px;">${p.type || '?'}</td>
                <td style="padding:2px 4px;">${p.size ? p.size.toFixed(1) : '-'}</td>
                <td style="padding:2px 4px;">${p.temperature ? kelvinToCelsius(p.temperature) + '°' : '-'}</td>
            </tr>
        `;
    });
    return html + '</tbody></table>';
}

// ---------- КАРТОЧКА ПЛАНЕТЫ ----------

// populationTrendArrow — ↓/↑/— рядом с числом населения относительно
// прошлого known-значения (modalState.previousPopulation, обновляется в
// refreshPlanets, index.js). Нет прошлого значения — стрелка не рисуется.
function populationTrendArrow(planet) {
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
        renderStarCard();
        return;
    }

    panel.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
            <h3 style="margin: 0; font-size: 1.2rem; color: #aaa;">${capitalize(planet.name) || `Планета #${selectedIndex + 1}`}${planet.population ? ` (<span id="planet-pop-num">${formatPopulation(planetPopulationAt(planet, Date.now()))}</span>${populationTrendArrow(planet)})` : ''}</h3>
            <div style="display:flex; gap:8px;">
                <button id="refresh-planet-btn" title="Пересчитать население от среды и перезагрузить данные" style="background: #2a2a4a; border: none; color: #aaa; padding: 6px 14px; border-radius: 4px; cursor: pointer; font-size: 0.95rem;">🔄 Обновить</button>
                <button id="back-to-list-btn" style="background: #2a2a4a; border: none; color: #aaa; padding: 6px 14px; border-radius: 4px; cursor: pointer; font-size: 0.95rem;">← Назад</button>
            </div>
        </div>
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