// web/static/js/surface/surface_weather.js
// Погода прогулки (спека 2026-09-22): выбор явления из физики планеты (§4),
// визуал 6 явлений в МИРОВЫХ колонках (§5), три прохода отрисовки между слоями
// мира (§6). Без сервера и без урона. Единственный хеш — hash1 из
// surface_world.js (§5.1 п.2), жребий — цикловой mulberry32 (§4.4).
// Координаты — мировые/экранно-логические ТОГО ЖЕ трансформа, что у слоёв мира
// (§6.5): модуль не применяет ни зум, ни devicePixelRatio повторно.
import { WEATHER_RULES, WEATHER_VISUALS, ZOOM, COLORS, TRIPLE_POINT_K, DIAMOND_DUST_K, GLASS_FIELD_K } from './surface_config.js';
import { hash1, parseHex, mulberry32 } from './surface_world.js';
import { drawLightning, drawLightningCloud, drawGlints, drawPellets, drawPelletGround, drawAsh, drawAshGround, drawGlow, drawFlake, drawBank, drawFacets } from './surface_weather_shapes.js';

const TWO_PI = Math.PI * 2;
const PARALLAX_MID = 0.7;

function mod(a, n) { return ((a % n) + n) % n; }
function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
function lerp(a, b, t) { return a + (b - a) * t; }

function rgbaCss(c, a) { return `rgba(${c.r},${c.g},${c.b},${clamp(a, 0, 1)})`; }

// scaleRgb — масштаб цвета на кадр (дешёвое умножение, градиенты не пересоздаются).
function scaleRgb(c, mul) {
    if (mul === 1) return c;
    return { r: Math.round(c.r * mul), g: Math.round(c.g * mul), b: Math.round(c.b * mul) };
}

// dimAir — копия params с `air.color × lightMul` (§6.4: атмосферные явления темнее
// ночью). Явный `particle`-цвет — не airColor, его не трогаем. Копия — потому что
// параметры запечены на окно, а lightMul меняется на кадр.
function dimAir(p, lightMul) {
    if (lightMul === 1) return p;
    return { ...p, air: { ...p.air, color: scaleRgb(p.air.color, lightMul) } };
}

// ==================== §4 ВЫБОР ЯВЛЕНИЯ ====================
// Пороги полос — ТОЛЬКО из pkg.suit (§4.1): своих констант температуры/
// давления не заводим. Границы включительные (§4.4 п.5).

function tempBand(pkg) {
    const [tMin, tMax] = pkg.suit.temp_comfort_k;
    if (pkg.temperature < tMin) return 'cold';
    if (pkg.temperature > tMax) return 'hot';
    return 'comfort';
}

function liquidMedium(pkg) {
    return (pkg.liquid_medium || '').trim().toLowerCase();
}

function liquidAbsent(pkg) {
    const lm = liquidMedium(pkg);
    return lm === '' || lm === 'нет';
}

// tempBandPhysical — физические полосы N2/N3 (тройные точки §3.4, без перекрытий).
// Вне гейта T < 273.16 полосы нет → M_temp = 1.0 (suit-полосы к ним НЕ применяются).
function tempBandPhysical(pkg) {
    const T = pkg.temperature;
    if (T < TRIPLE_POINT_K.N2) return 'extreme';
    if (T < TRIPLE_POINT_K.CH4) return 'severe';
    if (T < DIAMOND_DUST_K) return 'deep_frost';
    if (T < TRIPLE_POINT_K.H2O) return 'ice';
    return null;
}

// isWaterish — «водяной» вариант N2/N3: вещество вода ИЛИ категория крио/вода (§4.6 п.2).
function isWaterish(pkg) {
    return liquidMedium(pkg) === 'вода' || pkg.biome_category === 'крио' || pkg.biome_category === 'вода';
}

// allowedWeathers — жёсткие гейты §4.2. Сияние — только полоса комфорта И
// булева метка pkg.radioactive (решения создателя 2026-09-22 №2–3, §4.5).
// Новые явления требуют p ≥ 0.5 (выполняется вне тонкой полосы — она возвращается
// раньше), поэтому в тонкой полосе их нет по построению.
function allowedWeathers(pkg) {
    const p = pkg.pressure_atm;
    const [pMin, pMax] = pkg.suit.pressure_comfort_atm;
    if (p < pMin) return ['штиль', 'пыльная буря'];
    const list = ['штиль', 'туман', 'пыльная буря', 'метель', 'кислотный дождь'];
    const T = pkg.temperature;
    const cat = pkg.biome_category;
    if (T >= TRIPLE_POINT_K.H2O) list.push('гроза');
    if (T < TRIPLE_POINT_K.H2O) {
        list.push('ледяные иглы');
        if (!liquidAbsent(pkg) || cat === 'крио' || cat === 'вода') list.push('ледяной град');
    }
    if (cat === 'вулканизм') list.push('пепельный дождь');
    if (pkg.life && (cat === 'биосфера' || cat === 'вода')) list.push('свечение');
    if (p <= pMax && pkg.radioactive) list.push('полярное сияние');
    return list;
}

// weightFor — мягкий вес §4.3: W = base × M_temp × M_rad × M_tox × M_liq × M_biome.
// M_temp — suit-полосы (N1/N4/N5) ИЛИ физические полосы (N2/N3); незаданный = 1.0.
function weightFor(pkg, id) {
    const rules = WEATHER_RULES;
    let w = (rules.base[id] ?? 1) * (rules.byTemp[tempBand(pkg)]?.[id] ?? 1);
    const phys = tempBandPhysical(pkg);
    if (phys) w *= rules.byTempPhysical[phys]?.[id] ?? 1;
    if (pkg.toxic) w *= rules.byToxic[id] ?? 1;
    if (liquidAbsent(pkg)) w *= rules.byLiquidAbsent[id] ?? 1;
    else w *= rules.byLiquidPresent[id] ?? 1;
    w *= rules.byBiome[pkg.biome_category]?.[id] ?? 1;
    if (id === 'полярное сияние') {
        w *= clamp(1 + (pkg.radioactivity || 0) / rules.aurora.radDivisor, 1, rules.aurora.radMax);
    }
    return w;
}

function weightedPick(ids, weights, rng) {
    let total = 0;
    for (const w of weights) total += w;
    if (total <= 0) return 'штиль';
    let r = rng() * total;
    for (let i = 0; i < ids.length; i++) {
        r -= weights[i];
        if (r < 0) return ids[i];
    }
    return ids[ids.length - 1];
}

