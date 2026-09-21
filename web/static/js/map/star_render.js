// web/static/js/map/star_render.js
// Новый внешний вид звёзд на карте (прототип, идея 2026-09-22 «внешний вид
// звёзд»). Вся новая отрисовка одиночных звёзд — здесь; map_render.js только
// вызывает drawStar() и больше не рисует тёмную обводку.
//
// Направления (дизайн @gdesigner §4–§8):
//   A «Чистый глаз» — векторно: пересвеченное ядро → цвет → мягкий ореол,
//     разная яркость от hashString(sid), мерцание, аддитивный блум при малом
//     числе одиночных звёзд;
//   B «Деликатные лепестки» — 6 коротких тающих лепестков у ярких O/B/A;
//   C «Предзапечённый спрайт» — ядро+ореол запечены в offscreen-спрайт на
//     класс × 3 ведра яркости; гибрид: при сильном зуме ядро снова векторное;
//   D «Живая корона» — векторное ядро как в A + запечённый венец «плазмы»
//     (2 кадра на класс, вращение + кроссфейд).
// Экзотика (§8) — своей веткой по stype, минуя classGlow (у экзотики
// spectral_class пуст → classGlow[sspec] дал бы NaN и звезда пропала бы).
//
// Производительность (решение создателя 2026-09-22): дорогой вид — только
// когда одиночных звёзд на экране мало. Мерцание авто-стоп при singles > 400,
// аддитивное свечение — только при singles ≤ 250. Непрерывный кадр — один
// хозяин (ensureStarAnimLoop), авто-стоп на скрытой вкладке.
//
// Модуль не трогает document/канвас на верхнем уровне (Node-граф фронта,
// web/frontend_test.go): offscreen-канвасы создаются лениво, внутри функций.

import { state } from './config.js';
import { getStarShade, getStarColor } from './utils.js';
import { PRESETS } from './star_presets.js';

// requestRedraw — перерисовка карты. Модуль НЕ импортирует map_render.js:
// прямой импорт давал цикл map_render → star_render → map_render, и вызовы
// draw() падали с ReferenceError. map_render.js ставит свой draw через
// setStarRedraw (по образцу ship_sprites.setRedrawCallback).
let requestRedraw = () => {};

// setStarRedraw — установка колбэка перерисовки карты (map_render.js).
export function setStarRedraw(fn) {
    if (typeof fn === 'function') requestRedraw = fn;
}

// ==================== ПАРАМЕТРЫ (дизайн §3–§8) ====================

// Классовая база яркости (§3): O ярче всех, Y тусклее.
const CLASS_GLOW = {
    'O': 1.15, 'B': 1.12, 'A': 1.05, 'F': 1.00, 'G': 0.98,
    'K': 0.92, 'M': 0.85, 'L': 0.80, 'T': 0.72, 'Y': 0.65,
};

// Потолки против «стены света» на сильном зуме (§3): ядро не превращается
// в 420-пиксельный диск, дальше растёт только ореол.
const CORE_MAX_PX = 48;
const HALO_MAX_PX = 240;
const BLOOM_MAX_PX = 160;

// Пороги деградации (§3, решение создателя): мерцание и аддитивный слой —
// только при малом числе одиночных звёзд в кадре.
const TWINKLE_MAX_SINGLES = 400;
const ADDITIVE_MAX_SINGLES = 250;

// Зажигание (§3): разовое разгорание в момент появления названий.
const IGNITE_MS = 900;
const IGNITE_FLASH_AT = 0.2;   // пик вспышки ×1.3
const IGNITE_FLASH_W = 0.12;   // ширина гаусса вспышки

// Спрайт (C, §6): 128×128, 3 ведра яркости на класс.
const SPRITE_SIZE = 128;
const SPRITE_BUCKETS = 3;
const SPRITE_HYBRID_R = 34;    // R > 34 px — ядро снова векторное (снять «мыло»)

// Лепестки (B, §5): 6 штук через 60°, чередование длин.
const PETAL_COUNT = 6;
const PETAL_ALPHA = 0.18;

// «Корона» (D, §7): 2 запечённых кадра венца на класс, 192×192, асимметричные
// «языки» плазмы (fBm-шум 5 октав), вращение + кроссфейд периодом 3 с.
const CROWN_SIZE = 192;
const CROWN_FRAMES = 2;
const CROWN_NOISE_OCTAVES = 5;
const CROWN_RING_R = 0.55;     // базовый радиус кольца (доли полуразмера кадра)
const CROWN_DRAW_SCALE = 2.6;  // диаметр венца в кадре = 2.6·rh
const CROWN_ALPHA = 0.32;      // §7: альфа венца 0.32·B
const CROWN_SPIN = 0.03;       // §7: вращение 0.03·t (рад/с)
const CROWN_XFADE_HZ = 0.33;   // §7: кроссфейд кадров периодом 3 с

// Экзотика (§8).
const EXOTIC_TYPES = ['black_hole', 'neutron', 'white_dwarf', 'protostar'];

// ==================== ПРЕСЕТЫ И ТУМБЛЕРЫ (демо-инструмент создателя) ====================

// STAR_VISUAL_KEY — localStorage-ключ выбранного пресета (по образцу
// radarBoundaryVariant, спека 77a §9.1). Управление вынесено в дашборд
// (вкладка «⚙️ Графика», идея 2026-09-22), карта только читает ключ.
const STAR_VISUAL_KEY = 'starVisualPreset';

// STAR_VISUAL_TWINKLE_KEY — тумблер «Мерцание звёзд» (вкладка «Графика»).
// Раньше мерцание не имело тумблера: включалось всегда, пока singles ≤ 400.
const STAR_VISUAL_TWINKLE_KEY = 'starVisualTwinkle';

// STAR_VISUAL_LITE_KEY — режим «Для слабых ПК» (вкладка «Графика»). При '1'
// starVisualOptions() отдаёт самый дешёвый вид и все эффекты выключенными,
// не трогая сохранённые ключи игрока.
const STAR_VISUAL_LITE_KEY = 'starVisualLite';

