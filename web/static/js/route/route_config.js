// web/static/js/route/route_config.js
// Константы, пулы спрайтов и словари страницы мини-игры «Прокладка маршрута»
// (спека 2026-09-25-маршрут-мини-игра-интерфейс.md; осн. спека §6). Числовые
// константы поля — серверный контракт (internal/routegame/field.go,
// evaluate.go): расхождение даёт ложные отказы «Проложить».

export const SPRITE_BASE = '/static/sprites/route/';

// Пулы вариантов (визуальный раздел спеки §3.7): вариант выбирается случайно на
// запуск (режим наигрыша); заморозка — одно имя на тип в SPRITE_FREEZE.
export const POOLS = {
    star_core: ['star_core_01', 'star_core_02', 'star_core_03', 'star_core_04', 'star_core_05'],
    black_hole: ['black_hole_01', 'black_hole_02', 'black_hole_03'],
    beacon: ['beacon_01', 'beacon_02', 'beacon_03', 'beacon_04'],
    false_signal: ['false_signal_01', 'false_signal_02', 'false_signal_03', 'false_signal_04'],
    hazard_cloud: ['hazard_cloud_01', 'hazard_cloud_02', 'hazard_cloud_03'],
    nebula_bg: ['nebula_bg_01', 'nebula_bg_02', 'nebula_bg_03'],
};

// Заморозка после выбора создателя: type → имя файла пула. null = наигрыш
// (случайный вариант на запуск); иначе — фиксированный вариант (§3.7).
export const SPRITE_FREEZE = {
    star_core: null, black_hole: null, beacon: null,
    false_signal: null, hazard_cloud: null, nebula_bg: null,
};

// Константы серверного поля (§6.6 осн. спеки, сверено по коду).
export const ENDPOINT_R = 0.06; // endpointCaptureRadius — захват СТАРТА/ФИНИША
export const MAX_POINTS = 64;   // maxPathPoints — потолок точек полилинии
export const REF_OVERSHOOT = 1.05; // refOvershoot — для грубой оценки пути

// Палитра: базовые константы проекта + атмосферные тона (§3.1). Новые тона —
// только фон/туманность/свечение; цвета интерактива не подменяются.
export const COLORS = {
    bg: '#05070f',
    border: '#334155',
    grid: 'rgba(51, 65, 85, 0.4)',
    path: '#60a5fa',
    pathFree: 'rgba(96, 165, 250, 0.35)',
    capture: '#fde047',
    captureSoft: '#fde68a',
    success: '#4ade80',
    warning: '#f97316',
    danger: '#ef4444',
    cold: '#38bdf8',
    voidHalo: '#3b0764',
    warmHalo: '#fff7ed',
    nebula: ['#6366f1', '#c026d3', '#22d3ee', '#e879a9', '#8b5cf6'],
};

// ---- Словари HUD (§1/§2) ----

// Дальность — качественно (без чисел и ETA, решение 14). Границы — UI-деление
// (О3), приблизительные, не механика.
export function distanceWord(dist) {
    const d = Number(dist) || 0;
    if (d < 8000) return 'близкий';
    if (d < 20000) return 'средний';
    return 'дальний';
}

const STAR_TYPE_LABELS = {
    star: 'звезда', white_dwarf: 'белый карлик', neutron: 'нейтронная',
    black_hole: 'чёрная дыра', protostar: 'протозвезда',
};
const SYSTEM_LABELS = { single: 'одиночная', binary: 'двойная', multiple: 'кратная' };
const BELT_LABELS = {
    asteroid: 'пояс астероидов', kuiper: 'пояс Койпера', oort: 'облако Оорта',
    debris: 'обломочный пояс', dust_ring: 'пылевое кольцо',
};

// starClassWord — метка конца: спектральный класс, для экзотики — тип звезды.
export function starClassWord(star) {
    if (!star) return '—';
    if (star.spectral_class) return star.spectral_class;
    return STAR_TYPE_LABELS[star.star_type] || 'экзотика';
}

export function systemWord(star) { return SYSTEM_LABELS[star && star.system_type] || '—'; }

export function beltWord(b) { return !b ? '' : (b.name || BELT_LABELS[b.kind] || 'пояс'); }

// kToC — температура из K в °C (в UI только °C, решение 2026-09-18).
export function kToC(k) {
    return Math.round((Number(k) || 0) - 273.15);
}

// tempWord — °C со знаком (K в UI не показываем).
export function tempWord(k) {
    const c = kToC(k);
    return (c >= 0 ? '+' : '') + c + ' °C';
}

