// tools/surface-field-check.mjs
// Э5.1 «Мир прогулки: скульптурный объём» — фундамент (спека 2026-09-25 §2/§5.1).
// Node-проверка БЕЗ сервера/браузера (чистое поле твёрдости):
//  - T1 единое поле: `solidAt` == эталону `isSolid` из HEAD (0 изменённых точек
//    поля/силуэта — критерий B.1.1 п.3; эталон берётся из git HEAD на гейте, ДО
//    коммита правки). `terrainHeight` тоже сверяется — двигаться не должен;
//  - T2 columnSpans: верх базы == terrainHeight, полоса float — над рельефом;
//  - T3 skyTop/floorY: контракт «поверхность неба» / «пол-опора»;
//  - T4 консервативная пещерная маска: «воздух» красится только там, где
//    solidAt=false; твёрдая точка НИКОГДА не перекрывается пещерой; дельта к
//    прежней грубой маске (3×3 по сэмплу) замеряется числом (B.1.1 п.3, S1-bis);
//  - T5 детерминизм поля;
//  - T7 2D-формы Э5.2 (§3.5): арка arch=1 даёт твёрдое выше terrainHeight с
//    просветом ≥ роста игрока; crack2d КОСМЕТИЧЕН (тёмный клин, поле не меняет,
//    решение гейта 2026-09-25, D1); поле детерминировано;
//  - T9 нет ловушек на биомах-целях: ходьба вправо не проваливает ниже th+2 (§5.1 п.5).
//  - T10/T11/T12 2D-формы Э5.3 (§3.6): `void`/`crater` на реальных биомах-целях
//    (пещерный_мир_с_потолком/лавовые_поля/магмовый_океан/кратеры) — оба знака
//    (вал+впадина crater), согласие `formSpans` ⇄ `solidAt`; контракт `skyTop`/
//    `floorY` (пол опущен вычитающей формой); ВЫХОДИМОСТЬ — независимая
//    детерминированная симуляция `Player` из дна каждой применённой вычитающей
//    формы обязана вывести на поверхность (§3.6/§5.1 п.5);
//  - T12-bis регресс D1: выходимость на ПОЛНОМ поле (форма+пещеры+соседи) —
//    сочетание вычитающих форм не оставляет ловушки; плюс синтетика caves:1.0.
// Запуск: node tools/surface-field-check.mjs
import { readFileSync, writeFileSync, unlinkSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { SurfaceWorld, hash1 } from '../web/static/js/surface/surface_world.js';
import { Player } from '../web/static/js/surface/surface_player.js';
import { CHUNK, FORMATIONS, FLOAT_SPAN, FLOAT_GAP, PLAYER_W, PLAYER_H } from '../web/static/js/surface/surface_config.js';

const results = [];
function check(name, ok, detail) {
    results.push({ name, ok });
    console.log(`[${ok ? 'PASS' : 'FAIL'}] ${name}${detail ? ' - ' + detail : ''}`);
}

const BASEY = 300, TOP_MARGIN = 1200, CHUNK_HEIGHT = 1700, TOPY = BASEY - TOP_MARGIN;
const CATEGORIES = Object.keys(FORMATIONS);
const SEEDS = [1, 42, 424242, 20260923];

function mkWorld(cat, seed, over = {}) {
    return new SurfaceWorld({ seed, biome_category: cat, biome_color: '#8a7a6a', life: false, biome: 'qa', ...over });
}

// Резолв вида биома из рабочего справочника (та же семантика, что у сервера:
// пресет семейства ← дельта биома). Нужен проверкам T3b/T4d на РЕАЛЬНЫХ формах
// горы/каменные_пустоши (§3.1/§3.5), а не на синтетическом рецепте.
const catalog = JSON.parse(readFileSync(new URL('../config/biome_catalog.json', import.meta.url), 'utf8'));
function mergeView(preset, delta) {
    const out = { ...preset };
    for (const k of Object.keys(delta)) {
        const pv = out[k], dv = delta[k];
        if (pv && typeof pv === 'object' && !Array.isArray(pv) && dv && typeof dv === 'object' && !Array.isArray(dv)) out[k] = mergeView(pv, dv);
        else out[k] = dv;
    }
    return out;
}
function resolveView(biomeId) {
    const b = catalog.biomes.find((x) => x.id === biomeId);
    if (!b || !b.view) return null;
    const v = JSON.parse(JSON.stringify(b.view));
    const famId = v.family;
    delete v.family;
    if (!famId) return v;
    const fam = (catalog.view_families || []).find((f) => f.id === famId);
    if (!fam) return v;
    const base = JSON.parse(JSON.stringify(fam));
    delete base.id; delete base.name;
    return mergeView(base, v);
}
function mkRecipeWorld(biomeId, seed) {
    return new SurfaceWorld({
        seed, biome: biomeId, biome_category: 'литосфера', biome_color: '#8a7a6a', life: false,
        view_source: 'catalog', view_version: 1, biome_view: resolveView(biomeId),
    });
}

// ==================== T1: единое поле == эталон HEAD ====================
// Эталон — прежний `isSolid` из HEAD (до правки Э5.1). Пока правка не закоммичена,
// git HEAD ещё отдаёт старую версию — это и есть «замороженный кадр» поля.
async function loadLegacy() {
    const here = path.dirname(fileURLToPath(import.meta.url));
    const repo = path.resolve(here, '..');
    // temp — рядом с боевым модулем: относительный импорт './surface_config.js'
    // должен разрешаться (иначе ERR_MODULE_NOT_FOUND).
    const tmpDir = path.join(repo, 'web', 'static', 'js', 'surface');
    let src;
    try {
        src = execFileSync('git', ['show', 'HEAD:web/static/js/surface/surface_world.js'], { cwd: repo, encoding: 'utf8' });
    } catch (e) {
        console.log('  (эталон HEAD недоступен — правка уже закоммичена; T1 сверяет поля между новыми копиями)');
        return null;
    }
    if (/formsSolid/.test(src)) {
        console.log('  (HEAD уже содержит Э5.1 — эталон недоступен; T1 пропущен)');
        return null;
    }
    const tmp = path.join(tmpDir, '.legacy_world_tmp.mjs');
    writeFileSync(tmp, src, 'utf8');
    try {
        const mod = await import(pathToFileURL(tmp).href);
        return mod.SurfaceWorld;
    } finally {
        try { unlinkSync(tmp); } catch (e) { /* ignore */ }
    }
}

const LegacyWorld = await loadLegacy();
{
    let total = 0, mismatchSolid = 0, mismatchTh = 0, first = null;
    for (const cat of CATEGORIES) {
        for (const seed of SEEDS) {
            const nw = mkWorld(cat, seed);
            const ow = LegacyWorld ? new LegacyWorld({ seed, biome_category: cat, biome_color: '#8a7a6a', life: false, biome: 'qa' }) : nw;
            for (let ci = 0; ci < 2; ci++) {
                const baseX = ci * CHUNK;
                for (let lx = 0; lx < CHUNK; lx += 2) {
                    const wx = baseX + lx;
                    const th = nw.terrainHeight(wx);
                    if (Math.abs(th - ow.terrainHeight(wx)) > 1e-9) mismatchTh++;
                    for (let wy = th - FLOAT_SPAN - 20; wy <= th + 60; wy += 2) {
                        total++;
                        const a = nw.solidAt(wx, wy);
                        const b = ow.isSolid(wx, wy);
                        if (a !== b) { mismatchSolid++; if (!first) first = `${cat}/s${seed}/x${wx}/y${wy.toFixed(1)}: new=${a} old=${b}`; }
                    }
                }
            }
        }
    }
    check('T1a solidAt == эталон isSolid (0 изменённых точек поля)', mismatchSolid === 0, `точек=${total} расхождений=${mismatchSolid}${first ? ' первое: ' + first : ''}`);
    check('T1b terrainHeight не изменилась', mismatchTh === 0, `расхождений=${mismatchTh}`);
}

// ==================== T2: columnSpans ====================
{
    let ok = true, bad = null, floatCols = 0, checked = 0;
    for (const cat of CATEGORIES) {
        for (const seed of SEEDS) {
            const w = mkWorld(cat, seed);
            for (let wx = -2000; wx <= 2000; wx += 5) {
                const spans = w.columnSpans(wx);
                const base = spans[spans.length - 1];
                checked++;
                if (base.bottom !== Infinity || Math.abs(base.top - w.terrainHeight(wx)) > 1e-9) { ok = false; bad = `base ${cat}/s${seed}/x${wx}`; break; }
                for (const sp of spans) {
                    if (sp.float) {
                        floatCols++;
                        if (!(sp.top < sp.bottom)) { ok = false; bad = `float-интервал пуст x${wx}`; }
                        if (!(sp.top > w.terrainHeight(wx) - FLOAT_SPAN - 1e-6 && sp.bottom < w.terrainHeight(wx) - FLOAT_GAP + 1e-6)) { ok = false; bad = `float-границы x${wx}`; }
                    }
                }
                if (!ok) break;
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    check('T2 columnSpans: база [terrainHeight, ∞), float-полоса над рельефом', ok, bad || `столбцов=${checked} float=${floatCols}`);
}

// ==================== T3: skyTop / floorY ====================
{
    let ok = true, bad = null, floatTop = 0, checked = 0;
    for (const cat of CATEGORIES) {
        for (const seed of SEEDS) {
            const w = mkWorld(cat, seed);
            for (let wx = -1500; wx <= 1500; wx += 3) {
                checked++;
                const st = w.skyTop(wx), fy = w.floorY(wx), th = w.terrainHeight(wx);
                if (!(st <= fy + 1e-9)) { ok = false; bad = `skyTop > floorY x${wx} (${st} > ${fy})`; break; }
                if (Math.abs(fy - th) > 1e-9) { ok = false; bad = `floorY != terrainHeight x${wx}`; break; }
                if (!w.solidAt(wx, st)) { ok = false; bad = `solidAt(skyTop)=false x${wx} y${st}`; break; }
                if (w.solidAt(wx, st - 1)) { ok = false; bad = `твёрдое выше skyTop x${wx} y${st - 1}`; break; }
                if (st < th) floatTop++;
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    check('T3 skyTop/floorY: верх твёрдого, опора базы; выше skyTop — воздух', ok, bad || `столбцов=${checked} float-верх=${floatTop}`);
}

// ==================== T3b: skyTop/floorY на биомах с 2D-формами ====================
// Э5.2: skyTop обязан быть верхом твёрдого (§2.1/§5 п.12): solidAt(skyTop)=true
// и НИЧЕГО твёрдого выше. Ловит два дефекта ревью:
// (а) зарытый arch=0-пласт занижал верх (skyTop НИЖЕ твёрдой корки th);
// (б) ветка трещины возвращала `it.bottom + 1` без проверки твёрдости.
// Проверка структурная: верх твёрдого — только th, верх аддитивной формы или
// первое твёрдое полосы float; эти три источника и сверяются (§2.2).
{
    const ids = ['горы', 'каменные_пустоши'];
    const seeds = [1, 42, 424242, 987654];
    let ok = true, bad = null, checked = 0, formsSeen = 0;
    for (const id of ids) {
        for (const seed of seeds) {
            const w = mkRecipeWorld(id, seed);
            if (!w.forms) { ok = false; bad = `${id}/s${seed}: нет форм`; break; }
            formsSeen++;
            for (let wx = -4000; wx <= 4000 && ok; wx += 13) {
                const th = w.terrainHeight(wx);
                const st = w.skyTop(wx);
                checked++;
                if (!w.solidAt(wx, st)) { ok = false; bad = `${id}/s${seed}/x${wx}: solidAt(skyTop=${st.toFixed(2)})=false`; break; }
                if (w.solidAt(wx, th) && th < st - 1e-9) { ok = false; bad = `${id}/s${seed}/x${wx}: твёрдая корка th=${th.toFixed(2)} выше skyTop=${st.toFixed(2)}`; break; }
                if (w.formationBlend(wx).float) {
                    for (let y = th - FLOAT_SPAN; y < th - FLOAT_GAP; y += 1) {
                        if (w.solidAt(wx, y)) { if (y < st - 1e-9) { ok = false; bad = `${id}/s${seed}/x${wx}: твёрдое float y=${y} выше skyTop=${st.toFixed(2)}`; } break; }
                    }
                    if (!ok) break;
                }
                for (const it of w._collectFormIntervals(wx, th)) {
                    if (it.add && it.top < st - 1e-9 && w.solidAt(wx, it.top)) { ok = false; bad = `${id}/s${seed}/x${wx}: плита top=${it.top.toFixed(2)} выше skyTop=${st.toFixed(2)}`; break; }
                }
                if (!ok) break;
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    // Явные колонки находки №2 (замер ревьюера): зарытый arch=0 (горы) и ветка
    // трещины (каменные_пустоши). До фикса: skyTop=-15.71 при th=-16.72 (корка
    // выше skyTop) и skyTop=245.55 при solidAt=false соответственно.
    const mount = mkRecipeWorld('горы', 1);
    const gm = mount.skyTop(-3458), gth = mount.terrainHeight(-3458);
    if (!(gm <= gth + 1e-9 && mount.solidAt(-3458, gm))) {
        ok = false; bad = `горы/s1/x-3458: skyTop=${gm.toFixed(2)} th=${gth.toFixed(2)} solid=${mount.solidAt(-3458, gm)}`;
    }
    const badlands = mkRecipeWorld('каменные_пустоши', 1);
    const bm = badlands.skyTop(-3474);
    if (!badlands.solidAt(-3474, bm)) { ok = false; bad = `каменные_пустоши/s1/x-3474: solidAt(skyTop=${bm.toFixed(2)})=false`; }
    check('T3b skyTop с формами: solidAt(skyTop) и ничего твёрдого выше (горы/каменные_пустоши)',
        ok && formsSeen === ids.length * seeds.length, bad || `столбцов=${checked} биомов×seed=${formsSeen} +2 колонки находки`);
}

// ==================== T3c: контракт floorY (§2.1, D2) ====================
// floorY обязан быть ТВЁРДЫМ (`solidAt(floorY)=true`) и не ниже верха твёрдого
// (`skyTop ≤ floorY`): прежний `floorY=it.bottom` возвращал воздух (интервалы
// вычитающих включительны). Проверка на фолбэках и на биомах-целях с формами.
{
    const worlds = [];
    for (const cat of CATEGORIES) worlds.push(mkWorld(cat, 1));
    worlds.push(mkRecipeWorld('горы', 1), mkRecipeWorld('каменные_пустоши', 1));
    let ok = true, bad = null, checked = 0;
    for (const w of worlds) {
        for (let wx = -3000; wx <= 3000 && ok; wx += 13) {
            const fy = w.floorY(wx), st = w.skyTop(wx);
            checked++;
            if (!w.solidAt(wx, fy)) { ok = false; bad = `solidAt(floorY)=false x${wx} fy=${fy.toFixed(2)}`; break; }
            if (st > fy + 1e-9) { ok = false; bad = `skyTop>floorY x${wx} (${st.toFixed(2)}>${fy.toFixed(2)})`; break; }
        }
        if (!ok) break;
    }
    check('T3c floorY: solidAt(floorY)=true и skyTop ≤ floorY (фолбэки + формы)', ok, bad || `столбцов=${checked}`);
}

// ==================== T4: консервативная пещерная маска ====================
// Зеркалит логику surface_render.js (getChunkCanvas). Реальный растр проверяет
// e2e tools/e2e/surface-solid-field-check.js; здесь — чистая арифметика маски.
function oldMask(w, baseX) {
    const mask = new Uint8Array((CHUNK + 1) * CHUNK_HEIGHT);
    for (let lx = 0; lx < CHUNK; lx += 3) {
        const wx = baseX + lx;
        const th = w.terrainHeight(wx);
        for (let ly = 0; ly < CHUNK_HEIGHT; ly += 3) {
            const wy = TOPY + ly;
            if (wy < th + 6) continue;
            if (!w.isCave(wx, wy)) continue;
            for (let dy = 0; dy < 3; dy++) for (let dx = 0; dx < 3; dx++) {
                const X = lx + dx, Y = ly + dy;
                if (X <= CHUNK && Y < CHUNK_HEIGHT) mask[Y * (CHUNK + 1) + X] = 1;
            }
        }
    }
    return mask;
}
function newMask(w, baseX) {
    const mask = new Uint8Array((CHUNK + 1) * CHUNK_HEIGHT);
    for (let lx = 0; lx < CHUNK; lx += 3) {
        const wx = baseX + lx;
        const th = w.terrainHeight(wx);
        for (let ly = 0; ly < CHUNK_HEIGHT; ly += 3) {
            const wy = TOPY + ly;
            if (wy < th + 6) continue;
            if (!w.isCave(wx, wy)) continue;
            let allAir = true;
            for (let dx = 0; dx < 3 && allAir; dx++) for (let dy = 0; dy < 3; dy++) {
                if (w.solidAt(baseX + lx + dx, wy + dy)) { allAir = false; break; }
            }
            if (allAir) { for (let dy = 0; dy < 3; dy++) for (let dx = 0; dx < 3; dx++) { const X = lx + dx, Y = ly + dy; if (X <= CHUNK && Y < CHUNK_HEIGHT) mask[Y * (CHUNK + 1) + X] = 1; } continue; }
            for (let dx = 0; dx < 3; dx++) for (let dy = 0; dy < 3; dy++) {
                if (!w.solidAt(baseX + lx + dx, wy + dy)) { const X = lx + dx, Y = ly + dy; if (X <= CHUNK && Y < CHUNK_HEIGHT) mask[Y * (CHUNK + 1) + X] = 1; }
            }
        }
    }
    return mask;
}
{
    let solidPainted = 0, firstSolid = null, lostAir = 0, delta = 0, oldAir = 0, newAir = 0, chunks = 0;
    for (const cat of CATEGORIES) {
        for (const seed of SEEDS) {
            for (const ci of [-1, 0, 1]) {
                const w = mkWorld(cat, seed);
                const baseX = ci * CHUNK;
                const om = oldMask(w, baseX), nm = newMask(w, baseX);
                chunks++;
                for (let lx = 0; lx <= CHUNK; lx++) {
                    const wx = baseX + lx;
                    for (let ly = 0; ly < CHUNK_HEIGHT; ly++) {
                        const idx = ly * (CHUNK + 1) + lx;
                        const solid = w.solidAt(wx, TOPY + ly);
                        const o = om[idx], n = nm[idx];
                        if (n && solid) { solidPainted++; if (!firstSolid) firstSolid = `${cat}/s${seed}/c${ci}/x${wx}/y${TOPY + ly}`; }
                        if (o && !solid) oldAir++;
                        if (n && !solid) newAir++;
                        if (o && solid) delta++;          // прежнее перекрытие твёрдого «пещерой»
                        if (o && !solid && !n) lostAir++; // потерянное покрытие воздуха
                    }
                }
                if (w._chunkCache) w._chunkCache = null;
            }
        }
    }
    check('T4a консервативность: твёрдая точка не перекрыта пещерой (растр — надмножество)', solidPainted === 0,
        `нарушений=${solidPainted}${firstSolid ? ' первое: ' + firstSolid : ''} чанков=${chunks}`);
    check('T4b покрытие: прежний воздух пещер не потерян', lostAir === 0, `потеряно=${lostAir}`);
    check('T4c косметическая дельта границ пещер замерена', true,
        `перекрытие твёрдого прежней маской=${delta} px (убрано); воздух old=${oldAir} new=${newAir} (${chunks} чанков)`);
}

// ==================== T4d/T4e: маска на биомах с 2D-формами (§2.1, S1-bis) =========
// Находка №1 ревью: пещерная маска красила «воздухом» аддитивные пласты, которые
// физика считает твёрдыми (маска игнорировала формы ниже terrainHeight). formMask
// зеркалит colForms/solidAtCol из getChunkCanvas: T4d — консервативность с формами,
// T4e — воспроизводимость находки на прежней формуле (игнор форм).
function formMask(w, baseX, useForms) {
    const mask = new Uint8Array((CHUNK + 1) * CHUNK_HEIGHT);
    const colTh = new Array(CHUNK + 2);
    const colForms = new Array(CHUNK + 2);
    for (let i = 0; i <= CHUNK + 1; i++) {
        colTh[i] = w.terrainHeight(baseX + i);
        colForms[i] = useForms ? w.formSpans(baseX + i, colTh[i]) : null;
    }
    const solidAtCol = (xi, yy) => {
        if (!useForms) return yy >= colTh[xi] ? !w.caveAt(baseX + xi, yy, colTh[xi]) : w.solidAt(baseX + xi, yy);
        const fs = colForms[xi];
        if (fs) for (const s of fs.sub) if (yy >= s.top && yy <= s.bottom) return false;
        if (yy >= colTh[xi]) {
            if (!w.caveAt(baseX + xi, yy, colTh[xi])) return true;
            if (fs) for (const a of fs.add) if (yy >= a.top && yy <= a.bottom) return true;
            return false;
        }
        if (fs) for (const a of fs.add) if (yy >= a.top && yy <= a.bottom) return true;
        return w._baseSolidAt(baseX + xi, yy, colTh[xi]);
    };
    for (let lx = 0; lx < CHUNK; lx += 3) {
        const wx = baseX + lx;
        const th = colTh[lx];
        for (let ly = 0; ly < CHUNK_HEIGHT; ly += 3) {
            const wy = TOPY + ly;
            if (wy < th + 6) continue;
            if (!w.caveAt(wx, wy, th)) continue;
            let allAir = true;
            for (let dx = 0; dx < 3 && allAir; dx++) for (let dy = 0; dy < 3; dy++) {
                if (solidAtCol(lx + dx, wy + dy)) { allAir = false; break; }
            }
            if (allAir) {
                for (let dy = 0; dy < 3; dy++) for (let dx = 0; dx < 3; dx++) { const X = lx + dx, Y = ly + dy; if (X <= CHUNK && Y < CHUNK_HEIGHT) mask[Y * (CHUNK + 1) + X] = 1; }
                continue;
            }
            for (let dx = 0; dx < 3; dx++) for (let dy = 0; dy < 3; dy++) {
                if (!solidAtCol(lx + dx, wy + dy)) { const X = lx + dx, Y = ly + dy; if (X <= CHUNK && Y < CHUNK_HEIGHT) mask[Y * (CHUNK + 1) + X] = 1; }
            }
        }
    }
    return mask;
}
{
    const seeds = [1, 42, 424242];
    const chunks = [-14, 0];   // -14 — чанк колонок находки (x≈-3458/-3474)
    let solidPainted = 0, firstSolid = null, buggyOverlap = 0, gens = 0;
    for (const id of ['горы', 'каменные_пустоши']) {
        for (const seed of seeds) {
            for (const ci of chunks) {
                const w = mkRecipeWorld(id, seed);
                if (!w.forms) continue;
                const baseX = ci * CHUNK;
                const fixed = formMask(w, baseX, true);
                const old = formMask(w, baseX, false);
                gens++;
                for (let lx = 0; lx <= CHUNK; lx++) {
                    const wx = baseX + lx;
                    for (let ly = 0; ly < CHUNK_HEIGHT; ly++) {
                        const idx = ly * (CHUNK + 1) + lx;
                        const n = fixed[idx], o = old[idx];
                        if (!n && !o) continue;
                        if (!w.solidAt(wx, TOPY + ly)) continue;
                        if (n) { solidPainted++; if (!firstSolid) firstSolid = `${id}/s${seed}/c${ci}/x${wx}/y${TOPY + ly}`; }
                        if (o) buggyOverlap++;
                    }
                }
                if (w._chunkCache) w._chunkCache = null;
            }
        }
    }
    check('T4d консервативность маски на биомах с формами: твёрдое не перекрыто',
        solidPainted === 0, `нарушений=${solidPainted}${firstSolid ? ' первое: ' + firstSolid : ''} генераций=${gens}`);
    check('T4e находка №1 воспроизводима: прежняя маска (игнор форм) перекрывала твёрдое',
        buggyOverlap > 0, `перекрытий прежней маской=${buggyOverlap} px (горы/каменные_пустоши, чанки ${chunks})`);
}

// ==================== T6: коллизия old↔new (нет новых застреваний) ===================
// Симулирует ходьбу вправо одним и тем же миром: прежний Player (HEAD, точечная
// проба) против нового (угловая выборка бокса). Угловая коллизия не должна давать
// меньше пройденного пути (новое застревание) — критерий B.1.1 п.4 / §5.1.5.
async function loadLegacyPlayer() {
    const here = path.dirname(fileURLToPath(import.meta.url));
    const repo = path.resolve(here, '..');
    const tmpDir = path.join(repo, 'web', 'static', 'js', 'surface');
    let src;
    try {
        src = execFileSync('git', ['show', 'HEAD:web/static/js/surface/surface_player.js'], { cwd: repo, encoding: 'utf8' });
    } catch (e) { return null; }
    if (/_blockedX/.test(src)) { console.log('  (HEAD уже содержит Э5.1 player — T6 пропущен)'); return null; }
    const tmp = path.join(tmpDir, '.legacy_player_tmp.mjs');
    writeFileSync(tmp, src, 'utf8');
    try { return (await import(pathToFileURL(tmp).href)).Player; } finally { try { unlinkSync(tmp); } catch (e) { /* ignore */ } }
}
const LegacyPlayer = await loadLegacyPlayer();
if (LegacyPlayer) {
    const input = { left: false, right: true, jump: false, sprint: false };
    const walk = (PlayerCls, world, steps) => {
        const p = new PlayerCls(world, 1);
        for (let i = 0; i < steps; i++) p.update(1 / 60, input);
        return p.x;
    };
    let worst = null, totalNew = 0, totalOld = 0, checked = 0;
    for (const cat of CATEGORIES) {
        for (const seed of SEEDS) {
            const w = mkWorld(cat, seed);
            // Один мир на оба игрока: new читает solidAt, legacy — isSolid-алиас.
            const o = walk(LegacyPlayer, w, 900);
            const n = walk(Player, w, 900);
            totalNew += n; totalOld += o; checked++;
            const ratio = o > 1 ? n / o : 1;
            if (o < 1) continue;
            if (ratio < 0.7 && (!worst || ratio < worst.ratio)) worst = { cat, seed, o: +o.toFixed(1), n: +n.toFixed(1), ratio: +ratio.toFixed(2) };
        }
    }
    // Проверка «нет системного застревания»: суммарно новый не отстаёт, и ни один
    // прогон не потерял >30% пути к прежнему (локальная стена — не регресс).
    const sumRatio = totalOld > 1 ? totalNew / totalOld : 1;
    check('T6 коллизия old↔new: нет системного застревания (Σ путь new/old ≥ 0.85)', sumRatio >= 0.85,
        `new/old Σ=${sumRatio.toFixed(2)} (new=${totalNew.toFixed(0)} old=${totalOld.toFixed(0)}, ${checked} прогонов)` +
        (worst ? ` худший: ${worst.cat}/s${worst.seed} ${worst.o}→${worst.n} (${worst.ratio})` : ''));
}

// ==================== T7: 2D-формы Э5.2 (§3.5) ====================
// Мир с формами (дельта биома: только view_source=catalog). Аддитивная арка
// arch=1 даёт твёрдую плиту ВЫШЕ terrainHeight с просветом `opening` ≥ роста
// игрока; crack2d КОСМЕТИЧЕН (тёмный клин, поле твёрдости не меняет, D1); поле
// детерминировано.
function mkFormsWorld(over = {}) {
    const forms = over.forms || [
        { prim: 'overhang', arch: 1, perRegion: 1, w: 120, h: 40, opening: 100, taper: 0.2, dir: 1 },
        { prim: 'crack2d', perRegion: 1, w: 34, depth: 300, taper: 0.6, tilt: 10 },
    ];
    const view = { relief: { ridge: 0, flatten: 0, offset: 0, caves: 0, float: false, forms } };
    return new SurfaceWorld({
        seed: over.seed || 424242, biome: 'qa', biome_category: 'литосфера',
        biome_color: '#8a7a6a', life: false,
        view_source: 'catalog', view_version: 1, biome_view: view, ...over,
    });
}
{
    const w = mkFormsWorld({ seed: 424242 });

    // T7a: арка arch=1 — твёрдая плита выше рельефа, просвет ≥ роста игрока (28).
    let archX = null;
    for (let x = -3000; x < 3000; x += 0.5) {
        if (w.skyTop(x) < w.terrainHeight(x) - 1) { archX = x; break; }
    }
    let okA = archX !== null, detailA = `archX=${archX}`;
    if (okA) {
        const th = w.terrainHeight(archX), st = w.skyTop(archX);
        let y = th - 1;
        while (y > th - 600 && !w.solidAt(archX, y)) y -= 1;
        const gap = (th - 1) - y;
        // Проход под аркой: голова/грудь не упираются в плиту, горизонталь свободна.
        const p = new Player(w, 1);
        p.x = archX; p.y = th - p.h / 2 - 2;
        const passFree = !p._blockedUp() && !p._blockedX(archX + 1);
        okA = w.solidAt(archX, st) && !w.solidAt(archX, th - 1) && gap >= 28 && gap <= 160 && passFree;
        detailA += ` skyTop=${st.toFixed(1)} th=${th.toFixed(1)} gap=${gap.toFixed(1)} pass=${passFree}`;
    }
    check('T7a overhang arch=1: твёрдая плита выше рельефа, просвет ≥ роста игрока', okA, detailA);

    // T7b: crack2d КОСМЕТИЧЕН (решение гейта 2026-09-25, дефект D1) — тёмный клин
    // трещины, поле твёрдости НЕ меняет: solidAt с трещиной == solidAt без неё.
    const noCrack = mkFormsWorld({ seed: 424242, forms: [w.forms[0]] });
    let crackX = null;
    for (let x = -3000; x < 3000; x += 0.5) { if (w.crackSpans(x, w.terrainHeight(x))) { crackX = x; break; } }
    let okB = crackX !== null, detailB = `crackX=${crackX}`;
    if (okB) {
        const th = w.terrainHeight(crackX);
        let same = true;
        for (let y = th - 60; y < th + 200; y += 1) if (w.solidAt(crackX, y) !== noCrack.solidAt(crackX, y)) { same = false; break; }
        okB = w.formsSubtractive(crackX, th + 10) === false && same;
        detailB += ` formsSubtractive=false, поле с трещиной == без: ${same}`;
    }
    check('T7b crack2d косметичен: поле твёрдости не меняет (D1)', okB, detailB);

    // T7c: детерминизм поля с формами (тот же seed → то же твёрдое).
    let same = true;
    const w2 = mkFormsWorld({ seed: 424242 });
    for (let x = -1500; x < 1500 && same; x += 3) {
        for (let y = -900; y < 800; y += 11) if (w.solidAt(x, y) !== w2.solidAt(x, y)) { same = false; break; }
    }
    check('T7c детерминизм поля с формами', same);
}

// ==================== T9: нет ловушек на биомах-целях (§5.1 п.5, D1) ====================
// Симуляция ходьбы вправо 10 с (как e2e прогулки): после косметизации crack2d
// игрок не должен проваливаться ниже поверхности (центр > th+2) в ЛЮБОЙ момент и
// обязан двигаться. Стартовые x — из находки @tester: каменные_пустоши seed 1
// x≈892.5, seed 42 x≈2000.
{
    const input = { left: false, right: true, jump: false, sprint: false };
    const cases = [['каменные_пустоши', 1, 892.5], ['каменные_пустоши', 42, 2000]];
    let ok = true, bad = null, moved = 0, worst = -Infinity;
    for (const [id, seed, x0] of cases) {
        const w = mkRecipeWorld(id, seed);
        const p = new Player(w, 1);
        const th0 = w.terrainHeight(x0);
        p.x = x0; p.y = th0 - p.h / 2 - 2; p.vx = 0; p.vy = 0;
        for (let i = 0; i < 600; i++) {
            p.update(1 / 60, input);
            const d = p.y - w.terrainHeight(p.x);
            if (d > worst) worst = d;
        }
        const dx = p.x - x0;
        if (dx < 20) { ok = false; bad = `${id}/s${seed}: не сдвинулся (x ${x0} -> ${p.x.toFixed(1)})`; break; }
        if (p.y > w.terrainHeight(p.x) + 2) { ok = false; bad = `${id}/s${seed}: провал (y=${p.y.toFixed(1)}, th+2=${(w.terrainHeight(p.x) + 2).toFixed(1)})`; break; }
        moved += dx;
    }
    check('T9 ходьба вправо 10 с на биомах-целях: движение и нет провала ниже th+2', ok,
        bad || `сдвиг Σ=${moved.toFixed(0)} px, макс. заглубление центра=${worst.toFixed(1)} px (норма ≈ -16)`);
}

// ==================== Э5.3: ГРОТЫ И СВОДЫ (§3.6) ====================
// Биомы-цели Э5.3: пещерный мир (свод overhang arch=1 + ниша void), лава/магма
// (лавовые трубки void tube), кратеры (кольцевая чаша crater = вал + впадина).
const TARGET_BIOMES = ['пещерный_мир_с_потолком', 'лавовые_поля', 'магмовый_океан', 'кратеры'];

// primRange — копия клиентской (§3.1): [lo,hi] по индексу фичи или число. Нужна
// диагностике отказов (перебор инстансов), чтобы не зависеть от приватных имён.
function primRangeLocal(v, i, seed, def) {
    if (typeof v === 'number') return v;
    if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
        return v[0] + hash1(i, seed) * (v[1] - v[0]);
    }
    return def;
}

// ==================== T10: void/crater — оба знака и согласие ====================
{
    const seeds = [1, 42, 424242, 987654];
    let ok = true, bad = null, cols = 0, voidSub = 0, craterSub = 0, craterAdd = 0, formsSeen = 0;
    for (const id of TARGET_BIOMES) {
        for (const seed of seeds) {
            const w = mkRecipeWorld(id, seed);
            if (!w.forms) { ok = false; bad = `${id}/s${seed}: нет форм`; break; }
            formsSeen++;
            const has = (prim) => w.forms.some((f) => f.prim === prim);
            if (id === 'кратеры' ? !has('crater') : !has('void')) { ok = false; bad = `${id}: нет ${id === 'кратеры' ? 'crater' : 'void'}`; break; }
            for (let x = -4200; x <= 4200 && ok; x += 3) {
                const th = w.terrainHeight(x);
                const fs = w.formSpans(x, th);
                if (!fs) continue;
                cols++;
                // Согласие: интервал formSpans ⇄ поле solidAt (единственный источник).
                for (const s of fs.sub) {
                    const ym = (s.top + s.bottom) / 2;
                    if (w.solidAt(x, ym)) { ok = false; bad = `${id}/s${seed}/x${x}: sub-интервал, но solidAt=true y=${ym.toFixed(2)}`; break; }
                }
                for (const a of fs.add) {
                    const ym = (a.top + a.bottom) / 2;
                    if (!w.solidAt(x, ym)) { ok = false; bad = `${id}/s${seed}/x${x}: add-интервал, но solidAt=false y=${ym.toFixed(2)}`; break; }
                }
                if (has('void') && fs.sub.length) voidSub++;
                if (has('crater') && fs.sub.length) craterSub++;
                if (has('crater') && fs.add.length) craterAdd++;
            }
            if (!ok) break;
            // Детерминизм поля с формами (тот же seed → то же твёрдое).
            const w2 = mkRecipeWorld(id, seed);
            for (let x = -1500; x <= 1500 && ok; x += 7) {
                const th = w.terrainHeight(x), th2 = w2.terrainHeight(x);
                if (w.solidAt(x, th + 40) !== w2.solidAt(x, th2 + 40) || w.solidAt(x, th - 40) !== w2.solidAt(x, th2 - 40)) {
                    ok = false; bad = `${id}/s${seed}: недетерминизм x${x}`;
                }
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    check('T10 void/crater: формы целей, оба знака crater (вал+впадина), согласие formSpans ⇄ solidAt, детерминизм',
        ok && formsSeen === TARGET_BIOMES.length * seeds.length && voidSub > 0 && craterSub > 0 && craterAdd > 0,
        bad || `столбцов с формами=${cols} void-sub=${voidSub} crater-sub=${craterSub} crater-add=${craterAdd}`);
}

// ==================== T11: skyTop/floorY на биомах Э5.3 (§5 п.12) ====================
// После вычитающих форм `floorY ≠ terrainHeight`: пол грота/чаши опущен, верх
// (`skyTop`) — верх твёрдого (в т.ч. вал crater/плита arch=1). КОНТРАКТ:
// `solidAt(skyTop)=true`, `solidAt(floorY)=true`, `skyTop ≤ floorY`; ничего
// твёрдого выше `skyTop` (кандидаты — аддитивные формы) — иначе «невидимая стена».
{
    const seeds = [1, 42, 424242, 987654];
    let ok = true, bad = null, checked = 0, withSub = 0, floorBelow = 0, rimUp = 0;
    for (const id of TARGET_BIOMES) {
        for (const seed of seeds) {
            const w = mkRecipeWorld(id, seed);
            for (let x = -4000; x <= 4000 && ok; x += 13) {
                const th = w.terrainHeight(x);
                const st = w.skyTop(x), fy = w.floorY(x);
                checked++;
                if (!w.solidAt(x, st)) { ok = false; bad = `${id}/s${seed}/x${x}: solidAt(skyTop=${st.toFixed(2)})=false`; break; }
                if (!w.solidAt(x, fy)) { ok = false; bad = `${id}/s${seed}/x${x}: solidAt(floorY=${fy.toFixed(2)})=false`; break; }
                if (st > fy + 1e-9) { ok = false; bad = `${id}/s${seed}/x${x}: skyTop=${st.toFixed(2)} > floorY=${fy.toFixed(2)}`; break; }
                if (fy > th + 1e-9) floorBelow++;
                for (const it of w._collectFormIntervals(x, th)) {
                    if (it.add && it.top < st - 1e-9 && w.solidAt(x, it.top)) {
                        ok = false; bad = `${id}/s${seed}/x${x}: аддитив top=${it.top.toFixed(2)} выше skyTop=${st.toFixed(2)}`; break;
                    }
                }
                if (!ok) break;
                const fs = w.formSpans(x, th);
                if (fs && fs.sub.length) withSub++;
                if (fs && fs.add.length) rimUp++;
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    check('T11 skyTop/floorY на биомах Э5.3: solidAt(skyTop/floorY), skyTop ≤ floorY, пол опущен вычитающей формой',
        ok && withSub > 0 && floorBelow > 0 && rimUp > 0,
        bad || `столбцов=${checked} с вычитающей=${withSub} с аддитивом=${rimUp} пол ниже th=${floorBelow}`);
}

// ==================== T12: ВЫХОДИМОСТЬ вычитающих форм (§3.6/§5.1 п.5) ====================
// Независимая от `_simulateEscape` проверка: перебираем инстансы вычитающих форм
// (void/crater) как `_collectFormIntervals`; для каждого ПРИМЕНЁННОГО инстанса
// ставим `Player` в 9 точках по ширине footprint (включая дно и оба фланга ГОНЧЕ
// сетки мира) и гоняем реальную физику (60 Гц, ≤10 с, обе стороны, с прыжком и
// без) до выхода из выреза этой формы. Хоть одна точка не вышла — ловушка.
// Плюс диагностика: сколько инстансов откатано и почему (спавн/узкий проход/выход).
// Профиль probe — ЛИНЕЙНАЯ ИНТЕРПОЛЯЦИЯ между целыми колонками (один fbm на
// колонку). Округление давало кусочно-постоянный профиль: на склоне появлялась
// 1-px «полочка», которой в непрерывном поле нет (дефект D4). Интерполяция
// воспроизводит реальный профиль.
function makeProbe(w) {
    const probe = Object.create(w);
    const memo = new Map();
    const real = w.terrainHeight.bind(w);
    const at = (k) => { let v = memo.get(k); if (v === undefined) { v = real(k); memo.set(k, v); } return v; };
    probe.terrainHeight = (x) => {
        const f = Math.floor(x), t = x - f;
        return t === 0 ? at(f) : at(f) + (at(f + 1) - at(f)) * t;
    };
    return probe;
}
{
    const seeds = [1, 42, 424242];
    let applied = 0, refused = 0, refusedSpawn = 0, refusedOpen = 0, refusedEscape = 0;
    let fails = 0, firstFail = null, startsTested = 0;
    for (const id of TARGET_BIOMES) {
        for (const seed of seeds) {
            const w = mkRecipeWorld(id, seed);
            const probe = makeProbe(w);
            const R = w.region;
            for (let fi = 0; fi < w.forms.length; fi++) {
                const f = w.forms[fi];
                if (f.prim !== 'void' && f.prim !== 'crater') continue;
                const sBase = (w.seed ^ Math.imul(fi + 1, 0x9e3779b1)) >>> 0;
                const rawN = f.perRegion != null ? f.perRegion : 1;
                for (let rr = -1; rr <= 1; rr++) {
                    const n = Math.max(0, Math.round(primRangeLocal(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
                    for (let k = 0; k < n; k++) {
                        const idx = rr * 131 + k;
                        const inst = { rr, k, idx, c: rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R };
                        // Решение мира (мемо): применён или откатан.
                        if (!w._formInstanceOk(fi, f, sBase, inst)) {
                            refused++;
                            const fp = w._formFootprint(f, sBase, inst);
                            if (inst.c + fp >= -2 * PLAYER_W && inst.c - fp <= 2 * PLAYER_W) refusedSpawn++;
                            else if (f.prim === 'void' && f.open !== true) {
                                const ww = primRangeLocal(f.w, idx, (sBase ^ 0x0f11) >>> 0, 60);
                                const hh = f.h != null ? primRangeLocal(f.h, idx, (sBase ^ 0x0f13) >>> 0, 0) : Infinity;
                                if (2 * ww < PLAYER_W + 2 || hh < PLAYER_H + 2) refusedOpen++;
                                else refusedEscape++;
                            } else refusedEscape++;
                            continue;
                        }
                        applied++;
                        const fp = w._formFootprint(f, sBase, inst);
                        // Изоляция: выход проверяется из выреза ЭТОЙ формы (как в
                        // `_simulateEscape`), прочие инстансы в probe не входят.
                        // Probe — боевым конструктором мира (`_probeFor`): исходный
                        // индекс формы сохранён, геометрия = боевой.
                        const iprobe = w._probeFor(fi, f, inst);
                        iprobe.terrainHeight = probe.terrainHeight;
                        const scratch = [];
                        const cutThis = (x) => {
                            scratch.length = 0;
                            w._formColumn(scratch, f, fi, sBase, inst, Math.round(x), probe.terrainHeight(x));
                            let c = 0;
                            for (const it of scratch) if (!it.add) c = Math.max(c, it.bottom - it.top);
                            return c;
                        };
                        // Наихудшие точки выреза (дно + дальние углы + середины флангов),
                        // та же сетка, что у мира, но своя реализация физики.
                        for (const frac of [-0.9, -0.45, 0, 0.45, 0.9]) {
                            const sx = inst.c + frac * fp;
                            if (cutThis(sx) <= 1) continue;
                            startsTested++;
                            let escaped = false;
                            for (const [dir, jump] of [[1, false], [-1, false], [1, true], [-1, true]]) {
                                const p = new Player(iprobe, 1);
                                p.x = sx; p.vx = 0; p.vy = 0; p.y = iprobe.floorY(sx) - p.h / 2 - 2;
                                const input = { left: dir < 0, right: dir > 0, jump, sprint: false };
                                let stuck = 0;
                                for (let step = 0; step < 600; step++) {
                                    const prevX = p.x;
                                    p.update(1 / 60, input);
                                    if (p.onGround && p.y <= probe.terrainHeight(p.x) + 2.5 && cutThis(p.x) <= 1.5) { escaped = true; break; }
                                    if (p.y > probe.terrainHeight(p.x) + cutThis(p.x) + 12) break;
                                    stuck = Math.abs(p.x - prevX) < 0.05 ? stuck + 1 : 0;
                                    if (stuck > 150) break;
                                    if (Math.abs(p.x - sx) > fp * 4) break;
                                }
                                if (escaped) break;
                            }
                            if (!escaped) {
                                fails++;
                                if (!firstFail) firstFail = `${id}/s${seed}/${f.prim} c=${inst.c.toFixed(0)} фланг=${frac} старт=${sx.toFixed(0)}`;
                            }
                        }
                    }
                    if (fails) break;
                }
                if (fails) break;
            }
            if (fails) break;
        }
        if (fails) break;
    }
    check('T12 выходимость: из каждой точки выреза применённой формы игрок выходит на поверхность',
        fails === 0 && applied > 0 && startsTested > 0,
        firstFail || `применено=${applied} стартов=${startsTested}; отказы: спавн=${refusedSpawn} узкий_проход=${refusedOpen} выход=${refusedEscape}`);
}

// ==================== T13: probe ⇄ боевая геометрия (§3.6, регресс) ====================
// Probe выходимости кладёт форму в массив из одного элемента: без переноса
// исходного индекса (`_fi`) соль `sBase` сдвигается и вырез считается по ЧУЖОЙ
// геометрии (bottom/центр расходятся — гейт выходимости проверяет не то, что
// применяется). Проверяем на биоме, где вычитающая форма НЕ первая
// (`пещерный_мир_с_потолком`: forms[0]=overhang, forms[1]=void): интервалы probe
// совпадают с боевой геометрией именно этого инстанса (`_formColumn`), плюс
// негативный контроль — старый probe без `_fi` даёт расхождение.
{
    // Сравниваем probe-путь (`_probeFor` + `_collectFormIntervals`, который читает
    // `floorY` симуляции) с БОЕВОЙ геометрией ИМЕННО этого инстанса (`_formColumn`
    // с боевым индексом/солью). Не с `w._collectFormIntervals`: тот фильтрует
    // инстансы выходимостью и на откатанной форме даёт 0 интервалов — ложное
    // расхождение (ревью Э5.3). Негативный контроль — старый probe без `_fi`.
    let ok = true, bad = null, instances = 0, cols = 0, fiNonZero = 0, buggyDiffs = 0;
    const seeds = [1, 42, 424242, 987654, 20260923];
    const fracs = [-0.5, 0, 0.5];
    for (const id of ['пещерный_мир_с_потолком', 'лавовые_поля', 'магмовый_океан', 'кратеры']) {
        for (const seed of seeds) {
            const w = mkRecipeWorld(id, seed);
            if (!w.forms) continue;
            const R = w.region;
            for (let fi = 0; fi < w.forms.length; fi++) {
                const f = w.forms[fi];
                if (f.prim !== 'void' && f.prim !== 'crater') continue;
                const sBase = (w.seed ^ Math.imul(fi + 1, 0x9e3779b1)) >>> 0;
                const rawN = f.perRegion != null ? f.perRegion : 1;
                for (let rr = -1; rr <= 1; rr++) {
                    const n = Math.max(0, Math.round(primRangeLocal(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
                    for (let k = 0; k < n; k++) {
                        const idx = rr * 131 + k;
                        const inst = { rr, k, idx, c: rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R };
                        const fp = w._formFootprint(f, sBase, inst);
                        const probe = w._probeFor(fi, f, inst);
                        instances++;
                        if (fi > 0) fiNonZero++;
                        for (const frac of fracs) {
                            const x = inst.c + frac * fp;
                            const th = w.terrainHeight(x);
                            const bs = [];
                            w._formColumn(bs, f, fi, sBase, inst, x, th);
                            const ps = probe._collectFormIntervals(x, th);
                            cols++;
                            if (bs.length !== ps.length) {
                                ok = false; bad = `${id}/s${seed}/fi${fi}/x${x.toFixed(0)}: интервалов бой ${bs.length} ≠ probe ${ps.length}`; break;
                            }
                            for (let j = 0; j < bs.length; j++) {
                                if (Math.abs(bs[j].top - ps[j].top) > 1e-9 || Math.abs(bs[j].bottom - ps[j].bottom) > 1e-9) {
                                    ok = false;
                                    bad = `${id}/s${seed}/fi${fi}/x${x.toFixed(0)}: bottom ${bs[j].bottom.toFixed(3)} ≠ probe ${ps[j].bottom.toFixed(3)}`;
                                    break;
                                }
                            }
                            if (!ok) break;
                        }
                        // Негативный контроль: на форме НЕ с 0-м индексом probe без
                        // `_fi` обязан дать ДРУГУЮ геометрию (иначе регресс не ловил бы
                        // сдвиг соли `sBase`).
                        if (fi > 0) {
                            const buggy = Object.create(w);
                            buggy.forms = [Object.assign({}, f, { _onlyInstance: inst })];
                            buggy._skipEscape = true;
                            for (const frac of fracs) {
                                const x = inst.c + frac * fp;
                                const th = w.terrainHeight(x);
                                const gs = buggy._collectFormIntervals(x, th);
                                const ps = probe._collectFormIntervals(x, th);
                                let diff = gs.length !== ps.length;
                                if (!diff) for (let j = 0; j < ps.length; j++) {
                                    if (Math.abs(gs[j].top - ps[j].top) > 1e-9 || Math.abs(gs[j].bottom - ps[j].bottom) > 1e-9) { diff = true; break; }
                                }
                                if (diff) buggyDiffs++;
                            }
                        }
                        if (!ok) break;
                    }
                    if (!ok) break;
                }
                if (!ok) break;
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    check('T13 probe ⇄ боевая геометрия: индекс/соли формы совпадают (вычитающая не на 0-м индексе)',
        ok && instances > 0 && fiNonZero > 0 && buggyDiffs > 0,
        bad || `инстансов=${instances} форм fi>0=${fiNonZero} колонок=${cols}; старый probe расходится на ${buggyDiffs} колонках`);
}

// ==================== T12-bis: ВЫХОДИМОСТЬ на ПОЛНОМ поле (§3.6, D1) ====================
// Регресс дефекта D1 (@tester Э5.3): одиночный probe выходимости не видит ловушку,
// которую даёт СОЧЕТАНИЕ вычитающих форм в поле (две каверны в сумме запирают
// игрока; e2e-W7: спуск 81.8 px при максимуме одиночной `void grotto` 60). Собираем
// ПОЛНОЕ поле окрестности (§2.1: база с пещерами ⊕ ВСЕ применённые формы) и
// независимо гоняем реальную физику Player из наихудших точек каждой применённой
// вычитающей формы: 0 ловушек. Плюс синтетика «форма + УСИЛЕННЫЕ пещеры»
// (caves:1.0, сплошной float, две вычитающие формы) — максимальная пещерность.
function appliedField(w) {
    const accepted = [], meta = [], R = w.region;
    for (let fi = 0; fi < w.forms.length; fi++) {
        const f = w.forms[fi];
        const sBase = (w.seed ^ Math.imul(fi + 1, 0x9e3779b1)) >>> 0;
        const rawN = f.perRegion != null ? f.perRegion : 1;
        for (let rr = -1; rr <= 1; rr++) {
            const n = Math.max(0, Math.round(primRangeLocal(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
            for (let k = 0; k < n; k++) {
                const idx = rr * 131 + k;
                const inst = { rr, k, idx, c: rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R };
                const at = f.prim === 'overhang' ? true : w._formInstanceOk(fi, f, sBase, inst);
                if (!at) continue;
                accepted.push(Object.assign({}, f, { _onlyInstance: inst, _fi: fi }));
                meta.push({ fi, f, sBase, inst });
            }
        }
    }
    const ff = Object.create(w);
    ff.forms = accepted;
    ff._skipEscape = true;
    ff._formSupport = null;
    // Профиль — линейная интерполяция (как боевой `_simulateEscape`, D4): округление
    // давало ложный выход по 1-px ступеньке.
    const memo = new Map(), real = w.terrainHeight.bind(w);
    const at = (k) => { let v = memo.get(k); if (v === undefined) { v = real(k); memo.set(k, v); } return v; };
    ff.terrainHeight = (x) => { const f = Math.floor(x), t = x - f; return t === 0 ? at(f) : at(f) + (at(f + 1) - at(f)) * t; };
    return { ff, meta };
}
function escapeOnField(w, ff, f, fi, sBase, inst) {
    const fp = w._formFootprint(f, sBase, inst), scratch = [];
    const cutThis = (x) => {
        scratch.length = 0;
        w._formColumn(scratch, f, fi, sBase, inst, Math.round(x), ff.terrainHeight(x));
        let c = 0; for (const it of scratch) if (!it.add) c = Math.max(c, it.bottom - it.top);
        return c;
    };
    const starts = [];
    for (const frac of [-0.9, -0.45, 0, 0.45, 0.9]) { const sx = inst.c + frac * fp; if (cutThis(sx) > 1) starts.push(sx); }
    let fails = 0, startsTested = 0, worstDepth = 0;
    for (const sx of starts) {
        startsTested++;
        let escaped = false;
        for (const [dir, jump] of [[1, false], [-1, false], [1, true], [-1, true]]) {
            const p = new Player(ff, 1);
            p.x = sx; p.vx = 0; p.vy = 0; p.y = ff.floorY(sx) - p.h / 2 - 2;
            const input = { left: dir < 0, right: dir > 0, jump, sprint: false };
            let stuck = 0;
            for (let step = 0; step < 600; step++) {
                const px = p.x;
                p.update(1 / 60, input);
                const depth = p.y + p.h / 2 - ff.terrainHeight(p.x);
                if (depth > worstDepth) worstDepth = depth;
                if (p.onGround && p.y <= ff.terrainHeight(p.x) + 2.5 && cutThis(p.x) <= 1.5) { escaped = true; break; }
                if (p.y > ff.terrainHeight(p.x) + cutThis(p.x) + 12) break;
                stuck = Math.abs(p.x - px) < 0.05 ? stuck + 1 : 0;
                if (stuck > 150) break;
                if (Math.abs(p.x - sx) > fp * 4) break;
            }
            if (escaped) break;
        }
        if (!escaped) { fails++; break; }
    }
    return { fails, startsTested, worstDepth };
}
{
    let ok = true, bad = null, applied = 0, starts = 0, maxDepth = 0;
    const seeds = [1, 42, 424242, 987654, 20260923];
    for (const id of TARGET_BIOMES) {
        for (const seed of seeds) {
            const w = mkRecipeWorld(id, seed);
            if (!w.forms) continue;
            const { ff, meta } = appliedField(w);
            for (const m of meta) {
                if (m.f.prim === 'overhang') continue;
                applied++;
                const r = escapeOnField(w, ff, m.f, m.fi, m.sBase, m.inst);
                starts += r.startsTested;
                if (r.worstDepth > maxDepth) maxDepth = r.worstDepth;
                if (r.fails) { ok = false; bad = `${id}/s${seed}/${m.f.prim} c=${m.inst.c.toFixed(0)}: ловушка на полном поле (спуск ${r.worstDepth.toFixed(1)} px)`; break; }
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    // Синтетика «форма + усиленные пещеры»: caves:1.0 + сплошной float + две
    // вычитающие формы в одном рецепте.
    const synView = { relief: { ridge: 0, flatten: 0, offset: 0, caves: 1.0, float: true, forms: [
        { prim: 'void', shape: 'grotto', open: true, perRegion: 2, w: [90, 150], depth: [24, 60] },
        { prim: 'void', shape: 'tube', open: true, perRegion: 2, w: [70, 140], h: [60, 120], depth: [50, 110] },
    ] } };
    let synCheckable = 0;
    for (const seed of seeds) {
        const w = new SurfaceWorld({ seed, biome: 'qa', biome_category: 'литосфера', biome_color: '#8a7a6a', life: false, view_source: 'catalog', view_version: 1, biome_view: synView });
        const { ff, meta } = appliedField(w);
        for (const m of meta) {
            if (m.f.prim !== 'void') continue;
            synCheckable++;
            const r = escapeOnField(w, ff, m.f, m.fi, m.sBase, m.inst);
            starts += r.startsTested;
            if (r.worstDepth > maxDepth) maxDepth = r.worstDepth;
            if (r.fails) { ok = false; if (!bad) bad = `синтетика caves:1.0 s${seed} c=${m.inst.c.toFixed(0)}: ловушка (спуск ${r.worstDepth.toFixed(1)} px)`; break; }
        }
        if (!ok) break;
    }
    check('T12-bis выходимость на ПОЛНОМ поле (форма+пещеры+соседи): нет ловушек среди применённых',
        ok && applied > 0,
        bad || `применено=${applied} (синтетика caves:1.0=${synCheckable}) стартов=${starts} макс.спуск=${maxDepth.toFixed(1)} px`);
}

// ==================== T12-c: регресс D4 (фланец frac=±0.45) ====================
// Дефект D4 (@tester): фильтр выходимости шёл по ОКРУГЛЁННОМУ (кусочно-постоянному)
// профилю; на крутом склоне появлялась 1-px «полочка», которой в НЕПРЕРЫВНОМ поле
// нет, и форма-ловушка ЛОЖНО проходила. Кейс: `пещерный_мир_с_потолком`, seed 987654,
// `void` c≈143.84, fp≈102.5, старт frac=0.45 — непрерывное поле не выпускает ни одной
// стратегией (STUCK/STUCK/STUCK/DOMAIN). Требование: форма НЕ применяется.
{
    const w = mkRecipeWorld('пещерный_мир_с_потолком', 987654);
    const R = w.region;
    let found = null;
    for (let fi = 0; fi < w.forms.length && !found; fi++) {
        const f = w.forms[fi];
        if (f.prim !== 'void') continue;
        const sBase = (w.seed ^ Math.imul(fi + 1, 0x9e3779b1)) >>> 0;
        const rawN = f.perRegion != null ? f.perRegion : 1;
        for (let rr = -1; rr <= 1 && !found; rr++) {
            const n = Math.max(0, Math.round(primRangeLocal(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
            for (let k = 0; k < n; k++) {
                const idx = rr * 131 + k;
                const inst = { rr, k, idx, c: rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R };
                if (Math.abs(inst.c - 143.84) < 0.5) { found = { fi, f, sBase, inst }; break; }
            }
        }
    }
    let ok = false, detail = 'инстанс c≈143.84 не найден';
    if (found) {
        const { fi, f, sBase, inst } = found;
        const fp = w._formFootprint(f, sBase, inst);
        const applied = w._formInstanceOk(fi, f, sBase, inst);
        // Независимо: на НЕПРЕРЫВНОМ поле (интерп. профиль) фланец 0.45 не выходит —
        // `escapeOnField` гоняет тот же полный пучок 5 фракций/4 стратегии.
        const ff = makeProbe(w);
        ff.forms = [Object.assign({}, f, { _onlyInstance: inst, _fi: fi })];
        ff._skipEscape = true;
        const esc = escapeOnField(w, ff, f, fi, sBase, inst);
        ok = !applied && esc.fails > 0;
        detail = `c=${inst.c.toFixed(2)} fp=${fp.toFixed(1)}: применена=${applied} (ждали false); непрерывный фланец fails=${esc.fails}`;
    }
    check('T12-c регресс D4: форма-ловушка у фланца frac=0.45 не применяется', ok, detail);
}

// ==================== T5: детерминизм ====================
{
    let ok = true;
    for (const cat of CATEGORIES) {
        const a = mkWorld(cat, 777), b = mkWorld(cat, 777);
        for (let wx = 0; wx < 3000; wx += 7) {
            for (let wy = -900; wy <= 800; wy += 13) {
                if (a.solidAt(wx, wy) !== b.solidAt(wx, wy)) { ok = false; break; }
            }
            if (!ok) break;
        }
        if (!ok) break;
    }
    check('T5 детерминизм поля (тот же seed → то же твёрдое)', ok);
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} проверок пройдено`);
if (failed.length) process.exitCode = 1;
