// tools/surface-weather-check.mjs
// Node-чек подэтапа 2 погоды прогулки (спека 2026-09-22, тесты T1/T3/T9/T12 + T2):
//  - T1: жёсткие гейты новых явлений (вакуум, H₂O-репер, вещество+фаза, life, вулканизм);
//  - T2: маппинг id → primitive (5 новых отличаются от остальных 10);
//  - T3: детерминизм (seed, цикл) и отсутствие Math.random в исходниках;
//  - T9: пакет без sky/данных — без падения; weatherLabel для неизвестного id = id;
//  - T12: пороги совпадают с sourced-константами; 233 K помечен эмпирикой.
// Запуск из корня проекта:  node tools/surface-weather-check.mjs
import { pickWeatherRun, weatherLabel } from '../web/static/js/surface/surface_weather.js';
import { drawFlake, drawBank } from '../web/static/js/surface/surface_weather_shapes.js';
import { WEATHER_IDS, WEATHER_VISUALS, WEATHER_RULES, TRIPLE_POINT_K, DIAMOND_DUST_K, GLASS_FIELD_K } from '../web/static/js/surface/surface_config.js';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const results = [];
function check(name, ok, detail) {
    results.push({ name, ok });
    console.log(`[${ok ? 'PASS' : 'FAIL'}] ${name}${detail ? ' - ' + detail : ''}`);
}
const near = (a, b, eps = 1e-9) => Math.abs(a - b) <= eps;

const HERE = path.dirname(fileURLToPath(import.meta.url));

function mkPkg(over = {}) {
    return {
        seed: 123456,
        temperature: 300,
        pressure_atm: 1.0,
        suit: { temp_comfort_k: [263, 313], pressure_comfort_atm: [0.5, 3.0] },
        biome_category: 'литосфера',
        liquid_medium: '',
        toxic: false,
        radioactive: false,
        radioactivity: 0,
        life: false,
        biome_color: '#8a7a6a',
        ...over,
    };
}

// Сбор выбранных id/вариантов за n циклов (админский forceId — без жребия).
function sample(pkg, n = 800, forceId = null) {
    const ids = new Set(), variants = new Set();
    let prev = null;
    for (let c = 0; c < n; c++) {
        const run = pickWeatherRun(pkg, c, prev, forceId);
        ids.add(run.id);
        if (run.variant) variants.add(run.id + '|' + run.variant);
        prev = run.id;
    }
    return { ids, variants };
}
const idList = (s) => [...s].join(' ');

// ==================== T1: гейты ====================
check('T1: WEATHER_IDS — 11 явлений', WEATHER_IDS.length === 11, WEATHER_IDS.join(','));