// Класс качества (UI-слова, пороги §3.6 осн. спеки) — только после отправки.
export function qualityClassWord(q) {
    const v = Number(q) || 0;
    if (v >= 0.9) return 'Идеальный маршрут';
    if (v >= 0.7) return 'Отличный маршрут';
    if (v >= 0.5) return 'Хороший маршрут';
    return 'Маршрут проложен';
}

// Грубый индикатор длины СЛОВАМИ, без чисел (решение 14).
export function pathHintWord(ratio) {
    if (ratio < 0.9) return 'короче эталона';
    if (ratio <= 1.1) return 'примерно как эталон';
    return 'длиннее эталона';
}

// Причины недоступности offer (409, §2) — оверлей-причина.
export const REASON_OVERLAY = {
    no_flight: { title: 'Сейчас нет активного перелёта', note: 'Откройте карту и начните перелёт' },
    no_module: { title: 'Ускоритель не установлен', note: 'Установите модуль ускорителя на корабль' },
    unknown_game: { title: 'Мини-игра недоступна', note: 'Этот ускоритель пока не поддерживает игру' },
    already_active: { title: 'Ускорение уже действует', note: 'Смена цели или прибытие сбросит его — тогда ускориться можно снова' },
    too_short: { title: 'Перелёт слишком короткий', note: 'Осталось мало пути для партии и ускорения' },
    cooldown: { title: 'Ускорение перезаряжается', note: 'Можно не ждать — вернитесь на карту' },
};

// Причины отказа boost (400/409) — тост; путь сохраняем (§2). Сырой текст ответа
// в тост не подставляем: только человеческий текст по reason.
export const REASON_TOAST = {
    too_few_points: 'Слишком короткий путь',
    too_many_points: 'Слишком много точек пути',
    point_out_of_bounds: 'Точка вне поля',
    start_not_captured: 'Путь не начинается от СТАРТА',
    finish_not_captured: 'Путь не доходит до ФИНИШа',
    beacon_not_captured: 'Не все маяки на пути',
    changed: 'Маршрут изменился — вернитесь на карту',
    too_short: 'Перелёт слишком короткий',
    already_active: 'Ускорение уже действует',
    no_flight: 'Полёт уже завершён',
    no_module: 'Ускоритель не установлен',
    unknown_game: 'Этот ускоритель пока не поддерживает игру',
    cooldown: 'Ускорение перезаряжается',
};

// ---- Геометрия (общая для рендера, ввода и грубой оценки) ----

// computeView — квадрат 1:1 (letterbox) в свободной области между HUD.
export function computeView(vw, vh) {
    const avail = Math.max(120, vh - 150 - 190);
    const size = Math.max(120, Math.min(vw - 24, avail));
    return { x0: (vw - size) / 2, y0: 150 + Math.max(0, (avail - size) / 2), size };
}

// hexA — '#rrggbb' → 'rgba(r,g,b,a)'.
export function hexA(hex, a) {
    const h = String(hex || '#ffffff').replace('#', '');
    const full = h.length === 3 ? h[0] + h[0] + h[1] + h[1] + h[2] + h[2] : h;
    const n = parseInt(full, 16);
    return 'rgba(' + ((n >> 16) & 255) + ',' + ((n >> 8) & 255) + ',' + (n & 255) + ',' + a + ')';
}

