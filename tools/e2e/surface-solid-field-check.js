// tools/e2e/surface-solid-field-check.js
// Э5.1 «Мир прогулки: скульптурный объём» — фундамент (спека 2026-09-25 §2/§5.1).
// Проверяет РЕАЛЬНЫЙ растр чанка (getChunkCanvas) против единого поля solidAt:
//   * «что твёрдо — то видно»: каждая твёрдая точка в кадре нарисована (alpha>0);
//   * сетка seed × биомы (6 категорий-фолбэков + резолвленные рецепты);
//   * замер стоимости генерации чанка (baseline для бюджета ≤2×);
//   * 0 pageerror.
// Запуск: node surface-solid-field-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
function findExecutable() { for (const p of CHROME_PATHS) if (existsSync(p)) return p; for (const p of EDGE_PATHS) if (existsSync(p)) return p; return null; }

const results = [];
function report(step, ok, detail) { results.push({ step, ok, detail }); console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`); }

// Резолв вида биома из рабочего файла справочника (та же семантика, что у сервера).
function mergeView(preset, delta) {
  const out = { ...preset };
  for (const k of Object.keys(delta)) {
    const pv = out[k], dv = delta[k];
    if (pv && typeof pv === 'object' && !Array.isArray(pv) && dv && typeof dv === 'object' && !Array.isArray(dv)) out[k] = mergeView(pv, dv);
    else out[k] = dv;
  }
  return out;
}
function resolveViews() {
  const cat = JSON.parse(readFileSync(new URL('../../config/biome_catalog.json', import.meta.url), 'utf8'));
  const ids = ['горы', 'каменные_пустоши', 'пески_пустыни', 'джунгли', 'лавовые_поля', 'пещерный_мир_с_потолком', 'ледники', 'кристальные_рощи'];
  const out = [];
  for (const id of ids) {
    const b = cat.biomes.find((x) => x.id === id);
    if (!b || !b.view) continue;
    const v = JSON.parse(JSON.stringify(b.view));
    const famId = v.family; delete v.family;
    let resolved = v;
    if (famId) {
      const fam = (cat.view_families || []).find((f) => f.id === famId);
      if (fam) { const base = JSON.parse(JSON.stringify(fam)); delete base.id; delete base.name; resolved = mergeView(base, v); }
    }
    out.push({ id, category: b.category, color: b.color || '#8a7a6a', view: resolved });
  }
  return out;
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER FOUND'); process.exit(1); }
  const views = resolveViews();
  const browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const page = await context.newPage();
    const pageErrors = [];
    page.on('pageerror', (e) => pageErrors.push(String(e)));
    await page.goto(BASE_URL + '/login-page', { waitUntil: 'domcontentloaded', timeout: 30000 });

    const data = await page.evaluate(async (views) => {
      const { SurfaceWorld } = await import('/static/js/surface/surface_world.js');
      const { getChunkCanvas, CHUNK_TOP_MARGIN, CHUNK_HEIGHT } = await import('/static/js/surface/surface_render.js');
      const { CHUNK, FLOAT_SPAN, FORMATIONS, COLORS } = await import('/static/js/surface/surface_config.js');
      const TOPY = 300 - CHUNK_TOP_MARGIN;
      const seeds = [1, 987654321, 424242, 31415926];

      // Пещерный цвет (маска, §2.1) — RGB из COLORS.cave. Градиент depths только
      // ЗАТЕМНЯЕТ (source-atop), поэтому допуск 14 перекрывает любую глубину.
      const caveRgb = (() => {
        const s = COLORS.cave.replace('#', '');
        return [parseInt(s.slice(0, 2), 16), parseInt(s.slice(2, 4), 16), parseInt(s.slice(4, 6), 16)];
      })();

      function measure(world, index) {
        const baseX = index * CHUNK;
        const { canvas, topY } = getChunkCanvas(world, index);
        const ctx = canvas.getContext('2d');
        const img = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
        const pxScale = canvas.width / (CHUNK + 1), pyScale = canvas.height / CHUNK_HEIGHT;
        const at = (lx, ly) => {
          const px = Math.floor(lx * pxScale), py = Math.floor(ly * pyScale);
          if (px < 0 || px >= canvas.width || py < 0 || py >= canvas.height) return null;
          const o = (py * canvas.width + px) * 4;
          return [img[o], img[o + 1], img[o + 2], img[o + 3]];
        };
        const forms = !!world.forms;
        let total = 0, miss = 0, firstMiss = null, caveOnSolid = 0, firstCave = null;
        for (let lx = 0; lx <= CHUNK; lx++) {
          const wx = baseX + lx;
          const th = world.terrainHeight(wx);
          // Низ окна — на глубину трещины crack2d (Э5.2, depth ≤ 400), а не th+60:
          // «что твёрдо — то видно» проверяется и под поверхностью.
          const hi = Math.min(th + 420, topY + CHUNK_HEIGHT - 1);
          // Верх окна: у биомов с 2D-формами — ВЕРХ РАСТРА (плиты арок уходят выше
          // th на opening+h, §2.3/§5 п.6); иначе — над полосой float.
          const lo = forms ? topY : th - FLOAT_SPAN - 20;
          for (let wy = lo; wy <= hi; wy += 1) {
            const solid = world.solidAt(wx, wy);
            const p = at(lx, wy - topY);
            if (solid) {
              total++;
              if (!p || p[3] === 0) { miss++; if (!firstMiss) firstMiss = [Math.round(wx), +wy.toFixed(1)]; continue; }
            }
            // Находка №1: пиксель пещерного цвета НЕ должен совпадать с твёрдой
            // точкой (маска — часть твёрдого тела, §2.1/S1-bis).
            if (forms && solid && p && p[3] > 0
              && Math.abs(p[0] - caveRgb[0]) <= 14 && Math.abs(p[1] - caveRgb[1]) <= 14 && Math.abs(p[2] - caveRgb[2]) <= 14) {
              caveOnSolid++; if (!firstCave) firstCave = [Math.round(wx), +wy.toFixed(1)];
            }
          }
        }
        if (world._chunkCache) world._chunkCache.delete(index);
        return { total, miss, firstMiss, caveOnSolid, firstCave };
      }

      // Стоимость генерации чанка (без кэша): свежий мир на каждый замер.
      function timing(seed, over, n) {
        // Прогрев (JIT/аллокация канваса) вне замера — первый чанк завышает среднее.
        { const w = new SurfaceWorld({ seed, biome_color: '#8a7a6a', life: false, biome: 'qa', ...over }); getChunkCanvas(w, 0); }
        const t0 = performance.now();
        for (let k = 0; k < n; k++) {
          const w = new SurfaceWorld({ seed: seed + 1 + k, biome_color: '#8a7a6a', life: false, biome: 'qa', ...over });
          getChunkCanvas(w, 0);
        }
        return (performance.now() - t0) / n;
      }

      const rows = [];
      let total = 0, miss = 0, cave = 0;
      const firsts = [];
      const firstsCave = [];
      // 6 категорий-фолбэков
      for (const cat of Object.keys(FORMATIONS)) {
        let cTotal = 0, cMiss = 0, cCave = 0;
        for (const seed of seeds) {
          const w = new SurfaceWorld({ seed, biome_category: cat, biome_color: '#8a7a6a', life: false, biome: 'qa' });
          for (const index of [-1, 0, 1]) {
            const r = measure(w, index);
            cTotal += r.total; cMiss += r.miss; cCave += r.caveOnSolid;
            if (r.firstMiss) firsts.push([cat, r.firstMiss[0], r.firstMiss[1]]);
            if (r.firstCave) firstsCave.push([cat, r.firstCave[0], r.firstCave[1]]);
          }
        }
        rows.push({ name: cat, total: cTotal, miss: cMiss, cave: cCave });
        total += cTotal; miss += cMiss; cave += cCave;
      }
      // резолвленные рецепты
      for (const bv of views) {
        let cTotal = 0, cMiss = 0, cCave = 0;
        for (const seed of seeds) {
          const w = new SurfaceWorld({ seed, biome: bv.id, biome_category: bv.category, biome_color: bv.color, life: false, view_source: 'catalog', view_version: 1, biome_view: bv.view });
          for (const index of [-1, 0, 1]) {
            const r = measure(w, index);
            cTotal += r.total; cMiss += r.miss; cCave += r.caveOnSolid;
            if (r.firstMiss) firsts.push([bv.id, r.firstMiss[0], r.firstMiss[1]]);
            if (r.firstCave) firstsCave.push([bv.id, r.firstCave[0], r.firstCave[1]]);
          }
        }
        rows.push({ name: bv.id, total: cTotal, miss: cMiss, cave: cCave });
        total += cTotal; miss += cMiss; cave += cCave;
      }

      const genMsFallback = timing(424242, { biome_category: 'экзотика' }, 24);
      // S4 — БЕЗформенный рецепт: формы меряет S4b, чтобы S4 и S4b не дублировались.
      const formless = views.find((v) => {
        const rel = v.view && v.view.relief;
        return !(rel && Array.isArray(rel.forms) && rel.forms.length);
      }) || views[0];
      const genMsRecipe = views.length ? timing(424242, { biome: formless.id, biome_category: formless.category, biome_color: formless.color, view_source: 'catalog', view_version: 1, biome_view: formless.view }, 24) : null;
      // Э5.2: перф генерации чанка на целевых биомах с 2D-формами (бюджет ≤2× baseline, §5 п.7).
      const formsMs = {};
      // База бюджета форм (§5 п.7) — ТОТ ЖЕ рецепт без `relief.forms`. Fallback
      // экзотики — иной генератор (иная категория/формации) и базой формы не
      // является: сравнение с ним меряет разницу рецептов, а не вклад форм.
      const noFormsMs = {};
      for (const bv of views) {
        if (bv.id !== 'горы' && bv.id !== 'каменные_пустоши') continue;
        const over = { biome: bv.id, biome_category: bv.category, biome_color: bv.color, view_source: 'catalog', view_version: 1 };
        formsMs[bv.id] = timing(424242, { ...over, biome_view: bv.view }, 24);
        const vNo = JSON.parse(JSON.stringify(bv.view));
        if (vNo.relief) vNo.relief.forms = [];
        noFormsMs[bv.id] = timing(424242, { ...over, biome_view: vNo }, 24);
      }
      return { rows, total, miss, firsts, firstsCave, caveOnSolid: cave, genMsFallback, genMsRecipe, formlessId: formless.id, formsMs, noFormsMs, chunk: CHUNK };
    }, views);

    for (const r of data.rows) if (r.miss || r.cave) console.log(`  ${r.name}: solidPts=${r.total} miss=${r.miss} caveOnSolid=${r.cave}`);
    report('S1 «что твёрдо — то видно»: нет непокрашенных твёрдых точек и пещерного цвета на твёрдом',
      data.miss === 0 && data.caveOnSolid === 0,
      `solidPts=${data.total} miss=${data.miss} caveOnSolid=${data.caveOnSolid} first=${JSON.stringify(data.firsts.slice(0, 3))} firstCave=${JSON.stringify((data.firstsCave || []).slice(0, 2))}`);
    report('S2 покрытие сетки', data.rows.length >= 10, 'биомов=' + data.rows.length);
    report('S3 генерация чанка (fallback)', data.genMsFallback < 60, `avg=${data.genMsFallback.toFixed(1)} ms/chunk`);
    report(`S4 генерация чанка (безформенный рецепт ${data.formlessId})`, data.genMsRecipe == null || data.genMsRecipe < 60,
      data.genMsRecipe == null ? 'нет рецептов' : `avg=${data.genMsRecipe.toFixed(1)} ms/chunk`);
    const formsDetail = Object.entries(data.formsMs || {}).map(([k, v]) => `${k}=${v.toFixed(1)}`).join(' ');
    const noFormsDetail = Object.entries(data.noFormsMs || {}).map(([k, v]) => `${k}=${v.toFixed(1)}`).join(' ');
    const formsAbsOk = Object.values(data.formsMs || {}).every((v) => v < 60);
    const formsRelOk = Object.entries(data.formsMs || {}).every(([k, v]) => v <= 2 * (data.noFormsMs[k] ?? Infinity) + 1e-9);
    report('S4b генерация чанка (биомы с 2D-формами: <60 ms и ≤2× того же рецепта без форм)', formsAbsOk && formsRelOk,
      `без форм: ${noFormsDetail}; с формами: ${formsDetail}; fallback(экзотика)=${data.genMsFallback.toFixed(1)}`);
    report('S5 0 pageerror', pageErrors.length === 0, pageErrors.slice(0, 2).join(' | '));

    const failed = results.filter((r) => !r.ok).length;
    console.log('SUMMARY: ' + results.filter((r) => r.ok).length + ' PASS / ' + failed + ' FAIL');
    writeFileSync(path.join(ARTIFACTS_DIR, 'surface-solid-field-results.json'), JSON.stringify({ data, results }, null, 2));
    process.exit(failed ? 1 : 0);
  } catch (e) {
    console.log('EXCEPTION: ' + e.message + '\n' + e.stack);
    process.exit(1);
  } finally {
    await browser.close().catch(() => {});
  }
}
main();
