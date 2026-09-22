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

// CLASS_GLOW — классовая база яркости (§3 идеи: O ярче всех, Y тусклее).
// Единственный источник: и карта (star_render.js), и модалка (modal_render.js
// через coreStops), и starBrightness читают её отсюда.
export const CLASS_GLOW = {
    'O': 1.15, 'B': 1.12, 'A': 1.05, 'F': 1.00, 'G': 0.98,
    'K': 0.92, 'M': 0.85, 'L': 0.80, 'T': 0.72, 'Y': 0.65,
};

// coreStops — общая рецептура ядра «яркая точка» (правка @gdesigner 2026-09-22
// «звёзды-кольца-мишень»). g = CLASS_GLOW[sspec], k = clamp((g−0.82)/0.33, 0, 1):
//   r1 = 0.06 + 0.24·k   — граница сплошного белого;
//   r2 = 0.24 + 0.38·k   — цвет полностью побеждает;
//   a1 = 0.30 + 0.68·k   — альфа центра (классовая);
//   a2 = a1·(1−r1), ac = a1·(1−r2) — альфа на стопах r1/r2.
// Альфа монотонна по радиусу (наклон −a1), на краю 0 — нет «ямы», скачка
// вверх и резкого обрыва диска, которые глаз читал как концентрические кольца.
// Формула живёт ТОЛЬКО здесь: карта (star_render.js) и модалка
// (modal_render.js) импортируют её, локальных копий нет.
export function coreStops(sspec) {
    const g = CLASS_GLOW[sspec] || 1.0;
    const k = Math.min(Math.max((g - 0.82) / 0.33, 0), 1);
    const r1 = 0.06 + 0.24 * k;
    const r2 = 0.24 + 0.38 * k;
    const a1 = 0.30 + 0.68 * k;
    return { r1, r2, a1, a2: a1 * (1 - r1), ac: a1 * (1 - r2) };
}