// Пресеты (§9) — в чистом модуле данных star_presets.js (единый источник с
// дашбордом, чтобы PRESETS и PRESET_EFFECTS не расходились). D «Корона»:
// crown=true — ветка векторного ядра + живого венца.

// starVisualPreset — текущий пресет (localStorage, дефолт «Спрайт» —
// решение создателя 2026-09-22, гейт 2: спрайтовый кадр легче на сильном зуме).
export function starVisualPreset() {
    const v = localStorage.getItem(STAR_VISUAL_KEY);
    return PRESETS[v] ? v : 'sprite';
}

// setStarVisualPreset — выбор пресета (дашборд, вкладка «⚙️ Графика», и e2e).
export function setStarVisualPreset(v) {
    if (!PRESETS[v]) return;
    localStorage.setItem(STAR_VISUAL_KEY, v);
    resetIgnite();
    requestRedraw();
}

// starVisualOptions — эффективные настройки: пресет + тумблеры.
// Тумблеры хранятся отдельными ключами (вкладка «⚙️ Графика» дашборда).
export function starVisualOptions() {
    // Режим «Для слабых ПК» (идея 2026-09-22, §4): принудительно самый дешёвый
    // вид (спрайт) и все эффекты выключены. Сохранённые ключи игрока не
    // трогаются: при выключении режима возвращаются его прежние значения.
    if (toggle(STAR_VISUAL_LITE_KEY, false)) {
        return {
            mode: 'sprite',
            crown: false,
            twinkle: false,
            ignite: false,
            additive: false,
            exotic: false,
            petals: false,
        };
    }
    const preset = PRESETS[starVisualPreset()];
    return {
        mode: preset.mode,
        crown: !!preset.crown,
        twinkle: toggle(STAR_VISUAL_TWINKLE_KEY, true),
        petals: toggle('starVisualPetals', preset.petals),
        exotic: toggle('starVisualExotic', preset.exotic),
        ignite: toggle('starVisualIgnite', preset.ignite),
        additive: toggle('starVisualAdditive', preset.additive),
    };
}

// setStarVisualToggle — переключение тумблера (дашборд, вкладка «⚙️ Графика»).
export function setStarVisualToggle(key, on) {
    localStorage.setItem(key, on ? '1' : '0');
    if (key === 'starVisualIgnite') resetIgnite();
    requestRedraw();
}

function toggle(key, fallback) {
    const v = localStorage.getItem(key);
    if (v === null) return fallback;
    return v === '1';
}

// ==================== ДЕТЕРМИНИЗМ ОТ sid (§3) ====================

// hashString — 32-битный БЕЗЗНАКОВЫЙ хеш строки (тот же алгоритм, что в
// map_render.js; локальная копия, чтобы не тянуть map_render в Node-граф
// лишний раз). `>>> 0` обязателен: `& 0xFFFFFFFF` в JS даёт int32, и
// отрицательный хеш ломал starBits (b1 < 0 → Math.pow(b1, 1.8) = NaN).
export function hashString(s) {
    let hash = 0;
    for (let i = 0; i < s.length; i++) {
        hash = (hash * 31 + s.charCodeAt(i)) & 0xFFFFFFFF;
    }
    return hash >>> 0;
}

// starBits — три детерминированных числа 0..1 от sid (§3): b1 — яркость,
// b2 — фаза/ориентация, b3 — частота мерцания.
export function starBits(sid) {
    const h = hashString(String(sid || ''));
    return {
        b1: (h % 1024) / 1024,
        b2: ((h >>> 10) % 1024) / 1024,
        b3: ((h >>> 20) % 1024) / 1024,
    };
}

// starBrightness — множитель яркости (§3): перекос к тусклым t = b1^1.8,
// bright = 0.68 + 0.62·t (0.68…1.30); «маяки» b1 ≥ 0.965 (~3.5 %) — 1.35.
// Итог B = clamp(bright · classGlow[sspec], 0.5, 1.6). Экзотика — своя ветка
// (classGlow у неё нет — иначе NaN).
export function starBrightness(sspec, b1) {
    const t = Math.pow(b1, 1.8);
    let bright = 0.68 + 0.62 * t;
    if (b1 >= 0.965) bright = 1.35;
    const glow = CLASS_GLOW[sspec] || 1.0;
    return Math.min(Math.max(bright * glow, 0.5), 1.6);
}

// starTwinkle — множитель мерцания (§3): медленное «дыхание» 0.10–0.45 Гц,
// период 2.9–10 с, у каждой звезды своя фаза. Возвращает {a, r} — множители
// альфы ореола и радиуса ядра.
export function starTwinkle(b2, b3, tSec) {
    const f = 0.10 + 0.35 * b3;
    const phase = 2 * Math.PI * b2;
    const s = Math.sin(2 * Math.PI * f * tSec + phase);
    return { a: 1 + 0.08 * s, r: 1 + 0.04 * s };
}

// ==================== ЗАЖИГАНИЕ (§3) ====================

// igniteStart — время старта зажигания (мс) или 0. Триггер — пересечение
// порога появления названий снизу вверх (и один раз после загрузки, если
// scale уже выше).
let igniteStart = 0;
let igniteArmed = false;

// resetIgnite — сброс зажигания (смена пресета/тумблера).
export function resetIgnite() {
    igniteStart = 0;
    igniteArmed = false;
}

// igniteProgress — прогресс зажигания 0..1 (1 — завершено/не идёт).
export function igniteProgress(nowMs) {
    if (!igniteStart) return 1;
    const p = (nowMs - igniteStart) / IGNITE_MS;
    return p >= 1 ? 1 : Math.max(0, p);
}

// igniteActive — идёт ли зажигание (нужно для непрерывного кадра).
export function igniteActive(nowMs) {
    return igniteStart > 0 && nowMs - igniteStart < IGNITE_MS;
}

