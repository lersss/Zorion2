// web/static/js/map/star_presets.js
// Дефолты вида звёзд — единственный источник правды (идея 2026-09-22
// «внешний вид звёзд», финализация). Раньше наборы дублировались: `PRESETS`
// в map/star_render.js и `PRESET_EFFECTS` в dashboard/graphics.js — два места
// могли разойтись. Теперь оба читают этот модуль.
//
// Модуль — только данные, без DOM/localStorage на верхнем уровне: его
// импортирует и карта (star_render.js), и дашборд (dashboard/graphics.js), а
// граф фронта грузится в Node (web/frontend_test.go). Импортировать сюда
// map/config.js или map/star_render.js нельзя — config.js на верхнем уровне
// берёт #mapCanvas и сломает дашборд/Node.

// PRESETS — режим отрисовки и флаги эффектов на каждый вид звёзд. `mode`
// ('vector' | 'sprite') и `crown` — как рисовать; petals/exotic/ignite/additive —
// набор эффектов вида (fallback для ключей localStorage).
export const PRESETS = {
    eye:    { mode: 'vector', crown: false, petals: false, exotic: true,  ignite: true, additive: true },
    photo:  { mode: 'vector', crown: false, petals: true,  exotic: true,  ignite: true, additive: true },
    sprite: { mode: 'sprite', crown: false, petals: false, exotic: true,  ignite: true, additive: true },
    crown:  { mode: 'vector', crown: true,  petals: false, exotic: true,  ignite: true, additive: true },
};

// PRESET_EFFECTS — те же наборы эффектов, что в PRESETS, в форме чекбоксов
// вкладки «⚙️ Графика» (плюс `twinkle`, у которого нет своего поля в PRESETS:
// мерцание включено у всех видов). Держать рядом с PRESETS обязательно — иначе
// вернётся риск расхождения, ради устранения которого модуль и создан.
export const PRESET_EFFECTS = {
    sprite: { twinkle: true, ignite: true, additive: true, exotic: true, petals: false },
    eye:    { twinkle: true, ignite: true, additive: true, exotic: true, petals: false },
    photo:  { twinkle: true, ignite: true, additive: true, exotic: true, petals: true },
    crown:  { twinkle: true, ignite: true, additive: true, exotic: true, petals: false },
};
