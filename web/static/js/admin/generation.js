// web/static/js/admin/generation.js
import { fetchWithAuth } from './auth.js';
import { loadStats } from './stats.js';
import { loadWorlds } from './worlds.js';
import { pollJob, pollIntervals, runningJobs } from './poll.js';
import { notifyError, notifyInfo } from '../ui/toast.js';

// Пресеты генерации вселенной (проверены: 100k миров, 200 кластеров).
const UNIVERSE_PRESETS = {
    dense: {   // плотная галактика
        worlds: 100000, clusters: 200, mapSize: 45000, minDist: 170,
        clusterRadius: 2500, clusterSpacing: 5000, outlierPercent: 20
    },
    sparse: {  // просторная: большая галактика, миры дальше друг от друга
        worlds: 100000, clusters: 200, mapSize: 50000, minDist: 200,
        clusterRadius: 2500, clusterSpacing: 5000, outlierPercent: 20
    },
    compact: { // тесная: компактная галактика, миры близко, больше выбросов
        worlds: 100000, clusters: 200, mapSize: 36000, minDist: 150,
        clusterRadius: 1920, clusterSpacing: 3200, outlierPercent: 25
    },
    prototype: { // тестовая галактика: один мир для прототипа поселения
        worlds: 1, clusters: 1, mapSize: 800, minDist: 150,
        clusterRadius: 80, clusterSpacing: 160, outlierPercent: 0
    },
    swarm: {   // рой: компактная группа мелких миров
        worlds: 300, clusters: 6, mapSize: 3000, minDist: 170,
        clusterRadius: 744, clusterSpacing: 1300, outlierPercent: 15, shape: 'blob'
    },
    frontier: {   // пограничье: просторная галактика, много выбросов
        worlds: 1000, clusters: 8, mapSize: 6500, minDist: 230,
        clusterRadius: 1480, clusterSpacing: 3000, outlierPercent: 30, shape: 'blob'
    },
    constellation: {   // созвездие: много кластеров, круглая форма
        worlds: 3000, clusters: 40, mapSize: 8000, minDist: 150,
        clusterRadius: 780, clusterSpacing: 1350, outlierPercent: 10, shape: 'circle'
    },
    crossroads: {   // перекрёсток: средняя галактика, много миров
        worlds: 10000, clusters: 30, mapSize: 13000, minDist: 160,
        clusterRadius: 1764, clusterSpacing: 3000, outlierPercent: 20, shape: 'blob'
    },
    archipelago: {   // архипелаг: крупные острова-кластеры, круглая форма
        worlds: 20000, clusters: 12, mapSize: 21000, minDist: 180,
        clusterRadius: 4740, clusterSpacing: 8000, outlierPercent: 15, shape: 'circle'
    },
    outback: {   // глубинка: много кластеров, много выбросов
        worlds: 30000, clusters: 80, mapSize: 34000, minDist: 210,
        clusterRadius: 2250, clusterSpacing: 4600, outlierPercent: 30, shape: 'blob'
    },
    agglomeration: {   // скопление: очень много кластеров
        worlds: 50000, clusters: 150, mapSize: 30000, minDist: 160,
        clusterRadius: 1848, clusterSpacing: 3100, outlierPercent: 12, shape: 'blob'
    },
    fararm: {   // дальняя рука: огромная галактика, круглая форма
        worlds: 75000, clusters: 40, mapSize: 47000, minDist: 200,
        clusterRadius: 4800, clusterSpacing: 9700, outlierPercent: 18, shape: 'circle'
    },
    metropolis: {   // метрополия: максимум миров, много кластеров
        worlds: 100000, clusters: 300, mapSize: 42000, minDist: 165,
        clusterRadius: 1738, clusterSpacing: 3200, outlierPercent: 15, shape: 'blob'
    },
    outskirts: {   // окраина: огромная галактика, много выбросов
        worlds: 100000, clusters: 60, mapSize: 50000, minDist: 190,
        clusterRadius: 4136, clusterSpacing: 7600, outlierPercent: 35, shape: 'blob'
    },
};

