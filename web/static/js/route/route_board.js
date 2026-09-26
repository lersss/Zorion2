// web/static/js/route/route_board.js
// Чистые функции доски v9 «Планшет» мини-игры «Прокладка маршрута» (спека
// 2026-09-25-маршрут-мини-игра-интерфейс.md §7.6): координаты клетка↔экран,
// 4-связность, достройка «лестницы» между waypoints, локальные метки курса.
// Ни DOM, ни сети, ни времени — только данные доски (§8) и путь-цепочка клеток.
//
// Соглашения модели (internal/routegame/grid_model.go): i = cell % n,
// j = cell / n; направление dir: 0:+i, 1:−i, 2:+j, 3:−j.

export function iOf(n, cell) { return cell % n; }
export function jOf(n, cell) { return (cell / n) | 0; }
export function cellOf(n, i, j) { return j * n + i; }

// Шаг по цепочке клеток: 4-связное приращение индекса. +1 = +i, −1 = −i,
// +n = +j, −n = −j. Работает без n (выводится из знака/модуля разности).
function stepDir(a, b) {
    const d = b - a;
    if (d === 1) return 0;
    if (d === -1) return 1;
    if (d > 0) return 2;
    if (d < 0) return 3;
    return -1;
}

const DIR_DI = [1, -1, 0, 0];
const DIR_DJ = [0, 0, 1, -1];

export function dirOf(n, a, b) {
    const di = iOf(n, b) - iOf(n, a);
    const dj = jOf(n, b) - jOf(n, a);
    for (let d = 0; d < 4; d++) if (DIR_DI[d] === di && DIR_DJ[d] === dj) return d;
    return -1;
}

export function neighbors4(n, cell) {
    const i = iOf(n, cell);
    const j = jOf(n, cell);
    const out = [];
    if (i > 0) out.push(cellOf(n, i - 1, j));
    if (i < n - 1) out.push(cellOf(n, i + 1, j));
    if (j > 0) out.push(cellOf(n, i, j - 1));
    if (j < n - 1) out.push(cellOf(n, i, j + 1));
    return out;
}

export function adjacent4(n, a, b) {
    return Math.abs(iOf(n, a) - iOf(n, b)) + Math.abs(jOf(n, a) - jOf(n, b)) === 1;
}

// connectStair — 4-связная достройка от from (исключая) до to (включая).
// «Лестница» с минимумом манёвров: пока движемся по оси текущего направления
// (heading), поворот ставится как можно позже (решение создателя 2026-09-26).
// heading = dir последнего шага пути или −1 (тогда ось по большей дельте).
export function connectStair(n, from, to, heading) {
    const out = [];
    if (from === to) return out;
    let i = iOf(n, from);
    let j = jOf(n, from);
    const bi = iOf(n, to);
    const bj = jOf(n, to);
    let prefer = null; // true — сначала ось i, false — ось j
    if (heading === 0 || heading === 1) prefer = true;
    else if (heading === 2 || heading === 3) prefer = false;
    while (i !== bi || j !== bj) {
        const di = bi - i;
        const dj = bj - j;
        let moveI;
        if (di === 0) moveI = false;
        else if (dj === 0) moveI = true;
        else if (prefer === true) moveI = true;
        else if (prefer === false) moveI = false;
        else moveI = Math.abs(di) >= Math.abs(dj);
        if (moveI) i += di > 0 ? 1 : -1;
        else j += dj > 0 ? 1 : -1;
        out.push(cellOf(n, i, j));
    }
    return out;
}

// rebuildPath — полная цепочка из waypoints: старт + connectStair между
// соседними waypoints. Заголовок каждого сегмента — текущее направление цепочки.
export function rebuildPath(board, waypoints) {
    const n = board.n;
    const wp = waypoints || [];
    const chain = [];
    if (wp.length) chain.push(wp[0]);
    for (let k = 1; k < wp.length; k++) {
        const from = chain[chain.length - 1];
        let heading = -1;
        if (chain.length >= 2) heading = dirOf(n, chain[chain.length - 2], from);
        const steps = connectStair(n, from, wp[k], heading);
        for (const c of steps) chain.push(c);
    }
    return chain;
}

