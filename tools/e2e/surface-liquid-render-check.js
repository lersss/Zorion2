// tools/e2e/surface-liquid-render-check.js
// ЧК6 «Мир прогулки: вода» — ОТРИСОВКА слоя жидкости (спека 2026-09-25 §5).
// Проверяет РЕАЛЬНЫЙ растр чанка (getChunkCanvas) и фронтальный проход
// (drawLiquidFront/drawLiquidEmissive) для биомов категории «вода»:
//   * тело жидкости запечено в канвас земли (пиксель в [liquidLevel, bedY] непрозрачен);
//   * корка underIce запечена как часть земли (пиксель в [crustTop, liquidLevel]);
//   * фронтальный проход рисует зеркало/пелену (пиксели на кадре, без броска);
//   * свечение лавы рисуется (drawLiquidEmissive);
//   * 0 pageerror.
// Run: node surface-liquid-render-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
function findExecutable() { for (const p of CHROME_PATHS) if (existsSync(p)) return p; for (const p of EDGE_PATHS) if (existsSync(p)) return p; return null; }

const results = [];
function report(step, ok, detail) { results.push({ step, ok, detail }); console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`); }

function mergeView(preset, delta) {
  const out = { ...preset };
  for (const k of Object.keys(delta)) {
    const pv = out[k], dv = delta[k];
    if (pv && typeof pv === 'object' && !Array.isArray(pv) && dv && typeof dv === 'object' && !Array.isArray(dv)) out[k] = mergeView(pv, dv);
    else out[k] = dv;
  }
  return out;
}
function resolveViews(ids) {
  const cat = JSON.parse(readFileSync(new URL('../../config/biome_catalog.json', import.meta.url), 'utf8'));
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
  const views = resolveViews(['океаны', 'озёра_реки', 'подлёдные_океаны', 'магмовый_океан']);
  const browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const page = await context.newPage();
    const pageErrors = [];
    page.on('pageerror', (e) => pageErrors.push(String(e)));
    await page.goto(BASE_URL + '/login-page', { waitUntil: 'domcontentloaded', timeout: 30000 });

    const out = await page.evaluate(async (bv) => {
      const { SurfaceWorld } = await import('/static/js/surface/surface_world.js');
      const { getChunkCanvas, drawLiquidFront, drawLiquidEmissive, CHUNK_TOP_MARGIN, CHUNK_HEIGHT } = await import('/static/js/surface/surface_render.js');
      const { CHUNK } = await import('/static/js/surface/surface_config.js');
      const TOPY = 300 - CHUNK_TOP_MARGIN;
      const mk = (bv1, seed) => new SurfaceWorld({
        seed, biome: bv1.id, biome_category: bv1.category, biome_color: bv1.color,
        life: true, view_source: 'catalog', view_version: 1, biome_view: bv1.view,
      });
      const res = {};

      // Тело жидкости в канвасе чанка: пиксель в теле [lv, lv+depth] непрозрачен.
      {
        const w = mk(bv['океаны'], 424242);
        let checks = 0, opaque = 0;
        for (const idx of [0, 1, 2, 3]) {
          const { canvas } = getChunkCanvas(w, idx);
          const ctx = canvas.getContext('2d');
          const img = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
          const pxScale = canvas.width / (CHUNK + 1), pyScale = canvas.height / CHUNK_HEIGHT;
          const at = (lx, ly) => {
            const px = Math.floor(lx * pxScale), py = Math.floor(ly * pyScale);
            if (px < 0 || px >= canvas.width || py < 0 || py >= canvas.height) return 0;
            return img[(py * canvas.width + px) * 4 + 3];
          };
          for (let lx = 0; lx <= CHUNK; lx += 7) {
            const wx = idx * CHUNK + lx;
            const c = w._liquidCol(wx);
            if (!c) continue;
            checks++;
            if (at(lx, c.lv + c.depth / 2 - TOPY) > 0) opaque++;
          }
        }
        res.global = { checks, opaque };
      }

      // Корка underIce: пиксель в [crustTop, lv] непрозрачен; полынья — воды нет в корке.
      {
        const w = mk(bv['подлёдные_океаны'], 424242);
        let checks = 0, opaque = 0, polynyaOpen = 0, polynyaChecked = 0;
        for (const idx of [0, 1, 2, 3, 4]) {
          const { canvas } = getChunkCanvas(w, idx);
          const ctx = canvas.getContext('2d');
          const img = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
          const pxScale = canvas.width / (CHUNK + 1), pyScale = canvas.height / CHUNK_HEIGHT;
          const at = (lx, ly) => {
            const px = Math.floor(lx * pxScale), py = Math.floor(ly * pyScale);
            if (px < 0 || px >= canvas.width || py < 0 || py >= canvas.height) return 0;
            return img[(py * canvas.width + px) * 4 + 3];
          };
          for (let lx = 0; lx <= CHUNK; lx += 7) {
            const wx = idx * CHUNK + lx;
            const top = w.crustTop(wx);
            if (top !== null) { checks++; if (at(lx, (top + w.liquidLevel(wx)) / 2 - TOPY) > 0) opaque++; }
            else if (w.polynyaAt(wx) && w._liquidCol(wx)) {
              polynyaChecked++; if (at(lx, w.liquidLevel(wx) - 3 - TOPY) === 0) polynyaOpen++;
            }
          }
        }
        res.ice = { checks, opaque, polynyaChecked, polynyaOpen };
      }

      // Фронтальный проход: зеркало/пелена рисуют пиксели, без броска.
      {
        const w = mk(bv['океаны'], 424242);
        const canvas = document.createElement('canvas');
        canvas.width = 1280; canvas.height = 800;
        const ctx = canvas.getContext('2d');
        let wetX = 0; for (let x = 0; x < 4000; x++) if (w.liquidDepth(x) > 20) { wetX = x; break; }
        const camera = { x: wetX, y: w.baseY };
        let threw = null;
        try { drawLiquidFront(ctx, w, camera, 1280, 800, null); } catch (e) { threw = String(e); }
        const img = ctx.getImageData(0, 0, 1280, 800).data;
        let painted = 0;
        for (let i = 3; i < img.length; i += 4) if (img[i] > 0) painted++;
        res.front = { threw, painted };
      }

      // Свечение лавы: drawLiquidEmissive рисует пиксели.
      {
        const w = mk(bv['магмовый_океан'], 424242);
        let lavaX = 0; for (let x = 0; x < 60000; x++) if (w.liquidDepth(x) > 20) { lavaX = x; break; }
        const canvas = document.createElement('canvas');
        canvas.width = 1280; canvas.height = 800;
        const ctx = canvas.getContext('2d');
        let threw = null;
        try { drawLiquidEmissive(ctx, w, { x: lavaX, y: w.baseY }, 1280, 800, null); } catch (e) { threw = String(e); }
        const img = ctx.getImageData(0, 0, 1280, 800).data;
        let painted = 0;
        for (let i = 3; i < img.length; i += 4) if (img[i] > 0) painted++;
        res.glow = { threw, painted, hasGlow: !!w.liquid.glow };
      }

      return res;
    }, Object.fromEntries(views.map((v) => [v.id, v])));

    report('R1 тело жидкости запечено в чанк (океаны)',
      out.global.checks > 0 && out.global.opaque === out.global.checks,
      `точек=${out.global.checks} непрозрачных=${out.global.opaque}`);
    report('R2 корка underIce запечена в чанк',
      out.ice.checks > 0 && out.ice.opaque === out.ice.checks,
      `точек=${out.ice.checks} непрозрачных=${out.ice.opaque}`);
    report('R3 полынья: над зеркалом корки нет (пусто)',
      out.ice.polynyaChecked === 0 || out.ice.polynyaOpen === out.ice.polynyaChecked,
      `точек=${out.ice.polynyaChecked} открытых=${out.ice.polynyaOpen}`);
    report('R4 drawLiquidFront рисует без броска',
      out.front.threw === null && out.front.painted > 0,
      `throw=${out.front.threw || 'нет'} пикселей=${out.front.painted}`);
    report('R5 drawLiquidEmissive (лава) рисует без броска',
      out.glow.threw === null && out.glow.painted > 0,
      `throw=${out.glow.threw || 'нет'} пикселей=${out.glow.painted} glow=${out.glow.hasGlow}`);
    report('R6 0 pageerror', pageErrors.length === 0, pageErrors.slice(0, 2).join(' | '));
  } finally {
    await browser.close();
  }

  const failed = results.filter((r) => !r.ok);
  console.log(`\nSUMMARY: ${results.length - failed.length} PASS / ${failed.length} FAIL`);
  if (failed.length) process.exit(1);
}

main().catch((e) => { console.error(e); process.exit(1); });
