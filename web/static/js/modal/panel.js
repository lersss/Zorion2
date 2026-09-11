// web/static/js/modal/panel.js
import { modalState } from './state.js';
import { drawSystem } from './modal_render.js';
import { renderTabContent } from './tabs.js';

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
// список планет (selectedIndex === null) или карточку планеты.
export function renderRightPanel(planets, selectedIndex) {
    const panel = document.getElementById('right-panel');
    if (!panel) return;

    if (selectedIndex === null || selectedIndex === undefined) {
        renderList(panel, planets);
    } else {
        renderCard(panel, planets, selectedIndex);
    }
}

// renderStarCard — рисует карточку звезды в правой панели.
export function renderStarCard() {
    const panel = document.getElementById('right-panel');
    if (!panel) return;

    const name = modalState.worldName || 'Звезда';
    const spec = modalState.spectralClass || 'G';
    const color = modalState.starColor || '#fff4a3';
    const planetsCount = (modalState.planets || []).length;

    const temp = modalState.worldTemperature;
    const coordX = modalState.worldCoordX;
    const coordY = modalState.worldCoordY;

    const specInfo = getSpectralInfo(spec);

    panel.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
            <h3 style="margin: 0; font-size: 1.35rem; color: ${color}; cursor: default;">${capitalize(name)} <span style="font-size:0.9rem; color:#888; font-weight:normal;">(${spec})</span></h3>
            <span style="background:#2a2a4a; color:#aaa; padding:4px 12px; border-radius:12px; font-size:0.9rem;">Звезда ${spec}</span>
        </div>
        <div style="display:flex; align-items:center; gap:12px; margin-bottom:12px; padding:10px; background:#0d0d1a; border-radius:8px;">
            <div style="width:44px; height:44px; border-radius:50%; background: radial-gradient(circle at 35% 35%, #fff, ${color}); box-shadow:0 0 18px ${color};"></div>
            <div>
                <div style="font-size:1.05rem; color:#cbd5e1;"><strong>Спектральный класс:</strong> ${spec}</div>
                <div style="font-size:1.05rem; color:#88b0e0;"><strong>Тип:</strong> ${specInfo.type}</div>
                <div style="font-size:1.05rem; color:#cbd5e1;"><strong>Планет в системе:</strong> ${planetsCount}</div>
            </div>
        </div>
        <div style="font-size:1rem; line-height:1.7;">
            <p style="margin:4px 0;"><strong>Температура:</strong> ${temp ? temp.toFixed(0) + ' K' + ' (' + (temp - 273.15).toFixed(0) + ' °C)' : '—'}</p>
            <p style="margin:4px 0;"><strong>Цвет:</strong> ${specInfo.color}</p>
            <p style="margin:4px 0;"><strong>Относительный радиус:</strong> ${specInfo.radius}</p>
            <p style="margin:4px 0;"><strong>Светимость:</strong> ${specInfo.luminosity}</p>
            <p style="margin:4px 0;"><strong>Координаты:</strong> (${coordX ? coordX.toFixed(2) : '—'}; ${coordY ? coordY.toFixed(2) : '—'})</p>
            <p style="margin:4px 0;"><strong>Возраст:</strong> ${specInfo.age}</p>
            <p style="margin:8px 0; color:#888; font-size:0.95rem;">${specInfo.description}</p>
            <p style="margin:6px 0;">🔄 Кликните по планете, чтобы открыть её характеристики.</p>
            <p style="margin:6px 0; color:#666;">Клик по пустому пространству снимает выделение.</p>
        </div>
    `;
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

// ---------- СПИСОК ПЛАНЕТ ----------

function renderList(panel, planets) {
    panel.innerHTML = `
        <h3 style="margin: 0 0 8px 0; font-size: 1.2rem; color: #aaa;">Планеты</h3>
        <table style="width:100%; border-collapse: collapse; font-size: 0.95rem;">
            <thead>
                <tr>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333;">#</th>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333;">Тип</th>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333;">Размер</th>
                    <th style="text-align:left; color:#888; border-bottom:1px solid #333;">T</th>
                </tr>
            </thead>
            <tbody id="planet-list-body"></tbody>
        </table>
    `;

    const tbody = panel.querySelector('#planet-list-body');
    if (!planets || planets.length === 0) {
        tbody.innerHTML = `<tr><td colspan="4" style="text-align:center;color:#666;">Нет планет</td></tr>`;
        return;
    }

    planets.forEach((p, idx) => {
        const tr = document.createElement('tr');
        tr.dataset.index = idx;
        tr.style.cssText = `border-bottom: 1px solid #1a1a2e; cursor: pointer;`;
        tr.innerHTML = `
            <td>${capitalize(p.name || (idx + 1))}</td>
            <td>${p.type || 'неизвестно'}</td>
            <td>${p.size ? p.size.toFixed(2) : '-'}</td>
            <td>${p.temperature ? kelvinToCelsius(p.temperature) + ' °C' : '-'}</td>
        `;
        tr.addEventListener('mouseenter', () => { tr.style.background = '#1f1f3a'; });
        tr.addEventListener('mouseleave', () => { tr.style.background = 'transparent'; });
        tbody.appendChild(tr);
    });
}

// ---------- КАРТОЧКА ПЛАНЕТЫ ----------

function renderCard(panel, planets, selectedIndex) {
    const planet = planets[selectedIndex];
    if (!planet) {
        renderList(panel, planets);
        return;
    }

    panel.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
            <h3 style="margin: 0; font-size: 1.2rem; color: #aaa;">${capitalize(planet.name) || `Планета #${selectedIndex + 1}`}</h3>
            <button id="back-to-list-btn" style="background: #2a2a4a; border: none; color: #aaa; padding: 6px 14px; border-radius: 4px; cursor: pointer; font-size: 0.95rem;">← Назад</button>
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
        tabBtns.forEach(btn => {
            btn.style.color = btn.dataset.tab === tab ? '#fff' : '#888';
            btn.style.background = btn.dataset.tab === tab ? '#2a2a4a' : 'none';
        });
        renderTabContent(tab, planet, tabContent);
    }

    tabBtns.forEach(btn => {
        btn.addEventListener('click', () => switchTab(btn.dataset.tab));
    });

    switchTab('general');

    const backBtn = panel.querySelector('#back-to-list-btn');
    if (backBtn) {
        backBtn.addEventListener('click', () => {
            modalState.selectedPlanetIndex = null;
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