// tools/e2e/surface-swim-check.js
// ЧК6.2 «Мир прогулки: плавание и погружение» — живой прогон модулей прогулки в
// браузере (образец tools/e2e/surface-liquid-render-check.js). Проверяет
// НАСТОЯЩИЕ SurfaceWorld/Player/drawLiquidFront (не node-копию):
//   * вход в воду (режим плавания включается по liquidAt);
//   * без `down` игрок не тонет до дна (равновесие плавучести у зеркала);
//   * с `down` ныряет ниже зеркала;
//   * перемещение в воде (горизонталь);
//   * drawLiquidFront(player) рисует подводную подкраску без броска;
//   * подлёдные: вход через полынью (liquidAt под коркой);
//   * 0 pageerror.
// Run: node surface-swim-check.js   (BASE_URL env)
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
  const views = resolveViews(['океаны', 'подлёдные_океаны']);
  const browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const page = await context.newPage();
    const pageErrors = [];
    page.on('pageerror', (e) => pageErrors.push(String(e)));
    await page.goto(BASE_URL + '/login-page', { waitUntil: 'domcontentloaded', timeout: 30000 });

    const out = await page.evaluate(async (bv) => {
      const { SurfaceWorld } = await import('/static/js/surface/surface_world.js');
      const { Player } = await import('/static/js/surface/surface_player.js');
      const { drawLiquidFront } = await import('/static/js/surface/surface_render.js');
      const cfg = await import('/static/js/surface/surface_config.js');
      const mk = (b, seed) => new SurfaceWorld({
        seed, biome: b.id, biome_category: b.category, biome_color: b.color,
        life: true, view_source: 'catalog', view_version: 1, biome_view: b.view,
      });
      const NO = { left: false, right: false, jump: false, sprint: false, down: false };
      const sim = (p, inp, n) => { for (let i = 0; i < n; i++) p.update(1 / 60, inp); return p; };
      const longestWet = (w, minDepth) => {
        let best = null, s = -1; const end = 16000;
        for (let x = 0; x <= end; x++) {
          const wet = x < end && w.liquidDepth(x) >= minDepth;
          if (wet) { if (s < 0) s = x; }
          else if (s >= 0) { const len = x - s; if (!best || len > best.len) best = { s, e: x - 1, len, mid: Math.floor((s + x - 1) / 2) }; s = -1; }
        }
        return best;
      };
      const res = {};

      // --- океаны: вход, плавучесть, ныряние, перемещение ---
      {
        const w = mk(bv['океаны'], 424242);
        const run = longestWet(w, 90);
        const wx = run ? run.mid : 0;
        const lv = w.liquidLevel(wx), bd = w.bedY(wx);
        const p = new Player(w, 1);
        p.x = wx; p.vx = 0; p.vy = 0; p.onGround = false; p.y = lv + 10;
        res.inLiquid = w.liquidAt(p.x, p.y) !== '';
        // без down — не тонет до дна
        sim(p, NO, 300);
        res.headUnder = !!(p.y - p.h / 2 > lv);
        res.head = p.y - p.h / 2; res.lv = lv; res.bd = bd; res.yFloat = p.y;
        // с down — ныряет
        const pd = new Player(w, 1);
        pd.x = wx; pd.vx = 0; pd.vy = 0; pd.onGround = false; pd.y = lv + 10;
        sim(pd, { ...NO, down: true }, 300);
        res.diveY = pd.y; res.diveUnder = pd.y > lv + 20;
        // перемещение в воде
        const pm = new Player(w, 1);
        pm.x = wx; pm.vx = 0; pm.vy = 0; pm.onGround = false; pm.y = lv + 40;
        const x0 = pm.x;
        sim(pm, { ...NO, right: true }, 120);
        res.moveDX = pm.x - x0; res.moveVx = pm.vx;
        res.swimTarget = cfg.WALK_SPEED * cfg.PPM * cfg.SWIM_FACTOR;
        res.wetRun = run ? run.len : 0;

        // drawLiquidFront(player): подкраска при голове под зеркалом, без броска.
        const draw = (player) => {
          const c = document.createElement('canvas'); c.width = 1280; c.height = 800;
          const ctx = c.getContext('2d');
          let threw = null;
          try { drawLiquidFront(ctx, w, { x: wx, y: w.baseY }, 1280, 800, null, player); } catch (e) { threw = String(e); }
          const img = ctx.getImageData(0, 0, 1280, 800).data;
          let painted = 0; for (let i = 3; i < img.length; i += 4) if (img[i] > 0) painted++;
          return { threw, painted };
        };
        const pUnder = new Player(w, 1);
        pUnder.x = wx; pUnder.y = lv + 80; pUnder.vx = 0; pUnder.vy = 0; pUnder.onGround = false;
        const pAbove = new Player(w, 1);
        pAbove.x = wx; pAbove.y = lv - 80; pAbove.vx = 0; pAbove.vy = 0; pAbove.onGround = false;
        res.frontUnder = draw(pUnder);
        res.frontAbove = draw(pAbove);
        res.headUnderProbe = w.liquidAt(pUnder.x, pUnder.y - pUnder.h / 2 + 1) !== '';
      }

      // --- подлёдные: вход через полынью ---
      {
        const w = mk(bv['подлёдные_океаны'], 424242);
        let px = -1;
        for (let x = 8; x < 40000 - 8; x++) {
          if (!w._liquidCol(x)) continue;
          let allPoly = true;
          for (let d = -7; d <= 7; d++) if (!w.polynyaAt(x + d)) { allPoly = false; break; }
          if (allPoly) { px = x; break; }
        }
        if (px >= 0) {
          const pp = new Player(w, 1);
          pp.x = px; pp.vx = 0; pp.vy = 0; pp.onGround = false; pp.y = w.liquidLevel(px) - 50;
          let entered = false;
          for (let i = 0; i < 400; i++) { pp.update(1 / 60, NO); if (w.liquidAt(pp.x, pp.y) !== '') { entered = true; break; } }
          res.iceEnter = entered; res.icePx = px; res.iceY = pp.y; res.iceLv = w.liquidLevel(px);
        } else {
          res.iceEnter = false; res.icePx = -1;
        }
      }

      return res;
    }, Object.fromEntries(views.map((v) => [v.id, v])));

    report('W1 вход в воду: режим плавания включается по liquidAt', out.inLiquid === true, `inLiquid=${out.inLiquid}`);
    report('W2 без down не тонет до дна (голова выше зеркала)', out.headUnder === false && out.yFloat < out.bd - 5,
      `head=${out.head.toFixed(1)} lv=${out.lv.toFixed(1)} y=${out.yFloat.toFixed(1)} bd=${out.bd.toFixed(1)}`);
    report('W3 с down ныряет ниже зеркала', out.diveUnder === true, `diveY=${out.diveY.toFixed(1)} lv=${out.lv.toFixed(1)}`);
    report('W4 перемещение в воде (v ≈ WALK·SWIM_FACTOR)', Math.abs(out.moveVx - out.swimTarget) < 4 && out.moveDX > 20,
      `vx=${out.moveVx.toFixed(1)} ждём ${out.swimTarget} dx=${out.moveDX.toFixed(1)} (run=${out.wetRun})`);
    report('W5 drawLiquidFront(player): подводная подкраска без броска',
      out.frontUnder.threw === null && out.headUnderProbe === true && out.frontUnder.painted > out.frontAbove.painted,
      `throw=${out.frontUnder.threw || 'нет'} paintedUnder=${out.frontUnder.painted} paintedAbove=${out.frontAbove.painted} headUnder=${out.headUnderProbe}`);
    report('W6 подлёдные: вход через полынью (liquidAt под коркой)', out.iceEnter === true,
      `px=${out.icePx} y=${Number(out.iceY).toFixed(1)} lv=${Number(out.iceLv).toFixed(1)}`);
    report('W7 0 pageerror', pageErrors.length === 0, pageErrors.slice(0, 2).join(' | '));
  } finally {
    await browser.close();
  }

  const failed = results.filter((r) => !r.ok);
  console.log(`\nSUMMARY: ${results.length - failed.length} PASS / ${failed.length} FAIL`);
  if (failed.length) process.exit(1);
}

main().catch((e) => { console.error(e); process.exit(1); });