// applyPreset — заполняет поля формы из выбранного пресета.
export function applyPreset() {
    const p = UNIVERSE_PRESETS[document.getElementById('genPreset').value];
    if (!p) return;
    document.getElementById('genWorlds').value = p.worlds;
    document.getElementById('genClusters').value = p.clusters;
    document.getElementById('genMapSize').value = p.mapSize;
    document.getElementById('genMinDist').value = p.minDist;
    document.getElementById('genClusterRadius').value = p.clusterRadius;
    document.getElementById('genClusterSpacing').value = p.clusterSpacing;
    document.getElementById('genOutlierPercent').value = p.outlierPercent;
    document.getElementById('genShape').value = p.shape || 'blob';
}

export async function generateUniverse() {
    const worlds = parseInt(document.getElementById('genWorlds').value);
    const clusters = parseInt(document.getElementById('genClusters').value);
    const mapSize = parseInt(document.getElementById('genMapSize').value);
    const minDist = parseInt(document.getElementById('genMinDist').value);
    const clusterRadius = parseInt(document.getElementById('genClusterRadius').value);
    const clusterSpacing = parseInt(document.getElementById('genClusterSpacing').value);
    const outlierPercent = parseInt(document.getElementById('genOutlierPercent').value);
    const shape = document.getElementById('genShape').value;
    if (isNaN(worlds) || isNaN(clusters) || worlds <= 0 || clusters <= 0 || isNaN(mapSize) || mapSize <= 0 || isNaN(minDist) || minDist <= 0 || isNaN(clusterRadius) || clusterRadius <= 0 || isNaN(clusterSpacing) || clusterSpacing <= 0 || isNaN(outlierPercent) || outlierPercent < 0) {
        document.getElementById('genResult').textContent = '❌ Введите корректные числа';
        return;
    }
    if (!confirm(`Сгенерировать ${worlds} миров в ${clusters} кластерах?`)) return;

    document.getElementById('genResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('genProgress').style.display = 'block';
    document.getElementById('genProgressBar').value = 0;
    document.getElementById('genProgressText').textContent = '0 из ' + worlds;
    document.getElementById('cancelUniverseBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                world_count: worlds,
                cluster_count: clusters,
                map_size: mapSize,
                min_dist: minDist,
                cluster_radius: clusterRadius,
                cluster_spacing: clusterSpacing,
                outlier_percent: outlierPercent,
                shape: shape
            })
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('genResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('genProgress').style.display = 'none';
            document.getElementById('cancelUniverseBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['universe']) clearInterval(pollIntervals['universe']);
        pollIntervals['universe'] = setInterval(() => pollJob('generate_universe', 'genProgress', 'genResult', 'cancelUniverseBtn'), 1500);
    } catch (e) {
        document.getElementById('genResult').textContent = '❌ ' + e.message;
        document.getElementById('genProgress').style.display = 'none';
        document.getElementById('cancelUniverseBtn').style.display = 'none';
    }
}

export async function generatePlanets() {
    const preset = document.getElementById('planetPreset').value;
    if (preset === 'prototype') {
        await generatePrototypePlanet();
        return;
    }
    if (!confirm('Сгенерировать планеты для всех миров?')) return;
    document.getElementById('planetResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('planetProgress').style.display = 'block';
    document.getElementById('planetProgressBar').value = 0;
    document.getElementById('planetProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelPlanetsBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-planets', { method: 'POST' });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('planetResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('planetProgress').style.display = 'none';
            document.getElementById('cancelPlanetsBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['planets']) clearInterval(pollIntervals['planets']);
        pollIntervals['planets'] = setInterval(() => pollJob('generate_planets', 'planetProgress', 'planetResult', 'cancelPlanetsBtn'), 1500);
    } catch (e) {
        document.getElementById('planetResult').textContent = '❌ ' + e.message;
        document.getElementById('planetProgress').style.display = 'none';
        document.getElementById('cancelPlanetsBtn').style.display = 'none';
    }
}

export async function generateFactions() {
    if (!confirm('Сгенерировать фракции для всех обитаемых планет?')) return;
    document.getElementById('factionResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('factionProgress').style.display = 'block';
    document.getElementById('factionProgressBar').value = 0;
    document.getElementById('factionProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelFactionsBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-factions', { method: 'POST' });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('factionResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('factionProgress').style.display = 'none';
            document.getElementById('cancelFactionsBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['factions']) clearInterval(pollIntervals['factions']);
        pollIntervals['factions'] = setInterval(() => pollJob('generate_factions', 'factionProgress', 'factionResult', 'cancelFactionsBtn'), 1500);
    } catch (e) {
        document.getElementById('factionResult').textContent = '❌ ' + e.message;
        document.getElementById('factionProgress').style.display = 'none';
        document.getElementById('cancelFactionsBtn').style.display = 'none';
    }
}

export async function generateResources() {
    if (!confirm('Сгенерировать ресурсы для всех планет?')) return;
    document.getElementById('resourceResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('resourceProgress').style.display = 'block';
    document.getElementById('resourceProgressBar').value = 0;
    document.getElementById('resourceProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelResourcesBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-resources', { method: 'POST' });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('resourceResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('resourceProgress').style.display = 'none';
            document.getElementById('cancelResourcesBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['resources']) clearInterval(pollIntervals['resources']);
        pollIntervals['resources'] = setInterval(() => pollJob('generate_resources', 'resourceProgress', 'resourceResult', 'cancelResourcesBtn'), 1500);
    } catch (e) {
        document.getElementById('resourceResult').textContent = '❌ ' + e.message;
        document.getElementById('resourceProgress').style.display = 'none';
        document.getElementById('cancelResourcesBtn').style.display = 'none';
    }
}

// generatePrototypePlanet — тестовая галактика: землеподобная планета
// с поселением 1 уровня (~10 человек) для первого мира.
async function generatePrototypePlanet() {
    if (!confirm('Сгенерировать землеподобную планету с поселением (прототип)?')) return;
    document.getElementById('planetResult').textContent = '⏳ Генерация...';
    try {
        const res = await fetchWithAuth('/admin/generate-prototype-planet', { method: 'POST' });
        const text = await res.text();
        if (!res.ok) {
            document.getElementById('planetResult').textContent = '❌ Ошибка: ' + text;
            return;
        }
        const data = JSON.parse(text);
        document.getElementById('planetResult').textContent =
            `✅ Мир «${data.world_name}» → планета «${data.planet_name}» с поселением (${data.population} чел.)`;
    } catch (e) {
        document.getElementById('planetResult').textContent = '❌ ' + e.message;
    }
}

// ---------- ГЕНЕРАЦИЯ ПОСЕЛЕНИЙ (модель) ----------

// settlementFields — реестр полей planet.data (грузится при инициализации).
// Формат элемента: {key, label, type, unit, min, max, values}.
let settlementFields = [];

// getSettlementFields — реестр полей planet.data для форм «тонкой настройки»
// (близнецы в гипотезах используют тот же реестр, что правила поселений).
export function getSettlementFields() {
    return settlementFields;
}

// loadSettlementFields — подгружает реестр полей с сервера для формы правил.
export async function loadSettlementFields() {
    try {
        const res = await fetchWithAuth('/admin/settlement-fields');
        // Пустой пароль (ещё не введён в админку) даёт 401 спокойно, на нём
        // не ругаемся — после ввода пароля функцию вызовут ещё раз.
        if (res.status === 401) return;
        if (!res.ok) {
            notifyError('Не удалось загрузить поля: HTTP ' + res.status);
            return;
        }
        settlementFields = await res.json();
        // Событие для вкладки «Гипотезы»: форма близнецов рисует оси/baked-поля
        // по реестру — до загрузки они пусты.
        document.dispatchEvent(new CustomEvent('settlement-fields-loaded'));
    } catch (e) {
        if (e.message === 'Unauthorized') return;
        notifyError('Не удалось загрузить поля: ' + e.message);
    }
}

// generateRaceSettlements — отдельный проход: поселения рас (спека 99.2.21 §7).
// Доминанта кластера + подселение соседней расы на выбросах; крутилка —
// шанс заселения соседней расы (0–100%). Шанс доминанты и население —
// из пресета config/race_settlement.json (65a).
export async function generateRaceSettlements() {
    if (!confirm('Сгенерировать поселения рас?')) return;

    const neighborChance = parseFloat(document.getElementById('raceNeighborChance').value) / 100;
    if (isNaN(neighborChance) || neighborChance < 0 || neighborChance > 1) {
        document.getElementById('raceSettlementResult').textContent = '❌ Шанс: 0–100%';
        return;
    }

    document.getElementById('raceSettlementResult').textContent = '⏳ Генерация запущена...';
    document.getElementById('raceSettlementProgress').style.display = 'block';
    document.getElementById('raceSettlementProgressBar').value = 0;
    document.getElementById('raceSettlementProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelRaceSettlementsBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/generate-race-settlements', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ neighbor_chance: neighborChance }),
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('raceSettlementResult').textContent = '❌ Ошибка: ' + text;
            document.getElementById('raceSettlementProgress').style.display = 'none';
            document.getElementById('cancelRaceSettlementsBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['race_settlements']) clearInterval(pollIntervals['race_settlements']);
        pollIntervals['race_settlements'] = setInterval(() => pollJob('generate_race_settlements', 'raceSettlementProgress', 'raceSettlementResult', 'cancelRaceSettlementsBtn'), 1500);
    } catch (e) {
        document.getElementById('raceSettlementResult').textContent = '❌ ' + e.message;
        document.getElementById('raceSettlementProgress').style.display = 'none';
        document.getElementById('cancelRaceSettlementsBtn').style.display = 'none';
    }
}

export async function cancelGeneration(jobType) {
    if (!confirm(`Остановить генерацию?`)) return;
    try {
        const res = await fetchWithAuth(`/admin/generate-cancel?job=${jobType}`, { method: 'POST' });
        if (res.ok) {
            notifyInfo('Остановка запрошена');
            const btnId = jobType === 'generate_universe' ? 'cancelUniverseBtn' :
                          jobType === 'generate_planets' ? 'cancelPlanetsBtn' :
                          jobType === 'generate_factions' ? 'cancelFactionsBtn' :
                          jobType === 'generate_race_settlements' ? 'cancelRaceSettlementsBtn' :
                          jobType === 'hypothesis' ? 'cancelHypothesisBtn' :
                          'cancelResourcesBtn';
            // Кнопки в секции есть не у всех джобов (pacman, regenerate,
            // generate_npc) — null-guard, иначе после успешной отмены
            // падаем на null.style (красная кнопка-стоп переиспользует
            // cancelGeneration для любого jobType).
            const btn = document.getElementById(btnId);
            if (btn) btn.style.display = 'none';
        } else {
            const text = await res.text();
            notifyError('Ошибка: ' + text);
        }
    } catch (e) {
        notifyError('Ошибка: ' + e.message);
    }
}

// stopRunningJob — красная кнопка-стоп у заголовка «Генерация» (идея
// 2026-09-20): отменяет первый найденный running-джоб (на практике джобы
// взаимоисключают друг друга серверно, 409). Тот же confirm и POST, что
// в cancelGeneration — без дублирования; кнопка скрывается следующим
// тиком pollJob (runningJobs очищается).
export async function stopRunningJob() {
    const jobType = runningJobs.values().next().value;
    if (!jobType) return;
    await cancelGeneration(jobType);
}

export async function clearSettlements() {
    if (!confirm('Удалить ВСЕ поселения? Планеты не пострадают.')) return;
    const box = document.getElementById('clearSettlementsResult');
    box.textContent = '⏳ Очистка...';
    try {
        const res = await fetchWithAuth('/admin/clear-settlements', { method: 'POST' });
        const text = await res.text();
        if (!res.ok) {
            box.textContent = '❌ Ошибка: ' + text;
            return;
        }
        const data = JSON.parse(text);
        box.textContent = '✅ Удалено поселений: ' + data.deleted;
    } catch (e) {
        box.textContent = '❌ ' + e.message;
    }
}

export async function clearUniverse() {
    if (!confirm('Удалить ВСЕ миры?')) return;
    try {
        const res = await fetchWithAuth('/admin/clear', { method: 'POST', headers: { 'Content-Type': 'application/json' } });
        if (res.ok) {
            document.getElementById('clearResult').textContent = '✅ Вселенная очищена';
            loadStats();
            loadWorlds(1);
        } else {
            const text = await res.text();
            document.getElementById('clearResult').textContent = '❌ Ошибка: ' + text;
        }
    } catch (e) {
        document.getElementById('clearResult').textContent = '❌ ' + e.message;
    }
}

// startPacman — запуск пакмана (спека 2026-09-20 §2.1): порционный вайп
// галактики по спирали; событие видно всем игрокам на карте. Скорость —
// из инпута (миров/с, дефолт 1700); прогресс — pollJob('pacman', ...).
export async function startPacman() {
    const speedInput = document.getElementById('pacmanSpeed');
    // Любая положительная скорость, дробная допустима (0.01 — «кинорежим»).
    const worldsPerSecond = parseFloat(speedInput ? speedInput.value : '1700') || 1700;
    try {
        const res = await fetchWithAuth('/admin/pacman/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ worlds_per_second: worldsPerSecond, batch_size: 200, trajectory: 'spiral' }),
        });
        const text = await res.text();
        const resultEl = document.getElementById('pacmanResult');
        if (res.status === 202) {
            resultEl.textContent = '👾 Пакман запущен!';
            document.getElementById('pacmanProgress').style.display = 'block';
            pollIntervals['pacman'] = setInterval(() => pollJob('pacman', 'pacmanProgress', 'pacmanResult', 'cancelPacmanBtn'), 1000);
        } else if (res.status === 200) {
            resultEl.textContent = 'Галактика уже пуста';
        } else {
            resultEl.textContent = '❌ ' + text;
        }
    } catch (e) {
        document.getElementById('pacmanResult').textContent = '❌ ' + e.message;
    }
}

// ---------- ПОДВКЛАДКИ ГЕНЕРАЦИИ (99.2.3 §2) ----------

// switchGenSubTab — переключение подвкладки раздела «Генерация» (кнопки
// вместо дропдауна, 71a). name — подвкладка ('stars' | 'planets' | ...);
// без аргумента — восстановление сохранённой из localStorage (выбор
// переживает перезагрузку страницы).
export function switchGenSubTab(name) {
    const target = name || localStorage.getItem('adminGenSubTab') || 'stars';
    document.querySelectorAll('#tab-generation .gen-sub').forEach(d => {
        d.style.display = d.id === 'genSub-' + target ? 'block' : 'none';
    });
    document.querySelectorAll('.gen-subtab-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.genSub === target);
    });
    if (target === 'stars') loadGenConfig();
    localStorage.setItem('adminGenSubTab', target);
}

// applyGenSubTab — применяет сохранённую подвкладку (вызывается из tabs.js
// при активации вкладки «Генерация»).
export function applyGenSubTab() {
    switchGenSubTab();
}

// showTab — переход на другую вкладку админки (кнопки-заглушки подвкладок).
export function showTab(tabId) {
    const btn = document.querySelector(`.tab-btn[data-tab="${tabId}"]`);
    if (btn) btn.click();
}

// ---------- КОНФИГ ГЕНЕРАЦИИ ЗВЁЗД (99.2.3 §4) ----------

// SPECTRAL_CLASSES — порядок спектральных классов O–Y для форм весов.
const SPECTRAL_CLASSES = ['O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y'];

// SYSTEM_TYPES — порядок типов систем/объектов для форм весов (99.2.4 §4.1).
const SYSTEM_TYPES = [
    ['single', 'Одиночная'], ['binary', 'Двойная'], ['multiple', 'Кратная (3+)'],
    ['black_hole', 'Чёрная дыра'], ['neutron', 'Нейтронная'],
    ['white_dwarf', 'Белый карлик'], ['protostar', 'Протозвезда'], ['exotic', 'Прочая экзотика'],
];

// PLANET_MEAN_KEYS — все редактируемые значения таблицы средних (99.2.3 §4.3).
const PLANET_MEAN_KEYS = [
    'O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y',
    'black_hole', 'neutron', 'white_dwarf', 'protostar', 'exotic',
    'binary_wide_factor', 'binary_close_mean', 'multiple_factor',
];

// MASS_KEYS — типы с диапазоном массы (29a §4м): спектральные классы + экзотика.
const MASS_KEYS = [
    'O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y',
    'black_hole', 'neutron', 'white_dwarf', 'protostar', 'exotic',
];

// mrId — id поля диапазона массы ('black_hole', 'min' → 'mrBlackHole_min').
function mrId(key, bound) {
    const camel = key.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
    return 'mr' + camel[0].toUpperCase() + camel.slice(1) + '_' + bound;
}

// stId — id поля веса типа системы ('black_hole' → 'stBlackHole').
// snake_case → camelCase как в mrId/pmId: иначе 'black_hole' ищет
// несуществующий 'stBlack_hole' (баг 39c: вес ЧД/белого карлика не
// заполнялся/не сохранялся).
function stId(key) {
    const camel = key.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
    return 'st' + camel[0].toUpperCase() + camel.slice(1);
}

// pmId — id поля среднего ('black_hole' → 'pmBlackHole', 'O' → 'pmO').
function pmId(key) {
    const camel = key.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
    return 'pm' + camel[0].toUpperCase() + camel.slice(1);
}

// recalcGenWeights — автоподсчёт процентов (вес/сумма×100) для обоих блоков
// весов: спектральные — от суммы обычных звёзд, типы — от суммы всех систем.
export function recalcGenWeights() {
    let specSum = 0;
    const specVals = {};
    SPECTRAL_CLASSES.forEach(cls => {
        const el = document.getElementById('sw' + cls);
        const v = el ? (parseFloat(el.value) || 0) : 0;
        specVals[cls] = v;
        specSum += v;
    });
    SPECTRAL_CLASSES.forEach(cls => {
        const pct = specSum > 0 ? (specVals[cls] / specSum * 100) : 0;
        const el = document.getElementById('sw' + cls + '_pct');
        if (el) el.textContent = pct.toFixed(1) + '%';
    });
    const specSumEl = document.getElementById('spectralWeightsSum');
    if (specSumEl) specSumEl.textContent = 'Сумма: ' + specSum.toFixed(1);

    let sysSum = 0;
    const sysVals = {};
    SYSTEM_TYPES.forEach(([key]) => {
        const el = document.getElementById(stId(key));
        const v = el ? (parseFloat(el.value) || 0) : 0;
        sysVals[key] = v;
        sysSum += v;
    });
    SYSTEM_TYPES.forEach(([key]) => {
        const pct = sysSum > 0 ? (sysVals[key] / sysSum * 100) : 0;
        const el = document.getElementById(stId(key) + '_pct');
        if (el) el.textContent = pct.toFixed(1) + '%';
    });
    const sysSumEl = document.getElementById('systemTypeWeightsSum');
    if (sysSumEl) sysSumEl.textContent = 'Сумма: ' + sysSum.toFixed(1);
}

// genConfigLoaded — защита от повторной загрузки конфига на каждый клик.
let genConfigLoaded = false;

// loadGenConfig — GET /admin/generation/config → заполняет формы весов и средних.
export async function loadGenConfig() {
    const box = document.getElementById('genConfigResult');
    try {
        const res = await fetchWithAuth('/admin/generation/config');
        if (!res.ok) {
            if (box) box.textContent = '❌ Не удалось загрузить конфиг: HTTP ' + res.status;
            return;
        }
        const cfg = await res.json();
        const spec = (cfg.star_weights && cfg.star_weights.spectral) || {};
        SPECTRAL_CLASSES.forEach(cls => {
            const el = document.getElementById('sw' + cls);
            if (el && spec[cls] != null) el.value = spec[cls];
        });
        const sys = (cfg.star_weights && cfg.star_weights.system_types) || {};
        SYSTEM_TYPES.forEach(([key]) => {
            const el = document.getElementById(stId(key));
            if (el && sys[key] != null) el.value = sys[key];
        });
        const pm = cfg.planet_means || {};
        PLANET_MEAN_KEYS.forEach(key => {
            const el = document.getElementById(pmId(key));
            if (el && pm[key] != null) el.value = pm[key];
        });
        // Диапазоны массы (29a §4м): {min, max} по типу.
        const mr = cfg.stellar_mass_ranges || {};
        MASS_KEYS.forEach(key => {
            const r = mr[key];
            if (!r) return;
            const elMin = document.getElementById(mrId(key, 'min'));
            const elMax = document.getElementById(mrId(key, 'max'));
            if (elMin && r.min != null) elMin.value = r.min;
            if (elMax && r.max != null) elMax.value = r.max;
        });
        // Подкрутка под расу-дома (99.2.22 §4.3): мягкость s (0–100%) и
        // множитель числа планет в кластерах рас (0.7–1.3).
        const softness = document.getElementById('raceTuningSoftness');
        if (softness && cfg.race_tuning_softness != null) {
            softness.value = Math.round(cfg.race_tuning_softness * 100);
            const rng = document.getElementById('raceTuningSoftnessRange');
            if (rng) rng.value = softness.value;
        }
        const countMult = document.getElementById('racePlanetCount');
        if (countMult && cfg.race_cluster_planet_count_mult != null) {
            countMult.value = cfg.race_cluster_planet_count_mult;
            const rng = document.getElementById('racePlanetCountRange');
            if (rng) rng.value = countMult.value;
        }
        recalcGenWeights();
        genConfigLoaded = true;
        if (box) box.textContent = '✅ Конфиг загружен (дефолты или сохранённый)';
    } catch (e) {
        if (box) box.textContent = '❌ ' + e.message;
    }
}

// saveGenConfig — PUT /admin/generation/config с валидацией на сервере
// (веса: 409 при нулевой сумме; mean: 422 при > 8 — не клампится, 99.2.3 §4.5).
export async function saveGenConfig() {
    const box = document.getElementById('genConfigResult');
    const spectral = {};
    SPECTRAL_CLASSES.forEach(cls => {
        const el = document.getElementById('sw' + cls);
        spectral[cls] = el ? (parseFloat(el.value) || 0) : 0;
    });
    const system_types = {};
    SYSTEM_TYPES.forEach(([key]) => {
        const el = document.getElementById(stId(key));
        system_types[key] = el ? (parseFloat(el.value) || 0) : 0;
    });
    const planet_means = {};
    PLANET_MEAN_KEYS.forEach(key => {
        const el = document.getElementById(pmId(key));
        planet_means[key] = el ? (parseFloat(el.value) || 0) : 0;
    });
    // Диапазоны массы (29a §4м): {min, max} по типу.
    const stellar_mass_ranges = {};
    MASS_KEYS.forEach(key => {
        const elMin = document.getElementById(mrId(key, 'min'));
        const elMax = document.getElementById(mrId(key, 'max'));
        if (elMin && elMax) {
            const minV = parseFloat(elMin.value);
            const maxV = parseFloat(elMax.value);
            if (!isNaN(minV) && !isNaN(maxV)) {
                stellar_mass_ranges[key] = { min: minV, max: maxV };
            }
        }
    });
    // Подкрутка под расу-дома (99.2.22 §4.3): мягкость s (0–100%) и
    // множитель числа планет в кластерах рас (0.7–1.3).
    const softnessEl = document.getElementById('raceTuningSoftness');
    const race_tuning_softness = softnessEl ? (parseFloat(softnessEl.value) || 0) / 100 : 0.5;
    const countEl = document.getElementById('racePlanetCount');
    const race_cluster_planet_count_mult = countEl ? (parseFloat(countEl.value) || 1.1) : 1.1;

    const body = JSON.stringify({ star_weights: { spectral, system_types }, planet_means, stellar_mass_ranges, race_tuning_softness, race_cluster_planet_count_mult });
    try {
        const res = await fetchWithAuth('/admin/generation/config', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body,
        });
        if (!res.ok) {
            const text = await res.text();
            if (box) box.textContent = '❌ ' + (text || ('HTTP ' + res.status));
            return;
        }
        genConfigLoaded = true;
        if (box) box.textContent = '✅ Конфиг сохранён (применится при следующей генерации)';
    } catch (e) {
        if (box) box.textContent = '❌ ' + e.message;
    }
}

// ---------- ПЕРЕСЧЁТ ПЛАНЕТ (99.2.3 §5) ----------

// regeneratePlanets — POST /admin/regenerate-planets: ручной пересчёт планет
// выбранных миров по равномерному счёту (0–8), без весов и средних.
export async function regeneratePlanets() {
    const minP = parseInt(document.getElementById('regMinPlanets').value);
    const maxP = parseInt(document.getElementById('regMaxPlanets').value);
    if (isNaN(minP) || isNaN(maxP) || minP < 0 || maxP > 8 || minP > maxP) {
        document.getElementById('regenerateResult').textContent = '❌ Мин/макс планет: 0 ≤ мин ≤ макс ≤ 8';
        return;
    }
    const body = {
        min_planets: minP,
        max_planets: maxP,
        include_normal: document.getElementById('regIncludeNormal').checked,
        include_binary: document.getElementById('regIncludeBinary').checked,
        include_exotic: document.getElementById('regIncludeExotic').checked,
    };
    if (!body.include_normal && !body.include_binary && !body.include_exotic) {
        document.getElementById('regenerateResult').textContent = '❌ Выберите хотя бы одну категорию миров';
        return;
    }
    if (!confirm('Пересчитать планеты? Планеты выбранных миров будут перезаписаны.')) return;

    document.getElementById('regenerateResult').textContent = '⏳ Пересчёт запущен...';
    document.getElementById('regenerateProgress').style.display = 'block';
    document.getElementById('regenerateProgressBar').value = 0;
    document.getElementById('regenerateProgressText').textContent = 'Подготовка...';
    document.getElementById('cancelRegenerateBtn').style.display = 'inline-block';

    try {
        const res = await fetchWithAuth('/admin/regenerate-planets', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const text = await res.text();
            document.getElementById('regenerateResult').textContent = '❌ ' + text;
            document.getElementById('regenerateProgress').style.display = 'none';
            document.getElementById('cancelRegenerateBtn').style.display = 'none';
            return;
        }
        if (pollIntervals['regenerate']) clearInterval(pollIntervals['regenerate']);
        pollIntervals['regenerate'] = setInterval(() => pollJob('regenerate_planets', 'regenerateProgress', 'regenerateResult', 'cancelRegenerateBtn'), 1500);
    } catch (e) {
        document.getElementById('regenerateResult').textContent = '❌ ' + e.message;
        document.getElementById('regenerateProgress').style.display = 'none';
        document.getElementById('cancelRegenerateBtn').style.display = 'none';
    }
}
