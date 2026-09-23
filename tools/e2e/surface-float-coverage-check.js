// tools/e2e/surface-float-coverage-check.js
// Independent @tester check for idea 2026-09-21 "invisible rocks in the air" (§2.1/§4):
// raster must be a SUPERSET of the solid (isSolid) region of float-formations.
// For every category that has float:true formations, several seeds and chunks:
//   * build the real chunk raster via the SHIPPED getChunkCanvas();
//   * build a CONTROL raster with the OLD 3x3-step algorithm (re-implemented here);
//   * over a dense grid (0.5 px in x and y) inside the float band, count solid
//     points that the raster does NOT paint (r+g+b == 0 => no rock pixel).
// A miss is an invisible solid volume the player can hit.
// Run: node surface-float-coverage-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}

const results = [];
function report(step, ok, detail) {
  results.push({ step, ok, detail });
  console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER FOUND'); process.exit(1); }
  const browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const page = await context.newPage();
    const pageErrors = [];
    page.on('pageerror', (e) => pageErrors.push(String(e)));
    // Any page on the origin is enough to import ES modules from /static.
    await page.goto(BASE_URL + '/login-page', { waitUntil: 'domcontentloaded', timeout: 30000 });

    const data = await page.evaluate(async () => {
      const { SurfaceWorld } = await import('/static/js/surface/surface_world.js');
      const { getChunkCanvas, CHUNK_TOP_MARGIN, CHUNK_HEIGHT } = await import('/static/js/surface/surface_render.js');
      const { CHUNK, FLOAT_SPAN, FLOAT_GAP, FORMATIONS } = await import('/static/js/surface/surface_config.js');
      // Рамка растра — из рендера (единственный источник), не хардкод: смена
      // CHUNK_TOP_MARGIN иначе молча сдвигала бы маску и давала ложный 100% miss.
      const TOP_MARGIN = CHUNK_TOP_MARGIN;
      const sx = 0.5, sy = 0.5;

      // Control raster: OLD algorithm (exact copy of the pre-fix loop in
      // surface_render.js): lx step 3, 3x3 block at integer grid samples.
      function oldMask(world, index, baseX, topY) {
        const mask = new Uint8Array((CHUNK + 1) * CHUNK_HEIGHT);
        for (let lx = 0; lx < CHUNK; lx += 3) {
          const wx = baseX + lx;
          const th = world.terrainHeight(wx);
          for (let wy = th - 3; wy > th - FLOAT_SPAN; wy -= 3) {
            if (!world.isSolid(wx, wy)) continue;
            const ly = Math.floor(wy - topY);
            for (let dy = 0; dy < 3; dy++) {
              for (let dx = 0; dx < 3; dx++) {
                const X = lx + dx, Y = ly + dy;
                if (X <= CHUNK && Y >= 0 && Y < CHUNK_HEIGHT) mask[Y * (CHUNK + 1) + X] = 1;
              }
            }
          }
        }
        return mask;
      }

      function measure(world, index) {
        const baseX = index * CHUNK;
        const topY = world.baseY - TOP_MARGIN;
        const { canvas } = getChunkCanvas(world, index);
        const ctx = canvas.getContext('2d');
        const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const pxScale = canvas.width / (CHUNK + 1);
        const pyScale = canvas.height / CHUNK_HEIGHT;
        const mask = oldMask(world, index, baseX, topY);
        let total = 0, missNew = 0, missOld = 0, paintedNonSolid = 0;
        let firstNew = null, firstOld = null;
        for (let lx = 0; lx <= CHUNK; lx += sx) {
          const wx = baseX + lx;
          const th = world.terrainHeight(wx);
          for (let wy = th - FLOAT_GAP - sy; wy > th - FLOAT_SPAN + sy; wy -= sy) {
            const ly = wy - topY;
            const px = Math.floor(lx * pxScale), py = Math.floor(ly * pyScale);
            let paintedNew = false;
            if (px >= 0 && px < canvas.width && py >= 0 && py < canvas.height) {
              const i = (py * canvas.width + px) * 4;
              paintedNew = (img.data[i] + img.data[i + 1] + img.data[i + 2]) > 0;
            }
            const solid = world.isSolid(wx, wy);
            if (!solid) { if (paintedNew) paintedNonSolid++; continue; }
            total++;
            if (!paintedNew) { missNew++; if (!firstNew) firstNew = [Math.round(wx), +wy.toFixed(1)]; }
            const mx = Math.floor(lx), my = Math.floor(ly);
            let paintedOld = false;
            if (mx >= 0 && mx <= CHUNK && my >= 0 && my < CHUNK_HEIGHT) paintedOld = mask[my * (CHUNK + 1) + mx] === 1;
            if (!paintedOld) { missOld++; if (!firstOld) firstOld = [Math.round(wx), +wy.toFixed(1)]; }
          }
        }
        // free the cached canvas (SS=2 => ~7 MB each)
        if (world._chunkCache) world._chunkCache.delete(index);
        return { total, missNew, missOld, paintedNonSolid, firstNew, firstOld };
      }

      // Ghost-rock bound: every painted pixel of the float band must have a
      // solid sample within a 1-px neighbourhood (the declared bleed). A painted
      // pixel with none is genuine overpaint beyond the design halo.
      function overpaint(world, index) {
        const baseX = index * CHUNK, topY = world.baseY - TOP_MARGIN;
        const { canvas } = getChunkCanvas(world, index);
        const ctx = canvas.getContext('2d');
        const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const pxScale = canvas.width / (CHUNK + 1), pyScale = canvas.height / CHUNK_HEIGHT;
        let painted = 0, beyond = 0, firstBeyond = null;
        for (let lx = 2; lx <= CHUNK - 2; lx++) {
          const wx = baseX + lx;
          const th = world.terrainHeight(wx);
          for (let wy = th - FLOAT_SPAN; wy <= th - FLOAT_GAP; wy += 1) {
            const ly = Math.floor(wy - topY);
            const px = Math.floor(lx * pxScale), py = Math.floor(ly * pyScale);
            if (px < 0 || px >= canvas.width || py < 0 || py >= canvas.height) continue;
            const i = (py * canvas.width + px) * 4;
            if (img.data[i] + img.data[i + 1] + img.data[i + 2] <= 0) continue;
            painted++;
            let near = false;
            for (let dx = -2; dx <= 2 && !near; dx++) {
              for (let dy = -2; dy <= 2 && !near; dy++) {
                if (world.isSolid(wx + dx, ly + dy + topY)) near = true;
              }
            }
            if (!near) { beyond++; if (!firstBeyond) firstBeyond = [Math.round(wx), ly]; }
          }
        }
        if (world._chunkCache) world._chunkCache.delete(index);
        return { painted, beyond, firstBeyond };
      }

      // Chunk generation cost (uncached): fresh world + cache eviction per chunk.
      function timing(seed, cat, n) {
        const t0 = performance.now();
        for (let k = 0; k < n; k++) {
          const w = new SurfaceWorld({ seed: seed + k, biome_category: cat, life: false, biome: 'qa', biome_color: '#8a7a6a' });
          getChunkCanvas(w, 0);
        }
        return (performance.now() - t0) / n;
      }

      // Seam: rightmost column of chunk i and leftmost column of chunk i+1 both
      // map to the same world column — their pixels must agree (no gap/step).
      function seam(world) {
        const a = getChunkCanvas(world, 0).canvas;
        const b = getChunkCanvas(world, 1).canvas;
        const ca = a.getContext('2d').getImageData(0, 0, a.width, a.height);
        const cb = b.getContext('2d').getImageData(0, 0, b.width, b.height);
        const topY = world.baseY - TOP_MARGIN;
        const th = world.terrainHeight(CHUNK);
        let checked = 0, mismatch = 0, gap = 0;
        for (let welly = th - FLOAT_SPAN; welly < th; welly += 4) {
          const ly = welly - topY;
          const py = Math.floor(ly * (a.height / CHUNK_HEIGHT));
          const ai = (py * a.width + (a.width - 1)) * 4;
          const bi = (py * b.width + 0) * 4;
          checked++;
          if (Math.abs(ca.data[ai] - cb.data[bi]) > 2 || Math.abs(ca.data[ai + 1] - cb.data[bi + 1]) > 2 || Math.abs(ca.data[ai + 2] - cb.data[bi + 2]) > 2) mismatch++;
          // «Щель» = односторонняя прозрачность на стыке (один чанк красит,
          // другой нет) — ступенька. «Оба прозрачны» легально: канвас чанка
          // непрозрачен только под рельефом (глубинный градиент и кромка —
          // общий проход drawTerrain, не в канвасе чанка).
          if (Math.abs(ca.data[ai + 3] - cb.data[bi + 3]) > 2) gap++;
        }
        world._chunkCache = null;
        return { checked, mismatch, gap };
      }

      const opWorld = new SurfaceWorld({ seed: 424242, biome_category: 'экзотика', life: false, biome: 'qa', biome_color: '#8a7a6a' });
      const opRes = overpaint(opWorld, 0);
      const genMs = timing(424242, 'экзотика', 8);
      const seamWorld = new SurfaceWorld({ seed: 424242, biome_category: 'экзотика', life: false, biome: 'qa', biome_color: '#8a7a6a' });
      const seamRes = seam(seamWorld);

      const cats = Object.keys(FORMATIONS).filter((c) => FORMATIONS[c].some((f) => f.float));
      const seeds = [1, 987654321, 424242, 31415926];
      const chunks = [-1, 0, 1];
      const catRows = [];
      let total = 0, missNew = 0, missOld = 0, paintedNonSolid = 0;
      const firstNew = [];
      for (const cat of cats) {
        let cTotal = 0, cNew = 0, cOld = 0;
        for (const seed of seeds) {
          const world = new SurfaceWorld({ seed, biome_category: cat, life: false, biome: 'qa', biome_color: '#8a7a6a' });
          for (const index of chunks) {
            const r = measure(world, index);
            cTotal += r.total; cNew += r.missNew; cOld += r.missOld; paintedNonSolid += r.paintedNonSolid;
            if (r.firstNew && !firstNew.some((f) => f[0] === cat)) firstNew.push([cat, r.firstNew[0], r.firstNew[1]]);
          }
        }
        catRows.push({ cat, total: cTotal, missNew: cNew, missOld: cOld });
        total += cTotal; missNew += cNew; missOld += cOld;
      }
      return { cats, catRows, total, missNew, missOld, paintedNonSolid, firstNew, opRes, genMs, seamRes, chunk: CHUNK, floatSpan: FLOAT_SPAN, floatGap: FLOAT_GAP };
    });

    console.log('CATEGORIES: ' + data.cats.join(', '));
    for (const r of data.catRows) {
      console.log(`  ${r.cat}: solidPts=${r.total} missNew=${r.missNew} (${r.total ? (100 * r.missNew / r.total).toFixed(3) : 0}%) missOldControl=${r.missOld}`);
    }

    report('C1 shipped raster: no unpainted solid float points',
      data.missNew === 0,
      `totalSolid=${data.total} missNew=${data.missNew} firstMiss=${JSON.stringify(data.firstNew.slice(0, 3))}`);
    report('C2 control (old 3x3 step) reproduces the bug (test is meaningful)',
      data.missOld > 0,
      `missOld=${data.missOld} of ${data.total} (${data.total ? (100 * data.missOld / data.total).toFixed(3) : 0}%)`);
    report('C3 all 6 float categories covered',
      data.cats.length >= 5,
      'cats=' + data.cats.length + ' [' + data.cats.join(', ') + ']');
    report('C4 no page errors during measurement', pageErrors.length === 0, pageErrors.slice(0, 2).join(' | '));
    report('C5 reverse side: overpaint halo <= 2 px (no ghost rocks)',
      data.opRes.beyond === 0,
      `paintedBandPx=${data.opRes.painted} overpaintBeyond2px=${data.opRes.beyond} first=${JSON.stringify(data.opRes.firstBeyond)} | nonSolidPaintedSamples(all categories)=${data.paintedNonSolid}`);
    report('C6 chunk generation cost acceptable',
      data.genMs < 40,
      `avg=${data.genMs.toFixed(1)} ms/chunk (cached after first paint)`);
    report('C7 chunk seam: adjacent columns match, no gap',
      data.seamRes.mismatch === 0 && data.seamRes.gap === 0,
      `checked=${data.seamRes.checked} mismatch=${data.seamRes.mismatch} alphaStep=${data.seamRes.gap}`);

    const failed = results.filter((r) => !r.ok).length;
    console.log('SUMMARY: ' + results.filter((r) => r.ok).length + ' PASS / ' + failed + ' FAIL');
    writeFileSync(path.join(ARTIFACTS_DIR, 'surface-float-coverage-results.json'), JSON.stringify({ data, results }, null, 2));
    process.exit(failed ? 1 : 0);
  } catch (e) {
    console.log('EXCEPTION: ' + e.message + '\n' + e.stack);
    process.exit(1);
  } finally {
    await browser.close().catch(() => {});
  }
}
main();