// variantFor — вариант внутри явления (§5.3, §4.6). Набор явлений не меняет.
function variantFor(id, pkg) {
    if (id === 'туман') {
        // §4.6: порядок замещает правило пакета 1 (cold → toxic → hot).
        const lm = liquidMedium(pkg);
        if (lm === 'co2') return 'плотный CO₂';
        if (lm === 'метан' || lm === 'аммиак') return 'аммиачно-метановый';
        if (pkg.toxic) return 'серный';
        if (tempBand(pkg) === 'cold') return 'морозный';
        if (tempBand(pkg) === 'hot') return 'марево';
        return null;
    }
    if (id === 'кислотный дождь') {
        // §4.6: радиоактивный → инверсионный (p > suit-max) → кислотный → обычный.
        if (pkg.radioactive) return 'радиоактивный';
        if (pkg.pressure_atm > pkg.suit.pressure_comfort_atm[1]) return 'инверсионный';
        if (pkg.toxic) return 'кислотный';
        return 'обычный';
    }
    if (id === 'пыльная буря') {
        // §4.3: сухая стеклянно-кристаллическая форма при T ≥ GLASS_FIELD_K
        // (каталожные `стеклянные_поля`/`металлические_поля`); иначе базовая.
        if (pkg.temperature >= GLASS_FIELD_K) return 'стеклянно-кристаллическая';
        return null;
    }
    if (id === 'полярное сияние') {
        // §4.3: тот же гейт `radioactive`, что у явления; иначе базовое.
        if (pkg.radioactive) return 'магнитная буря';
        return null;
    }
    // Новые явления: детерминированный список приоритета (первое совпадение),
    // тотален — последний вариант срабатывает всегда при прошедшем гейте (§4.6).
    if (id === 'гроза') {
        if (pkg.toxic) return 'токсичная';
        if (liquidAbsent(pkg)) return 'сухая';
        return 'ливневая';
    }
    if (id === 'ледяные иглы') {
        const lm = liquidMedium(pkg);
        const T = pkg.temperature;
        if (lm === 'co2' && T < TRIPLE_POINT_K.CO2) return 'углекислые';
        if (lm === 'аммиак' && T < TRIPLE_POINT_K.NH3) return 'аммиачные';
        if (lm === 'метан' && T < TRIPLE_POINT_K.CH4) return 'метановые';
        if (isWaterish(pkg) && T < TRIPLE_POINT_K.H2O) return 'водяные';
        return 'иней';
    }
    if (id === 'ледяной град') {
        const lm = liquidMedium(pkg);
        const T = pkg.temperature;
        if (lm === 'co2' && T < TRIPLE_POINT_K.CO2) return 'углекислая крупа';
        if (lm === 'метан' && T < TRIPLE_POINT_K.CH4) return 'метановая крупа';
        if (isWaterish(pkg)) {
            if (T >= DIAMOND_DUST_K && T < TRIPLE_POINT_K.H2O) return 'град';
            if (T < DIAMOND_DUST_K) return 'крупа';
        }
        return 'базовая крупа';
    }
    if (id === 'пепельный дождь') {
        if (pkg.temperature < TRIPLE_POINT_K.NH3) return 'криовулканический';
        if (!liquidAbsent(pkg)) return 'влажный';
        return 'сухой';
    }
    if (id === 'свечение') {
        if (pkg.pressure_atm > pkg.suit.pressure_comfort_atm[1]) return 'светящийся туман';
        if (!liquidAbsent(pkg)) return 'светящийся ливень';
        return 'биоаэрозоль';
    }
    return null;
}

// ==================== §7.2 РАЗВЁРТКА ПАРАМЕТРОВ ====================

function resolveRange(rng, r) {
    return Array.isArray(r) ? lerp(r[0], r[1], rng()) : r;
}

// Новые блоки явлений (lightning/glint/pellet/ash/glow): диапазоны-параметры окна
// разворачиваются цикловым PRNG один раз на окно (§7.2); диапазоны ПО ЭЛЕМЕНТУ
// (размер/альфа/отскок/дрейф/облако) остаются парами и интерполируются в примитиве.
const RESOLVE_ONCE = new Set(['count', 'airCount', 'lowCount', 'parallax', 'boltRate', 'boltBurst', 'segments', 'branchDepth', 'flashMs', 'flashAlpha', 'echoMs', 'echoAlpha', 'groundGlow', 'bounceCount', 'airTint']);
function resolveBlock(rng, v, key) {
    if (Array.isArray(v)) {
        if (RESOLVE_ONCE.has(key) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
            return lerp(v[0], v[1], rng());
        }
        return v.map((x) => resolveBlock(rng, x, key));
    }
    if (v && typeof v === 'object') {
        const out = {};
        for (const k of Object.keys(v)) out[k] = resolveBlock(rng, v[k], k);
        return out;
    }
    return v;
}

// airRgb — цвет воздуха явления: от biome_color (shade на base) к blend-цели.
// Повторного сдвига по liquid_medium/температуре нет — он уже выполнен
// сервером в biome_color (§5.1 п.4).
function airRgb(pkg, airDef) {
    const c = parseHex(pkg.biome_color || '#8a7a6a');
    const f = airDef.base ?? 1;
    let r = c.r * f, g = c.g * f, b = c.b * f;
    if (airDef.blend) {
        const t = parseHex(airDef.blend[0] === 'sky' ? COLORS.skyBottom : airDef.blend[0]);
        const k = clamp(airDef.blend[1], 0, 1);
        r = lerp(r, t.r, k); g = lerp(g, t.g, k); b = lerp(b, t.b, k);
    }
    return { r: clamp(Math.round(r), 0, 255), g: clamp(Math.round(g), 0, 255), b: clamp(Math.round(b), 0, 255) };
}

