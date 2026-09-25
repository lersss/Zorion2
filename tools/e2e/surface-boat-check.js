// tools/e2e/surface-boat-check.js
// ЧК6.3 «Мир прогулки: лодка» — живой прогон модулей прогулки в браузере
// (образец tools/e2e/surface-swim-check.js). Проверяет НАСТОЯЩИЕ
// SurfaceWorld/Player/Boat/drawBoat (не node-копию):
//   * посадка у зеркала (dx=0), ход, скорость ×BOAT_FACTOR;
//   * мягкий стоп у края зеркала и твёрдого (полный силуэт);
//   * запрет лавы и underIce (включая полыньи);
//   * выход → Player.update (плавучесть);
//   * drawBoat рисует (фолбэк-силуэт), вне лодки — ничего; 0 pageerror; скриншот.
// Run: node surface-boat-check.js   (BASE_URL env)
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
  const views = resolveViews(['океаны', 'магмовый_океан', 'подлёдные_океаны']);
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
      const { Boat, drawBoat } = await import('/static/js/surface/surface_boat.js');
      const cfg = await import('/static/js/surface/surface_config.js');
      const mk = (b, seed) => new SurfaceWorld({
        seed, biome: b.id, biome_category: b.category, biome_color: b.color,
        life: true, view_source: 'catalog', view_version: 1, biome_view: b.view,
        liquid: b.view && b.view.liquid, liquid_source: 'explicit',
      });
      const NO = { left: false, right: false, jump: false, sprint: false, down: false };
      const longestWet = (w, minDepth) => {
        let best = null, s = -1; const end = 16000;
        for (let x = 0; x <= end; x++) {
          const wet = x < end && w.liquidDepth(x) >= minDepth;
          if (wet) { if (s < 0) s = x; }
          else if (s >= 0) { const len = x - s; if (!best || len > best.len) best = { s, e: x - 1, len, mid: Math.floor((s + x - 1) / 2) }; s = -1; }
        }
        return best;
      };
      const LV = 360;
      const fakeWorld = ({ lv = LV, dry = [], wetLo = 0, wetHi = 4000, depth = 40, solid = null, medium = 'вода', mode = 'global' } = {}) => {
        const drySet = new Set(dry);
        return {
          liquid: { medium, level: { mode } },
          liquidLevel(x) { return (x >= wetLo && x < wetHi) ? lv : Infinity; },
          _liquidCol(x) { if (x < wetLo || x >= wetHi) return null; if (drySet.has(x)) return null; return { lv, depth, bd: lv + depth }; },
          crustTop() { return null; },
          crustAt() { return false; },
          solidAt(x, y) { return solid ? solid(x, y) : false; },
        };
      };
      const stub = (x, y) => ({ x, y, h: cfg.PLAYER_H, vx: 0, vy: 0, facing: 1, distance: 0 });
      const res = {};

      // --- океаны: посадка, ход, скорость, отрисовка, выход ---
      {
        const w = mk(bv['океаны'], 424242);
        const run = longestWet(w, 40);
        const wx = run ? run.mid : 0;
        const p = new Player(w, 1);
        p.x = wx; p.vx = 0; p.vy = 0; p.onGround = false; p.y = w.liquidLevel(wx) + 7;
        const boat = new Boat(w);
        res.dx = boat.canBoard(w, p);
        res.boarded = boat.board(p);
        res.yLevel = Math.abs(p.y - (w.liquidLevel(boat.x) - cfg.BOAT_HULL)) < 1e-6;
        let vmax = 0;
        for (let i = 0; i < 120; i++) { boat.update(1 / 60, { ...NO, right: true }, p); vmax = Math.max(vmax, Math.abs(boat.vx)); }
        res.vmax = vmax; res.vTarget = cfg.WALK_SPEED * cfg.PPM * cfg.BOAT_FACTOR; res.dist = p.distance;
        // drawBoat: фолбэк-силуэт (спрайта нет) — пиксели есть; вне лодки — пусто.
        const draw = (aboard) => {
          const c = document.createElement('canvas'); c.width = 1280; c.height = 800;
          const ctx = c.getContext('2d');
          if (!aboard) boat.aboard = false;
          else boat.aboard = true;
          let threw = null;
          try { drawBoat(ctx, w, { x: boat.x, y: w.baseY }, 1280, 800, p, boat, 1000); } catch (e) { threw = String(e); }
          const img = ctx.getImageData(0, 0, 1280, 800).data;
          let painted = 0; for (let i = 3; i < img.length; i += 4) if (img[i] > 0) painted++;
          return { threw, painted };
        };
        res.drawAboard = draw(true);
        res.drawAway = draw(false);
        boat.aboard = true;
        boat.disembark(p);
        res.afterOut = !boat.aboard;
        for (let i = 0; i < 240; i++) p.update(1 / 60, NO);
        res.headAbove = (p.y - p.h / 2) < w.liquidLevel(p.x) - 0.5;
      }

      // --- мягкий стоп: край зеркала и твёрдое (полный силуэт) ---
      {
        const w = fakeWorld({ wetLo: 0, wetHi: 1500, depth: 40 });
        const boat = new Boat(w);
        boat.aboard = true; boat.x = 1000; boat.vx = 0;
        for (let i = 0; i < 600; i++) boat.update(1 / 60, { ...NO, right: true }, stub(1000, LV - cfg.BOAT_HULL));
        res.edgeX = boat.x; res.edgeVx = boat.vx;
        const ws = fakeWorld({ wetLo: 0, wetHi: 3000, depth: 40, solid: (x, y) => x >= 1500 && y <= LV - 5 });
        const bs = new Boat(ws);
        bs.aboard = true; bs.x = 1000; bs.vx = 0;
        let pen = false;
        for (let i = 0; i < 600; i++) {
          bs.update(1 / 60, { ...NO, right: true }, stub(1000, LV));
          for (let o = -17; o <= 17; o += 4) if (ws.solidAt(bs.x + o, LV - 10)) pen = true;
        }
        res.solidX = bs.x; res.solidPen = pen;
      }

      // --- запреты: лава и underIce (в полынье) ---
      {
        const lava = mk(bv['магмовый_океан'], 424242);
        const lr = longestWet(lava, 40); const lx = lr ? lr.mid : 0;
        const lp = new Player(lava, 1); lp.x = lx; lp.y = lava.liquidLevel(lx) + 7; lp.vx = 0; lp.vy = 0; lp.onGround = false;
        res.lavaDx = new Boat(lava).canBoard(lava, lp);

        const ice = mk(bv['подлёдные_океаны'], 424242);
        let ix = -1;
        for (let x = 8; x < 40000 - 8; x++) {
          if (!ice._liquidCol(x)) continue;
          let all = true;
          for (let d = -7; d <= 7; d++) if (!ice.polynyaAt(x + d)) { all = false; break; }
          if (all) { ix = x; break; }
        }
        const ip = new Player(ice, 1); ip.x = ix >= 0 ? ix : ice.spawnX; ip.y = ice.liquidLevel(ip.x) + 7; ip.vx = 0; ip.vy = 0; ip.onGround = false;
        res.iceDx = new Boat(ice).canBoard(ice, ip);
        res.icePoly = ix;
      }
      return res;
    }, Object.fromEntries(views.map((v) => [v.id, v])));

    report('E1 посадка у зеркала (dx=0)', out.dx === 0 && out.boarded === true, `canBoard=${out.dx} boarded=${out.boarded}`);
    report('E2 уровень y = liquidLevel − BOAT_HULL', out.yLevel === true, `yLevel=${out.yLevel}`);
    report('E3 скорость ≈ WALK·BOAT_FACTOR', Math.abs(out.vmax - out.vTarget) < 3 && out.dist > 50,
      `vmax=${out.vmax.toFixed(1)} ждём ${out.vTarget.toFixed(0)} dist=${out.dist.toFixed(1)}`);
    report('E4 drawBoat рисует (фолбэк-силуэт), вне лодки — ничего',
      out.drawAboard.threw === null && out.drawAboard.painted > 0 && out.drawAway.painted === 0,
      `throw=${out.drawAboard.threw || 'нет'} paintedAboard=${out.drawAboard.painted} paintedAway=${out.drawAway.painted}`);
    report('E5 выход → Player.update (плавучесть)', out.afterOut === true && out.headAbove === true,
      `afterOut=${out.afterOut} headAbove=${out.headAbove}`);
    report('E6 мягкий стоп: край зеркала', out.edgeVx === 0 && out.edgeX + 17 < 1500 && out.edgeX > 1400,
      `x=${out.edgeX.toFixed(2)} vx=${out.edgeVx}`);
    report('E7 мягкий стоп: твёрдое (полный силуэт)', out.solidPen === false && out.solidX + 17 < 1500,
      `x=${out.solidX.toFixed(2)} penetrated=${out.solidPen}`);
    report('E8 запрет лавы', out.lavaDx === null, `canBoard=${out.lavaDx}`);
    report('E9 запрет underIce (в полынье)', out.iceDx === null, `polynya=${out.icePoly} canBoard=${out.iceDx}`);
    report('E10 0 pageerror', pageErrors.length === 0, pageErrors.slice(0, 2).join(' | '));

    const shotPath = path.join(ARTIFACTS_DIR, 'surface-boat-check.png');
    await page.screenshot({ path: shotPath });
    console.log('screenshot: ' + shotPath);
  } finally {
    await browser.close();
  }

  const failed = results.filter((r) => !r.ok);
  console.log(`\nSUMMARY: ${results.length - failed.length} PASS / ${failed.length} FAIL`);
  if (failed.length) process.exit(1);
}

main().catch((e) => { console.error(e); process.exit(1); });
