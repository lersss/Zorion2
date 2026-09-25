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
  const ids = ['горы', 'пески_пустыни', 'джунгли', 'лавовые_поля', 'пещерный_мир_с_потолком', 'ледники', 'кристальные_рощи'];
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
      const { CHUNK, FLOAT_SPAN, FORMATIONS } = await import('/static/js/surface/surface_config.js');
      const TOPY = 300 - CHUNK_TOP_MARGIN;
      const seeds = [1, 987654321, 424242, 31415926];

      function measure(world, index) {
        const baseX = index * CHUNK;
        const { canvas, topY } = getChunkCanvas(world, index);
        const ctx = canvas.getContext('2d');
        const img = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
        const pxScale = canvas.width / (CHUNK + 1), pyScale = canvas.height / CHUNK_HEIGHT;
        const at = (lx, ly) => {
          const px = Math.floor(lx * pxScale), py = Math.floor(ly * pyScale);
          if (px < 0 || px >= canvas.width || py < 0 || py >= canvas.height) return -1;
          return img[(py * canvas.width + px) * 4 + 3];
        };
        let total = 0, miss = 0, firstMiss = null;
        for (let lx = 0; lx <= CHUNK; lx++) {
          const wx = baseX + lx;
          const th = world.terrainHeight(wx);
          const hi = Math.min(th + 60, topY + CHUNK_HEIGHT - 1);
          for (let wy = th - FLOAT_SPAN - 20; wy <= hi; wy += 1) {
            if (!world.solidAt(wx, wy)) continue;
            total++;
            const ly = wy - topY;
            const a = at(lx, ly);
            if (a === 0) { miss++; if (!firstMiss) firstMiss = [Math.round(wx), +wy.toFixed(1)]; }
          }
        }
        if (world._chunkCache) world._chunkCache.delete(index);
        return { total, miss, firstMiss };
      }

      // Стоимость генерации чанка (без кэша): свежий мир на каждый замер.
      function timing(seed, over, n) {
        const t0 = performance.now();
        for (let k = 0; k < n; k++) {
          const w = new SurfaceWorld({ seed: seed + k, biome_color: '#8a7a6a', life: false, biome: 'qa', ...over });
          getChunkCanvas(w, 0);
        }
        return (performance.now() - t0) / n;
      }

      const rows = [];
      let total = 0, miss = 0;
      const firsts = [];
      // 6 категорий-фолбэков
      for (const cat of Object.keys(FORMATIONS)) {
        let cTotal = 0, cMiss = 0;
        for (const seed of seeds) {
          const w = new SurfaceWorld({ seed, biome_category: cat, biome_color: '#8a7a6a', life: false, biome: 'qa' });
          for (const index of [-1, 0, 1]) {
            const r = measure(w, index);
            cTotal += r.total; cMiss += r.miss;
            if (r.firstMiss) firsts.push([cat, r.firstMiss[0], r.firstMiss[1]]);
          }
        }
        rows.push({ name: cat, total: cTotal, miss: cMiss });
        total += cTotal; miss += cMiss;
      }
      // резолвленные рецепты
      for (const bv of views) {
        let cTotal = 0, cMiss = 0;
        for (const seed of seeds) {
          const w = new SurfaceWorld({ seed, biome: bv.id, biome_category: bv.category, biome_color: bv.color, life: false, view_source: 'catalog', view_version: 1, biome_view: bv.view });
          for (const index of [-1, 0, 1]) {
            const r = measure(w, index);
            cTotal += r.total; cMiss += r.miss;
            if (r.firstMiss) firsts.push([bv.id, r.firstMiss[0], r.firstMiss[1]]);
          }
        }
        rows.push({ name: bv.id, total: cTotal, miss: cMiss });
        total += cTotal; miss += cMiss;
      }

      const genMsFallback = timing(424242, { biome_category: 'экзотика' }, 8);
      const genMsRecipe = views.length ? timing(424242, { biome: views[0].id, biome_category: views[0].category, biome_color: views[0].color, view_source: 'catalog', view_version: 1, biome_view: views[0].view }, 8) : null;
      return { rows, total, miss, firsts, genMsFallback, genMsRecipe, chunk: CHUNK };
    }, views);

    for (const r of data.rows) if (r.miss) console.log(`  ${r.name}: solidPts=${r.total} miss=${r.miss}`);
    report('S1 «что твёрдо — то видно»: нет непокрашенных твёрдых точек (сетка seed×биомы)',
      data.miss === 0, `solidPts=${data.total} miss=${data.miss} first=${JSON.stringify(data.firsts.slice(0, 3))}`);
    report('S2 покрытие сетки', data.rows.length >= 10, 'биомов=' + data.rows.length);
    report('S3 генерация чанка (fallback)', data.genMsFallback < 60, `avg=${data.genMsFallback.toFixed(1)} ms/chunk`);
    report('S4 генерация чанка (рецепт)', data.genMsRecipe == null || data.genMsRecipe < 60,
      data.genMsRecipe == null ? 'нет рецептов' : `avg=${data.genMsRecipe.toFixed(1)} ms/chunk`);
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