function buildParams(pkg, id, variant, rng) {
    const def = WEATHER_VISUALS[id];
    const varDef = (variant && def.variants) ? def.variants[variant] : null;
    const airDef = varDef && varDef.air ? { ...def.air, ...varDef.air } : def.air;
    // Пояса варианта: массив частичных переопределений по индексу (как «град»/«крупа»);
    // отсутствующие поля пояса берутся из базового, пустой массив — снять пояса.
    const beltsDef = (varDef && Array.isArray(varDef.belts))
        ? varDef.belts.map((b, i) => ({ ...(def.belts[i] || {}), ...b }))
        : (def.belts || []);
    // Явный `ground: null` у варианта снимает приземный эффект (не откатывается к базе).
    const groundDef = (varDef && 'ground' in varDef) ? varDef.ground : (def.ground || null);
    const alphaMul = (varDef && varDef.alphaMul) || 1;

    const air = {
        color: airRgb(pkg, airDef),
        far: resolveRange(rng, airDef.far),
        mid: resolveRange(rng, airDef.mid),
        near: resolveRange(rng, airDef.near),
        frame: resolveRange(rng, airDef.frame),
        sky: resolveRange(rng, airDef.sky),
    };

    // Туман на категории «вода» рисует слоистую гряду `bank` вместо круглых клочьев
    // `clump` (§5.2). Вариант `bankAll` (§4.3: CO₂/аммиачно-метановый туман) — гряда
    // независимо от категории. Прочие — прежние `clump`.
    const bankOnWater = (varDef && varDef.bankAll) || (!!def.bank && def.bank.watersOnly !== false && pkg.biome_category === 'вода');
    const belts = beltsDef.map((b) => ({
        parallax: b.parallax,
        tile: b.tile,
        vTile: b.vTile || b.tile,
        count: Math.max(1, Math.round(resolveRange(rng, b.count))),
        size: b.size || [1, 1],
        w: b.w || null,
        h: b.h || null,
        alpha: [(b.alpha[0]) * alphaMul, (b.alpha[1]) * alphaMul],
        fall: b.fall || [0, 0],
        sway: b.sway || 0,
        streak: b.streak || null,
        bounce: b.bounce || false,
        shape: (bankOnWater && b.shape === 'clump') ? 'bank' : (b.shape || 'dot'),
    }));

    const wind = {
        base: resolveRange(rng, def.wind.base),
        dir: rng() < 0.5 ? -1 : 1,
        period: resolveRange(rng, def.wind.gust.period),
        range: def.wind.gust.range,
        phase: rng() * TWO_PI,
    };

    const slant = def.slant ? { deg: resolveRange(rng, def.slant.deg), from: def.slant.from } : null;

    // Сияние: диапазоны разворачиваются цикловым PRNG один раз на окно;
    // параметры каждой шторы (глубина/ширина/высота/дрейф/волна) — здесь же,
    // чтобы спрайты запекались детерминированно (§5.7 спеки, направление §2).
    let aurora = null;
    if (def.aurora) {
        // Вариант («магнитная буря») переопределяет curtains/bandHeight/palette.
        const au = (varDef && varDef.aurora) ? { ...def.aurora, ...varDef.aurora } : def.aurora;
        const n = Math.max(1, Math.round(resolveRange(rng, au.curtains)));
        const curtains = [];
        for (let c = 0; c < n; c++) {
            curtains.push({
                x0: (c + 0.5) * (au.tile / n) + (rng() - 0.5) * 240,
                parallax: resolveRange(rng, au.parallax),
                widthFrac: resolveRange(rng, au.widthFrac),
                bandHeight: resolveRange(rng, au.bandHeight),
                strands: Math.max(4, Math.round(resolveRange(rng, au.strands))),
                strandWidth: au.strandWidth,
                strandLen: au.strandLen,
                bottomJitter: au.bottomJitter,
                waveLen: resolveRange(rng, au.bottomWave.len),
                waveAmp: resolveRange(rng, au.bottomWave.amp),
                warp: resolveRange(rng, au.warp),
                sway: resolveRange(rng, au.sway),
                driftPeriod: resolveRange(rng, au.driftPeriod),
                driftAmp: resolveRange(rng, au.driftAmp),
                taper: resolveRange(rng, au.taper),
                alpha: resolveRange(rng, au.alpha),
                haloAlpha: resolveRange(rng, au.haloAlpha),
                period: resolveRange(rng, au.period),
                phase: rng(),
            });
        }
        aurora = {
            tile: au.tile,
            curtains,
            reflection: resolveRange(rng, au.reflection),
            palette: au.palette,
            seed: pkg.seed >>> 0,
        };
    }

    const ground = groundDef ? {
        mode: groundDef.mode,
        offset: groundDef.offset ? resolveRange(rng, groundDef.offset) : 0,
        alpha: groundDef.alpha || [0.25, 0.45],
    } : null;

    const particle = (varDef && varDef.particle !== undefined) ? varDef.particle : (def.particle || null);

    // Новые блоки: слияние базового блока и переопределения варианта (как `air`),
    // затем разворот диапазонов-параметров окна.
    const blocks = {};
    for (const key of ['lightning', 'glint', 'pellet', 'ash', 'glow', 'flake', 'bank', 'facet']) {
        if (!def[key]) continue;
        blocks[key] = resolveBlock(rng, varDef && varDef[key] ? { ...def[key], ...varDef[key] } : def[key], key);
    }

    return {
        id, variant, air, belts, wind, slant, aurora, ground, particle,
        horizon: !!(varDef && varDef.horizon),
        jitter: (varDef && varDef.jitter) || 0,
        // §4.3: аддитивный блик всплеска дождя (радиоактивный вариант) — 0, если нет.
        splashGlow: (varDef && varDef.splashGlow) ? resolveRange(rng, varDef.splashGlow) : 0,
        biomeColor: pkg.biome_color || '#8a7a6a',
        ...blocks,
    };
}

// pickWeatherRun — один цикловой жребий (§4.4): id + вариант + развёрнутые
// параметры + доля окна. forceId — админский выбор (§8): без жребия.
export function pickWeatherRun(pkg, cycle, prevId = null, forceId = null) {
    const rng = mulberry32(((pkg.seed >>> 0) ^ 0x5eed ^ Math.imul(cycle, 0x9e3779b1)) >>> 0);
    let id = forceId;
    if (!id) {
        const allowed = allowedWeathers(pkg);
        id = weightedPick(allowed, allowed.map((x) => weightFor(pkg, x)), rng);
        // Анти-повтор (§4.4 п.4): ≥2 разрешённых и выпало то же — жребий
        // повторяется среди разрешённых без прошлого явления.
        if (WEATHER_RULES.repeatGuard && allowed.length >= 2 && id === prevId) {
            const rest = allowed.filter((x) => x !== prevId);
            id = weightedPick(rest, rest.map((x) => weightFor(pkg, x)), rng);
        }
    }
    const variant = variantFor(id, pkg);
    return { id, variant, params: buildParams(pkg, id, variant, rng), windowFrac: rng() };
}

// weatherLabel — подпись HUD (§5.3): вариант уточняет базовое имя; неизвестный
// id → сам id (инвариант §8 п.3 «не пусто»). Формулировки — @uidesigner (UI §5).
export function weatherLabel(run) {
    if (!run) return '';
    const def = WEATHER_VISUALS[run.id];
    if (!def) return run.id || '';
    const vl = def.variantLabels;
    if (vl && run.variant && vl[run.variant] !== undefined) return vl[run.variant];
    return def.label || run.id;
}

