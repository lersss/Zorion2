import { hash1, fbm1, num, primRange, clamp01 } from './surface_world_noise.js';

// ==================== ПРОФИЛЬНЫЕ ПРИМИТИВЫ РЕЛЬЕФА (§3.1, Э3) ====================
//
// Восемь примитивов — чистые функции (x, params, seed) → Δy (отрицательное = вверх),
// складываются с базой terrainHeight. Семантика общих параметров (§3.1): `amp` —
// полуразмах; `share` — доля регионов, где слой включён (детерминированно от seed
// и индекса региона); `viewOnly` — слой только в отрисовке (viewHeight); `dir` —
// сторона асимметрии. Единая реализация для боевого стека (terrainHeight) и яруса
// горизонта (§3.6) — второй реализации одной формы нет.

// primWave — асимметричная волна (дюны/барханы/валы/зыбь): пологий наветренный
// склон (доля `skew` длины) и крутой подветренный. `leeMaxDeg` — угол естественного
// отсыпа: подветренный склон не круче него, выше гребень «срезается» (модель
// лавинного срыва, §4.1). `dir` — сторона асимметрии (+1 = пологий слева).
function primWave(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0xa1, 600);
    const amp = primRange(l.amp, 0, seed ^ 0xa2, 40);
    const skew = Math.max(0.05, Math.min(0.95, num(l.skew, 0.85)));
    const ph = x / lambda;
    const i = Math.floor(ph);
    let f = ph - i;
    if (l.dir === -1) f = 1 - f; // зеркало стороны асимметрии
    let a = amp * (0.75 + 0.5 * hash1(i, seed ^ 0xa3));
    if (typeof l.leeMaxDeg === 'number') {
        const maxSlope = Math.tan(l.leeMaxDeg * Math.PI / 180);
        const leeLen = (1 - skew) * lambda;
        if (leeLen > 0 && a / leeLen > maxSlope) a = maxSlope * leeLen; // срыв гребня
    }
    return -a * (f < skew ? f / skew : (1 - f) / (1 - skew));
}

// primCrest — гребень/хребет: ridged-шум (1−|2n−1|), `sharpness` заостряет пики.
function primCrest(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0xc1, 700);
    const amp = primRange(l.amp, 0, seed ^ 0xc2, 120);
    const sharp = num(l.sharpness, 0.7);
    const ridged = 1 - Math.abs(2 * fbm1(x / lambda, seed ^ 0xc3, 2) - 1);
    return -amp * Math.pow(ridged, 0.6 + 0.8 * sharp);
}

// primSpike — отдельные узкие пики/шпили/иглы: `perRegion` (или `count`) штук на
// регион, треугольник высоты `h` и полуширины `w`. `taper` заостряет вершину;
// `cluster` — иглы группируются в кусты/рощи (спека 2026-09-23 §4.7.12), а не
// стоят поодиночке. Без `cluster` — прежнее поведение (сданное не меняется).
function primSpike(x, l, seed, region) {
    const R = region || 1400;
    const cntRaw = l.perRegion != null ? l.perRegion : l.count;
    const taper = num(l.taper, 0);
    const cluster = !!l.cluster;
    const base = Math.floor(x / R);
    let sum = 0;
    // Соседние регионы тоже: пик у границы региона не должен «обрезаться»
    // (иначе разрыв профиля на границе — уклон-скачок).
    for (let rr = base - 1; rr <= base + 1; rr++) {
        const n = Math.max(0, Math.round(primRange(cntRaw, rr, seed ^ 0xd0, 1)));
        if (!cluster) {
            for (let k = 0; k < n; k++) {
                const idx = rr * 131 + k;
                const c = rr * R + hash1(idx, seed ^ 0xd1) * R;
                const h = primRange(l.h, idx, seed ^ 0xd2, 120);
                const w = primRange(l.w, idx, seed ^ 0xd3, 40);
                const d = Math.abs(x - c);
                if (d < w) sum -= h * Math.pow(1 - d / w, 1 + 2 * taper);
            }
            continue;
        }
        // cluster: `n` игл собираются в несколько кустов-рощ; центры кустов
        // распределены по региону, иглы теснятся вокруг центра (куст читается
        // как заросль, а не набор одиночек).
        const groves = Math.max(1, Math.round(Math.sqrt(n)));
        const per = Math.ceil(n / groves);
        for (let g = 0; g < groves; g++) {
            const gid = rr * 197 + g;
            const gc = rr * R + ((g + 0.5) / groves) * R
                + (hash1(gid, seed ^ 0xd4) - 0.5) * (R / groves) * 0.4;
            for (let k = 0; k < per; k++) {
                if (g * per + k >= n) break;
                const idx = gid * 131 + k;
                const c = gc + (hash1(idx, seed ^ 0xd5) - 0.5) * R * 0.03;
                const h = primRange(l.h, idx, seed ^ 0xd2, 120);
                const w = primRange(l.w, idx, seed ^ 0xd3, 40);
                const d = Math.abs(x - c);
                if (d < w) sum -= h * Math.pow(1 - d / w, 1 + 2 * taper);
            }
        }
    }
    return sum;
}

