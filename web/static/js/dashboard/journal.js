// web/static/js/dashboard/journal.js
// Журнал открытий игрока (спека 86a §4.3): клиентское хранилище в
// localStorage, ключ zorion.journal. Единственная точка чтения/записи —
// остальные модули вызывают journal.record(...). Открытия — презентационный
// слой (И8): серверной таблицы нет, журнал per-browser (§7.4).

import { PLANET_TYPE_INFO } from './planet_types.js';

const JOURNAL_KEY = 'zorion.journal';

// EXOTIC_STAR_TYPES — 4 экзотических типа звёзд (29a/41a, спека 86a §4.2):
// знаменатель «открыто X из 4» и фильтр записи seenStarTypes.
export const EXOTIC_STAR_TYPES = ['white_dwarf', 'neutron', 'black_hole', 'protostar'];

const EMPTY_JOURNAL = {
    metRaces: [],        // race_id встреченных рас (пусто = люди — не пишем)
    seenStarTypes: [],   // экзотические star_type, встреченные игроком
    seenPlanetTypes: [], // типы планет, чьи карточки открыты первым посещением
    visitedWorlds: [],   // world_id миров, карточку которых открывали
    flights: 0,          // завершённые полёты
};

function load() {
    try {
        const raw = localStorage.getItem(JOURNAL_KEY);
        if (!raw) return { ...EMPTY_JOURNAL };
        const parsed = JSON.parse(raw);
        return {
            metRaces: Array.isArray(parsed.metRaces) ? parsed.metRaces : [],
            seenStarTypes: Array.isArray(parsed.seenStarTypes) ? parsed.seenStarTypes : [],
            seenPlanetTypes: Array.isArray(parsed.seenPlanetTypes) ? parsed.seenPlanetTypes : [],
            visitedWorlds: Array.isArray(parsed.visitedWorlds) ? parsed.visitedWorlds : [],
            flights: typeof parsed.flights === 'number' ? parsed.flights : 0,
        };
    } catch (e) {
        console.error('Журнал повреждён, сброс:', e);
        return { ...EMPTY_JOURNAL };
    }
}

function save(j) {
    try {
        localStorage.setItem(JOURNAL_KEY, JSON.stringify(j));
    } catch (e) {
        console.error('Не удалось сохранить журнал:', e);
    }
}

function pushUnique(arr, value) {
    if (!value) return arr;
    return arr.includes(value) ? arr : arr.concat(value);
}

// record — аддитивная запись встреченного (спека 86a §4.3). Ничего не
// блокирует и не перехватывает: только читает уже полученные данные.
// opts: { worldId, starType, planetTypes: [], raceIds: [] }
export function record(opts) {
    const j = load();
    if (opts.worldId) j.visitedWorlds = pushUnique(j.visitedWorlds, opts.worldId);
    if (opts.starType && EXOTIC_STAR_TYPES.includes(opts.starType)) {
        j.seenStarTypes = pushUnique(j.seenStarTypes, opts.starType);
    }
    // Типы планет — только из реестра энциклопедии (9 типов, §5.3.1):
    // генератор знает и «мёртвая» (classify.go:18), её в реестре нет —
    // внереестровые типы не пишем, иначе счётчик «X из 9» врёт (гейт 3, 86a).
    (opts.planetTypes || []).forEach(t => {
        if (PLANET_TYPE_INFO[t]) j.seenPlanetTypes = pushUnique(j.seenPlanetTypes, t);
    });
    // race_id пусто = легаси/люди — люди «встречены» по умолчанию (§5.1.3),
    // в журнал не пишем.
    (opts.raceIds || []).forEach(id => { if (id) j.metRaces = pushUnique(j.metRaces, id); });
    save(j);
}

// recordFlight — завершённый полёт (событие прибытия, спека 86a §4.3).
export function recordFlight() {
    const j = load();
    j.flights = (typeof j.flights === 'number' ? j.flights : 0) + 1;
    save(j);
}

// get — текущий журнал (копия для чтения).
export function get() {
    return load();
}