// ==================== ОБЩИЕ ХЕЛПЕРЫ ОТРИСОВКИ ====================

// viewWindow — видимое окно в экранно-логических px того же трансформа, что у
// слоёв мира (зум сужает видимую область, §6.5). Модуль зум НЕ применяет.
function viewWindow(vw, vh) {
    const hw = vw / (2 * ZOOM);
    const hh = vh / (2 * ZOOM);
    return { x0: vw / 2 - hw, x1: vw / 2 + hw, y0: vh / 2 - hh, y1: vh / 2 + hh };
}

// windOffset — снос windX(t) = базовая скорость × модулятор g(t) (§5.1 п.5):
// интеграл g по времени, направление — цикловой PRNG.
function windOffset(p, tSec) {
    const { base, dir, period, range, phase } = p.wind;
    if (!base) return 0;
    const w = TWO_PI / Math.max(0.001, period);
    const A = range[0] + (range[1] - range[0]) * 0.5;
    const B = (range[1] - range[0]) * 0.5 / w;
    return dir * base * (A * tSec + B * (Math.cos(phase) - Math.cos(w * tSec + phase)));
}

function horizonY(world, camera, vh) {
    return world.farHeight(camera.x * 0.35) - camera.y * 0.35 + vh * 0.35;
}

function groundY(world, wx, camera, vh) {
    // Якорь приземных эффектов — `skyTop` (§5 п.12): «поверхность неба» (низ
    // плиты/вал/верх свода) — осадки ложатся на кровлю, внутрь полости не сыплются.
    return world.skyTop(wx) - camera.y + vh / 2;
}

function slantDir(p) {
    if (!p.slant) return { dx: 0, dy: 1 };
    const t = Math.tan(p.slant.deg * Math.PI / 180);
    return p.slant.from === 'horizontal' ? { dx: 1, dy: t } : { dx: t, dy: 1 };
}

// particleRgb — цвет частиц: явный из конфига, иначе производный от airColor.
function particleRgb(p) {
    return p.particle ? parseHex(p.particle) : p.air.color;
}

function fillAir(ctx, p, alpha, vw, vh) {
    if (alpha <= 0) return;
    ctx.fillStyle = rgbaCss(p.air.color, alpha);
    ctx.fillRect(0, 0, vw, vh);
}

// ==================== ЧАСТИЦЫ (§5.1 п.2: мировые колонки + модульная обёртка) ====================
// Никакого перебора экранного шага: u_i фиксирована в мире, обёртка по тайлу
// даёт копии — частица не «перескакивает» между кадрами (корень §2.2 идеи).

function drawDots(ctx, world, camera, p, belt, beltIndex, tSec, win) {
    const S = belt.tile, V = belt.vTile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const x0 = win.x0 - 40, x1 = win.x1 + 40, y0 = win.y0 - 40, y1 = win.y1 + 40;
    const salt = 0x51a7 + beltIndex * 0x1f3b;
    const col = particleRgb(p);
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (world.seed ^ salt) >>> 0);
        const hy = hash1(i, (world.seed ^ (salt ^ 0x7777)) >>> 0);
        const hp = hash1(i, (world.seed ^ (salt ^ 0x1234)) >>> 0);
        const hf = hash1(i, (world.seed ^ (salt ^ 0x55)) >>> 0);
        const fall = lerp(belt.fall[0], belt.fall[1], hf);
        const size = lerp(belt.size[0], belt.size[1], hp);
        const alpha = lerp(belt.alpha[0], belt.alpha[1], hp);
        const sx0 = mod(hx * S - camera.x * par + wind, S);
        const sy0 = mod(hy * V - camera.y * par + fall * tSec, V);
        const phase = hash1(i, (world.seed ^ (salt ^ 0x9abc)) >>> 0) * TWO_PI;
        const sway = belt.sway ? Math.sin(tSec * 1.3 + phase) * belt.sway : 0;
        ctx.fillStyle = rgbaCss(col, alpha);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S + sway;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                ctx.fillRect(sx, sy0 + ky * V, size, size);
            }
        }
    }
}

function drawStreaks(ctx, world, camera, p, belt, beltIndex, tSec, win) {
    const S = belt.tile, V = belt.vTile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const x0 = win.x0 - 60, x1 = win.x1 + 60, y0 = win.y0 - 60, y1 = win.y1 + 60;
    const salt = 0x51a7 + beltIndex * 0x1f3b;
    const dir = p.wind.dir;
    const s = slantDir(p);
    const col = particleRgb(p);
    ctx.lineWidth = Math.max(1, belt.size[0]);
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (world.seed ^ salt) >>> 0);
        const hy = hash1(i, (world.seed ^ (salt ^ 0x7777)) >>> 0);
        const hp = hash1(i, (world.seed ^ (salt ^ 0x1234)) >>> 0);
        const hf = hash1(i, (world.seed ^ (salt ^ 0x55)) >>> 0);
        const fall = lerp(belt.fall[0], belt.fall[1], hf);
        const alpha = lerp(belt.alpha[0], belt.alpha[1], hp);
        const len = belt.streak
            ? lerp(belt.streak[0], belt.streak[1], hp)
            : Math.max(3, lerp(belt.size[0], belt.size[1], hp) * 2.5);
        const sx0 = mod(hx * S - camera.x * par + wind, S);
        const sy0 = mod(hy * V - camera.y * par + fall * tSec, V);
        ctx.strokeStyle = rgbaCss(col, alpha);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S;
            for (let ky = Math.ceil((y0 - sy0) / V); ky <= Math.floor((y1 - sy0) / V); ky++) {
                const sy = sy0 + ky * V;
                ctx.beginPath();
                ctx.moveTo(sx - dir * s.dx * len, sy - s.dy * len);
                ctx.lineTo(sx, sy);
                ctx.stroke();
            }
        }
    }
}