// igniteLabelAlpha — альфа подписи при каскаде (§3): alpha =
// clamp((t−0.25)/0.75) со сдвигом 0…250 мс по b2. t — прогресс зажигания.
export function igniteLabelAlpha(t, b2) {
    if (t >= 1) return 1;
    const delay = 0.25 * b2; // 0…250 мс в долях длительности
    const local = (t - delay) / 0.75;
    return Math.min(Math.max(local, 0), 1);
}

// igniteCoreScale — радиус ядра 0.4R→1.0R по easeOutCubic + вспышка ×1.3
// гауссом на t ≈ 0.2 (§3).
export function igniteCoreScale(t) {
    if (t >= 1) return 1;
    const e = 1 - Math.pow(1 - t, 3);
    const flash = 1 + 0.3 * Math.exp(-Math.pow((t - IGNITE_FLASH_AT) / IGNITE_FLASH_W, 2));
    return (0.4 + 0.6 * e) * flash;
}

// ==================== ВИДИМЫЙ РАЗМЕР И HIT-ЗОНА (§3, решение создателя п.2) ====================

// starVisualRadius — видимый радиус звезды (ореол с потолком): к нему
// привязываются позиция подписи и зона клика/наведения. R — базовый радиус
// clusterScreenRadius(c), считается вызывающим (map_render.js): сам модуль
// map_render.js не импортирует (разрыв цикла). Не путать с clusterScreenRadius
// (база, семантику не меняем).
export function starVisualRadius(c, R) {
    const { b2 } = starBits(c.sid || c.sname || '');
    const glow = CLASS_GLOW[c.sspec] || 1.0;
    const rh = Math.min(R * (1.8 + 1.1 * b2) * glow, HALO_MAX_PX);
    // Видимый радиус не превышает потолок ореола (HALO_MAX_PX): при сильном
    // зуме (R > 240) необрезанный R уводил якорь подписи/hover-пилюли и hit-зону
    // от нарисованного ореола — ровно дефект, который убирает решение создателя
    // 2026-09-22 п.2 (звоночек критика №2).
    return Math.min(Math.max(R, rh), HALO_MAX_PX);
}

// starHitRadius — радиус hit-зоны (клик/hover): видимый размер, но не
// больше 1.6·R — иначе на сильном зуме клик по пустоте выбирал бы звезду
// (звоночек критика №2, решение создателя «ок»). R — clusterScreenRadius(c),
// передаётся снаружи (map_render.js/events.js).
export function starHitRadius(c, R) {
    return Math.min(starVisualRadius(c, R), R * 1.6);
}

// ==================== СПРАЙТЫ (C, §6) ====================

// spriteCache — ключ "sspec|bucket" → offscreen canvas 128×128.
let spriteCache = null;

// spriteFor — спрайт класса и ведра яркости; создаётся лениво (offscreen
// canvas — только внутри функции, не на верхнем уровне модуля).
function spriteFor(sspec, bucket) {
    if (!spriteCache) spriteCache = new Map();
    const key = (sspec || 'G') + '|' + bucket;
    let cv = spriteCache.get(key);
    if (cv) return cv;
    cv = bakeSprite(sspec, bucket);
    spriteCache.set(key, cv);
    return cv;
}

// coreWhiteStops — доля белого «раскалённого» ядра пропорционально яркости
// класса (правка @gdesigner 2026-09-22 «белое ядро пропорционально яркости»):
// g = CLASS_GLOW[sspec], k = clamp((g−0.82)/0.33, 0, 1). При k=1 (O) получается
// ровно прежняя рецептура (0.30/0.62, 0.98/0.90) — «вау» ярких не меняем; при
// k=0 (L/T/Y) белый блик маленький и слабый, дальше доминирует собственный цвет.
// Общая формула для векторного ядра (drawCore) и спрайта (bakeSprite), чтобы
// «Глаз» и «Спрайт» совпадали. Новых градиентов на звезду нет — те же стопы.
export function coreWhiteStops(sspec) {
    const g = CLASS_GLOW[sspec] || 1.0;
    const k = Math.min(Math.max((g - 0.82) / 0.33, 0), 1);
    return {
        r1: 0.06 + 0.24 * k,
        r2: 0.24 + 0.38 * k,
        a1: 0.30 + 0.68 * k,
        a2: 0.16 + 0.74 * k,
    };
}

// bakeSprite — запекает ядро+ореол в offscreen-канвас (§6): рецептура A,
// но один раз на класс × ведро яркости. В кадре — один drawImage.
function bakeSprite(sspec, bucket) {
    const cv = document.createElement('canvas');
    cv.width = SPRITE_SIZE;
    cv.height = SPRITE_SIZE;
    const g = cv.getContext('2d');
    const cx = SPRITE_SIZE / 2;
    const cy = SPRITE_SIZE / 2;
    const color = getStarColor(sspec || 'G', 'star');
    const B = 0.7 + 0.3 * (bucket / (SPRITE_BUCKETS - 1)); // ведро яркости
    const rc = SPRITE_SIZE * 0.16;
    const rh = SPRITE_SIZE * 0.5;

    const halo = g.createRadialGradient(cx, cy, 0, cx, cy, rh);
    halo.addColorStop(0, rgba(color, 0.42 * B));
    halo.addColorStop(0.20, rgba(color, 0.20 * B));
    halo.addColorStop(0.55, rgba(color, 0.07 * B));
    halo.addColorStop(1, rgba(color, 0));
    g.fillStyle = halo;
    g.beginPath();
    g.arc(cx, cy, rh, 0, Math.PI * 2);
    g.fill();

    const core = g.createRadialGradient(cx, cy, 0, cx, cy, rc);
    const cs = coreWhiteStops(sspec || 'G');
    core.addColorStop(0, `rgba(255,255,255,${cs.a1})`);
    core.addColorStop(cs.r1, `rgba(255,255,255,${cs.a2})`);
    core.addColorStop(cs.r2, rgba(color, 0.98));
    core.addColorStop(1, rgba(color, 0.55));
    g.fillStyle = core;
    g.beginPath();
    g.arc(cx, cy, rc, 0, Math.PI * 2);
    g.fill();
    return cv;
}

