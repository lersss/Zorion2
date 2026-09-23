// web/static/js/game_date.js
//
// ⚠️ ЛОВУШКА (2026-09-23): любую игровую дату показывай ТОЛЬКО через этот модуль.
// `toLocaleString` / `Intl.DateTimeFormat` без явного UTC берут пояс браузера —
// у игроков в разных поясах одна метка покажется по-разному, и «общеигровой
// календарь» (+1000) ломается. Плюс не забудь +1000: сырой `new Date(...)` даст
// 2026-й вместо 3026-го. Арифметику времени (прогресс полёта, «осталось N»,
// сравнения Date.now()) сдвигать НЕЛЬЗЯ — здесь только показ.
//
// Общеигровой календарь (спека 2026-09-23-орбита-планеты-присутствие-и-снимок
// §7): год серверного времени показывается +1000 (GAME_YEAR_OFFSET), сами даты —
// в UTC (серверное время), одинаково у всех игроков; локальные часы браузера в
// отображении не участвуют. Сдвиг и формат — только на показе (И-С4): реальные
// метки в БД/API и вся арифметика времени (прогресс полёта, «осталось N»,
// сравнения Date.now()) не трогаются.
//
// Формат: gameDate → «23.09.3026, 14:05»; gameDateShort → «23.09.3026»;
// gameDateUtc → «23.09.3026, 14:05 UTC» (суффикс « UTC» — только у пресета
// «Балансировки»). Пустое/невалидное значение → «—».

export const GAME_YEAR_OFFSET = 1000;

// pad2 — двузначное число (05, 09, 14).
function pad2(n) {
    return String(n).padStart(2, '0');
}

// dateParts — части даты в UTC с игровым годом (+1000) или null для
// пустого/невалидного значения. Арифметика метки не меняется: год только
// прибавляется к отображаемому значению, сам Date не мутируется.
function dateParts(iso) {
    if (iso === null || iso === undefined || iso === '') return null;
    const d = new Date(iso);
    if (isNaN(d.getTime())) return null;
    return {
        day: pad2(d.getUTCDate()),
        month: pad2(d.getUTCMonth() + 1),
        year: d.getUTCFullYear() + GAME_YEAR_OFFSET,
        hours: pad2(d.getUTCHours()),
        minutes: pad2(d.getUTCMinutes()),
    };
}

// gameDate — «день.месяц.год, часы:минуты» (UTC, год +1000).
export function gameDate(iso) {
    const p = dateParts(iso);
    return p ? `${p.day}.${p.month}.${p.year}, ${p.hours}:${p.minutes}` : '—';
}

// gameDateShort — «день.месяц.год» (UTC, год +1000).
export function gameDateShort(iso) {
    const p = dateParts(iso);
    return p ? `${p.day}.${p.month}.${p.year}` : '—';
}

// gameDateUtc — как gameDate, но с суффиксом « UTC» (админ-подпись пресета
// «Балансировки», §7.2).
export function gameDateUtc(iso) {
    const p = dateParts(iso);
    return p ? `${p.day}.${p.month}.${p.year}, ${p.hours}:${p.minutes} UTC` : '—';
}