{
    const s = sample(mkPkg({ pressure_atm: 0.1, biome_category: 'биосфера', life: true, temperature: 300 }));
    check('T1: вакуум p<0.5 → только штиль/пыль (в т.ч. без свечения)',
        [...s.ids].every((x) => x === 'штиль' || x === 'пыльная буря'), idList(s.ids));
}
{
    const cold = sample(mkPkg({ temperature: 270, biome_category: 'литосфера' }));
    const warm = sample(mkPkg({ temperature: 300, biome_category: 'литосфера' }));
    check('T1: гроза только при T ≥ H₂O (273.16)',
        !cold.ids.has('гроза') && warm.ids.has('гроза'),
        `cold=[${idList(cold.ids)}] warm_has_гроза=${warm.ids.has('гроза')}`);
}
{
    // иглы/град только при T < H₂O
    const warm = sample(mkPkg({ temperature: 300, biome_category: 'крио' }));
    const cold = sample(mkPkg({ temperature: 200, biome_category: 'крио' }));
    check('T1: ледяные иглы/ледяной град только при T < H₂O',
        !warm.ids.has('ледяные иглы') && !warm.ids.has('ледяной град') &&
        cold.ids.has('ледяные иглы') && cold.ids.has('ледяной град'),
        `cold=[${idList(cold.ids)}]`);
}
{
    // крио-варианты требуют присутствия вещества и фазы
    const co2 = sample(mkPkg({ temperature: 200, biome_category: 'крио', liquid_medium: 'co2' }));
    const co2warm = sample(mkPkg({ temperature: 250, biome_category: 'крио', liquid_medium: 'co2' }));
    const ch4 = sample(mkPkg({ temperature: 80, biome_category: 'крио', liquid_medium: 'метан' }));
    const nh3 = sample(mkPkg({ temperature: 150, biome_category: 'крио', liquid_medium: 'аммиак' }));
    const water = sample(mkPkg({ temperature: 250, biome_category: 'вода', liquid_medium: '' }));
    const bare = sample(mkPkg({ temperature: 200, biome_category: 'экзотика', liquid_medium: '' }));
    check('T1: углекислые — co2 и T<CO₂ (216.55)',
        co2.variants.has('ледяные иглы|углекислые') && !co2warm.variants.has('ледяные иглы|углекислые'),
        `co2=[${[...co2.variants].join(' ')}]`);
    check('T1: метановые — метан и T<CH₄ (90.68)', ch4.variants.has('ледяные иглы|метановые'));
    check('T1: аммиачные — аммиак и T<NH₃ (195.40)', nh3.variants.has('ледяные иглы|аммиачные'));
    check('T1: водяные — вода/крио/вода и T<H₂O', water.variants.has('ледяные иглы|водяные'));
    check('T1: fallback иней — тело без известного вещества', bare.variants.has('ледяные иглы|иней'));
}
{
    const gran = sample(mkPkg({ temperature: 250, biome_category: 'вода' }));
    const grupa = sample(mkPkg({ temperature: 150, biome_category: 'вода' }));
    check('T1: град при 233 ≤ T < H₂O, крупа при T < 233',
        gran.variants.has('ледяной град|град') && grupa.variants.has('ледяной град|крупа'),
        `gran=[${[...gran.variants].filter((v) => v.startsWith('ледяной град')).join(' ')}] grupa=[${[...grupa.variants].filter((v) => v.startsWith('ледяной град')).join(' ')}]`);
}
{
    const vol = sample(mkPkg({ temperature: 400, biome_category: 'вулканизм' }));
    const vac = sample(mkPkg({ pressure_atm: 0.2, biome_category: 'вулканизм' }));
    check('T1: пепельный дождь — вулканизм и p ≥ 0.5 (toxic — множитель, не гейт)',
        vol.ids.has('пепельный дождь') && !vac.ids.has('пепельный дождь'),
        `vol_pepel=${vol.ids.has('пепельный дождь')}`);
    const noisy = sample(mkPkg({ temperature: 300, biome_category: 'вулканизм', toxic: true }));
    check('T1: пепел влажный/сухой/криовулканический — детерминированы',
        noisy.variants.has('пепельный дождь|сухой') || noisy.variants.has('пепельный дождь|влажный'),
        [...noisy.variants].filter((v) => v.startsWith('пепельный дождь')).join(' '));
}
{
    const dead = sample(mkPkg({ temperature: 300, biome_category: 'биосфера', life: false }));
    const alive = sample(mkPkg({ temperature: 300, biome_category: 'биосфера', life: true }));
    check('T1: life=false → без свечения; life=true → свечение есть',
        !dead.ids.has('свечение') && alive.ids.has('свечение'),
        `alive_has_свечение=${alive.ids.has('свечение')}`);
}
{
    // N₂-вариант не появляется нигде
    let nitro = false;
    for (const pkg of [
        mkPkg({ temperature: 50, biome_category: 'азот?', liquid_medium: 'n2' }),
        mkPkg({ temperature: 50, biome_category: 'крио', liquid_medium: 'n2' }),
        mkPkg({ temperature: 200, biome_category: 'крио', liquid_medium: 'co2' }),
    ]) {
        const s = sample(pkg, 400);
        for (const v of s.variants) if (/азот/.test(v)) nitro = true;
    }
    check('T1: N₂-вариант (азотные) не появляется', !nitro);
}