// primStep — террасы/пласты/меса: квантование низкочастотного профиля в уступы
// высоты `stepH`; `stepW` — горизонтальный масштаб, `jitter` — сдвиг границ.
function primStep(x, l, seed) {
    const stepH = primRange(l.stepH, 0, seed ^ 0xe1, 60);
    const stepW = Math.max(1, primRange(l.stepW, 0, seed ^ 0xe2, 240));
    const jitter = num(l.jitter, 0);
    const cell = Math.floor(x / stepW);
    const n = fbm1(x / (stepW * 3), seed ^ 0xe3, 2) + jitter * (hash1(cell, seed ^ 0xe4) - 0.5);
    return -stepH * Math.floor(clamp01(n) * 4);
}

// primDome — купол/всхолмление (холмы, пинго, тумули): плавные холмы ±`amp`.
// `squash` — «сплюснутость» (горизонтальное расширение купола).
function primDome(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0xf1, 300);
    const amp = primRange(l.amp, 0, seed ^ 0xf2, 50);
    const squash = Math.max(0.1, num(l.squash, 1));
    const n = fbm1(x / (lambda * squash), seed ^ 0xf3, 2);
    return -amp * (2 * n - 1);
}

// primCarve — вырез (каньон/русло/трещина/воронка): вниз на `depth` в центре русла
// полуширины `w`; `rim` — приподнятый борт у краёв («подмытые берега»), shape V/U.
// `closed` — гипотеза (образцами не задаётся).
function primCarve(x, l, seed) {
    const lambda = primRange(l.lambda, 0, seed ^ 0x11, 700);
    const depth = primRange(l.depth, 0, seed ^ 0x12, 60);
    const w = Math.max(1, primRange(l.w, 0, seed ^ 0x13, 160));
    const rim = num(l.rim, 0);
    const ph = x / lambda;
    const f = ph - Math.floor(ph);
    const d = Math.abs(f - 0.5) * lambda; // расстояние до центра русла (px)
    if (d >= w) return 0;
    const t = 1 - d / w;                  // 1 в центре, 0 на краю
    const prof = l.shape === 'V' ? t : t * t * (3 - 2 * t);
    // Борт (rim): вал у края русла — поднятие над базовой линией, максимум при
    // t≈0.15, ноль на самом краю (t=0) и дальше от края; «подмытые берега» §4.2.
    const lip = t < 0.3 ? Math.sin(Math.PI * t / 0.3) : 0;
    return depth * prof - depth * rim * lip;
}

// primFan — осыпь/конус выноса: уклон не круче `angleMax` (угол отсыпа), вынос =
// h / tan(angleMax). `w` (спека 2026-09-23 §4.7.12) — желаемая МИНИМАЛЬНАЯ ширина
// основания (полная): основание не уже, чем требует угол, угол никогда не круче
// `angleMax` — `halfW = max(h/tanA, w/2)`. `roughness` — неровность конуса —
// применяется ТОЛЬКО при явном `w` (гейт совместимости §4.7.13: сданные ЧК0/ЧК1
// несут инертную `roughness` и без `w` остаются гладкими).
function primFan(x, l, seed, region) {
    const R = region || 1400;
    const angleMax = num(l.angleMax, 34);
    const tanA = Math.tan(angleMax * Math.PI / 180);
    const hasW = l.w != null;
    const rough = hasW ? num(l.roughness, 0) : 0;
    const base = Math.floor(x / R);
    let sum = 0;
    // Соседние регионы — как у spike: конус у границы региона не обрезается.
    for (let rr = base - 1; rr <= base + 1; rr++) {
        const h = primRange(l.h, rr, seed ^ 0x21, 60);
        const wHalf = hasW ? primRange(l.w, rr, seed ^ 0x24, 0) / 2 : 0;
        const halfW = Math.max(8, h / tanA, wHalf);
        const c = rr * R + hash1(rr, seed ^ 0x23) * R;
        if (!hasW || rough <= 0) {
            const d = Math.abs(x - c);
            if (d <= halfW) sum -= h * (1 - d / halfW);
            continue;
        }
        // Шероховатый конус: несколько апексов внутри основания (верхняя
        // огибающая через min), каждый слагаемый не круче angleMax — угол
        // общей формы остаётся в пределах угла отсыпа.
        const sub = 3;
        for (let sl = 0; sl < sub; sl++) {
            const sid = rr * 53 + sl;
            const hh = h * (0.6 + 0.4 * hash1(sid, seed ^ 0x26));
            const cc = c + (hash1(sid, seed ^ 0x27) - 0.5) * (halfW * 0.6);
            const hw = Math.max(8, hh / tanA, wHalf);
            const dd = Math.abs(x - cc);
            if (dd <= hw) sum = Math.min(sum, -hh * (1 - dd / hw));
        }
    }
    return sum;
}

