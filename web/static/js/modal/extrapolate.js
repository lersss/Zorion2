// web/static/js/modal/extrapolate.js
// Косметическая тень населения между синками с сервером (модель «правда на
// сервере, синк по событию»): пока игрок смотрит, число живёт на экране по
// локальному счёту от последней чек-точки, а не дожидается запроса.
// Зеркалит формулу смерти от среды из internal/economy/settlement
// (mortality.go, recompute.go): N(t) = p0·exp(−λ·Δt/час), ниже n_dead — ноль.
// Это только отображение: при каждом синке (refreshPlanets) перерисовываемся
// от серверной правды (population/computed_at в ответе).

// populationAt — население поселения на момент nowMs (мс эпохи) по его
// чек-точке. Лямбды/чек-точки нет — возвращаем синхронизированное значение.
export function populationAt(s, nowMs) {
    if (!s) return 0;
    const exact = (typeof s.population_exact === 'number' && isFinite(s.population_exact))
        ? s.population_exact : s.population;
    const lambda = (typeof s.decay_lambda === 'number' && isFinite(s.decay_lambda)) ? s.decay_lambda : 0;
    const t0 = s.computed_at ? Date.parse(s.computed_at) : NaN;
    if (isNaN(t0) || lambda <= 0) return s.population || 0;
    const next = exact * Math.exp(-lambda * Math.max(0, nowMs - t0) / 3_600_000);
    const nDead = (typeof s.n_dead === 'number' && s.n_dead > 0) ? s.n_dead : 100;
    if (next < nDead) return 0;
    return Math.round(next);
}

// planetPopulationAt — население планеты = сумма по поселениям (зеркало
// attachSettlements: planets[].population = SUM settlements.population).
export function planetPopulationAt(planet, nowMs) {
    const list = planet && Array.isArray(planet.settlements) ? planet.settlements : null;
    if (!list) return planet && typeof planet.population === 'number' ? planet.population : 0;
    return list.reduce((sum, s) => sum + populationAt(s, nowMs), 0);
}

// canExtrapolate — есть ли данные для живой тени (чек-точка + лямбда).
export function canExtrapolate(planet) {
    const list = planet && Array.isArray(planet.settlements) ? planet.settlements : [];
    return list.some(s => s && typeof s.decay_lambda === 'number' && s.decay_lambda > 0);
}

// repaintPopulationNumbers — обновляет тексты населения на экране по текущему
// времени, не пересоздавая DOM. Вызывается из rAF-цикла модалки (index.js)
// примерно раз в секунду: в фоне rAF замирает, поэтому первый кадр после
// возврата во вкладку сразу даёт свежее число без запроса к серверу.
export function repaintPopulationNumbers(planet) {
    if (!canExtrapolate(planet)) return;
    const nowMs = Date.now();
    const planetNum = planetPopulationAt(planet, nowMs);

    const header = document.getElementById('planet-pop-num');
    if (header) header.textContent = formatPopulation(planetNum);

    document.querySelectorAll('[data-pop-planet]').forEach(el => {
        el.textContent = formatPopulation(planetNum);
    });

    const list = Array.isArray(planet.settlements) ? planet.settlements : [];
    list.forEach(s => {
        if (!s) return;
        const el = document.getElementById('pop-' + s.id);
        if (el) el.textContent = formatPopulation(populationAt(s, nowMs));
    });
}

function formatPopulation(n) {
    return n.toLocaleString('ru-RU');
}