// drawClumps — рваные клочья тумана: привязка к «поверхности неба» skyTop (§5.1
// п.6/§5 п.12), в горах выше (смещение к склону), марево — к линии горизонта.
function drawClumps(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec) {
    const S = belt.tile, par = belt.parallax;
    const wind = windOffset(p, tSec);
    const win = viewWindow(vw, vh);
    const x0 = win.x0 - 80, x1 = win.x1 + 80;
    const salt = 0x2b9f + beltIndex * 0x1f3b;
    const col = p.air.color;
    const groundOff = p.ground ? p.ground.offset : 0;
    const hy = p.horizon ? horizonY(world, camera, vh) : 0;
    for (let i = 0; i < belt.count; i++) {
        const hx = hash1(i, (world.seed ^ salt) >>> 0);
        const hp = hash1(i, (world.seed ^ (salt ^ 0x1234)) >>> 0);
        const hg = hash1(i, (world.seed ^ (salt ^ 0x0f0f)) >>> 0);
        const w = lerp(belt.w[0], belt.w[1], hp);
        const h = lerp(belt.h[0], belt.h[1], hp);
        const alpha = lerp(belt.alpha[0], belt.alpha[1], hp);
        const sx0 = mod(hx * S - camera.x * par + wind, S);
        for (let kx = Math.ceil((x0 - sx0) / S); kx <= Math.floor((x1 - sx0) / S); kx++) {
            const sx = sx0 + kx * S;
            let cy;
            if (p.horizon) cy = hy + (hg - 0.5) * 2 * (p.jitter || 2);
            else cy = world.skyTop(sx - vw / 2 + camera.x) + groundOff + (hg - 0.5) * 20;
            drawClump(ctx, sx, cy, w, h, col, alpha);
        }
    }
}

// drawClump — мягкий клочок: радиальный градиент, центр смещён вверх (рваный
// верхний край), эллипс через масштаб контекста.
function drawClump(ctx, x, y, w, h, col, alpha) {
    if (w <= 0 || h <= 0 || alpha <= 0) return;
    ctx.save();
    ctx.translate(x, y);
    ctx.scale(1, h / w);
    const g = ctx.createRadialGradient(0, -w * 0.12, 0, 0, 0, w / 2);
    g.addColorStop(0, rgbaCss(col, alpha));
    g.addColorStop(0.55, rgbaCss(col, alpha * 0.55));
    g.addColorStop(1, rgbaCss(col, 0));
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(0, 0, w / 2, 0, TWO_PI);
    ctx.fill();
    ctx.restore();
}

function drawBelt(ctx, world, camera, p, belt, beltIndex, tSec, vw, vh) {
    const win = viewWindow(vw, vh);
    if (belt.shape === 'clump') drawClumps(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec);
    else if (belt.shape === 'flake') drawFlake(ctx, world, camera, p, belt, beltIndex, tSec, win);
    else if (belt.shape === 'bank') drawBank(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec);
    else if (belt.shape === 'pellet') drawPellets(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec, win);
    else if (belt.shape === 'ash') drawAsh(ctx, world, camera, vw, vh, p, belt, beltIndex, tSec, win);
    else if (belt.shape === 'facet') drawFacets(ctx, world, camera, p, belt, beltIndex, tSec, win);
    else if (belt.shape === 'streak' || belt.shape === 'pair') drawStreaks(ctx, world, camera, p, belt, beltIndex, tSec, win);
    else drawDots(ctx, world, camera, p, belt, beltIndex, tSec, win);
}

// ==================== ПРИЗЕМНЫЕ ЭФФЕКТЫ (§5.1 п.6, §6.3) ====================
// Всё считается по world.skyTop(wx) — экранный Y для земли запрещён, а якорь —
// «поверхность неба» (§5 п.12): на биомах со сводом осадки ложатся на кровлю.

function snowGround(ctx, world, camera, vw, vh, p, tSec) {
    const win = viewWindow(vw, vh);
    const dir = p.wind.dir;
    const col = particleRgb(p);
    const step = 16;
    const alpha = lerp(p.ground.alpha[0], p.ground.alpha[1], 0.5);
    // Позёмка — низкие штрихи вдоль «поверхности неба» (skyTop, §5 п.12).
    ctx.strokeStyle = rgbaCss(col, alpha);
    ctx.lineWidth = 2;
    ctx.beginPath();
    for (let sx = win.x0; sx <= win.x1; sx += step) {
        const wx = sx - vw / 2 + camera.x;
        const h = hash1(Math.floor(wx / step), (world.seed ^ 0x50a1) >>> 0);
        if (h < 0.45) continue;
        const gy = groundY(world, wx, camera, vh);
        const len = 14 + h * 34;
        ctx.moveTo(sx - dir * len, gy - 2);
        ctx.lineTo(sx, gy - 2 - Math.sin(tSec * 2 + h * 6) * 2);
    }
    ctx.stroke();
    // Заносы — белые «шапки» на локальных максимумах рельефа.
    ctx.fillStyle = rgbaCss(col, alpha * 0.8);
    for (let sx = win.x0; sx <= win.x1; sx += 24) {
        const wx = sx - vw / 2 + camera.x;
        const th0 = world.skyTop(wx - 10), th1 = world.skyTop(wx), th2 = world.skyTop(wx + 10);
        if (th1 <= th0 && th1 <= th2) {
            const gy = th1 - camera.y + vh / 2;
            ctx.beginPath();
            ctx.ellipse(sx, gy + 1, 16, 5, 0, 0, TWO_PI);
            ctx.fill();
        }
    }
    // Вихри — короткие спиральные штрихи у земли.
    ctx.strokeStyle = rgbaCss(col, alpha);
    ctx.lineWidth = 1.5;
    for (let sx = win.x0; sx <= win.x1; sx += 40) {
        const wx = sx - vw / 2 + camera.x;
        const h = hash1(Math.floor(wx / 40), (world.seed ^ 0x77b1) >>> 0);
        if (h < 0.9) continue;
        const gy = groundY(world, wx, camera, vh);
        ctx.beginPath();
        for (let k = 0; k <= 6; k++) {
            const yy = gy - k * 4;
            const xx = sx + Math.sin(k * 0.9 + tSec * 3) * (3 + k * 0.8) * dir;
            if (k === 0) ctx.moveTo(xx, yy); else ctx.lineTo(xx, yy);
        }
        ctx.stroke();
    }
}

