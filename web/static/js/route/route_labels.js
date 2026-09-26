// web/static/js/route/route_labels.js
// Словари, метки и качественные формулировки страницы мини-игры «Прокладка
// маршрута» (спека 2026-09-25-маршрут-мини-игра-интерфейс.md §7.5/§8).
// Вынесено из route_config.js без изменения текстов и поведения; числа механики
// здесь не считаются — только тексты UI.

// ---- Словари объектов доски (§7.5, космический словарь §14.12) ----

export const SIG_LABELS = { 0: 'Тихий', 1: 'Ровный', 2: 'Гулкий' };
export const SURROUND_LABELS = { 0: 'Ничего', 1: 'Кордон', 2: 'Обрыв', 3: 'Мгла', 4: 'Течение' };
export const CONTENT_LABELS = {
    jackpot: 'Просвет',
    lure: 'Зов',
    trap: 'Воронка',
    decoy: 'Мираж',
    unstable: 'Сбой',
    empty: 'Вакуум',
};
export const RISK_BY_SIG = { 0: 'низкий', 1: 'средний', 2: 'высокий' };
// Ключи — сырые значения board.mode с сервера (не менять); игроку показывается
// метка чипа (§14.12.6).
export const MODE_LABELS = {
    'русла': 'Трассы',
    'течения': 'Течения',
    'шлюзы': 'Кордоны',
    'тупики/обманки': 'Обрывы и миражи',
    'топь': 'Мгла',
};

// MARK_LABELS — метки курса (§4.6), тексты без чисел. Метка dead_end — «Срыв»
// (объект dead_end_lane — «Обрыв»), чтобы одно слово не стояло в двух группах.
export const MARK_LABELS = {
    mud: 'Сопротивление', wall: 'Сопротивление',
    turn: 'Манёвр', dead_end: 'Срыв',
    current_against: 'Встречный поток', current_along: 'Попутный поток',
    hazard: 'Сбой', jackpot: 'Находка',
    gate_twice: 'Повторный кордон', bridge_twice: 'Повторный тоннель',
};

// ---- Разбор по факторам (§4.7.1/осн. §14.13) ----

// FACTOR_LABELS — имена факторов разбора по коду (§4.7.1; осн. §14.13.3/§14.13.4).
// Имена mud_cost («Сопротивление») и wall_cost («Помехи») разведены (критик #5):
// два разных фактора не должны давать одну строку с одним заголовком.
export const FACTOR_LABELS = {
    turn_cost: 'Манёвр',
    revisit: 'Петля',
    overshoot: 'Перелёт Цели',
    mud_cost: 'Сопротивление',
    mud_entry_repeat: 'Повторная мгла',
    gate_reentry: 'Повторный кордон',
    gate_useless: 'Кордон впустую',
    bridge_reuse: 'Повторный тоннель',
    dead_end: 'Срыв',
    current_against: 'Встречный поток',
    hidden_trap: 'Воронка — слепой срез',
    trap_entered_known: 'Воронка — знал и полез',
    decoy_penalty: 'Мираж',
    lure_missed: 'Зов прозеван',
    ping_wasted: 'Зонд впустую',
    ping_destabilize: 'Сбой',
    find_used: 'Находка',
    current_along: 'Попутный поток',
    wall_cost: 'Помехи',
};

// FACTOR_GLYPHS — code → глиф строки разбора (§4.7.1/§12.10): via 'heat' —
// метка курса (heatGlyph), via 'content' — содержимое сектора (contentGlyph),
// via 'new' — собственный глиф-фактор (§12.14: revisit/overshoot/ping_wasted).
// Цвет берётся внутри глифа (MARK_COLORS/COLORS), свотч не рисуется.
export const FACTOR_GLYPHS = {
    turn_cost: { via: 'heat', kind: 'turn' },
    revisit: { via: 'new' },
    overshoot: { via: 'new' },
    mud_cost: { via: 'heat', kind: 'mud' },
    mud_entry_repeat: { via: 'heat', kind: 'mud' },
    gate_reentry: { via: 'heat', kind: 'gate_twice' },
    gate_useless: { via: 'heat', kind: 'gate' },
    bridge_reuse: { via: 'heat', kind: 'bridge_twice' },
    dead_end: { via: 'heat', kind: 'dead_end' },
    current_against: { via: 'heat', kind: 'current_against' },
    hidden_trap: { via: 'content', kind: 'trap' },
    trap_entered_known: { via: 'content', kind: 'trap' },
    decoy_penalty: { via: 'content', kind: 'decoy' },
    lure_missed: { via: 'content', kind: 'lure' },
    ping_wasted: { via: 'new' },
    ping_destabilize: { via: 'heat', kind: 'hazard' },
    find_used: { via: 'content', kind: 'jackpot' },
    current_along: { via: 'heat', kind: 'current_along' },
    wall_cost: { via: 'heat', kind: 'wall' },
};

// BREAKDOWN_GROUPS — ключи сервера → заголовки; BREAKDOWN_ORDER — порядок
// отображения (§4.7.1: ошибки → находки → не ошибка).
export const BREAKDOWN_GROUPS = { error: 'Ошибки', gain: 'Находки', neutral: 'Не ошибка' };
export const BREAKDOWN_ORDER = ['error', 'gain', 'neutral'];