// ==================== ОТРИСОВКА ОДИНОЧНОЙ ЗВЕЗДЫ ====================

// drawStar — точка входа из map_render.js. Рисует одиночную звезду новым
// видом; состояния (focus/hover/«моя звезда»/компаньоны) остаются в
// map_render.js и рисуются поверх. R — базовый clusterScreenRadius(c) (считает
// вызывающий: своего импорта map_render.js у модуля нет).
export function drawStar(ctx, c, x, y, R, opts) {
    const o = opts || starVisualOptions();
    const stype = c.stype || 'star';

    // Экзотика — своя ветка (§8), минуя classGlow (иначе NaN/пропажа).
    if (o.exotic && EXOTIC_TYPES.includes(stype)) {
        drawExotic(ctx, c, x, y, R, stype, o);
        return;
    }

    const { b1, b2, b3 } = starBits(c.sid || c.sname || '');
    const B = starBrightness(c.sspec, b1);
    const now = Date.now();
    const t = igniteProgress(now);
    const tw = twinkleFactor(b2, b3, now, o);
    const color = getStarShade(c.sspec || 'G', c.stemp, c.stype);

    // D «Корона» (§7): векторное ядро как в A + живой венец плазмы (вращение и
    // кроссфейд двух запечённых кадров). Венец глушится режимом «Для слабых ПК»
    // (там mode='sprite', crown=false).
    if (o.crown) {
        drawHalo(ctx, x, y, R, color, B, b2, tw, t, false, c.sspec);
        drawBloom(ctx, x, y, R, B, tw, t, o);
        drawCore(ctx, x, y, R, color, B, b3, tw, t, c.sspec);
        drawCrownWreath(ctx, x, y, R, B, b2, c.sspec);
        if (o.petals) drawPetals(ctx, x, y, R, color, B, b2, c.sspec);
        return;
    }

    if (o.mode === 'sprite' && R <= SPRITE_HYBRID_R) {
        drawSpriteStar(ctx, c, x, y, R, B, tw, t);
        return;
    }

    // Гибрид C (§6): при сильном зуме ядро векторное, спрайтом — ореол.
    if (o.mode === 'sprite') {
        drawHalo(ctx, x, y, R, color, B, b2, tw, t, true, c.sspec);
        drawCore(ctx, x, y, R, color, B, b3, tw, t, c.sspec);
        return;
    }

    // A «Чистый глаз»: ореол → блум → ядро → лепестки.
    drawHalo(ctx, x, y, R, color, B, b2, tw, t, false, c.sspec);
    drawBloom(ctx, x, y, R, B, tw, t, o);
    drawCore(ctx, x, y, R, color, B, b3, tw, t, c.sspec);
    if (o.petals) drawPetals(ctx, x, y, R, color, B, b2, c.sspec);
}

// drawCompanion — «звёздный» вид компаньона двойной/кратной системы (решение
// создателя 2026-09-22, гейт 3): то же ядро+ореол, что у основной звезды, но
// уменьшенное (radius = 0.45·R родителя) — компаньон не конкурирует с ней.
// Детерминизм — от seed (sid родителя + индекс компаньона): яркость и форма
// ореола не зависят от кадра. Цвет — по спектру компаньона (sspec), без
// экзотики (компаньоны — обычные звёзды). Блум/лепестки не рисуем: у
// компаньона только ядро и мягкий ореол. Пресет «Спрайт» — ореол из
// запечённого спрайта, ядро векторное (как гибрид основной звезды).
export function drawCompanion(ctx, x, y, radius, sspec, opts, seed) {
    const o = opts || starVisualOptions();
    if (!(radius > 0)) return;
    const { b1, b2, b3 } = starBits(String(seed || sspec || ''));
    const B = starBrightness(sspec, b1);
    const now = Date.now();
    const t = igniteProgress(now);
    const tw = twinkleFactor(b2, b3, now, o);
    const color = getStarColor(sspec || 'G', 'star');
    drawHalo(ctx, x, y, radius, color, B, b2, tw, t, o.mode === 'sprite', sspec);
    drawCore(ctx, x, y, radius, color, B, b3, tw, t, sspec);
}

// twinkleFactor — множитель мерцания с авто-стопом (§3): при переполнении
// экрана одиночными звёздами мерцание выключено (дешёвый вид). Тумблер
// «Мерцание звёзд» (starVisualTwinkle) и режим «Для слабых ПК» гасят мерцание
// явным `twinkle: false` в opts.
function twinkleFactor(b2, b3, nowMs, o) {
    if (o.twinkle === false || !twinkleAllowed()) return { a: 1, r: 1 };
    return starTwinkle(b2, b3, nowMs / 1000);
}

// twinkleAllowed — мерцание разрешено, пока одиночных звёзд в кадре мало.
function twinkleAllowed() {
    return (state.starSingles || 0) <= TWINKLE_MAX_SINGLES;
}

// drawHalo — мягкий ореол (§4): градиент (x,y,0)→(x,y,rh), source-over.
// spriteOnly — режим гибрида C: ореол берётся из запечённого спрайта.
function drawHalo(ctx, x, y, R, color, B, b2, tw, t, spriteOnly, sspec) {
    const glow = CLASS_GLOW[sspec] || 1.0;
    const rh = Math.min(R * (1.8 + 1.1 * b2) * glow, HALO_MAX_PX);
    const alpha = (t >= 1 ? 1 : t) * tw.a;

    if (spriteOnly) {
        const bucket = Math.min(SPRITE_BUCKETS - 1, Math.floor(B / 1.6 * SPRITE_BUCKETS));
        const sprite = spriteFor(sspec, bucket);
        const s = rh * 2;
        ctx.save();
        ctx.globalAlpha = Math.min(1, alpha);
        ctx.drawImage(sprite, x - s / 2, y - s / 2, s, s);
        ctx.restore();
        return;
    }

    const g = ctx.createRadialGradient(x, y, 0, x, y, rh);
    g.addColorStop(0, rgba(color, 0.42 * B * alpha));
    g.addColorStop(0.20, rgba(color, 0.20 * B * alpha));
    g.addColorStop(0.55, rgba(color, 0.07 * B * alpha));
    g.addColorStop(1, rgba(color, 0));
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(x, y, rh, 0, Math.PI * 2);
    ctx.fill();
}