function dustGround(ctx, world, camera, vw, vh, p, tSec) {
    const win = viewWindow(vw, vh);
    const col = particleRgb(p);
    const alpha = lerp(p.ground.alpha[0], p.ground.alpha[1], 0.5);
    // «Юбка» песка — плотная полоса вдоль skyTop (§5 п.12).
    ctx.strokeStyle = rgbaCss(col, alpha);
    ctx.lineWidth = 22;
    ctx.lineCap = 'round';
    ctx.beginPath();
    let first = true;
    for (let sx = win.x0; sx <= win.x1; sx += 6) {
        const wx = sx - vw / 2 + camera.x;
        const gy = groundY(world, wx, camera, vh);
        if (first) { ctx.moveTo(sx, gy); first = false; } else ctx.lineTo(sx, gy);
    }
    ctx.stroke();
    ctx.lineCap = 'butt';
    // Вихрики-смерчи у формаций.
    ctx.lineWidth = 2;
    for (let sx = win.x0; sx <= win.x1; sx += 48) {
        const wx = sx - vw / 2 + camera.x;
        const h = hash1(Math.floor(wx / 48), (world.seed ^ 0x3cd1) >>> 0);
        if (h < 0.9) continue;
        const gy = groundY(world, wx, camera, vh);
        ctx.strokeStyle = rgbaCss(col, alpha * 0.8);
        ctx.beginPath();
        for (let k = 0; k <= 8; k++) {
            const yy = gy - k * 6;
            const xx = sx + Math.sin(k * 0.8 + tSec * 2.5) * (2 + k * 1.2) * p.wind.dir;
            if (k === 0) ctx.moveTo(xx, yy); else ctx.lineTo(xx, yy);
        }
        ctx.stroke();
    }
}

function rainGround(ctx, world, camera, vw, vh, p, tSec) {
    const win = viewWindow(vw, vh);
    const col = particleRgb(p);
    ctx.lineWidth = 1.5;
    for (let sx = win.x0; sx <= win.x1; sx += 22) {
        const wx = sx - vw / 2 + camera.x;
        const h = hash1(Math.floor(wx / 22), (world.seed ^ 0x51f0) >>> 0);
        const gy = groundY(world, wx, camera, vh);
        // Всплеск — расходящееся кольцо у земли (жизнь ~0.1 с).
        const rip = mod(tSec / 0.1 + h * 7, 1);
        const radius = lerp(3, 10, rip);
        ctx.strokeStyle = rgbaCss(col, (1 - rip) * p.ground.alpha[1]);
        ctx.beginPath();
        ctx.arc(sx, gy, radius, Math.PI, TWO_PI);
        ctx.stroke();
        // Радиоактивный вариант (§4.3): слабый аддитивный блик всплеска.
        if (p.splashGlow > 0) {
            ctx.save();
            ctx.globalCompositeOperation = 'lighter';
            ctx.fillStyle = rgbaCss(col, (1 - rip) * p.splashGlow);
            ctx.beginPath();
            ctx.arc(sx, gy, radius * 1.6, Math.PI, TWO_PI);
            ctx.fill();
            ctx.restore();
        }
        // Пар/дымок — поднимающийся вверх (жизнь 0.4–1.0 с).
        const sp = mod(tSec / 0.7 + h * 3.1, 1);
        ctx.fillStyle = rgbaCss(col, (1 - sp) * 0.28);
        ctx.fillRect(sx + Math.sin(tSec * 2 + h * 9) * 3, gy - sp * 30, 3, 3);
    }
}

function drawGround(ctx, world, camera, vw, vh, p, tSec) {
    if (!p.ground) return;
    if (p.ground.mode === 'snow') snowGround(ctx, world, camera, vw, vh, p, tSec);
    else if (p.ground.mode === 'skirt') dustGround(ctx, world, camera, vw, vh, p, tSec);
    else if (p.ground.mode === 'splash') rainGround(ctx, world, camera, vw, vh, p, tSec);
    else if (p.ground.mode === 'grain') drawPelletGround(ctx, world, camera, vw, vh, p);
    else if (p.ground.mode === 'ash') drawAshGround(ctx, world, camera, vw, vh, p);
    // 'valley'/'low' — клочья уже отрисованы поясами (shape 'clump');
    // 'glint' — искры рисуются передним проходом (примитив glint), не тут.
}

// ==================== СИЯНИЕ (§5.7) ====================

// auroraColors — палитра [низ, …, верх] по biome_category; источник — палитра
// развёрнутых params (вариант «магнитная буря» даёт 3 цвета с серединой #5f7cff).
function auroraColors(pal, category) {
    return (pal && pal[category]) || (pal && pal['литосфера']) || ['#8ee06a', '#e0c860'];
}

// Запекание занавеса (направление §3 п.5): пряди рисуются в offscreen-канвас
// один раз на окно погоды — покадровые createLinearGradient ×~320 при ZOOM 1.8
// и DPR дороги. В кадре — только drawImage с альфой волны.
const AURORA_SALT = 0x5a17;
const AURORA_BANDS = 40; // полос волны на кадр (шаг ~8 px при bandHeight ~320)

