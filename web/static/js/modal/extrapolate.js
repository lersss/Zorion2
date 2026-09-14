// web/static/js/modal/extrapolate.js
// Косметическая тень населения между синками с сервером (модель «правда на
// сервере, синк по событию»): пока игрок смотрит, число живёт на экране по
// локальному счёту от последней чек-точки, а не дожидается запроса.
// Зеркалит формулу изменения населения из internal/economy/settlement
// (mortality.go, recompute.go): N(t) = p0·(1−r)^Δt_сек · exp(−λ·Δt/час),
// где r — рекурсивная компонента (жара, r_per_sec), λ — прочие факторы
// (lambda_per_hour); ниже n_dead и ниже 1 — ноль (99.2.12).
// Это только отображение: при каждом синке (refreshPlanets) перерисовываемся
// от серверной правды (population/computed_at в ответе).

// populationAt — население поселения на момент nowMs (мс эпохи) по его
// чек-точке. Лямбды/чек-точки нет — возвращаем синхронизированное значение.
export function populationAt(s, nowMs) {
    if (!s) return 0;
    const exact = (typeof s.population_exact === 'number' && isFinite(s.population_exact))
        ? s.population_exact : s.population;
    const lambda = (typeof s.lambda_per_hour === 'number' && isFinite(s.lambda_per_hour)) ? s.lambda_per_hour : 0;
    const r = (typeof s.r_per_sec === 'number' && isFinite(s.r_per_sec) && s.r_per_sec > 0) ? s.r_per_sec : 0;
    const t0 = s.computed_at ? Date.parse(s.computed_at) : NaN;
    if (isNaN(t0) || (lambda <= 0 && r <= 0)) return s.population || 0;
    const dtSec = Math.max(0, nowMs - t0) / 1000;
    let next = exact * Math.pow(1 - r, dtSec) * Math.exp(-lambda * dtSec / 3600);
    const nDead = (typeof s.n_dead === 'number' && s.n_dead > 0) ? s.n_dead : 100;
    if (next < 1) return 0; // порог «p < 1 → поселение мёртвое» (99.2.12)
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

// canExtrapolate — есть ли данные для живой тени (чек-точка + лямбда или R).
export function canExtrapolate(planet) {
    const list = planet && Array.isArray(planet.settlements) ? planet.settlements : [];
    return list.some(s => s && (
        (typeof s.lambda_per_hour === 'number' && s.lambda_per_hour > 0) ||
        (typeof s.r_per_sec === 'number' && s.r_per_sec > 0)
    ));
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