// drawCore — пересвеченное ядро (§4): белый центр → цвет → прозрачность.
// Доля белого — coreWhiteStops(sspec) (правка @gdesigner 2026-09-22): у ярких
// классов ядро пересвечено как раньше, у тусклых белый блик мал и цвет виден.
function drawCore(ctx, x, y, R, color, B, b3, tw, t, sspec) {
    const rc = Math.min(0.85 * R * (0.9 + 0.2 * b3) * tw.r, CORE_MAX_PX) *
        (t >= 1 ? 1 : igniteCoreScale(t));
    if (!(rc > 0)) return;
    const g = ctx.createRadialGradient(x, y, 0, x, y, rc);
    const cs = coreWhiteStops(sspec);
    g.addColorStop(0, `rgba(255,255,255,${cs.a1})`);
    g.addColorStop(cs.r1, `rgba(255,255,255,${cs.a2})`);
    g.addColorStop(cs.r2, rgba(color, 0.98));
    g.addColorStop(1, rgba(color, 0.55));
    ctx.save();
    ctx.globalAlpha = Math.min(1, B);
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(x, y, rc, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// drawBloom — аддитивный блум (§4): только при малом числе одиночных звёзд
// и тумблере «Аддитивное»; иначе тот же градиент source-over с альфой ×0.5.
// После слоя режим рисования ОБЯЗАТЕЛЬНО возвращается (иначе подписи и
// маркеры смешаются «светом» — правка ТЗ по находке критика).
function drawBloom(ctx, x, y, R, B, tw, t, o) {
    const rb = Math.min(1.7 * R, BLOOM_MAX_PX);
    if (!(rb > 0)) return;
    const additive = o.additive && (state.starSingles || 0) <= ADDITIVE_MAX_SINGLES;
    const alpha = (t >= 1 ? 1 : t) * tw.a * (additive ? 1 : 0.5);
    const g = ctx.createRadialGradient(x, y, 0, x, y, rb);
    g.addColorStop(0, `rgba(255,255,255,${0.16 * B * alpha})`);
    g.addColorStop(1, 'rgba(255,255,255,0)');
    ctx.save();
    if (additive) ctx.globalCompositeOperation = 'lighter';
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(x, y, rb, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore(); // возврат source-over
}

// drawPetals — деликатные лепестки (B, §5): 6 штук через 60°, чередование
// длин 1.6R/2.6R, ширина у основания 0.18R, тают к концу. Только яркие O/B/A.
function drawPetals(ctx, x, y, R, color, B, b2, sspec) {
    if (!['O', 'B', 'A'].includes(sspec) || B < 1.0) return;
    const baseAngle = b2 * Math.PI * 2;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    for (let i = 0; i < PETAL_COUNT; i++) {
        const a = baseAngle + (i * Math.PI * 2) / PETAL_COUNT;
        const len = (i % 2 === 0 ? 1.6 : 2.6) * R;
        const w = 0.18 * R;
        const ex = x + Math.cos(a) * len;
        const ey = y + Math.sin(a) * len;
        const g = ctx.createLinearGradient(x, y, ex, ey);
        g.addColorStop(0, rgba(color, PETAL_ALPHA * B));
        g.addColorStop(1, rgba(color, 0));
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.moveTo(x + Math.cos(a + Math.PI / 2) * w, y + Math.sin(a + Math.PI / 2) * w);
        ctx.lineTo(ex, ey);
        ctx.lineTo(x + Math.cos(a - Math.PI / 2) * w, y + Math.sin(a - Math.PI / 2) * w);
        ctx.closePath();
        ctx.fill();
    }
    ctx.restore(); // возврат source-over
}

// drawSpriteStar — C (§6): один drawImage запечённого спрайта, разброс —
// альфой (B) и лёгким масштабом, без пересоздания градиентов.
function drawSpriteStar(ctx, c, x, y, R, B, tw, t) {
    const { b2 } = starBits(c.sid || c.sname || '');
    const glow = CLASS_GLOW[c.sspec] || 1.0;
    const rh = Math.min(R * (1.8 + 1.1 * b2) * glow, HALO_MAX_PX);
    const bucket = Math.min(SPRITE_BUCKETS - 1, Math.floor(B / 1.6 * SPRITE_BUCKETS));
    const sprite = spriteFor(c.sspec, bucket);
    const s = rh * 2 * (t >= 1 ? 1 : 0.6 + 0.4 * t);
    ctx.save();
    ctx.globalAlpha = Math.min(1, B * tw.a * (t >= 1 ? 1 : t));
    ctx.drawImage(sprite, x - s / 2, y - s / 2, s, s);
    ctx.restore();
}

// ==================== КОРОНА (D, §7) ====================

// crownCache — ключ "sspec|frame" → offscreen canvas 192×192 (кадр венца).
let crownCache = null;

// crownFrame — запечённый кадр венца класса; создаётся лениво (offscreen-canvas
// только внутри функции, не на верхнем уровне модуля). Кадры не зависят от
// пресета/тумблеров, поэтому кэш при их смене не сбрасывается.
function crownFrame(sspec, frame) {
    if (!crownCache) crownCache = new Map();
    const key = (sspec || 'G') + '|' + frame;
    let cv = crownCache.get(key);
    if (cv) return cv;
    cv = bakeCrown(sspec, frame);
    crownCache.set(key, cv);
    return cv;
}

// drawCrownWreath — венец «Корона» (D, §7): два запечённых кадра на класс,
// вращение rot = b2·2π + 0.03·t (t — секунды) и кроссфейд периодом 3 с
// (m = 0.5 + 0.5·sin(2π·0.33·t + phase)). Альфа венца 0.32·B, режим lighter;
// после слоя режим рисования возвращается (restore) — иначе подписи и маркеры
// смешаются «светом».
function drawCrownWreath(ctx, x, y, R, B, b2, sspec) {
    const glow = CLASS_GLOW[sspec] || 1.0;
    const rh = Math.min(R * (1.8 + 1.1 * b2) * glow, HALO_MAX_PX);
    if (!(rh > 0)) return;
    const tSec = Date.now() / 1000;
    const phase = 2 * Math.PI * b2;
    const m = 0.5 + 0.5 * Math.sin(2 * Math.PI * CROWN_XFADE_HZ * tSec + phase);
    const rot = b2 * Math.PI * 2 + CROWN_SPIN * tSec;
    const alpha = CROWN_ALPHA * B;
    const s = rh * CROWN_DRAW_SCALE;
    const frameA = crownFrame(sspec, 0);
    const frameB = crownFrame(sspec, 1);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.translate(x, y);
    ctx.rotate(rot);
    ctx.globalAlpha = Math.min(1, alpha * (1 - m));
    ctx.drawImage(frameA, -s / 2, -s / 2, s, s);
    ctx.globalAlpha = Math.min(1, alpha * m);
    ctx.drawImage(frameB, -s / 2, -s / 2, s, s);
    ctx.restore(); // возврат source-over и трансформа
}

// bakeCrown — запекает кадр венца (§7): асимметричное кольцо «плазмы» с
// «языками» на fBm-шуме (CROWN_NOISE_OCTAVES октав). Кольцевые сэмплы шума
// периодичны по углу (сэмплим на окружности), поэтому венец не имеет шва при
// вращении; у класса/кадра своё смещение базиса — «языки» у звёзд разные.
function bakeCrown(sspec, frame) {
    const cv = document.createElement('canvas');
    cv.width = CROWN_SIZE;
    cv.height = CROWN_SIZE;
    const g = cv.getContext('2d');
    const img = g.createImageData(CROWN_SIZE, CROWN_SIZE);
    const data = img.data;
    const [cr, cg, cb] = parseColor(getStarColor(sspec || 'G', 'star'));
    const half = CROWN_SIZE / 2;
    const ox = (hashString((sspec || 'G') + ':x') % 1000) / 250;
    const oy = (hashString((sspec || 'G') + ':y') % 1000) / 250;
    const fo = frame * 13.7;
    for (let y = 0; y < CROWN_SIZE; y++) {
        const dy = (y + 0.5 - half) / half;
        for (let x = 0; x < CROWN_SIZE; x++) {
            const dx = (x + 0.5 - half) / half;
            const r = Math.hypot(dx, dy);
            const a = Math.atan2(dy, dx);
            const ca = Math.cos(a), sa = Math.sin(a);
            // Крупные «языки» (низкая частота) и их мелкая дрожь (высокая).
            const lobes = fbm(ca * 2.2 + ox + fo, sa * 2.2 + oy, CROWN_NOISE_OCTAVES);
            const fine = fbm(ca * 5.0 + ox + fo * 1.7, sa * 5.0 + oy + 3.3, CROWN_NOISE_OCTAVES);
            const ringR = CROWN_RING_R + 0.14 * (lobes - 0.5) * 2;
            const thick = 0.08 + 0.12 * fine;
            const tr = (r - ringR) / thick;
            let aa = Math.exp(-tr * tr);
            // Слабое внутреннее свечение плазмы + мягкий край (не обрезать).
            aa += 0.10 * Math.exp(-Math.pow((r - CROWN_RING_R * 0.6) / 0.30, 2));
            const fade = Math.min(1, Math.max(0, (1.15 - r) / 0.25)) * Math.min(1, r / 0.18);
            aa = Math.min(1, Math.max(0, aa * fade));
            const i = (y * CROWN_SIZE + x) * 4;
            data[i] = cr;
            data[i + 1] = cg;
            data[i + 2] = cb;
            data[i + 3] = Math.round(aa * 255);
        }
    }
    g.putImageData(img, 0, 0);
    return cv;
}

// fbm — сумма октав value-шума (§7, 4–6 октав), нормированная в 0..1.
function fbm(x, y, octaves) {
    let sum = 0, amp = 0.5, f = 1, norm = 0;
    for (let i = 0; i < octaves; i++) {
        sum += amp * valueNoise2(x * f, y * f);
        norm += amp;
        amp *= 0.5;
        f *= 2;
    }
    return sum / norm;
}

// valueNoise2 — детерминированный 2D value-шум (гладкая интерполяция
// smoothstep). Общий rand не используется: результат не зависит от потока.
function valueNoise2(x, y) {
    const xi = Math.floor(x), yi = Math.floor(y);
    const xf = x - xi, yf = y - yi;
    const u = xf * xf * (3 - 2 * xf);
    const v = yf * yf * (3 - 2 * yf);
    const n00 = noiseVal(xi, yi), n10 = noiseVal(xi + 1, yi);
    const n01 = noiseVal(xi, yi + 1), n11 = noiseVal(xi + 1, yi + 1);
    return (n00 * (1 - u) + n10 * u) * (1 - v) + (n01 * (1 - u) + n11 * u) * v;
}

// noiseVal — псевдослучайное 0..1 от целых координат (целочисленный хеш).
function noiseVal(ix, iy) {
    let h = (ix * 374761393 + iy * 668265263) | 0;
    h = ((h ^ (h >>> 13)) * 1274126177) | 0;
    h = h ^ (h >>> 16);
    return (h >>> 0) / 4294967296;
}

// ==================== ЭКЗОТИКА (§8) ====================

// drawExotic — спецэффекты экзотических объектов: ЧД (тень + кольцо-
// аккреция), нейтронная (яркое ядро с ускоренным мерцанием), белый карлик
// (ровная крошечная точка), протозвезда (тёплое облако).
function drawExotic(ctx, c, x, y, R, stype, o) {
    const { b2, b3 } = starBits(c.sid || c.sname || '');
    const now = Date.now();
    const t = igniteProgress(now);
    const grow = t >= 1 ? 1 : 0.4 + 0.6 * (1 - Math.pow(1 - t, 3));

    if (stype === 'black_hole') {
        // Тень — диск 0.6R цвета #05030f.
        ctx.save();
        ctx.globalAlpha = Math.min(1, grow);
        ctx.fillStyle = '#05030f';
        ctx.beginPath();
        ctx.arc(x, y, 0.6 * R * grow, 0, Math.PI * 2);
        ctx.fill();
        ctx.restore();
        // Аккреционное кольцо — эллипс rx 1.9R, ry 0.62R, поворот от b2,
        // двумя дугами (передняя ярче).
        const rx = 1.9 * R * grow;
        const ry = 0.62 * R * grow;
        if (!(rx > 0)) return;
        ctx.save();
        ctx.translate(x, y);
        ctx.rotate(b2 * Math.PI);
        const g = ctx.createLinearGradient(-rx, 0, rx, 0);
        g.addColorStop(0, 'rgba(255,170,90,0)');
        g.addColorStop(0.5, 'rgba(255,170,90,0.55)');
        g.addColorStop(1, 'rgba(180,90,255,0.25)');
        ctx.strokeStyle = g;
        ctx.lineWidth = Math.max(1.5, 0.22 * R);
        ctx.beginPath();
        ctx.ellipse(0, 0, rx, ry, 0, 0, Math.PI); // передняя дуга
        ctx.stroke();
        ctx.globalAlpha = 0.5;
        ctx.beginPath();
        ctx.ellipse(0, 0, rx, ry, 0, Math.PI, Math.PI * 2); // задняя дуга
        ctx.stroke();
        ctx.restore();
        return;
    }

    if (stype === 'neutron') {
        // Ядро 0.35R #eaf6ff, ореол 1.2R alpha 0.5, мерцание ×2.5 по частоте
        // и ×1.5 по амплитуде (дрожит быстрее всех). Тумблер «Мерцание звёзд»
        // гасит и его (o.twinkle === false).
        const tw = o.twinkle === false ? { a: 1 } : starTwinkle(b2, b3, now / 1000);
        const fast = 1 + 1.5 * (tw.a - 1) * 2.5;
        const rh = 1.2 * R * grow;
        const g = ctx.createRadialGradient(x, y, 0, x, y, rh);
        g.addColorStop(0, `rgba(234,246,255,${0.5 * fast})`);
        g.addColorStop(1, 'rgba(234,246,255,0)');
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.arc(x, y, rh, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = '#eaf6ff';
        ctx.beginPath();
        ctx.arc(x, y, 0.35 * R * grow, 0, Math.PI * 2);
        ctx.fill();
        // Два конуса-луча — только в пресете «Фото» (§8).
        if (o.petals) drawNeutronBeams(ctx, x, y, R, b2);
        return;
    }

    if (stype === 'white_dwarf') {
        // Ядро 0.4R, ореол 1.4R alpha 0.3, B = 0.6, БЕЗ мерцания — ровный
        // мёртвый белый свет.
        const rh = 1.4 * R * grow;
        const g = ctx.createRadialGradient(x, y, 0, x, y, rh);
        g.addColorStop(0, 'rgba(240,240,240,0.3)');
        g.addColorStop(1, 'rgba(240,240,240,0)');
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.arc(x, y, rh, 0, Math.PI * 2);
        ctx.fill();
        ctx.save();
        ctx.globalAlpha = 0.6;
        ctx.fillStyle = '#f0f0f0';
        ctx.beginPath();
        ctx.arc(x, y, 0.4 * R * grow, 0, Math.PI * 2);
        ctx.fill();
        ctx.restore();
        return;
    }

    // protostar — тёплое облако-кокон: ядро 0.5R alpha 0.7 #ffb37a, ореол
    // 3.6R из двух смещённых центров (x ± 0.4R) alpha 0.18 #ff7950,
    // «дыхание» 0.05 Гц, амплитуда 0.15.
    const breath = 1 + 0.15 * Math.sin(2 * Math.PI * 0.05 * now / 1000 + b2 * Math.PI * 2);
    const rh = 3.6 * R * grow * breath;
    for (const dx of [-0.4 * R, 0.4 * R]) {
        const g = ctx.createRadialGradient(x + dx, y, 0, x + dx, y, rh);
        g.addColorStop(0, 'rgba(255,121,80,0.18)');
        g.addColorStop(1, 'rgba(255,121,80,0)');
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.arc(x + dx, y, rh, 0, Math.PI * 2);
        ctx.fill();
    }
    ctx.save();
    ctx.globalAlpha = 0.7;
    ctx.fillStyle = '#ffb37a';
    ctx.beginPath();
    ctx.arc(x, y, 0.5 * R * grow, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// drawNeutronBeams — два конуса-луча пульсара (длина 3.5R, угол ±7°,
// alpha 0.18) — только в пресете «Фото».
function drawNeutronBeams(ctx, x, y, R, b2) {
    const len = 3.5 * R;
    const spread = 7 * Math.PI / 180;
    const base = b2 * Math.PI * 2;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    for (const dir of [base, base + Math.PI]) {
        const g = ctx.createLinearGradient(x, y, x + Math.cos(dir) * len, y + Math.sin(dir) * len);
        g.addColorStop(0, 'rgba(234,246,255,0.18)');
        g.addColorStop(1, 'rgba(234,246,255,0)');
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.moveTo(x, y);
        ctx.lineTo(x + Math.cos(dir - spread) * len, y + Math.sin(dir - spread) * len);
        ctx.lineTo(x + Math.cos(dir + spread) * len, y + Math.sin(dir + spread) * len);
        ctx.closePath();
        ctx.fill();
    }
    ctx.restore();
}

// ==================== ПОДПИСИ (§3, читаемость) ====================

// drawStarName — подпись звезды: цвет #b6c2d2 с дешёвой тёмной подложкой
// (strokeText → fillText). Размер/якорь/пороги не меняются — их считает
// map_render.js и передаёт сюда.
export function drawStarName(ctx, text, x, y, fontSize, alpha) {
    if (!(alpha > 0)) return;
    ctx.save();
    ctx.globalAlpha = Math.min(1, alpha);
    ctx.font = `${fontSize}px system-ui`;
    ctx.textAlign = 'center';
    ctx.lineWidth = 2.5;
    ctx.strokeStyle = 'rgba(2,6,23,0.85)';
    ctx.strokeText(text, x, y);
    ctx.fillStyle = '#b6c2d2';
    ctx.fillText(text, x, y);
    ctx.restore();
}

// ==================== НЕПРЕРЫВНЫЙ КАДР (один хозяин) ====================

// starAnimFrame — id активного rAF-цикла (0 — нет). Один хозяин кадра:
// цикл поднимается только когда есть что анимировать (мерцание/зажигание),
// и не дублирует полёт игрока (animation.js) и циклы NPC/Пакмана.
let starAnimFrame = 0;

// starNeedsAnim — нужен ли непрерывный кадр: зажигание идёт ИЛИ анимируется
// венец «Короны» ИЛИ мерцание разрешено (мало одиночных звёзд), включено
// тумблером и вкладка видима.
export function starNeedsAnim() {
    if (typeof document !== 'undefined' && document.hidden) return false;
    if (igniteActive(Date.now())) return true;
    if ((state.starSingles || 0) <= 0) return false;
    const o = starVisualOptions();
    // «Корона» (D): венец вращается и «кипит» — кадр нужен независимо от
    // тумблера мерцания, но с тем же авто-стопом (singles > 400 — венец
    // статичен) и не в режиме «Для слабых ПК» (там mode='sprite', crown=false).
    if (o.crown) return twinkleAllowed();
    if (!twinkleAllowed()) return false;
    // Тумблер «Мерцание звёзд» / режим «Для слабых ПК»: без мерцания кадр ради
    // него не поднимается (главная экономия слабого ПК, §4 дизайна).
    if (!o.twinkle) return false;
    return true;
}

// ensureStarAnimLoop — поднять цикл перерисовки, если есть что анимировать.
// Вызывается из draw() (map_render.js) после отрисовки звёзд.
export function ensureStarAnimLoop() {
    if (starAnimFrame) return;
    if (!starNeedsAnim()) return;
    starAnimFrame = requestAnimationFrame(starAnimTick);
}

// starAnimTick — кадр мерцания/зажигания. Пропускаем кадр при полёте игрока
// (animation.js уже перерисовывает карту каждый кадр), при активном Пакмане
// (pacman.js) и при активной интерполяции NPC (npc_agents.js) — у кадра один
// хозяин, двойной полной перерисовки нет (решение создателя 2026-09-22 п.3).
// state.npcAnimActive — общий флаг цикла NPC (без импорта npc_agents.js:
// избегаем циклического импорта с модулем рендера).
function starAnimTick() {
    if (!starNeedsAnim()) {
        starAnimFrame = 0;
        return;
    }
    if (!state.isFlying && !(state.pacman && state.pacman.active) && !state.npcAnimActive) {
        requestRedraw();
    }
    starAnimFrame = requestAnimationFrame(starAnimTick);
}

// ==================== ЗАЖИГАНИЕ: ТРИГГЕР ====================

// updateIgniteTrigger — вызывается из draw() (map_render.js) каждый кадр:
// ловит пересечение порога появления названий снизу вверх (и один раз после
// загрузки, если scale уже выше). Запускает зажигание и поднимает кадр.
export function updateIgniteTrigger(scale, threshold) {
    const o = starVisualOptions();
    if (!o.ignite) {
        igniteArmed = false;
        return;
    }
    const above = scale > threshold;
    if (!igniteArmed) {
        igniteArmed = true;
        if (above) igniteStart = Date.now();
        return;
    }
    if (above && !igniteStart) {
        igniteStart = Date.now();
    }
}

// ==================== УТИЛИТЫ ====================

// rgba — '#aabbcc' | 'hsl(...)' → 'rgba(r,g,b,a)'. Для hsl-строк (getStarShade
// может вернуть hsl) используем canvas-нормализацию через кэш.
const rgbCache = new Map();

function rgba(color, alpha) {
    const key = String(color || '');
    let rgb = rgbCache.get(key);
    if (!rgb) {
        rgb = parseColor(key);
        rgbCache.set(key, rgb);
    }
    return `rgba(${rgb[0]},${rgb[1]},${rgb[2]},${alpha})`;
}

// parseColor — hex или hsl → [r,g,b]. hsl парсится вручную (без DOM).
function parseColor(color) {
    const s = String(color || '').trim();
    if (s.startsWith('#')) {
        let h = s.slice(1);
        if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
        const n = parseInt(h, 16);
        if (!isNaN(n)) return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
    }
    const m = s.match(/hsl\(\s*([\d.]+)\s*,\s*([\d.]+)%\s*,\s*([\d.]+)%\s*\)/i);
    if (m) return hslToRgb(parseFloat(m[1]), parseFloat(m[2]) / 100, parseFloat(m[3]) / 100);
    return [124, 108, 255];
}

// hslToRgb — h в градусах, s/l в 0..1 → [r,g,b] 0..255.
function hslToRgb(h, s, l) {
    const c = (1 - Math.abs(2 * l - 1)) * s;
    const hp = ((h % 360) + 360) % 360 / 60;
    const x = c * (1 - Math.abs((hp % 2) - 1));
    let r = 0, g = 0, b = 0;
    if (hp < 1) { r = c; g = x; }
    else if (hp < 2) { r = x; g = c; }
    else if (hp < 3) { g = c; b = x; }
    else if (hp < 4) { g = x; b = c; }
    else if (hp < 5) { r = x; b = c; }
    else { r = c; b = x; }
    const m = l - c / 2;
    return [
        Math.round((r + m) * 255),
        Math.round((g + m) * 255),
        Math.round((b + m) * 255),
    ];
}