function bakeAurora(p, world, vw, vh) {
    const a = p.aurora;
    const key = vw + 'x' + vh;
    if (a._baked && a._baked.key === key) return a._baked;
    const cols = auroraColors(a.palette, world.category);
    const low = parseHex(cols[0]);
    const high = parseHex(cols[cols.length - 1]);
    // Середина полосы — 3-й цвет («магнитная буря»); у двухцветной палитры
    // середина = верх, старый вид не меняется.
    const mid = parseHex(cols.length > 2 ? cols[1] : cols[cols.length - 1]);
    const seed = a.seed >>> 0;
    const curtains = a.curtains.map((c, ci) => {
        const width = Math.max(120, Math.round(c.widthFrac * vw));
        const bandH = c.bandHeight * vh;
        const spriteH = Math.round(bandH + c.waveAmp + c.bottomJitter[1] + 40);
        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = spriteH;
        const g = canvas.getContext('2d');
        g.globalCompositeOperation = 'lighter';
        g.lineCap = 'round';
        const salt = AURORA_SALT + ci * 977;
        // Сплошная тусклая завеса под прядями: пряди сливаются в одно свечение,
        // а не читаются набором линий (§2.1). Нижняя кромка — волнисто-рваная.
        const wN = 28;
        const edgeAt = (x, ph) => spriteH
            - ((0.5 + 0.5 * Math.sin((x / c.waveLen) * TWO_PI + ph)) * c.waveAmp + c.bottomJitter[0]);
        const wash = g.createLinearGradient(0, spriteH, 0, spriteH - bandH * 1.05);
        wash.addColorStop(0, rgbaCss(low, c.haloAlpha * 1.2));
        wash.addColorStop(0.55, rgbaCss(mid, c.haloAlpha * 0.8));
        wash.addColorStop(1, rgbaCss(high, 0));
        g.fillStyle = wash;
        g.globalAlpha = 1;
        g.beginPath();
        g.moveTo(0, spriteH);
        for (let k = 0; k <= wN; k++) g.lineTo((k / wN) * width, edgeAt((k / wN) * width, c.phase));
        for (let k = wN; k >= 0; k--) {
            const x = (k / wN) * width;
            const hh = 0.5 + 0.5 * Math.sin((x / (c.waveLen * 0.55)) * TWO_PI + c.phase * 1.7);
            g.lineTo(x, edgeAt(x, c.phase + 0.9) - bandH * (0.45 + 0.55 * hh));
        }
        g.closePath();
        g.fill();
        // Растворение концов занавеса: края стираются по горизонтали (§2.1).
        g.globalCompositeOperation = 'destination-out';
        const eg = g.createLinearGradient(0, 0, width, 0);
        const t = Math.max(0.02, c.taper);
        eg.addColorStop(0, 'rgba(0,0,0,1)');
        eg.addColorStop(t, 'rgba(0,0,0,0)');
        eg.addColorStop(1 - t, 'rgba(0,0,0,0)');
        eg.addColorStop(1, 'rgba(0,0,0,1)');
        g.fillStyle = eg;
        g.fillRect(0, 0, width, spriteH);
        g.globalCompositeOperation = 'lighter';
        for (let j = 0; j < c.strands; j++) {
            const h1 = hash1(j, (seed ^ salt) >>> 0);
            const h2 = hash1(j, (seed ^ (salt + 131)) >>> 0);
            const h3 = hash1(j, (seed ^ (salt + 313)) >>> 0);
            const h4 = hash1(j, (seed ^ (salt + 617)) >>> 0);
            // Позиции прядей — низкодискрепансная последовательность (золотое
            // сечение) + шум: регулярного шага между линиями не остаётся (§5.1).
            const sx = mod(j * 0.6180339887 + h1 * 0.06, 1) * width;
            // Растворение концов занавеса (§2.1): у краёв яркость сходит на нет.
            const edge = Math.min(sx, width - sx) / Math.max(1, width * c.taper);
            const taper = clamp(edge, 0, 1);
            if (taper <= 0) continue;
            const sw = lerp(c.strandWidth[0], c.strandWidth[1], h2);
            const len = lerp(c.strandLen[0], c.strandLen[1], h3) * bandH;
            // Волнисто-рваная нижняя кромка (§2.1): медленная волна по X +
            // рваность на прядь. Прямой горизонтальной кромки не остаётся.
            const bottomOff = (0.5 + 0.5 * Math.sin((sx / c.waveLen) * TWO_PI + c.phase)) * c.waveAmp
                + c.bottomJitter[0] + h4 * (c.bottomJitter[1] - c.bottomJitter[0]);
            const yBottom = spriteH - bottomOff;
            const yTop = yBottom - len;
            const warp = (h4 - 0.5) * 2 * c.warp;
            const grad = g.createLinearGradient(0, yBottom, 0, yTop);
            grad.addColorStop(0, rgbaCss(low, 0.95));
            grad.addColorStop(0.55, rgbaCss(mid, 0.60));
            grad.addColorStop(1, rgbaCss(high, 0));
            g.strokeStyle = grad;
            // Ореол (широкий, тусклый) + тело + ядро — мягкое свечение без
            // жёстких нитей (§2.2, §3 п.3). Аддитивно пряди сливаются.
            const strand = (w, alpha) => {
                g.globalAlpha = clamp(alpha * taper, 0, 1);
                g.lineWidth = w;
                g.beginPath();
                g.moveTo(sx, yBottom);
                g.quadraticCurveTo(sx + warp, (yBottom + yTop) / 2, sx + warp * 0.5, yTop);
                g.stroke();
            };
            strand(sw * 3.0, c.haloAlpha);
            strand(sw, c.alpha);
            strand(sw * 0.45, c.alpha * 1.3);
        }
        return {
            canvas, width, spriteH, x0: c.x0, parallax: c.parallax,
            driftPeriod: c.driftPeriod, driftAmp: c.driftAmp, phase: c.phase, period: c.period,
        };
    });
    a._baked = { key, curtains, tile: a.tile };
    return a._baked;
}

// drawCurtainBanded — запечённый занавес полосами: альфа полосы = огибающая
// бегущей снизу вверх волны яркости (§2.3). Иных покадровых градиентов нет.
function drawCurtainBanded(ctx, canvas, w, h, dx, dy, tSec, period, phase, baseAlpha) {
    const bandH = h / AURORA_BANDS;
    const frac = (((tSec / period + phase) % 1) + 1) % 1;
    const yBand = h - frac * h;
    const sigma = h * 0.16;
    for (let i = 0; i < AURORA_BANDS; i++) {
        const srcY = i * bandH;
        const d = (srcY + bandH / 2 - yBand) / sigma;
        const env = 0.20 + 0.80 * Math.exp(-0.5 * d * d);
        ctx.globalAlpha = clamp(baseAlpha * env, 0, 1);
        ctx.drawImage(canvas, 0, srcY, w, bandH + 1, dx, dy + srcY, w, bandH + 1);
    }
}

// drawAurora — 2–4 широких мягких занавеса (направление §2): разная глубина
// (параллакс 0.05–0.12), медленный дрейф, аддитивное свечение, чистое небо.
// alpha штор × nightFactor (§6.4): днём ровно 0 (не видно).
function drawAurora(ctx, world, camera, vw, vh, p, tSec, nightFactor) {
    if (nightFactor <= 0.001) return;
    const baked = bakeAurora(p, world, vw, vh);
    const win = viewWindow(vw, vh);
    const hy = horizonY(world, camera, vh);
    const baseAlpha = ctx.globalAlpha * nightFactor;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    for (const c of baked.curtains) {
        const drift = Math.sin((tSec / c.driftPeriod) * TWO_PI + c.phase) * c.driftAmp;
        let x = mod(c.x0 - camera.x * c.parallax + drift, baked.tile);
        x += Math.floor((win.x0 - 120 - x) / baked.tile) * baked.tile;
        const dy = hy - c.spriteH;
        for (; x < win.x1 + 120; x += baked.tile) {
            drawCurtainBanded(ctx, c.canvas, c.width, c.spriteH, x, dy, tSec, c.period, c.phase, baseAlpha);
        }
    }
    ctx.restore();
}