// factorName — имя с вариантом группы (ping_destabilize при neutral —
// «Сбой — в стороне»); неизвестный code → null (строку не рисуем).
export function factorName(code, group) {
    if (code === 'ping_destabilize' && group === 'neutral') return 'Сбой — в стороне';
    return FACTOR_LABELS[code] || null;
}

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
        ' · Мгла ' + ((b.mud || []).length) + ' · Трассы ' + ((b.lane || []).length)));
    if (p && p.destination_belts && p.destination_belts.length) out.push(chip('Пояс: ' + beltWord(p.destination_belts[0])));
    return out.join('');
}

// LEGEND_GROUPS — четыре свёрнутые группы попапа-легенды (§4.9): от «зачем летим»
// к «что на курсе». Все свёрнуты по умолчанию (раскрывашки — route_legend).
export const LEGEND_GROUPS = [
    { id: 'landmark', title: 'Ориентиры' },
    { id: 'field', title: 'Поле' },
    { id: 'sector', title: 'Секторы' },
    { id: 'marks', title: 'Метки курса' },
];

// LEGEND_ITEMS — азбука доски для попапа-легенды (§4.9): для каждой позиции
// {id, group, name, meaning} — название и одна строка смысла человеческим языком,
// без цветов и чисел механики. id из LEGEND_ITEMS → фигура (route_figures.drawLegendFigure),
// мини-фигура рисуется тем же кодом, что доска, — не спрайт и не цветной свотч.
// Путь/предпросмотр и вскрытое содержимое в легенду не входят (§4.9): название
// находки игрок читает в карточке сектора. Легенда статична (не состояние партии).
export const LEGEND_ITEMS = [
    { id: 'start', group: 'landmark', name: 'Старт', meaning: 'Твой корабль; отсюда начинается курс.' },
    { id: 'beacon', group: 'landmark', name: 'Маяк', meaning: 'Обязательная точка курса: без него маршрут не примут.' },
    { id: 'finish', group: 'landmark', name: 'Цель', meaning: 'Звезда перелёта — куда ведёт курс.' },
    { id: 'lane', group: 'field', name: 'Трасса', meaning: 'Прочищенный участок: лететь по нему дёшево и быстро.' },
    { id: 'mud', group: 'field', name: 'Мгла', meaning: 'Плотная мгла: корабль вязнет, каждый шаг дорог.' },
    { id: 'wall', group: 'field', name: 'Помехи', meaning: 'Помехи в гиперпространстве: пройти можно, но крайне дорого.' },
    { id: 'bottleneck', group: 'field', name: 'Разрыв', meaning: 'Узкая щель в помехах — единственный путь на ту сторону.' },
    { id: 'current', group: 'field', name: 'Течение', meaning: 'Направленный поток: по нему — дешевле, против — дорого.' },
    { id: 'gate', group: 'field', name: 'Кордон', meaning: 'На входе останавливают: каждый вход стоит времени.' },
    { id: 'bridge', group: 'field', name: 'Тоннель', meaning: 'Разовый проход: первый раз дёшево, повторно — дорого.' },
    { id: 'dead_end', group: 'field', name: 'Обрыв', meaning: 'Ложное ответвление: упирается в никуда, придётся возвращаться.' },
    { id: 'sector', group: 'sector', name: 'Сектор', meaning: 'Область с неизвестным содержимым: видна только гулкость.' },
    { id: 'sig', group: 'sector', name: 'Гулкость', sigRow: true, meaning: 'Эфир сектора: Тихий / Ровный / Гулкий — риск сорвать поле.' },
    { id: 'mark_resistance', group: 'marks', name: MARK_LABELS.mud, meaning: 'Клетка тормозит корабль: каждый такой шаг съедает время.' },
    { id: 'mark_turn', group: 'marks', name: MARK_LABELS.turn, meaning: 'Смена курса: каждый поворот стоит времени.' },
    { id: 'mark_dead_end', group: 'marks', name: MARK_LABELS.dead_end, meaning: 'Курс зашёл в ответвление без выхода — придётся возвращаться.' },
    { id: 'mark_against', group: 'marks', name: MARK_LABELS.current_against, meaning: 'Идёшь против течения: медленно и дорого.' },
    { id: 'mark_along', group: 'marks', name: MARK_LABELS.current_along, meaning: 'Течение несёт корабль: этот шаг дешевле.' },
    { id: 'mark_hazard', group: 'marks', name: MARK_LABELS.hazard, meaning: 'Зонд разворошил поле: клетка и подходы к ней дороже.' },
    { id: 'mark_jackpot', group: 'marks', name: MARK_LABELS.jackpot, meaning: 'Вскрытый сектор с дешёвым срезом на курсе — выигрыш времени.' },
    { id: 'mark_gate_twice', group: 'marks', name: MARK_LABELS.gate_twice, meaning: 'Через этот кордон уже шёл: второй вход снова стоит времени.' },
    { id: 'mark_bridge_twice', group: 'marks', name: MARK_LABELS.bridge_twice, meaning: 'Тоннель разовый: повторный проход дорог.' },
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
    start_mismatch: 'Курс не начинается от Старта',
    finish_mismatch: 'Курс не доходит до Цели',
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