// mulberry32 — детерминированный RNG фона от seed поля.
export function mulberry32(seed) {
    let s = (seed >>> 0) || 1;
    return function () {
        s = (s + 0x6d2b79f5) | 0;
        let t = Math.imul(s ^ (s >>> 15), 1 | s);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

export function dist(a, b) {
    return Math.hypot(a.x - b.x, a.y - b.y);
}

// pointSegDist — расстояние от точки до отрезка [a,b].
export function pointSegDist(p, a, b) {
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const len2 = dx * dx + dy * dy;
    if (len2 === 0) return dist(p, a);
    let t = ((p.x - a.x) * dx + (p.y - a.y) * dy) / len2;
    t = Math.max(0, Math.min(1, t));
    return dist(p, { x: a.x + t * dx, y: a.y + t * dy });
}

// pathEffLen — эффективная длина: сегмент в зоне умножается на её коэффициент
// (серверная формула §6.3; сам коэффициент в UI не выводим).
export function pathEffLen(path, zones) {
    let total = 0;
    for (let i = 0; i + 1 < path.length; i++) {
        const seg = dist(path[i], path[i + 1]);
        let mult = 1;
        for (const z of zones || []) {
            if (pointSegDist({ x: z.x, y: z.y }, path[i], path[i + 1]) <= z.r) mult *= z.coefficient;
        }
        total += seg * mult;
    }
    return total;
}

// greedyRouteLen — приблизительный эталон (эвристика «ближайший сосед»; сервер
// при ≤8 маяках делает полный перебор, §6.3) × REF_OVERSHOOT. Точность не
// требуется — только слова.
export function greedyRouteLen(field) {
    const beacons = (field.nodes || []).filter((n) => n.type === 'beacon');
    const route = [{ x: field.start.x, y: field.start.y }];
    let cur = route[0];
    const used = new Set();
    while (used.size < beacons.length) {
        let best = -1;
        let bestD = Infinity;
        beacons.forEach((n, i) => {
            if (used.has(i)) return;
            const d = dist(cur, n);
            if (d < bestD) { bestD = d; best = i; }
        });
        used.add(best);
        route.push({ x: beacons[best].x, y: beacons[best].y });
        cur = route[route.length - 1];
    }
    route.push({ x: field.finish.x, y: field.finish.y });
    return REF_OVERSHOOT * pathEffLen(route, field.zones);
}

// ---- Гейт «Проложить» и статус-строка (чистые функции от state) ----

export function beaconsTotal(field) {
    return (field.nodes || []).filter((n) => n.type === 'beacon').length;
}

export function remainingNow(state, now) {
    return state.remainingS - (now - state.remainingAt) / 1000;
}

function startCaptured(state) {
    return state.path.length > 0 && dist(state.path[0], state.field.start) <= ENDPOINT_R;
}

function finishCaptured(state) {
    const p = state.path[state.path.length - 1];
    return state.path.length > 1 && dist(p, state.field.finish) <= ENDPOINT_R;
}

export function missingBeacons(state) {
    return beaconsTotal(state.field) - state.fixedCaptured.size;
}

// gateReady — «Проложить» активна только при СТАРТЕ+ФИНИШЕ+всех маяках+остатке.
export function gateReady(state, now) {
    return !state.submitting && !state.finished && startCaptured(state) && finishCaptured(state)
        && missingBeacons(state) <= 0 && remainingNow(state, now) >= state.minBoostS;
}

// statusText — первая невыполненная причина (§3.3): нет СТАРТА → маяки →
// ФИНИШ → готово; при исчерпанном остатке — своя строка.
export function statusText(state, now) {
    if (!state.path.length) return 'Ведите путь от СТАРТА к звёзде-ФИНИШУ';
    if (missingBeacons(state) > 0) return 'Не хватает ' + missingBeacons(state) + ' маяков';
    if (!finishCaptured(state)) return 'Путь не доходит до ФИНИШа';
    if (remainingNow(state, now) < state.minBoostS) return 'Перелёт уже завершается — ускорить не получится';
    return 'Путь готов';
}

// pathHint — грубый индикатор длины СЛОВАМИ (без чисел, решение 14).
export function pathHint(state) {
    if (state.path.length < 2) return '';
    const lRef = greedyRouteLen(state.field);
    if (lRef <= 0) return '';
    return pathHintWord(pathEffLen(state.path, state.field.zones) / lRef);
}

// remainWord — остаток в читаемом виде (только после отправки, решение 14).
export function remainWord(s) {
    const v = Number(s) || 0;
    if (v <= 0) return '0 с';
    return v < 60 ? Math.round(v) + ' с' : Math.round(v / 60) + ' мин';
}

// ---- Захват узлов/зон полилинией ----

// nearestSeg — минимальное расстояние точки до полилинии.
export function nearestSeg(p, pts) {
    let best = Infinity;
    for (let i = 0; i + 1 < pts.length; i++) best = Math.min(best, pointSegDist(p, pts[i], pts[i + 1]));
    return best;
}

// nodeSet — индексы узлов типа type, захваченных полилинией (расстояние ≤ r).
export function nodeSet(field, pts, type) {
    const set = new Set();
    if (pts.length < 2) return set;
    (field.nodes || []).forEach((n, i) => {
        if (n.type === type && nearestSeg(n, pts) <= n.r) set.add(i);
    });
    return set;
}

// zoneSet — индексы зон, пересечённых полилинией (для подсветки/тона зоны).
export function zoneSet(field, pts) {
    const set = new Set();
    if (pts.length < 2) return set;
    (field.zones || []).forEach((z, i) => {
        for (let j = 0; j + 1 < pts.length; j++) {
            if (pointSegDist({ x: z.x, y: z.y }, pts[j], pts[j + 1]) <= z.r) { set.add(i); return; }
        }
    });
    return set;
}