// drawAuroraReflection — аддитивный тинт рельефа 4–8 % (§5.7), × nightFactor (§6.4).
function drawAuroraReflection(ctx, world, camera, vw, vh, p, nightFactor) {
    if (nightFactor <= 0.001) return;
    const cA = auroraColors(p.aurora.palette, world.category)[0];
    const win = viewWindow(vw, vh);
    const hy = horizonY(world, camera, vh);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.fillStyle = rgbaCss(parseHex(cA), p.aurora.reflection * nightFactor);
    ctx.fillRect(win.x0 - 60, hy, (win.x1 - win.x0) + 120, Math.max(0, vh - hy) + 120);
    ctx.restore();
}

// ==================== ТРИ ПРОХОДА (§6.2–6.4) ====================

// weatherStack — во время кроссфейда (§8) два явления с комплементарной альфой.
function weatherStack(w) {
    if (!w || !w.params) return [];
    if (!w.crossFrom || w.crossT == null) return [{ run: w, alpha: 1 }];
    const cf = WEATHER_RULES.crossfadeMs;
    const dur = (cf[0] + cf[1]) / 2;
    const p = clamp((performance.now() - w.crossT) / dur, 0, 1);
    if (p >= 1) return [{ run: w, alpha: 1 }];
    return [{ run: w.crossFrom, alpha: 1 - p }, { run: w, alpha: p }];
}

// drawWeatherBack — после неба, до дальнего силуэта: тинт неба в airColor (§6.3)
// + облачный пояс грозы (lightning.cloud — не эмиссивная часть, × lightMul).
// Шторы сияния сюда НЕ входят: эмиссивные элементы перенесены в drawEmissive (§6.3 п.4).
export function drawWeatherBack(ctx, world, camera, vw, vh, w, env) {
    if (!w) return;
    const tSec = performance.now() / 1000;
    const lightMul = env ? env.lightMul() : 1;
    for (const { run, alpha } of weatherStack(w)) {
        if (!run.params) continue;
        const p = dimAir(run.params, lightMul);
        ctx.save();
        ctx.globalAlpha = alpha;
        fillAir(ctx, p, p.air.sky, vw, vh);
        if (p.lightning) drawLightningCloud(ctx, world, camera, vw, vh, run.params, tSec, lightMul);
        ctx.restore();
    }
}

// drawWeatherMid — после дальнего силуэта, до рельефа: дымка дальнего силуэта
// (Air.far) + дальний (0.4) и средний (0.7) пояса частиц (§6.3).
export function drawWeatherMid(ctx, world, camera, vw, vh, w, env) {
    if (!w) return;
    const tSec = performance.now() / 1000;
    const lightMul = env ? env.lightMul() : 1;
    for (const { run, alpha } of weatherStack(w)) {
        if (!run.params) continue;
        const p = dimAir(run.params, lightMul);
        ctx.save();
        ctx.globalAlpha = alpha;
        fillAir(ctx, p, p.air.far, vw, vh);
        for (let bi = 0; bi < p.belts.length; bi++) {
            if (p.belts[bi].parallax > PARALLAX_MID + 1e-6) continue;
            drawBelt(ctx, world, camera, p, p.belts[bi], bi, tSec, vw, vh);
        }
        ctx.restore();
    }
}

// drawWeatherFront — после игрока: дымка рельефа/декора (Air.mid + Air.near) и
// пелена кадра (Air.frame) СНАЧАЛА, затем ближний пояс (1.0), искры «ледяных игл»
// и приземные эффекты — иначе крупные ближние частицы гасятся пеленой (§6.3).
// Эмиссивные элементы (reflection сияния) — в drawEmissive; искры glint — здесь
// (не эмиссивная часть, «игра света» × (1 + (lowSunBoost−1)·lowSun), §6.4).
export function drawWeatherFront(ctx, world, camera, vw, vh, w, env) {
    if (!w) return;
    const tSec = performance.now() / 1000;
    const lightMul = env ? env.lightMul() : 1;
    const nightFactor = env ? env.nightFactor() : 1;
    const lowSun = 4 * nightFactor * (1 - nightFactor);
    for (const { run, alpha } of weatherStack(w)) {
        if (!run.params) continue;
        const p = dimAir(run.params, lightMul);
        ctx.save();
        ctx.globalAlpha = alpha;
        fillAir(ctx, p, p.air.mid, vw, vh);
        fillAir(ctx, p, p.air.near, vw, vh);
        fillAir(ctx, p, p.air.frame, vw, vh);
        // «Свечение»: лёгкий airColor-тинт (§6.3 п.4) — в воздушном проходе, НЕ × lightMul.
        if (p.glow && p.glow.airTint > 0) {
            const pal = (p.glow.palette && (p.glow.palette[world.category] || p.glow.palette['биосфера'])) || ['#6ef0a0', '#8f7cff'];
            ctx.fillStyle = rgbaCss(parseHex(pal[0]), p.glow.airTint);
            ctx.fillRect(0, 0, vw, vh);
        }
        for (let bi = 0; bi < p.belts.length; bi++) {
            if (p.belts[bi].parallax <= PARALLAX_MID + 1e-6) continue;
            drawBelt(ctx, world, camera, p, p.belts[bi], bi, tSec, vw, vh);
        }
        if (p.glint) drawGlints(ctx, world, camera, vw, vh, p, tSec, lowSun);
        drawGround(ctx, world, camera, vw, vh, p, tSec);
        ctx.restore();
    }
}

// drawEmissive — ФИНАЛЬНЫЙ эмиссивный проход (спека §6.3 п.4, решение создателя
// 2026-09-22): после drawEnvironmentFront, поверх переднего ночного тинта. Рисует
// только свет: разряд+вспышку+groundGlow грозы, glow свечения, aurora+reflection
// сияния. Модуляции (§6.4): вспышка/эхо × (1 + 1.2·nightFactor), glow × (0.4 +
// 0.6·nightFactor), aurora/reflection × nightFactor. lightMul и передний тинт
// среды к эмиссивному НЕ применяются.
export function drawEmissive(ctx, world, camera, vw, vh, w, env) {
    if (!w) return;
    const tSec = performance.now() / 1000;
    const nightFactor = env ? env.nightFactor() : 1;
    for (const { run, alpha } of weatherStack(w)) {
        if (!run.params) continue;
        const p = run.params;
        ctx.save();
        ctx.globalAlpha = alpha;
        if (p.aurora) {
            drawAurora(ctx, world, camera, vw, vh, p, tSec, nightFactor);
            drawAuroraReflection(ctx, world, camera, vw, vh, p, nightFactor);
        }
        if (p.lightning) drawLightning(ctx, world, camera, vw, vh, p, tSec, nightFactor);
        if (p.glow) drawGlow(ctx, world, camera, vw, vh, p, tSec, nightFactor);
        ctx.restore();
    }
}
