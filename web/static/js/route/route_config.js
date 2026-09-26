// web/static/js/route/route_config.js
// Словари, палитра и константы страницы мини-игры «Прокладка маршрута» на доске
// v9 «Планшет» (спека 2026-09-25-маршрут-мини-игра-интерфейс.md §7.5/§8).
// Числа механики (цены, τ, пороги) — серверный контракт: клиент их не считает и
// в UI не показывает. Здесь только тексты UI, палитра и геометрия.

export const SPRITE_BASE = '/static/sprites/route/';

// Пулы вариантов (визуальный раздел спеки §12.4): вариант выбирается случайно на
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
// (случайный вариант на запуск); иначе — фиксированный вариант (§12.4).
export const SPRITE_FREEZE = {
    star_core: null, black_hole: null, beacon: null,
    false_signal: null, hazard_cloud: null, nebula_bg: null,
};

// Палитра: базовые константы проекта + доска v9 (§7.5, §12.2). Зарезервированные
// цвета интерактива (путь/захват/успех) не подменяются.
export const COLORS = {
    bg: '#05070f',
    border: '#334155',
    boardBg: '#0b1220',
    grid: 'rgba(51, 65, 85, 0.4)',
    lane: '#38bdf8',
    mud: '#8b5cf6',
    wall: '#334155',
    wallDark: '#1e293b',
    gate: '#f97316',
    bridge: '#22d3ee',
    current: '#38bdf8',
    deadEnd: '#ef4444',
    bottleneck: '#e2e8f0',
    sector: '#6366f1',
    unstable: '#ef4444',
    jackpot: '#fde047',
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

// dir — вектор шеврона течения: 0:+i, 1:−i, 2:+j, 3:−j.
export const DIR_VECTORS = [{ di: 1, dj: 0 }, { di: -1, dj: 0 }, { di: 0, dj: 1 }, { di: 0, dj: -1 }];

// ---- Словари объектов доски (§7.5) ----

export const SIG_LABELS = { 0: 'тихая', 1: 'средняя', 2: 'шумная' };
export const SURROUND_LABELS = { 0: 'пусто', 1: 'шлюз', 2: 'тупик', 3: 'топь', 4: 'течение' };
export const CONTENT_LABELS = {
    jackpot: 'Джекпот · дешёвый срез',
    lure: 'Приманка · дешёвый срез',
    trap: 'Капкан · очень дорого',
    decoy: 'Обманка · дорого',
    unstable: 'Нестабильность · помеха',
    empty: 'Пусто',
};
export const RISK_BY_SIG = { 0: 'низкий', 1: 'средний', 2: 'высокий' };
export const MODE_LABELS = {
    'русла': 'русла',
    'течения': 'течения',
    'шлюзы': 'шлюзы',
    'тупики/обманки': 'тупики/обманки',
    'топь': 'топь',
};

export function modeLabel(mode) { return MODE_LABELS[mode] || mode || '—'; }
export function sigLabel(sig) { return SIG_LABELS[sig] || '—'; }
export function surroundLabel(s) { return SURROUND_LABELS[s] || '—'; }
export function riskBySig(sig) { return RISK_BY_SIG[sig] || '—'; }
export function contentLabel(content) { return CONTENT_LABELS[content] || '—'; }

// bonusWord — нейтральные качественные слова результата по bonus со знаком
// (§4.7). Числа результата клиент не считает — только называет диапазон.
export function bonusWord(bonus) {
    const b = Number(bonus) || 0;
    if (b < 0) return 'Неудачный маршрут';
    if (b >= 0.42) return 'Блестящий маршрут';
    if (b >= 0.30) return 'Отличный маршрут';
    if (b >= 0.20) return 'Хороший маршрут';
    if (b >= 0.10) return 'Ровный маршрут';
    return 'Осторожный маршрут';
}

// ---- Паспорт (без чисел ETA, только °C и качественные слова) ----

// Дальность — качественно (без чисел и ETA, решение 14). Границы — UI-деление,
// приблизительные, не механика.
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

// remainWord — остаток в читаемом виде (только после отправки, решение 14).
export function remainWord(s) {
    const v = Number(s) || 0;
    if (v <= 0) return '0 с';
    return v < 60 ? Math.round(v) + ' с' : Math.round(v / 60) + ' мин';
}

// ---- Разметка HUD: паспорт-чипы и легенда объектов доски (§2/§4.9) ----
const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g,
    (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const chip = (t) => '<span class="hud-chip">' + esc(t) + '</span>';

// passportChips — чипы паспорт-полосы (без чисел ETA; температура только °C).
export function passportChips(p, board) {
    const b = board || {};
    const out = [];
    if (p && p.from && p.to) {
        out.push(chip('🛰 ' + distanceWord(p.dist)));
        out.push(chip(starClassWord(p.from) + ' → ' + starClassWord(p.to) + ' · ' + systemWord(p.to)));
        out.push(chip(tempWord(p.from.temperature) + ' → ' + tempWord(p.to.temperature)));
    }
    out.push(chip('Маяки ' + ((b.beacons || []).length) + ' · Секторы ' + ((b.sectors || []).length) +
        ' · Топь ' + ((b.mud || []).length) + ' · Русла ' + ((b.lane || []).length)));
    if (p && p.destination_belts && p.destination_belts.length) out.push(chip('Пояс: ' + beltWord(p.destination_belts[0])));
    return out.join('');
}

// LEGEND_ITEMS — список объектов доски для легенды (§4.9).
export const LEGEND_ITEMS = [
    { label: 'маяк', color: '#fde047' },
    { label: 'финиш', color: '#e2e8f0' },
    { label: 'русло', color: '#38bdf8' },
    { label: 'топь', color: '#8b5cf6' },
    { label: 'стена', color: '#334155' },
    { label: 'шлюз (платный)', color: '#f97316' },
    { label: 'мост (разовый)', color: '#22d3ee' },
    { label: 'течение', color: '#38bdf8' },
    { label: 'тупик', color: '#ef4444' },
    { label: 'узкий проход', color: '#e2e8f0' },
    { label: 'сектор σ', color: '#6366f1' },
];

// ---- Причины отказов (§3, §8): сырой текст ответа в UI не подставляем ----

// REASON_OVERLAY — оверлей-причина (409 offer и эквивалентные коды).
export const REASON_OVERLAY = {
    no_flight: { title: 'Сейчас нет активного перелёта', note: 'Откройте карту и начните перелёт' },
    no_module: { title: 'Ускоритель не установлен', note: 'Установите модуль ускорителя на корабль' },
    unknown_game: { title: 'Мини-игра недоступна', note: 'Этот ускоритель пока не поддерживает игру' },
    already_active: { title: 'Ускорение уже действует', note: 'Смена цели или прибытие сбросит его — тогда ускориться можно снова' },
    too_short: { title: 'Перелёт слишком короткий', note: 'Осталось мало пути для партии и ускорения' },
    cooldown: { title: 'Ускорение перезаряжается', note: 'Можно не ждать — вернитесь на карту' },
    no_field: { title: 'Доска не собралась', note: 'Поле этого перелёта не удалось построить — попробуйте другой перелёт' },
};

// REASON_TOAST — сообщения по коду причины (400/409 boost, 400/409 scan).
export const REASON_TOAST = {
    too_few_cells: 'Слишком короткий путь',
    cell_out_of_bounds: 'Клетка вне доски',
    not_adjacent: 'Путь разрывается — ведите путь заново',
    start_mismatch: 'Путь не начинается от СТАРТА',
    finish_mismatch: 'Путь не доходит до ФИНИШа',
    beacon_missing: 'Не все маяки на пути',
    changed: 'Маршрут изменился — вернитесь на карту',
    too_short: 'Перелёт слишком короткий',
    already_active: 'Ускорение уже действует',
    no_flight: 'Полёт уже завершён',
    no_module: 'Ускоритель не установлен',
    unknown_game: 'Этот ускоритель пока не поддерживает игру',
    no_field: 'Доска не собралась',
    no_pings: 'Импульсы закончились',
    bad_sector: 'Сектор не найден',
};

// ---- Геометрия и утилиты ----

// computeView — квадрат 1:1 (letterbox) в свободной области между HUD.
// Верх (паспорт+чип режима+статус) и низ (строка обратной связи/легенда +
// панель действий + «Проложить») зарезервированы; доска 1:1 по §6.
// n — из board.n (не хардкодить): рендер/ввод читают view.n.
export function computeView(vw, vh, n) {
    const top = 118;
    const bottom = 232;
    const avail = Math.max(120, vh - top - bottom);
    const size = Math.max(120, Math.min(vw - 24, avail));
    return { x0: (vw - size) / 2, y0: top + Math.max(0, (avail - size) / 2), size, n: n || 0 };
}

// hexA — '#rrggbb' → 'rgba(r,g,b,a)'.
export function hexA(hex, a) {
    const h = String(hex || '#ffffff').replace('#', '');
    const full = h.length === 3 ? h[0] + h[0] + h[1] + h[1] + h[2] + h[2] : h;
    const n = parseInt(full, 16);
    return 'rgba(' + ((n >> 16) & 255) + ',' + ((n >> 8) & 255) + ',' + (n & 255) + ',' + a + ')';
}

// mulberry32 — детерминированный RNG фона от seed.
export function mulberry32(seed) {
    let s = (seed >>> 0) || 1;
    return function () {
        s = (s + 0x6d2b79f5) | 0;
        let t = Math.imul(s ^ (s >>> 15), 1 | s);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

// hashSeed — числовой seed фона из fingerprint сегмента (косметика, детерминизм
// фона от сегмента; игровой истины не несёт).
export function hashSeed(str) {
    let h = 2166136261;
    const s = String(str || '');
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
    return h >>> 0;
}
