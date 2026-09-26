// web/static/js/route/route_config.js
// Базовые константы страницы мини-игры «Прокладка маршрута» на доске v9
// «Планшет» (спека 2026-09-25-маршрут-мини-игра-интерфейс.md §7.5/§8): спрайты,
// палитра и геометрия. Числа механики (цены, τ, пороги) — серверный контракт:
// клиент их не считает и в UI не показывает.
//
// Словари/метки вынесены в route_labels.js, геометрия и утилиты — в
// route_geometry.js; реэкспорт ниже сохраняет прежние импорты
// (`import * as C from './route_config.js'`).

export const SPRITE_BASE = '/static/sprites/route/';

// Пулы вариантов (визуальный раздел спеки §12.4): вариант выбирается случайно на
// запуск (режим наигрыша); заморозка — одно имя на тип в SPRITE_FREEZE.
// Рабочие (замороженные) варианты — `*_01`; прочие варианты пула лежат в
// `sprites/route/_pool/` и в предзагрузку не попадают (сцена не падает без них).
export const POOLS = {
    star_core: ['star_core_01'],
    black_hole: ['black_hole_01'],
    beacon: ['beacon_01'],
    false_signal: ['false_signal_01'],
    hazard_cloud: ['hazard_cloud_01'],
    nebula_bg: ['nebula_bg_01'],
};

// Заморозка после выбора победителей пула: type → имя файла пула. Выбор
// делегирован создателем визуальному дизайнеру (2026-09-26), победители — `_01`
// на каждый тип (§12.4). null = наигрыш (случайный вариант на запуск).
export const SPRITE_FREEZE = {
    star_core: 'star_core_01', black_hole: 'black_hole_01', beacon: 'beacon_01',
    false_signal: 'false_signal_01', hazard_cloud: 'hazard_cloud_01', nebula_bg: 'nebula_bg_01',
};

// Палитра: базовые константы проекта + доска v9 (§7.5, §12.2). Зарезервированные
// цвета интерактива (путь/захват/успех) не подменяются.
export const COLORS = {
    bg: '#05070f',
    border: '#334155',
    boardBg: '#0b1220',
    grid: 'rgba(51, 65, 85, 0.4)',
    lane: '#38bdf8',
    mud: '#7e22ce',
    mudGrain: '#c084fc',
    wall: '#334155',
    wallDark: '#1e293b',
    wallLine: '#475569',
    gate: '#fb923c',
    bridge: '#f0abfc',
    current: '#2dd4bf',
    deadEnd: '#f43f5e',
    bottleneck: '#cbd5e1',
    sector: '#7c8db5',
    unstable: '#ef4444',
    jackpot: '#facc15',
    lure: '#facc15',
    decoy: '#94a3b8',
    empty: '#64748b',
    heatCost: '#f59e0b',
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

export * from './route_labels.js';
export * from './route_geometry.js';
