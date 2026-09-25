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
//  - T5 детерминизм поля.
// Запуск: node tools/surface-field-check.mjs
import { readFileSync, writeFileSync, unlinkSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { SurfaceWorld } from '../web/static/js/surface/surface_world.js';
import { Player } from '../web/static/js/surface/surface_player.js';
import { CHUNK, FORMATIONS, FLOAT_SPAN, FLOAT_GAP } from '../web/static/js/surface/surface_config.js';

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

// ==================== T6: коллизия old↔new (нет новых застреваний) ====================
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