// primFlow — язык потока (лава/грязь/лёд/сель). Новые поля (спека 2026-09-23
// §4.7.12): `lobes` — число языков-лопастей на период (несколько стекающих
// языков, а не один вал); `slope` — асимметрия языка: знак = сторона срыва,
// модуль = крутизна (0 — симметричный вал); `levees` — доля боковых валов-гребней
// по краям потока. Без этих полей — прежний одиночный низкочастотный вал
// (сданные ЧК0/ЧК1/ЧК2 не меняются, §4.7.13).
function primFlow(x, l, seed) {
    const len = primRange(l.len, 0, seed ^ 0x31, 300);
    const w = primRange(l.w, 0, seed ^ 0x32, 120);
    if (l.lobes == null && l.slope == null && l.levees == null) {
        const n = fbm1(x / len, seed ^ 0x33, 1);
        return -w * 0.25 * (2 * n - 1);
    }
    const amp = w * 0.25;
    const lobes = Math.max(1, Math.round(primRange(l.lobes, 0, seed ^ 0x34, 1)));
    const slope = num(l.slope, 0);
    const levees = Math.max(0, num(l.levees, 0));
    const ph = x / len;
    const i = Math.floor(ph);
    let f = ph - i;
    if (slope < 0) f = 1 - f;                       // знак — сторона срыва
    const skew = Math.min(0.8, Math.abs(slope));    // модуль — крутизна асимметрии
    const half = 0.5 / lobes;
    // `lobes` языков-лопастей внутри периода: левый склон пологий, правый — срыв
    // (или наоборот при slope < 0); при skew = 0 языки симметричны. Языки не
    // доходят до границ периода (профиль 0 на стыке) — период непрерывен.
    let prof = 0;
    for (let k = 0; k < lobes; k++) {
        const c = (k + 0.5) / lobes;
        const s = f - c;
        const a = s < 0 ? half : half * (1 - skew);
        if (a <= 0 || Math.abs(s) >= a) continue;
        prof = Math.max(prof, 1 - Math.abs(s) / a);
    }
    // Боковые валы-гребни по краям потока (f → 0 и f → 1, §4.7.12).
    if (levees > 0) {
        const edge = 1 - Math.min(f, 1 - f) / half;
        if (edge > 0) prof = Math.max(prof, levees * edge);
    }
    const rise = amp * (0.75 + 0.5 * hash1(i, seed ^ 0x36));
    return -rise * prof;
}

// primHeight — диспетчер примитивов (§3.1). Неизвестный prim → 0 (forward-compat).
export function primHeight(prim, x, l, seed, region) {
    switch (prim) {
        case 'wave': return primWave(x, l, seed);
        case 'crest': return primCrest(x, l, seed);
        case 'spike': return primSpike(x, l, seed, region);
        case 'step': return primStep(x, l, seed);
        case 'dome': return primDome(x, l, seed);
        case 'carve': return primCarve(x, l, seed);
        case 'fan': return primFan(x, l, seed, region);
        case 'flow': return primFlow(x, l, seed);
        default: return 0;
    }
}

// horizonHeight — силуэт яруса горизонта (§3.6): та же библиотека примитивов, что и
// боевой стек (§3.1) — единая реализация формы, второго набора нет.
export function horizonHeight(prim, params, x, seed) {
    return primHeight(prim, x, params || {}, seed, 1400);
}

// liquidWave — смещение зеркала жидкости волной (ЧК6 §5.2): тот же примитив
// `wave`, что и рельеф (§3.1) — второй формы нет; прокрутка по времени t·speed,
// детерминированно от seed (Math.random запрещён, §9 п.7).
export function liquidWave(seed, wave, x, t) {
    const params = { prim: 'wave', lambda: wave.lambda, amp: wave.amp, skew: 0.6 };
    return primWave(x - t * (wave.speed || 0), params, (seed ^ 0x71e1) >>> 0);
}