// ==================== T2: маппинг id → primitive ====================
{
    const newIds = {
        'гроза': 'lightning', 'ледяные иглы': 'glint', 'ледяной град': 'pellet',
        'пепельный дождь': 'ash', 'свечение': 'glow',
    };
    const all = Object.keys(WEATHER_VISUALS).map((id) => WEATHER_VISUALS[id].primitive);
    check('T2: primitive есть у всех 11 явлений', all.every((p) => p != null));
    let ok = true, detail = [];
    for (const [id, prim] of Object.entries(newIds)) {
        const p = WEATHER_VISUALS[id] && WEATHER_VISUALS[id].primitive;
        const others = Object.keys(WEATHER_VISUALS).filter((x) => x !== id)
            .flatMap((x) => (Array.isArray(WEATHER_VISUALS[x].primitive) ? WEATHER_VISUALS[x].primitive : [WEATHER_VISUALS[x].primitive]));
        if (p !== prim || others.includes(prim)) { ok = false; detail.push(id + '=' + p); }
    }
    check('T2: 5 новых примитивов отличаются от остальных', ok, detail.join(' '));
}

// ==================== T3: детерминизм и Math.random ====================
{
    const pkg = mkPkg({ temperature: 300, biome_category: 'вулканизм', liquid_medium: 'вода', toxic: true, life: true, radioactive: true });
    let same = true;
    for (let c = 0; c < 200; c++) {
        const a = pickWeatherRun(pkg, c);
        const b = pickWeatherRun(pkg, c);
        if (a.id !== b.id || a.variant !== b.variant) same = false;
    }
    check('T3: (seed, цикл) → то же явление+вариант', same);
}
{
    const files = ['surface_weather.js', 'surface_weather_shapes.js', 'surface_config.js'];
    let bad = [];
    for (const f of files) {
        const src = readFileSync(path.join(HERE, '..', 'web', 'static', 'js', 'surface', f), 'utf8');
        if (/Math\.random\s*\(/.test(src)) bad.push(f);
    }
    check('T3: Math.random в исходниках погоды нет', bad.length === 0, bad.join(','));
}

// ==================== T9: клиент без данных ====================
{
    let threw = null;
    try {
        // suit в пакете есть всегда; «без данных» — нет sky/метки/жидкости/категории.
        const bare = { seed: 5, temperature: 300, pressure_atm: 1.0, suit: { temp_comfort_k: [263, 313], pressure_comfort_atm: [0.5, 3.0] } };
        pickWeatherRun(bare, 0);
        pickWeatherRun(bare, 1);
        const unknownCat = mkPkg({ temperature: 100, biome_category: 'неизвестная', liquid_medium: undefined, radioactive: undefined, life: undefined });
        delete unknownCat.sky;
        pickWeatherRun(unknownCat, 0);
    } catch (e) { threw = String(e); }
    check('T9: пакет без sky/данных — без падения', threw === null, threw || '');
    check('T9: weatherLabel(неизвестный id) = id; weatherLabel(null) = ""',
        weatherLabel({ id: 'нет-такого' }) === 'нет-такого' && weatherLabel(null) === '');
}

// ==================== подписи (§5.3) ====================
{
    check('Подписи: дождь обычный→«Дождь», кислотный→«Кислотный дождь»',
        weatherLabel({ id: 'кислотный дождь', variant: 'обычный' }) === 'Дождь' &&
        weatherLabel({ id: 'кислотный дождь', variant: 'кислотный' }) === 'Кислотный дождь');
    const newLabels = { 'гроза': 'Гроза', 'ледяные иглы': 'Ледяные иглы', 'ледяной град': 'Ледяной град', 'пепельный дождь': 'Пепельный дождь', 'свечение': 'Свечение' };
    let ok = true;
    for (const [id, l] of Object.entries(newLabels)) if (WEATHER_VISUALS[id].label !== l) ok = false;
    check('Подписи: 5 новых `label` по UI §3', ok);
    check('Подписи вариантов: гроза токсичная/сухая',
        weatherLabel({ id: 'гроза', variant: 'токсичная' }) === 'Кислотная гроза' &&
        weatherLabel({ id: 'гроза', variant: 'сухая' }) === 'Сухая гроза');
    check('Подписи вариантов: град «Град», *крупа «Ледяная крупа»',
        weatherLabel({ id: 'ледяной град', variant: 'град' }) === 'Град' &&
        weatherLabel({ id: 'ледяной град', variant: 'углекислая крупа' }) === 'Ледяная крупа' &&
        weatherLabel({ id: 'ледяной град', variant: 'базовая крупа' }) === 'Ледяная крупа');
    check('Подписи вариантов: пепел сухой/криовулканический «Пепелопад»',
        weatherLabel({ id: 'пепельный дождь', variant: 'сухой' }) === 'Пепелопад' &&
        weatherLabel({ id: 'пепельный дождь', variant: 'криовулканический' }) === 'Пепелопад');
    check('Подписи вариантов: свечение туман/ливень/споры',
        weatherLabel({ id: 'свечение', variant: 'светящийся туман' }) === 'Светящийся туман' &&
        weatherLabel({ id: 'свечение', variant: 'светящийся ливень' }) === 'Светящийся ливень' &&
        weatherLabel({ id: 'свечение', variant: 'биоаэрозоль' }) === 'Светящиеся споры');
}

// ==================== T12: пороги sourced ====================
{
    check('T12: тройные точки = NIST (§3.4)',
        near(TRIPLE_POINT_K.H2O, 273.16) && near(TRIPLE_POINT_K.CH4, 90.68) &&
        near(TRIPLE_POINT_K.N2, 63.15) && near(TRIPLE_POINT_K.CO2, 216.55) &&
        near(TRIPLE_POINT_K.NH3, 195.40),
        JSON.stringify(TRIPLE_POINT_K));
    const cfg = readFileSync(path.join(HERE, '..', 'web', 'static', 'js', 'surface', 'surface_config.js'), 'utf8');
    check('T12: DIAMOND_DUST_K = 233 и помечен эмпирикой',
        DIAMOND_DUST_K === 233 && /DIAMOND_DUST_K\s*=\s*233/.test(cfg) && /эмпирик/.test(cfg));
    const phys = WEATHER_RULES.byTempPhysical;
    check('T12: физические полосы без перекрытий (extreme/severe/deep_frost/ice)',
        !!phys && Object.keys(phys).join(',') === 'extreme,severe,deep_frost,ice',
        Object.keys(phys || {}).join(','));
}

// ==================== подэтап 3a: метель flake / туман bank (§5.1–5.2) ====================
{
    const m = WEATHER_VISUALS['метель'];
    const near = m.belts[2];
    check('3a: метель ближний пояс — flake, штрихов нет (T4)',
        near.shape === 'flake' && near.streak === undefined &&
        m.belts[0].shape === 'dot' && m.belts[1].shape === 'dot',
        `near.shape=${near.shape} streak=${near.streak}`);
    check('3a: метель блок flake по направлению §5',
        !!m.flake && m.flake.coreRatio === 0.55 && m.flake.arms[0] === 5 && m.flake.arms[1] === 6 &&
        m.flake.coreColor === '#ffffff' && m.flake.edgeColor === '#dbe7f7',
        JSON.stringify(m.flake));
    check('3a: примитивы flake/bank экспортированы',
        typeof drawFlake === 'function' && typeof drawBank === 'function');
}
{
    const f = WEATHER_VISUALS['туман'];
    check('3a: туман блок bank (watersOnly, §5.2)',
        !!f.bank && f.bank.watersOnly === true && f.bank.w[0] === 300 && f.bank.w[1] === 700 &&
        f.bank.h[0] === 30 && f.bank.h[1] === 90,
        JSON.stringify(f.bank));
    const water = pickWeatherRun(mkPkg({ biome_category: 'вода', temperature: 300 }), 0, null, 'туман');
    const land = pickWeatherRun(mkPkg({ biome_category: 'литосфера', temperature: 300 }), 0, null, 'туман');
    check('3a: на категории вода туман рисует bank вместо clump (T5)',
        water.params.belts.length > 0 && water.params.belts.every((b) => b.shape === 'bank') &&
        land.params.belts.length > 0 && land.params.belts.every((b) => b.shape === 'clump'),
        `water=[${water.params.belts.map((b) => b.shape)}] land=[${land.params.belts.map((b) => b.shape)}]`);
    check('3a: блок bank развёрнут в params тумана на воде',
        !!water.params.bank && Array.isArray(water.params.bank.w) && water.params.bank.w.length === 2,
        JSON.stringify(water.params.bank && water.params.bank.w));
    check('3a: метель ближний пояс в params — flake',
        pickWeatherRun(mkPkg({ temperature: 200, biome_category: 'крио' }), 0, null, 'метель').params.belts[2].shape === 'flake');
}

// ==================== подэтап 3b: 6 новых вариантов + подписи (§4.3/§4.6/UI §5) ====================

// Приоритет/тотальность variantFor (админский forceId — без жребия).
{
    const v = (over) => pickWeatherRun(mkPkg(over), 0, null, 'туман').variant;
    const co2 = v({ temperature: 220, biome_category: 'крио', liquid_medium: 'co2' });
    const meth = v({ temperature: 150, biome_category: 'крио', liquid_medium: 'метан' });
    const amm = v({ temperature: 150, biome_category: 'крио', liquid_medium: 'аммиак' });
    const tox = v({ temperature: 200, biome_category: 'крио', liquid_medium: 'вода', toxic: true });
    const cold = v({ temperature: 250, biome_category: 'литосфера' });
    const hot = v({ temperature: 400, biome_category: 'литосфера' });
    const base = v({ temperature: 300, biome_category: 'литосфера' });
    check('3b: туман — co2 > метан/аммиак > серный > морозный > марево > базовый (§4.6)',
        co2 === 'плотный CO₂' && meth === 'аммиачно-метановый' && amm === 'аммиачно-метановый' &&
        tox === 'серный' && cold === 'морозный' && hot === 'марево' && base === null,
        [co2, meth, amm, tox, cold, hot, String(base)].join(' | '));
    check('3b: туман — co2 доминирует над toxic (порядок вытесняет, §4.6 п.3)',
        v({ temperature: 220, biome_category: 'крио', liquid_medium: 'co2', toxic: true }) === 'плотный CO₂');
}
{
    const r = (over) => pickWeatherRun(mkPkg(over), 0, null, 'кислотный дождь').variant;
    const radio = r({ temperature: 300, radioactive: true, pressure_atm: 4.0, toxic: true });
    const inv = r({ temperature: 300, pressure_atm: 4.0, toxic: true });
    const acid = r({ temperature: 300, pressure_atm: 1.0, toxic: true });
    const norm = r({ temperature: 300, pressure_atm: 1.0 });
    check('3b: дождь — радиоактивный > инверсионный > кислотный > обычный (§4.6)',
        radio === 'радиоактивный' && inv === 'инверсионный' && acid === 'кислотный' && norm === 'обычный',
        [radio, inv, acid, norm].join(' | '));
}
{
    const dust = (over) => pickWeatherRun(mkPkg(over), 0, null, 'пыльная буря').variant;
    check('3b: пыльная буря — T ≥ 480 → стеклянно-кристаллическая, иначе базовая (§4.6)',
        dust({ temperature: 500 }) === 'стеклянно-кристаллическая' && dust({ temperature: 300 }) === null,
        `${dust({ temperature: 500 })} | ${String(dust({ temperature: 300 }))}`);
    const aur = (over) => pickWeatherRun(mkPkg(over), 0, null, 'полярное сияние').variant;
    check('3b: сияние — radioactive → магнитная буря, иначе базовое (§4.6)',
        aur({ radioactive: true }) === 'магнитная буря' && aur({ radioactive: false }) === null,
        `${aur({ radioactive: true })} | ${String(aur({ radioactive: false }))}`);
}

// Подписи новых вариантов (UI §5).
{
    check('3b: подписи вариантов тумана/дождя/пыли/сияния (UI §5)',
        weatherLabel({ id: 'кислотный дождь', variant: 'инверсионный' }) === 'Ливень' &&
        weatherLabel({ id: 'кислотный дождь', variant: 'радиоактивный' }) === 'Радиоактивный дождь' &&
        weatherLabel({ id: 'туман', variant: 'серный' }) === 'Смог' &&
        weatherLabel({ id: 'туман', variant: 'плотный CO₂' }) === 'Густой туман' &&
        weatherLabel({ id: 'туман', variant: 'аммиачно-метановый' }) === 'Мутный туман' &&
        weatherLabel({ id: 'туман', variant: 'морозный' }) === 'Морозный туман' &&
        weatherLabel({ id: 'туман', variant: 'марево' }) === 'Марево' &&
        weatherLabel({ id: 'пыльная буря', variant: 'стеклянно-кристаллическая' }) === 'Стеклянная буря' &&
        weatherLabel({ id: 'полярное сияние', variant: 'магнитная буря' }) === 'Магнитная буря');
}

// Визуальные блоки вариантов (направление §4, числа).
{
    const T = WEATHER_VISUALS['туман'];
    check('3b: туман.аммиачно-метановый — air.blend #7f8f66 (§4.3)',
        !!T.variants['аммиачно-метановый'] && Array.isArray(T.variants['аммиачно-метановый'].air.blend) &&
        T.variants['аммиачно-метановый'].air.blend[0] === '#7f8f66' && T.variants['аммиачно-метановый'].air.blend[1] === 0.5);
    check('3b: туман.серный — blend #b6c86a + alphaMul 1.3 сохранены (§4.3)',
        T.variants['серный'].air.blend[0] === '#b6c86a' && T.variants['серный'].alphaMul === 1.3);
    const co2 = pickWeatherRun(mkPkg({ temperature: 220, biome_category: 'крио', liquid_medium: 'co2' }), 0, null, 'туман');
    check('3b: туман.плотный CO₂ — bank 300–700 × 40–100, alpha 0.30–0.45, pulseY 4 (§4.3)',
        co2.params.belts.length > 0 && co2.params.belts.every((b) => b.shape === 'bank') &&
        co2.params.bank.w[0] === 300 && co2.params.bank.w[1] === 700 &&
        co2.params.bank.h[0] === 40 && co2.params.bank.h[1] === 100 &&
        co2.params.bank.alpha[0] === 0.30 && co2.params.bank.alpha[1] === 0.45 &&
        co2.params.bank.pulseY === 4,
        `shapes=[${co2.params.belts.map((b) => b.shape)}]`);
    const am = pickWeatherRun(mkPkg({ temperature: 150, biome_category: 'крио', liquid_medium: 'метан' }), 0, null, 'туман');
    check('3b: туман.аммиачно-метановый — bank, кромка ниже 10–25 px (§4.3)',
        am.params.belts.length > 0 && am.params.belts.every((b) => b.shape === 'bank') &&
        am.params.bank.offset[0] === 10 && am.params.bank.offset[1] === 25,
        `shapes=[${am.params.belts.map((b) => b.shape)}]`);
}
{
    const R = WEATHER_VISUALS['кислотный дождь'];
    check('3b: дождь.радиоактивный — particle #9fe060 + splashGlow + ground ×1.2 (§4.3)',
        R.variants['радиоактивный'].particle === '#9fe060' &&
        R.variants['радиоактивный'].splashGlow[0] === 0.06 && R.variants['радиоактивный'].splashGlow[1] === 0.12 &&
        R.variants['радиоактивный'].ground.alpha[0] === 0.36 && R.variants['радиоактивный'].ground.alpha[1] === 0.66);
    const radio = pickWeatherRun(mkPkg({ temperature: 300, radioactive: true }), 0, null, 'кислотный дождь');
    check('3b: дождь.радиоактивный — params particle/splashGlow/ground',
        radio.params.particle === '#9fe060' && radio.params.splashGlow >= 0.06 && radio.params.splashGlow <= 0.12 &&
        radio.params.ground.alpha[0] === 0.36,
        `splashGlow=${radio.params.splashGlow}`);
    const inv = pickWeatherRun(mkPkg({ temperature: 300, pressure_atm: 4.0 }), 0, null, 'кислотный дождь');
    check('3b: дождь.инверсионный — streak [18,30], fall/ground ×1.3 (§4.3)',
        inv.params.belts[2].streak[0] === 18 && inv.params.belts[2].streak[1] === 30 &&
        inv.params.belts[0].fall[0] === 650 && inv.params.belts[2].fall[1] === 1235 &&
        inv.params.ground.alpha[0] === 0.39 && inv.params.ground.alpha[1] === 0.715,
        `streak=[${inv.params.belts[2].streak}] fall0=${inv.params.belts[0].fall} ground=${inv.params.ground.alpha}`);
}
{
    const dust = pickWeatherRun(mkPkg({ temperature: 500 }), 0, null, 'пыльная буря');
    check('3b: пыльная буря.стеклянно-кристаллическая — грани + particle (§4.3)',
        dust.params.particle === '#dfe8f0' &&
        dust.params.belts[0].shape === 'facet' && dust.params.belts[1].shape === 'facet' &&
        dust.params.belts[2].shape === 'streak' &&
        dust.params.belts[0].alpha[0] === 0.4 && dust.params.belts[0].alpha[1] === 0.7 &&
        !!dust.params.facet && dust.params.facet.rayShare === 0.2 &&
        dust.params.facet.rayLen[0] === 4 && dust.params.facet.rayLen[1] === 9,
        `shapes=[${dust.params.belts.map((b) => b.shape)}] particle=${dust.params.particle}`);
}
{
    const aur = pickWeatherRun(mkPkg({ radioactive: true }), 0, null, 'полярное сияние');
    const pal = aur.params.aurora.palette;
    check('3b: сияние.магнитная буря — curtains [4,6], bandHeight [0.45,0.60], 3-цветная палитра (§4.3)',
        aur.params.aurora.curtains.length >= 4 && aur.params.aurora.curtains.length <= 6 &&
        aur.params.aurora.curtains.every((c) => c.bandHeight >= 0.45 && c.bandHeight <= 0.60) &&
        Object.values(pal).every((c) => Array.isArray(c) && c.length === 3 && c[1] === '#5f7cff'),
        `curtains=${aur.params.aurora.curtains.length}`);
}

// Примитивы-массивы (мелочь 3a, §2: вариант приём не меняет).
{
    check('3b: primitive метели [dot,flake], тумана [clump,bank] (§3)',
        JSON.stringify(WEATHER_VISUALS['метель'].primitive) === '["dot","flake"]' &&
        JSON.stringify(WEATHER_VISUALS['туман'].primitive) === '["clump","bank"]',
        `${JSON.stringify(WEATHER_VISUALS['метель'].primitive)} | ${JSON.stringify(WEATHER_VISUALS['туман'].primitive)}`);
    const cfg = readFileSync(path.join(HERE, '..', 'web', 'static', 'js', 'surface', 'surface_config.js'), 'utf8');
    check('T12/3b: GLASS_FIELD_K = 480 из каталога (не «на глаз»)',
        GLASS_FIELD_K === 480 && /GLASS_FIELD_K\s*=\s*480/.test(cfg) && /biome_catalog/.test(cfg),
        `GLASS_FIELD_K=${GLASS_FIELD_K}`);
}

const failed = results.filter((r) => !r.ok).length;
console.log(`SUMMARY: ${results.length - failed} PASS / ${failed} FAIL`);
process.exit(failed ? 1 : 0);