// cellAtPoint — экранная точка → клетка; за квадратом доски → −1 (клип).
export function cellAtPoint(view, x, y) {
    const cell = view.size / view.n;
    const i = Math.floor((x - view.x0) / cell);
    const j = Math.floor((y - view.y0) / cell);
    if (i < 0 || j < 0 || i >= view.n || j >= view.n) return -1;
    return cellOf(view.n, i, j);
}

export function cellCenter(view, cell) {
    const cellSize = view.size / view.n;
    return {
        x: view.x0 + (iOf(view.n, cell) + 0.5) * cellSize,
        y: view.y0 + (jOf(view.n, cell) + 0.5) * cellSize,
    };
}

export function turnCount(path) {
    let turns = 0;
    for (let k = 2; k < path.length; k++) {
        if (stepDir(path[k - 2], path[k - 1]) !== stepDir(path[k - 1], path[k])) turns++;
    }
    return turns;
}

// beaconsOnPath — маяки, чьи клетки лежат на пути (захват точный, §1).
export function beaconsOnPath(board, path) {
    const onPath = new Set(path);
    const out = [];
    for (const b of board.beacons || []) if (onPath.has(b)) out.push(b);
    return out;
}

// stepHeat — локальные метки курса шага (§4.6): точечные метки на клетках пути.
// kind: mud/wall/gate/gate_twice/bridge/bridge_twice/current_against/
// current_along/dead_end/turn/hazard/jackpot. Положительные метки — current_along
// и jackpot (jackpot/lure); hazard — вскрытый unstable-сектор.
export function stepHeat(board, path, revealed) {
    const out = [];
    if (!board || !path || path.length < 2) return out;
    const wall = new Set(board.wall || []);
    const mud = new Set(board.mud || []);
    const gate = new Set(board.gate || []);
    const bridge = new Set(board.bridge || []);
    const dead = new Set(board.dead_end || []);
    const current = new Map();
    for (const c of board.current || []) current.set(c.cell, c.dir);
    const rx = new Map();
    for (const r of revealed || []) rx.set(r.sector, r.content);
    const contentOf = new Map();
    (board.sectors || []).forEach((s, idx) => {
        const content = rx.get(idx);
        if (!content) return;
        for (const c of s.cells || []) contentOf.set(c, content);
    });
    const gateVisits = new Map();
    const bridgeVisits = new Map();
    for (let k = 1; k < path.length; k++) {
        const cell = path[k];
        const dir = stepDir(path[k - 1], cell);
        if (wall.has(cell)) out.push({ cell, kind: 'wall' });
        else if (mud.has(cell)) out.push({ cell, kind: 'mud' });
        if (current.has(cell)) {
            out.push({ cell, kind: dir === current.get(cell) ? 'current_along' : 'current_against' });
        }
        if (gate.has(cell)) {
            const v = (gateVisits.get(cell) || 0) + 1;
            gateVisits.set(cell, v);
            out.push({ cell, kind: v > 1 ? 'gate_twice' : 'gate' });
        }
        if (bridge.has(cell)) {
            const v = (bridgeVisits.get(cell) || 0) + 1;
            bridgeVisits.set(cell, v);
            out.push({ cell, kind: v > 1 ? 'bridge_twice' : 'bridge' });
        }
        if (dead.has(cell)) out.push({ cell, kind: 'dead_end' });
        const content = contentOf.get(cell);
        if (content === 'unstable') out.push({ cell, kind: 'hazard' });
        else if (content === 'jackpot' || content === 'lure') out.push({ cell, kind: 'jackpot' });
    }
    for (let k = 2; k < path.length; k++) {
        if (stepDir(path[k - 2], path[k - 1]) !== stepDir(path[k - 1], path[k])) {
            out.push({ cell: path[k - 1], kind: 'turn' });
        }
    }
    return out;
}
