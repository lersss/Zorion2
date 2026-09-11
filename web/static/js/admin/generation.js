// web/static/js/admin/generation.js
import { fetchWithAuth } from './auth.js';
import { loadStats } from './stats.js';
import { loadWorlds } from './worlds.js';
import { pollJob, pollIntervals } from './poll.js';
import { notifyError, notifyInfo } from '../ui/toast.js';

// Пресеты генерации вселенной (проверены: 100k миров, 200 кластеров).
const UNIVERSE_PRESETS = {
    dense: {   // плотная галактика
        worlds: 100000, clusters: 200, mapSize: 40000, minDist: 120,
        clusterRadius: 2000, clusterSpacing: 4000, outlierPercent: 20
    },
    sparse: {  // просторная: большая галактика, миры дальше друг от друга
        worlds: 100000, clusters: 200, mapSize: 50000, minDist: 150,
        clusterRadius: 2000, clusterSpacing: 4000, outlierPercent: 20
    },
    compact: { // тесная: компактная галактика, миры близко, больше выбросов
        worlds: 100000, clusters: 200, mapSize: 36000, minDist: 100,
        clusterRadius: 1600, clusterSpacing: 3200, outlierPercent: 25
    },
    prototype: { // тестовая галактика: один мир для прототипа поселения
        worlds: 1, clusters: 1, mapSize: 800, minDist: 100,
        clusterRadius: 80, clusterSpacing: 160, outlierPercent: 0
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
}

export async function generateUniverse() {
    const worlds = parseInt(document.getElementById('genWorlds').value);
    const clusters = parseInt(document.getElementById('genClusters').value);
    const mapSize = parseInt(document.getElementById('genMapSize').value);
    const minDist = parseInt(document.getElementById('genMinDist').value);
    const clusterRadius = parseInt(document.getElementById('genClusterRadius').value);
    const clusterSpacing = parseInt(document.getElementById('genClusterSpacing').value);
    const outlierPercent = parseInt(document.getElementById('genOutlierPercent').value);
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
                outlier_percent: outlierPercent
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

export async function cancelGeneration(jobType) {
    if (!confirm(`Остановить генерацию?`)) return;
    try {
        const res = await fetchWithAuth(`/admin/generate-cancel?job=${jobType}`, { method: 'POST' });
        if (res.ok) {
            notifyInfo('Остановка запрошена');
            const btnId = jobType === 'generate_universe' ? 'cancelUniverseBtn' :
                          jobType === 'generate_planets' ? 'cancelPlanetsBtn' :
                          jobType === 'generate_factions' ? 'cancelFactionsBtn' :
                          'cancelResourcesBtn';
            document.getElementById(btnId).style.display = 'none';
        } else {
            const text = await res.text();
            notifyError('Ошибка: ' + text);
        }
    } catch (e) {
        notifyError('Ошибка: ' + e.message);
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